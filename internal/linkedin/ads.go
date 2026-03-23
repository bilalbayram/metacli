package linkedin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const (
	pathAdAccounts                 = "/rest/adAccounts"
	pathAdAccountUsers             = "/rest/adAccountUsers"
	pathOrganizations              = "/rest/organizations"
	pathOrganizationAuthorizations = "/rest/organizationAuthorizations"
	pathCampaignGroups             = "/rest/adCampaignGroups"
	pathCampaigns                  = "/rest/adCampaigns"
	pathCreatives                  = "/rest/creatives"
	pathLeadForms                  = "/rest/leadForms"
	pathLeads                      = "/rest/leadFormResponses"
	pathTargetingFacets            = "/rest/adTargetingFacets"
)

var (
	defaultCampaignGroupStatuses = []string{
		"ACTIVE",
		"ARCHIVED",
		"CANCELED",
		"DRAFT",
		"PAUSED",
		"PENDING_DELETION",
		"REMOVED",
	}
	defaultCampaignStatuses = []string{
		"ACTIVE",
		"ARCHIVED",
		"CANCELED",
		"COMPLETED",
		"DRAFT",
		"PAUSED",
		"PENDING_DELETION",
		"REMOVED",
	}
)

type AdsService struct {
	Client *Client
}

type TargetingSearchInput struct {
	AccountURN URN
	Query      string
	PageSize   int
	PageToken  string
}

type TargetingValidateInput struct {
	AccountURN URN
	FacetURNs  []URN
}

func NewAdsService(client *Client) *AdsService {
	return &AdsService{Client: client}
}

func (s *AdsService) ListAdAccounts(ctx context.Context, search string, pageSize int, pageToken string) (*CollectionResult, error) {
	return s.listCollection(ctx, pathAdAccounts, valuesWithSearch(search, pageSize, pageToken))
}

func (s *AdsService) ListOrganizations(ctx context.Context, search string, pageSize int, pageToken string) (*CollectionResult, error) {
	return s.listCollection(ctx, pathOrganizations, valuesWithSearch(search, pageSize, pageToken))
}

func (s *AdsService) ListAccountRoles(ctx context.Context, accountURN URN, pageSize int, pageToken string) (*CollectionResult, error) {
	normalized, err := NormalizeSponsoredAccountURN(accountURN.String())
	if err != nil {
		return nil, err
	}
	if pageSize > 0 {
		return nil, errors.New("account role pagination is not supported by LinkedIn adAccountUsers?q=accounts")
	}
	if strings.TrimSpace(pageToken) != "" {
		return nil, errors.New("account role pagination is not supported by LinkedIn adAccountUsers?q=accounts")
	}
	values := map[string]string{
		"q":        "accounts",
		"accounts": normalized.String(),
	}
	return s.listCollection(ctx, pathAdAccountUsers, values)
}

func (s *AdsService) ListOrganizationRoles(ctx context.Context, organizationURN URN, pageSize int, pageToken string) (*CollectionResult, error) {
	if _, err := NormalizeOrganizationURN(organizationURN.String()); err != nil {
		return nil, err
	}
	values := map[string]string{
		"q":            "search",
		"organization": organizationURN.String(),
	}
	if pageSize > 0 {
		values[DefaultPageSizeParam] = fmt.Sprintf("%d", pageSize)
	}
	if strings.TrimSpace(pageToken) != "" {
		values[DefaultPageTokenParam] = pageToken
	}
	return s.listCollection(ctx, pathOrganizationAuthorizations, values)
}

func (s *AdsService) ListCampaignGroups(ctx context.Context, accountURN URN, search string, pageSize int, pageToken string) (*CollectionResult, error) {
	resourcePath, err := accountResourcePath(accountURN, "adCampaignGroups")
	if err != nil {
		return nil, err
	}
	return s.listCollection(ctx, resourcePath, valuesWithCampaignSearch(search, pageSize, pageToken, defaultCampaignGroupStatuses))
}

