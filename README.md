# Meta Marketing CLI

Fail-closed CLI for Meta Graph and Marketing APIs with auth profiles, write workflows, IG publishing, ops reporting, and enterprise governance.

This README reflects the latest 43 commits on this branch (through `0122ecf`, dated February 23, 2026).

## Highlights (Latest 43 Commits)

- Added full Marketing lifecycle command groups at root: `campaign`, `adset`, `ad`, `creative`, `audience`, `catalog`
- Added strict mutation linting against schema packs for write commands
- Added budget-change guardrails for campaign/adset (`--confirm-budget-change`)
- Added campaign/ad clone workflows with immutable-field sanitization
- Added IG media upload/status, caption validation, feed/reel/story publish, scheduling lifecycle (`list/cancel/retry`), and idempotency conflict handling
- Added Track A/Track B smoke coverage and docs
- Added `ops` daily report pipeline with strict envelope contract, drift/rate-limit/preflight/runtime checks, and deterministic exit codes
- Added strict enterprise mode with cutover path, workspace-scoped RBAC/policy checks, approval tokens, immutable audit events, secret governance, and fail-closed execute pipeline
- Added plugin namespace bootstrap commands: `wa`, `msgr`, `threads`, `capi`
- Added API `post` and `delete` (in addition to `get` and `batch`)

## Features

- Auth profiles with OS keychain-backed secrets
- Universal Graph API command surface (`get`, `post`, `delete`, `batch`)
- Insights export (`jsonl` or `csv`)
- Marketing mutation workflows: `campaign`, `adset`, `creative`, `ad`, `audience`, `catalog`
- Campaign/adset status and write guardrails with schema lint and budget confirmation checks
- Ad dependency validation against referenced ad sets and creatives
- IG publishing workflows: caption validation, media upload/status, immediate publish (feed/reel/story), scheduled publish lifecycle with idempotency controls
- Ops baseline + report pipeline with strict output contract and policy/warning exit mapping
- Enterprise governance pipeline with RBAC/policy/approval/secret-access/audit enforcement
- Schema pack listing/sync with signed manifest + checksum verification

## Installation

### Prerequisites

- Go `1.23+`
- macOS Keychain or Linux Secret Service (for secure token storage)
- `jq` for smoke scripts
- `python3` for Track B IG smoke script

### Build

```bash
go build -o meta ./cmd/meta
./meta --help
```

## Configuration and Secrets

- App config: `~/.meta/config.yaml`
- Config schema: `schema_version: 1`
- Default Graph API version: `v25.0`
- Default schema domain: `marketing`
- Secrets service name: `meta-marketing-cli`
- Secret refs use `keychain://...` and are stored in OS keychain only
- No plaintext/env-secret fallback is implemented

## Quick Start

### 1) Create and validate an auth profile

```bash
./meta auth add system-user \
  --profile prod \
  --business-id <BUSINESS_ID> \
  --app-id <APP_ID> \
  --token <SYSTEM_USER_TOKEN> \
  --app-secret <APP_SECRET>

./meta auth validate --profile prod
./meta auth list --profile prod
```

### 2) Use the Graph API directly

```bash
./meta --profile prod api get act_<AD_ACCOUNT_ID>/campaigns \
  --fields id,name,status \
  --follow-next \
  --limit 100

./meta --profile prod api post act_<AD_ACCOUNT_ID>/campaigns \
  --params "name=Example Campaign,objective=OUTCOME_SALES,status=PAUSED"
```

### 3) Run Insights export

```bash
./meta --profile prod insights run \
  --account-id <AD_ACCOUNT_ID> \
  --level campaign \
  --date-preset last_7d \
  --format jsonl
```

### 4) Run request lint and schema checks

```bash
./meta --profile prod lint request --file ./request.json --strict
./meta schema list
./meta schema sync --channel stable
./meta changelog check --version v25.0
```

## How To Use The CLI

### Common Global Flags

- `--profile <name>`: select auth profile
- `--output json|jsonl|table|csv`: output format (default `json`)
- `--debug`: enable debug logging

### Marketing Lifecycle (Track A)

