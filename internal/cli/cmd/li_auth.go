package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bilalbayram/metacli/internal/auth"
	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/linkedin"
	"github.com/spf13/cobra"
)

func newLIAuthSetupCommand(runtime Runtime) *cobra.Command {
	var (
		profile      string
		clientID     string
		clientSecret string
		version      string
		scopesRaw    string
		authFlow     string
		listenAddr   string
		redirectURI  string
		timeout      time.Duration
		openBrowser  bool
	)

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Run LinkedIn browser-based OAuth setup",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			profileName, err := resolveLIProfileName(runtime, profile)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta li auth setup", err)
			}
			resolvedRedirectURI := strings.TrimSpace(redirectURI)
			if resolvedRedirectURI == "" {
				resolvedRedirectURI = fmt.Sprintf("http://%s%s", strings.TrimSpace(listenAddr), defaultAuthCallbackPath)
			}

			resolvedVersion, err := bootstrapLinkedInProfile(profileName, clientID, clientSecret, version, csvToSlice(scopesRaw))
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li auth setup", err, linkedInEnvelopeProvider(version))
			}

			svc, err := newLinkedInAuthService()
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li auth setup", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			result, err := svc.Setup(cmd.Context(), profileName, linkedinSetupInput(profileName, resolvedRedirectURI, csvToSlice(scopesRaw), authFlow, timeout, openBrowser))
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li auth setup", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, "meta li auth setup", result, nil, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&clientID, "client-id", "", "LinkedIn application client id")
	cmd.Flags().StringVar(&clientSecret, "client-secret", "", "LinkedIn application client secret")
	cmd.Flags().StringVar(&version, "linkedin-version", "", "LinkedIn API version header (YYYYMM)")
	cmd.Flags().StringVar(&scopesRaw, "scopes", "", "Comma-separated OAuth scopes")
	cmd.Flags().StringVar(&authFlow, "auth-flow", string(linkedin.AuthFlowStandard), "LinkedIn authorization flow: standard|native-pkce")
	cmd.Flags().StringVar(&listenAddr, "listen-addr", defaultAuthListenAddr, "Local callback listener address")
	cmd.Flags().StringVar(&redirectURI, "redirect-uri", "", "OAuth redirect URI; defaults to http://<listen-addr>/oauth/callback")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultAuthTimeout, "OAuth callback timeout")
	cmd.Flags().BoolVar(&openBrowser, "open-browser", true, "Open the authorization URL in the default browser")
	return cmd
}

func newLIAuthValidateCommand(runtime Runtime) *cobra.Command {
	var profile string

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate and refresh the current LinkedIn token if needed",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			profileName, err := resolveLIProfileName(runtime, profile)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta li auth validate", err)
			}
			creds, resolvedVersion, err := resolveLinkedInProfileAndVersion(runtime, profileName, "")
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li auth validate", err, linkedInEnvelopeProvider(""))
			}
			svc, err := newLinkedInAuthService()
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li auth validate", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			result, err := svc.Validate(cmd.Context(), profileName)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li auth validate", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			result.Token.AccessToken = creds.Token
			result.Token.RefreshToken = creds.RefreshToken
			return writeSuccessWithProvider(cmd, runtime, "meta li auth validate", result, nil, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	return cmd
}

func newLIAuthScopesCommand(runtime Runtime) *cobra.Command {
	var profile string

	cmd := &cobra.Command{
		Use:   "scopes",
		Short: "List LinkedIn scopes granted to the active profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			profileName, err := resolveLIProfileName(runtime, profile)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta li auth scopes", err)
			}
			_, resolvedVersion, err := resolveLinkedInProfileAndVersion(runtime, profileName, "")
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li auth scopes", err, linkedInEnvelopeProvider(""))
			}
			svc, err := newLinkedInAuthService()
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li auth scopes", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			scopes, err := svc.Scopes(cmd.Context(), profileName)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li auth scopes", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, "meta li auth scopes", map[string]any{
				"profile": profileName,
				"scopes":  scopes,
			}, nil, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	return cmd
}

