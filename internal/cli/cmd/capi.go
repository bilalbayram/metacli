package cmd

import "github.com/spf13/cobra"

func NewCAPICommand(runtime Runtime) *cobra.Command {
	return newNamespaceBootstrapCommand(runtime, namespaceBootstrapSpec{
		PluginID:  "capi",
		Namespace: "capi",
		Short:     "Conversions API commands",
		Capabilities: []namespaceCapability{
			{Name: "send-event", Description: "Send conversion events to /events endpoint"},
			{Name: "test-event", Description: "Validate conversion payloads using test_event_code"},
		},
	})
}
