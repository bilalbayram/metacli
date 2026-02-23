package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
)

func TestNewCatalogCommandIncludesWorkflowSubcommands(t *testing.T) {
	t.Parallel()

	cmd := NewCatalogCommand(Runtime{})

	for _, name := range []string{"upload-items", "batch-items"} {
		sub, _, err := cmd.Find([]string{name})
		if err != nil {
			t.Fatalf("find %s subcommand: %v", name, err)
		}
		if sub == nil || sub.Name() != name {
			t.Fatalf("expected %s subcommand, got %#v", name, sub)
		}
	}
}

func TestCatalogUploadItemsExecutesBatchMutation(t *testing.T) {
	payload := `[{"retailer_id":"sku_1","data":{"name":"Shirt","price":"10.00 USD"}},{"retailer_id":"sku_2","data":{"name":"Hat","price":"5.00 USD"}}]`
	payloadPath := filepath.Join(t.TempDir(), "items.json")
	if err := os.WriteFile(payloadPath, []byte(payload), 0o644); err != nil {
		t.Fatalf("write payload file: %v", err)
	}

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response: `{
  "handles":[
    {"retailer_id":"sku_1","success":true},
    {"retailer_id":"sku_2","success":true}
  ]
}`,
	}
	useCatalogDependencies(t,
		func(profile string) (*ProfileCredentials, error) {
			if profile != "prod" {
				t.Fatalf("unexpected profile %q", profile)
			}
			return &ProfileCredentials{
				Name: "prod",
				Profile: config.Profile{
					Domain:       config.DefaultDomain,
					GraphVersion: config.DefaultGraphVersion,
				},
				Token: "test-token",
			}, nil
		},
		func() *graph.Client {
			client := graph.NewClient(stub, "https://graph.example.com")
			client.MaxRetries = 0
			return client
		},
	)

	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewCatalogCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"upload-items",
		"--catalog-id", "cat_123",
		"--file", payloadPath,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute catalog upload-items: %v", err)
	}

	requestURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if requestURL.Path != "/v25.0/cat_123/items_batch" {
		t.Fatalf("unexpected path %q", requestURL.Path)
	}
	form, err := url.ParseQuery(stub.lastBody)
	if err != nil {
		t.Fatalf("parse form body: %v", err)
	}
	if got := form.Get("item_type"); got != "PRODUCT_ITEM" {
		t.Fatalf("unexpected item_type %q", got)
	}

	var requests []map[string]any
	if err := json.Unmarshal([]byte(form.Get("requests")), &requests); err != nil {
		t.Fatalf("decode requests payload: %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("unexpected requests count %d", len(requests))
	}
	if got := requests[0]["method"]; got != "CREATE" {
		t.Fatalf("unexpected first method %#v", got)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta catalog upload-items")
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object payload, got %T", envelope["data"])
	}
	if got := data["success_count"]; got != float64(2) {
		t.Fatalf("unexpected success_count %v", got)
	}
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestCatalogBatchItemsAggregatesItemErrorsInStructuredEnvelope(t *testing.T) {
	payload := `[{"method":"UPDATE","retailer_id":"sku_1","data":{"price":"11.00 USD"}},{"method":"UPDATE","retailer_id":"sku_2","data":{"price":"bad"}}]`
	payloadPath := filepath.Join(t.TempDir(), "requests.json")
	if err := os.WriteFile(payloadPath, []byte(payload), 0o644); err != nil {
		t.Fatalf("write payload file: %v", err)
	}

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response: `{
  "handles":[
    {"retailer_id":"sku_1","success":true},
    {"retailer_id":"sku_2","errors":[{"message":"invalid price format"}]}
  ]
}`,
	}
	useCatalogDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name: "prod",
				Profile: config.Profile{
					Domain:       config.DefaultDomain,
					GraphVersion: config.DefaultGraphVersion,
				},
				Token: "test-token",
			}, nil
		},
		func() *graph.Client {
			client := graph.NewClient(stub, "https://graph.example.com")
			client.MaxRetries = 0
			return client
		},
	)

	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewCatalogCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"batch-items",
		"--catalog-id", "cat_123",
		"--file", payloadPath,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "failed with 1 item error(s)") {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", output.String())
	}

	envelope := decodeEnvelope(t, errOutput.Bytes())
	if got := envelope["command"]; got != "meta catalog batch-items" {
		t.Fatalf("unexpected command field %v", got)
	}
	if envelope["success"] != false {
		t.Fatalf("expected success=false, got %v", envelope["success"])
	}
	errorBody, ok := envelope["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error payload, got %T", envelope["error"])
	}
	if got := errorBody["type"]; got != "catalog_item_errors" {
		t.Fatalf("unexpected error type %v", got)
	}

	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data payload in error envelope, got %T", envelope["data"])
	}
	if got := data["error_count"]; got != float64(1) {
		t.Fatalf("unexpected error_count %v", got)
	}
}

func TestCatalogUploadItemsRequiresSingleInputSource(t *testing.T) {
	useCatalogDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name: "prod",
				Profile: config.Profile{
					Domain:       config.DefaultDomain,
					GraphVersion: config.DefaultGraphVersion,
				},
				Token: "test-token",
			}, nil
		},
		func() *graph.Client {
			return graph.NewClient(nil, "")
		},
	)

	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewCatalogCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"upload-items",
		"--catalog-id", "cat_123",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "either --file or --json must be provided") {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", output.String())
	}
	envelope := decodeEnvelope(t, errOutput.Bytes())
	if got := envelope["command"]; got != "meta catalog upload-items" {
		t.Fatalf("unexpected command field %v", got)
	}
}

func useCatalogDependencies(t *testing.T, loadFn func(string) (*ProfileCredentials, error), clientFn func() *graph.Client) {
	t.Helper()
	originalLoad := catalogLoadProfileCredentials
	originalClient := catalogNewGraphClient
	t.Cleanup(func() {
		catalogLoadProfileCredentials = originalLoad
		catalogNewGraphClient = originalClient
	})

	catalogLoadProfileCredentials = loadFn
	catalogNewGraphClient = clientFn
}
