package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/bilalbayram/metacli/internal/config"
)

const (
	TokenTypeSystemUser = "system_user"
	TokenTypeUser       = "user"
	TokenTypePage       = "page"
	TokenTypeApp        = "app"

	AuthProviderFacebookLogin = "facebook_login"
	AuthProviderInstagram     = "instagram_login"
	AuthProviderSystemUser    = "system_user"
	AuthProviderApp           = "app"

	AuthModeBoth      = "both"
	AuthModeFacebook  = "facebook"
	AuthModeInstagram = "instagram"

	DefaultGraphBaseURL = "https://graph.facebook.com"
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Service struct {
	configPath   string
	secrets      SecretStore
	httpClient   HTTPClient
	graphBaseURL string
}

type AddSystemUserInput struct {
	Profile    string
	BusinessID string
	AppID      string
	Token      string
	AppSecret  string
	AuthMode   string
	Scopes     []string
}

type AddUserInput struct {
	Profile         string
	AppID           string
	Token           string
	AppSecret       string
	AuthProvider    string
	AuthMode        string
	Scopes          []string
	IssuedAt        string
	ExpiresAt       string
	LastValidatedAt string
	IGUserID        string
	PageID          string
}

type PageTokenInput struct {
	Profile       string
	PageID        string
	SourceProfile string
}

type SetAppTokenInput struct {
	Profile   string
	AppID     string
	AppSecret string
	AuthMode  string
	Scopes    []string
}

type UpdateProfileBindingsInput struct {
	Profile  string
	PageID   string
	IGUserID string
}

type ExchangeCodeInput struct {
	AppID       string
	RedirectURI string
	Code        string
	CodeVerifier string
	Version     string
}

type DebugTokenResponse struct {
	Data map[string]any `json:"data"`
}

func NewService(configPath string, secrets SecretStore, httpClient HTTPClient, graphBaseURL string) *Service {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if graphBaseURL == "" {
		graphBaseURL = DefaultGraphBaseURL
	}
	return &Service{
		configPath:   configPath,
		secrets:      secrets,
		httpClient:   httpClient,
		graphBaseURL: strings.TrimSuffix(graphBaseURL, "/"),
	}
}

func (s *Service) AddSystemUser(ctx context.Context, input AddSystemUserInput) error {
	if strings.TrimSpace(input.Profile) == "" {
		return errors.New("profile is required")
	}
	if strings.TrimSpace(input.BusinessID) == "" {
		return errors.New("business id is required")
	}
	if strings.TrimSpace(input.AppID) == "" {
		return errors.New("app id is required")
	}
	if strings.TrimSpace(input.Token) == "" {
		return errors.New("token is required")
	}
	if strings.TrimSpace(input.AppSecret) == "" {
		return errors.New("app secret is required")
	}

	cfg, err := config.LoadOrCreate(s.configPath)
	if err != nil {
		return err
	}

	tokenRef, err := SecretRef(input.Profile, SecretToken)
	if err != nil {
		return err
	}
	if err := s.secrets.Set(tokenRef, input.Token); err != nil {
		return err
	}

	appSecretRef, err := SecretRef(input.Profile, SecretAppSecret)
	if err != nil {
		return err
	}
	if err := s.secrets.Set(appSecretRef, input.AppSecret); err != nil {
		return err
	}

	now := time.Now().UTC()
	scopes := normalizedScopesOrDefault(input.Scopes, []string{"ads_management", "business_management"})
	authMode := normalizedAuthModeOrDefault(input.AuthMode, AuthModeBoth)

	if err := cfg.UpsertProfile(input.Profile, config.Profile{
		Domain:          config.DefaultDomain,
		GraphVersion:    config.DefaultGraphVersion,
		TokenType:       TokenTypeSystemUser,
		BusinessID:      input.BusinessID,
		AppID:           input.AppID,
		TokenRef:        tokenRef,
		AppSecretRef:    appSecretRef,
		AuthProvider:    AuthProviderSystemUser,
		AuthMode:        authMode,
		Scopes:          scopes,
		IssuedAt:        now.Format(time.RFC3339),
		ExpiresAt:       now.AddDate(10, 0, 0).Format(time.RFC3339),
		LastValidatedAt: now.Format(time.RFC3339),
	}); err != nil {
		return err
	}

	return config.Save(s.configPath, cfg)
}

func (s *Service) AddUser(ctx context.Context, input AddUserInput) error {
	if strings.TrimSpace(input.Profile) == "" {
		return errors.New("profile is required")
	}
	if strings.TrimSpace(input.AppID) == "" {
		return errors.New("app id is required")
	}
	if strings.TrimSpace(input.Token) == "" {
		return errors.New("token is required")
	}
	if strings.TrimSpace(input.AppSecret) == "" {
		return errors.New("app secret is required")
	}

	cfg, err := config.LoadOrCreate(s.configPath)
	if err != nil {
		return err
	}

	tokenRef, err := SecretRef(input.Profile, SecretToken)
	if err != nil {
		return err
	}
	if err := s.secrets.Set(tokenRef, input.Token); err != nil {
		return err
	}
	appSecretRef, err := SecretRef(input.Profile, SecretAppSecret)
	if err != nil {
		return err
	}
	if err := s.secrets.Set(appSecretRef, input.AppSecret); err != nil {
		return err
	}

	now := time.Now().UTC()
	issuedAt := strings.TrimSpace(input.IssuedAt)
	if issuedAt == "" {
		issuedAt = now.Format(time.RFC3339)
	}
	expiresAt := strings.TrimSpace(input.ExpiresAt)
	if expiresAt == "" {
		expiresAt = now.AddDate(0, 0, 60).Format(time.RFC3339)
	}
	lastValidatedAt := strings.TrimSpace(input.LastValidatedAt)
	if lastValidatedAt == "" {
		lastValidatedAt = now.Format(time.RFC3339)
	}

	if err := cfg.UpsertProfile(input.Profile, config.Profile{
		Domain:          config.DefaultDomain,
		GraphVersion:    config.DefaultGraphVersion,
		TokenType:       TokenTypeUser,
		AppID:           input.AppID,
		PageID:          strings.TrimSpace(input.PageID),
		TokenRef:        tokenRef,
		AppSecretRef:    appSecretRef,
		AuthProvider:    normalizedAuthProviderOrDefault(input.AuthProvider, AuthProviderFacebookLogin),
		AuthMode:        normalizedAuthModeOrDefault(input.AuthMode, AuthModeBoth),
		Scopes:          normalizedScopesOrDefault(input.Scopes, []string{"ads_read"}),
		IssuedAt:        issuedAt,
		ExpiresAt:       expiresAt,
		LastValidatedAt: lastValidatedAt,
		IGUserID:        strings.TrimSpace(input.IGUserID),
	}); err != nil {
		return err
	}

	return config.Save(s.configPath, cfg)
}

func (s *Service) SetAppToken(ctx context.Context, input SetAppTokenInput) error {
	if strings.TrimSpace(input.Profile) == "" {
		return errors.New("profile is required")
	}
	if strings.TrimSpace(input.AppID) == "" {
		return errors.New("app id is required")
	}
	if strings.TrimSpace(input.AppSecret) == "" {
		return errors.New("app secret is required")
	}

	token, err := s.fetchAppToken(ctx, config.DefaultGraphVersion, input.AppID, input.AppSecret)
	if err != nil {
		return err
	}

	cfg, err := config.LoadOrCreate(s.configPath)
	if err != nil {
		return err
	}

	tokenRef, err := SecretRef(input.Profile, SecretToken)
	if err != nil {
		return err
	}
	if err := s.secrets.Set(tokenRef, token); err != nil {
		return err
	}

	appSecretRef, err := SecretRef(input.Profile, SecretAppSecret)
	if err != nil {
		return err
	}
	if err := s.secrets.Set(appSecretRef, input.AppSecret); err != nil {
		return err
	}

	now := time.Now().UTC()

	if err := cfg.UpsertProfile(input.Profile, config.Profile{
		Domain:          config.DefaultDomain,
		GraphVersion:    config.DefaultGraphVersion,
		TokenType:       TokenTypeApp,
		AppID:           input.AppID,
		TokenRef:        tokenRef,
		AppSecretRef:    appSecretRef,
		AuthProvider:    AuthProviderApp,
		AuthMode:        normalizedAuthModeOrDefault(input.AuthMode, AuthModeBoth),
		Scopes:          normalizedScopesOrDefault(input.Scopes, []string{"ads_management"}),
		IssuedAt:        now.Format(time.RFC3339),
		ExpiresAt:       now.AddDate(10, 0, 0).Format(time.RFC3339),
		LastValidatedAt: now.Format(time.RFC3339),
	}); err != nil {
		return err
	}

	return config.Save(s.configPath, cfg)
}

func (s *Service) DerivePageToken(ctx context.Context, input PageTokenInput) error {
	if strings.TrimSpace(input.Profile) == "" {
		return errors.New("profile is required")
	}
	if strings.TrimSpace(input.PageID) == "" {
		return errors.New("page id is required")
	}
	if strings.TrimSpace(input.SourceProfile) == "" {
		return errors.New("source profile is required")
	}

	cfg, err := config.Load(s.configPath)
	if err != nil {
		return err
	}
	sourceName, sourceProfile, err := cfg.ResolveProfile(input.SourceProfile)
	if err != nil {
		return err
	}
	sourceToken, err := s.secrets.Get(sourceProfile.TokenRef)
	if err != nil {
		return err
	}
	if strings.TrimSpace(sourceProfile.AppID) == "" {
		return errors.New("source profile app_id is required for page token derivation")
	}
	if strings.TrimSpace(sourceProfile.AppSecretRef) == "" {
		return errors.New("source profile app_secret_ref is required for page token derivation")
	}
	sourceAppSecret, err := s.secrets.Get(sourceProfile.AppSecretRef)
	if err != nil {
		return err
	}

	token, err := s.fetchPageToken(ctx, sourceProfile.GraphVersion, input.PageID, sourceToken)
	if err != nil {
		return err
	}

	tokenRef, err := SecretRef(input.Profile, SecretToken)
	if err != nil {
		return err
	}
	if err := s.secrets.Set(tokenRef, token); err != nil {
		return err
	}
	appSecretRef, err := SecretRef(input.Profile, SecretAppSecret)
	if err != nil {
		return err
	}
	if err := s.secrets.Set(appSecretRef, sourceAppSecret); err != nil {
		return err
	}

	now := time.Now().UTC()
	issuedAt := strings.TrimSpace(sourceProfile.IssuedAt)
	if issuedAt == "" {
		issuedAt = now.Format(time.RFC3339)
	}
	expiresAt := strings.TrimSpace(sourceProfile.ExpiresAt)
	if expiresAt == "" {
		expiresAt = now.AddDate(0, 0, 60).Format(time.RFC3339)
	}
	lastValidatedAt := now.Format(time.RFC3339)
	scopes := normalizedScopesOrDefault(sourceProfile.Scopes, []string{"pages_read_engagement"})
	if !containsScope(scopes, "pages_read_engagement") {
		scopes = append(scopes, "pages_read_engagement")
	}
	authProvider := normalizedAuthProviderOrDefault(sourceProfile.AuthProvider, AuthProviderFacebookLogin)
	authMode := normalizedAuthModeOrDefault(sourceProfile.AuthMode, AuthModeFacebook)

	if err := cfg.UpsertProfile(input.Profile, config.Profile{
		Domain:          config.DefaultDomain,
		GraphVersion:    sourceProfile.GraphVersion,
		TokenType:       TokenTypePage,
		AppID:           sourceProfile.AppID,
		PageID:          input.PageID,
		SourceProfile:   sourceName,
		TokenRef:        tokenRef,
		AppSecretRef:    appSecretRef,
		AuthProvider:    authProvider,
		AuthMode:        authMode,
		Scopes:          scopes,
		IssuedAt:        issuedAt,
		ExpiresAt:       expiresAt,
		LastValidatedAt: lastValidatedAt,
		IGUserID:        sourceProfile.IGUserID,
	}); err != nil {
		return err
	}

	return config.Save(s.configPath, cfg)
}

func (s *Service) ExchangeOAuthCode(ctx context.Context, input ExchangeCodeInput) (string, error) {
	if strings.TrimSpace(input.AppID) == "" {
		return "", errors.New("app id is required")
	}
	if strings.TrimSpace(input.RedirectURI) == "" {
		return "", errors.New("redirect uri is required")
	}
	if strings.TrimSpace(input.Code) == "" {
		return "", errors.New("authorization code is required")
	}
	version := input.Version
	if version == "" {
		version = config.DefaultGraphVersion
	}

	body := url.Values{}
	body.Set("client_id", input.AppID)
	body.Set("redirect_uri", input.RedirectURI)
	body.Set("code", input.Code)
	if strings.TrimSpace(input.CodeVerifier) != "" {
		body.Set("code_verifier", input.CodeVerifier)
	}

	response := map[string]any{}
	if err := s.doFormRequest(ctx, version, "oauth/access_token", body, "", "", &response); err != nil {
		return "", err
	}

	token, _ := response["access_token"].(string)
	if strings.TrimSpace(token) == "" {
		return "", errors.New("oauth token exchange response did not include access_token")
	}
	return token, nil
}

func (s *Service) ValidateProfile(ctx context.Context, profileName string) (*DebugTokenResponse, error) {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return nil, err
	}
	_, profile, err := cfg.ResolveProfile(profileName)
	if err != nil {
		return nil, err
	}

	token, err := s.secrets.Get(profile.TokenRef)
	if err != nil {
		return nil, err
	}

	debugAccessToken := token
	if profile.AppID != "" && profile.AppSecretRef != "" {
		appSecret, err := s.secrets.Get(profile.AppSecretRef)
		if err != nil {
			return nil, err
		}
		debugAccessToken, err = s.fetchAppToken(ctx, profile.GraphVersion, profile.AppID, appSecret)
		if err != nil {
			return nil, err
		}
	}

	resp, err := s.DebugToken(ctx, profile.GraphVersion, token, debugAccessToken)
	if err != nil {
		return nil, err
	}
	valid, ok := resp.Data["is_valid"].(bool)
	if !ok || !valid {
		return nil, errors.New("profile token is invalid")
	}
	return resp, nil
}

