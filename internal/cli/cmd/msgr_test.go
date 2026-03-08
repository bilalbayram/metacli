package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilalbayram/metacli/internal/graph"
)

func TestNewMSGRCommandIncludesHealthSubcommand(t *testing.T) {
	t.Parallel()

	cmd := NewMSGRCommand(Runtime{})
	if cmd.Name() != "msgr" {
		t.Fatalf("expected msgr command name, got %q", cmd.Name())
	}

	foundHealth := false
	for _, subcommand := range cmd.Commands() {
		if subcommand.Name() == "health" {
			foundHealth = true
			break
		}
	}
	if !foundHealth {
		t.Fatal("expected msgr command to include health subcommand")
	}
}

func TestMSGRHealthCommandWritesSuccessEnvelope(t *testing.T) {
	t.Parallel()

	output := &bytes.Buffer{}
	format := "json"
	runtime := Runtime{Output: &format}
	cmd := NewMSGRCommand(runtime)
	cmd.SetOut(output)
	cmd.SetErr(output)
	cmd.SetArgs([]string{"health"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute msgr health: %v", err)
	}

	var envelope map[string]any
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if envelope["command"] != "meta msgr health" {
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

func TestMSGRCommandFailsWithoutSubcommand(t *testing.T) {
	t.Parallel()

	cmd := NewMSGRCommand(Runtime{})
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when msgr is executed without subcommand")
	}
	if !strings.Contains(err.Error(), "msgr requires a subcommand") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func useMSGRDependencies(t *testing.T, loadFn func(string) (*ProfileCredentials, error), clientFn func() *graph.Client) {
	t.Helper()
	originalLoad := msgrLoadProfileCredentials
	originalClient := msgrNewGraphClient
	originalPreflight := profileAuthPreflight
	t.Cleanup(func() {
		msgrLoadProfileCredentials = originalLoad
		msgrNewGraphClient = originalClient
		profileAuthPreflight = originalPreflight
	})

	msgrLoadProfileCredentials = loadFn
	msgrNewGraphClient = clientFn
	profileAuthPreflight = func(string, []string, string) error {
		return nil
	}
}
