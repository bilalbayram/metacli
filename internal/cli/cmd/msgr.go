package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
	"github.com/bilalbayram/metacli/internal/plugin"
	"github.com/spf13/cobra"
)

const (
	msgrPluginID  = "messenger"
	msgrNamespace = "msgr"
)

var (
	msgrLoadProfileCredentials = loadProfileCredentials
	msgrNewGraphClient         = func() *graph.Client {
		return graph.NewClient(nil, "")
	}
)

func NewMSGRCommand(runtime Runtime) *cobra.Command {
	tracer, err := plugin.NewNamespaceTracer(msgrNamespace)
	if err != nil {
		return newPluginErrorCommand(msgrNamespace, err)
	}

	registry, err := newPluginRegistry(tracer, newMSGRPluginManifest(runtime))
	if err != nil {
		return newPluginErrorCommand(msgrNamespace, err)
	}
	return buildCommandFromRegistry(registry, msgrNamespace)
}

func newMSGRPluginManifest(runtime Runtime) plugin.Manifest {
	return plugin.Manifest{
		ID:      msgrPluginID,
		Command: msgrNamespace,
		Short:   "Messenger Platform commands",
		Build: func(pluginRuntime plugin.Runtime) (*cobra.Command, error) {
			msgrCmd := &cobra.Command{
				Use:   msgrNamespace,
				Short: "Messenger Platform commands",
				RunE: func(cmd *cobra.Command, _ []string) error {
					return requireSubcommand(cmd, msgrNamespace)
				},
			}
			msgrCmd.AddCommand(newMSGRHealthCommand(runtime, pluginRuntime))
			msgrCmd.AddCommand(newMSGRConversationsCommand(runtime, pluginRuntime))
			msgrCmd.AddCommand(newMSGRAutoReplyCommand(runtime, pluginRuntime))
			return msgrCmd, nil
		},
	}
}

func newMSGRHealthCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Verify Messenger plugin runtime scaffold",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  msgrPluginID,
				Namespace: msgrNamespace,
				Command:   "health",
			}); err != nil {
				return err
			}
			return writeSuccess(cmd, runtime, "meta msgr health", map[string]string{
				"namespace": msgrNamespace,
				"plugin":    msgrPluginID,
				"status":    "ok",
			}, nil, nil)
		},
	}
}

func resolveMSGRProfileAndVersion(runtime Runtime, profile string, version string) (*ProfileCredentials, string, error) {
	resolvedProfile := strings.TrimSpace(profile)
	if resolvedProfile == "" {
		resolvedProfile = runtime.ProfileName()
	}
	if resolvedProfile == "" {
		return nil, "", errors.New("profile is required (--profile or global --profile)")
	}

	creds, err := msgrLoadProfileCredentials(resolvedProfile)
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

func resolveMSGRPageID(flagValue string, profile config.Profile) (string, error) {
	if trimmed := strings.TrimSpace(flagValue); trimmed != "" {
		return trimmed, nil
	}
	pageID := strings.TrimSpace(profile.PageID)
	if pageID != "" {
		return pageID, nil
	}
	return "", fmt.Errorf("page id is required (--page-id or profile page_id)")
}
