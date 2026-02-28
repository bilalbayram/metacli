package cmd

import (
	"bytes"
	"encoding/base64"
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

func TestNewCreativeCommandIncludesWorkflowSubcommands(t *testing.T) {
	t.Parallel()

	cmd := NewCreativeCommand(Runtime{})

	for _, name := range []string{"upload", "upload-video", "create"} {
		sub, _, err := cmd.Find([]string{name})
		if err != nil {
			t.Fatalf("find %s subcommand: %v", name, err)
		}
		if sub == nil || sub.Name() != name {
			t.Fatalf("expected %s subcommand, got %#v", name, sub)
		}
	}
}

func TestCreativeUploadExecutesMutation(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "creative.jpg")
	fileBytes := []byte("creative-image-bytes")
	if err := os.WriteFile(filePath, fileBytes, 0o644); err != nil {
		t.Fatalf("write upload file: %v", err)
	}

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"images":{"creative.jpg":{"hash":"img_hash_1","id":"img_1"}}}`,
	}
	useCreativeDependencies(t,
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
	cmd := NewCreativeCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"upload",
		"--account-id", "act_1234",
		"--file", filePath,
		"--name", "creative.jpg",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute creative upload: %v", err)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if requestURL.Path != "/v25.0/act_1234/adimages" {
		t.Fatalf("unexpected path %q", requestURL.Path)
	}

	form, err := url.ParseQuery(stub.lastBody)
	if err != nil {
		t.Fatalf("parse request form: %v", err)
	}
	if got := form.Get("filename"); got != "creative.jpg" {
		t.Fatalf("unexpected filename %q", got)
	}
	if got := form.Get("bytes"); got != base64.StdEncoding.EncodeToString(fileBytes) {
		t.Fatalf("unexpected bytes payload %q", got)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta creative upload")
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object data payload, got %T", envelope["data"])
	}
	if got := data["image_hash"]; got != "img_hash_1" {
		t.Fatalf("unexpected image_hash %v", got)
	}
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestCreativeUploadVideoWaitReadyExecutesUploadAndStatusPoll(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "launch.mp4")
	fileBytes := []byte("video-payload")
	if err := os.WriteFile(filePath, fileBytes, 0o644); err != nil {
		t.Fatalf("write upload video file: %v", err)
	}

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		switch requestCount {
		case 1:
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected upload method %q", r.Method)
			}
			if r.URL.Path != "/v25.0/act_1234/advideos" {
				t.Fatalf("unexpected upload path %q", r.URL.Path)
			}
			if err := r.ParseMultipartForm(4 << 20); err != nil {
				t.Fatalf("parse upload multipart form: %v", err)
			}
			if got := r.FormValue("name"); got != "launch.mp4" {
				t.Fatalf("unexpected video name %q", got)
			}
			file, header, err := r.FormFile("source")
			if err != nil {
				t.Fatalf("open source file field: %v", err)
			}
			defer file.Close()
			if header.Filename != "launch.mp4" {
				t.Fatalf("unexpected file name %q", header.Filename)
			}
			payload, err := io.ReadAll(file)
			if err != nil {
				t.Fatalf("read source payload: %v", err)
			}
			if string(payload) != string(fileBytes) {
				t.Fatalf("unexpected source payload %q", string(payload))
			}
			_, _ = w.Write([]byte(`{"id":"vid_64","uploading_phase":{"status":"COMPLETE"}}`))
		case 2:
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected status method %q", r.Method)
			}
			if r.URL.Path != "/v25.0/vid_64" {
				t.Fatalf("unexpected status path %q", r.URL.Path)
			}
			_, _ = w.Write([]byte(`{"id":"vid_64","status":{"video_status":"ready","processing_phase":{"status":"complete"},"publishing_phase":{"status":"complete"}}}`))
		default:
			t.Fatalf("unexpected request count %d", requestCount)
		}
	}))
	defer server.Close()

	useCreativeDependencies(t,
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
			client := graph.NewClient(server.Client(), server.URL)
			client.MaxRetries = 0
			return client
		},
	)

	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewCreativeCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"upload-video",
		"--account-id", "act_1234",
		"--file", filePath,
		"--name", "launch.mp4",
		"--wait-ready",
		"--timeout", "100ms",
		"--poll-interval", "1ms",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute creative upload-video: %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("expected two graph requests, got %d", requestCount)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta creative upload-video")
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object data payload, got %T", envelope["data"])
	}
	if got := data["video_id"]; got != "vid_64" {
		t.Fatalf("unexpected video_id %v", got)
	}
	if got := data["ready"]; got != true {
		t.Fatalf("unexpected ready flag %v", got)
	}
	if got := data["final_status"]; got != "complete" {
		t.Fatalf("unexpected final_status %v", got)
	}
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestCreativeCreateExecutesMutation(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"id":"crt_991","name":"Launch Creative"}`,
	}
	schemaDir := writeCreativeSchemaPack(t)
	useCreativeDependencies(t,
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
	cmd := NewCreativeCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"create",
		"--account-id", "1234",
		"--params", "name=Launch Creative,object_story_id=123_456",
		"--schema-dir", schemaDir,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute creative create: %v", err)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if requestURL.Path != "/v25.0/act_1234/adcreatives" {
		t.Fatalf("unexpected path %q", requestURL.Path)
	}
	form, err := url.ParseQuery(stub.lastBody)
	if err != nil {
		t.Fatalf("parse request body: %v", err)
	}
	if got := form.Get("name"); got != "Launch Creative" {
		t.Fatalf("unexpected name %q", got)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta creative create")
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object payload, got %T", envelope["data"])
	}
	if got := data["creative_id"]; got != "crt_991" {
		t.Fatalf("unexpected creative_id %v", got)
	}
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestCreativeCreateFailsLintValidation(t *testing.T) {
	wasCalled := false
	schemaDir := writeCreativeSchemaPack(t)
	useCreativeDependencies(t,
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
	cmd := NewCreativeCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"create",
		"--account-id", "1234",
		"--params", "name=Launch Creative,invalid_field=1",
		"--schema-dir", schemaDir,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "creative mutation lint failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if wasCalled {
		t.Fatal("graph client should not execute on lint failure")
	}
	if output.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", output.String())
	}

	envelope := decodeEnvelope(t, errOutput.Bytes())
	if got := envelope["command"]; got != "meta creative create" {
		t.Fatalf("unexpected command field %v", got)
	}
	if envelope["success"] != false {
		t.Fatalf("expected success=false, got %v", envelope["success"])
	}
}

func useCreativeDependencies(t *testing.T, loadFn func(string) (*ProfileCredentials, error), clientFn func() *graph.Client) {
	t.Helper()
	configureTestResourceLedgerPath(t)
	originalLoad := creativeLoadProfileCredentials
	originalClient := creativeNewGraphClient
	t.Cleanup(func() {
		creativeLoadProfileCredentials = originalLoad
		creativeNewGraphClient = originalClient
	})

	creativeLoadProfileCredentials = loadFn
	creativeNewGraphClient = clientFn
}

func writeCreativeSchemaPack(t *testing.T) string {
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
  "entities":{"creative":["id","name","object_story_id","object_story_spec","asset_feed_spec"]},
  "endpoint_params":{"adcreatives.post":["name","object_story_id","object_story_spec","asset_feed_spec","url_tags","degrees_of_freedom_spec"]},
  "deprecated_params":{"adcreatives.post":["legacy_param"]}
}`
	if err := os.WriteFile(packPath, []byte(pack), 0o644); err != nil {
		t.Fatalf("write schema pack: %v", err)
	}
	return schemaDir
}
