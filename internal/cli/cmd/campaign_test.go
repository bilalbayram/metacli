package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
)

func TestNewCampaignCommandIncludesLifecycleSubcommands(t *testing.T) {
	t.Parallel()

	cmd := NewCampaignCommand(Runtime{})

	for _, name := range []string{"create", "update", "pause", "resume", "clone"} {
		sub, _, err := cmd.Find([]string{name})
		if err != nil {
			t.Fatalf("find %s subcommand: %v", name, err)
		}
		if sub == nil || sub.Name() != name {
			t.Fatalf("expected %s subcommand, got %#v", name, sub)
		}
	}
}

func TestCampaignCreateExecutesMutation(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"id":"991","name":"Launch"}`,
	}
	schemaDir := writeCampaignSchemaPack(t)
	useCampaignDependencies(t,
		func(profile string) (*ProfileCredentials, error) {
			if profile != "prod" {
				t.Fatalf("unexpected profile %q", profile)
			}
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
	cmd := NewCampaignCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"create",
		"--account-id", "act_1234",
		"--params", "name=Launch,objective=OUTCOME_SALES,status=PAUSED",
		"--schema-dir", schemaDir,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute campaign create: %v", err)
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
	if requestURL.Path != "/v25.0/act_1234/campaigns" {
		t.Fatalf("unexpected path %q", requestURL.Path)
	}
	form, err := url.ParseQuery(stub.lastBody)
	if err != nil {
		t.Fatalf("parse form body: %v", err)
	}
	if got := form.Get("name"); got != "Launch" {
		t.Fatalf("unexpected name %q", got)
	}
	if got := form.Get("objective"); got != "OUTCOME_SALES" {
		t.Fatalf("unexpected objective %q", got)
	}
	if got := form.Get("status"); got != "PAUSED" {
		t.Fatalf("unexpected status %q", got)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta campaign create")
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object data payload, got %T", envelope["data"])
	}
	if got := data["campaign_id"]; got != "991" {
		t.Fatalf("unexpected campaign id %v", got)
	}
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestCampaignCreateFailsLintValidation(t *testing.T) {
	wasCalled := false
	schemaDir := writeCampaignSchemaPack(t)
	useCampaignDependencies(t,
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
	cmd := NewCampaignCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"create",
		"--account-id", "1234",
		"--params", "name=Launch,objective=OUTCOME_SALES,invalid_field=1",
		"--schema-dir", schemaDir,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "campaign mutation lint failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if wasCalled {
		t.Fatal("graph client should not execute on lint failure")
	}
	if output.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", output.String())
	}

	envelope := decodeEnvelope(t, errOutput.Bytes())
	if got := envelope["command"]; got != "meta campaign create" {
		t.Fatalf("unexpected command field %v", got)
	}
	if envelope["success"] != false {
		t.Fatalf("expected success=false, got %v", envelope["success"])
	}
}

func TestCampaignUpdateExecutesMutation(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"success":true}`,
	}
	schemaDir := writeCampaignSchemaPack(t)
	useCampaignDependencies(t,
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
	cmd := NewCampaignCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"update",
		"--campaign-id", "777",
		"--params", "name=Updated Name",
		"--schema-dir", schemaDir,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute campaign update: %v", err)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if requestURL.Path != "/v25.0/777" {
		t.Fatalf("unexpected path %q", requestURL.Path)
	}
	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta campaign update")
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestCampaignUpdateFailsOnEmptyPayload(t *testing.T) {
	schemaDir := writeCampaignSchemaPack(t)
	useCampaignDependencies(t,
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
			return graph.NewClient(nil, "")
		},
	)

	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewCampaignCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"update",
		"--campaign-id", "777",
		"--schema-dir", schemaDir,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "payload cannot be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", output.String())
	}
	envelope := decodeEnvelope(t, errOutput.Bytes())
	if got := envelope["command"]; got != "meta campaign update" {
		t.Fatalf("unexpected command field %v", got)
	}
}

