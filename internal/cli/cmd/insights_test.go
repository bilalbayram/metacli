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
