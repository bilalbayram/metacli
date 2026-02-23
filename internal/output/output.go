package output

import (
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

const ContractVersion = "1.0"

type Envelope struct {
	ContractVersion string     `json:"contract_version"`
	Command         string     `json:"command"`
	Timestamp       string     `json:"timestamp"`
	RequestID       string     `json:"request_id"`
	Success         bool       `json:"success"`
	Data            any        `json:"data,omitempty"`
	Paging          any        `json:"paging,omitempty"`
	RateLimit       any        `json:"rate_limit,omitempty"`
	Error           *ErrorInfo `json:"error,omitempty"`
}

type ErrorInfo struct {
	Type         string `json:"type"`
	Code         int    `json:"code"`
	ErrorSubcode int    `json:"error_subcode"`
	Message      string `json:"message"`
	FBTraceID    string `json:"fbtrace_id,omitempty"`
	Retryable    bool   `json:"retryable"`
}

func NewEnvelope(command string, success bool, data any, paging any, rateLimit any, errorInfo *ErrorInfo) (Envelope, error) {
	requestID, err := newRequestID()
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		ContractVersion: ContractVersion,
		Command:         command,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		RequestID:       requestID,
		Success:         success,
		Data:            data,
		Paging:          paging,
		RateLimit:       rateLimit,
		Error:           errorInfo,
	}, nil
}

func Write(w io.Writer, format string, envelope Envelope) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return writeJSON(w, envelope)
	case "jsonl":
		return writeJSONL(w, envelope)
	case "table":
		return writeTable(w, envelope.Data)
	case "csv":
		return writeCSV(w, envelope.Data)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func writeJSON(w io.Writer, envelope Envelope) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(envelope)
}

func writeJSONL(w io.Writer, envelope Envelope) error {
	switch data := envelope.Data.(type) {
	case []map[string]any:
		for _, item := range data {
			line := envelope
			line.Data = item
			encoded, err := json.Marshal(line)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintln(w, string(encoded)); err != nil {
				return err
			}
		}
		return nil
	case []any:
		for _, item := range data {
			line := envelope
			line.Data = item
			encoded, err := json.Marshal(line)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintln(w, string(encoded)); err != nil {
				return err
			}
		}
		return nil
	default:
		encoded, err := json.Marshal(envelope)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(encoded))
		return err
	}
}

func writeTable(w io.Writer, data any) error {
	rows, headers, err := normalizeRows(data)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, strings.Join(headers, "\t")); err != nil {
		return err
	}
	for _, row := range rows {
		values := make([]string, 0, len(headers))
		for _, header := range headers {
			values = append(values, fmt.Sprint(row[header]))
		}
		if _, err := fmt.Fprintln(tw, strings.Join(values, "\t")); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func writeCSV(w io.Writer, data any) error {
	rows, headers, err := normalizeRows(data)
	if err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(headers); err != nil {
		return err
	}
	for _, row := range rows {
		record := make([]string, 0, len(headers))
		for _, header := range headers {
			record = append(record, fmt.Sprint(row[header]))
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func normalizeRows(data any) ([]map[string]any, []string, error) {
	switch typed := data.(type) {
	case []map[string]any:
		headers := orderedHeaders(typed)
		return typed, headers, nil
	case map[string]any:
		headers := orderedHeaders([]map[string]any{typed})
		return []map[string]any{typed}, headers, nil
	default:
		return nil, nil, errors.New("table/csv output requires map or []map data")
	}
}

func orderedHeaders(rows []map[string]any) []string {
	set := map[string]struct{}{}
	for _, row := range rows {
		for key := range row {
			set[key] = struct{}{}
		}
	}
	headers := make([]string, 0, len(set))
	for key := range set {
		headers = append(headers, key)
	}
	sort.Strings(headers)
	return headers
}

func newRequestID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate request id: %w", err)
	}
	return hex.EncodeToString(raw), nil
}
