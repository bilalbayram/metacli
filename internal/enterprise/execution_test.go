package enterprise

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestExecuteCommandRunsFullFailClosedPipeline(t *testing.T) {
	t.Parallel()

	cfg := validExecutionPipelineConfig()
	approvalToken := mustIssueApprovalGrant(
		t,
		ApprovalDecisionApproved,
		time.Now().UTC(),
		30*time.Minute,
		30*time.Minute,
	)

	executed := false
	trace, err := cfg.ExecuteCommand(CommandExecutionRequest{
		Principal:     "alice",
		Command:       "auth rotate",
		OrgName:       "acme",
		WorkspaceName: "prod",
		ApprovalToken: approvalToken,
		CorrelationID: "exec-full-001",
		AuditPipeline: NewAuditPipeline(),
		RequiredSecrets: []SecretExecutionRequirement{
			{Secret: "auth_rotation_key", Action: SecretActionRotate},
		},
		Execute: func(CommandExecutionContext) error {
			executed = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("execute command: %v", err)
	}
	if !executed {
		t.Fatal("expected execute callback to run")
	}
	if !trace.Authorization.Allowed {
		t.Fatal("expected authorization allow")
	}
	if trace.Authorization.ApprovalStatus != ApprovalStatusApproved {
		t.Fatalf("unexpected approval status: %q", trace.Authorization.ApprovalStatus)
	}
	if len(trace.SecretAccess) != 1 {
		t.Fatalf("unexpected secret access trace count: %d", len(trace.SecretAccess))
	}
	if !trace.SecretAccess[0].Allowed {
		t.Fatalf("expected secret access to allow, got %+v", trace.SecretAccess[0])
	}
	if trace.Execution.Status != ExecutionStatusSucceeded {
		t.Fatalf("unexpected execution status: %q", trace.Execution.Status)
	}
	if len(trace.AuditEvents) != 2 {
		t.Fatalf("unexpected audit event count: %d", len(trace.AuditEvents))
	}
	if trace.AuditEvents[0].EventType != AuditEventTypeDecision {
		t.Fatalf("unexpected decision event type: %q", trace.AuditEvents[0].EventType)
	}
	if trace.AuditEvents[1].EventType != AuditEventTypeExecution {
		t.Fatalf("unexpected execution event type: %q", trace.AuditEvents[1].EventType)
	}
	if trace.AuditEvents[1].ExecutionStatus != ExecutionStatusSucceeded {
		t.Fatalf("unexpected execution audit status: %q", trace.AuditEvents[1].ExecutionStatus)
	}
}

func TestExecuteCommandFailsClosedWhenGovernanceHookErrors(t *testing.T) {
	t.Parallel()

	cfg := validExecutionPipelineConfig()
	approvalToken := mustIssueApprovalGrant(
		t,
		ApprovalDecisionApproved,
		time.Now().UTC(),
		30*time.Minute,
		30*time.Minute,
	)

	executed := false
	trace, err := cfg.ExecuteCommand(CommandExecutionRequest{
		Principal:     "alice",
		Command:       "auth rotate",
		OrgName:       "acme",
		WorkspaceName: "prod",
		ApprovalToken: approvalToken,
		CorrelationID: "exec-hook-001",
		AuditPipeline: NewAuditPipeline(),
		RequiredSecrets: []SecretExecutionRequirement{
			{Secret: "auth_rotation_key", Action: SecretActionRotate},
		},
		SecretEnforcementHooks: []SecretPolicyEnforcementHook{
			nil,
		},
		Execute: func(CommandExecutionContext) error {
			executed = true
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected governance hook failure")
	}
	if !strings.Contains(err.Error(), "policy enforcement hook[0] is nil") {
		t.Fatalf("unexpected error: %v", err)
	}
	if executed {
		t.Fatal("execute callback should not run on governance failure")
	}
	if trace.Execution.Status != ExecutionStatusFailed {
		t.Fatalf("unexpected execution status: %q", trace.Execution.Status)
	}
	if !strings.Contains(trace.Execution.FailureReason, "policy enforcement hook[0] is nil") {
		t.Fatalf("unexpected execution failure reason: %q", trace.Execution.FailureReason)
	}
	if len(trace.AuditEvents) != 2 {
		t.Fatalf("unexpected audit event count: %d", len(trace.AuditEvents))
	}
	if trace.AuditEvents[1].ExecutionStatus != ExecutionStatusFailed {
		t.Fatalf("unexpected execution audit status: %q", trace.AuditEvents[1].ExecutionStatus)
	}
}

func TestExecuteCommandStopsWhenAuthorizationDenied(t *testing.T) {
	t.Parallel()

	cfg := validExecutionPipelineConfig()
	executed := false

	trace, err := cfg.ExecuteCommand(CommandExecutionRequest{
		Principal:     "alice",
		Command:       "auth rotate",
		OrgName:       "acme",
		WorkspaceName: "prod",
		CorrelationID: "exec-denied-001",
		AuditPipeline: NewAuditPipeline(),
		RequiredSecrets: []SecretExecutionRequirement{
			{Secret: "auth_rotation_key", Action: SecretActionRotate},
		},
		Execute: func(CommandExecutionContext) error {
			executed = true
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected authorization denial")
	}
	if !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected ErrAuthorizationDenied, got %v", err)
	}
	if executed {
		t.Fatal("execute callback should not run when authorization is denied")
	}
	if len(trace.SecretAccess) != 0 {
		t.Fatalf("unexpected secret access traces: %d", len(trace.SecretAccess))
	}
	if trace.Execution.Status != "" {
		t.Fatalf("expected no execution status, got %q", trace.Execution.Status)
	}
	if len(trace.AuditEvents) != 1 {
		t.Fatalf("unexpected audit event count: %d", len(trace.AuditEvents))
	}
}

func TestExecuteCommandRequiresAuditPipeline(t *testing.T) {
	t.Parallel()

	cfg := validExecutionPipelineConfig()

	_, err := cfg.ExecuteCommand(CommandExecutionRequest{
		Principal:     "alice",
		Command:       "api get",
		OrgName:       "acme",
		WorkspaceName: "prod",
		Execute: func(CommandExecutionContext) error {
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected missing audit pipeline error")
	}
	if !strings.Contains(err.Error(), "audit pipeline is required for enterprise execution") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteCommandRecordsExecutionFailureFromExecutor(t *testing.T) {
	t.Parallel()

	cfg := validExecutionPipelineConfig()
	approvalToken := mustIssueApprovalGrant(
		t,
		ApprovalDecisionApproved,
		time.Now().UTC(),
		30*time.Minute,
		30*time.Minute,
	)

	trace, err := cfg.ExecuteCommand(CommandExecutionRequest{
		Principal:     "alice",
		Command:       "auth rotate",
		OrgName:       "acme",
		WorkspaceName: "prod",
		ApprovalToken: approvalToken,
		CorrelationID: "exec-failed-001",
		AuditPipeline: NewAuditPipeline(),
		RequiredSecrets: []SecretExecutionRequirement{
			{Secret: "auth_rotation_key", Action: SecretActionRotate},
		},
		Execute: func(CommandExecutionContext) error {
			return errors.New("downstream execution timeout")
		},
	})
	if err == nil {
		t.Fatal("expected execution error")
	}
	if !strings.Contains(err.Error(), "downstream execution timeout") {
		t.Fatalf("unexpected error: %v", err)
	}
	if trace.Execution.Status != ExecutionStatusFailed {
		t.Fatalf("unexpected execution status: %q", trace.Execution.Status)
	}
	if trace.Execution.FailureReason != "downstream execution timeout" {
		t.Fatalf("unexpected execution failure reason: %q", trace.Execution.FailureReason)
	}
	if len(trace.AuditEvents) != 2 {
		t.Fatalf("unexpected audit event count: %d", len(trace.AuditEvents))
	}
	if trace.AuditEvents[1].ExecutionStatus != ExecutionStatusFailed {
		t.Fatalf("unexpected execution audit status: %q", trace.AuditEvents[1].ExecutionStatus)
	}
	if trace.AuditEvents[1].ExecutionError != "downstream execution timeout" {
		t.Fatalf("unexpected execution audit error: %q", trace.AuditEvents[1].ExecutionError)
	}
}

func validExecutionPipelineConfig() *Config {
	cfg := validApprovalAuthzConfig()
	cfg.SecretGovernance = SecretGovernance{
		Secrets: map[string]GovernedSecret{
			"auth_rotation_key": {
				Scope: SecretScope{
					Org:       "acme",
					Workspace: "prod",
				},
				Ownership: SecretOwnershipMetadata{
					OwnerPrincipal: "security.owner",
					OwnerTeam:      "security",
					Steward:        "security.bot",
				},
			},
		},
		Policies: []SecretAccessPolicy{
			{
				Principal: "alice",
				Secret:    "auth_rotation_key",
				Actions:   []string{SecretActionRotate},
				Org:       "acme",
				Workspace: "prod",
			},
		},
	}
	return cfg
}
