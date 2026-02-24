package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
)

func TestNewAdsetCommandIncludesLifecycleSubcommands(t *testing.T) {
	t.Parallel()

	cmd := NewAdsetCommand(Runtime{})

	for _, name := range []string{"create", "update", "pause", "resume"} {
		sub, _, err := cmd.Find([]string{name})
		if err != nil {
			t.Fatalf("find %s subcommand: %v", name, err)
		}
		if sub == nil || sub.Name() != name {
			t.Fatalf("expected %s subcommand, got %#v", name, sub)
		}
	}
}

func TestAdsetCreateExecutesMutationForNonBudgetPayload(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"id":"501","name":"Prospecting"}`,
	}
	schemaDir := writeAdsetSchemaPack(t)
	useAdsetDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name: "prod",
				Profile: config.Profile{
					Domain:       config.DefaultDomain,
					GraphVersion: config.DefaultGraphVersion,
				},
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
	errOutput := &bytes.Buffer{}
	cmd := NewAdsetCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"create",
		"--account-id", "act_1234",
		"--params", "name=Prospecting,campaign_id=cmp_1,status=PAUSED,billing_event=IMPRESSIONS,optimization_goal=OFFSITE_CONVERSIONS",
		"--schema-dir", schemaDir,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute adset create: %v", err)
	}

	if stub.calls != 1 {
		t.Fatalf("expected one graph call, got %d", stub.calls)
	}
	if stub.lastMethod != http.MethodPost {
		t.Fatalf("unexpected method %q", stub.lastMethod)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if requestURL.Path != "/v25.0/act_1234/adsets" {
		t.Fatalf("unexpected path %q", requestURL.Path)
	}

	form, err := url.ParseQuery(stub.lastBody)
	if err != nil {
		t.Fatalf("parse form body: %v", err)
	}
	if got := form.Get("name"); got != "Prospecting" {
		t.Fatalf("unexpected name %q", got)
	}
	if got := form.Get("campaign_id"); got != "cmp_1" {
		t.Fatalf("unexpected campaign_id %q", got)
	}
	if got := form.Get("daily_budget"); got != "" {
		t.Fatalf("expected no daily_budget, got %q", got)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta adset create")
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object data payload, got %T", envelope["data"])
	}
	if got := data["adset_id"]; got != "501" {
		t.Fatalf("unexpected adset id %v", got)
	}
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestAdsetCreateFailsWithoutBudgetConfirmation(t *testing.T) {
	wasCalled := false
	schemaDir := writeAdsetSchemaPack(t)
	useAdsetDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name: "prod",
				Profile: config.Profile{
					Domain:       config.DefaultDomain,
					GraphVersion: config.DefaultGraphVersion,
				},
				Token: "test-token",
			}, nil
		},
		func() *graph.Client {
			wasCalled = true
			return graph.NewClient(nil, "")
		},
	)

	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewAdsetCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"create",
		"--account-id", "1234",
		"--params", "name=Prospecting,campaign_id=cmp_1,daily_budget=1000",
		"--schema-dir", schemaDir,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "budget change detected") {
		t.Fatalf("unexpected error: %v", err)
	}
	if wasCalled {
		t.Fatal("graph client should not execute on missing budget confirmation")
	}
	if output.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", output.String())
	}

	envelope := decodeEnvelope(t, errOutput.Bytes())
	if got := envelope["command"]; got != "meta adset create" {
		t.Fatalf("unexpected command field %v", got)
	}
	if envelope["success"] != false {
		t.Fatalf("expected success=false, got %v", envelope["success"])
	}
}

func TestAdsetCreateFailsWhenIntentRequirementsBlockMutation(t *testing.T) {
	wasCalled := false
	schemaDir := writeAdsetSchemaPack(t)
	useAdsetDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name: "prod",
				Profile: config.Profile{
					Domain:       config.DefaultDomain,
					GraphVersion: config.DefaultGraphVersion,
				},
				Token: "test-token",
			}, nil
		},
		func() *graph.Client {
			wasCalled = true
			return graph.NewClient(nil, "")
		},
	)

	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewAdsetCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"create",
		"--account-id", "1234",
		"--params", "name=Prospecting,campaign_id=cmp_1,bid_amount=100",
		"--schema-dir", schemaDir,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "ad set intent requirements blocked mutation") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "field \"bid_strategy\" is required") {
		t.Fatalf("unexpected error detail: %v", err)
	}
	if wasCalled {
		t.Fatal("graph client should not execute on intent requirement violations")
	}
	if output.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", output.String())
	}
	envelope := decodeEnvelope(t, errOutput.Bytes())
	if got := envelope["command"]; got != "meta adset create" {
		t.Fatalf("unexpected command field %v", got)
	}
}

func TestAdsetCreateFailsWhenBudgetIsBelowCurrencyFloor(t *testing.T) {
	stub := &adsetQueuedHTTPClient{
		t: t,
		responses: []adsetQueuedResponse{
			{
				body: `{"currency":"USD"}`,
				assert: func(t *testing.T, req *http.Request, _ string) {
					t.Helper()
					if req.Method != http.MethodGet {
						t.Fatalf("unexpected method %q", req.Method)
					}
					if req.URL.Path != "/v25.0/act_1234" {
						t.Fatalf("unexpected currency-lookup path %q", req.URL.Path)
					}
					if got := req.URL.Query().Get("fields"); got != "currency" {
						t.Fatalf("unexpected currency-lookup fields query %q", got)
					}
				},
			},
		},
	}
	schemaDir := writeAdsetSchemaPack(t)
	useAdsetDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name: "prod",
				Profile: config.Profile{
					Domain:       config.DefaultDomain,
					GraphVersion: config.DefaultGraphVersion,
				},
				Token: "test-token",
			}, nil
		},
		func() *graph.Client {
			client := graph.NewClient(stub, "https://graph.example.com")
			client.MaxRetries = 0
			return client
		},
	)

	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewAdsetCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"create",
		"--account-id", "1234",
		"--params", "name=Prospecting,campaign_id=cmp_1,daily_budget=99",
		"--confirm-budget-change",
		"--schema-dir", schemaDir,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "ad set budget floor check blocked mutation") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "field \"daily_budget\" value 99 is below minimum 100") {
		t.Fatalf("unexpected error detail: %v", err)
	}
	if !strings.Contains(err.Error(), "remediation: set \"daily_budget\" >= 100 minor units before retrying") {
		t.Fatalf("unexpected remediation detail: %v", err)
	}
	if stub.calls != len(stub.responses) {
		t.Fatalf("expected %d graph calls, got %d", len(stub.responses), stub.calls)
	}
	if output.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", output.String())
	}
	envelope := decodeEnvelope(t, errOutput.Bytes())
	if got := envelope["command"]; got != "meta adset create" {
		t.Fatalf("unexpected command field %v", got)
	}
}

func TestAdsetUpdateFailsWhenBudgetIsBelowCurrencyFloor(t *testing.T) {
	stub := &adsetQueuedHTTPClient{
		t: t,
		responses: []adsetQueuedResponse{
			{
				body: `{"account_id":"act_1234"}`,
				assert: func(t *testing.T, req *http.Request, _ string) {
					t.Helper()
					if req.Method != http.MethodGet {
						t.Fatalf("unexpected method %q", req.Method)
					}
					if req.URL.Path != "/v25.0/8100" {
						t.Fatalf("unexpected account-lookup path %q", req.URL.Path)
					}
					if got := req.URL.Query().Get("fields"); got != "account_id" {
						t.Fatalf("unexpected account-lookup fields query %q", got)
					}
				},
			},
			{
				body: `{"currency":"USD"}`,
				assert: func(t *testing.T, req *http.Request, _ string) {
					t.Helper()
					if req.Method != http.MethodGet {
						t.Fatalf("unexpected method %q", req.Method)
					}
					if req.URL.Path != "/v25.0/act_1234" {
						t.Fatalf("unexpected currency-lookup path %q", req.URL.Path)
					}
					if got := req.URL.Query().Get("fields"); got != "currency" {
						t.Fatalf("unexpected currency-lookup fields query %q", got)
					}
				},
			},
		},
	}
	schemaDir := writeAdsetSchemaPack(t)
	useAdsetDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name: "prod",
				Profile: config.Profile{
					Domain:       config.DefaultDomain,
					GraphVersion: config.DefaultGraphVersion,
				},
				Token: "test-token",
			}, nil
		},
		func() *graph.Client {
			client := graph.NewClient(stub, "https://graph.example.com")
			client.MaxRetries = 0
			return client
		},
	)

	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewAdsetCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"update",
		"--adset-id", "8100",
		"--params", "daily_budget=99",
		"--confirm-budget-change",
		"--schema-dir", schemaDir,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "field \"daily_budget\" value 99 is below minimum 100") {
		t.Fatalf("unexpected error detail: %v", err)
	}
	if stub.calls != len(stub.responses) {
		t.Fatalf("expected %d graph calls, got %d", len(stub.responses), stub.calls)
	}
	if output.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", output.String())
	}
	envelope := decodeEnvelope(t, errOutput.Bytes())
	if got := envelope["command"]; got != "meta adset update" {
		t.Fatalf("unexpected command field %v", got)
	}
}

func TestAdsetUpdateBudgetMutationAllowsConfirmation(t *testing.T) {
	stub := &adsetQueuedHTTPClient{
		t: t,
		responses: []adsetQueuedResponse{
			{
				body: `{"account_id":"act_1234"}`,
				assert: func(t *testing.T, req *http.Request, _ string) {
					t.Helper()
					if req.Method != http.MethodGet {
						t.Fatalf("unexpected method %q", req.Method)
					}
					if req.URL.Path != "/v25.0/8100" {
						t.Fatalf("unexpected account-lookup path %q", req.URL.Path)
					}
					if got := req.URL.Query().Get("fields"); got != "account_id" {
						t.Fatalf("unexpected account-lookup fields query %q", got)
					}
				},
			},
			{
				body: `{"currency":"USD"}`,
				assert: func(t *testing.T, req *http.Request, _ string) {
					t.Helper()
					if req.Method != http.MethodGet {
						t.Fatalf("unexpected method %q", req.Method)
					}
					if req.URL.Path != "/v25.0/act_1234" {
						t.Fatalf("unexpected currency-lookup path %q", req.URL.Path)
					}
					if got := req.URL.Query().Get("fields"); got != "currency" {
						t.Fatalf("unexpected currency-lookup fields query %q", got)
					}
				},
			},
			{
				body: `{"success":true}`,
				assert: func(t *testing.T, req *http.Request, body string) {
					t.Helper()
					if req.Method != http.MethodPost {
						t.Fatalf("unexpected method %q", req.Method)
					}
					if req.URL.Path != "/v25.0/8100" {
						t.Fatalf("unexpected mutation path %q", req.URL.Path)
					}
					form, err := url.ParseQuery(body)
					if err != nil {
						t.Fatalf("parse form body: %v", err)
					}
					if got := form.Get("daily_budget"); got != "2000" {
						t.Fatalf("unexpected daily_budget %q", got)
					}
				},
			},
		},
	}
	schemaDir := writeAdsetSchemaPack(t)
	useAdsetDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name: "prod",
				Profile: config.Profile{
					Domain:       config.DefaultDomain,
					GraphVersion: config.DefaultGraphVersion,
				},
				Token: "test-token",
			}, nil
		},
		func() *graph.Client {
			client := graph.NewClient(stub, "https://graph.example.com")
			client.MaxRetries = 0
			return client
		},
	)

	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewAdsetCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"update",
		"--adset-id", "8100",
		"--params", "daily_budget=2000",
		"--confirm-budget-change",
		"--schema-dir", schemaDir,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute adset update: %v", err)
	}
	if stub.calls != len(stub.responses) {
		t.Fatalf("expected %d graph calls, got %d", len(stub.responses), stub.calls)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta adset update")
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestAdsetPauseExecutesMutation(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"success":true}`,
	}
	schemaDir := writeAdsetSchemaPack(t)
	useAdsetDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name: "prod",
				Profile: config.Profile{
					Domain:       config.DefaultDomain,
					GraphVersion: config.DefaultGraphVersion,
				},
				Token: "test-token",
			}, nil
		},
		func() *graph.Client {
			client := graph.NewClient(stub, "https://graph.example.com")
			client.MaxRetries = 0
			return client
		},
	)

	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewAdsetCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"pause",
		"--adset-id", "8101",
		"--schema-dir", schemaDir,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute adset pause: %v", err)
	}

	form, err := url.ParseQuery(stub.lastBody)
	if err != nil {
		t.Fatalf("parse request form: %v", err)
	}
	if got := form.Get("status"); got != "PAUSED" {
		t.Fatalf("unexpected status %q", got)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta adset pause")
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestAdsetResumeReturnsStructuredGraphError(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusBadRequest,
		response:   `{"error":{"message":"Unsupported post request","type":"GraphMethodException","code":100,"error_subcode":33,"fbtrace_id":"trace-123"}}`,
	}
	schemaDir := writeAdsetSchemaPack(t)
	useAdsetDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name: "prod",
				Profile: config.Profile{
					Domain:       config.DefaultDomain,
					GraphVersion: config.DefaultGraphVersion,
				},
				Token: "test-token",
			}, nil
		},
		func() *graph.Client {
			client := graph.NewClient(stub, "https://graph.example.com")
			client.MaxRetries = 0
			return client
		},
	)

	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewAdsetCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"resume",
		"--adset-id", "8101",
		"--schema-dir", schemaDir,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "meta api error") {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", output.String())
	}

	envelope := decodeEnvelope(t, errOutput.Bytes())
	if got := envelope["command"]; got != "meta adset resume" {
		t.Fatalf("unexpected command field %v", got)
	}
	if envelope["success"] != false {
		t.Fatalf("expected success=false, got %v", envelope["success"])
	}
	errorBody, ok := envelope["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error payload, got %T", envelope["error"])
	}
	if got := errorBody["code"]; got != float64(100) {
		t.Fatalf("unexpected error code %v", got)
	}
	if got := errorBody["error_subcode"]; got != float64(33) {
		t.Fatalf("unexpected error_subcode %v", got)
	}
}

