# Meta Marketing CLI Usage

Operator-focused guide for the `meta` binary. The CLI keeps a stable machine-readable envelope, but the command surfaces are provider-specific:
- `meta` = existing Meta / Graph / Instagram / Messenger / WhatsApp / Threads / CAPI / ops workflows.
- `meta li` = LinkedIn Marketing workflows.

Use `--help` on any command for the full flag set. Prefer explicit `--profile` values and `--output json` unless you are scripting around `jsonl`, `table`, or `csv`.

## Profile Model

Profiles live in `~/.meta/config.yaml` with `schema_version: 2`.

- Existing Meta profiles remain valid with an implicit `provider: meta`.
- LinkedIn profiles must set `provider: linkedin`.
- Meta profiles use `graph_version`.
- LinkedIn profiles use `linkedin_version` and never route through Graph path versioning.

Minimal shape:

```yaml
schema_version: 2
default_profile: prod
profiles:
  prod:
    domain: marketing
    graph_version: v25.0
    token_type: system_user
    app_id: <APP_ID>
    app_secret_ref: keychain://...
    token_ref: keychain://...
    auth_provider: system_user
    auth_mode: both
    scopes: [<meta scopes>]
    issued_at: 2026-03-23T00:00:00Z
    expires_at: 2026-03-24T00:00:00Z
    last_validated_at: 2026-03-23T00:00:00Z
  li-prod:
    provider: linkedin
    domain: marketing
    linkedin_version: "202601"
    client_id: <CLIENT_ID>
    client_secret_ref: keychain://...
    refresh_token_ref: keychain://...
    token_ref: keychain://...
    scopes: [<approved linkedin scopes>]
    issued_at: 2026-03-23T00:00:00Z
    expires_at: 2026-03-24T00:00:00Z
    last_validated_at: 2026-03-23T00:00:00Z
```

## Quick Start

### Meta

```bash
meta auth setup \
  --profile prod \
  --app-id <APP_ID> \
  --app-secret <APP_SECRET> \
  --mode both \
  --scope-pack solo_smb

meta auth validate --profile prod
```

### LinkedIn

Browser-based OAuth setup is the primary path.

Auth flow selection:
- `meta li auth setup` now defaults to the standard LinkedIn 3-legged OAuth endpoint: `https://www.linkedin.com/oauth/v2/authorization`
- Use `--auth-flow native-pkce` only if LinkedIn has explicitly enabled PKCE-native auth for your app; that path uses `https://www.linkedin.com/oauth/native-pkce/authorization`
- For most server-side or CLI operator workflows, the standard flow is the correct default

Where `CLIENT_ID` and `CLIENT_SECRET` come from:
- Go to the LinkedIn Developer Portal: `https://www.linkedin.com/developers/apps`
- Create a new app, or open an existing app under `My Apps`
- If you create a new app, LinkedIn requires the usual app metadata first, including an app name, your organization's LinkedIn Page, a privacy policy URL, and an app logo
- Open the app's `Auth` tab
- Copy the `Client ID` and reveal/copy the `Client Secret` from the application credentials area
- Add your OAuth callback URL under the same `Auth` tab before running `meta li auth setup`
- If you do not see the marketing scopes you need, request the appropriate product access under the app's `Products` tab first; LinkedIn only exposes scopes that your app has been approved for
- For Marketing API usage, also configure the allowed ad accounts from the app's `Products` tab when LinkedIn requires it for your tier

Recommended minimum check before using the CLI:
- confirm the app has the approved scopes you plan to request, such as `r_ads`, `rw_ads`, `r_ads_reporting`, or `r_marketing_leadgen_automation`
- confirm the authenticated LinkedIn member has the required ad account or organization roles, otherwise auth may succeed but resource commands will still fail closed

Best-effort identity lookup:
- `meta li auth setup` and `meta li auth validate` treat LinkedIn member lookup as optional
- for Ads-only apps, `whoami` may be `null` with a warning if the app does not have an identity scope such as `r_liteprofile`
- Ads commands such as `meta li account list` can still work even when member profile lookup is unavailable

```bash
meta li auth setup \
  --profile li-prod \
  --client-id <CLIENT_ID> \
  --client-secret <CLIENT_SECRET> \
  --auth-flow standard \
  --linkedin-version 202601 \
  --scopes <approved,comma-separated,LinkedIn,scopes> \
  --listen-addr 127.0.0.1:53682 \
  --open-browser

meta li auth validate --profile li-prod
meta li auth scopes --profile li-prod
meta li auth whoami --profile li-prod
```

## Existing Meta Surfaces

Top-level families on the existing Meta side:
- `auth`: `setup`, `login`, `login-manual`, `discover`, `validate`, `rotate`, `debug-token`, `list`
- `api`: `get`, `post`, `delete`, `batch`
- `campaign`, `adset`, `ad`, `creative`, `audience`, `catalog`
- `insights`: `accounts list`, `run`, `action-types`
- `ig`: `caption validate`, `media upload/status`, `publish feed|reel|story`, `publish schedule list|cancel|retry|run`, `insights ...`
- `msgr`: `health`, `auto-reply`, `conversations`
- `wa`: `health`, `capability`
- `threads`: `health`, `capability`
- `capi`: `health`, `capability`
- `ops`: `init`, `run`
- `doctor`: `tracer`
- `schema`: `list`, `sync`
- `lint`: `request`
- `changelog`: `check`
- `enterprise`: context, authorization, approvals, execution governance
- `smoke`: deterministic capability-aware smoke workflows

