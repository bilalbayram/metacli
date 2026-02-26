package insights

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bilalbayram/metacli/internal/graph"
)

func TestRunSyncInsights(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"impressions": "10"},
				{"impressions": "20"},
			},
		})
	}))
	defer server.Close()

	client := graph.NewClient(server.Client(), server.URL)
	svc := New(client)
	result, err := svc.Run(context.Background(), "v25.0", "token", "", RunOptions{
		AccountID:  "1",
		Level:      "campaign",
		DatePreset: "last_7d",
		Limit:      2,
		Async:      false,
	})
	if err != nil {
		t.Fatalf("run sync insights: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result.Rows))
	}
}

func TestRunAsyncInsights(t *testing.T) {
	t.Parallel()

	var pollCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v25.0/act_1/insights":
			if r.Method == http.MethodPost {
				_ = json.NewEncoder(w).Encode(map[string]any{"report_run_id": "run_1"})
				return
			}
		case "/v25.0/run_1":
			if atomic.AddInt32(&pollCount, 1) == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{"async_status": "Job Running", "async_percent_completion": 50})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"async_status": "Job Completed", "async_percent_completion": 100})
			return
		case "/v25.0/run_1/insights":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"clicks": "3"}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := graph.NewClient(server.Client(), server.URL)
	svc := New(client)
	svc.PollInterval = 1 * time.Millisecond
	svc.MaxPollAttempts = 3
	svc.Sleep = func(time.Duration) {}

	result, err := svc.Run(context.Background(), "v25.0", "token", "", RunOptions{
		AccountID:  "1",
		Level:      "campaign",
		DatePreset: "last_7d",
		Async:      true,
	})
	if err != nil {
		t.Fatalf("run async insights: %v", err)
	}
	if result.ReportRunID != "run_1" {
		t.Fatalf("unexpected report run id: %s", result.ReportRunID)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
}

func TestRunSyncInsightsIncludesFieldsFilter(t *testing.T) {
	t.Parallel()

	expectedFields := "spend,impressions,ctr"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("fields"); got != expectedFields {
			t.Fatalf("unexpected fields query: got=%q want=%q", got, expectedFields)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"spend": "10.00", "impressions": "100", "ctr": "1.0"},
			},
		})
	}))
	defer server.Close()

	client := graph.NewClient(server.Client(), server.URL)
	svc := New(client)
	result, err := svc.Run(context.Background(), "v25.0", "token", "", RunOptions{
		AccountID:  "1",
		Level:      "campaign",
		DatePreset: "last_7d",
		Fields:     []string{"spend", "impressions", "ctr"},
	})
	if err != nil {
		t.Fatalf("run sync insights with fields: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
}

func TestRunRejectsBlankFieldsFilter(t *testing.T) {
	t.Parallel()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := graph.NewClient(server.Client(), server.URL)
	svc := New(client)
	_, err := svc.Run(context.Background(), "v25.0", "token", "", RunOptions{
		AccountID:  "1",
		Level:      "campaign",
		DatePreset: "last_7d",
		Fields:     []string{"spend", "  "},
	})
	if err == nil {
		t.Fatal("expected error for blank fields filter")
	}
	if !strings.Contains(err.Error(), "blank entries") {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("expected zero network calls, got %d", got)
	}
}
