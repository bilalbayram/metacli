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
	"github.com/bilalbayram/metacli/internal/requirements"
	"github.com/bilalbayram/metacli/internal/schema"
	"github.com/spf13/cobra"
)

const (
	campaignMutationLintPath      = "act_0/campaigns"
	campaignRequirementsMutation  = "campaigns.post"
	campaignPayloadSourceInput    = "input"
	campaignPayloadSourceRule     = "requirements.inject_defaults"
	campaignPayloadSourceClone    = "clone.source_payload"
	campaignPayloadSourceOverride = "clone.override"
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
	campaignLoadRulePack        = requirements.LoadRulePack
	campaignBudgetGuardrailKeys = map[string]struct{}{
		"daily_budget":    {},
		"lifetime_budget": {},
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
	campaignCmd.AddCommand(newCampaignResolveRequirementsCommand(runtime))
	campaignCmd.AddCommand(newCampaignUpdateCommand(runtime))
	campaignCmd.AddCommand(newCampaignPauseCommand(runtime))
	campaignCmd.AddCommand(newCampaignResumeCommand(runtime))
	campaignCmd.AddCommand(newCampaignCloneCommand(runtime))
	return campaignCmd
}

func newCampaignCreateCommand(runtime Runtime) *cobra.Command {
	var (
		profile             string
		version             string
		accountID           string
		paramsRaw           string
		jsonRaw             string
		schemaDir           string
		rulesDir            string
		confirmBudgetChange bool
		dryRun              bool
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
			if err := enforceCampaignBudgetGuardrail(form, confirmBudgetChange); err != nil {
				return writeCommandError(cmd, runtime, "meta campaign create", err)
			}

			linter, err := newCampaignMutationLinter(creds, resolvedVersion, schemaDir)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign create", err)
			}
			if err := lintCampaignMutation(linter, form); err != nil {
				return writeCommandError(cmd, runtime, "meta campaign create", err)
			}

			resolution, err := resolveCampaignMutationRequirements(
				creds,
				resolvedVersion,
				schemaDir,
				rulesDir,
				campaignRequirementsMutation,
				accountID,
				form,
			)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign create", err)
			}
			if resolution.HasBlockingViolations() {
				return writeCommandError(
					cmd,
					runtime,
					"meta campaign create",
					fmt.Errorf("campaign requirements resolution blocked mutation: %s", resolution.ViolationSummary()),
				)
			}
			if err := lintCampaignMutation(linter, resolution.Payload.Final); err != nil {
				return writeCommandError(cmd, runtime, "meta campaign create", err)
			}

			plan := campaignMutationPlanResult{
				Operation:         "create",
				Mutation:          campaignRequirementsMutation,
				AccountID:         strings.TrimSpace(accountID),
				Resolution:        resolution,
				FinalPayload:      copyCampaignPayload(resolution.Payload.Final),
				PayloadProvenance: campaignPayloadProvenance(resolution.Payload.Final, resolution.Payload.Injected, campaignFieldSources(resolution.Payload.Input, campaignPayloadSourceInput), campaignPayloadSourceInput),
			}
			if dryRun {
				return writeSuccess(cmd, runtime, "meta campaign create", campaignDryRunResult{
					Status: "ok",
					DryRun: true,
					Plan:   plan,
				}, nil, nil)
			}

			result, err := campaignNewService(campaignNewGraphClient()).Create(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.CampaignCreateInput{
				AccountID: accountID,
				Params:    plan.FinalPayload,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign create", err)
			}
			if err := persistTrackedResource(trackedResourceInput{
				Command:       "meta campaign create",
				ResourceKind:  ops.ResourceKindCampaign,
				ResourceID:    result.CampaignID,
				CleanupAction: ops.CleanupActionPause,
				Profile:       creds.Name,
				GraphVersion:  resolvedVersion,
				AccountID:     accountID,
				Metadata: map[string]string{
					"operation": result.Operation,
				},
			}); err != nil {
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
	cmd.Flags().StringVar(&rulesDir, "rules-dir", "", "Runtime rule pack root directory override")
	cmd.Flags().BoolVar(&confirmBudgetChange, "confirm-budget-change", false, "Acknowledge budget mutation fields (daily_budget/lifetime_budget)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Resolve requirements and output plan without executing mutation")
	cmd.Flags().BoolVar(&dryRun, "plan", false, "Alias of --dry-run")
	return cmd
}

type campaignRequirementsResult struct {
	Status            string                  `json:"status"`
	Resolution        requirements.Resolution `json:"resolution"`
	FinalPayload      map[string]string       `json:"final_payload"`
	PayloadProvenance map[string]string       `json:"payload_provenance"`
}

type campaignMutationPlanResult struct {
	Operation         string                       `json:"operation"`
	Mutation          string                       `json:"mutation"`
	AccountID         string                       `json:"account_id,omitempty"`
	SourceCampaignID  string                       `json:"source_campaign_id,omitempty"`
	Resolution        requirements.Resolution      `json:"resolution"`
	FinalPayload      map[string]string            `json:"final_payload"`
	PayloadProvenance map[string]string            `json:"payload_provenance"`
	ClonePlan         *marketing.CampaignClonePlan `json:"clone_plan,omitempty"`
}

type campaignDryRunResult struct {
	Status string                     `json:"status"`
	DryRun bool                       `json:"dry_run"`
	Plan   campaignMutationPlanResult `json:"plan"`
}

func newCampaignResolveRequirementsCommand(runtime Runtime) *cobra.Command {
	var (
		profile   string
		version   string
		accountID string
		paramsRaw string
		jsonRaw   string
		schemaDir string
		rulesDir  string
	)

	cmd := &cobra.Command{
		Use:   "resolve-requirements",
		Short: "Resolve campaign create requirements and payload plan without executing mutation",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, resolvedVersion, err := resolveCampaignProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign resolve-requirements", err)
			}

			form, err := parseKeyValueList(paramsRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign resolve-requirements", err)
			}
			jsonForm, err := parseInlineJSONPayload(jsonRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign resolve-requirements", err)
			}
			if err := mergeParams(form, jsonForm, "--json"); err != nil {
				return writeCommandError(cmd, runtime, "meta campaign resolve-requirements", err)
			}

			resolution, err := resolveCampaignMutationRequirements(
				creds,
				resolvedVersion,
				schemaDir,
				rulesDir,
				campaignRequirementsMutation,
				accountID,
				form,
			)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign resolve-requirements", err)
			}

			result := campaignRequirementsResult{
				Status:            "ok",
				Resolution:        resolution,
				FinalPayload:      copyCampaignPayload(resolution.Payload.Final),
				PayloadProvenance: campaignPayloadProvenance(resolution.Payload.Final, resolution.Payload.Injected, campaignFieldSources(resolution.Payload.Input, campaignPayloadSourceInput), campaignPayloadSourceInput),
			}
			if resolution.HasBlockingViolations() {
				result.Status = "violations"
			}
			return writeSuccess(cmd, runtime, "meta campaign resolve-requirements", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&accountID, "account-id", "", "Ad account id (with or without act_ prefix)")
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Comma-separated mutation params (k=v,k2=v2)")
	cmd.Flags().StringVar(&jsonRaw, "json", "", "Inline JSON object payload")
	cmd.Flags().StringVar(&schemaDir, "schema-dir", schema.DefaultSchemaDir, "Schema pack root directory")
	cmd.Flags().StringVar(&rulesDir, "rules-dir", "", "Runtime rule pack root directory override")
	return cmd
}

func newCampaignUpdateCommand(runtime Runtime) *cobra.Command {
	var (
		profile             string
		version             string
		campaignID          string
		paramsRaw           string
		jsonRaw             string
		schemaDir           string
		confirmBudgetChange bool
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
			if err := enforceCampaignBudgetGuardrail(form, confirmBudgetChange); err != nil {
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
	cmd.Flags().BoolVar(&confirmBudgetChange, "confirm-budget-change", false, "Acknowledge budget mutation fields (daily_budget/lifetime_budget)")
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
		rulesDir         string
		dryRun           bool
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

			service := campaignNewService(campaignNewGraphClient())
			clonePlan, err := service.BuildClonePlan(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.CampaignCloneInput{
				SourceCampaignID: sourceCampaignID,
				TargetAccountID:  accountID,
				Overrides:        overrides,
				Fields:           cloneFields,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign clone", err)
			}

			resolution, err := resolveCampaignMutationRequirements(
				creds,
				resolvedVersion,
				schemaDir,
				rulesDir,
				campaignRequirementsMutation,
				clonePlan.TargetAccountID,
				clonePlan.Payload,
			)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign clone", err)
			}
			if resolution.HasBlockingViolations() {
				return writeCommandError(
					cmd,
					runtime,
					"meta campaign clone",
					fmt.Errorf("campaign requirements resolution blocked mutation: %s", resolution.ViolationSummary()),
				)
			}
			if err := lintCampaignMutation(linter, resolution.Payload.Final); err != nil {
				return writeCommandError(cmd, runtime, "meta campaign clone", err)
			}

			plan := campaignMutationPlanResult{
				Operation:         "clone",
				Mutation:          campaignRequirementsMutation,
				AccountID:         clonePlan.TargetAccountID,
				SourceCampaignID:  clonePlan.SourceCampaignID,
				Resolution:        resolution,
				FinalPayload:      copyCampaignPayload(resolution.Payload.Final),
				PayloadProvenance: campaignPayloadProvenance(resolution.Payload.Final, resolution.Payload.Injected, campaignCloneFieldSources(clonePlan), campaignPayloadSourceClone),
				ClonePlan:         cloneCampaignClonePlan(clonePlan),
			}
			if dryRun {
				return writeSuccess(cmd, runtime, "meta campaign clone", campaignDryRunResult{
					Status: "ok",
					DryRun: true,
					Plan:   plan,
				}, nil, nil)
			}

			createResult, err := service.Create(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.CampaignCreateInput{
				AccountID: clonePlan.TargetAccountID,
				Params:    plan.FinalPayload,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, "meta campaign clone", err)
			}

			result := marketing.CampaignCloneResult{
				Operation:        "clone",
				SourceCampaignID: clonePlan.SourceCampaignID,
				CampaignID:       createResult.CampaignID,
				RequestPath:      createResult.RequestPath,
				Payload:          copyCampaignPayload(plan.FinalPayload),
				RemovedFields:    append([]string(nil), clonePlan.RemovedFields...),
				Response:         cloneAnyMap(createResult.Response),
			}
			if err := persistTrackedResource(trackedResourceInput{
				Command:       "meta campaign clone",
				ResourceKind:  ops.ResourceKindCampaign,
				ResourceID:    result.CampaignID,
				CleanupAction: ops.CleanupActionPause,
				Profile:       creds.Name,
				GraphVersion:  resolvedVersion,
				AccountID:     clonePlan.TargetAccountID,
				SourceID:      clonePlan.SourceCampaignID,
				Metadata: map[string]string{
					"operation": result.Operation,
				},
			}); err != nil {
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
	cmd.Flags().StringVar(&rulesDir, "rules-dir", "", "Runtime rule pack root directory override")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Resolve clone requirements and output plan without executing mutation")
	cmd.Flags().BoolVar(&dryRun, "plan", false, "Alias of --dry-run")
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

func resolveCampaignMutationRequirements(
	creds *ProfileCredentials,
	version string,
	schemaDir string,
	rulesDir string,
	mutation string,
	accountID string,
	params map[string]string,
) (requirements.Resolution, error) {
	if creds == nil {
		return requirements.Resolution{}, errors.New("campaign profile credentials are required")
	}
	if strings.TrimSpace(version) == "" {
		return requirements.Resolution{}, errors.New("campaign version is required")
	}
	if strings.TrimSpace(mutation) == "" {
		return requirements.Resolution{}, errors.New("campaign mutation is required")
	}

	provider := campaignNewSchemaProvider(schemaDir)
	pack, err := provider.GetPack(creds.Profile.Domain, version)
	if err != nil {
		return requirements.Resolution{}, err
	}

	rulePack, err := campaignLoadRulePack(creds.Profile.Domain, version, rulesDir)
	if err != nil {
		return requirements.Resolution{}, err
	}
	resolver, err := requirements.NewResolver(pack, rulePack)
	if err != nil {
		return requirements.Resolution{}, err
	}

	return resolver.Resolve(requirements.ResolveInput{
		Mutation: mutation,
		Payload:  params,
		Profile: requirements.ProfileContext{
			ProfileName:  creds.Name,
			Domain:       creds.Profile.Domain,
			GraphVersion: version,
			TokenType:    creds.Profile.TokenType,
			Scopes:       append([]string(nil), creds.Profile.Scopes...),
			BusinessID:   creds.Profile.BusinessID,
			AccountID:    strings.TrimSpace(accountID),
			AppID:        creds.Profile.AppID,
		},
	})
}

func enforceCampaignBudgetGuardrail(params map[string]string, confirmed bool) error {
	if !campaignMutationChangesBudget(params) || confirmed {
		return nil
	}
	return errors.New("budget change detected in campaign mutation payload; rerun with --confirm-budget-change")
}

func campaignMutationChangesBudget(params map[string]string) bool {
	for key := range params {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if _, exists := campaignBudgetGuardrailKeys[normalized]; exists {
			return true
		}
	}
	return false
}

func campaignPayloadProvenance(final map[string]string, injected map[string]string, baseSources map[string]string, fallbackSource string) map[string]string {
	provenance := make(map[string]string, len(final))
	for field := range final {
		if _, isInjected := injected[field]; isInjected {
			provenance[field] = campaignPayloadSourceRule
			continue
		}
		if source, exists := baseSources[field]; exists {
			provenance[field] = source
			continue
		}
		provenance[field] = fallbackSource
	}
	return provenance
}

func campaignFieldSources(payload map[string]string, source string) map[string]string {
	sources := make(map[string]string, len(payload))
	for field := range payload {
		sources[field] = source
	}
	return sources
}

func campaignCloneFieldSources(plan *marketing.CampaignClonePlan) map[string]string {
	if plan == nil {
		return map[string]string{}
	}

	sources := make(map[string]string, len(plan.SourcePayload)+len(plan.Overrides))
	for field := range plan.SourcePayload {
		sources[field] = campaignPayloadSourceClone
	}
	for field := range plan.Overrides {
		sources[field] = campaignPayloadSourceOverride
	}
	return sources
}

func copyCampaignPayload(payload map[string]string) map[string]string {
	if len(payload) == 0 {
		return map[string]string{}
	}
	copied := make(map[string]string, len(payload))
	for key, value := range payload {
		copied[key] = value
	}
	return copied
}

func cloneCampaignClonePlan(plan *marketing.CampaignClonePlan) *marketing.CampaignClonePlan {
	if plan == nil {
		return nil
	}
	return &marketing.CampaignClonePlan{
		SourceCampaignID: plan.SourceCampaignID,
		TargetAccountID:  plan.TargetAccountID,
		Fields:           append([]string(nil), plan.Fields...),
		SourcePayload:    copyCampaignPayload(plan.SourcePayload),
		Overrides:        copyCampaignPayload(plan.Overrides),
		Payload:          copyCampaignPayload(plan.Payload),
		RemovedFields:    append([]string(nil), plan.RemovedFields...),
	}
}

func cloneAnyMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return map[string]any{}
	}
	copied := make(map[string]any, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}
