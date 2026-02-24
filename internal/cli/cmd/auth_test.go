package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/bilalbayram/metacli/internal/auth"
	"github.com/bilalbayram/metacli/internal/config"
)

type stubAuthService struct {
	addSystemUserInput *auth.AddSystemUserInput
	addSystemUserErr   error

	addUserInput *auth.AddUserInput
	addUserErr   error

	derivePageTokenInput *auth.PageTokenInput
	derivePageTokenErr   error

	setAppTokenInput *auth.SetAppTokenInput
	setAppTokenErr   error

	exchangeOAuthCodeInput *auth.ExchangeCodeInput
	exchangeOAuthCodeToken string
	exchangeOAuthCodeErr   error

	exchangeLongLivedInput *auth.ExchangeLongLivedUserTokenInput
	exchangeLongLived      auth.LongLivedToken
	exchangeLongLivedErr   error

	ensureValidProfile string
	ensureValidMinTTL  time.Duration
	ensureValidScopes  []string
	ensureValidResult  *auth.DebugTokenMetadata
	ensureValidErr     error

	validateProfileInput string
	validateProfileResp  *auth.DebugTokenResponse
	validateProfileErr   error

	rotateProfileInput string
	rotateProfileErr   error

	debugTokenInputToken      string
	debugTokenInputAccess     string
	debugTokenInputVersion    string
	debugTokenResp            *auth.DebugTokenResponse
	debugTokenErr             error
	listProfilesResp          map[string]config.Profile
	listProfilesErr           error
	discoveredPages           []auth.DiscoveredPage
	discoveredPagesErr        error
	updateProfileBindingsIn   *auth.UpdateProfileBindingsInput
	updateProfileBindingsErr  error
}

func (s *stubAuthService) AddSystemUser(_ context.Context, input auth.AddSystemUserInput) error {
	s.addSystemUserInput = &input
	return s.addSystemUserErr
}

func (s *stubAuthService) AddUser(_ context.Context, input auth.AddUserInput) error {
	s.addUserInput = &input
	return s.addUserErr
}

func (s *stubAuthService) DerivePageToken(_ context.Context, input auth.PageTokenInput) error {
	s.derivePageTokenInput = &input
	return s.derivePageTokenErr
}

func (s *stubAuthService) SetAppToken(_ context.Context, input auth.SetAppTokenInput) error {
	s.setAppTokenInput = &input
	return s.setAppTokenErr
}

func (s *stubAuthService) ExchangeOAuthCode(_ context.Context, input auth.ExchangeCodeInput) (string, error) {
	s.exchangeOAuthCodeInput = &input
	if s.exchangeOAuthCodeErr != nil {
		return "", s.exchangeOAuthCodeErr
	}
	if s.exchangeOAuthCodeToken == "" {
		return "short-token", nil
	}
	return s.exchangeOAuthCodeToken, nil
}

func (s *stubAuthService) ExchangeLongLivedUserToken(_ context.Context, input auth.ExchangeLongLivedUserTokenInput) (auth.LongLivedToken, error) {
	s.exchangeLongLivedInput = &input
	if s.exchangeLongLivedErr != nil {
		return auth.LongLivedToken{}, s.exchangeLongLivedErr
	}
	if s.exchangeLongLived.AccessToken == "" {
		return auth.LongLivedToken{AccessToken: "long-token", ExpiresInSeconds: 3600}, nil
	}
	return s.exchangeLongLived, nil
}

func (s *stubAuthService) EnsureValid(_ context.Context, profile string, minTTL time.Duration, requiredScopes []string) (*auth.DebugTokenMetadata, error) {
	s.ensureValidProfile = profile
	s.ensureValidMinTTL = minTTL
	s.ensureValidScopes = requiredScopes
	if s.ensureValidErr != nil {
		return nil, s.ensureValidErr
	}
	if s.ensureValidResult == nil {
		return &auth.DebugTokenMetadata{IsValid: true, Scopes: []string{"ads_read"}, ExpiresAt: time.Now().Add(24 * time.Hour)}, nil
	}
	return s.ensureValidResult, nil
}

