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
	adsetMutationLintPath = "act_0/adsets"
)

var (
	adsetLoadProfileCredentials = loadProfileCredentials
	adsetNewGraphClient         = func() *graph.Client {
		return graph.NewClient(nil, "")
	}
	adsetNewSchemaProvider = func(schemaDir string) schema.SchemaProvider {
		return schema.NewProvider(schemaDir, "", "")
	}
	adsetNewService = func(client *graph.Client) *marketing.AdSetService {
		return marketing.NewAdSetService(client)
	}
	adsetBudgetGuardrailKeys = map[string]struct{}{
		"daily_budget":    {},
		"lifetime_budget": {},
	}
)

func NewAdsetCommand(runtime Runtime) *cobra.Command {
	adsetCmd := &cobra.Command{
		Use:   "adset",
		Short: "Ad set lifecycle commands",
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("adset requires a subcommand")
		},
	}
	adsetCmd.AddCommand(newAdsetCreateCommand(runtime))
	adsetCmd.AddCommand(newAdsetUpdateCommand(runtime))
	adsetCmd.AddCommand(newAdsetPauseCommand(runtime))
	adsetCmd.AddCommand(newAdsetResumeCommand(runtime))
	return adsetCmd
}

func NewAdSetCommand(runtime Runtime) *cobra.Command {
	return NewAdsetCommand(runtime)
}

func newAdsetCreateCommand(runtime Runtime) *cobra.Command {
	var (
		profile             string
		version             string
		accountID           string
		paramsRaw           string
		jsonRaw             string
		schemaDir           string
		confirmBudgetChange bool
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an ad set",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, resolvedVersion, err := resolveAdsetProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta adset create", err)
			}

			form, err := parseKeyValueList(paramsRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta adset create", err)
			}
			jsonForm, err := parseInlineJSONPayload(jsonRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta adset create", err)
			}
			if err := mergeParams(form, jsonForm, "--json"); err != nil {
				return writeCommandError(cmd, runtime, "meta adset create", err)
			}
			if err := enforceAdsetBudgetGuardrail(form, confirmBudgetChange); err != nil {
				return writeCommandError(cmd, runtime, "meta adset create", err)
			}

			linter, err := newAdsetMutationLinter(creds, resolvedVersion, schemaDir)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta adset create", err)
			}
			if err := lintAdsetMutation(linter, form); err != nil {
				return writeCommandError(cmd, runtime, "meta adset create", err)
			}

			result, err := adsetNewService(adsetNewGraphClient()).Create(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.AdSetCreateInput{
				AccountID: accountID,
				Params:    form,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, "meta adset create", err)
			}

			return writeSuccess(cmd, runtime, "meta adset create", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&accountID, "account-id", "", "Ad account id (with or without act_ prefix)")
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Comma-separated mutation params (k=v,k2=v2)")
	cmd.Flags().StringVar(&jsonRaw, "json", "", "Inline JSON object payload")
	cmd.Flags().StringVar(&schemaDir, "schema-dir", schema.DefaultSchemaDir, "Schema pack root directory")
	cmd.Flags().BoolVar(&confirmBudgetChange, "confirm-budget-change", false, "Acknowledge budget mutation fields (daily_budget/lifetime_budget)")
	return cmd
}

func newAdsetUpdateCommand(runtime Runtime) *cobra.Command {
	var (
		profile             string
		version             string
		adSetID             string
		paramsRaw           string
		jsonRaw             string
		schemaDir           string
		confirmBudgetChange bool
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update an ad set",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, resolvedVersion, err := resolveAdsetProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta adset update", err)
			}

			form, err := parseKeyValueList(paramsRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta adset update", err)
			}
			jsonForm, err := parseInlineJSONPayload(jsonRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta adset update", err)
			}
			if err := mergeParams(form, jsonForm, "--json"); err != nil {
				return writeCommandError(cmd, runtime, "meta adset update", err)
			}
			if err := enforceAdsetBudgetGuardrail(form, confirmBudgetChange); err != nil {
				return writeCommandError(cmd, runtime, "meta adset update", err)
			}

			linter, err := newAdsetMutationLinter(creds, resolvedVersion, schemaDir)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta adset update", err)
			}
			if err := lintAdsetMutation(linter, form); err != nil {
				return writeCommandError(cmd, runtime, "meta adset update", err)
			}

			result, err := adsetNewService(adsetNewGraphClient()).Update(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.AdSetUpdateInput{
				AdSetID: adSetID,
				Params:  form,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, "meta adset update", err)
			}

			return writeSuccess(cmd, runtime, "meta adset update", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&adSetID, "adset-id", "", "Ad set id")
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Comma-separated mutation params (k=v,k2=v2)")
	cmd.Flags().StringVar(&jsonRaw, "json", "", "Inline JSON object payload")
	cmd.Flags().StringVar(&schemaDir, "schema-dir", schema.DefaultSchemaDir, "Schema pack root directory")
	cmd.Flags().BoolVar(&confirmBudgetChange, "confirm-budget-change", false, "Acknowledge budget mutation fields (daily_budget/lifetime_budget)")
	return cmd
}

