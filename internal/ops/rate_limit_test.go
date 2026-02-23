package ops

import (
	"path/filepath"
	"testing"
)

func TestEvaluateRateLimitThresholdPassesWhenUnderThreshold(t *testing.T) {
	t.Parallel()

	snapshot := RateLimitTelemetrySnapshot{
		AppCallCount:     40,
		AppTotalCPUTime:  35,
		AppTotalTime:     30,
		PageCallCount:    20,
		PageTotalCPUTime: 10,
		PageTotalTime:    5,
		AdAccountUtilPct: 25,
	}

	check := evaluateRateLimitThreshold(snapshot, DefaultRateLimitWarningThreshold, DefaultRateLimitThreshold)
	if check.Name != checkNameRateLimitThreshold {
		t.Fatalf("unexpected check name: %s", check.Name)
	}
	if check.Status != CheckStatusPass {
		t.Fatalf("unexpected status: got=%s want=%s", check.Status, CheckStatusPass)
	}
	if check.Blocking {
		t.Fatal("expected non-blocking check when usage is under threshold")
	}
}

func TestEvaluateRateLimitThresholdFailsWhenAtThreshold(t *testing.T) {
	t.Parallel()

	snapshot := RateLimitTelemetrySnapshot{
		AppCallCount:     DefaultRateLimitThreshold,
		AppTotalCPUTime:  10,
		AppTotalTime:     5,
		PageCallCount:    1,
		PageTotalCPUTime: 1,
		PageTotalTime:    1,
		AdAccountUtilPct: 2,
	}

	check := evaluateRateLimitThreshold(snapshot, DefaultRateLimitWarningThreshold, DefaultRateLimitThreshold)
	if check.Status != CheckStatusFail {
		t.Fatalf("unexpected status: got=%s want=%s", check.Status, CheckStatusFail)
	}
	if !check.Blocking {
		t.Fatal("expected blocking check when threshold is reached")
	}
}

func TestEvaluateRateLimitThresholdWarnsWhenAtWarningThreshold(t *testing.T) {
	t.Parallel()

	snapshot := RateLimitTelemetrySnapshot{
		AppCallCount:     DefaultRateLimitWarningThreshold,
		AppTotalCPUTime:  10,
		AppTotalTime:     5,
		PageCallCount:    1,
		PageTotalCPUTime: 1,
		PageTotalTime:    1,
		AdAccountUtilPct: 2,
	}

	check := evaluateRateLimitThreshold(snapshot, DefaultRateLimitWarningThreshold, DefaultRateLimitThreshold)
	if check.Status != CheckStatusFail {
		t.Fatalf("unexpected status: got=%s want=%s", check.Status, CheckStatusFail)
	}
	if check.Blocking {
		t.Fatal("expected non-blocking warning check at warning threshold")
	}
}

func TestRunWithOptionsUsesInjectedRateLimitTelemetry(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := InitBaseline(path); err != nil {
		t.Fatalf("init baseline: %v", err)
	}

	telemetry := &RateLimitTelemetrySnapshot{
		AppCallCount:     90,
		AppTotalCPUTime:  10,
		AppTotalTime:     5,
		PageCallCount:    2,
		PageTotalCPUTime: 2,
		PageTotalTime:    1,
		AdAccountUtilPct: 3,
	}
	result, err := RunWithOptions(path, RunOptions{
		RateLimitTelemetry: telemetry,
	})
	if err != nil {
		t.Fatalf("run with options: %v", err)
	}
	if result.Report.Checks[2].Name != checkNameRateLimitThreshold {
		t.Fatalf("unexpected check name: %s", result.Report.Checks[2].Name)
	}
	if result.Report.Checks[2].Status != CheckStatusFail {
		t.Fatalf("unexpected check status: %s", result.Report.Checks[2].Status)
	}
	if result.Report.Summary.Blocking != 1 {
		t.Fatalf("unexpected summary: %+v", result.Report.Summary)
	}
}
