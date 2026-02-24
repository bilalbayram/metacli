package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/bilalbayram/metacli/internal/auth"
	"github.com/bilalbayram/metacli/internal/config"
	"github.com/spf13/cobra"
)

const (
	defaultAuthListenAddr   = "127.0.0.1:53682"
	defaultAuthCallbackPath = "/oauth/callback"
	defaultAuthTimeout      = 180 * time.Second
	defaultAuthPreflightTTL = 72 * time.Hour
)

type authCLIService interface {
	AddSystemUser(context.Context, auth.AddSystemUserInput) error
	AddUser(context.Context, auth.AddUserInput) error
	DerivePageToken(context.Context, auth.PageTokenInput) error
	SetAppToken(context.Context, auth.SetAppTokenInput) error
	ExchangeOAuthCode(context.Context, auth.ExchangeCodeInput) (string, error)
	ExchangeLongLivedUserToken(context.Context, auth.ExchangeLongLivedUserTokenInput) (auth.LongLivedToken, error)
	EnsureValid(context.Context, string, time.Duration, []string) (*auth.DebugTokenMetadata, error)
	ValidateProfile(context.Context, string) (*auth.DebugTokenResponse, error)
	RotateProfile(context.Context, string) error
	DebugToken(context.Context, string, string, string) (*auth.DebugTokenResponse, error)
	ListProfiles() (map[string]config.Profile, error)
	DiscoverPagesAndIGBusinessAccounts(context.Context, string) ([]auth.DiscoveredPage, error)
	UpdateProfileBindings(context.Context, auth.UpdateProfileBindingsInput) error
}

var newAuthCLIService = newAuthService
var newAuthPKCE = auth.NewPKCE
var newAuthOAuthState = auth.NewOAuthState
var newAuthOAuthListener = func(redirectURI string, state string) (oauthCodeListener, error) {
	return auth.NewOAuthCallbackListener(redirectURI, state)
}
var buildAuthOAuthURLWithState = auth.BuildOAuthURLWithState
var openAuthBrowser = auth.OpenBrowser

type oauthCodeListener interface {
	RedirectURI() string
	Wait(context.Context, time.Duration) (string, error)
	Close(context.Context) error
}

type oauthLoginInput struct {
	Profile      string
	AppID        string
	AppSecret    string
	Scopes       []string
	ListenAddr   string
	RedirectURI  string
	Timeout      time.Duration
	OpenBrowser  bool
	Version      string
	AuthProvider string
	AuthMode     string
	PageID       string
	IGUserID     string
}

type oauthLoginResult struct {
	AuthURL       string
	RedirectURI   string
	ExpiresAt     time.Time
	Scopes        []string
	TokenType     string
	AuthProvider  string
	AuthMode      string
	Profile       string
	TokenIssuedAt time.Time
}

func NewAuthCommand(runtime Runtime) *cobra.Command {
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication and token profile management",
	}

	authCmd.AddCommand(newAuthAddCommand(runtime))
	authCmd.AddCommand(newAuthSetupCommand(runtime))
	authCmd.AddCommand(newAuthLoginCommand(runtime))
	authCmd.AddCommand(newAuthLoginManualCommand(runtime))
	authCmd.AddCommand(newAuthDiscoverCommand(runtime))
	authCmd.AddCommand(newAuthPageTokenCommand(runtime))
	authCmd.AddCommand(newAuthAppTokenCommand(runtime))
	authCmd.AddCommand(newAuthValidateCommand(runtime))
	authCmd.AddCommand(newAuthRotateCommand(runtime))
	authCmd.AddCommand(newAuthDebugTokenCommand(runtime))
	authCmd.AddCommand(newAuthListCommand(runtime))
	return authCmd
}

