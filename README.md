# Meta Marketing CLI

Meta Marketing CLI is a developer-first, fail-closed command-line interface for running Meta marketing operations end-to-end without Ads Manager. It combines automated auth setup, direct Graph API access, campaign/ad lifecycle workflows, Instagram publishing, reliability checks, and enterprise controls behind a stable machine-readable output contract.

# Features
- Automated auth setup with browser callback (`meta auth setup`)
- Keychain-only secret storage (no env/plaintext fallback)
- Fail-closed profile preflight before operational commands
- Direct Graph API access (`api get/post/delete/batch`)
- Campaign, ad set, ad, creative, audience, and catalog workflows
- Budget mutation guardrails for spend-changing writes
- Instagram media upload, status, publish, and schedule lifecycle
- Operations intelligence checks (schema drift, rate limits, policy preflight)
- Stable output envelope (`contract_version: 1.0`) for automation

# Installation

## Prerequisites
- Go `1.23+`
- macOS Keychain or Linux Secret Service (required)
- `jq` (recommended for scripting and smoke checks)

## Install with `go install`
```bash
go install github.com/bilalbayram/metacli/cmd/meta@latest
meta --help
```

## Build locally
```bash
go build -o meta ./cmd/meta
./meta --help
```

# Quick Start

## 1) Get `APP_ID`, `APP_SECRET`, `REDIRECT_URI`
Create a Meta app in the developer dashboard and collect your app credentials.

Use this callback URI pattern for CLI auth automation:

```bash
REDIRECT_URI=https://<REDIRECT_URI>/oauth/callback
```

You must allow this redirect URI in your app OAuth settings, Meta doesn't allow http, you can use cloudflared or similar service for it:

```bash
cloudflared tunnel --url http://127.0.0.1:53682
```

## 2) Scopes
Scope packs are used by `auth setup` to request a practical scope set for your workflow (`solo_smb`, `ads_only`, `ig_publish`).

