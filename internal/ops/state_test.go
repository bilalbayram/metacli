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

	expected := "{\n  \"schema_version\": 1,\n  \"baseline_version\": 4,\n  \"status\": \"initialized\",\n  \"snapshots\": {\n    \"changelog_occ\": {\n      \"latest_version\": \"v25.0\",\n      \"occ_digest\": \"occ.2025.stable\"\n    },\n    \"schema_pack\": {\n      \"domain\": \"marketing\",\n      \"version\": \"v25.0\",\n      \"sha256\": \"94d9287ab1d2445304fdbf6e5fb9a09e2e4cec9f3c655337e7d33a0114793529\"\n    },\n    \"rate_limit\": {\n      \"app_call_count\": 0,\n      \"app_total_cputime\": 0,\n      \"app_total_time\": 0,\n      \"page_call_count\": 0,\n      \"page_total_cputime\": 0,\n      \"page_total_time\": 0,\n      \"ad_account_util_pct\": 0\n    }\n  }\n}\n"
	if string(raw) != expected {
		t.Fatalf("unexpected state file contents:\n%s", string(raw))
	}
}

func TestLoadBaselineFailsOnUnknownField(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "baseline-state.json")
	raw := "{\n  \"schema_version\": 1,\n  \"baseline_version\": 4,\n  \"status\": \"initialized\",\n  \"snapshots\": {\n    \"changelog_occ\": {\n      \"latest_version\": \"v25.0\",\n      \"occ_digest\": \"occ.2025.stable\"\n    },\n    \"schema_pack\": {\n      \"domain\": \"marketing\",\n      \"version\": \"v25.0\",\n      \"sha256\": \"94d9287ab1d2445304fdbf6e5fb9a09e2e4cec9f3c655337e7d33a0114793529\"\n    },\n    \"rate_limit\": {\n      \"app_call_count\": 0,\n      \"app_total_cputime\": 0,\n      \"app_total_time\": 0,\n      \"page_call_count\": 0,\n      \"page_total_cputime\": 0,\n      \"page_total_time\": 0,\n      \"ad_account_util_pct\": 0\n    }\n  },\n  \"extra\": true\n}\n"
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write baseline fixture: %v", err)
	}

	_, err := LoadBaseline(path)
	if err == nil {
		t.Fatal("expected load baseline to fail on unknown field")
	}
}
