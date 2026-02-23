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
	"path/filepath"
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
		message, _ := json.Marshal(payload)
		signature := ed25519.Sign(priv, message)
		_ = json.NewEncoder(w).Encode(SignedManifest{
			Payload:   payload,
			Signature: base64.StdEncoding.EncodeToString(signature),
		})
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
