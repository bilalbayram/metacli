package linkedin

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestListCampaignsBuildsExpectedRequest(t *testing.T) {
	httpClient := &recordingHTTPClient{
		t: t,
		responses: []*http.Response{
			responseJSON(http.StatusOK, `{"elements":[{"id":"1"}],"paging":{"count":1}}`),
		},
	}
	client := NewClient(httpClient, "https://api.linkedin.com", "202402", "token-123")
	service := NewAdsService(client)

	accountURN, err := SponsoredAccountURN("123")
	if err != nil {
		t.Fatalf("account urn: %v", err)
	}
	result, err := service.ListCampaigns(context.Background(), accountURN, "spring", 25, "abc")
	if err != nil {
		t.Fatalf("list campaigns: %v", err)
	}
	if len(result.Elements) != 1 {
		t.Fatalf("unexpected result %#v", result)
	}

	req := httpClient.requests[0]
	if got := req.URL.Path; got != "/rest/adAccounts/123/adCampaigns" {
		t.Fatalf("unexpected path %q", got)
	}
	query := req.URL.Query()
	if got := query.Get("q"); got != "search" {
		t.Fatalf("unexpected q %q", got)
	}
	if got := query.Get("account"); got != "" {
		t.Fatalf("unexpected legacy account query %q", got)
	}
	if got := query.Get("search"); got != "(name:(values:List(spring)))" {
		t.Fatalf("unexpected search %q", got)
	}
	if got := req.URL.RawQuery; !strings.Contains(got, "search=(name:(values:List(spring)))") {
		t.Fatalf("unexpected raw query %q", got)
	}
	if got := query.Get("pageSize"); got != "25" {
		t.Fatalf("unexpected pageSize %q", got)
	}
	if got := query.Get("pageToken"); got != "abc" {
		t.Fatalf("unexpected pageToken %q", got)
	}
}

func TestListCampaignGroupsBuildsAccountScopedSearchRequest(t *testing.T) {
	httpClient := &recordingHTTPClient{
		t: t,
		responses: []*http.Response{
			responseJSON(http.StatusOK, `{"elements":[{"id":"1"}],"paging":{"count":1}}`),
		},
	}
	client := NewClient(httpClient, "https://api.linkedin.com", "202402", "token-123")
	service := NewAdsService(client)

	accountURN, err := SponsoredAccountURN("123")
	if err != nil {
		t.Fatalf("account urn: %v", err)
	}
	if _, err := service.ListCampaignGroups(context.Background(), accountURN, "", 100, ""); err != nil {
		t.Fatalf("list campaign groups: %v", err)
	}

	req := httpClient.requests[0]
	if got := req.URL.Path; got != "/rest/adAccounts/123/adCampaignGroups" {
		t.Fatalf("unexpected path %q", got)
	}
	if got := req.URL.Query().Get("search"); got != "(status:(values:List(ACTIVE,ARCHIVED,CANCELED,DRAFT,PAUSED,PENDING_DELETION,REMOVED)))" {
		t.Fatalf("unexpected default search %q", got)
	}
	if got := req.URL.RawQuery; !strings.Contains(got, "search=(status:(values:List(ACTIVE,ARCHIVED,CANCELED,DRAFT,PAUSED,PENDING_DELETION,REMOVED)))") {
		t.Fatalf("unexpected raw query %q", got)
	}
}

func TestListCreativesUsesAccountFinderRequest(t *testing.T) {
	httpClient := &recordingHTTPClient{
		t: t,
		responses: []*http.Response{
			responseJSON(http.StatusOK, `{"elements":[{"id":"1"}],"paging":{"count":1}}`),
		},
	}
	client := NewClient(httpClient, "https://api.linkedin.com", "202402", "token-123")
	service := NewAdsService(client)

	accountURN, err := SponsoredAccountURN("123")
	if err != nil {
		t.Fatalf("account urn: %v", err)
	}
	if _, err := service.ListCreatives(context.Background(), accountURN, "", 25, "cursor-1"); err != nil {
		t.Fatalf("list creatives: %v", err)
	}

	req := httpClient.requests[0]
	if got := req.URL.Path; got != "/rest/adAccounts/123/creatives" {
		t.Fatalf("unexpected path %q", got)
	}
	if got := req.URL.Query().Get("q"); got != "criteria" {
		t.Fatalf("unexpected q %q", got)
	}
	if got := req.Header.Get("X-RestLi-Method"); got != "FINDER" {
		t.Fatalf("unexpected finder header %q", got)
	}
	if got := req.URL.Query().Get("pageToken"); got != "cursor-1" {
		t.Fatalf("unexpected page token %q", got)
	}
	if got := req.URL.Query().Get("account"); got != "" {
		t.Fatalf("unexpected legacy account query %q", got)
	}
}

