package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNewIGCommandIncludesHealthSubcommand(t *testing.T) {
	t.Parallel()

	cmd := NewIGCommand(Runtime{})
	if cmd.Name() != "ig" {
		t.Fatalf("expected ig command name, got %q", cmd.Name())
	}

	foundHealth := false
	for _, subcommand := range cmd.Commands() {
		if subcommand.Name() == "health" {
			foundHealth = true
			break
		}
	}
	if !foundHealth {
		t.Fatal("expected ig command to include health subcommand")
	}
}

func TestIGHealthCommandWritesSuccessEnvelope(t *testing.T) {
	t.Parallel()

	output := &bytes.Buffer{}
	format := "json"
	runtime := Runtime{Output: &format}
	cmd := NewIGCommand(runtime)
	cmd.SetOut(output)
	cmd.SetErr(output)
	cmd.SetArgs([]string{"health"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute ig health: %v", err)
	}

	var envelope map[string]any
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if envelope["command"] != "meta ig health" {
		t.Fatalf("unexpected command field: %v", envelope["command"])
	}
	if envelope["success"] != true {
		t.Fatalf("expected success=true, got %v", envelope["success"])
	}

	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %#v", envelope["data"])
	}
	if data["status"] != "ok" {
		t.Fatalf("expected status=ok, got %v", data["status"])
	}
}

func TestIGCommandFailsWithoutSubcommand(t *testing.T) {
	t.Parallel()

	cmd := NewIGCommand(Runtime{})
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when ig is executed without subcommand")
	}
	if !strings.Contains(err.Error(), "ig requires a subcommand") {
		t.Fatalf("unexpected error: %v", err)
	}
}
