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
	"strconv"
	"strings"
	"time"
)

const (
	DefaultSchemaDir      = "schema-packs"
	DefaultManifestURL    = "https://raw.githubusercontent.com/bilalbayram/meta-marketing-cli-schema/main/stable/manifest.json"
	DefaultManifestPubKey = "Kwd20b0Rgz10RMMmLz57ShQ4m6fNnYw11f3UrhJ5j7A="

	SyncRemoteFailurePolicyHardFail    = "hard-fail"
	SyncRemoteFailurePolicyPinnedLocal = "pinned-local"

	SyncSourceRemote      = "remote"
	SyncSourcePinnedLocal = "pinned_local"

	SyncDriftSeverityError   = "error"
	SyncDriftSeverityWarning = "warning"
)

type Pack struct {
	Domain                 string              `json:"domain"`
	Version                string              `json:"version"`
	Entities               map[string][]string `json:"entities,omitempty"`
	EndpointParams         map[string][]string `json:"endpoint_params,omitempty"`
	EndpointRequiredParams map[string][]string `json:"endpoint_required_params,omitempty"`
	DeprecatedParams       map[string][]string `json:"deprecated_params,omitempty"`
}

type PackRef struct {
	Domain  string `json:"domain"`
	Version string `json:"version"`
	Path    string `json:"path,omitempty"`
}

type SyncRequest struct {
	Channel             string `json:"channel"`
	RemoteFailurePolicy string `json:"remote_failure_policy,omitempty"`
}

type SyncWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type DriftDiagnostic struct {
	Code           string `json:"code"`
	Severity       string `json:"severity"`
	Source         string `json:"source"`
	Domain         string `json:"domain,omitempty"`
	Version        string `json:"version,omitempty"`
	Path           string `json:"path,omitempty"`
	ExpectedSHA256 string `json:"expected_sha256,omitempty"`
	ActualSHA256   string `json:"actual_sha256,omitempty"`
	Message        string `json:"message"`
}

type SyncResult struct {
	Channel  string            `json:"channel"`
	Policy   string            `json:"policy"`
	Source   string            `json:"source"`
	Packs    []PackRef         `json:"packs"`
	Warnings []SyncWarning     `json:"warnings,omitempty"`
	Drift    []DriftDiagnostic `json:"drift,omitempty"`
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
	Channel     string         `json:"channel"`
	GeneratedAt string         `json:"generated_at"`
	Packs       []ManifestPack `json:"packs"`
}

type ManifestPack struct {
	Domain  string `json:"domain"`
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
}

type downloadedPack struct {
	Manifest   ManifestPack
	Body       []byte
	TargetPath string
}

type stagedPack struct {
	downloadedPack
	TempPath string
}

type commitRecord struct {
	TargetPath  string
	TempPath    string
	BackupPath  string
	HasExisting bool
	Committed   bool
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

func NormalizeRemoteFailurePolicy(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "", SyncRemoteFailurePolicyHardFail:
		return SyncRemoteFailurePolicyHardFail
	case SyncRemoteFailurePolicyPinnedLocal:
		return SyncRemoteFailurePolicyPinnedLocal
	default:
		return ""
	}
}

