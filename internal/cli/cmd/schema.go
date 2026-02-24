package cmd

import (
	"github.com/bilalbayram/metacli/internal/schema"
	"github.com/spf13/cobra"
)

func NewSchemaCommand(runtime Runtime) *cobra.Command {
	schemaCmd := &cobra.Command{
		Use:   "schema",
		Short: "Manage local schema packs",
	}
	schemaCmd.AddCommand(newSchemaListCommand(runtime))
	schemaCmd.AddCommand(newSchemaSyncCommand(runtime))
	return schemaCmd
}

func newSchemaListCommand(runtime Runtime) *cobra.Command {
	var schemaDir string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List locally available schema packs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			provider := schema.NewProvider(schemaDir, "", "")
			packs, err := provider.ListPacks()
			if err != nil {
				return writeCommandError(cmd, runtime, "meta schema list", err)
			}
			return writeSuccess(cmd, runtime, "meta schema list", packs, nil, nil)
		},
	}
	cmd.Flags().StringVar(&schemaDir, "schema-dir", schema.DefaultSchemaDir, "Schema pack root directory")
	return cmd
}

func newSchemaSyncCommand(runtime Runtime) *cobra.Command {
	var (
		channel             string
		schemaDir           string
		manifestURL         string
		publicKey           string
		remoteFailurePolicy string
	)
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync schema packs from signed remote manifest",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := schema.ValidateRemoteFailurePolicy(remoteFailurePolicy); err != nil {
				return writeCommandError(cmd, runtime, "meta schema sync", err)
			}
			provider := schema.NewProvider(schemaDir, manifestURL, publicKey)
			result, err := provider.SyncWithRequest(cmd.Context(), schema.SyncRequest{
				Channel:             channel,
				RemoteFailurePolicy: remoteFailurePolicy,
			})
			if err != nil {
				return writeCommandError(cmd, runtime, "meta schema sync", err)
			}
			return writeSuccess(cmd, runtime, "meta schema sync", result, nil, nil)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "stable", "Schema channel to sync")
	cmd.Flags().StringVar(&schemaDir, "schema-dir", schema.DefaultSchemaDir, "Schema pack root directory")
	cmd.Flags().StringVar(&manifestURL, "manifest-url", schema.DefaultManifestURL, "Signed schema manifest URL")
	cmd.Flags().StringVar(&publicKey, "public-key", schema.DefaultManifestPubKey, "Base64 Ed25519 public key for manifest verification")
	cmd.Flags().StringVar(
		&remoteFailurePolicy,
		"remote-failure-policy",
		schema.SyncRemoteFailurePolicyHardFail,
		"Policy when remote sync fails: hard-fail|pinned-local",
	)
	return cmd
}
