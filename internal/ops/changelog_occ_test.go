package ops

import (
	"strings"
	"testing"
)

func TestEvaluateChangelogOCCDeltaPassesWhenSnapshotUnchanged(t *testing.T) {
	t.Parallel()

	snapshot := ChangelogOCCSnapshot{
		LatestVersion: "v25.0",
		OCCDigest:     occSnapshotDigest,
	}

	check := evaluateChangelogOCCDelta(snapshot, snapshot)
	if check.Name != checkNameChangelogOCCDelta {
		t.Fatalf("unexpected check name: %s", check.Name)
	}
	if check.Status != CheckStatusPass {
		t.Fatalf("unexpected status: got=%s want=%s", check.Status, CheckStatusPass)
	}
	if check.Blocking {
		t.Fatal("expected non-blocking check when snapshot is unchanged")
	}
	if !strings.Contains(check.Message, "latest_version="+snapshot.LatestVersion) {
		t.Fatalf("expected latest version fingerprint in pass message, got %q", check.Message)
	}
	if !strings.Contains(check.Message, "occ_digest="+snapshot.OCCDigest) {
		t.Fatalf("expected OCC digest fingerprint in pass message, got %q", check.Message)
	}
}

func TestEvaluateChangelogOCCDeltaFailsWhenSnapshotChanges(t *testing.T) {
	t.Parallel()

	baseline := ChangelogOCCSnapshot{
		LatestVersion: "v24.0",
		OCCDigest:     "occ.2024.stable",
	}
	current := ChangelogOCCSnapshot{
		LatestVersion: "v25.0",
		OCCDigest:     occSnapshotDigest,
	}

	check := evaluateChangelogOCCDelta(baseline, current)
	if check.Status != CheckStatusFail {
		t.Fatalf("unexpected status: got=%s want=%s", check.Status, CheckStatusFail)
	}
	if !check.Blocking {
		t.Fatal("expected blocking check when snapshot drifts")
	}
	if !strings.Contains(check.Message, "baseline latest_version="+baseline.LatestVersion) {
		t.Fatalf("expected baseline latest version in drift message, got %q", check.Message)
	}
	if !strings.Contains(check.Message, "baseline latest_version="+baseline.LatestVersion+" occ_digest="+baseline.OCCDigest) {
		t.Fatalf("expected baseline fingerprint in drift message, got %q", check.Message)
	}
	if !strings.Contains(check.Message, "current latest_version="+current.LatestVersion+" occ_digest="+current.OCCDigest) {
		t.Fatalf("expected current fingerprint in drift message, got %q", check.Message)
	}
}
