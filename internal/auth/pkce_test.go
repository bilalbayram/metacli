package auth

import (
	"net/url"
	"strings"
	"testing"
)

func TestBuildOAuthURLWithState(t *testing.T) {
	t.Parallel()

	raw, err := BuildOAuthURLWithState(
		"app-123",
		"http://localhost:8080/callback",
		[]string{"ads_read", "pages_show_list"},
		"challenge-123",
		"state-123",
		"v25.0",
	)
	if err != nil {
		t.Fatalf("build oauth url with state: %v", err)
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse oauth url: %v", err)
	}
	if parsed.Path != "/v25.0/dialog/oauth" {
		t.Fatalf("unexpected oauth path: %s", parsed.Path)
	}

	query := parsed.Query()
	if query.Get("state") != "state-123" {
		t.Fatalf("unexpected oauth state: %s", query.Get("state"))
	}
	if query.Get("code_challenge_method") != "S256" {
		t.Fatalf("unexpected challenge method: %s", query.Get("code_challenge_method"))
	}
	if query.Get("scope") != "ads_read,pages_show_list" {
		t.Fatalf("unexpected scopes: %s", query.Get("scope"))
	}
}

func TestBuildOAuthURLWithStateRequiresState(t *testing.T) {
	t.Parallel()

	_, err := BuildOAuthURLWithState(
		"app-123",
		"http://localhost:8080/callback",
		[]string{"ads_read"},
		"challenge-123",
		"",
		"v25.0",
	)
	if err == nil || !strings.Contains(err.Error(), "oauth state is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
