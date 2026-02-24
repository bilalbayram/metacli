package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bilalbayram/metacli/internal/config"
)

func TestDiscoverPages(t *testing.T) {
	t.Parallel()

	tokenRef, err := SecretRef("prod", SecretToken)
	if err != nil {
		t.Fatalf("secret ref: %v", err)
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
	})

	secrets := newInMemorySecretStore()
	if err := secrets.Set(tokenRef, "user-token"); err != nil {
		t.Fatalf("set token secret: %v", err)
	}
	if err := secrets.Set(appSecretRef, "app-secret"); err != nil {
		t.Fatalf("set app secret: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v25.0/me/accounts" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("access_token") != "user-token" {
			t.Fatalf("unexpected access token: %s", query.Get("access_token"))
		}
		if query.Get("fields") != "id,name,instagram_business_account{id}" {
			t.Fatalf("unexpected fields query: %s", query.Get("fields"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"100","name":"Page A","instagram_business_account":{"id":"ig-100"}},{"id":"200","name":"Page B"}]}`))
	}))
	defer server.Close()

	svc := NewService(configPath, secrets, server.Client(), server.URL)
	pages, err := svc.DiscoverPages(context.Background(), "prod")
	if err != nil {
		t.Fatalf("discover pages: %v", err)
	}

	if len(pages) != 2 {
		t.Fatalf("unexpected page count: %d", len(pages))
	}
	if pages[0].PageID != "100" || pages[0].Name != "Page A" || pages[0].IGBusinessAccountID != "ig-100" {
		t.Fatalf("unexpected first page: %+v", pages[0])
	}
	if pages[1].PageID != "200" || pages[1].Name != "Page B" || pages[1].IGBusinessAccountID != "" {
		t.Fatalf("unexpected second page: %+v", pages[1])
	}
}

func TestDiscoverPagesRejectsInvalidGraphPayload(t *testing.T) {
	t.Parallel()

	tokenRef, err := SecretRef("prod", SecretToken)
	if err != nil {
		t.Fatalf("secret ref: %v", err)
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
	})

	secrets := newInMemorySecretStore()
	if err := secrets.Set(tokenRef, "user-token"); err != nil {
		t.Fatalf("set token secret: %v", err)
	}
	if err := secrets.Set(appSecretRef, "app-secret"); err != nil {
		t.Fatalf("set app secret: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"name":"missing-id"}]}`))
	}))
	defer server.Close()

	svc := NewService(configPath, secrets, server.Client(), server.URL)
	_, err = svc.DiscoverPages(context.Background(), "prod")
	if err == nil {
		t.Fatal("expected discovery failure for invalid payload")
	}
}