func (s *AdsService) ListCampaigns(ctx context.Context, accountURN URN, search string, pageSize int, pageToken string) (*CollectionResult, error) {
	resourcePath, err := accountResourcePath(accountURN, "adCampaigns")
	if err != nil {
		return nil, err
	}
	return s.listCollection(ctx, resourcePath, valuesWithCampaignSearch(search, pageSize, pageToken, defaultCampaignStatuses))
}

func (s *AdsService) ListCreatives(ctx context.Context, accountURN URN, search string, pageSize int, pageToken string) (*CollectionResult, error) {
	resourcePath, err := accountResourcePath(accountURN, "creatives")
	if err != nil {
		return nil, err
	}
	values, err := creativeListValues(search, pageSize, pageToken)
	if err != nil {
		return nil, err
	}
	return s.listCollectionWithHeaders(ctx, resourcePath, values, map[string]string{"X-RestLi-Method": "FINDER"})
}

func (s *AdsService) ListLeadForms(ctx context.Context, accountURN URN, pageSize int, pageToken string) (*CollectionResult, error) {
	if _, err := NormalizeSponsoredAccountURN(accountURN.String()); err != nil {
		return nil, err
	}
	values := map[string]string{
		"q":       "search",
		"account": accountURN.String(),
	}
	if pageSize > 0 {
		values[DefaultPageSizeParam] = fmt.Sprintf("%d", pageSize)
	}
	if strings.TrimSpace(pageToken) != "" {
		values[DefaultPageTokenParam] = pageToken
	}
	return s.listCollection(ctx, pathLeadForms, values)
}

func (s *AdsService) ListLeads(ctx context.Context, accountURN URN, pageSize int, pageToken string) (*CollectionResult, error) {
	if _, err := NormalizeSponsoredAccountURN(accountURN.String()); err != nil {
		return nil, err
	}
	values := map[string]string{
		"q":       "search",
		"account": accountURN.String(),
	}
	if pageSize > 0 {
		values[DefaultPageSizeParam] = fmt.Sprintf("%d", pageSize)
	}
	if strings.TrimSpace(pageToken) != "" {
		values[DefaultPageTokenParam] = pageToken
	}
	return s.listCollection(ctx, pathLeads, values)
}

func (s *AdsService) SearchTargeting(ctx context.Context, input TargetingSearchInput) (*CollectionResult, error) {
	if strings.TrimSpace(input.Query) == "" {
		return nil, errors.New("targeting search query is required")
	}
	if _, err := NormalizeSponsoredAccountURN(input.AccountURN.String()); err != nil {
		return nil, err
	}
	values := map[string]string{
		"q":       "search",
		"account": input.AccountURN.String(),
		"query":   strings.TrimSpace(input.Query),
	}
	if input.PageSize > 0 {
		values[DefaultPageSizeParam] = fmt.Sprintf("%d", input.PageSize)
	}
	if strings.TrimSpace(input.PageToken) != "" {
		values[DefaultPageTokenParam] = input.PageToken
	}
	return s.listCollection(ctx, pathTargetingFacets, values)
}

func (s *AdsService) ValidateTargeting(ctx context.Context, input TargetingValidateInput) (*Response, error) {
	if _, err := NormalizeSponsoredAccountURN(input.AccountURN.String()); err != nil {
		return nil, err
	}
	if len(input.FacetURNs) == 0 {
		return nil, errors.New("at least one targeting facet urn is required")
	}
	facets := make([]string, 0, len(input.FacetURNs))
	for _, facet := range input.FacetURNs {
		if strings.TrimSpace(facet.String()) == "" {
			return nil, errors.New("targeting facet urn is required")
		}
		facets = append(facets, facet.String())
	}
	body := map[string]any{
		"account": input.AccountURN.String(),
		"facets":  facets,
	}
	return s.Client.Do(ctx, Request{
		Method:   http.MethodPost,
		Path:     pathTargetingFacets + "/validate",
		Version:  s.clientVersion(),
		JSONBody: body,
	})
}

