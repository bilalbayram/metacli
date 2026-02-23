package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/bilalbayram/metacli/internal/ops"
)

type envelopeFixture struct {
	ContractVersion string          `json:"contract_version"`
	Command         string          `json:"command"`
	Success         bool            `json:"success"`
	ExitCode        int             `json:"exit_code"`
	Data            json.RawMessage `json:"data"`
	Error           *errorFixture   `json:"error"`
}

type errorFixture struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func TestOpsInitCommandWritesSuccessEnvelope(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "baseline-state.json")
	stdout, stderr, err := executeOpsCommand(Runtime{}, "init", "--state-path", statePath)
	if err != nil {
		t.Fatalf("execute ops init: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	envelope := decodeOpsEnvelope(t, []byte(stdout))
	if envelope.ContractVersion != ops.ContractVersion {
		t.Fatalf("unexpected contract version: %s", envelope.ContractVersion)
	}
	if envelope.Command != ops.CommandInit {
		t.Fatalf("unexpected command: %s", envelope.Command)
	}
	if !envelope.Success {
		t.Fatal("expected success=true")
	}
	if envelope.ExitCode != ops.ExitCodeSuccess {
		t.Fatalf("unexpected exit code: %d", envelope.ExitCode)
	}
	if envelope.Error != nil {
		t.Fatalf("unexpected error payload: %+v", envelope.Error)
	}

	var data ops.InitResult
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatalf("decode init data: %v", err)
	}
	if data.StatePath != statePath {
		t.Fatalf("unexpected state path: got=%s want=%s", data.StatePath, statePath)
	}
	if data.State.SchemaVersion != ops.StateSchemaVersion {
		t.Fatalf("unexpected state schema version: %d", data.State.SchemaVersion)
	}
	if data.State.BaselineVersion != ops.BaselineVersion {
		t.Fatalf("unexpected baseline version: %d", data.State.BaselineVersion)
	}
	if data.State.Status != "initialized" {
		t.Fatalf("unexpected state status: %s", data.State.Status)
	}

	rawState, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}
	expectedState := "{\n  \"schema_version\": 1,\n  \"baseline_version\": 1,\n  \"status\": \"initialized\"\n}\n"
	if string(rawState) != expectedState {
		t.Fatalf("unexpected state file contents:\n%s", string(rawState))
	}
}

func TestOpsRunCommandWritesReportSkeleton(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := ops.Initialize(statePath); err != nil {
		t.Fatalf("initialize baseline state: %v", err)
	}

	stdout, stderr, err := executeOpsCommand(Runtime{}, "run", "--state-path", statePath)
	if err != nil {
		t.Fatalf("execute ops run: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	envelope := decodeOpsEnvelope(t, []byte(stdout))
	if envelope.Command != ops.CommandRun {
		t.Fatalf("unexpected command: %s", envelope.Command)
	}
	if !envelope.Success {
		t.Fatal("expected success=true")
	}
	if envelope.ExitCode != ops.ExitCodeSuccess {
		t.Fatalf("unexpected exit code: %d", envelope.ExitCode)
	}

	var data ops.RunResult
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatalf("decode run data: %v", err)
	}
	if data.StatePath != statePath {
		t.Fatalf("unexpected state path: got=%s want=%s", data.StatePath, statePath)
	}
	if data.Report.SchemaVersion != ops.ReportSchemaVersion {
		t.Fatalf("unexpected report schema version: %d", data.Report.SchemaVersion)
	}
	if data.Report.Kind != "ops_report" {
		t.Fatalf("unexpected report kind: %s", data.Report.Kind)
	}
	if len(data.Report.Checks) != 0 {
		t.Fatalf("expected no checks, got %d", len(data.Report.Checks))
	}
	if data.Report.Summary.Total != 0 || data.Report.Summary.Passed != 0 || data.Report.Summary.Failed != 0 {
		t.Fatalf("expected zero summary values, got %+v", data.Report.Summary)
	}
}

func TestOpsRunCommandReturnsStateExitOnMissingBaseline(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "missing-baseline.json")
	stdout, stderr, err := executeOpsCommand(Runtime{}, "run", "--state-path", statePath)
	if err == nil {
		t.Fatal("expected ops run to fail")
	}

	var exitErr *ops.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != ops.ExitCodeState {
		t.Fatalf("unexpected exit code: got=%d want=%d", exitErr.Code, ops.ExitCodeState)
	}
	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}

	envelope := decodeOpsEnvelope(t, []byte(stderr))
	if envelope.Command != ops.CommandRun {
		t.Fatalf("unexpected command: %s", envelope.Command)
	}
	if envelope.Success {
		t.Fatal("expected success=false")
	}
	if envelope.ExitCode != ops.ExitCodeState {
		t.Fatalf("unexpected envelope exit code: got=%d want=%d", envelope.ExitCode, ops.ExitCodeState)
	}
	if envelope.Error == nil {
		t.Fatal("expected error payload")
	}
	if envelope.Error.Type != "state_error" {
		t.Fatalf("unexpected error type: %s", envelope.Error.Type)
	}
}

func executeOpsCommand(runtime Runtime, args ...string) (string, string, error) {
	cmd := NewOpsCommand(runtime)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func decodeOpsEnvelope(t *testing.T, raw []byte) envelopeFixture {
	t.Helper()

	var envelope envelopeFixture
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode envelope: %v\nraw=%s", err, string(raw))
	}
	return envelope
}
