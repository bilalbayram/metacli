package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
	"github.com/bilalbayram/metacli/internal/ops"
	"github.com/spf13/cobra"
)

type trackASmokeGraphCall struct {
	method   string
	path     string
	status   int
	response string
	assert   func(*testing.T, *http.Request)
}

func TestTrackAWorkflowSmokeCLICommands(t *testing.T) {
	schemaDir := writeTrackASmokeSchemaPack(t)
	uploadPayloadPath := writeTrackACatalogPayloadFile(t, "upload-items.json", `[{"retailer_id":"sku_track_1","data":{"name":"TrackA Shirt","price":"10.00 USD"}}]`)
	batchPayloadPath := writeTrackACatalogPayloadFile(t, "batch-items.json", `[{"method":"UPDATE","retailer_id":"sku_track_1","data":{"price":"bad"}}]`)
	ledgerPath := filepath.Join(t.TempDir(), "resource-ledger.json")
	t.Setenv(resourceLedgerPathEnv, ledgerPath)

	expectedCalls := []trackASmokeGraphCall{
		{
			method:   http.MethodPost,
			path:     "/v25.0/act_1234/campaigns",
			status:   http.StatusOK,
			response: `{"id":"cmp_1001","name":"TrackA Campaign"}`,
			assert: func(t *testing.T, req *http.Request) {
				form := parseTrackASmokeRequestForm(t, req)
				if got := form.Get("name"); got != "TrackA Campaign" {
					t.Fatalf("unexpected campaign name %q", got)
				}
				if got := form.Get("objective"); got != "OUTCOME_SALES" {
					t.Fatalf("unexpected campaign objective %q", got)
				}
				if got := form.Get("status"); got != "PAUSED" {
					t.Fatalf("unexpected campaign status %q", got)
				}
			},
		},
		{
			method:   http.MethodPost,
			path:     "/v25.0/act_1234/adsets",
			status:   http.StatusOK,
			response: `{"id":"adset_2001","name":"TrackA AdSet"}`,
			assert: func(t *testing.T, req *http.Request) {
				form := parseTrackASmokeRequestForm(t, req)
				if got := form.Get("campaign_id"); got != "cmp_1001" {
					t.Fatalf("unexpected adset campaign_id %q", got)
				}
				if got := form.Get("status"); got != "PAUSED" {
					t.Fatalf("unexpected adset status %q", got)
				}
				if got := form.Get("billing_event"); got != "IMPRESSIONS" {
					t.Fatalf("unexpected billing_event %q", got)
				}
				if got := form.Get("optimization_goal"); got != "OFFSITE_CONVERSIONS" {
					t.Fatalf("unexpected optimization_goal %q", got)
				}
			},
		},
		{
			method:   http.MethodPost,
			path:     "/v25.0/act_1234/adcreatives",
			status:   http.StatusOK,
			response: `{"id":"creative_3001","name":"TrackA Creative"}`,
			assert: func(t *testing.T, req *http.Request) {
				form := parseTrackASmokeRequestForm(t, req)
				if got := form.Get("name"); got != "TrackA Creative" {
					t.Fatalf("unexpected creative name %q", got)
				}
				if got := form.Get("object_story_id"); got != "123_456" {
					t.Fatalf("unexpected object_story_id %q", got)
				}
			},
		},
		{
			method:   http.MethodGet,
			path:     "/v25.0/adset_2001",
			status:   http.StatusOK,
			response: `{"id":"adset_2001"}`,
			assert: func(t *testing.T, req *http.Request) {
				if got := req.URL.Query().Get("fields"); got != "id" {
					t.Fatalf("unexpected adset validation fields %q", got)
				}
			},
		},
		{
			method:   http.MethodGet,
			path:     "/v25.0/creative_3001",
			status:   http.StatusOK,
			response: `{"id":"creative_3001"}`,
			assert: func(t *testing.T, req *http.Request) {
				if got := req.URL.Query().Get("fields"); got != "id" {
					t.Fatalf("unexpected creative validation fields %q", got)
				}
			},
		},
		{
			method:   http.MethodPost,
			path:     "/v25.0/act_1234/ads",
			status:   http.StatusOK,
			response: `{"id":"ad_4001","name":"TrackA Ad"}`,
			assert: func(t *testing.T, req *http.Request) {
				form := parseTrackASmokeRequestForm(t, req)
				if got := form.Get("adset_id"); got != "adset_2001" {
					t.Fatalf("unexpected adset_id %q", got)
				}
				if got := form.Get("creative"); got != `{"creative_id":"creative_3001"}` {
					t.Fatalf("unexpected creative payload %q", got)
				}
			},
		},
		{
			method:   http.MethodPost,
			path:     "/v25.0/act_1234/customaudiences",
			status:   http.StatusOK,
			response: `{"id":"aud_5001","name":"TrackA Audience"}`,
			assert: func(t *testing.T, req *http.Request) {
				form := parseTrackASmokeRequestForm(t, req)
				if got := form.Get("name"); got != "TrackA Audience" {
					t.Fatalf("unexpected audience name %q", got)
				}
				if got := form.Get("subtype"); got != "CUSTOM" {
					t.Fatalf("unexpected audience subtype %q", got)
				}
			},
		},
		{
			method: http.MethodPost,
			path:   "/v25.0/cat_6001/items_batch",
			status: http.StatusOK,
			response: `{
  "handles":[
    {"retailer_id":"sku_track_1","success":true}
  ]
}`,
			assert: func(t *testing.T, req *http.Request) {
				form := parseTrackASmokeRequestForm(t, req)
				assertCatalogRequestsPayload(t, form, "CREATE")
			},
		},
		{
			method: http.MethodPost,
			path:   "/v25.0/cat_6001/items_batch",
			status: http.StatusOK,
			response: `{
  "handles":[
    {"retailer_id":"sku_track_1","errors":[{"message":"invalid price format"}]}
  ]
}`,
			assert: func(t *testing.T, req *http.Request) {
				form := parseTrackASmokeRequestForm(t, req)
				assertCatalogRequestsPayload(t, form, "UPDATE")
			},
		},
	}

	callIndex := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if callIndex >= len(expectedCalls) {
			t.Fatalf("unexpected extra request %s %s", req.Method, req.URL.Path)
		}

		expected := expectedCalls[callIndex]
		callIndex++

		if req.Method != expected.method {
			t.Fatalf("call %d method mismatch: got %s want %s", callIndex, req.Method, expected.method)
		}
		if req.URL.Path != expected.path {
			t.Fatalf("call %d path mismatch: got %s want %s", callIndex, req.URL.Path, expected.path)
		}
		if expected.assert != nil {
			expected.assert(t, req)
		}

		w.Header().Set("Content-Type", "application/json")
		statusCode := expected.status
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		w.WriteHeader(statusCode)
		if _, err := w.Write([]byte(expected.response)); err != nil {
			t.Fatalf("write response body: %v", err)
		}
	}))
	defer server.Close()

	loadFn := func(profile string) (*ProfileCredentials, error) {
		if profile != "prod" {
			t.Fatalf("unexpected profile %q", profile)
		}
		return &ProfileCredentials{
			Name: "prod",
			Profile: config.Profile{
				Domain:       config.DefaultDomain,
				GraphVersion: config.DefaultGraphVersion,
			},
			Token:     "test-token",
			AppSecret: "test-secret",
		}, nil
	}
	clientFn := func() *graph.Client {
		client := graph.NewClient(server.Client(), server.URL)
		client.MaxRetries = 0
		return client
	}

	useCampaignDependencies(t, loadFn, clientFn)
	useAdsetDependencies(t, loadFn, clientFn)
	useCreativeDependencies(t, loadFn, clientFn)
	useAdDependencies(t, loadFn, clientFn)
	useAudienceDependencies(t, loadFn, clientFn)
	useCatalogDependencies(t, loadFn, clientFn)

	campaignEnvelope := runTrackASmokeSuccessCommand(t, NewCampaignCommand(testRuntime("prod")), []string{
		"create",
		"--account-id", "1234",
		"--params", "name=TrackA Campaign,objective=OUTCOME_SALES,status=PAUSED",
		"--schema-dir", schemaDir,
	}, "meta campaign create")
	campaignID := trackASmokeDataString(t, campaignEnvelope, "campaign_id")
	if campaignID != "cmp_1001" {
		t.Fatalf("unexpected campaign id %q", campaignID)
	}

	adsetEnvelope := runTrackASmokeSuccessCommand(t, NewAdsetCommand(testRuntime("prod")), []string{
		"create",
		"--account-id", "1234",
		"--params", "name=TrackA AdSet,campaign_id=cmp_1001,status=PAUSED,billing_event=IMPRESSIONS,optimization_goal=OFFSITE_CONVERSIONS",
		"--schema-dir", schemaDir,
	}, "meta adset create")
	adsetID := trackASmokeDataString(t, adsetEnvelope, "adset_id")
	if adsetID != "adset_2001" {
		t.Fatalf("unexpected adset id %q", adsetID)
	}

	creativeEnvelope := runTrackASmokeSuccessCommand(t, NewCreativeCommand(testRuntime("prod")), []string{
		"create",
		"--account-id", "1234",
		"--params", "name=TrackA Creative,object_story_id=123_456",
		"--schema-dir", schemaDir,
	}, "meta creative create")
	creativeID := trackASmokeDataString(t, creativeEnvelope, "creative_id")
	if creativeID != "creative_3001" {
		t.Fatalf("unexpected creative id %q", creativeID)
	}

	adEnvelope := runTrackASmokeSuccessCommand(t, NewAdCommand(testRuntime("prod")), []string{
		"create",
		"--account-id", "1234",
		"--params", "name=TrackA Ad,adset_id=adset_2001,status=PAUSED",
		"--json", fmt.Sprintf(`{"creative":{"creative_id":"%s"}}`, creativeID),
		"--schema-dir", schemaDir,
	}, "meta ad create")
	adID := trackASmokeDataString(t, adEnvelope, "ad_id")
	if adID != "ad_4001" {
		t.Fatalf("unexpected ad id %q", adID)
	}

	audienceEnvelope := runTrackASmokeSuccessCommand(t, NewAudienceCommand(testRuntime("prod")), []string{
		"create",
		"--account-id", "1234",
		"--params", "name=TrackA Audience,subtype=CUSTOM,description=TrackA smoke audience",
		"--schema-dir", schemaDir,
	}, "meta audience create")
	audienceID := trackASmokeDataString(t, audienceEnvelope, "audience_id")
	if audienceID != "aud_5001" {
		t.Fatalf("unexpected audience id %q", audienceID)
	}

	catalogUploadEnvelope := runTrackASmokeSuccessCommand(t, NewCatalogCommand(testRuntime("prod")), []string{
		"upload-items",
		"--catalog-id", "cat_6001",
		"--file", uploadPayloadPath,
	}, "meta catalog upload-items")
	uploadData := trackASmokeEnvelopeData(t, catalogUploadEnvelope)
	if got := uploadData["success_count"]; got != float64(1) {
		t.Fatalf("unexpected catalog success_count %v", got)
	}
	if got := uploadData["error_count"]; got != float64(0) {
		t.Fatalf("unexpected catalog error_count %v", got)
	}

	catalogErrorEnvelope := runTrackASmokeFailCommand(t, NewCatalogCommand(testRuntime("prod")), []string{
		"batch-items",
		"--catalog-id", "cat_6001",
		"--file", batchPayloadPath,
	}, "meta catalog batch-items", "failed with 1 item error(s)")

	errorData := trackASmokeEnvelopeData(t, catalogErrorEnvelope)
	if got := errorData["error_count"]; got != float64(1) {
		t.Fatalf("unexpected catalog batch error_count %v", got)
	}
	errorBody, ok := catalogErrorEnvelope["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error payload object, got %T", catalogErrorEnvelope["error"])
	}
	if got := errorBody["type"]; got != "catalog_item_errors" {
		t.Fatalf("unexpected error type %v", got)
	}

	if callIndex != len(expectedCalls) {
		t.Fatalf("expected %d graph calls, got %d", len(expectedCalls), callIndex)
	}

	ledger, err := ops.LoadResourceLedger(ledgerPath)
	if err != nil {
		t.Fatalf("load resource ledger: %v", err)
	}
	if len(ledger.Resources) != 5 {
		t.Fatalf("expected five tracked resources, got %d", len(ledger.Resources))
	}
	expectedResources := []struct {
		kind   string
		id     string
		action string
	}{
		{kind: ops.ResourceKindCampaign, id: "cmp_1001", action: ops.CleanupActionPause},
		{kind: ops.ResourceKindAdSet, id: "adset_2001", action: ops.CleanupActionPause},
		{kind: ops.ResourceKindCreative, id: "creative_3001", action: ops.CleanupActionDelete},
		{kind: ops.ResourceKindAd, id: "ad_4001", action: ops.CleanupActionPause},
		{kind: ops.ResourceKindAudience, id: "aud_5001", action: ops.CleanupActionDelete},
	}
	for index, expected := range expectedResources {
		resource := ledger.Resources[index]
		if resource.ResourceKind != expected.kind {
			t.Fatalf("unexpected tracked resource kind at index %d: got=%s want=%s", index, resource.ResourceKind, expected.kind)
		}
		if resource.ResourceID != expected.id {
			t.Fatalf("unexpected tracked resource id at index %d: got=%s want=%s", index, resource.ResourceID, expected.id)
		}
		if resource.CleanupAction != expected.action {
			t.Fatalf("unexpected tracked cleanup action at index %d: got=%s want=%s", index, resource.CleanupAction, expected.action)
		}
	}
}

