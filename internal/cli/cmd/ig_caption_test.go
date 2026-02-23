package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewIGCommandIncludesCaptionValidateSubcommand(t *testing.T) {
	cmd := NewIGCommand(Runtime{})

	captionCmd, _, err := cmd.Find([]string{"caption"})
	if err != nil {
		t.Fatalf("find caption command: %v", err)
	}
	if captionCmd == nil || captionCmd.Name() != "caption" {
		t.Fatalf("expected caption command, got %#v", captionCmd)
	}

	validateCmd, _, err := cmd.Find([]string{"caption", "validate"})
	if err != nil {
		t.Fatalf("find caption validate command: %v", err)
	}
	if validateCmd == nil || validateCmd.Name() != "validate" {
		t.Fatalf("expected caption validate command, got %#v", validateCmd)
	}
}

func TestIGCaptionValidateWritesSuccessEnvelope(t *testing.T) {
	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewIGCommand(Runtime{})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{"caption", "validate", "--caption", "hello #meta"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute caption validate: %v", err)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta ig caption validate")
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object payload, got %T", envelope["data"])
	}
	if valid, _ := data["valid"].(bool); !valid {
		t.Fatalf("expected valid result, got %v", data["valid"])
	}
	if hashtags, _ := data["hashtag_count"].(float64); hashtags != 1 {
		t.Fatalf("expected hashtag_count=1, got %v", hashtags)
	}
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestIGCaptionValidateStrictModeFailsOnWarnings(t *testing.T) {
	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewIGCommand(Runtime{})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"caption", "validate",
		"--caption", strings.Repeat("a", 2005),
		"--strict",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "strict mode:") {
		t.Fatalf("unexpected error: %v", err)
	}

	envelope := decodeEnvelope(t, errOutput.Bytes())
	if got := envelope["command"]; got != "meta ig caption validate" {
		t.Fatalf("unexpected command %v", got)
	}
	if envelope["success"] != false {
		t.Fatalf("expected success=false, got %v", envelope["success"])
	}
}

func TestIGCaptionValidateFailsWithoutCaption(t *testing.T) {
	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewIGCommand(Runtime{})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{"caption", "validate"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "caption is required") {
		t.Fatalf("unexpected error: %v", err)
	}

	envelope := decodeEnvelope(t, errOutput.Bytes())
	if got := envelope["command"]; got != "meta ig caption validate" {
		t.Fatalf("unexpected command %v", got)
	}
	if envelope["success"] != false {
		t.Fatalf("expected success=false, got %v", envelope["success"])
	}
}

func TestIGCaptionCommandFailsWithoutSubcommand(t *testing.T) {
	cmd := NewIGCommand(Runtime{})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"caption"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "ig caption requires a subcommand") {
		t.Fatalf("unexpected error: %v", err)
	}
}
