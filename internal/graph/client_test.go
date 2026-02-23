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
}
