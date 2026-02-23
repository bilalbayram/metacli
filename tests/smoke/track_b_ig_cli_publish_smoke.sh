#!/usr/bin/env bash
set -euo pipefail

require_bin() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required binary: $1" >&2
    exit 1
  fi
}

require_bin jq
require_bin python3

META_BIN="${META_BIN:-./meta}"
META_PROFILE="${META_PROFILE:-prod}"
: "${META_IG_USER_ID:?set META_IG_USER_ID to an instagram user id}"
: "${META_MEDIA_URL:?set META_MEDIA_URL to a public IMAGE URL}"

META_UPLOAD_MEDIA_URL="${META_UPLOAD_MEDIA_URL:-$META_MEDIA_URL}"
META_PUBLISH_MEDIA_URL="${META_PUBLISH_MEDIA_URL:-$META_MEDIA_URL}"
META_SCHEDULE_MEDIA_URL="${META_SCHEDULE_MEDIA_URL:-$META_MEDIA_URL}"
META_CAPTION="${META_CAPTION:-Track B smoke #meta}"
META_PUBLISH_CAPTION="${META_PUBLISH_CAPTION:-Track B publish #meta}"
META_SCHEDULE_CAPTION="${META_SCHEDULE_CAPTION:-Track B schedule #meta}"
META_PUBLISH_IDEMPOTENCY_KEY="${META_PUBLISH_IDEMPOTENCY_KEY:-track_b_publish_01}"
META_SCHEDULE_IDEMPOTENCY_KEY="${META_SCHEDULE_IDEMPOTENCY_KEY:-track_b_schedule_01}"

if [[ ! -x "$META_BIN" ]]; then
  echo "meta binary is not executable: $META_BIN" >&2
  echo "build it first: go build -o meta ./cmd/meta" >&2
  exit 1
fi

utc_plus_hours() {
  python3 - "$1" <<'PY'
import sys
from datetime import datetime, timedelta, timezone

hours = int(sys.argv[1])
value = datetime.now(timezone.utc).replace(microsecond=0) + timedelta(hours=hours)
print(value.isoformat().replace("+00:00", "Z"))
PY
}

META_SCHEDULE_PUBLISH_AT="${META_SCHEDULE_PUBLISH_AT:-$(utc_plus_hours 2)}"
META_SCHEDULE_RETRY_AT="${META_SCHEDULE_RETRY_AT:-$(utc_plus_hours 4)}"

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

schedule_state_path="$work_dir/ig-schedules.json"
conflict_error_path="$work_dir/ig-schedule-conflict-error.json"

run_meta() {
  "$META_BIN" --profile "$META_PROFILE" --output json "$@"
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

extract_data_string() {
  local payload="$1"
  local key="$2"
  printf '%s' "$payload" | jq -er --arg key "$key" '.data[$key]'
}

caption_payload="$(
  run_meta ig caption validate \
    --caption "$META_CAPTION"
)"
assert_success_envelope "$caption_payload" "meta ig caption validate"
printf '%s' "$caption_payload" | jq -e '.data.valid == true' >/dev/null

upload_payload="$(
  run_meta ig media upload \
    --ig-user-id "$META_IG_USER_ID" \
    --media-url "$META_UPLOAD_MEDIA_URL" \
    --caption "$META_CAPTION" \
    --media-type IMAGE
)"
assert_success_envelope "$upload_payload" "meta ig media upload"
creation_id="$(extract_data_string "$upload_payload" "creation_id")"

status_payload="$(
  run_meta ig media status \
    --creation-id "$creation_id"
)"
assert_success_envelope "$status_payload" "meta ig media status"
printf '%s' "$status_payload" | jq -e --arg creation_id "$creation_id" '
  .data.creation_id == $creation_id and
  (.data.status_code | type == "string") and
  (.data.status_code | length > 0)
' >/dev/null

publish_payload="$(
  run_meta ig publish feed \
    --ig-user-id "$META_IG_USER_ID" \
    --media-url "$META_PUBLISH_MEDIA_URL" \
    --caption "$META_PUBLISH_CAPTION" \
    --media-type IMAGE \
    --idempotency-key "$META_PUBLISH_IDEMPOTENCY_KEY"
)"
assert_success_envelope "$publish_payload" "meta ig publish feed"
printf '%s' "$publish_payload" | jq -e --arg key "$META_PUBLISH_IDEMPOTENCY_KEY" '
  .data.mode == "immediate" and
  .data.surface == "feed" and
  .data.media_type == "IMAGE" and
  .data.idempotency_key == $key and
  (.data.creation_id | type == "string") and
  (.data.creation_id | length > 0) and
  (.data.media_id | type == "string") and
  (.data.media_id | length > 0)
' >/dev/null

