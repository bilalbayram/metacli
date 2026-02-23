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
	campaignMutationLintPath = "act_0/campaigns"
)

var (
	campaignLoadProfileCredentials = loadProfileCredentials
	campaignNewGraphClient         = func() *graph.Client {
		return graph.NewClient(nil, "")
	}
	campaignNewSchemaProvider = func(schemaDir string) schema.SchemaProvider {
		return schema.NewProvider(schemaDir, "", "")
	}
	campaignNewService = func(client *graph.Client) *marketing.Service {
		return marketing.NewCampaignService(client)
	}
)

func NewCampaignCommand(runtime Runtime) *cobra.Command {
	campaignCmd := &cobra.Command{
		Use:   "campaign",
		Short: "Campaign lifecycle commands",
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("campaign requires a subcommand")
		},
	}
	campaignCmd.AddCommand(newCampaignCreateCommand(runtime))
	campaignCmd.AddCommand(newCampaignUpdateCommand(runtime))
	campaignCmd.AddCommand(newCampaignPauseCommand(runtime))
	campaignCmd.AddCommand(newCampaignResumeCommand(runtime))
	campaignCmd.AddCommand(newCampaignCloneCommand(runtime))
	return campaignCmd
}

func newCampaignCreateCommand(runtime Runtime) *cobra.Command {
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
		Short: "Create a campaign",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, resolvedVersion, err := resolveCampaignProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign create", err)
			}

			form, err := parseKeyValueList(paramsRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign create", err)
			}
			jsonForm, err := parseInlineJSONPayload(jsonRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign create", err)
			}
			if err := mergeParams(form, jsonForm, "--json"); err != nil {
				return writeCommandError(cmd, runtime, "meta campaign create", err)
			}

			linter, err := newCampaignMutationLinter(creds, resolvedVersion, schemaDir)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign create", err)
			}
			if err := lintCampaignMutation(linter, form); err != nil {
				return writeCommandError(cmd, runtime, "meta campaign create", err)
			}

			result, err := campaignNewService(campaignNewGraphClient()).Create(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.CampaignCreateInput{
				AccountID: accountID,
				Params:    form,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign create", err)
			}

			return writeSuccess(cmd, runtime, "meta campaign create", result, nil, nil)
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

func newCampaignUpdateCommand(runtime Runtime) *cobra.Command {
	var (
		profile    string
		version    string
		campaignID string
		paramsRaw  string
		jsonRaw    string
		schemaDir  string
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a campaign",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, resolvedVersion, err := resolveCampaignProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign update", err)
			}

			form, err := parseKeyValueList(paramsRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign update", err)
			}
			jsonForm, err := parseInlineJSONPayload(jsonRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign update", err)
			}
			if err := mergeParams(form, jsonForm, "--json"); err != nil {
				return writeCommandError(cmd, runtime, "meta campaign update", err)
			}

			linter, err := newCampaignMutationLinter(creds, resolvedVersion, schemaDir)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign update", err)
			}
			if err := lintCampaignMutation(linter, form); err != nil {
				return writeCommandError(cmd, runtime, "meta campaign update", err)
			}

			result, err := campaignNewService(campaignNewGraphClient()).Update(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.CampaignUpdateInput{
				CampaignID: campaignID,
				Params:     form,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign update", err)
			}

			return writeSuccess(cmd, runtime, "meta campaign update", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "Campaign id")
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Comma-separated mutation params (k=v,k2=v2)")
	cmd.Flags().StringVar(&jsonRaw, "json", "", "Inline JSON object payload")
	cmd.Flags().StringVar(&schemaDir, "schema-dir", schema.DefaultSchemaDir, "Schema pack root directory")
	return cmd
}

func newCampaignPauseCommand(runtime Runtime) *cobra.Command {
	return newCampaignStatusCommand(runtime, "pause", marketing.CampaignStatusPaused)
}

func newCampaignResumeCommand(runtime Runtime) *cobra.Command {
	return newCampaignStatusCommand(runtime, "resume", marketing.CampaignStatusActive)
}

func newCampaignStatusCommand(runtime Runtime, operation string, status string) *cobra.Command {
	var (
		profile    string
		version    string
		campaignID string
		schemaDir  string
	)

	commandName := fmt.Sprintf("meta campaign %s", operation)
	cmd := &cobra.Command{
		Use:   operation,
		Short: fmt.Sprintf("%s a campaign", operation),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, resolvedVersion, err := resolveCampaignProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, commandName, err)
			}

			linter, err := newCampaignMutationLinter(creds, resolvedVersion, schemaDir)
			if err != nil {
				return writeCommandError(cmd, runtime, commandName, err)
			}
			if err := lintCampaignMutation(linter, map[string]string{"status": status}); err != nil {
				return writeCommandError(cmd, runtime, commandName, err)
			}

			result, err := campaignNewService(campaignNewGraphClient()).SetStatus(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.CampaignStatusInput{
				CampaignID: campaignID,
				Status:     status,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, commandName, err)
			}

			return writeSuccess(cmd, runtime, commandName, result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "Campaign id")
	cmd.Flags().StringVar(&schemaDir, "schema-dir", schema.DefaultSchemaDir, "Schema pack root directory")
	return cmd
}

