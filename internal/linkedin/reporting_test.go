package linkedin

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestValidateReportingInputRejectsInvalidPackMetricCombo(t *testing.T) {
	dr := DateRange{
		Start: DateValue{Year: 2026, Month: 1, Day: 1},
		End:   DateValue{Year: 2026, Month: 1, Day: 31},
	}
	err := ValidateReportingInput(ReportingInput{
		AccountURNs: []URN{"urn:li:sponsoredAccount:123"},
		Level:       LevelCampaign,
		MetricPack:  PackLeadGen,
		Metrics:     []Metric{MetricLeads, MetricVideoViews},
		DateRange:   &dr,
	})
	if err == nil || !strings.Contains(err.Error(), "not allowed in pack") {
		t.Fatalf("expected pack validation failure, got %v", err)
	}
}

func TestValidateReportingInputRejectsInvalidPivotCombination(t *testing.T) {
	dr := DateRange{
		Start: DateValue{Year: 2026, Month: 1, Day: 1},
		End:   DateValue{Year: 2026, Month: 1, Day: 31},
	}
	err := ValidateReportingInput(ReportingInput{
		AccountURNs: []URN{"urn:li:sponsoredAccount:123"},
		Level:       LevelCreative,
		Metrics:     []Metric{MetricImpressions},
		Pivot:       PivotMemberCompany,
		DateRange:   &dr,
	})
	if err == nil || !strings.Contains(err.Error(), "not supported at creative level") {
		t.Fatalf("expected pivot validation failure, got %v", err)
	}
}

func TestValidateDateRange(t *testing.T) {
	err := ValidateDateRange(DateRange{
		Start: DateValue{Year: 2026, Month: 3, Day: 10},
		End:   DateValue{Year: 2026, Month: 3, Day: 9},
	})
	if err == nil {
		t.Fatal("expected invalid date range")
	}
}

