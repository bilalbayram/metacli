package linkedin

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type recordingHTTPClient struct {
	t *testing.T

	responses []*http.Response
	requests  []*http.Request
}

func (c *recordingHTTPClient) Do(req *http.Request) (*http.Response, error) {
	c.requests = append(c.requests, req)
	if len(c.responses) == 0 {
		c.t.Fatal("unexpected request with no stubbed response")
	}
	res := c.responses[0]
	c.responses = c.responses[1:]
	return res, nil
}

func responseJSON(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestDoInjectsLinkedInHeaders(t *testing.T) {
	httpClient := &recordingHTTPClient{
		t:         t,
		responses: []*http.Response{responseJSON(http.StatusOK, `{"ok":true}`)},
	}
	client := NewClient(httpClient, "https://api.linkedin.com", "202402", "token-123")

	resp, err := client.Do(context.Background(), Request{
		Method: http.MethodGet,
		Path:   "/rest/adAccounts",
		Query:  map[string]string{"q": "search"},
	})
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d", resp.StatusCode)
	}

	if len(httpClient.requests) != 1 {
		t.Fatalf("unexpected request count %d", len(httpClient.requests))
	}
	req := httpClient.requests[0]
	if got := req.Header.Get("Authorization"); got != "Bearer token-123" {
		t.Fatalf("unexpected auth header %q", got)
	}
	if got := req.Header.Get("Linkedin-Version"); got != "202402" {
		t.Fatalf("unexpected version header %q", got)
	}
	if got := req.Header.Get("X-Restli-Protocol-Version"); got != DefaultRestliProtocol {
		t.Fatalf("unexpected restli header %q", got)
	}
	if got := req.URL.Path; got != "/rest/adAccounts" {
		t.Fatalf("unexpected path %q", got)
	}
	if got := req.URL.Query().Get("q"); got != "search" {
		t.Fatalf("unexpected query %q", got)
	}
}

func TestDoUsesQueryTunnelingForLongQueries(t *testing.T) {
	httpClient := &recordingHTTPClient{
		t:         t,
		responses: []*http.Response{responseJSON(http.StatusOK, `{"ok":true}`)},
	}
	client := NewClient(httpClient, "", "202402", "token-123")
	client.QueryTunnelThreshold = 10

	_, err := client.Do(context.Background(), Request{
		Method: http.MethodGet,
		Path:   "/rest/adAccounts",
		Query: map[string]string{
			"q":      "search",
			"filter": "this-is-long",
		},
		AllowQueryTunneling: true,
	})
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	if len(httpClient.requests) != 1 {
		t.Fatalf("unexpected request count %d", len(httpClient.requests))
	}
	req := httpClient.requests[0]
	if req.Method != http.MethodPost {
		t.Fatalf("unexpected method %s", req.Method)
	}
	if got := req.Header.Get("X-HTTP-Method-Override"); got != http.MethodGet {
		t.Fatalf("unexpected override header %q", got)
	}
	if body, _ := io.ReadAll(req.Body); len(body) == 0 {
		t.Fatal("expected tunneled query in request body")
	}
}

func TestFetchCollectionFollowsNextURL(t *testing.T) {
	httpClient := &recordingHTTPClient{
		t: t,
		responses: []*http.Response{
			responseJSON(http.StatusOK, `{"elements":[{"id":"1"}],"paging":{"links":[{"rel":"next","href":"/rest/adAccounts?pageToken=abc"}]}}`),
			responseJSON(http.StatusOK, `{"elements":[{"id":"2"}],"paging":{"count":2}}`),
		},
	}
	client := NewClient(httpClient, "https://api.linkedin.com", "202402", "token-123")

	seen := make([]string, 0, 2)
	paging, err := client.FetchCollection(context.Background(), Request{
		Method: http.MethodGet,
		Path:   "/rest/adAccounts",
		Query:  map[string]string{"q": "search"},
	}, PaginationOptions{FollowNext: true}, func(row map[string]any) error {
		seen = append(seen, row["id"].(string))
		return nil
	})
	if err != nil {
		t.Fatalf("fetch collection: %v", err)
	}
	if len(seen) != 2 || seen[0] != "1" || seen[1] != "2" {
		t.Fatalf("unexpected rows: %#v", seen)
	}
	if paging == nil || paging.NextPageToken != "" {
		t.Fatalf("unexpected paging: %#v", paging)
	}
}
