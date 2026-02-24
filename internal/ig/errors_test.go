package ig

import (
	"errors"
	"testing"

	"github.com/bilalbayram/metacli/internal/graph"
)

func TestClassifyPublishScheduleErrorMarksTransientRetryable(t *testing.T) {
	t.Parallel()

	classified := ClassifyPublishScheduleError(&graph.TransientError{
		Message:    "transient status code 503",
		StatusCode: 503,
	})

	apiErr, ok := classified.(*graph.APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", classified)
	}
	if apiErr.Type != igErrorTypeTransient {
		t.Fatalf("unexpected type %q", apiErr.Type)
	}
	if !apiErr.Retryable {
		t.Fatalf("expected retryable=true, got %+v", apiErr)
	}
	if apiErr.Remediation == nil {
		t.Fatalf("expected remediation contract, got %+v", apiErr)
	}
	if apiErr.Remediation.Category != graph.RemediationCategoryTransient {
		t.Fatalf("unexpected remediation category %q", apiErr.Remediation.Category)
	}
}

func TestClassifyPublishScheduleErrorWrapsValidationError(t *testing.T) {
	t.Parallel()

	classified := ClassifyPublishScheduleError(errors.New("publish-at must be in the future"))
	apiErr, ok := classified.(*graph.APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", classified)
	}
	if apiErr.Type != igErrorTypeValidation {
		t.Fatalf("unexpected type %q", apiErr.Type)
	}
	if apiErr.Retryable {
		t.Fatalf("expected retryable=false, got %+v", apiErr)
	}
	if apiErr.Remediation == nil {
		t.Fatalf("expected remediation contract, got %+v", apiErr)
	}
	if apiErr.Remediation.Category != graph.RemediationCategoryValidation {
		t.Fatalf("unexpected remediation category %q", apiErr.Remediation.Category)
	}
}

func TestClassifyPublishScheduleErrorAddsMissingRemediationToGraphError(t *testing.T) {
	t.Parallel()

	classified := ClassifyPublishScheduleError(&graph.APIError{
		Type:         "GraphMethodException",
		Code:         100,
		ErrorSubcode: 33,
		Message:      "Unsupported post request",
		StatusCode:   400,
		Retryable:    false,
	})

	apiErr, ok := classified.(*graph.APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", classified)
	}
	if apiErr.Remediation == nil {
		t.Fatalf("expected remediation contract, got %+v", apiErr)
	}
	if apiErr.Remediation.Category != graph.RemediationCategoryNotFound {
		t.Fatalf("unexpected remediation category %q", apiErr.Remediation.Category)
	}
}

func TestNormalizeIdempotencyKeyValidation(t *testing.T) {
	t.Parallel()

	if _, err := normalizeIdempotencyKey("valid-key_01:feed"); err != nil {
		t.Fatalf("expected valid idempotency key, got %v", err)
	}

	if _, err := normalizeIdempotencyKey("invalid key"); err == nil {
		t.Fatal("expected invalid idempotency key error")
	}
}
