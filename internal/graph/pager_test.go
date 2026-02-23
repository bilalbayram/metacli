package graph

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchWithPagination(t *testing.T) {
	t.Parallel()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("after") == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "1"},
					{"id": "2"},
				},
				"paging": map[string]any{
					"next": server.URL + "/v25.0/act_1/insights?after=cursor_1",
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "3"}},
		})
	}))
	defer server.Close()

	var seen []string
	client := NewClient(server.Client(), server.URL)
	_, err := client.FetchWithPagination(context.Background(), Request{
		Method:  "GET",
		Path:    "/act_1/insights",
		Version: "v25.0",
	}, PaginationOptions{
		FollowNext: true,
		Limit:      3,
	}, func(item map[string]any) error {
		id, _ := item["id"].(string)
		seen = append(seen, id)
		return nil
	})
	if err != nil {
		t.Fatalf("fetch with pagination: %v", err)
	}
	if len(seen) != 3 {
		t.Fatalf("expected 3 items, got %d", len(seen))
	}
}

func TestValidateBatchRequestsRejectsUnsupportedMethods(t *testing.T) {
	t.Parallel()

	err := ValidateBatchRequests([]BatchRequest{
		{Method: "POST", Path: "/me"},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
