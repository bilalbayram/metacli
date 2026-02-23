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
	"strings"

	"github.com/bilalbayram/metacli/internal/auth"
	"github.com/bilalbayram/metacli/internal/config"
)

const (
	maxBatchRequests = 50
	httpMethodGet    = "GET"
)

type BatchRequest struct {
	Method string            `json:"method"`
	Path   string            `json:"path"`
	Params map[string]string `json:"params,omitempty"`
}

type BatchResult struct {
	Code int            `json:"code"`
	Body map[string]any `json:"body"`
}

func ValidateBatchRequests(requests []BatchRequest) error {
	if len(requests) == 0 {
		return errors.New("batch request list cannot be empty")
	}
	if len(requests) > maxBatchRequests {
		return fmt.Errorf("batch request count %d exceeds limit %d", len(requests), maxBatchRequests)
	}

	for idx, req := range requests {
		if strings.TrimSpace(req.Path) == "" {
			return fmt.Errorf("batch request %d path is required", idx)
		}
		if strings.ToUpper(strings.TrimSpace(req.Method)) != httpMethodGet {
			return fmt.Errorf("batch request %d uses unsupported method %q; only GET is supported in v1", idx, req.Method)
		}
	}
	return nil
}

func (c *Client) ExecuteGETBatch(ctx context.Context, version string, accessToken string, appSecret string, requests []BatchRequest) ([]BatchResult, error) {
	if err := ValidateBatchRequests(requests); err != nil {
		return nil, err
	}
	if strings.TrimSpace(accessToken) == "" {
		return nil, errors.New("access token is required for batch execution")
	}
	if version == "" {
		version = config.DefaultGraphVersion
	}

	entries := make([]map[string]string, 0, len(requests))
	for _, req := range requests {
		relativeURL := strings.TrimPrefix(req.Path, "/")
		if len(req.Params) > 0 {
			values := url.Values{}
			for key, value := range req.Params {
				values.Set(key, value)
			}
			relativeURL = relativeURL + "?" + values.Encode()
		}
		entries = append(entries, map[string]string{
			"method":       httpMethodGet,
			"relative_url": relativeURL,
		})
	}
	batchPayload, err := json.Marshal(entries)
	if err != nil {
		return nil, fmt.Errorf("marshal batch payload: %w", err)
	}

	form := url.Values{}
	form.Set("access_token", accessToken)
	form.Set("batch", string(batchPayload))
	if appSecret != "" {
		proof, err := auth.AppSecretProof(accessToken, appSecret)
		if err != nil {
			return nil, err
		}
		form.Set("appsecret_proof", proof)
	}

	endpoint, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse graph base url: %w", err)
	}
	endpoint.Path = path.Join(endpoint.Path, version)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewBufferString(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build batch request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", c.UserAgent)

	httpRes, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send batch request: %w", err)
	}
	defer httpRes.Body.Close()

	body, err := io.ReadAll(httpRes.Body)
	if err != nil {
		return nil, fmt.Errorf("read batch response: %w", err)
	}
	if httpRes.StatusCode < 200 || httpRes.StatusCode >= 300 {
		return nil, fmt.Errorf("batch request failed with status %d: %s", httpRes.StatusCode, strings.TrimSpace(string(body)))
	}

	var rawItems []struct {
		Code int    `json:"code"`
		Body string `json:"body"`
	}
	if err := json.Unmarshal(body, &rawItems); err != nil {
		return nil, fmt.Errorf("decode batch response: %w", err)
	}

	results := make([]BatchResult, 0, len(rawItems))
	for idx, item := range rawItems {
		parsedBody := map[string]any{}
		if strings.TrimSpace(item.Body) != "" {
			if err := json.Unmarshal([]byte(item.Body), &parsedBody); err != nil {
				return nil, fmt.Errorf("decode batch item %d body: %w", idx, err)
			}
		}
		if apiErr := parseAPIError(item.Code, parsedBody); apiErr != nil {
			return nil, fmt.Errorf("batch item %d failed: %w", idx, apiErr)
		}
		results = append(results, BatchResult{
			Code: item.Code,
			Body: parsedBody,
		})
	}
	return results, nil
}