func TestReportingServiceBuildsAnalyticsQuery(t *testing.T) {
	httpClient := &recordingHTTPClient{
		t: t,
		responses: []*http.Response{
			responseJSON(http.StatusOK, `{"elements":[{"pivotValues":["urn:li:sponsoredAccount:123"],"dateRange":{"start":{"year":2026,"month":3,"day":1},"end":{"year":2026,"month":3,"day":1}},"impressions":10,"clicks":2}],"paging":{"count":50,"start":50,"total":120}}`),
		},
	}
	client := NewClient(httpClient, "https://api.linkedin.com", "202402", "token-123")
	service := &ReportingService{Client: client}

	dr := DateRange{
		Start: DateValue{Year: 2026, Month: 3, Day: 1},
		End:   DateValue{Year: 2026, Month: 3, Day: 31},
	}
	result, err := service.Run(context.Background(), ReportingInput{
		AccountURNs:     []URN{"urn:li:sponsoredAccount:123"},
		Level:           LevelAccount,
		MetricPack:      PackBasic,
		Pivot:           PivotAccount,
		DateRange:       &dr,
		TimeGranularity: GranularityDaily,
		PageSize:        50,
		PageToken:       "50",
	})
	if err != nil {
		t.Fatalf("run reporting: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("unexpected row count %d", len(result.Rows))
	}
	if result.Paging == nil || result.Paging.NextPageToken != "100" {
		t.Fatalf("unexpected paging %#v", result.Paging)
	}

	req := httpClient.requests[0]
	if got := req.URL.Path; got != "/rest/adAnalytics" {
		t.Fatalf("unexpected path %q", got)
	}
	query := req.URL.Query()
	if got := query.Get("q"); got != string(QueryAnalytics) {
		t.Fatalf("unexpected q %q", got)
	}
	if got := query.Get("accounts"); got != "List(urn:li:sponsoredAccount:123)" {
		t.Fatalf("unexpected accounts query %q", got)
	}
	if got := query.Get("pivot"); got != string(PivotAccount) {
		t.Fatalf("unexpected pivot query %q", got)
	}
	if got := query.Get("fields"); got != "" {
		t.Fatalf("expected basic pack to omit fields, got %q", got)
	}
	if got := query.Get("dateRange"); !strings.Contains(got, "start:(year:2026,month:3,day:1)") {
		t.Fatalf("unexpected dateRange query %q", got)
	}
	if got := query.Get("count"); got != "50" {
		t.Fatalf("unexpected count %q", got)
	}
	if got := query.Get("start"); got != "50" {
		t.Fatalf("unexpected start %q", got)
	}
}

func TestReportingServiceUsesStatisticsForMultiPivotRuns(t *testing.T) {
	httpClient := &recordingHTTPClient{
		t: t,
		responses: []*http.Response{
			responseJSON(http.StatusOK, `{"elements":[{"pivotValues":["urn:li:organization:1"],"impressions":10}],"paging":{"count":1}}`),
		},
	}
	client := NewClient(httpClient, "https://api.linkedin.com", "202402", "token-123")
	service := &ReportingService{Client: client}

	dr := DateRange{
		Start: DateValue{Year: 2026, Month: 3, Day: 1},
		End:   DateValue{Year: 2026, Month: 3, Day: 31},
	}
	_, err := service.Run(context.Background(), ReportingInput{
		QueryKind:       QueryDemographic,
		AccountURNs:     []URN{"urn:li:sponsoredAccount:123"},
		Level:           LevelAccount,
		MetricPack:      PackDelivery,
		Pivots:          []Pivot{PivotMemberCompany, PivotMemberIndustry},
		DateRange:       &dr,
		TimeGranularity: GranularityDaily,
	})
	if err != nil {
		t.Fatalf("run reporting: %v", err)
	}

	req := httpClient.requests[0]
	if got := req.URL.Query().Get("q"); got != "statistics" {
		t.Fatalf("unexpected q %q", got)
	}
	if got := req.URL.Query().Get("pivots"); got != "List(MEMBER_COMPANY,MEMBER_INDUSTRY)" {
		t.Fatalf("unexpected pivots %q", got)
	}
	if got := req.URL.Query().Get("pivot"); got != "" {
		t.Fatalf("unexpected single pivot %q", got)
	}
	if got := req.URL.Query().Get("fields"); strings.Contains(got, "clicks") {
		t.Fatalf("explicit fields must not contain clicks: %q", got)
	}
}

func TestMetricPackMetricsReflectBasicDefaultMode(t *testing.T) {
	pack, err := NormalizeMetricPack("basic")
	if err != nil {
		t.Fatalf("normalize pack: %v", err)
	}
	metrics, err := MetricPackMetrics(pack)
	if err != nil {
		t.Fatalf("pack metrics: %v", err)
	}
	if len(metrics) != 0 {
		t.Fatalf("expected basic pack to omit explicit fields, got %v", metrics)
	}
}

func TestDemographicCaveatsReturnWarnings(t *testing.T) {
	warnings := DemographicCaveats(ReportingInput{
		Pivots: []Pivot{PivotMemberIndustry},
	})
	if len(warnings) == 0 {
		t.Fatal("expected demographic caveats")
	}
}

func TestTruncationWarning(t *testing.T) {
	warning := TruncationWarning(10, 10, "/rest/adAnalytics?pageToken=abc")
	if warning == nil || warning.Code != "truncation" {
		t.Fatalf("unexpected warning %#v", warning)
	}
}

func TestNormalizeMetricPackAndMetrics(t *testing.T) {
	pack, err := NormalizeMetricPack("delivery")
	if err != nil {
		t.Fatalf("normalize pack: %v", err)
	}
	metrics, err := MetricPackMetrics(pack)
	if err != nil {
		t.Fatalf("pack metrics: %v", err)
	}
	if len(metrics) == 0 {
		t.Fatal("expected pack metrics")
	}
}
