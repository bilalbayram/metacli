package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bilalbayram/metacli/internal/config"
)

func TestEnsureValidPassesForValidToken(t *testing.T) {
	t.Parallel()

	svc := newPreflightService(t, fmt.Sprintf(`{"data":{"is_valid":true,"scopes":["ads_read","pages_show_list"],"expires_at":%d}}`, time.Now().UTC().Add(2*time.Hour).Unix()))
	meta, err := svc.EnsureValid(context.Background(), "prod", 30*time.Minute, []string{"ads_read"})
	if err != nil {
		t.Fatalf("ensure valid: %v", err)
	}
	if !meta.IsValid {
		t.Fatal("expected token metadata to be valid")
	}
}

func TestEnsureValidFailsForInvalidToken(t *testing.T) {
	t.Parallel()

	svc := newPreflightService(t, `{"data":{"is_valid":false,"scopes":["ads_read"],"expires_at":9999999999}}`)
	_, err := svc.EnsureValid(context.Background(), "prod", 0, []string{"ads_read"})
	if err == nil {
		t.Fatal("expected ensure valid to fail for invalid token")
	}
	if !strings.Contains(err.Error(), "profile token is invalid") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureValidFailsForExpiredToken(t *testing.T) {
	t.Parallel()

	svc := newPreflightService(t, fmt.Sprintf(`{"data":{"is_valid":true,"scopes":["ads_read"],"expires_at":%d}}`, time.Now().UTC().Add(-1*time.Minute).Unix()))
	_, err := svc.EnsureValid(context.Background(), "prod", 0, []string{"ads_read"})
	if err == nil {
		t.Fatal("expected ensure valid to fail for expired token")
	}
	if !strings.Contains(err.Error(), "profile token is expired") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureValidFailsForTTLBelowMinimum(t *testing.T) {
	t.Parallel()

	svc := newPreflightService(t, fmt.Sprintf(`{"data":{"is_valid":true,"scopes":["ads_read"],"expires_at":%d}}`, time.Now().UTC().Add(5*time.Minute).Unix()))
	_, err := svc.EnsureValid(context.Background(), "prod", 10*time.Minute, []string{"ads_read"})
	if err == nil {
		t.Fatal("expected ensure valid to fail for low ttl")
	}
	if !strings.Contains(err.Error(), "below minimum ttl") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureValidFailsWhenScopesAreMissing(t *testing.T) {
	t.Parallel()

	svc := newPreflightService(t, fmt.Sprintf(`{"data":{"is_valid":true,"scopes":["ads_read"],"expires_at":%d}}`, time.Now().UTC().Add(2*time.Hour).Unix()))
	_, err := svc.EnsureValid(context.Background(), "prod", 30*time.Minute, []string{"ads_read", "pages_manage_posts"})
	if err == nil {
		t.Fatal("expected ensure valid to fail for missing scopes")
	}
	if !strings.Contains(err.Error(), "missing required scopes") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func newPreflightService(t *testing.T, debugTokenPayload string) *Service {
	t.Helper()

	tokenRef, err := SecretRef("prod", SecretToken)
	if err != nil {
		t.Fatalf("token ref: %v", err)
	}
	appSecretRef, err := SecretRef("prod", SecretAppSecret)
	if err != nil {
		t.Fatalf("app secret ref: %v", err)
	}

	configPath := mustWriteConfigWithProfile(t, "prod", config.Profile{
		Domain:       config.DefaultDomain,
		GraphVersion: "v25.0",
		TokenType:    TokenTypeUser,
		AppID:        "app-123",
		TokenRef:     tokenRef,
		AppSecretRef: appSecretRef,
		AuthProvider: "facebook_login",
		AuthMode:     "both",
		Scopes:       []string{"ads_read"},
	})

	secrets := newInMemorySecretStore()
	if err := secrets.Set(tokenRef, "profile-token"); err != nil {
		t.Fatalf("set token secret: %v", err)
	}
	if err := secrets.Set(appSecretRef, "app-secret"); err != nil {
		t.Fatalf("set app secret: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v25.0/oauth/access_token":
			query := r.URL.Query()
			if query.Get("grant_type") != "client_credentials" {
				t.Fatalf("unexpected grant_type: %s", query.Get("grant_type"))
			}
			if query.Get("client_id") != "app-123" {
				t.Fatalf("unexpected client_id: %s", query.Get("client_id"))
			}
			if query.Get("client_secret") != "app-secret" {
				t.Fatalf("unexpected client_secret: %s", query.Get("client_secret"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"app-token"}`))
		case "/v25.0/debug_token":
			query := r.URL.Query()
			if query.Get("input_token") != "profile-token" {
				t.Fatalf("unexpected input token: %s", query.Get("input_token"))
			}
			if query.Get("access_token") != "app-token" {
				t.Fatalf("unexpected debug access token: %s", query.Get("access_token"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(debugTokenPayload))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	return NewService(configPath, secrets, server.Client(), server.URL)
}
