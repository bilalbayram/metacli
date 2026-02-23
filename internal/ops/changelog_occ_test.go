package ops

import "testing"

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
}
