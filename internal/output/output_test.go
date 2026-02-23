package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONEnvelopeIncludesContractVersion(t *testing.T) {
	t.Parallel()

	envelope, err := NewEnvelope("meta auth list", true, map[string]any{"status": "ok"}, nil, nil, nil)
	if err != nil {
		t.Fatalf("new envelope: %v", err)
	}

	var buf bytes.Buffer
	if err := Write(&buf, "json", envelope); err != nil {
		t.Fatalf("write json: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if decoded["contract_version"] != ContractVersion {
		t.Fatalf("unexpected contract version: %v", decoded["contract_version"])
	}
	if decoded["command"] != "meta auth list" {
		t.Fatalf("unexpected command: %v", decoded["command"])
	}
}

func TestJSONLEnvelopeLineCountForSlice(t *testing.T) {
	t.Parallel()

	data := []map[string]any{
		{"id": "1"},
		{"id": "2"},
	}
	envelope, err := NewEnvelope("meta api get", true, data, nil, nil, nil)
	if err != nil {
		t.Fatalf("new envelope: %v", err)
	}

	var buf bytes.Buffer
	if err := Write(&buf, "jsonl", envelope); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 jsonl lines, got %d", len(lines))
	}

	for _, line := range lines {
		var decoded map[string]any
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("decode line: %v", err)
		}
		if decoded["contract_version"] != ContractVersion {
			t.Fatalf("unexpected contract version in line: %v", decoded["contract_version"])
		}
	}
}
