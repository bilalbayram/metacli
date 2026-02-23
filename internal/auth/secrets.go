package auth

import (
	"errors"
	"fmt"
	"strings"

	"github.com/zalando/go-keyring"
)

const (
	KeychainService = "meta-marketing-cli"
	SecretToken     = "token"
	SecretAppSecret = "app_secret"
)

type SecretStore interface {
	Set(ref string, value string) error
	Get(ref string) (string, error)
	Delete(ref string) error
}

type KeychainStore struct {
	service string
	backend keyringBackend
}

type keyringBackend interface {
	Set(service, user, password string) error
	Get(service, user string) (string, error)
	Delete(service, user string) error
}

type defaultKeyringBackend struct{}

func (defaultKeyringBackend) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}

func (defaultKeyringBackend) Get(service, user string) (string, error) {
	return keyring.Get(service, user)
}

func (defaultKeyringBackend) Delete(service, user string) error {
	return keyring.Delete(service, user)
}

func NewKeychainStore() *KeychainStore {
	return &KeychainStore{
		service: KeychainService,
		backend: defaultKeyringBackend{},
	}
}

func SecretRef(profile string, kind string) (string, error) {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return "", errors.New("profile is required for secret ref")
	}

	switch kind {
	case SecretToken, SecretAppSecret:
	default:
		return "", fmt.Errorf("unsupported secret kind %q", kind)
	}

	return fmt.Sprintf("keychain://%s/%s/%s", KeychainService, profile, kind), nil
}

func ParseSecretRef(ref string) (string, string, error) {
	if !strings.HasPrefix(ref, "keychain://") {
		return "", "", fmt.Errorf("invalid secret ref %q: expected keychain:// prefix", ref)
	}
	trimmed := strings.TrimPrefix(ref, "keychain://")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 3 {
		return "", "", fmt.Errorf("invalid secret ref %q: expected keychain://<service>/<profile>/<kind>", ref)
	}
	if parts[0] != KeychainService {
		return "", "", fmt.Errorf("invalid secret ref %q: unsupported service %q", ref, parts[0])
	}
	profile := strings.TrimSpace(parts[1])
	kind := strings.TrimSpace(parts[2])
	if profile == "" || kind == "" {
		return "", "", fmt.Errorf("invalid secret ref %q: empty profile or kind", ref)
	}
	if kind != SecretToken && kind != SecretAppSecret {
		return "", "", fmt.Errorf("invalid secret ref %q: unknown kind %q", ref, kind)
	}
	return profile, kind, nil
}

func (s *KeychainStore) Set(ref string, value string) error {
	profile, kind, err := ParseSecretRef(ref)
	if err != nil {
		return err
	}
	if strings.TrimSpace(value) == "" {
		return errors.New("secret value cannot be empty")
	}
	if err := s.backend.Set(s.service, accountName(profile, kind), value); err != nil {
		return fmt.Errorf("keychain set %q: %w", ref, err)
	}
	return nil
}

func (s *KeychainStore) Get(ref string) (string, error) {
	profile, kind, err := ParseSecretRef(ref)
	if err != nil {
		return "", err
	}
	value, err := s.backend.Get(s.service, accountName(profile, kind))
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", fmt.Errorf("keychain secret not found for %q: %w", ref, err)
		}
		return "", fmt.Errorf("keychain get %q: %w", ref, err)
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("keychain secret value is empty for %q", ref)
	}
	return value, nil
}

func (s *KeychainStore) Delete(ref string) error {
	profile, kind, err := ParseSecretRef(ref)
	if err != nil {
		return err
	}
	if err := s.backend.Delete(s.service, accountName(profile, kind)); err != nil {
		return fmt.Errorf("keychain delete %q: %w", ref, err)
	}
	return nil
}

func accountName(profile string, kind string) string {
	return fmt.Sprintf("%s:%s", profile, kind)
}
