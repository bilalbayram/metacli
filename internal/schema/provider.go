package schema

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	DefaultSchemaDir     = "schema-packs"
	DefaultManifestURL   = "https://raw.githubusercontent.com/bilalbayram/meta-marketing-cli-schema/main/stable/manifest.json"
	DefaultManifestPubKey = "Kwd20b0Rgz10RMMmLz57ShQ4m6fNnYw11f3UrhJ5j7A="
)

type Pack struct {
	Domain           string              `json:"domain"`
	Version          string              `json:"version"`
	Entities         map[string][]string `json:"entities,omitempty"`
	EndpointParams   map[string][]string `json:"endpoint_params,omitempty"`
	DeprecatedParams map[string][]string `json:"deprecated_params,omitempty"`
}

type PackRef struct {
	Domain  string `json:"domain"`
	Version string `json:"version"`
	Path    string `json:"path,omitempty"`
}

type Provider struct {
	BaseDir     string
	ManifestURL string
	PublicKey   string
	HTTPClient  *http.Client
}

type SignedManifest struct {
	Payload   ManifestPayload `json:"payload"`
	Signature string          `json:"signature"`
}

type ManifestPayload struct {
	Channel     string     `json:"channel"`
	GeneratedAt string     `json:"generated_at"`
	Packs       []ManifestPack `json:"packs"`
}

type ManifestPack struct {
	Domain  string `json:"domain"`
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
}

func NewProvider(baseDir string, manifestURL string, publicKey string) *Provider {
	if strings.TrimSpace(baseDir) == "" {
		baseDir = DefaultSchemaDir
	}
	if strings.TrimSpace(manifestURL) == "" {
		manifestURL = DefaultManifestURL
	}
	if strings.TrimSpace(publicKey) == "" {
		publicKey = DefaultManifestPubKey
	}
	return &Provider{
		BaseDir:     baseDir,
		ManifestURL: manifestURL,
		PublicKey:   publicKey,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *Provider) GetPack(domain string, version string) (*Pack, error) {
	if strings.TrimSpace(domain) == "" {
		return nil, errors.New("schema domain is required")
	}
	if strings.TrimSpace(version) == "" {
		return nil, errors.New("schema version is required")
	}

	path := filepath.Join(p.BaseDir, domain, version+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("schema pack not found for domain=%s version=%s at %s", domain, version, path)
		}
		return nil, fmt.Errorf("read schema pack %s: %w", path, err)
	}

	var pack Pack
	if err := json.Unmarshal(data, &pack); err != nil {
		return nil, fmt.Errorf("decode schema pack %s: %w", path, err)
	}
	if pack.Domain != domain || pack.Version != version {
		return nil, fmt.Errorf("schema pack identity mismatch in %s: expected %s/%s, got %s/%s", path, domain, version, pack.Domain, pack.Version)
	}
	return &pack, nil
}

func (p *Provider) ListPacks() ([]PackRef, error) {
	entries := make([]PackRef, 0)
	err := filepath.WalkDir(p.BaseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".json" {
			return nil
		}
		rel, err := filepath.Rel(p.BaseDir, path)
		if err != nil {
			return err
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) != 2 {
			return nil
		}
		version := strings.TrimSuffix(parts[1], ".json")
		entries = append(entries, PackRef{
			Domain:  parts[0],
			Version: version,
			Path:    path,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk schema packs: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Domain == entries[j].Domain {
			return entries[i].Version < entries[j].Version
		}
		return entries[i].Domain < entries[j].Domain
	})
	return entries, nil
}

func (p *Provider) Sync(ctx context.Context, channel string) ([]PackRef, error) {
	if strings.TrimSpace(channel) == "" {
		return nil, errors.New("schema sync channel is required")
	}
	signed, err := p.fetchManifest(ctx)
	if err != nil {
		return nil, err
	}
	if signed.Payload.Channel != channel {
		return nil, fmt.Errorf("manifest channel mismatch: expected %s got %s", channel, signed.Payload.Channel)
	}
	if err := verifyManifestSignature(signed.Payload, signed.Signature, p.PublicKey); err != nil {
		return nil, err
	}

	written := make([]PackRef, 0, len(signed.Payload.Packs))
	for _, pack := range signed.Payload.Packs {
		if err := p.syncPack(ctx, pack); err != nil {
			return nil, err
		}
		written = append(written, PackRef{
			Domain:  pack.Domain,
			Version: pack.Version,
			Path:    filepath.Join(p.BaseDir, pack.Domain, pack.Version+".json"),
		})
	}
	return written, nil
}

func (p *Provider) fetchManifest(ctx context.Context) (*SignedManifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.ManifestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build manifest request: %w", err)
	}
	res, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch schema manifest: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("read schema manifest: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("schema manifest request failed with status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	manifest := &SignedManifest{}
	if err := json.Unmarshal(body, manifest); err != nil {
		return nil, fmt.Errorf("decode schema manifest: %w", err)
	}
	if manifest.Signature == "" {
		return nil, errors.New("schema manifest signature is missing")
	}
	return manifest, nil
}

func (p *Provider) syncPack(ctx context.Context, pack ManifestPack) error {
	if pack.Domain == "" || pack.Version == "" || pack.URL == "" || pack.SHA256 == "" {
		return fmt.Errorf("manifest pack entry is incomplete for domain=%s version=%s", pack.Domain, pack.Version)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pack.URL, nil)
	if err != nil {
		return fmt.Errorf("build schema pack request for %s/%s: %w", pack.Domain, pack.Version, err)
	}
	res, err := p.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("download schema pack for %s/%s: %w", pack.Domain, pack.Version, err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("read schema pack for %s/%s: %w", pack.Domain, pack.Version, err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("schema pack request failed for %s/%s with status %d", pack.Domain, pack.Version, res.StatusCode)
	}
	sum := sha256.Sum256(body)
	actual := hex.EncodeToString(sum[:])
	if !strings.EqualFold(actual, pack.SHA256) {
		return fmt.Errorf("schema pack checksum mismatch for %s/%s: expected %s got %s", pack.Domain, pack.Version, pack.SHA256, actual)
	}

	targetDir := filepath.Join(p.BaseDir, pack.Domain)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create schema pack directory %s: %w", targetDir, err)
	}
	targetPath := filepath.Join(targetDir, pack.Version+".json")
	if err := os.WriteFile(targetPath, body, 0o644); err != nil {
		return fmt.Errorf("write schema pack file %s: %w", targetPath, err)
	}
	return nil
}

func verifyManifestSignature(payload ManifestPayload, signatureB64 string, pubKeyB64 string) error {
	pubKey, err := base64.StdEncoding.DecodeString(pubKeyB64)
	if err != nil {
		return fmt.Errorf("decode manifest public key: %w", err)
	}
	if len(pubKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid manifest public key length %d", len(pubKey))
	}
	signature, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return fmt.Errorf("decode manifest signature: %w", err)
	}
	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("invalid manifest signature length %d", len(signature))
	}
	message, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal manifest payload for verification: %w", err)
	}
	if !ed25519.Verify(ed25519.PublicKey(pubKey), message, signature) {
		return errors.New("schema manifest signature verification failed")
	}
	return nil
}
