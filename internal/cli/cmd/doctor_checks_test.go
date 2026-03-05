package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bilalbayram/metacli/internal/auth"
	"github.com/bilalbayram/metacli/internal/config"
	"github.com/spf13/cobra"
)

// mockSecretStore is used only in doctor check tests.
type mockSecretStore struct {
	values map[string]string
	getErr error
}

func newMockSecretStore() *mockSecretStore {
	return &mockSecretStore{values: map[string]string{}}
}

func (m *mockSecretStore) Set(ref string, value string) error {
	m.values[ref] = value
	return nil
}

func (m *mockSecretStore) Get(ref string) (string, error) {
	if m.getErr != nil {
		return "", m.getErr
	}
	v, ok := m.values[ref]
	if !ok {
		return "", fmt.Errorf("secret not found: %s", ref)
	}
	return v, nil
}

func (m *mockSecretStore) Delete(ref string) error {
	delete(m.values, ref)
	return nil
}

func mustWriteDoctorConfig(t *testing.T, profileName string, expiresAt string) (string, *config.Config) {
	t.Helper()
	now := time.Now().UTC()
	return mustWriteDoctorConfigWithIssuedAt(t, profileName, now.Add(-1*time.Hour).Format(time.RFC3339), expiresAt)
}

func mustWriteDoctorConfigWithIssuedAt(t *testing.T, profileName string, issuedAt string, expiresAt string) (string, *config.Config) {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	now := time.Now().UTC()
	if expiresAt == "" {
		expiresAt = now.Add(24 * time.Hour).Format(time.RFC3339)
	}
	if issuedAt == "" {
		issuedAt = now.Add(-1 * time.Hour).Format(time.RFC3339)
	}

	tokenRef, err := auth.SecretRef(profileName, auth.SecretToken)
	if err != nil {
		t.Fatalf("build token ref: %v", err)
	}
	appSecretRef, err := auth.SecretRef(profileName, auth.SecretAppSecret)
	if err != nil {
		t.Fatalf("build app secret ref: %v", err)
	}

	cfg := config.New()
	if err := cfg.UpsertProfile(profileName, config.Profile{
		Domain:          config.DefaultDomain,
		GraphVersion:    config.DefaultGraphVersion,
		TokenType:       "user",
		AppID:           "app-123",
		TokenRef:        tokenRef,
		AppSecretRef:    appSecretRef,
		AuthProvider:    "facebook_login",
		AuthMode:        "both",
		Scopes:          []string{"ads_read"},
		IssuedAt:        issuedAt,
		ExpiresAt:       expiresAt,
		LastValidatedAt: now.Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("upsert profile: %v", err)
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	return configPath, cfg
}

func newDebugTokenServer(t *testing.T, valid bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle both the app token endpoint and debug_token endpoint.
		if strings.Contains(r.URL.Path, "oauth/access_token") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"access_token": "app-token-123"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"is_valid": valid,
			},
		})
	}))
}

func executeDoctorChecks(t *testing.T, runtime Runtime, deps *doctorDeps) map[string]any {
	t.Helper()
	output := &bytes.Buffer{}
	cmd := &cobra.Command{Use: "doctor"}
	cmd.SetOut(output)
	cmd.SetErr(io.Discard)

	if err := runDoctorChecks(cmd, runtime, deps); err != nil {
		t.Fatalf("runDoctorChecks: %v", err)
	}

	return decodeEnvelope(t, output.Bytes())
}

func TestDoctorChecksHealthyConfig(t *testing.T) {
	configPath, _ := mustWriteDoctorConfig(t, "prod", "")

	store := newMockSecretStore()
	tokenRef, _ := auth.SecretRef("prod", auth.SecretToken)
	appSecretRef, _ := auth.SecretRef("prod", auth.SecretAppSecret)
	store.values[tokenRef] = "tok-123"
	store.values[appSecretRef] = "secret-123"

	server := newDebugTokenServer(t, true)
	defer server.Close()

	deps := &doctorDeps{
		configPath:   configPath,
		secretStore:  store,
		httpClient:   server.Client(),
		graphBaseURL: server.URL,
	}

	envelope := executeDoctorChecks(t, Runtime{Output: stringPtr("json")}, deps)
	assertEnvelopeBasics(t, envelope, "meta doctor")

	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %T", envelope["data"])
	}
	if got := data["status"]; got != "healthy" {
		t.Fatalf("expected healthy, got %v", got)
	}

	summary, ok := data["summary"].(map[string]any)
	if !ok {
		t.Fatalf("expected summary object, got %T", data["summary"])
	}
	if fail := summary["fail"].(float64); fail != 0 {
		t.Fatalf("expected 0 fails, got %v", fail)
	}
	if warn := summary["warn"].(float64); warn != 0 {
		t.Fatalf("expected 0 warns, got %v", warn)
	}
}

func TestDoctorChecksConfigMissing(t *testing.T) {
	deps := &doctorDeps{
		configPath:  "/nonexistent/config.yaml",
		secretStore: newMockSecretStore(),
	}

	envelope := executeDoctorChecks(t, Runtime{Output: stringPtr("json")}, deps)
	assertEnvelopeBasics(t, envelope, "meta doctor")

	data := envelope["data"].(map[string]any)
	if got := data["status"]; got != "unhealthy" {
		t.Fatalf("expected unhealthy, got %v", got)
	}

	checks := data["checks"].([]any)
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}
	check := checks[0].(map[string]any)
	if check["name"] != "config_file" {
		t.Fatalf("expected config_file check, got %v", check["name"])
	}
	if check["status"] != "fail" {
		t.Fatalf("expected fail status, got %v", check["status"])
	}
}

