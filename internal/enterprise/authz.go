package enterprise

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type Role struct {
	Capabilities     []string `yaml:"capabilities,omitempty"`
	DenyCapabilities []string `yaml:"deny_capabilities,omitempty"`
}

type Binding struct {
	Principal string `yaml:"principal"`
	Role      string `yaml:"role"`
	Org       string `yaml:"org"`
	Workspace string `yaml:"workspace"`
}

type CommandAuthorizationRequest struct {
	Principal     string
	Command       string
	OrgName       string
	WorkspaceName string
	CorrelationID string
	AuditPipeline *AuditPipeline
}

type CommandAuthorizationTrace struct {
	Principal          string                 `json:"principal"`
	Command            string                 `json:"command"`
	NormalizedCommand  string                 `json:"normalized_command"`
	RequiredCapability string                 `json:"required_capability,omitempty"`
	OrgName            string                 `json:"org_name"`
	OrgID              string                 `json:"org_id"`
	WorkspaceName      string                 `json:"workspace_name"`
	WorkspaceID        string                 `json:"workspace_id"`
	CorrelationID      string                 `json:"correlation_id,omitempty"`
	MatchedBindings    []BindingAuthorization `json:"matched_bindings"`
	DecisionTrace      []PolicyDecision       `json:"decision_trace"`
	AuditEvents        []AuditEvent           `json:"audit_events,omitempty"`
	Allowed            bool                   `json:"allowed"`
	DenyReason         string                 `json:"deny_reason,omitempty"`
}

type BindingAuthorization struct {
	Index                    int      `json:"index"`
	Principal                string   `json:"principal"`
	Role                     string   `json:"role"`
	OrgName                  string   `json:"org_name"`
	WorkspaceName            string   `json:"workspace_name"`
	Capabilities             []string `json:"capabilities"`
	DenyCapabilities         []string `json:"deny_capabilities"`
	GrantsRequiredCapability bool     `json:"grants_required_capability"`
	DeniesRequiredCapability bool     `json:"denies_required_capability"`
}

var ErrAuthorizationDenied = errors.New("authorization denied")

type DenyError struct {
	Principal     string
	Command       string
	Capability    string
	OrgName       string
	WorkspaceName string
	Reason        string
}

func (e *DenyError) Error() string {
	if e == nil {
		return "authorization denied"
	}
	if strings.TrimSpace(e.Reason) == "" {
		return "authorization denied"
	}
	return fmt.Sprintf("authorization denied: %s", e.Reason)
}

func (e *DenyError) Unwrap() error {
	return ErrAuthorizationDenied
}

