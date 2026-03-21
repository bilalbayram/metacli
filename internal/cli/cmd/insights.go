package cmd

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
	"github.com/bilalbayram/metacli/internal/insights"
	"github.com/bilalbayram/metacli/internal/output"
	"github.com/spf13/cobra"
)

var (
	insightsLoadProfileCredentials = loadProfileCredentials
	insightsNewGraphClient         = func() *graph.Client {
		return graph.NewClient(nil, "")
	}
	insightsNewService = insights.New
)

var insightsQualityMetricPackFields = []string{
	"account_id",
	"campaign_id",
	"campaign_name",
	"adset_id",
	"adset_name",
	"ad_id",
	"ad_name",
	"date_start",
	"date_stop",
	"spend",
	"impressions",
	"clicks",
	"ctr",
	"cpc",
	"cpm",
	"reach",
	"frequency",
	"actions",
	"cost_per_action_type",
	"purchase_roas",
	"outbound_clicks",
	"outbound_clicks_ctr",
}

var insightsLocalIntentMetricPackFields = []string{
	"account_id",
	"campaign_id",
	"campaign_name",
	"adset_id",
	"adset_name",
	"ad_id",
	"ad_name",
	"date_start",
	"date_stop",
	"actions",
	"cost_per_action_type",
}

var insightsActionDiscoveryFields = []string{
	"actions",
	"cost_per_action_type",
}

const insightsAccountsListFields = "id,account_id,name,account_status,currency,timezone_name"

func NewInsightsCommand(runtime Runtime) *cobra.Command {
	insightsCmd := &cobra.Command{
		Use:   "insights",
		Short: "Insights reporting commands",
	}
	insightsCmd.AddCommand(newInsightsAccountsCommand(runtime))
	insightsCmd.AddCommand(newInsightsRunCommand(runtime))
	insightsCmd.AddCommand(newInsightsActionTypesCommand(runtime))
	return insightsCmd
}

func newInsightsRunCommand(runtime Runtime) *cobra.Command {
	var (
		profile           string
		accountID         string
		level             string
		datePreset        string
		breakdowns        string
		attribution       string
		publisherPlatform string
		limit             int
		async             bool
		format            string
		metricPack        string
		version           string
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run an insights query",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if profile == "" {
				profile = runtime.ProfileName()
			}
			if profile == "" {
				return errors.New("profile is required (--profile or global --profile)")
			}
			if accountID == "" {
				return missingInsightsAccountIDError(profile)
			}
			level, err := normalizeInsightsLevel(level)
			if err != nil {
				return err
			}

			metricPack, err = normalizeInsightsMetricPack(metricPack)
			if err != nil {
				return err
			}
			fields := insightsFieldsForMetricPack(metricPack)
			format, err = normalizeInsightsFormat(format)
			if err != nil {
				return err
			}

			creds, err := insightsLoadProfileCredentials(profile)
			if err != nil {
				return err
			}
			if version == "" {
				version = creds.Profile.GraphVersion
			}
			if version == "" {
				version = config.DefaultGraphVersion
			}

			client := insightsNewGraphClient()
			service := insightsNewService(client)
			result, err := service.Run(cmd.Context(), version, creds.Token, creds.AppSecret, insights.RunOptions{
				AccountID:         accountID,
				Level:             level,
				DatePreset:        datePreset,
				Breakdowns:        csvToSlice(breakdowns),
				Attribution:       csvToSlice(attribution),
				Fields:            fields,
				Limit:             limit,
				Async:             async,
				PublisherPlatform: strings.ToLower(strings.TrimSpace(publisherPlatform)),
			})
			if err != nil {
				return err
			}
			if metricPack == "local_intent" {
				result.Rows = insights.NormalizeLocalIntentRows(result.Rows)
			}

			return writeInsightsOutput(cmd, "meta insights run", format, result.Rows, result.Pagination)
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&accountID, "account-id", "", "Ad account id without act_ prefix")
	cmd.Flags().StringVar(&level, "level", "campaign", "Insights level: account|campaign|adset|ad")
	cmd.Flags().StringVar(&datePreset, "date-preset", "last_7d", "Date preset (for example last_7d)")
	cmd.Flags().StringVar(&breakdowns, "breakdowns", "", "Comma-separated breakdowns")
	cmd.Flags().StringVar(&attribution, "attribution", "", "Comma-separated action attribution windows")
	cmd.Flags().StringVar(&publisherPlatform, "publisher-platform", "", "Filter insight rows to a publisher platform (for example instagram)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Limit total rows returned")
	cmd.Flags().BoolVar(&async, "async", false, "Run insights asynchronously")
	cmd.Flags().StringVar(&metricPack, "metric-pack", "basic", "Metric pack: basic|quality|local_intent")
	cmd.Flags().StringVar(&format, "format", "jsonl", "Export format: json|jsonl|csv")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	return cmd
}