func newAdsetPauseCommand(runtime Runtime) *cobra.Command {
	return newAdsetStatusCommand(runtime, "pause", marketing.AdSetStatusPaused)
}

func newAdsetResumeCommand(runtime Runtime) *cobra.Command {
	return newAdsetStatusCommand(runtime, "resume", marketing.AdSetStatusActive)
}

func newAdsetStatusCommand(runtime Runtime, operation string, status string) *cobra.Command {
	var (
		profile   string
		version   string
		adSetID   string
		schemaDir string
	)

	commandName := fmt.Sprintf("meta adset %s", operation)
	cmd := &cobra.Command{
		Use:   operation,
		Short: fmt.Sprintf("%s an ad set", operation),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, resolvedVersion, err := resolveAdsetProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, commandName, err)
			}

			linter, err := newAdsetMutationLinter(creds, resolvedVersion, schemaDir)
			if err != nil {
				return writeCommandError(cmd, runtime, commandName, err)
			}
			if err := lintAdsetMutation(linter, map[string]string{"status": status}); err != nil {
				return writeCommandError(cmd, runtime, commandName, err)
			}

			result, err := adsetNewService(adsetNewGraphClient()).SetStatus(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.AdSetStatusInput{
				AdSetID: adSetID,
				Status:  status,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, commandName, err)
			}

			return writeSuccess(cmd, runtime, commandName, result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&adSetID, "adset-id", "", "Ad set id")
	cmd.Flags().StringVar(&schemaDir, "schema-dir", schema.DefaultSchemaDir, "Schema pack root directory")
	return cmd
}

func resolveAdsetProfileAndVersion(runtime Runtime, profile string, version string) (*ProfileCredentials, string, error) {
	resolvedProfile := strings.TrimSpace(profile)
	if resolvedProfile == "" {
		resolvedProfile = runtime.ProfileName()
	}
	if resolvedProfile == "" {
		return nil, "", errors.New("profile is required (--profile or global --profile)")
	}

	creds, err := adsetLoadProfileCredentials(resolvedProfile)
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

func newAdsetMutationLinter(creds *ProfileCredentials, version string, schemaDir string) (*lint.Linter, error) {
	if creds == nil {
		return nil, errors.New("ad set profile credentials are required")
	}
	provider := adsetNewSchemaProvider(schemaDir)
	pack, err := provider.GetPack(creds.Profile.Domain, version)
	if err != nil {
		return nil, err
	}
	return lint.New(pack)
}

func lintAdsetMutation(linter *lint.Linter, params map[string]string) error {
	result := linter.Lint(&lint.RequestSpec{
		Method: "POST",
		Path:   adsetMutationLintPath,
		Params: params,
	}, true)
	if len(result.Errors) > 0 {
		return fmt.Errorf("ad set mutation lint failed with %d error(s): %s", len(result.Errors), strings.Join(result.Errors, "; "))
	}
	return nil
}

func enforceAdsetBudgetGuardrail(params map[string]string, confirmed bool) error {
	if !adsetMutationChangesBudget(params) || confirmed {
		return nil
	}
	return errors.New("budget change detected in ad set mutation payload; rerun with --confirm-budget-change")
}

func adsetMutationChangesBudget(params map[string]string) bool {
	for key := range params {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if _, exists := adsetBudgetGuardrailKeys[normalized]; exists {
			return true
		}
	}
	return false
}
