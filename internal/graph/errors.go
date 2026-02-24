package graph

import (
	"fmt"
	"sort"
	"strings"
)

const (
	RemediationCategoryAuth       = "auth"
	RemediationCategoryPermission = "permission"
	RemediationCategoryRateLimit  = "rate_limit"
	RemediationCategoryValidation = "validation"
	RemediationCategoryNotFound   = "not_found"
	RemediationCategoryConflict   = "conflict"
	RemediationCategoryTransient  = "transient"
	RemediationCategoryUnknown    = "unknown"
)

type Remediation struct {
	Category string   `json:"category"`
	Summary  string   `json:"summary"`
	Actions  []string `json:"actions,omitempty"`
	Fields   []string `json:"fields,omitempty"`
}

type APIError struct {
	Type         string         `json:"type"`
	Code         int            `json:"code"`
	ErrorSubcode int            `json:"error_subcode"`
	Message      string         `json:"message"`
	FBTraceID    string         `json:"fbtrace_id"`
	Retryable    bool           `json:"retryable"`
	Remediation  *Remediation   `json:"remediation,omitempty"`
	Diagnostics  map[string]any `json:"diagnostics,omitempty"`
	StatusCode   int            `json:"-"`
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf(
		"meta api error type=%s code=%d subcode=%d fbtrace_id=%s: %s",
		e.Type,
		e.Code,
		e.ErrorSubcode,
		e.FBTraceID,
		e.Message,
	)
}

type TransientError struct {
	Message    string
	StatusCode int
}

func (e *TransientError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func ClassifyRemediation(statusCode int, code int, subcode int, _ string, diagnostics map[string]any) Remediation {
	blameFields := extractBlameFields(diagnostics)

	switch {
	case statusCode == 429 || code == 4 || code == 17 || code == 32 || code == 613:
		return Remediation{
			Category: RemediationCategoryRateLimit,
			Summary:  "Meta API rate limits were reached.",
			Actions: []string{
				"Retry with exponential backoff.",
				"Reduce request concurrency for this account/app.",
			},
		}
	case code == 190:
		return Remediation{
			Category: RemediationCategoryAuth,
			Summary:  "Access token is invalid or expired.",
			Actions: []string{
				"Run `meta auth validate --profile <name>` to confirm token health.",
				"Refresh credentials with `meta auth setup` or `meta auth login`.",
			},
		}
	case code == 200 || code == 10:
		return Remediation{
			Category: RemediationCategoryPermission,
			Summary:  "Profile token is missing required permissions for this operation.",
			Actions: []string{
				"Verify required scopes are granted for the active profile.",
				"Confirm the token has access to the target business asset.",
			},
		}
	case code == 100 && subcode == 33:
		return Remediation{
			Category: RemediationCategoryNotFound,
			Summary:  "Referenced object or edge does not exist for this request.",
			Actions: []string{
				"Check object IDs and endpoint path for typos.",
				"Ensure the object belongs to the authenticated account context.",
			},
		}
	case code == 100:
		remediation := Remediation{
			Category: RemediationCategoryValidation,
			Summary:  "Request payload failed Meta validation.",
			Actions: []string{
				"Review required fields and payload shape for the endpoint.",
			},
			Fields: blameFields,
		}
		if len(blameFields) > 0 {
			remediation.Actions = append(remediation.Actions, fmt.Sprintf("Fix invalid field paths: %s.", strings.Join(blameFields, ", ")))
		}
		return remediation
	case statusCode >= 500:
		return Remediation{
			Category: RemediationCategoryTransient,
			Summary:  "Meta API returned a transient server-side failure.",
			Actions: []string{
				"Retry with backoff.",
				"Capture fbtrace_id if failures continue across retries.",
			},
		}
	default:
		return Remediation{
			Category: RemediationCategoryUnknown,
			Summary:  "Unknown Meta API failure.",
			Actions: []string{
				"Inspect `error.diagnostics` and `fbtrace_id` for the full Meta response.",
				"Adjust request inputs and retry after validation.",
			},
			Fields: blameFields,
		}
	}
}

func extractBlameFields(diagnostics map[string]any) []string {
	if len(diagnostics) == 0 {
		return nil
	}
	errorDataRaw, ok := diagnostics["error_data"]
	if !ok {
		return nil
	}
	errorData, ok := errorDataRaw.(map[string]any)
	if !ok {
		return nil
	}
	specsRaw, ok := errorData["blame_field_specs"]
	if !ok {
		return nil
	}
	specs, ok := specsRaw.([]any)
	if !ok {
		return nil
	}

	seen := map[string]struct{}{}
	for _, spec := range specs {
		field := normalizeBlameFieldSpec(spec)
		if field == "" {
			continue
		}
		seen[field] = struct{}{}
	}
	if len(seen) == 0 {
		return nil
	}

	fields := make([]string, 0, len(seen))
	for field := range seen {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

func normalizeBlameFieldSpec(raw any) string {
	switch typed := raw.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			part := normalizeBlameFieldSpec(item)
			if part == "" {
				continue
			}
			parts = append(parts, part)
		}
		return strings.Join(parts, ".")
	default:
		return strings.TrimSpace(fmt.Sprint(raw))
	}
}
