package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
	"github.com/spf13/cobra"
)

var (
	apiLoadProfileCredentials = loadProfileCredentials
	apiNewGraphClient         = func() *graph.Client {
		return graph.NewClient(nil, "")
	}
)

func NewAPICommand(runtime Runtime) *cobra.Command {
	apiCmd := &cobra.Command{
		Use:   "api",
		Short: "Universal Graph API commands",
	}
	apiCmd.AddCommand(newAPIGetCommand(runtime))
	apiCmd.AddCommand(newAPIPostCommand(runtime))
	apiCmd.AddCommand(newAPIDeleteCommand(runtime))
	apiCmd.AddCommand(newAPIBatchCommand(runtime))
	return apiCmd
}

func newAPIGetCommand(runtime Runtime) *cobra.Command {
	var (
		profile    string
		version    string
		paramsRaw  string
		fields     string
		followNext bool
		limit      int
		pageSize   int
		stream     bool
	)

	cmd := &cobra.Command{
		Use:   "get <path>",
		Short: "Run a Graph GET request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, resolvedVersion, err := resolveAPIProfileAndVersion(runtime, profile, version)
			if err != nil {
				return err
			}

			query, err := parseKeyValueList(paramsRaw)
			if err != nil {
				return err
			}
			if strings.TrimSpace(fields) != "" {
				query["fields"] = fields
			}
			if pageSize > 0 {
				query["limit"] = strconv.Itoa(pageSize)
			}

			client := apiNewGraphClient()
			request := graph.Request{
				Method:      "GET",
				Path:        args[0],
				Version:     resolvedVersion,
				Query:       query,
				AccessToken: creds.Token,
				AppSecret:   creds.AppSecret,
			}

			if followNext || stream {
				items := make([]map[string]any, 0)
				pagination, err := client.FetchWithPagination(cmd.Context(), request, graph.PaginationOptions{
					FollowNext: followNext || stream,
					Limit:      limit,
					PageSize:   pageSize,
					Stream:     stream,
				}, func(item map[string]any) error {
					if stream {
						line, err := json.Marshal(item)
						if err != nil {
							return err
						}
						_, err = fmt.Fprintln(cmd.OutOrStdout(), string(line))
						return err
					}
					items = append(items, item)
					return nil
				})
				if err != nil {
					return err
				}
				if stream {
					return nil
				}
				return writeSuccess(cmd, runtime, "meta api get", items, pagination, nil)
			}

			resp, err := client.Do(cmd.Context(), request)
			if err != nil {
				return err
			}
			return writeSuccess(cmd, runtime, "meta api get", resp.Body, nil, resp.RateLimit)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version (for example v25.0)")
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Comma-separated query params (k=v,k2=v2)")
	cmd.Flags().StringVar(&fields, "fields", "", "Comma-separated Graph fields")
	cmd.Flags().BoolVar(&followNext, "follow-next", false, "Follow paging.next links")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of records to return")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "Page size for paginated queries")
	cmd.Flags().BoolVar(&stream, "stream", false, "Stream records as newline-delimited JSON")
	return cmd
}

func newAPIPostCommand(runtime Runtime) *cobra.Command {
	var (
		profile   string
		version   string
		paramsRaw string
		jsonRaw   string
	)

	cmd := &cobra.Command{
		Use:   "post <path>",
		Short: "Run a Graph POST request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, resolvedVersion, err := resolveAPIProfileAndVersion(runtime, profile, version)
			if err != nil {
				return err
			}

			form, err := parseKeyValueList(paramsRaw)
			if err != nil {
				return err
			}
			jsonForm, err := parseInlineJSONPayload(jsonRaw)
			if err != nil {
				return err
			}
			if err := mergeParams(form, jsonForm, "--json"); err != nil {
				return err
			}

			resp, err := apiNewGraphClient().Do(cmd.Context(), graph.Request{
				Method:      "POST",
				Path:        args[0],
				Version:     resolvedVersion,
				Form:        form,
				AccessToken: creds.Token,
				AppSecret:   creds.AppSecret,
			})
			if err != nil {
				return err
			}
			return writeSuccess(cmd, runtime, "meta api post", resp.Body, nil, resp.RateLimit)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version (for example v25.0)")
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Comma-separated form params (k=v,k2=v2)")
	cmd.Flags().StringVar(&jsonRaw, "json", "", "Inline JSON object payload")
	return cmd
}

func newAPIDeleteCommand(runtime Runtime) *cobra.Command {
	var (
		profile   string
		version   string
		paramsRaw string
	)

	cmd := &cobra.Command{
		Use:   "delete <path>",
		Short: "Run a Graph DELETE request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, resolvedVersion, err := resolveAPIProfileAndVersion(runtime, profile, version)
			if err != nil {
				return err
			}
			query, err := parseKeyValueList(paramsRaw)
			if err != nil {
				return err
			}

			resp, err := apiNewGraphClient().Do(cmd.Context(), graph.Request{
				Method:      "DELETE",
				Path:        args[0],
				Version:     resolvedVersion,
				Query:       query,
				AccessToken: creds.Token,
				AppSecret:   creds.AppSecret,
			})
			if err != nil {
				return err
			}
			return writeSuccess(cmd, runtime, "meta api delete", resp.Body, nil, resp.RateLimit)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version (for example v25.0)")
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Comma-separated query params (k=v,k2=v2)")
	return cmd
}

func newAPIBatchCommand(runtime Runtime) *cobra.Command {
	var (
		profile  string
		version  string
		filePath string
		useStdin bool
	)

	cmd := &cobra.Command{
		Use:   "batch",
		Short: "Execute a GET-only Graph batch request (max 50 entries)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if filePath == "" && !useStdin {
				return errors.New("either --file or --stdin must be provided")
			}
			if filePath != "" && useStdin {
				return errors.New("use only one input source: --file or --stdin")
			}

			creds, resolvedVersion, err := resolveAPIProfileAndVersion(runtime, profile, version)
			if err != nil {
				return err
			}

			payload, err := readBatchPayload(filePath, useStdin)
			if err != nil {
				return err
			}

			requests := make([]graph.BatchRequest, 0)
			if err := json.Unmarshal(payload, &requests); err != nil {
				return fmt.Errorf("decode batch payload: %w", err)
			}

			client := apiNewGraphClient()
			results, err := client.ExecuteGETBatch(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, requests)
			if err != nil {
				return err
			}
			return writeSuccess(cmd, runtime, "meta api batch", results, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&filePath, "file", "", "Path to batch JSON file")
	cmd.Flags().BoolVar(&useStdin, "stdin", false, "Read batch JSON from stdin")
	return cmd
}

func resolveAPIProfileAndVersion(runtime Runtime, profile string, version string) (*ProfileCredentials, string, error) {
	resolvedProfile := strings.TrimSpace(profile)
	if resolvedProfile == "" {
		resolvedProfile = runtime.ProfileName()
	}
	if resolvedProfile == "" {
		return nil, "", errors.New("profile is required (--profile or global --profile)")
	}

	creds, err := apiLoadProfileCredentials(resolvedProfile)
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

func parseKeyValueList(raw string) (map[string]string, error) {
	out := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	parts := strings.Split(raw, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		index := strings.Index(part, "=")
		if index <= 0 {
			return nil, fmt.Errorf("invalid --params entry %q; expected key=value", part)
		}
		key := strings.TrimSpace(part[:index])
		value := strings.TrimSpace(part[index+1:])
		if key == "" {
			return nil, fmt.Errorf("invalid --params entry %q; key cannot be empty", part)
		}
		out[key] = value
	}
	return out, nil
}

func parseInlineJSONPayload(raw string) (map[string]string, error) {
	out := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}

	decoded := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, fmt.Errorf("decode --json payload: %w", err)
	}
	for key, value := range decoded {
		if strings.TrimSpace(key) == "" {
			return nil, errors.New("invalid --json payload: key cannot be empty")
		}
		encoded, err := encodeJSONValue(value)
		if err != nil {
			return nil, fmt.Errorf("invalid --json payload for key %q: %w", key, err)
		}
		out[key] = encoded
	}
	return out, nil
}

func encodeJSONValue(value any) (string, error) {
	if value == nil {
		return "", errors.New("null values are not supported")
	}
	if typed, ok := value.(string); ok {
		return typed, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func mergeParams(dst map[string]string, src map[string]string, sourceName string) error {
	for key, value := range src {
		if _, exists := dst[key]; exists {
			return fmt.Errorf("duplicate payload key %q from %s", key, sourceName)
		}
		dst[key] = value
	}
	return nil
}

func readBatchPayload(filePath string, useStdin bool) ([]byte, error) {
	if useStdin {
		reader := bufio.NewReader(os.Stdin)
		return io.ReadAll(reader)
	}
	return os.ReadFile(filePath)
}