func newAuthAddCommand(runtime Runtime) *cobra.Command {
	addCmd := &cobra.Command{
		Use:   "add",
		Short: "Add credentials",
	}

	var (
		profile    string
		businessID string
		appID      string
		token      string
		appSecret  string
	)

	systemUserCmd := &cobra.Command{
		Use:   "system-user",
		Short: "Add a system-user profile",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolvedProfile, err := resolveAuthProfile(runtime, profile)
			if err != nil {
				return err
			}

			svc, err := newAuthCLIService()
			if err != nil {
				return err
			}
			if err := svc.AddSystemUser(cmd.Context(), auth.AddSystemUserInput{
				Profile:    resolvedProfile,
				BusinessID: businessID,
				AppID:      appID,
				Token:      token,
				AppSecret:  appSecret,
				AuthMode:   auth.AuthModeBoth,
				Scopes:     []string{"ads_management", "business_management"},
			}); err != nil {
				return err
			}
			return writeSuccess(cmd, runtime, "meta auth add system-user", map[string]any{
				"status":  "ok",
				"profile": resolvedProfile,
			}, nil, nil)
		},
	}
	systemUserCmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	systemUserCmd.Flags().StringVar(&businessID, "business-id", "", "Business ID")
	systemUserCmd.Flags().StringVar(&appID, "app-id", "", "Meta App ID")
	systemUserCmd.Flags().StringVar(&token, "token", "", "System-user access token")
	systemUserCmd.Flags().StringVar(&appSecret, "app-secret", "", "Meta App Secret")
	mustMarkFlagRequired(systemUserCmd, "business-id")
	mustMarkFlagRequired(systemUserCmd, "app-id")
	mustMarkFlagRequired(systemUserCmd, "token")
	mustMarkFlagRequired(systemUserCmd, "app-secret")
	addCmd.AddCommand(systemUserCmd)
	return addCmd
}

func newAuthSetupCommand(runtime Runtime) *cobra.Command {
	var (
		profile        string
		appID          string
		appSecret      string
		redirectURI    string
		mode           string
		scopePack      string
		listenAddr     string
		timeout        time.Duration
		openBrowser    bool
		pageID         string
		igUserID       string
		nonInteractive bool
	)

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Guided auth setup with automated browser callback",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolvedProfile, err := resolveAuthProfile(runtime, profile)
			if err != nil {
				return err
			}
			resolvedMode, err := normalizeAuthMode(mode)
			if err != nil {
				return err
			}
			scopes, err := scopePackScopes(scopePack, resolvedMode)
			if err != nil {
				return err
			}

			svc, err := newAuthCLIService()
			if err != nil {
				return err
			}

			loginResult, err := runOAuthAutoLogin(cmd, svc, oauthLoginInput{
				Profile:      resolvedProfile,
				AppID:        appID,
				AppSecret:    appSecret,
				Scopes:       scopes,
				ListenAddr:   listenAddr,
				RedirectURI:  redirectURI,
				Timeout:      timeout,
				OpenBrowser:  openBrowser,
				Version:      config.DefaultGraphVersion,
				AuthProvider: authProviderFromMode(resolvedMode),
				AuthMode:     resolvedMode,
				PageID:       pageID,
				IGUserID:     igUserID,
			})
			if err != nil {
				return err
			}

			pages, err := svc.DiscoverPagesAndIGBusinessAccounts(cmd.Context(), resolvedProfile)
			if err != nil {
				return err
			}

			selectedPageID := strings.TrimSpace(pageID)
			if selectedPageID == "" {
				if firstPage, ok := firstDiscoveredPageID(pages); ok {
					selectedPageID = firstPage
				}
			}
			selectedIGUserID := strings.TrimSpace(igUserID)
			if selectedIGUserID == "" {
				if firstIG, ok := firstDiscoveredIGUserID(pages); ok {
					selectedIGUserID = firstIG
				}
			}

			if selectedPageID != "" || selectedIGUserID != "" {
				if err := svc.UpdateProfileBindings(cmd.Context(), auth.UpdateProfileBindingsInput{
					Profile:  resolvedProfile,
					PageID:   selectedPageID,
					IGUserID: selectedIGUserID,
				}); err != nil {
					return err
				}
			}

			return writeSuccess(cmd, runtime, "meta auth setup", map[string]any{
				"status":           "ok",
				"profile":          resolvedProfile,
				"mode":             resolvedMode,
				"scope_pack":       scopePack,
				"scopes":           scopes,
				"auth_url":         loginResult.AuthURL,
				"redirect_uri":     loginResult.RedirectURI,
				"token_expires_at": loginResult.ExpiresAt.Format(time.RFC3339),
				"pages":            pages,
				"selected_page_id": selectedPageID,
				"selected_ig_user": selectedIGUserID,
				"non_interactive":  nonInteractive,
			}, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&appID, "app-id", "", "Meta App ID")
	cmd.Flags().StringVar(&appSecret, "app-secret", "", "Meta App Secret")
	cmd.Flags().StringVar(&mode, "mode", auth.AuthModeBoth, "Auth mode: both|facebook|instagram")
	cmd.Flags().StringVar(&scopePack, "scope-pack", "solo_smb", "Scope pack: solo_smb|ads_only|ig_publish")
	cmd.Flags().StringVar(&listenAddr, "listen", defaultAuthListenAddr, "OAuth callback listener host:port")
	cmd.Flags().StringVar(&redirectURI, "redirect-uri", "", "OAuth redirect URI override (recommended for https tunnel domains)")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultAuthTimeout, "OAuth callback timeout")
	cmd.Flags().BoolVar(&openBrowser, "open-browser", true, "Open browser automatically")
	cmd.Flags().StringVar(&pageID, "page-id", "", "Optional page id binding")
	cmd.Flags().StringVar(&igUserID, "ig-user-id", "", "Optional Instagram user id binding")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Run without interactive prompts")
	mustMarkFlagRequired(cmd, "app-id")
	mustMarkFlagRequired(cmd, "app-secret")
	return cmd
}

