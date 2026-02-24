package ops

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bilalbayram/metacli/internal/graph"
)

const (
	CleanupModeDryRun = "dry_run"
	CleanupModeApply  = "apply"
)

const (
	CleanupClassificationDryRun  = "dry_run_planned"
	CleanupClassificationApplied = "applied"
	CleanupClassificationFailed  = "failed"
)

var (
	ErrCleanupApplyVersionRequired = errors.New("cleanup apply requires graph version")
	ErrCleanupApplyTokenRequired   = errors.New("cleanup apply requires access token")
)

type CleanupOptions struct {
	Apply     bool
	Version   string
	Token     string
	AppSecret string
	Executor  CleanupExecutor
}

type CleanupResult struct {
	LedgerPath string                  `json:"ledger_path"`
	Mode       string                  `json:"mode"`
	Summary    CleanupSummary          `json:"summary"`
	Resources  []CleanupResourceResult `json:"resources"`
}

type CleanupSummary struct {
	Total     int `json:"total"`
	DryRun    int `json:"dry_run"`
	Applied   int `json:"applied"`
	Failed    int `json:"failed"`
	Remaining int `json:"remaining"`
}

type CleanupResourceResult struct {
	Sequence       int    `json:"sequence"`
	Command        string `json:"command"`
	ResourceKind   string `json:"resource_kind"`
	ResourceID     string `json:"resource_id"`
	CleanupAction  string `json:"cleanup_action"`
	Classification string `json:"classification"`
	Success        bool   `json:"success"`
	Message        string `json:"message"`
}

type CleanupExecutor interface {
	Pause(ctx context.Context, version string, token string, appSecret string, resourceID string) error
	Delete(ctx context.Context, version string, token string, appSecret string, resourceID string) error
}

type cleanupGraphClient interface {
	Do(ctx context.Context, req graph.Request) (*graph.Response, error)
}

type GraphCleanupExecutor struct {
	client cleanupGraphClient
}

func NewGraphCleanupExecutor(client cleanupGraphClient) *GraphCleanupExecutor {
	if client == nil {
		client = graph.NewClient(nil, "")
	}
	return &GraphCleanupExecutor{client: client}
}

func (e *GraphCleanupExecutor) Pause(ctx context.Context, version string, token string, appSecret string, resourceID string) error {
	if e == nil || e.client == nil {
		return errors.New("cleanup graph client is required")
	}
	if strings.TrimSpace(resourceID) == "" {
		return errors.New("resource id is required")
	}

	_, err := e.client.Do(ctx, graph.Request{
		Method:      "POST",
		Path:        strings.TrimSpace(resourceID),
		Version:     strings.TrimSpace(version),
		Form:        map[string]string{"status": "PAUSED"},
		AccessToken: token,
		AppSecret:   appSecret,
	})
	if err != nil {
		return err
	}
	return nil
}

func (e *GraphCleanupExecutor) Delete(ctx context.Context, version string, token string, appSecret string, resourceID string) error {
	if e == nil || e.client == nil {
		return errors.New("cleanup graph client is required")
	}
	if strings.TrimSpace(resourceID) == "" {
		return errors.New("resource id is required")
	}

	response, err := e.client.Do(ctx, graph.Request{
		Method:      "DELETE",
		Path:        strings.TrimSpace(resourceID),
		Version:     strings.TrimSpace(version),
		AccessToken: token,
		AppSecret:   appSecret,
	})
	if err != nil {
		return err
	}

	successValue, hasSuccess := response.Body["success"]
	if !hasSuccess {
		return nil
	}
	success, ok := successValue.(bool)
	if !ok || !success {
		return errors.New("delete response did not confirm success")
	}
	return nil
}

func CleanupResourceLedger(ctx context.Context, ledgerPath string, options CleanupOptions) (CleanupResult, error) {
	ledgerPath = strings.TrimSpace(ledgerPath)
	if ledgerPath == "" {
		return CleanupResult{}, ErrResourceLedgerPathRequired
	}

	ledger, err := LoadResourceLedger(ledgerPath)
	if err != nil {
		return CleanupResult{}, err
	}

	mode := CleanupModeDryRun
	if options.Apply {
		mode = CleanupModeApply
	}
	result := CleanupResult{
		LedgerPath: ledgerPath,
		Mode:       mode,
		Resources:  make([]CleanupResourceResult, 0, len(ledger.Resources)),
	}

	var executor CleanupExecutor
	version := ""
	token := ""
	appSecret := ""
	if options.Apply {
		version = strings.TrimSpace(options.Version)
		if version == "" {
			return CleanupResult{}, ErrCleanupApplyVersionRequired
		}
		token = strings.TrimSpace(options.Token)
		if token == "" {
			return CleanupResult{}, ErrCleanupApplyTokenRequired
		}
		appSecret = options.AppSecret
		executor = options.Executor
		if executor == nil {
			executor = NewGraphCleanupExecutor(nil)
		}
	}

	remaining := make([]TrackedResource, 0, len(ledger.Resources))
	for _, resource := range ledger.Resources {
		resourceResult := CleanupResourceResult{
			Sequence:      resource.Sequence,
			Command:       resource.Command,
			ResourceKind:  resource.ResourceKind,
			ResourceID:    resource.ResourceID,
			CleanupAction: resource.CleanupAction,
		}

		if !options.Apply {
			resourceResult.Classification = CleanupClassificationDryRun
			resourceResult.Success = true
			resourceResult.Message = fmt.Sprintf(
				"would %s %s %s",
				resource.CleanupAction,
				resource.ResourceKind,
				resource.ResourceID,
			)
			result.Resources = append(result.Resources, resourceResult)
			remaining = append(remaining, resource)
			continue
		}

		err := applyCleanupAction(ctx, executor, version, token, appSecret, resource)
		if err != nil {
			resourceResult.Classification = CleanupClassificationFailed
			resourceResult.Success = false
			resourceResult.Message = err.Error()
			result.Resources = append(result.Resources, resourceResult)
			remaining = append(remaining, resource)
			continue
		}

		resourceResult.Classification = CleanupClassificationApplied
		resourceResult.Success = true
		resourceResult.Message = fmt.Sprintf(
			"%s %s %s",
			resource.CleanupAction,
			resource.ResourceKind,
			resource.ResourceID,
		)
		result.Resources = append(result.Resources, resourceResult)
	}

	result.Summary = summarizeCleanupResults(result.Resources, len(remaining))
	if options.Apply {
		ledger.Resources = remaining
		if err := SaveResourceLedger(ledgerPath, ledger); err != nil {
			return CleanupResult{}, err
		}
	}

	return result, nil
}

func summarizeCleanupResults(resources []CleanupResourceResult, remaining int) CleanupSummary {
	summary := CleanupSummary{
		Total:     len(resources),
		Remaining: remaining,
	}
	for _, resource := range resources {
		switch resource.Classification {
		case CleanupClassificationDryRun:
			summary.DryRun++
		case CleanupClassificationApplied:
			summary.Applied++
		case CleanupClassificationFailed:
			summary.Failed++
		}
	}
	return summary
}

func applyCleanupAction(ctx context.Context, executor CleanupExecutor, version string, token string, appSecret string, resource TrackedResource) error {
	switch resource.CleanupAction {
	case CleanupActionPause:
		return executor.Pause(ctx, version, token, appSecret, resource.ResourceID)
	case CleanupActionDelete:
		return executor.Delete(ctx, version, token, appSecret, resource.ResourceID)
	default:
		return fmt.Errorf(
			"unsupported cleanup action %q for resource %s",
			resource.CleanupAction,
			resource.ResourceID,
		)
	}
}
