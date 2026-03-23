package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bilalbayram/metacli/internal/auth"
	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/linkedin"
	"github.com/bilalbayram/metacli/internal/output"
)

const (
	defaultLinkedInProfileTTL = 5 * time.Minute
)

type linkedInProfileStore struct {
	configPath string
}

func (s linkedInProfileStore) Get(name string) (linkedin.Profile, error) {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return linkedin.Profile{}, err
	}
	resolvedName, profile, err := cfg.ResolveProfile(name)
	if err != nil {
		return linkedin.Profile{}, err
	}
	if provider := config.ResolveProvider(profile.Provider); provider != config.ProviderLinkedIn {
		return linkedin.Profile{}, fmt.Errorf("profile %q uses provider %q, expected %q", resolvedName, provider, config.ProviderLinkedIn)
	}
	expiresAt, err := parseOptionalRFC3339(profile.ExpiresAt)
	if err != nil {
		return linkedin.Profile{}, fmt.Errorf("profile %q expires_at is invalid: %w", resolvedName, err)
	}
	return linkedin.Profile{
		Provider:             config.ProviderLinkedIn,
		LinkedInVersion:      strings.TrimSpace(profile.LinkedInVersion),
		ClientID:             strings.TrimSpace(profile.ClientID),
		ClientSecretRef:      strings.TrimSpace(profile.ClientSecretRef),
		AccessTokenRef:       strings.TrimSpace(profile.TokenRef),
		RefreshTokenRef:      strings.TrimSpace(profile.RefreshTokenRef),
		Scopes:               append([]string(nil), profile.Scopes...),
		AccessTokenExpiresAt: expiresAt,
	}, nil
}

