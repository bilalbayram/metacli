package schema

import (
	"context"
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
	"sync"
	"testing"
)

func TestSyncVerifiesManifestAndWritesPack(t *testing.T) {
	t.Parallel()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	packBytes := []byte(`{"domain":"marketing","version":"v25.0","entities":{"campaign":["id","name"]}}`)
	packSum := sha256.Sum256(packBytes)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/packs/marketing/v25.0.json" {
			_, _ = w.Write(packBytes)
			return
		}

		payload := ManifestPayload{
			Channel:     "stable",
			GeneratedAt: "2026-02-23T00:00:00Z",
			Packs: []ManifestPack{
				{
					Domain:  "marketing",
					Version: "v25.0",
					URL:     server.URL + "/packs/marketing/v25.0.json",
					SHA256:  hex.EncodeToString(packSum[:]),
				},
			},
		}
		writeSignedManifest(t, w, priv, payload)
	}))
	defer server.Close()

	baseDir := t.TempDir()
	provider := NewProvider(baseDir, server.URL+"/manifest.json", base64.StdEncoding.EncodeToString(pub))
	refs, err := provider.Sync(context.Background(), "stable")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 synced pack, got %d", len(refs))
	}
	gotPath := filepath.Join(baseDir, "marketing", "v25.0.json")
	if refs[0].Path != gotPath {
		t.Fatalf("unexpected synced path: got=%s want=%s", refs[0].Path, gotPath)
	}

	pack, err := provider.GetPack("marketing", "v25.0")
	if err != nil {
		t.Fatalf("get pack: %v", err)
	}
	if pack.Domain != "marketing" || pack.Version != "v25.0" {
		t.Fatalf("unexpected pack identity: %#v", pack)
	}
}

func TestSyncIsAtomicWhenRemotePackFails(t *testing.T) {
	t.Parallel()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}

	oldBytes := []byte(`{"domain":"marketing","version":"v25.0","entities":{"campaign":["id"]}}`)
	newBytes := []byte(`{"domain":"marketing","version":"v25.0","entities":{"campaign":["id","name"]}}`)
	newSum := sha256.Sum256(newBytes)

	baseDir := t.TempDir()
	marketingDir := filepath.Join(baseDir, "marketing")
	if err := os.MkdirAll(marketingDir, 0o755); err != nil {
		t.Fatalf("create local schema dir: %v", err)
	}
	v25Path := filepath.Join(marketingDir, "v25.0.json")
	if err := os.WriteFile(v25Path, oldBytes, 0o644); err != nil {
		t.Fatalf("write pre-existing pack: %v", err)
	}

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/packs/marketing/v25.0.json":
			_, _ = w.Write(newBytes)
			return
		case "/packs/marketing/v26.0.json":
			http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
			return
		}

		payload := ManifestPayload{
			Channel:     "stable",
			GeneratedAt: "2026-02-24T00:00:00Z",
			Packs: []ManifestPack{
				{
					Domain:  "marketing",
					Version: "v25.0",
					URL:     server.URL + "/packs/marketing/v25.0.json",
					SHA256:  hex.EncodeToString(newSum[:]),
				},
				{
					Domain:  "marketing",
					Version: "v26.0",
					URL:     server.URL + "/packs/marketing/v26.0.json",
					SHA256:  strings.Repeat("a", 64),
				},
			},
		}
		writeSignedManifest(t, w, priv, payload)
	}))
	defer server.Close()

	provider := NewProvider(baseDir, server.URL+"/manifest.json", base64.StdEncoding.EncodeToString(pub))
	_, err = provider.Sync(context.Background(), "stable")
	if err == nil {
		t.Fatal("expected sync to fail when one remote pack fails")
	}

	gotV25, err := os.ReadFile(v25Path)
	if err != nil {
		t.Fatalf("read v25 after failed sync: %v", err)
	}
	if string(gotV25) != string(oldBytes) {
		t.Fatalf("expected pre-existing v25 pack to remain unchanged, got %q", string(gotV25))
	}
	if _, err := os.Stat(filepath.Join(marketingDir, "v26.0.json")); !os.IsNotExist(err) {
		t.Fatalf("expected v26 pack to not exist, got err=%v", err)
	}
}

