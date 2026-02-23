## 1) If I were building a Meta Marketing CLI, what features make sense given the APIs?

### Core design constraint: you’re really building a **Graph API CLI**

Meta’s Marketing API is “a collection of Graph API endpoints and other features”. ([Facebook Developers][1])
So the CLI should be built around Graph primitives:

* **Nodes / edges / fields model** (IDs + connections + `fields=` selection). ([Facebook Developers][2])
* **Explicit versioning** (pin `vXX.X` per profile/command). Graph API versions are published on a version list/changelog. ([Facebook Developers][3])

  * Marketing API has its own versioning/deprecation policy (not identical to the Platform Graph policy). ([Facebook Developers][4])
* **No runtime schema introspection going forward**: starting Graph API **v25.0**, `metadata=1` is deprecated and no longer returns node metadata. ([Facebook Developers][5])

That last point is huge for a CLI: you can’t rely on “discover fields/edges live” anymore (at least not via `metadata=1`).

### Feature set that fits the Marketing API + Graph constraints

#### A. “Universal Graph client” commands (the foundation)

These are what make the CLI future-proof even when Meta adds endpoints:

* `meta api get|post|delete <path>`

  * Supports query params, JSON bodies, file upload (multipart), and the `fields=` pattern.
* **Pagination helpers** (cursor-based): auto-follow `next` until limit, stream results.
* **Output modes**: raw JSON, jq-friendly JSON lines, table view, CSV export.
* **Error normalization**: always print `code`, `error_subcode`, message, and `fbtrace_id` so issues are debuggable and supportable. ([Facebook Developers][6])
* **Rate-limit aware retries/backoff**: Graph has documented rate limiting; once you hit a use-case limit, calls can fail with specific errors. ([Facebook Developers][7])
* **Batch request wrapper** (Graph batch API):

  * Build and submit batches from a file or stdin.
  * Enforce the hard limit: **50 requests per batch**; also surface that each call still counts separately for limits. ([Facebook Developers][8])

This layer is what lets you “use everything Meta releases” without waiting for your CLI to add a new subcommand.

#### B. Marketing-specific workflows (high ROI)

Once the raw Graph client exists, the CLI can add high-level commands that hide ugly details and reduce spend-risk.

1. **Account & asset discovery**

* List businesses, ad accounts, pixels, catalogs, pages linked to business, etc.
* “Resolve name → ID” helpers where possible (many Marketing operations are ID-first).

2. **Campaign lifecycle ops**

* Create/update campaigns, ad sets, ads.
* Status operations (`PAUSED`, etc.), budget/schedule edits, bulk pause/resume.
* “Clone” flows (read → transform → create).

3. **Creatives & assets**

* Upload images/videos, create creatives, validate previews.

4. **Insights/reporting**

* One command that wraps Insights API (consistent interface is part of the value proposition). ([Facebook Developers][9])
* Support:

  * level (campaign/adset/ad), time presets, breakdowns, attribution settings
  * export to CSV/Parquet
  * **async report runs** wrapper (start → poll → download), because Meta supports async/batch patterns for scaling. ([Facebook Developers][10])

5. **Audiences & measurement**

* Custom audiences, lookalikes, exclusions.
* Pixel/event source plumbing, offline conversions, etc.
* Catalog management (feeds + items batch). The catalog Items Batch endpoint has its own scaling constraints (e.g., `requests` can contain up to thousands of items). ([Facebook Developers][11])

6. **Safety/guardrails that a CLI can enforce**
   Even if you don’t care about “UX niceness”, a CLI that can mutate spend **must** reduce operator error:

* `--dry-run` for any write operation (print exact HTTP call it would make).
* `plan/apply` mode (Terraform-style): compute diffs first, then apply.
* Hard requirement for explicit targeting/budget confirmations on high-impact mutations.

#### C. “Version & changelog intelligence”

Because Meta’s versions and out-of-cycle changes matter operationally:

