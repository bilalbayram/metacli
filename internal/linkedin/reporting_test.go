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
		Metrics:     []Metric{MetricClicks},
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
			responseJSON(http.StatusOK, `{"elements":[{"pivotValues":["urn:li:sponsoredAccount:123"],"impressions":10}],"paging":{"count":1}}`),
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
		Level:           LevelCampaign,
		MetricPack:      PackBasic,
		Pivot:           PivotCampaign,
		DateRange:       &dr,
		TimeGranularity: GranularityDaily,
		PageSize:        50,
	})
	if err != nil {
		t.Fatalf("run reporting: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("unexpected row count %d", len(result.Rows))
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
	if got := query.Get("pivot"); got != string(PivotCampaign) {
		t.Fatalf("unexpected pivot query %q", got)
	}
	if got := query.Get("fields"); got != "dateRange,pivotValues,impressions,clicks" {
		t.Fatalf("unexpected fields query %q", got)
	}
	if got := req.URL.RawQuery; !strings.Contains(got, "fields=dateRange,pivotValues,impressions,clicks") {
		t.Fatalf("unexpected raw query %q", got)
	}
	if got := req.URL.RawQuery; strings.Contains(got, "fields=dateRange%2CpivotValues%2Cimpressions%2Cclicks") {
		t.Fatalf("fields projection should not escape commas: %q", got)
	}
	if got := query.Get("dateRange"); !strings.Contains(got, "start:(year:2026,month:3,day:1)") {
		t.Fatalf("unexpected dateRange query %q", got)
	}
	if got := query.Get("pageSize"); got != "50" {
		t.Fatalf("unexpected pageSize %q", got)
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
	pack, err := NormalizeMetricPack("basic")
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
