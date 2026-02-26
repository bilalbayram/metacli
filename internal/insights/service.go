package insights

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bilalbayram/metacli/internal/graph"
)

type RunOptions struct {
	AccountID   string
	Level       string
	DatePreset  string
	Breakdowns  []string
	Attribution []string
	Fields      []string
	Limit       int
	Async       bool
}

type Result struct {
	Rows        []map[string]any        `json:"rows"`
	Pagination  *graph.PaginationResult `json:"pagination,omitempty"`
	ReportRunID string                  `json:"report_run_id,omitempty"`
}

type Service struct {
	Client          *graph.Client
	PollInterval    time.Duration
	MaxPollAttempts int
	Sleep           func(time.Duration)
}

func New(client *graph.Client) *Service {
	if client == nil {
		client = graph.NewClient(nil, "")
	}
	return &Service{
		Client:          client,
		PollInterval:    2 * time.Second,
		MaxPollAttempts: 60,
		Sleep:           time.Sleep,
	}
}

func (s *Service) Run(ctx context.Context, version string, token string, appSecret string, options RunOptions) (*Result, error) {
	if strings.TrimSpace(options.AccountID) == "" {
		return nil, errors.New("account id is required")
	}
	if strings.TrimSpace(options.Level) == "" {
		return nil, errors.New("insights level is required")
	}
	if strings.TrimSpace(options.DatePreset) == "" {
		return nil, errors.New("date preset is required")
	}
	params := map[string]string{
		"level":       options.Level,
		"date_preset": options.DatePreset,
	}
	if len(options.Breakdowns) > 0 {
		params["breakdowns"] = strings.Join(options.Breakdowns, ",")
	}
	if len(options.Attribution) > 0 {
		params["action_attribution_windows"] = strings.Join(options.Attribution, ",")
	}
	if len(options.Fields) > 0 {
		normalizedFields, err := normalizeFields(options.Fields)
		if err != nil {
			return nil, err
		}
		params["fields"] = strings.Join(normalizedFields, ",")
	}
	if options.Limit > 0 {
		params["limit"] = strconv.Itoa(options.Limit)
	}

	path := fmt.Sprintf("act_%s/insights", options.AccountID)
	if !options.Async {
		return s.fetchInsights(ctx, version, path, token, appSecret, params, options.Limit)
	}

	runID, err := s.startAsyncRun(ctx, version, path, token, appSecret, params)
	if err != nil {
		return nil, err
	}
	if err := s.waitForRun(ctx, version, runID, token, appSecret); err != nil {
		return nil, err
	}
	result, err := s.fetchInsights(ctx, version, fmt.Sprintf("%s/insights", runID), token, appSecret, params, options.Limit)
	if err != nil {
		return nil, err
	}
	result.ReportRunID = runID
	return result, nil
}

func (s *Service) startAsyncRun(ctx context.Context, version string, path string, token string, appSecret string, params map[string]string) (string, error) {
	form := map[string]string{}
	for key, value := range params {
		form[key] = value
	}
	form["async"] = "true"

	resp, err := s.Client.Do(ctx, graph.Request{
		Method:      "POST",
		Path:        path,
		Version:     version,
		Form:        form,
		AccessToken: token,
		AppSecret:   appSecret,
	})
	if err != nil {
		return "", err
	}
	runID, _ := resp.Body["report_run_id"].(string)
	if strings.TrimSpace(runID) == "" {
		return "", errors.New("async insights response did not include report_run_id")
	}
	return runID, nil
}

func (s *Service) waitForRun(ctx context.Context, version string, runID string, token string, appSecret string) error {
	for attempt := 1; attempt <= s.MaxPollAttempts; attempt++ {
		resp, err := s.Client.Do(ctx, graph.Request{
			Method:  "GET",
			Path:    runID,
			Version: version,
			Query: map[string]string{
				"fields": "async_status,async_percent_completion",
			},
			AccessToken: token,
			AppSecret:   appSecret,
		})
		if err != nil {
			return err
		}
		status, _ := resp.Body["async_status"].(string)
		if isCompleted(status) {
			return nil
		}
		if strings.Contains(strings.ToLower(status), "fail") {
			return fmt.Errorf("async insights run %s failed with status %q", runID, status)
		}
		s.Sleep(s.PollInterval)
	}
	return fmt.Errorf("async insights run %s did not complete after %d attempts", runID, s.MaxPollAttempts)
}

func (s *Service) fetchInsights(ctx context.Context, version string, path string, token string, appSecret string, params map[string]string, limit int) (*Result, error) {
	items := make([]map[string]any, 0)
	pagination, err := s.Client.FetchWithPagination(ctx, graph.Request{
		Method:      "GET",
		Path:        path,
		Version:     version,
		Query:       params,
		AccessToken: token,
		AppSecret:   appSecret,
	}, graph.PaginationOptions{
		FollowNext: true,
		Limit:      limit,
	}, func(item map[string]any) error {
		items = append(items, item)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &Result{
		Rows:       items,
		Pagination: pagination,
	}, nil
}

func isCompleted(status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	return status == "job completed" || status == "completed"
}

func normalizeFields(fields []string) ([]string, error) {
	normalized := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		if trimmed == "" {
			return nil, errors.New("insights fields contains blank entries")
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return nil, errors.New("insights fields are required when fields filter is set")
	}
	return normalized, nil
}
