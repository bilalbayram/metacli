package insights

import (
	"strconv"
	"strings"
)

const (
	actionsField           = "actions"
	costPerActionTypeField = "cost_per_action_type"
)

type localIntentAlias struct {
	actionType string
	field      string
}

var localIntentAliases = []localIntentAlias{
	{actionType: "onsite_conversion.business_address_tap", field: "address_taps"},
	{actionType: "click_to_call_native_call_placed", field: "calls"},
	{actionType: "onsite_conversion.get_directions", field: "directions"},
	{actionType: "onsite_conversion.profile_visit", field: "profile_visits"},
}

var actionDiscoverySources = []string{actionsField, costPerActionTypeField}

func NormalizeLocalIntentRows(rows []map[string]any) []map[string]any {
	normalized := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		cloned := cloneRow(row)
		values := actionValues(row[actionsField])
		for _, alias := range localIntentAliases {
			value, ok := values[alias.actionType]
			if !ok {
				continue
			}
			cloned[alias.field] = value
		}
		normalized = append(normalized, cloned)
	}
	return normalized
}

func SummarizeLocalIntentRows(rows []map[string]any) map[string]any {
	summary := make(map[string]any)
	for _, alias := range localIntentAliases {
		var total float64
		var hasValue bool
		for _, row := range rows {
			value, ok := numericValue(row[alias.field])
			if !ok {
				continue
			}
			total += value
			hasValue = true
		}
		if !hasValue {
			continue
		}
		summary[alias.field] = compactNumericValue(total)
	}
	return summary
}

func DiscoverActionTypes(rows []map[string]any) []map[string]any {
	discovered := make(map[string]map[string]struct{})
	for _, row := range rows {
		for _, source := range actionDiscoverySources {
			for _, entry := range actionEntries(row[source]) {
				actionType := actionTypeFromEntry(entry)
				if actionType == "" {
					continue
				}
				sourceSet, ok := discovered[actionType]
				if !ok {
					sourceSet = make(map[string]struct{}, len(actionDiscoverySources))
					discovered[actionType] = sourceSet
				}
				sourceSet[source] = struct{}{}
			}
		}
	}

	actionTypes := make([]string, 0, len(discovered))
	for actionType := range discovered {
		actionTypes = append(actionTypes, actionType)
	}
	sortStrings(actionTypes)

	results := make([]map[string]any, 0, len(actionTypes))
	for _, actionType := range actionTypes {
		record := map[string]any{
			"action_type": actionType,
			"sources":     orderedSources(discovered[actionType]),
		}
		if normalizedField := normalizedFieldForActionType(actionType); normalizedField != "" {
			record["normalized_field"] = normalizedField
		}
		results = append(results, record)
	}
	return results
}

func cloneRow(row map[string]any) map[string]any {
	cloned := make(map[string]any, len(row))
	for key, value := range row {
		cloned[key] = value
	}
	return cloned
}

func actionValues(raw any) map[string]any {
	values := make(map[string]any)
	for _, entry := range actionEntries(raw) {
		actionType := actionTypeFromEntry(entry)
		if actionType == "" {
			continue
		}
		if _, exists := values[actionType]; exists {
			continue
		}
		value, ok := normalizedActionValue(entry["value"])
		if !ok {
			continue
		}
		values[actionType] = value
	}
	return values
}

func actionEntries(raw any) []map[string]any {
	switch typed := raw.(type) {
	case []map[string]any:
		return typed
	case []any:
		entries := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			entries = append(entries, entry)
		}
		return entries
	default:
		return nil
	}
}

func actionTypeFromEntry(entry map[string]any) string {
	actionType, _ := entry["action_type"].(string)
	return strings.TrimSpace(actionType)
}

func normalizedActionValue(raw any) (any, bool) {
	switch typed := raw.(type) {
	case nil:
		return nil, false
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil, false
		}
		if integerValue, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return integerValue, true
		}
		if floatValue, err := strconv.ParseFloat(trimmed, 64); err == nil {
			return floatValue, true
		}
		return trimmed, true
	case int:
		return typed, true
	case int8:
		return typed, true
	case int16:
		return typed, true
	case int32:
		return typed, true
	case int64:
		return typed, true
	case float32:
		return typed, true
	case float64:
		return typed, true
	default:
		return typed, true
	}
}

func numericValue(raw any) (float64, bool) {
	switch typed := raw.(type) {
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float32:
		return float64(typed), true
	case float64:
		return typed, true
	default:
		return 0, false
	}
}

func compactNumericValue(value float64) any {
	if value == float64(int64(value)) {
		return int64(value)
	}
	return value
}

func normalizedFieldForActionType(actionType string) string {
	for _, alias := range localIntentAliases {
		if alias.actionType == actionType {
			return alias.field
		}
	}
	return ""
}

func orderedSources(sourceSet map[string]struct{}) []string {
	sources := make([]string, 0, len(sourceSet))
	for _, source := range actionDiscoverySources {
		if _, ok := sourceSet[source]; ok {
			sources = append(sources, source)
		}
	}
	return sources
}

func sortStrings(values []string) {
	for index := 1; index < len(values); index++ {
		current := values[index]
		position := index - 1
		for position >= 0 && values[position] > current {
			values[position+1] = values[position]
			position--
		}
		values[position+1] = current
	}
}
