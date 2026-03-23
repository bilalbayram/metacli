package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bilalbayram/metacli/internal/linkedin"
	"github.com/spf13/cobra"
)

func newLIInsightsRunCommand(runtime Runtime) *cobra.Command {
	return newLIInsightsRunLikeCommand(runtime, "meta li insights run", "Run LinkedIn ad analytics", linkedin.QueryAnalytics)
}

func newLIInsightsDemographicRunCommand(runtime Runtime) *cobra.Command {
	return newLIInsightsRunLikeCommand(runtime, "meta li insights demographic run", "Run LinkedIn demographic reporting", linkedin.QueryDemographic)
}

func newLIInsightsRunLikeCommand(runtime Runtime, commandName string, short string, kind linkedin.QueryKind) *cobra.Command {
	var (
		profile         string
		version         string
		accountURNsRaw  string
		level           string
		metricPack      string
		metricsRaw      string
		pivot           string
		pivotsRaw       string
		since           string
		until           string
		timeGranularity string
		pageSize        int
		pageToken       string
		limit           int
		followNext      bool
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, resolvedVersion, err := resolveLinkedInProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(version))
			}
			client, err := newLinkedInClient(creds, resolvedVersion)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
			}
			accountURNs, err := parseLinkedInURNs(accountURNsRaw, linkedin.NormalizeSponsoredAccountURN)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
			}
			dateRange, err := parseLinkedInDateRange(since, until)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
			}

			input := linkedin.ReportingInput{
				QueryKind:       kind,
				AccountURNs:     accountURNs,
				Level:           linkedin.ReportingLevel(strings.ToUpper(strings.TrimSpace(level))),
				MetricPack:      linkedin.MetricPack(strings.ToLower(strings.TrimSpace(metricPack))),
				Pivot:           linkedin.Pivot(strings.ToUpper(strings.TrimSpace(pivot))),
				Pivots:          parseLinkedInPivots(pivotsRaw),
				DateRange:       dateRange,
				TimeGranularity: linkedin.TimeGranularity(strings.ToUpper(strings.TrimSpace(timeGranularity))),
				PageSize:        pageSize,
				PageToken:       strings.TrimSpace(pageToken),
				Limit:           limit,
				FollowNext:      followNext,
			}
			metrics, err := linkedin.NormalizeMetrics(csvToSlice(metricsRaw))
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
			}
			input.Metrics = metrics

			result, err := (&linkedin.ReportingService{Client: client}).Run(cmd.Context(), input)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, commandName, result, result.Paging, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	cmd.Flags().StringVar(&accountURNsRaw, "account-urns", "", "Comma-separated sponsored account URNs or ids")
	cmd.Flags().StringVar(&level, "level", string(linkedin.LevelCampaign), "Reporting level: ACCOUNT|CAMPAIGN_GROUP|CAMPAIGN|CREATIVE")
	cmd.Flags().StringVar(&metricPack, "metric-pack", string(linkedin.PackBasic), "Metric pack: basic|delivery|leadgen|video|b2b")
	cmd.Flags().StringVar(&metricsRaw, "metrics", "", "Comma-separated metrics override")
	cmd.Flags().StringVar(&pivot, "pivot", "", "Primary pivot")
	cmd.Flags().StringVar(&pivotsRaw, "pivots", "", "Comma-separated pivots")
	cmd.Flags().StringVar(&since, "since", "", "Inclusive start date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&until, "until", "", "Inclusive end date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&timeGranularity, "time-granularity", string(linkedin.GranularityDaily), "ALL|DAILY|MONTHLY")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "Page size")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "Pagination token (numeric offset for LinkedIn analytics)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum rows to return")
	cmd.Flags().BoolVar(&followNext, "follow-next", false, "Follow pagination")
	mustMarkFlagRequired(cmd, "account-urns")
	mustMarkFlagRequired(cmd, "since")
	mustMarkFlagRequired(cmd, "until")
	return cmd
}

