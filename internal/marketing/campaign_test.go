package marketing

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/bilalbayram/metacli/internal/graph"
)

type stubHTTPClient struct {
	t *testing.T

	statusCode int
	response   string
	err        error

	calls      int
	lastMethod string
	lastURL    string
	lastBody   string
}

func (c *stubHTTPClient) Do(req *http.Request) (*http.Response, error) {
	c.calls++
	c.lastMethod = req.Method
	c.lastURL = req.URL.String()
	if req.Body != nil {
		body, readErr := io.ReadAll(req.Body)
		if readErr != nil {
			c.t.Fatalf("read request body: %v", readErr)
		}
		c.lastBody = string(body)
	}

	if c.err != nil {
		return nil, c.err
	}
	return &http.Response{
		StatusCode: c.statusCode,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(c.response)),
	}, nil
}

func TestCampaignCreateExecutesGraphMutation(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"id":"12001","name":"Scale Q2"}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewCampaignService(client)

	result, err := service.Create(context.Background(), "v25.0", "token-1", "secret-1", CampaignCreateInput{
		AccountID: "act_1234",
		Params: map[string]string{
			"name":      "Scale Q2",
			"objective": "OUTCOME_SALES",
			"status":    "PAUSED",
		},
	})
	if err != nil {
		t.Fatalf("create campaign: %v", err)
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
	if requestURL.Path != "/v25.0/act_1234/campaigns" {
		t.Fatalf("unexpected request path %q", requestURL.Path)
	}

	form, err := url.ParseQuery(stub.lastBody)
	if err != nil {
		t.Fatalf("parse form body: %v", err)
	}
	if got := form.Get("name"); got != "Scale Q2" {
		t.Fatalf("unexpected name value %q", got)
	}
	if got := form.Get("objective"); got != "OUTCOME_SALES" {
		t.Fatalf("unexpected objective value %q", got)
	}
	if got := form.Get("status"); got != "PAUSED" {
		t.Fatalf("unexpected status value %q", got)
	}
	if got := form.Get("access_token"); got != "token-1" {
		t.Fatalf("unexpected access_token %q", got)
	}
	if got := form.Get("appsecret_proof"); got == "" {
		t.Fatal("expected appsecret_proof")
	}

	if result.Operation != "create" {
		t.Fatalf("unexpected operation %q", result.Operation)
	}
	if result.CampaignID != "12001" {
		t.Fatalf("unexpected campaign id %q", result.CampaignID)
	}
	if result.RequestPath != "act_1234/campaigns" {
		t.Fatalf("unexpected request path %q", result.RequestPath)
	}
}

func TestCampaignUpdateExecutesGraphMutation(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"success":true}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewCampaignService(client)

	result, err := service.Update(context.Background(), "v25.0", "token-1", "secret-1", CampaignUpdateInput{
		CampaignID: "22001",
		Params: map[string]string{
			"name": "Updated Name",
		},
	})
	if err != nil {
		t.Fatalf("update campaign: %v", err)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if requestURL.Path != "/v25.0/22001" {
		t.Fatalf("unexpected request path %q", requestURL.Path)
	}

	form, err := url.ParseQuery(stub.lastBody)
	if err != nil {
		t.Fatalf("parse form body: %v", err)
	}
	if got := form.Get("name"); got != "Updated Name" {
		t.Fatalf("unexpected name value %q", got)
	}

	if result.Operation != "update" {
		t.Fatalf("unexpected operation %q", result.Operation)
	}
	if result.CampaignID != "22001" {
		t.Fatalf("unexpected campaign id %q", result.CampaignID)
	}
}

func TestCampaignSetStatusExecutesStatusMutation(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"success":true}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewCampaignService(client)

	result, err := service.SetStatus(context.Background(), "v25.0", "token-1", "secret-1", CampaignStatusInput{
		CampaignID: "33001",
		Status:     CampaignStatusPaused,
	})
	if err != nil {
		t.Fatalf("pause campaign: %v", err)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if requestURL.Path != "/v25.0/33001" {
		t.Fatalf("unexpected request path %q", requestURL.Path)
	}

	form, err := url.ParseQuery(stub.lastBody)
	if err != nil {
		t.Fatalf("parse form body: %v", err)
	}
	if got := form.Get("status"); got != CampaignStatusPaused {
		t.Fatalf("unexpected status %q", got)
	}

	if result.Operation != strings.ToLower(CampaignStatusPaused) {
		t.Fatalf("unexpected operation %q", result.Operation)
	}
}

