package marketing

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
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

func (s *AdSetService) ResolveAccountID(ctx context.Context, version string, token string, appSecret string, adSetID string) (string, error) {
	if s == nil || s.Client == nil {
		return "", errors.New("ad set service client is required")
	}

	normalizedAdSetID, err := normalizeGraphID("ad set id", adSetID)
	if err != nil {
		return "", err
	}

	response, err := s.Client.Do(ctx, graph.Request{
		Method:  "GET",
		Path:    normalizedAdSetID,
		Version: strings.TrimSpace(version),
		Query: map[string]string{
			"fields": "account_id",
		},
		AccessToken: token,
		AppSecret:   appSecret,
	})
	if err != nil {
		return "", err
	}

	accountID, err := decodeGraphIDField(response.Body, "account_id")
	if err != nil {
		return "", fmt.Errorf("ad set account context lookup failed: %w", err)
	}
	return normalizeAdAccountID(accountID)
}

func (s *AdSetService) ResolveAccountCurrency(ctx context.Context, version string, token string, appSecret string, accountID string) (string, error) {
	if s == nil || s.Client == nil {
		return "", errors.New("ad set service client is required")
	}

	normalizedAccountID, err := normalizeAdAccountID(accountID)
	if err != nil {
		return "", err
	}

	path := fmt.Sprintf("act_%s", normalizedAccountID)
	response, err := s.Client.Do(ctx, graph.Request{
		Method:  "GET",
		Path:    path,
		Version: strings.TrimSpace(version),
		Query: map[string]string{
			"fields": "currency",
		},
		AccessToken: token,
		AppSecret:   appSecret,
	})
	if err != nil {
		return "", err
	}

	currencyRaw, exists := response.Body["currency"]
	if !exists {
		return "", errors.New("ad account currency lookup response did not include currency")
	}
	currency, ok := currencyRaw.(string)
	if !ok {
		return "", fmt.Errorf("ad account currency lookup response field currency has unsupported type %T", currencyRaw)
	}
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		return "", errors.New("ad account currency lookup response included empty currency")
	}
	return currency, nil
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

func decodeGraphIDField(body map[string]any, field string) (string, error) {
	if len(body) == 0 {
		return "", fmt.Errorf("response missing required field %q", field)
	}
	value, exists := body[field]
	if !exists || value == nil {
		return "", fmt.Errorf("response missing required field %q", field)
	}

	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return "", fmt.Errorf("response field %q is empty", field)
		}
		return trimmed, nil
	case float64:
		if math.Trunc(typed) != typed {
			return "", fmt.Errorf("response field %q must be an integer id value", field)
		}
		return strconv.FormatInt(int64(typed), 10), nil
	default:
		return "", fmt.Errorf("response field %q has unsupported type %T", field, value)
	}
}