func TestListAccountRolesUsesAccountsFinder(t *testing.T) {
	httpClient := &recordingHTTPClient{
		t: t,
		responses: []*http.Response{
			responseJSON(http.StatusOK, `{"elements":[{"role":"VIEWER"}],"paging":{"count":1}}`),
		},
	}
	client := NewClient(httpClient, "https://api.linkedin.com", "202402", "token-123")
	service := NewAdsService(client)

	accountURN, err := SponsoredAccountURN("123")
	if err != nil {
		t.Fatalf("account urn: %v", err)
	}
	if _, err := service.ListAccountRoles(context.Background(), accountURN, 0, ""); err != nil {
		t.Fatalf("list account roles: %v", err)
	}

	req := httpClient.requests[0]
	if got := req.URL.Path; got != "/rest/adAccountUsers" {
		t.Fatalf("unexpected path %q", got)
	}
	if got := req.URL.Query().Get("q"); got != "accounts" {
		t.Fatalf("unexpected q %q", got)
	}
	if got := req.URL.Query().Get("accounts"); got != accountURN.String() {
		t.Fatalf("unexpected accounts query %q", got)
	}
	if got := req.URL.RawQuery; !strings.Contains(got, "accounts=urn:li:sponsoredAccount:123") {
		t.Fatalf("unexpected raw query %q", got)
	}
	if got := req.URL.Query().Get("account"); got != "" {
		t.Fatalf("unexpected legacy account query %q", got)
	}
}

func TestValidateTargetingRejectsMissingFacets(t *testing.T) {
	service := NewAdsService(NewClient(nil, "", "202402", "token"))
	accountURN, _ := SponsoredAccountURN("123")
	_, err := service.ValidateTargeting(context.Background(), TargetingValidateInput{
		AccountURN: accountURN,
		FacetURNs:  nil,
	})
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected facet validation failure, got %v", err)
	}
}

func TestSearchTargetingRejectsBlankQuery(t *testing.T) {
	service := NewAdsService(NewClient(nil, "", "202402", "token"))
	accountURN, _ := SponsoredAccountURN("123")
	_, err := service.SearchTargeting(context.Background(), TargetingSearchInput{
		AccountURN: accountURN,
		Query:      " ",
	})
	if err == nil {
		t.Fatal("expected blank query failure")
	}
}

func TestNormalizeURNHelpers(t *testing.T) {
	urn, err := NormalizeSponsoredCampaignURN("456")
	if err != nil {
		t.Fatalf("normalize campaign urn: %v", err)
	}
	if got := urn.String(); got != "urn:li:adCampaign:456" {
		t.Fatalf("unexpected urn %q", got)
	}
}

func TestListAdAccountsBuildsSearchRequest(t *testing.T) {
	httpClient := &recordingHTTPClient{
		t: t,
		responses: []*http.Response{
			responseJSON(http.StatusOK, `{"elements":[{"id":"1"}],"paging":{"count":1}}`),
		},
	}
	client := NewClient(httpClient, "https://api.linkedin.com", "202402", "token-123")
	service := NewAdsService(client)

	if _, err := service.ListAdAccounts(context.Background(), "display", 10, "token"); err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	req := httpClient.requests[0]
	if got := req.URL.Query().Get("search"); got != "display" {
		t.Fatalf("unexpected search %q", got)
	}
	if got := req.URL.Query().Get("pageSize"); got != "10" {
		t.Fatalf("unexpected pageSize %q", got)
	}
}

func TestValuesWithSearchTrimAndEncode(t *testing.T) {
	values := valuesWithSearch(" spring ", 5, "abc")
	if got := values["search"]; got != "spring" {
		t.Fatalf("unexpected search %q", got)
	}
	if got := values["pageToken"]; got != "abc" {
		t.Fatalf("unexpected page token %q", got)
	}
}

func TestListLeadFormsRequiresAccountURN(t *testing.T) {
	service := NewAdsService(NewClient(nil, "", "202402", "token"))
	_, err := service.ListLeadForms(context.Background(), URN(""), 10, "")
	if err == nil {
		t.Fatal("expected account urn validation failure")
	}
}
