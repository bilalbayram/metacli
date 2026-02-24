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

type stubAuthSetupInput struct {
	Profile     string
	AppID       string
	AppSecret   string
	RedirectURI string
	Scopes      []string
	Mode        string
	PageID      string
	IGUserID    string
}

type stubAuthLoginAutoInput struct {
	Profile     string
	AppID       string
	AppSecret   string
	RedirectURI string
	Scopes      []string
}

type stubAuthDiscoverInput struct {
	Profile string
	Mode    string
}

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

	validateProfileResponse *auth.DebugTokenResponse
	validateProfileErr      error

	rotateProfileInput string
	rotateProfileErr   error

	debugTokenResponse *auth.DebugTokenResponse
	debugTokenErr      error

	listProfiles map[string]config.Profile
	listErr      error

	setupInput      *stubAuthSetupInput
	setupResult     map[string]any
	setupErr        error
	loginAutoInput  *stubAuthLoginAutoInput
	loginAutoResult map[string]any
	loginAutoErr    error
	discoverInput   *stubAuthDiscoverInput
	discoverResult  map[string]any
	discoverErr     error
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
		return "token-from-exchange", nil
	}
	return s.exchangeOAuthCodeToken, nil
}

func (s *stubAuthService) ValidateProfile(_ context.Context, _ string) (*auth.DebugTokenResponse, error) {
	if s.validateProfileErr != nil {
		return nil, s.validateProfileErr
	}
	if s.validateProfileResponse == nil {
		return &auth.DebugTokenResponse{Data: map[string]any{"is_valid": true}}, nil
	}
	return s.validateProfileResponse, nil
}

func (s *stubAuthService) RotateProfile(_ context.Context, profile string) error {
	s.rotateProfileInput = profile
	return s.rotateProfileErr
}

func (s *stubAuthService) DebugToken(_ context.Context, _, _, _ string) (*auth.DebugTokenResponse, error) {
	if s.debugTokenErr != nil {
		return nil, s.debugTokenErr
	}
	if s.debugTokenResponse == nil {
		return &auth.DebugTokenResponse{Data: map[string]any{"is_valid": true}}, nil
	}
	return s.debugTokenResponse, nil
}

func (s *stubAuthService) ListProfiles() (map[string]config.Profile, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.listProfiles == nil {
		return map[string]config.Profile{}, nil
	}
	return s.listProfiles, nil
}

func (s *stubAuthService) Setup(_ context.Context, input stubAuthSetupInput) (map[string]any, error) {
	s.setupInput = &input
	if s.setupErr != nil {
		return nil, s.setupErr
	}
	if s.setupResult == nil {
		return map[string]any{"setup": "ok"}, nil
	}
	return s.setupResult, nil
}

func (s *stubAuthService) LoginAuto(_ context.Context, input stubAuthLoginAutoInput) (map[string]any, error) {
	s.loginAutoInput = &input
	if s.loginAutoErr != nil {
		return nil, s.loginAutoErr
	}
	if s.loginAutoResult == nil {
		return map[string]any{"auth_url": "https://example.com/oauth"}, nil
	}
	return s.loginAutoResult, nil
}

func (s *stubAuthService) Discover(_ context.Context, input stubAuthDiscoverInput) (map[string]any, error) {
	s.discoverInput = &input
	if s.discoverErr != nil {
		return nil, s.discoverErr
	}
	if s.discoverResult == nil {
		return map[string]any{"items": []any{}}, nil
	}
	return s.discoverResult, nil
}

func useAuthServiceFactory(t *testing.T, factory func() (authCLIService, error)) {
	t.Helper()
	original := newAuthCLIService
	t.Cleanup(func() {
		newAuthCLIService = original
	})
	newAuthCLIService = factory
}

func useAuthNow(t *testing.T, nowFn func() time.Time) {
	t.Helper()
	original := authNow
	t.Cleanup(func() {
		authNow = original
	})
	authNow = nowFn
}

