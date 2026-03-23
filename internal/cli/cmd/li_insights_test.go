package cmd

import (
	"bytes"
	"io"
	"testing"
)

func TestLIInsightsMetricsListRepresentsBasicAsDefaultMode(t *testing.T) {
	cmd := newLIInsightsMetricsListCommand(testRuntime("prod"))
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute metrics list: %v", err)
	}

	envelope := decodeEnvelope(t, stdout.Bytes())
	assertEnvelopeBasics(t, envelope, "meta li insights metrics list")

	data, ok := envelope["data"].([]any)
	if !ok {
		t.Fatalf("expected data array, got %T", envelope["data"])
	}
	foundBasic := false
	for _, raw := range data {
		row, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("expected object row, got %T", raw)
		}
		pack, _ := row["metric_pack"].(string)
		if pack != "basic" {
			continue
		}
		foundBasic = true
		if got, _ := row["mode"].(string); got != "default" {
			t.Fatalf("unexpected basic mode %q", got)
		}
		if projected, _ := row["projected_fields"].(bool); projected {
			t.Fatalf("expected basic pack to omit projected fields")
		}
		defaultMetrics, ok := row["default_metrics"].([]any)
		if !ok || len(defaultMetrics) != 2 {
			t.Fatalf("unexpected default metrics %#v", row["default_metrics"])
		}
		if metric, _ := row["metric"].(string); metric != "" {
			t.Fatalf("expected empty explicit metric for basic, got %q", metric)
		}
	}
	if !foundBasic {
		t.Fatal("expected basic metric pack row")
	}
}
