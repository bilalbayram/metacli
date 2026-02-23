package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveAndLoadRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := New()
	if err := cfg.UpsertProfile("prod", Profile{
		Domain:       "marketing",
		GraphVersion: "v25.0",
		TokenType:    "system_user",
		TokenRef:     "keychain://meta-marketing-cli/prod/token",
	}); err != nil {
		t.Fatalf("upsert profile: %v", err)
	}

	if err := Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if loaded.SchemaVersion != SchemaVersion {
		t.Fatalf("unexpected schema version: got=%d want=%d", loaded.SchemaVersion, SchemaVersion)
	}
	if loaded.DefaultProfile != "prod" {
		t.Fatalf("unexpected default profile: got=%s", loaded.DefaultProfile)
	}
	if loaded.Profiles["prod"].TokenRef != "keychain://meta-marketing-cli/prod/token" {
		t.Fatalf("unexpected token ref: %s", loaded.Profiles["prod"].TokenRef)
	}
}

func TestLoadFailsOnUnknownField(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := `
schema_version: 1
default_profile: prod
profiles:
  prod:
    domain: marketing
    graph_version: v25.0
    token_type: system_user
    token_ref: keychain://meta-marketing-cli/prod/token
    unknown_field: should-fail
`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected load to fail on unknown field")
	}
	if !strings.Contains(err.Error(), "field unknown_field not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateFailsOnSchemaMismatch(t *testing.T) {
	t.Parallel()

	cfg := New()
	cfg.SchemaVersion = 99
	cfg.Profiles = map[string]Profile{}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected schema mismatch error")
	}
	if !strings.Contains(err.Error(), "unsupported config schema_version") {
		t.Fatalf("unexpected error: %v", err)
	}
}
