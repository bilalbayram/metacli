package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilalbayram/metacli/internal/ops"
)

func TestResolveResourceLedgerPathUsesFlagOverEnvOverDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	envPath := filepath.Join(t.TempDir(), "env-resource-ledger.json")
	flagPath := filepath.Join(t.TempDir(), "flag-resource-ledger.json")
	t.Setenv(resourceLedgerPathEnv, envPath)

	resolvedPath, err := resolveResourceLedgerPath(flagPath)
	if err != nil {
		t.Fatalf("resolve resource ledger path with flag: %v", err)
	}
	if resolvedPath != flagPath {
		t.Fatalf("unexpected flag-resolved path: got=%s want=%s", resolvedPath, flagPath)
	}

	resolvedPath, err = resolveResourceLedgerPath("")
	if err != nil {
		t.Fatalf("resolve resource ledger path with env: %v", err)
	}
	if resolvedPath != envPath {
		t.Fatalf("unexpected env-resolved path: got=%s want=%s", resolvedPath, envPath)
	}

	t.Setenv(resourceLedgerPathEnv, "")
	resolvedPath, err = resolveResourceLedgerPath("")
	if err != nil {
		t.Fatalf("resolve default resource ledger path: %v", err)
	}
	expectedDefaultPath := filepath.Join(home, ".meta", "ops", "resource-ledger.json")
	if resolvedPath != expectedDefaultPath {
		t.Fatalf("unexpected default-resolved path: got=%s want=%s", resolvedPath, expectedDefaultPath)
	}
}

func TestPersistTrackedResourceUsesDefaultResolvedLedgerPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(resourceLedgerPathEnv, "")

	if err := persistTrackedResource(trackedResourceInput{
		Command:       "meta campaign create",
		ResourceKind:  ops.ResourceKindCampaign,
		ResourceID:    "cmp_default_1001",
		CleanupAction: ops.CleanupActionPause,
	}); err != nil {
		t.Fatalf("persist tracked resource: %v", err)
	}

	defaultLedgerPath := filepath.Join(home, ".meta", "ops", "resource-ledger.json")
	ledger, err := ops.LoadResourceLedger(defaultLedgerPath)
	if err != nil {
		t.Fatalf("load default resource ledger: %v", err)
	}
	if len(ledger.Resources) != 1 {
		t.Fatalf("expected one tracked resource, got %d", len(ledger.Resources))
	}
	resource := ledger.Resources[0]
	if resource.Command != "meta campaign create" {
		t.Fatalf("unexpected command: %s", resource.Command)
	}
	if resource.ResourceKind != ops.ResourceKindCampaign || resource.ResourceID != "cmp_default_1001" {
		t.Fatalf("unexpected tracked resource identity: %+v", resource)
	}
}

func TestPersistTrackedResourceAllowsImplicitDefaultLedgerWriteFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(resourceLedgerPathEnv, "")

	blockingPath := filepath.Join(home, ".meta")
	if err := os.WriteFile(blockingPath, []byte("blocked"), 0o600); err != nil {
		t.Fatalf("write blocking default ledger path: %v", err)
	}

	if err := persistTrackedResource(trackedResourceInput{
		Command:       "meta ad create",
		ResourceKind:  ops.ResourceKindAd,
		ResourceID:    "ad_default_1001",
		CleanupAction: ops.CleanupActionPause,
	}); err != nil {
		t.Fatalf("persist tracked resource with implicit default ledger path failure should not error: %v", err)
	}
}

func TestPersistTrackedResourceFailsForExplicitEnvLedgerWriteFailure(t *testing.T) {
	blockingPath := filepath.Join(t.TempDir(), "ledger-parent-file")
	if err := os.WriteFile(blockingPath, []byte("blocked"), 0o600); err != nil {
		t.Fatalf("write blocking env ledger path: %v", err)
	}
	ledgerPath := filepath.Join(blockingPath, "resource-ledger.json")
	t.Setenv(resourceLedgerPathEnv, ledgerPath)

	err := persistTrackedResource(trackedResourceInput{
		Command:       "meta ad create",
		ResourceKind:  ops.ResourceKindAd,
		ResourceID:    "ad_env_1001",
		CleanupAction: ops.CleanupActionPause,
	})
	if err == nil {
		t.Fatalf("expected explicit env ledger path write failure to return error")
	}
	if !strings.Contains(err.Error(), "persist tracked resource in "+ledgerPath) {
		t.Fatalf("unexpected explicit env ledger error: %v", err)
	}
}

