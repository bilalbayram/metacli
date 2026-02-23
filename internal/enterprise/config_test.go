package enterprise

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFailsOnUnknownField(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "enterprise.yaml")
	raw := `
schema_version: 1
mode: enterprise
default_org: acme
orgs:
  acme:
    id: org_1
    default_workspace: prod
    workspaces:
      prod:
        id: ws_1
        unknown_field: should-fail
`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected load failure for unknown field")
	}
	if !strings.Contains(err.Error(), "field unknown_field not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateFailsOnSchemaMismatch(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.SchemaVersion = 99

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected schema mismatch error")
	}
	if !strings.Contains(err.Error(), "unsupported enterprise schema_version") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateFailsWhenModeUnsupported(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Mode = "legacy"

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected unsupported mode error")
	}
	if !strings.Contains(err.Error(), "unsupported enterprise mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveWorkspaceUsesDefaults(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	ctx, err := cfg.ResolveWorkspace("", "")
	if err != nil {
		t.Fatalf("resolve workspace: %v", err)
	}

	if ctx.OrgName != "acme" {
		t.Fatalf("unexpected org name: %q", ctx.OrgName)
	}
	if ctx.WorkspaceName != "prod" {
		t.Fatalf("unexpected workspace name: %q", ctx.WorkspaceName)
	}
	if ctx.WorkspaceID != "ws_1" {
		t.Fatalf("unexpected workspace id: %q", ctx.WorkspaceID)
	}
}

func TestResolveWorkspaceFailsForMissingWorkspace(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		SchemaVersion: SchemaVersion,
		Mode:          EnterpriseMode,
		DefaultOrg:    "acme",
		Orgs: map[string]Org{
			"acme": {
				ID: "org_1",
				Workspaces: map[string]Workspace{
					"prod": {ID: "ws_1"},
				},
			},
		},
	}

	_, err := cfg.ResolveWorkspace("", "")
	if err == nil {
		t.Fatal("expected workspace required error")
	}
	if !strings.Contains(err.Error(), "workspace is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveWorkspaceFailsForUnknownWorkspace(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	_, err := cfg.ResolveWorkspace("acme", "missing")
	if err == nil {
		t.Fatal("expected unknown workspace error")
	}
	if !strings.Contains(err.Error(), "workspace \"missing\" does not exist in org \"acme\"") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func validConfig() *Config {
	return &Config{
		SchemaVersion: SchemaVersion,
		Mode:          EnterpriseMode,
		DefaultOrg:    "acme",
		Orgs: map[string]Org{
			"acme": {
				ID:               "org_1",
				DefaultWorkspace: "prod",
				Workspaces: map[string]Workspace{
					"prod":   {ID: "ws_1"},
					"growth": {ID: "ws_2"},
				},
			},
		},
	}
}
