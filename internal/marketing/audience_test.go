package marketing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/bilalbayram/metacli/internal/graph"
)

func TestAudienceCreateExecutesGraphMutation(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"id":"aud_100","name":"VIP Buyers"}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewAudienceService(client)

	result, err := service.Create(context.Background(), "v25.0", "token-1", "secret-1", AudienceCreateInput{
		AccountID: "act_1234",
		Params: map[string]string{
			"name":        "VIP Buyers",
			"subtype":     "CUSTOM",
			"description": "High-value users",
		},
	})
	if err != nil {
		t.Fatalf("create audience: %v", err)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if requestURL.Path != "/v25.0/act_1234/customaudiences" {
		t.Fatalf("unexpected request path %q", requestURL.Path)
	}

	form, err := url.ParseQuery(stub.lastBody)
	if err != nil {
		t.Fatalf("parse form body: %v", err)
	}
	if got := form.Get("name"); got != "VIP Buyers" {
		t.Fatalf("unexpected name %q", got)
	}
	if got := form.Get("subtype"); got != "CUSTOM" {
		t.Fatalf("unexpected subtype %q", got)
	}

	if result.Operation != "create" {
		t.Fatalf("unexpected operation %q", result.Operation)
	}
	if result.AudienceID != "aud_100" {
		t.Fatalf("unexpected audience id %q", result.AudienceID)
	}
}

func TestAudienceUpdateExecutesGraphMutation(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"success":true}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewAudienceService(client)

	result, err := service.Update(context.Background(), "v25.0", "token-1", "secret-1", AudienceUpdateInput{
		AudienceID: "aud_200",
		Params: map[string]string{
			"description": "Updated Description",
		},
	})
	if err != nil {
		t.Fatalf("update audience: %v", err)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if requestURL.Path != "/v25.0/aud_200" {
		t.Fatalf("unexpected request path %q", requestURL.Path)
	}

	form, err := url.ParseQuery(stub.lastBody)
	if err != nil {
		t.Fatalf("parse form body: %v", err)
	}
	if got := form.Get("description"); got != "Updated Description" {
		t.Fatalf("unexpected description %q", got)
	}

	if result.Operation != "update" {
		t.Fatalf("unexpected operation %q", result.Operation)
	}
	if result.AudienceID != "aud_200" {
		t.Fatalf("unexpected audience id %q", result.AudienceID)
	}
}

func TestAudienceDeleteExecutesGraphMutation(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"success":true}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewAudienceService(client)

	result, err := service.Delete(context.Background(), "v25.0", "token-1", "secret-1", AudienceDeleteInput{
		AudienceID: "aud_300",
	})
	if err != nil {
		t.Fatalf("delete audience: %v", err)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if requestURL.Path != "/v25.0/aud_300" {
		t.Fatalf("unexpected request path %q", requestURL.Path)
	}
	if stub.lastMethod != http.MethodDelete {
		t.Fatalf("unexpected method %q", stub.lastMethod)
	}

	if result.Operation != "delete" {
		t.Fatalf("unexpected operation %q", result.Operation)
	}
	if result.AudienceID != "aud_300" {
		t.Fatalf("unexpected audience id %q", result.AudienceID)
	}
}