func newAuthLoginCommand(runtime Runtime) *cobra.Command {
	var (
		profile     string
		appID       string
		appSecret   string
		scopesRaw   string
		listenAddr  string
		redirectURI string
		timeout     time.Duration
		openBrowser bool
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate a user with browser callback OAuth flow",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolvedProfile, err := resolveAuthProfile(runtime, profile)
			if err != nil {
				return err
			}
			scopes := csvToSlice(scopesRaw)
			if len(scopes) == 0 {
				return errors.New("scopes are required")
			}
			svc, err := newAuthCLIService()
			if err != nil {
				return err
			}

			result, err := runOAuthAutoLogin(cmd, svc, oauthLoginInput{
				Profile:      resolvedProfile,
				AppID:        appID,
				AppSecret:    appSecret,
				Scopes:       scopes,
				ListenAddr:   listenAddr,
				RedirectURI:  redirectURI,
				Timeout:      timeout,
				OpenBrowser:  openBrowser,
				Version:      config.DefaultGraphVersion,
				AuthProvider: auth.AuthProviderFacebookLogin,
				AuthMode:     auth.AuthModeBoth,
			})
			if err != nil {
				return err
			}

			return writeSuccess(cmd, runtime, "meta auth login", map[string]any{
				"status":           "ok",
				"profile":          resolvedProfile,
				"auth_url":         result.AuthURL,
				"redirect_uri":     result.RedirectURI,
				"scopes":           result.Scopes,
				"token_expires_at": result.ExpiresAt.Format(time.RFC3339),
			}, nil, nil)
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&appID, "app-id", "", "Meta App ID")
	cmd.Flags().StringVar(&appSecret, "app-secret", "", "Meta App Secret")
	cmd.Flags().StringVar(&scopesRaw, "scopes", "", "Comma-separated OAuth scopes")
	cmd.Flags().StringVar(&listenAddr, "listen", defaultAuthListenAddr, "OAuth callback listener host:port")
	cmd.Flags().StringVar(&redirectURI, "redirect-uri", "", "OAuth redirect URI override (recommended for https tunnel domains)")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultAuthTimeout, "OAuth callback timeout")
	cmd.Flags().BoolVar(&openBrowser, "open-browser", true, "Open browser automatically")
	mustMarkFlagRequired(cmd, "app-id")
	mustMarkFlagRequired(cmd, "app-secret")
	mustMarkFlagRequired(cmd, "scopes")
	return cmd
}

