package ops

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilalbayram/metacli/internal/lint"
)

func TestEvaluateRuntimeResponseShapeDriftSkipsWithoutSnapshot(t *testing.T) {
	t.Parallel()

	check, err := evaluateRuntimeResponseShapeDrift(
		SchemaPackSnapshot{Domain: "marketing", Version: "v25.0", SHA256: "unused"},
		nil,
		nil,
		"",
	)
	if err != nil {
		t.Fatalf("evaluate runtime drift check: %v", err)
	}
	if check.Name != checkNameRuntimeResponseShapeDrift {
		t.Fatalf("unexpected check name: %s", check.Name)
	}
	if check.Status != CheckStatusPass {
		t.Fatalf("unexpected check status: %s", check.Status)
	}
	if check.Blocking {
		t.Fatal("expected skipped runtime drift check to be non-blocking")
	}
}

func TestEvaluateRuntimeResponseShapeDriftFailsOnUnknownObservedField(t *testing.T) {
	t.Parallel()

	check, err := evaluateRuntimeResponseShapeDrift(
		SchemaPackSnapshot{Domain: "marketing", Version: "v25.0", SHA256: "unused"},
		&RuntimeResponseShapeSnapshot{
			Method:         "GET",
			Path:           "/act_1/campaigns",
			ObservedFields: []string{"id", "unknown_runtime_field"},
		},
		nil,
		"",
	)
	if err != nil {
		t.Fatalf("evaluate runtime drift check: %v", err)
	}
	if check.Status != CheckStatusFail {
		t.Fatalf("unexpected check status: %s", check.Status)
	}
	if !check.Blocking {
		t.Fatal("expected unknown runtime field drift to be blocking")
	}
	if !strings.Contains(check.Message, "unknown field") {
		t.Fatalf("expected unknown field message, got %q", check.Message)
	}
}

func TestEvaluateRuntimeResponseShapeDriftPassesWithLinkedLintSpec(t *testing.T) {
	t.Parallel()

	check, err := evaluateRuntimeResponseShapeDrift(
		SchemaPackSnapshot{Domain: "marketing", Version: "v25.0", SHA256: "unused"},
		&RuntimeResponseShapeSnapshot{
			Method:         "GET",
			Path:           "/act_1/campaigns",
			ObservedFields: []string{"id", "name", "status"},
		},
		&lint.RequestSpec{
			Method: "GET",
			Path:   "/act_1/campaigns",
			Fields: []string{"id", "name"},
		},
		"fixtures/lint-request.json",
	)
	if err != nil {
		t.Fatalf("evaluate runtime drift check: %v", err)
	}
	if check.Status != CheckStatusPass {
		t.Fatalf("unexpected check status: %s", check.Status)
	}
	if check.Blocking {
		t.Fatal("expected passing runtime drift check to be non-blocking")
	}
	if !strings.Contains(check.Message, "lint_request_spec=fixtures/lint-request.json") {
		t.Fatalf("expected linked lint request reference in check message, got %q", check.Message)
	}
}

func TestEvaluateRuntimeResponseShapeDriftFailsWhenRequestedFieldMissing(t *testing.T) {
	t.Parallel()

	check, err := evaluateRuntimeResponseShapeDrift(
		SchemaPackSnapshot{Domain: "marketing", Version: "v25.0", SHA256: "unused"},
		&RuntimeResponseShapeSnapshot{
			Method:         "GET",
			Path:           "/act_1/campaigns",
			ObservedFields: []string{"id"},
		},
		&lint.RequestSpec{
			Method: "GET",
			Path:   "/act_1/campaigns",
			Fields: []string{"id", "name"},
		},
		"fixtures/lint-request.json",
	)
	if err != nil {
		t.Fatalf("evaluate runtime drift check: %v", err)
	}
	if check.Status != CheckStatusFail {
		t.Fatalf("unexpected check status: %s", check.Status)
	}
	if !check.Blocking {
		t.Fatal("expected missing requested fields drift to be blocking")
	}
	if !strings.Contains(check.Message, "missing requested fields") {
		t.Fatalf("expected missing field message, got %q", check.Message)
	}
}

func TestRunWithOptionsRejectsLintSpecWithoutRuntimeSnapshot(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := InitBaseline(path); err != nil {
		t.Fatalf("init baseline: %v", err)
	}

	_, err := RunWithOptions(path, RunOptions{
		LintRequestSpec: &lint.RequestSpec{
			Method: "GET",
			Path:   "/act_1/campaigns",
			Fields: []string{"id"},
		},
	})
	if err == nil {
		t.Fatal("expected run with lint spec and no runtime snapshot to fail")
	}
	if ExitCode(err) != ExitCodeInput {
		t.Fatalf("unexpected exit code: got=%d want=%d", ExitCode(err), ExitCodeInput)
	}
}

func TestRunWithOptionsIncludesRuntimeDriftFailure(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := InitBaseline(path); err != nil {
		t.Fatalf("init baseline: %v", err)
	}

	result, err := RunWithOptions(path, RunOptions{
		RuntimeResponse: &RuntimeResponseShapeSnapshot{
			Method:         "GET",
			Path:           "/act_1/campaigns",
			ObservedFields: []string{"id", "unknown_runtime_field"},
		},
	})
	if err != nil {
		t.Fatalf("run with runtime snapshot: %v", err)
	}
	if len(result.Report.Checks) != 5 {
		t.Fatalf("expected five checks, got %d", len(result.Report.Checks))
	}
	if result.Report.Checks[4].Name != checkNameRuntimeResponseShapeDrift {
		t.Fatalf("unexpected runtime drift check name: %s", result.Report.Checks[4].Name)
	}
	if result.Report.Checks[4].Status != CheckStatusFail {
		t.Fatalf("unexpected runtime drift check status: %s", result.Report.Checks[4].Status)
	}
	if !result.Report.Checks[4].Blocking {
		t.Fatal("expected runtime drift check failure to be blocking")
	}
	if result.Report.Summary.Blocking != 1 {
		t.Fatalf("unexpected summary blocking count: %+v", result.Report.Summary)
	}
}

func TestRunWithOptionsRejectsInvalidRuntimeSnapshot(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := InitBaseline(path); err != nil {
		t.Fatalf("init baseline: %v", err)
	}

	_, err := RunWithOptions(path, RunOptions{
		RuntimeResponse: &RuntimeResponseShapeSnapshot{
			Method:         "GET",
			Path:           "/act_1/campaigns",
			ObservedFields: []string{},
		},
	})
	if err == nil {
		t.Fatal("expected run to fail with invalid runtime snapshot")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != ExitCodeInput {
		t.Fatalf("unexpected exit code: got=%d want=%d", exitErr.Code, ExitCodeInput)
	}
}
