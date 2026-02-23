# Agency Enterprise Runbook

This runbook defines daily enterprise operations for multi-client agency usage with strict fail-closed controls.

## Preconditions

- Enterprise config exists at `~/.meta/enterprise.yaml`
- Config has `schema_version: 1` and `mode: enterprise`
- Each client is modeled as an org/workspace pair
- Required principals are bound to workspace roles
- High-risk command approvers are available

If starting from legacy config, run the cutover flow in `/Users/Bayram/Developer/meta-marketing-cli/docs/enterprise/mode-cutover.md`.

## 1) Workspace setup and validation

Validate workspace context:

```bash
meta enterprise context --config ~/.meta/enterprise.yaml --workspace <org>/<workspace>
```

Expected result:

- `org_name`, `org_id`, `workspace_name`, `workspace_id` resolve correctly
- command exits non-zero on invalid org/workspace references

## 2) Role assignment verification

Validate capability mapping for an operator:

```bash
meta enterprise policy eval \
  --config ~/.meta/enterprise.yaml \
  --principal <principal> \
  --capability <capability> \
  --workspace <org>/<workspace>
```

Expected result:

- `allowed=true` only when role bindings in that workspace grant capability
- deny reason is explicit and deterministic on missing binding/capability

## 3) Approval workflow for high-risk commands

Create approval request token:

```bash
meta enterprise approval request \
  --config ~/.meta/enterprise.yaml \
  --principal <principal> \
  --command "auth rotate" \
  --workspace <org>/<workspace> \
  --ttl 30m
```

Approve request:

```bash
meta enterprise approval approve \
  --request-token <request_token> \
  --approver <approver_principal> \
  --decision approved \
  --ttl 30m
```

Validate grant token:

```bash
meta enterprise approval validate \
  --config ~/.meta/enterprise.yaml \
  --grant-token <grant_token> \
  --principal <principal> \
  --command "auth rotate" \
  --workspace <org>/<workspace>
```

## 4) Governed execution path

Execute through enterprise pipeline:

```bash
meta enterprise execute \
  --config ~/.meta/enterprise.yaml \
  --principal <principal> \
  --command "auth rotate" \
  --workspace <org>/<workspace> \
  --approval-token <grant_token> \
  --correlation-id <correlation_id> \
  --require-secret auth_rotation_key:rotate
```

Expected controls:

- RBAC/policy must allow capability
- approval token must be valid for command fingerprint and TTL
- secret governance must authorize required secret action in workspace scope
- decision + execution audit events are emitted with shared correlation ID

## 5) Incident troubleshooting

### Authorization denied

Symptoms:

- error contains `authorization denied: ...`
- policy trace shows missing binding/capability or explicit deny

Actions:

- verify principal role binding in target workspace
- verify role contains required capability and no deny override

### Approval failures

Symptoms:

- `approval token is required`
- `approval grant expired`
- `fingerprint mismatch`

Actions:

- issue a new approval request/grant for exact principal/command/org/workspace
- verify TTL and approver decision

### Secret governance failures

Symptoms:

- `secret ... is scoped to ... not ...`
- `not allowed to <action> secret ...`
- `policy enforcement hook[...] is nil/denied`

Actions:

- confirm secret scope matches execution workspace
- confirm policy grants action for principal
- fix or remove failing enforcement hook

### Audit invariant violations

Symptoms:

- execution event rejected because decision event missing/denied/mismatched

Actions:

- ensure command runs through `meta enterprise execute`
- ensure unique correlation IDs and one execution per decision
