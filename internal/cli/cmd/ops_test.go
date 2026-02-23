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
	expectedState := "{\n  \"schema_version\": 1,\n  \"baseline_version\": 3,\n  \"status\": \"initialized\",\n  \"snapshots\": {\n    \"changelog_occ\": {\n      \"latest_version\": \"v25.0\",\n      \"occ_digest\": \"occ.2025.stable\"\n    },\n    \"schema_pack\": {\n      \"domain\": \"marketing\",\n      \"version\": \"v25.0\",\n      \"sha256\": \"432a308e09cb9e1c40c03e992a0f28d70600954f2cb1c939959512a1660a6774\"\n    }\n  }\n}\n"
	if string(rawState) != expectedState {
		t.Fatalf("unexpected state file contents:\n%s", string(rawState))
	}
}

func TestOpsRunCommandWritesReportWithChecks(t *testing.T) {
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
	if len(data.Report.Checks) != 2 {
		t.Fatalf("expected two checks, got %d", len(data.Report.Checks))
	}
	if data.Report.Checks[0].Name != "changelog_occ_delta" {
		t.Fatalf("unexpected first check name: %s", data.Report.Checks[0].Name)
	}
	if data.Report.Checks[1].Name != "schema_pack_drift" {
		t.Fatalf("unexpected second check name: %s", data.Report.Checks[1].Name)
	}
	for _, check := range data.Report.Checks {
		if check.Status != ops.CheckStatusPass {
			t.Fatalf("unexpected check status: %s", check.Status)
		}
		if check.Blocking {
			t.Fatal("expected non-blocking checks")
		}
	}
	if data.Report.Summary.Total != 2 || data.Report.Summary.Passed != 2 || data.Report.Summary.Failed != 0 || data.Report.Summary.Blocking != 0 {
		t.Fatalf("unexpected summary values: %+v", data.Report.Summary)
	}
}

func TestOpsRunCommandReturnsPolicyExitOnBlockingFindings(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := ops.Initialize(statePath); err != nil {
		t.Fatalf("initialize baseline state: %v", err)
	}

	state, err := ops.LoadBaseline(statePath)
	if err != nil {
		t.Fatalf("load baseline state: %v", err)
	}
	state.Snapshots.ChangelogOCC.LatestVersion = "v24.0"
	if err := ops.SaveBaseline(statePath, state); err != nil {
		t.Fatalf("save baseline state: %v", err)
	}

	stdout, stderr, err := executeOpsCommand(Runtime{}, "run", "--state-path", statePath)
	if err == nil {
		t.Fatal("expected blocking ops run to fail")
	}
	var exitErr *ops.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != ops.ExitCodePolicy {
		t.Fatalf("unexpected exit code: got=%d want=%d", exitErr.Code, ops.ExitCodePolicy)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	envelope := decodeOpsEnvelope(t, []byte(stdout))
	if envelope.Success {
		t.Fatal("expected success=false")
	}
	if envelope.ExitCode != ops.ExitCodePolicy {
		t.Fatalf("unexpected envelope exit code: got=%d want=%d", envelope.ExitCode, ops.ExitCodePolicy)
	}
	if envelope.Error == nil {
		t.Fatal("expected error payload")
	}
	if envelope.Error.Type != "blocking_findings" {
		t.Fatalf("unexpected error type: %s", envelope.Error.Type)
	}

	var data ops.RunResult
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatalf("decode run data: %v", err)
	}
	if data.Report.Summary.Blocking != 1 {
		t.Fatalf("unexpected blocking summary: %+v", data.Report.Summary)
	}
}

func TestOpsRunCommandReturnsPolicyExitOnSchemaPackDrift(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := ops.Initialize(statePath); err != nil {
		t.Fatalf("initialize baseline state: %v", err)
	}

	state, err := ops.LoadBaseline(statePath)
	if err != nil {
		t.Fatalf("load baseline state: %v", err)
	}
	state.Snapshots.SchemaPack.SHA256 = "drifted"
	if err := ops.SaveBaseline(statePath, state); err != nil {
		t.Fatalf("save baseline state: %v", err)
	}

	stdout, stderr, err := executeOpsCommand(Runtime{}, "run", "--state-path", statePath)
	if err == nil {
		t.Fatal("expected schema-drift ops run to fail")
	}
	var exitErr *ops.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != ops.ExitCodePolicy {
		t.Fatalf("unexpected exit code: got=%d want=%d", exitErr.Code, ops.ExitCodePolicy)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	envelope := decodeOpsEnvelope(t, []byte(stdout))
	if envelope.Success {
		t.Fatal("expected success=false")
	}

	var data ops.RunResult
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatalf("decode run data: %v", err)
	}
	if data.Report.Summary.Blocking != 1 {
		t.Fatalf("unexpected blocking summary: %+v", data.Report.Summary)
	}
	if data.Report.Checks[1].Name != "schema_pack_drift" {
		t.Fatalf("unexpected schema check name: %s", data.Report.Checks[1].Name)
	}
	if data.Report.Checks[1].Status != ops.CheckStatusFail {
		t.Fatalf("unexpected schema check status: %s", data.Report.Checks[1].Status)
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
