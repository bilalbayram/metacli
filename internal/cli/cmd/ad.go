package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
	"github.com/bilalbayram/metacli/internal/lint"
	"github.com/bilalbayram/metacli/internal/marketing"
	"github.com/bilalbayram/metacli/internal/ops"
	"github.com/bilalbayram/metacli/internal/schema"
	"github.com/spf13/cobra"
)

const (
	adMutationLintPath = "act_0/ads"
)

var (
	adLoadProfileCredentials = loadProfileCredentials
	adNewGraphClient         = func() *graph.Client {
		return graph.NewClient(nil, "")
	}
	adNewSchemaProvider = func(schemaDir string) schema.SchemaProvider {
		return schema.NewProvider(schemaDir, "", "")
	}
	adNewService = func(client *graph.Client) *marketing.AdService {
		return marketing.NewAdService(client)
	}
)

func NewAdCommand(runtime Runtime) *cobra.Command {
	adCmd := &cobra.Command{
		Use:   "ad",
		Short: "Ad lifecycle commands",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return requireSubcommand(cmd, "ad")
		},
	}
	adCmd.AddCommand(newAdListCommand(runtime))
	adCmd.AddCommand(newAdCreateCommand(runtime))
	adCmd.AddCommand(newAdUpdateCommand(runtime))
	adCmd.AddCommand(newAdPauseCommand(runtime))
	adCmd.AddCommand(newAdResumeCommand(runtime))
	adCmd.AddCommand(newAdCloneCommand(runtime))
	return adCmd
}

func newAdListCommand(runtime Runtime) *cobra.Command {
	var (
		profile            string
		version            string
		accountID          string
		campaignID         string
		adSetID            string
		fieldsRaw          string
		nameRaw            string
		statusRaw          string
		effectiveStatusRaw string
		activeOnly         bool
		limit              int
		pageSize           int
		followNext         bool
		schemaDir          string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List ads",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, resolvedVersion, err := resolveAdProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ad list", err)
			}

			linter, err := newAdMutationLinter(creds, resolvedVersion, schemaDir)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ad list", err)
			}

			fields := csvToSlice(fieldsRaw)
			if len(fields) == 0 {
				fields = append([]string(nil), marketing.DefaultAdReadFields...)
			}
			if err := lintAdListReadFields(linter, fields); err != nil {
				return writeCommandError(cmd, runtime, "meta ad list", err)
			}

			result, err := adNewService(adNewGraphClient()).List(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.AdListInput{
				AccountID:         accountID,
				CampaignID:        campaignID,
				AdSetID:           adSetID,
				Fields:            fields,
				Name:              nameRaw,
				Statuses:          csvToSlice(statusRaw),
				EffectiveStatuses: csvToSlice(effectiveStatusRaw),
				ActiveOnly:        activeOnly,
				Limit:             limit,
				PageSize:          pageSize,
				FollowNext:        followNext,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ad list", err)
			}

			return writeSuccess(cmd, runtime, "meta ad list", result.Ads, result.Paging, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&accountID, "account-id", "", "Ad account id (with or without act_ prefix)")
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "Campaign id filter")
	cmd.Flags().StringVar(&adSetID, "adset-id", "", "Ad set id filter")
	cmd.Flags().StringVar(&fieldsRaw, "fields", "", "Comma-separated Graph fields (defaults to ad read fields)")
	cmd.Flags().StringVar(&nameRaw, "name", "", "Case-insensitive ad name contains filter")
	cmd.Flags().StringVar(&statusRaw, "status", "", "Comma-separated ad status filter values")
	cmd.Flags().StringVar(&effectiveStatusRaw, "effective-status", "", "Comma-separated ad effective_status filter values")
	cmd.Flags().BoolVar(&activeOnly, "active-only", false, "Show only ACTIVE ads by status/effective_status")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of ads to return after filtering")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "Graph page size for ad reads")
	cmd.Flags().BoolVar(&followNext, "follow-next", false, "Follow paging.next links")
	cmd.Flags().StringVar(&schemaDir, "schema-dir", schema.DefaultSchemaDir, "Schema pack root directory")
	return cmd
}

