package ops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendResourceLedgerEntryWritesDeterministicLedgerFile(t *testing.T) {
	t.Parallel()

	ledgerPath := filepath.Join(t.TempDir(), "resource-ledger.json")

	if _, err := AppendResourceLedgerEntry(ledgerPath, TrackedResource{
		Command:       "meta campaign create",
		ResourceKind:  ResourceKindCampaign,
		ResourceID:    "cmp_1001",
		CleanupAction: CleanupActionPause,
		Profile:       "prod",
		GraphVersion:  "v25.0",
		AccountID:     "1234",
		Metadata: map[string]string{
			"operation": "create",
		},
	}); err != nil {
		t.Fatalf("append campaign tracked resource: %v", err)
	}

	if _, err := AppendResourceLedgerEntry(ledgerPath, TrackedResource{
		Command:       "meta adset create",
		ResourceKind:  ResourceKindAdSet,
		ResourceID:    "adset_2001",
		CleanupAction: CleanupActionPause,
		Profile:       "prod",
		GraphVersion:  "v25.0",
		AccountID:     "1234",
		Metadata: map[string]string{
			"operation": "create",
		},
	}); err != nil {
		t.Fatalf("append adset tracked resource: %v", err)
	}

	raw, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}

	expected := "{\n  \"schema_version\": 1,\n  \"resources\": [\n    {\n      \"sequence\": 1,\n      \"command\": \"meta campaign create\",\n      \"resource_kind\": \"campaign\",\n      \"resource_id\": \"cmp_1001\",\n      \"cleanup_action\": \"pause\",\n      \"profile\": \"prod\",\n      \"graph_version\": \"v25.0\",\n      \"account_id\": \"1234\",\n      \"metadata\": {\n        \"operation\": \"create\"\n      }\n    },\n    {\n      \"sequence\": 2,\n      \"command\": \"meta adset create\",\n      \"resource_kind\": \"adset\",\n      \"resource_id\": \"adset_2001\",\n      \"cleanup_action\": \"pause\",\n      \"profile\": \"prod\",\n      \"graph_version\": \"v25.0\",\n      \"account_id\": \"1234\",\n      \"metadata\": {\n        \"operation\": \"create\"\n      }\n    }\n  ]\n}\n"
	if string(raw) != expected {
		t.Fatalf("unexpected ledger file contents:\n%s", string(raw))
	}
}

func TestAppendResourceLedgerEntryRejectsConflictingDuplicateIdentity(t *testing.T) {
	t.Parallel()

	ledgerPath := filepath.Join(t.TempDir(), "resource-ledger.json")
	first := TrackedResource{
		Command:       "meta campaign create",
		ResourceKind:  ResourceKindCampaign,
		ResourceID:    "cmp_1001",
		CleanupAction: CleanupActionPause,
		Profile:       "prod",
		GraphVersion:  "v25.0",
		AccountID:     "1234",
	}
	if _, err := AppendResourceLedgerEntry(ledgerPath, first); err != nil {
		t.Fatalf("append first resource: %v", err)
	}

	_, err := AppendResourceLedgerEntry(ledgerPath, TrackedResource{
		Command:       "meta campaign clone",
		ResourceKind:  ResourceKindCampaign,
		ResourceID:    "cmp_1001",
		CleanupAction: CleanupActionPause,
		Profile:       "prod",
		GraphVersion:  "v25.0",
		AccountID:     "1234",
	})
	if err == nil {
		t.Fatal("expected conflicting duplicate identity to fail")
	}
	if !strings.Contains(err.Error(), "resource ledger entry conflict") {
		t.Fatalf("unexpected error: %v", err)
	}
}