func TestDoctorChecksExpiredToken(t *testing.T) {
	// Token that was valid but has since expired: issued 48h ago, expired 1h ago.
	expired := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	configPath, _ := mustWriteDoctorConfigWithIssuedAt(t, "prod", time.Now().UTC().Add(-48*time.Hour).Format(time.RFC3339), expired)

	store := newMockSecretStore()
	tokenRef, _ := auth.SecretRef("prod", auth.SecretToken)
	appSecretRef, _ := auth.SecretRef("prod", auth.SecretAppSecret)
	store.values[tokenRef] = "tok-123"
	store.values[appSecretRef] = "secret-123"

	server := newDebugTokenServer(t, true)
	defer server.Close()

	deps := &doctorDeps{
		configPath:   configPath,
		secretStore:  store,
		httpClient:   server.Client(),
		graphBaseURL: server.URL,
	}

	envelope := executeDoctorChecks(t, Runtime{Output: stringPtr("json")}, deps)
	data := envelope["data"].(map[string]any)

	// Should be at least "degraded" due to expired token warning.
	status := data["status"].(string)
	if status != "degraded" && status != "unhealthy" {
		t.Fatalf("expected degraded or unhealthy, got %v", status)
	}

	// Find the profile_completeness warn check.
	checks := data["checks"].([]any)
	found := false
	for _, raw := range checks {
		c := raw.(map[string]any)
		if c["name"] == "profile_completeness" && c["status"] == "warn" {
			found = true
			msg := c["message"].(string)
			if !strings.Contains(msg, "token expired") {
				t.Fatalf("expected expired message, got %q", msg)
			}
		}
	}
	if !found {
		t.Fatal("expected profile_completeness warn check for expired token")
	}
}

func TestDoctorChecksSecretStoreFail(t *testing.T) {
	configPath, _ := mustWriteDoctorConfig(t, "prod", "")

	store := newMockSecretStore()
	store.getErr = fmt.Errorf("keychain unavailable")

	deps := &doctorDeps{
		configPath:  configPath,
		secretStore: store,
	}

	envelope := executeDoctorChecks(t, Runtime{Output: stringPtr("json")}, deps)
	data := envelope["data"].(map[string]any)

	if got := data["status"]; got != "unhealthy" {
		t.Fatalf("expected unhealthy, got %v", got)
	}

	checks := data["checks"].([]any)
	foundSecret := false
	for _, raw := range checks {
		c := raw.(map[string]any)
		if c["name"] == "secret_store" && c["status"] == "fail" {
			foundSecret = true
		}
	}
	if !foundSecret {
		t.Fatal("expected secret_store fail check")
	}
}

func TestDoctorChecksProfileFilter(t *testing.T) {
	// Create config with two profiles.
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	now := time.Now().UTC()
	cfg := config.New()
	for _, name := range []string{"alpha", "beta"} {
		tokenRef, _ := auth.SecretRef(name, auth.SecretToken)
		appSecretRef, _ := auth.SecretRef(name, auth.SecretAppSecret)
		cfg.UpsertProfile(name, config.Profile{
			Domain:          config.DefaultDomain,
			GraphVersion:    config.DefaultGraphVersion,
			TokenType:       "user",
			AppID:           "app-" + name,
			TokenRef:        tokenRef,
			AppSecretRef:    appSecretRef,
			AuthProvider:    "facebook_login",
			AuthMode:        "both",
			Scopes:          []string{"ads_read"},
			IssuedAt:        now.Add(-1 * time.Hour).Format(time.RFC3339),
			ExpiresAt:       now.Add(24 * time.Hour).Format(time.RFC3339),
			LastValidatedAt: now.Format(time.RFC3339),
		})
	}
	config.Save(configPath, cfg)

	store := newMockSecretStore()
	for _, name := range []string{"alpha", "beta"} {
		tokenRef, _ := auth.SecretRef(name, auth.SecretToken)
		appSecretRef, _ := auth.SecretRef(name, auth.SecretAppSecret)
		store.values[tokenRef] = "tok-" + name
		store.values[appSecretRef] = "secret-" + name
	}

	server := newDebugTokenServer(t, true)
	defer server.Close()

	profile := "alpha"
	deps := &doctorDeps{
		configPath:   configPath,
		secretStore:  store,
		httpClient:   server.Client(),
		graphBaseURL: server.URL,
	}

	envelope := executeDoctorChecks(t, Runtime{Profile: &profile, Output: stringPtr("json")}, deps)
	data := envelope["data"].(map[string]any)

	// All profile-scoped checks should only reference "alpha".
	checks := data["checks"].([]any)
	for _, raw := range checks {
		c := raw.(map[string]any)
		if p, ok := c["profile"].(string); ok && p != "" {
			if p != "alpha" {
				t.Fatalf("expected only alpha profile checks, got %q", p)
			}
		}
	}
}

func TestDoctorTracerStillWorks(t *testing.T) {
	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewDoctorCommand(Runtime{Output: stringPtr("json")})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{"tracer"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute doctor tracer: %v", err)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta doctor tracer")
	data := envelope["data"].(map[string]any)
	if got := data["status"]; got != "ok" {
		t.Fatalf("expected ok, got %v", got)
	}
}
