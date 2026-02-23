package enterprise

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAuthorizeCommandAllowsBoundCapability(t *testing.T) {
	t.Parallel()

	cfg := validAuthzConfig()
	trace, err := cfg.AuthorizeCommand(CommandAuthorizationRequest{
		Principal:     "alice",
		Command:       "meta api get",
		OrgName:       "acme",
		WorkspaceName: "prod",
	})
	if err != nil {
		t.Fatalf("authorize command: %v", err)
	}
	if !trace.Allowed {
		t.Fatal("expected authorization to allow")
	}
	if trace.RequiredCapability != "graph.read" {
		t.Fatalf("unexpected required capability: %q", trace.RequiredCapability)
	}
	if len(trace.MatchedBindings) != 1 {
		t.Fatalf("unexpected matched bindings: %d", len(trace.MatchedBindings))
	}
	if !trace.MatchedBindings[0].GrantsRequiredCapability {
		t.Fatal("expected matched binding to grant capability")
	}
	if len(trace.DecisionTrace) != 2 {
		t.Fatalf("unexpected decision trace length: %d", len(trace.DecisionTrace))
	}
	if trace.DecisionTrace[0].Effect != PolicyEffectDeny || trace.DecisionTrace[0].Matched {
		t.Fatalf("unexpected decision trace[0]: %+v", trace.DecisionTrace[0])
	}
	if trace.DecisionTrace[1].Effect != PolicyEffectAllow || !trace.DecisionTrace[1].Matched {
		t.Fatalf("unexpected decision trace[1]: %+v", trace.DecisionTrace[1])
	}
}

func TestAuthorizeCommandRequiresApprovalForHighRiskCommand(t *testing.T) {
	t.Parallel()

	cfg := validApprovalAuthzConfig()

	trace, err := cfg.AuthorizeCommand(CommandAuthorizationRequest{
		Principal:     "alice",
		Command:       "auth rotate",
		OrgName:       "acme",
		WorkspaceName: "prod",
	})
	if err == nil {
		t.Fatal("expected deny error when approval token is missing")
	}
	if !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected ErrAuthorizationDenied, got %v", err)
	}
	if trace.Allowed {
		t.Fatal("expected denied trace when approval is missing")
	}
	if !trace.ApprovalRequired {
		t.Fatal("expected approval_required=true")
	}
	if trace.ApprovalStatus != ApprovalStatusRequired {
		t.Fatalf("unexpected approval status: %q", trace.ApprovalStatus)
	}
	if !strings.Contains(trace.DenyReason, "approval token is required") {
		t.Fatalf("unexpected deny reason: %q", trace.DenyReason)
	}
}

func TestAuthorizeCommandAllowsHighRiskCommandWithApprovedGrant(t *testing.T) {
	t.Parallel()

	cfg := validApprovalAuthzConfig()
	approvalToken := mustIssueApprovalGrant(
		t,
		ApprovalDecisionApproved,
		time.Now().UTC(),
		30*time.Minute,
		30*time.Minute,
	)

	trace, err := cfg.AuthorizeCommand(CommandAuthorizationRequest{
		Principal:     "alice",
		Command:       "auth rotate",
		OrgName:       "acme",
		WorkspaceName: "prod",
		ApprovalToken: approvalToken,
	})
	if err != nil {
		t.Fatalf("authorize command: %v", err)
	}
	if !trace.Allowed {
		t.Fatal("expected authorization to allow with approved grant")
	}
	if !trace.ApprovalRequired {
		t.Fatal("expected approval_required=true")
	}
	if trace.ApprovalStatus != ApprovalStatusApproved {
		t.Fatalf("unexpected approval status: %q", trace.ApprovalStatus)
	}
	if trace.ApprovalDecision != ApprovalDecisionApproved {
		t.Fatalf("unexpected approval decision: %q", trace.ApprovalDecision)
	}
	if strings.TrimSpace(trace.ApprovalExpiresAt) == "" {
		t.Fatal("expected approval expiry metadata")
	}
}

