package ops

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const ResourceLedgerSchemaVersion = 1

const (
	ResourceKindCampaign = "campaign"
	ResourceKindAdSet    = "adset"
	ResourceKindAd       = "ad"
	ResourceKindCreative = "creative"
	ResourceKindAudience = "audience"
)

const (
	CleanupActionPause  = "pause"
	CleanupActionDelete = "delete"
)

var (
	ErrResourceLedgerPathRequired = errors.New("resource ledger path is required")
	ErrResourceKindRequired       = errors.New("resource kind is required")
	ErrResourceIDRequired         = errors.New("resource id is required")
	ErrCleanupActionRequired      = errors.New("cleanup action is required")
	ErrResourceCommandRequired    = errors.New("resource command is required")
)

var allowedCleanupActionsByResourceKind = map[string]map[string]struct{}{
	ResourceKindCampaign: {
		CleanupActionPause: {},
	},
	ResourceKindAdSet: {
		CleanupActionPause: {},
	},
	ResourceKindAd: {
		CleanupActionPause: {},
	},
	ResourceKindCreative: {
		CleanupActionDelete: {},
	},
	ResourceKindAudience: {
		CleanupActionDelete: {},
	},
}

type ResourceLedger struct {
	SchemaVersion int               `json:"schema_version"`
	Resources     []TrackedResource `json:"resources"`
}