func TestNewAuthCommandIncludesAutomationSubcommands(t *testing.T) {
	cmd := NewAuthCommand(Runtime{})

	for _, name := range []string{"setup", "login", "login-manual", "discover"} {
		subcommand, _, err := cmd.Find([]string{name})
		if err != nil {
			t.Fatalf("find auth %s command: %v", name, err)
		}
		if subcommand == nil || subcommand.Name() != name {
			t.Fatalf("expected auth %s command, got %#v", name, subcommand)
		}
	}
}

func TestAuthAddSystemUserRequiresAppSecretFlag(t *testing.T) {
	service := &stubAuthService{}
	useAuthServiceFactory(t, func() (authCLIService, error) {
		return service, nil
	})

	cmd := NewAuthCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"add", "system-user",
		"--business-id", "123",
		"--app-id", "456",
		"--token", "token-1",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing --app-secret error")
	}
	if !strings.Contains(err.Error(), `required flag(s) "app-secret" not set`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if service.addSystemUserInput != nil {
		t.Fatal("expected AddSystemUser to not run when required flag is missing")
	}
}

func TestAuthLoginRejectsLegacyCodeFlag(t *testing.T) {
	cmd := NewAuthCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"login",
		"--app-id", "123",
		"--app-secret", "secret",
		"--redirect-uri", "https://localhost/callback",
		"--code", "legacy",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unknown --code flag error")
	}
	if !strings.Contains(err.Error(), "unknown flag: --code") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthLoginUsesAutoCallbackMethod(t *testing.T) {
	service := &stubAuthService{}
	useAuthServiceFactory(t, func() (authCLIService, error) {
		return service, nil
	})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewAuthCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{
		"login",
		"--app-id", "123",
		"--app-secret", "secret",
		"--redirect-uri", "https://localhost/callback",
		"--scopes", "pages_show_list,instagram_basic",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute auth login: %v", err)
	}

	if service.loginAutoInput == nil {
		t.Fatal("expected auto login method to be called")
	}
	if service.loginAutoInput.Profile != "prod" {
		t.Fatalf("unexpected profile %q", service.loginAutoInput.Profile)
	}
	if service.loginAutoInput.AppSecret != "secret" {
		t.Fatalf("unexpected app secret %q", service.loginAutoInput.AppSecret)
	}
	if len(service.loginAutoInput.Scopes) != 2 {
		t.Fatalf("unexpected scopes %#v", service.loginAutoInput.Scopes)
	}

	envelope := decodeEnvelope(t, stdout.Bytes())
	assertEnvelopeBasics(t, envelope, "meta auth login")
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object payload, got %T", envelope["data"])
	}
	if got := data["profile"]; got != "prod" {
		t.Fatalf("unexpected profile %v", got)
	}
	if got := data["auth_url"]; got != "https://example.com/oauth" {
		t.Fatalf("unexpected auth_url %v", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestAuthLoginManualUsesLegacyCodeExchangeFlow(t *testing.T) {
	service := &stubAuthService{exchangeOAuthCodeToken: "user-token"}
	useAuthServiceFactory(t, func() (authCLIService, error) {
		return service, nil
	})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewAuthCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{
		"login-manual",
		"--app-id", "123",
		"--redirect-uri", "https://localhost/callback",
		"--code", "oauth-code",
		"--scopes", "pages_show_list",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute auth login-manual: %v", err)
	}
	if service.exchangeOAuthCodeInput == nil {
		t.Fatal("expected OAuth exchange to run")
	}
	if got := service.exchangeOAuthCodeInput.Code; got != "oauth-code" {
		t.Fatalf("unexpected code %q", got)
	}
	if service.addUserInput == nil {
		t.Fatal("expected AddUser to run")
	}
	if got := service.addUserInput.Token; got != "user-token" {
		t.Fatalf("unexpected token %q", got)
	}

	envelope := decodeEnvelope(t, stdout.Bytes())
	assertEnvelopeBasics(t, envelope, "meta auth login-manual")
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestAuthValidateSupportsMinTTLAndRequiredScopes(t *testing.T) {
	fixedNow := time.Date(2026, 2, 24, 12, 0, 0, 0, time.UTC)
	useAuthNow(t, func() time.Time { return fixedNow })

	service := &stubAuthService{
		validateProfileResponse: &auth.DebugTokenResponse{
			Data: map[string]any{
				"is_valid":   true,
				"expires_at": float64(fixedNow.Add(2 * time.Hour).Unix()),
				"scopes":     []any{"pages_show_list", "instagram_basic"},
			},
		},
	}
	useAuthServiceFactory(t, func() (authCLIService, error) {
		return service, nil
	})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewAuthCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{
		"validate",
		"--min-ttl", "30m",
		"--require-scopes", "pages_show_list,instagram_basic",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute auth validate: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	envelope := decodeEnvelope(t, stdout.Bytes())
	assertEnvelopeBasics(t, envelope, "meta auth validate")
}

func TestAuthValidateFailsWhenRequiredScopeMissing(t *testing.T) {
	fixedNow := time.Date(2026, 2, 24, 12, 0, 0, 0, time.UTC)
	useAuthNow(t, func() time.Time { return fixedNow })

	service := &stubAuthService{
		validateProfileResponse: &auth.DebugTokenResponse{
			Data: map[string]any{
				"is_valid":   true,
				"expires_at": float64(fixedNow.Add(2 * time.Hour).Unix()),
				"scopes":     []any{"pages_show_list"},
			},
		},
	}
	useAuthServiceFactory(t, func() (authCLIService, error) {
		return service, nil
	})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewAuthCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{
		"validate",
		"--min-ttl", "30m",
		"--require-scopes", "pages_show_list,instagram_basic",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected scope validation error")
	}
	if !strings.Contains(err.Error(), "missing required scopes") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthDiscoverCallsServiceWithMode(t *testing.T) {
	service := &stubAuthService{}
	useAuthServiceFactory(t, func() (authCLIService, error) {
		return service, nil
	})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewAuthCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"discover", "--mode", "ig"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute auth discover: %v", err)
	}
	if service.discoverInput == nil {
		t.Fatal("expected discover method call")
	}
	if got := service.discoverInput.Mode; got != "ig" {
		t.Fatalf("unexpected mode %q", got)
	}
	envelope := decodeEnvelope(t, stdout.Bytes())
	assertEnvelopeBasics(t, envelope, "meta auth discover")
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestAuthDiscoverRejectsInvalidMode(t *testing.T) {
	service := &stubAuthService{}
	useAuthServiceFactory(t, func() (authCLIService, error) {
		return service, nil
	})

	cmd := NewAuthCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"discover", "--mode", "invalid"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected mode validation error")
	}
	if !strings.Contains(err.Error(), "mode must be one of: pages, ig") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthSetupCallsService(t *testing.T) {
	service := &stubAuthService{}
	useAuthServiceFactory(t, func() (authCLIService, error) {
		return service, nil
	})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewAuthCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{
		"setup",
		"--app-id", "123",
		"--app-secret", "secret",
		"--redirect-uri", "https://localhost/callback",
		"--scopes", "pages_show_list",
		"--mode", "pages",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute auth setup: %v", err)
	}
	if service.setupInput == nil {
		t.Fatal("expected setup method call")
	}
	if got := service.setupInput.AppSecret; got != "secret" {
		t.Fatalf("unexpected app secret %q", got)
	}
	envelope := decodeEnvelope(t, stdout.Bytes())
	assertEnvelopeBasics(t, envelope, "meta auth setup")
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestAuthSetupPropagatesServiceErrors(t *testing.T) {
	service := &stubAuthService{setupErr: errors.New("setup failed")}
	useAuthServiceFactory(t, func() (authCLIService, error) {
		return service, nil
	})

	cmd := NewAuthCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"setup",
		"--app-id", "123",
		"--app-secret", "secret",
		"--redirect-uri", "https://localhost/callback",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected setup failure")
	}
	if !strings.Contains(err.Error(), "setup failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}
