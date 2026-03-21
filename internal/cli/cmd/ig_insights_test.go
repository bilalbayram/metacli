package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
)

func TestNewIGCommandIncludesInsightsSubcommands(t *testing.T) {
	t.Parallel()

	cmd := NewIGCommand(Runtime{})

	insightsCmd, _, err := cmd.Find([]string{"insights"})
	if err != nil {
		t.Fatalf("find insights command: %v", err)
	}
	if insightsCmd == nil || insightsCmd.Name() != "insights" {
		t.Fatalf("expected insights command, got %#v", insightsCmd)
	}

	accountRunCmd, _, err := cmd.Find([]string{"insights", "account", "run"})
	if err != nil {
		t.Fatalf("find account run command: %v", err)
	}
	if accountRunCmd == nil || accountRunCmd.Name() != "run" {
		t.Fatalf("expected account run command, got %#v", accountRunCmd)
	}

	accountLocalIntentCmd, _, err := cmd.Find([]string{"insights", "account", "local-intent"})
	if err != nil {
		t.Fatalf("find account local-intent command: %v", err)
	}
	if accountLocalIntentCmd == nil || accountLocalIntentCmd.Name() != "local-intent" {
		t.Fatalf("expected account local-intent command, got %#v", accountLocalIntentCmd)
	}

	mediaListCmd, _, err := cmd.Find([]string{"insights", "media", "list"})
	if err != nil {
		t.Fatalf("find media list command: %v", err)
	}
	if mediaListCmd == nil || mediaListCmd.Name() != "list" {
		t.Fatalf("expected media list command, got %#v", mediaListCmd)
	}

	combinedCmd, _, err := cmd.Find([]string{"insights", "local-intent"})
	if err != nil {
		t.Fatalf("find combined local-intent command: %v", err)
	}
	if combinedCmd == nil || combinedCmd.Name() != "local-intent" {
		t.Fatalf("expected combined local-intent command, got %#v", combinedCmd)
	}
}

