package changelog

import (
	"testing"
	"time"
)

func TestCheckDetectsNeedsUpgradeForOlderVersion(t *testing.T) {
	t.Parallel()

	checker := NewChecker()
	result, err := checker.Check("v24.0", time.Date(2026, time.February, 23, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("check version: %v", err)
	}
	if !result.NeedsUpgrade {
		t.Fatal("expected upgrade to be required for older version")
	}
	if result.IsLatest {
		t.Fatal("expected non-latest version")
	}
}

func TestCheckWarnsNearDeprecation(t *testing.T) {
	t.Parallel()

	checker := NewChecker()
	result, err := checker.Check("v25.0", time.Date(2026, time.September, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("check version: %v", err)
	}
	if result.Warning == "" {
		t.Fatal("expected warning near deprecation")
	}
}
