package marketing

import (
	"context"
	"io"
	"net/http"
	"net/url"
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