func (c *Config) AuthorizeCommand(request CommandAuthorizationRequest) (CommandAuthorizationTrace, error) {
	trace := CommandAuthorizationTrace{}
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

	normalizedCommand, err := normalizeCommandReference(request.Command)
	if err != nil {
		return trace, err
	}

	workspaceContext, err := c.ResolveWorkspace(request.OrgName, request.WorkspaceName)
	if err != nil {
		return trace, err
	}

	trace = CommandAuthorizationTrace{
		Principal:         principal,
		Command:           request.Command,
		NormalizedCommand: normalizedCommand,
		OrgName:           workspaceContext.OrgName,
		OrgID:             workspaceContext.OrgID,
		WorkspaceName:     workspaceContext.WorkspaceName,
		WorkspaceID:       workspaceContext.WorkspaceID,
		MatchedBindings:   make([]BindingAuthorization, 0),
		DecisionTrace:     make([]PolicyDecision, 0),
		CorrelationID:     strings.TrimSpace(request.CorrelationID),
		AuditEvents:       make([]AuditEvent, 0),
	}
	if request.AuditPipeline != nil && trace.CorrelationID == "" {
		return trace, errors.New("correlation_id is required when audit pipeline is configured")
	}

	requiredCapability, ok := commandCapabilityByReference[normalizedCommand]
	if !ok {
		reason := fmt.Sprintf("command %q is not mapped to a capability", normalizedCommand)
		trace.DenyReason = reason
		denyErr := &DenyError{
			Principal:     principal,
			Command:       normalizedCommand,
			OrgName:       workspaceContext.OrgName,
			WorkspaceName: workspaceContext.WorkspaceName,
			Reason:        reason,
		}
		event, err := finalizeAuthorizationDecisionAudit(trace, request, denyErr)
		if event.EventID != "" {
			trace.AuditEvents = append(trace.AuditEvents, event)
		}
		return trace, err
	}
	trace.RequiredCapability = requiredCapability

	policyTrace, err := c.EvaluatePolicy(PolicyEvaluationRequest{
		Principal:     principal,
		Capability:    requiredCapability,
		OrgName:       workspaceContext.OrgName,
		WorkspaceName: workspaceContext.WorkspaceName,
	})
	trace.OrgName = policyTrace.OrgName
	trace.OrgID = policyTrace.OrgID
	trace.WorkspaceName = policyTrace.WorkspaceName
	trace.WorkspaceID = policyTrace.WorkspaceID
	trace.MatchedBindings = policyTrace.MatchedBindings
	trace.DecisionTrace = policyTrace.DecisionTrace
	trace.Allowed = policyTrace.Allowed
	trace.DenyReason = policyTrace.DenyReason
	if err != nil {
		var denyErr *DenyError
		if errors.As(err, &denyErr) {
			denyErr.Command = normalizedCommand
		}
		event, auditErr := finalizeAuthorizationDecisionAudit(trace, request, err)
		if event.EventID != "" {
			trace.AuditEvents = append(trace.AuditEvents, event)
		}
		return trace, auditErr
	}
	event, err := finalizeAuthorizationDecisionAudit(trace, request, nil)
	if event.EventID != "" {
		trace.AuditEvents = append(trace.AuditEvents, event)
	}
	return trace, err
}

func normalizeCommandReference(command string) (string, error) {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(command)))
	if len(parts) == 0 {
		return "", errors.New("command is required")
	}
	if parts[0] == "meta" {
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return "", errors.New("command is required")
	}
	return strings.Join(parts, " "), nil
}

var commandCapabilityByReference = map[string]string{
	"auth add system-user":   "auth.profile.write",
	"auth login":             "auth.user.login",
	"auth page-token":        "auth.page-token.write",
	"auth app-token set":     "auth.app-token.write",
	"auth validate":          "auth.validate",
	"auth rotate":            "auth.rotate",
	"auth debug-token":       "auth.debug-token",
	"auth list":              "auth.profile.read",
	"api get":                "graph.read",
	"api post":               "graph.write",
	"api delete":             "graph.write",
	"api batch":              "graph.batch.read",
	"insights run":           "insights.run",
	"lint request":           "lint.request",
	"schema list":            "schema.read",
	"schema sync":            "schema.write",
	"changelog check":        "changelog.read",
	"ig health":              "plugin.ig.health",
	"ops init":               "ops.baseline.write",
	"ops run":                "ops.baseline.read",
	"enterprise context":     "enterprise.workspace.read",
	"enterprise authz check": "enterprise.authz.check",
	"enterprise policy eval": "enterprise.policy.eval",
}

func validateRoles(roles map[string]Role) error {
	if len(roles) == 0 {
		return nil
	}

	for _, roleName := range sortedRoleNames(roles) {
		if err := validateRole(roleName, roles[roleName]); err != nil {
			return err
		}
	}
	return nil
}

func validateRole(roleName string, role Role) error {
	roleName = strings.TrimSpace(roleName)
	if roleName == "" {
		return errors.New("role name cannot be empty")
	}
	if len(role.Capabilities) == 0 && len(role.DenyCapabilities) == 0 {
		return fmt.Errorf("role %q capabilities or deny_capabilities are required", roleName)
	}

	seenAllow := map[string]struct{}{}
	for _, rawCapability := range role.Capabilities {
		capability := strings.TrimSpace(rawCapability)
		if capability == "" {
			return fmt.Errorf("role %q capability cannot be empty", roleName)
		}
		if _, ok := seenAllow[capability]; ok {
			return fmt.Errorf("role %q capability %q is duplicated", roleName, capability)
		}
		seenAllow[capability] = struct{}{}
	}

	seenDeny := map[string]struct{}{}
	for _, rawCapability := range role.DenyCapabilities {
		capability := strings.TrimSpace(rawCapability)
		if capability == "" {
			return fmt.Errorf("role %q deny capability cannot be empty", roleName)
		}
		if _, ok := seenDeny[capability]; ok {
			return fmt.Errorf("role %q deny capability %q is duplicated", roleName, capability)
		}
		seenDeny[capability] = struct{}{}
	}
	return nil
}

