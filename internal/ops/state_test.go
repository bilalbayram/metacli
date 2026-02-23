package ops

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitBaselineWritesDeterministicStateFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "ops", "baseline-state.json")
	state, err := InitBaseline(path)
	if err != nil {
		t.Fatalf("init baseline: %v", err)
	}

	if state.SchemaVersion != StateSchemaVersion {
		t.Fatalf("unexpected schema version: got=%d want=%d", state.SchemaVersion, StateSchemaVersion)
	}
	if state.BaselineVersion != BaselineVersion {
		t.Fatalf("unexpected baseline version: got=%d want=%d", state.BaselineVersion, BaselineVersion)
	}
	if state.Status != baselineStatusInitialized {
		t.Fatalf("unexpected status: %s", state.Status)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}

	expected := "{\n  \"schema_version\": 1,\n  \"baseline_version\": 1,\n  \"status\": \"initialized\"\n}\n"
	if string(raw) != expected {
		t.Fatalf("unexpected state file contents:\n%s", string(raw))
	}
}

func TestLoadBaselineFailsOnUnknownField(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "baseline-state.json")
	raw := "{\n  \"schema_version\": 1,\n  \"baseline_version\": 1,\n  \"status\": \"initialized\",\n  \"extra\": true\n}\n"
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write baseline fixture: %v", err)
	}

	_, err := LoadBaseline(path)
	if err == nil {
		t.Fatal("expected load baseline to fail on unknown field")
	}
}