func TestCampaignPauseExecutesMutation(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"success":true}`,
	}
	schemaDir := writeCampaignSchemaPack(t)
	useCampaignDependencies(t,
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
	cmd := NewCampaignCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"pause",
		"--campaign-id", "777",
		"--schema-dir", schemaDir,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute campaign pause: %v", err)
	}

	form, err := url.ParseQuery(stub.lastBody)
	if err != nil {
		t.Fatalf("parse request form: %v", err)
	}
	if got := form.Get("status"); got != "PAUSED" {
		t.Fatalf("unexpected status %q", got)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta campaign pause")
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestCampaignPauseFailsWithoutCampaignID(t *testing.T) {
	schemaDir := writeCampaignSchemaPack(t)
	useCampaignDependencies(t,
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
			return graph.NewClient(nil, "")
		},
	)

	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewCampaignCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"pause",
		"--schema-dir", schemaDir,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "campaign id is required") {
		t.Fatalf("unexpected error: %v", err)
	}
	envelope := decodeEnvelope(t, errOutput.Bytes())
	if got := envelope["command"]; got != "meta campaign pause" {
		t.Fatalf("unexpected command field %v", got)
	}
}

func TestCampaignResumeExecutesMutation(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"success":true}`,
	}
	schemaDir := writeCampaignSchemaPack(t)
	useCampaignDependencies(t,
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
	cmd := NewCampaignCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"resume",
		"--campaign-id", "777",
		"--schema-dir", schemaDir,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute campaign resume: %v", err)
	}

	form, err := url.ParseQuery(stub.lastBody)
	if err != nil {
		t.Fatalf("parse request form: %v", err)
	}
	if got := form.Get("status"); got != "ACTIVE" {
		t.Fatalf("unexpected status %q", got)
	}
	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta campaign resume")
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestCampaignResumeReturnsStructuredGraphError(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusBadRequest,
		response:   `{"error":{"message":"Unsupported post request","type":"GraphMethodException","code":100,"error_subcode":33,"fbtrace_id":"trace-123"}}`,
	}
	schemaDir := writeCampaignSchemaPack(t)
	useCampaignDependencies(t,
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
	cmd := NewCampaignCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"resume",
		"--campaign-id", "777",
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
	if got := envelope["command"]; got != "meta campaign resume" {
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

func TestCampaignCloneExecutesReadSanitizeCreateFlow(t *testing.T) {
	schemaDir := writeCampaignSchemaPack(t)

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		switch requestCount {
		case 1:
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected read method %s", r.Method)
			}
			if r.URL.Path != "/v25.0/source_77" {
				t.Fatalf("unexpected read path %q", r.URL.Path)
			}
			if got := r.URL.Query().Get("fields"); got != "id,name,objective,status,daily_budget,lifetime_budget" {
				t.Fatalf("unexpected fields query %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":              "source_77",
				"name":            "Source Name",
				"objective":       "OUTCOME_SALES",
				"status":          "PAUSED",
				"daily_budget":    "1000",
				"lifetime_budget": "0",
			})
		case 2:
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected create method %s", r.Method)
			}
			if r.URL.Path != "/v25.0/act_4242/campaigns" {
				t.Fatalf("unexpected create path %q", r.URL.Path)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read create request body: %v", err)
			}
			form, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("parse create request body: %v", err)
			}
			if got := form.Get("name"); got != "Clone Name" {
				t.Fatalf("unexpected cloned name %q", got)
			}
			if got := form.Get("status"); got != "ACTIVE" {
				t.Fatalf("unexpected cloned status %q", got)
			}
			if got := form.Get("objective"); got != "OUTCOME_SALES" {
				t.Fatalf("unexpected cloned objective %q", got)
			}
			if got := form.Get("id"); got != "" {
				t.Fatalf("immutable id should be removed, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "clone_88",
			})
		default:
			t.Fatalf("unexpected request count %d", requestCount)
		}
	}))
	defer server.Close()

	useCampaignDependencies(t,
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
			client := graph.NewClient(server.Client(), server.URL)
			client.MaxRetries = 0
			return client
		},
	)

	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewCampaignCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"clone",
		"--source-campaign-id", "source_77",
		"--account-id", "4242",
		"--params", "name=Clone Name,status=ACTIVE",
		"--schema-dir", schemaDir,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute campaign clone: %v", err)
	}

	if requestCount != 2 {
		t.Fatalf("expected two requests, got %d", requestCount)
	}
	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta campaign clone")
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object payload, got %T", envelope["data"])
	}
	if got := data["campaign_id"]; got != "clone_88" {
		t.Fatalf("unexpected campaign id %v", got)
	}
	if got := data["source_campaign_id"]; got != "source_77" {
		t.Fatalf("unexpected source campaign id %v", got)
	}
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestCampaignCloneFailsOnUnknownField(t *testing.T) {
	wasCalled := false
	schemaDir := writeCampaignSchemaPack(t)
	useCampaignDependencies(t,
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
	cmd := NewCampaignCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"clone",
		"--source-campaign-id", "source_77",
		"--account-id", "4242",
		"--fields", "id,unknown_field",
		"--schema-dir", schemaDir,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "campaign clone field lint failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if wasCalled {
		t.Fatal("graph client should not execute on lint failure")
	}
	if output.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", output.String())
	}
	envelope := decodeEnvelope(t, errOutput.Bytes())
	if got := envelope["command"]; got != "meta campaign clone" {
		t.Fatalf("unexpected command field %v", got)
	}
	if envelope["success"] != false {
		t.Fatalf("expected success=false, got %v", envelope["success"])
	}
}

func useCampaignDependencies(t *testing.T, loadFn func(string) (*ProfileCredentials, error), clientFn func() *graph.Client) {
	t.Helper()
	originalLoad := campaignLoadProfileCredentials
	originalClient := campaignNewGraphClient
	t.Cleanup(func() {
		campaignLoadProfileCredentials = originalLoad
		campaignNewGraphClient = originalClient
	})

	campaignLoadProfileCredentials = loadFn
	campaignNewGraphClient = clientFn
}

func writeCampaignSchemaPack(t *testing.T) string {
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
  "entities":{"campaign":["id","name","status","objective","daily_budget","lifetime_budget"]},
  "endpoint_params":{"campaigns.post":["name","status","objective","daily_budget","lifetime_budget"]},
  "deprecated_params":{"campaigns.post":["legacy_param"]}
}`
	if err := os.WriteFile(packPath, []byte(pack), 0o644); err != nil {
		t.Fatalf("write schema pack: %v", err)
	}
	return schemaDir
}
