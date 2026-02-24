package auth

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/bilalbayram/metacli/internal/config"
)

type inMemorySecretStore struct {
	values map[string]string
}

func newInMemorySecretStore() *inMemorySecretStore {
	return &inMemorySecretStore{
		values: map[string]string{},
	}
}

func (m *inMemorySecretStore) Set(ref string, value string) error {
	if ref == "" {
		return fmt.Errorf("secret ref is required")
	}
	if value == "" {
		return fmt.Errorf("secret value is required")
	}
	m.values[ref] = value
	return nil
}

func (m *inMemorySecretStore) Get(ref string) (string, error) {
	value, ok := m.values[ref]
	if !ok {
		return "", fmt.Errorf("secret not found: %s", ref)
	}
	return value, nil
}

func (m *inMemorySecretStore) Delete(ref string) error {
	delete(m.values, ref)
	return nil
}

func mustWriteConfigWithProfile(t *testing.T, profileName string, profile config.Profile) string {
	t.Helper()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	now := time.Now().UTC()
	if profile.Domain == "" {
		profile.Domain = config.DefaultDomain
	}
	if profile.GraphVersion == "" {
		profile.GraphVersion = config.DefaultGraphVersion
	}
	if profile.TokenType == "" {
		profile.TokenType = TokenTypeUser
	}
	if profile.TokenRef == "" {
		tokenRef, err := SecretRef(profileName, SecretToken)
		if err != nil {
			t.Fatalf("build token secret ref: %v", err)
		}
		profile.TokenRef = tokenRef
	}
	if profile.AppID == "" {
		profile.AppID = "app-123"
	}
	if profile.AppSecretRef == "" {
		appSecretRef, err := SecretRef(profileName, SecretAppSecret)
		if err != nil {
			t.Fatalf("build app secret ref: %v", err)
		}
		profile.AppSecretRef = appSecretRef
	}
	if profile.AuthProvider == "" {
		profile.AuthProvider = "facebook_login"
	}
	if profile.AuthMode == "" {
		profile.AuthMode = "both"
	}
	if len(profile.Scopes) == 0 {
		profile.Scopes = []string{"ads_read"}
	}
	if profile.IssuedAt == "" {
		profile.IssuedAt = now.Add(-1 * time.Hour).Format(time.RFC3339)
	}
	if profile.ExpiresAt == "" {
		profile.ExpiresAt = now.Add(24 * time.Hour).Format(time.RFC3339)
	}
	if profile.LastValidatedAt == "" {
		profile.LastValidatedAt = now.Format(time.RFC3339)
	}

	cfg := config.New()
	if err := cfg.UpsertProfile(profileName, profile); err != nil {
		t.Fatalf("upsert profile: %v", err)
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	return configPath
}