func newInsightsActionTypesCommand(runtime Runtime) *cobra.Command {
	var (
		profile           string
		accountID         string
		level             string
		datePreset        string
		attribution       string
		publisherPlatform string
		limit             int
		async             bool
		format            string
		version           string
	)

	cmd := &cobra.Command{
		Use:   "action-types",
		Short: "Discover raw action types returned by insights",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if profile == "" {
				profile = runtime.ProfileName()
			}
			if profile == "" {
				return errors.New("profile is required (--profile or global --profile)")
			}
			if accountID == "" {
				return missingInsightsAccountIDError(profile)
			}

			var err error
			level, err = normalizeInsightsLevel(level)
			if err != nil {
				return err
			}
			format, err = normalizeInsightsFormat(format)
			if err != nil {
				return err
			}

			creds, err := insightsLoadProfileCredentials(profile)
			if err != nil {
				return err
			}
			if version == "" {
				version = creds.Profile.GraphVersion
			}
			if version == "" {
				version = config.DefaultGraphVersion
			}

			client := insightsNewGraphClient()
			service := insightsNewService(client)
			result, err := service.Run(cmd.Context(), version, creds.Token, creds.AppSecret, insights.RunOptions{
				AccountID:         accountID,
				Level:             level,
				DatePreset:        datePreset,
				Attribution:       csvToSlice(attribution),
				Fields:            insightsActionDiscoveryFields,
				Limit:             limit,
				Async:             async,
				PublisherPlatform: strings.ToLower(strings.TrimSpace(publisherPlatform)),
			})
			if err != nil {
				return err
			}

			return writeInsightsOutput(cmd, "meta insights action-types", format, insights.DiscoverActionTypes(result.Rows), result.Pagination)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&accountID, "account-id", "", "Ad account id without act_ prefix")
	cmd.Flags().StringVar(&level, "level", "ad", "Insights level: account|campaign|adset|ad")
	cmd.Flags().StringVar(&datePreset, "date-preset", "last_30d", "Date preset (for example last_30d)")
	cmd.Flags().StringVar(&attribution, "attribution", "", "Comma-separated action attribution windows")
	cmd.Flags().StringVar(&publisherPlatform, "publisher-platform", "", "Filter insight rows to a publisher platform (for example instagram)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Limit total rows returned before aggregation")
	cmd.Flags().BoolVar(&async, "async", false, "Run insights asynchronously")
	cmd.Flags().StringVar(&format, "format", "json", "Export format: json|jsonl|csv")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	return cmd
}

func newInsightsAccountsCommand(runtime Runtime) *cobra.Command {
	accountsCmd := &cobra.Command{
		Use:   "accounts",
		Short: "Ad account discovery for insights",
	}
	accountsCmd.AddCommand(newInsightsAccountsListCommand(runtime))
	return accountsCmd
}

