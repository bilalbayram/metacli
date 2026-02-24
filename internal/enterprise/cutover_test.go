package enterprise

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFailsWithLegacyConfigMigrationError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(validLegacyConfigFixture()), 0o600); err != nil {
		t.Fatalf("write legacy fixture: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected legacy config to fail enterprise load")
	}
	if !strings.Contains(err.Error(), "legacy CLI config detected") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "meta enterprise mode cutover") {
		t.Fatalf("unexpected migration hint: %v", err)
	}
}

func TestValidateFailsWhenEnterpriseModeMissing(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Mode = ""

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected mode missing validation error")
	}
	if !strings.Contains(err.Error(), "enterprise mode is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCutoverLegacyConfigCreatesEnterpriseConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "config.yaml")
	enterprisePath := filepath.Join(dir, "enterprise.yaml")

	if err := os.WriteFile(legacyPath, []byte(validLegacyConfigFixture()), 0o600); err != nil {
		t.Fatalf("write legacy fixture: %v", err)
	}

	result, err := CutoverLegacyConfig(ModeCutoverRequest{
		LegacyConfigPath:     legacyPath,
		EnterpriseConfigPath: enterprisePath,
		OrgName:              "agency",
		OrgID:                "org_01",
		WorkspaceName:        "client_alpha",
		WorkspaceID:          "ws_01",
		Principal:            "ops.admin",
	})
	if err != nil {
		t.Fatalf("cutover legacy config: %v", err)
	}

	if result.BootstrapRole != cutoverBootstrapRole {
		t.Fatalf("unexpected bootstrap role: %q", result.BootstrapRole)
	}
	if result.BootstrapPrincipal != "ops.admin" {
		t.Fatalf("unexpected bootstrap principal: %q", result.BootstrapPrincipal)
	}
	if len(result.MigratedProfiles) != 1 || result.MigratedProfiles[0] != "prod" {
		t.Fatalf("unexpected migrated profiles: %+v", result.MigratedProfiles)
	}

	enterpriseCfg, err := Load(enterprisePath)
	if err != nil {
		t.Fatalf("load migrated enterprise config: %v", err)
	}
	if enterpriseCfg.Mode != EnterpriseMode {
		t.Fatalf("unexpected enterprise mode: %q", enterpriseCfg.Mode)
	}
	if _, ok := enterpriseCfg.Roles[cutoverBootstrapRole]; !ok {
		t.Fatalf("expected bootstrap role %q to exist", cutoverBootstrapRole)
	}
	if len(enterpriseCfg.Roles[cutoverBootstrapRole].Capabilities) == 0 {
		t.Fatalf("expected bootstrap role capabilities to be populated")
	}
	if len(enterpriseCfg.Bindings) != 1 {
		t.Fatalf("unexpected bootstrap binding count: %d", len(enterpriseCfg.Bindings))
	}
	if enterpriseCfg.Bindings[0].Principal != "ops.admin" {
		t.Fatalf("unexpected bootstrap binding principal: %q", enterpriseCfg.Bindings[0].Principal)
	}
}

func TestCutoverLegacyConfigFailsWhenOutputExistsWithoutForce(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "config.yaml")
	enterprisePath := filepath.Join(dir, "enterprise.yaml")

	if err := os.WriteFile(legacyPath, []byte(validLegacyConfigFixture()), 0o600); err != nil {
		t.Fatalf("write legacy fixture: %v", err)
	}
	if err := os.WriteFile(enterprisePath, []byte("occupied"), 0o600); err != nil {
		t.Fatalf("write occupied enterprise path: %v", err)
	}

	_, err := CutoverLegacyConfig(ModeCutoverRequest{
		LegacyConfigPath:     legacyPath,
		EnterpriseConfigPath: enterprisePath,
		OrgName:              "agency",
		OrgID:                "org_01",
		WorkspaceName:        "client_alpha",
		WorkspaceID:          "ws_01",
		Principal:            "ops.admin",
	})
	if err == nil {
		t.Fatal("expected existing enterprise config to fail cutover")
	}
	if !strings.Contains(err.Error(), "enterprise config already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func validLegacyConfigFixture() string {
	return `
schema_version: 2
default_profile: prod
profiles:
  prod:
    domain: marketing
    graph_version: v25.0
    token_type: system_user
    token_ref: keychain://meta-marketing-cli/prod/token
    app_id: app_123
    app_secret_ref: keychain://meta-marketing-cli/prod/app_secret
    auth_provider: system_user
    auth_mode: both
    scopes:
      - ads_management
      - business_management
    issued_at: "2026-01-15T00:00:00Z"
    expires_at: "2026-12-31T23:59:59Z"
    last_validated_at: "2026-01-16T00:00:00Z"
`
}