func newLIInsightsMetricsListCommand(runtime Runtime) *cobra.Command {
	var pack string
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List supported LinkedIn reporting metrics",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data := make([]map[string]any, 0)
			packs := []linkedin.MetricPack{linkedin.PackBasic, linkedin.PackDelivery, linkedin.PackLeadGen, linkedin.PackVideo, linkedin.PackB2B}
			for _, current := range packs {
				if strings.TrimSpace(pack) != "" && string(current) != strings.ToLower(strings.TrimSpace(pack)) {
					continue
				}
				if linkedin.MetricPackUsesDefaultFields(current) {
					defaultMetrics, err := linkedin.MetricPackDefaultMetrics(current)
					if err != nil {
						return writeCommandError(cmd, runtime, "meta li insights metrics list", err)
					}
					data = append(data, map[string]any{
						"metric_pack":      current,
						"metric":           "",
						"mode":             "default",
						"projected_fields": false,
						"default_metrics":  linkedInMetricStrings(defaultMetrics),
					})
					continue
				}
				metrics, err := linkedin.MetricPackMetrics(current)
				if err != nil {
					return writeCommandError(cmd, runtime, "meta li insights metrics list", err)
				}
				for _, metric := range metrics {
					data = append(data, map[string]any{
						"metric_pack":      current,
						"metric":           metric,
						"mode":             "fields",
						"projected_fields": true,
					})
				}
			}
			return writeSuccess(cmd, runtime, "meta li insights metrics list", data, nil, nil)
		},
	}
	listCmd.Flags().StringVar(&pack, "metric-pack", "", "Optional metric pack filter")

	cmd := newLISubcommandGroup("metrics", "LinkedIn reporting metric commands")
	cmd.AddCommand(listCmd)
	return cmd
}

func newLIInsightsPivotsListCommand(runtime Runtime) *cobra.Command {
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List supported LinkedIn reporting pivots",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pivots := []linkedin.Pivot{
				linkedin.PivotAccount,
				linkedin.PivotCampaignGroup,
				linkedin.PivotCampaign,
				linkedin.PivotCreative,
				linkedin.PivotDay,
				linkedin.PivotMonth,
				linkedin.PivotMemberCountry,
				linkedin.PivotMemberRegion,
				linkedin.PivotMemberSeniority,
				linkedin.PivotMemberCompany,
				linkedin.PivotMemberIndustry,
				linkedin.PivotMemberJobFunction,
			}
			rows := make([]map[string]any, 0, len(pivots))
			for _, pivot := range pivots {
				rows = append(rows, map[string]any{
					"pivot":       pivot,
					"demographic": strings.HasPrefix(string(pivot), "MEMBER_"),
				})
			}
			return writeSuccess(cmd, runtime, "meta li insights pivots list", rows, nil, nil)
		},
	}
	cmd := newLISubcommandGroup("pivots", "LinkedIn reporting pivot commands")
	cmd.AddCommand(listCmd)
	return cmd
}

func parseLinkedInURNs(raw string, normalize func(string) (linkedin.URN, error)) ([]linkedin.URN, error) {
	values := csvToSlice(raw)
	if len(values) == 0 {
		return nil, errors.New("at least one URN is required")
	}
	out := make([]linkedin.URN, 0, len(values))
	for _, value := range values {
		urn, err := normalize(value)
		if err != nil {
			return nil, err
		}
		out = append(out, urn)
	}
	return out, nil
}

func parseLinkedInPivots(raw string) []linkedin.Pivot {
	values := csvToSlice(raw)
	out := make([]linkedin.Pivot, 0, len(values))
	for _, value := range values {
		out = append(out, linkedin.Pivot(strings.ToUpper(strings.TrimSpace(value))))
	}
	return out
}

func parseLinkedInDateRange(since string, until string) (*linkedin.DateRange, error) {
	start, err := time.Parse("2006-01-02", strings.TrimSpace(since))
	if err != nil {
		return nil, fmt.Errorf("parse --since: %w", err)
	}
	end, err := time.Parse("2006-01-02", strings.TrimSpace(until))
	if err != nil {
		return nil, fmt.Errorf("parse --until: %w", err)
	}
	return &linkedin.DateRange{
		Start: linkedin.DateValue{Year: start.Year(), Month: int(start.Month()), Day: start.Day()},
		End:   linkedin.DateValue{Year: end.Year(), Month: int(end.Month()), Day: end.Day()},
	}, nil
}

func linkedInMetricStrings(metrics []linkedin.Metric) []string {
	out := make([]string, 0, len(metrics))
	for _, metric := range metrics {
		out = append(out, string(metric))
	}
	return out
}
