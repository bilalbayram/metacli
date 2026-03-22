package linkedin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryProfileStore struct {
	mu       sync.Mutex
	profiles map[string]Profile
}

func newMemoryProfileStore() *memoryProfileStore {
	return &memoryProfileStore{profiles: map[string]Profile{}}
}

func (m *memoryProfileStore) Get(name string) (Profile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	profile, ok := m.profiles[name]
	if !ok {
		return Profile{}, fmt.Errorf("profile not found: %s", name)
	}
	return profile, nil
}

func (m *memoryProfileStore) Upsert(name string, profile Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.profiles[name] = profile
	return nil
}

type memorySecretStore struct {
	mu      sync.Mutex
	secrets map[string]string
}

func newMemorySecretStore() *memorySecretStore {
	return &memorySecretStore{secrets: map[string]string{}}
}

func (m *memorySecretStore) Set(ref string, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.secrets[ref] = value
	return nil
}

func (m *memorySecretStore) Get(ref string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.secrets[ref]
	if !ok {
		return "", fmt.Errorf("secret not found: %s", ref)
	}
	return value, nil
}

type fakeOAuthListener struct {
	redirectURI string
	state       string
	code        string
	err         error
	closed      bool
}

func (f *fakeOAuthListener) RedirectURI() string { return f.redirectURI }
func (f *fakeOAuthListener) Wait(_ context.Context, _ time.Duration) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.code, nil
}
func (f *fakeOAuthListener) Close(_ context.Context) error {
	f.closed = true
	return nil
}

func TestBuildAuthorizationURLStandardFlowIncludesStateAndScopes(t *testing.T) {
	t.Parallel()

	raw, err := buildAuthorizationURL(
		DefaultAuthBaseURL,
		"client-123",
		"http://127.0.0.1:3456/callback",
		[]string{"r_ads", "r_basicprofile"},
		"",
		"state-123",
		AuthFlowStandard,
	)
	if err != nil {
		t.Fatalf("build authorization url: %v", err)
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse authorization url: %v", err)
	}
	if parsed.Path != "/oauth/v2/authorization" {
		t.Fatalf("unexpected path: %s", parsed.Path)
	}

	query := parsed.Query()
	if got := query.Get("response_type"); got != "code" {
		t.Fatalf("unexpected response_type: %s", got)
	}
	if got := query.Get("client_id"); got != "client-123" {
		t.Fatalf("unexpected client_id: %s", got)
	}
	if got := query.Get("redirect_uri"); got != "http://127.0.0.1:3456/callback" {
		t.Fatalf("unexpected redirect_uri: %s", got)
	}
	if got := query.Get("state"); got != "state-123" {
		t.Fatalf("unexpected state: %s", got)
	}
	if got := query.Get("code_challenge"); got != "" {
		t.Fatalf("unexpected code_challenge: %s", got)
	}
	if got := query.Get("code_challenge_method"); got != "" {
		t.Fatalf("unexpected code_challenge_method: %s", got)
	}
	if got := query.Get("scope"); got != "r_ads r_basicprofile" {
		t.Fatalf("unexpected scope: %s", got)
	}
}

func TestBuildAuthorizationURLNativePKCEIncludesChallenge(t *testing.T) {
	t.Parallel()

	raw, err := buildAuthorizationURL(
		DefaultAuthBaseURL,
		"client-123",
		"http://127.0.0.1:3456/callback",
		[]string{"r_ads", "r_basicprofile"},
		"challenge-123",
		"state-123",
		AuthFlowNativePKCE,
	)
	if err != nil {
		t.Fatalf("build authorization url: %v", err)
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse authorization url: %v", err)
	}
	if parsed.Path != "/oauth/native-pkce/authorization" {
		t.Fatalf("unexpected path: %s", parsed.Path)
	}

	query := parsed.Query()
	if got := query.Get("code_challenge"); got != "challenge-123" {
		t.Fatalf("unexpected code_challenge: %s", got)
	}
	if got := query.Get("code_challenge_method"); got != "S256" {
		t.Fatalf("unexpected code_challenge_method: %s", got)
	}
}

