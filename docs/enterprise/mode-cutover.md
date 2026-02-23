# Enterprise Mode Cutover

Track D runs in strict `mode: enterprise` only. Legacy `~/.meta/config.yaml` behavior is intentionally incompatible and fails closed.

## Why cutover exists

- `enterprise.yaml` enforces workspace-scoped RBAC/policy/approval/audit/secret governance.
- Legacy profile config (`default_profile` / `profiles`) is rejected by enterprise loaders with an explicit migration error.
- No compatibility fallback is provided.

## Cutover command

```bash
meta enterprise mode cutover \
  --legacy-config ~/.meta/config.yaml \
  --config ~/.meta/enterprise.yaml \
  --org agency \
  --org-id org_1 \
  --workspace prod \
  --workspace-id ws_1 \
  --principal ops.admin
```

What this does:

- reads legacy config profiles from `--legacy-config`
- writes a strict enterprise config to `--config`
- sets `mode: enterprise`
- creates a bootstrap role (`legacy-cutover-operator`) with known command capabilities
- binds the bootstrap principal to the configured org/workspace

## Fail-closed migration states

- existing enterprise output path without `--force` -> hard failure
- empty legacy profile map -> hard failure
- missing required cutover metadata (`org`, `org-id`, `workspace`, `workspace-id`, `principal`) -> hard failure
- loading legacy config through enterprise loader -> hard failure with cutover command hint

## Post-cutover verification

```bash
meta enterprise context --config ~/.meta/enterprise.yaml --workspace agency/prod
meta enterprise policy eval --config ~/.meta/enterprise.yaml --principal ops.admin --capability graph.read --workspace agency/prod
```
