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

func TestNewIGCommandIncludesMediaUploadAndStatusSubcommands(t *testing.T) {

	cmd := NewIGCommand(Runtime{})

	mediaCmd, _, err := cmd.Find([]string{"media"})
	if err != nil {
		t.Fatalf("find media command: %v", err)
	}
	if mediaCmd == nil || mediaCmd.Name() != "media" {
		t.Fatalf("expected media command, got %#v", mediaCmd)
	}

	uploadCmd, _, err := cmd.Find([]string{"media", "upload"})
	if err != nil {
		t.Fatalf("find media upload command: %v", err)
	}
	if uploadCmd == nil || uploadCmd.Name() != "upload" {
		t.Fatalf("expected media upload command, got %#v", uploadCmd)
	}

	statusCmd, _, err := cmd.Find([]string{"media", "status"})
	if err != nil {
		t.Fatalf("find media status command: %v", err)
	}
	if statusCmd == nil || statusCmd.Name() != "status" {
		t.Fatalf("expected media status command, got %#v", statusCmd)
	}
}

func TestIGMediaUploadCommandExecutesShapedRequest(t *testing.T) {

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"id":"creation_99","status_code":"IN_PROGRESS"}`,
	}
	useIGDependencies(t,
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
		"media", "upload",
		"--ig-user-id", "17841400008460056",
		"--media-url", "https://cdn.example.com/image.jpg",
		"--caption", "hello",
		"--media-type", "IMAGE",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute ig media upload: %v", err)
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
	if parsedURL.Path != "/v25.0/17841400008460056/media" {
		t.Fatalf("unexpected request path %q", parsedURL.Path)
	}
	form, err := url.ParseQuery(stub.lastBody)
	if err != nil {
		t.Fatalf("parse form body: %v", err)
	}
	if got := form.Get("image_url"); got != "https://cdn.example.com/image.jpg" {
		t.Fatalf("unexpected image_url %q", got)
	}
	if got := form.Get("caption"); got != "hello" {
		t.Fatalf("unexpected caption %q", got)
	}
	if got := form.Get("access_token"); got != "test-token" {
		t.Fatalf("unexpected access_token %q", got)
	}
	if got := form.Get("appsecret_proof"); got == "" {
		t.Fatal("expected appsecret_proof")
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta ig media upload")
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object payload, got %T", envelope["data"])
	}
	if got := data["creation_id"]; got != "creation_99" {
		t.Fatalf("unexpected creation id %v", got)
	}
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestIGMediaStatusCommandExecutesShapedRequest(t *testing.T) {

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"id":"creation_99","status":"FINISHED","status_code":"FINISHED"}`,
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
		"media", "status",
		"--creation-id", "creation_99",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute ig media status: %v", err)
	}

	if stub.calls != 1 {
		t.Fatalf("expected one graph call, got %d", stub.calls)
	}
	if stub.lastMethod != http.MethodGet {
		t.Fatalf("unexpected method %q", stub.lastMethod)
	}

	parsedURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if parsedURL.Path != "/v25.0/creation_99" {
		t.Fatalf("unexpected request path %q", parsedURL.Path)
	}
	query := parsedURL.Query()
	if got := query.Get("fields"); got != "id,status,status_code" {
		t.Fatalf("unexpected fields query %q", got)
	}
	if got := query.Get("access_token"); got != "test-token" {
		t.Fatalf("unexpected access token %q", got)
	}
	if got := query.Get("appsecret_proof"); got == "" {
		t.Fatal("expected appsecret_proof query value")
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta ig media status")
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object payload, got %T", envelope["data"])
	}
	if got := data["status"]; got != "FINISHED" {
		t.Fatalf("unexpected status %v", got)
	}
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestIGMediaUploadWritesStructuredErrorOnValidationFailure(t *testing.T) {

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
		"media", "upload",
		"--ig-user-id", "17841400008460056",
		"--media-type", "IMAGE",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "media url is required") {
		t.Fatalf("unexpected error: %v", err)
	}
	if wasCalled {
		t.Fatal("graph client should not execute on validation failure")
	}
	if output.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", output.String())
	}

	envelope := decodeEnvelope(t, errOutput.Bytes())
	if got := envelope["command"]; got != "meta ig media upload" {
		t.Fatalf("unexpected command field %v", got)
	}
	if envelope["success"] != false {
		t.Fatalf("expected success=false, got %v", envelope["success"])
	}
	errorBody, ok := envelope["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %T", envelope["error"])
	}
	message, _ := errorBody["message"].(string)
	if !strings.Contains(message, "media url is required") {
		t.Fatalf("unexpected error message %q", message)
	}
}

func useIGDependencies(t *testing.T, loadFn func(string) (*ProfileCredentials, error), clientFn func() *graph.Client) {
	t.Helper()
	originalLoad := igLoadProfileCredentials
	originalClient := igNewGraphClient
	t.Cleanup(func() {
		igLoadProfileCredentials = originalLoad
		igNewGraphClient = originalClient
	})

	igLoadProfileCredentials = loadFn
	igNewGraphClient = clientFn
}

func TestIGMediaStatusWritesStructuredErrorWhenCreationIDMissing(t *testing.T) {

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
	cmd.SetArgs([]string{"media", "status"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "creation id is required") {
		t.Fatalf("unexpected error: %v", err)
	}

	envelope := decodeEnvelope(t, errOutput.Bytes())
	if got := envelope["command"]; got != "meta ig media status" {
		t.Fatalf("unexpected command field %v", got)
	}
	if envelope["success"] != false {
		t.Fatalf("expected success=false, got %v", envelope["success"])
	}
}

func TestIGMediaCommandFailsWithoutSubcommand(t *testing.T) {

	cmd := NewIGCommand(Runtime{})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"media"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "ig media requires a subcommand") {
		t.Fatalf("unexpected error: %v", err)
	}
}