func newLIAuthWhoAmICommand(runtime Runtime) *cobra.Command {
	var profile string

	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Resolve the member associated with the active LinkedIn profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			profileName, err := resolveLIProfileName(runtime, profile)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta li auth whoami", err)
			}
			_, resolvedVersion, err := resolveLinkedInProfileAndVersion(runtime, profileName, "")
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li auth whoami", err, linkedInEnvelopeProvider(""))
			}
			svc, err := newLinkedInAuthService()
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li auth whoami", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			result, err := svc.WhoAmI(cmd.Context(), profileName)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li auth whoami", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, "meta li auth whoami", result, nil, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	return cmd
}

func resolveLIProfileName(runtime Runtime, profile string) (string, error) {
	resolved := strings.TrimSpace(profile)
	if resolved == "" {
		resolved = runtime.ProfileName()
	}
	if resolved == "" {
		return "", errors.New("profile is required (--profile or global --profile)")
	}
	return resolved, nil
}

func bootstrapLinkedInProfile(profileName string, clientID string, clientSecret string, version string, scopes []string) (string, error) {
	configPath, err := config.DefaultPath()
	if err != nil {
		return "", err
	}
	cfg, err := config.LoadOrCreate(configPath)
	if err != nil {
		return "", err
	}
	store := auth.NewSecretStore()

	existing := config.Profile{}
	if _, profile, resolveErr := cfg.ResolveProfile(profileName); resolveErr == nil {
		existing = profile
		if provider := config.ResolveProvider(existing.Provider); provider != config.ProviderLinkedIn {
			return "", fmt.Errorf("profile %q uses provider %q, expected %q", profileName, provider, config.ProviderLinkedIn)
		}
	}

	resolvedClientID := strings.TrimSpace(clientID)
	if resolvedClientID == "" {
		resolvedClientID = strings.TrimSpace(existing.ClientID)
	}
	if resolvedClientID == "" {
		return "", errors.New("client id is required")
	}
	resolvedVersion := strings.TrimSpace(version)
	if resolvedVersion == "" {
		resolvedVersion = strings.TrimSpace(existing.LinkedInVersion)
	}
	if resolvedVersion == "" {
		resolvedVersion = config.DefaultLinkedInVersion
	}
	resolvedScopes := scopes
	if len(resolvedScopes) == 0 {
		resolvedScopes = append([]string(nil), existing.Scopes...)
	}
	if len(resolvedScopes) == 0 {
		return "", errors.New("at least one LinkedIn scope is required")
	}

	tokenRef, err := auth.SecretRef(profileName, auth.SecretToken)
	if err != nil {
		return "", err
	}
	clientSecretRef, err := auth.SecretRef(profileName, auth.SecretClientSecret)
	if err != nil {
		return "", err
	}
	refreshTokenRef, err := auth.SecretRef(profileName, auth.SecretRefreshToken)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(clientSecret) != "" {
		if err := store.Set(clientSecretRef, clientSecret); err != nil {
			return "", err
		}
	} else if strings.TrimSpace(existing.ClientSecretRef) == "" {
		return "", errors.New("client secret is required")
	}

	now := time.Now().UTC()
	profile := config.Profile{
		Provider:        config.ProviderLinkedIn,
		Domain:          config.DefaultDomain,
		LinkedInVersion: resolvedVersion,
		ClientID:        resolvedClientID,
		TokenRef:        tokenRef,
		ClientSecretRef: clientSecretRef,
		RefreshTokenRef: refreshTokenRef,
		Scopes:          resolvedScopes,
		IssuedAt:        now.Format(time.RFC3339),
		ExpiresAt:       now.Add(defaultLinkedInProfileTTL).Format(time.RFC3339),
		LastValidatedAt: now.Format(time.RFC3339),
	}
	if existing.IssuedAt != "" {
		profile.IssuedAt = existing.IssuedAt
	}
	if existing.ExpiresAt != "" {
		profile.ExpiresAt = existing.ExpiresAt
	}
	if err := cfg.UpsertProfile(profileName, profile); err != nil {
		return "", err
	}
	if err := config.Save(configPath, cfg); err != nil {
		return "", err
	}
	return resolvedVersion, nil
}

func linkedinSetupInput(profileName string, redirectURI string, scopes []string, authFlow string, timeout time.Duration, openBrowser bool) linkedin.SetupInput {
	_ = profileName
	return linkedin.SetupInput{
		RedirectURI: strings.TrimSpace(redirectURI),
		Scopes:      scopes,
		AuthFlow:    strings.TrimSpace(authFlow),
		OpenBrowser: openBrowser,
		Timeout:     timeout,
	}
}