func runTrackASmokeSuccessCommand(t *testing.T, cmd *cobra.Command, args []string, commandName string) map[string]any {
	t.Helper()

	stdout, stderr, err := executeTrackASmokeCommand(t, cmd, args)
	if err != nil {
		t.Fatalf("execute %s: %v", commandName, err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr for %s, got %q", commandName, stderr.String())
	}

	envelope := decodeEnvelope(t, stdout.Bytes())
	assertEnvelopeBasics(t, envelope, commandName)
	return envelope
}

func runTrackASmokeFailCommand(t *testing.T, cmd *cobra.Command, args []string, commandName string, expectedErrorSubstring string) map[string]any {
	t.Helper()

	stdout, stderr, err := executeTrackASmokeCommand(t, cmd, args)
	if err == nil {
		t.Fatalf("expected command error for %s", commandName)
	}
	if !strings.Contains(err.Error(), expectedErrorSubstring) {
		t.Fatalf("unexpected command error for %s: %v", commandName, err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected empty stdout for %s, got %q", commandName, stdout.String())
	}

	envelope := decodeEnvelope(t, stderr.Bytes())
	if got := envelope["command"]; got != commandName {
		t.Fatalf("unexpected envelope command %v", got)
	}
	if got := envelope["success"]; got != false {
		t.Fatalf("expected success=false for %s, got %v", commandName, got)
	}
	return envelope
}

func executeTrackASmokeCommand(t *testing.T, cmd *cobra.Command, args []string) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout, stderr, err
}

func trackASmokeEnvelopeData(t *testing.T, envelope map[string]any) map[string]any {
	t.Helper()

	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object envelope data, got %T", envelope["data"])
	}
	return data
}

