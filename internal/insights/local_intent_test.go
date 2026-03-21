package insights

import (
	"reflect"
	"testing"
)

func TestNormalizeLocalIntentRowsPreservesRawActionFieldsAndAddsAliases(t *testing.T) {
	t.Parallel()

	rawActions := []any{
		map[string]any{"action_type": "onsite_conversion.business_address_tap", "value": "12"},
		map[string]any{"action_type": "click_to_call_native_call_placed", "value": "4"},
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

func TestNormalizeLocalIntentRowsLeavesCallConnectAndConfirmRawOnly(t *testing.T) {
	t.Parallel()

	normalized := NormalizeLocalIntentRows([]map[string]any{
		{
			"actions": []any{
				map[string]any{"action_type": "click_to_call_native_20s_call_connect", "value": "2"},
				map[string]any{"action_type": "click_to_call_native_60s_call_connect", "value": "1"},
				map[string]any{"action_type": "click_to_call_call_confirm", "value": "3"},
				map[string]any{"action_type": "call_confirm_grouped", "value": "4"},
			},
		},
	})

	if _, ok := normalized[0]["calls"]; ok {
		t.Fatalf("expected call connect/confirm actions to remain raw-only, got %#v", normalized[0]["calls"])
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
				map[string]any{"action_type": "click_to_call_native_call_placed", "value": "4"},
				map[string]any{"action_type": "link_click", "value": "9"},
			},
			"cost_per_action_type": []any{
				map[string]any{"action_type": "click_to_call_native_call_placed", "value": "8.50"},
			},
		},
		{
			"actions": []any{
				map[string]any{"action_type": "click_to_call_native_call_placed", "value": "8"},
				map[string]any{"action_type": "click_to_call_native_20s_call_connect", "value": "2"},
			},
			"cost_per_action_type": []any{
				map[string]any{"action_type": "link_click", "value": "0.80"},
			},
		},
	})

	if len(discovered) != 3 {
		t.Fatalf("expected 3 discovered action types, got %d", len(discovered))
	}

	first := discovered[0]
	if got := first["action_type"]; got != "click_to_call_native_20s_call_connect" {
		t.Fatalf("unexpected first action type %#v", got)
	}
	if _, ok := first["normalized_field"]; ok {
		t.Fatalf("expected no normalized field for call connect, got %#v", first["normalized_field"])
	}
	if got := first["sources"]; !reflect.DeepEqual(got, []string{"actions"}) {
		t.Fatalf("unexpected sources for call connect %#v", got)
	}

	second := discovered[1]
	if got := second["action_type"]; got != "click_to_call_native_call_placed" {
		t.Fatalf("unexpected second action type %#v", got)
	}
	if got := second["normalized_field"]; got != "calls" {
		t.Fatalf("unexpected normalized field %#v", got)
	}
	if got := second["sources"]; !reflect.DeepEqual(got, []string{"actions", "cost_per_action_type"}) {
		t.Fatalf("unexpected sources for call placed %#v", got)
	}

	third := discovered[2]
	if got := third["action_type"]; got != "link_click" {
		t.Fatalf("unexpected third action type %#v", got)
	}
	if _, ok := third["normalized_field"]; ok {
		t.Fatalf("expected no normalized field for link_click, got %#v", third["normalized_field"])
	}
	if got := third["sources"]; !reflect.DeepEqual(got, []string{"actions", "cost_per_action_type"}) {
		t.Fatalf("unexpected sources for link_click %#v", got)
	}
}

func TestSummarizeLocalIntentRowsSumsKnownAliasFields(t *testing.T) {
	t.Parallel()

	summary := SummarizeLocalIntentRows([]map[string]any{
		{"calls": int64(180), "directions": int64(7)},
		{"calls": int64(55), "profile_visits": int64(18)},
	})

	if got := summary["calls"]; got != int64(235) {
		t.Fatalf("unexpected calls summary %#v", got)
	}
	if got := summary["directions"]; got != int64(7) {
		t.Fatalf("unexpected directions summary %#v", got)
	}
	if got := summary["profile_visits"]; got != int64(18) {
		t.Fatalf("unexpected profile_visits summary %#v", got)
	}
	if _, ok := summary["address_taps"]; ok {
		t.Fatalf("expected address_taps to be omitted when not present, got %#v", summary["address_taps"])
	}
}
