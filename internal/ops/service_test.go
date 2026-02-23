package ops

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestRunReturnsReportSkeleton(t *testing.T) {
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
	if len(result.Report.Checks) != 0 {
		t.Fatalf("expected empty checks, got %d", len(result.Report.Checks))
	}
	if result.Report.Summary.Total != 0 || result.Report.Summary.Passed != 0 || result.Report.Summary.Failed != 0 {
		t.Fatalf("expected zero summary counts, got %+v", result.Report.Summary)
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
