package linkedin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultBaseURL              = "https://api.linkedin.com"
	DefaultVersionHeader        = "Linkedin-Version"
	DefaultRestliProtocolHeader = "X-Restli-Protocol-Version"
	DefaultRestliProtocol       = "2.0.0"
	DefaultUserAgent            = "meta-marketing-cli/linkedin"
	DefaultQueryTunnelThreshold = 1800
	DefaultPageSizeParam        = "pageSize"
	DefaultPageTokenParam       = "pageToken"
	DefaultOffsetCountParam     = "count"
	DefaultOffsetStartParam     = "start"
)

type Client struct {
	BaseURL               string
	HTTP                  HTTPClient
	UserAgent             string
	Version               string
	AccessToken           string
	RestliProtocolVersion string
	QueryTunnelThreshold  int
	EnableQueryTunneling  bool
	DefaultHeaders        map[string]string
}

type Request struct {
	Method              string
	Path                string
	Version             string
	Query               map[string]string
	Headers             map[string]string
	AccessToken         string
	JSONBody            any
	FormBody            map[string]string
	AllowQueryTunneling bool
}

type Response struct {
	StatusCode int
	Headers    http.Header
	Raw        []byte
	Body       any
}

type CursorPaginationOptions struct {
	PageSizeParam  string
	PageTokenParam string
	PageSize       int
	PageToken      string
}

type CursorPaginationResult struct {
	Pages         int    `json:"pages"`
	NextPageToken string `json:"next_page_token,omitempty"`
}

type PaginationOptions struct {
	FollowNext     bool
	Limit          int
	PageSize       int
	PageToken      string
	PageSizeParam  string
	PageTokenParam string
}

type PagingInfo struct {
	Pages         int    `json:"pages,omitempty"`
	NextPageToken string `json:"next_page_token,omitempty"`
}

type CollectionResult struct {
	Elements []map[string]any `json:"elements"`
	Paging   *PagingInfo      `json:"paging,omitempty"`
}

func NewClient(httpClient HTTPClient, baseURL string, version string, accessToken string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		BaseURL:               strings.TrimSuffix(baseURL, "/"),
		HTTP:                  httpClient,
		UserAgent:             DefaultUserAgent,
		Version:               strings.TrimSpace(version),
		AccessToken:           strings.TrimSpace(accessToken),
		RestliProtocolVersion: DefaultRestliProtocol,
		QueryTunnelThreshold:  DefaultQueryTunnelThreshold,
		EnableQueryTunneling:  true,
	}
}

func (c *Client) Do(ctx context.Context, req Request) (*Response, error) {
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}
	if strings.TrimSpace(req.Path) == "" {
		return nil, errors.New("linkedin request path is required")
	}

	version := strings.TrimSpace(req.Version)
	if version == "" {
		version = strings.TrimSpace(c.Version)
	}
	if err := ValidateVersion(version); err != nil {
		return nil, err
	}

	endpoint, err := c.buildEndpoint(req.Path)
	if err != nil {
		return nil, err
	}

	query := cloneStringMap(req.Query)
	if query == nil {
		query = map[string]string{}
	}

	overrideMethod := ""
	if c.shouldTunnel(method, query, req.AllowQueryTunneling) {
		overrideMethod = method
		method = http.MethodPost
	}

	body, contentType, err := c.buildBody(method, req, query, overrideMethod != "")
	if err != nil {
		return nil, err
	}

	endpoint.RawQuery = buildQuery(query)
	httpReq, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return nil, fmt.Errorf("build linkedin request: %w", err)
	}
	c.applyHeaders(httpReq, req, version, contentType, overrideMethod)

	httpRes, err := c.httpClient().Do(httpReq)
	if err != nil {
		return nil, &Error{
			Category:  ErrorCategoryTransient,
			Message:   fmt.Sprintf("send request: %v", err),
			Retryable: true,
		}
	}
	defer httpRes.Body.Close()

	raw, err := io.ReadAll(httpRes.Body)
	if err != nil {
		return nil, &Error{
			Category:   ErrorCategoryTransient,
			StatusCode: httpRes.StatusCode,
			Message:    fmt.Sprintf("read response: %v", err),
			Retryable:  true,
		}
	}

	var decoded any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil, fmt.Errorf("decode linkedin response JSON: %w", err)
		}
	}

	if httpRes.StatusCode < 200 || httpRes.StatusCode >= 300 {
		return nil, annotateRequestError(parseAPIError(httpRes.StatusCode, decoded, httpRes.Header, raw), httpReq)
	}

	return &Response{
		StatusCode: httpRes.StatusCode,
		Headers:    httpRes.Header.Clone(),
		Raw:        raw,
		Body:       decoded,
	}, nil
}

