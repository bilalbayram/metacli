package marketing

import (
	"fmt"
	"strings"
)

const readFilterStatusActive = "ACTIVE"

type entityReadFilters struct {
	NameContains      string
	Statuses          map[string]struct{}
	EffectiveStatuses map[string]struct{}
	ActiveOnly        bool
}

func normalizeEntityReadFields(entity string, fields []string, defaults []string) ([]string, error) {
	if len(fields) == 0 {
		return append([]string(nil), defaults...), nil
	}

	normalized := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		if trimmed == "" {
			return nil, fmt.Errorf("%s fields contain blank entries", entity)
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return nil, fmt.Errorf("%s fields are required when fields filter is set", entity)
	}
	return normalized, nil
}

func normalizeEntityReadFilters(name string, statuses []string, effectiveStatuses []string, activeOnly bool) (entityReadFilters, error) {
	statusFilter, err := normalizeEntityStatusSet("status", statuses)
	if err != nil {
		return entityReadFilters{}, err
	}
	effectiveStatusFilter, err := normalizeEntityStatusSet("effective status", effectiveStatuses)
	if err != nil {
		return entityReadFilters{}, err
	}

	return entityReadFilters{
		NameContains:      strings.ToLower(strings.TrimSpace(name)),
		Statuses:          statusFilter,
		EffectiveStatuses: effectiveStatusFilter,
		ActiveOnly:        activeOnly,
	}, nil
}

func normalizeEntityStatusSet(label string, values []string) (map[string]struct{}, error) {
	if len(values) == 0 {
		return map[string]struct{}{}, nil
	}

	normalized := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.ToUpper(strings.TrimSpace(value))
		if trimmed == "" {
			return nil, fmt.Errorf("%s filter contains blank entries", label)
		}
		normalized[trimmed] = struct{}{}
	}
	return normalized, nil
}

func mergeEntityReadFields(base []string, extras ...string) []string {
	out := make([]string, 0, len(base)+len(extras))
	seen := make(map[string]struct{}, len(base)+len(extras))
	for _, field := range base {
		trimmed := strings.TrimSpace(field)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	for _, field := range extras {
		trimmed := strings.TrimSpace(field)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func matchEntityReadFilters(item map[string]any, filters entityReadFilters) bool {
	if filters.NameContains != "" {
		nameValue := strings.ToLower(entityItemStringValue(item, "name"))
		if !strings.Contains(nameValue, filters.NameContains) {
			return false
		}
	}

	statusValue := strings.ToUpper(entityItemStringValue(item, "status"))
	if len(filters.Statuses) > 0 {
		if _, exists := filters.Statuses[statusValue]; !exists {
			return false
		}
	}

	effectiveStatusValue := strings.ToUpper(entityItemStringValue(item, "effective_status"))
	if len(filters.EffectiveStatuses) > 0 {
		if _, exists := filters.EffectiveStatuses[effectiveStatusValue]; !exists {
			return false
		}
	}

	if filters.ActiveOnly {
		if statusValue != readFilterStatusActive && effectiveStatusValue != readFilterStatusActive {
			return false
		}
	}

	return true
}

func entityItemStringValue(item map[string]any, key string) string {
	if len(item) == 0 {
		return ""
	}
	value, exists := item[key]
	if !exists || value == nil {
		return ""
	}
	if typed, ok := value.(string); ok {
		return strings.TrimSpace(typed)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func projectEntityReadFields(item map[string]any, fields []string) map[string]any {
	projected := make(map[string]any, len(fields))
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		if trimmed == "" {
			continue
		}
		value, exists := item[trimmed]
		if exists {
			projected[trimmed] = value
			continue
		}
		projected[trimmed] = nil
	}
	return projected
}

func matchEntityIDFilter(item map[string]any, field string, expectedID string) bool {
	expected := strings.TrimSpace(expectedID)
	if expected == "" {
		return true
	}
	return entityItemStringValue(item, field) == expected
}
