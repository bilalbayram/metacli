package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

const appName = "meta"

type GlobalFlags struct {
	Profile string
	Output  string
	Debug   bool
}

func Execute() error {
	root := NewRootCommand()
	return root.Execute()
}

func NewRootCommand() *cobra.Command {
	flags := &GlobalFlags{}

	cmd := &cobra.Command{
		Use:               appName,
		Short:             "Meta Marketing CLI",
		Long:              "Meta Marketing CLI provides authenticated access to Meta Graph and Marketing APIs.",
		SilenceErrors:     true,
		SilenceUsage:      true,
		PersistentPreRunE: validateGlobalFlags(flags),
	}

	cmd.PersistentFlags().StringVar(&flags.Profile, "profile", "", "Auth profile name")
	cmd.PersistentFlags().StringVar(&flags.Output, "output", "json", "Output format: json|jsonl|table|csv")
	cmd.PersistentFlags().BoolVar(&flags.Debug, "debug", false, "Enable debug logging")

	cmd.AddCommand(newNotImplementedCommand("auth"))
	cmd.AddCommand(newNotImplementedCommand("api"))
	cmd.AddCommand(newNotImplementedCommand("insights"))
	cmd.AddCommand(newNotImplementedCommand("lint"))
	cmd.AddCommand(newNotImplementedCommand("schema"))
	cmd.AddCommand(newNotImplementedCommand("changelog"))

	return cmd
}

func validateGlobalFlags(flags *GlobalFlags) func(*cobra.Command, []string) error {
	return func(_ *cobra.Command, _ []string) error {
		switch flags.Output {
		case "json", "jsonl", "table", "csv":
			return nil
		default:
			return WrapExit(ExitCodeInput, fmt.Errorf("invalid --output value %q; expected json|jsonl|table|csv", flags.Output))
		}
	}
}

func newNotImplementedCommand(name string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: fmt.Sprintf("%s command group", name),
		RunE: func(_ *cobra.Command, _ []string) error {
			return WrapExit(ExitCodeUnknown, fmt.Errorf("%s command group is not implemented yet", name))
		},
	}
}
