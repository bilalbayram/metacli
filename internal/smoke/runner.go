package smoke

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/bilalbayram/metacli/internal/graph"
	"github.com/bilalbayram/metacli/internal/ops"
)

const (
	ReportSchemaVersion = 2
	ReportKind          = "smoke_report"
)

const (
	StepStatusExecuted = "executed"
	StepStatusSkipped  = "skipped"
	StepStatusFailed   = "failed"
)

const (
	CapabilityStatusAvailable    = "available"
	CapabilityStatusUnavailable  = "unavailable"
	CapabilityStatusNotEvaluated = "not_evaluated"
)

const (
	RunOutcomeClean    = "clean"
	RunOutcomeWarning  = "warning"
	RunOutcomeBlocking = "blocking"
)

const (
	stepNameAccountContext = "account_context"
	stepNameCampaignCreate = "campaign_create"
	stepNameAudienceCreate = "audience_create"
	stepNameCatalogUpload  = "catalog_upload"

	capabilityAudience = "audience"
	capabilityCatalog  = "catalog"
)

var (
	ErrClientRequired   = errors.New("smoke graph client is required")
	ErrTokenRequired    = errors.New("smoke token is required")
	ErrVersionRequired  = errors.New("smoke graph version is required")
	ErrAccountIDMissing = errors.New("smoke account id is required")
)

type GraphClient interface {
	Do(ctx context.Context, req graph.Request) (*graph.Response, error)
}

type Runner struct {
	Client GraphClient
}

type RunInput struct {
	ProfileName    string
	Version        string
	AccountID      string
	Token          string
	AppSecret      string
	OptionalPolicy string
	CatalogID      string
}

type RunResult struct {
	Report Report `json:"report"`
}

type Report struct {
	SchemaVersion    int                `json:"schema_version"`
	Kind             string             `json:"kind"`
	ProfileName      string             `json:"profile_name"`
	GraphVersion     string             `json:"graph_version"`
	OptionalPolicy   string             `json:"optional_policy"`
	Account          AccountContext     `json:"account"`
	Summary          Summary            `json:"summary"`
	Outcome          string             `json:"outcome"`
	Capabilities     []CapabilityStatus `json:"capabilities"`
	Steps            []Step             `json:"steps"`
	CreatedResources []CreatedResource  `json:"created_resources"`
	Failures         []Failure          `json:"failures"`
	RateLimit        RateLimitReport    `json:"rate_limit"`
}

type AccountContext struct {
	InputAccountID string `json:"input_account_id"`
	AccountID      string `json:"account_id"`
	Name           string `json:"name,omitempty"`
	Currency       string `json:"currency,omitempty"`
	AccountStatus  int    `json:"account_status,omitempty"`
}

type Summary struct {
	TotalSteps        int `json:"total_steps"`
	ExecutedSteps     int `json:"executed_steps"`
	SkippedSteps      int `json:"skipped_steps"`
	FailedSteps       int `json:"failed_steps"`
	Warnings          int `json:"warnings"`
	Blocking          int `json:"blocking"`
	CreatedResources  int `json:"created_resources"`
	CapabilitySkipped int `json:"capability_skipped"`
}

type CapabilityStatus struct {
	Name     string `json:"name"`
	Optional bool   `json:"optional"`
	Status   string `json:"status"`
	Policy   string `json:"policy"`
	Reason   string `json:"reason,omitempty"`
}

type Step struct {
	Name       string             `json:"name"`
	Optional   bool               `json:"optional"`
	Capability string             `json:"capability,omitempty"`
	Status     string             `json:"status"`
	Blocking   bool               `json:"blocking"`
	Warning    bool               `json:"warning"`
	Message    string             `json:"message,omitempty"`
	RateLimit  *RateLimitMetadata `json:"rate_limit,omitempty"`
}

type CreatedResource struct {
	Sequence      int    `json:"sequence"`
	Command       string `json:"command"`
	ResourceKind  string `json:"resource_kind"`
	ResourceID    string `json:"resource_id"`
	CleanupAction string `json:"cleanup_action"`
	AccountID     string `json:"account_id"`
	Step          string `json:"step"`
}

