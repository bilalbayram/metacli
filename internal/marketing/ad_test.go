package marketing

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/bilalbayram/metacli/internal/graph"
)

func TestAdCreateValidatesDependenciesBeforeMutation(t *testing.T) {
	t.Parallel()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		switch requestCount {
		case 1:
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected adset validation method %s", r.Method)
			}
			if r.URL.Path != "/v25.0/adset_1" {
				t.Fatalf("unexpected adset validation path %q", r.URL.Path)
			}
			if got := r.URL.Query().Get("fields"); got != "id" {
				t.Fatalf("unexpected adset validation fields %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "adset_1"})
		case 2:
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected creative validation method %s", r.Method)
			}
			if r.URL.Path != "/v25.0/creative_1" {
				t.Fatalf("unexpected creative validation path %q", r.URL.Path)
			}
			if got := r.URL.Query().Get("fields"); got != "id" {
				t.Fatalf("unexpected creative validation fields %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "creative_1"})
		case 3:
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected create method %s", r.Method)
			}
			if r.URL.Path != "/v25.0/act_999/ads" {
				t.Fatalf("unexpected create path %q", r.URL.Path)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read create body: %v", err)
			}
			form, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("parse create body: %v", err)
			}
			if got := form.Get("adset_id"); got != "adset_1" {
				t.Fatalf("unexpected adset_id %q", got)
			}
			if got := form.Get("creative"); got != `{"creative_id":"creative_1"}` {
				t.Fatalf("unexpected creative payload %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "new_ad_1"})
		default:
			t.Fatalf("unexpected request count %d", requestCount)
		}
	}))
	defer server.Close()

	client := graph.NewClient(server.Client(), server.URL)
	service := NewAdService(client)

	result, err := service.Create(context.Background(), "v25.0", "token-1", "", AdCreateInput{
		AccountID: "999",
		Params: map[string]string{
			"name":     "Creative Iteration A",
			"adset_id": "adset_1",
			"creative": `{"creative_id":"creative_1"}`,
			"status":   "PAUSED",
		},
	})
	if err != nil {
		t.Fatalf("create ad: %v", err)
	}

	if requestCount != 3 {
		t.Fatalf("expected three requests, got %d", requestCount)
	}
	if result.Operation != "create" {
		t.Fatalf("unexpected operation %q", result.Operation)
	}
	if result.AdID != "new_ad_1" {
		t.Fatalf("unexpected ad id %q", result.AdID)
	}
}

func TestAdUpdateSkipsDependencyValidationWhenReferencesNotPresent(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"success":true}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewAdService(client)

	result, err := service.Update(context.Background(), "v25.0", "token-1", "secret-1", AdUpdateInput{
		AdID: "ad_22",
		Params: map[string]string{
			"name": "Updated Name",
		},
	})
	if err != nil {
		t.Fatalf("update ad: %v", err)
	}

	if stub.calls != 1 {
		t.Fatalf("expected one request, got %d", stub.calls)
	}
	if !strings.Contains(stub.lastURL, "/v25.0/ad_22") {
		t.Fatalf("unexpected update URL %q", stub.lastURL)
	}
	if result.AdID != "ad_22" {
		t.Fatalf("unexpected ad id %q", result.AdID)
	}
}

func TestAdCreateFailsWhenAdSetValidationResponseMissingID(t *testing.T) {
	t.Parallel()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	}))
	defer server.Close()

	client := graph.NewClient(server.Client(), server.URL)
	service := NewAdService(client)

	_, err := service.Create(context.Background(), "v25.0", "token-1", "", AdCreateInput{
		AccountID: "999",
		Params: map[string]string{
			"name":     "Creative Iteration A",
			"adset_id": "adset_1",
		},
	})
	if err == nil {
		t.Fatal("expected create error")
	}
	if !strings.Contains(err.Error(), "validation response did not include id") {
		t.Fatalf("unexpected error: %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("expected only one validation request, got %d", requestCount)
	}
}

