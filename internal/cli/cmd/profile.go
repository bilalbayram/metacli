package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bilalbayram/metacli/internal/auth"
	"github.com/bilalbayram/metacli/internal/config"
)

var profileAuthPreflight = runProfileAuthPreflight
var profileLinkedInPreflight = runLinkedInProfileAuthPreflight

type ProfileCredentials struct {
	Name         string
	Provider     string
	Profile      config.Profile
	Token        string
	AppSecret    string
	ClientSecret string
	RefreshToken string
}

func loadProfileCredentials(profile string) (*ProfileCredentials, error) {
	return loadProviderProfileCredentials(profile, config.ProviderMeta)
}

func loadLinkedInProfileCredentials(profile string) (*ProfileCredentials, error) {
	return loadProviderProfileCredentials(profile, config.ProviderLinkedIn)
}

func loadProviderProfileCredentials(profile string, expectedProvider string) (*ProfileCredentials, error) {
	if strings.TrimSpace(profile) == "" {
		return nil, errors.New("profile is required")
	}

	configPath, err := config.DefaultPath()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	name, selected, err := cfg.ResolveProfile(profile)
	if err != nil {
		return nil, err
	}
	provider := config.ResolveProvider(selected.Provider)
	if expectedProvider != "" && provider != expectedProvider {
		return nil, fmt.Errorf("profile %q uses provider %q, expected %q", name, provider, expectedProvider)
	}

	switch provider {
	case config.ProviderMeta:
		if err := profileAuthPreflight(name, selected.Scopes, configPath); err != nil {
			return nil, fmt.Errorf("auth preflight failed for profile %q: %w", name, err)
		}
	case config.ProviderLinkedIn:
		if err := profileLinkedInPreflight(name, selected, configPath); err != nil {
			return nil, fmt.Errorf("linkedin auth preflight failed for profile %q: %w", name, err)
		}
	}

	store := auth.NewSecretStore()
	token, err := store.Get(selected.TokenRef)
	if err != nil {
		return nil, err
	}
	out := &ProfileCredentials{
		Name:     name,
		Provider: provider,
		Profile:  selected,
		Token:    token,
	}

	if provider == config.ProviderMeta && selected.AppSecretRef != "" {
		appSecret, err := store.Get(selected.AppSecretRef)
		if err != nil {
			return nil, fmt.Errorf("load app secret for profile %q: %w", profile, err)
		}
		out.AppSecret = appSecret
	}
	if provider == config.ProviderLinkedIn {
		if selected.ClientSecretRef != "" {
			clientSecret, err := store.Get(selected.ClientSecretRef)
			if err != nil {
				return nil, fmt.Errorf("load client secret for profile %q: %w", profile, err)
			}
			out.ClientSecret = clientSecret
		}
		if selected.RefreshTokenRef != "" {
			refreshToken, err := store.Get(selected.RefreshTokenRef)
			if err != nil {
				return nil, fmt.Errorf("load refresh token for profile %q: %w", profile, err)
			}
			out.RefreshToken = refreshToken
		}
	}
	return out, nil
}

func runProfileAuthPreflight(profile string, requiredScopes []string, configPath string) error {
	if strings.TrimSpace(profile) == "" {
		return errors.New("profile is required")
	}
	if strings.TrimSpace(configPath) == "" {
		return errors.New("config path is required")
	}

	svc := auth.NewService(configPath, auth.NewSecretStore(), nil, auth.DefaultGraphBaseURL)
	if _, err := svc.EnsureValid(context.Background(), profile, 72*time.Hour, requiredScopes); err != nil {
		return err
	}
	return nil
}