* `meta changelog check`

  * compares your pinned version against the currently available versions list and warns if you’re approaching expiry. ([Facebook Developers][3])
* `meta marketing occ` (out-of-cycle changes watcher)

  * alerts when OCC pages change (these are how you get surprised by targeting/business rule changes). ([Facebook Developers][12])
* `meta lint`

  * validates that requests you’re about to run don’t use deprecated params/fields (you maintain a local ruleset).

This compensates for losing `metadata=1` introspection in v25+. ([Facebook Developers][5])

---

## 2) Not just marketing: how to cover “everything Meta exposes via APIs”

The only scalable strategy is:

### A. Make the CLI **multi-product, plugin-based**

* Core: HTTP + auth + retries + pagination + batching + output.
* Plugins: each product = domain + auth requirements + convenience commands.

Meta APIs are not just one surface:

* Graph API for social data & many products. ([Facebook Developers][2])
* WhatsApp Cloud API message operations. ([Facebook Developers][13])
* Messenger Platform Send API. ([Facebook Developers][14])
* Instagram content publishing via container → publish flow. ([Facebook Developers][15])
* Threads API (posting, metrics; accessed via `graph.threads.com` or `graph.threads.net`). ([Facebook Developers][16])
* Conversions API (server-side events via `/events`). ([Facebook Developers][17])
* Ad Library API (distinct product; Meta even publishes a script repo with a simple CLI for it). ([Facebook][18])

### B. Provide a “universal call” escape hatch for new endpoints

Because Meta will ship endpoints faster than you can add wrappers:

```bash
meta api get /{id} --fields "id,name"
meta api post /{id}/edge --json @payload.json
```

That’s how you cover “everything” *immediately*.

### C. Build **schema packs** instead of live discovery

Since `metadata=1` is deprecated in v25, you need a local representation of:

* known objects
* allowed fields/edges
* parameter names and enums
* common validation rules

Sources for schema packs:

* scrape/compile Meta docs (versioned)
* ship updates with the CLI
* optionally let users pin a schema pack version independently of the binary

And still keep the raw `meta api` call for anything not in the schema pack.

---

## 3) How auth should work in a Meta CLI

### The reality: “Auth” is not one thing — it’s **token type + permissions + asset access**

Meta uses access tokens and permissions as granular authorization. ([Facebook Developers][19])
Your CLI needs to handle at least these token classes:

1. **User access token** (interactive user)
2. **Page access token** (Page operations; derived from a user token)
3. **System user access token** (server-to-server automation in Business Manager)
4. **App access token** (app-level endpoints, debugging; *not* user data)

### A. Recommended auth modes for a CLI

#### Mode 1: System User token (best for automation / prod)

Why: it avoids tying your automation to a human login.

* Meta provides system users under Business Manager and guidance for installing apps and generating tokens. ([Facebook Developers][20])
* There is an API edge to create system user access tokens: `POST /{business_id}/system_user_access_tokens`. ([Facebook Developers][21])
* Meta documentation describes **expiring** system user tokens (valid 60 days) that must be refreshed within that window. ([Facebook Developers][20])
* Other Meta guidance also describes system user tokens as non-expiring in certain approaches (i.e., the motivation for using them is that they don’t “go away” like user tokens when users leave). ([Facebook Developers][22])
  Practical implication: the CLI must treat expiry as configurable/observable and expose “refresh/rotate token” workflows, not assume permanence.

CLI features for this mode:

* `meta auth add system-user --token … --business …`
* `meta auth validate` (calls token debug endpoints where possible)
* `meta auth rotate` (prompts for generating a new token and swapping it in config)

#### Mode 2: User login (good for dev, one-off ops)

Implement OAuth 2.0 authorization code flow for installed apps:

* If you implement an OIDC/PKCE-style flow, you avoid embedding a client secret in the CLI (important because a distributed CLI binary is not a confidential client). Meta documents obtaining OIDC tokens with PKCE. ([Facebook Developers][23])
* User tokens can be exchanged to long-lived tokens (~60 days in common flows). ([Facebook Developers][24])