func newAdCreateCommand(runtime Runtime) *cobra.Command {
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
		Short: "Create an ad",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, resolvedVersion, err := resolveAdProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ad create", err)
			}

			form, err := parseKeyValueList(paramsRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ad create", err)
			}
			jsonForm, err := parseInlineJSONPayload(jsonRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ad create", err)
			}
			if err := mergeParams(form, jsonForm, "--json"); err != nil {
				return writeCommandError(cmd, runtime, "meta ad create", err)
			}

			linter, err := newAdMutationLinter(creds, resolvedVersion, schemaDir)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ad create", err)
			}
			if err := lintAdMutation(linter, form); err != nil {
				return writeCommandError(cmd, runtime, "meta ad create", err)
			}

			result, err := adNewService(adNewGraphClient()).Create(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.AdCreateInput{
				AccountID: accountID,
				Params:    form,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ad create", err)
			}
			if err := persistTrackedResource(trackedResourceInput{
				Command:       "meta ad create",
				ResourceKind:  ops.ResourceKindAd,
				ResourceID:    result.AdID,
				CleanupAction: ops.CleanupActionPause,
				Profile:       creds.Name,
				GraphVersion:  resolvedVersion,
				AccountID:     accountID,
				Metadata: map[string]string{
					"operation": result.Operation,
				},
			}); err != nil {
				return writeCommandError(cmd, runtime, "meta ad create", err)
			}

			return writeSuccess(cmd, runtime, "meta ad create", result, nil, nil)
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

func newAdUpdateCommand(runtime Runtime) *cobra.Command {
	var (
		profile   string
		version   string
		adID      string
		paramsRaw string
		jsonRaw   string
		schemaDir string
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update an ad",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, resolvedVersion, err := resolveAdProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ad update", err)
			}

			form, err := parseKeyValueList(paramsRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ad update", err)
			}
			jsonForm, err := parseInlineJSONPayload(jsonRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ad update", err)
			}
			if err := mergeParams(form, jsonForm, "--json"); err != nil {
				return writeCommandError(cmd, runtime, "meta ad update", err)
			}

			linter, err := newAdMutationLinter(creds, resolvedVersion, schemaDir)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ad update", err)
			}
			if err := lintAdMutation(linter, form); err != nil {
				return writeCommandError(cmd, runtime, "meta ad update", err)
			}

			result, err := adNewService(adNewGraphClient()).Update(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.AdUpdateInput{
				AdID:   adID,
				Params: form,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ad update", err)
			}

			return writeSuccess(cmd, runtime, "meta ad update", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&adID, "ad-id", "", "Ad id")
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Comma-separated mutation params (k=v,k2=v2)")
	cmd.Flags().StringVar(&jsonRaw, "json", "", "Inline JSON object payload")
	cmd.Flags().StringVar(&schemaDir, "schema-dir", schema.DefaultSchemaDir, "Schema pack root directory")
	return cmd
}

func newAdPauseCommand(runtime Runtime) *cobra.Command {
	return newAdStatusCommand(runtime, "pause", marketing.AdStatusPaused)
}

func newAdResumeCommand(runtime Runtime) *cobra.Command {
	return newAdStatusCommand(runtime, "resume", marketing.AdStatusActive)
}

func newAdStatusCommand(runtime Runtime, operation string, status string) *cobra.Command {
	var (
		profile   string
		version   string
		adID      string
		schemaDir string
	)

	commandName := fmt.Sprintf("meta ad %s", operation)
	cmd := &cobra.Command{
		Use:   operation,
		Short: fmt.Sprintf("%s an ad", operation),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, resolvedVersion, err := resolveAdProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, commandName, err)
			}

			linter, err := newAdMutationLinter(creds, resolvedVersion, schemaDir)
			if err != nil {
				return writeCommandError(cmd, runtime, commandName, err)
			}
			if err := lintAdMutation(linter, map[string]string{"status": status}); err != nil {
				return writeCommandError(cmd, runtime, commandName, err)
			}

			result, err := adNewService(adNewGraphClient()).SetStatus(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.AdStatusInput{
				AdID:   adID,
				Status: status,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, commandName, err)
			}

			return writeSuccess(cmd, runtime, commandName, result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&adID, "ad-id", "", "Ad id")
	cmd.Flags().StringVar(&schemaDir, "schema-dir", schema.DefaultSchemaDir, "Schema pack root directory")
	return cmd
}