func newAuthLoginManualCommand(runtime Runtime) *cobra.Command {
	var (
		profile     string
		appID       string
		appSecret   string
		redirectURI string
		scopesRaw   string
		code        string
	)

	cmd := &cobra.Command{
		Use:   "login-manual",
		Short: "Authenticate with explicit authorization code",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolvedProfile, err := resolveAuthProfile(runtime, profile)
			if err != nil {
				return err
			}
			scopes := csvToSlice(scopesRaw)
			if len(scopes) == 0 {
				return errors.New("scopes are required")
			}

			svc, err := newAuthCLIService()
			if err != nil {
				return err
			}

			shortToken, err := svc.ExchangeOAuthCode(cmd.Context(), auth.ExchangeCodeInput{
				AppID:       appID,
				RedirectURI: redirectURI,
				Code:        code,
				Version:     config.DefaultGraphVersion,
			})
			if err != nil {
				return err
			}

			longLived, err := svc.ExchangeLongLivedUserToken(cmd.Context(), auth.ExchangeLongLivedUserTokenInput{
				AppID:           appID,
				AppSecret:       appSecret,
				ShortLivedToken: shortToken,
				Version:         config.DefaultGraphVersion,
			})
			if err != nil {
				return err
			}

			debugResp, err := svc.DebugToken(cmd.Context(), config.DefaultGraphVersion, longLived.AccessToken, fmt.Sprintf("%s|%s", appID, appSecret))
			if err != nil {
				return err
			}
			metadata, err := auth.NormalizeDebugTokenMetadata(debugResp)
			if err != nil {
				return err
			}
			if !metadata.IsValid {
				return errors.New("profile token is invalid")
			}

			now := time.Now().UTC()
			expiresAt := metadata.ExpiresAt
			if expiresAt.IsZero() {
				expiresAt = now.Add(60 * 24 * time.Hour)
			}

			if err := svc.AddUser(cmd.Context(), auth.AddUserInput{
				Profile:         resolvedProfile,
				AppID:           appID,
				Token:           longLived.AccessToken,
				AppSecret:       appSecret,
				AuthProvider:    auth.AuthProviderFacebookLogin,
				AuthMode:        auth.AuthModeBoth,
				Scopes:          metadata.Scopes,
				IssuedAt:        now.Format(time.RFC3339),
				ExpiresAt:       expiresAt.Format(time.RFC3339),
				LastValidatedAt: now.Format(time.RFC3339),
			}); err != nil {
				return err
			}

			return writeSuccess(cmd, runtime, "meta auth login-manual", map[string]any{
				"status":           "ok",
				"profile":          resolvedProfile,
				"redirect_uri":     redirectURI,
				"scopes":           metadata.Scopes,
				"token_expires_at": expiresAt.Format(time.RFC3339),
			}, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&appID, "app-id", "", "Meta App ID")
	cmd.Flags().StringVar(&appSecret, "app-secret", "", "Meta App Secret")
	cmd.Flags().StringVar(&redirectURI, "redirect-uri", "", "OAuth redirect URI")
	cmd.Flags().StringVar(&scopesRaw, "scopes", "", "Comma-separated OAuth scopes")
	cmd.Flags().StringVar(&code, "code", "", "OAuth authorization code")
	mustMarkFlagRequired(cmd, "app-id")
	mustMarkFlagRequired(cmd, "app-secret")
	mustMarkFlagRequired(cmd, "redirect-uri")
	mustMarkFlagRequired(cmd, "scopes")
	mustMarkFlagRequired(cmd, "code")
	return cmd
}

func newAuthDiscoverCommand(runtime Runtime) *cobra.Command {
	var (
		profile string
		mode    string
	)

	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Discover pages and Instagram account bindings",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolvedProfile, err := resolveAuthProfile(runtime, profile)
			if err != nil {
				return err
			}
			resolvedMode, err := normalizeDiscoverMode(mode)
			if err != nil {
				return err
			}

			svc, err := newAuthCLIService()
			if err != nil {
				return err
			}
			pages, err := svc.DiscoverPagesAndIGBusinessAccounts(cmd.Context(), resolvedProfile)
			if err != nil {
				return err
			}

			data := map[string]any{
				"status":  "ok",
				"profile": resolvedProfile,
				"mode":    resolvedMode,
			}
			switch resolvedMode {
			case "pages":
				data["pages"] = pages
			case "ig":
				data["instagram_accounts"] = flattenIGAccounts(pages)
			}

			return writeSuccess(cmd, runtime, "meta auth discover", data, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&mode, "mode", "", "Discovery mode: pages|ig")
	mustMarkFlagRequired(cmd, "mode")
	return cmd
}

func newAuthPageTokenCommand(runtime Runtime) *cobra.Command {
	var (
		profile       string
		pageID        string
		sourceProfile string
	)

	cmd := &cobra.Command{
		Use:   "page-token",
		Short: "Derive and store a page token using a source profile token",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolvedProfile, err := resolveAuthProfile(runtime, profile)
			if err != nil {
				return err
			}

			svc, err := newAuthCLIService()
			if err != nil {
				return err
			}
			if err := svc.DerivePageToken(cmd.Context(), auth.PageTokenInput{
				Profile:       resolvedProfile,
				PageID:        pageID,
				SourceProfile: sourceProfile,
			}); err != nil {
				return err
			}

			return writeSuccess(cmd, runtime, "meta auth page-token", map[string]any{
				"status":         "ok",
				"profile":        resolvedProfile,
				"source_profile": sourceProfile,
			}, nil, nil)
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Target profile name")
	cmd.Flags().StringVar(&pageID, "page-id", "", "Page ID")
	cmd.Flags().StringVar(&sourceProfile, "source-profile", "", "Source profile with user/system token")
	mustMarkFlagRequired(cmd, "page-id")
	mustMarkFlagRequired(cmd, "source-profile")
	return cmd
}

func newAuthAppTokenCommand(runtime Runtime) *cobra.Command {
	appTokenCmd := &cobra.Command{
		Use:   "app-token",
		Short: "Manage app tokens",
	}

	var (
		profile   string
		appID     string
		appSecret string
	)
	setCmd := &cobra.Command{
		Use:   "set",
		Short: "Create and store an app token from app credentials",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolvedProfile, err := resolveAuthProfile(runtime, profile)
			if err != nil {
				return err
			}
			svc, err := newAuthCLIService()
			if err != nil {
				return err
			}
			if err := svc.SetAppToken(cmd.Context(), auth.SetAppTokenInput{
				Profile:   resolvedProfile,
				AppID:     appID,
				AppSecret: appSecret,
				AuthMode:  auth.AuthModeBoth,
				Scopes:    []string{"ads_management"},
			}); err != nil {
				return err
			}
			return writeSuccess(cmd, runtime, "meta auth app-token set", map[string]any{
				"status":  "ok",
				"profile": resolvedProfile,
			}, nil, nil)
		},
	}
	setCmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	setCmd.Flags().StringVar(&appID, "app-id", "", "Meta App ID")
	setCmd.Flags().StringVar(&appSecret, "app-secret", "", "Meta App Secret")
	mustMarkFlagRequired(setCmd, "app-id")
	mustMarkFlagRequired(setCmd, "app-secret")
	appTokenCmd.AddCommand(setCmd)
	return appTokenCmd
}

func newAuthValidateCommand(runtime Runtime) *cobra.Command {
	var (
		profile       string
		minTTL        time.Duration
		requireScopes string
	)
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate token configured for a profile",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolvedProfile, err := resolveAuthProfile(runtime, profile)
			if err != nil {
				return err
			}
			required := csvToSlice(requireScopes)

			svc, err := newAuthCLIService()
			if err != nil {
				return err
			}

			metadata, err := svc.EnsureValid(cmd.Context(), resolvedProfile, minTTL, required)
			if err != nil {
				return err
			}
			resp, err := svc.ValidateProfile(cmd.Context(), resolvedProfile)
			if err != nil {
				return err
			}

			return writeSuccess(cmd, runtime, "meta auth validate", map[string]any{
				"status":         "ok",
				"profile":        resolvedProfile,
				"min_ttl":        minTTL.String(),
				"require_scopes": required,
				"metadata": map[string]any{
					"is_valid":   metadata.IsValid,
					"scopes":     metadata.Scopes,
					"expires_at": metadata.ExpiresAt.Format(time.RFC3339),
				},
				"debug": resp.Data,
			}, nil, nil)
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().DurationVar(&minTTL, "min-ttl", defaultAuthPreflightTTL, "Minimum remaining token TTL (for example 30m, 12h)")
	cmd.Flags().StringVar(&requireScopes, "require-scopes", "", "Comma-separated scopes that must be present")
	return cmd
}

func newAuthRotateCommand(runtime Runtime) *cobra.Command {
	var profile string
	cmd := &cobra.Command{
		Use:   "rotate",
		Short: "Rotate token for a profile",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolvedProfile, err := resolveAuthProfile(runtime, profile)
			if err != nil {
				return err
			}
			svc, err := newAuthCLIService()
			if err != nil {
				return err
			}
			if err := svc.RotateProfile(cmd.Context(), resolvedProfile); err != nil {
				return err
			}
			return writeSuccess(cmd, runtime, "meta auth rotate", map[string]any{
				"status":  "ok",
				"profile": resolvedProfile,
			}, nil, nil)
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	return cmd
}

func newAuthDebugTokenCommand(runtime Runtime) *cobra.Command {
	var (
		profile string
		token   string
	)
	cmd := &cobra.Command{
		Use:   "debug-token",
		Short: "Debug token metadata via /debug_token",
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, err := newAuthCLIService()
			if err != nil {
				return err
			}
			resolvedProfile, err := resolveAuthProfile(runtime, profile)
			if err != nil {
				return err
			}

			var (
				version     string
				accessToken string
			)
			if token == "" {
				cfgPath, err := config.DefaultPath()
				if err != nil {
					return err
				}
				cfg, err := config.Load(cfgPath)
				if err != nil {
					return err
				}
				_, selected, err := cfg.ResolveProfile(resolvedProfile)
				if err != nil {
					return err
				}
				tokenStore := auth.NewKeychainStore()
				token, err = tokenStore.Get(selected.TokenRef)
				if err != nil {
					return err
				}
				version = selected.GraphVersion
				accessToken = token
				if selected.AppID != "" && selected.AppSecretRef != "" {
					secret, err := tokenStore.Get(selected.AppSecretRef)
					if err != nil {
						return err
					}
					accessToken = fmt.Sprintf("%s|%s", selected.AppID, secret)
				}
			} else {
				version = config.DefaultGraphVersion
				accessToken = token
			}

			resp, err := svc.DebugToken(cmd.Context(), version, token, accessToken)
			if err != nil {
				return err
			}
			return writeSuccess(cmd, runtime, "meta auth debug-token", map[string]any{
				"status": "ok",
				"debug":  resp.Data,
			}, nil, nil)
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&token, "token", "", "Token value (optional)")
	return cmd
}

func newAuthListCommand(runtime Runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List configured auth profiles",
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, err := newAuthCLIService()
			if err != nil {
				return err
			}
			profiles, err := svc.ListProfiles()
			if err != nil {
				return err
			}

			return writeSuccess(cmd, runtime, "meta auth list", map[string]any{
				"status":   "ok",
				"profiles": profiles,
			}, nil, nil)
		},
	}
	return cmd
}

