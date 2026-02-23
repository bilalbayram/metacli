package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

func AppSecretProof(accessToken string, appSecret string) (string, error) {
	if accessToken == "" {
		return "", errors.New("access token is required for appsecret_proof")
	}
	if appSecret == "" {
		return "", errors.New("app secret is required for appsecret_proof")
	}

	mac := hmac.New(sha256.New, []byte(appSecret))
	if _, err := mac.Write([]byte(accessToken)); err != nil {
		return "", err
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}
