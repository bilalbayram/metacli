package smoke_test

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
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
	Error           *ops.ErrorInfo  `json:"error"`
}

type runSectionLineFixture struct {
	StatePath           string            `json:"state_path"`
	ReportSchemaVersion int               `json:"report_schema_version"`
	ReportKind          string            `json:"report_kind"`
	ReportOutcome       string            `json:"report_outcome"`
	Section             ops.ReportSection `json:"section"`
}

func TestTrackCDailyReportJSONContractAndFingerprints(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := ops.Initialize(statePath); err != nil {
		t.Fatalf("initialize baseline state: %v", err)
	}

	result, err := ops.Run(statePath)
	if err != nil {
		t.Fatalf("run daily report: %v", err)
	}

	var output bytes.Buffer
	envelope := ops.NewSuccessEnvelope(ops.CommandRun, result)
	if err := ops.WriteEnvelope(&output, "json", envelope); err != nil {
		t.Fatalf("write json envelope: %v", err)
	}

	decodedEnvelope := decodeEnvelope(t, output.Bytes())
	if decodedEnvelope.ContractVersion != ops.ContractVersion {
		t.Fatalf("unexpected contract version: got=%s want=%s", decodedEnvelope.ContractVersion, ops.ContractVersion)
	}
	if decodedEnvelope.Command != ops.CommandRun {
		t.Fatalf("unexpected command: got=%s want=%s", decodedEnvelope.Command, ops.CommandRun)
	}
	if !decodedEnvelope.Success {
		t.Fatal("expected success envelope")
	}
	if decodedEnvelope.ExitCode != ops.ExitCodeSuccess {
		t.Fatalf("unexpected exit code: got=%d want=%d", decodedEnvelope.ExitCode, ops.ExitCodeSuccess)
	}
	if decodedEnvelope.Error != nil {
		t.Fatalf("unexpected error payload: %+v", decodedEnvelope.Error)
	}

	var runResult ops.RunResult
	if err := json.Unmarshal(decodedEnvelope.Data, &runResult); err != nil {
		t.Fatalf("decode run data: %v", err)
	}
	if runResult.StatePath != statePath {
		t.Fatalf("unexpected state path: got=%s want=%s", runResult.StatePath, statePath)
	}
	if runResult.Report.SchemaVersion != ops.ReportSchemaVersion {
		t.Fatalf("unexpected report schema version: got=%d want=%d", runResult.Report.SchemaVersion, ops.ReportSchemaVersion)
	}
	if runResult.Report.Kind != "ops_report" {
		t.Fatalf("unexpected report kind: %s", runResult.Report.Kind)
	}
	if runResult.Report.Outcome != ops.RunOutcomeClean {
		t.Fatalf("unexpected report outcome: %s", runResult.Report.Outcome)
	}
	if len(runResult.Report.Sections) != 4 {
		t.Fatalf("expected four report sections, got %d", len(runResult.Report.Sections))
	}

	expectedSectionOrder := []string{"monitor", "drift", "rate_limit", "preflight"}
	for index, section := range runResult.Report.Sections {
		if section.Name != expectedSectionOrder[index] {
			t.Fatalf("unexpected section at index %d: got=%s want=%s", index, section.Name, expectedSectionOrder[index])
		}
	}

	baseline, err := ops.LoadBaseline(statePath)
	if err != nil {
		t.Fatalf("load baseline: %v", err)
	}

	occFingerprint := strings.TrimSpace(runResult.Report.Baseline.Snapshots.ChangelogOCC.OCCDigest)
	if occFingerprint == "" {
		t.Fatal("expected non-empty changelog OCC fingerprint")
	}
	if occFingerprint != baseline.Snapshots.ChangelogOCC.OCCDigest {
		t.Fatalf("unexpected changelog OCC fingerprint: got=%s want=%s", occFingerprint, baseline.Snapshots.ChangelogOCC.OCCDigest)
	}

	schemaFingerprint := strings.TrimSpace(runResult.Report.Baseline.Snapshots.SchemaPack.SHA256)
	if schemaFingerprint == "" {
		t.Fatal("expected non-empty schema pack fingerprint")
	}
	if len(schemaFingerprint) != 64 {
		t.Fatalf("unexpected schema pack fingerprint length: got=%d want=64", len(schemaFingerprint))
	}
	if schemaFingerprint != baseline.Snapshots.SchemaPack.SHA256 {
		t.Fatalf("unexpected schema fingerprint: got=%s want=%s", schemaFingerprint, baseline.Snapshots.SchemaPack.SHA256)
	}

	occCheck := findCheck(t, runResult.Report.Checks, "changelog_occ_delta")
	if !strings.Contains(occCheck.Message, "occ_digest="+occFingerprint) {
		t.Fatalf("expected changelog check message to include OCC fingerprint, got %q", occCheck.Message)
	}

	schemaCheck := findCheck(t, runResult.Report.Checks, "schema_pack_drift")
	if !strings.Contains(schemaCheck.Message, "sha256="+schemaFingerprint) {
		t.Fatalf("expected schema drift check message to include schema fingerprint, got %q", schemaCheck.Message)
	}
}

