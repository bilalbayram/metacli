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
META_DATE_PRESET="${META_DATE_PRESET:-last_7d}"
META_GRAPH_VERSION="${META_GRAPH_VERSION:-}"
META_IG_USER_ID="${META_IG_USER_ID:-}"
META_MEDIA_LIMIT="${META_MEDIA_LIMIT:-10}"
META_MEDIA_METRIC="${META_MEDIA_METRIC:-profile_visits}"
META_MEDIA_PERIOD="${META_MEDIA_PERIOD:-lifetime}"
META_IG_MEDIA_ID="${META_IG_MEDIA_ID:-}"
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

ig_user_args=()
if [[ -n "$META_IG_USER_ID" ]]; then
  ig_user_args=(--ig-user-id "$META_IG_USER_ID")
fi

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

run_meta() {
  if [[ ${#version_args[@]} -gt 0 ]]; then
    "$META_BIN" --profile "$META_PROFILE" "$@" "${version_args[@]}"
    return
  fi
  "$META_BIN" --profile "$META_PROFILE" "$@"
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

paid_payload="$(
  run_meta insights run \
    --account-id "$META_ACCOUNT_ID" \
    --date-preset "$META_DATE_PRESET" \
    --level account \
    --metric-pack local_intent \
    --publisher-platform instagram \
    --format json
)"
printf '%s' "$paid_payload" >"$work_dir/paid-instagram.json"
assert_success_envelope "$paid_payload" "meta insights run"
printf '%s' "$paid_payload" | jq -e '
  .data | type == "array" and length > 0
' >/dev/null || fail "paid instagram local-intent returned zero rows; choose an account/date preset with instagram placement delivery"
printf '%s' "$paid_payload" | jq -e '
  all(.data[];
    .publisher_platform == "instagram" and
    (if has("actions") then (.actions | type == "array") else true end) and
    (if has("cost_per_action_type") then (.cost_per_action_type | type == "array") else true end) and
    (if has("calls") then (.calls | type == "number") else true end) and
    (if has("directions") then (.directions | type == "number") else true end) and
    (if has("address_taps") then (.address_taps | type == "number") else true end) and
    (if has("profile_visits") then (.profile_visits | type == "number") else true end)
  )
' >/dev/null

account_local_intent_payload="$(
  if [[ ${#ig_user_args[@]} -gt 0 ]]; then
    run_meta ig insights account local-intent \
      "${ig_user_args[@]}" \
      --date-preset "$META_DATE_PRESET" \
      --format json
  else
    run_meta ig insights account local-intent \
      --date-preset "$META_DATE_PRESET" \
      --format json
  fi
)"
printf '%s' "$account_local_intent_payload" >"$work_dir/account-local-intent.json"
assert_success_envelope "$account_local_intent_payload" "meta ig insights account local-intent"
printf '%s' "$account_local_intent_payload" | jq -e '
  .data | type == "object" and
  (.summary | type == "object") and
  (.raw_metrics | type == "array" and length > 0) and
  (if .summary.calls then (.summary.calls | type == "number") else true end) and
  (if .summary.directions then (.summary.directions | type == "number") else true end) and
  (if .summary.email_contacts then (.summary.email_contacts | type == "number") else true end) and
  (if .summary.text_contacts then (.summary.text_contacts | type == "number") else true end) and
  (if .summary.book_now then (.summary.book_now | type == "number") else true end) and
  (if .summary.profile_views then (.summary.profile_views | type == "number") else true end)
' >/dev/null

media_list_payload="$(
  if [[ ${#ig_user_args[@]} -gt 0 ]]; then
    run_meta ig insights media list \
      "${ig_user_args[@]}" \
      --limit "$META_MEDIA_LIMIT" \
      --format json
  else
    run_meta ig insights media list \
      --limit "$META_MEDIA_LIMIT" \
      --format json
  fi
)"
printf '%s' "$media_list_payload" >"$work_dir/media-list.json"
assert_success_envelope "$media_list_payload" "meta ig insights media list"
printf '%s' "$media_list_payload" | jq -e '
  .data | type == "array" and length > 0 and
  all(.[]; (.id | type == "string" and length > 0))
' >/dev/null || fail "media list returned zero rows; confirm the profile is bound to an active Instagram account with media"

if [[ -z "$META_IG_MEDIA_ID" ]]; then
  META_IG_MEDIA_ID="$(
    jq -r '
      [.data[] | select(.media_product_type == "FEED") | .id][0] //
      .data[0].id
    ' "$work_dir/media-list.json"
  )"
fi
[[ -n "$META_IG_MEDIA_ID" && "$META_IG_MEDIA_ID" != "null" ]] || fail "unable to resolve a media id from media list output"

media_run_payload="$(
  run_meta ig insights media run \
    --media-id "$META_IG_MEDIA_ID" \
    --metric "$META_MEDIA_METRIC" \
    --period "$META_MEDIA_PERIOD" \
    --format json
)"
printf '%s' "$media_run_payload" >"$work_dir/media-run.json"
assert_success_envelope "$media_run_payload" "meta ig insights media run"
printf '%s' "$media_run_payload" | jq -e --arg media_id "$META_IG_MEDIA_ID" '
  .data | type == "array" and length == 1 and
  .[0].media_id == $media_id and
  (.[0].raw_metrics | type == "array")
' >/dev/null

combined_payload="$(
  if [[ ${#ig_user_args[@]} -gt 0 ]]; then
    run_meta ig insights local-intent \
      --account-id "$META_ACCOUNT_ID" \
      "${ig_user_args[@]}" \
      --date-preset "$META_DATE_PRESET" \
      --format json
  else
    run_meta ig insights local-intent \
      --account-id "$META_ACCOUNT_ID" \
      --date-preset "$META_DATE_PRESET" \
      --format json
  fi
)"
printf '%s' "$combined_payload" >"$work_dir/combined-local-intent.json"
assert_success_envelope "$combined_payload" "meta ig insights local-intent"
printf '%s' "$combined_payload" | jq -e '
  .data | type == "object" and
  (.range.since | type == "string" and length > 0) and
  (.range.until | type == "string" and length > 0) and
  (.summary | type == "object") and
  (.paid_instagram.summary | type == "object") and
  (.paid_instagram.rows | type == "array") and
  all(.paid_instagram.rows[]?; .publisher_platform == "instagram") and
  (.instagram_account.summary | type == "object") and
  (.instagram_account.raw_metrics | type == "array" and length > 0)
' >/dev/null

paid_calls="$(jq -r '.data.paid_instagram.summary.calls // empty' "$work_dir/combined-local-intent.json")"
account_calls="$(jq -r '.data.instagram_account.summary.calls // empty' "$work_dir/combined-local-intent.json")"
combined_calls="$(jq -r '.data.summary.calls.combined // empty' "$work_dir/combined-local-intent.json")"
media_count="$(jq -r '.data | length' "$work_dir/media-list.json")"

echo "IG insights smoke completed."
echo "profile=$META_PROFILE"
echo "account_id=$META_ACCOUNT_ID"
echo "ig_user_id=${META_IG_USER_ID:-profile-default}"
echo "date_preset=$META_DATE_PRESET"
echo "media_id=$META_IG_MEDIA_ID"
echo "media_count=$media_count"
if [[ -n "$paid_calls" ]]; then
  echo "paid_instagram_calls=$paid_calls"
fi
if [[ -n "$account_calls" ]]; then
  echo "instagram_account_calls=$account_calls"
fi
if [[ -n "$combined_calls" ]]; then
  echo "combined_calls=$combined_calls"
fi
