package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

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
	enterpriseCmd.AddCommand(newEnterpriseApprovalCommand(runtime))
	enterpriseCmd.AddCommand(newEnterprisePolicyCommand(runtime))
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
		configPath      string
		principal       string
		commandRef      string
		orgName         string
		workspace       string
		approvalToken   string
		correlationID   string
		executionStatus string
		executionError  string
	)

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Evaluate command authorization in an enterprise workspace",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(executionError) != "" && strings.TrimSpace(executionStatus) == "" {
				return errors.New("execution status is required when execution error is provided")
			}

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

			var auditPipeline *enterprise.AuditPipeline
			if strings.TrimSpace(correlationID) != "" || strings.TrimSpace(executionStatus) != "" {
				auditPipeline = enterprise.NewAuditPipeline()
			}

			trace, err := cfg.AuthorizeCommand(enterprise.CommandAuthorizationRequest{
				Principal:     principal,
				Command:       commandRef,
				OrgName:       resolvedOrg,
				WorkspaceName: resolvedWorkspace,
				ApprovalToken: approvalToken,
				CorrelationID: correlationID,
				AuditPipeline: auditPipeline,
			})
			if err != nil {
				return err
			}

			if strings.TrimSpace(executionStatus) != "" {
				executionEvent, err := auditPipeline.RecordExecution(enterprise.ExecutionAuditRecord{
					CorrelationID: correlationID,
					Principal:     trace.Principal,
					Command:       trace.NormalizedCommand,
					Capability:    trace.RequiredCapability,
					OrgName:       trace.OrgName,
					WorkspaceName: trace.WorkspaceName,
					Status:        executionStatus,
					FailureReason: executionError,
				})
				if err != nil {
					return err
				}
				trace.AuditEvents = append(trace.AuditEvents, executionEvent)
			}
			return writeSuccess(cmd, runtime, "meta enterprise authz check", trace, nil, nil)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "Path to enterprise config file")
	cmd.Flags().StringVar(&principal, "principal", "", "Principal identity to evaluate")
	cmd.Flags().StringVar(&commandRef, "command", "", "Command reference to authorize (for example \"api get\")")
	cmd.Flags().StringVar(&orgName, "org", "", "Enterprise org name")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace name or org/workspace")
	cmd.Flags().StringVar(&approvalToken, "approval-token", "", "Approval grant token for high-risk commands")
	cmd.Flags().StringVar(&correlationID, "correlation-id", "", "Correlation id for immutable decision/execution audit events")
	cmd.Flags().StringVar(&executionStatus, "execution-status", "", "Execution status to record (succeeded|failed)")
	cmd.Flags().StringVar(&executionError, "execution-error", "", "Execution failure reason (required when --execution-status=failed)")
	mustMarkFlagRequired(cmd, "principal")
	mustMarkFlagRequired(cmd, "command")
	return cmd
}

func newEnterpriseApprovalCommand(runtime Runtime) *cobra.Command {
	approvalCmd := &cobra.Command{
		Use:   "approval",
		Short: "Enterprise approval grants for high-risk commands",
	}
	approvalCmd.AddCommand(newEnterpriseApprovalRequestCommand(runtime))
	approvalCmd.AddCommand(newEnterpriseApprovalApproveCommand(runtime))
	approvalCmd.AddCommand(newEnterpriseApprovalValidateCommand(runtime))
	return approvalCmd
}

func newEnterpriseApprovalRequestCommand(runtime Runtime) *cobra.Command {
	var (
		configPath string
		principal  string
		commandRef string
		orgName    string
		workspace  string
		ttl        string
	)

	cmd := &cobra.Command{
		Use:   "request",
		Short: "Create an approval request token for a command",
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
			workspaceContext, err := cfg.ResolveWorkspace(resolvedOrg, resolvedWorkspace)
			if err != nil {
				return err
			}
			requestTTL, err := parseEnterpriseApprovalTTL(ttl)
			if err != nil {
				return err
			}

			token, err := enterprise.CreateApprovalRequestToken(enterprise.ApprovalRequestTokenRequest{
				Principal:     principal,
				Command:       commandRef,
				OrgName:       workspaceContext.OrgName,
				WorkspaceName: workspaceContext.WorkspaceName,
				TTL:           requestTTL,
			})
			if err != nil {
				return err
			}
			return writeSuccess(cmd, runtime, "meta enterprise approval request", token, nil, nil)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "Path to enterprise config file")
	cmd.Flags().StringVar(&principal, "principal", "", "Principal requesting approval")
	cmd.Flags().StringVar(&commandRef, "command", "", "Command reference to approve (for example \"auth rotate\")")
	cmd.Flags().StringVar(&orgName, "org", "", "Enterprise org name")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace name or org/workspace")
	cmd.Flags().StringVar(&ttl, "ttl", "15m", "Request token TTL (for example 15m, 1h)")
	mustMarkFlagRequired(cmd, "principal")
	mustMarkFlagRequired(cmd, "command")
	return cmd
}

