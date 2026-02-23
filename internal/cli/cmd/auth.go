package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bilalbayram/metacli/internal/auth"
	"github.com/bilalbayram/metacli/internal/config"
	"github.com/spf13/cobra"
)

func NewAuthCommand(runtime Runtime) *cobra.Command {
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication and token profile management",
	}

	authCmd.AddCommand(newAuthAddCommand(runtime))
	authCmd.AddCommand(newAuthLoginCommand(runtime))
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

			svc, err := newAuthService()
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
	addCmd.AddCommand(systemUserCmd)
	return addCmd
}

func newAuthLoginCommand(runtime Runtime) *cobra.Command {
	var (
		profile     string
		appID       string
		redirectURI string
		scopes      string
		code        string
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate a user via OAuth PKCE",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if profile == "" {
				profile = runtime.ProfileName()
			}
			if profile == "" {
				return errors.New("profile is required (--profile or global --profile)")
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

			svc, err := newAuthService()
			if err != nil {
				return err
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

			return writeSuccess(cmd, runtime, "meta auth login", map[string]any{
				"status":   "ok",
				"profile":  profile,
				"auth_url": authURL,
			}, nil, nil)
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&appID, "app-id", "", "Meta App ID")
	cmd.Flags().StringVar(&redirectURI, "redirect-uri", "", "OAuth redirect URI")
	cmd.Flags().StringVar(&scopes, "scopes", "", "Comma-separated OAuth scopes")
	cmd.Flags().StringVar(&code, "code", "", "Authorization code (optional if interactive)")
	mustMarkFlagRequired(cmd, "app-id")
	mustMarkFlagRequired(cmd, "redirect-uri")
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

			svc, err := newAuthService()
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
			svc, err := newAuthService()
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
	var profile string
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
			svc, err := newAuthService()
			if err != nil {
				return err
			}
			resp, err := svc.ValidateProfile(cmd.Context(), profile)
			if err != nil {
				return err
			}
			return writeSuccess(cmd, runtime, "meta auth validate", map[string]any{
				"status":  "ok",
				"profile": profile,
				"debug":   resp.Data,
			}, nil, nil)
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
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
			svc, err := newAuthService()
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
			svc, err := newAuthService()
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
			svc, err := newAuthService()
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

func newAuthService() (*auth.Service, error) {
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
