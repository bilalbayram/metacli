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
		return nil, errors.New("ad clone payload is empty after sanitization")
	}
	if err := s.validateDependencies(ctx, version, token, appSecret, clonePayload); err != nil {
		return nil, err
	}

	createPath := fmt.Sprintf("act_%s/ads", accountID)
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

	adID, _ := createResponse.Body["id"].(string)
	if strings.TrimSpace(adID) == "" {
		return nil, errors.New("ad clone create response did not include id")
	}

	return &AdCloneResult{
		Operation:     "clone",
		SourceAdID:    sourceAdID,
		AdID:          adID,
		RequestPath:   createPath,
		Payload:       clonePayload,
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
	return out, nil
}

func sanitizeAdClonePayload(source map[string]any) (map[string]string, []string, error) {
	if len(source) == 0 {
		return nil, nil, errors.New("ad clone source payload is empty")
	}

	payload := map[string]string{}
	removed := make([]string, 0)
	for key, value := range source {
		normalizedKey := strings.TrimSpace(key)
		if normalizedKey == "" {
			continue
		}
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
