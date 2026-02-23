package cmd

import "github.com/spf13/cobra"

func NewThreadsCommand(runtime Runtime) *cobra.Command {
	return newNamespaceBootstrapCommand(runtime, namespaceBootstrapSpec{
		PluginID:  "threads",
		Namespace: "threads",
		Short:     "Threads API commands",
		Capabilities: []namespaceCapability{
			{Name: "publish-post", Description: "Publish text or media posts via Threads API"},
			{Name: "read-insights", Description: "Fetch Threads media and account insights"},
		},
	})
}