func (s *Service) RotateProfile(ctx context.Context, profileName string) error {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return err
	}
	_, profile, err := cfg.ResolveProfile(profileName)
	if err != nil {
		return err
	}

	if profile.TokenType != TokenTypeApp {
		return fmt.Errorf("token rotation is only supported for %q profiles in v1", TokenTypeApp)
	}
	if profile.AppID == "" {
		return errors.New("app profile does not include app_id")
	}
	if profile.AppSecretRef == "" {
		return errors.New("app profile does not include app_secret_ref")
	}

	appSecret, err := s.secrets.Get(profile.AppSecretRef)
	if err != nil {
		return err
	}
	token, err := s.fetchAppToken(ctx, profile.GraphVersion, profile.AppID, appSecret)
	if err != nil {
		return err
	}
	if err := s.secrets.Set(profile.TokenRef, token); err != nil {
		return err
	}
	return nil
}

func (s *Service) DebugToken(ctx context.Context, version string, token string, accessToken string) (*DebugTokenResponse, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("token is required")
	}
	if strings.TrimSpace(accessToken) == "" {
		return nil, errors.New("debug access token is required")
	}
	if version == "" {
		version = config.DefaultGraphVersion
	}

	values := url.Values{}
	values.Set("input_token", token)
	values.Set("access_token", accessToken)

	out := &DebugTokenResponse{}
	if err := s.doRequest(ctx, http.MethodGet, version, "debug_token", values, "", "", out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) ListProfiles() (map[string]config.Profile, error) {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return nil, err
	}
	clone := make(map[string]config.Profile, len(cfg.Profiles))
	for name, profile := range cfg.Profiles {
		clone[name] = profile
	}
	return clone, nil
}

