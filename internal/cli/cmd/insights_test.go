package cmd

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
)

func TestNewInsightsCommandIncludesAccountsSubcommand(t *testing.T) {
	cmd := NewInsightsCommand(testRuntime("prod"))

	accountsCmd, _, err := cmd.Find([]string{"accounts"})
	if err != nil {
		t.Fatalf("find accounts subcommand: %v", err)
	}
	if accountsCmd == nil || accountsCmd.Name() != "accounts" {
		t.Fatalf("expected accounts subcommand, got %v", accountsCmd)
	}

	listCmd, _, err := cmd.Find([]string{"accounts", "list"})
	if err != nil {
		t.Fatalf("find accounts list subcommand: %v", err)
	}
	if listCmd == nil || listCmd.Name() != "list" {
		t.Fatalf("expected accounts list subcommand, got %v", listCmd)
	}

	actionTypesCmd, _, err := cmd.Find([]string{"action-types"})
	if err != nil {
		t.Fatalf("find action-types subcommand: %v", err)
	}
	if actionTypesCmd == nil || actionTypesCmd.Name() != "action-types" {
		t.Fatalf("expected action-types subcommand, got %v", actionTypesCmd)
	}
}

func TestInsightsRunMissingAccountIDIncludesGuidance(t *testing.T) {
	cmd := newInsightsRunCommand(testRuntime("prod"))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing account id error")
	}
	if !strings.Contains(err.Error(), "account id is required (--account-id)") {
		t.Fatalf("unexpected error text: %v", err)
	}
	if !strings.Contains(err.Error(), "meta insights accounts list --active-only --profile prod") {
		t.Fatalf("expected account discovery guidance, got: %v", err)
	}
}

func TestInsightsRunRejectsInvalidMetricPack(t *testing.T) {
	wasCalled := false
	useInsightsDependencies(t,
		func(string) (*ProfileCredentials, error) {
			wasCalled = true
			return nil, nil
		},
		func() *graph.Client {
			wasCalled = true
			return graph.NewClient(nil, "")
		},
	)

	cmd := newInsightsRunCommand(testRuntime("prod"))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"--account-id", "123",
		"--metric-pack", "unknown",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected invalid metric-pack error")
	}
	if !strings.Contains(err.Error(), "invalid --metric-pack value") {
		t.Fatalf("unexpected error text: %v", err)
	}
	if wasCalled {
		t.Fatal("expected dependency calls to be skipped on invalid metric-pack")
	}
}

func TestInsightsRunQualityMetricPackRequestsExpandedFields(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"data":[{"campaign_id":"1","spend":"10","impressions":"100"}]}`,
	}
	useInsightsStubDependencies(t, stub)

	cmd := newInsightsRunCommand(testRuntime("prod"))
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"--account-id", "123",
		"--metric-pack", "quality",
		"--format", "jsonl",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute insights run: %v", err)
	}
	if stub.calls != 1 {
		t.Fatalf("expected one graph call, got %d", stub.calls)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if got := requestURL.Path; got != "/v25.0/act_123/insights" {
		t.Fatalf("unexpected request path: %q", got)
	}
	if got := requestURL.Query().Get("fields"); got != strings.Join(insightsQualityMetricPackFields, ",") {
		t.Fatalf("unexpected fields query: %q", got)
	}
}

func TestInsightsRunLocalIntentMetricPackAddsFlatAliases(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response: `{"data":[{"campaign_id":"1","actions":[
{"action_type":"onsite_conversion.business_address_tap","value":"12"},
{"action_type":"click_to_call_native_call_placed","value":"4"},
{"action_type":"onsite_conversion.get_directions","value":"7"},
{"action_type":"onsite_conversion.profile_visit","value":"18"}
],"cost_per_action_type":[{"action_type":"onsite_conversion.business_address_tap","value":"1.75"}]}]}`,
	}
	useInsightsStubDependencies(t, stub)

	cmd := newInsightsRunCommand(testRuntime("prod"))
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"--account-id", "123",
		"--metric-pack", "local_intent",
		"--format", "json",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute insights run: %v", err)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if got := requestURL.Query().Get("fields"); got != strings.Join(insightsLocalIntentMetricPackFields, ",") {
		t.Fatalf("unexpected fields query: %q", got)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta insights run")
	data, ok := envelope["data"].([]any)
	if !ok || len(data) != 1 {
		t.Fatalf("expected one row in envelope, got %#v", envelope["data"])
	}
	row, ok := data[0].(map[string]any)
	if !ok {
		t.Fatalf("expected object row, got %T", data[0])
	}
	if got := row["address_taps"]; got != float64(12) {
		t.Fatalf("unexpected address_taps %#v", got)
	}
	if got := row["calls"]; got != float64(4) {
		t.Fatalf("unexpected calls %#v", got)
	}
	if got := row["directions"]; got != float64(7) {
		t.Fatalf("unexpected directions %#v", got)
	}
	if got := row["profile_visits"]; got != float64(18) {
		t.Fatalf("unexpected profile_visits %#v", got)
	}
	if _, ok := row["actions"].([]any); !ok {
		t.Fatalf("expected raw actions to be preserved, got %T", row["actions"])
	}
	if _, ok := row["cost_per_action_type"].([]any); !ok {
		t.Fatalf("expected raw cost_per_action_type to be preserved, got %T", row["cost_per_action_type"])
	}
}

func TestInsightsRunAcceptsJSONFormatAndAccountLevel(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"data":[{"account_id":"123"}]}`,
	}
	useInsightsStubDependencies(t, stub)

	cmd := newInsightsRunCommand(testRuntime("prod"))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"--account-id", "123",
		"--level", "account",
		"--format", "json",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute insights run: %v", err)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if got := requestURL.Query().Get("level"); got != "account" {
		t.Fatalf("unexpected level query: %q", got)
	}
}