type Failure struct {
	Step         string `json:"step"`
	Optional     bool   `json:"optional"`
	Blocking     bool   `json:"blocking"`
	Type         string `json:"type"`
	Message      string `json:"message"`
	StatusCode   int    `json:"status_code,omitempty"`
	Code         int    `json:"code,omitempty"`
	ErrorSubcode int    `json:"error_subcode,omitempty"`
}

type RateLimitReport struct {
	Observed            bool               `json:"observed"`
	Samples             int                `json:"samples"`
	MaxAppCallCount     int                `json:"max_app_call_count"`
	MaxAppTotalCPUTime  int                `json:"max_app_total_cputime"`
	MaxAppTotalTime     int                `json:"max_app_total_time"`
	MaxPageCallCount    int                `json:"max_page_call_count"`
	MaxPageTotalCPUTime int                `json:"max_page_total_cputime"`
	MaxPageTotalTime    int                `json:"max_page_total_time"`
	MaxAdAccountUtilPct int                `json:"max_ad_account_util_pct"`
	Last                *RateLimitMetadata `json:"last,omitempty"`
}

type RateLimitMetadata struct {
	AppUsage         map[string]any `json:"app_usage,omitempty"`
	PageUsage        map[string]any `json:"page_usage,omitempty"`
	AdAccountUsage   map[string]any `json:"ad_account_usage,omitempty"`
	AppCallCount     int            `json:"app_call_count,omitempty"`
	AppTotalCPUTime  int            `json:"app_total_cputime,omitempty"`
	AppTotalTime     int            `json:"app_total_time,omitempty"`
	PageCallCount    int            `json:"page_call_count,omitempty"`
	PageTotalCPUTime int            `json:"page_total_cputime,omitempty"`
	PageTotalTime    int            `json:"page_total_time,omitempty"`
	AdAccountUtilPct int            `json:"ad_account_util_pct,omitempty"`
}

func NewRunner(client GraphClient) *Runner {
	return &Runner{Client: client}
}

