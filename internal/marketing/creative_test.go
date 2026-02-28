package marketing

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bilalbayram/metacli/internal/graph"
)

func TestCreativeUploadEncodesFileAndCallsAdImages(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "creative.jpg")
	fileBytes := []byte("image-bytes-123")
	if err := os.WriteFile(filePath, fileBytes, 0o644); err != nil {
		t.Fatalf("write upload file: %v", err)
	}

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"images":{"creative.jpg":{"hash":"img_hash_1","id":"img_1","url":"https://cdn.example.com/img"}}}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewCreativeService(client)

	result, err := service.Upload(context.Background(), "v25.0", "token-1", "secret-1", CreativeUploadInput{
		AccountID: "act_1234",
		FilePath:  filePath,
		FileName:  "creative.jpg",
	})
	if err != nil {
		t.Fatalf("upload creative image: %v", err)
	}

	if stub.calls != 1 {
		t.Fatalf("expected one call, got %d", stub.calls)
	}
	if stub.lastMethod != http.MethodPost {
		t.Fatalf("unexpected method %q", stub.lastMethod)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if requestURL.Path != "/v25.0/act_1234/adimages" {
		t.Fatalf("unexpected request path %q", requestURL.Path)
	}

	form, err := url.ParseQuery(stub.lastBody)
	if err != nil {
		t.Fatalf("parse form body: %v", err)
	}
	if got := form.Get("filename"); got != "creative.jpg" {
		t.Fatalf("unexpected filename %q", got)
	}
	if got := form.Get("bytes"); got != base64.StdEncoding.EncodeToString(fileBytes) {
		t.Fatalf("unexpected bytes payload %q", got)
	}

	if result.Operation != "upload" {
		t.Fatalf("unexpected operation %q", result.Operation)
	}
	if result.AccountID != "act_1234" {
		t.Fatalf("unexpected account id %q", result.AccountID)
	}
	if result.ImageHash != "img_hash_1" {
		t.Fatalf("unexpected image hash %q", result.ImageHash)
	}
}

func TestCreativeUploadFailsWhenResponseMissingImages(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "creative.jpg")
	if err := os.WriteFile(filePath, []byte("bytes"), 0o644); err != nil {
		t.Fatalf("write upload file: %v", err)
	}

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"success":true}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewCreativeService(client)

	_, err := service.Upload(context.Background(), "v25.0", "token-1", "secret-1", CreativeUploadInput{
		AccountID: "1234",
		FilePath:  filePath,
	})
	if err == nil {
		t.Fatal("expected upload error")
	}
	if !strings.Contains(err.Error(), "did not include images") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreativeCreateExecutesGraphMutation(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"id":"9001","name":"Creative 1"}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewCreativeService(client)

	result, err := service.Create(context.Background(), "v25.0", "token-1", "secret-1", CreativeCreateInput{
		AccountID: "act_1234",
		Params: map[string]string{
			"name":            "Creative 1",
			"object_story_id": "123_456",
		},
	})
	if err != nil {
		t.Fatalf("create creative: %v", err)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if requestURL.Path != "/v25.0/act_1234/adcreatives" {
		t.Fatalf("unexpected request path %q", requestURL.Path)
	}

	form, err := url.ParseQuery(stub.lastBody)
	if err != nil {
		t.Fatalf("parse form body: %v", err)
	}
	if got := form.Get("name"); got != "Creative 1" {
		t.Fatalf("unexpected name %q", got)
	}
	if got := form.Get("object_story_id"); got != "123_456" {
		t.Fatalf("unexpected object_story_id %q", got)
	}

	if result.Operation != "create" {
		t.Fatalf("unexpected operation %q", result.Operation)
	}
	if result.CreativeID != "9001" {
		t.Fatalf("unexpected creative id %q", result.CreativeID)
	}
}

