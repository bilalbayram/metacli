# Meta Marketing CLI

A fail-fast, read-focused CLI for Meta Graph and Marketing API operations.

## v1 Scope

This release is read-first and includes:

- Auth profile management (`meta auth ...`)
- Universal GET access (`meta api get`)
- GET-only batch execution (`meta api batch`)
- Insights reporting (`meta insights run`)
- Schema pack management (`meta schema list`, `meta schema sync`)
- Request linting (`meta lint request`)
- Version checks (`meta changelog check`)

Out of scope for v1:

- `meta api post`
- `meta api delete`
- campaign/adset/ad/audience/catalog/creative mutation workflows

## Installation

```bash
go build -o meta ./cmd/meta
```

## Configuration and Secrets

- Config path: `~/.meta/config.yaml`
- Config schema version: `1`
- Default Graph version: `v25.0`
- Secrets: OS keychain only (macOS Keychain / Linux Secret Service)
- No env/plaintext secret fallback

## Quick Start

1. Add a system user profile:

```bash
./meta auth add system-user \
  --profile prod \
  --business-id <BUSINESS_ID> \
  --app-id <APP_ID> \
  --token <SYSTEM_USER_TOKEN> \
  --app-secret <APP_SECRET>
```

2. Validate profile token:

```bash
./meta auth validate --profile prod
```

3. Run a Graph GET request:

```bash
./meta api get act_<AD_ACCOUNT_ID>/campaigns \
  --profile prod \
  --fields id,name,status \
  --follow-next \
  --limit 100
```

4. Run insights export:

```bash
./meta insights run \
  --profile prod \
  --account-id <AD_ACCOUNT_ID> \
  --level campaign \
  --date-preset last_7d \
  --format jsonl
```

## Track A Workflow (Smoke Demo)

CLI-only Track A workflow coverage (campaign -> adset -> creative -> ad -> audience -> catalog) is scripted in:

- `internal/testutil/track_a_cli_smoke.sh`

The script is fail-fast and validates both success envelopes and an expected structured error envelope (`meta catalog batch-items`).

Prerequisites:

- built CLI binary: `go build -o meta ./cmd/meta`
- authenticated profile with write permissions
- `jq` installed

Run:

```bash
META_BIN=./meta \
META_PROFILE=prod \
META_ACCOUNT_ID=<AD_ACCOUNT_ID> \
META_CATALOG_ID=<CATALOG_ID> \
META_SCHEMA_DIR=./schema-packs \
./internal/testutil/track_a_cli_smoke.sh
```

## Command Surface

### Auth

- `meta auth add system-user`
- `meta auth login`
- `meta auth page-token`
- `meta auth app-token set`
- `meta auth validate`
- `meta auth rotate`
- `meta auth debug-token`
- `meta auth list`

### API

- `meta api get <path>`
- `meta api batch --file <path> | --stdin`

### Insights

- `meta insights run`

### Schema

- `meta schema list`
- `meta schema sync --channel stable`

### Lint

- `meta lint request --file <request.json> [--strict]`

### Changelog

- `meta changelog check --version <vXX.X>`

## Output Contract

When using `--output json` or `--output jsonl`, responses are wrapped in contract `1.0`:

- `contract_version`
- `command`
- `timestamp`
- `request_id`
- `success`
- `data`
- `paging`
- `rate_limit`
- `error`

Error payload fields:

- `type`
- `code`
- `error_subcode`
- `message`
- `fbtrace_id`
- `retryable`

## Fail-Fast Rules

The CLI intentionally exits non-zero instead of silently degrading when:

- config file/schema is invalid
- keychain secrets are missing or inaccessible
- schema packs are missing for requested version/domain
- batch payload includes unsupported methods in v1
- token validation fails

## Schema Packs

Built-in pack path:

- `schema-packs/marketing/v25.0.json`

Remote sync verifies:

- manifest signature (Ed25519)
- pack checksum (SHA-256)

## CI

GitHub Actions workflow:

- macOS + Linux test/build matrix
- Linux race detector
- build artifact upload
- tag-based release smoke cross-build

## Plan

Original implementation plan:

- `PLAN.md`
