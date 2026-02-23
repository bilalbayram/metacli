package cmd

import (
	"errors"

	"github.com/bilalbayram/metacli/internal/plugin"
	"github.com/spf13/cobra"
)

const (
	igPluginID  = "instagram"
	igNamespace = "ig"
)

func NewIGCommand(runtime Runtime) *cobra.Command {
	tracer, err := plugin.NewNamespaceTracer(igNamespace)
	if err != nil {
		return newPluginErrorCommand(igNamespace, err)
	}

	registry, err := newPluginRegistry(tracer, newIGPluginManifest(runtime))
	if err != nil {
		return newPluginErrorCommand(igNamespace, err)
	}
	return buildCommandFromRegistry(registry, igNamespace)
}

func newIGPluginManifest(runtime Runtime) plugin.Manifest {
	return plugin.Manifest{
		ID:      igPluginID,
		Command: igNamespace,
		Short:   "Instagram Graph commands",
		Build: func(pluginRuntime plugin.Runtime) (*cobra.Command, error) {
			igCmd := &cobra.Command{
				Use:   igNamespace,
				Short: "Instagram Graph commands",
				RunE: func(_ *cobra.Command, _ []string) error {
					return errors.New("ig requires a subcommand")
				},
			}
			igCmd.AddCommand(newIGHealthCommand(runtime, pluginRuntime))
			return igCmd, nil
		},
	}
}

func newIGHealthCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Verify IG plugin runtime scaffold",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  igPluginID,
				Namespace: igNamespace,
				Command:   "health",
			}); err != nil {
				return err
			}
			return writeSuccess(cmd, runtime, "meta ig health", map[string]string{
				"namespace": igNamespace,
				"plugin":    igPluginID,
				"status":    "ok",
			}, nil, nil)
		},
	}
}
