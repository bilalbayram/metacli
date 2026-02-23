# Track C Daily Report Automation

This document defines the `meta ops run` daily-report path used by automation for Track C.

## Scope

- Command: `meta ops run`
- Contract: `ops.v1` envelope + `ops_report` payload
- Baseline source: `meta ops init` state file
- Exit semantics: warnings and blocking findings return non-success exit codes

## Daily Automation Path

1. Initialize baseline once:

```bash
meta --output json ops init --state-path "$HOME/.meta/ops/baseline-state.json"
```

2. Run the daily report:

```bash
meta --output jsonl ops run --state-path "$HOME/.meta/ops/baseline-state.json"
```

3. Optional enrichments for automation jobs:

```bash
meta --output json ops run \
  --state-path "$HOME/.meta/ops/baseline-state.json" \
  --rate-telemetry-file /path/to/rate-telemetry.json \
  --runtime-response-file /path/to/runtime-response.json \
  --lint-request-file /path/to/lint-request.json \
  --preflight-config-path /path/to/config.yaml
```

The automation must treat non-zero command exit as a failed run. There is no fallback path.

## Report Contract

Successful runs return an envelope with:

- `contract_version`: `ops.v1`
- `command`: `meta ops run`
- `success`: `true` for clean runs, `false` for warning/blocking runs
- `exit_code`: strict policy/warning mapping
- `data`: `RunResult`

`RunResult.report` guarantees:

- `schema_version`: `1`
- `kind`: `ops_report`
- deterministic section order: `monitor`, `drift`, `rate_limit`, `preflight`
- deterministic check order:
  - `changelog_occ_delta`
  - `schema_pack_drift`
  - `rate_limit_threshold`
  - `permission_policy_preflight`
  - `runtime_response_shape_drift`

For `jsonl`, one envelope line is emitted per section in the section order above.
For `csv`, rows are emitted in section/check order with a stable header.

## Fingerprint Behavior

Two baseline fingerprints are carried and enforced by the report path:

- changelog OCC fingerprint: `baseline.snapshots.changelog_occ.occ_digest`
- schema pack fingerprint: `baseline.snapshots.schema_pack.sha256`

Behavior guarantees:

- unchanged fingerprints: checks pass and messages include fingerprint values
- fingerprint drift: checks become blocking and messages include baseline + current fingerprints

## Smoke Coverage

Smoke suite: `internal/ops/tests/smoke/daily_report_smoke_test.go`

The suite locks:

- `json` envelope/report contract for daily run
- `jsonl` section-line contract and order
- `csv` section/check row contract and order
- fingerprint propagation + blocking behavior on drift

Run locally:

```bash
go test ./internal/ops/tests/smoke -count=1 -v
```