func (s *stubAuthService) ValidateProfile(_ context.Context, profile string) (*auth.DebugTokenResponse, error) {
	s.validateProfileInput = profile
	if s.validateProfileErr != nil {
		return nil, s.validateProfileErr
	}
	if s.validateProfileResp == nil {
		return &auth.DebugTokenResponse{Data: map[string]any{"is_valid": true}}, nil
	}
	return s.validateProfileResp, nil
}

func (s *stubAuthService) RotateProfile(_ context.Context, profile string) error {
	s.rotateProfileInput = profile
	return s.rotateProfileErr
}

func (s *stubAuthService) DebugToken(_ context.Context, version, token, accessToken string) (*auth.DebugTokenResponse, error) {
	s.debugTokenInputVersion = version
	s.debugTokenInputToken = token
	s.debugTokenInputAccess = accessToken
	if s.debugTokenErr != nil {
		return nil, s.debugTokenErr
	}
	if s.debugTokenResp == nil {
		return &auth.DebugTokenResponse{Data: map[string]any{"is_valid": true, "scopes": []any{"ads_read"}, "expires_at": float64(time.Now().Add(24 * time.Hour).Unix())}}, nil
	}
	return s.debugTokenResp, nil
}

func (s *stubAuthService) ListProfiles() (map[string]config.Profile, error) {
	if s.listProfilesErr != nil {
		return nil, s.listProfilesErr
	}
	if s.listProfilesResp == nil {
		return map[string]config.Profile{}, nil
	}
	return s.listProfilesResp, nil
}

func (s *stubAuthService) DiscoverPagesAndIGBusinessAccounts(_ context.Context, _ string) ([]auth.DiscoveredPage, error) {
	if s.discoveredPagesErr != nil {
		return nil, s.discoveredPagesErr
	}
	return s.discoveredPages, nil
}

func (s *stubAuthService) UpdateProfileBindings(_ context.Context, input auth.UpdateProfileBindingsInput) error {
	s.updateProfileBindingsIn = &input
	return s.updateProfileBindingsErr
}

type fakeOAuthListener struct {
	redirectURI string
	code        string
	err         error
}

func (f *fakeOAuthListener) RedirectURI() string { return f.redirectURI }
func (f *fakeOAuthListener) Wait(_ context.Context, _ time.Duration) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.code, nil
}
func (f *fakeOAuthListener) Close(_ context.Context) error { return nil }

func useAuthServiceFactory(t *testing.T, factory func() (authCLIService, error)) {
	t.Helper()
	original := newAuthCLIService
	t.Cleanup(func() { newAuthCLIService = original })
	newAuthCLIService = factory
}

func useOAuthAutomationStubs(t *testing.T) {
	t.Helper()
	originalPKCE := newAuthPKCE
	originalState := newAuthOAuthState
	originalListener := newAuthOAuthListener
	originalBuildURL := buildAuthOAuthURLWithState
	originalOpen := openAuthBrowser
	t.Cleanup(func() {
		newAuthPKCE = originalPKCE
		newAuthOAuthState = originalState
		newAuthOAuthListener = originalListener
		buildAuthOAuthURLWithState = originalBuildURL
		openAuthBrowser = originalOpen
	})

	newAuthPKCE = func() (string, string, error) {
		return "pkce-verifier", "pkce-challenge", nil
	}
	newAuthOAuthState = func() (string, error) {
		return "oauth-state", nil
	}
	newAuthOAuthListener = func(_ string, _ string) (oauthCodeListener, error) {
		return &fakeOAuthListener{redirectURI: "http://127.0.0.1:4444/oauth/callback", code: "auth-code"}, nil
	}
	buildAuthOAuthURLWithState = func(_ string, _ string, _ []string, _ string, _ string, _ string) (string, error) {
		return "https://example.com/oauth", nil
	}
	openAuthBrowser = func(_ string) error { return nil }
}

