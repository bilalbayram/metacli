package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validProfile() Profile {
	return Profile{
		Domain:          "marketing",
		GraphVersion:    "v25.0",
		TokenType:       "system_user",
		AppID:           "1234567890",
		TokenRef:        "keychain://meta-marketing-cli/prod/token",
		AppSecretRef:    "keychain://meta-marketing-cli/prod/app-secret",
		AuthProvider:    "system_user",
		AuthMode:        "both",
		Scopes:          []string{"ads_read"},
		IssuedAt:        "2026-01-01T00:00:00Z",
		ExpiresAt:       "2026-12-31T00:00:00Z",
		LastValidatedAt: "2026-01-15T00:00:00Z",
		IGUserID:        "17841400000000000",
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := New()
	if err := cfg.UpsertProfile("prod", validProfile()); err != nil {
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
schema_version: 2
default_profile: prod
profiles:
  prod:
    domain: marketing
    graph_version: v25.0
    token_type: system_user
    app_id: "1234567890"
    token_ref: keychain://meta-marketing-cli/prod/token
    app_secret_ref: keychain://meta-marketing-cli/prod/app-secret
    auth_provider: system_user
    auth_mode: both
    scopes:
      - ads_read
    issued_at: 2026-01-01T00:00:00Z
    expires_at: 2026-12-31T00:00:00Z
    last_validated_at: 2026-01-15T00:00:00Z
    ig_user_id: "17841400000000000"
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

func TestLoadFailsOnPreviousSchemaVersion(t *testing.T) {
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
    app_id: "1234567890"
    token_ref: keychain://meta-marketing-cli/prod/token
    app_secret_ref: keychain://meta-marketing-cli/prod/app-secret
    auth_provider: system_user
    auth_mode: both
    scopes:
      - ads_read
    issued_at: 2026-01-01T00:00:00Z
    expires_at: 2026-12-31T00:00:00Z
    last_validated_at: 2026-01-15T00:00:00Z
`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected load to fail on previous schema version")
	}
	if !strings.Contains(err.Error(), "unsupported config schema_version=1") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpsertProfileValidationFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mutate   func(*Profile)
		wantText string
	}{
		{
			name: "token ref required",
			mutate: func(p *Profile) {
				p.TokenRef = ""
			},
			wantText: `token_ref is required`,
		},
		{
			name: "token type required",
			mutate: func(p *Profile) {
				p.TokenType = ""
			},
			wantText: `token_type is required`,
		},
		{
			name: "token type must be allowed",
			mutate: func(p *Profile) {
				p.TokenType = "invalid"
			},
			wantText: `token_type must be one of`,
		},
		{
			name: "app id required",
			mutate: func(p *Profile) {
				p.AppID = ""
			},
			wantText: `app_id is required`,
		},
		{
			name: "app secret ref required",
			mutate: func(p *Profile) {
				p.AppSecretRef = ""
			},
			wantText: `app_secret_ref is required`,
		},
		{
			name: "auth provider required",
			mutate: func(p *Profile) {
				p.AuthProvider = ""
			},
			wantText: `auth_provider is required`,
		},
		{
			name: "auth provider must be allowed",
			mutate: func(p *Profile) {
				p.AuthProvider = "invalid"
			},
			wantText: `auth_provider must be one of`,
		},
		{
			name: "auth mode required",
			mutate: func(p *Profile) {
				p.AuthMode = ""
			},
			wantText: `auth_mode is required`,
		},
		{
			name: "auth mode must be allowed",
			mutate: func(p *Profile) {
				p.AuthMode = "invalid"
			},
			wantText: `auth_mode must be one of`,
		},
		{
			name: "issued at must parse",
			mutate: func(p *Profile) {
				p.IssuedAt = "invalid"
			},
			wantText: `issued_at must be RFC3339`,
		},
		{
			name: "expires at must parse",
			mutate: func(p *Profile) {
				p.ExpiresAt = "invalid"
			},
			wantText: `expires_at must be RFC3339`,
		},
		{
			name: "last validated at must parse",
			mutate: func(p *Profile) {
				p.LastValidatedAt = "invalid"
			},
			wantText: `last_validated_at must be RFC3339`,
		},
		{
			name: "expires at must be after issued at",
			mutate: func(p *Profile) {
				p.ExpiresAt = p.IssuedAt
			},
			wantText: `expires_at must be after issued_at`,
		},
		{
			name: "scopes required",
			mutate: func(p *Profile) {
				p.Scopes = []string{}
			},
			wantText: `scopes must contain at least one scope`,
		},
		{
			name: "scopes cannot contain blanks",
			mutate: func(p *Profile) {
				p.Scopes = []string{"ads_read", " "}
			},
			wantText: `scopes contains blank entries`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := New()
			profile := validProfile()
			tc.mutate(&profile)

			err := cfg.UpsertProfile("prod", profile)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.wantText) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestUpsertProfileAllowsSupportedTokenTypes(t *testing.T) {
	t.Parallel()

	tokenTypes := []string{"system_user", "user", "page", "app"}
	for _, tokenType := range tokenTypes {
		tokenType := tokenType
		t.Run(tokenType, func(t *testing.T) {
			t.Parallel()

			cfg := New()
			profile := validProfile()
			profile.TokenType = tokenType

			if err := cfg.UpsertProfile("prod", profile); err != nil {
				t.Fatalf("upsert profile for token_type=%s: %v", tokenType, err)
			}
		})
	}
}