schedule_payload="$(
  run_meta ig publish feed \
    --ig-user-id "$META_IG_USER_ID" \
    --media-url "$META_SCHEDULE_MEDIA_URL" \
    --caption "$META_SCHEDULE_CAPTION" \
    --media-type IMAGE \
    --idempotency-key "$META_SCHEDULE_IDEMPOTENCY_KEY" \
    --publish-at "$META_SCHEDULE_PUBLISH_AT" \
    --schedule-state-path "$schedule_state_path"
)"
assert_success_envelope "$schedule_payload" "meta ig publish feed"
schedule_id="$(printf '%s' "$schedule_payload" | jq -er '.data.schedule.schedule_id')"
printf '%s' "$schedule_payload" | jq -e '
  .data.mode == "scheduled" and
  .data.duplicate_suppressed == false and
  .data.schedule.status == "scheduled"
' >/dev/null

duplicate_payload="$(
  run_meta ig publish feed \
    --ig-user-id "$META_IG_USER_ID" \
    --media-url "$META_SCHEDULE_MEDIA_URL" \
    --caption "$META_SCHEDULE_CAPTION" \
    --media-type IMAGE \
    --idempotency-key "$META_SCHEDULE_IDEMPOTENCY_KEY" \
    --publish-at "$META_SCHEDULE_PUBLISH_AT" \
    --schedule-state-path "$schedule_state_path"
)"
assert_success_envelope "$duplicate_payload" "meta ig publish feed"
printf '%s' "$duplicate_payload" | jq -e --arg schedule_id "$schedule_id" '
  .data.mode == "scheduled" and
  .data.duplicate_suppressed == true and
  .data.schedule.schedule_id == $schedule_id
' >/dev/null

conflict_stdout=""
set +e
conflict_stdout="$(
  run_meta ig publish feed \
    --ig-user-id "$META_IG_USER_ID" \
    --media-url "${META_SCHEDULE_MEDIA_URL}?variant=conflict" \
    --caption "$META_SCHEDULE_CAPTION" \
    --media-type IMAGE \
    --idempotency-key "$META_SCHEDULE_IDEMPOTENCY_KEY" \
    --publish-at "$META_SCHEDULE_PUBLISH_AT" \
    --schedule-state-path "$schedule_state_path" \
    2>"$conflict_error_path"
)"
conflict_exit=$?
set -e

if [[ $conflict_exit -eq 0 ]]; then
  echo "expected schedule idempotency conflict, but command succeeded" >&2
  exit 1
fi
if [[ -n "$conflict_stdout" ]]; then
  echo "expected empty stdout from idempotency conflict command" >&2
  exit 1
fi

jq -e '
  .command == "meta ig publish feed" and
  .contract_version == "1.0" and
  .success == false and
  .error.type == "ig_idempotency_conflict" and
  .error.retryable == false
' "$conflict_error_path" >/dev/null

schedule_list_payload="$(
  run_meta ig publish schedule list \
    --schedule-state-path "$schedule_state_path"
)"
assert_success_envelope "$schedule_list_payload" "meta ig publish schedule list"
printf '%s' "$schedule_list_payload" | jq -e --arg schedule_id "$schedule_id" '
  .data.total == 1 and
  .data.schedules[0].schedule_id == $schedule_id and
  .data.schedules[0].status == "scheduled"
' >/dev/null

cancel_payload="$(
  run_meta ig publish schedule cancel \
    --schedule-id "$schedule_id" \
    --schedule-state-path "$schedule_state_path"
)"
assert_success_envelope "$cancel_payload" "meta ig publish schedule cancel"
printf '%s' "$cancel_payload" | jq -e '
  .data.operation == "cancel" and
  .data.schedule.status == "canceled"
' >/dev/null

retry_payload="$(
  run_meta ig publish schedule retry \
    --schedule-id "$schedule_id" \
    --publish-at "$META_SCHEDULE_RETRY_AT" \
    --schedule-state-path "$schedule_state_path"
)"
assert_success_envelope "$retry_payload" "meta ig publish schedule retry"
printf '%s' "$retry_payload" | jq -e --arg schedule_id "$schedule_id" '
  .data.operation == "retry" and
  .data.schedule.schedule_id == $schedule_id and
  .data.schedule.status == "scheduled" and
  .data.schedule.retry_count == 1
' >/dev/null

scheduled_only_payload="$(
  run_meta ig publish schedule list \
    --status scheduled \
    --schedule-state-path "$schedule_state_path"
)"
assert_success_envelope "$scheduled_only_payload" "meta ig publish schedule list"
printf '%s' "$scheduled_only_payload" | jq -e --arg schedule_id "$schedule_id" '
  .data.total == 1 and
  .data.schedules[0].schedule_id == $schedule_id and
  .data.schedules[0].status == "scheduled"
' >/dev/null

echo "Track B IG publish smoke workflow completed."
echo "creation_id=$creation_id"
echo "schedule_id=$schedule_id"
