package msgr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/bilalbayram/metacli/internal/graph"
)

type ListConversationsOptions struct {
	PageID string
	Limit  int
}

type ListConversationsResult struct {
	PageID        string                  `json:"page_id"`
	Conversations []map[string]any        `json:"conversations"`
	Pagination    *graph.PaginationResult `json:"pagination,omitempty"`
}

type ReplyOptions struct {
	RecipientID string
	Message     string
}

type ReplyResult struct {
	RecipientID string         `json:"recipient_id"`
	Response    map[string]any `json:"response"`
}

type SetGreetingOptions struct {
	PageID  string
	Message string
	Locale  string
}

type SetGreetingResult struct {
	PageID   string         `json:"page_id"`
	Response map[string]any `json:"response"`
}

type Service struct {
	Client *graph.Client
}

func New(client *graph.Client) *Service {
	if client == nil {
		client = graph.NewClient(nil, "")
	}
	return &Service{Client: client}
}

func (s *Service) ListConversations(ctx context.Context, version string, token string, appSecret string, options ListConversationsOptions) (*ListConversationsResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("messenger service client is required")
	}

	req, pageID, err := BuildListConversationsRequest(version, token, appSecret, options)
	if err != nil {
		return nil, err
	}

	var conversations []map[string]any
	paginationResult, err := s.Client.FetchWithPagination(ctx, req, graph.PaginationOptions{
		FollowNext: true,
		Limit:      options.Limit,
	}, func(item map[string]any) error {
		conversations = append(conversations, item)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &ListConversationsResult{
		PageID:        pageID,
		Conversations: conversations,
		Pagination:    paginationResult,
	}, nil
}

func (s *Service) Reply(ctx context.Context, version string, token string, appSecret string, options ReplyOptions) (*ReplyResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("messenger service client is required")
	}

	req, err := BuildReplyRequest(version, token, appSecret, options)
	if err != nil {
		return nil, err
	}

	response, err := s.Client.Do(ctx, req)
	if err != nil {
		return nil, err
	}

	return &ReplyResult{
		RecipientID: strings.TrimSpace(options.RecipientID),
		Response:    response.Body,
	}, nil
}

func (s *Service) SetGreeting(ctx context.Context, version string, token string, appSecret string, options SetGreetingOptions) (*SetGreetingResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("messenger service client is required")
	}

	req, pageID, err := BuildSetGreetingRequest(version, token, appSecret, options)
	if err != nil {
		return nil, err
	}

	response, err := s.Client.Do(ctx, req)
	if err != nil {
		return nil, err
	}

	return &SetGreetingResult{
		PageID:   pageID,
		Response: response.Body,
	}, nil
}

func BuildListConversationsRequest(version string, token string, appSecret string, options ListConversationsOptions) (graph.Request, string, error) {
	pageID := strings.TrimSpace(options.PageID)
	if pageID == "" {
		return graph.Request{}, "", errors.New("page id is required")
	}

	return graph.Request{
		Method:  "GET",
		Path:    fmt.Sprintf("%s/conversations", pageID),
		Version: strings.TrimSpace(version),
		Query: map[string]string{
			"fields": "id,updated_time,participants,messages{message,from,created_time}",
		},
		AccessToken: token,
		AppSecret:   appSecret,
	}, pageID, nil
}

func BuildReplyRequest(version string, token string, appSecret string, options ReplyOptions) (graph.Request, error) {
	recipientID := strings.TrimSpace(options.RecipientID)
	if recipientID == "" {
		return graph.Request{}, errors.New("recipient id is required")
	}

	message := strings.TrimSpace(options.Message)
	if message == "" {
		return graph.Request{}, errors.New("message is required")
	}

	recipientPayload, err := marshalJSONFormValue(map[string]string{"id": recipientID})
	if err != nil {
		return graph.Request{}, fmt.Errorf("encode recipient payload: %w", err)
	}
	messagePayload, err := marshalJSONFormValue(map[string]string{"text": message})
	if err != nil {
		return graph.Request{}, fmt.Errorf("encode message payload: %w", err)
	}

	return graph.Request{
		Method:  "POST",
		Path:    "me/messages",
		Version: strings.TrimSpace(version),
		Form: map[string]string{
			"recipient":      recipientPayload,
			"message":        messagePayload,
			"messaging_type": "RESPONSE",
		},
		AccessToken: token,
		AppSecret:   appSecret,
	}, nil
}

func BuildSetGreetingRequest(version string, token string, appSecret string, options SetGreetingOptions) (graph.Request, string, error) {
	pageID := strings.TrimSpace(options.PageID)
	if pageID == "" {
		return graph.Request{}, "", errors.New("page id is required")
	}

	message := strings.TrimSpace(options.Message)
	if message == "" {
		return graph.Request{}, "", errors.New("message is required")
	}

	locale := strings.TrimSpace(options.Locale)
	if locale == "" {
		locale = "default"
	}

	greetingPayload, err := marshalJSONFormValue([]map[string]string{{
		"locale": locale,
		"text":   message,
	}})
	if err != nil {
		return graph.Request{}, "", fmt.Errorf("encode greeting payload: %w", err)
	}

	return graph.Request{
		Method:  "POST",
		Path:    fmt.Sprintf("%s/messenger_profile", pageID),
		Version: strings.TrimSpace(version),
		Form: map[string]string{
			"greeting": greetingPayload,
		},
		AccessToken: token,
		AppSecret:   appSecret,
	}, pageID, nil
}

func marshalJSONFormValue(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