func newInsightsAccountsListCommand(runtime Runtime) *cobra.Command {
	var (
		profile    string
		activeOnly bool
		version    string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available ad accounts",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if profile == "" {
				profile = runtime.ProfileName()
			}
			if profile == "" {
				return errors.New("profile is required (--profile or global --profile)")
			}

			creds, err := insightsLoadProfileCredentials(profile)
			if err != nil {
				return err
			}
			if version == "" {
				version = creds.Profile.GraphVersion
			}
			if version == "" {
				version = config.DefaultGraphVersion
			}

			items := make([]map[string]any, 0)
			client := insightsNewGraphClient()
			pagination, err := client.FetchWithPagination(cmd.Context(), graph.Request{
				Method:  "GET",
				Path:    "me/adaccounts",
				Version: version,
				Query: map[string]string{
					"fields": insightsAccountsListFields,
				},
				AccessToken: creds.Token,
				AppSecret:   creds.AppSecret,
			}, graph.PaginationOptions{
				FollowNext: true,
			}, func(item map[string]any) error {
				items = append(items, item)
				return nil
			})
			if err != nil {
				return err
			}

			if activeOnly {
				items = filterActiveAdAccounts(items)
			}

			return writeSuccess(cmd, runtime, "meta insights accounts list", items, pagination, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().BoolVar(&activeOnly, "active-only", false, "Show only active ad accounts (account_status=1)")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	return cmd
}

func missingInsightsAccountIDError(profile string) error {
	suggestion := "meta insights accounts list --active-only"
	if strings.TrimSpace(profile) != "" {
		suggestion = fmt.Sprintf("%s --profile %s", suggestion, profile)
	}
	return fmt.Errorf("account id is required (--account-id). discover active accounts with: %s", suggestion)
}

func insightsFieldsForMetricPack(metricPack string) []string {
	switch metricPack {
	case "quality":
		return append([]string(nil), insightsQualityMetricPackFields...)
	case "local_intent":
		return append([]string(nil), insightsLocalIntentMetricPackFields...)
	default:
		return nil
	}
}

func normalizeInsightsMetricPack(metricPack string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(metricPack))
	switch normalized {
	case "", "basic":
		return "basic", nil
	case "quality", "local_intent":
		return normalized, nil
	default:
		return "", errors.New("invalid --metric-pack value: expected basic|quality|local_intent")
	}
}

func normalizeInsightsLevel(level string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(level))
	switch normalized {
	case "account", "campaign", "adset", "ad":
		return normalized, nil
	default:
		return "", errors.New("invalid --level value: expected account|campaign|adset|ad")
	}
}

func normalizeInsightsFormat(format string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(format))
	switch normalized {
	case "json", "jsonl", "csv":
		return normalized, nil
	default:
		return "", errors.New("invalid --format value: expected json|jsonl|csv")
	}
}

func writeInsightsOutput(cmd *cobra.Command, commandName string, format string, data any, paging any) error {
	env, err := output.NewEnvelope(commandName, true, data, paging, nil, nil)
	if err != nil {
		return err
	}
	return output.Write(cmd.OutOrStdout(), format, env)
}

func filterActiveAdAccounts(items []map[string]any) []map[string]any {
	filtered := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if !isActiveAdAccountStatus(item["account_status"]) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func isActiveAdAccountStatus(raw any) bool {
	switch typed := raw.(type) {
	case int:
		return typed == 1
	case int8:
		return typed == 1
	case int16:
		return typed == 1
	case int32:
		return typed == 1
	case int64:
		return typed == 1
	case uint:
		return typed == 1
	case uint8:
		return typed == 1
	case uint16:
		return typed == 1
	case uint32:
		return typed == 1
	case uint64:
		return typed == 1
	case float32:
		return typed == 1
	case float64:
		return typed == 1
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return false
		}
		status, err := strconv.Atoi(trimmed)
		return err == nil && status == 1
	default:
		return false
	}
}
