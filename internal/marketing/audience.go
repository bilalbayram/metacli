package marketing

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bilalbayram/metacli/internal/graph"
)

type AudienceService struct {
	Client *graph.Client
}

type AudienceCreateInput struct {
	AccountID string
	Params    map[string]string
}

type AudienceUpdateInput struct {
	AudienceID string
	Params     map[string]string
}

type AudienceDeleteInput struct {
	AudienceID string
}

type AudienceMutationResult struct {
	Operation   string         `json:"operation"`
	AudienceID  string         `json:"audience_id"`
	RequestPath string         `json:"request_path"`
	Response    map[string]any `json:"response"`
}

func NewAudienceService(client *graph.Client) *AudienceService {
	if client == nil {
		client = graph.NewClient(nil, "")
	}
	return &AudienceService{Client: client}
}

func (s *AudienceService) Create(ctx context.Context, version string, token string, appSecret string, input AudienceCreateInput) (*AudienceMutationResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("audience service client is required")
	}

	accountID, err := normalizeAdAccountID(input.AccountID)
	if err != nil {
		return nil, err
	}
	form, err := normalizeAudienceMutationParams(input.Params)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("act_%s/customaudiences", accountID)
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

	audienceID, _ := response.Body["id"].(string)
	if strings.TrimSpace(audienceID) == "" {
		return nil, errors.New("audience create response did not include id")
	}

	return &AudienceMutationResult{
		Operation:   "create",
		AudienceID:  audienceID,
		RequestPath: path,
		Response:    response.Body,
	}, nil
}

func (s *AudienceService) Update(ctx context.Context, version string, token string, appSecret string, input AudienceUpdateInput) (*AudienceMutationResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("audience service client is required")
	}

	audienceID, err := normalizeGraphID("audience id", input.AudienceID)
	if err != nil {
		return nil, err
	}
	form, err := normalizeAudienceMutationParams(input.Params)
	if err != nil {
		return nil, err
	}

	response, err := s.Client.Do(ctx, graph.Request{
		Method:      "POST",
		Path:        audienceID,
		Version:     strings.TrimSpace(version),
		Form:        form,
		AccessToken: token,
		AppSecret:   appSecret,
	})
	if err != nil {
		return nil, err
	}

	if responseID, ok := response.Body["id"].(string); ok && strings.TrimSpace(responseID) != "" {
		audienceID = responseID
	}

	return &AudienceMutationResult{
		Operation:   "update",
		AudienceID:  audienceID,
		RequestPath: audienceID,
		Response:    response.Body,
	}, nil
}

func (s *AudienceService) Delete(ctx context.Context, version string, token string, appSecret string, input AudienceDeleteInput) (*AudienceMutationResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("audience service client is required")
	}

	audienceID, err := normalizeGraphID("audience id", input.AudienceID)
	if err != nil {
		return nil, err
	}

	response, err := s.Client.Do(ctx, graph.Request{
		Method:      "DELETE",
		Path:        audienceID,
		Version:     strings.TrimSpace(version),
		AccessToken: token,
		AppSecret:   appSecret,
	})
	if err != nil {
		return nil, err
	}

	successValue, hasSuccess := response.Body["success"]
	if hasSuccess {
		success, ok := successValue.(bool)
		if !ok || !success {
			return nil, errors.New("audience delete response was not successful")
		}
	}

	return &AudienceMutationResult{
		Operation:   "delete",
		AudienceID:  audienceID,
		RequestPath: audienceID,
		Response:    response.Body,
	}, nil
}

func normalizeAudienceMutationParams(params map[string]string) (map[string]string, error) {
	normalized := map[string]string{}
	for key, value := range params {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			return nil, errors.New("audience mutation param key cannot be empty")
		}
		normalized[trimmedKey] = strings.TrimSpace(value)
	}
	if len(normalized) == 0 {
		return nil, errors.New("audience mutation payload cannot be empty")
	}
	return normalized, nil
}
