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
		return apiErr
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
		}
	}

	return &graph.APIError{
		Type:      igErrorTypeValidation,
		Code:      igErrorCodeValidation,
		Message:   err.Error(),
		Retryable: false,
	}
}

func newStateTransitionError(scheduleID string, from string, to string) *graph.APIError {
	return &graph.APIError{
		Type:      igErrorTypeStateTransition,
		Code:      igErrorCodeStateTransition,
		Message:   fmt.Sprintf("schedule %s cannot transition from %s to %s", scheduleID, from, to),
		Retryable: false,
	}
}

func newScheduleNotFoundError(scheduleID string) *graph.APIError {
	return &graph.APIError{
		Type:      igErrorTypeNotFound,
		Code:      igErrorCodeNotFound,
		Message:   fmt.Sprintf("schedule %s not found", scheduleID),
		Retryable: false,
	}
}

func newIdempotencyConflictError(idempotencyKey string, scheduleID string) *graph.APIError {
	return &graph.APIError{
		Type:      igErrorTypeIdempotencyConflict,
		Code:      igErrorCodeIdempotencyConflict,
		Message:   fmt.Sprintf("idempotency key %q already maps to schedule %s with different payload", idempotencyKey, scheduleID),
		Retryable: false,
	}
}

func newMediaNotReadyError(statusCode string) *graph.APIError {
	return &graph.APIError{
		Type:      igErrorTypeMediaNotReady,
		Code:      igErrorCodeMediaNotReady,
		Message:   fmt.Sprintf("instagram media container is not ready for publish: status_code=%s", statusCode),
		Retryable: true,
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
