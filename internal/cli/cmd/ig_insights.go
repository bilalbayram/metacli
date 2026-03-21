package cmd

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/ig"
	"github.com/bilalbayram/metacli/internal/insights"
	"github.com/bilalbayram/metacli/internal/plugin"
	"github.com/spf13/cobra"
)

var igInsightsNow = time.Now

func newIGInsightsCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	insightsCmd := &cobra.Command{
		Use:   "insights",
		Short: "Instagram account and media insights commands",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return requireSubcommand(cmd, "ig insights")
		},
	}
	insightsCmd.AddCommand(newIGInsightsAccountCommand(runtime, pluginRuntime))
	insightsCmd.AddCommand(newIGInsightsMediaCommand(runtime, pluginRuntime))
	insightsCmd.AddCommand(newIGInsightsCombinedLocalIntentCommand(runtime, pluginRuntime))
	return insightsCmd
}

func newIGInsightsAccountCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	accountCmd := &cobra.Command{
		Use:   "account",
		Short: "Instagram account insights commands",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return requireSubcommand(cmd, "ig insights account")
		},
	}
	accountCmd.AddCommand(newIGInsightsAccountRunCommand(runtime, pluginRuntime))
	accountCmd.AddCommand(newIGInsightsAccountLocalIntentCommand(runtime, pluginRuntime))
	return accountCmd
}

func newIGInsightsMediaCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	mediaCmd := &cobra.Command{
		Use:   "media",
		Short: "Instagram media insights commands",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return requireSubcommand(cmd, "ig insights media")
		},
	}
	mediaCmd.AddCommand(newIGInsightsMediaListCommand(runtime, pluginRuntime))
	mediaCmd.AddCommand(newIGInsightsMediaRunCommand(runtime, pluginRuntime))
	return mediaCmd
}

func newIGInsightsAccountRunCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	var (
		profile    string
		version    string
		igUserID   string
		metrics    []string
		period     string
		metricType string
		breakdown  string
		since      string
		until      string
		timeframe  string
		format     string
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Fetch raw Instagram account insights",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  igPluginID,
				Namespace: igNamespace,
				Command:   "insights-account-run",
			}); err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights account run", err)
			}

			creds, resolvedVersion, err := resolveIGProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights account run", err)
			}
			resolvedIGUserID, err := requireResolvedIGUserID(igUserID, creds.Profile)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights account run", err)
			}
			format, err = normalizeInsightsFormat(format)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights account run", err)
			}

			service := ig.New(igNewGraphClient())
			result, err := service.AccountInsights(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, ig.AccountInsightsOptions{
				IGUserID:   resolvedIGUserID,
				Metrics:    metrics,
				Period:     period,
				MetricType: metricType,
				Breakdown:  breakdown,
				Since:      since,
				Until:      until,
				Timeframe:  timeframe,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights account run", err)
			}

			return writeInsightsOutput(cmd, "meta ig insights account run", format, result.RawMetrics, result.Pagination)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&igUserID, "ig-user-id", "", "Instagram user id (optional when profile has ig_user_id)")
	cmd.Flags().StringSliceVar(&metrics, "metric", nil, "Metric name(s); repeat the flag or pass a comma-separated list")
	cmd.Flags().StringVar(&period, "period", "", "Metric period")
	cmd.Flags().StringVar(&metricType, "metric-type", "", "Metric type")
	cmd.Flags().StringVar(&breakdown, "breakdown", "", "Metric breakdown")
	cmd.Flags().StringVar(&since, "since", "", "Start date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&until, "until", "", "End date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&timeframe, "timeframe", "", "Timeframe value")
	cmd.Flags().StringVar(&format, "format", "json", "Export format: json|jsonl|csv")
	return cmd
}

func newIGInsightsAccountLocalIntentCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	var (
		profile    string
		version    string
		igUserID   string
		datePreset string
		since      string
		until      string
		timeframe  string
		format     string
	)

	cmd := &cobra.Command{
		Use:   "local-intent",
		Short: "Fetch Instagram account local-intent insights",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  igPluginID,
				Namespace: igNamespace,
				Command:   "insights-account-local-intent",
			}); err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights account local-intent", err)
			}

			creds, resolvedVersion, err := resolveIGProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights account local-intent", err)
			}
			resolvedIGUserID, err := requireResolvedIGUserID(igUserID, creds.Profile)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights account local-intent", err)
			}
			format, err = normalizeInsightsFormat(format)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights account local-intent", err)
			}

			resolvedSince, resolvedUntil, err := resolveIGInsightsRange(since, until, datePreset, "")
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights account local-intent", err)
			}

			service := ig.New(igNewGraphClient())
			rawMetrics, err := fetchIGAccountLocalIntentRawMetrics(cmd, service, resolvedVersion, creds.Token, creds.AppSecret, resolvedIGUserID, resolvedSince, resolvedUntil, timeframe)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights account local-intent", err)
			}

			report := ig.BuildAccountLocalIntentReport(rawMetrics)
			data := map[string]any{
				"summary":     report.Summary,
				"raw_metrics": report.RawMetrics,
			}
			return writeInsightsOutput(cmd, "meta ig insights account local-intent", format, data, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&igUserID, "ig-user-id", "", "Instagram user id (optional when profile has ig_user_id)")
	cmd.Flags().StringVar(&datePreset, "date-preset", "", "Resolve a common date preset into --since/--until")
	cmd.Flags().StringVar(&since, "since", "", "Start date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&until, "until", "", "End date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&timeframe, "timeframe", "", "Timeframe value")
	cmd.Flags().StringVar(&format, "format", "json", "Export format: json|jsonl|csv")
	return cmd
}

func newIGInsightsMediaListCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	var (
		profile  string
		version  string
		igUserID string
		limit    int
		format   string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Instagram media for insights discovery",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  igPluginID,
				Namespace: igNamespace,
				Command:   "insights-media-list",
			}); err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights media list", err)
			}

			creds, resolvedVersion, err := resolveIGProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights media list", err)
			}
			resolvedIGUserID, err := requireResolvedIGUserID(igUserID, creds.Profile)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights media list", err)
			}
			format, err = normalizeInsightsFormat(format)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights media list", err)
			}

			service := ig.New(igNewGraphClient())
			result, err := service.MediaList(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, ig.MediaListOptions{
				IGUserID: resolvedIGUserID,
				Limit:    limit,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights media list", err)
			}

			return writeInsightsOutput(cmd, "meta ig insights media list", format, result.Media, result.Pagination)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&igUserID, "ig-user-id", "", "Instagram user id (optional when profile has ig_user_id)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Limit total media rows returned")
	cmd.Flags().StringVar(&format, "format", "json", "Export format: json|jsonl|csv")
	return cmd
}

func newIGInsightsMediaRunCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	var (
		profile    string
		version    string
		mediaIDs   []string
		metrics    []string
		period     string
		metricType string
		breakdown  string
		since      string
		until      string
		timeframe  string
		format     string
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Fetch raw Instagram media insights",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  igPluginID,
				Namespace: igNamespace,
				Command:   "insights-media-run",
			}); err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights media run", err)
			}

			creds, resolvedVersion, err := resolveIGProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights media run", err)
			}
			format, err = normalizeInsightsFormat(format)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights media run", err)
			}

			service := ig.New(igNewGraphClient())
			rows, err := service.MediaInsights(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, ig.MediaInsightsOptions{
				MediaIDs:   mediaIDs,
				Metrics:    metrics,
				Period:     period,
				MetricType: metricType,
				Breakdown:  breakdown,
				Since:      since,
				Until:      until,
				Timeframe:  timeframe,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights media run", err)
			}

			return writeInsightsOutput(cmd, "meta ig insights media run", format, mediaInsightsRowsToMaps(rows), nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringSliceVar(&mediaIDs, "media-id", nil, "Instagram media id(s); repeat the flag or pass a comma-separated list")
	cmd.Flags().StringSliceVar(&metrics, "metric", nil, "Metric name(s); repeat the flag or pass a comma-separated list")
	cmd.Flags().StringVar(&period, "period", "", "Metric period")
	cmd.Flags().StringVar(&metricType, "metric-type", "", "Metric type")
	cmd.Flags().StringVar(&breakdown, "breakdown", "", "Metric breakdown")
	cmd.Flags().StringVar(&since, "since", "", "Start date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&until, "until", "", "End date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&timeframe, "timeframe", "", "Timeframe value")
	cmd.Flags().StringVar(&format, "format", "json", "Export format: json|jsonl|csv")
	return cmd
}

func newIGInsightsCombinedLocalIntentCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	var (
		profile    string
		version    string
		accountID  string
		igUserID   string
		datePreset string
		since      string
		until      string
		format     string
	)

	cmd := &cobra.Command{
		Use:   "local-intent",
		Short: "Combine paid Instagram and Instagram account local-intent insights",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  igPluginID,
				Namespace: igNamespace,
				Command:   "insights-local-intent",
			}); err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights local-intent", err)
			}
			if strings.TrimSpace(accountID) == "" {
				return writeCommandError(cmd, runtime, "meta ig insights local-intent", errors.New("account id is required (--account-id)"))
			}

			creds, resolvedVersion, err := resolveIGProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights local-intent", err)
			}
			resolvedIGUserID, err := requireResolvedIGUserID(igUserID, creds.Profile)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights local-intent", err)
			}
			format, err = normalizeInsightsFormat(format)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights local-intent", err)
			}

			resolvedSince, resolvedUntil, err := resolveIGInsightsRange(since, until, datePreset, "last_7d")
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights local-intent", err)
			}

			paidService := insights.New(igNewGraphClient())
			paidResult, err := paidService.Run(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, insights.RunOptions{
				AccountID:         accountID,
				Level:             "account",
				Since:             resolvedSince,
				Until:             resolvedUntil,
				Fields:            insightsFieldsForMetricPack("local_intent"),
				PublisherPlatform: "instagram",
			})
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights local-intent", err)
			}
			paidRows := insights.NormalizeLocalIntentRows(paidResult.Rows)
			paidSummary := insights.SummarizeLocalIntentRows(paidRows)

			igService := ig.New(igNewGraphClient())
			accountRawMetrics, err := fetchIGAccountLocalIntentRawMetrics(cmd, igService, resolvedVersion, creds.Token, creds.AppSecret, resolvedIGUserID, resolvedSince, resolvedUntil, "")
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig insights local-intent", err)
			}
			accountReport := ig.BuildAccountLocalIntentReport(accountRawMetrics)

			data := map[string]any{
				"range": map[string]any{
					"since": resolvedSince,
					"until": resolvedUntil,
				},
				"summary": buildCombinedSummary(paidSummary, accountReport.Summary),
				"paid_instagram": map[string]any{
					"summary": paidSummary,
					"rows":    paidRows,
				},
				"instagram_account": map[string]any{
					"summary":     accountReport.Summary,
					"raw_metrics": accountReport.RawMetrics,
				},
			}
			return writeInsightsOutput(cmd, "meta ig insights local-intent", format, data, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&accountID, "account-id", "", "Ad account id without act_ prefix")
	cmd.Flags().StringVar(&igUserID, "ig-user-id", "", "Instagram user id (optional when profile has ig_user_id)")
	cmd.Flags().StringVar(&datePreset, "date-preset", "last_7d", "Resolve a common date preset into --since/--until")
	cmd.Flags().StringVar(&since, "since", "", "Start date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&until, "until", "", "End date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&format, "format", "json", "Export format: json|jsonl|csv")
	return cmd
}

func requireResolvedIGUserID(flagValue string, profile config.Profile) (string, error) {
	resolved := resolveIGUserID(flagValue, profile)
	if resolved == "" {
		return "", errors.New("ig user id is required (--ig-user-id or profile ig_user_id)")
	}
	return resolved, nil
}

func fetchIGAccountLocalIntentRawMetrics(cmd *cobra.Command, service *ig.Service, version string, token string, appSecret string, igUserID string, since string, until string, timeframe string) ([]map[string]any, error) {
	links, err := service.AccountInsights(cmd.Context(), version, token, appSecret, ig.AccountInsightsOptions{
		IGUserID:   igUserID,
		Metrics:    []string{"profile_links_taps"},
		Period:     "day",
		MetricType: "total_value",
		Breakdown:  "contact_button_type",
		Since:      since,
		Until:      until,
		Timeframe:  timeframe,
	})
	if err != nil {
		return nil, err
	}
	views, err := service.AccountInsights(cmd.Context(), version, token, appSecret, ig.AccountInsightsOptions{
		IGUserID:   igUserID,
		Metrics:    []string{"profile_views"},
		Period:     "day",
		MetricType: "total_value",
		Since:      since,
		Until:      until,
		Timeframe:  timeframe,
	})
	if err != nil {
		return nil, err
	}

	rawMetrics := make([]map[string]any, 0, len(links.RawMetrics)+len(views.RawMetrics))
	rawMetrics = append(rawMetrics, links.RawMetrics...)
	rawMetrics = append(rawMetrics, views.RawMetrics...)
	return rawMetrics, nil
}

