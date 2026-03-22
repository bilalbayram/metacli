package linkedin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type ErrorCategory string

const (
	ErrorCategoryAuth       ErrorCategory = "auth"
	ErrorCategoryPermission ErrorCategory = "permission"
	ErrorCategoryValidation ErrorCategory = "validation"
	ErrorCategoryRateLimit  ErrorCategory = "rate_limit"
	ErrorCategoryVersion    ErrorCategory = "version"
	ErrorCategoryTransient  ErrorCategory = "transient"
	ErrorCategoryNotFound   ErrorCategory = "not_found"
	ErrorCategoryConflict   ErrorCategory = "conflict"
	ErrorCategoryUnknown    ErrorCategory = "unknown"
)

type Error struct {
	Category         ErrorCategory  `json:"category"`
	StatusCode       int            `json:"status_code"`
	Code             string         `json:"code,omitempty"`
	ServiceErrorCode int            `json:"service_error_code,omitempty"`
	ErrorDetailType  string         `json:"error_detail_type,omitempty"`
	Message          string         `json:"message"`
	Retryable        bool           `json:"retryable"`
	Raw              []byte         `json:"raw,omitempty"`
	Diagnostics      map[string]any `json:"diagnostics,omitempty"`
}

type APIError = Error

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	parts := []string{fmt.Sprintf("linkedin api error category=%s status=%d", e.Category, e.StatusCode)}
	if strings.TrimSpace(e.Code) != "" {
		parts = append(parts, fmt.Sprintf("code=%s", e.Code))
	}
	if strings.TrimSpace(e.ErrorDetailType) != "" {
		parts = append(parts, fmt.Sprintf("detail_type=%s", e.ErrorDetailType))
	}
	if strings.TrimSpace(e.Message) != "" {
		parts = append(parts, e.Message)
	}
	return strings.Join(parts, ": ")
}

func ValidateVersion(version string) error {
	if strings.TrimSpace(version) == "" {
		return fmt.Errorf("linkedin version is required")
	}
	return nil
}

func parseAPIError(statusCode int, decoded any, headers http.Header, raw []byte) error {
	payload, _ := decoded.(map[string]any)
	message := extractMessage(payload)
	category := classifyLinkedInCategory(statusCode, payload, message)
	return &Error{
		Category:    category,
		StatusCode:  statusCode,
		Message:     message,
		Retryable:   category == ErrorCategoryTransient || category == ErrorCategoryRateLimit,
		Raw:         append([]byte(nil), raw...),
		Diagnostics: cloneAnyMap(payload),
	}
}

func classifyLinkedInError(statusCode int, body []byte) error {
	var decoded any
	if len(body) > 0 {
		_ = json.Unmarshal(body, &decoded)
	}
	return parseAPIError(statusCode, decoded, nil, body)
}

func classifyLinkedInCategory(statusCode int, payload map[string]any, message string) ErrorCategory {
	_ = payload
	upper := strings.ToUpper(strings.TrimSpace(message))
	switch {
	case statusCode == http.StatusUnauthorized || strings.Contains(upper, "INVALID TOKEN") || strings.Contains(upper, "EXPIRED"):
		return ErrorCategoryAuth
	case statusCode == http.StatusForbidden || strings.Contains(upper, "PERMISSION") || strings.Contains(upper, "ACCESS DENIED"):
		return ErrorCategoryPermission
	case statusCode == http.StatusNotFound:
		return ErrorCategoryNotFound
	case statusCode == http.StatusConflict:
		return ErrorCategoryConflict
	case statusCode == http.StatusTooManyRequests || strings.Contains(upper, "RATE") || strings.Contains(upper, "THROTTLE"):
		return ErrorCategoryRateLimit
	case statusCode == http.StatusUpgradeRequired || strings.Contains(upper, "VERSION"):
		return ErrorCategoryVersion
	case statusCode >= 500:
		return ErrorCategoryTransient
	case statusCode == http.StatusBadRequest:
		if strings.Contains(upper, "VERSION") {
			return ErrorCategoryVersion
		}
		return ErrorCategoryValidation
	default:
		if strings.Contains(upper, "VERSION") {
			return ErrorCategoryVersion
		}
		if strings.Contains(upper, "RATE") {
			return ErrorCategoryRateLimit
		}
		if strings.Contains(upper, "PERMISSION") {
			return ErrorCategoryPermission
		}
		if strings.Contains(upper, "NOT FOUND") {
			return ErrorCategoryNotFound
		}
		return ErrorCategoryUnknown
	}
}

func cloneAnyMap(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = cloneAnyValue(value)
	}
	return cloned
}

func cloneAnyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnyMap(typed)
	case []any:
		cloned := make([]any, 0, len(typed))
		for _, item := range typed {
			cloned = append(cloned, cloneAnyValue(item))
		}
		return cloned
	default:
		return typed
	}
}

func stringField(payload map[string]any, key string) string {
	if len(payload) == 0 {
		return ""
	}
	value, ok := payload[key]
	if !ok {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}

func mustMarshalLoose(v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return raw
}

func extractMessage(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	if errObj, ok := payload["error"].(map[string]any); ok {
		if message, ok := errObj["message"].(string); ok && strings.TrimSpace(message) != "" {
			return strings.TrimSpace(message)
		}
		if desc, ok := errObj["error_description"].(string); ok && strings.TrimSpace(desc) != "" {
			return strings.TrimSpace(desc)
		}
	}
	if message, ok := payload["message"].(string); ok && strings.TrimSpace(message) != "" {
		return strings.TrimSpace(message)
	}
	if desc, ok := payload["error_description"].(string); ok && strings.TrimSpace(desc) != "" {
		return strings.TrimSpace(desc)
	}
	return ""
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnyMap(typed)
	case []any:
		cloned := make([]any, 0, len(typed))
		for _, item := range typed {
			cloned = append(cloned, cloneAny(item))
		}
		return cloned
	default:
		return typed
	}
}