func runOAuthAutoLogin(cmd *cobra.Command, svc authCLIService, input oauthLoginInput) (oauthLoginResult, error) {
	if strings.TrimSpace(input.Profile) == "" {
		return oauthLoginResult{}, errors.New("profile is required")
	}
	if strings.TrimSpace(input.AppID) == "" {
		return oauthLoginResult{}, errors.New("app id is required")
	}
	if strings.TrimSpace(input.AppSecret) == "" {
		return oauthLoginResult{}, errors.New("app secret is required")
	}
	if len(input.Scopes) == 0 {
		return oauthLoginResult{}, errors.New("scopes are required")
	}
	if input.Timeout <= 0 {
		return oauthLoginResult{}, errors.New("timeout must be greater than zero")
	}
	listenerURI, err := localCallbackRedirectURI(input.ListenAddr)
	if err != nil {
		return oauthLoginResult{}, err
	}

	verifier, challenge, err := newAuthPKCE()
	if err != nil {
		return oauthLoginResult{}, err
	}
	state, err := newAuthOAuthState()
	if err != nil {
		return oauthLoginResult{}, err
	}
	listener, err := newAuthOAuthListener(listenerURI, state)
	if err != nil {
		return oauthLoginResult{}, err
	}
	defer listener.Close(context.Background())

	localResolvedRedirectURI := listener.RedirectURI()
	oauthRedirectURI := strings.TrimSpace(input.RedirectURI)
	if oauthRedirectURI == "" {
		oauthRedirectURI = localResolvedRedirectURI
	}
	if err := validateOAuthRedirectURI(oauthRedirectURI); err != nil {
		return oauthLoginResult{}, err
	}

	authURL, err := buildAuthOAuthURLWithState(input.AppID, oauthRedirectURI, input.Scopes, challenge, state, input.Version)
	if err != nil {
		return oauthLoginResult{}, err
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Open this URL and complete login:\n%s\n", authURL)
	if input.OpenBrowser {
		if err := openAuthBrowser(authURL); err != nil {
			return oauthLoginResult{}, err
		}
	}

	code, err := listener.Wait(cmd.Context(), input.Timeout)
	if err != nil {
		return oauthLoginResult{}, err
	}

	shortToken, err := svc.ExchangeOAuthCode(cmd.Context(), auth.ExchangeCodeInput{
		AppID:        input.AppID,
		RedirectURI:  oauthRedirectURI,
		Code:         code,
		CodeVerifier: verifier,
		Version:      input.Version,
	})
	if err != nil {
		return oauthLoginResult{}, err
	}
	longLived, err := svc.ExchangeLongLivedUserToken(cmd.Context(), auth.ExchangeLongLivedUserTokenInput{
		AppID:           input.AppID,
		AppSecret:       input.AppSecret,
		ShortLivedToken: shortToken,
		Version:         input.Version,
	})
	if err != nil {
		return oauthLoginResult{}, err
	}

	debugResp, err := svc.DebugToken(cmd.Context(), input.Version, longLived.AccessToken, fmt.Sprintf("%s|%s", input.AppID, input.AppSecret))
	if err != nil {
		return oauthLoginResult{}, err
	}
	metadata, err := auth.NormalizeDebugTokenMetadata(debugResp)
	if err != nil {
		return oauthLoginResult{}, err
	}
	if !metadata.IsValid {
		return oauthLoginResult{}, errors.New("profile token is invalid")
	}

	now := time.Now().UTC()
	expiresAt := metadata.ExpiresAt
	if expiresAt.IsZero() {
		if longLived.ExpiresInSeconds > 0 {
			expiresAt = now.Add(time.Duration(longLived.ExpiresInSeconds) * time.Second)
		} else {
			expiresAt = now.Add(60 * 24 * time.Hour)
		}
	}

	if err := svc.AddUser(cmd.Context(), auth.AddUserInput{
		Profile:         input.Profile,
		AppID:           input.AppID,
		Token:           longLived.AccessToken,
		AppSecret:       input.AppSecret,
		AuthProvider:    input.AuthProvider,
		AuthMode:        input.AuthMode,
		Scopes:          metadata.Scopes,
		IssuedAt:        now.Format(time.RFC3339),
		ExpiresAt:       expiresAt.Format(time.RFC3339),
		LastValidatedAt: now.Format(time.RFC3339),
		PageID:          input.PageID,
		IGUserID:        input.IGUserID,
	}); err != nil {
		return oauthLoginResult{}, err
	}

	return oauthLoginResult{
		AuthURL:       authURL,
		RedirectURI:   oauthRedirectURI,
		ExpiresAt:     expiresAt,
		Scopes:        metadata.Scopes,
		TokenType:     auth.TokenTypeUser,
		AuthProvider:  input.AuthProvider,
		AuthMode:      input.AuthMode,
		Profile:       input.Profile,
		TokenIssuedAt: now,
	}, nil
}

func resolveAuthProfile(runtime Runtime, profile string) (string, error) {
	resolved := strings.TrimSpace(profile)
	if resolved == "" {
		resolved = runtime.ProfileName()
	}
	if resolved == "" {
		return "", errors.New("profile is required (--profile or global --profile)")
	}
	return resolved, nil
}

func normalizeAuthMode(mode string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	switch normalized {
	case auth.AuthModeBoth, auth.AuthModeFacebook, auth.AuthModeInstagram:
		return normalized, nil
	default:
		return "", errors.New("mode must be one of: both, facebook, instagram")
	}
}

func normalizeDiscoverMode(mode string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	switch normalized {
	case "pages", "ig":
		return normalized, nil
	default:
		return "", errors.New("mode must be one of: pages, ig")
	}
}

func localCallbackRedirectURI(listenAddr string) (string, error) {
	listenAddr = strings.TrimSpace(listenAddr)
	if listenAddr == "" {
		return "", errors.New("listen address is required")
	}
	if _, _, err := net.SplitHostPort(listenAddr); err != nil {
		return "", fmt.Errorf("invalid --listen %q: expected host:port", listenAddr)
	}
	uri := &url.URL{
		Scheme: "http",
		Host:   listenAddr,
		Path:   defaultAuthCallbackPath,
	}
	return uri.String(), nil
}

func validateOAuthRedirectURI(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid --redirect-uri %q: %w", raw, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("invalid --redirect-uri %q: expected absolute URI", raw)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("invalid --redirect-uri %q: scheme must be http or https", raw)
	}
	return nil
}

