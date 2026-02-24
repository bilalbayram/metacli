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
	"slices"
	"strings"
	"testing"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
)

func TestNewAdCommandIncludesLifecycleSubcommands(t *testing.T) {
	t.Parallel()

	cmd := NewAdCommand(Runtime{})

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

func TestAdCreateValidatesDependenciesAndExecutesMutation(t *testing.T) {
	schemaDir := writeAdSchemaPack(t)

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		switch requestCount {
		case 1:
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected adset validation method %s", r.Method)
			}
			if r.URL.Path != "/v25.0/adset_1" {
				t.Fatalf("unexpected adset validation path %q", r.URL.Path)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "adset_1"})
		case 2:
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected creative validation method %s", r.Method)
			}
			if r.URL.Path != "/v25.0/creative_1" {
				t.Fatalf("unexpected creative validation path %q", r.URL.Path)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "creative_1"})
		case 3:
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected create method %s", r.Method)
			}
			if r.URL.Path != "/v25.0/act_1234/ads" {
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
			if got := form.Get("adset_id"); got != "adset_1" {
				t.Fatalf("unexpected adset_id %q", got)
			}
			if got := form.Get("creative"); got != `{"creative_id":"creative_1"}` {
				t.Fatalf("unexpected creative payload %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ad_501"})
		default:
			t.Fatalf("unexpected request count %d", requestCount)
		}
	}))
	defer server.Close()

	useAdDependencies(t,
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
	cmd := NewAdCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"create",
		"--account-id", "1234",
		"--params", "name=Creative Iteration A,adset_id=adset_1,status=PAUSED",
		"--json", `{"creative":{"creative_id":"creative_1"}}`,
		"--schema-dir", schemaDir,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute ad create: %v", err)
	}

	if requestCount != 3 {
		t.Fatalf("expected three requests, got %d", requestCount)
	}
	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta ad create")
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object payload, got %T", envelope["data"])
	}
	if got := data["ad_id"]; got != "ad_501" {
		t.Fatalf("unexpected ad id %v", got)
	}
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestAdCreateFailsWhenDependencyValidationReturnsGraphError(t *testing.T) {
	schemaDir := writeAdSchemaPack(t)

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount != 1 {
			t.Fatalf("unexpected request count %d", requestCount)
		}
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message":       "Unsupported get request",
				"type":          "GraphMethodException",
				"code":          100,
				"error_subcode": 33,
				"fbtrace_id":    "trace-123",
			},
		})
	}))
	defer server.Close()

	useAdDependencies(t,
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
	cmd := NewAdCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"create",
		"--account-id", "1234",
		"--params", "name=Creative Iteration A,adset_id=adset_1",
		"--json", `{"creative":{"creative_id":"creative_1"}}`,
		"--schema-dir", schemaDir,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "validate ad set reference") {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", output.String())
	}
	if requestCount != 1 {
		t.Fatalf("expected one request, got %d", requestCount)
	}

	envelope := decodeEnvelope(t, errOutput.Bytes())
	if got := envelope["command"]; got != "meta ad create" {
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
}

