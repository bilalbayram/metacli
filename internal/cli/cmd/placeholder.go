package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewPlaceholderGroup(name string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: fmt.Sprintf("%s command group", name),
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("%s command group is not implemented yet", name)
		},
	}
}