func (c *Client) CursorPages(ctx context.Context, req Request, opts CursorPaginationOptions, fn func(*Response, string) error) (*CursorPaginationResult, error) {
	if fn == nil {
		return nil, errors.New("cursor page callback is required")
	}

	current := cloneRequest(req)
	if current.Query == nil {
		current.Query = map[string]string{}
	}
	pageSizeKey := normalizePaginationKey(opts.PageSizeParam, DefaultPageSizeParam)
	pageTokenKey := normalizePaginationKey(opts.PageTokenParam, DefaultPageTokenParam)
	if opts.PageSize > 0 {
		current.Query[pageSizeKey] = strconv.Itoa(opts.PageSize)
	}
	if strings.TrimSpace(opts.PageToken) != "" {
		current.Query[pageTokenKey] = strings.TrimSpace(opts.PageToken)
	}

	result := &CursorPaginationResult{}
	for {
		resp, err := c.Do(ctx, current)
		if err != nil {
			return nil, err
		}
		result.Pages++

		nextPageToken := ExtractNextPageToken(resp.Body)
		if err := fn(resp, nextPageToken); err != nil {
			return nil, err
		}
		result.NextPageToken = nextPageToken
		if nextPageToken == "" {
			return result, nil
		}
		current.Query[pageTokenKey] = nextPageToken
	}
}

func (c *Client) FetchCollection(ctx context.Context, req Request, opts PaginationOptions, visit func(map[string]any) error) (*PagingInfo, error) {
	if visit == nil {
		return nil, errors.New("collection visit callback is required")
	}
	current := cloneRequest(req)
	if current.Query == nil {
		current.Query = map[string]string{}
	}
	pageSizeKey := normalizePaginationKey(opts.PageSizeParam, DefaultPageSizeParam)
	pageTokenKey := normalizePaginationKey(opts.PageTokenParam, DefaultPageTokenParam)
	if opts.PageSize > 0 {
		current.Query[pageSizeKey] = strconv.Itoa(opts.PageSize)
	}
	if strings.TrimSpace(opts.PageToken) != "" {
		current.Query[pageTokenKey] = strings.TrimSpace(opts.PageToken)
	}

	result := &PagingInfo{}
	remaining := opts.Limit
	for {
		resp, err := c.Do(ctx, current)
		if err != nil {
			return nil, err
		}
		result.Pages++

		elements, err := decodeCollectionElements(resp.Body)
		if err != nil {
			return nil, err
		}
		for _, element := range elements {
			if err := visit(element); err != nil {
				return nil, err
			}
			if remaining > 0 {
				remaining--
				if remaining == 0 {
					result.NextPageToken = ExtractNextPageToken(resp.Body)
					return result, nil
				}
			}
		}

		nextPageToken := ExtractNextPageToken(resp.Body)
		result.NextPageToken = nextPageToken
		if !opts.FollowNext || nextPageToken == "" {
			return result, nil
		}
		current.Query[pageTokenKey] = nextPageToken
	}
}