func TestSyncWithRequestPinnedLocalFallsBackOnRemoteFailure(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	marketingDir := filepath.Join(baseDir, "marketing")
	if err := os.MkdirAll(marketingDir, 0o755); err != nil {
		t.Fatalf("create schema dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(marketingDir, "v25.0.json"), []byte(`{"domain":"marketing","version":"v25.0"}`), 0o644); err != nil {
		t.Fatalf("write local pack: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "manifest unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	provider := NewProvider(baseDir, server.URL+"/manifest.json", DefaultManifestPubKey)
	result, err := provider.SyncWithRequest(context.Background(), SyncRequest{
		Channel:             "stable",
		RemoteFailurePolicy: SyncRemoteFailurePolicyPinnedLocal,
	})
	if err != nil {
		t.Fatalf("sync with pinned-local policy: %v", err)
	}
	if result.Source != SyncSourcePinnedLocal {
		t.Fatalf("expected pinned-local source, got %q", result.Source)
	}
	if result.Policy != SyncRemoteFailurePolicyPinnedLocal {
		t.Fatalf("unexpected policy %q", result.Policy)
	}
	if len(result.Packs) != 1 {
		t.Fatalf("expected one local pack, got %d", len(result.Packs))
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Code != "remote_sync_failed" {
		t.Fatalf("unexpected warnings: %#v", result.Warnings)
	}
}

func TestSyncDownloadsManifestPacksDeterministically(t *testing.T) {
	t.Parallel()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}

	v25Body := []byte(`{"domain":"marketing","version":"v25.0"}`)
	v26Body := []byte(`{"domain":"marketing","version":"v26.0"}`)
	v25Sum := sha256.Sum256(v25Body)
	v26Sum := sha256.Sum256(v26Body)

	requestedPackPaths := make([]string, 0, 2)
	var requestedMu sync.Mutex

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/packs/marketing/v25.0.json":
			requestedMu.Lock()
			requestedPackPaths = append(requestedPackPaths, r.URL.Path)
			requestedMu.Unlock()
			_, _ = w.Write(v25Body)
			return
		case "/packs/marketing/v26.0.json":
			requestedMu.Lock()
			requestedPackPaths = append(requestedPackPaths, r.URL.Path)
			requestedMu.Unlock()
			_, _ = w.Write(v26Body)
			return
		}

		payload := ManifestPayload{
			Channel:     "stable",
			GeneratedAt: "2026-02-24T00:00:00Z",
			Packs: []ManifestPack{
				{
					Domain:  "marketing",
					Version: "v26.0",
					URL:     server.URL + "/packs/marketing/v26.0.json",
					SHA256:  hex.EncodeToString(v26Sum[:]),
				},
				{
					Domain:  "marketing",
					Version: "v25.0",
					URL:     server.URL + "/packs/marketing/v25.0.json",
					SHA256:  hex.EncodeToString(v25Sum[:]),
				},
			},
		}
		writeSignedManifest(t, w, priv, payload)
	}))
	defer server.Close()

	provider := NewProvider(t.TempDir(), server.URL+"/manifest.json", base64.StdEncoding.EncodeToString(pub))
	if _, err := provider.Sync(context.Background(), "stable"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	requestedMu.Lock()
	defer requestedMu.Unlock()
	if len(requestedPackPaths) != 2 {
		t.Fatalf("expected two pack downloads, got %#v", requestedPackPaths)
	}
	if requestedPackPaths[0] != "/packs/marketing/v25.0.json" || requestedPackPaths[1] != "/packs/marketing/v26.0.json" {
		t.Fatalf("expected deterministic download order [v25,v26], got %#v", requestedPackPaths)
	}
}