func (s *Service) UpdateProfileBindings(ctx context.Context, input UpdateProfileBindingsInput) error {
	_ = ctx
	profileName := strings.TrimSpace(input.Profile)
	if profileName == "" {
		return errors.New("profile is required")
	}

	cfg, err := config.Load(s.configPath)
	if err != nil {
		return err
	}
	name, profile, err := cfg.ResolveProfile(profileName)
	if err != nil {
		return err
	}

	pageID := strings.TrimSpace(input.PageID)
	if pageID != "" {
		profile.PageID = pageID
	}
	igUserID := strings.TrimSpace(input.IGUserID)
	if igUserID != "" {
		profile.IGUserID = igUserID
	}
	profile.LastValidatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := cfg.UpsertProfile(name, profile); err != nil {
		return err
	}
	return config.Save(s.configPath, cfg)
}

func normalizedScopesOrDefault(scopes []string, fallback []string) []string {
	out := make([]string, 0, len(scopes))
	seen := map[string]struct{}{}
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, exists := seen[scope]; exists {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	if len(out) > 0 {
		return out
	}
	fallbackOut := make([]string, 0, len(fallback))
	for _, scope := range fallback {
		scope = strings.TrimSpace(scope)
		if scope != "" {
			fallbackOut = append(fallbackOut, scope)
		}
	}
	return fallbackOut
}

func containsScope(scopes []string, scope string) bool {
	for _, current := range scopes {
		if current == scope {
			return true
		}
	}
	return false
}

func normalizedAuthProviderOrDefault(provider string, fallback string) string {
	switch strings.TrimSpace(provider) {
	case AuthProviderFacebookLogin, AuthProviderInstagram, AuthProviderSystemUser, AuthProviderApp:
		return strings.TrimSpace(provider)
	default:
		return fallback
	}
}

func normalizedAuthModeOrDefault(mode string, fallback string) string {
	switch strings.TrimSpace(mode) {
	case AuthModeBoth, AuthModeFacebook, AuthModeInstagram:
		return strings.TrimSpace(mode)
	default:
		return fallback
	}
}

func (s *Service) fetchAppToken(ctx context.Context, version string, appID string, appSecret string) (string, error) {
	values := url.Values{}
	values.Set("client_id", appID)
	values.Set("client_secret", appSecret)
	values.Set("grant_type", "client_credentials")

	response := map[string]any{}
	if err := s.doRequest(ctx, http.MethodGet, version, "oauth/access_token", values, "", "", &response); err != nil {
		return "", err
	}
	token, _ := response["access_token"].(string)
	if strings.TrimSpace(token) == "" {
		return "", errors.New("app token response did not include access_token")
	}
	return token, nil
}

func (s *Service) fetchPageToken(ctx context.Context, version string, pageID string, sourceToken string) (string, error) {
	values := url.Values{}
	values.Set("fields", "access_token")
	values.Set("access_token", sourceToken)

	response := map[string]any{}
	if err := s.doRequest(ctx, http.MethodGet, version, pageID, values, "", "", &response); err != nil {
		return "", err
	}
	token, _ := response["access_token"].(string)
	if strings.TrimSpace(token) == "" {
		return "", errors.New("page token response did not include access_token")
	}
	return token, nil
}

func (s *Service) doFormRequest(ctx context.Context, version string, relPath string, form url.Values, token string, appSecret string, out any) error {
	if version == "" {
		version = config.DefaultGraphVersion
	}
	endpoint, err := url.Parse(s.graphBaseURL)
	if err != nil {
		return fmt.Errorf("parse graph base url: %w", err)
	}
	endpoint.Path = path.Join(endpoint.Path, version, relPath)

	if token != "" {
		form.Set("access_token", token)
	}
	if token != "" && appSecret != "" {
		proof, err := AppSecretProof(token, appSecret)
		if err != nil {
			return err
		}
		form.Set("appsecret_proof", proof)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request %s: %w", endpoint.String(), err)
	}
	return decodeJSONResponse(res, out)
}

func (s *Service) doRequest(ctx context.Context, method string, version string, relPath string, values url.Values, token string, appSecret string, out any) error {
	if version == "" {
		version = config.DefaultGraphVersion
	}
	endpoint, err := url.Parse(s.graphBaseURL)
	if err != nil {
		return fmt.Errorf("parse graph base url: %w", err)
	}
	endpoint.Path = path.Join(endpoint.Path, version, relPath)

	query := url.Values{}
	for key, vals := range values {
		for _, value := range vals {
			query.Add(key, value)
		}
	}

	if token != "" {
		query.Set("access_token", token)
	}
	if token != "" && appSecret != "" {
		proof, err := AppSecretProof(token, appSecret)
		if err != nil {
			return err
		}
		query.Set("appsecret_proof", proof)
	}

	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	res, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request %s: %w", endpoint.String(), err)
	}
	return decodeJSONResponse(res, out)
}

func decodeJSONResponse(res *http.Response, out any) error {
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	var envelope struct {
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Error != nil {
		message, _ := envelope.Error["message"].(string)
		code := envelope.Error["code"]
		subcode := envelope.Error["error_subcode"]
		fbtrace, _ := envelope.Error["fbtrace_id"].(string)
		return fmt.Errorf("meta api error code=%v subcode=%v fbtrace_id=%s: %s", code, subcode, fbtrace, message)
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("request failed with status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode json response: %w", err)
	}
	return nil
}