func useAdsetDependencies(t *testing.T, loadFn func(string) (*ProfileCredentials, error), clientFn func() *graph.Client) {
	t.Helper()
	originalLoad := adsetLoadProfileCredentials
	originalClient := adsetNewGraphClient
	t.Cleanup(func() {
		adsetLoadProfileCredentials = originalLoad
		adsetNewGraphClient = originalClient
	})

	adsetLoadProfileCredentials = loadFn
	adsetNewGraphClient = clientFn
}

func writeAdsetSchemaPack(t *testing.T) string {
	t.Helper()
	schemaDir := t.TempDir()
	marketingDir := filepath.Join(schemaDir, config.DefaultDomain)
	if err := os.MkdirAll(marketingDir, 0o755); err != nil {
		t.Fatalf("create schema dir: %v", err)
	}

	packPath := filepath.Join(marketingDir, config.DefaultGraphVersion+".json")
	pack := `{
  "domain":"marketing",
  "version":"v25.0",
  "entities":{"adset":["id","name","status","campaign_id","billing_event","optimization_goal","daily_budget","lifetime_budget"]},
  "endpoint_params":{"adsets.post":["name","campaign_id","status","billing_event","optimization_goal","daily_budget","lifetime_budget"]},
  "deprecated_params":{"adsets.post":["legacy_param"]}
}`
	if err := os.WriteFile(packPath, []byte(pack), 0o644); err != nil {
		t.Fatalf("write schema pack: %v", err)
	}
	return schemaDir
}

