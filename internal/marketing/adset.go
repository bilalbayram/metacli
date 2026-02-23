package marketing

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bilalbayram/metacli/internal/graph"
)

const (
	AdSetStatusActive = "ACTIVE"
	AdSetStatusPaused = "PAUSED"
)

type AdSetMutationResult struct {
	Operation   string         `json:"operation"`
	AdSetID     string         `json:"adset_id"`
	RequestPath string         `json:"request_path"`
	Response    map[string]any `json:"response"`
}

type AdSetCreateInput struct {
	AccountID string
	Params    map[string]string
}

type AdSetUpdateInput struct {
	AdSetID string
	Params  map[string]string
}

type AdSetStatusInput struct {
	AdSetID string
	Status  string
}

type AdSetService struct {
	Client *graph.Client
}

func NewAdSetService(client *graph.Client) *AdSetService {
	if client == nil {
		client = graph.NewClient(nil, "")
	}
	return &AdSetService{Client: client}
}

func NewAdsetService(client *graph.Client) *AdSetService {
	return NewAdSetService(client)
}

func (s *AdSetService) Create(ctx context.Context, version string, token string, appSecret string, input AdSetCreateInput) (*AdSetMutationResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("ad set service client is required")
	}

	accountID, err := normalizeAdAccountID(input.AccountID)
	if err != nil {
		return nil, err
	}
	form, err := normalizeAdSetMutationParams(input.Params)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("act_%s/adsets", accountID)
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

	adSetID, _ := response.Body["id"].(string)
	if strings.TrimSpace(adSetID) == "" {
		return nil, errors.New("ad set create response did not include id")
	}

	return &AdSetMutationResult{
		Operation:   "create",
		AdSetID:     adSetID,
		RequestPath: path,
		Response:    response.Body,
	}, nil
}

func (s *AdSetService) Update(ctx context.Context, version string, token string, appSecret string, input AdSetUpdateInput) (*AdSetMutationResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("ad set service client is required")
	}

	adSetID, err := normalizeGraphID("ad set id", input.AdSetID)
	if err != nil {
		return nil, err
	}
	form, err := normalizeAdSetMutationParams(input.Params)
	if err != nil {
		return nil, err
	}

	response, err := s.Client.Do(ctx, graph.Request{
		Method:      "POST",
		Path:        adSetID,
		Version:     strings.TrimSpace(version),
		Form:        form,
		AccessToken: token,
		AppSecret:   appSecret,
	})
	if err != nil {
		return nil, err
	}

	if responseID, ok := response.Body["id"].(string); ok && strings.TrimSpace(responseID) != "" {
		adSetID = responseID
	}

	return &AdSetMutationResult{
		Operation:   "update",
		AdSetID:     adSetID,
		RequestPath: adSetID,
		Response:    response.Body,
	}, nil
}

func (s *AdSetService) SetStatus(ctx context.Context, version string, token string, appSecret string, input AdSetStatusInput) (*AdSetMutationResult, error) {
	normalizedStatus, err := normalizeAdSetStatus(input.Status)
	if err != nil {
		return nil, err
	}

	result, err := s.Update(ctx, version, token, appSecret, AdSetUpdateInput{
		AdSetID: input.AdSetID,
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

func normalizeAdSetStatus(value string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch normalized {
	case AdSetStatusActive, AdSetStatusPaused:
		return normalized, nil
	case "":
		return "", errors.New("ad set status is required")
	default:
		return "", fmt.Errorf("unsupported ad set status %q: expected ACTIVE|PAUSED", value)
	}
}

func normalizeAdSetMutationParams(params map[string]string) (map[string]string, error) {
	normalized, err := normalizeOptionalParams(params)
	if err != nil {
		return nil, err
	}
	if len(normalized) == 0 {
		return nil, errors.New("ad set mutation payload cannot be empty")
	}
	return normalized, nil
}
