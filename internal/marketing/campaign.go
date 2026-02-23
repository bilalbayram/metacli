package marketing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/bilalbayram/metacli/internal/graph"
)

const (
	CampaignStatusActive = "ACTIVE"
	CampaignStatusPaused = "PAUSED"
)

var DefaultCampaignCloneFields = []string{
	"id",
	"name",
	"objective",
	"status",
	"daily_budget",
	"lifetime_budget",
}

var immutableCampaignCloneFields = map[string]struct{}{
	"id":                {},
	"account_id":        {},
	"effective_status":  {},
	"configured_status": {},
	"created_time":      {},
	"updated_time":      {},
}

type CampaignMutationResult struct {
	Operation   string         `json:"operation"`
	CampaignID  string         `json:"campaign_id"`
	RequestPath string         `json:"request_path"`
	Response    map[string]any `json:"response"`
}

type CampaignCreateInput struct {
	AccountID string
	Params    map[string]string
}

type CampaignUpdateInput struct {
	CampaignID string
	Params     map[string]string
}

type CampaignStatusInput struct {
	CampaignID string
	Status     string
}

type CampaignCloneInput struct {
	SourceCampaignID string
	TargetAccountID  string
	Overrides        map[string]string
	Fields           []string
}

type CampaignCloneResult struct {
	Operation        string            `json:"operation"`
	SourceCampaignID string            `json:"source_campaign_id"`
	CampaignID       string            `json:"campaign_id"`
	RequestPath      string            `json:"request_path"`
	Payload          map[string]string `json:"payload"`
	RemovedFields    []string          `json:"removed_fields"`
	Response         map[string]any    `json:"response"`
}

type Service struct {
	Client *graph.Client
}

func NewCampaignService(client *graph.Client) *Service {
	if client == nil {
		client = graph.NewClient(nil, "")
	}
	return &Service{Client: client}
}

func (s *Service) Create(ctx context.Context, version string, token string, appSecret string, input CampaignCreateInput) (*CampaignMutationResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("campaign service client is required")
	}

	accountID, err := normalizeAdAccountID(input.AccountID)
	if err != nil {
		return nil, err
	}

	form, err := normalizeMutationParams(input.Params)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("act_%s/campaigns", accountID)
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

	campaignID, _ := response.Body["id"].(string)
	if strings.TrimSpace(campaignID) == "" {
		return nil, errors.New("campaign create response did not include id")
	}

	return &CampaignMutationResult{
		Operation:   "create",
		CampaignID:  campaignID,
		RequestPath: path,
		Response:    response.Body,
	}, nil
}

func (s *Service) Update(ctx context.Context, version string, token string, appSecret string, input CampaignUpdateInput) (*CampaignMutationResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("campaign service client is required")
	}

	campaignID, err := normalizeGraphID("campaign id", input.CampaignID)
	if err != nil {
		return nil, err
	}

	form, err := normalizeMutationParams(input.Params)
	if err != nil {
		return nil, err
	}

	response, err := s.Client.Do(ctx, graph.Request{
		Method:      "POST",
		Path:        campaignID,
		Version:     strings.TrimSpace(version),
		Form:        form,
		AccessToken: token,
		AppSecret:   appSecret,
	})
	if err != nil {
		return nil, err
	}

	if responseID, ok := response.Body["id"].(string); ok && strings.TrimSpace(responseID) != "" {
		campaignID = responseID
	}

	return &CampaignMutationResult{
		Operation:   "update",
		CampaignID:  campaignID,
		RequestPath: campaignID,
		Response:    response.Body,
	}, nil
}

