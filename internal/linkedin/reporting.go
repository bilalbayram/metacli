package linkedin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Metric string
type MetricPack string
type Pivot string
type ReportingLevel string
type TimeGranularity string
type QueryKind string

const (
	MetricImpressions         Metric = "impressions"
	MetricClicks              Metric = "clicks"
	MetricSpend               Metric = "spend"
	MetricCPC                 Metric = "cpc"
	MetricCPM                 Metric = "cpm"
	MetricCTR                 Metric = "ctr"
	MetricReach               Metric = "reach"
	MetricFrequency           Metric = "frequency"
	MetricVideoViews          Metric = "videoViews"
	MetricVideoStarts         Metric = "videoStarts"
	MetricVideoCompletions    Metric = "videoCompletions"
	MetricVideoCompletionRate Metric = "videoCompletionRate"
	MetricLeads               Metric = "leads"
	MetricLeadFormOpens       Metric = "leadFormOpens"
	MetricCostPerLead         Metric = "costPerLead"
	MetricCompanyClicks       Metric = "companyClicks"
	MetricMemberCompany       Metric = "memberCompany"
	MetricMemberSeniority     Metric = "memberSeniority"
	MetricMemberJobFunction   Metric = "memberJobFunction"

	PackBasic    MetricPack = "basic"
	PackDelivery MetricPack = "delivery"
	PackLeadGen  MetricPack = "leadgen"
	PackVideo    MetricPack = "video"
	PackB2B      MetricPack = "b2b"

	PivotAccount           Pivot = "ACCOUNT"
	PivotCampaignGroup     Pivot = "CAMPAIGN_GROUP"
	PivotCampaign          Pivot = "CAMPAIGN"
	PivotCreative          Pivot = "CREATIVE"
	PivotDay               Pivot = "DAY"
	PivotMonth             Pivot = "MONTH"
	PivotMemberCountry     Pivot = "MEMBER_COUNTRY"
	PivotMemberRegion      Pivot = "MEMBER_REGION"
	PivotMemberSeniority   Pivot = "MEMBER_SENIORITY"
	PivotMemberCompany     Pivot = "MEMBER_COMPANY"
	PivotMemberIndustry    Pivot = "MEMBER_INDUSTRY"
	PivotMemberJobFunction Pivot = "MEMBER_JOB_FUNCTION"

	LevelAccount       ReportingLevel = "ACCOUNT"
	LevelCampaignGroup ReportingLevel = "CAMPAIGN_GROUP"
	LevelCampaign      ReportingLevel = "CAMPAIGN"
	LevelCreative      ReportingLevel = "CREATIVE"

	GranularityAll     TimeGranularity = "ALL"
	GranularityDaily   TimeGranularity = "DAILY"
	GranularityMonthly TimeGranularity = "MONTHLY"

	QueryAnalytics   QueryKind = "analytics"
	QueryDemographic QueryKind = "demographic"
)

type DateValue struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
}

type DateRange struct {
	Start DateValue `json:"start"`
	End   DateValue `json:"end"`
}

type ReportingInput struct {
	QueryKind       QueryKind       `json:"query_kind,omitempty"`
	AccountURNs     []URN           `json:"account_urns"`
	Level           ReportingLevel  `json:"level"`
	MetricPack      MetricPack      `json:"metric_pack,omitempty"`
	Metrics         []Metric        `json:"metrics,omitempty"`
	Pivot           Pivot           `json:"pivot,omitempty"`
	Pivots          []Pivot         `json:"pivots,omitempty"`
	DateRange       *DateRange      `json:"date_range,omitempty"`
	TimeGranularity TimeGranularity `json:"time_granularity,omitempty"`
	PageSize        int             `json:"page_size,omitempty"`
	PageToken       string          `json:"page_token,omitempty"`
	Limit           int             `json:"limit,omitempty"`
	FollowNext      bool            `json:"follow_next,omitempty"`
}

type ReportingWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Limit   int    `json:"limit,omitempty"`
	Rows    int    `json:"rows,omitempty"`
	More    bool   `json:"more,omitempty"`
}

type ReportingResult struct {
	Rows     []map[string]any   `json:"rows"`
	Paging   *PagingInfo        `json:"paging,omitempty"`
	Warnings []ReportingWarning `json:"warnings,omitempty"`
}

type ReportingService struct {
	Client *Client
}