func ValidateRemoteFailurePolicy(policy string) error {
	if NormalizeRemoteFailurePolicy(policy) == "" {
		return fmt.Errorf("schema remote failure policy must be one of [%s %s], got %q", SyncRemoteFailurePolicyHardFail, SyncRemoteFailurePolicyPinnedLocal, policy)
	}
	return nil
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

	if _, err := verifyPackBytes(data, domain, version, "", path); err != nil {
		return nil, err
	}

	var pack Pack
	if err := json.Unmarshal(data, &pack); err != nil {
		return nil, fmt.Errorf("decode schema pack %s: %w", path, err)
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
	sortPackRefs(entries)
	return entries, nil
}

func (p *Provider) Sync(ctx context.Context, channel string) ([]PackRef, error) {
	result, err := p.SyncWithRequest(ctx, SyncRequest{Channel: channel, RemoteFailurePolicy: SyncRemoteFailurePolicyHardFail})
	if err != nil {
		return nil, err
	}
	return result.Packs, nil
}

func (p *Provider) SyncWithRequest(ctx context.Context, request SyncRequest) (SyncResult, error) {
	channel := strings.TrimSpace(request.Channel)
	if channel == "" {
		return SyncResult{}, errors.New("schema sync channel is required")
	}
	policy := NormalizeRemoteFailurePolicy(request.RemoteFailurePolicy)
	if policy == "" {
		return SyncResult{}, fmt.Errorf("schema remote failure policy must be one of [%s %s], got %q", SyncRemoteFailurePolicyHardFail, SyncRemoteFailurePolicyPinnedLocal, request.RemoteFailurePolicy)
	}

	signed, err := p.fetchManifest(ctx)
	if err != nil {
		return p.resolveRemoteFailure(channel, policy, nil, err)
	}
	if signed.Payload.Channel != channel {
		return p.resolveRemoteFailure(channel, policy, nil, fmt.Errorf("manifest channel mismatch: expected %s got %s", channel, signed.Payload.Channel))
	}
	if err := verifyManifestSignature(signed.Payload, signed.Signature, p.PublicKey); err != nil {
		return p.resolveRemoteFailure(channel, policy, nil, err)
	}

	downloaded, err := p.downloadManifestPacks(ctx, signed.Payload.Packs)
	if err != nil {
		return p.resolveRemoteFailure(channel, policy, &signed.Payload, err)
	}
	if err := p.commitDownloadedPacksAtomically(downloaded); err != nil {
		return p.resolveRemoteFailure(channel, policy, &signed.Payload, err)
	}

	packs := make([]PackRef, 0, len(downloaded))
	for _, item := range downloaded {
		packs = append(packs, PackRef{
			Domain:  item.Manifest.Domain,
			Version: item.Manifest.Version,
			Path:    item.TargetPath,
		})
	}
	sortPackRefs(packs)

	return SyncResult{
		Channel: channel,
		Policy:  policy,
		Source:  SyncSourceRemote,
		Packs:   packs,
	}, nil
}

func (p *Provider) resolveRemoteFailure(channel string, policy string, manifest *ManifestPayload, remoteErr error) (SyncResult, error) {
	if policy == SyncRemoteFailurePolicyHardFail {
		return SyncResult{}, remoteErr
	}

	packs, drift, err := p.resolvePinnedLocal(manifest)
	if err != nil {
		if len(drift) == 0 {
			return SyncResult{}, fmt.Errorf("schema sync remote failure: %w; pinned-local fallback failed: %v", remoteErr, err)
		}
		return SyncResult{}, fmt.Errorf(
			"schema sync remote failure: %w; pinned-local fallback failed: %v; blocking drift diagnostics=%s",
			remoteErr,
			err,
			driftSummary(drift),
		)
	}

	return SyncResult{
		Channel: channel,
		Policy:  policy,
		Source:  SyncSourcePinnedLocal,
		Packs:   packs,
		Warnings: []SyncWarning{
			{
				Code:    "remote_sync_failed",
				Message: fmt.Sprintf("remote schema sync failed; using integrity-checked pinned local packs: %s", remoteErr.Error()),
			},
		},
		Drift: drift,
	}, nil
}

func (p *Provider) resolvePinnedLocal(manifest *ManifestPayload) ([]PackRef, []DriftDiagnostic, error) {
	if manifest == nil || len(manifest.Packs) == 0 {
		packs, drift, err := p.verifyLocalPacksWithoutManifest()
		if err != nil {
			return nil, drift, err
		}
		if len(packs) == 0 {
			return nil, drift, errors.New("no local schema packs are available for pinned-local fallback")
		}
		return packs, drift, nil
	}
	return p.verifyLocalPacksAgainstManifest(manifest.Packs)
}

func (p *Provider) verifyLocalPacksWithoutManifest() ([]PackRef, []DriftDiagnostic, error) {
	refs, err := p.ListPacks()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, errors.New("schema pack directory does not exist")
		}
		return nil, nil, err
	}

	drift := make([]DriftDiagnostic, 0)
	verified := make([]PackRef, 0, len(refs))
	for _, ref := range refs {
		path := ref.Path
		if strings.TrimSpace(path) == "" {
			path = filepath.Join(p.BaseDir, ref.Domain, ref.Version+".json")
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			drift = append(drift, DriftDiagnostic{
				Code:     "local_pack_read_failed",
				Severity: SyncDriftSeverityError,
				Source:   "pinned_local",
				Domain:   ref.Domain,
				Version:  ref.Version,
				Path:     path,
				Message:  fmt.Sprintf("read local schema pack %s: %v", path, readErr),
			})
			continue
		}
		if _, verifyErr := verifyPackBytes(body, ref.Domain, ref.Version, "", path); verifyErr != nil {
			drift = append(drift, DriftDiagnostic{
				Code:     "local_pack_integrity_failed",
				Severity: SyncDriftSeverityError,
				Source:   "pinned_local",
				Domain:   ref.Domain,
				Version:  ref.Version,
				Path:     path,
				Message:  verifyErr.Error(),
			})
			continue
		}
		verified = append(verified, PackRef{Domain: ref.Domain, Version: ref.Version, Path: path})
	}

	sortPackRefs(verified)
	sortDriftDiagnostics(drift)
	if hasBlockingDriftDiagnostics(drift) {
		return nil, drift, errors.New("pinned-local fallback integrity verification failed")
	}
	if len(verified) == 0 {
		return nil, drift, errors.New("no local schema packs are available for pinned-local fallback")
	}
	return verified, drift, nil
}