func TestInsightsRunPublisherPlatformInstagramFiltersRows(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response: `{"data":[
{"publisher_platform":"facebook","campaign_id":"1"},
{"publisher_platform":"instagram","campaign_id":"2"}
]}`,
	}
	useInsightsStubDependencies(t, stub)

	cmd := newInsightsRunCommand(testRuntime("prod"))
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"--account-id", "123",
		"--publisher-platform", "instagram",
		"--format", "json",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute insights run: %v", err)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if got := requestURL.Query().Get("breakdowns"); got != "publisher_platform" {
		t.Fatalf("unexpected breakdowns query: %q", got)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	data, ok := envelope["data"].([]any)
	if !ok || len(data) != 1 {
		t.Fatalf("expected one filtered row, got %#v", envelope["data"])
	}
	row, ok := data[0].(map[string]any)
	if !ok {
		t.Fatalf("expected object row, got %T", data[0])
	}
	if got := row["publisher_platform"]; got != "instagram" {
		t.Fatalf("unexpected filtered publisher_platform %#v", got)
	}
}

func TestInsightsActionTypesReturnsStructuredDiscoveryOutput(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response: `{"data":[{"actions":[
{"action_type":"click_to_call_native_call_placed","value":"4"},
{"action_type":"click_to_call_native_20s_call_connect","value":"2"},
{"action_type":"link_click","value":"9"}
],"cost_per_action_type":[
{"action_type":"click_to_call_native_call_placed","value":"8.50"}
]}]}`,
	}
	useInsightsStubDependencies(t, stub)

	cmd := newInsightsActionTypesCommand(testRuntime("prod"))
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--account-id", "123"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute insights action-types: %v", err)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if got := requestURL.Query().Get("level"); got != "ad" {
		t.Fatalf("unexpected level query: %q", got)
	}
	if got := requestURL.Query().Get("fields"); got != strings.Join(insightsActionDiscoveryFields, ",") {
		t.Fatalf("unexpected fields query: %q", got)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta insights action-types")
	data, ok := envelope["data"].([]any)
	if !ok || len(data) != 3 {
		t.Fatalf("expected three discovered action types, got %#v", envelope["data"])
	}
	first, ok := data[0].(map[string]any)
	if !ok {
		t.Fatalf("expected object record, got %T", data[0])
	}
	if got := first["action_type"]; got != "click_to_call_native_20s_call_connect" {
		t.Fatalf("unexpected first action type %#v", got)
	}
	if _, ok := first["normalized_field"]; ok {
		t.Fatalf("expected no normalized field for call connect, got %#v", first["normalized_field"])
	}
	second, ok := data[1].(map[string]any)
	if !ok {
		t.Fatalf("expected object record, got %T", data[1])
	}
	if got := second["action_type"]; got != "click_to_call_native_call_placed" {
		t.Fatalf("unexpected second action type %#v", got)
	}
	if got := second["normalized_field"]; got != "calls" {
		t.Fatalf("unexpected normalized field %#v", got)
	}
	third, ok := data[2].(map[string]any)
	if !ok {
		t.Fatalf("expected object record, got %T", data[2])
	}
	if got := third["action_type"]; got != "link_click" {
		t.Fatalf("unexpected third action type %#v", got)
	}
	if _, ok := third["normalized_field"]; ok {
		t.Fatalf("expected no normalized field for link_click, got %#v", third["normalized_field"])
	}
}

