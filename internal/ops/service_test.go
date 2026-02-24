package ops

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestRunReturnsReportWithChangelogOCCCheck(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := InitBaseline(path); err != nil {
		t.Fatalf("init baseline: %v", err)
	}

	result, err := Run(path)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if result.Report.SchemaVersion != ReportSchemaVersion {
		t.Fatalf("unexpected report schema version: got=%d want=%d", result.Report.SchemaVersion, ReportSchemaVersion)
	}
	if result.Report.Kind != "ops_report" {
		t.Fatalf("unexpected report kind: %s", result.Report.Kind)
	}
	if len(result.Report.Checks) != 5 {
		t.Fatalf("expected five checks, got %d", len(result.Report.Checks))
	}
	if result.Report.Checks[0].Name != checkNameChangelogOCCDelta {
		t.Fatalf("unexpected first check name: %s", result.Report.Checks[0].Name)
	}
	if result.Report.Checks[1].Name != checkNameSchemaPackDrift {
		t.Fatalf("unexpected second check name: %s", result.Report.Checks[1].Name)
	}
	if result.Report.Checks[2].Name != checkNameRateLimitThreshold {
		t.Fatalf("unexpected third check name: %s", result.Report.Checks[2].Name)
	}
	if result.Report.Checks[3].Name != checkNamePermissionPolicyPreflight {
		t.Fatalf("unexpected fourth check name: %s", result.Report.Checks[3].Name)
	}
	if result.Report.Checks[4].Name != checkNameRuntimeResponseShapeDrift {
		t.Fatalf("unexpected fifth check name: %s", result.Report.Checks[4].Name)
	}
	for _, check := range result.Report.Checks {
		if check.Status != CheckStatusPass {
			t.Fatalf("unexpected check status: %s", check.Status)
		}
		if check.Blocking {
			t.Fatal("expected non-blocking status for unchanged snapshots")
		}
	}
	if result.Report.Summary.Total != 5 || result.Report.Summary.Passed != 5 || result.Report.Summary.Failed != 0 || result.Report.Summary.Warnings != 0 || result.Report.Summary.Blocking != 0 {
		t.Fatalf("unexpected summary counts: %+v", result.Report.Summary)
	}
	if result.Report.Outcome != RunOutcomeClean {
		t.Fatalf("unexpected report outcome: %s", result.Report.Outcome)
	}
	if len(result.Report.Sections) != 4 {
		t.Fatalf("expected four report sections, got %d", len(result.Report.Sections))
	}
	if result.Report.Sections[0].Name != reportSectionMonitor {
		t.Fatalf("unexpected first section name: %s", result.Report.Sections[0].Name)
	}
	if result.Report.Sections[1].Name != reportSectionDrift {
		t.Fatalf("unexpected second section name: %s", result.Report.Sections[1].Name)
	}
	if result.Report.Sections[2].Name != reportSectionRateLimit {
		t.Fatalf("unexpected third section name: %s", result.Report.Sections[2].Name)
	}
	if result.Report.Sections[3].Name != reportSectionPreflight {
		t.Fatalf("unexpected fourth section name: %s", result.Report.Sections[3].Name)
	}
	if len(result.Report.Sections[1].Checks) != 2 {
		t.Fatalf("expected drift section to contain two checks, got %d", len(result.Report.Sections[1].Checks))
	}
	if result.Report.Sections[1].Checks[0].Name != checkNameSchemaPackDrift {
		t.Fatalf("unexpected first drift check name: %s", result.Report.Sections[1].Checks[0].Name)
	}
	if result.Report.Sections[1].Checks[1].Name != checkNameRuntimeResponseShapeDrift {
		t.Fatalf("unexpected second drift check name: %s", result.Report.Sections[1].Checks[1].Name)
	}
}

func TestExitCodeSemantics(t *testing.T) {
	t.Parallel()

	if code := ExitCode(nil); code != ExitCodeSuccess {
		t.Fatalf("unexpected success code: %d", code)
	}

	wrapped := WrapExit(ExitCodeInput, errors.New("invalid input"))
	if code := ExitCode(wrapped); code != ExitCodeInput {
		t.Fatalf("unexpected wrapped code: got=%d want=%d", code, ExitCodeInput)
	}

	if code := ExitCode(errors.New("plain failure")); code != ExitCodeUnknown {
		t.Fatalf("unexpected fallback code: got=%d want=%d", code, ExitCodeUnknown)
	}
}

func TestRunExitCodeUsesPolicyCodeForBlockingFindings(t *testing.T) {
	t.Parallel()

	report := Report{
		Summary: Summary{Blocking: 1},
	}
	if code := RunExitCode(report); code != ExitCodePolicy {
		t.Fatalf("unexpected run exit code: got=%d want=%d", code, ExitCodePolicy)
	}
}

func TestRunExitCodeUsesWarningCodeForWarningFindings(t *testing.T) {
	t.Parallel()

	report := Report{
		Summary: Summary{Warnings: 1},
	}
	if code := RunExitCode(report); code != ExitCodeWarning {
		t.Fatalf("unexpected run exit code: got=%d want=%d", code, ExitCodeWarning)
	}
}

func TestRunWithOptionsFailsClosedOnStrictOptionalPreflight(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := InitBaseline(path); err != nil {
		t.Fatalf("init baseline: %v", err)
	}

	result, err := RunWithOptions(path, RunOptions{
		OptionalModulePolicy: OptionalModulePolicyStrict,
	})
	if err != nil {
		t.Fatalf("run with strict optional preflight policy: %v", err)
	}
	if len(result.Report.Checks) != 1 {
		t.Fatalf("expected fail-fast preflight report to include one check, got %d", len(result.Report.Checks))
	}
	if result.Report.Checks[0].Name != checkNamePermissionPolicyPreflight {
		t.Fatalf("unexpected check name: %s", result.Report.Checks[0].Name)
	}
	if result.Report.Checks[0].Status != CheckStatusFail {
		t.Fatalf("unexpected check status: %s", result.Report.Checks[0].Status)
	}
	if !result.Report.Checks[0].Blocking {
		t.Fatal("expected strict optional preflight failure to be blocking")
	}
	if result.Report.Summary.Blocking != 1 {
		t.Fatalf("unexpected summary: %+v", result.Report.Summary)
	}
	if result.Report.Outcome != RunOutcomeBlocking {
		t.Fatalf("unexpected report outcome: %s", result.Report.Outcome)
	}
	if code := RunExitCode(result.Report); code != ExitCodePolicy {
		t.Fatalf("unexpected run exit code: got=%d want=%d", code, ExitCodePolicy)
	}
}