func TestAuthorizeCommandDeniesHighRiskCommandWithRejectedGrant(t *testing.T) {
	t.Parallel()

	cfg := validApprovalAuthzConfig()
	approvalToken := mustIssueApprovalGrant(
		t,
		ApprovalDecisionRejected,
		time.Now().UTC(),
		30*time.Minute,
		30*time.Minute,
	)

	trace, err := cfg.AuthorizeCommand(CommandAuthorizationRequest{
		Principal:     "alice",
		Command:       "auth rotate",
		OrgName:       "acme",
		WorkspaceName: "prod",
		ApprovalToken: approvalToken,
	})
	if err == nil {
		t.Fatal("expected deny error for rejected grant")
	}
	if !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected ErrAuthorizationDenied, got %v", err)
	}
	if trace.Allowed {
		t.Fatal("expected denied trace")
	}
	if trace.ApprovalStatus != ApprovalStatusRejected {
		t.Fatalf("unexpected approval status: %q", trace.ApprovalStatus)
	}
	if !strings.Contains(trace.DenyReason, "rejected") {
		t.Fatalf("unexpected deny reason: %q", trace.DenyReason)
	}
}

func TestAuthorizeCommandDeniesHighRiskCommandWithExpiredGrant(t *testing.T) {
	t.Parallel()

	cfg := validApprovalAuthzConfig()
	approvalToken := mustIssueApprovalGrant(
		t,
		ApprovalDecisionApproved,
		time.Now().UTC().Add(-2*time.Hour),
		90*time.Minute,
		30*time.Minute,
	)

	trace, err := cfg.AuthorizeCommand(CommandAuthorizationRequest{
		Principal:     "alice",
		Command:       "auth rotate",
		OrgName:       "acme",
		WorkspaceName: "prod",
		ApprovalToken: approvalToken,
	})
	if err == nil {
		t.Fatal("expected deny error for expired grant")
	}
	if !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected ErrAuthorizationDenied, got %v", err)
	}
	if trace.Allowed {
		t.Fatal("expected denied trace")
	}
	if trace.ApprovalStatus != ApprovalStatusExpired {
		t.Fatalf("unexpected approval status: %q", trace.ApprovalStatus)
	}
	if !strings.Contains(trace.DenyReason, "expired") {
		t.Fatalf("unexpected deny reason: %q", trace.DenyReason)
	}
}

func TestAuthorizeCommandDeniesWhenWorkspaceBindingMissing(t *testing.T) {
	t.Parallel()

	cfg := validAuthzConfig()
	trace, err := cfg.AuthorizeCommand(CommandAuthorizationRequest{
		Principal:     "bob",
		Command:       "api get",
		OrgName:       "acme",
		WorkspaceName: "growth",
	})
	if err == nil {
		t.Fatal("expected deny error when workspace binding is missing")
	}
	if !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected ErrAuthorizationDenied, got %v", err)
	}
	var denyErr *DenyError
	if !errors.As(err, &denyErr) {
		t.Fatalf("expected DenyError, got %T", err)
	}
	if trace.Allowed {
		t.Fatal("expected denied trace")
	}
	if !strings.Contains(trace.DenyReason, "no role binding") {
		t.Fatalf("unexpected deny reason: %q", trace.DenyReason)
	}
}

func TestAuthorizeCommandDeniesWhenCapabilityMissing(t *testing.T) {
	t.Parallel()

	cfg := validAuthzConfig()
	trace, err := cfg.AuthorizeCommand(CommandAuthorizationRequest{
		Principal:     "alice",
		Command:       "api post",
		OrgName:       "acme",
		WorkspaceName: "prod",
	})
	if err == nil {
		t.Fatal("expected deny error when capability is missing")
	}
	if !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected ErrAuthorizationDenied, got %v", err)
	}
	if trace.Allowed {
		t.Fatal("expected denied trace")
	}
	if len(trace.MatchedBindings) != 1 {
		t.Fatalf("unexpected matched bindings: %d", len(trace.MatchedBindings))
	}
	if trace.MatchedBindings[0].GrantsRequiredCapability {
		t.Fatal("binding should not grant missing capability")
	}
	if len(trace.DecisionTrace) != 2 {
		t.Fatalf("unexpected decision trace length: %d", len(trace.DecisionTrace))
	}
	if trace.DecisionTrace[0].Effect != PolicyEffectDeny || trace.DecisionTrace[0].Matched {
		t.Fatalf("unexpected decision trace[0]: %+v", trace.DecisionTrace[0])
	}
	if trace.DecisionTrace[1].Effect != PolicyEffectAllow || trace.DecisionTrace[1].Matched {
		t.Fatalf("unexpected decision trace[1]: %+v", trace.DecisionTrace[1])
	}
	if !strings.Contains(trace.DenyReason, "missing capability") {
		t.Fatalf("unexpected deny reason: %q", trace.DenyReason)
	}
}

