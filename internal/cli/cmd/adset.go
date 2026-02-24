package cmd

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
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
	adsetBidStrategyRequirements = map[string]adsetBidStrategyRequirement{
		"LOWEST_COST_WITHOUT_CAP": {
			ForbidBidAmount: true,
		},
		"LOWEST_COST_WITH_BID_CAP": {
			RequireBidAmount: true,
		},
		"COST_CAP": {
			RequireBidAmount: true,
		},
		"TARGET_COST": {
			RequireBidAmount: true,
		},
	}
	adsetBudgetFloorMinorUnitsByCurrency = map[string]int64{
		"AUD": 100,
		"CAD": 100,
		"EUR": 100,
		"GBP": 100,
		"JPY": 100,
		"NZD": 100,
		"SGD": 100,
		"TRY": 100,
		"USD": 100,
	}
)

type adsetBidStrategyRequirement struct {
	RequireBidAmount bool
	ForbidBidAmount  bool
}

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
			if err := resolveAdsetIntentRequirements(form); err != nil {
				return writeCommandError(cmd, runtime, "meta adset create", err)
			}

			linter, err := newAdsetMutationLinter(creds, resolvedVersion, schemaDir)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta adset create", err)
			}
			if err := lintAdsetMutation(linter, form); err != nil {
				return writeCommandError(cmd, runtime, "meta adset create", err)
			}

			service := adsetNewService(adsetNewGraphClient())
			if err := enforceAdsetBudgetFloorChecks(cmd.Context(), service, resolvedVersion, creds.Token, creds.AppSecret, accountID, "", form); err != nil {
				return writeCommandError(cmd, runtime, "meta adset create", err)
			}

			result, err := service.Create(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.AdSetCreateInput{
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
			if err := resolveAdsetIntentRequirements(form); err != nil {
				return writeCommandError(cmd, runtime, "meta adset update", err)
			}

			linter, err := newAdsetMutationLinter(creds, resolvedVersion, schemaDir)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta adset update", err)
			}
			if err := lintAdsetMutation(linter, form); err != nil {
				return writeCommandError(cmd, runtime, "meta adset update", err)
			}

			service := adsetNewService(adsetNewGraphClient())
			if err := enforceAdsetBudgetFloorChecks(cmd.Context(), service, resolvedVersion, creds.Token, creds.AppSecret, "", adSetID, form); err != nil {
				return writeCommandError(cmd, runtime, "meta adset update", err)
			}

			result, err := service.Update(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.AdSetUpdateInput{
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

func resolveAdsetIntentRequirements(params map[string]string) error {
	bidStrategy, hasBidStrategy := adsetMutationParamValue(params, "bid_strategy")
	bidAmount, hasBidAmount := adsetMutationParamValue(params, "bid_amount")
	if hasBidAmount {
		if _, err := parseAdsetMinorUnitField("bid_amount", bidAmount, "ad set intent requirements"); err != nil {
			return err
		}
	}

	if !hasBidStrategy {
		if hasBidAmount {
			return errors.New(
				"ad set intent requirements blocked mutation: field \"bid_strategy\" is required when field \"bid_amount\" is provided; remediation: set --params bid_strategy=<LOWEST_COST_WITH_BID_CAP|COST_CAP|TARGET_COST>",
			)
		}
		return nil
	}

	normalizedStrategy := strings.ToUpper(strings.TrimSpace(bidStrategy))
	if normalizedStrategy == "" {
		return errors.New(
			"ad set intent requirements blocked mutation: field \"bid_strategy\" cannot be empty; remediation: provide a supported bid strategy value",
		)
	}
	requirement, exists := adsetBidStrategyRequirements[normalizedStrategy]
	if !exists {
		return fmt.Errorf(
			"ad set intent requirements blocked mutation: field \"bid_strategy\" value %q is unsupported; remediation: use one of [%s]",
			normalizedStrategy,
			strings.Join(sortedAdsetSupportedBidStrategies(), ", "),
		)
	}

	if requirement.RequireBidAmount && !hasBidAmount {
		return fmt.Errorf(
			"ad set intent requirements blocked mutation: field \"bid_amount\" is required when \"bid_strategy\"=%q; remediation: set --params bid_amount=<integer_minor_units>",
			normalizedStrategy,
		)
	}
	if requirement.ForbidBidAmount && hasBidAmount {
		return fmt.Errorf(
			"ad set intent requirements blocked mutation: field \"bid_amount\" is not allowed when \"bid_strategy\"=%q; remediation: remove \"bid_amount\" or select a capped bid strategy",
			normalizedStrategy,
		)
	}

	return nil
}

func enforceAdsetBudgetFloorChecks(
	ctx context.Context,
	service *marketing.AdSetService,
	version string,
	token string,
	appSecret string,
	accountID string,
	adSetID string,
	params map[string]string,
) error {
	budgetFields := adsetBudgetFieldValues(params)
	if len(budgetFields) == 0 {
		return nil
	}
	if service == nil {
		return errors.New("ad set budget floor check blocked mutation: ad set service is required; remediation: retry with a valid ad set service client")
	}

	resolvedAccountID, err := resolveAdsetBudgetAccountID(ctx, service, version, token, appSecret, accountID, adSetID)
	if err != nil {
		return err
	}

	currency, err := service.ResolveAccountCurrency(ctx, version, token, appSecret, resolvedAccountID)
	if err != nil {
		return fmt.Errorf(
			"ad set budget floor check blocked mutation: failed to resolve account currency for account %q: %w; remediation: verify account access and retry",
			resolvedAccountID,
			err,
		)
	}

	floor, exists := adsetBudgetFloorMinorUnitsByCurrency[currency]
	if !exists {
		return fmt.Errorf(
			"ad set budget floor check blocked mutation: unsupported account currency %q; remediation: use one of [%s] or extend currency floor rules",
			currency,
			strings.Join(sortedAdsetSupportedFloorCurrencies(), ", "),
		)
	}

	for _, field := range []string{"daily_budget", "lifetime_budget"} {
		rawValue, hasField := budgetFields[field]
		if !hasField {
			continue
		}
		amount, parseErr := parseAdsetMinorUnitField(field, rawValue, "ad set budget floor check")
		if parseErr != nil {
			return parseErr
		}
		if amount < floor {
			return fmt.Errorf(
				"ad set budget floor check blocked mutation: field %q value %d is below minimum %d for currency %s (account %s); remediation: set %q >= %d minor units before retrying",
				field,
				amount,
				floor,
				currency,
				resolvedAccountID,
				field,
				floor,
			)
		}
	}

	return nil
}

func resolveAdsetBudgetAccountID(
	ctx context.Context,
	service *marketing.AdSetService,
	version string,
	token string,
	appSecret string,
	accountID string,
	adSetID string,
) (string, error) {
	if strings.TrimSpace(accountID) != "" {
		normalized, err := normalizeAdsetAccountID(accountID)
		if err != nil {
			return "", fmt.Errorf(
				"ad set budget floor check blocked mutation: invalid account context: %w; remediation: provide --account-id in act_<ID> or <ID> format",
				err,
			)
		}
		return normalized, nil
	}

	resolved, err := service.ResolveAccountID(ctx, version, token, appSecret, adSetID)
	if err != nil {
		return "", fmt.Errorf(
			"ad set budget floor check blocked mutation: failed to resolve ad set account context from adset_id %q: %w; remediation: verify --adset-id and profile access",
			strings.TrimSpace(adSetID),
			err,
		)
	}
	return resolved, nil
}

func adsetBudgetFieldValues(params map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range params {
		normalizedKey := strings.ToLower(strings.TrimSpace(key))
		if normalizedKey != "daily_budget" && normalizedKey != "lifetime_budget" {
			continue
		}
		out[normalizedKey] = strings.TrimSpace(value)
	}
	return out
}

func adsetMutationParamValue(params map[string]string, field string) (string, bool) {
	normalizedField := strings.ToLower(strings.TrimSpace(field))
	for key, value := range params {
		if strings.ToLower(strings.TrimSpace(key)) != normalizedField {
			continue
		}
		return strings.TrimSpace(value), true
	}
	return "", false
}

func parseAdsetMinorUnitField(field string, raw string, workflow string) (int64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, fmt.Errorf(
			"%s blocked mutation: field %q cannot be empty; remediation: provide an integer minor-unit amount",
			workflow,
			field,
		)
	}
	parsed, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf(
			"%s blocked mutation: field %q value %q must be an integer minor-unit amount; remediation: provide whole-number minor units (for example 100)",
			workflow,
			field,
			raw,
		)
	}
	if parsed < 0 {
		return 0, fmt.Errorf(
			"%s blocked mutation: field %q value %d must be non-negative; remediation: provide a value >= 0",
			workflow,
			field,
			parsed,
		)
	}
	return parsed, nil
}

func normalizeAdsetAccountID(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(normalized), "act_") {
		normalized = normalized[4:]
	}
	if normalized == "" {
		return "", errors.New("account id is required")
	}
	if strings.Contains(normalized, "/") {
		return "", fmt.Errorf("invalid account id %q: expected single graph id token", value)
	}
	return normalized, nil
}

func sortedAdsetSupportedBidStrategies() []string {
	values := make([]string, 0, len(adsetBidStrategyRequirements))
	for strategy := range adsetBidStrategyRequirements {
		values = append(values, strategy)
	}
	sort.Strings(values)
	return values
}

func sortedAdsetSupportedFloorCurrencies() []string {
	values := make([]string, 0, len(adsetBudgetFloorMinorUnitsByCurrency))
	for currency := range adsetBudgetFloorMinorUnitsByCurrency {
		values = append(values, currency)
	}
	sort.Strings(values)
	return values
}