func TestTrackCDailyReportJSONLAndCSVContract(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := ops.Initialize(statePath); err != nil {
		t.Fatalf("initialize baseline state: %v", err)
	}

	result, err := ops.Run(statePath)
	if err != nil {
		t.Fatalf("run daily report: %v", err)
	}
	envelope := ops.NewSuccessEnvelope(ops.CommandRun, result)

	var jsonlOutput bytes.Buffer
	if err := ops.WriteEnvelope(&jsonlOutput, "jsonl", envelope); err != nil {
		t.Fatalf("write jsonl envelope: %v", err)
	}

	jsonlLines := strings.Split(strings.TrimSpace(jsonlOutput.String()), "\n")
	if len(jsonlLines) != 4 {
		t.Fatalf("expected four jsonl lines, got %d", len(jsonlLines))
	}

	expectedSections := []string{"monitor", "drift", "rate_limit", "preflight"}
	for index, line := range jsonlLines {
		decodedEnvelope := decodeEnvelope(t, []byte(line))
		if decodedEnvelope.Command != ops.CommandRun {
			t.Fatalf("unexpected command in jsonl line %d: %s", index, decodedEnvelope.Command)
		}
		if !decodedEnvelope.Success {
			t.Fatalf("expected success=true in jsonl line %d", index)
		}
		if decodedEnvelope.ExitCode != ops.ExitCodeSuccess {
			t.Fatalf("unexpected exit code in jsonl line %d: got=%d want=%d", index, decodedEnvelope.ExitCode, ops.ExitCodeSuccess)
		}

		var sectionLine runSectionLineFixture
		if err := json.Unmarshal(decodedEnvelope.Data, &sectionLine); err != nil {
			t.Fatalf("decode jsonl line %d: %v", index, err)
		}
		if sectionLine.StatePath != statePath {
			t.Fatalf("unexpected state path in jsonl line %d: got=%s want=%s", index, sectionLine.StatePath, statePath)
		}
		if sectionLine.ReportSchemaVersion != ops.ReportSchemaVersion {
			t.Fatalf("unexpected schema version in jsonl line %d: got=%d want=%d", index, sectionLine.ReportSchemaVersion, ops.ReportSchemaVersion)
		}
		if sectionLine.ReportKind != "ops_report" {
			t.Fatalf("unexpected report kind in jsonl line %d: %s", index, sectionLine.ReportKind)
		}
		if sectionLine.ReportOutcome != ops.RunOutcomeClean {
			t.Fatalf("unexpected report outcome in jsonl line %d: %s", index, sectionLine.ReportOutcome)
		}
		if sectionLine.Section.Name != expectedSections[index] {
			t.Fatalf("unexpected section order in jsonl line %d: got=%s want=%s", index, sectionLine.Section.Name, expectedSections[index])
		}
	}

	var csvOutput bytes.Buffer
	if err := ops.WriteEnvelope(&csvOutput, "csv", envelope); err != nil {
		t.Fatalf("write csv envelope: %v", err)
	}

	records, err := csv.NewReader(strings.NewReader(csvOutput.String())).ReadAll()
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

	expectedSectionChecks := [][2]string{
		{"monitor", "changelog_occ_delta"},
		{"drift", "schema_pack_drift"},
		{"drift", "runtime_response_shape_drift"},
		{"rate_limit", "rate_limit_threshold"},
		{"preflight", "permission_policy_preflight"},
	}

	for index, expected := range expectedSectionChecks {
		row := records[index+1]
		if row[0] != statePath {
			t.Fatalf("unexpected state path in row %d: got=%s want=%s", index+1, row[0], statePath)
		}
		if row[1] != ops.RunOutcomeClean {
			t.Fatalf("unexpected report outcome in row %d: got=%s want=%s", index+1, row[1], ops.RunOutcomeClean)
		}
		if row[2] != expected[0] {
			t.Fatalf("unexpected section in row %d: got=%s want=%s", index+1, row[2], expected[0])
		}
		if row[9] != expected[1] {
			t.Fatalf("unexpected check in row %d: got=%s want=%s", index+1, row[9], expected[1])
		}
	}
}