func authProviderFromMode(mode string) string {
	if mode == auth.AuthModeInstagram {
		return auth.AuthProviderInstagram
	}
	return auth.AuthProviderFacebookLogin
}

func scopePackScopes(pack string, mode string) ([]string, error) {
	base := map[string][]string{
		"solo_smb": {
			"ads_management",
			"ads_read",
			"business_management",
			"pages_show_list",
			"pages_read_engagement",
			"pages_manage_posts",
			"instagram_basic",
			"instagram_content_publish",
		},
		"ads_only": {
			"ads_management",
			"ads_read",
			"business_management",
		},
		"ig_publish": {
			"instagram_basic",
			"instagram_content_publish",
			"pages_show_list",
			"pages_read_engagement",
		},
	}
	pack = strings.ToLower(strings.TrimSpace(pack))
	scopes, ok := base[pack]
	if !ok {
		return nil, errors.New("scope-pack must be one of: solo_smb, ads_only, ig_publish")
	}
	out := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if mode == auth.AuthModeFacebook && strings.HasPrefix(scope, "instagram_") {
			continue
		}
		if mode == auth.AuthModeInstagram {
			if strings.HasPrefix(scope, "ads_") || scope == "business_management" {
				continue
			}
		}
		out = append(out, scope)
	}
	if len(out) == 0 {
		return nil, errors.New("scope-pack selection produced empty scope set")
	}
	return out, nil
}

