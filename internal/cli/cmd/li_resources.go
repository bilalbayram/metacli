package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/bilalbayram/metacli/internal/linkedin"
	"github.com/spf13/cobra"
)

func newLIAccountListCommand(runtime Runtime) *cobra.Command {
	var (
		profile   string
		version   string
		search    string
		pageSize  int
		pageToken string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List LinkedIn ad accounts visible to the active profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, resolvedVersion, service, err := resolveLIAdsService(runtime, profile, version)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li account list", err, linkedInEnvelopeProvider(version))
			}
			_ = creds
			result, err := service.ListAdAccounts(cmd.Context(), search, pageSize, pageToken)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li account list", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, "meta li account list", result.Elements, result.Paging, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	cmd.Flags().StringVar(&search, "search", "", "Optional account search expression")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "Page size")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "Cursor token")
	return cmd
}

func newLIAccountRolesCommand(runtime Runtime) *cobra.Command {
	var (
		profile    string
		version    string
		accountURN string
		pageSize   int
		pageToken  string
	)

	cmd := &cobra.Command{
		Use:   "roles",
		Short: "List role assignments for a LinkedIn ad account",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, resolvedVersion, service, err := resolveLIAdsService(runtime, profile, version)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li account roles", err, linkedInEnvelopeProvider(version))
			}
			normalized, err := linkedin.NormalizeSponsoredAccountURN(accountURN)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li account roles", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			result, err := service.ListAccountRoles(cmd.Context(), normalized, pageSize, pageToken)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li account roles", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, "meta li account roles", result.Elements, result.Paging, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	cmd.Flags().StringVar(&accountURN, "account-urn", "", "Sponsored account URN or numeric account id")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "Page size")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "Pagination token (numeric offset for LinkedIn account roles)")
	mustMarkFlagRequired(cmd, "account-urn")
	return cmd
}

func newLIOrganizationListCommand(runtime Runtime) *cobra.Command {
	var (
		profile   string
		version   string
		search    string
		pageSize  int
		pageToken string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List LinkedIn organizations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, resolvedVersion, service, err := resolveLIAdsService(runtime, profile, version)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li organization list", err, linkedInEnvelopeProvider(version))
			}
			result, err := service.ListOrganizations(cmd.Context(), search, pageSize, pageToken)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li organization list", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, "meta li organization list", result.Elements, result.Paging, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	cmd.Flags().StringVar(&search, "search", "", "Optional organization search expression")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "Page size")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "Cursor token")
	return cmd
}

func newLIOrganizationGetCommand(runtime Runtime) *cobra.Command {
	return newLIGetEntityCommand(runtime, "meta li organization get", "Get a LinkedIn organization", "/rest/organizations")
}

func newLIOrganizationRolesCommand(runtime Runtime) *cobra.Command {
	var (
		profile         string
		version         string
		organizationURN string
		pageSize        int
		pageToken       string
	)

	cmd := &cobra.Command{
		Use:   "roles",
		Short: "List role assignments for a LinkedIn organization",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, resolvedVersion, service, err := resolveLIAdsService(runtime, profile, version)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li organization roles", err, linkedInEnvelopeProvider(version))
			}
			normalized, err := linkedin.NormalizeOrganizationURN(organizationURN)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li organization roles", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			result, err := service.ListOrganizationRoles(cmd.Context(), normalized, pageSize, pageToken)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li organization roles", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, "meta li organization roles", result.Elements, result.Paging, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	cmd.Flags().StringVar(&organizationURN, "organization-urn", "", "Organization URN or numeric organization id")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "Page size")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "Cursor token")
	mustMarkFlagRequired(cmd, "organization-urn")
	return cmd
}

func newLICampaignGroupListCommand(runtime Runtime) *cobra.Command {
	return newLIListByAccountCommand(runtime, "meta li campaign-group list", "List LinkedIn campaign groups", func(service *linkedin.AdsService, ctx context.Context, account linkedin.URN, search string, pageSize int, pageToken string) (*linkedin.CollectionResult, error) {
		return service.ListCampaignGroups(ctx, account, search, pageSize, pageToken)
	})
}