func TestAdsetCreateSupportsBudgetFromJSONWithConfirmation(t *testing.T) {
	stub := &adsetQueuedHTTPClient{
		t: t,
		responses: []adsetQueuedResponse{
			{
				body: `{"currency":"USD"}`,
				assert: func(t *testing.T, req *http.Request, _ string) {
					t.Helper()
					if req.Method != http.MethodGet {
						t.Fatalf("unexpected method %q", req.Method)
					}
					if req.URL.Path != "/v25.0/act_1234" {
						t.Fatalf("unexpected currency-lookup path %q", req.URL.Path)
					}
					if got := req.URL.Query().Get("fields"); got != "currency" {
						t.Fatalf("unexpected currency-lookup fields query %q", got)
					}
				},
			},
			{
				body: `{"id":"777"}`,
				assert: func(t *testing.T, req *http.Request, body string) {
					t.Helper()
					if req.Method != http.MethodPost {
						t.Fatalf("unexpected method %q", req.Method)
					}
					if req.URL.Path != "/v25.0/act_1234/adsets" {
						t.Fatalf("unexpected mutation path %q", req.URL.Path)
					}
					form, err := url.ParseQuery(body)
					if err != nil {
						t.Fatalf("parse form body: %v", err)
					}
					if got := form.Get("daily_budget"); got != "1300" {
						t.Fatalf("unexpected daily_budget %q", got)
					}
				},
			},
		},
	}
	schemaDir := writeAdsetSchemaPack(t)
	useAdsetDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name: "prod",
				Profile: config.Profile{
					Domain:       config.DefaultDomain,
					GraphVersion: config.DefaultGraphVersion,
				},
				Token: "test-token",
			}, nil
		},
		func() *graph.Client {
			client := graph.NewClient(stub, "https://graph.example.com")
			client.MaxRetries = 0
			return client
		},
	)

	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewAdsetCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"create",
		"--account-id", "1234",
		"--params", "name=Prospecting,campaign_id=cmp_1",
		"--json", `{"daily_budget":1300}`,
		"--confirm-budget-change",
		"--schema-dir", schemaDir,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute adset create with json budget: %v", err)
	}
	if stub.calls != len(stub.responses) {
		t.Fatalf("expected %d graph calls, got %d", len(stub.responses), stub.calls)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta adset create")
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestAdsetCreateFailsWithoutBudgetConfirmationForJSONBudget(t *testing.T) {
	wasCalled := false
	schemaDir := writeAdsetSchemaPack(t)
	useAdsetDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name: "prod",
				Profile: config.Profile{
					Domain:       config.DefaultDomain,
					GraphVersion: config.DefaultGraphVersion,
				},
				Token: "test-token",
			}, nil
		},
		func() *graph.Client {
			wasCalled = true
			return graph.NewClient(nil, "")
		},
	)

	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewAdsetCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"create",
		"--account-id", "1234",
		"--params", "name=Prospecting,campaign_id=cmp_1",
		"--json", `{"daily_budget":1300}`,
		"--schema-dir", schemaDir,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "budget change detected") {
		t.Fatalf("unexpected error: %v", err)
	}
	if wasCalled {
		t.Fatal("graph client should not execute when json payload changes budget without confirmation")
	}

	envelope := decodeEnvelope(t, errOutput.Bytes())
	if got := envelope["command"]; got != "meta adset create" {
		t.Fatalf("unexpected command field %v", got)
	}
}

