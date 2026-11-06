package main

import (
	"flag"
	"github.com/lorvanto/monitor-go/internal/app"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestConsoleDestinationDefaultsAndOverrides(t *testing.T) {
	for _, tc := range []struct {
		name     string
		envSet   bool
		env      string
		args     []string
		expected string
	}{
		{name: "built-in destination", expected: "https://console.i7f.cn"},
		{name: "explicit empty environment", envSet: true, expected: ""},
		{name: "custom environment", envSet: true, env: "https://console.example.com", expected: "https://console.example.com"},
		{name: "empty flag overrides default", args: []string{"-console-url="}, expected: ""},
		{name: "flag overrides environment", envSet: true, env: "https://console.example.com", args: []string{"-console-url=https://other.example.com"}, expected: "https://other.example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MONITOR_CONSOLE_URL", tc.env)
			if !tc.envSet {
				if err := os.Unsetenv("MONITOR_CONSOLE_URL"); err != nil {
					t.Fatal(err)
				}
			}
			flags := flag.NewFlagSet("monitor", flag.ContinueOnError)
			url := flags.String("console-url", defaultConsoleURL(), "")
			if err := flags.Parse(tc.args); err != nil {
				t.Fatal(err)
			}
			if *url != tc.expected {
				t.Fatalf("console URL = %q, want %q", *url, tc.expected)
			}
		})
	}
}

func TestRequiredReportingRejectsLegacyOverrides(t *testing.T) {
	for _, tc := range []struct {
		name, env   string
		unset, flag bool
		url         string
		wantError   bool
	}{
		{name: "default", unset: true, flag: true, url: "https://console.example.com"},
		{name: "true environment", env: "true", flag: true, url: "https://console.example.com"},
		{name: "false environment", env: "false", flag: true, url: "https://console.example.com", wantError: true},
		{name: "false flag", env: "true", flag: false, url: "https://console.example.com", wantError: true},
		{name: "invalid environment", env: "typo", flag: true, url: "https://console.example.com", wantError: true},
		{name: "empty environment", env: "", flag: true, url: "https://console.example.com", wantError: true},
		{name: "empty destination", env: "true", flag: true, url: "", wantError: true},
		{name: "blank destination", env: "true", flag: true, url: "  ", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MONITOR_REQUIRE_ACTIVATION", tc.env)
			if tc.unset {
				os.Unsetenv("MONITOR_REQUIRE_ACTIVATION")
			}
			err := validateRequiredReporting(tc.flag, tc.url)
			if (err != nil) != tc.wantError {
				t.Fatalf("configuration error=%v wantError=%v", err, tc.wantError)
			}
		})
	}
}

func TestMonitorCLIHelperProcess(t *testing.T) {
	if os.Getenv("MONITOR_CLI_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"monitor"}, os.Args[i+1:]...)
			main()
			os.Exit(0)
		}
	}
	os.Exit(99)
}
func invokeMonitorCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	command := exec.Command(os.Args[0], append([]string{"-test.run=^TestMonitorCLIHelperProcess$", "--"}, args...)...)
	command.Env = append(os.Environ(), "MONITOR_CLI_HELPER=1")
	output, err := command.CombinedOutput()
	return string(output), err
}
func TestMonitorCLIFalseAndEmptyDestinationFailBeforeOpeningData(t *testing.T) {
	for _, tc := range []struct {
		name, env, url string
		args           []string
	}{
		{name: "false env", env: "false", url: "https://console.example.com"},
		{name: "false flag", env: "true", url: "https://console.example.com", args: []string{"-require-activation=false"}},
		{name: "empty env destination", env: "true", url: ""},
		{name: "empty flag destination", env: "true", url: "https://console.example.com", args: []string{"-console-url="}},
		{name: "blank flag destination", env: "true", url: "https://console.example.com", args: []string{"-console-url= "}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MONITOR_REQUIRE_ACTIVATION", tc.env)
			t.Setenv("MONITOR_CONSOLE_URL", tc.url)
			data := filepath.Join(t.TempDir(), "must-not-exist")
			output, err := invokeMonitorCLI(t, append([]string{"-data=" + data}, tc.args...)...)
			code, ok := err.(*exec.ExitError)
			if !ok || code.ExitCode() != 2 || !strings.Contains(output, "required") {
				t.Fatalf("CLI accepted invalid configuration: %v %s", err, output)
			}
			if _, err := os.Stat(data); !os.IsNotExist(err) {
				t.Fatal("invalid CLI configuration touched data directory")
			}
		})
	}
}
func TestMonitorVersionAndBackupIgnoreActivationConfiguration(t *testing.T) {
	t.Setenv("MONITOR_REQUIRE_ACTIVATION", "false")
	t.Setenv("MONITOR_CONSOLE_URL", "")
	output, err := invokeMonitorCLI(t, "-version", "-require-activation=false")
	if err != nil || strings.TrimSpace(output) == "" {
		t.Fatalf("version was blocked: %v %s", err, output)
	}
	data := filepath.Join(t.TempDir(), "data")
	store, err := app.OpenStore(data)
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	backup := filepath.Join(t.TempDir(), "backup")
	output, err = invokeMonitorCLI(t, "backup", "-data", data, "-out", backup)
	if err != nil || !strings.Contains(output, "Backup complete:") {
		t.Fatalf("backup was blocked: %v %s", err, output)
	}
}
