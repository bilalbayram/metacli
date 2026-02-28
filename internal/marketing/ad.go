package marketing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/bilalbayram/metacli/internal/graph"
)

const (
	AdStatusActive = "ACTIVE"
	AdStatusPaused = "PAUSED"
)

var DefaultAdCloneFields = []string{
	"id",
	"name",
	"status",
	"adset_id",
	"creative",
}

var DefaultAdReadFields = []string{
	"id",
	"name",
	"status",
	"effective_status",
	"campaign_id",
	"adset_id",
	"creative",
}

var requiredAdCloneDependencyFields = []string{
	"adset_id",
	"creative",
}

const (
	adValidationErrorType                 = "ad_validation_error"
	adValidationCodeCloneIncomplete       = 422100
	adValidationCodeCloneDependency       = 422101
	adValidationCodeCloneSourceAmbiguous  = 422102
	adValidationCodeClonePayloadNormalize = 422103
)

var immutableAdCloneFields = map[string]struct{}{
	"id":                {},
	"account_id":        {},
	"effective_status":  {},
	"configured_status": {},
	"created_time":      {},
	"updated_time":      {},
	"campaign_id":       {},
}

type AdMutationResult struct {
	Operation   string         `json:"operation"`
	AdID        string         `json:"ad_id"`
	RequestPath string         `json:"request_path"`
	Response    map[string]any `json:"response"`
}

type AdCreateInput struct {
	AccountID string
	Params    map[string]string
}

type AdUpdateInput struct {
	AdID   string
	Params map[string]string
}

type AdStatusInput struct {
	AdID   string
	Status string
}

type AdListInput struct {
	AccountID         string
	CampaignID        string
	AdSetID           string
	Fields            []string
	Name              string
	Statuses          []string
	EffectiveStatuses []string
	ActiveOnly        bool
	Limit             int
	PageSize          int
	FollowNext        bool
}

type AdListResult struct {
	Operation   string                  `json:"operation"`
	RequestPath string                  `json:"request_path"`
	Ads         []map[string]any        `json:"ads"`
	Paging      *graph.PaginationResult `json:"paging,omitempty"`
}

type AdCloneInput struct {
	SourceAdID      string
	TargetAccountID string
	Overrides       map[string]string
	Fields          []string
}

type AdCloneResult struct {
	Operation     string            `json:"operation"`
	SourceAdID    string            `json:"source_ad_id"`
	AdID          string            `json:"ad_id"`
	RequestPath   string            `json:"request_path"`
	Payload       map[string]string `json:"payload"`
	RemovedFields []string          `json:"removed_fields"`
	Response      map[string]any    `json:"response"`
}

type AdService struct {
	Client *graph.Client
}

func NewAdService(client *graph.Client) *AdService {
	if client == nil {
		client = graph.NewClient(nil, "")
	}
	return &AdService{Client: client}
}