func resolveIGInsightsRange(since string, until string, datePreset string, fallbackPreset string) (string, string, error) {
	trimmedSince := strings.TrimSpace(since)
	trimmedUntil := strings.TrimSpace(until)
	if trimmedSince != "" || trimmedUntil != "" {
		if trimmedSince == "" || trimmedUntil == "" {
			return "", "", errors.New("both --since and --until are required when either is set")
		}
		return trimmedSince, trimmedUntil, nil
	}

	preset := strings.TrimSpace(datePreset)
	if preset == "" {
		preset = strings.TrimSpace(fallbackPreset)
	}
	if preset == "" {
		return "", "", nil
	}
	return resolveDatePresetRange(preset, igInsightsNow().In(time.Local))
}

func resolveDatePresetRange(datePreset string, now time.Time) (string, string, error) {
	normalized := strings.ToLower(strings.TrimSpace(datePreset))
	if normalized == "" {
		return "", "", errors.New("date preset is required")
	}

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch normalized {
	case "today":
		return formatDate(today), formatDate(today), nil
	case "yesterday":
		yesterday := today.AddDate(0, 0, -1)
		return formatDate(yesterday), formatDate(yesterday), nil
	case "this_month":
		start := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location())
		return formatDate(start), formatDate(today), nil
	case "last_month":
		start := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location()).AddDate(0, -1, 0)
		end := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location()).AddDate(0, 0, -1)
		return formatDate(start), formatDate(end), nil
	}

	matches := lastNDaysPattern.FindStringSubmatch(normalized)
	if len(matches) == 2 {
		days, err := strconv.Atoi(matches[1])
		if err != nil || days <= 0 {
			return "", "", fmt.Errorf("unsupported date preset %q", datePreset)
		}
		start := today.AddDate(0, 0, -days)
		end := today.AddDate(0, 0, -1)
		return formatDate(start), formatDate(end), nil
	}

	return "", "", fmt.Errorf("unsupported --date-preset value %q; use --since/--until for exact ranges", datePreset)
}

var lastNDaysPattern = regexp.MustCompile(`^last_(\d+)d$`)

func formatDate(value time.Time) string {
	return value.Format("2006-01-02")
}

func buildCombinedSummary(paidSummary map[string]any, accountSummary map[string]any) map[string]any {
	fields := make([]string, 0, len(paidSummary)+len(accountSummary))
	seen := make(map[string]struct{}, len(paidSummary)+len(accountSummary))
	for key := range paidSummary {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		fields = append(fields, key)
	}
	for key := range accountSummary {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		fields = append(fields, key)
	}
	sort.Strings(fields)

	summary := make(map[string]any, len(fields))
	for _, field := range fields {
		entry := make(map[string]any)
		total := 0.0
		hasTotal := false
		if value, ok := paidSummary[field]; ok {
			entry["paid_instagram"] = value
			if numeric, ok := summaryNumericValue(value); ok {
				total += numeric
				hasTotal = true
			}
		}
		if value, ok := accountSummary[field]; ok {
			entry["instagram_account"] = value
			if numeric, ok := summaryNumericValue(value); ok {
				total += numeric
				hasTotal = true
			}
		}
		if hasTotal {
			entry["combined"] = compactSummaryNumeric(total)
		}
		summary[field] = entry
	}
	return summary
}

func summaryNumericValue(raw any) (float64, bool) {
	switch typed := raw.(type) {
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float32:
		return float64(typed), true
	case float64:
		return typed, true
	default:
		return 0, false
	}
}

func compactSummaryNumeric(value float64) any {
	if value == float64(int64(value)) {
		return int64(value)
	}
	return value
}

func mediaInsightsRowsToMaps(rows []ig.MediaInsightsRow) []map[string]any {
	mapped := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		mapped = append(mapped, map[string]any{
			"media_id":    row.MediaID,
			"raw_metrics": row.RawMetrics,
		})
	}
	return mapped
}
