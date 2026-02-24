package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/bilalbayram/metacli/internal/auth"
	"github.com/bilalbayram/metacli/internal/config"
	"github.com/spf13/cobra"
)

type authCLIService interface {
	AddSystemUser(context.Context, auth.AddSystemUserInput) error
	AddUser(context.Context, auth.AddUserInput) error
	DerivePageToken(context.Context, auth.PageTokenInput) error
	SetAppToken(context.Context, auth.SetAppTokenInput) error
	ExchangeOAuthCode(context.Context, auth.ExchangeCodeInput) (string, error)
	ValidateProfile(context.Context, string) (*auth.DebugTokenResponse, error)
	RotateProfile(context.Context, string) error
	DebugToken(context.Context, string, string, string) (*auth.DebugTokenResponse, error)
	ListProfiles() (map[string]config.Profile, error)
}

var (
	newAuthCLIService = newAuthService
	authNow           = time.Now

	contextType  = reflect.TypeOf((*context.Context)(nil)).Elem()
	errorType    = reflect.TypeOf((*error)(nil)).Elem()
	durationType = reflect.TypeOf(time.Duration(0))
)

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
			if profile == "" {
				profile = runtime.ProfileName()
			}
			if profile == "" {
				return errors.New("profile is required (--profile or global --profile)")
			}

			svc, err := newAuthCLIService()
			if err != nil {
				return err
			}
			if err := svc.AddSystemUser(cmd.Context(), auth.AddSystemUserInput{
				Profile:    profile,
				BusinessID: businessID,
				AppID:      appID,
				Token:      token,
				AppSecret:  appSecret,
			}); err != nil {
				return err
			}
			return writeSuccess(cmd, runtime, "meta auth add system-user", map[string]any{
				"status":  "ok",
				"profile": profile,
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
		profile     string
		appID       string
		appSecret   string
		redirectURI string
		scopes      string
		mode        string
		pageID      string
		igUserID    string
	)

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Automate auth setup and profile binding",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if profile == "" {
				profile = runtime.ProfileName()
			}
			if profile == "" {
				return errors.New("profile is required (--profile or global --profile)")
			}
			resolvedMode, err := normalizeAuthDiscoverMode(mode)
			if err != nil {
				return err
			}

			svc, err := newAuthCLIService()
			if err != nil {
				return err
			}

			result, used, err := callAuthOptionalAutomationMethod(cmd.Context(), svc, []string{"Setup", "SetupAutomation"}, map[string]any{
				"profile":      profile,
				"app_id":       appID,
				"app_secret":   appSecret,
				"redirect_uri": redirectURI,
				"scopes":       csvToSlice(scopes),
				"mode":         resolvedMode,
				"page_id":      pageID,
				"ig_user_id":   igUserID,
				"version":      config.DefaultGraphVersion,
			})
			if err != nil {
				return err
			}
			if !used {
				return errors.New("auth setup requires updated auth service API (TODO: implement Setup)")
			}

			return writeSuccess(cmd, runtime, "meta auth setup", mergeAuthCommandData(map[string]any{
				"status":  "ok",
				"profile": profile,
				"mode":    resolvedMode,
			}, result), nil, nil)
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&appID, "app-id", "", "Meta App ID")
	cmd.Flags().StringVar(&appSecret, "app-secret", "", "Meta App Secret")
	cmd.Flags().StringVar(&redirectURI, "redirect-uri", "", "OAuth redirect URI")
	cmd.Flags().StringVar(&scopes, "scopes", "", "Comma-separated OAuth scopes")
	cmd.Flags().StringVar(&mode, "mode", "pages", "Discovery mode: pages|ig")
	cmd.Flags().StringVar(&pageID, "page-id", "", "Optional page id override")
	cmd.Flags().StringVar(&igUserID, "ig-user-id", "", "Optional Instagram user id override")
	mustMarkFlagRequired(cmd, "app-id")
	mustMarkFlagRequired(cmd, "app-secret")
	mustMarkFlagRequired(cmd, "redirect-uri")
	return cmd
}