func (s *AdService) List(ctx context.Context, version string, token string, appSecret string, input AdListInput) (*AdListResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("ad service client is required")
	}
	if input.Limit < 0 {
		return nil, errors.New("ad list limit must be >= 0")
	}
	if input.PageSize < 0 {
		return nil, errors.New("ad list page size must be >= 0")
	}

	accountID, err := normalizeAdAccountID(input.AccountID)
	if err != nil {
		return nil, err
	}
	fields, err := normalizeAdReadFields(input.Fields)
	if err != nil {
		return nil, err
	}
	filters, err := normalizeEntityReadFilters(input.Name, input.Statuses, input.EffectiveStatuses, input.ActiveOnly)
	if err != nil {
		return nil, err
	}

	campaignID := ""
	if strings.TrimSpace(input.CampaignID) != "" {
		campaignID, err = normalizeGraphID("campaign id", input.CampaignID)
		if err != nil {
			return nil, err
		}
	}

	adSetID := ""
	if strings.TrimSpace(input.AdSetID) != "" {
		adSetID, err = normalizeGraphID("ad set id", input.AdSetID)
		if err != nil {
			return nil, err
		}
	}

	path := fmt.Sprintf("act_%s/ads", accountID)
	switch {
	case adSetID != "":
		path = fmt.Sprintf("%s/ads", adSetID)
	case campaignID != "":
		path = fmt.Sprintf("%s/ads", campaignID)
	}

	fetchFields := mergeEntityReadFields(fields, "name", "status", "effective_status", "campaign_id", "adset_id")
	query := map[string]string{
		"fields": strings.Join(fetchFields, ","),
	}
	if input.PageSize > 0 {
		query["limit"] = strconv.Itoa(input.PageSize)
	}

	rows := make([]map[string]any, 0)
	pagination, err := s.Client.FetchWithPagination(ctx, graph.Request{
		Method:      "GET",
		Path:        path,
		Version:     strings.TrimSpace(version),
		Query:       query,
		AccessToken: token,
		AppSecret:   appSecret,
	}, graph.PaginationOptions{
		FollowNext: input.FollowNext,
		PageSize:   input.PageSize,
	}, func(item map[string]any) error {
		if !matchEntityReadFilters(item, filters) {
			return nil
		}
		if !matchEntityIDFilter(item, "campaign_id", campaignID) {
			return nil
		}
		if !matchEntityIDFilter(item, "adset_id", adSetID) {
			return nil
		}
		rows = append(rows, projectEntityReadFields(item, fields))
		if input.Limit > 0 && len(rows) > input.Limit {
			rows = rows[:input.Limit]
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if input.Limit > 0 && len(rows) > input.Limit {
		rows = rows[:input.Limit]
	}

	return &AdListResult{
		Operation:   "list",
		RequestPath: path,
		Ads:         rows,
		Paging:      pagination,
	}, nil
}

func (s *AdService) Create(ctx context.Context, version string, token string, appSecret string, input AdCreateInput) (*AdMutationResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("ad service client is required")
	}

	accountID, err := normalizeAdAccountID(input.AccountID)
	if err != nil {
		return nil, err
	}
	form, err := normalizeAdMutationParams(input.Params)
	if err != nil {
		return nil, err
	}
	if err := s.validateDependencies(ctx, version, token, appSecret, form); err != nil {
		return nil, err
	}

	path := fmt.Sprintf("act_%s/ads", accountID)
	response, err := s.Client.Do(ctx, graph.Request{
		Method:      "POST",
		Path:        path,
		Version:     strings.TrimSpace(version),
		Form:        form,
		AccessToken: token,
		AppSecret:   appSecret,
	})
	if err != nil {
		return nil, err
	}

	adID, _ := response.Body["id"].(string)
	if strings.TrimSpace(adID) == "" {
		return nil, errors.New("ad create response did not include id")
	}

	return &AdMutationResult{
		Operation:   "create",
		AdID:        adID,
		RequestPath: path,
		Response:    response.Body,
	}, nil
}

func (s *AdService) Update(ctx context.Context, version string, token string, appSecret string, input AdUpdateInput) (*AdMutationResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("ad service client is required")
	}

	adID, err := normalizeGraphID("ad id", input.AdID)
	if err != nil {
		return nil, err
	}
	form, err := normalizeAdMutationParams(input.Params)
	if err != nil {
		return nil, err
	}
	if err := s.validateDependencies(ctx, version, token, appSecret, form); err != nil {
		return nil, err
	}

	response, err := s.Client.Do(ctx, graph.Request{
		Method:      "POST",
		Path:        adID,
		Version:     strings.TrimSpace(version),
		Form:        form,
		AccessToken: token,
		AppSecret:   appSecret,
	})
	if err != nil {
		return nil, err
	}

	if responseID, ok := response.Body["id"].(string); ok && strings.TrimSpace(responseID) != "" {
		adID = responseID
	}

	return &AdMutationResult{
		Operation:   "update",
		AdID:        adID,
		RequestPath: adID,
		Response:    response.Body,
	}, nil
}

