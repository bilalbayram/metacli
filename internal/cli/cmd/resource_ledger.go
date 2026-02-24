package cmd

import (
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
	ledgerPath := strings.TrimSpace(os.Getenv(resourceLedgerPathEnv))
	if ledgerPath == "" {
		return nil
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
		return fmt.Errorf("persist tracked resource in %s: %w", ledgerPath, err)
	}
	return nil
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