func TestIGInsightsAccountRunExecutesShapedRequest(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"data":[{"name":"profile_views","period":"day","total_value":{"value":5918}}]}`,
	}
	useIGDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name:      "prod",
				Profile:   config.Profile{GraphVersion: "v25.0", IGUserID: "17841401876639191"},
				Token:     "test-token",
				AppSecret: "test-secret",
			}, nil
		},
		func() *graph.Client {
			client := graph.NewClient(stub, "https://graph.example.com")
			client.MaxRetries = 0
			return client
		},
	)

	output := &bytes.Buffer{}
	cmd := NewIGCommand(testRuntime("prod"))
	cmd.SetOut(output)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"insights", "account", "run",
		"--metric", "profile_views",
		"--period", "day",
		"--metric-type", "total_value",
		"--since", "2026-03-14",
		"--until", "2026-03-21",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute ig insights account run: %v", err)
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

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta ig insights account run")
	data, ok := envelope["data"].([]any)
	if !ok || len(data) != 1 {
		t.Fatalf("expected one raw metric, got %#v", envelope["data"])
	}
}

func TestIGInsightsAccountLocalIntentReturnsNormalizedSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch metric := r.URL.Query().Get("metric"); metric {
		case "profile_links_taps":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{
						"name": "profile_links_taps",
						"total_value": map[string]any{
							"breakdowns": []map[string]any{
								{
									"dimension_keys": []string{"contact_button_type"},
									"results": []map[string]any{
										{"dimension_values": []string{"CALL"}, "value": 56},
										{"dimension_values": []string{"DIRECTION"}, "value": 286},
									},
								},
							},
						},
					},
				},
			})
		case "profile_views":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{
						"name":        "profile_views",
						"total_value": map[string]any{"value": 5918},
					},
				},
			})
		default:
			t.Fatalf("unexpected metric query %q", metric)
		}
	}))
	defer server.Close()

	useIGDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name:      "prod",
				Profile:   config.Profile{GraphVersion: "v25.0", IGUserID: "17841401876639191"},
				Token:     "test-token",
				AppSecret: "test-secret",
			}, nil
		},
		func() *graph.Client {
			client := graph.NewClient(server.Client(), server.URL)
			client.MaxRetries = 0
			return client
		},
	)

	output := &bytes.Buffer{}
	cmd := NewIGCommand(testRuntime("prod"))
	cmd.SetOut(output)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"insights", "account", "local-intent",
		"--since", "2026-03-14",
		"--until", "2026-03-21",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute ig insights account local-intent: %v", err)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta ig insights account local-intent")
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object payload, got %T", envelope["data"])
	}
	summary, ok := data["summary"].(map[string]any)
	if !ok {
		t.Fatalf("expected summary object, got %T", data["summary"])
	}
	if got := summary["calls"]; got != float64(56) {
		t.Fatalf("unexpected calls summary %#v", got)
	}
	if got := summary["directions"]; got != float64(286) {
		t.Fatalf("unexpected directions summary %#v", got)
	}
	if got := summary["profile_views"]; got != float64(5918) {
		t.Fatalf("unexpected profile_views summary %#v", got)
	}
	rawMetrics, ok := data["raw_metrics"].([]any)
	if !ok || len(rawMetrics) != 2 {
		t.Fatalf("expected two raw metrics, got %#v", data["raw_metrics"])
	}
}

func TestIGInsightsCombinedLocalIntentReturnsLayeredReport(t *testing.T) {
	originalNow := igInsightsNow
	igInsightsNow = func() time.Time {
		return time.Date(2026, 3, 21, 12, 0, 0, 0, time.FixedZone("UTC+3", 3*60*60))
	}
	t.Cleanup(func() {
		igInsightsNow = originalNow
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/act_123/insights"):
			if got := r.URL.Query().Get("breakdowns"); got != "publisher_platform" {
				t.Fatalf("unexpected paid breakdowns query %q", got)
			}
			if got := r.URL.Query().Get("time_range"); got != `{"since":"2026-03-14","until":"2026-03-20"}` {
				t.Fatalf("unexpected paid time_range query %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"publisher_platform": "facebook", "actions": []map[string]any{{"action_type": "click_to_call_native_call_placed", "value": "20"}}},
					{"publisher_platform": "instagram", "actions": []map[string]any{{"action_type": "click_to_call_native_call_placed", "value": "180"}}},
				},
			})
		case strings.HasSuffix(r.URL.Path, "/17841401876639191/insights") && r.URL.Query().Get("metric") == "profile_links_taps":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{
						"name": "profile_links_taps",
						"total_value": map[string]any{
							"breakdowns": []map[string]any{
								{
									"dimension_keys": []string{"contact_button_type"},
									"results": []map[string]any{
										{"dimension_values": []string{"CALL"}, "value": 55},
										{"dimension_values": []string{"DIRECTION"}, "value": 282},
									},
								},
							},
						},
					},
				},
			})
		case strings.HasSuffix(r.URL.Path, "/17841401876639191/insights") && r.URL.Query().Get("metric") == "profile_views":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{
						"name":        "profile_views",
						"total_value": map[string]any{"value": 5491},
					},
				},
			})
		default:
			t.Fatalf("unexpected request path=%q query=%q", r.URL.Path, r.URL.RawQuery)
		}
	}))
	defer server.Close()

	useIGDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name:      "prod",
				Profile:   config.Profile{GraphVersion: "v25.0", IGUserID: "17841401876639191"},
				Token:     "test-token",
				AppSecret: "test-secret",
			}, nil
		},
		func() *graph.Client {
			client := graph.NewClient(server.Client(), server.URL)
			client.MaxRetries = 0
			return client
		},
	)

	output := &bytes.Buffer{}
	cmd := NewIGCommand(testRuntime("prod"))
	cmd.SetOut(output)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"insights", "local-intent",
		"--account-id", "123",
		"--date-preset", "last_7d",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute ig insights local-intent: %v", err)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta ig insights local-intent")
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object payload, got %T", envelope["data"])
	}
	rangeData, ok := data["range"].(map[string]any)
	if !ok {
		t.Fatalf("expected range object, got %T", data["range"])
	}
	if got := rangeData["since"]; got != "2026-03-14" {
		t.Fatalf("unexpected since range %#v", got)
	}
	if got := rangeData["until"]; got != "2026-03-20" {
		t.Fatalf("unexpected until range %#v", got)
	}

	summary, ok := data["summary"].(map[string]any)
	if !ok {
		t.Fatalf("expected summary object, got %T", data["summary"])
	}
	calls, ok := summary["calls"].(map[string]any)
	if !ok {
		t.Fatalf("expected calls summary object, got %T", summary["calls"])
	}
	if got := calls["paid_instagram"]; got != float64(180) {
		t.Fatalf("unexpected paid_instagram calls %#v", got)
	}
	if got := calls["instagram_account"]; got != float64(55) {
		t.Fatalf("unexpected instagram_account calls %#v", got)
	}
	if got := calls["combined"]; got != float64(235) {
		t.Fatalf("unexpected combined calls %#v", got)
	}

	paidInstagram, ok := data["paid_instagram"].(map[string]any)
	if !ok {
		t.Fatalf("expected paid_instagram object, got %T", data["paid_instagram"])
	}
	rows, ok := paidInstagram["rows"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("expected one filtered paid row, got %#v", paidInstagram["rows"])
	}
	row, ok := rows[0].(map[string]any)
	if !ok {
		t.Fatalf("expected object row, got %T", rows[0])
	}
	if got := row["publisher_platform"]; got != "instagram" {
		t.Fatalf("unexpected paid row publisher_platform %#v", got)
	}
}