func newLICampaignGroupGetCommand(runtime Runtime) *cobra.Command {
	return newLIGetEntityCommand(runtime, "meta li campaign-group get", "Get a LinkedIn campaign group", "/rest/adCampaignGroups")
}

func newLICampaignGroupUpdateCommand(runtime Runtime) *cobra.Command {
	return newLIUpdateEntityCommand(runtime, "meta li campaign-group update", "Update a LinkedIn campaign group", "/rest/adCampaignGroups", "PARTIAL_UPDATE", nil)
}

func newLICampaignGroupPauseCommand(runtime Runtime) *cobra.Command {
	return newLIStatusEntityCommand(runtime, "pause", "meta li campaign-group pause", "Pause a LinkedIn campaign group", "/rest/adCampaignGroups", "status", "PAUSED")
}

func newLICampaignGroupResumeCommand(runtime Runtime) *cobra.Command {
	return newLIStatusEntityCommand(runtime, "resume", "meta li campaign-group resume", "Resume a LinkedIn campaign group", "/rest/adCampaignGroups", "status", "ACTIVE")
}

func newLICampaignListCommand(runtime Runtime) *cobra.Command {
	return newLIListByAccountCommand(runtime, "meta li campaign list", "List LinkedIn campaigns", func(service *linkedin.AdsService, ctx context.Context, account linkedin.URN, search string, pageSize int, pageToken string) (*linkedin.CollectionResult, error) {
		return service.ListCampaigns(ctx, account, search, pageSize, pageToken)
	})
}

func newLICampaignGetCommand(runtime Runtime) *cobra.Command {
	return newLIGetEntityCommand(runtime, "meta li campaign get", "Get a LinkedIn campaign", "/rest/adCampaigns")
}

func newLICampaignCreateCommand(runtime Runtime) *cobra.Command {
	var (
		profile               string
		version               string
		accountURN            string
		jsonRaw               string
		confirmBudgetChange   bool
		confirmScheduleChange bool
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a LinkedIn campaign",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := parseJSONObjectBody(jsonRaw)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li campaign create", err, linkedInEnvelopeProvider(version))
			}
			if budgetPresent(body) && !confirmBudgetChange {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li campaign create", errors.New("campaign budget mutations require --confirm-budget-change"), linkedInEnvelopeProvider(version))
			}
			if schedulePresent(body) && !confirmScheduleChange {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li campaign create", errors.New("campaign schedule mutations require --confirm-schedule-change"), linkedInEnvelopeProvider(version))
			}
			creds, resolvedVersion, err := resolveLinkedInProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li campaign create", err, linkedInEnvelopeProvider(version))
			}
			client, err := newLinkedInClient(creds, resolvedVersion)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li campaign create", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			account, err := linkedin.NormalizeSponsoredAccountURN(accountURN)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li campaign create", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			if _, exists := body["account"]; !exists {
				body["account"] = account.String()
			}
			resp, err := client.Do(cmd.Context(), linkedin.Request{
				Method:   http.MethodPost,
				Path:     "/rest/adCampaigns",
				Version:  resolvedVersion,
				JSONBody: body,
			})
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li campaign create", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, "meta li campaign create", resp.Body, nil, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	cmd.Flags().StringVar(&accountURN, "account-urn", "", "Sponsored account URN or numeric account id")
	cmd.Flags().StringVar(&jsonRaw, "json", "", "Inline JSON payload")
	cmd.Flags().BoolVar(&confirmBudgetChange, "confirm-budget-change", false, "Acknowledge campaign budget mutations")
	cmd.Flags().BoolVar(&confirmScheduleChange, "confirm-schedule-change", false, "Acknowledge campaign schedule mutations")
	mustMarkFlagRequired(cmd, "account-urn")
	mustMarkFlagRequired(cmd, "json")
	return cmd
}

