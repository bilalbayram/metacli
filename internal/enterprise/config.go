package enterprise

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const SchemaVersion = 1
const EnterpriseMode = "enterprise"

type Config struct {
	SchemaVersion    int              `yaml:"schema_version"`
	Mode             string           `yaml:"mode"`
	DefaultOrg       string           `yaml:"default_org,omitempty"`
	Orgs             map[string]Org   `yaml:"orgs"`
	Roles            map[string]Role  `yaml:"roles,omitempty"`
	Bindings         []Binding        `yaml:"bindings,omitempty"`
	SecretGovernance SecretGovernance `yaml:"secret_governance,omitempty"`
}

type Org struct {
	ID               string               `yaml:"id"`
	DefaultWorkspace string               `yaml:"default_workspace,omitempty"`
	Workspaces       map[string]Workspace `yaml:"workspaces"`
}

type Workspace struct {
	ID string `yaml:"id"`
}

type WorkspaceContext struct {
	OrgName       string `json:"org_name"`
	OrgID         string `json:"org_id"`
	WorkspaceName string `json:"workspace_name"`
	WorkspaceID   string `json:"workspace_id"`
}

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".meta", "enterprise.yaml"), nil
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: enterprise config file does not exist at %s", os.ErrNotExist, path)
		}
		return nil, fmt.Errorf("read enterprise config %s: %w", path, err)
	}
	if isLegacy, err := detectLegacyConfigPayload(data); err != nil {
		return nil, fmt.Errorf("decode enterprise config %s: %w", path, err)
	} else if isLegacy {
		return nil, fmt.Errorf(
			"legacy CLI config detected at %s; run %q",
			path,
			legacyCutoverCommandHint(path),
		)
	}

	cfg := &Config{}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(cfg); err != nil {
		return nil, fmt.Errorf("decode enterprise config %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func Save(path string, cfg *Config) error {
	if cfg == nil {
		return errors.New("enterprise config is nil")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create enterprise config directory for %s: %w", path, err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal enterprise config: %w", err)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(path), ".enterprise-*.yaml")
	if err != nil {
		return fmt.Errorf("create temp enterprise config file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp enterprise config file: %w", err)
	}
	if err := tmpFile.Chmod(0o600); err != nil {
		tmpFile.Close()
		return fmt.Errorf("chmod temp enterprise config file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp enterprise config file: %w", err)
	}
	if err := os.Rename(tmpFile.Name(), path); err != nil {
		return fmt.Errorf("replace enterprise config file %s: %w", path, err)
	}
	return nil
}

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("enterprise config is nil")
	}
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported enterprise schema_version=%d (expected %d)", c.SchemaVersion, SchemaVersion)
	}
	mode := strings.TrimSpace(c.Mode)
	if mode == "" {
		return errors.New("enterprise mode is required; run `meta enterprise mode cutover`")
	}
	if mode != EnterpriseMode {
		return fmt.Errorf(
			"unsupported enterprise mode %q; only %q is allowed (run `meta enterprise mode cutover`)",
			mode,
			EnterpriseMode,
		)
	}
	if len(c.Orgs) == 0 {
		return errors.New("enterprise orgs map is required")
	}

	for _, orgName := range sortedOrgNames(c.Orgs) {
		if err := validateOrg(orgName, c.Orgs[orgName]); err != nil {
			return err
		}
	}

	defaultOrg := strings.TrimSpace(c.DefaultOrg)
	if defaultOrg != "" {
		if _, ok := c.Orgs[defaultOrg]; !ok {
			return fmt.Errorf("default_org %q does not exist", defaultOrg)
		}
	}
	if err := validateRoles(c.Roles); err != nil {
		return err
	}
	if err := validateBindings(c.Orgs, c.Roles, c.Bindings); err != nil {
		return err
	}
	if err := validateSecretGovernance(c.Orgs, c.SecretGovernance); err != nil {
		return err
	}
	return nil
}

func (c *Config) ResolveWorkspace(orgName, workspaceName string) (WorkspaceContext, error) {
	if c == nil {
		return WorkspaceContext{}, errors.New("enterprise config is nil")
	}
	if err := c.Validate(); err != nil {
		return WorkspaceContext{}, err
	}

	orgName = strings.TrimSpace(orgName)
	workspaceName = strings.TrimSpace(workspaceName)

	if orgName == "" {
		orgName = strings.TrimSpace(c.DefaultOrg)
	}
	if orgName == "" {
		return WorkspaceContext{}, errors.New("org is required (--org or default_org)")
	}

	org, ok := c.Orgs[orgName]
	if !ok {
		return WorkspaceContext{}, fmt.Errorf("org %q does not exist", orgName)
	}

	if workspaceName == "" {
		workspaceName = strings.TrimSpace(org.DefaultWorkspace)
	}
	if workspaceName == "" {
		return WorkspaceContext{}, fmt.Errorf("workspace is required (--workspace or org %q default_workspace)", orgName)
	}

	workspace, ok := org.Workspaces[workspaceName]
	if !ok {
		return WorkspaceContext{}, fmt.Errorf("workspace %q does not exist in org %q", workspaceName, orgName)
	}

	return WorkspaceContext{
		OrgName:       orgName,
		OrgID:         org.ID,
		WorkspaceName: workspaceName,
		WorkspaceID:   workspace.ID,
	}, nil
}

func validateOrg(name string, org Org) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("org name cannot be empty")
	}
	if strings.TrimSpace(org.ID) == "" {
		return fmt.Errorf("org %q id is required", name)
	}
	if len(org.Workspaces) == 0 {
		return fmt.Errorf("org %q workspaces map is required", name)
	}

	for _, workspaceName := range sortedWorkspaceNames(org.Workspaces) {
		if err := validateWorkspace(name, workspaceName, org.Workspaces[workspaceName]); err != nil {
			return err
		}
	}

	defaultWorkspace := strings.TrimSpace(org.DefaultWorkspace)
	if defaultWorkspace != "" {
		if _, ok := org.Workspaces[defaultWorkspace]; !ok {
			return fmt.Errorf("org %q default_workspace %q does not exist", name, defaultWorkspace)
		}
	}
	return nil
}

func validateWorkspace(orgName, workspaceName string, workspace Workspace) error {
	workspaceName = strings.TrimSpace(workspaceName)
	if workspaceName == "" {
		return fmt.Errorf("org %q workspace name cannot be empty", orgName)
	}
	if strings.TrimSpace(workspace.ID) == "" {
		return fmt.Errorf("org %q workspace %q id is required", orgName, workspaceName)
	}
	return nil
}

func sortedOrgNames(orgs map[string]Org) []string {
	names := make([]string, 0, len(orgs))
	for name := range orgs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedWorkspaceNames(workspaces map[string]Workspace) []string {
	names := make([]string, 0, len(workspaces))
	for name := range workspaces {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func detectLegacyConfigPayload(data []byte) (bool, error) {
	payload := map[string]any{}
	if err := yaml.Unmarshal(data, &payload); err != nil {
		return false, err
	}
	if _, exists := payload["profiles"]; exists {
		return true, nil
	}
	if _, exists := payload["default_profile"]; exists {
		return true, nil
	}
	return false, nil
}

func legacyCutoverCommandHint(path string) string {
	return fmt.Sprintf(
		"meta enterprise mode cutover --legacy-config %s --config %s --org <org> --org-id <org-id> --workspace <workspace> --workspace-id <workspace-id> --principal <principal>",
		path,
		path,
	)
}
