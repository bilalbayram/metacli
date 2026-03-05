package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileStoreRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewFileStore(filepath.Join(dir, "secrets.json"))

	ref, err := SecretRef("prod", SecretToken)
	if err != nil {
		t.Fatalf("secret ref: %v", err)
	}
	if err := store.Set(ref, "my-token"); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := store.Get(ref)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != "my-token" {
		t.Fatalf("got=%q want=%q", got, "my-token")
	}
}

func TestFileStoreDelete(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewFileStore(filepath.Join(dir, "secrets.json"))

	ref, _ := SecretRef("staging", SecretAppSecret)
	_ = store.Set(ref, "secret-123")
	if err := store.Delete(ref); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := store.Get(ref)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestFileStoreGetMissing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewFileStore(filepath.Join(dir, "secrets.json"))

	ref, _ := SecretRef("prod", SecretToken)
	_, err := store.Get(ref)
	if err == nil {
		t.Fatal("expected error for missing secret")
	}
}

func TestFileStoreRejectsEmptyValue(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewFileStore(filepath.Join(dir, "secrets.json"))

	ref, _ := SecretRef("prod", SecretToken)
	if err := store.Set(ref, "  "); err == nil {
		t.Fatal("expected error for empty value")
	}
}

func TestFileStoreCreatesDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "secrets.json")
	store := NewFileStore(nested)

	ref, _ := SecretRef("prod", SecretToken)
	if err := store.Set(ref, "val"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := os.Stat(nested); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}

func TestFileStoreFilePermissions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	p := filepath.Join(dir, "secrets.json")
	store := NewFileStore(p)

	ref, _ := SecretRef("prod", SecretToken)
	_ = store.Set(ref, "val")

	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("permissions=%o want=0600", perm)
	}
}

func TestFileStoreMultipleSecrets(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewFileStore(filepath.Join(dir, "secrets.json"))

	ref1, _ := SecretRef("prod", SecretToken)
	ref2, _ := SecretRef("prod", SecretAppSecret)
	ref3, _ := SecretRef("staging", SecretToken)

	_ = store.Set(ref1, "token-1")
	_ = store.Set(ref2, "secret-2")
	_ = store.Set(ref3, "token-3")

	v1, _ := store.Get(ref1)
	v2, _ := store.Get(ref2)
	v3, _ := store.Get(ref3)

	if v1 != "token-1" || v2 != "secret-2" || v3 != "token-3" {
		t.Fatalf("unexpected values: %q %q %q", v1, v2, v3)
	}
}