func trackASmokeDataString(t *testing.T, envelope map[string]any, key string) string {
	t.Helper()

	data := trackASmokeEnvelopeData(t, envelope)
	value, ok := data[key].(string)
	if !ok {
		t.Fatalf("expected %s to be a string, got %T", key, data[key])
	}
	return value
}

func parseTrackASmokeRequestForm(t *testing.T, req *http.Request) url.Values {
	t.Helper()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		t.Fatalf("parse request body: %v", err)
	}
	return form
}

func assertCatalogRequestsPayload(t *testing.T, form url.Values, method string) {
	t.Helper()

	if got := form.Get("item_type"); got != "PRODUCT_ITEM" {
		t.Fatalf("unexpected catalog item_type %q", got)
	}

	var requests []map[string]any
	if err := json.Unmarshal([]byte(form.Get("requests")), &requests); err != nil {
		t.Fatalf("decode catalog requests payload: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected one catalog request, got %d", len(requests))
	}
	if got := requests[0]["method"]; got != method {
		t.Fatalf("unexpected catalog request method %v", got)
	}
	if got := requests[0]["retailer_id"]; got != "sku_track_1" {
		t.Fatalf("unexpected retailer_id %v", got)
	}
}

func writeTrackACatalogPayloadFile(t *testing.T, fileName string, payload string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), fileName)
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write catalog payload %s: %v", fileName, err)
	}
	return path
}

