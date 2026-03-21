package ig

import (
	"context"
	"net/http"
	"net/url"
	"reflect"
	"testing"

	"github.com/bilalbayram/metacli/internal/graph"
)

func TestAccountInsightsShapesRequestAndPreservesRawMetrics(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"data":[{"name":"profile_views","period":"day","total_value":{"value":5918}}],"paging":{"next":"https://graph.example.com/v25.0/178414/insights?after=cursor_1"}}`,
	}
	service := New(graph.NewClient(stub, "https://graph.example.com"))

	result, err := service.AccountInsights(context.Background(), "v25.0", "token", "secret", AccountInsightsOptions{
		IGUserID:   "17841401876639191",
		Metrics:    []string{"profile_views"},
		Period:     "day",
		MetricType: "total_value",
		Since:      "2026-03-14",
		Until:      "2026-03-21",
	})
	if err != nil {
		t.Fatalf("account insights: %v", err)
	}

	parsedURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if parsedURL.Path != "/v25.0/17841401876639191/insights" {
		t.Fatalf("unexpected request path %q", parsedURL.Path)
	}
	query := parsedURL.Query()
	if got := query.Get("metric"); got != "profile_views" {
		t.Fatalf("unexpected metric query %q", got)
	}
	if got := query.Get("period"); got != "day" {
		t.Fatalf("unexpected period query %q", got)
	}
	if got := query.Get("metric_type"); got != "total_value" {
		t.Fatalf("unexpected metric_type query %q", got)
	}
	if got := query.Get("since"); got != "2026-03-14" {
		t.Fatalf("unexpected since query %q", got)
	}
	if got := query.Get("until"); got != "2026-03-21" {
		t.Fatalf("unexpected until query %q", got)
	}
	if len(result.RawMetrics) != 1 {
		t.Fatalf("expected 1 raw metric, got %d", len(result.RawMetrics))
	}
	if got := result.RawMetrics[0]["name"]; got != "profile_views" {
		t.Fatalf("unexpected raw metric name %#v", got)
	}
	if result.Pagination == nil || result.Pagination.Next == "" {
		t.Fatalf("expected pagination next link, got %#v", result.Pagination)
	}
}

func TestMediaListShapesRequest(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"data":[{"id":"1","media_type":"IMAGE"}]}`,
	}
	service := New(graph.NewClient(stub, "https://graph.example.com"))

	result, err := service.MediaList(context.Background(), "v25.0", "token", "secret", MediaListOptions{
		IGUserID: "17841401876639191",
		Limit:    25,
	})
	if err != nil {
		t.Fatalf("media list: %v", err)
	}

	parsedURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if parsedURL.Path != "/v25.0/17841401876639191/media" {
		t.Fatalf("unexpected request path %q", parsedURL.Path)
	}
	if got := parsedURL.Query().Get("fields"); got != defaultMediaListFields {
		t.Fatalf("unexpected fields query %q", got)
	}
	if len(result.Media) != 1 {
		t.Fatalf("expected 1 media item, got %d", len(result.Media))
	}
}

func TestMediaInsightsGroupsRawMetricsByMediaID(t *testing.T) {
	t.Parallel()

	client := &sequenceHTTPClient{
		t: t,
		responses: []sequenceStubResponse{
			{statusCode: http.StatusOK, response: `{"data":[{"name":"profile_visits","values":[{"value":2}]}]}`},
			{statusCode: http.StatusOK, response: `{"data":[{"name":"profile_visits","values":[{"value":5}]}]}`},
		},
	}
	service := New(graph.NewClient(client, "https://graph.example.com"))

	rows, err := service.MediaInsights(context.Background(), "v25.0", "token", "secret", MediaInsightsOptions{
		MediaIDs: []string{"17939913222153455", "17939913222153456"},
		Metrics:  []string{"profile_visits"},
		Period:   "lifetime",
	})
	if err != nil {
		t.Fatalf("media insights: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 media insight rows, got %d", len(rows))
	}
	if rows[0].MediaID != "17939913222153455" || rows[1].MediaID != "17939913222153456" {
		t.Fatalf("unexpected media ids %#v", rows)
	}
	if got := rows[0].RawMetrics[0]["name"]; got != "profile_visits" {
		t.Fatalf("unexpected first raw metric %#v", got)
	}
}

func TestBuildAccountLocalIntentReportNormalizesSupportedBreakdowns(t *testing.T) {
	t.Parallel()

	rawMetrics := []map[string]any{
		{
			"name": "profile_links_taps",
			"total_value": map[string]any{
				"breakdowns": []any{
					map[string]any{
						"dimension_keys": []any{"contact_button_type"},
						"results": []any{
							map[string]any{"dimension_values": []any{"CALL"}, "value": 56},
							map[string]any{"dimension_values": []any{"DIRECTION"}, "value": 286},
							map[string]any{"dimension_values": []any{"EMAIL"}, "value": 7},
							map[string]any{"dimension_values": []any{"TEXT"}, "value": 3},
							map[string]any{"dimension_values": []any{"BOOK_NOW"}, "value": 2},
						},
					},
				},
			},
		},
		{
			"name": "profile_views",
			"total_value": map[string]any{
				"value": 5918,
			},
		},
	}

	report := BuildAccountLocalIntentReport(rawMetrics)
	if !reflect.DeepEqual(report.RawMetrics, rawMetrics) {
		t.Fatalf("expected raw metrics to be preserved, got %#v", report.RawMetrics)
	}
	if got := report.Summary["calls"]; got != int64(56) {
		t.Fatalf("unexpected calls summary %#v", got)
	}
	if got := report.Summary["directions"]; got != int64(286) {
		t.Fatalf("unexpected directions summary %#v", got)
	}
	if got := report.Summary["email_contacts"]; got != int64(7) {
		t.Fatalf("unexpected email_contacts summary %#v", got)
	}
	if got := report.Summary["text_contacts"]; got != int64(3) {
		t.Fatalf("unexpected text_contacts summary %#v", got)
	}
	if got := report.Summary["book_now"]; got != int64(2) {
		t.Fatalf("unexpected book_now summary %#v", got)
	}
	if got := report.Summary["profile_views"]; got != int64(5918) {
		t.Fatalf("unexpected profile_views summary %#v", got)
	}
}
