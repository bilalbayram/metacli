package enterprise

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	SecretActionRead   = "read"
	SecretActionWrite  = "write"
	SecretActionRotate = "rotate"
)

type SecretGovernance struct {
	Secrets  map[string]GovernedSecret `yaml:"secrets,omitempty"`
	Policies []SecretAccessPolicy      `yaml:"policies,omitempty"`
}

type GovernedSecret struct {
	Scope     SecretScope             `yaml:"scope"`
	Ownership SecretOwnershipMetadata `yaml:"ownership"`
	Metadata  map[string]string       `yaml:"metadata,omitempty"`
}

type SecretScope struct {
	Org       string `yaml:"org"`
	Workspace string `yaml:"workspace"`
}

type SecretOwnershipMetadata struct {
	OwnerPrincipal string `yaml:"owner_principal"`
	OwnerTeam      string `yaml:"owner_team,omitempty"`
	Steward        string `yaml:"steward,omitempty"`
}

type SecretAccessPolicy struct {
	Principal string   `yaml:"principal"`
	Secret    string   `yaml:"secret"`
	Actions   []string `yaml:"actions"`
	Org       string   `yaml:"org,omitempty"`
	Workspace string   `yaml:"workspace,omitempty"`
}

type SecretAccessRequest struct {
	Principal        string
	Secret           string
	Action           string
	OrgName          string
	WorkspaceName    string
	EnforcementHooks []SecretPolicyEnforcementHook
}

type SecretPolicyEnforcementHook interface {
	Enforce(trace SecretAccessTrace) error
}

type SecretPolicyEnforcementHookFunc func(trace SecretAccessTrace) error

func (fn SecretPolicyEnforcementHookFunc) Enforce(trace SecretAccessTrace) error {
	if fn == nil {
		return errors.New("secret policy enforcement hook function is nil")
	}
	return fn(trace)
}

type SecretAccessTrace struct {
	Principal       string                      `json:"principal"`
	Secret          string                      `json:"secret"`
	Action          string                      `json:"action"`
	OrgName         string                      `json:"org_name"`
	OrgID           string                      `json:"org_id"`
	WorkspaceName   string                      `json:"workspace_name"`
	WorkspaceID     string                      `json:"workspace_id"`
	ScopeMatched    bool                        `json:"scope_matched"`
	Ownership       SecretOwnershipMetadata     `json:"ownership"`
	OwnerMatched    bool                        `json:"owner_matched"`
	MatchedPolicies []SecretPolicyDecisionTrace `json:"matched_policies"`
	HookDecisions   []SecretHookDecisionTrace   `json:"hook_decisions"`
	Allowed         bool                        `json:"allowed"`
	DenyReason      string                      `json:"deny_reason,omitempty"`
}

type SecretPolicyDecisionTrace struct {
	Index            int      `json:"index"`
	Principal        string   `json:"principal"`
	Secret           string   `json:"secret"`
	Actions          []string `json:"actions"`
	Org              string   `json:"org,omitempty"`
	Workspace        string   `json:"workspace,omitempty"`
	PrincipalMatched bool     `json:"principal_matched"`
	ScopeMatched     bool     `json:"scope_matched"`
	GrantsAction     bool     `json:"grants_action"`
}

type SecretHookDecisionTrace struct {
	Index   int    `json:"index"`
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

func validateSecretGovernance(orgs map[string]Org, governance SecretGovernance) error {
	if len(governance.Secrets) == 0 && len(governance.Policies) == 0 {
		return nil
	}
	if len(governance.Secrets) == 0 {
		return errors.New("secret_governance.secrets map is required when policies are defined")
	}

	for _, secretName := range sortedSecretNames(governance.Secrets) {
		if err := validateGovernedSecret(secretName, governance.Secrets[secretName], orgs); err != nil {
			return err
		}
	}
	for index, policy := range governance.Policies {
		if err := validateSecretPolicy(index, policy, governance.Secrets, orgs); err != nil {
			return err
		}
	}
	return nil
}

func validateGovernedSecret(secretName string, secret GovernedSecret, orgs map[string]Org) error {
	secretName = strings.TrimSpace(secretName)
	if secretName == "" {
		return errors.New("secret_governance secret name cannot be empty")
	}

	scopeOrg := strings.TrimSpace(secret.Scope.Org)
	scopeWorkspace := strings.TrimSpace(secret.Scope.Workspace)
	if scopeOrg == "" || scopeWorkspace == "" {
		return fmt.Errorf("secret_governance.secrets[%q] scope org/workspace are required", secretName)
	}
	org, ok := orgs[scopeOrg]
	if !ok {
		return fmt.Errorf("secret_governance.secrets[%q] scope org %q does not exist", secretName, scopeOrg)
	}
	if _, ok := org.Workspaces[scopeWorkspace]; !ok {
		return fmt.Errorf(
			"secret_governance.secrets[%q] scope workspace %q does not exist in org %q",
			secretName,
			scopeWorkspace,
			scopeOrg,
		)
	}

	ownerPrincipal := strings.TrimSpace(secret.Ownership.OwnerPrincipal)
	if ownerPrincipal == "" {
		return fmt.Errorf("secret_governance.secrets[%q] ownership.owner_principal is required", secretName)
	}
	for key := range secret.Metadata {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("secret_governance.secrets[%q] metadata key cannot be empty", secretName)
		}
	}
	return nil
}

