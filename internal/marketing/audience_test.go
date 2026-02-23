package marketing

import (
	"context"
	"net/http"
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