var metricPackDefinitions = map[MetricPack][]Metric{
	PackBasic: {
		MetricImpressions,
		MetricClicks,
		MetricSpend,
		MetricCPC,
		MetricCPM,
		MetricCTR,
		MetricReach,
		MetricFrequency,
	},
	PackDelivery: {
		MetricImpressions,
		MetricClicks,
		MetricSpend,
		MetricCPC,
		MetricCPM,
		MetricCTR,
		MetricReach,
		MetricFrequency,
	},
	PackLeadGen: {
		MetricImpressions,
		MetricClicks,
		MetricSpend,
		MetricLeads,
		MetricLeadFormOpens,
		MetricCostPerLead,
	},
	PackVideo: {
		MetricImpressions,
		MetricClicks,
		MetricSpend,
		MetricVideoViews,
		MetricVideoStarts,
		MetricVideoCompletions,
		MetricVideoCompletionRate,
	},
	PackB2B: {
		MetricImpressions,
		MetricClicks,
		MetricSpend,
		MetricCompanyClicks,
		MetricMemberCompany,
		MetricMemberSeniority,
		MetricMemberJobFunction,
	},
}

var demographicPivots = map[Pivot]struct{}{
	PivotMemberCountry:     {},
	PivotMemberRegion:      {},
	PivotMemberSeniority:   {},
	PivotMemberCompany:     {},
	PivotMemberIndustry:    {},
	PivotMemberJobFunction: {},
}

var creativeUnsafeLeadGenMetrics = map[Metric]struct{}{
	MetricLeads:         {},
	MetricLeadFormOpens: {},
	MetricCostPerLead:   {},
}

func NormalizeMetricPack(raw string) (MetricPack, error) {
	pack := MetricPack(strings.ToLower(strings.TrimSpace(raw)))
	if pack == "" {
		return "", errors.New("metric pack is required")
	}
	if _, ok := metricPackDefinitions[pack]; !ok {
		return "", fmt.Errorf("unsupported metric pack %q", raw)
	}
	return pack, nil
}

func MetricPackMetrics(pack MetricPack) ([]Metric, error) {
	metrics, ok := metricPackDefinitions[pack]
	if !ok {
		return nil, fmt.Errorf("unsupported metric pack %q", pack)
	}
	out := make([]Metric, len(metrics))
	copy(out, metrics)
	return out, nil
}

func NormalizeMetrics(metrics []string) ([]Metric, error) {
	out := make([]Metric, 0, len(metrics))
	seen := map[Metric]struct{}{}
	for _, raw := range metrics {
		metric := Metric(strings.TrimSpace(raw))
		if metric == "" {
			return nil, errors.New("metrics contains blank entries")
		}
		if _, ok := seen[metric]; ok {
			continue
		}
		seen[metric] = struct{}{}
		out = append(out, metric)
	}
	return out, nil
}

func NormalizePivots(values []string) ([]Pivot, error) {
	out := make([]Pivot, 0, len(values))
	seen := map[Pivot]struct{}{}
	for _, raw := range values {
		pivot := Pivot(strings.ToUpper(strings.TrimSpace(raw)))
		if pivot == "" {
			return nil, errors.New("pivots contains blank entries")
		}
		if _, ok := seen[pivot]; ok {
			continue
		}
		seen[pivot] = struct{}{}
		out = append(out, pivot)
	}
	return out, nil
}

func ValidateDateRange(r DateRange) error {
	if !validDate(r.Start) {
		return errors.New("start date is invalid")
	}
	if !validDate(r.End) {
		return errors.New("end date is invalid")
	}
	start := time.Date(r.Start.Year, time.Month(r.Start.Month), r.Start.Day, 0, 0, 0, 0, time.UTC)
	end := time.Date(r.End.Year, time.Month(r.End.Month), r.End.Day, 0, 0, 0, 0, time.UTC)
	if end.Before(start) {
		return errors.New("date range end must not be before start")
	}
	return nil
}

func (r DateRange) Encode() string {
	return fmt.Sprintf("(start:(year:%d,month:%d,day:%d),end:(year:%d,month:%d,day:%d))", r.Start.Year, r.Start.Month, r.Start.Day, r.End.Year, r.End.Month, r.End.Day)
}