Common examples:

```bash
meta --profile prod api get act_<AD_ACCOUNT_ID>/campaigns \
  --fields id,name,status \
  --follow-next

meta --profile prod campaign create \
  --account-id <AD_ACCOUNT_ID> \
  --params "name=Launch Campaign,objective=OUTCOME_SALES,status=PAUSED"

meta --profile prod insights accounts list --active-only --output table

meta schema sync

meta lint request \
  --file ./requests/campaign-create.json \
  --strict

meta --profile prod ig publish feed \
  --media-url https://cdn.example.com/launch.jpg \
  --caption "Shipped from CLI" \
  --idempotency-key launch-feed-001

meta smoke run

meta msgr health
```

## LinkedIn Surfaces

Top-level namespaces:
- `auth`
- `api`
- `account`
- `organization`
- `campaign-group`
- `campaign`
- `creative`
- `insights`
- `targeting`
- `lead-form`
- `lead`

Provider caveats:
- `li account list` is the discovery entrypoint for ad accounts.
- LinkedIn raw API and reporting use `--version YYYYMM` or the profile's `linkedin_version`.
- `li targeting` is policy-sensitive. Use lawful targeting only and expect validation failures to block unsafe combinations.
- `li lead sync` is stateful and idempotent. Use `--state-file` and `--reset` intentionally.

Auth:

```bash
meta li auth setup \
  --profile li-prod \
  --client-id <CLIENT_ID> \
  --client-secret <CLIENT_SECRET> \
  --auth-flow standard \
  --linkedin-version 202601 \
  --scopes <approved,comma-separated,LinkedIn,scopes>

meta li auth validate --profile li-prod
meta li auth scopes --profile li-prod
meta li auth whoami --profile li-prod
```

Raw API:

```bash
meta li api get /rest/adAccounts \
  --profile li-prod \
  --version 202601 \
  --follow-next \
  --page-size 100

meta li api post /rest/adCampaignGroups \
  --profile li-prod \
  --version 202601 \
  --json '{"name":"CLI test group"}'

meta li api delete /rest/<resource>/<id> \
  --profile li-prod \
  --version 202601
```

Discovery:

```bash
meta li account list \
  --profile li-prod \
  --search active \
  --page-size 100

meta li organization list \
  --profile li-prod \
  --search "Acme"

meta li campaign-group list \
  --profile li-prod \
  --account-urn urn:li:sponsoredAccount:<ID>

meta li campaign list \
  --profile li-prod \
  --account-urn urn:li:sponsoredAccount:<ID>

meta li creative list \
  --profile li-prod \
  --account-urn urn:li:sponsoredAccount:<ID>
```

Reporting:

```bash
meta --profile li-prod li insights metrics list
meta --profile li-prod li insights pivots list

meta li insights run \
  --profile li-prod \
  --account-urns urn:li:sponsoredAccount:<ID> \
  --since 2026-03-01 \
  --until 2026-03-07 \
  --level CAMPAIGN \
  --metric-pack delivery
```

Core ad ops:

```bash
meta li campaign-group pause \
  --profile li-prod \
  --version 202601 \
  --confirm \
  <CAMPAIGN_GROUP_ID>

meta li campaign create \
  --profile li-prod \
  --account-urn urn:li:sponsoredAccount:<ID> \
  --confirm-budget-change \
  --confirm-schedule-change \
  --json '{"name":"CLI Campaign"}'

meta li creative create \
  --profile li-prod \
  --account-urn urn:li:sponsoredAccount:<ID> \
  --json '{"name":"CLI Creative"}'
```

Targeting:

```bash
meta --profile li-prod li targeting facets --version 202601
meta --profile li-prod li targeting search --version 202601
meta li targeting validate \
  --profile li-prod \
  --account-urn urn:li:sponsoredAccount:<ID> \
  --facet-urns <URN_1,URN_2>
```

Lead intake:

```bash
meta li lead-form list \
  --profile li-prod \
  --account-urn urn:li:sponsoredAccount:<ID>

meta li lead list \
  --profile li-prod \
  --account-urn urn:li:sponsoredAccount:<ID>

meta li lead sync \
  --profile li-prod \
  --account-urn urn:li:sponsoredAccount:<ID> \
  --state-file ~/.meta/li-leads.state.json \
  --page-size 100

meta li lead webhook list --profile li-prod
```

## Practical Notes

- Use `meta doctor tracer` when you want a quick integrity check on command plumbing.
- Use `meta auth validate` or `meta li auth validate` before running a batch of writes.
- Keep output automation-friendly. `json` and `jsonl` are the safest defaults for scripts.
- For LinkedIn, prefer raw `li api` calls only when a higher-level command does not yet exist. For routine ops, use the typed resource commands first.
