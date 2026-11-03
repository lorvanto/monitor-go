package main

import (
	"flag"
	"os"
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
		{name: "built-in destination", expected: "https://feiniu.i7f.cn:5090"},
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

func TestActivationDefaultsAndExplicitOfflineOverrides(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  *string
		args []string
		want bool
	}{
		{name: "default requires activation", want: true},
		{name: "offline environment", env: newString("false"), want: false},
		{name: "required environment", env: newString("true"), want: true},
		{name: "offline flag", args: []string{"-require-activation=false"}, want: false},
		{name: "flag overrides offline environment", env: newString("false"), args: []string{"-require-activation=true"}, want: true},
		{name: "offline flag overrides required environment", env: newString("true"), args: []string{"-require-activation=false"}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MONITOR_REQUIRE_ACTIVATION", "true")
			if tc.env == nil {
				os.Unsetenv("MONITOR_REQUIRE_ACTIVATION")
			} else {
				t.Setenv("MONITOR_REQUIRE_ACTIVATION", *tc.env)
			}
			value, err := defaultRequireActivation()
			if err != nil {
				t.Fatal(err)
			}
			flags := flag.NewFlagSet("monitor", flag.ContinueOnError)
			required := flags.Bool("require-activation", value, "")
			if err := flags.Parse(tc.args); err != nil {
				t.Fatal(err)
			}
			if *required != tc.want {
				t.Fatalf("require activation=%v, expected %v", *required, tc.want)
			}
		})
	}
	for _, invalid := range []string{"", "disabled", "typo"} {
		t.Setenv("MONITOR_REQUIRE_ACTIVATION", invalid)
		if _, err := defaultRequireActivation(); err == nil {
			t.Fatal("invalid environment silently disabled activation")
		}
	}
}

func newString(value string) *string { return &value }
