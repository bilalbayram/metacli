package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func writeEnterpriseConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "enterprise.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write enterprise config: %v", err)
	}
	return path
}