func newLICampaignUpdateCommand(runtime Runtime) *cobra.Command {
	var (
		profile               string
		version               string
		jsonRaw               string
		confirmBudgetChange   bool
		confirmScheduleChange bool
	)
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a LinkedIn campaign",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := parseJSONObjectBody(jsonRaw)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li campaign update", err, linkedInEnvelopeProvider(version))
			}
			if budgetPresent(body) && !confirmBudgetChange {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li campaign update", errors.New("campaign budget mutations require --confirm-budget-change"), linkedInEnvelopeProvider(version))
			}
			if schedulePresent(body) && !confirmScheduleChange {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li campaign update", errors.New("campaign schedule mutations require --confirm-schedule-change"), linkedInEnvelopeProvider(version))
			}
			creds, resolvedVersion, err := resolveLinkedInProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li campaign update", err, linkedInEnvelopeProvider(version))
			}
			client, err := newLinkedInClient(creds, resolvedVersion)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li campaign update", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			resp, err := client.Do(cmd.Context(), linkedin.Request{
				Method:   http.MethodPost,
				Path:     liEntityPath("/rest/adCampaigns", args[0]),
				Version:  resolvedVersion,
				Headers:  linkedInRestliHeaders("PARTIAL_UPDATE"),
				JSONBody: body,
			})
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li campaign update", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, "meta li campaign update", resp.Body, nil, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	cmd.Flags().StringVar(&jsonRaw, "json", "", "Inline JSON payload")
	cmd.Flags().BoolVar(&confirmBudgetChange, "confirm-budget-change", false, "Acknowledge campaign budget mutations")
	cmd.Flags().BoolVar(&confirmScheduleChange, "confirm-schedule-change", false, "Acknowledge campaign schedule mutations")
	mustMarkFlagRequired(cmd, "json")
	return cmd
}

func newLICampaignPauseCommand(runtime Runtime) *cobra.Command {
	return newLIStatusEntityCommand(runtime, "pause", "meta li campaign pause", "Pause a LinkedIn campaign", "/rest/adCampaigns", "status", "PAUSED")
}

func newLICampaignResumeCommand(runtime Runtime) *cobra.Command {
	return newLIStatusEntityCommand(runtime, "resume", "meta li campaign resume", "Resume a LinkedIn campaign", "/rest/adCampaigns", "status", "ACTIVE")
}

func newLICreativeListCommand(runtime Runtime) *cobra.Command {
	return newLIListByAccountCommand(runtime, "meta li creative list", "List LinkedIn creatives", func(service *linkedin.AdsService, ctx context.Context, account linkedin.URN, search string, pageSize int, pageToken string) (*linkedin.CollectionResult, error) {
		return service.ListCreatives(ctx, account, search, pageSize, pageToken)
	})
}

func newLICreativeGetCommand(runtime Runtime) *cobra.Command {
	return newLIGetEntityCommand(runtime, "meta li creative get", "Get a LinkedIn creative", "/rest/creatives")
}

func newLICreativeCreateCommand(runtime Runtime) *cobra.Command {
	return newLICreateEntityCommand(runtime, "meta li creative create", "Create a LinkedIn creative", "/rest/creatives", true, validateLinkedInCreativeWrite)
}

func newLICreativeUpdateCommand(runtime Runtime) *cobra.Command {
	return newLIUpdateEntityCommand(runtime, "meta li creative update", "Update a LinkedIn creative", "/rest/creatives", "PARTIAL_UPDATE", validateLinkedInCreativeWrite)
}

func newLICreativeArchiveCommand(runtime Runtime) *cobra.Command {
	return newLIStatusEntityCommand(runtime, "archive", "meta li creative archive", "Archive a LinkedIn creative", "/rest/creatives", "intendedStatus", "ARCHIVED")
}

