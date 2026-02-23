package ops

import "testing"

func TestEvaluateSchemaPackDriftPassesWhenSnapshotUnchanged(t *testing.T) {
	t.Parallel()

	snapshot := SchemaPackSnapshot{
		Domain:  "marketing",
		Version: "v25.0",
		SHA256:  "abc123",
	}

	check := evaluateSchemaPackDrift(snapshot, snapshot)
	if check.Name != checkNameSchemaPackDrift {
		t.Fatalf("unexpected check name: %s", check.Name)
	}
	if check.Status != CheckStatusPass {
		t.Fatalf("unexpected status: got=%s want=%s", check.Status, CheckStatusPass)
	}
	if check.Blocking {
		t.Fatal("expected non-blocking check when schema snapshot is unchanged")
	}
}

func TestEvaluateSchemaPackDriftFailsWhenSnapshotChanges(t *testing.T) {
	t.Parallel()

	baseline := SchemaPackSnapshot{
		Domain:  "marketing",
		Version: "v25.0",
		SHA256:  "baseline",
	}
	current := SchemaPackSnapshot{
		Domain:  "marketing",
		Version: "v25.0",
		SHA256:  "current",
	}

	check := evaluateSchemaPackDrift(baseline, current)
	if check.Status != CheckStatusFail {
		t.Fatalf("unexpected status: got=%s want=%s", check.Status, CheckStatusFail)
	}
	if !check.Blocking {
		t.Fatal("expected blocking check when schema snapshot drifts")
	}
}

func TestCaptureSchemaPackSnapshot(t *testing.T) {
	t.Parallel()

	snapshot, err := captureSchemaPackSnapshot()
	if err != nil {
		t.Fatalf("capture schema pack snapshot: %v", err)
	}
	if snapshot.Domain != "marketing" {
		t.Fatalf("unexpected domain: %s", snapshot.Domain)
	}
	if snapshot.Version != "v25.0" {
		t.Fatalf("unexpected version: %s", snapshot.Version)
	}
	if snapshot.SHA256 != "432a308e09cb9e1c40c03e992a0f28d70600954f2cb1c939959512a1660a6774" {
		t.Fatalf("unexpected checksum: %s", snapshot.SHA256)
	}
}