func (c *Client) buildEndpoint(rawPath string) (*url.URL, error) {
	base, err := url.Parse(strings.TrimSpace(c.BaseURL))
	if err != nil {
		return nil, fmt.Errorf("parse linkedin base url: %w", err)
	}
	base.Path = path.Join(base.Path, strings.TrimPrefix(strings.TrimSpace(rawPath), "/"))
	return base, nil
}

func (c *Client) shouldTunnel(method string, query map[string]string, requestTunnel bool) bool {
	if method != http.MethodGet && method != http.MethodDelete {
		return false
	}
	if len(query) == 0 {
		return false
	}
	if !(c.EnableQueryTunneling || requestTunnel) {
		return false
	}
	encoded := buildQuery(query)
	threshold := c.QueryTunnelThreshold
	if threshold <= 0 {
		threshold = DefaultQueryTunnelThreshold
	}
	return len(encoded) > threshold
}

func (c *Client) buildBody(method string, req Request, query map[string]string, tunneled bool) (io.Reader, string, error) {
	if req.JSONBody != nil && len(req.FormBody) > 0 {
		return nil, "", errors.New("linkedin request cannot use both JSON body and form body")
	}
	if tunneled && req.JSONBody != nil {
		return nil, "", errors.New("linkedin query tunneling is only supported for form bodies")
	}

	if tunneled {
		form := cloneStringMap(req.FormBody)
		if form == nil {
			form = map[string]string{}
		}
		for key, value := range query {
			form[key] = value
		}
		return strings.NewReader(encodeForm(form)), "application/x-www-form-urlencoded", nil
	}

	switch {
	case len(req.FormBody) > 0:
		return strings.NewReader(encodeForm(req.FormBody)), "application/x-www-form-urlencoded", nil
	case req.JSONBody != nil:
		raw, err := json.Marshal(req.JSONBody)
		if err != nil {
			return nil, "", fmt.Errorf("encode linkedin request JSON: %w", err)
		}
		return bytes.NewReader(raw), "application/json", nil
	case method == http.MethodPost || method == http.MethodDelete:
		return bytes.NewReader(nil), "", nil
	default:
		return nil, "", nil
	}
}

