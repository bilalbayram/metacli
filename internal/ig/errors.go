package ig

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bilalbayram/metacli/internal/graph"
)

const (
	igErrorTypeValidation          = "ig_validation_error"
	igErrorTypeStateTransition     = "ig_state_transition_error"
	igErrorTypeNotFound            = "ig_not_found_error"
	igErrorTypeIdempotencyConflict = "ig_idempotency_conflict"
	igErrorTypeMediaNotReady       = "ig_media_not_ready"
	igErrorTypeTransient           = "ig_transient_error"

	igErrorCodeValidation          = 422000
	igErrorCodeStateTransition     = 409100
	igErrorCodeNotFound            = 404100
	igErrorCodeIdempotencyConflict = 409101
	igErrorCodeMediaNotReady       = 425100
	igErrorCodeTransient           = 503100
)

const maxIdempotencyKeyLength = 128

// ClassifyPublishScheduleError maps IG publish/schedule errors to a structured error
// envelope payload with explicit retryability semantics.
func ClassifyPublishScheduleError(err error) error {
	if err == nil {
		return nil
	}

	var apiErr *graph.APIError
	if errors.As(err, &apiErr) {
		classified := *apiErr
		if classified.Remediation == nil {
			remediation := graph.ClassifyRemediation(classified.StatusCode, classified.Code, classified.ErrorSubcode, classified.Message, classified.Diagnostics)
			classified.Remediation = &remediation
		}
		return &classified
	}

	var transientErr *graph.TransientError
	if errors.As(err, &transientErr) {
		statusCode := transientErr.StatusCode
		code := igErrorCodeTransient
		if statusCode > 0 {
			code = statusCode
		}
		return &graph.APIError{
			Type:       igErrorTypeTransient,
			Code:       code,
			Message:    transientErr.Error(),
			StatusCode: statusCode,
			Retryable:  true,
			Remediation: newIGRemediation(
				graph.RemediationCategoryTransient,
				"Temporary Instagram publish failure.",
				"Retry with exponential backoff.",
				"Capture diagnostics and fbtrace_id when retries keep failing.",
			),
		}
	}

	return &graph.APIError{
		Type:      igErrorTypeValidation,
		Code:      igErrorCodeValidation,
		Message:   err.Error(),
		Retryable: false,
		Remediation: newIGRemediation(
			graph.RemediationCategoryValidation,
			"Instagram publish request validation failed.",
			"Fix the invalid schedule or publish input and rerun the command.",
		),
	}
}

func newStateTransitionError(scheduleID string, from string, to string) *graph.APIError {
	return &graph.APIError{
		Type:      igErrorTypeStateTransition,
		Code:      igErrorCodeStateTransition,
		Message:   fmt.Sprintf("schedule %s cannot transition from %s to %s", scheduleID, from, to),
		Retryable: false,
		Remediation: newIGRemediation(
			graph.RemediationCategoryConflict,
			"Requested schedule transition is invalid for current state.",
			"List schedules and use a valid transition for the current status.",
		),
	}
}

func newScheduleNotFoundError(scheduleID string) *graph.APIError {
	return &graph.APIError{
		Type:      igErrorTypeNotFound,
		Code:      igErrorCodeNotFound,
		Message:   fmt.Sprintf("schedule %s not found", scheduleID),
		Retryable: false,
		Remediation: newIGRemediation(
			graph.RemediationCategoryNotFound,
			"Referenced schedule record does not exist.",
			"Verify schedule_id and schedule state path before retrying.",
		),
	}
}

func newIdempotencyConflictError(idempotencyKey string, scheduleID string) *graph.APIError {
	return &graph.APIError{
		Type:      igErrorTypeIdempotencyConflict,
		Code:      igErrorCodeIdempotencyConflict,
		Message:   fmt.Sprintf("idempotency key %q already maps to schedule %s with different payload", idempotencyKey, scheduleID),
		Retryable: false,
		Remediation: newIGRemediation(
			graph.RemediationCategoryConflict,
			"Idempotency key is already bound to a different payload.",
			"Reuse the original payload with this key or supply a new idempotency key.",
		),
	}
}

func newMediaNotReadyError(statusCode string) *graph.APIError {
	return &graph.APIError{
		Type:      igErrorTypeMediaNotReady,
		Code:      igErrorCodeMediaNotReady,
		Message:   fmt.Sprintf("instagram media container is not ready for publish: status_code=%s", statusCode),
		Retryable: true,
		Remediation: newIGRemediation(
			graph.RemediationCategoryTransient,
			"Instagram media container is still processing.",
			"Poll media status until status_code is FINISHED before publishing.",
		),
	}
}

func normalizeIdempotencyKey(raw string) (string, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return "", nil
	}
	if len(normalized) > maxIdempotencyKeyLength {
		return "", fmt.Errorf("idempotency key exceeds %d characters", maxIdempotencyKeyLength)
	}
	for _, char := range normalized {
		if isAllowedIdempotencyCharacter(char) {
			continue
		}
		return "", fmt.Errorf("invalid idempotency key %q: only A-Z, a-z, 0-9, '.', '-', '_', and ':' are supported", raw)
	}
	return normalized, nil
}

func isAllowedIdempotencyCharacter(char rune) bool {
	if char >= 'a' && char <= 'z' {
		return true
	}
	if char >= 'A' && char <= 'Z' {
		return true
	}
	if char >= '0' && char <= '9' {
		return true
	}

	switch char {
	case '.', '-', '_', ':':
		return true
	default:
		return false
	}
}

func newIGRemediation(category string, summary string, actions ...string) *graph.Remediation {
	cleanActions := make([]string, 0, len(actions))
	for _, action := range actions {
		trimmed := strings.TrimSpace(action)
		if trimmed == "" {
			continue
		}
		cleanActions = append(cleanActions, trimmed)
	}
	return &graph.Remediation{
		Category: category,
		Summary:  strings.TrimSpace(summary),
		Actions:  cleanActions,
	}
}