CLI UX pattern:

* `meta auth login`

  * starts a localhost callback
  * opens browser
  * stores resulting token in OS keychain / encrypted store

#### Mode 3: Page token (Pages/IG publishing)

Meta’s Access Token Guide states you obtain a Page token by first obtaining a user token, then using it to fetch a Page access token via the Graph API. ([Facebook Developers][25])
So the CLI should support:

* `meta auth page-token --page <id>` (derives and stores it)
* automatic selection of page token when calling endpoints that require it

#### Mode 4: App token (debugging + app-only endpoints)

Meta docs explicitly show using an app token by passing `access_token=<app_id>|<app_secret>` for some calls. ([Facebook Developers][25])
Also:

* The `/debug_token` endpoint requires an **app access token** (or a developer user token for that app). ([Facebook Developers][26])

CLI should implement:

* `meta auth app-token set` (stores app_id + secret securely; computes app token on demand)
* `meta auth debug-token <token>` (wraps `/debug_token`)

### B. Token debugging and safety checks

A Meta CLI should always be able to answer:

* what app issued this token?
* what permissions does it actually have?
* when does it expire?
* is it valid for the target asset (ad account / page / pixel)?

The `/debug_token` endpoint exists specifically for inspecting tokens. ([Facebook Developers][26])

### C. “Require App Secret” / appsecret_proof

If the app has “Require App Secret” enabled, calls must include `appsecret_proof`:

* Meta documents generating `appsecret_proof` as an HMAC-SHA256 of the access token using the app secret, and sending it as a parameter. ([Facebook Developers][27])
  So the CLI should:
* compute and attach `appsecret_proof` automatically when the profile has an app secret configured
* never log tokens/secrets by default (redact; allow `--debug` with explicit opt-in)

### D. Multi-domain token formats (if you truly cover “all Meta APIs”)

Some Meta surfaces use different app-token prefixes/formats:

* Threads examples show `TH|<APP_ID>|<APP_SECRET>` for app access tokens. ([Facebook Developers][28])
* Gaming graph domain uses `GG|{app_id}|{app_secret}`. ([Facebook Developers][29])
* Oculus/Quest surfaces use `OC|App_ID|App_Secret`. ([Meta for Developers][30])

So your CLI should store auth profiles per **API domain/product**, not assume one token format everywhere.

---

If you want a minimal blueprint that still covers everything Meta will ship:

1. ship a universal `meta api` caller + batching + pagination + auth profiles
2. add opinionated “marketing workflows” on top (campaign CRUD + insights + catalogs + audiences)
3. keep product plugins (WhatsApp, Messenger, Instagram, Threads, CAPI) as optional modules that mostly map to well-typed wrappers around the same core HTTP/auth engine.