func (r *Runner) Run(ctx context.Context, input RunInput) (RunResult, error) {
	if r == nil || r.Client == nil {
		return RunResult{}, ErrClientRequired
	}

	normalizedPolicy := NormalizeOptionalPolicy(input.OptionalPolicy)
	if normalizedPolicy == "" {
		return RunResult{}, fmt.Errorf("%w: %q", ErrInvalidOptionalPolicy, input.OptionalPolicy)
	}

	version := strings.TrimSpace(input.Version)
	if version == "" {
		return RunResult{}, ErrVersionRequired
	}
	token := strings.TrimSpace(input.Token)
	if token == "" {
		return RunResult{}, ErrTokenRequired
	}
	accountID, err := normalizeAdAccountID(input.AccountID)
	if err != nil {
		return RunResult{}, err
	}

	report := Report{
		SchemaVersion:  ReportSchemaVersion,
		Kind:           ReportKind,
		ProfileName:    strings.TrimSpace(input.ProfileName),
		GraphVersion:   version,
		OptionalPolicy: normalizedPolicy,
		Account: AccountContext{
			InputAccountID: strings.TrimSpace(input.AccountID),
			AccountID:      accountID,
		},
		Capabilities: []CapabilityStatus{
			{
				Name:     capabilityAudience,
				Optional: true,
				Status:   CapabilityStatusNotEvaluated,
				Policy:   normalizedPolicy,
			},
			{
				Name:     capabilityCatalog,
				Optional: true,
				Status:   CapabilityStatusNotEvaluated,
				Policy:   normalizedPolicy,
			},
		},
		Steps:            make([]Step, 0, 4),
		CreatedResources: []CreatedResource{},
		Failures:         []Failure{},
	}

	capabilityIndex := map[string]int{
		capabilityAudience: 0,
		capabilityCatalog:  1,
	}

	setCapability := func(name string, status string, reason string) {
		index, ok := capabilityIndex[name]
		if !ok {
			return
		}
		report.Capabilities[index].Status = status
		report.Capabilities[index].Reason = strings.TrimSpace(reason)
	}

	appendStep := func(step Step) {
		if step.RateLimit != nil {
			mergeRateLimitReport(&report.RateLimit, step.RateLimit)
		}
		report.Steps = append(report.Steps, step)
	}

	appendFailureFromError := func(stepName string, optional bool, blocking bool, err error) {
		failure := failureFromError(stepName, optional, blocking, err)
		report.Failures = append(report.Failures, failure)
	}

	appendFailure := func(stepName string, optional bool, blocking bool, failureType string, message string) {
		report.Failures = append(report.Failures, Failure{
			Step:     stepName,
			Optional: optional,
			Blocking: blocking,
			Type:     strings.TrimSpace(failureType),
			Message:  strings.TrimSpace(message),
		})
	}

	appendCreatedResource := func(kind string, id string, cleanupAction string, stepName string) {
		sequence := len(report.CreatedResources) + 1
		report.CreatedResources = append(report.CreatedResources, CreatedResource{
			Sequence:      sequence,
			Command:       CommandRun,
			ResourceKind:  strings.TrimSpace(kind),
			ResourceID:    strings.TrimSpace(id),
			CleanupAction: strings.TrimSpace(cleanupAction),
			AccountID:     accountID,
			Step:          stepName,
		})
	}

	blocked := false
	blockReason := ""

	appendBlockedStep := func(stepName string, optional bool, capability string) {
		step := Step{
			Name:       stepName,
			Optional:   optional,
			Capability: capability,
			Status:     StepStatusSkipped,
			Blocking:   false,
			Warning:    false,
			Message:    fmt.Sprintf("step skipped because run is blocked: %s", strings.TrimSpace(blockReason)),
		}
		appendStep(step)
	}

	handleOptionalUnavailable := func(stepName string, capability string, reason string) {
		reason = strings.TrimSpace(reason)
		if reason == "" {
			reason = "optional capability is unavailable"
		}
		setCapability(capability, CapabilityStatusUnavailable, reason)

		if normalizedPolicy == OptionalPolicyStrict {
			message := fmt.Sprintf("optional capability unavailable under policy=%s: %s", normalizedPolicy, reason)
			appendStep(Step{
				Name:       stepName,
				Optional:   true,
				Capability: capability,
				Status:     StepStatusFailed,
				Blocking:   true,
				Warning:    false,
				Message:    message,
			})
			appendFailure(stepName, true, true, "optional_capability_unavailable", message)
			blocked = true
			blockReason = message
			return
		}

		appendStep(Step{
			Name:       stepName,
			Optional:   true,
			Capability: capability,
			Status:     StepStatusSkipped,
			Blocking:   false,
			Warning:    true,
			Message:    fmt.Sprintf("optional capability skipped under policy=%s: %s", normalizedPolicy, reason),
		})
	}

	{
		step := Step{
			Name:     stepNameAccountContext,
			Optional: false,
		}
		response, err := r.Client.Do(ctx, graph.Request{
			Method:  http.MethodGet,
			Path:    fmt.Sprintf("act_%s", accountID),
			Version: version,
			Query: map[string]string{
				"fields": "id,name,account_status,currency",
			},
			AccessToken: token,
			AppSecret:   input.AppSecret,
		})
		if err != nil {
			step.Status = StepStatusFailed
			step.Blocking = true
			step.Message = err.Error()
			appendStep(step)
			appendFailureFromError(stepNameAccountContext, false, true, err)
			blocked = true
			blockReason = step.Message
		} else {
			step.Status = StepStatusExecuted
			if metadata := rateLimitMetadataFromGraph(response.RateLimit); metadata != nil {
				step.RateLimit = metadata
			}

			if resolvedAccount, ok := response.Body["id"].(string); ok && strings.TrimSpace(resolvedAccount) != "" {
				normalizedResolved, normalizeErr := normalizeAdAccountID(resolvedAccount)
				if normalizeErr == nil {
					report.Account.AccountID = normalizedResolved
				}
			}
			if name, ok := response.Body["name"].(string); ok {
				report.Account.Name = strings.TrimSpace(name)
			}
			if currency, ok := response.Body["currency"].(string); ok {
				report.Account.Currency = strings.ToUpper(strings.TrimSpace(currency))
			}
			report.Account.AccountStatus = intFromAny(response.Body["account_status"])
			step.Message = fmt.Sprintf(
				"account context resolved: account_id=%s currency=%s account_status=%d",
				report.Account.AccountID,
				report.Account.Currency,
				report.Account.AccountStatus,
			)
			appendStep(step)
		}
	}

	if blocked {
		appendBlockedStep(stepNameCampaignCreate, false, "")
	} else {
		step := Step{
			Name:     stepNameCampaignCreate,
			Optional: false,
		}
		response, err := r.Client.Do(ctx, graph.Request{
			Method:  http.MethodPost,
			Path:    fmt.Sprintf("act_%s/campaigns", accountID),
			Version: version,
			Form: map[string]string{
				"name":                            "CLI_SmokeV2_Campaign",
				"objective":                       "OUTCOME_TRAFFIC",
				"status":                          "PAUSED",
				"special_ad_categories":           "[]",
				"is_adset_budget_sharing_enabled": "false",
			},
			AccessToken: token,
			AppSecret:   input.AppSecret,
		})
		if err != nil {
			step.Status = StepStatusFailed
			step.Blocking = true
			step.Message = err.Error()
			appendStep(step)
			appendFailureFromError(stepNameCampaignCreate, false, true, err)
			blocked = true
			blockReason = step.Message
		} else {
			campaignID, _ := response.Body["id"].(string)
			campaignID = strings.TrimSpace(campaignID)
			if campaignID == "" {
				step.Status = StepStatusFailed
				step.Blocking = true
				step.Message = "campaign create response did not include id"
				appendStep(step)
				appendFailure(stepNameCampaignCreate, false, true, "runtime_error", step.Message)
				blocked = true
				blockReason = step.Message
			} else {
				step.Status = StepStatusExecuted
				if metadata := rateLimitMetadataFromGraph(response.RateLimit); metadata != nil {
					step.RateLimit = metadata
				}
				step.Message = fmt.Sprintf("campaign created: campaign_id=%s", campaignID)
				appendStep(step)
				appendCreatedResource(ops.ResourceKindCampaign, campaignID, ops.CleanupActionPause, stepNameCampaignCreate)
			}
		}
	}

	if blocked {
		appendBlockedStep(stepNameAudienceCreate, true, capabilityAudience)
	} else {
		step := Step{
			Name:       stepNameAudienceCreate,
			Optional:   true,
			Capability: capabilityAudience,
		}
		response, err := r.Client.Do(ctx, graph.Request{
			Method:  http.MethodPost,
			Path:    fmt.Sprintf("act_%s/customaudiences", accountID),
			Version: version,
			Form: map[string]string{
				"name":                 "CLI_SmokeV2_Audience",
				"subtype":              "CUSTOM",
				"customer_file_source": "USER_PROVIDED_ONLY",
				"description":          "Smoke runner v2 audience check",
			},
			AccessToken: token,
			AppSecret:   input.AppSecret,
		})
		if err != nil {
			if reason, unavailable := classifyOptionalCapabilityUnavailable(err); unavailable {
				handleOptionalUnavailable(stepNameAudienceCreate, capabilityAudience, reason)
			} else {
				setCapability(capabilityAudience, CapabilityStatusAvailable, "")
				step.Status = StepStatusFailed
				step.Blocking = true
				step.Message = err.Error()
				appendStep(step)
				appendFailureFromError(stepNameAudienceCreate, true, true, err)
				blocked = true
				blockReason = step.Message
			}
		} else {
			audienceID, _ := response.Body["id"].(string)
			audienceID = strings.TrimSpace(audienceID)
			if audienceID == "" {
				step.Status = StepStatusFailed
				step.Blocking = true
				step.Message = "audience create response did not include id"
				appendStep(step)
				appendFailure(stepNameAudienceCreate, true, true, "runtime_error", step.Message)
				blocked = true
				blockReason = step.Message
			} else {
				setCapability(capabilityAudience, CapabilityStatusAvailable, "")
				step.Status = StepStatusExecuted
				if metadata := rateLimitMetadataFromGraph(response.RateLimit); metadata != nil {
					step.RateLimit = metadata
				}
				step.Message = fmt.Sprintf("audience created: audience_id=%s", audienceID)
				appendStep(step)
				appendCreatedResource(ops.ResourceKindAudience, audienceID, ops.CleanupActionDelete, stepNameAudienceCreate)
			}
		}
	}

	if blocked {
		appendBlockedStep(stepNameCatalogUpload, true, capabilityCatalog)
	} else {
		trimmedCatalogID := strings.TrimSpace(input.CatalogID)
		if trimmedCatalogID == "" {
			handleOptionalUnavailable(stepNameCatalogUpload, capabilityCatalog, "catalog_id is required for catalog optional module")
		} else {
			step := Step{
				Name:       stepNameCatalogUpload,
				Optional:   true,
				Capability: capabilityCatalog,
			}
			requestsPayload, err := json.Marshal([]map[string]any{
				{
					"method":      "CREATE",
					"retailer_id": "cli-smoke-v2-sku-001",
					"data": map[string]any{
						"name":         "CLI Smoke V2 Product",
						"description":  "Smoke runner v2 catalog probe",
						"availability": "in stock",
						"condition":    "new",
						"price":        "9.99 USD",
						"url":          "https://example.com/p/cli-smoke-v2-sku-001",
						"image_url":    "https://picsum.photos/1200",
					},
				},
			})
			if err != nil {
				step.Status = StepStatusFailed
				step.Blocking = true
				step.Message = fmt.Sprintf("encode catalog request payload: %v", err)
				appendStep(step)
				appendFailure(stepNameCatalogUpload, true, true, "runtime_error", step.Message)
				blocked = true
				blockReason = step.Message
			} else {
				response, requestErr := r.Client.Do(ctx, graph.Request{
					Method:  http.MethodPost,
					Path:    fmt.Sprintf("%s/items_batch", strings.TrimPrefix(trimmedCatalogID, "/")),
					Version: version,
					Form: map[string]string{
						"item_type": "PRODUCT_ITEM",
						"requests":  string(requestsPayload),
					},
					AccessToken: token,
					AppSecret:   input.AppSecret,
				})
				if requestErr != nil {
					if reason, unavailable := classifyOptionalCapabilityUnavailable(requestErr); unavailable {
						handleOptionalUnavailable(stepNameCatalogUpload, capabilityCatalog, reason)
					} else {
						setCapability(capabilityCatalog, CapabilityStatusAvailable, "")
						step.Status = StepStatusFailed
						step.Blocking = true
						step.Message = requestErr.Error()
						appendStep(step)
						appendFailureFromError(stepNameCatalogUpload, true, true, requestErr)
						blocked = true
						blockReason = step.Message
					}
				} else {
					itemErrors := countCatalogItemErrors(response.Body)
					if itemErrors > 0 {
						setCapability(capabilityCatalog, CapabilityStatusAvailable, "")
						step.Status = StepStatusFailed
						step.Blocking = true
						step.Message = fmt.Sprintf("catalog upload returned %d item error(s)", itemErrors)
						if metadata := rateLimitMetadataFromGraph(response.RateLimit); metadata != nil {
							step.RateLimit = metadata
						}
						appendStep(step)
						appendFailure(stepNameCatalogUpload, true, true, "catalog_item_errors", step.Message)
						blocked = true
						blockReason = step.Message
					} else {
						setCapability(capabilityCatalog, CapabilityStatusAvailable, "")
						step.Status = StepStatusExecuted
						if metadata := rateLimitMetadataFromGraph(response.RateLimit); metadata != nil {
							step.RateLimit = metadata
						}
						step.Message = "catalog upload executed with no item errors"
						appendStep(step)
					}
				}
			}
		}
	}

	finalizeReport(&report)
	return RunResult{Report: report}, nil
}

