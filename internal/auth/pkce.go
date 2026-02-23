package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

func NewPKCE() (verifier string, challenge string, err error) {
	raw := make([]byte, 64)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("generate pkce verifier: %w", err)
	}

	verifier = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func BuildOAuthURL(appID string, redirectURI string, scopes []string, challenge string, version string) (string, error) {
	if strings.TrimSpace(appID) == "" {
		return "", errors.New("app id is required")
	}
	if strings.TrimSpace(redirectURI) == "" {
		return "", errors.New("redirect uri is required")
	}
	if strings.TrimSpace(challenge) == "" {
		return "", errors.New("pkce challenge is required")
	}
	if strings.TrimSpace(version) == "" {
		return "", errors.New("graph version is required")
	}

	values := url.Values{}
	values.Set("client_id", appID)
	values.Set("redirect_uri", redirectURI)
	values.Set("response_type", "code")
	values.Set("code_challenge", challenge)
	values.Set("code_challenge_method", "S256")
	if len(scopes) > 0 {
		values.Set("scope", strings.Join(scopes, ","))
	}

	return fmt.Sprintf("https://www.facebook.com/%s/dialog/oauth?%s", version, values.Encode()), nil
}
