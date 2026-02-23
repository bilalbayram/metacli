package plugin

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/cobra"
)

type CommandBuilder func(runtime Runtime) (*cobra.Command, error)

type Manifest struct {
	ID      string
	Command string
	Short   string
	Build   CommandBuilder
}

type Registry struct {
	runtime   Runtime
	mu        sync.RWMutex
	byID      map[string]Manifest
	byCommand map[string]string
}

func NewRegistry(runtime Runtime) (*Registry, error) {
	if runtime.tracer == nil {
		return nil, errors.New("plugin runtime tracer is required")
	}
	return &Registry{
		runtime:   runtime,
		byID:      make(map[string]Manifest),
		byCommand: make(map[string]string),
	}, nil
}

func (r *Registry) Register(manifest Manifest) error {
	if err := validateManifest(manifest); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.byID[manifest.ID]; exists {
		return fmt.Errorf("plugin %q is already registered", manifest.ID)
	}
	if ownerID, exists := r.byCommand[manifest.Command]; exists {
		return fmt.Errorf("command name collision for %q: already owned by plugin %q", manifest.Command, ownerID)
	}

	r.byID[manifest.ID] = manifest
	r.byCommand[manifest.Command] = manifest.ID
	return nil
}

func (r *Registry) Build(commandName string) (*cobra.Command, error) {
	if err := validateNameToken("command name", commandName); err != nil {
		return nil, err
	}

	r.mu.RLock()
	ownerID, exists := r.byCommand[commandName]
	if !exists {
		r.mu.RUnlock()
		return nil, fmt.Errorf("plugin command %q is not registered", commandName)
	}
	manifest := r.byID[ownerID]
	r.mu.RUnlock()

	command, err := manifest.Build(r.runtime)
	if err != nil {
		return nil, fmt.Errorf("build plugin %q command %q: %w", manifest.ID, manifest.Command, err)
	}
	if err := validateBuiltCommand(manifest, command); err != nil {
		return nil, err
	}
	return command, nil
}

func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byID)
}

func (r *Registry) HasCommand(commandName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.byCommand[commandName]
	return exists
}

func (r *Registry) Commands() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	commands := make([]string, 0, len(r.byCommand))
	for command := range r.byCommand {
		commands = append(commands, command)
	}
	sort.Strings(commands)
	return commands
}

func validateManifest(manifest Manifest) error {
	if err := validateNameToken("plugin id", manifest.ID); err != nil {
		return err
	}
	if err := validateNameToken("command name", manifest.Command); err != nil {
		return err
	}
	if strings.TrimSpace(manifest.Short) == "" {
		return errors.New("plugin short description is required")
	}
	if manifest.Build == nil {
		return errors.New("plugin command builder is required")
	}
	return nil
}

func validateBuiltCommand(manifest Manifest, command *cobra.Command) error {
	if command == nil {
		return fmt.Errorf("plugin %q command %q returned a nil cobra command", manifest.ID, manifest.Command)
	}
	declared := commandToken(command.Use)
	if declared == "" {
		return fmt.Errorf("plugin %q command %q returned empty use", manifest.ID, manifest.Command)
	}
	if declared != manifest.Command {
		return fmt.Errorf("plugin %q command mismatch: manifest %q, built %q", manifest.ID, manifest.Command, declared)
	}
	if strings.TrimSpace(command.Short) == "" {
		return fmt.Errorf("plugin %q command %q requires short description", manifest.ID, manifest.Command)
	}
	if command.RunE == nil && command.Run == nil && len(command.Commands()) == 0 {
		return fmt.Errorf("plugin %q command %q has no execution path", manifest.ID, manifest.Command)
	}
	return nil
}

func commandToken(use string) string {
	parts := strings.Fields(strings.TrimSpace(use))
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}
