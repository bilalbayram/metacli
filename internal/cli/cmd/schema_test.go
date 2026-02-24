package cmd

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilalbayram/metacli/internal/schema"
)

func TestSchemaSyncRejectsInvalidRemoteFailurePolicy(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeSchemaCommand(Runtime{}, "sync", "--remote-failure-policy", "maybe")
	if err == nil {
		t.Fatal("expected invalid policy to fail")
	}
	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}
	envelope := decodeEnvelope(t, []byte(stderr))
	errorBody, ok := envelope["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected structured error payload, got %T", envelope["error"])
	}
	message, _ := errorBody["message"].(string)
	if !strings.Contains(message, "schema remote failure policy must be one of") {
		t.Fatalf("unexpected error message: %q", message)
	}
}

func TestSchemaSyncPinnedLocalPolicyReturnsWarningResult(t *testing.T) {
	t.Parallel()

	schemaDir := t.TempDir()
	marketingDir := filepath.Join(schemaDir, "marketing")
	if err := os.MkdirAll(marketingDir, 0o755); err != nil {
		t.Fatalf("create schema dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(marketingDir, "v25.0.json"), []byte(`{"domain":"marketing","version":"v25.0"}`), 0o644); err != nil {
		t.Fatalf("write local pack: %v", err)
	}

	server := newStatusServer(t, 503, "manifest unavailable")
	defer server.Close()

	stdout, stderr, err := executeSchemaCommand(Runtime{},
		"sync",
		"--schema-dir", schemaDir,
		"--manifest-url", server.URL+"/manifest.json",
		"--remote-failure-policy", schema.SyncRemoteFailurePolicyPinnedLocal,
	)
	if err != nil {
		t.Fatalf("expected pinned-local sync to succeed, got %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	envelope := decodeEnvelope(t, []byte(stdout))
	assertEnvelopeBasics(t, envelope, "meta schema sync")

	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected sync result payload, got %T", envelope["data"])
	}
	if got, _ := data["source"].(string); got != schema.SyncSourcePinnedLocal {
		t.Fatalf("unexpected sync source %q", got)
	}
	warnings, ok := data["warnings"].([]any)
	if !ok || len(warnings) != 1 {
		t.Fatalf("expected one warning, got %#v", data["warnings"])
	}
	packs, ok := data["packs"].([]any)
	if !ok || len(packs) != 1 {
		t.Fatalf("expected one pack in sync result, got %#v", data["packs"])
	}
}

func TestSchemaSyncPinnedLocalPolicyReturnsStructuredDriftDiagnostics(t *testing.T) {
	t.Parallel()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}

	schemaDir := t.TempDir()
	marketingDir := filepath.Join(schemaDir, "marketing")
	if err := os.MkdirAll(marketingDir, 0o755); err != nil {
		t.Fatalf("create schema dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(marketingDir, "v25.0.json"), []byte(`{"domain":"marketing","version":"v25.0","entities":{"campaign":["id"]}}`), 0o644); err != nil {
		t.Fatalf("write local pack: %v", err)
	}

	remoteBody := []byte(`{"domain":"marketing","version":"v25.0","entities":{"campaign":["id","name"]}}`)
	remoteSum := sha256.Sum256(remoteBody)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/packs/marketing/v25.0.json" {
			http.Error(w, "pack unavailable", http.StatusServiceUnavailable)
			return
		}
		payload := schema.ManifestPayload{
			Channel:     "stable",
			GeneratedAt: "2026-02-24T00:00:00Z",
			Packs: []schema.ManifestPack{
				{
					Domain:  "marketing",
					Version: "v25.0",
					URL:     server.URL + "/packs/marketing/v25.0.json",
					SHA256:  hex.EncodeToString(remoteSum[:]),
				},
			},
		}
		writeSignedSchemaManifest(t, w, priv, payload)
	}))
	defer server.Close()

	stdout, stderr, err := executeSchemaCommand(Runtime{},
		"sync",
		"--schema-dir", schemaDir,
		"--manifest-url", server.URL+"/manifest.json",
		"--public-key", base64.StdEncoding.EncodeToString(pub),
		"--remote-failure-policy", schema.SyncRemoteFailurePolicyPinnedLocal,
	)
	if err != nil {
		t.Fatalf("expected pinned-local sync to succeed with diagnostics, got %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	envelope := decodeEnvelope(t, []byte(stdout))
	assertEnvelopeBasics(t, envelope, "meta schema sync")

	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected sync result payload, got %T", envelope["data"])
	}
	if got, _ := data["source"].(string); got != schema.SyncSourcePinnedLocal {
		t.Fatalf("unexpected sync source %q", got)
	}

	drift, ok := data["drift"].([]any)
	if !ok || len(drift) != 1 {
		t.Fatalf("expected one drift diagnostic, got %#v", data["drift"])
	}
	diagnostic, ok := drift[0].(map[string]any)
	if !ok {
		t.Fatalf("expected drift diagnostic object, got %T", drift[0])
	}
	if got, _ := diagnostic["code"].(string); got != "local_pack_checksum_drift" {
		t.Fatalf("unexpected drift code %q", got)
	}
	if got, _ := diagnostic["severity"].(string); got != schema.SyncDriftSeverityWarning {
		t.Fatalf("unexpected drift severity %q", got)
	}
	expectedSHA, _ := diagnostic["expected_sha256"].(string)
	actualSHA, _ := diagnostic["actual_sha256"].(string)
	if expectedSHA == "" || actualSHA == "" || expectedSHA == actualSHA {
		t.Fatalf("expected checksum drift fields, got expected=%q actual=%q", expectedSHA, actualSHA)
	}
}

func TestSchemaSyncHardFailPolicyReturnsStructuredError(t *testing.T) {
	t.Parallel()

	server := newStatusServer(t, 503, "manifest unavailable")
	defer server.Close()

	stdout, stderr, err := executeSchemaCommand(Runtime{},
		"sync",
		"--schema-dir", t.TempDir(),
		"--manifest-url", server.URL+"/manifest.json",
		"--remote-failure-policy", schema.SyncRemoteFailurePolicyHardFail,
	)
	if err == nil {
		t.Fatal("expected hard-fail policy to return error")
	}
	if stdout != "" {
		t.Fatalf("expected empty stdout on error, got %q", stdout)
	}
	envelope := decodeEnvelope(t, []byte(stderr))
	errorBody, ok := envelope["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected structured error payload, got %T", envelope["error"])
	}
	message, _ := errorBody["message"].(string)
	if !strings.Contains(message, "schema manifest request failed") {
		t.Fatalf("unexpected error message: %q", message)
	}
}

func executeSchemaCommand(runtime Runtime, args ...string) (string, string, error) {
	cmd := NewSchemaCommand(runtime)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func newStatusServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func writeSignedSchemaManifest(t *testing.T, w http.ResponseWriter, key ed25519.PrivateKey, payload schema.ManifestPayload) {
	t.Helper()

	message, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	signature := ed25519.Sign(key, message)
	if err := json.NewEncoder(w).Encode(schema.SignedManifest{
		Payload:   payload,
		Signature: base64.StdEncoding.EncodeToString(signature),
	}); err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
}
