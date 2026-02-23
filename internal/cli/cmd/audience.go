package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
	"github.com/bilalbayram/metacli/internal/lint"
	"github.com/bilalbayram/metacli/internal/marketing"
	"github.com/bilalbayram/metacli/internal/schema"
	"github.com/spf13/cobra"
)

const (
	audienceMutationLintPath = "act_0/customaudiences"
)

var (
	audienceLoadProfileCredentials = loadProfileCredentials
	audienceNewGraphClient         = func() *graph.Client {
		return graph.NewClient(nil, "")
	}
	audienceNewSchemaProvider = func(schemaDir string) schema.SchemaProvider {
		return schema.NewProvider(schemaDir, "", "")
	}
	audienceNewService = func(client *graph.Client) *marketing.AudienceService {
		return marketing.NewAudienceService(client)
	}
)

func NewAudienceCommand(runtime Runtime) *cobra.Command {
	audienceCmd := &cobra.Command{
		Use:   "audience",
		Short: "Audience lifecycle commands",
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("audience requires a subcommand")
		},
	}
	audienceCmd.AddCommand(newAudienceCreateCommand(runtime))
	audienceCmd.AddCommand(newAudienceUpdateCommand(runtime))
	audienceCmd.AddCommand(newAudienceDeleteCommand(runtime))
	return audienceCmd
}

func newAudienceCreateCommand(runtime Runtime) *cobra.Command {
	var (
		profile   string
		version   string
		accountID string
		paramsRaw string
		jsonRaw   string
		schemaDir string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a custom audience",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, resolvedVersion, err := resolveAudienceProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta audience create", err)
			}

			form, err := parseKeyValueList(paramsRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta audience create", err)
			}
			jsonForm, err := parseInlineJSONPayload(jsonRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta audience create", err)
			}
			if err := mergeParams(form, jsonForm, "--json"); err != nil {
				return writeCommandError(cmd, runtime, "meta audience create", err)
			}

			linter, err := newAudienceMutationLinter(creds, resolvedVersion, schemaDir)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta audience create", err)
			}
			if err := lintAudienceMutation(linter, form); err != nil {
				return writeCommandError(cmd, runtime, "meta audience create", err)
			}

			result, err := audienceNewService(audienceNewGraphClient()).Create(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.AudienceCreateInput{
				AccountID: accountID,
				Params:    form,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, "meta audience create", err)
			}

			return writeSuccess(cmd, runtime, "meta audience create", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&accountID, "account-id", "", "Ad account id (with or without act_ prefix)")
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Comma-separated mutation params (k=v,k2=v2)")
	cmd.Flags().StringVar(&jsonRaw, "json", "", "Inline JSON object payload")
	cmd.Flags().StringVar(&schemaDir, "schema-dir", schema.DefaultSchemaDir, "Schema pack root directory")
	return cmd
}

func newAudienceUpdateCommand(runtime Runtime) *cobra.Command {
	var (
		profile    string
		version    string
		audienceID string
		paramsRaw  string
		jsonRaw    string
		schemaDir  string
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a custom audience",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, resolvedVersion, err := resolveAudienceProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta audience update", err)
			}

			form, err := parseKeyValueList(paramsRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta audience update", err)
			}
			jsonForm, err := parseInlineJSONPayload(jsonRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta audience update", err)
			}
			if err := mergeParams(form, jsonForm, "--json"); err != nil {
				return writeCommandError(cmd, runtime, "meta audience update", err)
			}

			linter, err := newAudienceMutationLinter(creds, resolvedVersion, schemaDir)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta audience update", err)
			}
			if err := lintAudienceMutation(linter, form); err != nil {
				return writeCommandError(cmd, runtime, "meta audience update", err)
			}

			result, err := audienceNewService(audienceNewGraphClient()).Update(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.AudienceUpdateInput{
				AudienceID: audienceID,
				Params:     form,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, "meta audience update", err)
			}

			return writeSuccess(cmd, runtime, "meta audience update", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&audienceID, "audience-id", "", "Audience id")
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Comma-separated mutation params (k=v,k2=v2)")
	cmd.Flags().StringVar(&jsonRaw, "json", "", "Inline JSON object payload")
	cmd.Flags().StringVar(&schemaDir, "schema-dir", schema.DefaultSchemaDir, "Schema pack root directory")
	return cmd
}

func newAudienceDeleteCommand(runtime Runtime) *cobra.Command {
	var (
		profile    string
		version    string
		audienceID string
	)

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a custom audience",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, resolvedVersion, err := resolveAudienceProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta audience delete", err)
			}

			result, err := audienceNewService(audienceNewGraphClient()).Delete(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.AudienceDeleteInput{
				AudienceID: audienceID,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, "meta audience delete", err)
			}

			return writeSuccess(cmd, runtime, "meta audience delete", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&audienceID, "audience-id", "", "Audience id")
	return cmd
}

func resolveAudienceProfileAndVersion(runtime Runtime, profile string, version string) (*ProfileCredentials, string, error) {
	resolvedProfile := strings.TrimSpace(profile)
	if resolvedProfile == "" {
		resolvedProfile = runtime.ProfileName()
	}
	if resolvedProfile == "" {
		return nil, "", errors.New("profile is required (--profile or global --profile)")
	}

	creds, err := audienceLoadProfileCredentials(resolvedProfile)
	if err != nil {
		return nil, "", err
	}

	resolvedVersion := strings.TrimSpace(version)
	if resolvedVersion == "" {
		resolvedVersion = creds.Profile.GraphVersion
	}
	if resolvedVersion == "" {
		resolvedVersion = config.DefaultGraphVersion
	}
	return creds, resolvedVersion, nil
}

func newAudienceMutationLinter(creds *ProfileCredentials, version string, schemaDir string) (*lint.Linter, error) {
	if creds == nil {
		return nil, errors.New("audience profile credentials are required")
	}
	provider := audienceNewSchemaProvider(schemaDir)
	pack, err := provider.GetPack(creds.Profile.Domain, version)
	if err != nil {
		return nil, err
	}
	return lint.New(pack)
}

func lintAudienceMutation(linter *lint.Linter, params map[string]string) error {
	result := linter.Lint(&lint.RequestSpec{
		Method: "POST",
		Path:   audienceMutationLintPath,
		Params: params,
	}, true)
	if len(result.Errors) > 0 {
		return fmt.Errorf("audience mutation lint failed with %d error(s): %s", len(result.Errors), strings.Join(result.Errors, "; "))
	}
	return nil
}
