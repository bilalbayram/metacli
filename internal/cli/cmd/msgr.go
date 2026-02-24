package cmd

import "github.com/spf13/cobra"

func NewMSGRCommand(runtime Runtime) *cobra.Command {
	return newNamespaceBootstrapCommandForNamespace(runtime, "msgr")
}
