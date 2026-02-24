package cmd

import "github.com/spf13/cobra"

func NewCAPICommand(runtime Runtime) *cobra.Command {
	return newNamespaceBootstrapCommandForNamespace(runtime, "capi")
}
