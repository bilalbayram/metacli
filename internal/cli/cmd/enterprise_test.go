package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bilalbayram/metacli/internal/enterprise"
)

func TestEnterpriseContextResolvesWorkspace(t *testing.T) {
	t.Parallel()

	configPath := writeEnterpriseConfig(t, `
schema_version: 1
default_org: acme
orgs:
  acme:
    id: org_1
    default_workspace: prod
    workspaces:
      prod:
        id: ws_1
      growth:
        id: ws_2
`)

	cmd := newEnterpriseContextCommand(Runtime{})
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{"--config", configPath, "--workspace", "acme/growth"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute command: %v", err)
	}

	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			OrgName       string `json:"org_name"`
			OrgID         string `json:"org_id"`
			WorkspaceName string `json:"workspace_name"`
			WorkspaceID   string `json:"workspace_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v", err)
	}

	if !envelope.Success {
		t.Fatal("expected success output")
	}
	if envelope.Data.OrgName != "acme" {
		t.Fatalf("unexpected org name: %q", envelope.Data.OrgName)
	}
	if envelope.Data.WorkspaceName != "growth" {
		t.Fatalf("unexpected workspace name: %q", envelope.Data.WorkspaceName)
	}
	if envelope.Data.WorkspaceID != "ws_2" {
		t.Fatalf("unexpected workspace id: %q", envelope.Data.WorkspaceID)
	}
}

func TestEnterpriseContextFailsWhenWorkspaceMissing(t *testing.T) {
	t.Parallel()

	configPath := writeEnterpriseConfig(t, `
schema_version: 1
default_org: acme
orgs:
  acme:
    id: org_1
    workspaces:
      prod:
        id: ws_1
`)

	cmd := newEnterpriseContextCommand(Runtime{})
	cmd.SetOut(bytes.NewBuffer(nil))
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{"--config", configPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command to fail when workspace is missing")
	}
	if !strings.Contains(err.Error(), "workspace is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnterpriseContextFailsWhenWorkspaceInvalid(t *testing.T) {
	t.Parallel()

	configPath := writeEnterpriseConfig(t, `
schema_version: 1
default_org: acme
orgs:
  acme:
    id: org_1
    default_workspace: prod
    workspaces:
      prod:
        id: ws_1
`)

	cmd := newEnterpriseContextCommand(Runtime{})
	cmd.SetOut(bytes.NewBuffer(nil))
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{"--config", configPath, "--workspace", "unknown"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command to fail when workspace is invalid")
	}
	if !strings.Contains(err.Error(), "workspace \"unknown\" does not exist") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnterpriseAuthzCheckAllowsCommand(t *testing.T) {
	t.Parallel()

	configPath := writeEnterpriseConfig(t, `
schema_version: 1
default_org: acme
orgs:
  acme:
    id: org_1
    default_workspace: prod
    workspaces:
      prod:
        id: ws_1
roles:
  reader:
    capabilities:
      - graph.read
bindings:
  - principal: alice
    role: reader
    org: acme
    workspace: prod
`)

	cmd := newEnterpriseAuthzCheckCommand(Runtime{})
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{
		"--config", configPath,
		"--principal", "alice",
		"--command", "meta api get",
		"--workspace", "acme/prod",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute command: %v", err)
	}

	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Allowed            bool   `json:"allowed"`
			RequiredCapability string `json:"required_capability"`
			OrgName            string `json:"org_name"`
			WorkspaceName      string `json:"workspace_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !envelope.Success {
		t.Fatal("expected success output")
	}
	if !envelope.Data.Allowed {
		t.Fatal("expected allowed authorization result")
	}
	if envelope.Data.RequiredCapability != "graph.read" {
		t.Fatalf("unexpected required capability: %q", envelope.Data.RequiredCapability)
	}
	if envelope.Data.OrgName != "acme" {
		t.Fatalf("unexpected org name: %q", envelope.Data.OrgName)
	}
	if envelope.Data.WorkspaceName != "prod" {
		t.Fatalf("unexpected workspace name: %q", envelope.Data.WorkspaceName)
	}
}