func TestAuthorizeCommandDeniesWhenCapabilityExplicitlyDenied(t *testing.T) {
	t.Parallel()

	cfg := validAuthzConfig()
	cfg.Roles["blocked"] = Role{
		DenyCapabilities: []string{"graph.read"},
	}
	cfg.Bindings = append(cfg.Bindings, Binding{
		Principal: "alice",
		Role:      "blocked",
		Org:       "acme",
		Workspace: "prod",
	})

	trace, err := cfg.AuthorizeCommand(CommandAuthorizationRequest{
		Principal:     "alice",
		Command:       "api get",
		OrgName:       "acme",
		WorkspaceName: "prod",
	})
	if err == nil {
		t.Fatal("expected deny error when capability is explicitly denied")
	}
	if !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected ErrAuthorizationDenied, got %v", err)
	}
	if trace.Allowed {
		t.Fatal("expected denied trace")
	}
	if !strings.Contains(trace.DenyReason, "explicitly denied capability") {
		t.Fatalf("unexpected deny reason: %q", trace.DenyReason)
	}
	if len(trace.MatchedBindings) != 2 {
		t.Fatalf("unexpected matched bindings: %d", len(trace.MatchedBindings))
	}
	if !trace.MatchedBindings[1].DeniesRequiredCapability {
		t.Fatal("expected deny binding to deny required capability")
	}
}

func TestAuthorizeCommandDeniesUnmappedCommand(t *testing.T) {
	t.Parallel()

	cfg := validAuthzConfig()
	trace, err := cfg.AuthorizeCommand(CommandAuthorizationRequest{
		Principal:     "alice",
		Command:       "api run-custom",
		OrgName:       "acme",
		WorkspaceName: "prod",
	})
	if err == nil {
		t.Fatal("expected deny error for unmapped command")
	}
	if !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected ErrAuthorizationDenied, got %v", err)
	}
	if trace.Allowed {
		t.Fatal("expected denied trace")
	}
	if !strings.Contains(trace.DenyReason, "not mapped to a capability") {
		t.Fatalf("unexpected deny reason: %q", trace.DenyReason)
	}
}

func TestAuthorizeCommandRecordsDecisionAuditEvent(t *testing.T) {
	t.Parallel()

	cfg := validAuthzConfig()
	pipeline := NewAuditPipeline()

	trace, err := cfg.AuthorizeCommand(CommandAuthorizationRequest{
		Principal:     "alice",
		Command:       "meta api get",
		OrgName:       "acme",
		WorkspaceName: "prod",
		CorrelationID: "corr-allow-001",
		AuditPipeline: pipeline,
	})
	if err != nil {
		t.Fatalf("authorize command: %v", err)
	}
	if trace.CorrelationID != "corr-allow-001" {
		t.Fatalf("unexpected correlation id: %q", trace.CorrelationID)
	}
	if len(trace.AuditEvents) != 1 {
		t.Fatalf("unexpected audit event count: %d", len(trace.AuditEvents))
	}
	if trace.AuditEvents[0].EventType != AuditEventTypeDecision {
		t.Fatalf("unexpected audit event type: %q", trace.AuditEvents[0].EventType)
	}
	if trace.AuditEvents[0].Allowed == nil || !*trace.AuditEvents[0].Allowed {
		t.Fatalf("unexpected allowed state: %v", trace.AuditEvents[0].Allowed)
	}
	events := pipeline.Events()
	if len(events) != 1 {
		t.Fatalf("unexpected pipeline event count: %d", len(events))
	}
}

func TestAuthorizeCommandRecordsDeniedDecisionAuditEvent(t *testing.T) {
	t.Parallel()

	cfg := validAuthzConfig()
	pipeline := NewAuditPipeline()

	trace, err := cfg.AuthorizeCommand(CommandAuthorizationRequest{
		Principal:     "alice",
		Command:       "api post",
		OrgName:       "acme",
		WorkspaceName: "prod",
		CorrelationID: "corr-deny-001",
		AuditPipeline: pipeline,
	})
	if err == nil {
		t.Fatal("expected deny error")
	}
	if !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected ErrAuthorizationDenied, got %v", err)
	}
	if len(trace.AuditEvents) != 1 {
		t.Fatalf("unexpected audit event count: %d", len(trace.AuditEvents))
	}
	if trace.AuditEvents[0].Allowed == nil || *trace.AuditEvents[0].Allowed {
		t.Fatalf("expected denied decision allowed=false, got %v", trace.AuditEvents[0].Allowed)
	}
	if strings.TrimSpace(trace.AuditEvents[0].DenyReason) == "" {
		t.Fatal("expected deny reason in audit event")
	}
}

