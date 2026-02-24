package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGenerateOAuthState(t *testing.T) {
	t.Parallel()

	stateA, err := GenerateOAuthState()
	if err != nil {
		t.Fatalf("generate oauth state A: %v", err)
	}
	stateB, err := GenerateOAuthState()
	if err != nil {
		t.Fatalf("generate oauth state B: %v", err)
	}

	if stateA == "" || stateB == "" {
		t.Fatal("generated oauth state must not be empty")
	}
	if stateA == stateB {
		t.Fatal("generated oauth state should be random and unique")
	}
}

func TestNormalizeDebugTokenMetadata(t *testing.T) {
	t.Parallel()

	exp := time.Now().UTC().Add(2 * time.Hour).Unix()
	meta, err := NormalizeDebugTokenMetadata(&DebugTokenResponse{
		Data: map[string]any{
			"is_valid":   true,
			"scopes":     []any{"ads_read", "pages_show_list"},
			"expires_at": float64(exp),
		},
	})
	if err != nil {
		t.Fatalf("normalize debug token: %v", err)
	}
	if !meta.IsValid {
		t.Fatal("metadata should be valid")
	}
	if len(meta.Scopes) != 2 {
		t.Fatalf("unexpected scopes length: %d", len(meta.Scopes))
	}
	if meta.ExpiresAt.Unix() != exp {
		t.Fatalf("unexpected expires_at: got=%d want=%d", meta.ExpiresAt.Unix(), exp)
	}
}

func TestNormalizeDebugTokenMetadataRejectsInvalidShape(t *testing.T) {
	t.Parallel()

	_, err := NormalizeDebugTokenMetadata(&DebugTokenResponse{
		Data: map[string]any{
			"is_valid": "yes",
		},
	})
	if err == nil {
		t.Fatal("expected normalize failure for invalid debug token shape")
	}
}

func TestExchangeLongLivedUserToken(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v25.0/oauth/access_token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("grant_type") != "fb_exchange_token" {
			t.Fatalf("unexpected grant_type: %s", query.Get("grant_type"))
		}
		if query.Get("client_id") != "app-123" {
			t.Fatalf("unexpected client_id: %s", query.Get("client_id"))
		}
		if query.Get("client_secret") != "secret-123" {
			t.Fatalf("unexpected client_secret: %s", query.Get("client_secret"))
		}
		if query.Get("fb_exchange_token") != "short-token" {
			t.Fatalf("unexpected fb_exchange_token: %s", query.Get("fb_exchange_token"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"long-token","expires_in":5183944}`))
	}))
	defer server.Close()

	svc := NewService("", nil, server.Client(), server.URL)
	result, err := svc.ExchangeLongLivedUserToken(context.Background(), ExchangeLongLivedUserTokenInput{
		AppID:           "app-123",
		AppSecret:       "secret-123",
		ShortLivedToken: "short-token",
		Version:         "v25.0",
	})
	if err != nil {
		t.Fatalf("exchange long lived token: %v", err)
	}
	if result.AccessToken != "long-token" {
		t.Fatalf("unexpected long-lived token: %s", result.AccessToken)
	}
	if result.ExpiresInSeconds != 5183944 {
		t.Fatalf("unexpected expires_in: %d", result.ExpiresInSeconds)
	}
}
