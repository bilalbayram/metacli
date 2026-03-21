package insights

import (
	"reflect"
	"testing"
)

func TestNormalizeLocalIntentRowsPreservesRawActionFieldsAndAddsAliases(t *testing.T) {
	t.Parallel()

	rawActions := []any{
		map[string]any{"action_type": "onsite_conversion.business_address_tap", "value": "12"},
		map[string]any{"action_type": "onsite_conversion.call", "value": "4"},
		map[string]any{"action_type": "onsite_conversion.get_directions", "value": "7"},
		map[string]any{"action_type": "onsite_conversion.profile_visit", "value": "18"},
	}
	rawCosts := []any{
		map[string]any{"action_type": "onsite_conversion.business_address_tap", "value": "1.75"},
	}
	rows := []map[string]any{
		{
			"campaign_id":          "1",
			"actions":              rawActions,
			"cost_per_action_type": rawCosts,
		},
	}

	normalized := NormalizeLocalIntentRows(rows)
	if len(normalized) != 1 {
		t.Fatalf("expected 1 row, got %d", len(normalized))
	}

	row := normalized[0]
	if !reflect.DeepEqual(row["actions"], rawActions) {
		t.Fatalf("expected raw actions to be preserved, got %#v", row["actions"])
	}
	if !reflect.DeepEqual(row["cost_per_action_type"], rawCosts) {
		t.Fatalf("expected raw cost_per_action_type to be preserved, got %#v", row["cost_per_action_type"])
	}
	if got := row["address_taps"]; got != int64(12) {
		t.Fatalf("unexpected address_taps value %#v", got)
	}
	if got := row["calls"]; got != int64(4) {
		t.Fatalf("unexpected calls value %#v", got)
	}
	if got := row["directions"]; got != int64(7) {
		t.Fatalf("unexpected directions value %#v", got)
	}
	if got := row["profile_visits"]; got != int64(18) {
		t.Fatalf("unexpected profile_visits value %#v", got)
	}
}

func TestNormalizeLocalIntentRowsOmitsMissingAliases(t *testing.T) {
	t.Parallel()

	normalized := NormalizeLocalIntentRows([]map[string]any{
		{
			"actions": []any{
				map[string]any{"action_type": "link_click", "value": "3"},
			},
		},
	})

	row := normalized[0]
	for _, key := range []string{"address_taps", "calls", "directions", "profile_visits"} {
		if _, ok := row[key]; ok {
			t.Fatalf("expected %s to be omitted when raw action is absent", key)
		}
	}
}

func TestDiscoverActionTypesDedupesAcrossRowsAndSources(t *testing.T) {
	t.Parallel()

	discovered := DiscoverActionTypes([]map[string]any{
		{
			"actions": []any{
				map[string]any{"action_type": "onsite_conversion.business_address_tap", "value": "12"},
				map[string]any{"action_type": "link_click", "value": "9"},
			},
			"cost_per_action_type": []any{
				map[string]any{"action_type": "onsite_conversion.business_address_tap", "value": "1.75"},
			},
		},
		{
			"actions": []any{
				map[string]any{"action_type": "onsite_conversion.business_address_tap", "value": "20"},
			},
			"cost_per_action_type": []any{
				map[string]any{"action_type": "link_click", "value": "0.80"},
			},
		},
	})

	if len(discovered) != 2 {
		t.Fatalf("expected 2 discovered action types, got %d", len(discovered))
	}

	first := discovered[0]
	if got := first["action_type"]; got != "link_click" {
		t.Fatalf("unexpected first action type %#v", got)
	}
	if got := first["sources"]; !reflect.DeepEqual(got, []string{"actions", "cost_per_action_type"}) {
		t.Fatalf("unexpected sources for link_click %#v", got)
	}
	if _, ok := first["normalized_field"]; ok {
		t.Fatalf("expected no normalized field for link_click, got %#v", first["normalized_field"])
	}

	second := discovered[1]
	if got := second["action_type"]; got != "onsite_conversion.business_address_tap" {
		t.Fatalf("unexpected second action type %#v", got)
	}
	if got := second["normalized_field"]; got != "address_taps" {
		t.Fatalf("unexpected normalized field %#v", got)
	}
	if got := second["sources"]; !reflect.DeepEqual(got, []string{"actions", "cost_per_action_type"}) {
		t.Fatalf("unexpected sources for address tap %#v", got)
	}
}
