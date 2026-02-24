package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/bilalbayram/metacli/internal/ops"
)

const resourceLedgerPathEnv = "META_RESOURCE_LEDGER_PATH"

type trackedResourceInput struct {
	Command       string
	ResourceKind  string
	ResourceID    string
	CleanupAction string
	Profile       string
	GraphVersion  string
	AccountID     string
	SourceID      string
	Metadata      map[string]string
}

func persistTrackedResource(input trackedResourceInput) error {
	ledgerPath, explicitPath, err := resolveResourceLedgerPathForTracking("")
	if err != nil {
		if !explicitPath {
			return nil
		}
		return fmt.Errorf("resolve resource ledger path: %w", err)
	}

	entry := ops.TrackedResource{
		Command:       strings.TrimSpace(input.Command),
		ResourceKind:  strings.TrimSpace(input.ResourceKind),
		ResourceID:    strings.TrimSpace(input.ResourceID),
		CleanupAction: strings.TrimSpace(input.CleanupAction),
		Profile:       strings.TrimSpace(input.Profile),
		GraphVersion:  strings.TrimSpace(input.GraphVersion),
		AccountID:     strings.TrimSpace(input.AccountID),
		SourceID:      strings.TrimSpace(input.SourceID),
		Metadata:      normalizeTrackedResourceMetadata(input.Metadata),
	}
	if _, err := ops.AppendResourceLedgerEntry(ledgerPath, entry); err != nil {
		if !explicitPath && isResourceLedgerPathOrWriteError(err) {
			return nil
		}
		return fmt.Errorf("persist tracked resource in %s: %w", ledgerPath, err)
	}
	return nil
}

func resolveResourceLedgerPath(path string) (string, error) {
	resolvedPath, _, err := resolveResourceLedgerPathForTracking(path)
	return resolvedPath, err
}

func normalizeTrackedResourceMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(metadata))
	for key, value := range metadata {
		normalizedKey := strings.TrimSpace(key)
		normalizedValue := strings.TrimSpace(value)
		if normalizedKey == "" || normalizedValue == "" {
			continue
		}
		normalized[normalizedKey] = normalizedValue
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func resolveResourceLedgerPathForTracking(path string) (string, bool, error) {
	resolvedPath := strings.TrimSpace(path)
	if resolvedPath != "" {
		return resolvedPath, true, nil
	}

	envPath := strings.TrimSpace(os.Getenv(resourceLedgerPathEnv))
	if envPath != "" {
		return envPath, true, nil
	}

	defaultPath, err := ops.DefaultResourceLedgerPath()
	if err != nil {
		return "", false, err
	}
	return defaultPath, false, nil
}

func isResourceLedgerPathOrWriteError(err error) bool {
	if err == nil {
		return false
	}

	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return true
	}

	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return true
	}

	var syscallErr *os.SyscallError
	if errors.As(err, &syscallErr) {
		return true
	}

	return strings.Contains(err.Error(), "resource ledger path ") && strings.Contains(err.Error(), " is a directory")
}
