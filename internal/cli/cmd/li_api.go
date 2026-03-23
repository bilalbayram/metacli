package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/bilalbayram/metacli/internal/linkedin"
	"github.com/spf13/cobra"
)

func newLIApiGetCommand(runtime Runtime) *cobra.Command {
	var (
		profile      string
		version      string
		paramsRaw    string
		fields       string
		restliMethod string
		followNext   bool
		limit        int
		pageSize     int
		stream       bool
	)

	cmd := &cobra.Command{
		Use:   "get <path>",
		Short: "Run a raw LinkedIn GET request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, resolvedVersion, err := resolveLinkedInProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li api get", err, linkedInEnvelopeProvider(version))
			}
			client, err := newLinkedInClient(creds, resolvedVersion)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li api get", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			query, err := parseKeyValueList(paramsRaw)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li api get", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			if strings.TrimSpace(fields) != "" {
				query["fields"] = fields
			}
			if pageSize > 0 {
				query[linkedin.DefaultPageSizeParam] = fmt.Sprintf("%d", pageSize)
			}

			request := linkedin.Request{
				Method:              http.MethodGet,
				Path:                normalizeLinkedInAPIPath(args[0]),
				Version:             resolvedVersion,
				Query:               query,
				Headers:             linkedInRestliHeaders(restliMethod),
				AllowQueryTunneling: true,
			}

			if followNext || stream {
				rows := make([]map[string]any, 0)
				paging, err := client.FetchCollection(cmd.Context(), request, linkedin.PaginationOptions{
					FollowNext: followNext || stream,
					Limit:      limit,
					PageSize:   pageSize,
				}, func(row map[string]any) error {
					if stream {
						line, marshalErr := json.Marshal(row)
						if marshalErr != nil {
							return marshalErr
						}
						_, writeErr := fmt.Fprintln(cmd.OutOrStdout(), string(line))
						return writeErr
					}
					rows = append(rows, row)
					return nil
				})
				if err != nil {
					return writeCommandErrorWithProvider(cmd, runtime, "meta li api get", err, linkedInEnvelopeProvider(resolvedVersion))
				}
				if stream {
					return nil
				}
				return writeSuccessWithProvider(cmd, runtime, "meta li api get", rows, paging, nil, linkedInEnvelopeProvider(resolvedVersion))
			}

			resp, err := client.Do(cmd.Context(), request)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li api get", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, "meta li api get", resp.Body, nil, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Comma-separated query params (k=v,k2=v2)")
	cmd.Flags().StringVar(&fields, "fields", "", "Comma-separated fields selector")
	cmd.Flags().StringVar(&restliMethod, "restli-method", "", "Optional X-RestLi-Method header value")
	cmd.Flags().BoolVar(&followNext, "follow-next", false, "Follow cursor pagination")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of records to return")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "Page size for paginated queries")
	cmd.Flags().BoolVar(&stream, "stream", false, "Stream collection elements as JSONL")
	return cmd
}

func newLIApiPostCommand(runtime Runtime) *cobra.Command {
	var (
		profile      string
		version      string
		paramsRaw    string
		jsonRaw      string
		restliMethod string
	)

	cmd := &cobra.Command{
		Use:   "post <path>",
		Short: "Run a raw LinkedIn POST request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, resolvedVersion, err := resolveLinkedInProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li api post", err, linkedInEnvelopeProvider(version))
			}
			client, err := newLinkedInClient(creds, resolvedVersion)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li api post", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			query, err := parseKeyValueList(paramsRaw)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li api post", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			body, err := parseRawJSONBody(jsonRaw)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li api post", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			resp, err := client.Do(cmd.Context(), linkedin.Request{
				Method:   http.MethodPost,
				Path:     normalizeLinkedInAPIPath(args[0]),
				Version:  resolvedVersion,
				Query:    query,
				Headers:  linkedInRestliHeaders(restliMethod),
				JSONBody: body,
			})
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li api post", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, "meta li api post", resp.Body, nil, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Comma-separated query params (k=v,k2=v2)")
	cmd.Flags().StringVar(&jsonRaw, "json", "", "Inline JSON payload")
	cmd.Flags().StringVar(&restliMethod, "restli-method", "", "Optional X-RestLi-Method header value")
	return cmd
}

func newLIApiDeleteCommand(runtime Runtime) *cobra.Command {
	var (
		profile      string
		version      string
		paramsRaw    string
		restliMethod string
	)

	cmd := &cobra.Command{
		Use:   "delete <path>",
		Short: "Run a raw LinkedIn DELETE request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, resolvedVersion, err := resolveLinkedInProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li api delete", err, linkedInEnvelopeProvider(version))
			}
			client, err := newLinkedInClient(creds, resolvedVersion)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li api delete", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			query, err := parseKeyValueList(paramsRaw)
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li api delete", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			resp, err := client.Do(cmd.Context(), linkedin.Request{
				Method:              http.MethodDelete,
				Path:                normalizeLinkedInAPIPath(args[0]),
				Version:             resolvedVersion,
				Query:               query,
				Headers:             linkedInRestliHeaders(restliMethod),
				AllowQueryTunneling: true,
			})
			if err != nil {
				return writeCommandErrorWithProvider(cmd, runtime, "meta li api delete", err, linkedInEnvelopeProvider(resolvedVersion))
			}
			return writeSuccessWithProvider(cmd, runtime, "meta li api delete", resp.Body, nil, nil, linkedInEnvelopeProvider(resolvedVersion))
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "LinkedIn profile name")
	cmd.Flags().StringVar(&version, "version", "", "LinkedIn version header (YYYYMM)")
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Comma-separated query params (k=v,k2=v2)")
	cmd.Flags().StringVar(&restliMethod, "restli-method", "", "Optional X-RestLi-Method header value")
	return cmd
}

func normalizeLinkedInAPIPath(raw string) string {
	path := strings.TrimSpace(raw)
	if path == "" {
		return "/rest"
	}
	if strings.HasPrefix(path, "/rest/") || strings.HasPrefix(path, "/v2/") {
		return path
	}
	return "/rest/" + strings.TrimPrefix(path, "/")
}

func linkedInRestliHeaders(restliMethod string) map[string]string {
	method := strings.TrimSpace(restliMethod)
	if method == "" {
		return nil
	}
	return map[string]string{"X-RestLi-Method": method}
}

func parseRawJSONBody(raw string) (any, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, fmt.Errorf("decode --json payload: %w", err)
	}
	return decoded, nil
}
