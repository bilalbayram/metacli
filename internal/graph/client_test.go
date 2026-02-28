package graph

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestShouldRetryClassifier(t *testing.T) {
	t.Parallel()

	cases := []struct {
		status int
		code   int
		want   bool
	}{
		{status: 429, code: 0, want: true},
		{status: 500, code: 0, want: true},
		{status: 400, code: 4, want: true},
		{status: 400, code: 17, want: true},
		{status: 400, code: 32, want: true},
		{status: 400, code: 613, want: true},
		{status: 400, code: 100, want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			t.Parallel()
			got := ShouldRetry(tc.status, tc.code)
			if got != tc.want {
				t.Fatalf("ShouldRetry(%d, %d)=%v want=%v", tc.status, tc.code, got, tc.want)
			}
		})
	}
}

func TestClientRetriesOnMetaRateLimitCode(t *testing.T) {
	t.Parallel()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&calls, 1)
		if count == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"Rate limit","type":"OAuthException","code":613,"error_subcode":0,"fbtrace_id":"abc"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"id":"1"}]}`))
	}))
	defer server.Close()

	client := NewClient(server.Client(), server.URL)
	client.InitialBackoff = 1 * time.Millisecond
	client.MaxBackoff = 1 * time.Millisecond
	client.Sleep = func(time.Duration) {}

	resp, err := client.Do(context.Background(), Request{
		Method:  http.MethodGet,
		Path:    "/act_123/insights",
		Version: "v25.0",
	})
	if err != nil {
		t.Fatalf("client do: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected exactly 2 attempts, got %d", calls)
	}
}

func TestClientNormalizesGraphError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Unsupported get request","type":"GraphMethodException","code":100,"error_subcode":33,"fbtrace_id":"trace-1"}}`))
	}))
	defer server.Close()

	client := NewClient(server.Client(), server.URL)
	client.InitialBackoff = 1 * time.Millisecond
	client.MaxBackoff = 1 * time.Millisecond
	client.Sleep = func(time.Duration) {}

	_, err := client.Do(context.Background(), Request{
		Method:  http.MethodGet,
		Path:    "/bad-path",
		Version: "v25.0",
	})
	if err == nil {
		t.Fatal("expected graph error")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Code != 100 || apiErr.ErrorSubcode != 33 {
		t.Fatalf("unexpected error mapping: %#v", apiErr)
	}
	if apiErr.Retryable {
		t.Fatalf("expected non-retryable error")
	}
	if apiErr.Remediation == nil {
		t.Fatalf("expected remediation contract in api error: %#v", apiErr)
	}
	if apiErr.Remediation.Category != RemediationCategoryNotFound {
		t.Fatalf("unexpected remediation category: %#v", apiErr.Remediation)
	}
	diagnostics := apiErr.Diagnostics
	if diagnostics == nil {
		t.Fatalf("expected diagnostics payload in api error: %#v", apiErr)
	}
	if got := diagnostics["fbtrace_id"]; got != "trace-1" {
		t.Fatalf("unexpected fbtrace diagnostics value %v", got)
	}
}

func TestClientClassifiesGraphValidationFromBlameFieldSpecs(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid parameter","type":"OAuthException","code":100,"error_subcode":0,"fbtrace_id":"trace-2","error_data":{"blame_field_specs":[["targeting","geo_locations"],["daily_budget"]]}}}`))
	}))
	defer server.Close()

	client := NewClient(server.Client(), server.URL)
	client.InitialBackoff = 1 * time.Millisecond
	client.MaxBackoff = 1 * time.Millisecond
	client.Sleep = func(time.Duration) {}

	_, err := client.Do(context.Background(), Request{
		Method:  http.MethodGet,
		Path:    "/bad-path",
		Version: "v25.0",
	})
	if err == nil {
		t.Fatal("expected graph error")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Remediation == nil {
		t.Fatalf("expected remediation contract, got %#v", apiErr)
	}
	if apiErr.Remediation.Category != RemediationCategoryValidation {
		t.Fatalf("unexpected remediation category %#v", apiErr.Remediation)
	}
	if len(apiErr.Remediation.Fields) != 2 {
		t.Fatalf("unexpected remediation fields %#v", apiErr.Remediation.Fields)
	}
}

func TestClientPreservesUnknownErrorDiagnostics(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"strange failure","type":"OAuthException","code":987654,"error_subcode":1234,"fbtrace_id":"trace-unknown","error_user_title":"Needs attention","error_user_msg":"Unexpected combination"}}`))
	}))
	defer server.Close()

	client := NewClient(server.Client(), server.URL)
	client.InitialBackoff = 1 * time.Millisecond
	client.MaxBackoff = 1 * time.Millisecond
	client.Sleep = func(time.Duration) {}

	_, err := client.Do(context.Background(), Request{
		Method:  http.MethodGet,
		Path:    "/bad-path",
		Version: "v25.0",
	})
	if err == nil {
		t.Fatal("expected graph error")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
	if got := apiErr.FBTraceID; got != "trace-unknown" {
		t.Fatalf("unexpected fbtrace_id %q", got)
	}
	if apiErr.Remediation == nil || apiErr.Remediation.Category != RemediationCategoryUnknown {
		t.Fatalf("expected unknown remediation category, got %#v", apiErr.Remediation)
	}
	if apiErr.Diagnostics == nil {
		t.Fatalf("expected diagnostics payload, got %#v", apiErr)
	}
	if got := apiErr.Diagnostics["error_user_title"]; got != "Needs attention" {
		t.Fatalf("unexpected diagnostics payload: %#v", apiErr.Diagnostics)
	}
}

func TestClientSupportsMultipartRequestBodies(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %q", r.Method)
		}
		if r.URL.Path != "/v25.0/act_1234/advideos" {
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
		if contentType := r.Header.Get("Content-Type"); !strings.Contains(contentType, "multipart/form-data") {
			t.Fatalf("unexpected content type %q", contentType)
		}
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		if got := r.FormValue("name"); got != "clip.mp4" {
			t.Fatalf("unexpected name field %q", got)
		}
		if got := r.FormValue("access_token"); got != "token-1" {
			t.Fatalf("unexpected access token %q", got)
		}
		if got := r.FormValue("appsecret_proof"); strings.TrimSpace(got) == "" {
			t.Fatal("expected appsecret_proof field")
		}
		file, header, err := r.FormFile("source")
		if err != nil {
			t.Fatalf("open multipart file field: %v", err)
		}
		defer file.Close()
		if header.Filename != "clip.mp4" {
			t.Fatalf("unexpected file name %q", header.Filename)
		}
		payload, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read multipart payload: %v", err)
		}
		if string(payload) != "video-bytes" {
			t.Fatalf("unexpected multipart payload %q", string(payload))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"vid_1"}`))
	}))
	defer server.Close()

	client := NewClient(server.Client(), server.URL)
	client.MaxRetries = 0
	response, err := client.Do(context.Background(), Request{
		Method:  http.MethodPost,
		Path:    "act_1234/advideos",
		Version: "v25.0",
		Form: map[string]string{
			"name": "clip.mp4",
		},
		Multipart: &MultipartFile{
			FieldName: "source",
			FileName:  "clip.mp4",
			FileBytes: []byte("video-bytes"),
		},
		AccessToken: "token-1",
		AppSecret:   "secret-1",
	})
	if err != nil {
		t.Fatalf("client do multipart request: %v", err)
	}
	if got, _ := response.Body["id"].(string); got != "vid_1" {
		t.Fatalf("unexpected response id %q", got)
	}
}
