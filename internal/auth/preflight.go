package auth

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bilalbayram/metacli/internal/config"
)

func (s *Service) EnsureValid(ctx context.Context, profileName string, minTTL time.Duration, requiredScopes []string) (*DebugTokenMetadata, error) {
	if minTTL < 0 {
		return nil, errors.New("minimum ttl cannot be negative")
	}

	normalizedRequiredScopes, err := normalizeScopesInput(requiredScopes)
	if err != nil {
		return nil, err
	}

	cfg, err := config.Load(s.configPath)
	if err != nil {
		return nil, err
	}
	resolvedProfileName, profile, err := cfg.ResolveProfile(profileName)
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

	debugResponse, err := s.DebugToken(ctx, profile.GraphVersion, token, debugAccessToken)
	if err != nil {
		return nil, err
	}

	metadata, err := NormalizeDebugTokenMetadata(debugResponse)
	if err != nil {
		return nil, err
	}
	if !metadata.IsValid {
		return nil, fmt.Errorf("profile token is invalid for profile %q", resolvedProfileName)
	}

	now := time.Now().UTC()
	if !metadata.ExpiresAt.IsZero() {
		if !metadata.ExpiresAt.After(now) {
			return nil, fmt.Errorf("profile token is expired for profile %q (expires_at=%s)", resolvedProfileName, metadata.ExpiresAt.Format(time.RFC3339))
		}
		if minTTL > 0 && metadata.ExpiresAt.Before(now.Add(minTTL)) {
			return nil, fmt.Errorf(
				"profile token ttl is below minimum ttl for profile %q (expires_at=%s min_ttl=%s)",
				resolvedProfileName,
				metadata.ExpiresAt.Format(time.RFC3339),
				minTTL,
			)
		}
	}

	missing := findMissingScopes(normalizedRequiredScopes, metadata.Scopes)
	if len(missing) > 0 {
		return nil, fmt.Errorf("profile token is missing required scopes for profile %q: %s", resolvedProfileName, strings.Join(missing, ","))
	}

	return &metadata, nil
}

func normalizeScopesInput(scopes []string) ([]string, error) {
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			return nil, errors.New("required scopes contains blank entry")
		}
		if _, exists := seen[scope]; exists {
			continue
		}
		seen[scope] = struct{}{}
		normalized = append(normalized, scope)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func findMissingScopes(required []string, granted []string) []string {
	grantedSet := map[string]struct{}{}
	for _, scope := range granted {
		grantedSet[scope] = struct{}{}
	}

	missing := make([]string, 0)
	for _, scope := range required {
		if _, ok := grantedSet[scope]; ok {
			continue
		}
		missing = append(missing, scope)
	}
	sort.Strings(missing)
	return missing
}
