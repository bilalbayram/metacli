package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
	"github.com/bilalbayram/metacli/internal/marketing"
	"github.com/bilalbayram/metacli/internal/output"
	"github.com/spf13/cobra"
)

var (
	catalogLoadProfileCredentials = loadProfileCredentials
	catalogNewGraphClient         = func() *graph.Client {
		return graph.NewClient(nil, "")
	}
	catalogNewService = func(client *graph.Client) *marketing.CatalogService {
		return marketing.NewCatalogService(client)
	}
)

func NewCatalogCommand(runtime Runtime) *cobra.Command {
	catalogCmd := &cobra.Command{
		Use:   "catalog",
		Short: "Catalog item upload and batch workflows",
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("catalog requires a subcommand")
		},
	}
	catalogCmd.AddCommand(newCatalogUploadItemsCommand(runtime))
	catalogCmd.AddCommand(newCatalogBatchItemsCommand(runtime))
	return catalogCmd
}

func newCatalogUploadItemsCommand(runtime Runtime) *cobra.Command {
	var (
		profile      string
		version      string
		catalogID    string
		itemType     string
		filePath     string
		jsonRaw      string
		domainPolicy string
	)

	cmd := &cobra.Command{
		Use:   "upload-items",
		Short: "Upload catalog items with CREATE batch requests",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateDomainGatePolicy(domainPolicy); err != nil {
				return writeCommandError(cmd, runtime, "meta catalog upload-items", err)
			}

			creds, resolvedVersion, err := resolveCatalogProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta catalog upload-items", err)
			}
			proceed, err := enforceMarketingDomainGate(cmd, runtime, "meta catalog upload-items", domainPolicy, creds.Profile.Domain)
			if err != nil {
				return err
			}
			if !proceed {
				return nil
			}

			items, err := parseCatalogUploadItemsInput(filePath, jsonRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta catalog upload-items", err)
			}

			result, err := catalogNewService(catalogNewGraphClient()).UploadItems(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.CatalogUploadItemsInput{
				CatalogID: catalogID,
				ItemType:  itemType,
				Items:     items,
			})
			if err != nil {
				var itemErr *marketing.CatalogBatchItemErrors
				if errors.As(err, &itemErr) {
					return writeCatalogBatchItemError(cmd, runtime, "meta catalog upload-items", result, itemErr)
				}
				return writeCommandError(cmd, runtime, "meta catalog upload-items", err)
			}

			return writeSuccess(cmd, runtime, "meta catalog upload-items", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&catalogID, "catalog-id", "", "Catalog id")
	cmd.Flags().StringVar(&itemType, "item-type", "", "Catalog item type (default PRODUCT_ITEM)")
	cmd.Flags().StringVar(&filePath, "file", "", "Path to JSON items file")
	cmd.Flags().StringVar(&jsonRaw, "json", "", "Inline JSON payload containing items")
	cmd.Flags().StringVar(&domainPolicy, "domain-policy", domainGatePolicyStrict, "Domain gating policy for non-marketing profiles: strict|skip")
	return cmd
}

func newCatalogBatchItemsCommand(runtime Runtime) *cobra.Command {
	var (
		profile      string
		version      string
		catalogID    string
		itemType     string
		filePath     string
		jsonRaw      string
		domainPolicy string
	)

	cmd := &cobra.Command{
		Use:   "batch-items",
		Short: "Send explicit catalog item batch requests",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateDomainGatePolicy(domainPolicy); err != nil {
				return writeCommandError(cmd, runtime, "meta catalog batch-items", err)
			}

			creds, resolvedVersion, err := resolveCatalogProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta catalog batch-items", err)
			}
			proceed, err := enforceMarketingDomainGate(cmd, runtime, "meta catalog batch-items", domainPolicy, creds.Profile.Domain)
			if err != nil {
				return err
			}
			if !proceed {
				return nil
			}

			requests, err := parseCatalogBatchRequestsInput(filePath, jsonRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta catalog batch-items", err)
			}

			result, err := catalogNewService(catalogNewGraphClient()).BatchItems(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.CatalogBatchItemsInput{
				CatalogID: catalogID,
				ItemType:  itemType,
				Requests:  requests,
			})
			if err != nil {
				var itemErr *marketing.CatalogBatchItemErrors
				if errors.As(err, &itemErr) {
					return writeCatalogBatchItemError(cmd, runtime, "meta catalog batch-items", result, itemErr)
				}
				return writeCommandError(cmd, runtime, "meta catalog batch-items", err)
			}

			return writeSuccess(cmd, runtime, "meta catalog batch-items", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&catalogID, "catalog-id", "", "Catalog id")
	cmd.Flags().StringVar(&itemType, "item-type", "", "Catalog item type (default PRODUCT_ITEM)")
	cmd.Flags().StringVar(&filePath, "file", "", "Path to JSON batch file")
	cmd.Flags().StringVar(&jsonRaw, "json", "", "Inline JSON payload containing requests")
	cmd.Flags().StringVar(&domainPolicy, "domain-policy", domainGatePolicyStrict, "Domain gating policy for non-marketing profiles: strict|skip")
	return cmd
}

