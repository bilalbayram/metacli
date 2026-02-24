package cmd

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

type namespaceCommandCase struct {
	namespace              string
	pluginID               string
	supportedCapability    string
	discoveredCapabilities []string
	newCommand             func(Runtime) *cobra.Command
}

func TestNamespaceBootstrapCommandsIncludeHealthAndCapability(t *testing.T) {
	for _, tc := range namespaceCommandCases() {
		tc := tc
		t.Run(tc.namespace, func(t *testing.T) {
			cmd := tc.newCommand(Runtime{})

			healthCmd, _, err := cmd.Find([]string{"health"})
			if err != nil {
				t.Fatalf("find health command: %v", err)
			}
			if healthCmd == nil || healthCmd.Name() != "health" {
				t.Fatalf("expected health command, got %#v", healthCmd)
			}

			capabilityCmd, _, err := cmd.Find([]string{"capability"})
			if err != nil {
				t.Fatalf("find capability command: %v", err)
			}
			if capabilityCmd == nil || capabilityCmd.Name() != "capability" {
				t.Fatalf("expected capability command, got %#v", capabilityCmd)
			}
		})
	}
}

func TestNamespaceBootstrapHealthWritesSuccessEnvelope(t *testing.T) {
	for _, tc := range namespaceCommandCases() {
		tc := tc
		t.Run(tc.namespace, func(t *testing.T) {
			output := &bytes.Buffer{}
			errOutput := &bytes.Buffer{}
			cmd := tc.newCommand(Runtime{Output: stringPtr("json")})
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetOut(output)
			cmd.SetErr(errOutput)
			cmd.SetArgs([]string{"health"})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute health: %v", err)
			}

			envelope := decodeEnvelope(t, output.Bytes())
			assertEnvelopeBasics(t, envelope, "meta "+tc.namespace+" health")
			data, ok := envelope["data"].(map[string]any)
			if !ok {
				t.Fatalf("expected object payload, got %T", envelope["data"])
			}
			if got := data["namespace"]; got != tc.namespace {
				t.Fatalf("unexpected namespace %v", got)
			}
			if got := data["plugin"]; got != tc.pluginID {
				t.Fatalf("unexpected plugin %v", got)
			}
			if got := data["status"]; got != "ok" {
				t.Fatalf("unexpected status %v", got)
			}
			if errOutput.Len() != 0 {
				t.Fatalf("expected empty stderr, got %q", errOutput.String())
			}
		})
	}
}

func TestNamespaceBootstrapCapabilityWritesSuccessEnvelope(t *testing.T) {
	for _, tc := range namespaceCommandCases() {
		tc := tc
		t.Run(tc.namespace, func(t *testing.T) {
			output := &bytes.Buffer{}
			errOutput := &bytes.Buffer{}
			cmd := tc.newCommand(Runtime{Output: stringPtr("json")})
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetOut(output)
			cmd.SetErr(errOutput)
			cmd.SetArgs([]string{"capability", "--name", tc.supportedCapability})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute capability: %v", err)
			}

			envelope := decodeEnvelope(t, output.Bytes())
			assertEnvelopeBasics(t, envelope, "meta "+tc.namespace+" capability")
			data, ok := envelope["data"].(map[string]any)
			if !ok {
				t.Fatalf("expected object payload, got %T", envelope["data"])
			}
			if got := data["capability"]; got != tc.supportedCapability {
				t.Fatalf("unexpected capability %v", got)
			}
			if supported, _ := data["supported"].(bool); !supported {
				t.Fatalf("expected supported=true, got %v", data["supported"])
			}
			if errOutput.Len() != 0 {
				t.Fatalf("expected empty stderr, got %q", errOutput.String())
			}
		})
	}
}

func TestNamespaceBootstrapCapabilityDiscoveryWritesSuccessEnvelope(t *testing.T) {
	for _, tc := range namespaceCommandCases() {
		tc := tc
		t.Run(tc.namespace, func(t *testing.T) {
			output := &bytes.Buffer{}
			errOutput := &bytes.Buffer{}
			cmd := tc.newCommand(Runtime{Output: stringPtr("json")})
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetOut(output)
			cmd.SetErr(errOutput)
			cmd.SetArgs([]string{"capability", "--discover"})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute capability discovery: %v", err)
			}

			envelope := decodeEnvelope(t, output.Bytes())
			assertEnvelopeBasics(t, envelope, "meta "+tc.namespace+" capability")
			data, ok := envelope["data"].(map[string]any)
			if !ok {
				t.Fatalf("expected object payload, got %T", envelope["data"])
			}
			if got := data["namespace"]; got != tc.namespace {
				t.Fatalf("unexpected namespace %v", got)
			}
			if got := data["plugin"]; got != tc.pluginID {
				t.Fatalf("unexpected plugin %v", got)
			}

			capabilities, ok := data["capabilities"].([]any)
			if !ok {
				t.Fatalf("expected capabilities array, got %T", data["capabilities"])
			}
			if got := len(capabilities); got != len(tc.discoveredCapabilities) {
				t.Fatalf("unexpected capability count %d", got)
			}
			if got := int(data["capability_count"].(float64)); got != len(tc.discoveredCapabilities) {
				t.Fatalf("unexpected capability_count %v", data["capability_count"])
			}

			for idx, raw := range capabilities {
				capability, ok := raw.(map[string]any)
				if !ok {
					t.Fatalf("expected capability object, got %T", raw)
				}
				if got := capability["name"]; got != tc.discoveredCapabilities[idx] {
					t.Fatalf("unexpected capability order %v", got)
				}
				if supported, _ := capability["supported"].(bool); !supported {
					t.Fatalf("expected supported=true, got %v", capability["supported"])
				}
			}
			if errOutput.Len() != 0 {
				t.Fatalf("expected empty stderr, got %q", errOutput.String())
			}
		})
	}
}

