package enterprise

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateFailsWhenSecretOwnershipOwnerMissing(t *testing.T) {
	t.Parallel()

	cfg := validSecretGovernanceConfig()
	cfg.SecretGovernance.Secrets["ads_token"] = GovernedSecret{
		Scope: SecretScope{
			Org:       "acme",
			Workspace: "prod",
		},
		Ownership: SecretOwnershipMetadata{},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validate failure for missing owner_principal")
	}
	if !strings.Contains(err.Error(), "ownership.owner_principal is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEvaluateSecretAccessAllowsOwnerBaseline(t *testing.T) {
	t.Parallel()

	cfg := validSecretGovernanceConfig()

	trace, err := cfg.EvaluateSecretAccess(SecretAccessRequest{
		Principal:     "alice",
		Secret:        "ads_token",
		Action:        SecretActionWrite,
		OrgName:       "acme",
		WorkspaceName: "prod",
	})
	if err != nil {
		t.Fatalf("evaluate secret access: %v", err)
	}
	if !trace.Allowed {
		t.Fatal("expected owner baseline access to allow")
	}
	if !trace.ScopeMatched {
		t.Fatal("expected scope matched")
	}
	if !trace.OwnerMatched {
		t.Fatal("expected owner matched")
	}
}

func TestEvaluateSecretAccessAllowsPolicyGrant(t *testing.T) {
	t.Parallel()

	cfg := validSecretGovernanceConfig()

	trace, err := cfg.EvaluateSecretAccess(SecretAccessRequest{
		Principal:     "bob",
		Secret:        "ads_token",
		Action:        SecretActionRead,
		OrgName:       "acme",
		WorkspaceName: "prod",
	})
	if err != nil {
		t.Fatalf("evaluate secret access: %v", err)
	}
	if !trace.Allowed {
		t.Fatal("expected policy grant access to allow")
	}
	if trace.OwnerMatched {
		t.Fatal("expected non-owner policy grant")
	}
	if len(trace.MatchedPolicies) == 0 {
		t.Fatal("expected matched policy trace")
	}
	if !trace.MatchedPolicies[0].GrantsAction {
		t.Fatalf("expected policy trace grant, got %+v", trace.MatchedPolicies[0])
	}
}

func TestEvaluateSecretAccessDeniesScopeMismatch(t *testing.T) {
	t.Parallel()

	cfg := validSecretGovernanceConfig()

	trace, err := cfg.EvaluateSecretAccess(SecretAccessRequest{
		Principal:     "alice",
		Secret:        "ads_token",
		Action:        SecretActionRead,
		OrgName:       "acme",
		WorkspaceName: "growth",
	})
	if err == nil {
		t.Fatal("expected scope mismatch deny")
	}
	if !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected ErrAuthorizationDenied, got %v", err)
	}
	if trace.Allowed {
		t.Fatal("expected denied trace")
	}
	if !strings.Contains(trace.DenyReason, "is scoped to") {
		t.Fatalf("unexpected deny reason: %q", trace.DenyReason)
	}
}

func TestEvaluateSecretAccessDeniesWhenPolicyActionMissing(t *testing.T) {
	t.Parallel()

	cfg := validSecretGovernanceConfig()

	trace, err := cfg.EvaluateSecretAccess(SecretAccessRequest{
		Principal:     "bob",
		Secret:        "ads_token",
		Action:        SecretActionWrite,
		OrgName:       "acme",
		WorkspaceName: "prod",
	})
	if err == nil {
		t.Fatal("expected deny when policy action is missing")
	}
	if !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected ErrAuthorizationDenied, got %v", err)
	}
	if trace.Allowed {
		t.Fatal("expected denied trace")
	}
	if !strings.Contains(trace.DenyReason, "not allowed to write secret") {
		t.Fatalf("unexpected deny reason: %q", trace.DenyReason)
	}
}

func TestEvaluateSecretAccessHookDenies(t *testing.T) {
	t.Parallel()

	cfg := validSecretGovernanceConfig()

	trace, err := cfg.EvaluateSecretAccess(SecretAccessRequest{
		Principal:     "bob",
		Secret:        "ads_token",
		Action:        SecretActionRead,
		OrgName:       "acme",
		WorkspaceName: "prod",
		EnforcementHooks: []SecretPolicyEnforcementHook{
			SecretPolicyEnforcementHookFunc(func(SecretAccessTrace) error {
				return errors.New("change-ticket required")
			}),
		},
	})
	if err == nil {
		t.Fatal("expected hook deny")
	}
	if !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected ErrAuthorizationDenied, got %v", err)
	}
	if trace.Allowed {
		t.Fatal("expected denied trace after hook failure")
	}
	if len(trace.HookDecisions) != 1 {
		t.Fatalf("expected one hook decision, got %d", len(trace.HookDecisions))
	}
	if trace.HookDecisions[0].Allowed {
		t.Fatalf("expected hook denial, got %+v", trace.HookDecisions[0])
	}
	if !strings.Contains(trace.DenyReason, "policy enforcement hook[0] denied access") {
		t.Fatalf("unexpected deny reason: %q", trace.DenyReason)
	}
}

func TestEvaluateSecretAccessFailsOnNilHook(t *testing.T) {
	t.Parallel()

	cfg := validSecretGovernanceConfig()

	trace, err := cfg.EvaluateSecretAccess(SecretAccessRequest{
		Principal:     "bob",
		Secret:        "ads_token",
		Action:        SecretActionRead,
		OrgName:       "acme",
		WorkspaceName: "prod",
		EnforcementHooks: []SecretPolicyEnforcementHook{
			nil,
		},
	})
	if err == nil {
		t.Fatal("expected nil hook failure")
	}
	if !strings.Contains(err.Error(), "policy enforcement hook[0] is nil") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !trace.Allowed {
		t.Fatal("expected trace to be marked allowed before hook execution")
	}
}

func validSecretGovernanceConfig() *Config {
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
		SecretGovernance: SecretGovernance{
			Secrets: map[string]GovernedSecret{
				"ads_token": {
					Scope: SecretScope{
						Org:       "acme",
						Workspace: "prod",
					},
					Ownership: SecretOwnershipMetadata{
						OwnerPrincipal: "alice",
						OwnerTeam:      "marketing-platform",
						Steward:        "security-bot",
					},
					Metadata: map[string]string{
						"classification": "high",
					},
				},
			},
			Policies: []SecretAccessPolicy{
				{
					Principal: "bob",
					Secret:    "ads_token",
					Actions:   []string{SecretActionRead},
					Org:       "acme",
					Workspace: "prod",
				},
			},
		},
	}
}