func (p *Provider) verifyLocalPacksAgainstManifest(manifestPacks []ManifestPack) ([]PackRef, []DriftDiagnostic, error) {
	packs := append([]ManifestPack(nil), manifestPacks...)
	sortManifestPacks(packs)

	drift := make([]DriftDiagnostic, 0)
	refs := make([]PackRef, 0, len(packs))
	for _, pack := range packs {
		targetPath := filepath.Join(p.BaseDir, pack.Domain, pack.Version+".json")
		body, err := os.ReadFile(targetPath)
		if err != nil {
			code := "local_pack_read_failed"
			if errors.Is(err, os.ErrNotExist) {
				code = "local_pack_missing"
			}
			drift = append(drift, DriftDiagnostic{
				Code:           code,
				Severity:       SyncDriftSeverityError,
				Source:         "manifest_compare",
				Domain:         pack.Domain,
				Version:        pack.Version,
				Path:           targetPath,
				ExpectedSHA256: strings.ToLower(pack.SHA256),
				Message:        fmt.Sprintf("read local schema pack %s: %v", targetPath, err),
			})
			continue
		}
		actualSHA, verifyErr := verifyPackBytes(body, pack.Domain, pack.Version, "", targetPath)
		if verifyErr != nil {
			drift = append(drift, DriftDiagnostic{
				Code:           "local_pack_integrity_failed",
				Severity:       SyncDriftSeverityError,
				Source:         "manifest_compare",
				Domain:         pack.Domain,
				Version:        pack.Version,
				Path:           targetPath,
				ExpectedSHA256: strings.ToLower(pack.SHA256),
				ActualSHA256:   actualSHA,
				Message:        verifyErr.Error(),
			})
			continue
		}
		if !strings.EqualFold(actualSHA, pack.SHA256) {
			drift = append(drift, DriftDiagnostic{
				Code:           "local_pack_checksum_drift",
				Severity:       SyncDriftSeverityWarning,
				Source:         "manifest_compare",
				Domain:         pack.Domain,
				Version:        pack.Version,
				Path:           targetPath,
				ExpectedSHA256: strings.ToLower(pack.SHA256),
				ActualSHA256:   strings.ToLower(actualSHA),
				Message: fmt.Sprintf(
					"local schema pack checksum drift for %s/%s: expected %s got %s",
					pack.Domain,
					pack.Version,
					strings.ToLower(pack.SHA256),
					strings.ToLower(actualSHA),
				),
			})
		}
		refs = append(refs, PackRef{Domain: pack.Domain, Version: pack.Version, Path: targetPath})
	}

	sortPackRefs(refs)
	sortDriftDiagnostics(drift)
	if hasBlockingDriftDiagnostics(drift) {
		return nil, drift, errors.New("pinned-local fallback integrity verification failed")
	}
	if len(refs) == 0 {
		return nil, drift, errors.New("no local schema packs are available for pinned-local fallback")
	}
	return refs, drift, nil
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

func (p *Provider) downloadManifestPacks(ctx context.Context, packs []ManifestPack) ([]downloadedPack, error) {
	ordered := append([]ManifestPack(nil), packs...)
	sortManifestPacks(ordered)

	downloaded := make([]downloadedPack, 0, len(ordered))
	seenTargets := make(map[string]struct{}, len(ordered))
	for _, pack := range ordered {
		item, err := p.downloadManifestPack(ctx, pack)
		if err != nil {
			return nil, err
		}
		if _, exists := seenTargets[item.TargetPath]; exists {
			return nil, fmt.Errorf("manifest contains duplicate schema pack target %s", item.TargetPath)
		}
		seenTargets[item.TargetPath] = struct{}{}
		downloaded = append(downloaded, item)
	}
	return downloaded, nil
}

func (p *Provider) downloadManifestPack(ctx context.Context, pack ManifestPack) (downloadedPack, error) {
	if pack.Domain == "" || pack.Version == "" || pack.URL == "" || pack.SHA256 == "" {
		return downloadedPack{}, fmt.Errorf("manifest pack entry is incomplete for domain=%s version=%s", pack.Domain, pack.Version)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pack.URL, nil)
	if err != nil {
		return downloadedPack{}, fmt.Errorf("build schema pack request for %s/%s: %w", pack.Domain, pack.Version, err)
	}
	res, err := p.HTTPClient.Do(req)
	if err != nil {
		return downloadedPack{}, fmt.Errorf("download schema pack for %s/%s: %w", pack.Domain, pack.Version, err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return downloadedPack{}, fmt.Errorf("read schema pack for %s/%s: %w", pack.Domain, pack.Version, err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return downloadedPack{}, fmt.Errorf("schema pack request failed for %s/%s with status %d", pack.Domain, pack.Version, res.StatusCode)
	}

	targetPath := filepath.Join(p.BaseDir, pack.Domain, pack.Version+".json")
	actualSHA, err := verifyPackBytes(body, pack.Domain, pack.Version, pack.SHA256, targetPath)
	if err != nil {
		return downloadedPack{}, err
	}
	if !strings.EqualFold(actualSHA, pack.SHA256) {
		return downloadedPack{}, fmt.Errorf("schema pack checksum mismatch for %s/%s: expected %s got %s", pack.Domain, pack.Version, pack.SHA256, actualSHA)
	}

	return downloadedPack{
		Manifest:   pack,
		Body:       body,
		TargetPath: targetPath,
	}, nil
}

func (p *Provider) commitDownloadedPacksAtomically(downloaded []downloadedPack) error {
	if len(downloaded) == 0 {
		return nil
	}

	items := append([]downloadedPack(nil), downloaded...)
	sort.Slice(items, func(i, j int) bool {
		return items[i].TargetPath < items[j].TargetPath
	})

	staged, err := p.stageDownloadedPacks(items)
	if err != nil {
		return err
	}
	if err := p.commitStagedPacks(staged); err != nil {
		return err
	}
	return nil
}

func (p *Provider) stageDownloadedPacks(downloaded []downloadedPack) ([]stagedPack, error) {
	staged := make([]stagedPack, 0, len(downloaded))
	for _, item := range downloaded {
		targetDir := filepath.Dir(item.TargetPath)
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			p.cleanupStagedPacks(staged)
			return nil, fmt.Errorf("create schema pack directory %s: %w", targetDir, err)
		}
		tmp, err := os.CreateTemp(targetDir, ".schema-sync-*.tmp")
		if err != nil {
			p.cleanupStagedPacks(staged)
			return nil, fmt.Errorf("create staged schema pack file in %s: %w", targetDir, err)
		}
		tmpPath := tmp.Name()
		if _, err := tmp.Write(item.Body); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
			p.cleanupStagedPacks(staged)
			return nil, fmt.Errorf("write staged schema pack file %s: %w", tmpPath, err)
		}
		if err := tmp.Chmod(0o644); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
			p.cleanupStagedPacks(staged)
			return nil, fmt.Errorf("set mode on staged schema pack file %s: %w", tmpPath, err)
		}
		if err := tmp.Close(); err != nil {
			_ = os.Remove(tmpPath)
			p.cleanupStagedPacks(staged)
			return nil, fmt.Errorf("close staged schema pack file %s: %w", tmpPath, err)
		}

		stagedBody, err := os.ReadFile(tmpPath)
		if err != nil {
			_ = os.Remove(tmpPath)
			p.cleanupStagedPacks(staged)
			return nil, fmt.Errorf("read staged schema pack file %s: %w", tmpPath, err)
		}
		if _, err := verifyPackBytes(stagedBody, item.Manifest.Domain, item.Manifest.Version, item.Manifest.SHA256, tmpPath); err != nil {
			_ = os.Remove(tmpPath)
			p.cleanupStagedPacks(staged)
			return nil, err
		}

		staged = append(staged, stagedPack{
			downloadedPack: item,
			TempPath:       tmpPath,
		})
	}
	return staged, nil
}