func TestAudienceCreateFailsWhenResponseMissingID(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"success":true}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewAudienceService(client)

	_, err := service.Create(context.Background(), "v25.0", "token-1", "secret-1", AudienceCreateInput{
		AccountID: "1234",
		Params: map[string]string{
			"name":    "Audience",
			"subtype": "CUSTOM",
		},
	})
	if err == nil {
		t.Fatal("expected create error")
	}
	if !strings.Contains(err.Error(), "did not include id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAudienceUpdateRejectsEmptyPayload(t *testing.T) {
	t.Parallel()

	service := NewAudienceService(graph.NewClient(nil, ""))
	_, err := service.Update(context.Background(), "v25.0", "token-1", "secret-1", AudienceUpdateInput{
		AudienceID: "aud_200",
		Params:     map[string]string{},
	})
	if err == nil {
		t.Fatal("expected update error")
	}
	if !strings.Contains(err.Error(), "payload cannot be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAudienceDeleteFailsWhenSuccessIsFalse(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"success":false}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewAudienceService(client)

	_, err := service.Delete(context.Background(), "v25.0", "token-1", "secret-1", AudienceDeleteInput{
		AudienceID: "aud_300",
	})
	if err == nil {
		t.Fatal("expected delete error")
	}
	if !strings.Contains(err.Error(), "not successful") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAudienceListExecutesGraphReadWithDefaultFields(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response: `{"data":[
			{"id":"aud_2","name":"Second"},
			{"id":"aud_1","name":"First"}
		]}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewAudienceService(client)

	result, err := service.List(context.Background(), "v25.0", "token-1", "secret-1", AudienceListInput{
		AccountID: "act_1234",
		Limit:     25,
	})
	if err != nil {
		t.Fatalf("list audiences: %v", err)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if requestURL.Path != "/v25.0/act_1234/customaudiences" {
		t.Fatalf("unexpected request path %q", requestURL.Path)
	}
	if got := requestURL.Query().Get("fields"); got != "id,name,subtype,time_updated,retention_days" {
		t.Fatalf("unexpected fields query %q", got)
	}
	if got := requestURL.Query().Get("limit"); got != "25" {
		t.Fatalf("unexpected limit query %q", got)
	}

	if result.Operation != "list" {
		t.Fatalf("unexpected operation %q", result.Operation)
	}
	if result.RequestPath != "act_1234/customaudiences" {
		t.Fatalf("unexpected request path %q", result.RequestPath)
	}
	if len(result.Audiences) != 2 {
		t.Fatalf("expected 2 audiences, got %d", len(result.Audiences))
	}
	if got := result.Audiences[0]["id"]; got != "aud_1" {
		t.Fatalf("expected deterministic sort, got first id %v", got)
	}
	if got := result.Audiences[1]["id"]; got != "aud_2" {
		t.Fatalf("expected deterministic sort, got second id %v", got)
	}
	if result.Paging == nil {
		t.Fatal("expected paging metadata")
	}
	if got := result.Paging.PagesFetched; got != 1 {
		t.Fatalf("unexpected pages fetched %d", got)
	}
	if got := result.Paging.ItemsFetched; got != 2 {
		t.Fatalf("unexpected items fetched %d", got)
	}
}

func TestAudienceListFollowsPaginationWhenRequested(t *testing.T) {
	t.Parallel()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("after") == "" {
			if got := r.URL.Query().Get("fields"); got != "id,name" {
				t.Fatalf("unexpected fields query on first page %q", got)
			}
			payload := map[string]any{
				"data": []map[string]any{
					{"id": "aud_2", "name": "Second"},
				},
				"paging": map[string]any{
					"next": server.URL + "/v25.0/act_1234/customaudiences?after=cursor_1",
				},
			}
			if err := json.NewEncoder(w).Encode(payload); err != nil {
				t.Fatalf("encode first page: %v", err)
			}
			return
		}
		payload := map[string]any{
			"data": []map[string]any{
				{"id": "aud_1", "name": "First"},
			},
		}
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			t.Fatalf("encode second page: %v", err)
		}
	}))
	defer server.Close()

	client := graph.NewClient(server.Client(), server.URL)
	client.MaxRetries = 0
	service := NewAudienceService(client)

	result, err := service.List(context.Background(), "v25.0", "token-1", "secret-1", AudienceListInput{
		AccountID:  "1234",
		Fields:     []string{"id", "name"},
		FollowNext: true,
	})
	if err != nil {
		t.Fatalf("list audiences with pagination: %v", err)
	}
	if len(result.Audiences) != 2 {
		t.Fatalf("expected 2 audiences, got %d", len(result.Audiences))
	}
	if got := result.Audiences[0]["id"]; got != "aud_1" {
		t.Fatalf("unexpected first id %v", got)
	}
	if got := result.Audiences[1]["id"]; got != "aud_2" {
		t.Fatalf("unexpected second id %v", got)
	}
	if result.Paging == nil {
		t.Fatal("expected paging metadata")
	}
	if got := result.Paging.PagesFetched; got != 2 {
		t.Fatalf("unexpected pages fetched %d", got)
	}
	if got := result.Paging.ItemsFetched; got != 2 {
		t.Fatalf("unexpected items fetched %d", got)
	}
}

func TestAudienceListSortsMissingOrInvalidIDsLast(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response: `{"data":[
			{"id":"aud_2","name":"Second"},
			{"name":"Missing"},
			{"id":"aud_1","name":"First"},
			{"id":123,"name":"Numeric"}
		]}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewAudienceService(client)

	result, err := service.List(context.Background(), "v25.0", "token-1", "secret-1", AudienceListInput{
		AccountID: "1234",
	})
	if err != nil {
		t.Fatalf("list audiences: %v", err)
	}
	if len(result.Audiences) != 4 {
		t.Fatalf("expected 4 audiences, got %d", len(result.Audiences))
	}
	if got := result.Audiences[0]["id"]; got != "aud_1" {
		t.Fatalf("expected first valid id aud_1, got %v", got)
	}
	if got := result.Audiences[1]["id"]; got != "aud_2" {
		t.Fatalf("expected second valid id aud_2, got %v", got)
	}
	if got := result.Audiences[2]["name"]; got != "Missing" {
		t.Fatalf("expected missing-id item to remain ahead of non-string id item, got %v", got)
	}
	if got := result.Audiences[3]["name"]; got != "Numeric" {
		t.Fatalf("expected non-string-id item last among invalid ids, got %v", got)
	}
}

func TestAudienceGetExecutesGraphRead(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"id":"aud_700","name":"Core Buyers","subtype":"CUSTOM"}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewAudienceService(client)

	result, err := service.Get(context.Background(), "v25.0", "token-1", "secret-1", AudienceGetInput{
		AudienceID: "aud_700",
		Fields:     []string{"id", "name"},
	})
	if err != nil {
		t.Fatalf("get audience: %v", err)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if requestURL.Path != "/v25.0/aud_700" {
		t.Fatalf("unexpected request path %q", requestURL.Path)
	}
	if got := requestURL.Query().Get("fields"); got != "id,name" {
		t.Fatalf("unexpected fields query %q", got)
	}
	if result.Operation != "get" {
		t.Fatalf("unexpected operation %q", result.Operation)
	}
	if result.RequestPath != "aud_700" {
		t.Fatalf("unexpected request path %q", result.RequestPath)
	}
	if got := result.Audience["id"]; got != "aud_700" {
		t.Fatalf("unexpected audience id %v", got)
	}
}

func TestAudienceListRejectsMissingAccountID(t *testing.T) {
	t.Parallel()

	service := NewAudienceService(graph.NewClient(nil, ""))
	_, err := service.List(context.Background(), "v25.0", "token-1", "secret-1", AudienceListInput{})
	if err == nil {
		t.Fatal("expected list error")
	}
	if !strings.Contains(err.Error(), "account id is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAudienceGetRejectsMissingAudienceID(t *testing.T) {
	t.Parallel()

	service := NewAudienceService(graph.NewClient(nil, ""))
	_, err := service.Get(context.Background(), "v25.0", "token-1", "secret-1", AudienceGetInput{})
	if err == nil {
		t.Fatal("expected get error")
	}
	if !strings.Contains(err.Error(), "audience id is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
