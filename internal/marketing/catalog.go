package marketing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/bilalbayram/metacli/internal/graph"
)

const (
	CatalogBatchMethodCreate = "CREATE"
	CatalogBatchMethodUpdate = "UPDATE"
	CatalogBatchMethodDelete = "DELETE"
	CatalogBatchMethodUpsert = "UPSERT"

	defaultCatalogItemType = "PRODUCT_ITEM"
)

type CatalogService struct {
	Client *graph.Client
}

type CatalogUploadItem struct {
	RetailerID string         `json:"retailer_id"`
	Data       map[string]any `json:"data"`
}

type CatalogBatchRequest struct {
	Method     string         `json:"method"`
	RetailerID string         `json:"retailer_id,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
}

type CatalogUploadItemsInput struct {
	CatalogID string
	ItemType  string
	Items     []CatalogUploadItem
}

type CatalogBatchItemsInput struct {
	CatalogID string
	ItemType  string
	Requests  []CatalogBatchRequest
}

type CatalogItemError struct {
	Index      int    `json:"index"`
	Method     string `json:"method"`
	RetailerID string `json:"retailer_id,omitempty"`
	Message    string `json:"message"`
}

type CatalogBatchItemResult struct {
	Index      int            `json:"index"`
	Method     string         `json:"method"`
	RetailerID string         `json:"retailer_id,omitempty"`
	Success    bool           `json:"success"`
	Errors     []string       `json:"errors,omitempty"`
	Raw        map[string]any `json:"raw"`
}

type CatalogBatchResult struct {
	Operation    string                   `json:"operation"`
	CatalogID    string                   `json:"catalog_id"`
	ItemType     string                   `json:"item_type"`
	RequestPath  string                   `json:"request_path"`
	TotalItems   int                      `json:"total_items"`
	SuccessCount int                      `json:"success_count"`
	ErrorCount   int                      `json:"error_count"`
	ItemErrors   []CatalogItemError       `json:"item_errors,omitempty"`
	Items        []CatalogBatchItemResult `json:"items"`
	Response     map[string]any           `json:"response"`
}

type CatalogBatchItemErrors struct {
	Operation  string
	ItemErrors []CatalogItemError
}

func (e *CatalogBatchItemErrors) Error() string {
	if e == nil || len(e.ItemErrors) == 0 {
		return "catalog batch failed with item errors"
	}

	previewCount := 3
	if len(e.ItemErrors) < previewCount {
		previewCount = len(e.ItemErrors)
	}

	preview := make([]string, 0, previewCount)
	for i := 0; i < previewCount; i++ {
		item := e.ItemErrors[i]
		retailerPart := ""
		if strings.TrimSpace(item.RetailerID) != "" {
			retailerPart = fmt.Sprintf(" retailer_id=%s", item.RetailerID)
		}
		preview = append(preview, fmt.Sprintf("item[%d]%s: %s", item.Index, retailerPart, item.Message))
	}

	extra := ""
	if len(e.ItemErrors) > previewCount {
		extra = fmt.Sprintf(" (+%d more)", len(e.ItemErrors)-previewCount)
	}

	operation := strings.ReplaceAll(strings.TrimSpace(e.Operation), "_", " ")
	if operation == "" {
		operation = "catalog batch"
	}
	return fmt.Sprintf("%s failed with %d item error(s): %s%s", operation, len(e.ItemErrors), strings.Join(preview, "; "), extra)
}

func NewCatalogService(client *graph.Client) *CatalogService {
	if client == nil {
		client = graph.NewClient(nil, "")
	}
	return &CatalogService{Client: client}
}

func (s *CatalogService) UploadItems(ctx context.Context, version string, token string, appSecret string, input CatalogUploadItemsInput) (*CatalogBatchResult, error) {
	requests, err := normalizeCatalogUploadItems(input.Items)
	if err != nil {
		return nil, err
	}
	return s.executeBatchItems(ctx, version, token, appSecret, "upload_items", CatalogBatchItemsInput{
		CatalogID: input.CatalogID,
		ItemType:  input.ItemType,
		Requests:  requests,
	})
}

func (s *CatalogService) BatchItems(ctx context.Context, version string, token string, appSecret string, input CatalogBatchItemsInput) (*CatalogBatchResult, error) {
	return s.executeBatchItems(ctx, version, token, appSecret, "batch_items", input)
}

func (s *CatalogService) executeBatchItems(ctx context.Context, version string, token string, appSecret string, operation string, input CatalogBatchItemsInput) (*CatalogBatchResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("catalog service client is required")
	}

	catalogID, itemType, requests, err := normalizeCatalogBatchInput(input)
	if err != nil {
		return nil, err
	}

	requestsPayload := make([]map[string]any, 0, len(requests))
	for _, req := range requests {
		entry := map[string]any{
			"method":      req.Method,
			"retailer_id": req.RetailerID,
		}
		if len(req.Data) > 0 {
			entry["data"] = req.Data
		}
		requestsPayload = append(requestsPayload, entry)
	}
	encodedRequests, err := json.Marshal(requestsPayload)
	if err != nil {
		return nil, fmt.Errorf("encode catalog requests payload: %w", err)
	}

	path := fmt.Sprintf("%s/items_batch", catalogID)
	response, err := s.Client.Do(ctx, graph.Request{
		Method:  "POST",
		Path:    path,
		Version: strings.TrimSpace(version),
		Form: map[string]string{
			"item_type": itemType,
			"requests":  string(encodedRequests),
		},
		AccessToken: token,
		AppSecret:   appSecret,
	})
	if err != nil {
		return nil, err
	}

	results, itemErrors, err := parseCatalogBatchItemsResponse(response.Body, requests)
	if err != nil {
		return nil, err
	}

	successCount := 0
	for _, result := range results {
		if result.Success {
			successCount++
		}
	}
	errorCount := len(results) - successCount

	batchResult := &CatalogBatchResult{
		Operation:    operation,
		CatalogID:    catalogID,
		ItemType:     itemType,
		RequestPath:  path,
		TotalItems:   len(results),
		SuccessCount: successCount,
		ErrorCount:   errorCount,
		ItemErrors:   itemErrors,
		Items:        results,
		Response:     response.Body,
	}
	if len(itemErrors) > 0 {
		return batchResult, &CatalogBatchItemErrors{
			Operation:  operation,
			ItemErrors: itemErrors,
		}
	}
	return batchResult, nil
}

func normalizeCatalogUploadItems(items []CatalogUploadItem) ([]CatalogBatchRequest, error) {
	if len(items) == 0 {
		return nil, errors.New("catalog upload items list cannot be empty")
	}

	requests := make([]CatalogBatchRequest, 0, len(items))
	for idx, item := range items {
		retailerID := strings.TrimSpace(item.RetailerID)
		if retailerID == "" {
			return nil, fmt.Errorf("catalog upload item %d retailer_id is required", idx)
		}
		if len(item.Data) == 0 {
			return nil, fmt.Errorf("catalog upload item %d data is required", idx)
		}
		data := map[string]any{}
		for key, value := range item.Data {
			trimmedKey := strings.TrimSpace(key)
			if trimmedKey == "" {
				return nil, fmt.Errorf("catalog upload item %d data key cannot be empty", idx)
			}
			data[trimmedKey] = value
		}
		requests = append(requests, CatalogBatchRequest{
			Method:     CatalogBatchMethodCreate,
			RetailerID: retailerID,
			Data:       data,
		})
	}
	return requests, nil
}

func normalizeCatalogBatchInput(input CatalogBatchItemsInput) (string, string, []CatalogBatchRequest, error) {
	catalogID, err := normalizeGraphID("catalog id", input.CatalogID)
	if err != nil {
		return "", "", nil, err
	}
	itemType, err := normalizeCatalogItemType(input.ItemType)
	if err != nil {
		return "", "", nil, err
	}
	requests, err := normalizeCatalogBatchRequests(input.Requests)
	if err != nil {
		return "", "", nil, err
	}
	return catalogID, itemType, requests, nil
}

func normalizeCatalogItemType(value string) (string, error) {
	itemType := strings.ToUpper(strings.TrimSpace(value))
	if itemType == "" {
		itemType = defaultCatalogItemType
	}
	if strings.Contains(itemType, " ") || strings.Contains(itemType, "/") {
		return "", fmt.Errorf("invalid catalog item type %q", value)
	}
	return itemType, nil
}

func normalizeCatalogBatchRequests(requests []CatalogBatchRequest) ([]CatalogBatchRequest, error) {
	if len(requests) == 0 {
		return nil, errors.New("catalog batch requests cannot be empty")
	}

	normalized := make([]CatalogBatchRequest, 0, len(requests))
	for idx, request := range requests {
		method, err := normalizeCatalogBatchMethod(request.Method)
		if err != nil {
			return nil, fmt.Errorf("catalog batch request %d: %w", idx, err)
		}
		retailerID := strings.TrimSpace(request.RetailerID)
		if retailerID == "" {
			return nil, fmt.Errorf("catalog batch request %d retailer_id is required", idx)
		}

		data := map[string]any{}
		for key, value := range request.Data {
			trimmedKey := strings.TrimSpace(key)
			if trimmedKey == "" {
				return nil, fmt.Errorf("catalog batch request %d data key cannot be empty", idx)
			}
			data[trimmedKey] = value
		}

		if method != CatalogBatchMethodDelete && len(data) == 0 {
			return nil, fmt.Errorf("catalog batch request %d data is required for method %s", idx, method)
		}

		normalized = append(normalized, CatalogBatchRequest{
			Method:     method,
			RetailerID: retailerID,
			Data:       data,
		})
	}
	return normalized, nil
}

func normalizeCatalogBatchMethod(method string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(method))
	switch normalized {
	case CatalogBatchMethodCreate, CatalogBatchMethodUpdate, CatalogBatchMethodDelete, CatalogBatchMethodUpsert:
		return normalized, nil
	case "":
		return "", errors.New("method is required")
	default:
		return "", fmt.Errorf("unsupported method %q: expected CREATE|UPDATE|DELETE|UPSERT", method)
	}
}

func parseCatalogBatchItemsResponse(response map[string]any, requests []CatalogBatchRequest) ([]CatalogBatchItemResult, []CatalogItemError, error) {
	rawItems, err := extractCatalogBatchItems(response)
	if err != nil {
		return nil, nil, err
	}
	if len(rawItems) != len(requests) {
		return nil, nil, fmt.Errorf("catalog batch response item count %d did not match request count %d", len(rawItems), len(requests))
	}

	results := make([]CatalogBatchItemResult, 0, len(rawItems))
	itemErrors := make([]CatalogItemError, 0)
	for idx, raw := range rawItems {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("catalog batch response item %d was not an object", idx)
		}

		itemResultErrors := collectCatalogItemErrorMessages(item)
		success := resolveCatalogBatchItemSuccess(item, len(itemResultErrors) == 0)
		if len(itemResultErrors) > 0 {
			success = false
		}

		retailerID := strings.TrimSpace(stringFromCatalogAny(item["retailer_id"]))
		if retailerID == "" {
			retailerID = requests[idx].RetailerID
		}
		if !success && len(itemResultErrors) == 0 {
			itemResultErrors = append(itemResultErrors, "item reported unsuccessful status")
		}

		results = append(results, CatalogBatchItemResult{
			Index:      idx,
			Method:     requests[idx].Method,
			RetailerID: retailerID,
			Success:    success,
			Errors:     itemResultErrors,
			Raw:        item,
		})

		if !success {
			for _, message := range itemResultErrors {
				itemErrors = append(itemErrors, CatalogItemError{
					Index:      idx,
					Method:     requests[idx].Method,
					RetailerID: retailerID,
					Message:    message,
				})
			}
		}
	}

	return results, itemErrors, nil
}

func extractCatalogBatchItems(response map[string]any) ([]any, error) {
	for _, key := range []string{"handles", "responses", "results", "data"} {
		raw, ok := response[key]
		if !ok {
			continue
		}
		items, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("catalog batch response %q field must be an array", key)
		}
		return items, nil
	}
	return nil, errors.New("catalog batch response did not include per-item entries")
}

func collectCatalogItemErrorMessages(item map[string]any) []string {
	messages := make([]string, 0)
	appendMessages := func(raw any) {
		for _, message := range flattenCatalogErrorMessages(raw) {
			trimmed := strings.TrimSpace(message)
			if trimmed == "" {
				continue
			}
			messages = append(messages, trimmed)
		}
	}

	if raw, ok := item["errors"]; ok {
		appendMessages(raw)
	}
	if raw, ok := item["error"]; ok {
		appendMessages(raw)
	}
	return messages
}

func flattenCatalogErrorMessages(raw any) []string {
	switch typed := raw.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{typed}
	case []any:
		out := make([]string, 0, len(typed))
		for _, value := range typed {
			out = append(out, flattenCatalogErrorMessages(value)...)
		}
		return out
	case map[string]any:
		if message := strings.TrimSpace(stringFromCatalogAny(typed["message"])); message != "" {
			return []string{message}
		}
		if message := strings.TrimSpace(stringFromCatalogAny(typed["error_user_msg"])); message != "" {
			return []string{message}
		}
		if nested, ok := typed["error"]; ok {
			return flattenCatalogErrorMessages(nested)
		}
		encoded, err := json.Marshal(typed)
		if err != nil {
			return []string{fmt.Sprintf("%v", typed)}
		}
		return []string{string(encoded)}
	default:
		if raw == nil {
			return nil
		}
		return []string{fmt.Sprintf("%v", raw)}
	}
}

func resolveCatalogBatchItemSuccess(item map[string]any, fallback bool) bool {
	if raw, ok := item["success"]; ok {
		if success, isBool := raw.(bool); isBool {
			return success
		}
	}
	if raw, ok := item["status"]; ok {
		status := strings.ToLower(strings.TrimSpace(stringFromCatalogAny(raw)))
		switch {
		case strings.Contains(status, "fail"), strings.Contains(status, "error"), strings.Contains(status, "reject"):
			return false
		case strings.Contains(status, "success"), strings.Contains(status, "ok"), strings.Contains(status, "complete"):
			return true
		}
	}
	return fallback
}

func stringFromCatalogAny(value any) string {
	typed, _ := value.(string)
	return typed
}
