package ops

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

type cleanupExecutorStub struct {
	paused       []string
	deleted      []string
	pauseErrors  map[string]error
	deleteErrors map[string]error
}

func (s *cleanupExecutorStub) Pause(_ context.Context, _ string, _ string, _ string, resourceID string) error {
	s.paused = append(s.paused, resourceID)
	if s.pauseErrors != nil {
		if err, exists := s.pauseErrors[resourceID]; exists {
			return err
		}
	}
	return nil
}

func (s *cleanupExecutorStub) Delete(_ context.Context, _ string, _ string, _ string, resourceID string) error {
	s.deleted = append(s.deleted, resourceID)
	if s.deleteErrors != nil {
		if err, exists := s.deleteErrors[resourceID]; exists {
			return err
		}
	}
	return nil
}

func TestCleanupResourceLedgerDryRunClassifiesWithoutMutatingLedger(t *testing.T) {
	t.Parallel()

	ledgerPath := filepath.Join(t.TempDir(), "resource-ledger.json")
	ledger := NewResourceLedger()
	ledger.Resources = append(ledger.Resources, TrackedResource{
		Sequence:      1,
		Command:       "meta campaign create",
		ResourceKind:  ResourceKindCampaign,
		ResourceID:    "cmp_1001",
		CleanupAction: CleanupActionPause,
	})
	ledger.Resources = append(ledger.Resources, TrackedResource{
		Sequence:      2,
		Command:       "meta audience create",
		ResourceKind:  ResourceKindAudience,
		ResourceID:    "aud_2001",
		CleanupAction: CleanupActionDelete,
	})
	if err := SaveResourceLedger(ledgerPath, ledger); err != nil {
		t.Fatalf("save ledger: %v", err)
	}

	result, err := CleanupResourceLedger(context.Background(), ledgerPath, CleanupOptions{})
	if err != nil {
		t.Fatalf("cleanup dry-run: %v", err)
	}

	if result.Mode != CleanupModeDryRun {
		t.Fatalf("unexpected cleanup mode: %s", result.Mode)
	}
	if result.Summary.Total != 2 || result.Summary.DryRun != 2 || result.Summary.Applied != 0 || result.Summary.Failed != 0 || result.Summary.Remaining != 2 {
		t.Fatalf("unexpected summary: %+v", result.Summary)
	}
	for _, resource := range result.Resources {
		if resource.Classification != CleanupClassificationDryRun {
			t.Fatalf("unexpected classification: %s", resource.Classification)
		}
		if !resource.Success {
			t.Fatal("expected dry-run resource classification to be successful")
		}
	}

	loaded, err := LoadResourceLedger(ledgerPath)
	if err != nil {
		t.Fatalf("reload ledger: %v", err)
	}
	if len(loaded.Resources) != 2 {
		t.Fatalf("expected dry-run to keep ledger resources, got %d", len(loaded.Resources))
	}
}

func TestCleanupResourceLedgerApplyPersistsOnlyFailedEntries(t *testing.T) {
	t.Parallel()

	ledgerPath := filepath.Join(t.TempDir(), "resource-ledger.json")
	ledger := NewResourceLedger()
	ledger.Resources = append(ledger.Resources, TrackedResource{
		Sequence:      1,
		Command:       "meta campaign create",
		ResourceKind:  ResourceKindCampaign,
		ResourceID:    "cmp_1001",
		CleanupAction: CleanupActionPause,
	})
	ledger.Resources = append(ledger.Resources, TrackedResource{
		Sequence:      2,
		Command:       "meta audience create",
		ResourceKind:  ResourceKindAudience,
		ResourceID:    "aud_2001",
		CleanupAction: CleanupActionDelete,
	})
	if err := SaveResourceLedger(ledgerPath, ledger); err != nil {
		t.Fatalf("save ledger: %v", err)
	}

	executor := &cleanupExecutorStub{
		deleteErrors: map[string]error{
			"aud_2001": errors.New("delete failed"),
		},
	}

	result, err := CleanupResourceLedger(context.Background(), ledgerPath, CleanupOptions{
		Apply:    true,
		Version:  "v25.0",
		Token:    "token",
		Executor: executor,
	})
	if err != nil {
		t.Fatalf("cleanup apply: %v", err)
	}

	if result.Mode != CleanupModeApply {
		t.Fatalf("unexpected cleanup mode: %s", result.Mode)
	}
	if result.Summary.Total != 2 || result.Summary.Applied != 1 || result.Summary.Failed != 1 || result.Summary.DryRun != 0 || result.Summary.Remaining != 1 {
		t.Fatalf("unexpected summary: %+v", result.Summary)
	}
	if len(executor.paused) != 1 || executor.paused[0] != "cmp_1001" {
		t.Fatalf("unexpected paused resources: %+v", executor.paused)
	}
	if len(executor.deleted) != 1 || executor.deleted[0] != "aud_2001" {
		t.Fatalf("unexpected deleted resources: %+v", executor.deleted)
	}

	loaded, err := LoadResourceLedger(ledgerPath)
	if err != nil {
		t.Fatalf("reload ledger: %v", err)
	}
	if len(loaded.Resources) != 1 {
		t.Fatalf("expected one remaining ledger entry, got %d", len(loaded.Resources))
	}
	if loaded.Resources[0].ResourceKind != ResourceKindAudience || loaded.Resources[0].ResourceID != "aud_2001" {
		t.Fatalf("unexpected remaining ledger resource: %+v", loaded.Resources[0])
	}
}
