package cmd

import "github.com/spf13/cobra"

func NewMSGRCommand(runtime Runtime) *cobra.Command {
	return newNamespaceBootstrapCommand(runtime, namespaceBootstrapSpec{
		PluginID:  "messenger",
		Namespace: "msgr",
		Short:     "Messenger Platform commands",
		Capabilities: []namespaceCapability{
			{Name: "send-api", Description: "Send messages through the Messenger Send API"},
			{Name: "webhook", Description: "Consume Messenger webhook events"},
		},
	})
}
