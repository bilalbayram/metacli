package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/bilalbayram/metacli/internal/config"
)

type ExchangeLongLivedUserTokenInput struct {
	AppID           string
	AppSecret       string
	ShortLivedToken string
	Version         string
}

type LongLivedToken struct {
	AccessToken      string
	ExpiresInSeconds int64
}

type DebugTokenMetadata struct {
	IsValid   bool
	Scopes    []string
	ExpiresAt time.Time
}

func GenerateOAuthState() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate oauth state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func NewOAuthState() (string, error) {
	return GenerateOAuthState()
}

func (s *Service) ExchangeLongLivedUserToken(ctx context.Context, input ExchangeLongLivedUserTokenInput) (LongLivedToken, error) {
	if strings.TrimSpace(input.AppID) == "" {
		return LongLivedToken{}, errors.New("app id is required")
	}
	if strings.TrimSpace(input.AppSecret) == "" {
		return LongLivedToken{}, errors.New("app secret is required")
	}
	if strings.TrimSpace(input.ShortLivedToken) == "" {
		return LongLivedToken{}, errors.New("short-lived token is required")
	}

	version := input.Version
	if version == "" {
		version = config.DefaultGraphVersion
	}

	values := url.Values{}
	values.Set("grant_type", "fb_exchange_token")
	values.Set("client_id", input.AppID)
	values.Set("client_secret", input.AppSecret)
	values.Set("fb_exchange_token", input.ShortLivedToken)

	response := map[string]any{}
	if err := s.doRequest(ctx, http.MethodGet, version, "oauth/access_token", values, "", "", &response); err != nil {
		return LongLivedToken{}, err
	}

	token, _ := response["access_token"].(string)
	if strings.TrimSpace(token) == "" {
		return LongLivedToken{}, errors.New("long-lived token response did not include access_token")
	}

	expiresIn, err := parseJSONInt64Field(response, "expires_in")
	if err != nil {
		return LongLivedToken{}, err
	}

	return LongLivedToken{
		AccessToken:      token,
		ExpiresInSeconds: expiresIn,
	}, nil
}

func NormalizeDebugTokenMetadata(resp *DebugTokenResponse) (DebugTokenMetadata, error) {
	if resp == nil {
		return DebugTokenMetadata{}, errors.New("debug token response is nil")
	}
	if resp.Data == nil {
		return DebugTokenMetadata{}, errors.New("debug token response data is required")
	}

	isValid, ok := resp.Data["is_valid"].(bool)
	if !ok {
		return DebugTokenMetadata{}, errors.New("debug token response is missing boolean is_valid")
	}

	scopes, err := parseScopes(resp.Data["scopes"])
	if err != nil {
		return DebugTokenMetadata{}, err
	}
	sort.Strings(scopes)

	expiresAt, err := parseExpiresAt(resp.Data["expires_at"])
	if err != nil {
		return DebugTokenMetadata{}, err
	}

	return DebugTokenMetadata{
		IsValid:   isValid,
		Scopes:    scopes,
		ExpiresAt: expiresAt,
	}, nil
}

func parseExpiresAt(raw any) (time.Time, error) {
	if raw == nil {
		return time.Time{}, nil
	}
	seconds, err := parseInt64(raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("debug token response contains invalid expires_at: %w", err)
	}
	if seconds <= 0 {
		return time.Time{}, nil
	}
	return time.Unix(seconds, 0).UTC(), nil
}

func parseScopes(raw any) ([]string, error) {
	if raw == nil {
		return []string{}, nil
	}

	switch typed := raw.(type) {
	case []string:
		scopes := make([]string, 0, len(typed))
		seen := map[string]struct{}{}
		for _, scope := range typed {
			scope = strings.TrimSpace(scope)
			if scope == "" {
				return nil, errors.New("debug token response scopes contains blank entry")
			}
			if _, ok := seen[scope]; ok {
				continue
			}
			seen[scope] = struct{}{}
			scopes = append(scopes, scope)
		}
		return scopes, nil
	case []any:
		scopes := make([]string, 0, len(typed))
		seen := map[string]struct{}{}
		for _, entry := range typed {
			scope, ok := entry.(string)
			if !ok {
				return nil, errors.New("debug token response scopes must contain only strings")
			}
			scope = strings.TrimSpace(scope)
			if scope == "" {
				return nil, errors.New("debug token response scopes contains blank entry")
			}
			if _, ok := seen[scope]; ok {
				continue
			}
			seen[scope] = struct{}{}
			scopes = append(scopes, scope)
		}
		return scopes, nil
	default:
		return nil, errors.New("debug token response scopes must be an array")
	}
}

func parseJSONInt64Field(raw map[string]any, field string) (int64, error) {
	value, ok := raw[field]
	if !ok {
		return 0, fmt.Errorf("long-lived token response did not include %s", field)
	}
	parsed, err := parseInt64(value)
	if err != nil {
		return 0, fmt.Errorf("long-lived token response contained invalid %s: %w", field, err)
	}
	return parsed, nil
}

func parseInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case int:
		return int64(typed), nil
	case int32:
		return int64(typed), nil
	case int64:
		return typed, nil
	case float32:
		return int64(typed), nil
	case float64:
		return int64(typed), nil
	default:
		return 0, fmt.Errorf("expected number but got %T", value)
	}
}