func (s *AdService) SetStatus(ctx context.Context, version string, token string, appSecret string, input AdStatusInput) (*AdMutationResult, error) {
	normalizedStatus, err := normalizeAdStatus(input.Status)
	if err != nil {
		return nil, err
	}

	result, err := s.Update(ctx, version, token, appSecret, AdUpdateInput{
		AdID: input.AdID,
		Params: map[string]string{
			"status": normalizedStatus,
		},
	})
	if err != nil {
		return nil, err
	}

	result.Operation = strings.ToLower(normalizedStatus)
	return result, nil
}

func (s *AdService) Clone(ctx context.Context, version string, token string, appSecret string, input AdCloneInput) (*AdCloneResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("ad service client is required")
	}

	sourceAdID, err := normalizeGraphID("source ad id", input.SourceAdID)
	if err != nil {
		return nil, err
	}
	accountID, err := normalizeAdAccountID(input.TargetAccountID)
	if err != nil {
		return nil, err
	}
	fields, err := normalizeAdCloneFields(input.Fields)
	if err != nil {
		return nil, err
	}
	overrides, err := normalizeOptionalParams(input.Overrides)
	if err != nil {
		return nil, err
	}

	sourceResponse, err := s.Client.Do(ctx, graph.Request{
		Method:  "GET",
		Path:    sourceAdID,
		Version: strings.TrimSpace(version),
		Query: map[string]string{
			"fields": strings.Join(fields, ","),
		},
		AccessToken: token,
		AppSecret:   appSecret,
	})
	if err != nil {
		return nil, err
	}

	clonePayload, removedFields, err := sanitizeAdClonePayload(sourceResponse.Body)
	if err != nil {
		return nil, err
	}
	for key, value := range overrides {
		clonePayload[key] = value
	}
	if len(clonePayload) == 0 {
		return nil, newAdValidationError(
			adValidationCodeCloneIncomplete,
			"ad clone payload is empty after sanitization",
			[]string{"fields", "params", "json"},
			"Ad clone payload has no mutable fields to create a new ad.",
			"Include mutable source fields via --fields (for example: name,status,adset_id,creative).",
			"Provide override params via --params/--json for required create fields.",
		)
	}
	normalizedClonePayload, err := normalizeAdClonePayload(clonePayload)
	if err != nil {
		return nil, err
	}
	if err := s.validateDependencies(ctx, version, token, appSecret, normalizedClonePayload); err != nil {
		return nil, err
	}

	createPath := fmt.Sprintf("act_%s/ads", accountID)
	createResponse, err := s.Client.Do(ctx, graph.Request{
		Method:      "POST",
		Path:        createPath,
		Version:     strings.TrimSpace(version),
		Form:        normalizedClonePayload,
		AccessToken: token,
		AppSecret:   appSecret,
	})
	if err != nil {
		return nil, err
	}

	adID, _ := createResponse.Body["id"].(string)
	if strings.TrimSpace(adID) == "" {
		return nil, errors.New("ad clone create response did not include id")
	}

	return &AdCloneResult{
		Operation:     "clone",
		SourceAdID:    sourceAdID,
		AdID:          adID,
		RequestPath:   createPath,
		Payload:       normalizedClonePayload,
		RemovedFields: removedFields,
		Response:      createResponse.Body,
	}, nil
}

func (s *AdService) validateDependencies(ctx context.Context, version string, token string, appSecret string, params map[string]string) error {
	if err := s.validateAdSetDependency(ctx, version, token, appSecret, params); err != nil {
		return err
	}
	return s.validateCreativeDependency(ctx, version, token, appSecret, params)
}

func (s *AdService) validateAdSetDependency(ctx context.Context, version string, token string, appSecret string, params map[string]string) error {
	adSetRaw, exists := params["adset_id"]
	if !exists {
		return nil
	}
	adSetID, err := normalizeGraphID("ad set id", adSetRaw)
	if err != nil {
		return err
	}
	return s.validateDependencyExists(ctx, version, token, appSecret, adSetID, "ad set")
}