func firstDiscoveredPageID(pages []auth.DiscoveredPage) (string, bool) {
	for _, page := range pages {
		id := strings.TrimSpace(page.PageID)
		if id != "" {
			return id, true
		}
	}
	return "", false
}

func firstDiscoveredIGUserID(pages []auth.DiscoveredPage) (string, bool) {
	for _, page := range pages {
		id := strings.TrimSpace(page.IGBusinessAccountID)
		if id != "" {
			return id, true
		}
	}
	return "", false
}

func flattenIGAccounts(pages []auth.DiscoveredPage) []map[string]any {
	out := make([]map[string]any, 0)
	for _, page := range pages {
		igID := strings.TrimSpace(page.IGBusinessAccountID)
		if igID == "" {
			continue
		}
		out = append(out, map[string]any{
			"page_id":    page.PageID,
			"page_name":  page.Name,
			"ig_user_id": igID,
		})
	}
	return out
}

func newAuthService() (authCLIService, error) {
	cfgPath, err := config.DefaultPath()
	if err != nil {
		return nil, err
	}
	return auth.NewService(cfgPath, auth.NewKeychainStore(), nil, auth.DefaultGraphBaseURL), nil
}

func mustMarkFlagRequired(cmd *cobra.Command, name string) {
	if err := cmd.MarkFlagRequired(name); err != nil {
		panic(fmt.Sprintf("mark flag %q required for %s: %v", name, cmd.Name(), err))
	}
}

func csvToSlice(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
