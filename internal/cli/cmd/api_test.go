package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
)

type stubHTTPClient struct {
	t *testing.T

	statusCode int
	response   string
	err        error

	calls      int
	lastMethod string
	lastURL    string
	lastBody   string
}

func (c *stubHTTPClient) Do(req *http.Request) (*http.Response, error) {
	c.calls++
	c.lastMethod = req.Method
	c.lastURL = req.URL.String()
	if req.Body != nil {
		body, readErr := io.ReadAll(req.Body)
		if readErr != nil {
			c.t.Fatalf("read request body: %v", readErr)
		}
		c.lastBody = string(body)
	}

	if c.err != nil {
		return nil, c.err
	}

	return &http.Response{
		StatusCode: c.statusCode,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(c.response)),
	}, nil
}

func TestNewAPICommandAddsPostAndDelete(t *testing.T) {
	runtime := testRuntime("prod")
	cmd := NewAPICommand(runtime)

	post, _, postErr := cmd.Find([]string{"post"})
	if postErr != nil {
		t.Fatalf("find post subcommand: %v", postErr)
	}
	if post == nil || post.Name() != "post" {
		t.Fatalf("expected post subcommand, got %v", post)
	}

	deleteCmd, _, deleteErr := cmd.Find([]string{"delete"})
	if deleteErr != nil {
		t.Fatalf("find delete subcommand: %v", deleteErr)
	}
	if deleteCmd == nil || deleteCmd.Name() != "delete" {
		t.Fatalf("expected delete subcommand, got %v", deleteCmd)
	}
}

func TestAPIPostBuildsFormPayloadFromParamsAndJSON(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"id":"42","status":"ACTIVE"}`,
	}
	useStubDependencies(t, stub)

	cmd := newAPIPostCommand(testRuntime("prod"))
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"act_123/campaigns",
		"--params", "name=My Campaign,status=PAUSED",
		"--json", `{"daily_budget":1000,"tracking_specs":[{"action.type":"offsite_conversion"}]}`,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute post command: %v", err)
	}

	if stub.calls != 1 {
		t.Fatalf("expected one graph call, got %d", stub.calls)
	}
	if stub.lastMethod != http.MethodPost {
		t.Fatalf("unexpected method %q", stub.lastMethod)
	}

	parsedURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if parsedURL.Path != "/v25.0/act_123/campaigns" {
		t.Fatalf("unexpected request path %q", parsedURL.Path)
	}
	if parsedURL.RawQuery != "" {
		t.Fatalf("expected empty query string, got %q", parsedURL.RawQuery)
	}

	formValues, err := url.ParseQuery(stub.lastBody)
	if err != nil {
		t.Fatalf("parse form body: %v", err)
	}
	if got := formValues.Get("name"); got != "My Campaign" {
		t.Fatalf("unexpected form value name=%q", got)
	}
	if got := formValues.Get("status"); got != "PAUSED" {
		t.Fatalf("unexpected form value status=%q", got)
	}
	if got := formValues.Get("daily_budget"); got != "1000" {
		t.Fatalf("unexpected form value daily_budget=%q", got)
	}
	if got := formValues.Get("tracking_specs"); got != `[{"action.type":"offsite_conversion"}]` {
		t.Fatalf("unexpected form value tracking_specs=%q", got)
	}
	if got := formValues.Get("access_token"); got != "test-token" {
		t.Fatalf("unexpected access token value %q", got)
	}
	if got := formValues.Get("appsecret_proof"); got == "" {
		t.Fatal("expected appsecret_proof to be set")
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta api post")

	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object data payload, got %T", envelope["data"])
	}
	if got, _ := data["id"].(string); got != "42" {
		t.Fatalf("unexpected response id %q", got)
	}
}

func TestAPIDeleteBuildsQueryPayloadFromParams(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"success":true}`,
	}
	useStubDependencies(t, stub)

	cmd := newAPIDeleteCommand(testRuntime("prod"))
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"act_123/campaigns",
		"--params", "status=PAUSED,hard_delete=true",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute delete command: %v", err)
	}

	if stub.calls != 1 {
		t.Fatalf("expected one graph call, got %d", stub.calls)
	}
	if stub.lastMethod != http.MethodDelete {
		t.Fatalf("unexpected method %q", stub.lastMethod)
	}
	if stub.lastBody != "" {
		t.Fatalf("expected empty request body for delete, got %q", stub.lastBody)
	}

	parsedURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if parsedURL.Path != "/v25.0/act_123/campaigns" {
		t.Fatalf("unexpected request path %q", parsedURL.Path)
	}
	query := parsedURL.Query()
	if got := query.Get("status"); got != "PAUSED" {
		t.Fatalf("unexpected query value status=%q", got)
	}
	if got := query.Get("hard_delete"); got != "true" {
		t.Fatalf("unexpected query value hard_delete=%q", got)
	}
	if got := query.Get("access_token"); got != "test-token" {
		t.Fatalf("unexpected access token value %q", got)
	}
	if got := query.Get("appsecret_proof"); got == "" {
		t.Fatal("expected appsecret_proof query value")
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta api delete")
}