func TestAdCreateFailsOnInvalidCreativeReference(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"id":"new_ad_1"}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewAdService(client)

	_, err := service.Create(context.Background(), "v25.0", "token-1", "", AdCreateInput{
		AccountID: "999",
		Params: map[string]string{
			"name":     "Creative Iteration A",
			"creative": `{"object_story_spec":{"page_id":"1"}}`,
		},
	})
	if err == nil {
		t.Fatal("expected create error")
	}
	if !strings.Contains(err.Error(), "creative reference must include creative_id or id") {
		t.Fatalf("unexpected error: %v", err)
	}
	if stub.calls != 0 {
		t.Fatalf("expected zero requests, got %d", stub.calls)
	}
}

func TestAdCloneReadsSanitizesValidatesAndCreates(t *testing.T) {
	t.Parallel()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		switch requestCount {
		case 1:
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected source read method %s", r.Method)
			}
			if r.URL.Path != "/v25.0/source_ad_9" {
				t.Fatalf("unexpected source read path %q", r.URL.Path)
			}
			if got := r.URL.Query().Get("fields"); got != "id,name,status,adset_id,creative,account_id,effective_status" {
				t.Fatalf("unexpected clone fields %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":               "source_ad_9",
				"name":             "Source Ad",
				"status":           "PAUSED",
				"adset_id":         "adset_2",
				"creative":         map[string]any{"id": "creative_2"},
				"account_id":       "123",
				"effective_status": "PAUSED",
			})
		case 2:
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected adset validation method %s", r.Method)
			}
			if r.URL.Path != "/v25.0/adset_2" {
				t.Fatalf("unexpected adset validation path %q", r.URL.Path)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "adset_2"})
		case 3:
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected creative validation method %s", r.Method)
			}
			if r.URL.Path != "/v25.0/creative_2" {
				t.Fatalf("unexpected creative validation path %q", r.URL.Path)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "creative_2"})
		case 4:
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected clone create method %s", r.Method)
			}
			if r.URL.Path != "/v25.0/act_321/ads" {
				t.Fatalf("unexpected clone create path %q", r.URL.Path)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read clone create body: %v", err)
			}
			form, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("parse clone create body: %v", err)
			}
			if got := form.Get("name"); got != "Clone Ad" {
				t.Fatalf("unexpected cloned name %q", got)
			}
			if got := form.Get("id"); got != "" {
				t.Fatalf("immutable id should not be cloned, got %q", got)
			}
			if got := form.Get("account_id"); got != "" {
				t.Fatalf("immutable account_id should not be cloned, got %q", got)
			}
			if got := form.Get("creative"); got != `{"creative_id":"creative_2"}` {
				t.Fatalf("unexpected creative payload %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "clone_ad_11"})
		default:
			t.Fatalf("unexpected request count %d", requestCount)
		}
	}))
	defer server.Close()

	client := graph.NewClient(server.Client(), server.URL)
	service := NewAdService(client)

	result, err := service.Clone(context.Background(), "v25.0", "token-1", "", AdCloneInput{
		SourceAdID:      "source_ad_9",
		TargetAccountID: "321",
		Overrides: map[string]string{
			"name": "Clone Ad",
		},
		Fields: []string{"id", "name", "status", "adset_id", "creative", "account_id", "effective_status"},
	})
	if err != nil {
		t.Fatalf("clone ad: %v", err)
	}

	if requestCount != 4 {
		t.Fatalf("expected four requests, got %d", requestCount)
	}
	if result.Operation != "clone" {
		t.Fatalf("unexpected operation %q", result.Operation)
	}
	if result.SourceAdID != "source_ad_9" {
		t.Fatalf("unexpected source ad id %q", result.SourceAdID)
	}
	if result.AdID != "clone_ad_11" {
		t.Fatalf("unexpected ad id %q", result.AdID)
	}
	if got := result.Payload["creative"]; got != `{"creative_id":"creative_2"}` {
		t.Fatalf("unexpected normalized creative payload %q", got)
	}
	if !slices.Contains(result.RemovedFields, "id") {
		t.Fatalf("expected removed fields to contain id, got %v", result.RemovedFields)
	}
	if !slices.Contains(result.RemovedFields, "account_id") {
		t.Fatalf("expected removed fields to contain account_id, got %v", result.RemovedFields)
	}
}