func RunOutcomeForReport(report Report) string {
	if strings.TrimSpace(report.Outcome) != "" {
		return report.Outcome
	}
	return summarizeOutcome(report.Summary)
}

func RunExitCode(report Report) int {
	switch RunOutcomeForReport(report) {
	case RunOutcomeBlocking:
		return ExitCodePolicy
	case RunOutcomeWarning:
		return ExitCodeWarning
	default:
		return ExitCodeSuccess
	}
}

func finalizeReport(report *Report) {
	if report == nil {
		return
	}

	summary := Summary{
		TotalSteps:       len(report.Steps),
		CreatedResources: len(report.CreatedResources),
	}
	for _, step := range report.Steps {
		switch step.Status {
		case StepStatusExecuted:
			summary.ExecutedSteps++
		case StepStatusSkipped:
			summary.SkippedSteps++
			if step.Warning {
				summary.Warnings++
				summary.CapabilitySkipped++
			}
		case StepStatusFailed:
			summary.FailedSteps++
		}
		if step.Blocking {
			summary.Blocking++
		}
	}
	report.Summary = summary
	report.Outcome = summarizeOutcome(summary)
}

func summarizeOutcome(summary Summary) string {
	if summary.Blocking > 0 {
		return RunOutcomeBlocking
	}
	if summary.Warnings > 0 {
		return RunOutcomeWarning
	}
	return RunOutcomeClean
}

