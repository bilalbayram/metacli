package enterprise

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	legacyconfig "github.com/bilalbayram/metacli/internal/config"
)

const cutoverBootstrapRole = "legacy-cutover-operator"

type ModeCutoverRequest struct {
	LegacyConfigPath     string
	EnterpriseConfigPath string
	OrgName              string
	OrgID                string
	WorkspaceName        string
	WorkspaceID          string
	Principal            string
	Force                bool
}

type ModeCutoverResult struct {
	LegacyConfigPath     string   `json:"legacy_config_path"`
	EnterpriseConfigPath string   `json:"enterprise_config_path"`
	OrgName              string   `json:"org_name"`
	OrgID                string   `json:"org_id"`
	WorkspaceName        string   `json:"workspace_name"`
	WorkspaceID          string   `json:"workspace_id"`
	BootstrapRole        string   `json:"bootstrap_role"`
	BootstrapPrincipal   string   `json:"bootstrap_principal"`
	MigratedProfiles     []string `json:"migrated_profiles"`
}

func CutoverLegacyConfig(request ModeCutoverRequest) (ModeCutoverResult, error) {
	legacyConfigPath := strings.TrimSpace(request.LegacyConfigPath)
	if legacyConfigPath == "" {
		return ModeCutoverResult{}, errors.New("legacy_config_path is required")
	}
	enterpriseConfigPath := strings.TrimSpace(request.EnterpriseConfigPath)
	if enterpriseConfigPath == "" {
		return ModeCutoverResult{}, errors.New("enterprise_config_path is required")
	}
	orgName := strings.TrimSpace(request.OrgName)
	if orgName == "" {
		return ModeCutoverResult{}, errors.New("org_name is required")
	}
	orgID := strings.TrimSpace(request.OrgID)
	if orgID == "" {
		return ModeCutoverResult{}, errors.New("org_id is required")
	}
	workspaceName := strings.TrimSpace(request.WorkspaceName)
	if workspaceName == "" {
		return ModeCutoverResult{}, errors.New("workspace_name is required")
	}
	workspaceID := strings.TrimSpace(request.WorkspaceID)
	if workspaceID == "" {
		return ModeCutoverResult{}, errors.New("workspace_id is required")
	}
	principal := strings.TrimSpace(request.Principal)
	if principal == "" {
		return ModeCutoverResult{}, errors.New("principal is required")
	}

	if !request.Force {
		if _, err := os.Stat(enterpriseConfigPath); err == nil {
			return ModeCutoverResult{}, fmt.Errorf(
				"enterprise config already exists at %s; rerun cutover with force to overwrite",
				enterpriseConfigPath,
			)
		} else if !errors.Is(err, os.ErrNotExist) {
			return ModeCutoverResult{}, fmt.Errorf("stat enterprise config %s: %w", enterpriseConfigPath, err)
		}
	}

	legacyCfg, err := legacyconfig.Load(legacyConfigPath)
	if err != nil {
		return ModeCutoverResult{}, fmt.Errorf("load legacy config %s: %w", legacyConfigPath, err)
	}
	if len(legacyCfg.Profiles) == 0 {
		return ModeCutoverResult{}, errors.New("legacy config profiles map is required for cutover")
	}

	enterpriseCfg := &Config{
		SchemaVersion: SchemaVersion,
		Mode:          EnterpriseMode,
		DefaultOrg:    orgName,
		Orgs: map[string]Org{
			orgName: {
				ID:               orgID,
				DefaultWorkspace: workspaceName,
				Workspaces: map[string]Workspace{
					workspaceName: {ID: workspaceID},
				},
			},
		},
		Roles: map[string]Role{
			cutoverBootstrapRole: {
				Capabilities: allKnownCapabilities(),
			},
		},
		Bindings: []Binding{
			{
				Principal: principal,
				Role:      cutoverBootstrapRole,
				Org:       orgName,
				Workspace: workspaceName,
			},
		},
	}

	if err := Save(enterpriseConfigPath, enterpriseCfg); err != nil {
		return ModeCutoverResult{}, err
	}

	return ModeCutoverResult{
		LegacyConfigPath:     legacyConfigPath,
		EnterpriseConfigPath: enterpriseConfigPath,
		OrgName:              orgName,
		OrgID:                orgID,
		WorkspaceName:        workspaceName,
		WorkspaceID:          workspaceID,
		BootstrapRole:        cutoverBootstrapRole,
		BootstrapPrincipal:   principal,
		MigratedProfiles:     sortedLegacyProfileNames(legacyCfg.Profiles),
	}, nil
}

func allKnownCapabilities() []string {
	seen := map[string]struct{}{}
	capabilities := make([]string, 0, len(commandCapabilityByReference))
	for _, rawCapability := range commandCapabilityByReference {
		capability := strings.TrimSpace(rawCapability)
		if capability == "" {
			continue
		}
		if _, exists := seen[capability]; exists {
			continue
		}
		seen[capability] = struct{}{}
		capabilities = append(capabilities, capability)
	}
	sort.Strings(capabilities)
	return capabilities
}

func sortedLegacyProfileNames(profiles map[string]legacyconfig.Profile) []string {
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