func TestAdCloneFailsWhenOverrideBreaksCreativeReference(t *testing.T) {
	t.Parallel()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		switch requestCount {
		case 1:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "source_ad_9",
				"name":     "Source Ad",
				"adset_id": "adset_2",
				"creative": map[string]any{"id": "creative_2"},
			})
		case 2:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "adset_2"})
		default:
			t.Fatalf("unexpected request count %d", requestCount)
		}
	}))
	defer server.Close()

	client := graph.NewClient(server.Client(), server.URL)
	service := NewAdService(client)

	_, err := service.Clone(context.Background(), "v25.0", "token-1", "", AdCloneInput{
		SourceAdID:      "source_ad_9",
		TargetAccountID: "321",
		Overrides: map[string]string{
			"creative": `{"object_story_spec":{"page_id":"1"}}`,
		},
	})
	if err == nil {
		t.Fatal("expected clone error")
	}
	if !strings.Contains(err.Error(), "creative reference must include creative_id or id") {
		t.Fatalf("unexpected error: %v", err)
	}
	var apiErr *graph.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected graph API error, got %T", err)
	}
	if apiErr.Code != adValidationCodeCloneDependency {
		t.Fatalf("unexpected error code %d", apiErr.Code)
	}
	if apiErr.Remediation == nil || !slices.Equal(apiErr.Remediation.Fields, []string{"creative"}) {
		t.Fatalf("unexpected remediation fields %#v", apiErr.Remediation)
	}
	if requestCount != 1 {
		t.Fatalf("expected only source read request, got %d", requestCount)
	}
}

func TestAdCloneReturnsRemediationWhenSourceMissingCreative(t *testing.T) {
	t.Parallel()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount != 1 {
			t.Fatalf("unexpected request count %d", requestCount)
		}
		if got := r.URL.Query().Get("fields"); got != "id,name,adset_id,creative" {
			t.Fatalf("unexpected clone fields %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "source_ad_10",
			"name":     "Source Ad",
			"adset_id": "adset_10",
		})
	}))
	defer server.Close()

	client := graph.NewClient(server.Client(), server.URL)
	service := NewAdService(client)

	_, err := service.Clone(context.Background(), "v25.0", "token-1", "", AdCloneInput{
		SourceAdID:      "source_ad_10",
		TargetAccountID: "321",
		Fields:          []string{"id", "name"},
	})
	if err == nil {
		t.Fatal("expected clone error")
	}

	var apiErr *graph.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected graph API error, got %T", err)
	}
	if apiErr.Type != adValidationErrorType {
		t.Fatalf("unexpected error type %q", apiErr.Type)
	}
	if apiErr.Code != adValidationCodeCloneIncomplete {
		t.Fatalf("unexpected error code %d", apiErr.Code)
	}
	if apiErr.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unexpected status code %d", apiErr.StatusCode)
	}
	if apiErr.Remediation == nil {
		t.Fatalf("expected remediation details, got nil")
	}
	if apiErr.Remediation.Category != graph.RemediationCategoryValidation {
		t.Fatalf("unexpected remediation category %q", apiErr.Remediation.Category)
	}
	if !slices.Equal(apiErr.Remediation.Fields, []string{"creative"}) {
		t.Fatalf("unexpected remediation fields %v", apiErr.Remediation.Fields)
	}
	if !slices.Contains(apiErr.Remediation.Actions, "Include the missing fields in --fields or provide overrides via --params/--json.") {
		t.Fatalf("unexpected remediation actions %v", apiErr.Remediation.Actions)
	}
	if requestCount != 1 {
		t.Fatalf("expected only source read request, got %d", requestCount)
	}
}

func TestAdCreateFailsWhenResponseMissingID(t *testing.T) {
	t.Parallel()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		switch requestCount {
		case 1:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "adset_1"})
		case 2:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "creative_1"})
		case 3:
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			t.Fatalf("unexpected request count %d", requestCount)
		}
	}))
	defer server.Close()

	client := graph.NewClient(server.Client(), server.URL)
	service := NewAdService(client)

	_, err := service.Create(context.Background(), "v25.0", "token-1", "", AdCreateInput{
		AccountID: "999",
		Params: map[string]string{
			"name":     "Creative Iteration A",
			"adset_id": "adset_1",
			"creative": `{"creative_id":"creative_1"}`,
		},
	})
	if err == nil {
		t.Fatal("expected create error")
	}
	if !strings.Contains(err.Error(), "did not include id") {
		t.Fatalf("unexpected error: %v", err)
	}
}