func TestAuthorizeCommandFailsWhenAuditPipelineConfiguredWithoutCorrelationID(t *testing.T) {
	t.Parallel()

	cfg := validAuthzConfig()

	_, err := cfg.AuthorizeCommand(CommandAuthorizationRequest{
		Principal:     "alice",
		Command:       "api get",
		OrgName:       "acme",
		WorkspaceName: "prod",
		AuditPipeline: NewAuditPipeline(),
	})
	if err == nil {
		t.Fatal("expected correlation id validation error")
	}
	if !strings.Contains(err.Error(), "correlation_id is required when audit pipeline is configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateFailsWhenBindingRoleDoesNotExist(t *testing.T) {
	t.Parallel()

	cfg := validAuthzConfig()
	cfg.Bindings = []Binding{
		{
			Principal: "alice",
			Role:      "missing-role",
			Org:       "acme",
			Workspace: "prod",
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validate failure for missing role")
	}
	if !strings.Contains(err.Error(), "binding[0] role \"missing-role\" does not exist") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateFailsWhenBindingWorkspaceDoesNotExist(t *testing.T) {
	t.Parallel()

	cfg := validAuthzConfig()
	cfg.Bindings = []Binding{
		{
			Principal: "alice",
			Role:      "reader",
			Org:       "acme",
			Workspace: "missing",
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validate failure for missing workspace")
	}
	if !strings.Contains(err.Error(), "binding[0] workspace \"missing\" does not exist in org \"acme\"") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func validAuthzConfig() *Config {
	return &Config{
		SchemaVersion: SchemaVersion,
		DefaultOrg:    "acme",
		Orgs: map[string]Org{
			"acme": {
				ID:               "org_1",
				DefaultWorkspace: "prod",
				Workspaces: map[string]Workspace{
					"prod":   {ID: "ws_prod"},
					"growth": {ID: "ws_growth"},
				},
			},
		},
		Roles: map[string]Role{
			"reader": {
				Capabilities: []string{"graph.read", "enterprise.workspace.read"},
			},
			"writer": {
				Capabilities: []string{"graph.write"},
			},
		},
		Bindings: []Binding{
			{
				Principal: "alice",
				Role:      "reader",
				Org:       "acme",
				Workspace: "prod",
			},
			{
				Principal: "alice",
				Role:      "writer",
				Org:       "acme",
				Workspace: "growth",
			},
		},
	}
}

func validApprovalAuthzConfig() *Config {
	cfg := validAuthzConfig()
	cfg.Roles["rotator"] = Role{
		Capabilities: []string{"auth.rotate"},
	}
	cfg.Bindings = append(cfg.Bindings, Binding{
		Principal: "alice",
		Role:      "rotator",
		Org:       "acme",
		Workspace: "prod",
	})
	return cfg
}

func mustIssueApprovalGrant(
	t *testing.T,
	decision string,
	now time.Time,
	requestTTL time.Duration,
	grantTTL time.Duration,
) string {
	t.Helper()

	requestToken, err := CreateApprovalRequestToken(ApprovalRequestTokenRequest{
		Principal:     "alice",
		Command:       "auth rotate",
		OrgName:       "acme",
		WorkspaceName: "prod",
		TTL:           requestTTL,
		Now: func() time.Time {
			return now
		},
	})
	if err != nil {
		t.Fatalf("create approval request token: %v", err)
	}
	grantToken, err := CreateApprovalGrantToken(ApprovalGrantTokenRequest{
		RequestToken: requestToken.RequestToken,
		Approver:     "security.lead",
		Decision:     decision,
		TTL:          grantTTL,
		Now: func() time.Time {
			return now
		},
	})
	if err != nil {
		t.Fatalf("create approval grant token: %v", err)
	}
	return grantToken.GrantToken
}