func newLILeadFormListCommand(runtime Runtime) *cobra.Command {
	var (
		profile    string
		version    string
		accountURN string
		pageSize   int
		pageToken  string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List LinkedIn lead forms for an account",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, resolvedVersion, service, err := resolveLIAdsService(runtime, profile, version)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li lead-form list", err, linkedInEnvelopeProvider(version))
			}
			account, err := linkedin.NormalizeSponsoredAccountURN(accountURN)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li lead-form list", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			result, err := service.ListLeadForms(cmd.Context(), account, pageSize, pageToken)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li lead-form list", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, "meta li lead-form list", result.Elements, result.Paging, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	cmd.Flags().StringVar(&accountURN, "account-urn", "", "Sponsored account URN or numeric account id")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "Page size")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "Cursor token")
	mustMarkFlagRequired(cmd, "account-urn")
	return cmd
}

func newLILeadFormGetCommand(runtime Runtime) *cobra.Command {
	return newLIGetEntityCommand(runtime, "meta li lead-form get", "Get a LinkedIn lead form", "/rest/leadForms")
}

func newLILeadFormCreateCommand(runtime Runtime) *cobra.Command {
	return newLICreateEntityCommand(runtime, "meta li lead-form create", "Create a LinkedIn lead form", "/rest/leadForms", true, nil)
}

func newLILeadListCommand(runtime Runtime) *cobra.Command {
	var (
		profile    string
		version    string
		accountURN string
		pageSize   int
		pageToken  string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List LinkedIn leads for an account",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, resolvedVersion, service, err := resolveLIAdsService(runtime, profile, version)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li lead list", err, linkedInEnvelopeProvider(version))
			}
			account, err := linkedin.NormalizeSponsoredAccountURN(accountURN)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li lead list", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			result, err := service.ListLeads(cmd.Context(), account, pageSize, pageToken)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li lead list", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, "meta li lead list", result.Elements, result.Paging, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	cmd.Flags().StringVar(&accountURN, "account-urn", "", "Sponsored account URN or numeric account id")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "Page size")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "Cursor token")
	mustMarkFlagRequired(cmd, "account-urn")
	return cmd
}

func newLILeadGetCommand(runtime Runtime) *cobra.Command {
	return newLIGetEntityCommand(runtime, "meta li lead get", "Get a LinkedIn lead", "/rest/leadFormResponses")
}

func newLILeadSyncCommand(runtime Runtime) *cobra.Command {
	var (
		profile    string
		version    string
		accountURN string
		pageSize   int
		stateFile  string
		reset      bool
	)
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Incrementally pull LinkedIn leads with stored cursor state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			profileName, err := resolveLIProfileName(runtime, profile)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta li lead sync", err)
			}
			_, resolvedVersion, service, err := resolveLIAdsService(runtime, profileName, version)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li lead sync", err, linkedInEnvelopeProvider(version))
			}
			account, err := linkedin.NormalizeSponsoredAccountURN(accountURN)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li lead sync", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			resolvedStateFile, err := resolveLILeadSyncStateFile(profileName, stateFile)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li lead sync", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			state, err := loadLILeadSyncState(resolvedStateFile, reset)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li lead sync", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			result, err := service.ListLeads(cmd.Context(), account, pageSize, state.PageToken)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li lead sync", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			filtered, seenIDs := filterNewLeadRows(result.Elements, state.SeenIDs)
			state.PageToken = ""
			if result.Paging != nil {
				state.PageToken = strings.TrimSpace(result.Paging.NextPageToken)
			}
			state.SeenIDs = seenIDs
			if err := saveLILeadSyncState(resolvedStateFile, state); err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li lead sync", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, "meta li lead sync", map[string]any{
				"rows":            filtered,
				"next_page_token": state.PageToken,
				"state_file":      resolvedStateFile,
			}, result.Paging, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	cmd.Flags().StringVar(&accountURN, "account-urn", "", "Sponsored account URN or numeric account id")
	cmd.Flags().IntVar(&pageSize, "page-size", 100, "Page size")
	cmd.Flags().StringVar(&stateFile, "state-file", "", "State file path for incremental sync")
	cmd.Flags().BoolVar(&reset, "reset", false, "Reset stored sync state before pulling")
	mustMarkFlagRequired(cmd, "account-urn")
	return cmd
}

