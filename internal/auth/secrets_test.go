package auth

import (
	"errors"
	"testing"
)

type memoryKeyring struct {
	values map[string]string
}

func newMemoryKeyring() *memoryKeyring {
	return &memoryKeyring{values: map[string]string{}}
}

func (m *memoryKeyring) Set(service, user, password string) error {
	m.values[service+"::"+user] = password
	return nil
}

func (m *memoryKeyring) Get(service, user string) (string, error) {
	value, ok := m.values[service+"::"+user]
	if !ok {
		return "", errors.New("not found")
	}
	return value, nil
}

func (m *memoryKeyring) Delete(service, user string) error {
	delete(m.values, service+"::"+user)
	return nil
}

func TestSecretRefDeterministic(t *testing.T) {
	t.Parallel()

	ref, err := SecretRef("prod", SecretToken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	const expected = "keychain://meta-marketing-cli/prod/token"
	if ref != expected {
		t.Fatalf("unexpected ref: got=%s want=%s", ref, expected)
	}
}

func TestKeychainStoreRoundTrip(t *testing.T) {
	t.Parallel()

	mem := newMemoryKeyring()
	store := &KeychainStore{
		service: KeychainService,
		backend: mem,
	}

	ref, err := SecretRef("staging", SecretToken)
	if err != nil {
		t.Fatalf("secret ref: %v", err)
	}
	if err := store.Set(ref, "secret-value"); err != nil {
		t.Fatalf("set secret: %v", err)
	}
	value, err := store.Get(ref)
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if value != "secret-value" {
		t.Fatalf("unexpected secret value: %s", value)
	}
}

func TestParseSecretRefRejectsUnknownService(t *testing.T) {
	t.Parallel()

	_, _, err := ParseSecretRef("keychain://wrong/prod/token")
	if err == nil {
		t.Fatal("expected parse error for wrong service")
	}
}
