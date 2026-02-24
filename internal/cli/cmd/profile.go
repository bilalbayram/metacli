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

type ProfileCredentials struct {
	Name      string
	Profile   config.Profile
	Token     string
	AppSecret string
}

func loadProfileCredentials(profile string) (*ProfileCredentials, error) {
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

	if err := profileAuthPreflight(name, selected.Scopes, configPath); err != nil {
		return nil, fmt.Errorf("auth preflight failed for profile %q: %w", name, err)
	}

	store := auth.NewKeychainStore()
	token, err := store.Get(selected.TokenRef)
	if err != nil {
		return nil, err
	}
	out := &ProfileCredentials{
		Name:    name,
		Profile: selected,
		Token:   token,
	}

	if selected.AppSecretRef != "" {
		appSecret, err := store.Get(selected.AppSecretRef)
		if err != nil {
			return nil, fmt.Errorf("load app secret for profile %q: %w", profile, err)
		}
		out.AppSecret = appSecret
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

	svc := auth.NewService(configPath, auth.NewKeychainStore(), nil, auth.DefaultGraphBaseURL)
	if _, err := svc.EnsureValid(context.Background(), profile, 72*time.Hour, requiredScopes); err != nil {
		return err
	}
	return nil
}