func (p *Provider) commitStagedPacks(staged []stagedPack) error {
	records := make([]commitRecord, 0, len(staged))
	for _, item := range staged {
		record := commitRecord{
			TargetPath: item.TargetPath,
			TempPath:   item.TempPath,
		}
		if _, err := os.Stat(item.TargetPath); err == nil {
			backupPath := item.TargetPath + ".schema-sync-backup-" + strconv.FormatInt(time.Now().UnixNano(), 10)
			if err := os.Rename(item.TargetPath, backupPath); err != nil {
				records = append(records, record)
				rollbackErr := p.rollbackCommittedPacks(records)
				if rollbackErr != nil {
					return fmt.Errorf("backup existing schema pack file %s: %w (rollback failed: %v)", item.TargetPath, err, rollbackErr)
				}
				return fmt.Errorf("backup existing schema pack file %s: %w", item.TargetPath, err)
			}
			record.HasExisting = true
			record.BackupPath = backupPath
		} else if !errors.Is(err, os.ErrNotExist) {
			records = append(records, record)
			rollbackErr := p.rollbackCommittedPacks(records)
			if rollbackErr != nil {
				return fmt.Errorf("check existing schema pack file %s: %w (rollback failed: %v)", item.TargetPath, err, rollbackErr)
			}
			return fmt.Errorf("check existing schema pack file %s: %w", item.TargetPath, err)
		}

		if err := os.Rename(item.TempPath, item.TargetPath); err != nil {
			records = append(records, record)
			rollbackErr := p.rollbackCommittedPacks(records)
			if rollbackErr != nil {
				return fmt.Errorf("commit schema pack file %s: %w (rollback failed: %v)", item.TargetPath, err, rollbackErr)
			}
			return fmt.Errorf("commit schema pack file %s: %w", item.TargetPath, err)
		}

		record.Committed = true
		record.TempPath = ""
		records = append(records, record)
	}

	for _, record := range records {
		if record.HasExisting {
			_ = os.Remove(record.BackupPath)
		}
	}
	return nil
}

