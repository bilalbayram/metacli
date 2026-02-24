package cmd

import "github.com/spf13/cobra"

func NewThreadsCommand(runtime Runtime) *cobra.Command {
	return newNamespaceBootstrapCommandForNamespace(runtime, "threads")
}