func ValidateReportingInput(input ReportingInput) error {
	if len(input.AccountURNs) == 0 {
		return errors.New("at least one account urn is required")
	}
	if err := ValidateReportingLevel(input.Level); err != nil {
		return err
	}
	if input.DateRange == nil {
		return errors.New("date range is required")
	}
	if err := ValidateDateRange(*input.DateRange); err != nil {
		return err
	}

	pack := input.MetricPack
	if pack != "" {
		normalized, err := NormalizeMetricPack(string(pack))
		if err != nil {
			return err
		}
		pack = normalized
	}

	metrics, err := NormalizeMetrics(metricsToStrings(input.Metrics))
	if err != nil {
		return err
	}
	if pack != "" {
		packMetrics, _ := MetricPackMetrics(pack)
		if len(metrics) == 0 {
			metrics = packMetrics
		} else {
			allowed := metricSet(packMetrics)
			for _, metric := range metrics {
				if _, ok := allowed[metric]; !ok {
					return fmt.Errorf("metric %q is not allowed in pack %q", metric, pack)
				}
			}
		}
	}

	pivots, err := NormalizePivots(pivotsToStrings(input.Pivots))
	if err != nil {
		return err
	}
	if input.Pivot != "" {
		pivots = append([]Pivot{Pivot(strings.ToUpper(strings.TrimSpace(string(input.Pivot))))}, pivots...)
	}
	pivots = dedupePivots(pivots)
	if len(pivots) > 1 && input.QueryKind != QueryDemographic {
		return errors.New("only one pivot is supported for analytics queries")
	}
	if input.Level == LevelCreative {
		for _, pivot := range pivots {
			if _, ok := demographicPivots[pivot]; ok {
				return fmt.Errorf("pivot %q is not supported at creative level", pivot)
			}
		}
	}
	for _, metric := range metrics {
		if _, ok := creativeUnsafeLeadGenMetrics[metric]; ok {
			for _, pivot := range pivots {
				if pivot == PivotCreative {
					return fmt.Errorf("metric %q cannot be used with pivot %q", metric, pivot)
				}
			}
		}
	}

	if input.QueryKind == QueryDemographic {
		if len(demographicWarnings(pivots)) == 0 {
			return errors.New("demographic queries require at least one demographic pivot")
		}
	}
	return nil
}

func ValidateReportingLevel(level ReportingLevel) error {
	switch level {
	case LevelAccount, LevelCampaignGroup, LevelCampaign, LevelCreative:
		return nil
	default:
		return fmt.Errorf("unsupported reporting level %q", level)
	}
}

func DemographicCaveats(input ReportingInput) []ReportingWarning {
	warnings := demographicWarnings(dedupedPivotsFromInput(input))
	if len(warnings) == 0 {
		return nil
	}
	out := make([]ReportingWarning, 0, len(warnings)+1)
	out = append(out, warnings...)
	out = append(out, ReportingWarning{
		Code:    "demographic_threshold",
		Message: "LinkedIn demographic reporting is subject to privacy thresholds and may omit sparse slices.",
	})
	return out
}

func TruncationWarning(limit int, rows int, nextPageURL string) *ReportingWarning {
	if limit <= 0 || rows < limit || strings.TrimSpace(nextPageURL) == "" {
		return nil
	}
	return &ReportingWarning{
		Code:    "truncation",
		Message: "reporting output may be truncated because additional pages are available and follow-next is disabled.",
		Limit:   limit,
		Rows:    rows,
		More:    true,
	}
}

