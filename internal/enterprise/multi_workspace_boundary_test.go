package enterprise

import (
	"errors"
	"fmt"
	"testing"
)

func TestMultiWorkspaceBoundaryHarnessEnforcesWorkspaceScopedBindings(t *testing.T) {
	t.Parallel()

	harness := newMultiWorkspaceBoundaryHarness()

	allowTrace, err := harness.execute(
		"alice",
		"api get",
		"agency",
		"alpha",
		[]SecretExecutionRequirement{{Secret: "alpha_api_token", Action: SecretActionRead}},
	)
	if err != nil {
		t.Fatalf("expected alpha workspace execution to allow: %v", err)
	}
	if !allowTrace.Authorization.Allowed {
		t.Fatal("expected alpha authorization allow")
	}
	if len(allowTrace.Authorization.MatchedBindings) != 1 {
		t.Fatalf("unexpected matched bindings for allow trace: %d", len(allowTrace.Authorization.MatchedBindings))
	}
	if allowTrace.Authorization.MatchedBindings[0].WorkspaceName != "alpha" {
		t.Fatalf("unexpected binding workspace: %q", allowTrace.Authorization.MatchedBindings[0].WorkspaceName)
	}

	denyTrace, err := harness.execute(
		"alice",
		"api get",
		"agency",
		"bravo",
		[]SecretExecutionRequirement{{Secret: "bravo_api_token", Action: SecretActionRead}},
	)
	if err == nil {
		t.Fatal("expected cross-workspace execution to deny")
	}
	if !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected ErrAuthorizationDenied, got %v", err)
	}
	expectedReason := `principal "alice" has no role binding in "agency"/"bravo"`
	if denyTrace.Authorization.DenyReason != expectedReason {
		t.Fatalf("unexpected deny reason:\nwant: %q\ngot:  %q", expectedReason, denyTrace.Authorization.DenyReason)
	}
	if len(denyTrace.Authorization.MatchedBindings) != 0 {
		t.Fatalf("expected no matched bindings in denied workspace, got %d", len(denyTrace.Authorization.MatchedBindings))
	}
}

func TestMultiWorkspaceBoundaryHarnessPreventsSecretScopeLeakage(t *testing.T) {
	t.Parallel()

	harness := newMultiWorkspaceBoundaryHarness()
	expectedSecretDenyReason := `secret "bravo_api_token" is scoped to "agency"/"bravo" not "agency"/"alpha"`

	for run := 0; run < 2; run++ {
		trace, err := harness.execute(
			"alice",
			"api get",
			"agency",
			"alpha",
			[]SecretExecutionRequirement{{Secret: "bravo_api_token", Action: SecretActionRead}},
		)
		if err == nil {
			t.Fatalf("run %d: expected secret scope mismatch denial", run)
		}
		if !errors.Is(err, ErrAuthorizationDenied) {
			t.Fatalf("run %d: expected ErrAuthorizationDenied, got %v", run, err)
		}
		if len(trace.SecretAccess) != 1 {
			t.Fatalf("run %d: unexpected secret access trace count: %d", run, len(trace.SecretAccess))
		}
		if trace.SecretAccess[0].DenyReason != expectedSecretDenyReason {
			t.Fatalf(
				"run %d: unexpected secret deny reason:\nwant: %q\ngot:  %q",
				run,
				expectedSecretDenyReason,
				trace.SecretAccess[0].DenyReason,
			)
		}
		if trace.Execution.Status != ExecutionStatusFailed {
			t.Fatalf("run %d: unexpected execution status: %q", run, trace.Execution.Status)
		}
	}
}

