package cmd

import (
	"errors"
	"fmt"

	"github.com/bilalbayram/metacli/internal/plugin"
	"github.com/spf13/cobra"
)

func newPluginRegistry(tracer plugin.Tracer, manifests ...plugin.Manifest) (*plugin.Registry, error) {
	runtime, err := plugin.NewRuntime(tracer)
	if err != nil {
		return nil, err
	}
	registry, err := plugin.NewRegistry(runtime)
	if err != nil {
		return nil, err
	}
	for _, manifest := range manifests {
		if err := registry.Register(manifest); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func buildCommandFromRegistry(registry *plugin.Registry, commandName string) *cobra.Command {
	if registry == nil {
		return newPluginErrorCommand(commandName, errors.New("plugin registry is required"))
	}
	command, err := registry.Build(commandName)
	if err != nil {
		return newPluginErrorCommand(commandName, err)
	}
	return command
}

func newPluginErrorCommand(commandName string, cause error) *cobra.Command {
	return &cobra.Command{
		Use:   commandName,
		Short: fmt.Sprintf("%s plugin command", commandName),
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("initialize plugin command %q: %w", commandName, cause)
		},
	}
}
