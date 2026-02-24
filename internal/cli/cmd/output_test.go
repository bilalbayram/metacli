package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/bilalbayram/metacli/internal/graph"
	"github.com/spf13/cobra"
)

func TestWriteCommandErrorIncludesStructuredGraphRemediation(t *testing.T) {
	t.Parallel()

	errOutput := &bytes.Buffer{}
	cmd := &cobra.Command{Use: "test"}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(errOutput)

	apiErr := &graph.APIError{
		Type:         "GraphMethodException",
		Code:         100,
		ErrorSubcode: 33,
		StatusCode:   400,
		Message:      "Unsupported post request",
		FBTraceID:    "trace-1",
		Retryable:    false,
		Remediation: &graph.Remediation{
			Category: graph.RemediationCategoryNotFound,
			Summary:  "Referenced object or edge does not exist for this request.",
			Actions: []string{
				"Check object IDs and endpoint path for typos.",
			},
			Fields: []string{"campaign_id"},
		},
		Diagnostics: map[string]any{
			"error_user_title": "Unsupported post request",
		},
	}

	returnedErr := writeCommandError(cmd, runtimeWithJSONOutput(), "meta api post", apiErr)
	if !errors.Is(returnedErr, apiErr) {
		t.Fatalf("expected original error to be returned, got %v", returnedErr)
	}

	envelope := decodeCommandOutputEnvelope(t, errOutput.Bytes())
	errorBody, ok := envelope["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error payload, got %T", envelope["error"])
	}
	if got := errorBody["status_code"]; got != float64(400) {
		t.Fatalf("unexpected status_code %v", got)
	}

	remediation, ok := errorBody["remediation"].(map[string]any)
	if !ok {
		t.Fatalf("expected remediation payload, got %T", errorBody["remediation"])
	}
	if got := remediation["category"]; got != graph.RemediationCategoryNotFound {
		t.Fatalf("unexpected remediation category %v", got)
	}

	diagnostics, ok := errorBody["diagnostics"].(map[string]any)
	if !ok {
		t.Fatalf("expected diagnostics payload, got %T", errorBody["diagnostics"])
	}
	if got := diagnostics["error_user_title"]; got != "Unsupported post request" {
		t.Fatalf("unexpected diagnostics payload %#v", diagnostics)
	}
}

func TestWriteCommandErrorAddsDefaultRemediationForGenericErrors(t *testing.T) {
	t.Parallel()

	errOutput := &bytes.Buffer{}
	cmd := &cobra.Command{Use: "test"}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(errOutput)

	execErr := errors.New("caption is required")
	returnedErr := writeCommandError(cmd, runtimeWithJSONOutput(), "meta ig publish feed", execErr)
	if !errors.Is(returnedErr, execErr) {
		t.Fatalf("expected original error to be returned, got %v", returnedErr)
	}

	envelope := decodeCommandOutputEnvelope(t, errOutput.Bytes())
	errorBody, ok := envelope["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error payload, got %T", envelope["error"])
	}

	remediation, ok := errorBody["remediation"].(map[string]any)
	if !ok {
		t.Fatalf("expected remediation payload, got %T", errorBody["remediation"])
	}
	if got := remediation["category"]; got != graph.RemediationCategoryUnknown {
		t.Fatalf("unexpected remediation category %v", got)
	}
}

func runtimeWithJSONOutput() Runtime {
	output := "json"
	return Runtime{Output: &output}
}

func decodeCommandOutputEnvelope(t *testing.T, raw []byte) map[string]any {
	t.Helper()

	decoded := map[string]any{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return decoded
}
