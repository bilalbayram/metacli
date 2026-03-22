package linkedin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	sharedauth "github.com/bilalbayram/metacli/internal/auth"
)

const (
	ProviderLinkedIn              = "linkedin"
	DefaultAuthBaseURL            = "https://www.linkedin.com"
	DefaultAPIBaseURL             = "https://api.linkedin.com"
	standardAuthorizationPath     = "/oauth/v2/authorization"
	nativePKCEAuthorizationPath   = "/oauth/native-pkce/authorization"
	tokenEndpointPath             = "/oauth/v2/accessToken"
	meEndpointPath                = "/v2/me"
	restLiProtocolVersion         = "2.0.0"
	defaultRefreshExpirySkew      = 0
	defaultRequestTimeoutFallback = 30 * time.Second
)

type AuthFlow string

const (
	AuthFlowStandard   AuthFlow = "standard"
	AuthFlowNativePKCE AuthFlow = "native-pkce"
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type SecretStore interface {
	Set(ref string, value string) error
	Get(ref string) (string, error)
}

type ProfileStore interface {
	Get(name string) (Profile, error)
	Upsert(name string, profile Profile) error
}

type oauthCodeListener interface {
	RedirectURI() string
	Wait(context.Context, time.Duration) (string, error)
	Close(context.Context) error
}

type Profile struct {
	Provider              string
	LinkedInVersion       string
	ClientID              string
	ClientSecretRef       string
	AccessTokenRef        string
	RefreshTokenRef       string
	Scopes                []string
	AccessTokenExpiresAt  time.Time
	RefreshTokenExpiresAt time.Time
}

type SetupInput struct {
	Profile     string
	RedirectURI string
	Scopes      []string
	AuthFlow    string
	OpenBrowser bool
	Timeout     time.Duration
}

type SetupResult struct {
	ProfileName string        `json:"profile_name"`
	State       string        `json:"state"`
	AuthFlow    string        `json:"auth_flow"`
	AuthURL     string        `json:"auth_url"`
	RedirectURI string        `json:"redirect_uri"`
	Token       TokenBundle   `json:"token"`
	WhoAmI      *WhoAmIResult `json:"whoami,omitempty"`
	Refreshed   bool          `json:"refreshed"`
}

type ValidateResult struct {
	ProfileName string        `json:"profile_name"`
	Refreshed   bool          `json:"refreshed"`
	Token       TokenBundle   `json:"token"`
	WhoAmI      *WhoAmIResult `json:"whoami"`
}

type TokenBundle struct {
	AccessToken           string    `json:"access_token"`
	RefreshToken          string    `json:"refresh_token,omitempty"`
	AccessTokenExpiresAt  time.Time `json:"access_token_expires_at"`
	RefreshTokenExpiresAt time.Time `json:"refresh_token_expires_at,omitempty"`
	Scopes                []string  `json:"scopes"`
}

type WhoAmIResult struct {
	ID        string         `json:"id,omitempty"`
	FirstName string         `json:"first_name,omitempty"`
	LastName  string         `json:"last_name,omitempty"`
	FullName  string         `json:"full_name,omitempty"`
	Raw       map[string]any `json:"raw,omitempty"`
}

type tokenEndpointResponse struct {
	AccessToken           string `json:"access_token"`
	ExpiresIn             int64  `json:"expires_in"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresIn int64  `json:"refresh_token_expires_in"`
	Scope                 string `json:"scope"`
}

type Service struct {
	HTTPClient  HTTPClient
	AuthBaseURL string
	APIBaseURL  string
	Now         func() time.Time
	Profiles    ProfileStore
	Secrets     SecretStore
}

var (
	newPKCE     = sharedauth.NewPKCE
	newState    = sharedauth.NewOAuthState
	newListener = func(redirectURI string, state string) (oauthCodeListener, error) {
		return sharedauth.NewOAuthCallbackListener(redirectURI, state)
	}
	openBrowser = sharedauth.OpenBrowser
)

func NewService(httpClient HTTPClient, authBaseURL string, apiBaseURL string, profiles ProfileStore, secrets SecretStore) *Service {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultRequestTimeoutFallback}
	}
	if strings.TrimSpace(authBaseURL) == "" {
		authBaseURL = DefaultAuthBaseURL
	}
	if strings.TrimSpace(apiBaseURL) == "" {
		apiBaseURL = DefaultAPIBaseURL
	}
	return &Service{
		HTTPClient:  httpClient,
		AuthBaseURL: strings.TrimSuffix(strings.TrimSpace(authBaseURL), "/"),
		APIBaseURL:  strings.TrimSuffix(strings.TrimSpace(apiBaseURL), "/"),
		Now:         time.Now,
		Profiles:    profiles,
		Secrets:     secrets,
	}
}

func (s *Service) Setup(ctx context.Context, profileName string, input SetupInput) (*SetupResult, error) {
	profile, err := s.loadProfile(profileName)
	if err != nil {
		return nil, err
	}
	if err := validateLinkedInProfile(profile); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.RedirectURI) == "" {
		return nil, errors.New("redirect uri is required")
	}
	if input.Timeout <= 0 {
		input.Timeout = 180 * time.Second
	}

	authFlow, err := normalizeAuthFlow(input.AuthFlow)
	if err != nil {
		return nil, err
	}

	clientSecret := ""
	if authFlow == AuthFlowStandard {
		clientSecret, err = s.loadSecret(profile.ClientSecretRef)
		if err != nil {
			return nil, fmt.Errorf("load client secret for profile %q: %w", profileName, err)
		}
	}

	codeVerifier := ""
	codeChallenge := ""
	if authFlow == AuthFlowNativePKCE {
		codeVerifier, codeChallenge, err = newPKCE()
		if err != nil {
			return nil, err
		}
	}

	state, err := newState()
	if err != nil {
		return nil, err
	}

	listener, err := newListener(input.RedirectURI, state)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = listener.Close(context.Background())
	}()

	scopes := normalizeScopes(input.Scopes)
	if len(scopes) == 0 {
		scopes = normalizeScopes(profile.Scopes)
	}

	authURL, err := buildAuthorizationURL(s.AuthBaseURL, profile.ClientID, listener.RedirectURI(), scopes, codeChallenge, state, authFlow)
	if err != nil {
		return nil, err
	}

	if input.OpenBrowser {
		if err := openBrowser(authURL); err != nil {
			return nil, err
		}
	}

	code, err := listener.Wait(ctx, input.Timeout)
	if err != nil {
		return nil, err
	}

	token, err := s.exchangeAuthorizationCode(ctx, profile, code, codeVerifier, listener.RedirectURI(), clientSecret, authFlow)
	if err != nil {
		return nil, err
	}

	if err := s.persistTokenBundle(profileName, profile, token); err != nil {
		return nil, err
	}

	whoami, err := s.WhoAmIWithToken(ctx, profile.LinkedInVersion, token.AccessToken)
	if err != nil {
		return nil, err
	}

	return &SetupResult{
		ProfileName: profileName,
		State:       state,
		AuthFlow:    string(authFlow),
		AuthURL:     authURL,
		RedirectURI: listener.RedirectURI(),
		Token:       token,
		WhoAmI:      whoami,
		Refreshed:   false,
	}, nil
}

func (s *Service) Validate(ctx context.Context, profileName string) (*ValidateResult, error) {
	profile, token, refreshed, err := s.resolveActiveToken(ctx, profileName)
	if err != nil {
		return nil, err
	}
	whoami, err := s.WhoAmIWithToken(ctx, profile.LinkedInVersion, token.AccessToken)
	if err != nil {
		return nil, err
	}
	return &ValidateResult{
		ProfileName: profileName,
		Refreshed:   refreshed,
		Token:       token,
		WhoAmI:      whoami,
	}, nil
}

func (s *Service) Scopes(ctx context.Context, profileName string) ([]string, error) {
	profile, token, _, err := s.resolveActiveToken(ctx, profileName)
	if err != nil {
		return nil, err
	}
	if len(token.Scopes) > 0 {
		return append([]string(nil), token.Scopes...), nil
	}
	return append([]string(nil), profile.Scopes...), nil
}

func (s *Service) WhoAmI(ctx context.Context, profileName string) (*WhoAmIResult, error) {
	profile, token, _, err := s.resolveActiveToken(ctx, profileName)
	if err != nil {
		return nil, err
	}
	return s.WhoAmIWithToken(ctx, profile.LinkedInVersion, token.AccessToken)
}

func (s *Service) WhoAmIWithToken(ctx context.Context, version string, accessToken string) (*WhoAmIResult, error) {
	if strings.TrimSpace(accessToken) == "" {
		return nil, errors.New("access token is required")
	}
	if strings.TrimSpace(version) == "" {
		return nil, errors.New("linkedin version is required")
	}

	var payload map[string]any
	if err := s.doJSONRequest(ctx, http.MethodGet, meEndpointPath, version, nil, accessToken, nil, &payload); err != nil {
		return nil, err
	}
	return parseWhoAmI(payload), nil
}

func (s *Service) exchangeAuthorizationCode(ctx context.Context, profile Profile, code string, codeVerifier string, redirectURI string, clientSecret string, authFlow AuthFlow) (TokenBundle, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", profile.ClientID)
	switch authFlow {
	case AuthFlowStandard:
		if strings.TrimSpace(clientSecret) == "" {
			return TokenBundle{}, errors.New("client secret is required for LinkedIn standard auth flow")
		}
		form.Set("client_secret", clientSecret)
	case AuthFlowNativePKCE:
		if strings.TrimSpace(codeVerifier) == "" {
			return TokenBundle{}, errors.New("code verifier is required for LinkedIn native PKCE auth flow")
		}
		form.Set("code_verifier", codeVerifier)
	default:
		return TokenBundle{}, fmt.Errorf("unsupported LinkedIn auth flow %q", authFlow)
	}

	var payload tokenEndpointResponse
	if err := s.doTokenRequest(ctx, form, &payload); err != nil {
		return TokenBundle{}, err
	}
	return parseTokenBundle(s.Now(), payload)
}

func (s *Service) RefreshAccessToken(ctx context.Context, profileName string) (TokenBundle, error) {
	profile, err := s.loadProfile(profileName)
	if err != nil {
		return TokenBundle{}, err
	}
	if err := validateLinkedInProfile(profile); err != nil {
		return TokenBundle{}, err
	}
	refreshToken, err := s.loadSecret(profile.RefreshTokenRef)
	if err != nil {
		return TokenBundle{}, fmt.Errorf("load refresh token for profile %q: %w", profileName, err)
	}
	clientSecret, err := s.loadSecret(profile.ClientSecretRef)
	if err != nil {
		return TokenBundle{}, fmt.Errorf("load client secret for profile %q: %w", profileName, err)
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", profile.ClientID)
	form.Set("client_secret", clientSecret)

	var payload tokenEndpointResponse
	if err := s.doTokenRequest(ctx, form, &payload); err != nil {
		return TokenBundle{}, err
	}
	bundle, err := parseTokenBundle(s.Now(), payload)
	if err != nil {
		return TokenBundle{}, err
	}
	if bundle.RefreshToken == "" {
		bundle.RefreshToken = refreshToken
	}
	if err := s.persistTokenBundle(profileName, profile, bundle); err != nil {
		return TokenBundle{}, err
	}
	return bundle, nil
}

func (s *Service) resolveActiveToken(ctx context.Context, profileName string) (Profile, TokenBundle, bool, error) {
	profile, err := s.loadProfile(profileName)
	if err != nil {
		return Profile{}, TokenBundle{}, false, err
	}
	if err := validateLinkedInProfile(profile); err != nil {
		return Profile{}, TokenBundle{}, false, err
	}

	accessToken, err := s.loadSecret(profile.AccessTokenRef)
	if err != nil {
		return Profile{}, TokenBundle{}, false, fmt.Errorf("load access token for profile %q: %w", profileName, err)
	}
	refreshToken := ""
	if strings.TrimSpace(profile.RefreshTokenRef) != "" {
		refreshToken, _ = s.loadSecret(profile.RefreshTokenRef)
	}

	bundle := TokenBundle{
		AccessToken:           accessToken,
		RefreshToken:          refreshToken,
		AccessTokenExpiresAt:  profile.AccessTokenExpiresAt,
		RefreshTokenExpiresAt: profile.RefreshTokenExpiresAt,
		Scopes:                append([]string(nil), profile.Scopes...),
	}
	if !needsRefresh(s.Now(), profile.AccessTokenExpiresAt) {
		return profile, bundle, false, nil
	}
	if strings.TrimSpace(profile.RefreshTokenRef) == "" || strings.TrimSpace(refreshToken) == "" {
		return profile, bundle, false, fmt.Errorf("profile %q access token expired and refresh token is unavailable", profileName)
	}

	refreshed, err := s.refreshFromSecret(ctx, profileName, profile, refreshToken)
	if err != nil {
		return profile, bundle, false, err
	}
	return profile, refreshed, true, nil
}

func (s *Service) refreshFromSecret(ctx context.Context, profileName string, profile Profile, refreshToken string) (TokenBundle, error) {
	clientSecret, err := s.loadSecret(profile.ClientSecretRef)
	if err != nil {
		return TokenBundle{}, fmt.Errorf("load client secret for profile %q: %w", profileName, err)
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", profile.ClientID)
	form.Set("client_secret", clientSecret)

	var payload tokenEndpointResponse
	if err := s.doTokenRequest(ctx, form, &payload); err != nil {
		return TokenBundle{}, err
	}
	bundle, err := parseTokenBundle(s.Now(), payload)
	if err != nil {
		return TokenBundle{}, err
	}
	if bundle.RefreshToken == "" {
		bundle.RefreshToken = refreshToken
	}
	if err := s.persistTokenBundle(profileName, profile, bundle); err != nil {
		return TokenBundle{}, err
	}
	return bundle, nil
}

func (s *Service) persistTokenBundle(profileName string, profile Profile, bundle TokenBundle) error {
	if strings.TrimSpace(profile.AccessTokenRef) == "" {
		return fmt.Errorf("profile %q access_token_ref is required", profileName)
	}
	if err := s.Secrets.Set(profile.AccessTokenRef, bundle.AccessToken); err != nil {
		return err
	}
	if strings.TrimSpace(profile.RefreshTokenRef) != "" && strings.TrimSpace(bundle.RefreshToken) != "" {
		if err := s.Secrets.Set(profile.RefreshTokenRef, bundle.RefreshToken); err != nil {
			return err
		}
	}
	profile.AccessTokenExpiresAt = bundle.AccessTokenExpiresAt
	profile.RefreshTokenExpiresAt = bundle.RefreshTokenExpiresAt
	profile.Scopes = append([]string(nil), bundle.Scopes...)
	return s.saveProfile(profileName, profile)
}

func (s *Service) doTokenRequest(ctx context.Context, form url.Values, out any) error {
	endpoint, err := url.Parse(s.AuthBaseURL)
	if err != nil {
		return fmt.Errorf("parse linkedin auth base url: %w", err)
	}
	endpoint.Path = joinLinkedInPath(endpoint.Path, tokenEndpointPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build linkedin token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	return s.doRequest(req, out)
}

func (s *Service) doJSONRequest(ctx context.Context, method string, path string, version string, query map[string]string, accessToken string, body any, out any) error {
	endpoint, err := url.Parse(s.APIBaseURL)
	if err != nil {
		return fmt.Errorf("parse linkedin api base url: %w", err)
	}
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/") + path
	values := url.Values{}
	for key, value := range query {
		if strings.TrimSpace(value) == "" {
			continue
		}
		values.Set(key, value)
	}
	endpoint.RawQuery = values.Encode()

	var bodyReader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal linkedin api request body: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bodyReader)
	if err != nil {
		return fmt.Errorf("build linkedin api request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("X-Restli-Protocol-Version", restLiProtocolVersion)
	if strings.TrimSpace(version) != "" {
		req.Header.Set("LinkedIn-Version", version)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return s.doRequest(req, out)
}

func (s *Service) doRequest(req *http.Request, out any) error {
	client := s.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultRequestTimeoutFallback}
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send request %s: %w", req.URL.String(), err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response %s: %w", req.URL.String(), err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return classifyLinkedInError(resp.StatusCode, body)
	}
	if out == nil {
		return nil
	}
	if len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode response %s: %w", req.URL.String(), err)
	}
	return nil
}

func buildAuthorizationURL(authBaseURL string, clientID string, redirectURI string, scopes []string, codeChallenge string, state string, authFlow AuthFlow) (string, error) {
	if strings.TrimSpace(clientID) == "" {
		return "", errors.New("client id is required")
	}
	if strings.TrimSpace(redirectURI) == "" {
		return "", errors.New("redirect uri is required")
	}
	if strings.TrimSpace(state) == "" {
		return "", errors.New("state is required")
	}

	endpoint, err := url.Parse(strings.TrimRight(strings.TrimSpace(authBaseURL), "/"))
	if err != nil {
		return "", fmt.Errorf("parse linkedin auth base url: %w", err)
	}
	switch authFlow {
	case AuthFlowStandard:
		endpoint.Path = standardAuthorizationPath
	case AuthFlowNativePKCE:
		if strings.TrimSpace(codeChallenge) == "" {
			return "", errors.New("code challenge is required")
		}
		endpoint.Path = nativePKCEAuthorizationPath
	default:
		return "", fmt.Errorf("unsupported LinkedIn auth flow %q", authFlow)
	}

	values := url.Values{}
	values.Set("response_type", "code")
	values.Set("client_id", clientID)
	values.Set("redirect_uri", redirectURI)
	values.Set("state", state)
	if authFlow == AuthFlowNativePKCE {
		values.Set("code_challenge", codeChallenge)
		values.Set("code_challenge_method", "S256")
	}
	if len(scopes) > 0 {
		values.Set("scope", strings.Join(normalizeScopes(scopes), " "))
	}
	endpoint.RawQuery = values.Encode()
	return endpoint.String(), nil
}

func normalizeAuthFlow(raw string) (AuthFlow, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(AuthFlowStandard):
		return AuthFlowStandard, nil
	case string(AuthFlowNativePKCE):
		return AuthFlowNativePKCE, nil
	default:
		return "", fmt.Errorf("unsupported LinkedIn auth flow %q (expected %q or %q)", raw, AuthFlowStandard, AuthFlowNativePKCE)
	}
}

func parseTokenBundle(now time.Time, response tokenEndpointResponse) (TokenBundle, error) {
	if strings.TrimSpace(response.AccessToken) == "" {
		return TokenBundle{}, errors.New("linkedin token response did not include access_token")
	}
	bundle := TokenBundle{
		AccessToken:  response.AccessToken,
		RefreshToken: strings.TrimSpace(response.RefreshToken),
		Scopes:       parseScopeList(response.Scope),
	}
	if response.ExpiresIn > 0 {
		bundle.AccessTokenExpiresAt = now.UTC().Add(time.Duration(response.ExpiresIn) * time.Second)
	}
	if response.RefreshTokenExpiresIn > 0 {
		bundle.RefreshTokenExpiresAt = now.UTC().Add(time.Duration(response.RefreshTokenExpiresIn) * time.Second)
	}
	return bundle, nil
}

func parseScopeList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ' ', ',', '\t', '\n', '\r':
			return true
		default:
			return false
		}
	})
	return normalizeScopes(fields)
}

func normalizeScopes(scopes []string) []string {
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
	return out
}

func needsRefresh(now time.Time, expiresAt time.Time) bool {
	if expiresAt.IsZero() {
		return false
	}
	return !now.UTC().Before(expiresAt.UTC())
}

func parseWhoAmI(payload map[string]any) *WhoAmIResult {
	if len(payload) == 0 {
		return &WhoAmIResult{Raw: map[string]any{}}
	}
	result := &WhoAmIResult{
		Raw: cloneMap(payload),
	}
	if id, _ := payload["id"].(string); strings.TrimSpace(id) != "" {
		result.ID = strings.TrimSpace(id)
	}
	result.FirstName = pickLinkedInName(payload, "first")
	result.LastName = pickLinkedInName(payload, "last")
	if first, last := strings.TrimSpace(result.FirstName), strings.TrimSpace(result.LastName); first != "" || last != "" {
		result.FullName = strings.TrimSpace(strings.Join([]string{first, last}, " "))
	}
	if result.FullName == "" {
		if full, _ := payload["localizedFullName"].(string); strings.TrimSpace(full) != "" {
			result.FullName = strings.TrimSpace(full)
		}
	}
	return result
}

func pickLinkedInName(payload map[string]any, kind string) string {
	if payload == nil {
		return ""
	}
	flatKey := "localizedFirstName"
	if kind == "last" {
		flatKey = "localizedLastName"
	}
	if flat, _ := payload[flatKey].(string); strings.TrimSpace(flat) != "" {
		return strings.TrimSpace(flat)
	}
	raw, _ := payload[kind+"Name"].(map[string]any)
	if len(raw) == 0 {
		return ""
	}
	if localized, _ := raw["localized"].(map[string]any); len(localized) > 0 {
		if preferred := preferredLocaleKey(raw); preferred != "" {
			if value, ok := localized[preferred].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
		for _, value := range localized {
			if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

func preferredLocaleKey(raw map[string]any) string {
	preferred, _ := raw["preferredLocale"].(map[string]any)
	if len(preferred) == 0 {
		return ""
	}
	lang, _ := preferred["language"].(string)
	country, _ := preferred["country"].(string)
	lang = strings.TrimSpace(lang)
	country = strings.TrimSpace(country)
	if lang == "" || country == "" {
		return ""
	}
	return strings.ToLower(lang) + "_" + strings.ToUpper(country)
}

func cloneMap(source map[string]any) map[string]any {
	if len(source) == 0 {
		return map[string]any{}
	}
	clone := make(map[string]any, len(source))
	for key, value := range source {
		clone[key] = cloneAny(value)
	}
	return clone
}

func validateLinkedInProfile(profile Profile) error {
	if strings.TrimSpace(profile.Provider) != "" && strings.TrimSpace(profile.Provider) != ProviderLinkedIn {
		return fmt.Errorf("unsupported provider %q", profile.Provider)
	}
	if strings.TrimSpace(profile.ClientID) == "" {
		return errors.New("client id is required")
	}
	if strings.TrimSpace(profile.ClientSecretRef) == "" {
		return errors.New("client secret ref is required")
	}
	if strings.TrimSpace(profile.AccessTokenRef) == "" {
		return errors.New("access token ref is required")
	}
	if strings.TrimSpace(profile.RefreshTokenRef) == "" {
		return errors.New("refresh token ref is required")
	}
	if strings.TrimSpace(profile.LinkedInVersion) == "" {
		return errors.New("linkedin version is required")
	}
	return nil
}

func (s *Service) loadProfile(profileName string) (Profile, error) {
	if s.Profiles == nil {
		return Profile{}, errors.New("profile store is required")
	}
	profileName = strings.TrimSpace(profileName)
	if profileName == "" {
		return Profile{}, errors.New("profile name is required")
	}
	profile, err := s.Profiles.Get(profileName)
	if err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func (s *Service) saveProfile(profileName string, profile Profile) error {
	if s.Profiles == nil {
		return nil
	}
	return s.Profiles.Upsert(strings.TrimSpace(profileName), profile)
}

func (s *Service) loadSecret(ref string) (string, error) {
	if s.Secrets == nil {
		return "", errors.New("secret store is required")
	}
	return s.Secrets.Get(ref)
}

func joinLinkedInPath(basePath string, childPath string) string {
	basePath = strings.TrimSuffix(basePath, "/")
	childPath = "/" + strings.TrimPrefix(childPath, "/")
	return basePath + childPath
}