func newAuthLoginCommand(runtime Runtime) *cobra.Command {
	var (
		profile     string
		appID       string
		appSecret   string
		redirectURI string
		scopes      string
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate a user via automated OAuth callback flow",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if profile == "" {
				profile = runtime.ProfileName()
			}
			if profile == "" {
				return errors.New("profile is required (--profile or global --profile)")
			}

			svc, err := newAuthCLIService()
			if err != nil {
				return err
			}

			result, used, err := callAuthOptionalAutomationMethod(cmd.Context(), svc, []string{"LoginAutoCallback", "LoginAuto", "Login"}, map[string]any{
				"profile":      profile,
				"app_id":       appID,
				"app_secret":   appSecret,
				"redirect_uri": redirectURI,
				"scopes":       csvToSlice(scopes),
				"version":      config.DefaultGraphVersion,
			})
			if err != nil {
				return err
			}
			if !used {
				return errors.New("auth login requires updated auth service API (TODO: implement auto callback login)")
			}

			return writeSuccess(cmd, runtime, "meta auth login", mergeAuthCommandData(map[string]any{
				"status":  "ok",
				"profile": profile,
			}, result), nil, nil)
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&appID, "app-id", "", "Meta App ID")
	cmd.Flags().StringVar(&appSecret, "app-secret", "", "Meta App Secret")
	cmd.Flags().StringVar(&redirectURI, "redirect-uri", "", "OAuth redirect URI")
	cmd.Flags().StringVar(&scopes, "scopes", "", "Comma-separated OAuth scopes")
	mustMarkFlagRequired(cmd, "app-id")
	mustMarkFlagRequired(cmd, "app-secret")
	mustMarkFlagRequired(cmd, "redirect-uri")
	return cmd
}

func newAuthLoginManualCommand(runtime Runtime) *cobra.Command {
	var (
		profile     string
		appID       string
		appSecret   string
		redirectURI string
		scopes      string
		code        string
	)

	cmd := &cobra.Command{
		Use:   "login-manual",
		Short: "Authenticate a user by pasting an OAuth authorization code",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if profile == "" {
				profile = runtime.ProfileName()
			}
			if profile == "" {
				return errors.New("profile is required (--profile or global --profile)")
			}

			svc, err := newAuthCLIService()
			if err != nil {
				return err
			}

			result, used, err := callAuthOptionalAutomationMethod(cmd.Context(), svc, []string{"LoginManual", "LoginWithCode"}, map[string]any{
				"profile":      profile,
				"app_id":       appID,
				"app_secret":   appSecret,
				"redirect_uri": redirectURI,
				"scopes":       csvToSlice(scopes),
				"code":         code,
				"version":      config.DefaultGraphVersion,
			})
			if err != nil {
				return err
			}
			if used {
				return writeSuccess(cmd, runtime, "meta auth login-manual", mergeAuthCommandData(map[string]any{
					"status":  "ok",
					"profile": profile,
				}, result), nil, nil)
			}

			verifier, challenge, err := auth.NewPKCE()
			if err != nil {
				return err
			}
			scopeList := csvToSlice(scopes)
			authURL, err := auth.BuildOAuthURL(appID, redirectURI, scopeList, challenge, config.DefaultGraphVersion)
			if err != nil {
				return err
			}
			if code == "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "Open this URL in your browser and complete login:\n%s\nAuthorization code: ", authURL)
				reader := bufio.NewReader(cmd.InOrStdin())
				line, readErr := reader.ReadString('\n')
				if readErr != nil {
					return fmt.Errorf("read authorization code from stdin: %w", readErr)
				}
				code = strings.TrimSpace(line)
			}
			if code == "" {
				return errors.New("authorization code is required")
			}

			token, err := svc.ExchangeOAuthCode(context.Background(), auth.ExchangeCodeInput{
				AppID:        appID,
				RedirectURI:  redirectURI,
				Code:         code,
				CodeVerifier: verifier,
				Version:      config.DefaultGraphVersion,
			})
			if err != nil {
				return err
			}

			if err := svc.AddUser(cmd.Context(), auth.AddUserInput{
				Profile: profile,
				AppID:   appID,
				Token:   token,
			}); err != nil {
				return err
			}

			return writeSuccess(cmd, runtime, "meta auth login-manual", map[string]any{
				"status":   "ok",
				"profile":  profile,
				"auth_url": authURL,
			}, nil, nil)
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&appID, "app-id", "", "Meta App ID")
	cmd.Flags().StringVar(&appSecret, "app-secret", "", "Meta App Secret (optional for PKCE fallback)")
	cmd.Flags().StringVar(&redirectURI, "redirect-uri", "", "OAuth redirect URI")
	cmd.Flags().StringVar(&scopes, "scopes", "", "Comma-separated OAuth scopes")
	cmd.Flags().StringVar(&code, "code", "", "Authorization code (optional if interactive)")
	mustMarkFlagRequired(cmd, "app-id")
	mustMarkFlagRequired(cmd, "redirect-uri")
	return cmd
}