func TestAdUpdateFailsOnInvalidCreativeReferenceBeforeAPI(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"success":true}`,
	}
	schemaDir := writeAdSchemaPack(t)
	useAdDependencies(t,
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
	cmd := NewAdCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"update",
		"--ad-id", "ad_77",
		"--json", `{"creative":{"object_story_spec":{"page_id":"1"}}}`,
		"--schema-dir", schemaDir,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "creative reference must include creative_id or id") {
		t.Fatalf("unexpected error: %v", err)
	}
	if stub.calls != 0 {
		t.Fatalf("expected zero graph requests, got %d", stub.calls)
	}
	if output.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", output.String())
	}
	envelope := decodeEnvelope(t, errOutput.Bytes())
	if got := envelope["command"]; got != "meta ad update" {
		t.Fatalf("unexpected command field %v", got)
	}
}

func TestAdPauseExecutesMutation(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"success":true}`,
	}
	schemaDir := writeAdSchemaPack(t)
	useAdDependencies(t,
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
	cmd := NewAdCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"pause",
		"--ad-id", "ad_88",
		"--schema-dir", schemaDir,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute ad pause: %v", err)
	}

	form, err := url.ParseQuery(stub.lastBody)
	if err != nil {
		t.Fatalf("parse request form: %v", err)
	}
	if got := form.Get("status"); got != "PAUSED" {
		t.Fatalf("unexpected status %q", got)
	}
	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta ad pause")
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestAdCloneExecutesReadSanitizeValidateCreateFlow(t *testing.T) {
	schemaDir := writeAdSchemaPack(t)

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		switch requestCount {
		case 1:
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected read method %s", r.Method)
			}
			if r.URL.Path != "/v25.0/source_ad_1" {
				t.Fatalf("unexpected read path %q", r.URL.Path)
			}
			if got := r.URL.Query().Get("fields"); got != "id,name,status,adset_id,creative" {
				t.Fatalf("unexpected read fields %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "source_ad_1",
				"name":     "Source Ad",
				"status":   "PAUSED",
				"adset_id": "adset_9",
				"creative": map[string]any{"id": "creative_9"},
			})
		case 2:
			if r.URL.Path != "/v25.0/adset_9" {
				t.Fatalf("unexpected adset validation path %q", r.URL.Path)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "adset_9"})
		case 3:
			if r.URL.Path != "/v25.0/creative_9" {
				t.Fatalf("unexpected creative validation path %q", r.URL.Path)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "creative_9"})
		case 4:
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected create method %s", r.Method)
			}
			if r.URL.Path != "/v25.0/act_222/ads" {
				t.Fatalf("unexpected create path %q", r.URL.Path)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read create body: %v", err)
			}
			form, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("parse create body: %v", err)
			}
			if got := form.Get("name"); got != "Cloned Ad" {
				t.Fatalf("unexpected cloned name %q", got)
			}
			if got := form.Get("adset_id"); got != "adset_9" {
				t.Fatalf("unexpected adset_id %q", got)
			}
			if got := form.Get("creative"); got != `{"creative_id":"creative_9"}` {
				t.Fatalf("unexpected creative payload %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "clone_ad_2"})
		default:
			t.Fatalf("unexpected request count %d", requestCount)
		}
	}))
	defer server.Close()

	useAdDependencies(t,
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
	cmd := NewAdCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"clone",
		"--source-ad-id", "source_ad_1",
		"--account-id", "222",
		"--params", "name=Cloned Ad",
		"--schema-dir", schemaDir,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute ad clone: %v", err)
	}

	if requestCount != 4 {
		t.Fatalf("expected four requests, got %d", requestCount)
	}
	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta ad clone")
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object payload, got %T", envelope["data"])
	}
	if got := data["ad_id"]; got != "clone_ad_2" {
		t.Fatalf("unexpected ad id %v", got)
	}
	if got := data["source_ad_id"]; got != "source_ad_1" {
		t.Fatalf("unexpected source ad id %v", got)
	}
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestAdCloneReturnsRemediationForIncompleteClonePayload(t *testing.T) {
	schemaDir := writeAdSchemaPack(t)

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount != 1 {
			t.Fatalf("unexpected request count %d", requestCount)
		}
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected read method %s", r.Method)
		}
		if r.URL.Path != "/v25.0/source_ad_3" {
			t.Fatalf("unexpected read path %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("fields"); got != "name,adset_id,creative" {
			t.Fatalf("unexpected read fields %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":     "Source Ad",
			"adset_id": "adset_11",
		})
	}))
	defer server.Close()

	useAdDependencies(t,
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
	cmd := NewAdCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"clone",
		"--source-ad-id", "source_ad_3",
		"--account-id", "222",
		"--fields", "name",
		"--schema-dir", schemaDir,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "ad clone payload is incomplete") {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", output.String())
	}
	if requestCount != 1 {
		t.Fatalf("expected only source read request, got %d", requestCount)
	}

	envelope := decodeEnvelope(t, errOutput.Bytes())
	if got := envelope["command"]; got != "meta ad clone" {
		t.Fatalf("unexpected command field %v", got)
	}
	if envelope["success"] != false {
		t.Fatalf("expected success=false, got %v", envelope["success"])
	}
	errorBody, ok := envelope["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error payload, got %T", envelope["error"])
	}
	if got := errorBody["type"]; got != "ad_validation_error" {
		t.Fatalf("unexpected error type %v", got)
	}
	if got := errorBody["code"]; got != float64(422100) {
		t.Fatalf("unexpected error code %v", got)
	}
	remediation, ok := errorBody["remediation"].(map[string]any)
	if !ok {
		t.Fatalf("expected remediation payload, got %T", errorBody["remediation"])
	}
	if got := remediation["category"]; got != "validation" {
		t.Fatalf("unexpected remediation category %v", got)
	}
	fieldsAny, ok := remediation["fields"].([]any)
	if !ok {
		t.Fatalf("expected remediation fields, got %T", remediation["fields"])
	}
	fields := make([]string, 0, len(fieldsAny))
	for _, field := range fieldsAny {
		typed, ok := field.(string)
		if !ok {
			t.Fatalf("expected string remediation field, got %T", field)
		}
		fields = append(fields, typed)
	}
	if !slices.Equal(fields, []string{"creative"}) {
		t.Fatalf("unexpected remediation fields %v", fields)
	}
	actionsAny, ok := remediation["actions"].([]any)
	if !ok {
		t.Fatalf("expected remediation actions, got %T", remediation["actions"])
	}
	actions := make([]string, 0, len(actionsAny))
	for _, action := range actionsAny {
		typed, ok := action.(string)
		if !ok {
			t.Fatalf("expected string remediation action, got %T", action)
		}
		actions = append(actions, typed)
	}
	if !slices.Contains(actions, "Include the missing fields in --fields or provide overrides via --params/--json.") {
		t.Fatalf("unexpected remediation actions %v", actions)
	}
}

func TestAdResumeReturnsStructuredGraphError(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusBadRequest,
		response:   `{"error":{"message":"Unsupported post request","type":"GraphMethodException","code":100,"error_subcode":33,"fbtrace_id":"trace-123"}}`,
	}
	schemaDir := writeAdSchemaPack(t)
	useAdDependencies(t,
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
	cmd := NewAdCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"resume",
		"--ad-id", "ad_88",
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
	if got := envelope["command"]; got != "meta ad resume" {
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
}

func useAdDependencies(t *testing.T, loadFn func(string) (*ProfileCredentials, error), clientFn func() *graph.Client) {
	t.Helper()
	originalLoad := adLoadProfileCredentials
	originalClient := adNewGraphClient
	t.Cleanup(func() {
		adLoadProfileCredentials = originalLoad
		adNewGraphClient = originalClient
	})

	adLoadProfileCredentials = loadFn
	adNewGraphClient = clientFn
}

func writeAdSchemaPack(t *testing.T) string {
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
  "entities":{"ad":["id","name","status","adset_id","creative"]},
  "endpoint_params":{"ads.post":["name","adset_id","status","creative"]},
  "deprecated_params":{"ads.post":["legacy_param"]}
}`
	if err := os.WriteFile(packPath, []byte(pack), 0o644); err != nil {
		t.Fatalf("write schema pack: %v", err)
	}
	return schemaDir
}
