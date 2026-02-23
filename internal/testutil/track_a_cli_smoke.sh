#!/usr/bin/env bash
set -euo pipefail

require_bin() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required binary: $1" >&2
    exit 1
  fi
}

require_bin jq

META_BIN="${META_BIN:-./meta}"
META_PROFILE="${META_PROFILE:-prod}"
META_SCHEMA_DIR="${META_SCHEMA_DIR:-./schema-packs}"
: "${META_ACCOUNT_ID:?set META_ACCOUNT_ID to an ad account id (with or without act_ prefix)}"
: "${META_CATALOG_ID:?set META_CATALOG_ID to a catalog id}"

if [[ ! -x "$META_BIN" ]]; then
  echo "meta binary is not executable: $META_BIN" >&2
  echo "build it first: go build -o meta ./cmd/meta" >&2
  exit 1
fi

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

upload_payload="$work_dir/catalog-upload-items.json"
batch_payload="$work_dir/catalog-batch-items.json"
batch_error="$work_dir/catalog-batch-error.json"

cat >"$upload_payload" <<'JSON'
[
  {
    "retailer_id": "sku_track_a_1",
    "data": {
      "name": "Track A Shirt",
      "price": "10.00 USD"
    }
  }
]
JSON

cat >"$batch_payload" <<'JSON'
[
  {
    "method": "UPDATE",
    "retailer_id": "sku_track_a_1",
    "data": {
      "price": "bad"
    }
  }
]
JSON

run_meta() {
  "$META_BIN" --profile "$META_PROFILE" --output json "$@"
}

assert_success_envelope() {
  local payload="$1"
  local command_name="$2"
  printf '%s' "$payload" | jq -e --arg cmd "$command_name" '
    .command == $cmd and .success == true
  ' >/dev/null
}

extract_data_string() {
  local payload="$1"
  local key="$2"
  printf '%s' "$payload" | jq -er --arg key "$key" '.data[$key]'
}

campaign_payload="$(
  run_meta campaign create \
    --account-id "$META_ACCOUNT_ID" \
    --params "name=TrackA Campaign,objective=OUTCOME_SALES,status=PAUSED" \
    --schema-dir "$META_SCHEMA_DIR"
)"
assert_success_envelope "$campaign_payload" "meta campaign create"
campaign_id="$(extract_data_string "$campaign_payload" "campaign_id")"

adset_payload="$(
  run_meta adset create \
    --account-id "$META_ACCOUNT_ID" \
    --params "name=TrackA AdSet,campaign_id=$campaign_id,status=PAUSED,billing_event=IMPRESSIONS,optimization_goal=OFFSITE_CONVERSIONS" \
    --schema-dir "$META_SCHEMA_DIR"
)"
assert_success_envelope "$adset_payload" "meta adset create"
adset_id="$(extract_data_string "$adset_payload" "adset_id")"

creative_payload="$(
  run_meta creative create \
    --account-id "$META_ACCOUNT_ID" \
    --params "name=TrackA Creative,object_story_id=123_456" \
    --schema-dir "$META_SCHEMA_DIR"
)"
assert_success_envelope "$creative_payload" "meta creative create"
creative_id="$(extract_data_string "$creative_payload" "creative_id")"

creative_ref_json="$(jq -cn --arg creative_id "$creative_id" '{creative:{creative_id:$creative_id}}')"
ad_payload="$(
  run_meta ad create \
    --account-id "$META_ACCOUNT_ID" \
    --params "name=TrackA Ad,adset_id=$adset_id,status=PAUSED" \
    --json "$creative_ref_json" \
    --schema-dir "$META_SCHEMA_DIR"
)"
assert_success_envelope "$ad_payload" "meta ad create"
ad_id="$(extract_data_string "$ad_payload" "ad_id")"

audience_payload="$(
  run_meta audience create \
    --account-id "$META_ACCOUNT_ID" \
    --params "name=TrackA Audience,subtype=CUSTOM,description=TrackA smoke audience" \
    --schema-dir "$META_SCHEMA_DIR"
)"
assert_success_envelope "$audience_payload" "meta audience create"
audience_id="$(extract_data_string "$audience_payload" "audience_id")"

catalog_upload_payload="$(
  run_meta catalog upload-items \
    --catalog-id "$META_CATALOG_ID" \
    --file "$upload_payload"
)"
assert_success_envelope "$catalog_upload_payload" "meta catalog upload-items"
printf '%s' "$catalog_upload_payload" | jq -e '
  .data.success_count == 1 and .data.error_count == 0
' >/dev/null

batch_stdout=""
set +e
batch_stdout="$(
  run_meta catalog batch-items \
    --catalog-id "$META_CATALOG_ID" \
    --file "$batch_payload" \
    2>"$batch_error"
)"
batch_exit=$?
set -e

if [[ $batch_exit -eq 0 ]]; then
  echo "expected catalog batch-items to fail but command succeeded" >&2
  exit 1
fi
if [[ -n "$batch_stdout" ]]; then
  echo "expected empty stdout from failing catalog batch-items command" >&2
  exit 1
fi

jq -e '
  .command == "meta catalog batch-items" and
  .success == false and
  .error.type == "catalog_item_errors" and
  .data.error_count == 1
' "$batch_error" >/dev/null

echo "Track A CLI smoke workflow completed."
echo "campaign_id=$campaign_id"
echo "adset_id=$adset_id"
echo "creative_id=$creative_id"
echo "ad_id=$ad_id"
echo "audience_id=$audience_id"
