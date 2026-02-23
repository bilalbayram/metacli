package cmd

import (
	"errors"
	"strings"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
	"github.com/bilalbayram/metacli/internal/insights"
	"github.com/bilalbayram/metacli/internal/output"
	"github.com/spf13/cobra"
)

func NewInsightsCommand(runtime Runtime) *cobra.Command {
	insightsCmd := &cobra.Command{
		Use:   "insights",
		Short: "Insights reporting commands",
	}
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
				return errors.New("account id is required")
			}

			creds, err := loadProfileCredentials(profile)
			if err != nil {
				return err
			}
			if version == "" {
				version = creds.Profile.GraphVersion
			}
			if version == "" {
				version = config.DefaultGraphVersion
			}

			client := graph.NewClient(nil, "")
			service := insights.New(client)
			result, err := service.Run(cmd.Context(), version, creds.Token, creds.AppSecret, insights.RunOptions{
				AccountID:   accountID,
				Level:       level,
				DatePreset:  datePreset,
				Breakdowns:  csvToSlice(breakdowns),
				Attribution: csvToSlice(attribution),
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
	cmd.Flags().StringVar(&format, "format", "jsonl", "Export format: csv|jsonl")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	return cmd
}