func TestAdsetPauseEnvelopeDataContainsAdSetID(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"success":true}`,
	}
	schemaDir := writeAdsetSchemaPack(t)
	useAdsetDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name: "prod",
				Profile: config.Profile{
					Domain:       config.DefaultDomain,
					GraphVersion: config.DefaultGraphVersion,
				},
				Token: "test-token",
			}, nil
		},
		func() *graph.Client {
			client := graph.NewClient(stub, "https://graph.example.com")
			client.MaxRetries = 0
			return client
		},
	)

	output := &bytes.Buffer{}
	cmd := NewAdsetCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{
		"pause",
		"--adset-id", "8102",
		"--schema-dir", schemaDir,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute adset pause: %v", err)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object payload, got %T", envelope["data"])
	}
	if got := data["adset_id"]; got != "8102" {
		t.Fatalf("unexpected adset id %v", got)
	}
}

func TestAdsetResumeWritesValidJSONEnvelope(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"success":true}`,
	}
	schemaDir := writeAdsetSchemaPack(t)
	useAdsetDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name: "prod",
				Profile: config.Profile{
					Domain:       config.DefaultDomain,
					GraphVersion: config.DefaultGraphVersion,
				},
				Token: "test-token",
			}, nil
		},
		func() *graph.Client {
			client := graph.NewClient(stub, "https://graph.example.com")
			client.MaxRetries = 0
			return client
		},
	)

	output := &bytes.Buffer{}
	cmd := NewAdsetCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{
		"resume",
		"--adset-id", "8103",
		"--schema-dir", schemaDir,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute adset resume: %v", err)
	}

	decoded := map[string]any{}
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("decode output JSON: %v", err)
	}
	if got := decoded["command"]; got != "meta adset resume" {
		t.Fatalf("unexpected command %v", got)
	}
}

