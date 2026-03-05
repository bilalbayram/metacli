package auth

import "github.com/zalando/go-keyring"

// NewSecretStore returns the best available SecretStore for the current environment.
// It tries the OS keychain first; if unavailable (containers, CI, headless servers),
// it falls back to a file-backed store at ~/.meta/secrets.json.
func NewSecretStore() SecretStore {
	// Probe keychain availability with a harmless read.
	_, err := keyring.Get(KeychainService, "__probe__")
	if err == keyring.ErrNotFound {
		// Keychain is reachable — the key just doesn't exist.
		return NewKeychainStore()
	}
	if err == nil {
		// Shouldn't happen for a probe key, but keychain is reachable.
		return NewKeychainStore()
	}

	// Any other error means the keychain backend is unavailable.
	path, pathErr := DefaultFileStorePath()
	if pathErr != nil {
		// Can't resolve home dir — fall back to keychain anyway and let it fail later.
		return NewKeychainStore()
	}
	return NewFileStore(path)
}
