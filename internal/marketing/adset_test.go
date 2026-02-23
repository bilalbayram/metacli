package marketing

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/bilalbayram/metacli/internal/graph"
)

func TestAdSetCreateExecutesGraphMutation(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"id":"51001","name":"Prospecting A"}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewAdSetService(client)

	result, err := service.Create(context.Background(), "v25.0", "token-1", "secret-1", AdSetCreateInput{
		AccountID: "act_5678",
		Params: map[string]string{
			"name":              "Prospecting A",
			"campaign_id":       "cp_77",
			"status":            "PAUSED",
			"daily_budget":      "1200",
			"optimization_goal": "OFFSITE_CONVERSIONS",
		},
	})
	if err != nil {
		t.Fatalf("create ad set: %v", err)
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
	if requestURL.Path != "/v25.0/act_5678/adsets" {
		t.Fatalf("unexpected request path %q", requestURL.Path)
	}

	form, err := url.ParseQuery(stub.lastBody)
	if err != nil {
		t.Fatalf("parse form body: %v", err)
	}
	if got := form.Get("name"); got != "Prospecting A" {
		t.Fatalf("unexpected name value %q", got)
	}
	if got := form.Get("campaign_id"); got != "cp_77" {
		t.Fatalf("unexpected campaign_id value %q", got)
	}
	if got := form.Get("daily_budget"); got != "1200" {
		t.Fatalf("unexpected daily_budget value %q", got)
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
	if result.AdSetID != "51001" {
		t.Fatalf("unexpected ad set id %q", result.AdSetID)
	}
	if result.RequestPath != "act_5678/adsets" {
		t.Fatalf("unexpected request path %q", result.RequestPath)
	}
}

func TestAdSetUpdateExecutesGraphMutation(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"success":true}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewAdSetService(client)

	result, err := service.Update(context.Background(), "v25.0", "token-1", "secret-1", AdSetUpdateInput{
		AdSetID: "91001",
		Params: map[string]string{
			"name": "Updated Ad Set",
		},
	})
	if err != nil {
		t.Fatalf("update ad set: %v", err)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if requestURL.Path != "/v25.0/91001" {
		t.Fatalf("unexpected request path %q", requestURL.Path)
	}

	form, err := url.ParseQuery(stub.lastBody)
	if err != nil {
		t.Fatalf("parse form body: %v", err)
	}
	if got := form.Get("name"); got != "Updated Ad Set" {
		t.Fatalf("unexpected name value %q", got)
	}

	if result.Operation != "update" {
		t.Fatalf("unexpected operation %q", result.Operation)
	}
	if result.AdSetID != "91001" {
		t.Fatalf("unexpected ad set id %q", result.AdSetID)
	}
}

func TestAdSetSetStatusExecutesStatusMutation(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"success":true}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewAdSetService(client)

	result, err := service.SetStatus(context.Background(), "v25.0", "token-1", "secret-1", AdSetStatusInput{
		AdSetID: "91002",
		Status:  AdSetStatusPaused,
	})
	if err != nil {
		t.Fatalf("pause ad set: %v", err)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if requestURL.Path != "/v25.0/91002" {
		t.Fatalf("unexpected request path %q", requestURL.Path)
	}

	form, err := url.ParseQuery(stub.lastBody)
	if err != nil {
		t.Fatalf("parse form body: %v", err)
	}
	if got := form.Get("status"); got != AdSetStatusPaused {
		t.Fatalf("unexpected status %q", got)
	}

	if result.Operation != strings.ToLower(AdSetStatusPaused) {
		t.Fatalf("unexpected operation %q", result.Operation)
	}
}

func TestAdSetCreateFailsWhenResponseMissingID(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"success":true}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewAdSetService(client)

	_, err := service.Create(context.Background(), "v25.0", "token-1", "secret-1", AdSetCreateInput{
		AccountID: "1234",
		Params: map[string]string{
			"name":        "Name",
			"campaign_id": "cp_88",
		},
	})
	if err == nil {
		t.Fatal("expected create error")
	}
	if !strings.Contains(err.Error(), "did not include id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdSetUpdateRejectsEmptyPayload(t *testing.T) {
	t.Parallel()

	service := NewAdSetService(graph.NewClient(nil, ""))
	_, err := service.Update(context.Background(), "v25.0", "token-1", "secret-1", AdSetUpdateInput{
		AdSetID: "91001",
		Params:  map[string]string{},
	})
	if err == nil {
		t.Fatal("expected update error")
	}
	if !strings.Contains(err.Error(), "payload cannot be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}
