package smoke_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMain(m *testing.M) {
	tempHome, err := os.MkdirTemp("", "metacli-ops-smoke-home-*")
	if err != nil {
		panic(err)
	}

	if err := seedDefaultSchemaDir(tempHome, "../../../../schema-packs/marketing/v25.0.json"); err != nil {
		_ = os.RemoveAll(tempHome)
		panic(err)
	}

	if err := os.Setenv("HOME", tempHome); err != nil {
		_ = os.RemoveAll(tempHome)
		panic(err)
	}

	code := m.Run()
	_ = os.RemoveAll(tempHome)
	os.Exit(code)
}

func seedDefaultSchemaDir(tempHome string, repoPackRelativePath string) error {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return os.ErrNotExist
	}

	repoPackPath := filepath.Join(filepath.Dir(currentFile), repoPackRelativePath)
	body, err := os.ReadFile(repoPackPath)
	if err != nil {
		return err
	}

	targetPath := filepath.Join(tempHome, ".meta", "schema-packs", "marketing", "v25.0.json")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}

	return os.WriteFile(targetPath, body, 0o644)
}
