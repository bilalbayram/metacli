package marketing

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/bilalbayram/metacli/internal/graph"
)

var DefaultAudienceReadFields = []string{
	"id",
	"name",
	"subtype",
	"time_updated",
	"retention_days",
}

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

type AudienceListInput struct {
	AccountID  string
	Fields     []string
	Limit      int
	FollowNext bool
}

type AudienceGetInput struct {
	AudienceID string
	Fields     []string
}

type AudienceMutationResult struct {
	Operation   string         `json:"operation"`
	AudienceID  string         `json:"audience_id"`
	RequestPath string         `json:"request_path"`
	Response    map[string]any `json:"response"`
}

type AudienceListResult struct {
	Operation   string                  `json:"operation"`
	RequestPath string                  `json:"request_path"`
	Audiences   []map[string]any        `json:"audiences"`
	Paging      *graph.PaginationResult `json:"paging,omitempty"`
}

type AudienceGetResult struct {
	Operation   string         `json:"operation"`
	RequestPath string         `json:"request_path"`
	Audience    map[string]any `json:"audience"`
}

func NewAudienceService(client *graph.Client) *AudienceService {
	if client == nil {
		client = graph.NewClient(nil, "")
	}
	return &AudienceService{Client: client}
}

func (s *AudienceService) List(ctx context.Context, version string, token string, appSecret string, input AudienceListInput) (*AudienceListResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("audience service client is required")
	}

	accountID, err := normalizeAdAccountID(input.AccountID)
	if err != nil {
		return nil, err
	}
	fields, err := normalizeAudienceReadFields(input.Fields)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("act_%s/customaudiences", accountID)
	query := map[string]string{
		"fields": strings.Join(fields, ","),
	}
	if input.Limit > 0 {
		query["limit"] = strconv.Itoa(input.Limit)
	}

	items := make([]map[string]any, 0)
	pagination, err := s.Client.FetchWithPagination(ctx, graph.Request{
		Method:      "GET",
		Path:        path,
		Version:     strings.TrimSpace(version),
		Query:       query,
		AccessToken: token,
		AppSecret:   appSecret,
	}, graph.PaginationOptions{
		FollowNext: input.FollowNext,
		Limit:      input.Limit,
	}, func(item map[string]any) error {
		items = append(items, item)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sortAudienceItemsByID(items)
	return &AudienceListResult{
		Operation:   "list",
		RequestPath: path,
		Audiences:   items,
		Paging:      pagination,
	}, nil
}

func (s *AudienceService) Get(ctx context.Context, version string, token string, appSecret string, input AudienceGetInput) (*AudienceGetResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("audience service client is required")
	}

	audienceID, err := normalizeGraphID("audience id", input.AudienceID)
	if err != nil {
		return nil, err
	}
	fields, err := normalizeAudienceReadFields(input.Fields)
	if err != nil {
		return nil, err
	}

	response, err := s.Client.Do(ctx, graph.Request{
		Method:  "GET",
		Path:    audienceID,
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

	return &AudienceGetResult{
		Operation:   "get",
		RequestPath: audienceID,
		Audience:    response.Body,
	}, nil
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

func normalizeAudienceReadFields(fields []string) ([]string, error) {
	if len(fields) == 0 {
		return append([]string(nil), DefaultAudienceReadFields...), nil
	}

	normalized := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		if trimmed == "" {
			return nil, errors.New("audience fields contain blank entries")
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return nil, errors.New("audience fields are required when fields filter is set")
	}
	return normalized, nil
}

func sortAudienceItemsByID(items []map[string]any) {
	sort.SliceStable(items, func(i, j int) bool {
		leftID, leftHasID := audienceItemID(items[i])
		rightID, rightHasID := audienceItemID(items[j])

		switch {
		case leftHasID && rightHasID:
			return leftID < rightID
		case leftHasID:
			return true
		case rightHasID:
			return false
		default:
			return false
		}
	})
}

func audienceItemID(item map[string]any) (string, bool) {
	if len(item) == 0 {
		return "", false
	}
	idRaw, exists := item["id"]
	if !exists || idRaw == nil {
		return "", false
	}
	id, ok := idRaw.(string)
	if !ok {
		return "", false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return "", false
	}
	return id, true
}