func TestSetupPersistsTokensAndWhoAmIWithStandardFlow(t *testing.T) {
	profiles := newMemoryProfileStore()
	secrets := newMemorySecretStore()

	profileName := "prod"
	err := profiles.Upsert(profileName, Profile{
		Provider:        ProviderLinkedIn,
		LinkedInVersion: "202602",
		ClientID:        "client-123",
		ClientSecretRef: "secret-ref",
		AccessTokenRef:  "access-ref",
		RefreshTokenRef: "refresh-ref",
		Scopes:          []string{"r_ads"},
	})
	if err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	if err := secrets.Set("secret-ref", "secret-123"); err != nil {
		t.Fatalf("seed secret: %v", err)
	}

	originalState := newState
	originalListener := newListener
	originalBrowser := openBrowser
	defer func() {
		newState = originalState
		newListener = originalListener
		openBrowser = originalBrowser
	}()
	newState = func() (string, error) {
		return "state-123", nil
	}

	listener := &fakeOAuthListener{redirectURI: "http://127.0.0.1:3456/callback", code: "auth-code"}
	var openedURL string
	newListener = func(redirectURI string, state string) (oauthCodeListener, error) {
		if redirectURI != "http://127.0.0.1:3456/callback" {
			t.Fatalf("unexpected redirect uri: %s", redirectURI)
		}
		if state != "state-123" {
			t.Fatalf("unexpected state: %s", state)
		}
		return listener, nil
	}
	openBrowser = func(raw string) error {
		openedURL = raw
		return nil
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/v2/accessToken":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
				t.Fatalf("unexpected content-type: %s", got)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			if got := r.PostForm.Get("grant_type"); got != "authorization_code" {
				t.Fatalf("unexpected grant_type: %s", got)
			}
			if got := r.PostForm.Get("code"); got != "auth-code" {
				t.Fatalf("unexpected code: %s", got)
			}
			if got := r.PostForm.Get("redirect_uri"); got != "http://127.0.0.1:3456/callback" {
				t.Fatalf("unexpected redirect_uri: %s", got)
			}
			if got := r.PostForm.Get("client_id"); got != "client-123" {
				t.Fatalf("unexpected client_id: %s", got)
			}
			if got := r.PostForm.Get("client_secret"); got != "secret-123" {
				t.Fatalf("unexpected client_secret: %s", got)
			}
			if got := r.PostForm.Get("code_verifier"); got != "" {
				t.Fatalf("unexpected code_verifier: %s", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-123","expires_in":3600,"refresh_token":"refresh-123","refresh_token_expires_in":7200,"scope":"r_ads r_basicprofile"}`))
		case "/v2/me":
			if got := r.Header.Get("Authorization"); got != "Bearer access-123" {
				t.Fatalf("unexpected authorization header: %s", got)
			}
			if got := r.Header.Get("LinkedIn-Version"); got != "202602" {
				t.Fatalf("unexpected LinkedIn-Version header: %s", got)
			}
			if got := r.Header.Get("X-Restli-Protocol-Version"); got != restLiProtocolVersion {
				t.Fatalf("unexpected restli header: %s", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"urn:li:person:abc","localizedFirstName":"Ada","localizedLastName":"Lovelace"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	svc := NewService(server.Client(), server.URL, server.URL, profiles, secrets)
	result, err := svc.Setup(context.Background(), profileName, SetupInput{
		RedirectURI: "http://127.0.0.1:3456/callback",
		Scopes:      []string{"r_ads", "r_basicprofile"},
		OpenBrowser: true,
		Timeout:     time.Second,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	if openedURL == "" {
		t.Fatal("expected browser to be opened")
	}
	if !strings.Contains(openedURL, "/oauth/v2/authorization") {
		t.Fatalf("unexpected authorization url: %s", openedURL)
	}
	if !strings.Contains(openedURL, "state=state-123") {
		t.Fatalf("unexpected authorization url: %s", openedURL)
	}

	access, err := secrets.Get("access-ref")
	if err != nil {
		t.Fatalf("load access token: %v", err)
	}
	if access != "access-123" {
		t.Fatalf("unexpected access token: %s", access)
	}
	refresh, err := secrets.Get("refresh-ref")
	if err != nil {
		t.Fatalf("load refresh token: %v", err)
	}
	if refresh != "refresh-123" {
		t.Fatalf("unexpected refresh token: %s", refresh)
	}

	if result == nil || result.Token.AccessToken != "access-123" {
		t.Fatalf("unexpected setup token result: %#v", result)
	}
	if result.AuthFlow != string(AuthFlowStandard) {
		t.Fatalf("unexpected auth flow: %s", result.AuthFlow)
	}
	if result.WhoAmI == nil || result.WhoAmI.FullName != "Ada Lovelace" {
		t.Fatalf("unexpected whoami result: %#v", result.WhoAmI)
	}
}

func TestSetupNativePKCEUsesCodeVerifierExchange(t *testing.T) {
	profiles := newMemoryProfileStore()
	secrets := newMemorySecretStore()

	profileName := "prod"
	err := profiles.Upsert(profileName, Profile{
		Provider:        ProviderLinkedIn,
		LinkedInVersion: "202602",
		ClientID:        "client-123",
		ClientSecretRef: "secret-ref",
		AccessTokenRef:  "access-ref",
		RefreshTokenRef: "refresh-ref",
		Scopes:          []string{"r_ads"},
	})
	if err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	if err := secrets.Set("secret-ref", "secret-123"); err != nil {
		t.Fatalf("seed secret: %v", err)
	}

	originalPKCE := newPKCE
	originalState := newState
	originalListener := newListener
	originalBrowser := openBrowser
	defer func() {
		newPKCE = originalPKCE
		newState = originalState
		newListener = originalListener
		openBrowser = originalBrowser
	}()

	newPKCE = func() (string, string, error) {
		return "verifier-123", "challenge-123", nil
	}
	newState = func() (string, error) {
		return "state-123", nil
	}

	listener := &fakeOAuthListener{redirectURI: "http://127.0.0.1:3456/callback", code: "auth-code"}
	var openedURL string
	newListener = func(redirectURI string, state string) (oauthCodeListener, error) {
		return listener, nil
	}
	openBrowser = func(raw string) error {
		openedURL = raw
		return nil
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/v2/accessToken":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			if got := r.PostForm.Get("code_verifier"); got != "verifier-123" {
				t.Fatalf("unexpected code_verifier: %s", got)
			}
			if got := r.PostForm.Get("client_secret"); got != "" {
				t.Fatalf("unexpected client_secret: %s", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-123","expires_in":3600,"scope":"r_ads"}`))
		case "/v2/me":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"urn:li:person:abc","localizedFirstName":"Ada","localizedLastName":"Lovelace"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	svc := NewService(server.Client(), server.URL, server.URL, profiles, secrets)
	result, err := svc.Setup(context.Background(), profileName, SetupInput{
		RedirectURI: "http://127.0.0.1:3456/callback",
		Scopes:      []string{"r_ads"},
		AuthFlow:    string(AuthFlowNativePKCE),
		OpenBrowser: true,
		Timeout:     time.Second,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	if !strings.Contains(openedURL, "/oauth/native-pkce/authorization") {
		t.Fatalf("unexpected authorization url: %s", openedURL)
	}
	if result.AuthFlow != string(AuthFlowNativePKCE) {
		t.Fatalf("unexpected auth flow: %s", result.AuthFlow)
	}
}

func TestValidateRefreshesExpiredToken(t *testing.T) {
	profiles := newMemoryProfileStore()
	secrets := newMemorySecretStore()

	now := time.Now().UTC()
	err := profiles.Upsert("prod", Profile{
		Provider:              ProviderLinkedIn,
		LinkedInVersion:       "202602",
		ClientID:              "client-123",
		ClientSecretRef:       "secret-ref",
		AccessTokenRef:        "access-ref",
		RefreshTokenRef:       "refresh-ref",
		Scopes:                []string{"r_ads"},
		AccessTokenExpiresAt:  now.Add(-1 * time.Hour),
		RefreshTokenExpiresAt: now.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	if err := secrets.Set("access-ref", "expired-access"); err != nil {
		t.Fatalf("seed access token: %v", err)
	}
	if err := secrets.Set("refresh-ref", "refresh-123"); err != nil {
		t.Fatalf("seed refresh token: %v", err)
	}
	if err := secrets.Set("secret-ref", "secret-123"); err != nil {
		t.Fatalf("seed client secret: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/v2/accessToken":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			if got := r.PostForm.Get("grant_type"); got != "refresh_token" {
				t.Fatalf("unexpected grant_type: %s", got)
			}
			if got := r.PostForm.Get("refresh_token"); got != "refresh-123" {
				t.Fatalf("unexpected refresh token: %s", got)
			}
			if got := r.PostForm.Get("client_id"); got != "client-123" {
				t.Fatalf("unexpected client_id: %s", got)
			}
			if got := r.PostForm.Get("client_secret"); got != "secret-123" {
				t.Fatalf("unexpected client_secret: %s", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"fresh-access","expires_in":3600,"refresh_token":"fresh-refresh","refresh_token_expires_in":7200,"scope":"r_ads r_basicprofile"}`))
		case "/v2/me":
			if got := r.Header.Get("Authorization"); got != "Bearer fresh-access" {
				t.Fatalf("unexpected authorization header: %s", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"urn:li:person:xyz","firstName":{"localized":{"en_US":"Grace"},"preferredLocale":{"language":"en","country":"US"}},"lastName":{"localized":{"en_US":"Hopper"},"preferredLocale":{"language":"en","country":"US"}}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	svc := NewService(server.Client(), server.URL, server.URL, profiles, secrets)
	result, err := svc.Validate(context.Background(), "prod")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !result.Refreshed {
		t.Fatal("expected token refresh")
	}
	if result.Token.AccessToken != "fresh-access" {
		t.Fatalf("unexpected refreshed access token: %s", result.Token.AccessToken)
	}
	if result.WhoAmI == nil || result.WhoAmI.FullName != "Grace Hopper" {
		t.Fatalf("unexpected whoami: %#v", result.WhoAmI)
	}

	access, err := secrets.Get("access-ref")
	if err != nil {
		t.Fatalf("load updated access token: %v", err)
	}
	if access != "fresh-access" {
		t.Fatalf("unexpected persisted access token: %s", access)
	}
	refresh, err := secrets.Get("refresh-ref")
	if err != nil {
		t.Fatalf("load updated refresh token: %v", err)
	}
	if refresh != "fresh-refresh" {
		t.Fatalf("unexpected persisted refresh token: %s", refresh)
	}
	updatedProfile, err := profiles.Get("prod")
	if err != nil {
		t.Fatalf("load updated profile: %v", err)
	}
	if updatedProfile.AccessTokenExpiresAt.Before(now) {
		t.Fatal("expected access token expiry to be updated")
	}
}

func TestScopesReturnsStoredScopes(t *testing.T) {
	profiles := newMemoryProfileStore()
	secrets := newMemorySecretStore()
	err := profiles.Upsert("prod", Profile{
		Provider:             ProviderLinkedIn,
		LinkedInVersion:      "202602",
		ClientID:             "client-123",
		ClientSecretRef:      "secret-ref",
		AccessTokenRef:       "access-ref",
		RefreshTokenRef:      "refresh-ref",
		Scopes:               []string{"r_ads", "r_basicprofile"},
		AccessTokenExpiresAt: time.Now().UTC().Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	if err := secrets.Set("access-ref", "access-123"); err != nil {
		t.Fatalf("seed access token: %v", err)
	}

	svc := NewService(&http.Client{Timeout: time.Second}, DefaultAPIBaseURL, DefaultAPIBaseURL, profiles, secrets)
	scopes, err := svc.Scopes(context.Background(), "prod")
	if err != nil {
		t.Fatalf("scopes: %v", err)
	}
	if len(scopes) != 2 || scopes[0] != "r_ads" || scopes[1] != "r_basicprofile" {
		t.Fatalf("unexpected scopes: %#v", scopes)
	}
}

func TestClassifyLinkedInErrorCategories(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantCat ErrorCategory
	}{
		{
			name:    "validation",
			status:  http.StatusBadRequest,
			body:    `{"error":"invalid_request","error_description":"missing client_id"}`,
			wantCat: ErrorCategoryValidation,
		},
		{
			name:    "auth",
			status:  http.StatusUnauthorized,
			body:    `{"message":"invalid token"}`,
			wantCat: ErrorCategoryAuth,
		},
		{
			name:    "permission",
			status:  http.StatusForbidden,
			body:    `{"message":"access denied"}`,
			wantCat: ErrorCategoryPermission,
		},
		{
			name:    "rate limit",
			status:  http.StatusTooManyRequests,
			body:    `{"message":"too many requests"}`,
			wantCat: ErrorCategoryRateLimit,
		},
		{
			name:    "version",
			status:  http.StatusUpgradeRequired,
			body:    `{"message":"unsupported version"}`,
			wantCat: ErrorCategoryVersion,
		},
		{
			name:    "not found",
			status:  http.StatusNotFound,
			body:    `{"message":"missing"}`,
			wantCat: ErrorCategoryNotFound,
		},
		{
			name:    "conflict",
			status:  http.StatusConflict,
			body:    `{"message":"conflict"}`,
			wantCat: ErrorCategoryConflict,
		},
		{
			name:    "transient",
			status:  http.StatusInternalServerError,
			body:    `{"message":"server error"}`,
			wantCat: ErrorCategoryTransient,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := classifyLinkedInError(tc.status, []byte(tc.body))
			var apiErr *Error
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected linked in error, got %T", err)
			}
			if apiErr.Category != tc.wantCat {
				t.Fatalf("unexpected category: got=%s want=%s", apiErr.Category, tc.wantCat)
			}
		})
	}
}

func TestParseWhoAmISupportsLocalizedNames(t *testing.T) {
	t.Parallel()

	result := parseWhoAmI(map[string]any{
		"id": "urn:li:person:abc",
		"firstName": map[string]any{
			"localized": map[string]any{"en_US": "Linus"},
			"preferredLocale": map[string]any{
				"language": "en",
				"country":  "US",
			},
		},
		"lastName": map[string]any{
			"localized": map[string]any{"en_US": "Torvalds"},
			"preferredLocale": map[string]any{
				"language": "en",
				"country":  "US",
			},
		},
	})
	if result.ID != "urn:li:person:abc" {
		t.Fatalf("unexpected id: %s", result.ID)
	}
	if result.FullName != "Linus Torvalds" {
		t.Fatalf("unexpected full name: %s", result.FullName)
	}
}