type TrackedResource struct {
	Sequence      int               `json:"sequence"`
	Command       string            `json:"command"`
	ResourceKind  string            `json:"resource_kind"`
	ResourceID    string            `json:"resource_id"`
	CleanupAction string            `json:"cleanup_action"`
	Profile       string            `json:"profile,omitempty"`
	GraphVersion  string            `json:"graph_version,omitempty"`
	AccountID     string            `json:"account_id,omitempty"`
	SourceID      string            `json:"source_id,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

func NewResourceLedger() ResourceLedger {
	return ResourceLedger{
		SchemaVersion: ResourceLedgerSchemaVersion,
		Resources:     []TrackedResource{},
	}
}

func DefaultResourceLedgerPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".meta", "ops", "resource-ledger.json"), nil
}

func LoadResourceLedger(path string) (ResourceLedger, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return ResourceLedger{}, ErrResourceLedgerPathRequired
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ResourceLedger{}, fmt.Errorf("%w: resource ledger does not exist at %s", os.ErrNotExist, path)
		}
		return ResourceLedger{}, fmt.Errorf("read resource ledger %s: %w", path, err)
	}

	var ledger ResourceLedger
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&ledger); err != nil {
		return ResourceLedger{}, fmt.Errorf("decode resource ledger %s: %w", path, err)
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return ResourceLedger{}, fmt.Errorf("decode resource ledger %s: multiple JSON values", path)
		}
		return ResourceLedger{}, fmt.Errorf("decode resource ledger %s: %w", path, err)
	}
	if err := ledger.Validate(); err != nil {
		return ResourceLedger{}, err
	}
	return ledger, nil
}

func SaveResourceLedger(path string, ledger ResourceLedger) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return ErrResourceLedgerPathRequired
	}
	if err := ledger.Validate(); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create resource ledger directory for %s: %w", path, err)
	}

	payload, err := marshalResourceLedger(ledger)
	if err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(dir, ".resource-ledger-*.json")
	if err != nil {
		return fmt.Errorf("create temp resource ledger file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(payload); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp resource ledger file: %w", err)
	}
	if err := tmpFile.Chmod(0o600); err != nil {
		tmpFile.Close()
		return fmt.Errorf("chmod temp resource ledger file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp resource ledger file: %w", err)
	}
	if err := os.Rename(tmpFile.Name(), path); err != nil {
		return fmt.Errorf("replace resource ledger %s: %w", path, err)
	}
	return nil
}

func AppendResourceLedgerEntry(path string, entry TrackedResource) (ResourceLedger, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return ResourceLedger{}, ErrResourceLedgerPathRequired
	}

	entry = normalizeTrackedResource(entry)
	if err := entry.validate(false); err != nil {
		return ResourceLedger{}, err
	}

	ledger, err := loadResourceLedgerForAppend(path)
	if err != nil {
		return ResourceLedger{}, err
	}

	for _, existing := range ledger.Resources {
		if !sameTrackedResourceIdentity(existing, entry) {
			continue
		}
		if !trackedResourceEquivalent(existing, entry) {
			return ResourceLedger{}, fmt.Errorf(
				"resource ledger entry conflict for %s %s",
				existing.ResourceKind,
				existing.ResourceID,
			)
		}
		return ledger, nil
	}

	entry.Sequence = nextTrackedResourceSequence(ledger.Resources)
	ledger.Resources = append(ledger.Resources, entry)
	if err := SaveResourceLedger(path, ledger); err != nil {
		return ResourceLedger{}, err
	}
	return ledger, nil
}

func (l ResourceLedger) Validate() error {
	if l.SchemaVersion != ResourceLedgerSchemaVersion {
		return fmt.Errorf(
			"unsupported resource ledger schema_version=%d (expected %d)",
			l.SchemaVersion,
			ResourceLedgerSchemaVersion,
		)
	}

	lastSequence := 0
	seen := make(map[string]TrackedResource, len(l.Resources))
	for index, resource := range l.Resources {
		if err := resource.validate(true); err != nil {
			return fmt.Errorf("resource ledger resources[%d]: %w", index, err)
		}
		if resource.Sequence <= lastSequence {
			return fmt.Errorf(
				"resource ledger resources[%d] sequence must be strictly increasing (got %d after %d)",
				index,
				resource.Sequence,
				lastSequence,
			)
		}
		lastSequence = resource.Sequence
		key := trackedResourceIdentity(resource)
		if existing, exists := seen[key]; exists {
			return fmt.Errorf(
				"resource ledger contains duplicate resource identity %s at sequences %d and %d",
				key,
				existing.Sequence,
				resource.Sequence,
			)
		}
		seen[key] = resource
	}
	return nil
}

func (r TrackedResource) validate(requireSequence bool) error {
	if requireSequence && r.Sequence <= 0 {
		return errors.New("sequence must be greater than zero")
	}

	resourceKind := strings.TrimSpace(r.ResourceKind)
	if resourceKind == "" {
		return ErrResourceKindRequired
	}
	cleanupActions, ok := allowedCleanupActionsByResourceKind[resourceKind]
	if !ok {
		return fmt.Errorf("unsupported resource kind %q", r.ResourceKind)
	}

	resourceID := strings.TrimSpace(r.ResourceID)
	if resourceID == "" {
		return ErrResourceIDRequired
	}
	if strings.TrimSpace(r.Command) == "" {
		return ErrResourceCommandRequired
	}

	cleanupAction := strings.TrimSpace(r.CleanupAction)
	if cleanupAction == "" {
		return ErrCleanupActionRequired
	}
	if _, exists := cleanupActions[cleanupAction]; !exists {
		return fmt.Errorf(
			"cleanup action %q is not supported for resource kind %q",
			r.CleanupAction,
			r.ResourceKind,
		)
	}
	if err := validateTrackedResourceMetadata(r.Metadata); err != nil {
		return err
	}
	return nil
}

func normalizeTrackedResource(resource TrackedResource) TrackedResource {
	resource.Command = strings.TrimSpace(resource.Command)
	resource.ResourceKind = strings.TrimSpace(resource.ResourceKind)
	resource.ResourceID = strings.TrimSpace(resource.ResourceID)
	resource.CleanupAction = strings.TrimSpace(resource.CleanupAction)
	resource.Profile = strings.TrimSpace(resource.Profile)
	resource.GraphVersion = strings.TrimSpace(resource.GraphVersion)
	resource.AccountID = strings.TrimSpace(resource.AccountID)
	resource.SourceID = strings.TrimSpace(resource.SourceID)
	resource.Metadata = normalizeTrackedResourceMetadata(resource.Metadata)
	return resource
}

func validateTrackedResourceMetadata(metadata map[string]string) error {
	for key, value := range metadata {
		normalizedKey := strings.TrimSpace(key)
		if normalizedKey == "" {
			return errors.New("resource metadata keys cannot be empty")
		}
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("resource metadata value for %q cannot be empty", key)
		}
	}
	return nil
}

func normalizeTrackedResourceMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(metadata))
	for key, value := range metadata {
		normalized[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return normalized
}

func nextTrackedResourceSequence(resources []TrackedResource) int {
	sequence := 0
	for _, resource := range resources {
		if resource.Sequence > sequence {
			sequence = resource.Sequence
		}
	}
	return sequence + 1
}

func trackedResourceIdentity(resource TrackedResource) string {
	return strings.TrimSpace(resource.ResourceKind) + ":" + strings.TrimSpace(resource.ResourceID)
}

func sameTrackedResourceIdentity(left TrackedResource, right TrackedResource) bool {
	return trackedResourceIdentity(left) == trackedResourceIdentity(right)
}

func trackedResourceEquivalent(existing TrackedResource, incoming TrackedResource) bool {
	return strings.TrimSpace(existing.Command) == strings.TrimSpace(incoming.Command) &&
		strings.TrimSpace(existing.ResourceKind) == strings.TrimSpace(incoming.ResourceKind) &&
		strings.TrimSpace(existing.ResourceID) == strings.TrimSpace(incoming.ResourceID) &&
		strings.TrimSpace(existing.CleanupAction) == strings.TrimSpace(incoming.CleanupAction) &&
		strings.TrimSpace(existing.Profile) == strings.TrimSpace(incoming.Profile) &&
		strings.TrimSpace(existing.GraphVersion) == strings.TrimSpace(incoming.GraphVersion) &&
		strings.TrimSpace(existing.AccountID) == strings.TrimSpace(incoming.AccountID) &&
		strings.TrimSpace(existing.SourceID) == strings.TrimSpace(incoming.SourceID) &&
		stringMapEqual(existing.Metadata, incoming.Metadata)
}

func stringMapEqual(left map[string]string, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, leftValue := range left {
		rightValue, exists := right[key]
		if !exists {
			return false
		}
		if leftValue != rightValue {
			return false
		}
	}
	return true
}

func loadResourceLedgerForAppend(path string) (ResourceLedger, error) {
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return ResourceLedger{}, fmt.Errorf("resource ledger path %s is a directory", path)
		}
		return LoadResourceLedger(path)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return ResourceLedger{}, fmt.Errorf("stat resource ledger %s: %w", path, err)
	}
	return NewResourceLedger(), nil
}

func marshalResourceLedger(ledger ResourceLedger) ([]byte, error) {
	data, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal resource ledger: %w", err)
	}
	return append(data, '\n'), nil
}