func (s *ReportingService) Run(ctx context.Context, input ReportingInput) (*ReportingResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("reporting client is required")
	}
	if err := ValidateReportingInput(input); err != nil {
		return nil, err
	}

	metrics, _ := normalizeReportingMetrics(input)
	pivots := dedupedPivotsFromInput(input)
	if len(pivots) == 0 && input.QueryKind != QueryDemographic {
		pivots = []Pivot{Pivot(strings.ToUpper(string(input.Level)))}
	}

	query := map[string]string{}
	query["q"] = string(defaultQueryKind(input.QueryKind))
	query["dateRange"] = input.DateRange.Encode()
	if input.TimeGranularity != "" {
		query["timeGranularity"] = strings.ToUpper(string(input.TimeGranularity))
	}
	query["accounts"] = listValue(urnStrings(input.AccountURNs)...)
	if len(metrics) > 0 {
		query["fields"] = strings.Join(metricsToStrings(metrics), ",")
	}
	if len(pivots) > 0 {
		if input.QueryKind == QueryDemographic {
			query["pivots"] = listValue(pivotStrings(pivots)...)
		} else {
			query["pivot"] = string(pivots[0])
		}
	}
	if input.PageSize > 0 {
		query[DefaultPageSizeParam] = fmt.Sprintf("%d", input.PageSize)
	}
	if strings.TrimSpace(input.PageToken) != "" {
		query[DefaultPageTokenParam] = strings.TrimSpace(input.PageToken)
	}

	result := &ReportingResult{Rows: make([]map[string]any, 0)}
	paging, err := s.Client.FetchCollection(ctx, Request{
		Method:  http.MethodGet,
		Path:    "/rest/adAnalytics",
		Version: s.Client.Version,
		Query:   query,
	}, PaginationOptions{
		FollowNext: input.FollowNext,
		Limit:      input.Limit,
		PageSize:   input.PageSize,
		PageToken:  strings.TrimSpace(input.PageToken),
	}, func(row map[string]any) error {
		result.Rows = append(result.Rows, row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	result.Paging = paging
	if warning := TruncationWarning(input.Limit, len(result.Rows), nextPageToken(paging)); warning != nil {
		result.Warnings = append(result.Warnings, *warning)
	}
	result.Warnings = append(result.Warnings, DemographicCaveats(input)...)
	return result, nil
}

func normalizeReportingMetrics(input ReportingInput) ([]Metric, error) {
	metrics := append([]Metric(nil), input.Metrics...)
	if len(metrics) == 0 && input.MetricPack != "" {
		pack, err := NormalizeMetricPack(string(input.MetricPack))
		if err != nil {
			return nil, err
		}
		def, err := MetricPackMetrics(pack)
		if err != nil {
			return nil, err
		}
		metrics = def
	}
	return metrics, nil
}

func defaultQueryKind(kind QueryKind) QueryKind {
	if strings.TrimSpace(string(kind)) == "" {
		return QueryAnalytics
	}
	return kind
}

func metricsToStrings(metrics []Metric) []string {
	out := make([]string, 0, len(metrics))
	for _, metric := range metrics {
		out = append(out, string(metric))
	}
	return out
}

func pivotsToStrings(pivots []Pivot) []string {
	out := make([]string, 0, len(pivots))
	for _, pivot := range pivots {
		out = append(out, string(pivot))
	}
	return out
}

func pivotStrings(pivots []Pivot) []string {
	return pivotsToStrings(pivots)
}

func urnStrings(urns []URN) []string {
	out := make([]string, 0, len(urns))
	for _, urn := range urns {
		out = append(out, urn.String())
	}
	return out
}

func listValue(values ...string) string {
	values = compactStrings(values)
	return "List(" + strings.Join(values, ",") + ")"
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func dedupePivots(values []Pivot) []Pivot {
	out := make([]Pivot, 0, len(values))
	seen := map[Pivot]struct{}{}
	for _, pivot := range values {
		pivot = Pivot(strings.ToUpper(strings.TrimSpace(string(pivot))))
		if pivot == "" {
			continue
		}
		if _, ok := seen[pivot]; ok {
			continue
		}
		seen[pivot] = struct{}{}
		out = append(out, pivot)
	}
	return out
}

func dedupedPivotsFromInput(input ReportingInput) []Pivot {
	pivots := make([]Pivot, 0, len(input.Pivots)+1)
	if input.Pivot != "" {
		pivots = append(pivots, Pivot(strings.ToUpper(strings.TrimSpace(string(input.Pivot)))))
	}
	pivots = append(pivots, input.Pivots...)
	return dedupePivots(pivots)
}

func metricSet(metrics []Metric) map[Metric]struct{} {
	out := make(map[Metric]struct{}, len(metrics))
	for _, metric := range metrics {
		out[metric] = struct{}{}
	}
	return out
}

func validDate(value DateValue) bool {
	if value.Year <= 0 || value.Month <= 0 || value.Month > 12 || value.Day <= 0 || value.Day > 31 {
		return false
	}
	t := time.Date(value.Year, time.Month(value.Month), value.Day, 0, 0, 0, 0, time.UTC)
	return t.Year() == value.Year && int(t.Month()) == value.Month && t.Day() == value.Day
}

func demographicWarnings(pivots []Pivot) []ReportingWarning {
	for _, pivot := range pivots {
		if _, ok := demographicPivots[pivot]; ok {
			return []ReportingWarning{
				{
					Code:    "demographic_threshold",
					Message: "LinkedIn demographic reporting can suppress sparse groups to protect member privacy.",
				},
				{
					Code:    "demographic_latency",
					Message: "Demographic reporting may lag behind delivery reporting by several hours.",
				},
			}
		}
	}
	return nil
}

func nextPageToken(paging *PagingInfo) string {
	if paging == nil {
		return ""
	}
	return strings.TrimSpace(paging.NextPageToken)
}