func TestMultiWorkspaceBoundaryHarnessKeepsDenyPoliciesWorkspaceLocal(t *testing.T) {
	t.Parallel()

	harness := newMultiWorkspaceBoundaryHarness()

	alphaTrace, err := harness.execute("eve", "api get", "agency", "alpha", nil)
	if err != nil {
		t.Fatalf("expected alpha workspace allow for eve: %v", err)
	}
	if !alphaTrace.Authorization.Allowed {
		t.Fatal("expected alpha authorization allow for eve")
	}
	if len(alphaTrace.Authorization.MatchedBindings) != 1 {
		t.Fatalf("unexpected alpha matched bindings count: %d", len(alphaTrace.Authorization.MatchedBindings))
	}
	if alphaTrace.Authorization.MatchedBindings[0].Role != "reader" {
		t.Fatalf("unexpected alpha role binding: %q", alphaTrace.Authorization.MatchedBindings[0].Role)
	}

	bravoTrace, err := harness.execute("eve", "api get", "agency", "bravo", nil)
	if err == nil {
		t.Fatal("expected bravo workspace deny for eve")
	}
	if !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected ErrAuthorizationDenied, got %v", err)
	}
	expectedReason := `principal "eve" is explicitly denied capability "graph.read" in "agency"/"bravo"`
	if bravoTrace.Authorization.DenyReason != expectedReason {
		t.Fatalf("unexpected deny reason:\nwant: %q\ngot:  %q", expectedReason, bravoTrace.Authorization.DenyReason)
	}
	if len(bravoTrace.Authorization.MatchedBindings) != 1 {
		t.Fatalf("unexpected bravo matched bindings count: %d", len(bravoTrace.Authorization.MatchedBindings))
	}
	if bravoTrace.Authorization.MatchedBindings[0].Role != "blocked" {
		t.Fatalf("unexpected bravo role binding: %q", bravoTrace.Authorization.MatchedBindings[0].Role)
	}
}

type multiWorkspaceBoundaryHarness struct {
	config          *Config
	correlationSeed int
}

func newMultiWorkspaceBoundaryHarness() *multiWorkspaceBoundaryHarness {
	return &multiWorkspaceBoundaryHarness{
		config: &Config{
			SchemaVersion: SchemaVersion,
			DefaultOrg:    "agency",
			Orgs: map[string]Org{
				"agency": {
					ID:               "org_agency_001",
					DefaultWorkspace: "alpha",
					Workspaces: map[string]Workspace{
						"alpha": {ID: "ws_alpha_001"},
						"bravo": {ID: "ws_bravo_001"},
					},
				},
			},
			Roles: map[string]Role{
				"reader": {
					Capabilities: []string{"graph.read"},
				},
				"blocked": {
					DenyCapabilities: []string{"graph.read"},
				},
			},
			Bindings: []Binding{
				{
					Principal: "alice",
					Role:      "reader",
					Org:       "agency",
					Workspace: "alpha",
				},
				{
					Principal: "bob",
					Role:      "reader",
					Org:       "agency",
					Workspace: "bravo",
				},
				{
					Principal: "eve",
					Role:      "reader",
					Org:       "agency",
					Workspace: "alpha",
				},
				{
					Principal: "eve",
					Role:      "blocked",
					Org:       "agency",
					Workspace: "bravo",
				},
			},
			SecretGovernance: SecretGovernance{
				Secrets: map[string]GovernedSecret{
					"alpha_api_token": {
						Scope: SecretScope{
							Org:       "agency",
							Workspace: "alpha",
						},
						Ownership: SecretOwnershipMetadata{
							OwnerPrincipal: "security.alpha",
							OwnerTeam:      "agency-security",
						},
					},
					"bravo_api_token": {
						Scope: SecretScope{
							Org:       "agency",
							Workspace: "bravo",
						},
						Ownership: SecretOwnershipMetadata{
							OwnerPrincipal: "security.bravo",
							OwnerTeam:      "agency-security",
						},
					},
				},
				Policies: []SecretAccessPolicy{
					{
						Principal: "alice",
						Secret:    "alpha_api_token",
						Actions:   []string{SecretActionRead},
						Org:       "agency",
						Workspace: "alpha",
					},
					{
						Principal: "bob",
						Secret:    "bravo_api_token",
						Actions:   []string{SecretActionRead},
						Org:       "agency",
						Workspace: "bravo",
					},
				},
			},
		},
	}
}

func (h *multiWorkspaceBoundaryHarness) execute(
	principal string,
	command string,
	orgName string,
	workspaceName string,
	requiredSecrets []SecretExecutionRequirement,
) (CommandExecutionTrace, error) {
	h.correlationSeed++
	correlationID := fmt.Sprintf("boundary-%03d", h.correlationSeed)
	return h.config.ExecuteCommand(CommandExecutionRequest{
		Principal:       principal,
		Command:         command,
		OrgName:         orgName,
		WorkspaceName:   workspaceName,
		CorrelationID:   correlationID,
		AuditPipeline:   NewAuditPipeline(),
		RequiredSecrets: requiredSecrets,
		Execute: func(CommandExecutionContext) error {
			return nil
		},
	})
}
