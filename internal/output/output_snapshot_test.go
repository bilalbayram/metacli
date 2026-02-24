package output

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvelopeContractSnapshotJSONSuccess(t *testing.T) {
	t.Parallel()

	envelope, err := NewEnvelope(
		"meta campaign create",
		true,
		map[string]any{
			"id":     "cmp_1001",
			"status": "PAUSED",
		},
		map[string]any{
			"cursors": map[string]any{
				"before": "a",
				"after":  "b",
			},
			"next": "https://graph.facebook.com/v25.0/act_1/campaigns?after=b",
		},
		map[string]any{
			"x-app-usage": map[string]any{
				"call_count":    13,
				"total_cputime": 9,
				"total_time":    8,
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("new envelope: %v", err)
	}

	var buf bytes.Buffer
	if err := Write(&buf, "json", envelope); err != nil {
		t.Fatalf("write json envelope: %v", err)
	}

	assertSnapshot(t, "envelope-success.json", normalizeJSONSnapshot(t, buf.Bytes()))
}

func TestEnvelopeContractSnapshotJSONStructuredError(t *testing.T) {
	t.Parallel()

	envelope, err := NewEnvelope(
		"meta ad create",
		false,
		nil,
		nil,
		nil,
		&ErrorInfo{
			Type:         "GraphMethodException",
			Code:         100,
			ErrorSubcode: 33,
			StatusCode:   400,
			Message:      "Unsupported post request",
			FBTraceID:    "trace-1",
			Retryable:    false,
			Remediation: &Remediation{
				Category: "not_found",
				Summary:  "Referenced object or edge does not exist for this request.",
				Actions: []string{
					"Check object IDs and endpoint path for typos.",
					"Ensure the object belongs to the authenticated account context.",
				},
				Fields: []string{"campaign_id"},
			},
			Diagnostics: map[string]any{
				"error_user_title": "Unsupported post request",
				"error_data": map[string]any{
					"blame_field_specs": []any{
						[]any{"campaign_id"},
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("new envelope: %v", err)
	}

	var buf bytes.Buffer
	if err := Write(&buf, "json", envelope); err != nil {
		t.Fatalf("write json envelope: %v", err)
	}

	assertSnapshot(t, "envelope-error.json", normalizeJSONSnapshot(t, buf.Bytes()))
}

func TestEnvelopeContractSnapshotJSONLSlice(t *testing.T) {
	t.Parallel()

	envelope, err := NewEnvelope(
		"meta api get",
		true,
		[]map[string]any{
			{"id": "1", "name": "first"},
			{"id": "2", "name": "second"},
		},
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("new envelope: %v", err)
	}

	var buf bytes.Buffer
	if err := Write(&buf, "jsonl", envelope); err != nil {
		t.Fatalf("write jsonl envelope: %v", err)
	}

	assertSnapshot(t, "envelope-jsonl-slice.json", normalizeJSONLSnapshot(t, buf.Bytes()))
}

func assertSnapshot(t *testing.T, name string, got string) {
	t.Helper()

	path := filepath.Join("testdata", name)
	wantBytes, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("snapshot file %s is missing; expected contents:\n%s", path, got)
		}
		t.Fatalf("read snapshot file %s: %v", path, err)
	}

	want := string(wantBytes)
	if got != want {
		t.Fatalf("snapshot mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

func normalizeJSONSnapshot(t *testing.T, raw []byte) string {
	t.Helper()

	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode json snapshot payload: %v", err)
	}
	normalizeEnvelopeDynamicFields(decoded)
	normalized, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		t.Fatalf("encode normalized json snapshot payload: %v", err)
	}
	return string(normalized) + "\n"
}

func normalizeJSONLSnapshot(t *testing.T, raw []byte) string {
	t.Helper()

	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	normalizedLines := make([]any, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var decoded any
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("decode jsonl snapshot line: %v", err)
		}
		normalizeEnvelopeDynamicFields(decoded)
		normalizedLines = append(normalizedLines, decoded)
	}

	normalized, err := json.MarshalIndent(normalizedLines, "", "  ")
	if err != nil {
		t.Fatalf("encode normalized jsonl snapshot payload: %v", err)
	}
	return string(normalized) + "\n"
}

func normalizeEnvelopeDynamicFields(value any) {
	switch typed := value.(type) {
	case map[string]any:
		if _, ok := typed["timestamp"]; ok {
			typed["timestamp"] = "TIMESTAMP_PLACEHOLDER"
		}
		if _, ok := typed["request_id"]; ok {
			typed["request_id"] = "REQUEST_ID_PLACEHOLDER"
		}
		for _, item := range typed {
			normalizeEnvelopeDynamicFields(item)
		}
	case []any:
		for _, item := range typed {
			normalizeEnvelopeDynamicFields(item)
		}
	}
}