func TestAPIPostReturnsNormalizedGraphError(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusBadRequest,
		response:   `{"error":{"message":"Unsupported post request","type":"GraphMethodException","code":100,"error_subcode":33,"fbtrace_id":"trace-1"}}`,
	}
	useStubDependencies(t, stub)

	cmd := newAPIPostCommand(testRuntime("prod"))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"act_123/campaigns"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	want := "meta api error type=GraphMethodException code=100 subcode=33 fbtrace_id=trace-1: Unsupported post request"
	if err.Error() != want {
		t.Fatalf("unexpected error message:\n got: %q\nwant: %q", err.Error(), want)
	}
}

func TestAPIPostRejectsInvalidInlineJSON(t *testing.T) {
	wasCalled := false
	useDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name:      "prod",
				Profile:   config.Profile{GraphVersion: "v25.0"},
				Token:     "test-token",
				AppSecret: "test-secret",
			}, nil
		},
		func() *graph.Client {
			wasCalled = true
			return graph.NewClient(nil, "")
		},
	)

	cmd := newAPIPostCommand(testRuntime("prod"))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"act_123/campaigns",
		"--json", `{"name":"broken",`,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "decode --json payload") {
		t.Fatalf("expected json decode error, got %q", err.Error())
	}
	if wasCalled {
		t.Fatal("graph client should not be called when --json is invalid")
	}
}

func useStubDependencies(t *testing.T, httpClient graph.HTTPClient) {
	t.Helper()
	useDependencies(t,
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

func useDependencies(t *testing.T, loadFn func(string) (*ProfileCredentials, error), clientFn func() *graph.Client) {
	t.Helper()
	originalLoad := apiLoadProfileCredentials
	originalClient := apiNewGraphClient
	t.Cleanup(func() {
		apiLoadProfileCredentials = originalLoad
		apiNewGraphClient = originalClient
	})

	apiLoadProfileCredentials = loadFn
	apiNewGraphClient = clientFn
}

func testRuntime(profile string) Runtime {
	output := "json"
	debug := false
	return Runtime{
		Profile: &profile,
		Output:  &output,
		Debug:   &debug,
	}
}

func decodeEnvelope(t *testing.T, raw []byte) map[string]any {
	t.Helper()

	decoded := map[string]any{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return decoded
}

func assertEnvelopeBasics(t *testing.T, envelope map[string]any, command string) {
	t.Helper()

	if got, _ := envelope["command"].(string); got != command {
		t.Fatalf("unexpected envelope command %q", got)
	}
	success, ok := envelope["success"].(bool)
	if !ok {
		t.Fatalf("expected success bool, got %T", envelope["success"])
	}
	if !success {
		t.Fatalf("expected successful envelope, got %+v", envelope)
	}
}