func TestSyncWithRequestHardFailOnRemoteFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "manifest unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	provider := NewProvider(t.TempDir(), server.URL+"/manifest.json", DefaultManifestPubKey)
	_, err := provider.SyncWithRequest(context.Background(), SyncRequest{
		Channel:             "stable",
		RemoteFailurePolicy: SyncRemoteFailurePolicyHardFail,
	})
	if err == nil {
		t.Fatal("expected hard-fail policy to return error")
	}
	if !strings.Contains(err.Error(), "schema manifest request failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSyncWithRequestPinnedLocalFailsOnLocalIntegrityDriftWithoutManifest(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	marketingDir := filepath.Join(baseDir, "marketing")
	if err := os.MkdirAll(marketingDir, 0o755); err != nil {
		t.Fatalf("create schema dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(marketingDir, "v25.0.json"), []byte(`{"domain":"other","version":"v25.0"}`), 0o644); err != nil {
		t.Fatalf("write local pack: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "manifest unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	provider := NewProvider(baseDir, server.URL+"/manifest.json", DefaultManifestPubKey)
	_, err := provider.SyncWithRequest(context.Background(), SyncRequest{
		Channel:             "stable",
		RemoteFailurePolicy: SyncRemoteFailurePolicyPinnedLocal,
	})
	if err == nil {
		t.Fatal("expected pinned-local fallback to fail on local integrity drift")
	}
	if !strings.Contains(err.Error(), "blocking drift diagnostics=error:local_pack_integrity_failed:marketing/v25.0") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSyncWithRequestPinnedLocalEmitsChecksumDriftDiagnostics(t *testing.T) {
	t.Parallel()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}

	baseDir := t.TempDir()
	marketingDir := filepath.Join(baseDir, "marketing")
	if err := os.MkdirAll(marketingDir, 0o755); err != nil {
		t.Fatalf("create schema dir: %v", err)
	}
	localBody := []byte(`{"domain":"marketing","version":"v25.0","entities":{"campaign":["id"]}}`)
	if err := os.WriteFile(filepath.Join(marketingDir, "v25.0.json"), localBody, 0o644); err != nil {
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
		payload := ManifestPayload{
			Channel:     "stable",
			GeneratedAt: "2026-02-24T00:00:00Z",
			Packs: []ManifestPack{
				{
					Domain:  "marketing",
					Version: "v25.0",
					URL:     server.URL + "/packs/marketing/v25.0.json",
					SHA256:  hex.EncodeToString(remoteSum[:]),
				},
			},
		}
		writeSignedManifest(t, w, priv, payload)
	}))
	defer server.Close()

	provider := NewProvider(baseDir, server.URL+"/manifest.json", base64.StdEncoding.EncodeToString(pub))
	result, err := provider.SyncWithRequest(context.Background(), SyncRequest{
		Channel:             "stable",
		RemoteFailurePolicy: SyncRemoteFailurePolicyPinnedLocal,
	})
	if err != nil {
		t.Fatalf("sync with pinned-local policy: %v", err)
	}
	if len(result.Drift) != 1 {
		t.Fatalf("expected one drift diagnostic, got %d", len(result.Drift))
	}
	diagnostic := result.Drift[0]
	if diagnostic.Code != "local_pack_checksum_drift" {
		t.Fatalf("unexpected drift diagnostic code: %s", diagnostic.Code)
	}
	if diagnostic.Severity != SyncDriftSeverityWarning {
		t.Fatalf("expected warning drift severity, got %s", diagnostic.Severity)
	}
	if diagnostic.ExpectedSHA256 == diagnostic.ActualSHA256 {
		t.Fatalf("expected checksum drift details, got %+v", diagnostic)
	}
}

func TestSyncWithRequestPinnedLocalFailsWhenManifestPackMissingLocally(t *testing.T) {
	t.Parallel()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}

	remoteBody := []byte(`{"domain":"marketing","version":"v25.0"}`)
	remoteSum := sha256.Sum256(remoteBody)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/packs/marketing/v25.0.json" {
			http.Error(w, "pack unavailable", http.StatusServiceUnavailable)
			return
		}
		payload := ManifestPayload{
			Channel:     "stable",
			GeneratedAt: "2026-02-24T00:00:00Z",
			Packs: []ManifestPack{
				{
					Domain:  "marketing",
					Version: "v25.0",
					URL:     server.URL + "/packs/marketing/v25.0.json",
					SHA256:  hex.EncodeToString(remoteSum[:]),
				},
			},
		}
		writeSignedManifest(t, w, priv, payload)
	}))
	defer server.Close()

	provider := NewProvider(t.TempDir(), server.URL+"/manifest.json", base64.StdEncoding.EncodeToString(pub))
	_, err = provider.SyncWithRequest(context.Background(), SyncRequest{
		Channel:             "stable",
		RemoteFailurePolicy: SyncRemoteFailurePolicyPinnedLocal,
	})
	if err == nil {
		t.Fatal("expected pinned-local fallback to fail when required local pack is missing")
	}
	if !strings.Contains(err.Error(), "blocking drift diagnostics") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRemoteFailurePolicy(t *testing.T) {
	t.Parallel()

	if err := ValidateRemoteFailurePolicy("hard-fail"); err != nil {
		t.Fatalf("hard-fail should be valid: %v", err)
	}
	if err := ValidateRemoteFailurePolicy("pinned-local"); err != nil {
		t.Fatalf("pinned-local should be valid: %v", err)
	}
	if err := ValidateRemoteFailurePolicy("not-valid"); err == nil {
		t.Fatal("expected invalid policy to fail")
	}
}

func writeSignedManifest(t *testing.T, w http.ResponseWriter, key ed25519.PrivateKey, payload ManifestPayload) {
	t.Helper()

	message, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	signature := ed25519.Sign(key, message)
	if err := json.NewEncoder(w).Encode(SignedManifest{
		Payload:   payload,
		Signature: base64.StdEncoding.EncodeToString(signature),
	}); err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
}