func (s linkedInProfileStore) Upsert(name string, liProfile linkedin.Profile) error {
	cfg, err := config.LoadOrCreate(s.configPath)
	if err != nil {
		return err
	}

	profile := config.Profile{
		Provider:        config.ProviderLinkedIn,
		Domain:          config.DefaultDomain,
		LinkedInVersion: strings.TrimSpace(liProfile.LinkedInVersion),
		ClientID:        strings.TrimSpace(liProfile.ClientID),
		TokenRef:        strings.TrimSpace(liProfile.AccessTokenRef),
		ClientSecretRef: strings.TrimSpace(liProfile.ClientSecretRef),
		RefreshTokenRef: strings.TrimSpace(liProfile.RefreshTokenRef),
		Scopes:          append([]string(nil), liProfile.Scopes...),
		IssuedAt:        time.Now().UTC().Format(time.RFC3339),
		ExpiresAt:       formatOptionalRFC3339(liProfile.AccessTokenExpiresAt),
		LastValidatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if existingName, existing, err := cfg.ResolveProfile(name); err == nil && existingName != "" {
		profile.IssuedAt = existing.IssuedAt
		if profile.ExpiresAt == "" {
			profile.ExpiresAt = existing.ExpiresAt
		}
		if profile.LinkedInVersion == "" {
			profile.LinkedInVersion = existing.LinkedInVersion
		}
		if profile.ClientID == "" {
			profile.ClientID = existing.ClientID
		}
		if profile.TokenRef == "" {
			profile.TokenRef = existing.TokenRef
		}
		if profile.ClientSecretRef == "" {
			profile.ClientSecretRef = existing.ClientSecretRef
		}
		if profile.RefreshTokenRef == "" {
			profile.RefreshTokenRef = existing.RefreshTokenRef
		}
		if len(profile.Scopes) == 0 {
			profile.Scopes = append([]string(nil), existing.Scopes...)
		}
	}

	if profile.IssuedAt == "" {
		profile.IssuedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if profile.ExpiresAt == "" {
		profile.ExpiresAt = time.Now().UTC().Add(defaultLinkedInProfileTTL).Format(time.RFC3339)
	}
	if err := cfg.UpsertProfile(name, profile); err != nil {
		return err
	}
	return config.Save(s.configPath, cfg)
}

func newLinkedInAuthService() (*linkedin.Service, error) {
	configPath, err := config.DefaultPath()
	if err != nil {
		return nil, err
	}
	return linkedin.NewService(
		nil,
		linkedin.DefaultAuthBaseURL,
		linkedin.DefaultAPIBaseURL,
		linkedInProfileStore{configPath: configPath},
		auth.NewSecretStore(),
	), nil
}

func newLinkedInClient(creds *ProfileCredentials, version string) (*linkedin.Client, error) {
	if creds == nil {
		return nil, errors.New("profile credentials are required")
	}
	if creds.Provider != config.ProviderLinkedIn {
		return nil, fmt.Errorf("profile %q uses provider %q, expected %q", creds.Name, creds.Provider, config.ProviderLinkedIn)
	}
	resolvedVersion := strings.TrimSpace(version)
	if resolvedVersion == "" {
		resolvedVersion = strings.TrimSpace(creds.Profile.LinkedInVersion)
	}
	if resolvedVersion == "" {
		resolvedVersion = config.DefaultLinkedInVersion
	}
	return linkedin.NewClient(nil, linkedin.DefaultBaseURL, resolvedVersion, creds.Token), nil
}

func linkedInEnvelopeProvider(version string) *output.Provider {
	return &output.Provider{
		Name:    config.ProviderLinkedIn,
		Version: strings.TrimSpace(version),
	}
}

func resolveLinkedInProfileAndVersion(runtime Runtime, profile string, version string) (*ProfileCredentials, string, error) {
	resolvedProfile := strings.TrimSpace(profile)
	if resolvedProfile == "" {
		resolvedProfile = runtime.ProfileName()
	}
	if resolvedProfile == "" {
		return nil, "", errors.New("profile is required (--profile or global --profile)")
	}

	creds, err := loadLinkedInProfileCredentials(resolvedProfile)
	if err != nil {
		return nil, "", err
	}

	resolvedVersion := strings.TrimSpace(version)
	if resolvedVersion == "" {
		resolvedVersion = strings.TrimSpace(creds.Profile.LinkedInVersion)
	}
	if resolvedVersion == "" {
		resolvedVersion = config.DefaultLinkedInVersion
	}
	return creds, resolvedVersion, nil
}

func runLinkedInProfileAuthPreflight(profile string, selected config.Profile, configPath string) error {
	if strings.TrimSpace(profile) == "" {
		return errors.New("profile is required")
	}
	if config.ResolveProvider(selected.Provider) != config.ProviderLinkedIn {
		return fmt.Errorf("profile %q is not a linkedin profile", profile)
	}
	expiresAt, err := parseOptionalRFC3339(selected.ExpiresAt)
	if err != nil {
		return fmt.Errorf("parse profile expiry: %w", err)
	}
	if expiresAt.IsZero() || expiresAt.After(time.Now().UTC()) {
		return nil
	}
	svc, err := newLinkedInAuthServiceWithConfigPath(configPath)
	if err != nil {
		return err
	}
	token, err := svc.RefreshAccessToken(context.Background(), profile)
	if err != nil {
		return err
	}
	if token.AccessTokenExpiresAt.IsZero() || !token.AccessTokenExpiresAt.After(time.Now().UTC()) {
		return fmt.Errorf("profile token is expired (expires_at=%s)", selected.ExpiresAt)
	}
	return nil
}

func newLinkedInAuthServiceWithConfigPath(configPath string) (*linkedin.Service, error) {
	return linkedin.NewService(
		nil,
		linkedin.DefaultAuthBaseURL,
		linkedin.DefaultAPIBaseURL,
		linkedInProfileStore{configPath: configPath},
		auth.NewSecretStore(),
	), nil
}

func parseOptionalRFC3339(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, raw)
}

func formatOptionalRFC3339(value time.Time) string {
	if value.IsZero() {
		return time.Now().UTC().Add(defaultLinkedInProfileTTL).Format(time.RFC3339)
	}
	return value.UTC().Format(time.RFC3339)
}
