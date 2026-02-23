package smoke

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilalbayram/metacli/internal/cli"
)

func TestEnterpriseAgencySmokeApprovalToExecutionFlow(t *testing.T) {
	t.Parallel()

	configPath := writeEnterpriseSmokeConfig(t, true)

	_, _, err := runEnterpriseSmokeCommand([]string{
		"--output", "json",
		"enterprise", "context",
		"--config", configPath,
		"--workspace", "agency/alpha",
	})
	if err != nil {
		t.Fatalf("resolve enterprise context: %v", err)
	}

	grantToken := issueApprovalGrant(t, configPath, "ops.admin")

	stdout, _, err := runEnterpriseSmokeCommand([]string{
		"--output", "json",
		"enterprise", "execute",
		"--config", configPath,
		"--principal", "ops.admin",
		"--command", "auth rotate",
		"--workspace", "agency/alpha",
		"--approval-token", grantToken,
		"--correlation-id", "smoke-success-001",
		"--require-secret", "auth_rotation_key:rotate",
	})
	if err != nil {
		t.Fatalf("run enterprise execute smoke: %v", err)
	}

	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Execution struct {
				Status string `json:"status"`
			} `json:"execution"`
			AuditEvents []struct {
				EventType       string `json:"event_type"`
				ExecutionStatus string `json:"execution_status"`
			} `json:"audit_events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode execute envelope: %v", err)
	}
	if !envelope.Success {
		t.Fatal("expected execute smoke success envelope")
	}
	if envelope.Data.Execution.Status != "succeeded" {
		t.Fatalf("unexpected execution status: %q", envelope.Data.Execution.Status)
	}
	if len(envelope.Data.AuditEvents) != 2 {
		t.Fatalf("unexpected audit event count: %d", len(envelope.Data.AuditEvents))
	}
	if envelope.Data.AuditEvents[0].EventType != "decision" {
		t.Fatalf("unexpected first audit event: %+v", envelope.Data.AuditEvents[0])
	}
	if envelope.Data.AuditEvents[1].EventType != "execution" {
		t.Fatalf("unexpected second audit event: %+v", envelope.Data.AuditEvents[1])
	}
	if envelope.Data.AuditEvents[1].ExecutionStatus != "succeeded" {
		t.Fatalf("unexpected execution audit status: %q", envelope.Data.AuditEvents[1].ExecutionStatus)
	}
}

func TestEnterpriseAgencySmokeIncidentDeniedSecretAccess(t *testing.T) {
	t.Parallel()

	configPath := writeEnterpriseSmokeConfig(t, false)
	grantToken := issueApprovalGrant(t, configPath, "ops.admin")

	_, _, err := runEnterpriseSmokeCommand([]string{
		"--output", "json",
		"enterprise", "execute",
		"--config", configPath,
		"--principal", "ops.admin",
		"--command", "auth rotate",
		"--workspace", "agency/alpha",
		"--approval-token", grantToken,
		"--correlation-id", "smoke-deny-001",
		"--require-secret", "auth_rotation_key:rotate",
	})
	if err == nil {
		t.Fatal("expected execute smoke to fail when secret access is denied")
	}
	if !strings.Contains(err.Error(), "authorization denied") {
		t.Fatalf("unexpected execute failure: %v", err)
	}
	if !strings.Contains(err.Error(), "not allowed to rotate secret") {
		t.Fatalf("unexpected deny reason: %v", err)
	}
}

func issueApprovalGrant(t *testing.T, configPath string, principal string) string {
	t.Helper()

	requestOut, _, err := runEnterpriseSmokeCommand([]string{
		"--output", "json",
		"enterprise", "approval", "request",
		"--config", configPath,
		"--principal", principal,
		"--command", "auth rotate",
		"--workspace", "agency/alpha",
		"--ttl", "30m",
	})
	if err != nil {
		t.Fatalf("create smoke approval request token: %v", err)
	}

	requestToken := extractTokenField(t, requestOut.Bytes(), "request_token")

	approveOut, _, err := runEnterpriseSmokeCommand([]string{
		"--output", "json",
		"enterprise", "approval", "approve",
		"--request-token", requestToken,
		"--approver", "security.lead",
		"--decision", "approved",
		"--ttl", "30m",
	})
	if err != nil {
		t.Fatalf("create smoke approval grant token: %v", err)
	}
	return extractTokenField(t, approveOut.Bytes(), "grant_token")
}

func extractTokenField(t *testing.T, payload []byte, field string) string {
	t.Helper()

	envelope := map[string]any{}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}

	dataValue, ok := envelope["data"]
	if !ok {
		t.Fatalf("missing envelope data: %+v", envelope)
	}
	dataMap, ok := dataValue.(map[string]any)
	if !ok {
		t.Fatalf("invalid envelope data shape: %+v", dataValue)
	}
	tokenValue, ok := dataMap[field]
	if !ok {
		t.Fatalf("missing %s in envelope data: %+v", field, dataMap)
	}
	token, ok := tokenValue.(string)
	if !ok || strings.TrimSpace(token) == "" {
		t.Fatalf("invalid %s value: %+v", field, tokenValue)
	}
	return token
}

func runEnterpriseSmokeCommand(args []string) (*bytes.Buffer, *bytes.Buffer, error) {
	root := cli.NewRootCommand()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)
	err := root.Execute()
	return stdout, stderr, err
}

func writeEnterpriseSmokeConfig(t *testing.T, allowSecretAccess bool) string {
	t.Helper()

	secretPolicyPrincipal := "unbound.principal"
	if allowSecretAccess {
		secretPolicyPrincipal = "ops.admin"
	}

	content := `
schema_version: 1
mode: enterprise
default_org: agency
orgs:
  agency:
    id: org_1
    default_workspace: alpha
    workspaces:
      alpha:
        id: ws_1
roles:
  rotator:
    capabilities:
      - auth.rotate
bindings:
  - principal: ops.admin
    role: rotator
    org: agency
    workspace: alpha
secret_governance:
  secrets:
    auth_rotation_key:
      scope:
        org: agency
        workspace: alpha
      ownership:
        owner_principal: security.owner
  policies:
    - principal: ` + secretPolicyPrincipal + `
      secret: auth_rotation_key
      actions:
        - rotate
      org: agency
      workspace: alpha
`

	path := filepath.Join(t.TempDir(), "enterprise.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write enterprise smoke config: %v", err)
	}
	return path
}