func (s *AdsService) listCollection(ctx context.Context, path string, query map[string]string) (*CollectionResult, error) {
	return s.listCollectionWithHeaders(ctx, path, query, nil)
}

func (s *AdsService) listCollectionWithHeaders(ctx context.Context, path string, query map[string]string, headers map[string]string) (*CollectionResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("ads client is required")
	}
	result := &CollectionResult{Elements: make([]map[string]any, 0)}
	paging, err := s.Client.FetchCollection(ctx, Request{
		Method:  http.MethodGet,
		Path:    path,
		Version: s.clientVersion(),
		Query:   query,
		Headers: headers,
	}, PaginationOptions{
		FollowNext: true,
		PageSize:   queryPageSize(query),
	}, func(element map[string]any) error {
		result.Elements = append(result.Elements, element)
		return nil
	})
	if err != nil {
		return nil, err
	}
	result.Paging = paging
	return result, nil
}

func (s *AdsService) clientVersion() string {
	if s == nil || s.Client == nil {
		return ""
	}
	return s.Client.Version
}

func valuesWithSearch(search string, pageSize int, pageToken string) map[string]string {
	values := map[string]string{"q": "search"}
	if strings.TrimSpace(search) != "" {
		values["search"] = strings.TrimSpace(search)
	}
	if pageSize > 0 {
		values[DefaultPageSizeParam] = fmt.Sprintf("%d", pageSize)
	}
	if strings.TrimSpace(pageToken) != "" {
		values[DefaultPageTokenParam] = pageToken
	}
	return values
}

func valuesWithCampaignSearch(search string, pageSize int, pageToken string, defaultStatuses []string) map[string]string {
	values := pagedValues(pageSize, pageToken)
	values["q"] = "search"
	values["search"] = namedSearchExpression(search, defaultStatuses)
	return values
}

func creativeListValues(search string, pageSize int, pageToken string) (map[string]string, error) {
	values := pagedValues(pageSize, pageToken)
	values["q"] = "criteria"
	values["sortOrder"] = "ASCENDING"
	search = strings.TrimSpace(search)
	if search == "" {
		return values, nil
	}
	return nil, errors.New("creative search currently supports only LinkedIn criteria finder inputs via meta li api; omit --search for full account listing")
}

func pagedValues(pageSize int, pageToken string) map[string]string {
	values := map[string]string{}
	if pageSize > 0 {
		values[DefaultPageSizeParam] = fmt.Sprintf("%d", pageSize)
	}
	if strings.TrimSpace(pageToken) != "" {
		values[DefaultPageTokenParam] = strings.TrimSpace(pageToken)
	}
	return values
}

func namedSearchExpression(search string, defaultStatuses []string) string {
	search = strings.TrimSpace(search)
	switch {
	case search == "":
		return searchExpression("status", defaultStatuses...)
	case strings.HasPrefix(search, "("):
		return search
	default:
		return searchExpression("name", search)
	}
}

func searchExpression(field string, values ...string) string {
	return fmt.Sprintf("(%s:(values:%s))", strings.TrimSpace(field), listValue(values...))
}

func accountResourcePath(accountURN URN, resource string) (string, error) {
	normalized, err := NormalizeSponsoredAccountURN(accountURN.String())
	if err != nil {
		return "", err
	}
	_, _, id, err := ParseURN(normalized.String())
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s/%s", pathAdAccounts, id, strings.TrimPrefix(strings.TrimSpace(resource), "/")), nil
}

func queryPageSize(values map[string]string) int {
	if values == nil {
		return 0
	}
	size, _ := strconv.Atoi(strings.TrimSpace(values[DefaultPageSizeParam]))
	return size
}