func newAdCloneCommand(runtime Runtime) *cobra.Command {
	var (
		profile    string
		version    string
		sourceAdID string
		accountID  string
		fieldsRaw  string
		paramsRaw  string
		jsonRaw    string
		schemaDir  string
	)

	cmd := &cobra.Command{
		Use:   "clone",
		Short: "Clone an ad into a target ad account",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, resolvedVersion, err := resolveAdProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ad clone", err)
			}

			overrides, err := parseKeyValueList(paramsRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ad clone", err)
			}
			jsonOverrides, err := parseInlineJSONPayload(jsonRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ad clone", err)
			}
			if err := mergeParams(overrides, jsonOverrides, "--json"); err != nil {
				return writeCommandError(cmd, runtime, "meta ad clone", err)
			}

			linter, err := newAdMutationLinter(creds, resolvedVersion, schemaDir)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ad clone", err)
			}

			cloneFields := csvToSlice(fieldsRaw)
			if len(cloneFields) == 0 {
				cloneFields = append([]string(nil), marketing.DefaultAdCloneFields...)
			}
			if err := lintAdReadFields(linter, cloneFields); err != nil {
				return writeCommandError(cmd, runtime, "meta ad clone", err)
			}
			if err := lintAdMutation(linter, overrides); err != nil {
				return writeCommandError(cmd, runtime, "meta ad clone", err)
			}

			result, err := adNewService(adNewGraphClient()).Clone(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.AdCloneInput{
				SourceAdID:      sourceAdID,
				TargetAccountID: accountID,
				Overrides:       overrides,
				Fields:          cloneFields,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ad clone", err)
			}
			if err := persistTrackedResource(trackedResourceInput{
				Command:       "meta ad clone",
				ResourceKind:  ops.ResourceKindAd,
				ResourceID:    result.AdID,
				CleanupAction: ops.CleanupActionPause,
				Profile:       creds.Name,
				GraphVersion:  resolvedVersion,
				AccountID:     accountID,
				SourceID:      sourceAdID,
				Metadata: map[string]string{
					"operation": result.Operation,
				},
			}); err != nil {
				return writeCommandError(cmd, runtime, "meta ad clone", err)
			}

			return writeSuccess(cmd, runtime, "meta ad clone", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&sourceAdID, "source-ad-id", "", "Source ad id")
	cmd.Flags().StringVar(&accountID, "account-id", "", "Target ad account id (with or without act_ prefix)")
	cmd.Flags().StringVar(&fieldsRaw, "fields", strings.Join(marketing.DefaultAdCloneFields, ","), "Comma-separated fields to read from source ad")
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Comma-separated override params (k=v,k2=v2)")
	cmd.Flags().StringVar(&jsonRaw, "json", "", "Inline JSON object overrides")
	cmd.Flags().StringVar(&schemaDir, "schema-dir", schema.DefaultSchemaDir, "Schema pack root directory")
	return cmd
}

func resolveAdProfileAndVersion(runtime Runtime, profile string, version string) (*ProfileCredentials, string, error) {
	resolvedProfile := strings.TrimSpace(profile)
	if resolvedProfile == "" {
		resolvedProfile = runtime.ProfileName()
	}
	if resolvedProfile == "" {
		return nil, "", errors.New("profile is required (--profile or global --profile)")
	}

	creds, err := adLoadProfileCredentials(resolvedProfile)
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

func newAdMutationLinter(creds *ProfileCredentials, version string, schemaDir string) (*lint.Linter, error) {
	if creds == nil {
		return nil, errors.New("ad profile credentials are required")
	}
	provider := adNewSchemaProvider(schemaDir)
	pack, err := provider.GetPack(creds.Profile.Domain, version)
	if err != nil {
		return nil, err
	}
	return lint.New(pack)
}

func lintAdMutation(linter *lint.Linter, params map[string]string) error {
	result := linter.Lint(&lint.RequestSpec{
		Method: "POST",
		Path:   adMutationLintPath,
		Params: params,
	}, true)
	if len(result.Errors) > 0 {
		return fmt.Errorf("ad mutation lint failed with %d error(s): %s", len(result.Errors), strings.Join(result.Errors, "; "))
	}
	return nil
}

func lintAdReadFields(linter *lint.Linter, fields []string) error {
	result := linter.Lint(&lint.RequestSpec{
		Method: "GET",
		Path:   adMutationLintPath,
		Fields: fields,
	}, true)
	if len(result.Errors) > 0 {
		return fmt.Errorf("ad clone field lint failed with %d error(s): %s", len(result.Errors), strings.Join(result.Errors, "; "))
	}
	return nil
}

func lintAdListReadFields(linter *lint.Linter, fields []string) error {
	result := linter.Lint(&lint.RequestSpec{
		Method: "GET",
		Path:   adMutationLintPath,
		Fields: fields,
	}, true)
	if len(result.Errors) > 0 {
		return fmt.Errorf("ad list field lint failed with %d error(s): %s", len(result.Errors), strings.Join(result.Errors, "; "))
	}
	return nil
}
