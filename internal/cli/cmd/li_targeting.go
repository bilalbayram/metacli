package cmd

import (
	"errors"
	"net/http"
	"strings"

	"github.com/bilalbayram/metacli/internal/linkedin"
	"github.com/spf13/cobra"
)

func newLITargetingFacetsCommand(runtime Runtime) *cobra.Command {
	var (
		profile string
		version string
	)
	cmd := &cobra.Command{
		Use:   "facets",
		Short: "List LinkedIn targeting facets. Avoid protected-characteristic targeting and comply with LinkedIn anti-discrimination policies.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, resolvedVersion, err := resolveLinkedInProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li targeting facets", err, linkedInEnvelopeProvider(version))
			}
			client, err := newLinkedInClient(creds, resolvedVersion)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li targeting facets", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			rows := make([]map[string]any, 0)
			paging, err := client.FetchCollection(cmd.Context(), linkedin.Request{
				Method:  http.MethodGet,
				Path:    "/rest/adTargetingFacets",
				Version: resolvedVersion,
			}, linkedin.PaginationOptions{FollowNext: true}, func(row map[string]any) error {
				rows = append(rows, row)
				return nil
			})
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li targeting facets", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, "meta li targeting facets", rows, paging, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	return cmd
}

func newLITargetingEntitiesCommand(runtime Runtime) *cobra.Command {
	var (
		profile string
		version string
		facet   string
	)
	cmd := &cobra.Command{
		Use:   "entities",
		Short: "List targeting entities for a facet. Use only lawful, policy-compliant targeting criteria.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLITargetingFinder(runtime, cmd, "meta li targeting entities", profile, version, map[string]string{
				"q":     "adTargetingFacet",
				"facet": strings.TrimSpace(facet),
			})
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	cmd.Flags().StringVar(&facet, "facet", "", "Targeting facet URN")
	mustMarkFlagRequired(cmd, "facet")
	return cmd
}

func newLITargetingSearchCommand(runtime Runtime) *cobra.Command {
	var (
		profile string
		version string
		facet   string
		query   string
	)
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Typeahead targeting search. Do not target protected classes or infer sensitive attributes.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(query) == "" {
				return writeCommandError(cmd, runtime, "meta li targeting search", errors.New("query is required"))
			}
			return runLITargetingFinder(runtime, cmd, "meta li targeting search", profile, version, map[string]string{
				"q":     "typeahead",
				"facet": strings.TrimSpace(facet),
				"query": strings.TrimSpace(query),
			})
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	cmd.Flags().StringVar(&facet, "facet", "", "Targeting facet URN")
	cmd.Flags().StringVar(&query, "query", "", "Search string")
	mustMarkFlagRequired(cmd, "facet")
	mustMarkFlagRequired(cmd, "query")
	return cmd
}

func newLITargetingSimilarCommand(runtime Runtime) *cobra.Command {
	var (
		profile string
		version string
		facet   string
		entity  string
	)
	cmd := &cobra.Command{
		Use:   "similar",
		Short: "List entities similar to a selected targeting entity. Review results for policy compliance before activation.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLITargetingFinder(runtime, cmd, "meta li targeting similar", profile, version, map[string]string{
				"q":     "similarEntities",
				"facet": strings.TrimSpace(facet),
				"urn":   strings.TrimSpace(entity),
			})
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	cmd.Flags().StringVar(&facet, "facet", "", "Targeting facet URN")
	cmd.Flags().StringVar(&entity, "entity-urn", "", "Targeting entity URN")
	mustMarkFlagRequired(cmd, "facet")
	mustMarkFlagRequired(cmd, "entity-urn")
	return cmd
}

func newLITargetingValidateCommand(runtime Runtime) *cobra.Command {
	var (
		profile    string
		version    string
		accountURN string
		facetsRaw  string
	)
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a set of targeting facets against a sponsored account. Policy compliance is still your responsibility.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, resolvedVersion, service, err := resolveLIAdsService(runtime, profile, version)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li targeting validate", err, linkedInEnvelopeProvider(version))
			}
			account, err := linkedin.NormalizeSponsoredAccountURN(accountURN)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li targeting validate", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			values := csvToSlice(facetsRaw)
			if len(values) == 0 {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li targeting validate", errors.New("at least one facet urn is required"), linkedInEnvelopeProvider(resolvedVersion))
			}
			urns := make([]linkedin.URN, 0, len(values))
			for _, value := range values {
				urn, err := linkedin.NormalizeURN(value, "")
				if err != nil {
					return writeCommandErrorWithProvider(cmd, runtime, "meta li targeting validate", err, linkedInEnvelopeProvider(resolvedVersion))
				}
				urns = append(urns, urn)
			}
			resp, err := service.ValidateTargeting(cmd.Context(), linkedin.TargetingValidateInput{AccountURN: account, FacetURNs: urns})
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li targeting validate", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, "meta li targeting validate", resp.Body, nil, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	cmd.Flags().StringVar(&accountURN, "account-urn", "", "Sponsored account URN or numeric account id")
	cmd.Flags().StringVar(&facetsRaw, "facet-urns", "", "Comma-separated targeting facet URNs")
	mustMarkFlagRequired(cmd, "account-urn")
	mustMarkFlagRequired(cmd, "facet-urns")
	return cmd
}

func runLITargetingFinder(runtime Runtime, cmd *cobra.Command, commandName string, profile string, version string, query map[string]string) error {
	creds, resolvedVersion, err := resolveLinkedInProfileAndVersion(runtime, profile, version)
	if err != nil {
		return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(version))
	}
	client, err := newLinkedInClient(creds, resolvedVersion)
	if err != nil {
		return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
	}
	rows := make([]map[string]any, 0)
	paging, err := client.FetchCollection(cmd.Context(), linkedin.Request{
		Method:  http.MethodGet,
		Path:    "/rest/adTargetingEntities",
		Version: resolvedVersion,
		Query:   query,
	}, linkedin.PaginationOptions{FollowNext: true}, func(row map[string]any) error {
		rows = append(rows, row)
		return nil
	})
	if err != nil {
		return writeCommandErrorWithProvider(cmd, runtime, commandName, err, linkedInEnvelopeProvider(resolvedVersion))
	}
	return writeSuccessWithProvider(cmd, runtime, commandName, rows, paging, nil, linkedInEnvelopeProvider(resolvedVersion))
}
