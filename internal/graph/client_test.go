package graph

import (
	"context"
	"net/http"
	"net/http/httptest"
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