func TestEnterpriseAuthzCheckDeniesCommand(t *testing.T) {
	t.Parallel()

	configPath := writeEnterpriseConfig(t, `
schema_version: 1
default_org: acme
orgs:
  acme:
    id: org_1
    default_workspace: prod
    workspaces:
      prod:
        id: ws_1
roles:
  reader:
    capabilities:
      - graph.read
bindings:
  - principal: alice
    role: reader
    org: acme
    workspace: prod
`)

	cmd := newEnterpriseAuthzCheckCommand(Runtime{})
	cmd.SetOut(bytes.NewBuffer(nil))
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{
		"--config", configPath,
		"--principal", "alice",
		"--command", "api post",
		"--workspace", "prod",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command to fail for denied authorization")
	}
	if !strings.Contains(err.Error(), "authorization denied") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnterpriseAuthzCheckRecordsDecisionAndExecutionAuditEvents(t *testing.T) {
	t.Parallel()

	configPath := writeEnterpriseConfig(t, `
schema_version: 1
default_org: acme
orgs:
  acme:
    id: org_1
    default_workspace: prod
    workspaces:
      prod:
        id: ws_1
roles:
  reader:
    capabilities:
      - graph.read
bindings:
  - principal: alice
    role: reader
    org: acme
    workspace: prod
`)

	cmd := newEnterpriseAuthzCheckCommand(Runtime{})
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{
		"--config", configPath,
		"--principal", "alice",
		"--command", "meta api get",
		"--workspace", "acme/prod",
		"--correlation-id", "corr-cli-001",
		"--execution-status", "succeeded",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute command: %v", err)
	}

	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Allowed       bool   `json:"allowed"`
			CorrelationID string `json:"correlation_id"`
			AuditEvents   []struct {
				EventType       string `json:"event_type"`
				CorrelationID   string `json:"correlation_id"`
				ExecutionStatus string `json:"execution_status"`
				PreviousDigest  string `json:"previous_digest"`
				Digest          string `json:"digest"`
				Allowed         *bool  `json:"allowed"`
			} `json:"audit_events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !envelope.Success {
		t.Fatal("expected success output")
	}
	if !envelope.Data.Allowed {
		t.Fatal("expected allowed authorization result")
	}
	if envelope.Data.CorrelationID != "corr-cli-001" {
		t.Fatalf("unexpected correlation id: %q", envelope.Data.CorrelationID)
	}
	if len(envelope.Data.AuditEvents) != 2 {
		t.Fatalf("unexpected audit event count: %d", len(envelope.Data.AuditEvents))
	}
	if envelope.Data.AuditEvents[0].EventType != "decision" {
		t.Fatalf("unexpected first event type: %q", envelope.Data.AuditEvents[0].EventType)
	}
	if envelope.Data.AuditEvents[0].Allowed == nil || !*envelope.Data.AuditEvents[0].Allowed {
		t.Fatalf("expected decision allowed=true, got %v", envelope.Data.AuditEvents[0].Allowed)
	}
	if envelope.Data.AuditEvents[1].EventType != "execution" {
		t.Fatalf("unexpected second event type: %q", envelope.Data.AuditEvents[1].EventType)
	}
	if envelope.Data.AuditEvents[1].ExecutionStatus != "succeeded" {
		t.Fatalf("unexpected execution status: %q", envelope.Data.AuditEvents[1].ExecutionStatus)
	}
	if envelope.Data.AuditEvents[1].PreviousDigest != envelope.Data.AuditEvents[0].Digest {
		t.Fatalf(
			"unexpected digest chain: second.previous=%q first.digest=%q",
			envelope.Data.AuditEvents[1].PreviousDigest,
			envelope.Data.AuditEvents[0].Digest,
		)
	}
}

func TestEnterpriseAuthzCheckFailsWhenExecutionStatusMissingCorrelationID(t *testing.T) {
	t.Parallel()

	configPath := writeEnterpriseConfig(t, `
schema_version: 1
default_org: acme
orgs:
  acme:
    id: org_1
    default_workspace: prod
    workspaces:
      prod:
        id: ws_1
roles:
  reader:
    capabilities:
      - graph.read
bindings:
  - principal: alice
    role: reader
    org: acme
    workspace: prod
`)

	cmd := newEnterpriseAuthzCheckCommand(Runtime{})
	cmd.SetOut(bytes.NewBuffer(nil))
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{
		"--config", configPath,
		"--principal", "alice",
		"--command", "meta api get",
		"--workspace", "acme/prod",
		"--execution-status", "succeeded",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command to fail when correlation id is missing")
	}
	if !strings.Contains(err.Error(), "correlation_id is required when audit pipeline is configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnterpriseAuthzCheckFailsWhenExecutionErrorProvidedWithoutStatus(t *testing.T) {
	t.Parallel()

	configPath := writeEnterpriseConfig(t, `
schema_version: 1
default_org: acme
orgs:
  acme:
    id: org_1
    default_workspace: prod
    workspaces:
      prod:
        id: ws_1
roles:
  reader:
    capabilities:
      - graph.read
bindings:
  - principal: alice
    role: reader
    org: acme
    workspace: prod
`)

	cmd := newEnterpriseAuthzCheckCommand(Runtime{})
	cmd.SetOut(bytes.NewBuffer(nil))
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{
		"--config", configPath,
		"--principal", "alice",
		"--command", "meta api get",
		"--workspace", "acme/prod",
		"--correlation-id", "corr-cli-001",
		"--execution-error", "request timeout",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command to fail when execution status is missing")
	}
	if !strings.Contains(err.Error(), "execution status is required when execution error is provided") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnterpriseApprovalRequestApproveValidateFlow(t *testing.T) {
	t.Parallel()

	configPath := writeEnterpriseConfig(t, `
schema_version: 1
default_org: acme
orgs:
  acme:
    id: org_1
    default_workspace: prod
    workspaces:
      prod:
        id: ws_1
`)

	requestCmd := newEnterpriseApprovalRequestCommand(Runtime{})
	requestStdout := &bytes.Buffer{}
	requestCmd.SetOut(requestStdout)
	requestCmd.SetErr(bytes.NewBuffer(nil))
	requestCmd.SetArgs([]string{
		"--config", configPath,
		"--principal", "alice",
		"--command", "auth rotate",
		"--workspace", "acme/prod",
		"--ttl", "30m",
	})
	if err := requestCmd.Execute(); err != nil {
		t.Fatalf("execute request command: %v", err)
	}

	var requestEnvelope struct {
		Success bool `json:"success"`
		Data    struct {
			RequestToken string `json:"request_token"`
			Fingerprint  string `json:"fingerprint"`
		} `json:"data"`
	}
	if err := json.Unmarshal(requestStdout.Bytes(), &requestEnvelope); err != nil {
		t.Fatalf("decode request output: %v", err)
	}
	if !requestEnvelope.Success {
		t.Fatal("expected request command success output")
	}
	if strings.TrimSpace(requestEnvelope.Data.RequestToken) == "" {
		t.Fatal("expected request token output")
	}
	if strings.TrimSpace(requestEnvelope.Data.Fingerprint) == "" {
		t.Fatal("expected request fingerprint output")
	}

	approveCmd := newEnterpriseApprovalApproveCommand(Runtime{})
	approveStdout := &bytes.Buffer{}
	approveCmd.SetOut(approveStdout)
	approveCmd.SetErr(bytes.NewBuffer(nil))
	approveCmd.SetArgs([]string{
		"--request-token", requestEnvelope.Data.RequestToken,
		"--approver", "security.lead",
		"--decision", "approved",
		"--ttl", "30m",
	})
	if err := approveCmd.Execute(); err != nil {
		t.Fatalf("execute approve command: %v", err)
	}

	var approveEnvelope struct {
		Success bool `json:"success"`
		Data    struct {
			GrantToken  string `json:"grant_token"`
			Fingerprint string `json:"fingerprint"`
			Decision    string `json:"decision"`
		} `json:"data"`
	}
	if err := json.Unmarshal(approveStdout.Bytes(), &approveEnvelope); err != nil {
		t.Fatalf("decode approve output: %v", err)
	}
	if !approveEnvelope.Success {
		t.Fatal("expected approve command success output")
	}
	if strings.TrimSpace(approveEnvelope.Data.GrantToken) == "" {
		t.Fatal("expected grant token output")
	}
	if approveEnvelope.Data.Decision != "approved" {
		t.Fatalf("unexpected decision: %q", approveEnvelope.Data.Decision)
	}
	if approveEnvelope.Data.Fingerprint != requestEnvelope.Data.Fingerprint {
		t.Fatalf(
			"unexpected fingerprint mismatch: request=%q approve=%q",
			requestEnvelope.Data.Fingerprint,
			approveEnvelope.Data.Fingerprint,
		)
	}

	validateCmd := newEnterpriseApprovalValidateCommand(Runtime{})
	validateStdout := &bytes.Buffer{}
	validateCmd.SetOut(validateStdout)
	validateCmd.SetErr(bytes.NewBuffer(nil))
	validateCmd.SetArgs([]string{
		"--config", configPath,
		"--grant-token", approveEnvelope.Data.GrantToken,
		"--principal", "alice",
		"--command", "auth rotate",
		"--workspace", "acme/prod",
	})
	if err := validateCmd.Execute(); err != nil {
		t.Fatalf("execute validate command: %v", err)
	}

	var validateEnvelope struct {
		Success bool `json:"success"`
		Data    struct {
			Valid       bool   `json:"valid"`
			Status      string `json:"status"`
			Fingerprint string `json:"fingerprint"`
		} `json:"data"`
	}
	if err := json.Unmarshal(validateStdout.Bytes(), &validateEnvelope); err != nil {
		t.Fatalf("decode validate output: %v", err)
	}
	if !validateEnvelope.Success {
		t.Fatal("expected validate command success output")
	}
	if !validateEnvelope.Data.Valid {
		t.Fatal("expected validate command to return valid=true")
	}
	if validateEnvelope.Data.Status != "approved" {
		t.Fatalf("unexpected validate status: %q", validateEnvelope.Data.Status)
	}
	if validateEnvelope.Data.Fingerprint != requestEnvelope.Data.Fingerprint {
		t.Fatalf(
			"unexpected validation fingerprint mismatch: request=%q validate=%q",
			requestEnvelope.Data.Fingerprint,
			validateEnvelope.Data.Fingerprint,
		)
	}
}

func TestEnterpriseAuthzCheckAllowsHighRiskCommandWithApprovalToken(t *testing.T) {
	t.Parallel()

	configPath := writeEnterpriseConfig(t, `
schema_version: 1
default_org: acme
orgs:
  acme:
    id: org_1
    default_workspace: prod
    workspaces:
      prod:
        id: ws_1
roles:
  rotator:
    capabilities:
      - auth.rotate
bindings:
  - principal: alice
    role: rotator
    org: acme
    workspace: prod
`)

	now := time.Now().UTC()
	requestToken, err := enterprise.CreateApprovalRequestToken(enterprise.ApprovalRequestTokenRequest{
		Principal:     "alice",
		Command:       "auth rotate",
		OrgName:       "acme",
		WorkspaceName: "prod",
		TTL:           20 * time.Minute,
		Now: func() time.Time {
			return now
		},
	})
	if err != nil {
		t.Fatalf("create approval request token: %v", err)
	}
	grantToken, err := enterprise.CreateApprovalGrantToken(enterprise.ApprovalGrantTokenRequest{
		RequestToken: requestToken.RequestToken,
		Approver:     "security.lead",
		Decision:     enterprise.ApprovalDecisionApproved,
		TTL:          20 * time.Minute,
		Now: func() time.Time {
			return now
		},
	})
	if err != nil {
		t.Fatalf("create approval grant token: %v", err)
	}

	cmd := newEnterpriseAuthzCheckCommand(Runtime{})
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{
		"--config", configPath,
		"--principal", "alice",
		"--command", "auth rotate",
		"--workspace", "acme/prod",
		"--approval-token", grantToken.GrantToken,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute command: %v", err)
	}

	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Allowed          bool   `json:"allowed"`
			ApprovalRequired bool   `json:"approval_required"`
			ApprovalStatus   string `json:"approval_status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !envelope.Success {
		t.Fatal("expected success output")
	}
	if !envelope.Data.Allowed {
		t.Fatal("expected high-risk authorization result to be allowed")
	}
	if !envelope.Data.ApprovalRequired {
		t.Fatal("expected approval_required=true")
	}
	if envelope.Data.ApprovalStatus != "approved" {
		t.Fatalf("unexpected approval status: %q", envelope.Data.ApprovalStatus)
	}
}

