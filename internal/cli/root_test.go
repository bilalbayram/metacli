package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootRegistersDoctorCommand(t *testing.T) {
	root := NewRootCommand()

	doctorCmd, _, err := root.Find([]string{"doctor"})
	if err != nil {
		t.Fatalf("find doctor command: %v", err)
	}
	if doctorCmd == nil || doctorCmd.Name() != "doctor" {
		t.Fatalf("expected doctor command, got %#v", doctorCmd)
	}

	tracerCmd, _, err := root.Find([]string{"doctor", "tracer"})
	if err != nil {
		t.Fatalf("find doctor tracer command: %v", err)
	}
	if tracerCmd == nil || tracerCmd.Name() != "tracer" {
		t.Fatalf("expected tracer command, got %#v", tracerCmd)
	}
}

func TestRootVersionFlags(t *testing.T) {
	t.Parallel()

	for _, flag := range []string{"--version", "-v"} {
		flag := flag
		t.Run(flag, func(t *testing.T) {
			t.Parallel()

			root := NewRootCommand()
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			root.SetOut(stdout)
			root.SetErr(stderr)
			root.SetArgs([]string{flag})

			if err := root.Execute(); err != nil {
				t.Fatalf("execute %s: %v", flag, err)
			}

			if got := strings.TrimSpace(stdout.String()); got != Version {
				t.Fatalf("unexpected version output: got %q want %q", got, Version)
			}
			if stderr.Len() != 0 {
				t.Fatalf("expected empty stderr for %s, got %q", flag, stderr.String())
			}
		})
	}
}

func TestSubcommandRequiredErrorsPrintHelp(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		args        []string
		errorString string
		usagePrefix string
	}{
		{
			name:        "ig",
			args:        []string{"ig"},
			errorString: "ig requires a subcommand",
			usagePrefix: "meta ig",
		},
		{
			name:        "ig_media",
			args:        []string{"ig", "media"},
			errorString: "ig media requires a subcommand",
			usagePrefix: "meta ig media",
		},
		{
			name:        "ig_publish_schedule",
			args:        []string{"ig", "publish", "schedule"},
			errorString: "ig publish schedule requires a subcommand",
			usagePrefix: "meta ig publish schedule",
		},
		{
			name:        "doctor",
			args:        []string{"doctor"},
			errorString: "doctor requires a subcommand",
			usagePrefix: "meta doctor",
		},
		{
			name:        "wa",
			args:        []string{"wa"},
			errorString: "wa requires a subcommand",
			usagePrefix: "meta wa",
		},
		{
			name:        "campaign",
			args:        []string{"campaign"},
			errorString: "campaign requires a subcommand",
			usagePrefix: "meta campaign",
		},
		{
			name:        "adset",
			args:        []string{"adset"},
			errorString: "adset requires a subcommand",
			usagePrefix: "meta adset",
		},
		{
			name:        "ad",
			args:        []string{"ad"},
			errorString: "ad requires a subcommand",
			usagePrefix: "meta ad",
		},
		{
			name:        "audience",
			args:        []string{"audience"},
			errorString: "audience requires a subcommand",
			usagePrefix: "meta audience",
		},
		{
			name:        "creative",
			args:        []string{"creative"},
			errorString: "creative requires a subcommand",
			usagePrefix: "meta creative",
		},
		{
			name:        "catalog",
			args:        []string{"catalog"},
			errorString: "catalog requires a subcommand",
			usagePrefix: "meta catalog",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := NewRootCommand()
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			root.SetOut(stdout)
			root.SetErr(stderr)
			root.SetArgs(tc.args)

			err := root.Execute()
			if err == nil {
				t.Fatalf("expected error for %v", tc.args)
			}
			if !strings.Contains(err.Error(), tc.errorString) {
				t.Fatalf("unexpected error for %v: %v", tc.args, err)
			}

			errOutput := stderr.String()
			if !strings.Contains(errOutput, tc.errorString) {
				t.Fatalf("expected stderr to include %q, got %q", tc.errorString, errOutput)
			}
			if !strings.Contains(errOutput, "Usage:") {
				t.Fatalf("expected stderr to include usage, got %q", errOutput)
			}
			if !strings.Contains(errOutput, tc.usagePrefix) {
				t.Fatalf("expected stderr to include %q usage, got %q", tc.usagePrefix, errOutput)
			}
			if stdout.Len() != 0 {
				t.Fatalf("expected empty stdout for %v, got %q", tc.args, stdout.String())
			}
		})
	}
}