func newAuthDiscoverCommand(runtime Runtime) *cobra.Command {
	var (
		profile string
		mode    string
	)

	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Discover bindable assets for auth profiles",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if profile == "" {
				profile = runtime.ProfileName()
			}
			if profile == "" {
				return errors.New("profile is required (--profile or global --profile)")
			}
			resolvedMode, err := normalizeAuthDiscoverMode(mode)
			if err != nil {
				return err
			}

			svc, err := newAuthCLIService()
			if err != nil {
				return err
			}

			result, used, err := callAuthOptionalAutomationMethod(cmd.Context(), svc, []string{"Discover"}, map[string]any{
				"profile": profile,
				"mode":    resolvedMode,
			})
			if err != nil {
				return err
			}
			if !used {
				return errors.New("auth discover requires updated auth service API (TODO: implement Discover)")
			}

			return writeSuccess(cmd, runtime, "meta auth discover", mergeAuthCommandData(map[string]any{
				"status":  "ok",
				"profile": profile,
				"mode":    resolvedMode,
			}, result), nil, nil)
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
			if profile == "" {
				profile = runtime.ProfileName()
			}
			if profile == "" {
				return errors.New("profile is required (--profile or global --profile)")
			}
			if sourceProfile == "" {
				sourceProfile = runtime.ProfileName()
			}

			svc, err := newAuthCLIService()
			if err != nil {
				return err
			}
			if err := svc.DerivePageToken(cmd.Context(), auth.PageTokenInput{
				Profile:       profile,
				PageID:        pageID,
				SourceProfile: sourceProfile,
			}); err != nil {
				return err
			}

			return writeSuccess(cmd, runtime, "meta auth page-token", map[string]any{
				"status":         "ok",
				"profile":        profile,
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
			if profile == "" {
				profile = runtime.ProfileName()
			}
			if profile == "" {
				return errors.New("profile is required (--profile or global --profile)")
			}
			svc, err := newAuthCLIService()
			if err != nil {
				return err
			}
			if err := svc.SetAppToken(cmd.Context(), auth.SetAppTokenInput{
				Profile:   profile,
				AppID:     appID,
				AppSecret: appSecret,
			}); err != nil {
				return err
			}
			return writeSuccess(cmd, runtime, "meta auth app-token set", map[string]any{
				"status":  "ok",
				"profile": profile,
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
			if profile == "" {
				profile = runtime.ProfileName()
			}
			if profile == "" {
				return errors.New("profile is required (--profile or global --profile)")
			}
			svc, err := newAuthCLIService()
			if err != nil {
				return err
			}

			requiredScopeList := csvToSlice(requireScopes)
			resp, err := validateAuthProfileWithConstraints(cmd.Context(), svc, profile, minTTL, requiredScopeList)
			if err != nil {
				return err
			}

			result := map[string]any{
				"status":  "ok",
				"profile": profile,
				"debug":   resp.Data,
			}
			if minTTL > 0 {
				result["min_ttl"] = minTTL.String()
			}
			if len(requiredScopeList) > 0 {
				result["require_scopes"] = requiredScopeList
			}

			return writeSuccess(cmd, runtime, "meta auth validate", result, nil, nil)
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().DurationVar(&minTTL, "min-ttl", 0, "Minimum remaining token TTL (for example 30m, 12h)")
	cmd.Flags().StringVar(&requireScopes, "require-scopes", "", "Comma-separated scopes that must be present")
	return cmd
}

func newAuthRotateCommand(runtime Runtime) *cobra.Command {
	var profile string
	cmd := &cobra.Command{
		Use:   "rotate",
		Short: "Rotate token for a profile",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if profile == "" {
				profile = runtime.ProfileName()
			}
			if profile == "" {
				return errors.New("profile is required (--profile or global --profile)")
			}
			svc, err := newAuthCLIService()
			if err != nil {
				return err
			}
			if err := svc.RotateProfile(cmd.Context(), profile); err != nil {
				return err
			}
			return writeSuccess(cmd, runtime, "meta auth rotate", map[string]any{
				"status":  "ok",
				"profile": profile,
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
			if profile == "" {
				profile = runtime.ProfileName()
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
				_, selected, err := cfg.ResolveProfile(profile)
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

func newAuthService() (authCLIService, error) {
	cfgPath, err := config.DefaultPath()
	if err != nil {
		return nil, err
	}
	return auth.NewService(cfgPath, auth.NewKeychainStore(), nil, auth.DefaultGraphBaseURL), nil
}

func validateAuthProfileWithConstraints(ctx context.Context, svc authCLIService, profile string, minTTL time.Duration, requiredScopes []string) (*auth.DebugTokenResponse, error) {
	result, used, err := callAuthOptionalAutomationMethod(ctx, svc, []string{"ValidateProfileWithOptions", "ValidateWithOptions"}, map[string]any{
		"profile":           profile,
		"profile_name":      profile,
		"min_ttl":           minTTL,
		"min_ttl_seconds":   int64(minTTL / time.Second),
		"require_scopes":    requiredScopes,
		"required_scopes":   requiredScopes,
		"requiredScopeList": requiredScopes,
	})
	if err != nil {
		return nil, err
	}

	var resp *auth.DebugTokenResponse
	if used {
		resp, err = extractDebugTokenResponse(result)
		if err != nil {
			return nil, err
		}
	}
	if resp == nil {
		resp, err = svc.ValidateProfile(ctx, profile)
		if err != nil {
			return nil, err
		}
	}
	if err := enforceAuthValidateConstraints(resp.Data, minTTL, requiredScopes); err != nil {
		return nil, err
	}
	return resp, nil
}

func extractDebugTokenResponse(result any) (*auth.DebugTokenResponse, error) {
	if result == nil {
		return nil, nil
	}
	switch typed := result.(type) {
	case *auth.DebugTokenResponse:
		return typed, nil
	case auth.DebugTokenResponse:
		copy := typed
		return &copy, nil
	case map[string]any:
		if data, ok := typed["data"].(map[string]any); ok {
			return &auth.DebugTokenResponse{Data: data}, nil
		}
		if data, ok := typed["debug"].(map[string]any); ok {
			return &auth.DebugTokenResponse{Data: data}, nil
		}
		return &auth.DebugTokenResponse{Data: typed}, nil
	}

	value := reflect.ValueOf(result)
	if !value.IsValid() {
		return nil, nil
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, nil
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return nil, fmt.Errorf("unsupported validate response type %T", result)
	}

	field := value.FieldByName("Data")
	if !field.IsValid() {
		return nil, fmt.Errorf("validate response %T does not expose Data", result)
	}
	if mapped, ok := asAuthDataMap(field.Interface()); ok {
		return &auth.DebugTokenResponse{Data: mapped}, nil
	}
	return nil, fmt.Errorf("validate response %T Data is not an object", result)
}

func enforceAuthValidateConstraints(debugData map[string]any, minTTL time.Duration, requiredScopes []string) error {
	if minTTL > 0 {
		expiresAt, ok := parseUnixSeconds(debugData["expires_at"])
		if !ok {
			return errors.New("token metadata missing expires_at required by --min-ttl")
		}
		remaining := time.Unix(expiresAt, 0).Sub(authNow())
		if remaining < minTTL {
			return fmt.Errorf("token ttl %s is below required minimum %s", remaining.Round(time.Second), minTTL)
		}
	}

	if len(requiredScopes) > 0 {
		scopeSet := extractDebugTokenScopes(debugData)
		missing := make([]string, 0, len(requiredScopes))
		for _, required := range requiredScopes {
			required = strings.TrimSpace(required)
			if required == "" {
				continue
			}
			if _, ok := scopeSet[required]; !ok {
				missing = append(missing, required)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("token is missing required scopes: %s", strings.Join(missing, ", "))
		}
	}
	return nil
}

func parseUnixSeconds(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int8:
		return int64(typed), true
	case int16:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case uint:
		return int64(typed), true
	case uint8:
		return int64(typed), true
	case uint16:
		return int64(typed), true
	case uint32:
		return int64(typed), true
	case uint64:
		return int64(typed), true
	case float32:
		return int64(typed), true
	case float64:
		return int64(typed), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func extractDebugTokenScopes(debugData map[string]any) map[string]struct{} {
	scopeSet := map[string]struct{}{}
	collectDebugTokenScopes(scopeSet, debugData["scopes"])
	collectDebugTokenScopes(scopeSet, debugData["granted_scopes"])
	collectDebugTokenScopes(scopeSet, debugData["granular_scopes"])
	collectDebugTokenScopes(scopeSet, debugData["scope"])
	return scopeSet
}

func collectDebugTokenScopes(scopeSet map[string]struct{}, raw any) {
	switch typed := raw.(type) {
	case string:
		for _, scope := range csvToSlice(typed) {
			scopeSet[scope] = struct{}{}
		}
	case []string:
		for _, scope := range typed {
			scope = strings.TrimSpace(scope)
			if scope != "" {
				scopeSet[scope] = struct{}{}
			}
		}
	case []any:
		for _, item := range typed {
			collectDebugTokenScopes(scopeSet, item)
		}
	case map[string]any:
		if scope, ok := typed["scope"].(string); ok {
			scope = strings.TrimSpace(scope)
			if scope != "" {
				scopeSet[scope] = struct{}{}
			}
		}
		if scope, ok := typed["name"].(string); ok {
			scope = strings.TrimSpace(scope)
			if scope != "" {
				scopeSet[scope] = struct{}{}
			}
		}
	}
}

func normalizeAuthDiscoverMode(mode string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	switch normalized {
	case "pages", "ig":
		return normalized, nil
	default:
		return "", errors.New("mode must be one of: pages, ig")
	}
}

func mergeAuthCommandData(data map[string]any, result any) map[string]any {
	if result == nil {
		return data
	}
	if mapped, ok := asAuthDataMap(result); ok {
		for key, value := range mapped {
			data[key] = value
		}
		return data
	}
	data["result"] = result
	return data
}

func asAuthDataMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case *map[string]any:
		if typed == nil {
			return nil, false
		}
		return *typed, true
	}

	ref := reflect.ValueOf(value)
	if !ref.IsValid() || ref.Kind() != reflect.Map {
		return nil, false
	}
	if ref.Type().Key().Kind() != reflect.String {
		return nil, false
	}
	mapped := make(map[string]any, ref.Len())
	iter := ref.MapRange()
	for iter.Next() {
		mapped[iter.Key().String()] = iter.Value().Interface()
	}
	return mapped, true
}

func callAuthOptionalAutomationMethod(ctx context.Context, svc any, methodNames []string, input map[string]any) (any, bool, error) {
	for _, methodName := range methodNames {
		result, used, err := callAuthAutomationMethod(ctx, svc, methodName, input)
		if !used {
			continue
		}
		return result, true, err
	}
	return nil, false, nil
}

func callAuthAutomationMethod(ctx context.Context, svc any, methodName string, input map[string]any) (any, bool, error) {
	method := reflect.ValueOf(svc).MethodByName(methodName)
	if !method.IsValid() {
		return nil, false, nil
	}

	methodType := method.Type()
	if methodType.NumIn() == 0 || methodType.NumIn() > 2 {
		return nil, true, fmt.Errorf("auth service method %s has unsupported signature", methodName)
	}

	args := make([]reflect.Value, 0, methodType.NumIn())
	inputOffset := 0
	if methodType.In(0).Implements(contextType) {
		args = append(args, reflect.ValueOf(ctx))
		inputOffset = 1
	}
	if methodType.NumIn() != inputOffset+1 {
		return nil, true, fmt.Errorf("auth service method %s has unsupported signature", methodName)
	}

	inputArg, err := buildAuthMethodInput(methodType.In(inputOffset), input)
	if err != nil {
		return nil, true, fmt.Errorf("prepare %s input: %w", methodName, err)
	}
	args = append(args, inputArg)

	result, err := parseAuthMethodResult(method.Call(args))
	if err != nil {
		return nil, true, err
	}
	return result, true, nil
}

func buildAuthMethodInput(argType reflect.Type, input map[string]any) (reflect.Value, error) {
	switch {
	case argType.Kind() == reflect.Struct:
		value := reflect.New(argType).Elem()
		if err := populateAuthInputStruct(value, input); err != nil {
			return reflect.Value{}, err
		}
		return value, nil
	case argType.Kind() == reflect.Pointer && argType.Elem().Kind() == reflect.Struct:
		value := reflect.New(argType.Elem())
		if err := populateAuthInputStruct(value.Elem(), input); err != nil {
			return reflect.Value{}, err
		}
		return value, nil
	case argType.Kind() == reflect.Map && argType.Key().Kind() == reflect.String && argType.Elem().Kind() == reflect.Interface:
		value := reflect.MakeMapWithSize(argType, len(input))
		for key, raw := range input {
			mapKey := reflect.ValueOf(key)
			if mapKey.Type() != argType.Key() {
				mapKey = mapKey.Convert(argType.Key())
			}
			if raw == nil {
				value.SetMapIndex(mapKey, reflect.Zero(argType.Elem()))
				continue
			}
			value.SetMapIndex(mapKey, reflect.ValueOf(raw))
		}
		return value, nil
	default:
		return reflect.Value{}, fmt.Errorf("unsupported input type %s", argType.String())
	}
}

func populateAuthInputStruct(target reflect.Value, input map[string]any) error {
	normalized := map[string]any{}
	for key, value := range input {
		normalized[canonicalAuthInputKey(key)] = value
	}

	targetType := target.Type()
	for i := 0; i < target.NumField(); i++ {
		fieldType := targetType.Field(i)
		fieldValue := target.Field(i)
		if !fieldValue.CanSet() || fieldType.PkgPath != "" {
			continue
		}
		raw, ok := normalized[canonicalAuthInputKey(fieldType.Name)]
		if !ok {
			continue
		}
		if err := assignAuthInputValue(fieldValue, raw); err != nil {
			return fmt.Errorf("set field %s: %w", fieldType.Name, err)
		}
	}
	return nil
}

func assignAuthInputValue(target reflect.Value, value any) error {
	if !target.CanSet() || value == nil {
		return nil
	}
	if target.Type() == durationType {
		switch typed := value.(type) {
		case time.Duration:
			target.SetInt(int64(typed))
			return nil
		case int64:
			target.SetInt(typed)
			return nil
		case int:
			target.SetInt(int64(typed))
			return nil
		case string:
			parsed, err := time.ParseDuration(strings.TrimSpace(typed))
			if err != nil {
				return err
			}
			target.SetInt(int64(parsed))
			return nil
		}
	}

	valueRef := reflect.ValueOf(value)
	if valueRef.Type().AssignableTo(target.Type()) {
		target.Set(valueRef)
		return nil
	}
	if valueRef.Type().ConvertibleTo(target.Type()) {
		target.Set(valueRef.Convert(target.Type()))
		return nil
	}

	switch target.Kind() {
	case reflect.String:
		switch typed := value.(type) {
		case []string:
			target.SetString(strings.Join(typed, ","))
		case time.Duration:
			target.SetString(typed.String())
		default:
			target.SetString(strings.TrimSpace(fmt.Sprint(value)))
		}
		return nil
	case reflect.Bool:
		if typed, ok := value.(bool); ok {
			target.SetBool(typed)
			return nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch typed := value.(type) {
		case time.Duration:
			target.SetInt(int64(typed / time.Second))
			return nil
		case int:
			target.SetInt(int64(typed))
			return nil
		case int64:
			target.SetInt(typed)
			return nil
		case float64:
			target.SetInt(int64(typed))
			return nil
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		switch typed := value.(type) {
		case time.Duration:
			target.SetUint(uint64(typed / time.Second))
			return nil
		case int:
			target.SetUint(uint64(typed))
			return nil
		case int64:
			target.SetUint(uint64(typed))
			return nil
		case float64:
			target.SetUint(uint64(typed))
			return nil
		}
	case reflect.Slice:
		if target.Type().Elem().Kind() != reflect.String {
			break
		}
		scopes := []string{}
		switch typed := value.(type) {
		case []string:
			scopes = append(scopes, typed...)
		case []any:
			for _, item := range typed {
				if scope, ok := item.(string); ok {
					scopes = append(scopes, scope)
				}
			}
		case string:
			scopes = csvToSlice(typed)
		default:
			return fmt.Errorf("unsupported slice value type %T", value)
		}
		normalized := make([]string, 0, len(scopes))
		for _, scope := range scopes {
			scope = strings.TrimSpace(scope)
			if scope != "" {
				normalized = append(normalized, scope)
			}
		}
		target.Set(reflect.ValueOf(normalized))
		return nil
	}
	return fmt.Errorf("unsupported assignment %T -> %s", value, target.Type().String())
}

func canonicalAuthInputKey(value string) string {
	builder := strings.Builder{}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			builder.WriteRune(char)
		}
	}
	return strings.ToLower(builder.String())
}

func parseAuthMethodResult(results []reflect.Value) (any, error) {
	if len(results) == 0 {
		return nil, nil
	}
	last := results[len(results)-1]
	if last.Type().Implements(errorType) {
		if !last.IsNil() {
			return nil, last.Interface().(error)
		}
		if len(results) == 1 {
			return nil, nil
		}
		return results[0].Interface(), nil
	}
	return results[0].Interface(), nil
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
