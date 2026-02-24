package cmd

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

type runSectionLineFixture struct {
	StatePath           string            `json:"state_path"`
	ReportSchemaVersion int               `json:"report_schema_version"`
	ReportKind          string            `json:"report_kind"`
	ReportOutcome       string            `json:"report_outcome"`
	Section             ops.ReportSection `json:"section"`
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
	expectedState := "{\n  \"schema_version\": 1,\n  \"baseline_version\": 4,\n  \"status\": \"initialized\",\n  \"snapshots\": {\n    \"changelog_occ\": {\n      \"latest_version\": \"v25.0\",\n      \"occ_digest\": \"occ.2025.stable\"\n    },\n    \"schema_pack\": {\n      \"domain\": \"marketing\",\n      \"version\": \"v25.0\",\n      \"sha256\": \"94d9287ab1d2445304fdbf6e5fb9a09e2e4cec9f3c655337e7d33a0114793529\"\n    },\n    \"rate_limit\": {\n      \"app_call_count\": 0,\n      \"app_total_cputime\": 0,\n      \"app_total_time\": 0,\n      \"page_call_count\": 0,\n      \"page_total_cputime\": 0,\n      \"page_total_time\": 0,\n      \"ad_account_util_pct\": 0\n    }\n  }\n}\n"
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
	if len(data.Report.Checks) != 5 {
		t.Fatalf("expected five checks, got %d", len(data.Report.Checks))
	}
	if data.Report.Checks[0].Name != "changelog_occ_delta" {
		t.Fatalf("unexpected first check name: %s", data.Report.Checks[0].Name)
	}
	if data.Report.Checks[1].Name != "schema_pack_drift" {
		t.Fatalf("unexpected second check name: %s", data.Report.Checks[1].Name)
	}
	if data.Report.Checks[2].Name != "rate_limit_threshold" {
		t.Fatalf("unexpected third check name: %s", data.Report.Checks[2].Name)
	}
	if data.Report.Checks[3].Name != "permission_policy_preflight" {
		t.Fatalf("unexpected fourth check name: %s", data.Report.Checks[3].Name)
	}
	if data.Report.Checks[4].Name != "runtime_response_shape_drift" {
		t.Fatalf("unexpected fifth check name: %s", data.Report.Checks[4].Name)
	}
	for _, check := range data.Report.Checks {
		if check.Status != ops.CheckStatusPass {
			t.Fatalf("unexpected check status: %s", check.Status)
		}
		if check.Blocking {
			t.Fatal("expected non-blocking checks")
		}
	}
	if data.Report.Summary.Total != 5 || data.Report.Summary.Passed != 5 || data.Report.Summary.Failed != 0 || data.Report.Summary.Warnings != 0 || data.Report.Summary.Blocking != 0 {
		t.Fatalf("unexpected summary values: %+v", data.Report.Summary)
	}
	if data.Report.Outcome != ops.RunOutcomeClean {
		t.Fatalf("unexpected report outcome: %s", data.Report.Outcome)
	}
	if len(data.Report.Sections) != 4 {
		t.Fatalf("expected four sections, got %d", len(data.Report.Sections))
	}
	if data.Report.Sections[0].Name != "monitor" {
		t.Fatalf("unexpected first section name: %s", data.Report.Sections[0].Name)
	}
	if data.Report.Sections[1].Name != "drift" {
		t.Fatalf("unexpected second section name: %s", data.Report.Sections[1].Name)
	}
	if data.Report.Sections[2].Name != "rate_limit" {
		t.Fatalf("unexpected third section name: %s", data.Report.Sections[2].Name)
	}
	if data.Report.Sections[3].Name != "preflight" {
		t.Fatalf("unexpected fourth section name: %s", data.Report.Sections[3].Name)
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

func TestOpsRunCommandReturnsPolicyExitOnRateLimitThreshold(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := ops.Initialize(statePath); err != nil {
		t.Fatalf("initialize baseline state: %v", err)
	}

	telemetryPath := filepath.Join(t.TempDir(), "telemetry.json")
	telemetry := "{\n  \"app_call_count\": 85,\n  \"app_total_cputime\": 20,\n  \"app_total_time\": 10,\n  \"page_call_count\": 10,\n  \"page_total_cputime\": 5,\n  \"page_total_time\": 3,\n  \"ad_account_util_pct\": 2\n}\n"
	if err := os.WriteFile(telemetryPath, []byte(telemetry), 0o600); err != nil {
		t.Fatalf("write telemetry fixture: %v", err)
	}

	stdout, stderr, err := executeOpsCommand(Runtime{}, "run", "--state-path", statePath, "--rate-telemetry-file", telemetryPath)
	if err == nil {
		t.Fatal("expected rate-limit threshold ops run to fail")
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
	if data.Report.Checks[2].Name != "rate_limit_threshold" {
		t.Fatalf("unexpected rate limit check name: %s", data.Report.Checks[2].Name)
	}
	if data.Report.Checks[2].Status != ops.CheckStatusFail {
		t.Fatalf("unexpected rate limit check status: %s", data.Report.Checks[2].Status)
	}
}

func TestOpsRunCommandReturnsWarningExitOnRateLimitWarningThreshold(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := ops.Initialize(statePath); err != nil {
		t.Fatalf("initialize baseline state: %v", err)
	}

	telemetryPath := filepath.Join(t.TempDir(), "telemetry-warning.json")
	telemetry := "{\n  \"app_call_count\": 65,\n  \"app_total_cputime\": 20,\n  \"app_total_time\": 10,\n  \"page_call_count\": 10,\n  \"page_total_cputime\": 5,\n  \"page_total_time\": 3,\n  \"ad_account_util_pct\": 2\n}\n"
	if err := os.WriteFile(telemetryPath, []byte(telemetry), 0o600); err != nil {
		t.Fatalf("write telemetry fixture: %v", err)
	}

	stdout, stderr, err := executeOpsCommand(Runtime{}, "run", "--state-path", statePath, "--rate-telemetry-file", telemetryPath)
	if err == nil {
		t.Fatal("expected rate-limit warning ops run to fail")
	}
	var exitErr *ops.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != ops.ExitCodeWarning {
		t.Fatalf("unexpected exit code: got=%d want=%d", exitErr.Code, ops.ExitCodeWarning)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	envelope := decodeOpsEnvelope(t, []byte(stdout))
	if envelope.Success {
		t.Fatal("expected success=false")
	}
	if envelope.ExitCode != ops.ExitCodeWarning {
		t.Fatalf("unexpected envelope exit code: got=%d want=%d", envelope.ExitCode, ops.ExitCodeWarning)
	}
	if envelope.Error == nil {
		t.Fatal("expected error payload")
	}
	if envelope.Error.Type != "warning_findings" {
		t.Fatalf("unexpected error type: %s", envelope.Error.Type)
	}

	var data ops.RunResult
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatalf("decode run data: %v", err)
	}
	if data.Report.Summary.Warnings != 1 || data.Report.Summary.Blocking != 0 {
		t.Fatalf("unexpected summary values: %+v", data.Report.Summary)
	}
	if data.Report.Outcome != ops.RunOutcomeWarning {
		t.Fatalf("unexpected report outcome: %s", data.Report.Outcome)
	}
}

func TestOpsRunCommandWritesDeterministicJSONLSections(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := ops.Initialize(statePath); err != nil {
		t.Fatalf("initialize baseline state: %v", err)
	}

	stdout, stderr, err := executeOpsCommand(runtimeWithOutput("jsonl"), "run", "--state-path", statePath)
	if err != nil {
		t.Fatalf("execute ops run: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected four jsonl lines, got %d", len(lines))
	}

	expectedSections := []string{"monitor", "drift", "rate_limit", "preflight"}
	for index, line := range lines {
		envelope := decodeOpsEnvelope(t, []byte(line))
		if envelope.Command != ops.CommandRun {
			t.Fatalf("unexpected command: %s", envelope.Command)
		}
		if !envelope.Success {
			t.Fatal("expected success=true")
		}
		if envelope.ExitCode != ops.ExitCodeSuccess {
			t.Fatalf("unexpected envelope exit code: %d", envelope.ExitCode)
		}

		var sectionData runSectionLineFixture
		if err := json.Unmarshal(envelope.Data, &sectionData); err != nil {
			t.Fatalf("decode jsonl section data: %v", err)
		}
		if sectionData.StatePath != statePath {
			t.Fatalf("unexpected state path: got=%s want=%s", sectionData.StatePath, statePath)
		}
		if sectionData.ReportOutcome != ops.RunOutcomeClean {
			t.Fatalf("unexpected report outcome: %s", sectionData.ReportOutcome)
		}
		if sectionData.Section.Name != expectedSections[index] {
			t.Fatalf("unexpected section order at index %d: %s", index, sectionData.Section.Name)
		}
	}
}

func TestOpsRunCommandWritesDeterministicCSVSections(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := ops.Initialize(statePath); err != nil {
		t.Fatalf("initialize baseline state: %v", err)
	}

	stdout, stderr, err := executeOpsCommand(runtimeWithOutput("csv"), "run", "--state-path", statePath)
	if err != nil {
		t.Fatalf("execute ops run: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	reader := csv.NewReader(strings.NewReader(stdout))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("read csv output: %v", err)
	}
	if len(records) != 6 {
		t.Fatalf("expected 6 csv rows (header + 5 checks), got %d", len(records))
	}

	expectedHeader := []string{
		"state_path",
		"report_outcome",
		"section",
		"section_outcome",
		"section_total",
		"section_passed",
		"section_failed",
		"section_warnings",
		"section_blocking",
		"check_name",
		"check_status",
		"check_blocking",
		"check_message",
	}
	if strings.Join(records[0], ",") != strings.Join(expectedHeader, ",") {
		t.Fatalf("unexpected csv header: %v", records[0])
	}

	expectedSectionAndChecks := [][2]string{
		{"monitor", "changelog_occ_delta"},
		{"drift", "schema_pack_drift"},
		{"drift", "runtime_response_shape_drift"},
		{"rate_limit", "rate_limit_threshold"},
		{"preflight", "permission_policy_preflight"},
	}
	for index, expected := range expectedSectionAndChecks {
		row := records[index+1]
		if row[0] != statePath {
			t.Fatalf("unexpected state path in row %d: %s", index+1, row[0])
		}
		if row[1] != ops.RunOutcomeClean {
			t.Fatalf("unexpected report outcome in row %d: %s", index+1, row[1])
		}
		if row[2] != expected[0] {
			t.Fatalf("unexpected section name in row %d: %s", index+1, row[2])
		}
		if row[9] != expected[1] {
			t.Fatalf("unexpected check name in row %d: %s", index+1, row[9])
		}
	}
}

func TestOpsRunCommandReturnsPolicyExitOnRuntimeResponseShapeDrift(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := ops.Initialize(statePath); err != nil {
		t.Fatalf("initialize baseline state: %v", err)
	}

	runtimeSnapshotPath := filepath.Join(t.TempDir(), "runtime-snapshot.json")
	runtimeSnapshot := "{\n  \"method\": \"GET\",\n  \"path\": \"/act_1/campaigns\",\n  \"observed_fields\": [\"id\", \"unknown_runtime_field\"]\n}\n"
	if err := os.WriteFile(runtimeSnapshotPath, []byte(runtimeSnapshot), 0o600); err != nil {
		t.Fatalf("write runtime snapshot fixture: %v", err)
	}

	stdout, stderr, err := executeOpsCommand(Runtime{}, "run", "--state-path", statePath, "--runtime-response-file", runtimeSnapshotPath)
	if err == nil {
		t.Fatal("expected runtime shape drift ops run to fail")
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
	if data.Report.Checks[4].Name != "runtime_response_shape_drift" {
		t.Fatalf("unexpected runtime drift check name: %s", data.Report.Checks[4].Name)
	}
	if data.Report.Checks[4].Status != ops.CheckStatusFail {
		t.Fatalf("unexpected runtime drift check status: %s", data.Report.Checks[4].Status)
	}
}

func TestOpsRunCommandReturnsRuntimeExitOnRuntimeEvaluationError(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := ops.Initialize(statePath); err != nil {
		t.Fatalf("initialize baseline state: %v", err)
	}

	state, err := ops.LoadBaseline(statePath)
	if err != nil {
		t.Fatalf("load baseline state: %v", err)
	}
	state.Snapshots.SchemaPack.Domain = "missing-domain"
	if err := ops.SaveBaseline(statePath, state); err != nil {
		t.Fatalf("save baseline state: %v", err)
	}

	runtimeSnapshotPath := filepath.Join(t.TempDir(), "runtime-snapshot.json")
	runtimeSnapshot := "{\n  \"method\": \"GET\",\n  \"path\": \"/act_1/campaigns\",\n  \"observed_fields\": [\"id\"]\n}\n"
	if err := os.WriteFile(runtimeSnapshotPath, []byte(runtimeSnapshot), 0o600); err != nil {
		t.Fatalf("write runtime snapshot fixture: %v", err)
	}

	stdout, stderr, err := executeOpsCommand(Runtime{}, "run", "--state-path", statePath, "--runtime-response-file", runtimeSnapshotPath)
	if err == nil {
		t.Fatal("expected runtime evaluation failure")
	}
	var exitErr *ops.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != ops.ExitCodeRuntime {
		t.Fatalf("unexpected exit code: got=%d want=%d", exitErr.Code, ops.ExitCodeRuntime)
	}
	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}

	envelope := decodeOpsEnvelope(t, []byte(stderr))
	if envelope.Success {
		t.Fatal("expected success=false")
	}
	if envelope.ExitCode != ops.ExitCodeRuntime {
		t.Fatalf("unexpected envelope exit code: got=%d want=%d", envelope.ExitCode, ops.ExitCodeRuntime)
	}
	if envelope.Error == nil {
		t.Fatal("expected error payload")
	}
	if envelope.Error.Type != "runtime_error" {
		t.Fatalf("unexpected error type: %s", envelope.Error.Type)
	}
}

func TestOpsRunCommandReturnsInputExitWhenLintRequestProvidedWithoutRuntimeSnapshot(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := ops.Initialize(statePath); err != nil {
		t.Fatalf("initialize baseline state: %v", err)
	}

	lintRequestPath := filepath.Join(t.TempDir(), "request-spec.json")
	lintRequest := "{\n  \"method\": \"GET\",\n  \"path\": \"/act_1/campaigns\",\n  \"fields\": [\"id\", \"name\"]\n}\n"
	if err := os.WriteFile(lintRequestPath, []byte(lintRequest), 0o600); err != nil {
		t.Fatalf("write lint request fixture: %v", err)
	}

	stdout, stderr, err := executeOpsCommand(Runtime{}, "run", "--state-path", statePath, "--lint-request-file", lintRequestPath)
	if err == nil {
		t.Fatal("expected ops run to fail when lint request spec is provided without runtime snapshot")
	}
	var exitErr *ops.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != ops.ExitCodeInput {
		t.Fatalf("unexpected exit code: got=%d want=%d", exitErr.Code, ops.ExitCodeInput)
	}
	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}

	envelope := decodeOpsEnvelope(t, []byte(stderr))
	if envelope.Success {
		t.Fatal("expected success=false")
	}
	if envelope.ExitCode != ops.ExitCodeInput {
		t.Fatalf("unexpected envelope exit code: got=%d want=%d", envelope.ExitCode, ops.ExitCodeInput)
	}
}

func TestOpsRunCommandLinksLintRequestSpecForRuntimeShapeDriftCheck(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := ops.Initialize(statePath); err != nil {
		t.Fatalf("initialize baseline state: %v", err)
	}

	runtimeSnapshotPath := filepath.Join(t.TempDir(), "runtime-snapshot.json")
	runtimeSnapshot := "{\n  \"method\": \"GET\",\n  \"path\": \"/act_1/campaigns\",\n  \"observed_fields\": [\"id\", \"name\"]\n}\n"
	if err := os.WriteFile(runtimeSnapshotPath, []byte(runtimeSnapshot), 0o600); err != nil {
		t.Fatalf("write runtime snapshot fixture: %v", err)
	}

	lintRequestPath := filepath.Join(t.TempDir(), "request-spec.json")
	lintRequest := "{\n  \"method\": \"GET\",\n  \"path\": \"/act_1/campaigns\",\n  \"fields\": [\"id\", \"name\"]\n}\n"
	if err := os.WriteFile(lintRequestPath, []byte(lintRequest), 0o600); err != nil {
		t.Fatalf("write lint request fixture: %v", err)
	}

	stdout, stderr, err := executeOpsCommand(
		Runtime{},
		"run",
		"--state-path", statePath,
		"--runtime-response-file", runtimeSnapshotPath,
		"--lint-request-file", lintRequestPath,
	)
	if err != nil {
		t.Fatalf("execute ops run: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	envelope := decodeOpsEnvelope(t, []byte(stdout))
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
	if data.Report.Checks[4].Name != "runtime_response_shape_drift" {
		t.Fatalf("unexpected runtime drift check name: %s", data.Report.Checks[4].Name)
	}
	if data.Report.Checks[4].Status != ops.CheckStatusPass {
		t.Fatalf("unexpected runtime drift check status: %s", data.Report.Checks[4].Status)
	}
	if !strings.Contains(data.Report.Checks[4].Message, "lint_request_spec="+lintRequestPath) {
		t.Fatalf("expected lint request reference in runtime drift message, got %q", data.Report.Checks[4].Message)
	}
}

func TestOpsRunCommandReturnsInputExitOnInvalidRateTelemetryFile(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := ops.Initialize(statePath); err != nil {
		t.Fatalf("initialize baseline state: %v", err)
	}

	telemetryPath := filepath.Join(t.TempDir(), "telemetry.json")
	telemetry := "{\n  \"app_call_count\": 10,\n  \"unknown\": true\n}\n"
	if err := os.WriteFile(telemetryPath, []byte(telemetry), 0o600); err != nil {
		t.Fatalf("write telemetry fixture: %v", err)
	}

	stdout, stderr, err := executeOpsCommand(Runtime{}, "run", "--state-path", statePath, "--rate-telemetry-file", telemetryPath)
	if err == nil {
		t.Fatal("expected ops run to fail with invalid telemetry")
	}
	var exitErr *ops.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != ops.ExitCodeInput {
		t.Fatalf("unexpected exit code: got=%d want=%d", exitErr.Code, ops.ExitCodeInput)
	}
	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}

	envelope := decodeOpsEnvelope(t, []byte(stderr))
	if envelope.Success {
		t.Fatal("expected success=false")
	}
	if envelope.ExitCode != ops.ExitCodeInput {
		t.Fatalf("unexpected envelope exit code: got=%d want=%d", envelope.ExitCode, ops.ExitCodeInput)
	}
	if envelope.Error == nil {
		t.Fatal("expected error payload")
	}
	if envelope.Error.Type != "input_error" {
		t.Fatalf("unexpected error type: %s", envelope.Error.Type)
	}
}

func TestOpsRunCommandReturnsPolicyExitOnPreflightViolations(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := ops.Initialize(statePath); err != nil {
		t.Fatalf("initialize baseline state: %v", err)
	}

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	configBody := "schema_version: 2\ndefault_profile: prod\nprofiles:\n  prod:\n    domain: marketing\n    graph_version: v25.0\n    token_type: system_user\n    token_ref: keychain://meta-marketing-cli/prod/token\n    app_id: app_123\n    app_secret_ref: keychain://meta-marketing-cli/prod/app_secret\n    auth_provider: system_user\n    auth_mode: both\n    scopes:\n      - ads_management\n      - business_management\n    issued_at: \"2026-01-15T00:00:00Z\"\n    expires_at: \"2026-12-31T23:59:59Z\"\n    last_validated_at: \"2026-01-16T00:00:00Z\"\n"
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	stdout, stderr, err := executeOpsCommand(runtimeWithProfile("prod"), "run", "--state-path", statePath, "--preflight-config-path", configPath)
	if err == nil {
		t.Fatal("expected ops run to fail on preflight violations")
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
	if data.Report.Checks[3].Name != "permission_policy_preflight" {
		t.Fatalf("unexpected preflight check name: %s", data.Report.Checks[3].Name)
	}
	if data.Report.Checks[3].Status != ops.CheckStatusFail {
		t.Fatalf("unexpected preflight check status: %s", data.Report.Checks[3].Status)
	}
	if data.Report.Summary.Blocking != 1 {
		t.Fatalf("unexpected summary values: %+v", data.Report.Summary)
	}
}

func TestOpsRunCommandPassesPreflightWithValidProfileConfig(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := ops.Initialize(statePath); err != nil {
		t.Fatalf("initialize baseline state: %v", err)
	}

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	configBody := "schema_version: 2\ndefault_profile: prod\nprofiles:\n  prod:\n    domain: marketing\n    graph_version: v25.0\n    token_type: user\n    app_id: app_123\n    token_ref: keychain://meta-marketing-cli/prod/token\n    app_secret_ref: keychain://meta-marketing-cli/prod/app_secret\n    auth_provider: facebook_login\n    auth_mode: both\n    scopes:\n      - ads_read\n    issued_at: \"2026-01-15T00:00:00Z\"\n    expires_at: \"2026-12-31T23:59:59Z\"\n    last_validated_at: \"2026-01-16T00:00:00Z\"\n"
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	stdout, stderr, err := executeOpsCommand(runtimeWithProfile("prod"), "run", "--state-path", statePath, "--preflight-config-path", configPath)
	if err != nil {
		t.Fatalf("execute ops run: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	envelope := decodeOpsEnvelope(t, []byte(stdout))
	if !envelope.Success {
		t.Fatal("expected success=true")
	}
	var data ops.RunResult
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatalf("decode run data: %v", err)
	}
	if data.Report.Checks[3].Name != "permission_policy_preflight" {
		t.Fatalf("unexpected preflight check name: %s", data.Report.Checks[3].Name)
	}
	if data.Report.Checks[3].Status != ops.CheckStatusPass {
		t.Fatalf("unexpected preflight check status: %s", data.Report.Checks[3].Status)
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

func runtimeWithProfile(profile string) Runtime {
	output := "json"
	debug := false
	return Runtime{
		Profile: &profile,
		Output:  &output,
		Debug:   &debug,
	}
}

func runtimeWithOutput(format string) Runtime {
	debug := false
	return Runtime{
		Output: &format,
		Debug:  &debug,
	}
}
