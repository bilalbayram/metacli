package cmd

import (
	"errors"
	"fmt"
	"sort"

	"github.com/bilalbayram/metacli/internal/plugin"
	"github.com/spf13/cobra"
)

const (
	doctorPluginID  = "doctor"
	doctorNamespace = "doctor"
)

type doctorTraceStatus struct {
	Recorded  bool   `json:"recorded"`
	PluginID  string `json:"plugin"`
	Namespace string `json:"namespace"`
	Command   string `json:"command"`
}

type doctorNamespaceDiscovery struct {
	Namespace       string                      `json:"namespace"`
	Plugin          string                      `json:"plugin"`
	Capabilities    []namespaceCapabilityStatus `json:"capabilities"`
	CapabilityCount int                         `json:"capability_count"`
}

type doctorTracerResult struct {
	Namespace      string                     `json:"namespace"`
	Plugin         string                     `json:"plugin"`
	Status         string                     `json:"status"`
	Trace          doctorTraceStatus          `json:"trace"`
	Discoveries    []doctorNamespaceDiscovery `json:"discoveries"`
	NamespaceCount int                        `json:"namespace_count"`
}

func NewDoctorCommand(runtime Runtime) *cobra.Command {
	tracer, err := plugin.NewNamespaceTracer(doctorNamespace)
	if err != nil {
		return newPluginErrorCommand(doctorNamespace, err)
	}

	registry, err := newPluginRegistry(tracer, newDoctorPluginManifest(runtime, tracer))
	if err != nil {
		return newPluginErrorCommand(doctorNamespace, err)
	}
	return buildCommandFromRegistry(registry, doctorNamespace)
}

func newDoctorPluginManifest(runtime Runtime, tracer *plugin.NamespaceTracer) plugin.Manifest {
	return plugin.Manifest{
		ID:      doctorPluginID,
		Command: doctorNamespace,
		Short:   "Doctor diagnostics commands",
		Build: func(pluginRuntime plugin.Runtime) (*cobra.Command, error) {
			doctorCmd := &cobra.Command{
				Use:   doctorNamespace,
				Short: "Doctor diagnostics commands",
				RunE: func(cmd *cobra.Command, _ []string) error {
					return requireSubcommand(cmd, doctorNamespace)
				},
			}
			doctorCmd.AddCommand(newDoctorTracerCommand(runtime, pluginRuntime, tracer))
			return doctorCmd, nil
		},
	}
}

func newDoctorTracerCommand(runtime Runtime, pluginRuntime plugin.Runtime, tracer *plugin.NamespaceTracer) *cobra.Command {
	return &cobra.Command{
		Use:   "tracer",
		Short: "Discover namespace capabilities and verify tracer integrity",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			commandName := "meta doctor tracer"
			if tracer == nil {
				return writeCommandError(cmd, runtime, commandName, errors.New("doctor namespace tracer is required"))
			}

			expectedEvent := plugin.TraceEvent{
				PluginID:  doctorPluginID,
				Namespace: doctorNamespace,
				Command:   "tracer",
			}
			if err := pluginRuntime.Trace(expectedEvent); err != nil {
				return writeCommandError(cmd, runtime, commandName, err)
			}
			if err := assertDoctorTraceRecorded(tracer, expectedEvent); err != nil {
				return writeCommandError(cmd, runtime, commandName, err)
			}

			discoveries, err := discoverNamespaceBootstrapCapabilities()
			if err != nil {
				return writeCommandError(cmd, runtime, commandName, err)
			}

			result := doctorTracerResult{
				Namespace: doctorNamespace,
				Plugin:    doctorPluginID,
				Status:    "ok",
				Trace: doctorTraceStatus{
					Recorded:  true,
					PluginID:  expectedEvent.PluginID,
					Namespace: expectedEvent.Namespace,
					Command:   expectedEvent.Command,
				},
				Discoveries:    discoveries,
				NamespaceCount: len(discoveries),
			}
			return writeSuccess(cmd, runtime, commandName, result, nil, nil)
		},
	}
}

func assertDoctorTraceRecorded(tracer *plugin.NamespaceTracer, expected plugin.TraceEvent) error {
	events := tracer.Events()
	if len(events) == 0 {
		return errors.New("doctor tracer did not record events")
	}
	last := events[len(events)-1]
	if last.PluginID != expected.PluginID || last.Namespace != expected.Namespace || last.Command != expected.Command {
		return fmt.Errorf(
			"doctor tracer recorded unexpected event plugin=%q namespace=%q command=%q",
			last.PluginID,
			last.Namespace,
			last.Command,
		)
	}
	return nil
}

func discoverNamespaceBootstrapCapabilities() ([]doctorNamespaceDiscovery, error) {
	specs := namespaceBootstrapSpecs()
	if len(specs) == 0 {
		return nil, errors.New("namespace bootstrap specs are required")
	}

	discoveries := make([]doctorNamespaceDiscovery, 0, len(specs))
	for _, spec := range specs {
		if err := validateNamespaceBootstrapSpec(spec); err != nil {
			return nil, fmt.Errorf("namespace %q: %w", spec.Namespace, err)
		}

		capabilities, err := discoverNamespaceCapabilities(spec)
		if err != nil {
			return nil, fmt.Errorf("namespace %q: %w", spec.Namespace, err)
		}

		discoveries = append(discoveries, doctorNamespaceDiscovery{
			Namespace:       spec.Namespace,
			Plugin:          spec.PluginID,
			Capabilities:    capabilities,
			CapabilityCount: len(capabilities),
		})
	}

	sort.Slice(discoveries, func(i, j int) bool {
		return discoveries[i].Namespace < discoveries[j].Namespace
	})
	return discoveries, nil
}