func (p *Provider) cleanupStagedPacks(staged []stagedPack) {
	for _, item := range staged {
		if strings.TrimSpace(item.TempPath) == "" {
			continue
		}
		_ = os.Remove(item.TempPath)
	}
}

func (p *Provider) rollbackCommittedPacks(records []commitRecord) error {
	rollbackErrs := make([]error, 0)
	for i := len(records) - 1; i >= 0; i-- {
		record := records[i]
		if record.Committed {
			if err := os.Remove(record.TargetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				rollbackErrs = append(rollbackErrs, fmt.Errorf("remove partially committed schema pack %s: %w", record.TargetPath, err))
			}
		}
		if record.HasExisting {
			if err := os.Rename(record.BackupPath, record.TargetPath); err != nil {
				rollbackErrs = append(rollbackErrs, fmt.Errorf("restore schema pack backup %s: %w", record.TargetPath, err))
			}
		}
		if strings.TrimSpace(record.TempPath) != "" {
			if err := os.Remove(record.TempPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				rollbackErrs = append(rollbackErrs, fmt.Errorf("remove staged schema pack file %s: %w", record.TempPath, err))
			}
		}
	}
	if len(rollbackErrs) == 0 {
		return nil
	}
	return errors.Join(rollbackErrs...)
}

func verifyPackBytes(body []byte, expectedDomain string, expectedVersion string, expectedSHA string, path string) (string, error) {
	sum := sha256.Sum256(body)
	actualSHA := hex.EncodeToString(sum[:])
	if strings.TrimSpace(expectedSHA) != "" && !strings.EqualFold(actualSHA, expectedSHA) {
		return actualSHA, fmt.Errorf(
			"schema pack checksum mismatch for %s/%s: expected %s got %s",
			expectedDomain,
			expectedVersion,
			expectedSHA,
			actualSHA,
		)
	}

	var pack Pack
	if err := json.Unmarshal(body, &pack); err != nil {
		return actualSHA, fmt.Errorf("decode schema pack %s: %w", path, err)
	}
	if pack.Domain != expectedDomain || pack.Version != expectedVersion {
		return actualSHA, fmt.Errorf(
			"schema pack identity mismatch in %s: expected %s/%s, got %s/%s",
			path,
			expectedDomain,
			expectedVersion,
			pack.Domain,
			pack.Version,
		)
	}
	return actualSHA, nil
}