func normalizeAdAccountID(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", ErrAccountIDMissing
	}
	trimmed = strings.TrimPrefix(trimmed, "act_")
	if trimmed == "" {
		return "", ErrAccountIDMissing
	}
	for _, r := range trimmed {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("invalid account id %q: expected numeric graph id", value)
		}
	}
	return trimmed, nil
}

func failureFromError(stepName string, optional bool, blocking bool, err error) Failure {
	failure := Failure{
		Step:     stepName,
		Optional: optional,
		Blocking: blocking,
		Type:     "error",
		Message:  strings.TrimSpace(err.Error()),
	}

	var apiErr *graph.APIError
	if errors.As(err, &apiErr) {
		failure.Type = strings.TrimSpace(apiErr.Type)
		if failure.Type == "" {
			failure.Type = "api_error"
		}
		failure.Message = strings.TrimSpace(apiErr.Message)
		if failure.Message == "" {
			failure.Message = strings.TrimSpace(err.Error())
		}
		failure.StatusCode = apiErr.StatusCode
		failure.Code = apiErr.Code
		failure.ErrorSubcode = apiErr.ErrorSubcode
	}
	return failure
}

func classifyOptionalCapabilityUnavailable(err error) (string, bool) {
	var apiErr *graph.APIError
	if !errors.As(err, &apiErr) {
		return "", false
	}

	if apiErr.StatusCode == http.StatusForbidden {
		return describeAPIError(apiErr), true
	}

	switch apiErr.Code {
	case 10, 200, 294:
		return describeAPIError(apiErr), true
	}

	lowerMessage := strings.ToLower(strings.TrimSpace(apiErr.Message))
	switch {
	case strings.Contains(lowerMessage, "permission"),
		strings.Contains(lowerMessage, "not authorized"),
		strings.Contains(lowerMessage, "insufficient"),
		strings.Contains(lowerMessage, "unsupported request"):
		return describeAPIError(apiErr), true
	default:
		return "", false
	}
}

