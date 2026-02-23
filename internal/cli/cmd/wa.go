package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bilalbayram/metacli/internal/plugin"
	"github.com/spf13/cobra"
)

type namespaceCapability struct {
	Name        string
	Description string
}

type namespaceBootstrapSpec struct {
	PluginID     string
	Namespace    string
	Short        string
	Capabilities []namespaceCapability
}

func NewWACommand(runtime Runtime) *cobra.Command {
	return newNamespaceBootstrapCommand(runtime, namespaceBootstrapSpec{
		PluginID:  "whatsapp",
		Namespace: "wa",
		Short:     "WhatsApp Cloud API commands",
		Capabilities: []namespaceCapability{
			{Name: "send-message", Description: "Send WhatsApp Cloud API messages"},
			{Name: "media-upload", Description: "Upload media assets for WhatsApp messages"},
		},
	})
}

func newNamespaceBootstrapCommand(runtime Runtime, spec namespaceBootstrapSpec) *cobra.Command {
	tracer, err := plugin.NewNamespaceTracer(spec.Namespace)
	if err != nil {
		return newPluginErrorCommand(spec.Namespace, err)
	}

	registry, err := newPluginRegistry(tracer, newNamespaceBootstrapManifest(runtime, spec))
	if err != nil {
		return newPluginErrorCommand(spec.Namespace, err)
	}
	return buildCommandFromRegistry(registry, spec.Namespace)
}

func newNamespaceBootstrapManifest(runtime Runtime, spec namespaceBootstrapSpec) plugin.Manifest {
	return plugin.Manifest{
		ID:      spec.PluginID,
		Command: spec.Namespace,
		Short:   spec.Short,
		Build: func(pluginRuntime plugin.Runtime) (*cobra.Command, error) {
			if err := validateNamespaceBootstrapSpec(spec); err != nil {
				return nil, err
			}

			namespaceCmd := &cobra.Command{
				Use:   spec.Namespace,
				Short: spec.Short,
				RunE: func(_ *cobra.Command, _ []string) error {
					return fmt.Errorf("%s requires a subcommand", spec.Namespace)
				},
			}
			namespaceCmd.AddCommand(newNamespaceHealthCommand(runtime, pluginRuntime, spec))
			namespaceCmd.AddCommand(newNamespaceCapabilityCommand(runtime, pluginRuntime, spec))
			return namespaceCmd, nil
		},
	}
}

func newNamespaceHealthCommand(runtime Runtime, pluginRuntime plugin.Runtime, spec namespaceBootstrapSpec) *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: fmt.Sprintf("Verify %s plugin bootstrap health", strings.ToUpper(spec.Namespace)),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  spec.PluginID,
				Namespace: spec.Namespace,
				Command:   "health",
			}); err != nil {
				return writeCommandError(cmd, runtime, fmt.Sprintf("meta %s health", spec.Namespace), err)
			}
			if len(spec.Capabilities) == 0 {
				return writeCommandError(cmd, runtime, fmt.Sprintf("meta %s health", spec.Namespace), errors.New("capabilities are required"))
			}
			return writeSuccess(cmd, runtime, fmt.Sprintf("meta %s health", spec.Namespace), map[string]any{
				"namespace":        spec.Namespace,
				"plugin":           spec.PluginID,
				"status":           "ok",
				"capability_count": len(spec.Capabilities),
			}, nil, nil)
		},
	}
}

func newNamespaceCapabilityCommand(runtime Runtime, pluginRuntime plugin.Runtime, spec namespaceBootstrapSpec) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "capability",
		Short: "Validate namespace capability support",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  spec.PluginID,
				Namespace: spec.Namespace,
				Command:   "capability",
			}); err != nil {
				return writeCommandError(cmd, runtime, fmt.Sprintf("meta %s capability", spec.Namespace), err)
			}

			capability, err := findNamespaceCapability(spec, name)
			if err != nil {
				return writeCommandError(cmd, runtime, fmt.Sprintf("meta %s capability", spec.Namespace), err)
			}

			return writeSuccess(cmd, runtime, fmt.Sprintf("meta %s capability", spec.Namespace), map[string]any{
				"namespace":   spec.Namespace,
				"plugin":      spec.PluginID,
				"capability":  capability.Name,
				"description": capability.Description,
				"supported":   true,
			}, nil, nil)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Capability name")
	return cmd
}

func validateNamespaceBootstrapSpec(spec namespaceBootstrapSpec) error {
	if strings.TrimSpace(spec.PluginID) == "" {
		return errors.New("plugin id is required")
	}
	if strings.TrimSpace(spec.Namespace) == "" {
		return errors.New("namespace is required")
	}
	if strings.TrimSpace(spec.Short) == "" {
		return errors.New("namespace short description is required")
	}
	if len(spec.Capabilities) == 0 {
		return errors.New("at least one capability is required")
	}

	seen := map[string]struct{}{}
	for _, capability := range spec.Capabilities {
		name := strings.TrimSpace(capability.Name)
		if name == "" {
			return errors.New("capability name is required")
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("duplicate capability %q", capability.Name)
		}
		seen[name] = struct{}{}
		if strings.TrimSpace(capability.Description) == "" {
			return fmt.Errorf("capability %q description is required", capability.Name)
		}
	}
	return nil
}

func findNamespaceCapability(spec namespaceBootstrapSpec, requested string) (*namespaceCapability, error) {
	name := strings.TrimSpace(requested)
	if name == "" {
		return nil, errors.New("capability name is required (--name)")
	}
	for _, capability := range spec.Capabilities {
		if capability.Name == name {
			copied := capability
			return &copied, nil
		}
	}
	return nil, fmt.Errorf("unsupported capability %q for namespace %q", requested, spec.Namespace)
}
