package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/bilalbayram/metacli/internal/auth"
	"github.com/bilalbayram/metacli/internal/config"
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Client struct {
	BaseURL        string
	HTTP           HTTPClient
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Sleep          func(time.Duration)
	UserAgent      string
}

type Request struct {
	Method      string
	Path        string
	Version     string
	Query       map[string]string
	Form        map[string]string
	AccessToken string
	AppSecret   string
}

type Response struct {
	StatusCode int
	Body       map[string]any
	Raw        []byte
	Headers    http.Header
	RateLimit  RateLimit
}

type RateLimit struct {
	AppUsage      map[string]any `json:"app_usage,omitempty"`
	PageUsage     map[string]any `json:"page_usage,omitempty"`
	AdAccountUsage map[string]any `json:"ad_account_usage,omitempty"`
}

func NewClient(httpClient HTTPClient, baseURL string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if baseURL == "" {
		baseURL = auth.DefaultGraphBaseURL
	}

	return &Client{
		BaseURL:        strings.TrimSuffix(baseURL, "/"),
		HTTP:           httpClient,
		MaxRetries:     4,
		InitialBackoff: 300 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		Sleep:          time.Sleep,
		UserAgent:      "meta-marketing-cli/1.0",
	}
}

func (c *Client) Do(ctx context.Context, req Request) (*Response, error) {
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}
	if req.Path == "" {
		return nil, errors.New("graph request path is required")
	}
	version := req.Version
	if version == "" {
		version = config.DefaultGraphVersion
	}
	attempt := 0
	backoff := c.InitialBackoff

	for {
		attempt++
		response, err := c.doOnce(ctx, method, version, req)
		if err == nil {
			return response, nil
		}

		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Retryable && attempt <= c.MaxRetries {
			c.Sleep(backoff)
			backoff = nextBackoff(backoff, c.MaxBackoff)
			continue
		}

		var transient *TransientError
		if errors.As(err, &transient) && attempt <= c.MaxRetries {
			c.Sleep(backoff)
			backoff = nextBackoff(backoff, c.MaxBackoff)
			continue
		}

		return nil, err
	}
}

func (c *Client) doOnce(ctx context.Context, method string, version string, req Request) (*Response, error) {
	endpoint, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse graph base url: %w", err)
	}
	endpoint.Path = path.Join(endpoint.Path, version, strings.TrimPrefix(req.Path, "/"))

	query := url.Values{}
	for key, value := range req.Query {
		query.Set(key, value)
	}

	bodyReader := io.Reader(nil)
	if method == http.MethodGet || method == http.MethodDelete {
		if req.AccessToken != "" {
			query.Set("access_token", req.AccessToken)
		}
		if req.AccessToken != "" && req.AppSecret != "" {
			proof, err := auth.AppSecretProof(req.AccessToken, req.AppSecret)
			if err != nil {
				return nil, err
			}
			query.Set("appsecret_proof", proof)
		}
	} else {
		form := url.Values{}
		for key, value := range req.Form {
			form.Set(key, value)
		}
		if req.AccessToken != "" {
			form.Set("access_token", req.AccessToken)
		}
		if req.AccessToken != "" && req.AppSecret != "" {
			proof, err := auth.AppSecretProof(req.AccessToken, req.AppSecret)
			if err != nil {
				return nil, err
			}
			form.Set("appsecret_proof", proof)
		}
		bodyReader = bytes.NewBufferString(form.Encode())
	}
	endpoint.RawQuery = query.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build graph request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", c.UserAgent)
	if method != http.MethodGet && method != http.MethodDelete {
		httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	httpRes, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, &TransientError{Message: fmt.Sprintf("send request: %v", err)}
	}
	defer httpRes.Body.Close()

	body, err := io.ReadAll(httpRes.Body)
	if err != nil {
		return nil, &TransientError{Message: fmt.Sprintf("read response: %v", err)}
	}

	parsed := map[string]any{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("decode response JSON: %w", err)
		}
	}

	if apiErr := parseAPIError(httpRes.StatusCode, parsed); apiErr != nil {
		return nil, apiErr
	}
	if httpRes.StatusCode >= 500 || httpRes.StatusCode == http.StatusTooManyRequests {
		return nil, &TransientError{
			Message:    fmt.Sprintf("transient status code %d", httpRes.StatusCode),
			StatusCode: httpRes.StatusCode,
		}
	}
	if httpRes.StatusCode < 200 || httpRes.StatusCode >= 300 {
		return nil, fmt.Errorf("request failed with status %d", httpRes.StatusCode)
	}

	return &Response{
		StatusCode: httpRes.StatusCode,
		Body:       parsed,
		Raw:        body,
		Headers:    httpRes.Header.Clone(),
		RateLimit:  parseRateLimit(httpRes.Header),
	}, nil
}

func parseRateLimit(headers http.Header) RateLimit {
	return RateLimit{
		AppUsage:      parseUsageHeader(headers.Get("X-App-Usage")),
		PageUsage:     parseUsageHeader(headers.Get("X-Page-Usage")),
		AdAccountUsage: parseUsageHeader(headers.Get("X-Ad-Account-Usage")),
	}
}

func parseUsageHeader(value string) map[string]any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parsed := map[string]any{}
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		return map[string]any{
			"raw": value,
		}
	}
	return parsed
}

func parseAPIError(statusCode int, payload map[string]any) *APIError {
	rawErr, ok := payload["error"]
	if !ok {
		if statusCode == http.StatusTooManyRequests {
			return &APIError{
				Type:       "rate_limit",
				Code:       http.StatusTooManyRequests,
				Message:    "rate limited",
				StatusCode: statusCode,
				Retryable:  true,
			}
		}
		return nil
	}
	errMap, ok := rawErr.(map[string]any)
	if !ok {
		return &APIError{
			Type:       "unknown",
			Message:    "unparseable error payload",
			StatusCode: statusCode,
			Retryable:  statusCode >= 500 || statusCode == http.StatusTooManyRequests,
		}
	}

	errCode := intFromAny(errMap["code"])
	subcode := intFromAny(errMap["error_subcode"])
	message, _ := errMap["message"].(string)
	errType, _ := errMap["type"].(string)
	trace, _ := errMap["fbtrace_id"].(string)
	retryable := ShouldRetry(statusCode, errCode)

	return &APIError{
		Type:         errType,
		Code:         errCode,
		ErrorSubcode: subcode,
		Message:      message,
		FBTraceID:    trace,
		StatusCode:   statusCode,
		Retryable:    retryable,
	}
}

func ShouldRetry(statusCode int, code int) bool {
	if statusCode == http.StatusTooManyRequests {
		return true
	}
	if statusCode >= 500 {
		return true
	}
	switch code {
	case 4, 17, 32, 613:
		return true
	default:
		return false
	}
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case string:
		parsed, err := strconv.Atoi(typed)
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func nextBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}