func hasBlockingDriftDiagnostics(drift []DriftDiagnostic) bool {
	for _, diagnostic := range drift {
		if diagnostic.Severity == SyncDriftSeverityError {
			return true
		}
	}
	return false
}

func sortPackRefs(entries []PackRef) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Domain == entries[j].Domain {
			if entries[i].Version == entries[j].Version {
				return entries[i].Path < entries[j].Path
			}
			return entries[i].Version < entries[j].Version
		}
		return entries[i].Domain < entries[j].Domain
	})
}

func sortManifestPacks(packs []ManifestPack) {
	sort.Slice(packs, func(i, j int) bool {
		if packs[i].Domain == packs[j].Domain {
			if packs[i].Version == packs[j].Version {
				return packs[i].URL < packs[j].URL
			}
			return packs[i].Version < packs[j].Version
		}
		return packs[i].Domain < packs[j].Domain
	})
}

func sortDriftDiagnostics(diagnostics []DriftDiagnostic) {
	sort.Slice(diagnostics, func(i, j int) bool {
		if diagnostics[i].Severity != diagnostics[j].Severity {
			return diagnostics[i].Severity < diagnostics[j].Severity
		}
		if diagnostics[i].Code != diagnostics[j].Code {
			return diagnostics[i].Code < diagnostics[j].Code
		}
		if diagnostics[i].Domain != diagnostics[j].Domain {
			return diagnostics[i].Domain < diagnostics[j].Domain
		}
		if diagnostics[i].Version != diagnostics[j].Version {
			return diagnostics[i].Version < diagnostics[j].Version
		}
		if diagnostics[i].Path != diagnostics[j].Path {
			return diagnostics[i].Path < diagnostics[j].Path
		}
		return diagnostics[i].Message < diagnostics[j].Message
	})
}

func driftSummary(drift []DriftDiagnostic) string {
	if len(drift) == 0 {
		return ""
	}
	parts := make([]string, 0, len(drift))
	for _, item := range drift {
		parts = append(parts, fmt.Sprintf("%s:%s:%s/%s", item.Severity, item.Code, item.Domain, item.Version))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
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
