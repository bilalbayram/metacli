#!/usr/bin/env bash
set -euo pipefail

require_bin() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required binary: $1" >&2
    exit 1
  fi
}

fail() {
  echo "$1" >&2
  exit 1
}

require_bin jq

META_BIN="${META_BIN:-./meta}"
META_PROFILE="${META_PROFILE:-prod}"
META_DATE_PRESET="${META_DATE_PRESET:-last_30d}"
META_ACTION_TYPES_LEVEL="${META_ACTION_TYPES_LEVEL:-ad}"
META_LOCAL_INTENT_LEVEL="${META_LOCAL_INTENT_LEVEL:-account}"
META_ACTION_TYPES_LIMIT="${META_ACTION_TYPES_LIMIT:-200}"
META_LOCAL_INTENT_LIMIT="${META_LOCAL_INTENT_LIMIT:-5}"
META_GRAPH_VERSION="${META_GRAPH_VERSION:-}"
: "${META_ACCOUNT_ID:?set META_ACCOUNT_ID to an ad account id (with or without act_ prefix)}"

if [[ ! -x "$META_BIN" ]]; then
  echo "meta binary is not executable: $META_BIN" >&2
  echo "build it first: go build -o meta ./cmd/meta" >&2
  exit 1
fi

version_args=()
if [[ -n "$META_GRAPH_VERSION" ]]; then
  version_args=(--version "$META_GRAPH_VERSION")
fi

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

action_types_file="$work_dir/action-types.json"
local_intent_file="$work_dir/local-intent.json"

run_meta() {
  "$META_BIN" --profile "$META_PROFILE" "$@" "${version_args[@]}"
}

assert_success_envelope() {
  local payload="$1"
  local command_name="$2"
  printf '%s' "$payload" | jq -e --arg cmd "$command_name" '
    .command == $cmd and
    .contract_version == "1.0" and
    .success == true
  ' >/dev/null
}

assert_action_types_shape() {
  local payload="$1"
  printf '%s' "$payload" | jq -e '
    .data | type == "array"
  ' >/dev/null

  printf '%s' "$payload" | jq -e '
    [.data[]?.action_type] | length == (unique | length)
  ' >/dev/null

  printf '%s' "$payload" | jq -e '
    all(.data[]?;
      (.action_type | type == "string" and length > 0) and
      (.sources | type == "array" and length > 0) and
      all(.sources[]; . == "actions" or . == "cost_per_action_type") and
      (
        if has("normalized_field") then
          .normalized_field == "address_taps" or
          .normalized_field == "calls" or
          .normalized_field == "directions" or
          .normalized_field == "profile_visits"
        else
          true
        end
      )
    )
  ' >/dev/null
}

assert_local_intent_shape() {
  local payload="$1"
  printf '%s' "$payload" | jq -e '
    .data | type == "array" and length > 0
  ' >/dev/null || fail "local-intent query returned zero rows; widen META_DATE_PRESET or choose an account with recent insights data"

  printf '%s' "$payload" | jq -e '
    all(.data[];
      type == "object" and
      (has("normalized_actions") | not) and
      (if has("actions") then (.actions | type == "array") else true end) and
      (if has("cost_per_action_type") then (.cost_per_action_type | type == "array") else true end) and
      (if has("actions") then all(.actions[]?; (.action_type | type == "string") and has("value")) else true end) and
      (if has("cost_per_action_type") then all(.cost_per_action_type[]?; (.action_type | type == "string") and has("value")) else true end) and
      (if has("address_taps") then (.address_taps | type == "number") else true end) and
      (if has("calls") then (.calls | type == "number") else true end) and
      (if has("directions") then (.directions | type == "number") else true end) and
      (if has("profile_visits") then (.profile_visits | type == "number") else true end)
    )
  ' >/dev/null
}

action_types_payload="$(
  run_meta insights action-types \
    --account-id "$META_ACCOUNT_ID" \
    --date-preset "$META_DATE_PRESET" \
    --level "$META_ACTION_TYPES_LEVEL" \
    --limit "$META_ACTION_TYPES_LIMIT" \
    --format json
)"
printf '%s' "$action_types_payload" >"$action_types_file"
assert_success_envelope "$action_types_payload" "meta insights action-types"
assert_action_types_shape "$action_types_payload"

local_intent_payload="$(
  run_meta insights run \
    --account-id "$META_ACCOUNT_ID" \
    --date-preset "$META_DATE_PRESET" \
    --level "$META_LOCAL_INTENT_LEVEL" \
    --metric-pack local_intent \
    --limit "$META_LOCAL_INTENT_LIMIT" \
    --format json
)"
printf '%s' "$local_intent_payload" >"$local_intent_file"
assert_success_envelope "$local_intent_payload" "meta insights run"
assert_local_intent_shape "$local_intent_payload"

discovered_sources_json="$(jq -c '[.data[]?.sources[]?] | unique' "$action_types_file")"
present_raw_fields_json="$(jq -c '[.data[] | keys_unsorted[] | select(. == "actions" or . == "cost_per_action_type")] | unique' "$local_intent_file")"
missing_raw_fields_json="$(jq -cn --argjson discovered "$discovered_sources_json" --argjson present "$present_raw_fields_json" '$discovered - $present')"
if [[ "$missing_raw_fields_json" != "[]" ]]; then
  fail "local-intent output is missing raw fields seen by action discovery: $missing_raw_fields_json"
fi

discovered_alias_fields_json="$(jq -c '[.data[]? | select(has("normalized_field")) | .normalized_field] | unique' "$action_types_file")"
present_alias_fields_json="$(jq -c '[.data[] | keys_unsorted[] | select(. == "address_taps" or . == "calls" or . == "directions" or . == "profile_visits")] | unique' "$local_intent_file")"
missing_alias_fields_json="$(jq -cn --argjson discovered "$discovered_alias_fields_json" --argjson present "$present_alias_fields_json" '$discovered - $present')"
if [[ "$missing_alias_fields_json" != "[]" ]]; then
  fail "local-intent output is missing normalized alias fields for discovered raw action types: $missing_alias_fields_json"
fi

action_type_count="$(jq -r '.data | length' "$action_types_file")"
local_intent_row_count="$(jq -r '.data | length' "$local_intent_file")"
discovered_alias_fields="$(jq -r '[.data[]? | select(has("normalized_field")) | .normalized_field] | unique | join(",")' "$action_types_file")"
present_alias_fields="$(jq -r '[.data[] | keys_unsorted[] | select(. == "address_taps" or . == "calls" or . == "directions" or . == "profile_visits")] | unique | join(",")' "$local_intent_file")"

echo "Insights local-intent smoke completed."
echo "profile=$META_PROFILE"
echo "account_id=$META_ACCOUNT_ID"
echo "date_preset=$META_DATE_PRESET"
echo "action_types_level=$META_ACTION_TYPES_LEVEL"
echo "local_intent_level=$META_LOCAL_INTENT_LEVEL"
echo "action_type_count=$action_type_count"
echo "local_intent_row_count=$local_intent_row_count"
if [[ -n "$discovered_alias_fields" ]]; then
  echo "discovered_alias_fields=$discovered_alias_fields"
  echo "present_alias_fields=$present_alias_fields"
else
  echo "discovered_alias_fields="
  echo "present_alias_fields=$present_alias_fields"
  echo "note=no known local-intent aliases were discovered for this account/date preset"
fi