func TestNewAuthCommandIncludesAutomationSubcommands(t *testing.T) {
	cmd := NewAuthCommand(Runtime{})
	for _, name := range []string{"setup", "login", "login-manual", "discover"} {
		sub, _, err := cmd.Find([]string{name})
		if err != nil {
			t.Fatalf("find %s command: %v", name, err)
		}
		if sub == nil || sub.Name() != name {
			t.Fatalf("expected %s command, got %#v", name, sub)
		}
	}
}

func TestAuthAddSystemUserRequiresAppSecretFlag(t *testing.T) {
	service := &stubAuthService{}
	useAuthServiceFactory(t, func() (authCLIService, error) { return service, nil })

	cmd := NewAuthCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"add", "system-user", "--business-id", "1", "--app-id", "2", "--token", "tok"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing app-secret flag error")
	}
	if !strings.Contains(err.Error(), `required flag(s) "app-secret" not set`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthLoginRejectsLegacyCodeFlag(t *testing.T) {
	cmd := NewAuthCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"login", "--app-id", "1", "--app-secret", "s", "--scopes", "ads_read", "--code", "legacy"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unknown --code flag error")
	}
	if !strings.Contains(err.Error(), "unknown flag: --code") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthLoginRunsAutoCallbackAndPersistsProfile(t *testing.T) {
	service := &stubAuthService{}
	useAuthServiceFactory(t, func() (authCLIService, error) { return service, nil })
	useOAuthAutomationStubs(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewAuthCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"login", "--app-id", "app_1", "--app-secret", "secret_1", "--scopes", "ads_read,pages_show_list", "--listen", "127.0.0.1:5555"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute login: %v", err)
	}
	if service.addUserInput == nil {
		t.Fatal("expected AddUser call")
	}
	if service.addUserInput.AppSecret != "secret_1" {
		t.Fatalf("unexpected app secret %q", service.addUserInput.AppSecret)
	}
	if service.addUserInput.Profile != "prod" {
		t.Fatalf("unexpected profile %q", service.addUserInput.Profile)
	}
	envelope := decodeEnvelope(t, stdout.Bytes())
	assertEnvelopeBasics(t, envelope, "meta auth login")
	if !strings.Contains(stderr.String(), "https://example.com/oauth") {
		t.Fatalf("expected auth url in stderr, got %q", stderr.String())
	}
}

func TestAuthLoginUsesRedirectURIOverrideForOAuthExchange(t *testing.T) {
	service := &stubAuthService{}
	useAuthServiceFactory(t, func() (authCLIService, error) { return service, nil })
	useOAuthAutomationStubs(t)

	capturedRedirectURI := ""
	originalBuild := buildAuthOAuthURLWithState
	t.Cleanup(func() {
		buildAuthOAuthURLWithState = originalBuild
	})
	buildAuthOAuthURLWithState = func(_ string, redirectURI string, _ []string, _ string, _ string, _ string) (string, error) {
		capturedRedirectURI = redirectURI
		return "https://example.com/oauth", nil
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewAuthCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{
		"login",
		"--app-id", "app_1",
		"--app-secret", "secret_1",
		"--scopes", "ads_read,pages_show_list",
		"--listen", "127.0.0.1:5555",
		"--redirect-uri", "https://auth.example.com/oauth/callback",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute login with redirect override: %v", err)
	}

	if capturedRedirectURI != "https://auth.example.com/oauth/callback" {
		t.Fatalf("unexpected oauth redirect uri passed to auth URL builder: %q", capturedRedirectURI)
	}
	if service.exchangeOAuthCodeInput == nil {
		t.Fatal("expected OAuth exchange call")
	}
	if service.exchangeOAuthCodeInput.RedirectURI != "https://auth.example.com/oauth/callback" {
		t.Fatalf("unexpected exchange redirect uri: %q", service.exchangeOAuthCodeInput.RedirectURI)
	}

	envelope := decodeEnvelope(t, stdout.Bytes())
	assertEnvelopeBasics(t, envelope, "meta auth login")
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object payload, got %T", envelope["data"])
	}
	if got := data["redirect_uri"]; got != "https://auth.example.com/oauth/callback" {
		t.Fatalf("unexpected output redirect_uri %v", got)
	}
	if !strings.Contains(stderr.String(), "https://example.com/oauth") {
		t.Fatalf("expected auth url in stderr, got %q", stderr.String())
	}
}

func TestAuthSetupDiscoversAndBindsDefaults(t *testing.T) {
	service := &stubAuthService{
		discoveredPages: []auth.DiscoveredPage{{PageID: "p_1", Name: "Page One", IGBusinessAccountID: "ig_1"}},
	}
	useAuthServiceFactory(t, func() (authCLIService, error) { return service, nil })
	useOAuthAutomationStubs(t)

	stdout := &bytes.Buffer{}
	cmd := NewAuthCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(stdout)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"setup", "--app-id", "app_1", "--app-secret", "secret_1", "--mode", "both", "--scope-pack", "solo_smb"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute setup: %v", err)
	}
	if service.updateProfileBindingsIn == nil {
		t.Fatal("expected profile binding update")
	}
	if service.updateProfileBindingsIn.PageID != "p_1" || service.updateProfileBindingsIn.IGUserID != "ig_1" {
		t.Fatalf("unexpected binding payload: %+v", service.updateProfileBindingsIn)
	}
	envelope := decodeEnvelope(t, stdout.Bytes())
	assertEnvelopeBasics(t, envelope, "meta auth setup")
}