func newLILeadWebhookCommand(runtime Runtime) *cobra.Command {
	cmd := newLISubcommandGroup("webhook", "LinkedIn lead webhook commands")
	cmd.AddCommand(newLILeadWebhookSubscribeCommand(runtime))
	cmd.AddCommand(newLILeadWebhookListCommand(runtime))
	cmd.AddCommand(newLILeadWebhookDeleteCommand(runtime))
	return cmd
}

func newLILeadWebhookSubscribeCommand(runtime Runtime) *cobra.Command {
	return newLICreateEntityCommand(runtime, "meta li lead webhook subscribe", "Subscribe a LinkedIn lead webhook", "/rest/leadWebhookSubscriptions", false, nil)
}

func newLILeadWebhookListCommand(runtime Runtime) *cobra.Command {
	return newLIListPathCommand(runtime, "meta li lead webhook list", "List LinkedIn lead webhook subscriptions", "/rest/leadWebhookSubscriptions")
}

func newLILeadWebhookDeleteCommand(runtime Runtime) *cobra.Command {
	return newLIDeleteEntityCommand(runtime, "meta li lead webhook delete", "Delete a LinkedIn lead webhook subscription", "/rest/leadWebhookSubscriptions")
}

func resolveLIAdsService(runtime Runtime, profile string, version string) (*ProfileCredentials, string, *linkedin.AdsService, error) {
	creds, resolvedVersion, err := resolveLinkedInProfileAndVersion(runtime, profile, version)
	if err != nil {
		return nil, "", nil, err
	}
	client, err := newLinkedInClient(creds, resolvedVersion)
	if err != nil {
		return nil, "", nil, err
	}
	return creds, resolvedVersion, linkedin.NewAdsService(client), nil
}

func newLIListPathCommand(runtime Runtime, commandName string, short string, path string) *cobra.Command {
	var (
		profile   string
		version   string
		pageSize  int
		pageToken string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, resolvedVersion, err := resolveLinkedInProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(version))
			}
			client, err := newLinkedInClient(creds, resolvedVersion)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
			}
			query := map[string]string{}
			if pageSize > 0 {
				query[linkedin.DefaultPageSizeParam] = fmt.Sprintf("%d", pageSize)
			}
			if strings.TrimSpace(pageToken) != "" {
				query[linkedin.DefaultPageTokenParam] = strings.TrimSpace(pageToken)
			}
			rows := make([]map[string]any, 0)
			paging, err := client.FetchCollection(cmd.Context(), linkedin.Request{
				Method:  http.MethodGet,
				Path:    path,
				Version: resolvedVersion,
				Query:   query,
			}, linkedin.PaginationOptions{FollowNext: true, PageSize: pageSize, PageToken: pageToken}, func(row map[string]any) error {
				rows = append(rows, row)
				return nil
			})
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, commandName, rows, paging, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "Page size")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "Cursor token")
	return cmd
}

func newLIListByAccountCommand(runtime Runtime, commandName string, short string, listFn func(*linkedin.AdsService, context.Context, linkedin.URN, string, int, string) (*linkedin.CollectionResult, error)) *cobra.Command {
	var (
		profile    string
		version    string
		accountURN string
		search     string
		pageSize   int
		pageToken  string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, resolvedVersion, service, err := resolveLIAdsService(runtime, profile, version)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(version))
			}
			account, err := linkedin.NormalizeSponsoredAccountURN(accountURN)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
			}
			result, err := listFn(service, cmd.Context(), account, search, pageSize, pageToken)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, commandName, result.Elements, result.Paging, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	cmd.Flags().StringVar(&accountURN, "account-urn", "", "Sponsored account URN or numeric account id")
	cmd.Flags().StringVar(&search, "search", "", "Optional search expression")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "Page size")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "Cursor token")
	mustMarkFlagRequired(cmd, "account-urn")
	return cmd
}