func newEnterpriseApprovalApproveCommand(runtime Runtime) *cobra.Command {
	var (
		requestToken string
		approver     string
		decision     string
		ttl          string
	)

	cmd := &cobra.Command{
		Use:   "approve",
		Short: "Approve or reject an approval request token",
		RunE: func(cmd *cobra.Command, _ []string) error {
			grantTTL, err := parseEnterpriseApprovalTTL(ttl)
			if err != nil {
				return err
			}

			token, err := enterprise.CreateApprovalGrantToken(enterprise.ApprovalGrantTokenRequest{
				RequestToken: requestToken,
				Approver:     approver,
				Decision:     decision,
				TTL:          grantTTL,
			})
			if err != nil {
				return err
			}
			return writeSuccess(cmd, runtime, "meta enterprise approval approve", token, nil, nil)
		},
	}

	cmd.Flags().StringVar(&requestToken, "request-token", "", "Approval request token")
	cmd.Flags().StringVar(&approver, "approver", "", "Approver principal")
	cmd.Flags().StringVar(&decision, "decision", "", "Decision (approved|rejected)")
	cmd.Flags().StringVar(&ttl, "ttl", "15m", "Grant token TTL (for example 15m, 1h)")
	mustMarkFlagRequired(cmd, "request-token")
	mustMarkFlagRequired(cmd, "approver")
	mustMarkFlagRequired(cmd, "decision")
	return cmd
}

func newEnterpriseApprovalValidateCommand(runtime Runtime) *cobra.Command {
	var (
		configPath string
		grantToken string
		principal  string
		commandRef string
		orgName    string
		workspace  string
	)

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate an approval grant token against command context",
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
			workspaceContext, err := cfg.ResolveWorkspace(resolvedOrg, resolvedWorkspace)
			if err != nil {
				return err
			}

			result, err := enterprise.ValidateApprovalGrantToken(enterprise.ApprovalGrantValidationRequest{
				GrantToken:    grantToken,
				Principal:     principal,
				Command:       commandRef,
				OrgName:       workspaceContext.OrgName,
				WorkspaceName: workspaceContext.WorkspaceName,
			})
			if err != nil {
				return err
			}
			return writeSuccess(cmd, runtime, "meta enterprise approval validate", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "Path to enterprise config file")
	cmd.Flags().StringVar(&grantToken, "grant-token", "", "Approval grant token")
	cmd.Flags().StringVar(&principal, "principal", "", "Principal executing the command")
	cmd.Flags().StringVar(&commandRef, "command", "", "Command reference to validate")
	cmd.Flags().StringVar(&orgName, "org", "", "Enterprise org name")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace name or org/workspace")
	mustMarkFlagRequired(cmd, "grant-token")
	mustMarkFlagRequired(cmd, "principal")
	mustMarkFlagRequired(cmd, "command")
	return cmd
}

func parseEnterpriseApprovalTTL(raw string) (time.Duration, error) {
	ttlValue := strings.TrimSpace(raw)
	if ttlValue == "" {
		return 0, errors.New("ttl is required")
	}
	ttl, err := time.ParseDuration(ttlValue)
	if err != nil {
		return 0, fmt.Errorf("parse ttl %q: %w", raw, err)
	}
	if ttl <= 0 {
		return 0, errors.New("ttl must be greater than zero")
	}
	return ttl, nil
}

func newEnterprisePolicyCommand(runtime Runtime) *cobra.Command {
	policyCmd := &cobra.Command{
		Use:   "policy",
		Short: "Enterprise policy evaluation",
	}
	policyCmd.AddCommand(newEnterprisePolicyEvalCommand(runtime))
	return policyCmd
}

func newEnterprisePolicyEvalCommand(runtime Runtime) *cobra.Command {
	var (
		configPath string
		principal  string
		capability string
		orgName    string
		workspace  string
	)

	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Evaluate enterprise policy for a capability",
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

			trace, err := cfg.EvaluatePolicy(enterprise.PolicyEvaluationRequest{
				Principal:     principal,
				Capability:    capability,
				OrgName:       resolvedOrg,
				WorkspaceName: resolvedWorkspace,
			})
			if err != nil {
				return err
			}
			return writeSuccess(cmd, runtime, "meta enterprise policy eval", trace, nil, nil)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "Path to enterprise config file")
	cmd.Flags().StringVar(&principal, "principal", "", "Principal identity to evaluate")
	cmd.Flags().StringVar(&capability, "capability", "", "Capability to evaluate (for example \"graph.read\")")
	cmd.Flags().StringVar(&orgName, "org", "", "Enterprise org name")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace name or org/workspace")
	mustMarkFlagRequired(cmd, "principal")
	mustMarkFlagRequired(cmd, "capability")
	return cmd
}
