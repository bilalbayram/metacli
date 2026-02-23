package ops

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bilalbayram/metacli/internal/changelog"
	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/schema"
)

const (
	StateSchemaVersion = 1
	BaselineVersion    = 4
)

const baselineStatusInitialized = "initialized"
const occSnapshotDigest = "occ.2025.stable"

var (
	ErrStatePathRequired    = errors.New("state path is required")
	ErrBaselineAlreadyExist = errors.New("baseline state already exists")
)

type BaselineState struct {
	SchemaVersion   int       `json:"schema_version"`
	BaselineVersion int       `json:"baseline_version"`
	Status          string    `json:"status"`
	Snapshots       Snapshots `json:"snapshots"`
}

type Snapshots struct {
	ChangelogOCC ChangelogOCCSnapshot       `json:"changelog_occ"`
	SchemaPack   SchemaPackSnapshot         `json:"schema_pack"`
	RateLimit    RateLimitTelemetrySnapshot `json:"rate_limit"`
}

type ChangelogOCCSnapshot struct {
	LatestVersion string `json:"latest_version"`
	OCCDigest     string `json:"occ_digest"`
}

type SchemaPackSnapshot struct {
	Domain  string `json:"domain"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

type RateLimitTelemetrySnapshot struct {
	AppCallCount     int `json:"app_call_count"`
	AppTotalCPUTime  int `json:"app_total_cputime"`
	AppTotalTime     int `json:"app_total_time"`
	PageCallCount    int `json:"page_call_count"`
	PageTotalCPUTime int `json:"page_total_cputime"`
	PageTotalTime    int `json:"page_total_time"`
	AdAccountUtilPct int `json:"ad_account_util_pct"`
}

func NewBaselineState() (BaselineState, error) {
	changelogSnapshot, err := captureChangelogOCCSnapshot(time.Now().UTC())
	if err != nil {
		return BaselineState{}, err
	}
	schemaPackSnapshot, err := captureSchemaPackSnapshot()
	if err != nil {
		return BaselineState{}, err
	}
	rateLimitSnapshot, err := captureRateLimitTelemetrySnapshot()
	if err != nil {
		return BaselineState{}, err
	}
	return BaselineState{
		SchemaVersion:   StateSchemaVersion,
		BaselineVersion: BaselineVersion,
		Status:          baselineStatusInitialized,
		Snapshots: Snapshots{
			ChangelogOCC: changelogSnapshot,
			SchemaPack:   schemaPackSnapshot,
			RateLimit:    rateLimitSnapshot,
		},
	}, nil
}

func DefaultStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".meta", "ops", "baseline-state.json"), nil
}

func InitBaseline(path string) (BaselineState, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return BaselineState{}, ErrStatePathRequired
	}

	if _, err := os.Stat(path); err == nil {
		return BaselineState{}, fmt.Errorf("%w at %s", ErrBaselineAlreadyExist, path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return BaselineState{}, fmt.Errorf("stat baseline state %s: %w", path, err)
	}

	state, err := NewBaselineState()
	if err != nil {
		return BaselineState{}, err
	}
	if err := SaveBaseline(path, state); err != nil {
		return BaselineState{}, err
	}
	return state, nil
}

func SaveBaseline(path string, state BaselineState) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return ErrStatePathRequired
	}
	if err := state.Validate(); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create baseline directory for %s: %w", path, err)
	}

	payload, err := marshalState(state)
	if err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(dir, ".baseline-*.json")
	if err != nil {
		return fmt.Errorf("create temp baseline file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(payload); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp baseline file: %w", err)
	}
	if err := tmpFile.Chmod(0o600); err != nil {
		tmpFile.Close()
		return fmt.Errorf("chmod temp baseline file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp baseline file: %w", err)
	}
	if err := os.Rename(tmpFile.Name(), path); err != nil {
		return fmt.Errorf("replace baseline state %s: %w", path, err)
	}
	return nil
}

func LoadBaseline(path string) (BaselineState, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return BaselineState{}, ErrStatePathRequired
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return BaselineState{}, fmt.Errorf("%w: baseline state does not exist at %s", os.ErrNotExist, path)
		}
		return BaselineState{}, fmt.Errorf("read baseline state %s: %w", path, err)
	}

	var state BaselineState
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return BaselineState{}, fmt.Errorf("decode baseline state %s: %w", path, err)
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return BaselineState{}, fmt.Errorf("decode baseline state %s: multiple JSON values", path)
		}
		return BaselineState{}, fmt.Errorf("decode baseline state %s: %w", path, err)
	}
	if err := state.Validate(); err != nil {
		return BaselineState{}, err
	}
	return state, nil
}

func (s BaselineState) Validate() error {
	if s.SchemaVersion != StateSchemaVersion {
		return fmt.Errorf("unsupported baseline schema_version=%d (expected %d)", s.SchemaVersion, StateSchemaVersion)
	}
	if s.BaselineVersion != BaselineVersion {
		return fmt.Errorf("unsupported baseline baseline_version=%d (expected %d)", s.BaselineVersion, BaselineVersion)
	}
	if s.Status != baselineStatusInitialized {
		return fmt.Errorf("baseline status must be %q", baselineStatusInitialized)
	}
	if err := s.Snapshots.Validate(); err != nil {
		return err
	}
	return nil
}

func (s Snapshots) Validate() error {
	if err := s.ChangelogOCC.Validate(); err != nil {
		return err
	}
	if err := s.SchemaPack.Validate(); err != nil {
		return err
	}
	if err := s.RateLimit.Validate(); err != nil {
		return err
	}
	return nil
}

func (s ChangelogOCCSnapshot) Validate() error {
	if strings.TrimSpace(s.LatestVersion) == "" {
		return errors.New("baseline changelog_occ.latest_version is required")
	}
	if strings.TrimSpace(s.OCCDigest) == "" {
		return errors.New("baseline changelog_occ.occ_digest is required")
	}
	return nil
}

func (s SchemaPackSnapshot) Validate() error {
	if strings.TrimSpace(s.Domain) == "" {
		return errors.New("baseline schema_pack.domain is required")
	}
	if strings.TrimSpace(s.Version) == "" {
		return errors.New("baseline schema_pack.version is required")
	}
	if strings.TrimSpace(s.SHA256) == "" {
		return errors.New("baseline schema_pack.sha256 is required")
	}
	return nil
}

func (s RateLimitTelemetrySnapshot) Validate() error {
	if err := validateUsagePercent("baseline rate_limit.app_call_count", s.AppCallCount); err != nil {
		return err
	}
	if err := validateUsagePercent("baseline rate_limit.app_total_cputime", s.AppTotalCPUTime); err != nil {
		return err
	}
	if err := validateUsagePercent("baseline rate_limit.app_total_time", s.AppTotalTime); err != nil {
		return err
	}
	if err := validateUsagePercent("baseline rate_limit.page_call_count", s.PageCallCount); err != nil {
		return err
	}
	if err := validateUsagePercent("baseline rate_limit.page_total_cputime", s.PageTotalCPUTime); err != nil {
		return err
	}
	if err := validateUsagePercent("baseline rate_limit.page_total_time", s.PageTotalTime); err != nil {
		return err
	}
	if err := validateUsagePercent("baseline rate_limit.ad_account_util_pct", s.AdAccountUtilPct); err != nil {
		return err
	}
	return nil
}

func captureChangelogOCCSnapshot(now time.Time) (ChangelogOCCSnapshot, error) {
	checker := changelog.NewChecker()
	result, err := checker.Check(config.DefaultGraphVersion, now.UTC())
	if err != nil {
		return ChangelogOCCSnapshot{}, fmt.Errorf("capture changelog snapshot: %w", err)
	}
	return ChangelogOCCSnapshot{
		LatestVersion: result.LatestVersion,
		OCCDigest:     occSnapshotDigest,
	}, nil
}

func captureSchemaPackSnapshot() (SchemaPackSnapshot, error) {
	path, err := resolveSchemaPackSnapshotSource(config.DefaultDomain, config.DefaultGraphVersion)
	if err != nil {
		return SchemaPackSnapshot{}, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return SchemaPackSnapshot{}, fmt.Errorf("read schema pack snapshot source %s: %w", path, err)
	}
	sum := sha256.Sum256(body)
	return SchemaPackSnapshot{
		Domain:  config.DefaultDomain,
		Version: config.DefaultGraphVersion,
		SHA256:  hex.EncodeToString(sum[:]),
	}, nil
}

func captureRateLimitTelemetrySnapshot() (RateLimitTelemetrySnapshot, error) {
	snapshot := RateLimitTelemetrySnapshot{
		AppCallCount:     0,
		AppTotalCPUTime:  0,
		AppTotalTime:     0,
		PageCallCount:    0,
		PageTotalCPUTime: 0,
		PageTotalTime:    0,
		AdAccountUtilPct: 0,
	}
	if err := snapshot.Validate(); err != nil {
		return RateLimitTelemetrySnapshot{}, err
	}
	return snapshot, nil
}

func resolveSchemaPackSnapshotSource(domain string, version string) (string, error) {
	relative := filepath.Join(schema.DefaultSchemaDir, domain, version+".json")
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory for schema pack snapshot source: %w", err)
	}

	current := wd
	for {
		candidate := filepath.Join(current, relative)
		info, statErr := os.Stat(candidate)
		if statErr == nil && !info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", fmt.Errorf("schema pack snapshot source not found for %s/%s at %s", domain, version, relative)
}

func validateUsagePercent(name string, value int) error {
	if value < 0 || value > 100 {
		return fmt.Errorf("%s must be between 0 and 100 (got %d)", name, value)
	}
	return nil
}

func marshalState(state BaselineState) ([]byte, error) {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal baseline state: %w", err)
	}
	return append(data, '\n'), nil
}