func newCampaignCloneCommand(runtime Runtime) *cobra.Command {
	var (
		profile          string
		version          string
		sourceCampaignID string
		accountID        string
		fieldsRaw        string
		paramsRaw        string
		jsonRaw          string
		schemaDir        string
	)

	cmd := &cobra.Command{
		Use:   "clone",
		Short: "Clone a campaign into a target ad account",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, resolvedVersion, err := resolveCampaignProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign clone", err)
			}

			overrides, err := parseKeyValueList(paramsRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign clone", err)
			}
			jsonOverrides, err := parseInlineJSONPayload(jsonRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign clone", err)
			}
			if err := mergeParams(overrides, jsonOverrides, "--json"); err != nil {
				return writeCommandError(cmd, runtime, "meta campaign clone", err)
			}

			linter, err := newCampaignMutationLinter(creds, resolvedVersion, schemaDir)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign clone", err)
			}

			cloneFields := csvToSlice(fieldsRaw)
			if len(cloneFields) == 0 {
				cloneFields = append([]string(nil), marketing.DefaultCampaignCloneFields...)
			}
			if err := lintCampaignReadFields(linter, cloneFields); err != nil {
				return writeCommandError(cmd, runtime, "meta campaign clone", err)
			}
			if err := lintCampaignMutation(linter, overrides); err != nil {
				return writeCommandError(cmd, runtime, "meta campaign clone", err)
			}

			result, err := campaignNewService(campaignNewGraphClient()).Clone(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.CampaignCloneInput{
				SourceCampaignID: sourceCampaignID,
				TargetAccountID:  accountID,
				Overrides:        overrides,
				Fields:           cloneFields,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign clone", err)
			}

			return writeSuccess(cmd, runtime, "meta campaign clone", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&sourceCampaignID, "source-campaign-id", "", "Source campaign id")
	cmd.Flags().StringVar(&accountID, "account-id", "", "Target ad account id (with or without act_ prefix)")
	cmd.Flags().StringVar(&fieldsRaw, "fields", strings.Join(marketing.DefaultCampaignCloneFields, ","), "Comma-separated fields to read from source campaign")
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Comma-separated override params (k=v,k2=v2)")
	cmd.Flags().StringVar(&jsonRaw, "json", "", "Inline JSON object overrides")
	cmd.Flags().StringVar(&schemaDir, "schema-dir", schema.DefaultSchemaDir, "Schema pack root directory")
	return cmd
}

func resolveCampaignProfileAndVersion(runtime Runtime, profile string, version string) (*ProfileCredentials, string, error) {
	resolvedProfile := strings.TrimSpace(profile)
	if resolvedProfile == "" {
		resolvedProfile = runtime.ProfileName()
	}
	if resolvedProfile == "" {
		return nil, "", errors.New("profile is required (--profile or global --profile)")
	}

	creds, err := campaignLoadProfileCredentials(resolvedProfile)
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

func newCampaignMutationLinter(creds *ProfileCredentials, version string, schemaDir string) (*lint.Linter, error) {
	if creds == nil {
		return nil, errors.New("campaign profile credentials are required")
	}
	provider := campaignNewSchemaProvider(schemaDir)
	pack, err := provider.GetPack(creds.Profile.Domain, version)
	if err != nil {
		return nil, err
	}
	return lint.New(pack)
}

func lintCampaignMutation(linter *lint.Linter, params map[string]string) error {
	result := linter.Lint(&lint.RequestSpec{
		Method: "POST",
		Path:   campaignMutationLintPath,
		Params: params,
	}, true)
	if len(result.Errors) > 0 {
		return fmt.Errorf("campaign mutation lint failed with %d error(s): %s", len(result.Errors), strings.Join(result.Errors, "; "))
	}
	return nil
}

func lintCampaignReadFields(linter *lint.Linter, fields []string) error {
	result := linter.Lint(&lint.RequestSpec{
		Method: "GET",
		Path:   campaignMutationLintPath,
		Fields: fields,
	}, true)
	if len(result.Errors) > 0 {
		return fmt.Errorf("campaign clone field lint failed with %d error(s): %s", len(result.Errors), strings.Join(result.Errors, "; "))
	}
	return nil
}
