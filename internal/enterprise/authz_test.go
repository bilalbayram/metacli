package enterprise

import (
	"errors"
	"strings"
	"testing"
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