func (s *AdService) validateCreativeDependency(ctx context.Context, version string, token string, appSecret string, params map[string]string) error {
	creativeRaw, exists := params["creative"]
	if !exists {
		return nil
	}
	creativeID, err := extractCreativeReferenceID(creativeRaw)
	if err != nil {
		return err
	}
	return s.validateDependencyExists(ctx, version, token, appSecret, creativeID, "creative")
}

func (s *AdService) validateDependencyExists(ctx context.Context, version string, token string, appSecret string, dependencyID string, label string) error {
	response, err := s.Client.Do(ctx, graph.Request{
		Method:  "GET",
		Path:    dependencyID,
		Version: strings.TrimSpace(version),
		Query: map[string]string{
			"fields": "id",
		},
		AccessToken: token,
		AppSecret:   appSecret,
	})
	if err != nil {
		return fmt.Errorf("validate %s reference %q: %w", label, dependencyID, err)
	}
	responseID, _ := response.Body["id"].(string)
	if strings.TrimSpace(responseID) == "" {
		return fmt.Errorf("%s reference %q validation response did not include id", label, dependencyID)
	}
	return nil
}

func extractCreativeReferenceID(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("creative reference is required")
	}
	if !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
		return normalizeGraphID("creative id", trimmed)
	}

	decoded := map[string]any{}
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return "", fmt.Errorf("creative reference must be a graph id or JSON object: %w", err)
	}

	for _, key := range []string{"creative_id", "id"} {
		value, exists := decoded[key]
		if !exists {
			continue
		}
		typed, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("creative reference field %q must be a string", key)
		}
		return normalizeGraphID("creative id", typed)
	}
	return "", errors.New("creative reference must include creative_id or id")
}

func normalizeAdStatus(value string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch normalized {
	case AdStatusActive, AdStatusPaused:
		return normalized, nil
	case "":
		return "", errors.New("ad status is required")
	default:
		return "", fmt.Errorf("unsupported ad status %q: expected ACTIVE|PAUSED", value)
	}
}

func normalizeAdMutationParams(params map[string]string) (map[string]string, error) {
	normalized, err := normalizeOptionalParams(params)
	if err != nil {
		return nil, err
	}
	if len(normalized) == 0 {
		return nil, errors.New("ad mutation payload cannot be empty")
	}
	return normalized, nil
}

func normalizeAdReadFields(fields []string) ([]string, error) {
	return normalizeEntityReadFields("ad", fields, DefaultAdReadFields)
}

func normalizeAdCloneFields(fields []string) ([]string, error) {
	if len(fields) == 0 {
		return append([]string(nil), DefaultAdCloneFields...), nil
	}

	out := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		if trimmed == "" {
			return nil, errors.New("clone field cannot be empty")
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil, errors.New("clone fields are required")
	}
	for _, dependencyField := range requiredAdCloneDependencyFields {
		if _, exists := seen[dependencyField]; exists {
			continue
		}
		seen[dependencyField] = struct{}{}
		out = append(out, dependencyField)
	}
	return out, nil
}

func sanitizeAdClonePayload(source map[string]any) (map[string]string, []string, error) {
	if len(source) == 0 {
		return nil, nil, errors.New("ad clone source payload is empty")
	}

	payload := map[string]string{}
	removed := make([]string, 0)
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	seenNormalizedKeys := map[string]struct{}{}
	for _, key := range keys {
		value := source[key]
		normalizedKey := strings.TrimSpace(key)
		if normalizedKey == "" {
			continue
		}
		if _, exists := seenNormalizedKeys[normalizedKey]; exists {
			return nil, nil, newAdValidationError(
				adValidationCodeCloneSourceAmbiguous,
				fmt.Sprintf("ad clone source payload contains duplicate field %q after normalization", normalizedKey),
				[]string{normalizedKey},
				"Source ad payload cannot be normalized deterministically.",
				"Adjust requested --fields so each normalized field appears once.",
			)
		}
		seenNormalizedKeys[normalizedKey] = struct{}{}
		if _, immutable := immutableAdCloneFields[normalizedKey]; immutable {
			removed = append(removed, normalizedKey)
			continue
		}
		if value == nil {
			continue
		}

		encoded, err := encodeGraphValue(value)
		if err != nil {
			return nil, nil, fmt.Errorf("encode clone value for field %q: %w", normalizedKey, err)
		}
		payload[normalizedKey] = encoded
	}

	sort.Strings(removed)
	return payload, removed, nil
}

