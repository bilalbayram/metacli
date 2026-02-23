# Track B IG Publish Smoke Demo

Track B validates the IG publishing flow end-to-end with CLI commands and strict contract assertions across:

- `ig caption validate`
- `ig media upload`
- `ig media status`
- `ig publish feed` (immediate)
- `ig publish schedule list|cancel|retry`
- idempotency duplicate suppression + idempotency conflict
- retry transition handling for scheduled publishes

## Prerequisites

- built CLI binary:

```bash
go build -o meta ./cmd/meta
```

- authenticated profile with IG publishing permissions
- `jq` and `python3` installed
- a public image URL reachable by Instagram Graph API

## Smoke Script

Script path:

- `tests/smoke/track_b_ig_cli_publish_smoke.sh`

Required environment variables:

- `META_IG_USER_ID`
- `META_MEDIA_URL`

Optional overrides:

- `META_BIN` (default `./meta`)
- `META_PROFILE` (default `prod`)
- `META_UPLOAD_MEDIA_URL`
- `META_PUBLISH_MEDIA_URL`
- `META_SCHEDULE_MEDIA_URL`
- `META_CAPTION`
- `META_PUBLISH_CAPTION`
- `META_SCHEDULE_CAPTION`
- `META_PUBLISH_IDEMPOTENCY_KEY`
- `META_SCHEDULE_IDEMPOTENCY_KEY`
- `META_SCHEDULE_PUBLISH_AT` (RFC3339, defaults to now+2h)
- `META_SCHEDULE_RETRY_AT` (RFC3339, defaults to now+4h)

Run:

```bash
META_BIN=./meta \
META_PROFILE=prod \
META_IG_USER_ID=<IG_USER_ID> \
META_MEDIA_URL=https://cdn.example.com/image.jpg \
tests/smoke/track_b_ig_cli_publish_smoke.sh
```

## Assertions Enforced

The script fails fast and enforces:

- success envelopes include `contract_version=1.0` and expected `command`
- upload returns a non-empty `creation_id`
- status returns a non-empty `status_code`
- immediate publish returns `mode=immediate`, `surface=feed`, and non-empty `creation_id`/`media_id`
- scheduled publish returns `mode=scheduled`
- second schedule call with same idempotency key and identical payload returns `duplicate_suppressed=true`
- schedule payload drift with same idempotency key fails with structured `ig_idempotency_conflict` (`retryable=false`)
- schedule cancel and retry transitions produce expected status and retry counters

## Local Smoke Coverage

In addition to the shell demo, automated Track B CLI smoke coverage is implemented in:

- `internal/cli/cmd/track_b_smoke_test.go`