func validateBindings(orgs map[string]Org, roles map[string]Role, bindings []Binding) error {
	if len(bindings) == 0 {
		return nil
	}
	if len(roles) == 0 {
		return errors.New("enterprise roles map is required when bindings are defined")
	}

	for index, binding := range bindings {
		if err := validateBinding(index, binding, orgs, roles); err != nil {
			return err
		}
	}
	return nil
}

func validateBinding(index int, binding Binding, orgs map[string]Org, roles map[string]Role) error {
	principal := strings.TrimSpace(binding.Principal)
	if principal == "" {
		return fmt.Errorf("binding[%d] principal is required", index)
	}

	roleName := strings.TrimSpace(binding.Role)
	if roleName == "" {
		return fmt.Errorf("binding[%d] role is required", index)
	}
	if _, ok := roles[roleName]; !ok {
		return fmt.Errorf("binding[%d] role %q does not exist", index, roleName)
	}

	orgName := strings.TrimSpace(binding.Org)
	if orgName == "" {
		return fmt.Errorf("binding[%d] org is required", index)
	}
	org, ok := orgs[orgName]
	if !ok {
		return fmt.Errorf("binding[%d] org %q does not exist", index, orgName)
	}

	workspaceName := strings.TrimSpace(binding.Workspace)
	if workspaceName == "" {
		return fmt.Errorf("binding[%d] workspace is required", index)
	}
	if _, ok := org.Workspaces[workspaceName]; !ok {
		return fmt.Errorf(
			"binding[%d] workspace %q does not exist in org %q",
			index,
			workspaceName,
			orgName,
		)
	}
	return nil
}

func sortedRoleNames(roles map[string]Role) []string {
	names := make([]string, 0, len(roles))
	for name := range roles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func uniqueSortedCapabilities(capabilities []string) []string {
	seen := map[string]struct{}{}
	values := make([]string, 0, len(capabilities))
	for _, rawCapability := range capabilities {
		capability := strings.TrimSpace(rawCapability)
		if capability == "" {
			continue
		}
		if _, ok := seen[capability]; ok {
			continue
		}
		seen[capability] = struct{}{}
		values = append(values, capability)
	}
	sort.Strings(values)
	return values
}

func containsCapability(capabilities []string, requiredCapability string) bool {
	for _, capability := range capabilities {
		if capability == requiredCapability {
			return true
		}
	}
	return false
}

func finalizeAuthorizationDecisionAudit(
	trace CommandAuthorizationTrace,
	request CommandAuthorizationRequest,
	authorizationErr error,
) (AuditEvent, error) {
	if request.AuditPipeline == nil {
		return AuditEvent{}, authorizationErr
	}

	command := strings.TrimSpace(trace.NormalizedCommand)
	if command == "" {
		command = strings.TrimSpace(request.Command)
	}
	denyReason := strings.TrimSpace(trace.DenyReason)
	if !trace.Allowed && denyReason == "" && authorizationErr != nil {
		denyReason = strings.TrimSpace(authorizationErr.Error())
	}

	event, auditErr := request.AuditPipeline.RecordDecision(DecisionAuditRecord{
		CorrelationID: trace.CorrelationID,
		Principal:     trace.Principal,
		Command:       command,
		Capability:    trace.RequiredCapability,
		OrgName:       trace.OrgName,
		WorkspaceName: trace.WorkspaceName,
		Allowed:       trace.Allowed,
		DenyReason:    denyReason,
	})
	if auditErr != nil {
		if authorizationErr != nil {
			return AuditEvent{}, errors.Join(authorizationErr, auditErr)
		}
		return AuditEvent{}, auditErr
	}
	return event, authorizationErr
}