Official docs:
- [Facebook Login](https://developers.facebook.com/docs/facebook-login/)
- [Permissions Reference](https://developers.facebook.com/docs/permissions/reference/)
- [Access Tokens](https://developers.facebook.com/docs/facebook-login/access-tokens/)
- [Long-Lived Access Tokens](https://developers.facebook.com/docs/facebook-login/guides/access-tokens/get-long-lived)
- [Instagram Content Publishing](https://developers.facebook.com/docs/instagram-api/guides/content-publishing/)
- [Marketing APIs](https://developers.facebook.com/docs/marketing-apis/)

## 3) Setup auth and validate profile
```bash
./meta auth setup \
  --profile prod \
  --app-id <APP_ID> \
  --app-secret <APP_SECRET> \
  --mode both \
  --scope-pack solo_smb \
  --listen 127.0.0.1:53682 \
  --timeout 180s \
  --open-browser

./meta auth validate \
  --profile prod \
  --min-ttl 72h
```

# Usage

## Graph API Directly
```bash
./meta --profile prod api get act_<AD_ACCOUNT_ID>/campaigns \
  --fields id,name,status \
  --follow-next \
  --limit 100

./meta --profile prod api post act_<AD_ACCOUNT_ID>/campaigns \
  --params "name=Launch Campaign,objective=OUTCOME_SALES,status=PAUSED"

./meta --profile prod api delete <OBJECT_ID>
```

## Ad Creation
```bash
./meta --profile prod campaign create \
  --account-id <AD_ACCOUNT_ID> \
  --params "name=Launch Campaign,objective=OUTCOME_SALES,status=PAUSED" \
  --schema-dir ./schema-packs

./meta --profile prod adset create \
  --account-id <AD_ACCOUNT_ID> \
  --params "name=Prospecting,campaign_id=<CAMPAIGN_ID>,status=PAUSED,billing_event=IMPRESSIONS,optimization_goal=OFFSITE_CONVERSIONS" \
  --schema-dir ./schema-packs

./meta --profile prod ad create \
  --account-id <AD_ACCOUNT_ID> \
  --params "name=Launch Ad,adset_id=<ADSET_ID>,status=PAUSED" \
  --json '{"creative":{"creative_id":"<CREATIVE_ID>"}}' \
  --schema-dir ./schema-packs
```

## IG Publication
```bash
./meta --profile prod ig caption validate \
  --caption "Launch post #meta #ads" \
  --strict

./meta --profile prod ig media upload \
  --ig-user-id <IG_USER_ID> \
  --media-url https://cdn.example.com/image.jpg \
  --caption "Launch post #meta"

./meta --profile prod ig publish feed \
  --media-url https://cdn.example.com/image.jpg \
  --caption "Launch post #meta" \
  --idempotency-key publish-feed-001
```

## Marketing Write Workflows
Primary write command families:
- `campaign`: `create`, `update`, `pause`, `resume`, `clone`
- `adset`: `create`, `update`, `pause`, `resume`
- `ad`: `create`, `update`, `pause`, `resume`, `clone`
- `creative`: `upload`, `create`
- `audience`: `create`, `update`, `delete`
- `catalog`: `upload-items`, `batch-items`

Budget mutation guardrail example:
```bash
./meta --profile prod campaign update \
  --campaign-id <CAMPAIGN_ID> \
  --params "daily_budget=5000" \
  --confirm-budget-change
```

## Instagram Publishing + Plugin Runtime
- `ig media upload|status`
- `ig publish feed|reel|story`
- `ig publish schedule list|cancel|retry`
- Plugin namespace stubs: `wa`, `msgr`, `threads`, `capi`

## Operations Intelligence + Reliability
```bash
./meta --output json ops init --state-path "$HOME/.meta/ops/baseline-state.json"

./meta --output json ops run \
  --state-path "$HOME/.meta/ops/baseline-state.json" \
  --preflight-config-path "$HOME/.meta/config.yaml"
```

## Enterprise Hardening
```bash
./meta enterprise mode cutover \
  --legacy-config ~/.meta/config.yaml \
  --config ~/.meta/enterprise.yaml \
  --org agency \
  --org-id org_1 \
  --workspace prod \
  --workspace-id ws_1 \
  --principal ops.admin

./meta enterprise execute \
  --config ~/.meta/enterprise.yaml \
  --principal ops.admin \
  --command "auth rotate" \
  --workspace agency/prod \
  --approval-token <GRANT_TOKEN> \
  --correlation-id corr-20260224-001 \
  --require-secret auth_rotation_key:rotate
```

## Zero Ads Manager Workflow
```bash
# 1) Automated auth
./meta auth setup --profile prod --app-id <APP_ID> --app-secret <APP_SECRET> --mode both --scope-pack solo_smb

# 2) Create campaign + ad set + creative + ad
./meta --profile prod campaign create --account-id <AD_ACCOUNT_ID> --params "name=CLI Campaign,objective=OUTCOME_SALES,status=PAUSED" --schema-dir ./schema-packs
./meta --profile prod adset create --account-id <AD_ACCOUNT_ID> --params "name=CLI AdSet,campaign_id=<CAMPAIGN_ID>,status=PAUSED,billing_event=IMPRESSIONS,optimization_goal=OFFSITE_CONVERSIONS" --schema-dir ./schema-packs
./meta --profile prod creative create --account-id <AD_ACCOUNT_ID> --params "name=CLI Creative,object_story_id=123_456" --schema-dir ./schema-packs
./meta --profile prod ad create --account-id <AD_ACCOUNT_ID> --params "name=CLI Ad,adset_id=<ADSET_ID>,status=PAUSED" --json '{"creative":{"creative_id":"<CREATIVE_ID>"}}' --schema-dir ./schema-packs

# 3) Publish IG content
./meta --profile prod ig publish feed --media-url https://cdn.example.com/launch.jpg --caption "Shipped from CLI" --idempotency-key launch-feed-001

# 4) Run ops check
./meta --profile prod --output json ops run --state-path "$HOME/.meta/ops/baseline-state.json"
```

# Token Lifecycle Model

| `token_type` | Primary Use | Required Profile Keys (v2) | Lifecycle Path |
|---|---|---|---|
| `system_user` | Business automation and system flows | `app_id`, `business_id`, `token_ref`, `app_secret_ref`, auth metadata fields | Added via `auth add system-user`; validated by preflight; hard-fails on invalid/TTL/scope issues |
| `user` | OAuth user context for marketing + IG | `app_id`, `token_ref`, `app_secret_ref`, auth metadata fields | Created via `auth setup`/`auth login`; long-lived exchange + debug validation enforced |
| `page` | Page-scoped actions | `app_id`, `page_id`, `source_profile`, `token_ref`, `app_secret_ref`, auth metadata fields | Derived via `auth page-token`; source credentials and preflight checks required |
| `app` | App-level service token operations | `app_id`, `token_ref`, `app_secret_ref`, auth metadata fields | Created via `auth app-token set`; rotatable via `auth rotate` |

Auth metadata fields required on every profile in schema v2:
- `auth_provider`
- `auth_mode`
- `scopes`
- `issued_at`
- `expires_at`
- `last_validated_at`

# Complete Command Reference

## Core API and Schema

| Command Family | Purpose | Key Commands |
|---|---|---|
| `auth` | Authentication and profile/token lifecycle | `add system-user`, `setup`, `login`, `discover`, `page-token`, `app-token set`, `validate`, `rotate`, `debug-token`, `list` |
| `api` | Direct Graph API access | `get`, `post`, `delete`, `batch` |
| `insights` | Reporting queries and export | `run` |
| `lint` | Request lint against schema packs | `request` |
| `schema` | Local schema pack management | `list`, `sync` |
| `changelog` | Version/change checks | `check` |

## Marketing Workflows

| Command Family | Purpose | Key Commands |
|---|---|---|
| `campaign` | Campaign lifecycle | `create`, `update`, `pause`, `resume`, `clone` |
| `adset` | Ad set lifecycle | `create`, `update`, `pause`, `resume` |
| `ad` | Ad lifecycle | `create`, `update`, `pause`, `resume`, `clone` |
| `creative` | Creative assets | `upload`, `create` |
| `audience` | Audience lifecycle | `create`, `update`, `delete` |
| `catalog` | Catalog item ingestion/mutation | `upload-items`, `batch-items` |

## Instagram and Adjacent Product Namespaces

| Command Family | Purpose | Key Commands |
|---|---|---|
| `ig` | Instagram publishing lifecycle | `health`, `media upload`, `media status`, `caption validate`, `publish feed`, `publish reel`, `publish story`, `publish schedule list/cancel/retry` |
| `wa` | WhatsApp namespace scaffold | `health`, `capability` |
| `msgr` | Messenger namespace scaffold | `health`, `capability` |
| `threads` | Threads namespace scaffold | `health`, `capability` |
| `capi` | Conversions API namespace scaffold | `health`, `capability` |

## Ops and Governance

| Command Family | Purpose | Key Commands |
|---|---|---|
| `ops` | Reliability checks and report pipeline | `init`, `run` |
| `enterprise` | Org/workspace authorization and execution governance | `context`, `authz check`, `execute`, `mode cutover`, `approval request`, `approval approve`, `approval validate`, `policy eval` |

Global flags (all commands):
- `--profile <name>`
- `--output json|jsonl|table|csv`
- `--debug`

Use `./meta <family> --help` and `./meta <family> <command> --help` for full flag-level details.

# Output Contract

Most commands emit the stable envelope with `contract_version: "1.0"`:
- `contract_version`
- `command`
- `timestamp`
- `request_id`
- `success`
- `data`
- `paging`
- `rate_limit`
- `error`

# Exit Codes

- `1`: unknown failure
- `2`: config failure
- `3`: auth failure
- `4`: input/validation failure
- `5`: API failure

# Security Model

- Secrets are stored in OS keychain only (`meta-marketing-cli` service namespace)
- Config references secrets by keychain ref (`keychain://...`)
- Commands fail closed when required auth/config/schema data is missing or invalid
- No hidden fallback to environment variables or plaintext secrets
