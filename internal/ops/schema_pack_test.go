package ops

import (
	"strings"
	"testing"
)

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
	if !strings.Contains(check.Message, "domain="+snapshot.Domain) {
		t.Fatalf("expected schema domain in pass message, got %q", check.Message)
	}
	if !strings.Contains(check.Message, "version="+snapshot.Version) {
		t.Fatalf("expected schema version in pass message, got %q", check.Message)
	}
	if !strings.Contains(check.Message, "sha256="+snapshot.SHA256) {
		t.Fatalf("expected schema fingerprint in pass message, got %q", check.Message)
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
	if !strings.Contains(check.Message, "baseline domain="+baseline.Domain+" version="+baseline.Version+" sha256="+baseline.SHA256) {
		t.Fatalf("expected baseline schema fingerprint in drift message, got %q", check.Message)
	}
	if !strings.Contains(check.Message, "current domain="+current.Domain+" version="+current.Version+" sha256="+current.SHA256) {
		t.Fatalf("expected current schema fingerprint in drift message, got %q", check.Message)
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
