package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileStore implements SecretStore using a JSON file on disk.
// Used as a fallback when the OS keychain (Secret Service / Keychain) is unavailable,
// e.g. in containers, CI, or headless servers.
type FileStore struct {
	path string
	mu   sync.Mutex
}

// NewFileStore creates a FileStore that persists secrets to the given path.
// The parent directory is created if it doesn't exist.
func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

// DefaultFileStorePath returns ~/.meta/secrets.json.
func DefaultFileStorePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".meta", "secrets.json"), nil
}

func (f *FileStore) Set(ref string, value string) error {
	_, _, err := ParseSecretRef(ref)
	if err != nil {
		return err
	}
	if strings.TrimSpace(value) == "" {
		return errors.New("secret value cannot be empty")
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	data, err := f.load()
	if err != nil {
		return err
	}
	data[ref] = value
	return f.save(data)
}

func (f *FileStore) Get(ref string) (string, error) {
	_, _, err := ParseSecretRef(ref)
	if err != nil {
		return "", err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	data, err := f.load()
	if err != nil {
		return "", err
	}
	value, ok := data[ref]
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("file secret not found for %q", ref)
	}
	return value, nil
}

func (f *FileStore) Delete(ref string) error {
	_, _, err := ParseSecretRef(ref)
	if err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	data, err := f.load()
	if err != nil {
		return err
	}
	delete(data, ref)
	return f.save(data)
}

func (f *FileStore) load() (map[string]string, error) {
	raw, err := os.ReadFile(f.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read secrets file: %w", err)
	}
	var data map[string]string
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parse secrets file: %w", err)
	}
	return data, nil
}

func (f *FileStore) save(data map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0700); err != nil {
		return fmt.Errorf("create secrets directory: %w", err)
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal secrets: %w", err)
	}
	if err := os.WriteFile(f.path, raw, 0600); err != nil {
		return fmt.Errorf("write secrets file: %w", err)
	}
	return nil
}
