package cmd

import (
	"errors"
	"time"

	"github.com/bilalbayram/metacli/internal/changelog"
	"github.com/spf13/cobra"
)

func NewChangelogCommand(runtime Runtime) *cobra.Command {
	changelogCmd := &cobra.Command{
		Use:   "changelog",
		Short: "Version and changelog utilities",
	}
	changelogCmd.AddCommand(newChangelogCheckCommand(runtime))
	return changelogCmd
}

func newChangelogCheckCommand(runtime Runtime) *cobra.Command {
	var version string
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check pinned version against known latest/deprecation windows",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if version == "" {
				return errors.New("version is required")
			}
			checker := changelog.NewChecker()
			result, err := checker.Check(version, time.Now().UTC())
			if err != nil {
				return err
			}
			return writeSuccess(cmd, runtime, "meta changelog check", result, nil, nil)
		},
	}
	cmd.Flags().StringVar(&version, "version", "", "Graph API version to check (for example v25.0)")
	return cmd
}