func describeAPIError(apiErr *graph.APIError) string {
	if apiErr == nil {
		return ""
	}
	message := strings.TrimSpace(apiErr.Message)
	if message == "" {
		message = "api error"
	}
	parts := make([]string, 0, 3)
	if apiErr.StatusCode > 0 {
		parts = append(parts, fmt.Sprintf("status=%d", apiErr.StatusCode))
	}
	if apiErr.Code > 0 {
		parts = append(parts, fmt.Sprintf("code=%d", apiErr.Code))
	}
	if apiErr.ErrorSubcode > 0 {
		parts = append(parts, fmt.Sprintf("subcode=%d", apiErr.ErrorSubcode))
	}
	if len(parts) == 0 {
		return message
	}
	return fmt.Sprintf("%s (%s)", message, strings.Join(parts, " "))
}

func countCatalogItemErrors(body map[string]any) int {
	rawHandles, ok := body["handles"]
	if !ok {
		return 0
	}
	handles, ok := rawHandles.([]any)
	if !ok {
		return 0
	}
	count := 0
	for _, rawHandle := range handles {
		handle, ok := rawHandle.(map[string]any)
		if !ok {
			continue
		}
		rawErrors, ok := handle["errors"]
		if !ok {
			continue
		}
		errorsList, ok := rawErrors.([]any)
		if !ok {
			continue
		}
		count += len(errorsList)
	}
	return count
}