```bash
./meta --profile prod campaign create \
  --account-id <AD_ACCOUNT_ID> \
  --params "name=TrackA Campaign,objective=OUTCOME_SALES,status=PAUSED" \
  --schema-dir ./schema-packs

./meta --profile prod adset create \
  --account-id <AD_ACCOUNT_ID> \
  --params "name=TrackA AdSet,campaign_id=<CAMPAIGN_ID>,status=PAUSED,billing_event=IMPRESSIONS,optimization_goal=OFFSITE_CONVERSIONS" \
  --schema-dir ./schema-packs

./meta --profile prod creative create \
  --account-id <AD_ACCOUNT_ID> \
  --params "name=TrackA Creative,object_story_id=123_456" \
  --schema-dir ./schema-packs

./meta --profile prod ad create \
  --account-id <AD_ACCOUNT_ID> \
  --params "name=TrackA Ad,adset_id=<ADSET_ID>,status=PAUSED" \
  --json '{"creative":{"creative_id":"<CREATIVE_ID>"}}' \
  --schema-dir ./schema-packs

./meta --profile prod audience create \
  --account-id <AD_ACCOUNT_ID> \
  --params "name=TrackA Audience,subtype=CUSTOM,description=TrackA smoke audience" \
  --schema-dir ./schema-packs
```

Budget guardrail example (required when mutating budget fields):

```bash
./meta --profile prod campaign update \
  --campaign-id <CAMPAIGN_ID> \
  --params "daily_budget=5000" \
  --confirm-budget-change
```

### Catalog Upload and Batch

```bash
./meta --profile prod catalog upload-items \
  --catalog-id <CATALOG_ID> \
  --file ./catalog-upload-items.json

./meta --profile prod catalog batch-items \
  --catalog-id <CATALOG_ID> \
  --file ./catalog-batch-items.json
```

### Instagram Publish and Scheduling (Track B)

```bash
./meta --profile prod ig caption validate \
  --caption "Launch post #meta #ads" \
  --strict

./meta --profile prod ig media upload \
  --ig-user-id <IG_USER_ID> \
  --media-url https://cdn.example.com/image.jpg \
  --caption "Launch post #meta"

./meta --profile prod ig publish feed \
  --ig-user-id <IG_USER_ID> \
  --media-url https://cdn.example.com/image.jpg \
  --caption "Launch post #meta" \
  --idempotency-key publish-feed-001

./meta --profile prod ig publish reel \
  --ig-user-id <IG_USER_ID> \
  --media-url https://cdn.example.com/reel.mp4 \
  --caption "Product reel #meta" \
  --publish-at 2026-02-24T16:00:00Z \
  --idempotency-key schedule-reel-001

./meta --profile prod ig publish schedule list
./meta --profile prod ig publish schedule cancel --schedule-id <SCHEDULE_ID>
./meta --profile prod ig publish schedule retry --schedule-id <SCHEDULE_ID> --publish-at 2026-02-24T18:00:00Z
```

### Ops Baseline and Daily Report (Track C)

```bash
./meta --output json ops init --state-path "$HOME/.meta/ops/baseline-state.json"

./meta --output jsonl ops run --state-path "$HOME/.meta/ops/baseline-state.json"

./meta --output json ops run \
  --state-path "$HOME/.meta/ops/baseline-state.json" \
  --rate-telemetry-file ./rate-telemetry.json \
  --runtime-response-file ./runtime-response.json \
  --lint-request-file ./lint-request.json \
  --preflight-config-path "$HOME/.meta/config.yaml"
```

### Enterprise Governance (Track D)

```bash
./meta enterprise mode cutover \
  --legacy-config ~/.meta/config.yaml \
  --config ~/.meta/enterprise.yaml \
  --org agency \
  --org-id org_1 \
  --workspace prod \
  --workspace-id ws_1 \
  --principal ops.admin

./meta enterprise context \
  --config ~/.meta/enterprise.yaml \
  --workspace agency/prod

./meta enterprise approval request \
  --config ~/.meta/enterprise.yaml \
  --principal ops.admin \
  --command "auth rotate" \
  --workspace agency/prod \
  --ttl 30m

./meta enterprise approval approve \
  --request-token <REQUEST_TOKEN> \
  --approver security.lead \
  --decision approved \
  --ttl 30m

./meta enterprise execute \
  --config ~/.meta/enterprise.yaml \
  --principal ops.admin \
  --command "auth rotate" \
  --workspace agency/prod \
  --approval-token <GRANT_TOKEN> \
  --correlation-id corr-20260223-001 \
  --require-secret auth_rotation_key:rotate
```

### Namespace Bootstrap Commands

```bash
./meta wa health
./meta wa capability --name send-message

./meta msgr health
./meta threads capability --name publish-post
./meta capi capability --name send-event
```

## Complete Command Reference