func newLIGetEntityCommand(runtime Runtime, commandName string, short string, basePath string) *cobra.Command {
	var (
		profile string
		version string
	)
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, resolvedVersion, err := resolveLinkedInProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(version))
			}
			client, err := newLinkedInClient(creds, resolvedVersion)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
			}
			resp, err := client.Do(cmd.Context(), linkedin.Request{
				Method:  http.MethodGet,
				Path:    liEntityPath(basePath, args[0]),
				Version: resolvedVersion,
			})
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, commandName, resp.Body, nil, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	return cmd
}

func newLICreateEntityCommand(runtime Runtime, commandName string, short string, path string, requireAccount bool, validator func(map[string]any) error) *cobra.Command {
	var (
		profile    string
		version    string
		accountURN string
		jsonRaw    string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, resolvedVersion, err := resolveLinkedInProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(version))
			}
			client, err := newLinkedInClient(creds, resolvedVersion)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
			}
			body, err := parseJSONObjectBody(jsonRaw)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
			}
			if requireAccount {
				account, normalizeErr := linkedin.NormalizeSponsoredAccountURN(accountURN)
				if normalizeErr != nil {
					return writeCommandErrorWithProvider(cmd, runtime, commandName, normalizeErr, linkedInEnvelopeProvider(resolvedVersion))
				}
				if _, exists := body["account"]; !exists {
					body["account"] = account.String()
				}
			}
			if validator != nil {
				if err := validator(body); err != nil {
					return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
				}
			}
			resp, err := client.Do(cmd.Context(), linkedin.Request{
				Method:   http.MethodPost,
				Path:     path,
				Version:  resolvedVersion,
				JSONBody: body,
			})
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, commandName, resp.Body, nil, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	cmd.Flags().StringVar(&accountURN, "account-urn", "", "Sponsored account URN or numeric account id")
	cmd.Flags().StringVar(&jsonRaw, "json", "", "Inline JSON payload")
	if requireAccount {
		mustMarkFlagRequired(cmd, "account-urn")
	}
	mustMarkFlagRequired(cmd, "json")
	return cmd
}

func newLIUpdateEntityCommand(runtime Runtime, commandName string, short string, basePath string, restliMethod string, validator func(map[string]any) error) *cobra.Command {
	var (
		profile string
		version string
		jsonRaw string
	)
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, resolvedVersion, err := resolveLinkedInProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(version))
			}
			client, err := newLinkedInClient(creds, resolvedVersion)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
			}
			body, err := parseJSONObjectBody(jsonRaw)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
			}
			if validator != nil {
				if err := validator(body); err != nil {
					return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
				}
			}
			resp, err := client.Do(cmd.Context(), linkedin.Request{
				Method:   http.MethodPost,
				Path:     liEntityPath(basePath, args[0]),
				Version:  resolvedVersion,
				Headers:  linkedInRestliHeaders(restliMethod),
				JSONBody: body,
			})
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, commandName, resp.Body, nil, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	cmd.Flags().StringVar(&jsonRaw, "json", "", "Inline JSON payload")
	mustMarkFlagRequired(cmd, "json")
	return cmd
}

func newLIStatusEntityCommand(runtime Runtime, use string, commandName string, short string, basePath string, field string, status string) *cobra.Command {
	var (
		profile string
		version string
		confirm bool
	)
	cmd := &cobra.Command{
		Use:   use + " <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirm {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, errors.New("use --confirm to apply live status changes"), linkedInEnvelopeProvider(version))
			}
			creds, resolvedVersion, err := resolveLinkedInProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(version))
			}
			client, err := newLinkedInClient(creds, resolvedVersion)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
			}
			resp, err := client.Do(cmd.Context(), linkedin.Request{
				Method:   http.MethodPost,
				Path:     liEntityPath(basePath, args[0]),
				Version:  resolvedVersion,
				Headers:  linkedInRestliHeaders("PARTIAL_UPDATE"),
				JSONBody: map[string]any{field: status},
			})
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, commandName, resp.Body, nil, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Confirm a live status mutation")
	return cmd
}