func TestCreativeCreateRejectsEmptyPayload(t *testing.T) {
	t.Parallel()

	service := NewCreativeService(graph.NewClient(nil, ""))
	_, err := service.Create(context.Background(), "v25.0", "token-1", "secret-1", CreativeCreateInput{
		AccountID: "1234",
		Params:    map[string]string{},
	})
	if err == nil {
		t.Fatal("expected create error")
	}
	if !strings.Contains(err.Error(), "payload cannot be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreativeCreateFailsWhenResponseMissingID(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"success":true}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewCreativeService(client)

	_, err := service.Create(context.Background(), "v25.0", "token-1", "secret-1", CreativeCreateInput{
		AccountID: "1234",
		Params: map[string]string{
			"name": "Creative 1",
		},
	})
	if err == nil {
		t.Fatal("expected create error")
	}
	if !strings.Contains(err.Error(), "did not include id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreativeUploadVideoWaitReadyPollsUntilReady(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "video.mp4")
	fileBytes := []byte("video-bytes-123")
	if err := os.WriteFile(filePath, fileBytes, 0o644); err != nil {
		t.Fatalf("write video file: %v", err)
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
			if got := r.FormValue("name"); got != "video.mp4" {
				t.Fatalf("unexpected name %q", got)
			}
			file, header, err := r.FormFile("source")
			if err != nil {
				t.Fatalf("read source form file: %v", err)
			}
			defer file.Close()
			if header.Filename != "video.mp4" {
				t.Fatalf("unexpected uploaded file name %q", header.Filename)
			}
			payload, err := io.ReadAll(file)
			if err != nil {
				t.Fatalf("read uploaded payload: %v", err)
			}
			if string(payload) != string(fileBytes) {
				t.Fatalf("unexpected uploaded payload %q", string(payload))
			}
			_, _ = w.Write([]byte(`{"id":"vid_1","uploading_phase":{"status":"complete"}}`))
		case 2:
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected status method %q", r.Method)
			}
			if r.URL.Path != "/v25.0/vid_1" {
				t.Fatalf("unexpected status path %q", r.URL.Path)
			}
			if got := r.URL.Query().Get("fields"); got != "id,status" {
				t.Fatalf("unexpected status fields %q", got)
			}
			_, _ = w.Write([]byte(`{"id":"vid_1","status":{"processing_phase":{"status":"IN_PROGRESS"}}}`))
		case 3:
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected status method %q", r.Method)
			}
			_, _ = w.Write([]byte(`{"id":"vid_1","status":{"video_status":"ready","processing_phase":{"status":"complete"},"publishing_phase":{"status":"complete"}}}`))
		default:
			t.Fatalf("unexpected request count %d", requestCount)
		}
	}))
	defer server.Close()

	client := graph.NewClient(server.Client(), server.URL)
	client.MaxRetries = 0
	service := NewCreativeService(client)
	result, err := service.UploadVideo(context.Background(), "v25.0", "token-1", "secret-1", CreativeVideoUploadInput{
		AccountID:    "act_1234",
		FilePath:     filePath,
		FileName:     "video.mp4",
		WaitReady:    true,
		Timeout:      100 * time.Millisecond,
		PollInterval: 1 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("upload video with wait ready: %v", err)
	}

	if requestCount != 3 {
		t.Fatalf("expected three requests, got %d", requestCount)
	}
	if result.Operation != "upload-video" {
		t.Fatalf("unexpected operation %q", result.Operation)
	}
	if result.VideoID != "vid_1" {
		t.Fatalf("unexpected video id %q", result.VideoID)
	}
	if !result.Ready {
		t.Fatal("expected ready=true")
	}
	if result.FinalStatus != "complete" {
		t.Fatalf("unexpected final status %q", result.FinalStatus)
	}
}

func TestCreativeUploadVideoWaitReadyTimesOut(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "video.mp4")
	if err := os.WriteFile(filePath, []byte("video-bytes"), 0o644); err != nil {
		t.Fatalf("write video file: %v", err)
	}

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			_, _ = w.Write([]byte(`{"id":"vid_timeout"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"vid_timeout","processing_phase":{"status":"IN_PROGRESS"}}`))
	}))
	defer server.Close()

	client := graph.NewClient(server.Client(), server.URL)
	client.MaxRetries = 0
	service := NewCreativeService(client)
	_, err := service.UploadVideo(context.Background(), "v25.0", "token-1", "secret-1", CreativeVideoUploadInput{
		AccountID:    "1234",
		FilePath:     filePath,
		WaitReady:    true,
		Timeout:      2 * time.Millisecond,
		PollInterval: 1 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "timed out") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreativeUploadVideoWaitReadyFailsOnTerminalStatus(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "video.mp4")
	if err := os.WriteFile(filePath, []byte("video-bytes"), 0o644); err != nil {
		t.Fatalf("write video file: %v", err)
	}

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			_, _ = w.Write([]byte(`{"id":"vid_fail"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"vid_fail","processing_phase":{"status":"ERROR"}}`))
	}))
	defer server.Close()

	client := graph.NewClient(server.Client(), server.URL)
	client.MaxRetries = 0
	service := NewCreativeService(client)
	_, err := service.UploadVideo(context.Background(), "v25.0", "token-1", "secret-1", CreativeVideoUploadInput{
		AccountID:    "1234",
		FilePath:     filePath,
		WaitReady:    true,
		Timeout:      100 * time.Millisecond,
		PollInterval: 1 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected terminal status error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}
