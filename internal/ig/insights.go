package ig

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bilalbayram/metacli/internal/graph"
)

const defaultMediaListFields = "id,caption,media_product_type,media_type,permalink,timestamp"

type AccountInsightsOptions struct {
	IGUserID   string
	Metrics    []string
	Period     string
	MetricType string
	Breakdown  string
	Since      string
	Until      string
	Timeframe  string
}

type AccountInsightsResult struct {
	IGUserID   string                  `json:"ig_user_id"`
	RawMetrics []map[string]any        `json:"raw_metrics"`
	Pagination *graph.PaginationResult `json:"pagination,omitempty"`
}

type MediaListOptions struct {
	IGUserID string
	Limit    int
}

type MediaListResult struct {
	IGUserID   string                  `json:"ig_user_id"`
	Media      []map[string]any        `json:"media"`
	Pagination *graph.PaginationResult `json:"pagination,omitempty"`
}

type MediaInsightsOptions struct {
	MediaIDs   []string
	Metrics    []string
	Period     string
	MetricType string
	Breakdown  string
	Since      string
	Until      string
	Timeframe  string
}

type MediaInsightsRow struct {
	MediaID    string           `json:"media_id"`
	RawMetrics []map[string]any `json:"raw_metrics"`
}

func (s *Service) AccountInsights(ctx context.Context, version string, token string, appSecret string, options AccountInsightsOptions) (*AccountInsightsResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("instagram service client is required")
	}

	igUserID, query, err := buildInsightsQuery("ig user id", options.IGUserID, options.Metrics, options.Period, options.MetricType, options.Breakdown, options.Since, options.Until, options.Timeframe)
	if err != nil {
		return nil, err
	}

	response, err := s.Client.Do(ctx, graph.Request{
		Method:      "GET",
		Path:        fmt.Sprintf("%s/insights", igUserID),
		Version:     strings.TrimSpace(version),
		Query:       query,
		AccessToken: token,
		AppSecret:   appSecret,
	})
	if err != nil {
		return nil, err
	}

	rawMetrics := extractBodyItems(response.Body)
	return &AccountInsightsResult{
		IGUserID:   igUserID,
		RawMetrics: rawMetrics,
		Pagination: extractPagination(response.Body, len(rawMetrics)),
	}, nil
}

func (s *Service) MediaList(ctx context.Context, version string, token string, appSecret string, options MediaListOptions) (*MediaListResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("instagram service client is required")
	}

	igUserID, err := normalizeGraphID("ig user id", options.IGUserID)
	if err != nil {
		return nil, err
	}

	media := make([]map[string]any, 0)
	pagination, err := s.Client.FetchWithPagination(ctx, graph.Request{
		Method:  "GET",
		Path:    fmt.Sprintf("%s/media", igUserID),
		Version: strings.TrimSpace(version),
		Query: map[string]string{
			"fields": defaultMediaListFields,
		},
		AccessToken: token,
		AppSecret:   appSecret,
	}, graph.PaginationOptions{
		FollowNext: true,
		Limit:      options.Limit,
	}, func(item map[string]any) error {
		media = append(media, item)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &MediaListResult{
		IGUserID:   igUserID,
		Media:      media,
		Pagination: pagination,
	}, nil
}

func (s *Service) MediaInsights(ctx context.Context, version string, token string, appSecret string, options MediaInsightsOptions) ([]MediaInsightsRow, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("instagram service client is required")
	}

	metrics, err := normalizeMetrics(options.Metrics)
	if err != nil {
		return nil, err
	}
	period := strings.TrimSpace(options.Period)
	if period == "" {
		return nil, errors.New("period is required")
	}
	if len(options.MediaIDs) == 0 {
		return nil, errors.New("at least one media id is required")
	}

	rows := make([]MediaInsightsRow, 0, len(options.MediaIDs))
	for _, rawMediaID := range options.MediaIDs {
		mediaID, query, err := buildInsightsQuery("media id", rawMediaID, metrics, period, options.MetricType, options.Breakdown, options.Since, options.Until, options.Timeframe)
		if err != nil {
			return nil, err
		}

		response, err := s.Client.Do(ctx, graph.Request{
			Method:      "GET",
			Path:        fmt.Sprintf("%s/insights", mediaID),
			Version:     strings.TrimSpace(version),
			Query:       query,
			AccessToken: token,
			AppSecret:   appSecret,
		})
		if err != nil {
			return nil, err
		}

		rows = append(rows, MediaInsightsRow{
			MediaID:    mediaID,
			RawMetrics: extractBodyItems(response.Body),
		})
	}

	return rows, nil
}

