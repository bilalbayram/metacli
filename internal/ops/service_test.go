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
	if len(result.Report.Checks) != 1 {
		t.Fatalf("expected one check, got %d", len(result.Report.Checks))
	}
	check := result.Report.Checks[0]
	if check.Name != checkNameChangelogOCCDelta {
		t.Fatalf("unexpected check name: %s", check.Name)
	}
	if check.Status != CheckStatusPass {
		t.Fatalf("unexpected check status: %s", check.Status)
	}
	if check.Blocking {
		t.Fatal("expected non-blocking status for unchanged snapshot")
	}
	if result.Report.Summary.Total != 1 || result.Report.Summary.Passed != 1 || result.Report.Summary.Failed != 0 || result.Report.Summary.Blocking != 0 {
		t.Fatalf("unexpected summary counts: %+v", result.Report.Summary)
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