func validateSecretPolicy(
	index int,
	policy SecretAccessPolicy,
	secrets map[string]GovernedSecret,
	orgs map[string]Org,
) error {
	principal := strings.TrimSpace(policy.Principal)
	if principal == "" {
		return fmt.Errorf("secret_governance.policies[%d] principal is required", index)
	}

	secretName := strings.TrimSpace(policy.Secret)
	if secretName == "" {
		return fmt.Errorf("secret_governance.policies[%d] secret is required", index)
	}
	secret, ok := secrets[secretName]
	if !ok {
		return fmt.Errorf("secret_governance.policies[%d] secret %q does not exist", index, secretName)
	}

	actions, err := normalizeSecretActions(policy.Actions)
	if err != nil {
		return fmt.Errorf("secret_governance.policies[%d] %w", index, err)
	}
	if len(actions) == 0 {
		return fmt.Errorf("secret_governance.policies[%d] actions are required", index)
	}

	policyOrg := strings.TrimSpace(policy.Org)
	policyWorkspace := strings.TrimSpace(policy.Workspace)
	if policyOrg == "" && policyWorkspace == "" {
		return nil
	}
	if policyOrg == "" || policyWorkspace == "" {
		return fmt.Errorf("secret_governance.policies[%d] org and workspace must be provided together", index)
	}

	org, ok := orgs[policyOrg]
	if !ok {
		return fmt.Errorf("secret_governance.policies[%d] org %q does not exist", index, policyOrg)
	}
	if _, ok := org.Workspaces[policyWorkspace]; !ok {
		return fmt.Errorf(
			"secret_governance.policies[%d] workspace %q does not exist in org %q",
			index,
			policyWorkspace,
			policyOrg,
		)
	}
	if policyOrg != strings.TrimSpace(secret.Scope.Org) || policyWorkspace != strings.TrimSpace(secret.Scope.Workspace) {
		return fmt.Errorf(
			"secret_governance.policies[%d] scope %q/%q does not match secret %q scope %q/%q",
			index,
			policyOrg,
			policyWorkspace,
			secretName,
			strings.TrimSpace(secret.Scope.Org),
			strings.TrimSpace(secret.Scope.Workspace),
		)
	}
	return nil
}

