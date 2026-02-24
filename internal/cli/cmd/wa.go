package cmd

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/bilalbayram/metacli/internal/plugin"
	"github.com/spf13/cobra"
)

type namespaceCapability struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type namespaceCapabilityStatus struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Supported   bool   `json:"supported"`
}

type namespaceBootstrapSpec struct {
	PluginID     string
	Namespace    string
	Short        string
	Capabilities []namespaceCapability
}

var builtinNamespaceBootstrapSpecs = []namespaceBootstrapSpec{
	{
		PluginID:  "whatsapp",
		Namespace: "wa",
		Short:     "WhatsApp Cloud API commands",
		Capabilities: []namespaceCapability{
			{Name: "send-message", Description: "Send WhatsApp Cloud API messages"},
			{Name: "media-upload", Description: "Upload media assets for WhatsApp messages"},
		},
	},
	{
		PluginID:  "messenger",
		Namespace: "msgr",
		Short:     "Messenger Platform commands",
		Capabilities: []namespaceCapability{
			{Name: "send-api", Description: "Send messages through the Messenger Send API"},
			{Name: "webhook", Description: "Consume Messenger webhook events"},
		},
	},
	{
		PluginID:  "threads",
		Namespace: "threads",
		Short:     "Threads API commands",
		Capabilities: []namespaceCapability{
			{Name: "publish-post", Description: "Publish text or media posts via Threads API"},
			{Name: "read-insights", Description: "Fetch Threads media and account insights"},
		},
	},
	{
		PluginID:  "capi",
		Namespace: "capi",
		Short:     "Conversions API commands",
		Capabilities: []namespaceCapability{
			{Name: "send-event", Description: "Send conversion events to /events endpoint"},
			{Name: "test-event", Description: "Validate conversion payloads using test_event_code"},
		},
	},
}

func NewWACommand(runtime Runtime) *cobra.Command {
	return newNamespaceBootstrapCommandForNamespace(runtime, "wa")
}

func newNamespaceBootstrapCommandForNamespace(runtime Runtime, namespace string) *cobra.Command {
	spec, err := namespaceBootstrapSpecFor(namespace)
	if err != nil {
		return newPluginErrorCommand(namespace, err)
	}
	return newNamespaceBootstrapCommand(runtime, spec)
}

func namespaceBootstrapSpecs() []namespaceBootstrapSpec {
	specs := make([]namespaceBootstrapSpec, 0, len(builtinNamespaceBootstrapSpecs))
	for _, spec := range builtinNamespaceBootstrapSpecs {
		capabilities := make([]namespaceCapability, len(spec.Capabilities))
		copy(capabilities, spec.Capabilities)
		spec.Capabilities = capabilities
		specs = append(specs, spec)
	}

	sort.Slice(specs, func(i, j int) bool {
		return specs[i].Namespace < specs[j].Namespace
	})
	return specs
}

func namespaceBootstrapSpecFor(namespace string) (namespaceBootstrapSpec, error) {
	requested := strings.TrimSpace(namespace)
	if requested == "" {
		return namespaceBootstrapSpec{}, errors.New("namespace is required")
	}
	for _, spec := range namespaceBootstrapSpecs() {
		if spec.Namespace == requested {
			return spec, nil
		}
	}
	return namespaceBootstrapSpec{}, fmt.Errorf("namespace %q bootstrap spec is not registered", namespace)
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
	var (
		name     string
		discover bool
	)
	cmd := &cobra.Command{
		Use:   "capability",
		Short: "Validate namespace capability support",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			traceCommand := "capability"
			if discover {
				traceCommand = "capability-discover"
			}
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  spec.PluginID,
				Namespace: spec.Namespace,
				Command:   traceCommand,
			}); err != nil {
				return writeCommandError(cmd, runtime, fmt.Sprintf("meta %s capability", spec.Namespace), err)
			}

			if discover {
				if strings.TrimSpace(name) != "" {
					return writeCommandError(cmd, runtime, fmt.Sprintf("meta %s capability", spec.Namespace), errors.New("capability discovery does not accept --name"))
				}

				capabilities, err := discoverNamespaceCapabilities(spec)
				if err != nil {
					return writeCommandError(cmd, runtime, fmt.Sprintf("meta %s capability", spec.Namespace), err)
				}
				return writeSuccess(cmd, runtime, fmt.Sprintf("meta %s capability", spec.Namespace), map[string]any{
					"namespace":        spec.Namespace,
					"plugin":           spec.PluginID,
					"capabilities":     capabilities,
					"capability_count": len(capabilities),
				}, nil, nil)
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
	cmd.Flags().BoolVar(&discover, "discover", false, "Discover supported capabilities")
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

func discoverNamespaceCapabilities(spec namespaceBootstrapSpec) ([]namespaceCapabilityStatus, error) {
	if len(spec.Capabilities) == 0 {
		return nil, errors.New("capabilities are required")
	}

	seen := map[string]struct{}{}
	capabilities := make([]namespaceCapabilityStatus, 0, len(spec.Capabilities))
	for _, capability := range spec.Capabilities {
		name := strings.TrimSpace(capability.Name)
		if name == "" {
			return nil, errors.New("capability name is required")
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("duplicate capability %q", capability.Name)
		}
		seen[name] = struct{}{}

		description := strings.TrimSpace(capability.Description)
		if description == "" {
			return nil, fmt.Errorf("capability %q description is required", capability.Name)
		}

		capabilities = append(capabilities, namespaceCapabilityStatus{
			Name:        name,
			Description: description,
			Supported:   true,
		})
	}

	sort.Slice(capabilities, func(i, j int) bool {
		return capabilities[i].Name < capabilities[j].Name
	})
	return capabilities, nil
}
