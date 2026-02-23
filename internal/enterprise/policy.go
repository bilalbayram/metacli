package enterprise

import (
	"errors"
	"fmt"
	"strings"
)

const (
	PolicyEffectDeny  = "deny"
	PolicyEffectAllow = "allow"
)

type PolicyEvaluationRequest struct {
	Principal     string
	Capability    string
	OrgName       string
	WorkspaceName string
}

type PolicyEvaluationTrace struct {
	Principal       string                 `json:"principal"`
	Capability      string                 `json:"capability"`
	OrgName         string                 `json:"org_name"`
	OrgID           string                 `json:"org_id"`
	WorkspaceName   string                 `json:"workspace_name"`
	WorkspaceID     string                 `json:"workspace_id"`
	MatchedBindings []BindingAuthorization `json:"matched_bindings"`
	DecisionTrace   []PolicyDecision       `json:"decision_trace"`
	Allowed         bool                   `json:"allowed"`
	DenyReason      string                 `json:"deny_reason,omitempty"`
}

type PolicyDecision struct {
	Step         int    `json:"step"`
	BindingIndex int    `json:"binding_index"`
	Role         string `json:"role"`
	Effect       string `json:"effect"`
	Capability   string `json:"capability"`
	Matched      bool   `json:"matched"`
}

func (c *Config) EvaluatePolicy(request PolicyEvaluationRequest) (PolicyEvaluationTrace, error) {
	trace := PolicyEvaluationTrace{
		MatchedBindings: make([]BindingAuthorization, 0),
		DecisionTrace:   make([]PolicyDecision, 0),
	}
	if c == nil {
		return trace, errors.New("enterprise config is nil")
	}
	if err := c.Validate(); err != nil {
		return trace, err
	}

	principal := strings.TrimSpace(request.Principal)
	if principal == "" {
		return trace, errors.New("principal is required")
	}
	capability := strings.TrimSpace(request.Capability)
	if capability == "" {
		return trace, errors.New("capability is required")
	}

	workspaceContext, err := c.ResolveWorkspace(request.OrgName, request.WorkspaceName)
	if err != nil {
		return trace, err
	}

	trace = PolicyEvaluationTrace{
		Principal:       principal,
		Capability:      capability,
		OrgName:         workspaceContext.OrgName,
		OrgID:           workspaceContext.OrgID,
		WorkspaceName:   workspaceContext.WorkspaceName,
		WorkspaceID:     workspaceContext.WorkspaceID,
		MatchedBindings: make([]BindingAuthorization, 0),
		DecisionTrace:   make([]PolicyDecision, 0),
	}

	allowed := false
	denied := false
	step := 0
	for index, binding := range c.Bindings {
		bindingPrincipal := strings.TrimSpace(binding.Principal)
		bindingRole := strings.TrimSpace(binding.Role)
		bindingOrg := strings.TrimSpace(binding.Org)
		bindingWorkspace := strings.TrimSpace(binding.Workspace)
		if bindingPrincipal != principal || bindingOrg != workspaceContext.OrgName || bindingWorkspace != workspaceContext.WorkspaceName {
			continue
		}

		role := c.Roles[bindingRole]
		allowCapabilities := uniqueSortedCapabilities(role.Capabilities)
		denyCapabilities := uniqueSortedCapabilities(role.DenyCapabilities)
		grantsRequiredCapability := containsCapability(allowCapabilities, capability)
		deniesRequiredCapability := containsCapability(denyCapabilities, capability)

		trace.MatchedBindings = append(trace.MatchedBindings, BindingAuthorization{
			Index:                    index,
			Principal:                bindingPrincipal,
			Role:                     bindingRole,
			OrgName:                  bindingOrg,
			WorkspaceName:            bindingWorkspace,
			Capabilities:             allowCapabilities,
			DenyCapabilities:         denyCapabilities,
			GrantsRequiredCapability: grantsRequiredCapability,
			DeniesRequiredCapability: deniesRequiredCapability,
		})

		trace.DecisionTrace = append(trace.DecisionTrace, PolicyDecision{
			Step:         step,
			BindingIndex: index,
			Role:         bindingRole,
			Effect:       PolicyEffectDeny,
			Capability:   capability,
			Matched:      deniesRequiredCapability,
		})
		step++
		trace.DecisionTrace = append(trace.DecisionTrace, PolicyDecision{
			Step:         step,
			BindingIndex: index,
			Role:         bindingRole,
			Effect:       PolicyEffectAllow,
			Capability:   capability,
			Matched:      grantsRequiredCapability,
		})
		step++

		if grantsRequiredCapability {
			allowed = true
		}
		if deniesRequiredCapability {
			denied = true
		}
	}

	if denied {
		trace.DenyReason = fmt.Sprintf(
			"principal %q is explicitly denied capability %q in %q/%q",
			principal,
			capability,
			workspaceContext.OrgName,
			workspaceContext.WorkspaceName,
		)
		return trace, &DenyError{
			Principal:     principal,
			Capability:    capability,
			OrgName:       workspaceContext.OrgName,
			WorkspaceName: workspaceContext.WorkspaceName,
			Reason:        trace.DenyReason,
		}
	}

	if allowed {
		trace.Allowed = true
		return trace, nil
	}

	if len(trace.MatchedBindings) == 0 {
		trace.DenyReason = fmt.Sprintf(
			"principal %q has no role binding in %q/%q",
			principal,
			workspaceContext.OrgName,
			workspaceContext.WorkspaceName,
		)
	} else {
		trace.DenyReason = fmt.Sprintf(
			"principal %q is missing capability %q in %q/%q",
			principal,
			capability,
			workspaceContext.OrgName,
			workspaceContext.WorkspaceName,
		)
	}
	return trace, &DenyError{
		Principal:     principal,
		Capability:    capability,
		OrgName:       workspaceContext.OrgName,
		WorkspaceName: workspaceContext.WorkspaceName,
		Reason:        trace.DenyReason,
	}
}