func (c *Config) EvaluateSecretAccess(request SecretAccessRequest) (SecretAccessTrace, error) {
	trace := SecretAccessTrace{
		MatchedPolicies: make([]SecretPolicyDecisionTrace, 0),
		HookDecisions:   make([]SecretHookDecisionTrace, 0),
	}
	if c == nil {
		return trace, errors.New("enterprise config is nil")
	}
	if err := c.Validate(); err != nil {
		return trace, err
	}
	if len(c.SecretGovernance.Secrets) == 0 {
		return trace, errors.New("secret governance is not configured")
	}

	principal := strings.TrimSpace(request.Principal)
	if principal == "" {
		return trace, errors.New("principal is required")
	}
	secretName := strings.TrimSpace(request.Secret)
	if secretName == "" {
		return trace, errors.New("secret is required")
	}
	action, err := normalizeSecretAction(request.Action)
	if err != nil {
		return trace, err
	}

	workspaceContext, err := c.ResolveWorkspace(request.OrgName, request.WorkspaceName)
	if err != nil {
		return trace, err
	}

	trace = SecretAccessTrace{
		Principal:       principal,
		Secret:          secretName,
		Action:          action,
		OrgName:         workspaceContext.OrgName,
		OrgID:           workspaceContext.OrgID,
		WorkspaceName:   workspaceContext.WorkspaceName,
		WorkspaceID:     workspaceContext.WorkspaceID,
		MatchedPolicies: make([]SecretPolicyDecisionTrace, 0),
		HookDecisions:   make([]SecretHookDecisionTrace, 0),
	}

	secret, ok := c.SecretGovernance.Secrets[secretName]
	if !ok {
		trace.DenyReason = fmt.Sprintf("secret %q is not governed", secretName)
		return trace, newSecretAccessDeniedError(principal, secretName, action, workspaceContext, trace.DenyReason)
	}
	trace.Ownership = secret.Ownership

	scopeOrg := strings.TrimSpace(secret.Scope.Org)
	scopeWorkspace := strings.TrimSpace(secret.Scope.Workspace)
	if scopeOrg != workspaceContext.OrgName || scopeWorkspace != workspaceContext.WorkspaceName {
		trace.DenyReason = fmt.Sprintf(
			"secret %q is scoped to %q/%q not %q/%q",
			secretName,
			scopeOrg,
			scopeWorkspace,
			workspaceContext.OrgName,
			workspaceContext.WorkspaceName,
		)
		return trace, newSecretAccessDeniedError(principal, secretName, action, workspaceContext, trace.DenyReason)
	}
	trace.ScopeMatched = true

	ownerPrincipal := strings.TrimSpace(secret.Ownership.OwnerPrincipal)
	stewardPrincipal := strings.TrimSpace(secret.Ownership.Steward)
	trace.OwnerMatched = principal == ownerPrincipal || (stewardPrincipal != "" && principal == stewardPrincipal)

	policyAllows := false
	for index, policy := range c.SecretGovernance.Policies {
		policySecret := strings.TrimSpace(policy.Secret)
		if policySecret != secretName {
			continue
		}
		policyPrincipal := strings.TrimSpace(policy.Principal)
		policyScopeMatched := true
		policyOrg := strings.TrimSpace(policy.Org)
		policyWorkspace := strings.TrimSpace(policy.Workspace)
		if policyOrg != "" || policyWorkspace != "" {
			policyScopeMatched = policyOrg == workspaceContext.OrgName && policyWorkspace == workspaceContext.WorkspaceName
		}
		actions, err := normalizeSecretActions(policy.Actions)
		if err != nil {
			return trace, fmt.Errorf("invalid secret_governance.policies[%d]: %w", index, err)
		}
		grantsAction := policyPrincipal == principal && policyScopeMatched && containsString(actions, action)
		trace.MatchedPolicies = append(trace.MatchedPolicies, SecretPolicyDecisionTrace{
			Index:            index,
			Principal:        policyPrincipal,
			Secret:           policySecret,
			Actions:          actions,
			Org:              policyOrg,
			Workspace:        policyWorkspace,
			PrincipalMatched: policyPrincipal == principal,
			ScopeMatched:     policyScopeMatched,
			GrantsAction:     grantsAction,
		})
		if grantsAction {
			policyAllows = true
		}
	}

	if !trace.OwnerMatched && !policyAllows {
		trace.DenyReason = fmt.Sprintf(
			"principal %q is not allowed to %s secret %q in %q/%q",
			principal,
			action,
			secretName,
			workspaceContext.OrgName,
			workspaceContext.WorkspaceName,
		)
		return trace, newSecretAccessDeniedError(principal, secretName, action, workspaceContext, trace.DenyReason)
	}

	trace.Allowed = true
	for index, hook := range request.EnforcementHooks {
		if hook == nil {
			return trace, fmt.Errorf("policy enforcement hook[%d] is nil", index)
		}
		if err := hook.Enforce(trace); err != nil {
			reason := strings.TrimSpace(err.Error())
			if reason == "" {
				reason = "hook rejected access"
			}
			trace.HookDecisions = append(trace.HookDecisions, SecretHookDecisionTrace{
				Index:   index,
				Allowed: false,
				Reason:  reason,
			})
			trace.Allowed = false
			trace.DenyReason = fmt.Sprintf("policy enforcement hook[%d] denied access: %s", index, reason)
			return trace, newSecretAccessDeniedError(principal, secretName, action, workspaceContext, trace.DenyReason)
		}
		trace.HookDecisions = append(trace.HookDecisions, SecretHookDecisionTrace{
			Index:   index,
			Allowed: true,
		})
	}

	return trace, nil
}

func newSecretAccessDeniedError(
	principal string,
	secretName string,
	action string,
	context WorkspaceContext,
	reason string,
) error {
	return &DenyError{
		Principal:     principal,
		Capability:    fmt.Sprintf("secret.%s:%s", action, secretName),
		OrgName:       context.OrgName,
		WorkspaceName: context.WorkspaceName,
		Reason:        reason,
	}
}

func normalizeSecretAction(action string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(action))
	switch normalized {
	case SecretActionRead, SecretActionWrite, SecretActionRotate:
		return normalized, nil
	default:
		return "", fmt.Errorf("secret action %q is not supported", action)
	}
}

func normalizeSecretActions(actions []string) ([]string, error) {
	seen := map[string]struct{}{}
	normalizedActions := make([]string, 0, len(actions))
	for _, rawAction := range actions {
		action, err := normalizeSecretAction(rawAction)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[action]; exists {
			return nil, fmt.Errorf("action %q is duplicated", action)
		}
		seen[action] = struct{}{}
		normalizedActions = append(normalizedActions, action)
	}
	sort.Strings(normalizedActions)
	return normalizedActions, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sortedSecretNames(secrets map[string]GovernedSecret) []string {
	names := make([]string, 0, len(secrets))
	for name := range secrets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