func (c *Client) applyHeaders(req *http.Request, input Request, version string, contentType string, overrideMethod string) {
	if req == nil {
		return
	}

	for key, value := range c.DefaultHeaders {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		req.Header.Set(key, value)
	}
	for key, value := range input.Headers {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		req.Header.Set(key, value)
	}

	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(c.UserAgent) == "" {
		req.Header.Set("User-Agent", DefaultUserAgent)
	} else {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	protocol := strings.TrimSpace(c.RestliProtocolVersion)
	if protocol == "" {
		protocol = DefaultRestliProtocol
	}
	req.Header.Set(DefaultRestliProtocolHeader, protocol)
	if strings.TrimSpace(version) != "" {
		req.Header.Set(DefaultVersionHeader, version)
	}
	token := strings.TrimSpace(input.AccessToken)
	if token == "" {
		token = strings.TrimSpace(c.AccessToken)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if overrideMethod != "" {
		req.Header.Set("X-HTTP-Method-Override", overrideMethod)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
}

func (c *Client) httpClient() HTTPClient {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func cloneRequest(req Request) Request {
	clone := req
	clone.Query = cloneStringMap(req.Query)
	clone.Headers = cloneStringMap(req.Headers)
	clone.FormBody = cloneStringMap(req.FormBody)
	return clone
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func buildQuery(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, encodeQueryPair(key, values[key]))
	}
	return strings.Join(parts, "&")
}

func encodeForm(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, encodeQueryPair(key, values[key]))
	}
	return strings.Join(parts, "&")
}

func encodeQueryPair(key string, value string) string {
	escapedKey := url.QueryEscape(key)
	escapedValue := url.QueryEscape(value)
	escapedValue = restliValueEscaper.Replace(escapedValue)
	return escapedKey + "=" + escapedValue
}

var restliValueEscaper = strings.NewReplacer(
	"%28", "(",
	"%29", ")",
	"%2C", ",",
	"%3A", ":",
)

func normalizePaginationKey(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func ExtractNextPageToken(body any) string {
	switch typed := body.(type) {
	case map[string]any:
		return cursorTokenFromMap(typed)
	default:
		return ""
	}
}

func cursorTokenFromMap(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	for _, key := range []string{"nextPageToken", "next_page_token"} {
		if token, ok := payload[key].(string); ok && strings.TrimSpace(token) != "" {
			return strings.TrimSpace(token)
		}
	}
	for _, key := range []string{"paging", "metadata"} {
		child, ok := payload[key].(map[string]any)
		if !ok {
			continue
		}
		for _, tokenKey := range []string{"nextPageToken", "pageToken", "next_page_token"} {
			if token, ok := child[tokenKey].(string); ok && strings.TrimSpace(token) != "" {
				return strings.TrimSpace(token)
			}
		}
		if links, ok := child["links"].([]any); ok {
			for _, raw := range links {
				link, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				if rel, _ := link["rel"].(string); strings.EqualFold(strings.TrimSpace(rel), "next") {
					if href, _ := link["href"].(string); strings.TrimSpace(href) != "" {
						if token := tokenFromNextHref(href); token != "" {
							return token
						}
					}
				}
			}
		}
		if token := offsetTokenFromPaging(child); token != "" {
			return token
		}
	}
	return ""
}

func tokenFromNextHref(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	for _, key := range []string{"pageToken", "page_token", "page"} {
		if token := strings.TrimSpace(parsed.Query().Get(key)); token != "" {
			return token
		}
	}
	if token := strings.TrimSpace(parsed.Query().Get(DefaultOffsetStartParam)); token != "" {
		return token
	}
	return ""
}

func offsetTokenFromPaging(payload map[string]any) string {
	start, ok := numericField(payload, "start")
	if !ok {
		return ""
	}
	count, ok := numericField(payload, "count")
	if !ok || count <= 0 {
		return ""
	}
	total, ok := numericField(payload, "total")
	if !ok || total <= 0 {
		return ""
	}
	next := start + count
	if next >= total {
		return ""
	}
	return strconv.Itoa(next)
}

func numericField(payload map[string]any, key string) (int, bool) {
	value, ok := payload[key]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		return int(typed), true
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	default:
		return 0, false
	}
}

func annotateRequestError(err error, req *http.Request) error {
	apiErr, ok := err.(*Error)
	if !ok || req == nil {
		return err
	}
	if apiErr.Diagnostics == nil {
		apiErr.Diagnostics = map[string]any{}
	}
	apiErr.Diagnostics["request"] = map[string]any{
		"method":    req.Method,
		"path":      req.URL.Path,
		"raw_query": req.URL.RawQuery,
	}
	return apiErr
}

func decodeCollectionElements(body any) ([]map[string]any, error) {
	switch typed := body.(type) {
	case map[string]any:
		elementsRaw, ok := typed["elements"]
		if !ok {
			return nil, errors.New("collection response missing elements")
		}
		elementsAny, ok := elementsRaw.([]any)
		if !ok {
			return nil, errors.New("collection response elements must be an array")
		}
		elements := make([]map[string]any, 0, len(elementsAny))
		for _, raw := range elementsAny {
			element, ok := raw.(map[string]any)
			if !ok {
				return nil, errors.New("collection response element must be an object")
			}
			elements = append(elements, element)
		}
		return elements, nil
	case []any:
		elements := make([]map[string]any, 0, len(typed))
		for _, raw := range typed {
			element, ok := raw.(map[string]any)
			if !ok {
				return nil, errors.New("collection response element must be an object")
			}
			elements = append(elements, element)
		}
		return elements, nil
	default:
		return nil, errors.New("collection response must be an object or array")
	}
}