func TestCampaignListExecutesGraphReadWithFiltersAndProjection(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response: `{"data":[
			{"id":"cmp_1","name":"Launch Alpha","status":"ACTIVE","effective_status":"ACTIVE","objective":"OUTCOME_SALES"},
			{"id":"cmp_2","name":"Launch Beta","status":"ACTIVE","effective_status":"ACTIVE","objective":"OUTCOME_TRAFFIC"},
			{"id":"cmp_3","name":"Retarget","status":"PAUSED","effective_status":"PAUSED","objective":"OUTCOME_SALES"}
		]}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewCampaignService(client)

	result, err := service.List(context.Background(), "v25.0", "token-1", "secret-1", CampaignListInput{
		AccountID: "act_1234",
		Fields:    []string{"id", "name", "status"},
		Name:      "launch",
		Statuses:  []string{"active"},
		Limit:     1,
		PageSize:  25,
	})
	if err != nil {
		t.Fatalf("list campaigns: %v", err)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if requestURL.Path != "/v25.0/act_1234/campaigns" {
		t.Fatalf("unexpected request path %q", requestURL.Path)
	}
	if got := requestURL.Query().Get("limit"); got != "25" {
		t.Fatalf("unexpected page-size query %q", got)
	}
	if got := requestURL.Query().Get("fields"); got != "id,name,status,effective_status" {
		t.Fatalf("unexpected fields query %q", got)
	}

	if result.Operation != "list" {
		t.Fatalf("unexpected operation %q", result.Operation)
	}
	if result.RequestPath != "act_1234/campaigns" {
		t.Fatalf("unexpected request path %q", result.RequestPath)
	}
	if len(result.Campaigns) != 1 {
		t.Fatalf("expected one filtered campaign, got %d", len(result.Campaigns))
	}
	first := result.Campaigns[0]
	if got := first["id"]; got != "cmp_1" {
		t.Fatalf("unexpected first campaign id %v", got)
	}
	if _, exists := first["effective_status"]; exists {
		t.Fatalf("did not expect hidden filter field in projected output: %v", first)
	}
	if result.Paging == nil {
		t.Fatal("expected paging metadata")
	}
}

func TestCampaignListFollowsPaginationWhenRequested(t *testing.T) {
	t.Parallel()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("after") == "" {
			payload := map[string]any{
				"data": []map[string]any{
					{"id": "cmp_1", "name": "Launch Alpha", "status": "ACTIVE", "effective_status": "ACTIVE"},
				},
				"paging": map[string]any{
					"next": server.URL + "/v25.0/act_1234/campaigns?after=cursor_1",
				},
			}
			if err := json.NewEncoder(w).Encode(payload); err != nil {
				t.Fatalf("encode first page: %v", err)
			}
			return
		}
		payload := map[string]any{
			"data": []map[string]any{
				{"id": "cmp_2", "name": "Launch Beta", "status": "PAUSED", "effective_status": "ACTIVE"},
			},
		}
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			t.Fatalf("encode second page: %v", err)
		}
	}))
	defer server.Close()

	client := graph.NewClient(server.Client(), server.URL)
	client.MaxRetries = 0
	service := NewCampaignService(client)

	result, err := service.List(context.Background(), "v25.0", "token-1", "secret-1", CampaignListInput{
		AccountID:  "1234",
		Fields:     []string{"id", "name"},
		FollowNext: true,
		PageSize:   1,
	})
	if err != nil {
		t.Fatalf("list campaigns with pagination: %v", err)
	}
	if len(result.Campaigns) != 2 {
		t.Fatalf("expected two campaigns, got %d", len(result.Campaigns))
	}
	if result.Paging == nil {
		t.Fatal("expected paging metadata")
	}
	if got := result.Paging.PagesFetched; got != 2 {
		t.Fatalf("unexpected pages fetched %d", got)
	}
}

func TestCampaignCreateFailsWhenResponseMissingID(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"success":true}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewCampaignService(client)

	_, err := service.Create(context.Background(), "v25.0", "token-1", "secret-1", CampaignCreateInput{
		AccountID: "1234",
		Params: map[string]string{
			"name":      "Name",
			"objective": "OUTCOME_SALES",
		},
	})
	if err == nil {
		t.Fatal("expected create error")
	}
	if !strings.Contains(err.Error(), "did not include id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCampaignUpdateRejectsEmptyPayload(t *testing.T) {
	t.Parallel()

	service := NewCampaignService(graph.NewClient(nil, ""))
	_, err := service.Update(context.Background(), "v25.0", "token-1", "secret-1", CampaignUpdateInput{
		CampaignID: "22001",
		Params:     map[string]string{},
	})
	if err == nil {
		t.Fatal("expected update error")
	}
	if !strings.Contains(err.Error(), "payload cannot be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCampaignCloneReadsSanitizesAndCreates(t *testing.T) {
	t.Parallel()

	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		switch requestCount {
		case 1:
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected method for clone read: %s", r.Method)
			}
			if r.URL.Path != "/v25.0/source_1" {
				t.Fatalf("unexpected clone read path %q", r.URL.Path)
			}
			if got := r.URL.Query().Get("fields"); got != "id,name,objective,status,daily_budget,account_id,effective_status" {
				t.Fatalf("unexpected fields query %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":               "source_1",
				"name":             "Original Campaign",
				"objective":        "OUTCOME_SALES",
				"status":           "PAUSED",
				"daily_budget":     "1000",
				"account_id":       "123",
				"effective_status": "PAUSED",
			})
		case 2:
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method for clone create: %s", r.Method)
			}
			if r.URL.Path != "/v25.0/act_999/campaigns" {
				t.Fatalf("unexpected clone create path %q", r.URL.Path)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read clone create body: %v", err)
			}
			form, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("parse clone create form: %v", err)
			}
			if got := form.Get("name"); got != "Cloned Campaign" {
				t.Fatalf("unexpected cloned name %q", got)
			}
			if got := form.Get("status"); got != "ACTIVE" {
				t.Fatalf("unexpected cloned status %q", got)
			}
			if got := form.Get("objective"); got != "OUTCOME_SALES" {
				t.Fatalf("unexpected cloned objective %q", got)
			}
			if got := form.Get("daily_budget"); got != "1000" {
				t.Fatalf("unexpected cloned daily_budget %q", got)
			}
			if got := form.Get("id"); got != "" {
				t.Fatalf("immutable id should not be cloned, got %q", got)
			}
			if got := form.Get("account_id"); got != "" {
				t.Fatalf("immutable account_id should not be cloned, got %q", got)
			}
			if got := form.Get("effective_status"); got != "" {
				t.Fatalf("immutable effective_status should not be cloned, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "new_1",
			})
		default:
			t.Fatalf("unexpected request count %d", requestCount)
		}
	}))
	defer server.Close()

	client := graph.NewClient(server.Client(), server.URL)
	service := NewCampaignService(client)

	result, err := service.Clone(context.Background(), "v25.0", "token-1", "", CampaignCloneInput{
		SourceCampaignID: "source_1",
		TargetAccountID:  "999",
		Overrides: map[string]string{
			"name":   "Cloned Campaign",
			"status": "ACTIVE",
		},
		Fields: []string{"id", "name", "objective", "status", "daily_budget", "account_id", "effective_status"},
	})
	if err != nil {
		t.Fatalf("clone campaign: %v", err)
	}

	if requestCount != 2 {
		t.Fatalf("expected two requests, got %d", requestCount)
	}
	if result.Operation != "clone" {
		t.Fatalf("unexpected operation %q", result.Operation)
	}
	if result.SourceCampaignID != "source_1" {
		t.Fatalf("unexpected source campaign id %q", result.SourceCampaignID)
	}
	if result.CampaignID != "new_1" {
		t.Fatalf("unexpected campaign id %q", result.CampaignID)
	}
	if !slices.Contains(result.RemovedFields, "id") {
		t.Fatalf("expected removed fields to contain id, got %v", result.RemovedFields)
	}
	if !slices.Contains(result.RemovedFields, "account_id") {
		t.Fatalf("expected removed fields to contain account_id, got %v", result.RemovedFields)
	}
	if !slices.Contains(result.RemovedFields, "effective_status") {
		t.Fatalf("expected removed fields to contain effective_status, got %v", result.RemovedFields)
	}
}

func TestCampaignCloneFailsWhenPayloadEmptyAfterSanitization(t *testing.T) {
	t.Parallel()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":               "source_1",
			"account_id":       "123",
			"effective_status": "PAUSED",
		})
	}))
	defer server.Close()

	client := graph.NewClient(server.Client(), server.URL)
	service := NewCampaignService(client)

	_, err := service.Clone(context.Background(), "v25.0", "token-1", "", CampaignCloneInput{
		SourceCampaignID: "source_1",
		TargetAccountID:  "999",
		Fields:           []string{"id", "account_id", "effective_status"},
	})
	if err == nil {
		t.Fatal("expected clone error")
	}
	if !strings.Contains(err.Error(), "payload is empty after sanitization") {
		t.Fatalf("unexpected error: %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("expected only source read request, got %d", requestCount)
	}
}

func TestCampaignCloneFailsWhenCreateResponseMissingID(t *testing.T) {
	t.Parallel()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":        "source_1",
				"name":      "Original",
				"objective": "OUTCOME_SALES",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
		})
	}))
	defer server.Close()

	client := graph.NewClient(server.Client(), server.URL)
	service := NewCampaignService(client)

	_, err := service.Clone(context.Background(), "v25.0", "token-1", "", CampaignCloneInput{
		SourceCampaignID: "source_1",
		TargetAccountID:  "999",
	})
	if err == nil {
		t.Fatal("expected clone error")
	}
	if !strings.Contains(err.Error(), "did not include id") {
		t.Fatalf("unexpected error: %v", err)
	}
}
