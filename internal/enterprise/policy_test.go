package enterprise

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestEvaluatePolicyDenyPrecedence(t *testing.T) {
	t.Parallel()

	cfg := validAuthzConfig()
	cfg.Roles["blocked"] = Role{
		DenyCapabilities: []string{"graph.read"},
	}
	cfg.Bindings = []Binding{
		{
			Principal: "alice",
			Role:      "reader",
			Org:       "acme",
			Workspace: "prod",
		},
		{
			Principal: "alice",
			Role:      "blocked",
			Org:       "acme",
			Workspace: "prod",
		},
	}

	trace, err := cfg.EvaluatePolicy(PolicyEvaluationRequest{
		Principal:     "alice",
		Capability:    "graph.read",
		OrgName:       "acme",
		WorkspaceName: "prod",
	})
	if err == nil {
		t.Fatal("expected deny error when deny capability matches")
	}
	if !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected ErrAuthorizationDenied, got %v", err)
	}
	if trace.Allowed {
		t.Fatal("expected deny result when deny capability matches")
	}
	if !strings.Contains(trace.DenyReason, "explicitly denied capability") {
		t.Fatalf("unexpected deny reason: %q", trace.DenyReason)
	}
	if len(trace.MatchedBindings) != 2 {
		t.Fatalf("unexpected matched bindings: %d", len(trace.MatchedBindings))
	}
	if !trace.MatchedBindings[0].GrantsRequiredCapability {
		t.Fatal("expected reader binding to grant required capability")
	}
	if trace.MatchedBindings[0].DeniesRequiredCapability {
		t.Fatal("reader binding should not deny required capability")
	}
	if trace.MatchedBindings[1].GrantsRequiredCapability {
		t.Fatal("blocked binding should not grant required capability")
	}
	if !trace.MatchedBindings[1].DeniesRequiredCapability {
		t.Fatal("expected blocked binding to deny required capability")
	}
}

func TestEvaluatePolicyDecisionTraceDeterministic(t *testing.T) {
	t.Parallel()

	cfg := validAuthzConfig()
	cfg.Roles["blocked"] = Role{
		DenyCapabilities: []string{"graph.read"},
	}
	cfg.Bindings = []Binding{
		{
			Principal: "alice",
			Role:      "reader",
			Org:       "acme",
			Workspace: "prod",
		},
		{
			Principal: "alice",
			Role:      "blocked",
			Org:       "acme",
			Workspace: "prod",
		},
	}

	firstTrace, _ := cfg.EvaluatePolicy(PolicyEvaluationRequest{
		Principal:     "alice",
		Capability:    "graph.read",
		OrgName:       "acme",
		WorkspaceName: "prod",
	})
	secondTrace, _ := cfg.EvaluatePolicy(PolicyEvaluationRequest{
		Principal:     "alice",
		Capability:    "graph.read",
		OrgName:       "acme",
		WorkspaceName: "prod",
	})

	if !reflect.DeepEqual(firstTrace.DecisionTrace, secondTrace.DecisionTrace) {
		t.Fatalf("decision traces differ between runs\nfirst=%+v\nsecond=%+v", firstTrace.DecisionTrace, secondTrace.DecisionTrace)
	}
	if len(firstTrace.DecisionTrace) != 4 {
		t.Fatalf("unexpected decision trace length: %d", len(firstTrace.DecisionTrace))
	}
	if firstTrace.DecisionTrace[0].Effect != PolicyEffectDeny || firstTrace.DecisionTrace[0].Matched {
		t.Fatalf("unexpected decision step[0]: %+v", firstTrace.DecisionTrace[0])
	}
	if firstTrace.DecisionTrace[1].Effect != PolicyEffectAllow || !firstTrace.DecisionTrace[1].Matched {
		t.Fatalf("unexpected decision step[1]: %+v", firstTrace.DecisionTrace[1])
	}
	if firstTrace.DecisionTrace[2].Effect != PolicyEffectDeny || !firstTrace.DecisionTrace[2].Matched {
		t.Fatalf("unexpected decision step[2]: %+v", firstTrace.DecisionTrace[2])
	}
	if firstTrace.DecisionTrace[3].Effect != PolicyEffectAllow || firstTrace.DecisionTrace[3].Matched {
		t.Fatalf("unexpected decision step[3]: %+v", firstTrace.DecisionTrace[3])
	}
}