func TestNamespaceBootstrapCapabilityDiscoveryIsDeterministic(t *testing.T) {
	for _, tc := range namespaceCommandCases() {
		tc := tc
		t.Run(tc.namespace, func(t *testing.T) {
			output := &bytes.Buffer{}
			errOutput := &bytes.Buffer{}
			cmd := tc.newCommand(Runtime{Output: stringPtr("json")})
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetOut(output)
			cmd.SetErr(errOutput)

			cmd.SetArgs([]string{"capability", "--discover"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute first capability discovery: %v", err)
			}
			firstEnvelope := decodeEnvelope(t, output.Bytes())
			firstData, ok := firstEnvelope["data"].(map[string]any)
			if !ok {
				t.Fatalf("expected first data object, got %T", firstEnvelope["data"])
			}

			output.Reset()
			errOutput.Reset()
			cmd.SetArgs([]string{"capability", "--discover"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute second capability discovery: %v", err)
			}
			secondEnvelope := decodeEnvelope(t, output.Bytes())
			secondData, ok := secondEnvelope["data"].(map[string]any)
			if !ok {
				t.Fatalf("expected second data object, got %T", secondEnvelope["data"])
			}

			if !reflect.DeepEqual(firstData, secondData) {
				t.Fatalf("expected deterministic discovery data, first=%v second=%v", firstData, secondData)
			}
			if errOutput.Len() != 0 {
				t.Fatalf("expected empty stderr, got %q", errOutput.String())
			}
		})
	}
}

func TestNamespaceBootstrapCapabilityFailsForMissingOrUnsupportedCapability(t *testing.T) {
	for _, tc := range namespaceCommandCases() {
		tc := tc
		t.Run(tc.namespace, func(t *testing.T) {
			testCases := []struct {
				name      string
				args      []string
				errorText string
			}{
				{
					name:      "missing name",
					args:      []string{"capability"},
					errorText: "capability name is required",
				},
				{
					name:      "unsupported",
					args:      []string{"capability", "--name", "unsupported-cap"},
					errorText: "unsupported capability",
				},
				{
					name:      "discover with name",
					args:      []string{"capability", "--discover", "--name", tc.supportedCapability},
					errorText: "capability discovery does not accept --name",
				},
			}

			for _, failure := range testCases {
				failure := failure
				t.Run(failure.name, func(t *testing.T) {
					output := &bytes.Buffer{}
					errOutput := &bytes.Buffer{}
					cmd := tc.newCommand(Runtime{Output: stringPtr("json")})
					cmd.SilenceErrors = true
					cmd.SilenceUsage = true
					cmd.SetOut(output)
					cmd.SetErr(errOutput)
					cmd.SetArgs(failure.args)

					err := cmd.Execute()
					if err == nil {
						t.Fatal("expected command error")
					}
					if !strings.Contains(err.Error(), failure.errorText) {
						t.Fatalf("unexpected error: %v", err)
					}
					if output.Len() != 0 {
						t.Fatalf("expected empty stdout, got %q", output.String())
					}

					envelope := decodeEnvelope(t, errOutput.Bytes())
					if got := envelope["command"]; got != "meta "+tc.namespace+" capability" {
						t.Fatalf("unexpected command field %v", got)
					}
					if envelope["success"] != false {
						t.Fatalf("expected success=false, got %v", envelope["success"])
					}
				})
			}
		})
	}
}

func TestNamespaceBootstrapCommandFailsWithoutSubcommand(t *testing.T) {
	for _, tc := range namespaceCommandCases() {
		tc := tc
		t.Run(tc.namespace, func(t *testing.T) {
			cmd := tc.newCommand(Runtime{})
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetArgs([]string{})

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected command error")
			}
			if !strings.Contains(err.Error(), tc.namespace+" requires a subcommand") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestNamespaceBootstrapFailsFastOnMalformedSpec(t *testing.T) {
	cmd := newNamespaceBootstrapCommand(Runtime{}, namespaceBootstrapSpec{
		PluginID:     "badplugin",
		Namespace:    "badns",
		Short:        "Broken namespace",
		Capabilities: nil,
	})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"health"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected bootstrap error")
	}
	if !strings.Contains(err.Error(), "at least one capability is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func namespaceCommandCases() []namespaceCommandCase {
	return []namespaceCommandCase{
		{
			namespace:              "wa",
			pluginID:               "whatsapp",
			supportedCapability:    "send-message",
			discoveredCapabilities: []string{"media-upload", "send-message"},
			newCommand:             NewWACommand,
		},
		{
			namespace:              "msgr",
			pluginID:               "messenger",
			supportedCapability:    "send-api",
			discoveredCapabilities: []string{"send-api", "webhook"},
			newCommand:             NewMSGRCommand,
		},
		{
			namespace:              "threads",
			pluginID:               "threads",
			supportedCapability:    "publish-post",
			discoveredCapabilities: []string{"publish-post", "read-insights"},
			newCommand:             NewThreadsCommand,
		},
		{
			namespace:              "capi",
			pluginID:               "capi",
			supportedCapability:    "send-event",
			discoveredCapabilities: []string{"send-event", "test-event"},
			newCommand:             NewCAPICommand,
		},
	}
}

func stringPtr(value string) *string {
	return &value
}