func TestAuthValidateUsesEnsureValidWithDefaults(t *testing.T) {
	service := &stubAuthService{
		ensureValidResult: &auth.DebugTokenMetadata{IsValid: true, Scopes: []string{"ads_read"}, ExpiresAt: time.Now().Add(96 * time.Hour)},
		validateProfileResp: &auth.DebugTokenResponse{Data: map[string]any{"is_valid": true}},
	}
	useAuthServiceFactory(t, func() (authCLIService, error) { return service, nil })

	stdout := &bytes.Buffer{}
	cmd := NewAuthCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(stdout)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"validate"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute validate: %v", err)
	}
	if service.ensureValidMinTTL != defaultAuthPreflightTTL {
		t.Fatalf("unexpected min ttl: got=%s want=%s", service.ensureValidMinTTL, defaultAuthPreflightTTL)
	}
	envelope := decodeEnvelope(t, stdout.Bytes())
	assertEnvelopeBasics(t, envelope, "meta auth validate")
}

func TestAuthDiscoverRejectsInvalidMode(t *testing.T) {
	service := &stubAuthService{}
	useAuthServiceFactory(t, func() (authCLIService, error) { return service, nil })

	cmd := NewAuthCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"discover", "--mode", "invalid"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected invalid mode error")
	}
	if !strings.Contains(err.Error(), "mode must be one of: pages, ig") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthSetupPropagatesServiceErrors(t *testing.T) {
	service := &stubAuthService{exchangeOAuthCodeErr: errors.New("exchange failed")}
	useAuthServiceFactory(t, func() (authCLIService, error) { return service, nil })
	useOAuthAutomationStubs(t)

	cmd := NewAuthCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"setup", "--app-id", "app_1", "--app-secret", "secret_1"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected setup error")
	}
	if !strings.Contains(err.Error(), "exchange failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthLoginFailsWithInvalidRedirectURIOverride(t *testing.T) {
	service := &stubAuthService{}
	useAuthServiceFactory(t, func() (authCLIService, error) { return service, nil })
	useOAuthAutomationStubs(t)

	cmd := NewAuthCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"login",
		"--app-id", "app_1",
		"--app-secret", "secret_1",
		"--scopes", "ads_read",
		"--redirect-uri", "/not-absolute",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected invalid redirect-uri error")
	}
	if !strings.Contains(err.Error(), "invalid --redirect-uri") {
		t.Fatalf("unexpected error: %v", err)
	}
}