func TestOpsCleanupCommandReadsTrackedResourcesFromEnvLedgerPath(t *testing.T) {
	ledgerPath := filepath.Join(t.TempDir(), "resource-ledger.json")
	t.Setenv(resourceLedgerPathEnv, ledgerPath)

	if err := persistTrackedResource(trackedResourceInput{
		Command:       "meta audience create",
		ResourceKind:  ops.ResourceKindAudience,
		ResourceID:    "aud_env_1001",
		CleanupAction: ops.CleanupActionDelete,
	}); err != nil {
		t.Fatalf("persist tracked resource: %v", err)
	}

	stdout, stderr, err := executeOpsCommand(Runtime{}, "cleanup")
	if err != nil {
		t.Fatalf("execute ops cleanup with env ledger path: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	envelope := decodeOpsEnvelope(t, []byte(stdout))
	if !envelope.Success {
		t.Fatalf("expected success=true, got envelope=%+v", envelope)
	}

	var data ops.CleanupResult
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatalf("decode cleanup data: %v", err)
	}
	if data.LedgerPath != ledgerPath {
		t.Fatalf("unexpected cleanup ledger path: got=%s want=%s", data.LedgerPath, ledgerPath)
	}
	if data.Summary.Total != 1 || data.Summary.DryRun != 1 || data.Summary.Remaining != 1 {
		t.Fatalf("unexpected cleanup summary: %+v", data.Summary)
	}
	if len(data.Resources) != 1 || data.Resources[0].ResourceID != "aud_env_1001" {
		t.Fatalf("unexpected cleanup resources: %+v", data.Resources)
	}
}

func TestOpsCleanupCommandLedgerPathFlagOverridesEnv(t *testing.T) {
	envLedgerPath := filepath.Join(t.TempDir(), "env-resource-ledger.json")
	flagLedgerPath := filepath.Join(t.TempDir(), "flag-resource-ledger.json")
	t.Setenv(resourceLedgerPathEnv, envLedgerPath)

	if _, err := ops.AppendResourceLedgerEntry(envLedgerPath, ops.TrackedResource{
		Command:       "meta campaign create",
		ResourceKind:  ops.ResourceKindCampaign,
		ResourceID:    "cmp_env_1001",
		CleanupAction: ops.CleanupActionPause,
	}); err != nil {
		t.Fatalf("append env tracked resource: %v", err)
	}
	if _, err := ops.AppendResourceLedgerEntry(flagLedgerPath, ops.TrackedResource{
		Command:       "meta audience create",
		ResourceKind:  ops.ResourceKindAudience,
		ResourceID:    "aud_flag_2001",
		CleanupAction: ops.CleanupActionDelete,
	}); err != nil {
		t.Fatalf("append flag tracked resource: %v", err)
	}

	stdout, stderr, err := executeOpsCommand(Runtime{}, "cleanup", "--ledger-path", flagLedgerPath)
	if err != nil {
		t.Fatalf("execute ops cleanup with explicit ledger path: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	envelope := decodeOpsEnvelope(t, []byte(stdout))
	if !envelope.Success {
		t.Fatalf("expected success=true, got envelope=%+v", envelope)
	}

	var data ops.CleanupResult
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatalf("decode cleanup data: %v", err)
	}
	if data.LedgerPath != flagLedgerPath {
		t.Fatalf("unexpected cleanup ledger path: got=%s want=%s", data.LedgerPath, flagLedgerPath)
	}
	if data.Summary.Total != 1 {
		t.Fatalf("unexpected cleanup summary: %+v", data.Summary)
	}
	if len(data.Resources) != 1 || data.Resources[0].ResourceID != "aud_flag_2001" {
		t.Fatalf("unexpected cleanup resources: %+v", data.Resources)
	}
}

func configureTestResourceLedgerPath(t *testing.T) string {
	t.Helper()

	existingPath := strings.TrimSpace(os.Getenv(resourceLedgerPathEnv))
	if existingPath != "" {
		return existingPath
	}

	ledgerPath := filepath.Join(t.TempDir(), "resource-ledger.json")
	t.Setenv(resourceLedgerPathEnv, ledgerPath)
	return ledgerPath
}
