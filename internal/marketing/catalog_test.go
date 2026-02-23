package marketing

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/bilalbayram/metacli/internal/graph"
)

func TestCatalogUploadItemsExecutesItemsBatchMutation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if r.URL.Path != "/v25.0/cat_123/items_batch" {
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		form, err := url.ParseQuery(string(body))
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
			t.Fatalf("unexpected requests length %d", len(requests))
		}
		if got := requests[0]["method"]; got != CatalogBatchMethodCreate {
			t.Fatalf("unexpected method %#v", got)
		}
		if got := requests[1]["retailer_id"]; got != "sku_2" {
			t.Fatalf("unexpected retailer id %#v", got)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"handles": []map[string]any{
				{"retailer_id": "sku_1", "success": true},
				{"retailer_id": "sku_2", "success": true},
			},
		})
	}))
	defer server.Close()

	client := graph.NewClient(server.Client(), server.URL)
	client.MaxRetries = 0
	service := NewCatalogService(client)

	result, err := service.UploadItems(context.Background(), "v25.0", "token-1", "secret-1", CatalogUploadItemsInput{
		CatalogID: "cat_123",
		Items: []CatalogUploadItem{
			{
				RetailerID: "sku_1",
				Data: map[string]any{
					"name":  "Shirt",
					"price": "10.00 USD",
				},
			},
			{
				RetailerID: "sku_2",
				Data: map[string]any{
					"name":  "Hat",
					"price": "5.00 USD",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("upload catalog items: %v", err)
	}

	if result.Operation != "upload_items" {
		t.Fatalf("unexpected operation %q", result.Operation)
	}
	if result.CatalogID != "cat_123" {
		t.Fatalf("unexpected catalog id %q", result.CatalogID)
	}
	if result.TotalItems != 2 {
		t.Fatalf("unexpected total items %d", result.TotalItems)
	}
	if result.SuccessCount != 2 || result.ErrorCount != 0 {
		t.Fatalf("unexpected success/error counts %d/%d", result.SuccessCount, result.ErrorCount)
	}
}

func TestCatalogBatchItemsAggregatesPerItemErrors(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response: `{
  "handles":[
    {"retailer_id":"sku_1","success":true},
    {"retailer_id":"sku_2","errors":[{"message":"invalid price value"}]}
  ]
}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewCatalogService(client)

	result, err := service.BatchItems(context.Background(), "v25.0", "token-1", "secret-1", CatalogBatchItemsInput{
		CatalogID: "cat_123",
		Requests: []CatalogBatchRequest{
			{
				Method:     CatalogBatchMethodUpdate,
				RetailerID: "sku_1",
				Data: map[string]any{
					"price": "11.00 USD",
				},
			},
			{
				Method:     CatalogBatchMethodUpdate,
				RetailerID: "sku_2",
				Data: map[string]any{
					"price": "not-a-price",
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected aggregated item error")
	}

	var itemErr *CatalogBatchItemErrors
	if !errors.As(err, &itemErr) {
		t.Fatalf("expected CatalogBatchItemErrors, got %T", err)
	}
	if len(itemErr.ItemErrors) != 1 {
		t.Fatalf("unexpected item errors length %d", len(itemErr.ItemErrors))
	}
	if !strings.Contains(itemErr.Error(), "invalid price value") {
		t.Fatalf("unexpected error message: %v", itemErr.Error())
	}

	if result == nil {
		t.Fatal("expected non-nil result on item errors")
	}
	if result.ErrorCount != 1 {
		t.Fatalf("unexpected error count %d", result.ErrorCount)
	}
	if len(result.ItemErrors) != 1 {
		t.Fatalf("unexpected item errors in result %d", len(result.ItemErrors))
	}
}

func TestCatalogBatchItemsFailsOnMismatchedResultCount(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"handles":[{"retailer_id":"sku_1","success":true}]}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := NewCatalogService(client)

	_, err := service.BatchItems(context.Background(), "v25.0", "token-1", "secret-1", CatalogBatchItemsInput{
		CatalogID: "cat_123",
		Requests: []CatalogBatchRequest{
			{
				Method:     CatalogBatchMethodDelete,
				RetailerID: "sku_1",
			},
			{
				Method:     CatalogBatchMethodDelete,
				RetailerID: "sku_2",
			},
		},
	})
	if err == nil {
		t.Fatal("expected mismatch error")
	}
	if !strings.Contains(err.Error(), "did not match request count") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCatalogBatchItemsRejectsUnsupportedMethod(t *testing.T) {
	t.Parallel()

	service := NewCatalogService(graph.NewClient(nil, ""))
	_, err := service.BatchItems(context.Background(), "v25.0", "token-1", "secret-1", CatalogBatchItemsInput{
		CatalogID: "cat_123",
		Requests: []CatalogBatchRequest{
			{
				Method:     "MERGE",
				RetailerID: "sku_1",
				Data: map[string]any{
					"name": "Shirt",
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected method validation error")
	}
	if !strings.Contains(err.Error(), "unsupported method") {
		t.Fatalf("unexpected error: %v", err)
	}
}
