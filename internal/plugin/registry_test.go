package plugin

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRegisterRejectsMalformedPluginManifest(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t)

	tests := []struct {
		name     string
		manifest Manifest
		errorMsg string
	}{
		{
			name: "missing plugin id",
			manifest: Manifest{
				Command: "ig",
				Short:   "Instagram commands",
				Build:   testBuilder("ig", "Instagram commands"),
			},
			errorMsg: "plugin id is required",
		},
		{
			name: "invalid command token",
			manifest: Manifest{
				ID:      "instagram",
				Command: "IG",
				Short:   "Instagram commands",
				Build:   testBuilder("IG", "Instagram commands"),
			},
			errorMsg: "invalid command name",
		},
		{
			name: "missing short",
			manifest: Manifest{
				ID:      "instagram",
				Command: "ig",
				Build:   testBuilder("ig", "Instagram commands"),
			},
			errorMsg: "plugin short description is required",
		},
		{
			name: "missing builder",
			manifest: Manifest{
				ID:      "instagram",
				Command: "ig",
				Short:   "Instagram commands",
			},
			errorMsg: "plugin command builder is required",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := registry.Register(tc.manifest)
			if err == nil {
				t.Fatal("expected registration error")
			}
			if !strings.Contains(err.Error(), tc.errorMsg) {
				t.Fatalf("expected error containing %q, got %v", tc.errorMsg, err)
			}
		})
	}
}

func TestRegisterConflictDoesNotMutateRegistry(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t)
	first := Manifest{
		ID:      "instagram",
		Command: "ig",
		Short:   "Instagram commands",
		Build:   testBuilder("ig", "Instagram commands"),
	}
	if err := registry.Register(first); err != nil {
		t.Fatalf("register first plugin: %v", err)
	}

	conflict := Manifest{
		ID:      "instagram-duplicate",
		Command: "ig",
		Short:   "Duplicate command",
		Build:   testBuilder("ig", "Duplicate command"),
	}
	err := registry.Register(conflict)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !strings.Contains(err.Error(), "command name collision") {
		t.Fatalf("expected command collision error, got %v", err)
	}
	if registry.Count() != 1 {
		t.Fatalf("expected registry size to remain 1, got %d", registry.Count())
	}

	built, buildErr := registry.Build("ig")
	if buildErr != nil {
		t.Fatalf("build existing command after conflict: %v", buildErr)
	}
	if built.Short != first.Short {
		t.Fatalf("expected original plugin command to be preserved, got %q", built.Short)
	}
}

func TestBuildRejectsMalformedBuiltCommand(t *testing.T) {
	t.Parallel()

	registry := newTestRegistry(t)
	manifest := Manifest{
		ID:      "instagram",
		Command: "ig",
		Short:   "Instagram commands",
		Build: func(_ Runtime) (*cobra.Command, error) {
			return &cobra.Command{
				Use:   "instagram",
				Short: "Instagram commands",
				RunE: func(_ *cobra.Command, _ []string) error {
					return nil
				},
			}, nil
		},
	}
	if err := registry.Register(manifest); err != nil {
		t.Fatalf("register plugin: %v", err)
	}

	_, err := registry.Build("ig")
	if err == nil {
		t.Fatal("expected build error")
	}
	if !strings.Contains(err.Error(), "command mismatch") {
		t.Fatalf("expected mismatch error, got %v", err)
	}
}

func TestRuntimeRequiresTracer(t *testing.T) {
	t.Parallel()

	_, err := NewRuntime(nil)
	if err == nil {
		t.Fatal("expected runtime constructor to fail without tracer")
	}
}

func TestNamespaceTracerRejectsCrossNamespaceEvents(t *testing.T) {
	t.Parallel()

	tracer, err := NewNamespaceTracer("ig")
	if err != nil {
		t.Fatalf("new tracer: %v", err)
	}
	runtime, err := NewRuntime(tracer)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	err = runtime.Trace(TraceEvent{
		PluginID:  "instagram",
		Namespace: "threads",
		Command:   "health",
	})
	if err == nil {
		t.Fatal("expected namespace trace error")
	}
	if !strings.Contains(err.Error(), "namespace tracer mismatch") {
		t.Fatalf("expected namespace mismatch error, got %v", err)
	}
}

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()

	runtime, err := NewRuntime(NopTracer{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	registry, err := NewRegistry(runtime)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	return registry
}

func testBuilder(use string, short string) CommandBuilder {
	return func(_ Runtime) (*cobra.Command, error) {
		return &cobra.Command{
			Use:   use,
			Short: short,
			RunE: func(_ *cobra.Command, _ []string) error {
				return nil
			},
		}, nil
	}
}
