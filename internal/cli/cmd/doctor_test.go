package cmd

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/bilalbayram/metacli/internal/plugin"
)

func TestDoctorCommandIncludesTracerSubcommand(t *testing.T) {
	cmd := NewDoctorCommand(Runtime{})

	tracerCmd, _, err := cmd.Find([]string{"tracer"})
	if err != nil {
		t.Fatalf("find tracer command: %v", err)
	}
	if tracerCmd == nil || tracerCmd.Name() != "tracer" {
		t.Fatalf("expected tracer command, got %#v", tracerCmd)
	}
}

func TestDoctorTracerWritesSuccessEnvelope(t *testing.T) {
	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewDoctorCommand(Runtime{Output: stringPtr("json")})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{"tracer"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute doctor tracer: %v", err)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta doctor tracer")
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object payload, got %T", envelope["data"])
	}
	if got := data["namespace"]; got != doctorNamespace {
		t.Fatalf("unexpected namespace %v", got)
	}
	if got := data["plugin"]; got != doctorPluginID {
		t.Fatalf("unexpected plugin %v", got)
	}
	if got := data["status"]; got != "ok" {
		t.Fatalf("unexpected status %v", got)
	}

	trace, ok := data["trace"].(map[string]any)
	if !ok {
		t.Fatalf("expected trace object, got %T", data["trace"])
	}
	if recorded, _ := trace["recorded"].(bool); !recorded {
		t.Fatalf("expected recorded=true, got %v", trace["recorded"])
	}
	if got := trace["plugin"]; got != doctorPluginID {
		t.Fatalf("unexpected trace plugin %v", got)
	}
	if got := trace["namespace"]; got != doctorNamespace {
		t.Fatalf("unexpected trace namespace %v", got)
	}
	if got := trace["command"]; got != "tracer" {
		t.Fatalf("unexpected trace command %v", got)
	}

	discoveries, ok := data["discoveries"].([]any)
	if !ok {
		t.Fatalf("expected discoveries array, got %T", data["discoveries"])
	}
	if got := len(discoveries); got != 4 {
		t.Fatalf("unexpected discovery size %d", got)
	}
	if got := int(data["namespace_count"].(float64)); got != 4 {
		t.Fatalf("unexpected namespace_count %v", data["namespace_count"])
	}

	expected := map[string][]string{}
	for _, tc := range namespaceCommandCases() {
		expected[tc.namespace] = tc.discoveredCapabilities
	}
	expectedNamespaces := []string{"capi", "msgr", "threads", "wa"}
	for idx, raw := range discoveries {
		discovery, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("expected discovery object, got %T", raw)
		}
		namespace, _ := discovery["namespace"].(string)
		if namespace != expectedNamespaces[idx] {
			t.Fatalf("unexpected namespace order %v", namespace)
		}
		capabilities, ok := discovery["capabilities"].([]any)
		if !ok {
			t.Fatalf("expected capabilities array, got %T", discovery["capabilities"])
		}
		if got := len(capabilities); got != len(expected[namespace]) {
			t.Fatalf("unexpected capability count %d", got)
		}
		for capIdx, rawCapability := range capabilities {
			capability, ok := rawCapability.(map[string]any)
			if !ok {
				t.Fatalf("expected capability object, got %T", rawCapability)
			}
			if got := capability["name"]; got != expected[namespace][capIdx] {
				t.Fatalf("unexpected capability order %v", got)
			}
			if supported, _ := capability["supported"].(bool); !supported {
				t.Fatalf("expected supported=true, got %v", capability["supported"])
			}
		}
	}
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestDoctorTracerIsDeterministicAcrossRuns(t *testing.T) {
	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewDoctorCommand(Runtime{Output: stringPtr("json")})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)

	cmd.SetArgs([]string{"tracer"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute first tracer run: %v", err)
	}
	firstEnvelope := decodeEnvelope(t, output.Bytes())
	firstData, ok := firstEnvelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected first data object, got %T", firstEnvelope["data"])
	}

	output.Reset()
	errOutput.Reset()
	cmd.SetArgs([]string{"tracer"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute second tracer run: %v", err)
	}
	secondEnvelope := decodeEnvelope(t, output.Bytes())
	secondData, ok := secondEnvelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected second data object, got %T", secondEnvelope["data"])
	}

	if !reflect.DeepEqual(firstData, secondData) {
		t.Fatalf("expected deterministic doctor data, first=%v second=%v", firstData, secondData)
	}
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestDoctorTracerFailsClosedWhenTracerIsMissing(t *testing.T) {
	pluginRuntime, err := plugin.NewRuntime(plugin.NopTracer{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := newDoctorTracerCommand(Runtime{Output: stringPtr("json")}, pluginRuntime, nil)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{})

	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "doctor namespace tracer is required") {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", output.String())
	}

	envelope := decodeEnvelope(t, errOutput.Bytes())
	if got := envelope["command"]; got != "meta doctor tracer" {
		t.Fatalf("unexpected command field %v", got)
	}
	if envelope["success"] != false {
		t.Fatalf("expected success=false, got %v", envelope["success"])
	}
}

func TestDoctorCommandFailsWithoutSubcommand(t *testing.T) {
	cmd := NewDoctorCommand(Runtime{})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "doctor requires a subcommand") {
		t.Fatalf("unexpected error: %v", err)
	}
}