func resolveCatalogProfileAndVersion(runtime Runtime, profile string, version string) (*ProfileCredentials, string, error) {
	resolvedProfile := strings.TrimSpace(profile)
	if resolvedProfile == "" {
		resolvedProfile = runtime.ProfileName()
	}
	if resolvedProfile == "" {
		return nil, "", errors.New("profile is required (--profile or global --profile)")
	}

	creds, err := catalogLoadProfileCredentials(resolvedProfile)
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

func parseCatalogUploadItemsInput(filePath string, jsonRaw string) ([]marketing.CatalogUploadItem, error) {
	payload, err := readCatalogInputPayload(filePath, jsonRaw)
	if err != nil {
		return nil, err
	}

	decoded, err := decodeCatalogJSONArray(payload, "items")
	if err != nil {
		return nil, err
	}

	items := make([]marketing.CatalogUploadItem, 0, len(decoded))
	for idx, raw := range decoded {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("catalog upload item %d must be an object", idx)
		}
		retailerID := strings.TrimSpace(catalogString(item["retailer_id"]))
		if retailerID == "" {
			return nil, fmt.Errorf("catalog upload item %d retailer_id is required", idx)
		}
		rawData, ok := item["data"]
		if !ok {
			return nil, fmt.Errorf("catalog upload item %d data is required", idx)
		}
		data, ok := rawData.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("catalog upload item %d data must be an object", idx)
		}
		items = append(items, marketing.CatalogUploadItem{
			RetailerID: retailerID,
			Data:       data,
		})
	}
	return items, nil
}

func parseCatalogBatchRequestsInput(filePath string, jsonRaw string) ([]marketing.CatalogBatchRequest, error) {
	payload, err := readCatalogInputPayload(filePath, jsonRaw)
	if err != nil {
		return nil, err
	}

	decoded, err := decodeCatalogJSONArray(payload, "requests")
	if err != nil {
		return nil, err
	}

	requests := make([]marketing.CatalogBatchRequest, 0, len(decoded))
	for idx, raw := range decoded {
		request, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("catalog batch request %d must be an object", idx)
		}

		method := strings.TrimSpace(catalogString(request["method"]))
		if method == "" {
			return nil, fmt.Errorf("catalog batch request %d method is required", idx)
		}
		retailerID := strings.TrimSpace(catalogString(request["retailer_id"]))

		data := map[string]any{}
		if rawData, ok := request["data"]; ok {
			typed, isMap := rawData.(map[string]any)
			if !isMap {
				return nil, fmt.Errorf("catalog batch request %d data must be an object", idx)
			}
			data = typed
		}

		requests = append(requests, marketing.CatalogBatchRequest{
			Method:     method,
			RetailerID: retailerID,
			Data:       data,
		})
	}
	return requests, nil
}

func readCatalogInputPayload(filePath string, jsonRaw string) ([]byte, error) {
	trimmedPath := strings.TrimSpace(filePath)
	trimmedJSON := strings.TrimSpace(jsonRaw)
	switch {
	case trimmedPath == "" && trimmedJSON == "":
		return nil, errors.New("either --file or --json must be provided")
	case trimmedPath != "" && trimmedJSON != "":
		return nil, errors.New("use only one input source: --file or --json")
	case trimmedPath != "":
		payload, err := os.ReadFile(trimmedPath)
		if err != nil {
			return nil, fmt.Errorf("read catalog payload file %q: %w", trimmedPath, err)
		}
		return payload, nil
	default:
		return []byte(trimmedJSON), nil
	}
}

func decodeCatalogJSONArray(payload []byte, key string) ([]any, error) {
	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, fmt.Errorf("decode catalog payload: %w", err)
	}

	switch typed := decoded.(type) {
	case []any:
		return typed, nil
	case map[string]any:
		raw, ok := typed[key]
		if !ok {
			return nil, fmt.Errorf("catalog payload object must include %q array", key)
		}
		entries, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("catalog payload %q must be an array", key)
		}
		return entries, nil
	default:
		return nil, errors.New("catalog payload must be a JSON array or object")
	}
}

func writeCatalogBatchItemError(cmd *cobra.Command, runtime Runtime, commandName string, result *marketing.CatalogBatchResult, err error) error {
	errorInfo := &output.ErrorInfo{
		Type:      "catalog_item_errors",
		Message:   err.Error(),
		Retryable: false,
	}

	envelope, envErr := output.NewEnvelope(commandName, false, result, nil, nil, errorInfo)
	if envErr != nil {
		return fmt.Errorf("%w (secondary output error: %v)", err, envErr)
	}
	if writeErr := output.Write(cmd.ErrOrStderr(), selectedOutputFormat(runtime), envelope); writeErr != nil {
		return fmt.Errorf("%w (secondary output error: %v)", err, writeErr)
	}
	return err
}

func catalogString(value any) string {
	typed, _ := value.(string)
	return typed
}