func (s *Service) SetStatus(ctx context.Context, version string, token string, appSecret string, input CampaignStatusInput) (*CampaignMutationResult, error) {
	normalizedStatus, err := normalizeCampaignStatus(input.Status)
	if err != nil {
		return nil, err
	}

	result, err := s.Update(ctx, version, token, appSecret, CampaignUpdateInput{
		CampaignID: input.CampaignID,
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

func (s *Service) Clone(ctx context.Context, version string, token string, appSecret string, input CampaignCloneInput) (*CampaignCloneResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("campaign service client is required")
	}

	sourceCampaignID, err := normalizeGraphID("source campaign id", input.SourceCampaignID)
	if err != nil {
		return nil, err
	}
	accountID, err := normalizeAdAccountID(input.TargetAccountID)
	if err != nil {
		return nil, err
	}
	fields, err := normalizeCloneFields(input.Fields)
	if err != nil {
		return nil, err
	}
	overrides, err := normalizeOptionalParams(input.Overrides)
	if err != nil {
		return nil, err
	}

	sourceResponse, err := s.Client.Do(ctx, graph.Request{
		Method:  "GET",
		Path:    sourceCampaignID,
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

	clonePayload, removedFields, err := sanitizeCampaignClonePayload(sourceResponse.Body)
	if err != nil {
		return nil, err
	}
	for key, value := range overrides {
		clonePayload[key] = value
	}
	if len(clonePayload) == 0 {
		return nil, errors.New("campaign clone payload is empty after sanitization")
	}

	createPath := fmt.Sprintf("act_%s/campaigns", accountID)
	createResponse, err := s.Client.Do(ctx, graph.Request{
		Method:      "POST",
		Path:        createPath,
		Version:     strings.TrimSpace(version),
		Form:        clonePayload,
		AccessToken: token,
		AppSecret:   appSecret,
	})
	if err != nil {
		return nil, err
	}

	campaignID, _ := createResponse.Body["id"].(string)
	if strings.TrimSpace(campaignID) == "" {
		return nil, errors.New("campaign clone create response did not include id")
	}

	return &CampaignCloneResult{
		Operation:        "clone",
		SourceCampaignID: sourceCampaignID,
		CampaignID:       campaignID,
		RequestPath:      createPath,
		Payload:          clonePayload,
		RemovedFields:    removedFields,
		Response:         createResponse.Body,
	}, nil
}

func normalizeAdAccountID(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(normalized), "act_") {
		normalized = normalized[4:]
	}
	if normalized == "" {
		return "", errors.New("account id is required")
	}
	if strings.Contains(normalized, "/") {
		return "", fmt.Errorf("invalid account id %q: expected single graph id token", value)
	}
	return normalized, nil
}

func normalizeGraphID(label string, value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	if strings.Contains(normalized, "/") {
		return "", fmt.Errorf("invalid %s %q: expected single graph id token", label, value)
	}
	return normalized, nil
}

func normalizeCampaignStatus(value string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch normalized {
	case CampaignStatusActive, CampaignStatusPaused:
		return normalized, nil
	case "":
		return "", errors.New("campaign status is required")
	default:
		return "", fmt.Errorf("unsupported campaign status %q: expected ACTIVE|PAUSED", value)
	}
}

func normalizeMutationParams(params map[string]string) (map[string]string, error) {
	normalized, err := normalizeOptionalParams(params)
	if err != nil {
		return nil, err
	}
	if len(normalized) == 0 {
		return nil, errors.New("campaign mutation payload cannot be empty")
	}
	return normalized, nil
}

func normalizeOptionalParams(params map[string]string) (map[string]string, error) {
	normalized := map[string]string{}
	for key, value := range params {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			return nil, errors.New("mutation param key cannot be empty")
		}
		normalized[trimmedKey] = strings.TrimSpace(value)
	}
	return normalized, nil
}

func normalizeCloneFields(fields []string) ([]string, error) {
	if len(fields) == 0 {
		return append([]string(nil), DefaultCampaignCloneFields...), nil
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
	return out, nil
}

func sanitizeCampaignClonePayload(source map[string]any) (map[string]string, []string, error) {
	if len(source) == 0 {
		return nil, nil, errors.New("campaign clone source payload is empty")
	}

	payload := map[string]string{}
	removed := make([]string, 0)
	for key, value := range source {
		normalizedKey := strings.TrimSpace(key)
		if normalizedKey == "" {
			continue
		}

		if _, immutable := immutableCampaignCloneFields[normalizedKey]; immutable {
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

func encodeGraphValue(value any) (string, error) {
	if value == nil {
		return "", errors.New("nil graph values are not supported")
	}
	if typed, ok := value.(string); ok {
		return typed, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