func mergeRateLimitReport(report *RateLimitReport, metadata *RateLimitMetadata) {
	if report == nil || metadata == nil {
		return
	}
	report.Observed = true
	report.Samples++
	report.Last = copyRateLimitMetadata(metadata)
	report.MaxAppCallCount = maxInt(report.MaxAppCallCount, metadata.AppCallCount)
	report.MaxAppTotalCPUTime = maxInt(report.MaxAppTotalCPUTime, metadata.AppTotalCPUTime)
	report.MaxAppTotalTime = maxInt(report.MaxAppTotalTime, metadata.AppTotalTime)
	report.MaxPageCallCount = maxInt(report.MaxPageCallCount, metadata.PageCallCount)
	report.MaxPageTotalCPUTime = maxInt(report.MaxPageTotalCPUTime, metadata.PageTotalCPUTime)
	report.MaxPageTotalTime = maxInt(report.MaxPageTotalTime, metadata.PageTotalTime)
	report.MaxAdAccountUtilPct = maxInt(report.MaxAdAccountUtilPct, metadata.AdAccountUtilPct)
}

func rateLimitMetadataFromGraph(rate graph.RateLimit) *RateLimitMetadata {
	appUsage := cloneAnyMap(rate.AppUsage)
	pageUsage := cloneAnyMap(rate.PageUsage)
	adAccountUsage := cloneAnyMap(rate.AdAccountUsage)
	if len(appUsage) == 0 && len(pageUsage) == 0 && len(adAccountUsage) == 0 {
		return nil
	}
	return &RateLimitMetadata{
		AppUsage:         appUsage,
		PageUsage:        pageUsage,
		AdAccountUsage:   adAccountUsage,
		AppCallCount:     intFromAny(appUsage["call_count"]),
		AppTotalCPUTime:  intFromAny(appUsage["total_cputime"]),
		AppTotalTime:     intFromAny(appUsage["total_time"]),
		PageCallCount:    intFromAny(pageUsage["call_count"]),
		PageTotalCPUTime: intFromAny(pageUsage["total_cputime"]),
		PageTotalTime:    intFromAny(pageUsage["total_time"]),
		AdAccountUtilPct: maxAdAccountUtilPct(adAccountUsage),
	}
}

func maxAdAccountUtilPct(value any) int {
	switch typed := value.(type) {
	case map[string]any:
		maxValue := 0
		for key, item := range typed {
			normalizedKey := strings.ToLower(strings.TrimSpace(key))
			candidate := maxAdAccountUtilPct(item)
			if strings.Contains(normalizedKey, "util") && strings.Contains(normalizedKey, "pct") {
				candidate = maxInt(candidate, intFromAny(item))
			}
			maxValue = maxInt(maxValue, candidate)
		}
		return maxValue
	case []any:
		maxValue := 0
		for _, item := range typed {
			maxValue = maxInt(maxValue, maxAdAccountUtilPct(item))
		}
		return maxValue
	default:
		return intFromAny(typed)
	}
}

func copyRateLimitMetadata(metadata *RateLimitMetadata) *RateLimitMetadata {
	if metadata == nil {
		return nil
	}
	copied := *metadata
	copied.AppUsage = cloneAnyMap(metadata.AppUsage)
	copied.PageUsage = cloneAnyMap(metadata.PageUsage)
	copied.AdAccountUsage = cloneAnyMap(metadata.AdAccountUsage)
	return &copied
}

func cloneAnyMap(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = cloneAny(value)
	}
	return cloned
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnyMap(typed)
	case []any:
		cloned := make([]any, 0, len(typed))
		for _, item := range typed {
			cloned = append(cloned, cloneAny(item))
		}
		return cloned
	default:
		return typed
	}
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0
		}
		return int(parsed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func maxInt(left int, right int) int {
	if right > left {
		return right
	}
	return left
}
