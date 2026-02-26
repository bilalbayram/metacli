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

const insightsAccountsListFields = "id,account_id,name,account_status,currency,timezone_name"

func NewInsightsCommand(runtime Runtime) *cobra.Command {
	insightsCmd := &cobra.Command{
		Use:   "insights",
		Short: "Insights reporting commands",
	}
	insightsCmd.AddCommand(newInsightsAccountsCommand(runtime))
	insightsCmd.AddCommand(newInsightsRunCommand(runtime))
	return insightsCmd
}

func newInsightsRunCommand(runtime Runtime) *cobra.Command {
	var (
		profile     string
		accountID   string
		level       string
		datePreset  string
		breakdowns  string
		attribution string
		limit       int
		async       bool
		format      string
		metricPack  string
		version     string
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

			fields, err := insightsFieldsForMetricPack(metricPack)
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
				AccountID:   accountID,
				Level:       level,
				DatePreset:  datePreset,
				Breakdowns:  csvToSlice(breakdowns),
				Attribution: csvToSlice(attribution),
				Fields:      fields,
				Limit:       limit,
				Async:       async,
			})
			if err != nil {
				return err
			}

			env, err := output.NewEnvelope("meta insights run", true, result.Rows, result.Pagination, nil, nil)
			if err != nil {
				return err
			}
			switch strings.ToLower(strings.TrimSpace(format)) {
			case "jsonl", "csv":
				return output.Write(cmd.OutOrStdout(), format, env)
			default:
				return errors.New("invalid --format value: expected csv|jsonl")
			}
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&accountID, "account-id", "", "Ad account id without act_ prefix")
	cmd.Flags().StringVar(&level, "level", "campaign", "Insights level: campaign|adset|ad")
	cmd.Flags().StringVar(&datePreset, "date-preset", "last_7d", "Date preset (for example last_7d)")
	cmd.Flags().StringVar(&breakdowns, "breakdowns", "", "Comma-separated breakdowns")
	cmd.Flags().StringVar(&attribution, "attribution", "", "Comma-separated action attribution windows")
	cmd.Flags().IntVar(&limit, "limit", 0, "Limit total rows returned")
	cmd.Flags().BoolVar(&async, "async", false, "Run insights asynchronously")
	cmd.Flags().StringVar(&metricPack, "metric-pack", "basic", "Metric pack: basic|quality")
	cmd.Flags().StringVar(&format, "format", "jsonl", "Export format: csv|jsonl")
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

func insightsFieldsForMetricPack(metricPack string) ([]string, error) {
	normalized := strings.ToLower(strings.TrimSpace(metricPack))
	switch normalized {
	case "", "basic":
		return nil, nil
	case "quality":
		return append([]string(nil), insightsQualityMetricPackFields...), nil
	default:
		return nil, errors.New("invalid --metric-pack value: expected basic|quality")
	}
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
