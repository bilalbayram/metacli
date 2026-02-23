package marketing

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bilalbayram/metacli/internal/graph"
)

const (
	CampaignStatusActive = "ACTIVE"
	CampaignStatusPaused = "PAUSED"
)

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
	normalized := map[string]string{}
	for key, value := range params {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			return nil, errors.New("mutation param key cannot be empty")
		}
		normalized[trimmedKey] = strings.TrimSpace(value)
	}
	if len(normalized) == 0 {
		return nil, errors.New("campaign mutation payload cannot be empty")
	}
	return normalized, nil
}
