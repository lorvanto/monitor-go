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