```text
meta
├── ad
│   ├── create
│   ├── update
│   ├── pause
│   ├── resume
│   └── clone
├── adset
│   ├── create
│   ├── update
│   ├── pause
│   └── resume
├── api
│   ├── get <path>
│   ├── post <path>
│   ├── delete <path>
│   └── batch
├── audience
│   ├── create
│   ├── update
│   └── delete
├── auth
│   ├── add system-user
│   ├── login
│   ├── page-token
│   ├── app-token set
│   ├── validate
│   ├── rotate
│   ├── debug-token
│   └── list
├── campaign
│   ├── create
│   ├── update
│   ├── pause
│   ├── resume
│   └── clone
├── capi
│   ├── health
│   └── capability
├── catalog
│   ├── upload-items
│   └── batch-items
├── changelog
│   └── check
├── completion
├── creative
│   ├── upload
│   └── create
├── enterprise
│   ├── context
│   ├── authz check
│   ├── execute
│   ├── mode cutover
│   ├── approval request
│   ├── approval approve
│   ├── approval validate
│   └── policy eval
├── ig
│   ├── health
│   ├── media upload
│   ├── media status
│   ├── caption validate
│   ├── publish feed
│   ├── publish reel
│   ├── publish story
│   └── publish schedule
│       ├── list
│       ├── cancel
│       └── retry
├── insights
│   └── run
├── lint
│   └── request
├── msgr
│   ├── health
│   └── capability
├── ops
│   ├── init
│   └── run
├── schema
│   ├── list
│   └── sync
├── threads
│   ├── health
│   └── capability
└── wa
    ├── health
    └── capability
```

Use `./meta <group> --help` and `./meta <group> <command> --help` for full flag details.

## Output Contracts

### Standard CLI Envelope (`contract_version: "1.0"`)

Most commands return:

- `contract_version`
- `command`
- `timestamp`
- `request_id`
- `success`
- `data`
- `paging` (when paginated)
- `rate_limit` (when available)
- `error` (`type`, `code`, `error_subcode`, `message`, `fbtrace_id`, `retryable`)

### Ops Envelope (`contract_version: "ops.v1"`)

`meta ops init` and `meta ops run` return:

- `contract_version`
- `command`
- `success`
- `exit_code`
- `data`
- `error`

`meta ops run` exit code mapping:

- `0`: clean
- `8`: blocking findings
- `16`: warning findings

### Output Format Notes

- Global `--output` supports: `json`, `jsonl`, `table`, `csv`
- `insights run` uses `--format csv|jsonl`
- `ops init` requires `--output json`
- `ops run` allows `--output json|jsonl|csv`

### Standard CLI Exit Codes

For non-ops commands:

- `1`: unknown/unclassified failure
- `2`: config failure
- `3`: auth failure
- `4`: input validation failure
- `5`: Graph/API failure

## Fail-Closed Behavior

The CLI exits non-zero instead of degrading when:

- profile is missing or invalid
- config/enterprise config schema is invalid
- keychain secrets are missing or inaccessible
- schema packs are missing or fail verification
- mutation lint fails against schema packs
- budget mutation occurs without explicit confirmation
- enterprise mode is legacy/misaligned (cutover required)
- enterprise authorization/approval/secret governance/audit checks fail
- IG schedule state transitions are invalid or idempotency conflicts are detected

No compatibility fallback path is implemented for strict enterprise execution or missing security prerequisites.

## Smoke Workflows and Docs

- Track A CLI smoke workflow script: [`internal/testutil/track_a_cli_smoke.sh`](internal/testutil/track_a_cli_smoke.sh)
- Track B IG publish smoke script: [`tests/smoke/track_b_ig_cli_publish_smoke.sh`](tests/smoke/track_b_ig_cli_publish_smoke.sh)
- Track B guide: [`docs/ig/track-b-publish-smoke.md`](docs/ig/track-b-publish-smoke.md)
- Track C ops daily report guide: [`docs/ops/track-c-daily-report.md`](docs/ops/track-c-daily-report.md)
- Enterprise agency runbook: [`docs/enterprise/agency-runbook.md`](docs/enterprise/agency-runbook.md)
- Enterprise cutover guide: [`docs/enterprise/mode-cutover.md`](docs/enterprise/mode-cutover.md)
- Enterprise smoke doc: [`docs/enterprise/smoke.md`](docs/enterprise/smoke.md)

## Development and CI

Run tests:

```bash
go test ./... -count=1 -v
go test ./internal/ops/tests/smoke -count=1 -v
go test ./internal/enterprise/tests/smoke -count=1 -v
```

CI workflow: [`.github/workflows/ci.yaml`](.github/workflows/ci.yaml)

- macOS + Linux matrix
- race detector on Linux
- build artifact upload
- tagged release smoke build artifacts