func TestEnterprisePolicyEvalReturnsTrace(t *testing.T) {
	t.Parallel()

	configPath := writeEnterpriseConfig(t, `
schema_version: 1
default_org: acme
orgs:
  acme:
    id: org_1
    default_workspace: prod
    workspaces:
      prod:
        id: ws_1
roles:
  reader:
    capabilities:
      - graph.read
bindings:
  - principal: alice
    role: reader
    org: acme
    workspace: prod
`)

	cmd := newEnterprisePolicyEvalCommand(Runtime{})
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{
		"--config", configPath,
		"--principal", "alice",
		"--capability", "graph.read",
		"--workspace", "acme/prod",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute command: %v", err)
	}

	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Allowed       bool `json:"allowed"`
			DecisionTrace []struct {
				Effect     string `json:"effect"`
				Matched    bool   `json:"matched"`
				Capability string `json:"capability"`
			} `json:"decision_trace"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !envelope.Success {
		t.Fatal("expected success output")
	}
	if !envelope.Data.Allowed {
		t.Fatal("expected allowed policy evaluation result")
	}
	if len(envelope.Data.DecisionTrace) != 2 {
		t.Fatalf("unexpected decision trace length: %d", len(envelope.Data.DecisionTrace))
	}
	if envelope.Data.DecisionTrace[0].Effect != "deny" || envelope.Data.DecisionTrace[0].Matched {
		t.Fatalf("unexpected decision trace[0]: %+v", envelope.Data.DecisionTrace[0])
	}
	if envelope.Data.DecisionTrace[1].Effect != "allow" || !envelope.Data.DecisionTrace[1].Matched {
		t.Fatalf("unexpected decision trace[1]: %+v", envelope.Data.DecisionTrace[1])
	}
}

func writeEnterpriseConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "enterprise.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write enterprise config: %v", err)
	}
	return path
}