func normalizeAdClonePayload(payload map[string]string) (map[string]string, error) {
	normalized, err := normalizeAdMutationParams(payload)
	if err != nil {
		return nil, err
	}

	missingFields := make([]string, 0, len(requiredAdCloneDependencyFields))
	for _, field := range requiredAdCloneDependencyFields {
		value, exists := normalized[field]
		if !exists || strings.TrimSpace(value) == "" {
			missingFields = append(missingFields, field)
		}
	}
	if len(missingFields) > 0 {
		sort.Strings(missingFields)
		return nil, newAdValidationError(
			adValidationCodeCloneIncomplete,
			fmt.Sprintf("ad clone payload is incomplete: missing dependency fields %s", strings.Join(missingFields, ", ")),
			missingFields,
			"Ad clone requires both adset and creative references.",
			"Include the missing fields in --fields or provide overrides via --params/--json.",
			"Verify the source ad has stable adset and creative links before cloning.",
		)
	}

	adSetID, err := normalizeGraphID("ad set id", normalized["adset_id"])
	if err != nil {
		return nil, newAdValidationError(
			adValidationCodeCloneDependency,
			err.Error(),
			[]string{"adset_id"},
			"Ad clone dependency preflight failed for adset_id.",
			"Set adset_id to a valid single graph id token.",
		)
	}

	creativeID, err := extractCreativeReferenceID(normalized["creative"])
	if err != nil {
		return nil, newAdValidationError(
			adValidationCodeCloneDependency,
			err.Error(),
			[]string{"creative"},
			"Ad clone dependency preflight failed for creative.",
			"Set creative to a graph id or JSON object containing creative_id/id.",
		)
	}
	creativeReference, err := encodeGraphValue(map[string]string{"creative_id": creativeID})
	if err != nil {
		return nil, newAdValidationError(
			adValidationCodeClonePayloadNormalize,
			fmt.Sprintf("encode normalized creative reference: %v", err),
			[]string{"creative"},
			"Ad clone failed while normalizing creative payload.",
			"Retry with a plain creative_id value in --json to avoid nested serialization issues.",
		)
	}

	normalized["adset_id"] = adSetID
	normalized["creative"] = creativeReference

	if statusRaw, exists := normalized["status"]; exists {
		normalizedStatus, err := normalizeAdStatus(statusRaw)
		if err != nil {
			return nil, newAdValidationError(
				adValidationCodeClonePayloadNormalize,
				err.Error(),
				[]string{"status"},
				"Ad clone status normalization failed.",
				"Use ACTIVE or PAUSED for status overrides.",
			)
		}
		normalized["status"] = normalizedStatus
	}

	return normalized, nil
}

func newAdValidationError(code int, message string, fields []string, summary string, actions ...string) *graph.APIError {
	return &graph.APIError{
		Type:       adValidationErrorType,
		Code:       code,
		Message:    strings.TrimSpace(message),
		StatusCode: http.StatusUnprocessableEntity,
		Retryable:  false,
		Remediation: &graph.Remediation{
			Category: graph.RemediationCategoryValidation,
			Summary:  strings.TrimSpace(summary),
			Actions:  normalizeRemediationActions(actions),
			Fields:   normalizeRemediationFields(fields),
		},
	}
}

func normalizeRemediationActions(actions []string) []string {
	if len(actions) == 0 {
		return nil
	}
	out := make([]string, 0, len(actions))
	for _, action := range actions {
		trimmed := strings.TrimSpace(action)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeRemediationFields(fields []string) []string {
	if len(fields) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}
