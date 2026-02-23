package graph

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type PaginationOptions struct {
	FollowNext bool
	Limit      int
	PageSize   int
	Stream     bool
}

type PaginationResult struct {
	PagesFetched int    `json:"pages_fetched"`
	ItemsFetched int    `json:"items_fetched"`
	Next         string `json:"next,omitempty"`
}

func (c *Client) FetchWithPagination(ctx context.Context, req Request, options PaginationOptions, onItem func(map[string]any) error) (*PaginationResult, error) {
	if strings.ToUpper(req.Method) != httpMethodGet {
		return nil, fmt.Errorf("pagination only supports GET requests")
	}
	if req.Query == nil {
		req.Query = map[string]string{}
	}
	if options.PageSize > 0 && req.Query["limit"] == "" {
		req.Query["limit"] = strconv.Itoa(options.PageSize)
	}

	result := &PaginationResult{}
	current := req

	for {
		resp, err := c.Do(ctx, current)
		if err != nil {
			return nil, err
		}
		result.PagesFetched++

		items := extractDataItems(resp.Body)
		for _, item := range items {
			if options.Limit > 0 && result.ItemsFetched >= options.Limit {
				return result, nil
			}
			result.ItemsFetched++
			if onItem != nil {
				if err := onItem(item); err != nil {
					return nil, err
				}
			}
		}

		next := extractNextPage(resp.Body)
		result.Next = next
		if !options.FollowNext || next == "" {
			return result, nil
		}

		nextReq, err := followRequestFromNextURL(next, current)
		if err != nil {
			return nil, err
		}
		current = nextReq
	}
}

func extractDataItems(payload map[string]any) []map[string]any {
	raw, ok := payload["data"].([]any)
	if !ok {
		return nil
	}

	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		itemMap, ok := item.(map[string]any)
		if ok {
			out = append(out, itemMap)
		}
	}
	return out
}

func extractNextPage(payload map[string]any) string {
	paging, ok := payload["paging"].(map[string]any)
	if !ok {
		return ""
	}
	next, _ := paging["next"].(string)
	return next
}

func followRequestFromNextURL(nextURL string, previous Request) (Request, error) {
	parsed, err := url.Parse(nextURL)
	if err != nil {
		return Request{}, fmt.Errorf("parse paging.next url %q: %w", nextURL, err)
	}
	segments := strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")
	if len(segments) < 2 {
		return Request{}, fmt.Errorf("invalid paging.next path %q", parsed.Path)
	}
	version := previous.Version
	if version == "" {
		version = segments[0]
	}
	relPath := strings.Join(segments[1:], "/")

	query := map[string]string{}
	for key, values := range parsed.Query() {
		if len(values) == 0 {
			continue
		}
		if key == "access_token" || key == "appsecret_proof" {
			continue
		}
		query[key] = values[len(values)-1]
	}

	return Request{
		Method:      httpMethodGet,
		Path:        relPath,
		Version:     version,
		Query:       query,
		AccessToken: previous.AccessToken,
		AppSecret:   previous.AppSecret,
	}, nil
}
