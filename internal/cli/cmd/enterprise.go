package cmd

import (
	"fmt"
	"strings"

	"github.com/bilalbayram/metacli/internal/enterprise"
	"github.com/spf13/cobra"
)

func NewEnterpriseCommand(runtime Runtime) *cobra.Command {
	enterpriseCmd := &cobra.Command{
		Use:   "enterprise",
		Short: "Enterprise org/workspace context commands",
	}
	enterpriseCmd.AddCommand(newEnterpriseContextCommand(runtime))
	enterpriseCmd.AddCommand(newEnterpriseAuthzCommand(runtime))
	return enterpriseCmd
}

func newEnterpriseContextCommand(runtime Runtime) *cobra.Command {
	var (
		configPath string
		orgName    string
		workspace  string
	)

	cmd := &cobra.Command{
		Use:   "context",
		Short: "Resolve enterprise workspace context",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolvedOrg, resolvedWorkspace, err := resolveWorkspaceSelection(orgName, workspace)
			if err != nil {
				return err
			}
			if configPath == "" {
				configPath, err = enterprise.DefaultPath()
				if err != nil {
					return err
				}
			}

			cfg, err := enterprise.Load(configPath)
			if err != nil {
				return err
			}
			ctx, err := cfg.ResolveWorkspace(resolvedOrg, resolvedWorkspace)
			if err != nil {
				return err
			}
			return writeSuccess(cmd, runtime, "meta enterprise context", ctx, nil, nil)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "Path to enterprise config file")
	cmd.Flags().StringVar(&orgName, "org", "", "Enterprise org name")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace name or org/workspace")
	return cmd
}

func resolveWorkspaceSelection(orgName, workspace string) (string, string, error) {
	orgName = strings.TrimSpace(orgName)
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return orgName, "", nil
	}
	if !strings.Contains(workspace, "/") {
		return orgName, workspace, nil
	}

	parts := strings.Split(workspace, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid --workspace value %q; expected <workspace> or <org>/<workspace>", workspace)
	}

	refOrg := strings.TrimSpace(parts[0])
	refWorkspace := strings.TrimSpace(parts[1])
	if refOrg == "" || refWorkspace == "" {
		return "", "", fmt.Errorf("invalid --workspace value %q; expected <workspace> or <org>/<workspace>", workspace)
	}
	if orgName != "" && orgName != refOrg {
		return "", "", fmt.Errorf("workspace reference %q conflicts with --org %q", workspace, orgName)
	}
	return refOrg, refWorkspace, nil
}

func newEnterpriseAuthzCommand(runtime Runtime) *cobra.Command {
	authzCmd := &cobra.Command{
		Use:   "authz",
		Short: "Enterprise authorization checks",
	}
	authzCmd.AddCommand(newEnterpriseAuthzCheckCommand(runtime))
	return authzCmd
}

func newEnterpriseAuthzCheckCommand(runtime Runtime) *cobra.Command {
	var (
		configPath string
		principal  string
		commandRef string
		orgName    string
		workspace  string
	)

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Evaluate command authorization in an enterprise workspace",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolvedOrg, resolvedWorkspace, err := resolveWorkspaceSelection(orgName, workspace)
			if err != nil {
				return err
			}
			if configPath == "" {
				configPath, err = enterprise.DefaultPath()
				if err != nil {
					return err
				}
			}

			cfg, err := enterprise.Load(configPath)
			if err != nil {
				return err
			}

			trace, err := cfg.AuthorizeCommand(enterprise.CommandAuthorizationRequest{
				Principal:     principal,
				Command:       commandRef,
				OrgName:       resolvedOrg,
				WorkspaceName: resolvedWorkspace,
			})
			if err != nil {
				return err
			}
			return writeSuccess(cmd, runtime, "meta enterprise authz check", trace, nil, nil)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "Path to enterprise config file")
	cmd.Flags().StringVar(&principal, "principal", "", "Principal identity to evaluate")
	cmd.Flags().StringVar(&commandRef, "command", "", "Command reference to authorize (for example \"api get\")")
	cmd.Flags().StringVar(&orgName, "org", "", "Enterprise org name")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace name or org/workspace")
	mustMarkFlagRequired(cmd, "principal")
	mustMarkFlagRequired(cmd, "command")
	return cmd
}
