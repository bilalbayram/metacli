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

type igPublishSequenceResponse struct {
	statusCode int
	response   string
	err        error
}

type igPublishCapturedCall struct {
	method string
	url    string
	body   string
}

type igPublishSequenceHTTPClient struct {
	t *testing.T

	responses []igPublishSequenceResponse
	calls     []igPublishCapturedCall
}

func (c *igPublishSequenceHTTPClient) Do(req *http.Request) (*http.Response, error) {
	body := ""
	if req.Body != nil {
		rawBody, readErr := io.ReadAll(req.Body)
		if readErr != nil {
			c.t.Fatalf("read request body: %v", readErr)
		}
		body = string(rawBody)
	}
	c.calls = append(c.calls, igPublishCapturedCall{
		method: req.Method,
		url:    req.URL.String(),
		body:   body,
	})

	if len(c.responses) == 0 {
		c.t.Fatal("unexpected graph request: no stubbed responses remaining")
	}
	response := c.responses[0]
	c.responses = c.responses[1:]
	if response.err != nil {
		return nil, response.err
	}

	return &http.Response{
		StatusCode: response.statusCode,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(response.response)),
	}, nil
}

func TestNewIGCommandIncludesPublishFeedSubcommands(t *testing.T) {
	t.Parallel()

	cmd := NewIGCommand(Runtime{})

	publishCmd, _, err := cmd.Find([]string{"publish"})
	if err != nil {
		t.Fatalf("find publish command: %v", err)
	}
	if publishCmd == nil || publishCmd.Name() != "publish" {
		t.Fatalf("expected publish command, got %#v", publishCmd)
	}

	feedCmd, _, err := cmd.Find([]string{"publish", "feed"})
	if err != nil {
		t.Fatalf("find publish feed command: %v", err)
	}
	if feedCmd == nil || feedCmd.Name() != "feed" {
		t.Fatalf("expected publish feed command, got %#v", feedCmd)
	}
}

func TestIGPublishFeedCommandExecutesImmediateFlow(t *testing.T) {
	stub := &igPublishSequenceHTTPClient{
		t: t,
		responses: []igPublishSequenceResponse{
			{
				statusCode: http.StatusOK,
				response:   `{"id":"creation_99","status_code":"IN_PROGRESS"}`,
			},
			{
				statusCode: http.StatusOK,
				response:   `{"id":"creation_99","status":"FINISHED","status_code":"FINISHED"}`,
			},
			{
				statusCode: http.StatusOK,
				response:   `{"id":"media_44"}`,
			},
		},
	}
	useIGDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name:      "prod",
				Profile:   config.Profile{GraphVersion: "v25.0"},
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
	cmd := NewIGCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"publish", "feed",
		"--ig-user-id", "17841400008460056",
		"--media-url", "https://cdn.example.com/image.jpg",
		"--caption", "hello #meta",
		"--media-type", "IMAGE",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute ig publish feed: %v", err)
	}

	if len(stub.calls) != 3 {
		t.Fatalf("expected three graph calls, got %d", len(stub.calls))
	}

	uploadURL, err := url.Parse(stub.calls[0].url)
	if err != nil {
		t.Fatalf("parse upload url: %v", err)
	}
	if uploadURL.Path != "/v25.0/17841400008460056/media" {
		t.Fatalf("unexpected upload path %q", uploadURL.Path)
	}

	statusURL, err := url.Parse(stub.calls[1].url)
	if err != nil {
		t.Fatalf("parse status url: %v", err)
	}
	if statusURL.Path != "/v25.0/creation_99" {
		t.Fatalf("unexpected status path %q", statusURL.Path)
	}
	if got := statusURL.Query().Get("fields"); got != "id,status,status_code" {
		t.Fatalf("unexpected status fields query %q", got)
	}

	publishURL, err := url.Parse(stub.calls[2].url)
	if err != nil {
		t.Fatalf("parse publish url: %v", err)
	}
	if publishURL.Path != "/v25.0/17841400008460056/media_publish" {
		t.Fatalf("unexpected publish path %q", publishURL.Path)
	}
	publishForm, err := url.ParseQuery(stub.calls[2].body)
	if err != nil {
		t.Fatalf("parse publish form: %v", err)
	}
	if got := publishForm.Get("creation_id"); got != "creation_99" {
		t.Fatalf("unexpected creation_id %q", got)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta ig publish feed")
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object payload, got %T", envelope["data"])
	}
	if got := data["mode"]; got != "immediate" {
		t.Fatalf("unexpected mode %v", got)
	}
	if got := data["creation_id"]; got != "creation_99" {
		t.Fatalf("unexpected creation_id %v", got)
	}
	if got := data["media_id"]; got != "media_44" {
		t.Fatalf("unexpected media_id %v", got)
	}
	if got := data["status_code"]; got != "FINISHED" {
		t.Fatalf("unexpected status_code %v", got)
	}
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestIGPublishFeedCommandWritesStructuredErrorWhenNotReady(t *testing.T) {
	stub := &igPublishSequenceHTTPClient{
		t: t,
		responses: []igPublishSequenceResponse{
			{
				statusCode: http.StatusOK,
				response:   `{"id":"creation_99","status_code":"IN_PROGRESS"}`,
			},
			{
				statusCode: http.StatusOK,
				response:   `{"id":"creation_99","status":"IN_PROGRESS","status_code":"IN_PROGRESS"}`,
			},
		},
	}
	useIGDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name:      "prod",
				Profile:   config.Profile{GraphVersion: "v25.0"},
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
	cmd := NewIGCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"publish", "feed",
		"--ig-user-id", "17841400008460056",
		"--media-url", "https://cdn.example.com/image.jpg",
		"--caption", "hello #meta",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "not ready for publish") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stub.calls) != 2 {
		t.Fatalf("expected upload+status calls only, got %d", len(stub.calls))
	}

	envelope := decodeEnvelope(t, errOutput.Bytes())
	if got := envelope["command"]; got != "meta ig publish feed" {
		t.Fatalf("unexpected command field %v", got)
	}
	if envelope["success"] != false {
		t.Fatalf("expected success=false, got %v", envelope["success"])
	}
}

func TestIGPublishFeedCommandFailsFastOnCaptionValidation(t *testing.T) {
	wasCalled := false
	useIGDependencies(t,
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

	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewIGCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"publish", "feed",
		"--ig-user-id", "17841400008460056",
		"--media-url", "https://cdn.example.com/image.jpg",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "caption is required") {
		t.Fatalf("unexpected error: %v", err)
	}
	if wasCalled {
		t.Fatal("graph client should not execute on caption validation failure")
	}
	if output.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", output.String())
	}

	envelope := decodeEnvelope(t, errOutput.Bytes())
	if got := envelope["command"]; got != "meta ig publish feed" {
		t.Fatalf("unexpected command field %v", got)
	}
	if envelope["success"] != false {
		t.Fatalf("expected success=false, got %v", envelope["success"])
	}
}

func TestIGPublishCommandFailsWithoutSubcommand(t *testing.T) {
	cmd := NewIGCommand(Runtime{})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"publish"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "ig publish requires a subcommand") {
		t.Fatalf("unexpected error: %v", err)
	}
}