func buildInsightsQuery(idLabel string, rawID string, rawMetrics []string, period string, metricType string, breakdown string, since string, until string, timeframe string) (string, map[string]string, error) {
	graphID, err := normalizeGraphID(idLabel, rawID)
	if err != nil {
		return "", nil, err
	}

	metrics, err := normalizeMetrics(rawMetrics)
	if err != nil {
		return "", nil, err
	}
	period = strings.TrimSpace(period)
	if period == "" {
		return "", nil, errors.New("period is required")
	}

	query := map[string]string{
		"metric": strings.Join(metrics, ","),
		"period": period,
	}
	if value := strings.TrimSpace(metricType); value != "" {
		query["metric_type"] = value
	}
	if value := strings.TrimSpace(breakdown); value != "" {
		query["breakdown"] = value
	}
	trimmedSince := strings.TrimSpace(since)
	trimmedUntil := strings.TrimSpace(until)
	if trimmedSince != "" || trimmedUntil != "" {
		if trimmedSince == "" || trimmedUntil == "" {
			return "", nil, errors.New("both since and until are required when a time range is provided")
		}
		query["since"] = trimmedSince
		query["until"] = trimmedUntil
	}
	if value := strings.TrimSpace(timeframe); value != "" {
		query["timeframe"] = value
	}
	return graphID, query, nil
}

func normalizeMetrics(values []string) ([]string, error) {
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return nil, errors.New("at least one metric is required")
	}
	return normalized, nil
}

func extractBodyItems(body map[string]any) []map[string]any {
	raw, ok := body["data"].([]any)
	if !ok {
		return nil
	}
	items := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		items = append(items, row)
	}
	return items
}

func extractPagination(body map[string]any, itemCount int) *graph.PaginationResult {
	paging, ok := body["paging"].(map[string]any)
	if !ok {
		return nil
	}
	next, _ := paging["next"].(string)
	return &graph.PaginationResult{
		PagesFetched: 1,
		ItemsFetched: itemCount,
		Next:         strings.TrimSpace(next),
	}
}

type AccountLocalIntentReport struct {
	Summary    map[string]any   `json:"summary"`
	RawMetrics []map[string]any `json:"raw_metrics"`
}

var accountLocalIntentAliases = map[string]string{
	"CALL":      "calls",
	"DIRECTION": "directions",
	"EMAIL":     "email_contacts",
	"TEXT":      "text_contacts",
	"BOOK_NOW":  "book_now",
}

func BuildAccountLocalIntentReport(rawMetrics []map[string]any) AccountLocalIntentReport {
	report := AccountLocalIntentReport{
		Summary:    map[string]any{},
		RawMetrics: append([]map[string]any(nil), rawMetrics...),
	}

	for _, metric := range rawMetrics {
		name, _ := metric["name"].(string)
		switch strings.TrimSpace(name) {
		case "profile_links_taps":
			for key, value := range accountLocalIntentBreakdownSummary(metric) {
				report.Summary[key] = value
			}
		case "profile_views":
			value, ok := nestedMetricTotal(metric)
			if !ok {
				continue
			}
			report.Summary["profile_views"] = value
		}
	}

	return report
}

func accountLocalIntentBreakdownSummary(metric map[string]any) map[string]any {
	summary := make(map[string]any)
	totalValue, ok := metric["total_value"].(map[string]any)
	if !ok {
		return summary
	}
	breakdowns, ok := totalValue["breakdowns"].([]any)
	if !ok {
		return summary
	}
	for _, rawBreakdown := range breakdowns {
		breakdown, ok := rawBreakdown.(map[string]any)
		if !ok {
			continue
		}
		results, ok := breakdown["results"].([]any)
		if !ok {
			continue
		}
		for _, rawResult := range results {
			result, ok := rawResult.(map[string]any)
			if !ok {
				continue
			}
			dimensionValues, ok := result["dimension_values"].([]any)
			if !ok || len(dimensionValues) == 0 {
				continue
			}
			dimensionValue, _ := dimensionValues[0].(string)
			fieldName, ok := accountLocalIntentAliases[strings.TrimSpace(dimensionValue)]
			if !ok {
				continue
			}
			value, ok := compactMetricValue(result["value"])
			if !ok {
				continue
			}
			summary[fieldName] = value
		}
	}
	return summary
}

func nestedMetricTotal(metric map[string]any) (any, bool) {
	totalValue, ok := metric["total_value"].(map[string]any)
	if !ok {
		return nil, false
	}
	return compactMetricValue(totalValue["value"])
}

func compactMetricValue(raw any) (any, bool) {
	switch typed := raw.(type) {
	case nil:
		return nil, false
	case int:
		return int64(typed), true
	case int8:
		return int64(typed), true
	case int16:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case float32:
		return compactFloat(float64(typed)), true
	case float64:
		return compactFloat(typed), true
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil, false
		}
		if value, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return value, true
		}
		if value, err := strconv.ParseFloat(trimmed, 64); err == nil {
			return compactFloat(value), true
		}
		return trimmed, true
	default:
		return typed, true
	}
}

func compactFloat(value float64) any {
	if value == float64(int64(value)) {
		return int64(value)
	}
	return value
}