func TestResolveAdsetIntentRequirements(t *testing.T) {
	cases := []struct {
		name    string
		params  map[string]string
		wantErr string
	}{
		{
			name: "bid amount without strategy",
			params: map[string]string{
				"bid_amount": "150",
			},
			wantErr: "field \"bid_strategy\" is required",
		},
		{
			name: "unsupported strategy",
			params: map[string]string{
				"bid_strategy": "AUTO_BID",
			},
			wantErr: "value \"AUTO_BID\" is unsupported",
		},
		{
			name: "capped strategy missing bid amount",
			params: map[string]string{
				"bid_strategy": "COST_CAP",
			},
			wantErr: "field \"bid_amount\" is required",
		},
		{
			name: "without-cap strategy forbids bid amount",
			params: map[string]string{
				"bid_strategy": "LOWEST_COST_WITHOUT_CAP",
				"bid_amount":   "200",
			},
			wantErr: "field \"bid_amount\" is not allowed",
		},
		{
			name: "blank bid amount fails parse",
			params: map[string]string{
				"bid_strategy": "COST_CAP",
				"bid_amount":   "  ",
			},
			wantErr: "field \"bid_amount\" cannot be empty",
		},
		{
			name: "valid capped strategy with bid amount",
			params: map[string]string{
				"bid_strategy": "TARGET_COST",
				"bid_amount":   "300",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := resolveAdsetIntentRequirements(tc.params)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("resolve intent requirements: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

type adsetQueuedResponse struct {
	statusCode int
	body       string
	assert     func(*testing.T, *http.Request, string)
}

type adsetQueuedHTTPClient struct {
	t *testing.T

	responses []adsetQueuedResponse
	calls     int

	lastMethod string
	lastURL    string
	lastBody   string
}

func (c *adsetQueuedHTTPClient) Do(req *http.Request) (*http.Response, error) {
	c.calls++
	c.lastMethod = req.Method
	c.lastURL = req.URL.String()
	c.lastBody = ""
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			c.t.Fatalf("read request body: %v", err)
		}
		c.lastBody = string(body)
	}
	if c.calls > len(c.responses) {
		c.t.Fatalf("unexpected graph request %d: %s %s", c.calls, req.Method, req.URL.String())
	}

	response := c.responses[c.calls-1]
	if response.assert != nil {
		response.assert(c.t, req, c.lastBody)
	}

	statusCode := response.statusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	return &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(response.body)),
	}, nil
}