func TestTrackCDailyReportFingerprintDriftIsBlocking(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := ops.Initialize(statePath); err != nil {
		t.Fatalf("initialize baseline state: %v", err)
	}

	state, err := ops.LoadBaseline(statePath)
	if err != nil {
		t.Fatalf("load baseline: %v", err)
	}
	state.Snapshots.ChangelogOCC.OCCDigest = "occ.drift.test"
	state.Snapshots.SchemaPack.SHA256 = strings.Repeat("f", 64)
	if err := ops.SaveBaseline(statePath, state); err != nil {
		t.Fatalf("save baseline: %v", err)
	}

	result, err := ops.Run(statePath)
	if err != nil {
		t.Fatalf("run daily report: %v", err)
	}
	if result.Report.Outcome != ops.RunOutcomeBlocking {
		t.Fatalf("unexpected report outcome: got=%s want=%s", result.Report.Outcome, ops.RunOutcomeBlocking)
	}
	if result.Report.Summary.Blocking != 2 {
		t.Fatalf("expected two blocking findings from fingerprint drift, got %+v", result.Report.Summary)
	}
	if code := ops.RunExitCode(result.Report); code != ops.ExitCodePolicy {
		t.Fatalf("unexpected exit code: got=%d want=%d", code, ops.ExitCodePolicy)
	}

	occCheck := findCheck(t, result.Report.Checks, "changelog_occ_delta")
	if occCheck.Status != ops.CheckStatusFail {
		t.Fatalf("unexpected changelog check status: %s", occCheck.Status)
	}
	if !occCheck.Blocking {
		t.Fatal("expected changelog fingerprint drift to be blocking")
	}
	if !strings.Contains(occCheck.Message, "baseline latest_version=") || !strings.Contains(occCheck.Message, "current latest_version=") {
		t.Fatalf("expected changelog drift details in message, got %q", occCheck.Message)
	}

	schemaCheck := findCheck(t, result.Report.Checks, "schema_pack_drift")
	if schemaCheck.Status != ops.CheckStatusFail {
		t.Fatalf("unexpected schema check status: %s", schemaCheck.Status)
	}
	if !schemaCheck.Blocking {
		t.Fatal("expected schema fingerprint drift to be blocking")
	}
	if !strings.Contains(schemaCheck.Message, "baseline domain=") || !strings.Contains(schemaCheck.Message, "current domain=") {
		t.Fatalf("expected schema drift details in message, got %q", schemaCheck.Message)
	}
}

func decodeEnvelope(t *testing.T, raw []byte) envelopeFixture {
	t.Helper()

	var envelope envelopeFixture
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode envelope: %v\nraw=%s", err, string(raw))
	}
	return envelope
}

func findCheck(t *testing.T, checks []ops.Check, name string) ops.Check {
	t.Helper()

	for _, check := range checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("missing check with name %q", name)
	return ops.Check{}
}
