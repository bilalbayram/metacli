package marketing

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