func TestInsightsActionTypesPublisherPlatformInstagramFiltersRows(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response: `{"data":[
{"publisher_platform":"facebook","actions":[{"action_type":"link_click","value":"9"}]},
{"publisher_platform":"instagram","actions":[{"action_type":"click_to_call_native_call_placed","value":"4"}],"cost_per_action_type":[{"action_type":"click_to_call_native_call_placed","value":"8.50"}]}
]}`,
	}
	useInsightsStubDependencies(t, stub)

	cmd := newInsightsActionTypesCommand(testRuntime("prod"))
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"--account-id", "123",
		"--publisher-platform", "instagram",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute insights action-types: %v", err)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if got := requestURL.Query().Get("breakdowns"); got != "publisher_platform" {
		t.Fatalf("unexpected breakdowns query: %q", got)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	data, ok := envelope["data"].([]any)
	if !ok || len(data) != 1 {
		t.Fatalf("expected one discovered action type, got %#v", envelope["data"])
	}
	row, ok := data[0].(map[string]any)
	if !ok {
		t.Fatalf("expected object record, got %T", data[0])
	}
	if got := row["action_type"]; got != "click_to_call_native_call_placed" {
		t.Fatalf("unexpected action_type %#v", got)
	}
	if got := row["normalized_field"]; got != "calls" {
		t.Fatalf("unexpected normalized_field %#v", got)
	}
}

func TestInsightsAccountsListActiveOnlyFiltersInactiveAccounts(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response: `{"data":[
{"id":"act_1","account_id":"1","name":"Active","account_status":1,"currency":"USD","timezone_name":"America/Los_Angeles"},
{"id":"act_2","account_id":"2","name":"Disabled","account_status":101,"currency":"USD","timezone_name":"America/Los_Angeles"}
]}`,
	}
	useInsightsStubDependencies(t, stub)

	cmd := newInsightsAccountsListCommand(testRuntime("prod"))
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--active-only"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute insights accounts list: %v", err)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if got := requestURL.Path; got != "/v25.0/me/adaccounts" {
		t.Fatalf("unexpected request path: %q", got)
	}
	if got := requestURL.Query().Get("fields"); got != insightsAccountsListFields {
		t.Fatalf("unexpected fields query: %q", got)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta insights accounts list")
	data, ok := envelope["data"].([]any)
	if !ok {
		t.Fatalf("expected array data payload, got %T", envelope["data"])
	}
	if len(data) != 1 {
		t.Fatalf("expected one active account, got %d", len(data))
	}
	first, ok := data[0].(map[string]any)
	if !ok {
		t.Fatalf("expected object row, got %T", data[0])
	}
	if got, _ := first["id"].(string); got != "act_1" {
		t.Fatalf("unexpected account id: %q", got)
	}
}

func useInsightsStubDependencies(t *testing.T, httpClient graph.HTTPClient) {
	t.Helper()
	useInsightsDependencies(t,
		func(profile string) (*ProfileCredentials, error) {
			if profile != "prod" {
				t.Fatalf("unexpected profile %q", profile)
			}
			return &ProfileCredentials{
				Name:      "prod",
				Profile:   config.Profile{GraphVersion: "v25.0"},
				Token:     "test-token",
				AppSecret: "test-secret",
			}, nil
		},
		func() *graph.Client {
			client := graph.NewClient(httpClient, "https://graph.example.com")
			client.MaxRetries = 0
			return client
		},
	)
}

func useInsightsDependencies(t *testing.T, loadFn func(string) (*ProfileCredentials, error), clientFn func() *graph.Client) {
	t.Helper()
	originalLoad := insightsLoadProfileCredentials
	originalClient := insightsNewGraphClient
	t.Cleanup(func() {
		insightsLoadProfileCredentials = originalLoad
		insightsNewGraphClient = originalClient
	})

	insightsLoadProfileCredentials = loadFn
	insightsNewGraphClient = clientFn
}
