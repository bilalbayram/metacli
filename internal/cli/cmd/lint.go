package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/lint"
	"github.com/bilalbayram/metacli/internal/schema"
	"github.com/spf13/cobra"
)

func NewLintCommand(runtime Runtime) *cobra.Command {
	lintCmd := &cobra.Command{
		Use:   "lint",
		Short: "Lint API request specs against schema packs",
	}
	lintCmd.AddCommand(newLintRequestCommand(runtime))
	return lintCmd
}

func newLintRequestCommand(runtime Runtime) *cobra.Command {
	var (
		filePath string
		profile  string
		version  string
		strict   bool
		schemaDir string
	)
	cmd := &cobra.Command{
		Use:   "request",
		Short: "Lint a request spec file",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(filePath) == "" {
				return errors.New("request spec file is required")
			}
			if profile == "" {
				profile = runtime.ProfileName()
			}
			if profile == "" {
				return errors.New("profile is required (--profile or global --profile)")
			}

			creds, err := loadProfileCredentials(profile)
			if err != nil {
				return err
			}
			if version == "" {
				version = creds.Profile.GraphVersion
			}
			if version == "" {
				version = config.DefaultGraphVersion
			}

			spec, err := lint.LoadRequestSpec(filePath)
			if err != nil {
				return err
			}

			provider := schema.NewProvider(schemaDir, "", "")
			pack, err := provider.GetPack(creds.Profile.Domain, version)
			if err != nil {
				return err
			}
			linter, err := lint.New(pack)
			if err != nil {
				return err
			}
			result := linter.Lint(spec, strict)
			if len(result.Errors) > 0 {
				return fmt.Errorf("lint failed with %d error(s): %s", len(result.Errors), strings.Join(result.Errors, "; "))
			}
			return writeSuccess(cmd, runtime, "meta lint request", map[string]any{
				"errors":   result.Errors,
				"warnings": result.Warnings,
			}, nil, nil)
		},
	}
	cmd.Flags().StringVar(&filePath, "file", "", "Path to JSON request spec file")
	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().BoolVar(&strict, "strict", false, "Treat unknown fields/params as errors")
	cmd.Flags().StringVar(&schemaDir, "schema-dir", schema.DefaultSchemaDir, "Schema pack root directory")
	return cmd
}