func writeTrackASmokeSchemaPack(t *testing.T) string {
	t.Helper()

	schemaDir := t.TempDir()
	marketingDir := filepath.Join(schemaDir, config.DefaultDomain)
	if err := os.MkdirAll(marketingDir, 0o755); err != nil {
		t.Fatalf("create schema dir: %v", err)
	}

	packPath := filepath.Join(marketingDir, config.DefaultGraphVersion+".json")
	pack := `{
  "domain":"marketing",
  "version":"v25.0",
  "entities":{
    "campaign":["id","name","status","objective","daily_budget","lifetime_budget"],
    "adset":["id","name","status","campaign_id","billing_event","optimization_goal","daily_budget","lifetime_budget"],
    "creative":["id","name","object_story_id","object_story_spec","asset_feed_spec"],
    "ad":["id","name","status","adset_id","creative"],
    "audience":["id","name","subtype","description","retention_days"]
  },
  "endpoint_params":{
    "campaigns.post":["name","status","objective","daily_budget","lifetime_budget"],
    "adsets.post":["name","campaign_id","status","billing_event","optimization_goal","daily_budget","lifetime_budget"],
    "adcreatives.post":["name","object_story_id","object_story_spec","asset_feed_spec","url_tags","degrees_of_freedom_spec"],
    "ads.post":["name","adset_id","status","creative"],
    "customaudiences.post":["name","subtype","description","customer_file_source","rule","retention_days","prefill"]
  },
  "deprecated_params":{
    "campaigns.post":["legacy_param"],
    "adsets.post":["legacy_param"],
    "adcreatives.post":["legacy_param"],
    "ads.post":["legacy_param"],
    "customaudiences.post":["legacy_param"]
  }
}`
	if err := os.WriteFile(packPath, []byte(pack), 0o644); err != nil {
		t.Fatalf("write schema pack: %v", err)
	}
	return schemaDir
}
