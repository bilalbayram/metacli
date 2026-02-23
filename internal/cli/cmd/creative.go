package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
	"github.com/bilalbayram/metacli/internal/lint"
	"github.com/bilalbayram/metacli/internal/marketing"
	"github.com/bilalbayram/metacli/internal/schema"
	"github.com/spf13/cobra"
)

const (
	creativeMutationLintPath = "act_0/adcreatives"
)

var (
	creativeLoadProfileCredentials = loadProfileCredentials
	creativeNewGraphClient         = func() *graph.Client {
		return graph.NewClient(nil, "")
	}
	creativeNewSchemaProvider = func(schemaDir string) schema.SchemaProvider {
		return schema.NewProvider(schemaDir, "", "")
	}
	creativeNewService = func(client *graph.Client) *marketing.CreativeService {
		return marketing.NewCreativeService(client)
	}
)

func NewCreativeCommand(runtime Runtime) *cobra.Command {
	creativeCmd := &cobra.Command{
		Use:   "creative",
		Short: "Creative upload and create workflows",
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("creative requires a subcommand")
		},
	}
	creativeCmd.AddCommand(newCreativeUploadCommand(runtime))
	creativeCmd.AddCommand(newCreativeCreateCommand(runtime))
	return creativeCmd
}

func newCreativeUploadCommand(runtime Runtime) *cobra.Command {
	var (
		profile   string
		version   string
		accountID string
		filePath  string
		fileName  string
	)

	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload an image asset to an ad account",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, resolvedVersion, err := resolveCreativeProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta creative upload", err)
			}

			result, err := creativeNewService(creativeNewGraphClient()).Upload(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.CreativeUploadInput{
				AccountID: accountID,
				FilePath:  filePath,
				FileName:  fileName,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, "meta creative upload", err)
			}

			return writeSuccess(cmd, runtime, "meta creative upload", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&accountID, "account-id", "", "Ad account id (with or without act_ prefix)")
	cmd.Flags().StringVar(&filePath, "file", "", "Path to creative image file")
	cmd.Flags().StringVar(&fileName, "name", "", "Uploaded file name override")
	return cmd
}

func newCreativeCreateCommand(runtime Runtime) *cobra.Command {
	var (
		profile   string
		version   string
		accountID string
		paramsRaw string
		jsonRaw   string
		schemaDir string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an ad creative",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, resolvedVersion, err := resolveCreativeProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta creative create", err)
			}

			form, err := parseKeyValueList(paramsRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta creative create", err)
			}
			jsonForm, err := parseInlineJSONPayload(jsonRaw)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta creative create", err)
			}
			if err := mergeParams(form, jsonForm, "--json"); err != nil {
				return writeCommandError(cmd, runtime, "meta creative create", err)
			}

			linter, err := newCreativeMutationLinter(creds, resolvedVersion, schemaDir)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta creative create", err)
			}
			if err := lintCreativeMutation(linter, form); err != nil {
				return writeCommandError(cmd, runtime, "meta creative create", err)
			}

			result, err := creativeNewService(creativeNewGraphClient()).Create(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, marketing.CreativeCreateInput{
				AccountID: accountID,
				Params:    form,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, "meta creative create", err)
			}

			return writeSuccess(cmd, runtime, "meta creative create", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&accountID, "account-id", "", "Ad account id (with or without act_ prefix)")
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Comma-separated mutation params (k=v,k2=v2)")
	cmd.Flags().StringVar(&jsonRaw, "json", "", "Inline JSON object payload")
	cmd.Flags().StringVar(&schemaDir, "schema-dir", schema.DefaultSchemaDir, "Schema pack root directory")
	return cmd
}

func resolveCreativeProfileAndVersion(runtime Runtime, profile string, version string) (*ProfileCredentials, string, error) {
	resolvedProfile := strings.TrimSpace(profile)
	if resolvedProfile == "" {
		resolvedProfile = runtime.ProfileName()
	}
	if resolvedProfile == "" {
		return nil, "", errors.New("profile is required (--profile or global --profile)")
	}

	creds, err := creativeLoadProfileCredentials(resolvedProfile)
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

func newCreativeMutationLinter(creds *ProfileCredentials, version string, schemaDir string) (*lint.Linter, error) {
	if creds == nil {
		return nil, errors.New("creative profile credentials are required")
	}
	provider := creativeNewSchemaProvider(schemaDir)
	pack, err := provider.GetPack(creds.Profile.Domain, version)
	if err != nil {
		return nil, err
	}
	return lint.New(pack)
}

func lintCreativeMutation(linter *lint.Linter, params map[string]string) error {
	result := linter.Lint(&lint.RequestSpec{
		Method: "POST",
		Path:   creativeMutationLintPath,
		Params: params,
	}, true)
	if len(result.Errors) > 0 {
		return fmt.Errorf("creative mutation lint failed with %d error(s): %s", len(result.Errors), strings.Join(result.Errors, "; "))
	}
	return nil
}