[1]: https://developers.facebook.com/docs/marketing-api/?utm_source=chatgpt.com "Marketing API - Meta for Developers - Facebook"
[2]: https://developers.facebook.com/docs/graph-api/overview/?utm_source=chatgpt.com "Graph API Overview"
[3]: https://developers.facebook.com/docs/graph-api/changelog/versions/?utm_source=chatgpt.com "Versions - Graph API"
[4]: https://developers.facebook.com/docs/marketing-api/overview/versioning/?utm_source=chatgpt.com "Versioning - Marketing API"
[5]: https://developers.facebook.com/blog/post/2026/02/18/introducing-graph-api-v25-and-marketing-api-v25/?utm_source=chatgpt.com "Introducing Graph API v25.0 and Marketing API v25.0I"
[6]: https://developers.facebook.com/docs/graph-api/guides/error-handling/?utm_source=chatgpt.com "Handle Errors - Graph API"
[7]: https://developers.facebook.com/docs/graph-api/overview/rate-limiting/?utm_source=chatgpt.com "Rate Limits - Graph API - Meta for Developers"
[8]: https://developers.facebook.com/docs/graph-api/batch-requests/?utm_source=chatgpt.com "Batch Requests - Graph API"
[9]: https://developers.facebook.com/docs/marketing-api/insights/?utm_source=chatgpt.com "Insights API - Marketing API - Meta for Developers - Facebook"
[10]: https://developers.facebook.com/docs/marketing-api/asyncrequests/?utm_source=chatgpt.com "Async and Batch Requests - Marketing API"
[11]: https://developers.facebook.com/docs/marketing-api/reference/product-catalog/items_batch/?utm_source=chatgpt.com "Graph API Reference v25.0: Product Catalog Items Batch"
[12]: https://developers.facebook.com/docs/marketing-api/out-of-cycle-changes/occ-2025/?utm_source=chatgpt.com "2025 - Marketing API - Meta for Developers - Facebook"
[13]: https://developers.facebook.com/documentation/business-messaging/whatsapp/reference/whatsapp-business-phone-number/message-api?utm_source=chatgpt.com "Messages - WhatsApp Cloud API - Meta for Developers"
[14]: https://developers.facebook.com/docs/messenger-platform/reference/send-api/?utm_source=chatgpt.com "Send API - Messenger Platform - Meta for Developers"
[15]: https://developers.facebook.com/docs/instagram-platform/content-publishing/?utm_source=chatgpt.com "Publish Content - Instagram Platform - Meta for Developers"
[16]: https://developers.facebook.com/docs/threads/overview/?utm_source=chatgpt.com "Overview - Threads API - Meta for Developers"
[17]: https://developers.facebook.com/docs/marketing-api/conversions-api/guides/end-to-end-implementation/?utm_source=chatgpt.com "Conversions API End-to-End Implementation"
[18]: https://www.facebook.com/ads/library/api/?utm_source=chatgpt.com "Meta Ad Library API"
[19]: https://developers.facebook.com/docs/permissions/?utm_source=chatgpt.com "Permissions Reference - Graph API - Meta for Developers"
[20]: https://developers.facebook.com/docs/business-management-apis/system-users/install-apps-and-generate-tokens/?utm_source=chatgpt.com "Install Apps, Generate, Refresh, and Revoke Tokens"
[21]: https://developers.facebook.com/docs/marketing-api/reference/business/system_user_access_tokens/?utm_source=chatgpt.com "Business System User Access Tokens - Meta for Developers"
[22]: https://developers.facebook.com/docs/business-management-apis/business-manager/guides/on-behalf-of/?utm_source=chatgpt.com "On Behalf Of - Business Management APIs"
[23]: https://developers.facebook.com/docs/facebook-login/guides/advanced/oidc-token/?utm_source=chatgpt.com "OIDC Token with Manual Flow - Facebook Login"
[24]: https://developers.facebook.com/docs/facebook-login/guides/access-tokens/get-long-lived/?utm_source=chatgpt.com "Get Long-Lived Tokens - Facebook Login"
[25]: https://developers.facebook.com/docs/facebook-login/guides/access-tokens/?utm_source=chatgpt.com "Access Token Guide - Facebook Login - Meta for Developers"
[26]: https://developers.facebook.com/docs/graph-api/reference/debug_token/?utm_source=chatgpt.com "Debug Token - Graph API - Meta for Developers"
[27]: https://developers.facebook.com/docs/graph-api/guides/secure-requests/?utm_source=chatgpt.com "Secure Requests - Graph API"
[28]: https://developers.facebook.com/docs/threads/reference/oembed/?utm_source=chatgpt.com "oEmbed - Threads API - Meta for Developers"
[29]: https://developers.facebook.com/docs/games/build/gaming-services/domain/?utm_source=chatgpt.com "Gaming Graph Domain (graph.fb.gg) - Facebook Games"
[30]: https://developers.meta.com/horizon/documentation/native/ps-attestation-api/?utm_source=chatgpt.com "Meta Quest Attestation API"