func newLIDeleteEntityCommand(runtime Runtime, commandName string, short string, basePath string) *cobra.Command {
	var (
		profile string
		version string
	)
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, resolvedVersion, err := resolveLinkedInProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(version))
			}
			client, err := newLinkedInClient(creds, resolvedVersion)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
			}
			resp, err := client.Do(cmd.Context(), linkedin.Request{
				Method:  http.MethodDelete,
				Path:    liEntityPath(basePath, args[0]),
				Version: resolvedVersion,
			})
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, commandName, resp.Body, nil, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	return cmd
}

func liEntityPath(basePath string, id string) string {
	return strings.TrimSuffix(basePath, "/") + "/" + url.PathEscape(strings.TrimSpace(id))
}

func parseJSONObjectBody(raw string) (map[string]any, error) {
	body, err := parseRawJSONBody(raw)
	if err != nil {
		return nil, err
	}
	if body == nil {
		return nil, errors.New("inline JSON payload is required")
	}
	object, ok := body.(map[string]any)
	if !ok {
		return nil, errors.New("inline JSON payload must be an object")
	}
	return object, nil
}

func validateLinkedInCampaignWrite(body map[string]any) error {
	if len(body) == 0 {
		return errors.New("campaign payload cannot be empty")
	}
	return nil
}

func validateLinkedInCreativeWrite(body map[string]any) error {
	if len(body) == 0 {
		return errors.New("creative payload cannot be empty")
	}
	for _, key := range []string{"type", "creativeType"} {
		if raw, ok := body[key].(string); ok && strings.TrimSpace(raw) != "" {
			switch strings.ToUpper(strings.TrimSpace(raw)) {
			case "SINGLE_IMAGE", "ARTICLE", "VIDEO", "LEAD_GEN", "LEADGEN":
				return nil
			default:
				return fmt.Errorf("unsupported creative type %q", raw)
			}
		}
	}
	return nil
}

func budgetPresent(body map[string]any) bool {
	for _, key := range []string{"dailyBudget", "lifetimeBudget", "totalBudget", "budget"} {
		if _, ok := body[key]; ok {
			return true
		}
	}
	return false
}

func schedulePresent(body map[string]any) bool {
	for _, key := range []string{"runSchedule", "schedule", "start", "end"} {
		if _, ok := body[key]; ok {
			return true
		}
	}
	return false
}

type liLeadSyncState struct {
	PageToken string            `json:"page_token,omitempty"`
	SeenIDs   map[string]string `json:"seen_ids,omitempty"`
}

func resolveLILeadSyncStateFile(profile string, stateFile string) (string, error) {
	if strings.TrimSpace(stateFile) != "" {
		return stateFile, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".meta", "linkedin-sync", profile+".json"), nil
}

func loadLILeadSyncState(path string, reset bool) (liLeadSyncState, error) {
	if reset {
		return liLeadSyncState{SeenIDs: map[string]string{}}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return liLeadSyncState{SeenIDs: map[string]string{}}, nil
		}
		return liLeadSyncState{}, err
	}
	state := liLeadSyncState{}
	if err := json.Unmarshal(raw, &state); err != nil {
		return liLeadSyncState{}, err
	}
	if state.SeenIDs == nil {
		state.SeenIDs = map[string]string{}
	}
	return state, nil
}

func saveLILeadSyncState(path string, state liLeadSyncState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func filterNewLeadRows(rows []map[string]any, seen map[string]string) ([]map[string]any, map[string]string) {
	if seen == nil {
		seen = map[string]string{}
	}
	filtered := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		id := strings.TrimSpace(fmt.Sprint(row["id"]))
		if id == "" {
			filtered = append(filtered, row)
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = id
		filtered = append(filtered, row)
	}
	return filtered, seen
}
