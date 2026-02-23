package enterprise

import (
	"errors"
	"fmt"
	"strings"
)

type SecretExecutionRequirement struct {
	Secret string
	Action string
}

type CommandExecutionRequest struct {
	Principal              string
	Command                string
	OrgName                string
	WorkspaceName          string
	ApprovalToken          string
	CorrelationID          string
	RequiredSecrets        []SecretExecutionRequirement
	SecretEnforcementHooks []SecretPolicyEnforcementHook
	AuditPipeline          *AuditPipeline
	Execute                func(CommandExecutionContext) error
}

type CommandExecutionContext struct {
	Authorization CommandAuthorizationTrace
	SecretAccess  []SecretAccessTrace
}

type CommandExecutionTrace struct {
	Authorization CommandAuthorizationTrace `json:"authorization"`
	SecretAccess  []SecretAccessTrace       `json:"secret_access"`
	AuditEvents   []AuditEvent              `json:"audit_events"`
	Execution     ExecutionOutcome          `json:"execution"`
}

type ExecutionOutcome struct {
	Status        string `json:"status"`
	FailureReason string `json:"failure_reason,omitempty"`
}

func (c *Config) ExecuteCommand(request CommandExecutionRequest) (CommandExecutionTrace, error) {
	trace := CommandExecutionTrace{
		SecretAccess: make([]SecretAccessTrace, 0),
		AuditEvents:  make([]AuditEvent, 0),
	}
	if c == nil {
		return trace, errors.New("enterprise config is nil")
	}
	if request.AuditPipeline == nil {
		return trace, errors.New("audit pipeline is required for enterprise execution")
	}
	if request.Execute == nil {
		return trace, errors.New("execute function is required for enterprise execution")
	}

	requiredSecrets, err := normalizeExecutionSecretRequirements(request.RequiredSecrets)
	if err != nil {
		return trace, err
	}

	authorizationTrace, err := c.AuthorizeCommand(CommandAuthorizationRequest{
		Principal:     request.Principal,
		Command:       request.Command,
		OrgName:       request.OrgName,
		WorkspaceName: request.WorkspaceName,
		ApprovalToken: request.ApprovalToken,
		CorrelationID: request.CorrelationID,
		AuditPipeline: request.AuditPipeline,
	})
	trace.Authorization = authorizationTrace
	trace.AuditEvents = append(trace.AuditEvents, cloneAuditEvents(authorizationTrace.AuditEvents)...)
	if err != nil {
		return trace, err
	}

	for _, requirement := range requiredSecrets {
		secretTrace, secretErr := c.EvaluateSecretAccess(SecretAccessRequest{
			Principal:        authorizationTrace.Principal,
			Secret:           requirement.Secret,
			Action:           requirement.Action,
			OrgName:          authorizationTrace.OrgName,
			WorkspaceName:    authorizationTrace.WorkspaceName,
			EnforcementHooks: request.SecretEnforcementHooks,
		})
		trace.SecretAccess = append(trace.SecretAccess, secretTrace)
		if secretErr != nil {
			trace.Execution = ExecutionOutcome{
				Status:        ExecutionStatusFailed,
				FailureReason: strings.TrimSpace(secretErr.Error()),
			}
			event, executionErr := finalizeExecutionAudit(request, trace, secretErr)
			if event.EventID != "" {
				trace.AuditEvents = append(trace.AuditEvents, event)
			}
			return trace, executionErr
		}
		if !secretTrace.Allowed {
			denyErr := fmt.Errorf(
				"secret governance denied %q action %q in %q/%q",
				requirement.Secret,
				requirement.Action,
				authorizationTrace.OrgName,
				authorizationTrace.WorkspaceName,
			)
			trace.Execution = ExecutionOutcome{
				Status:        ExecutionStatusFailed,
				FailureReason: denyErr.Error(),
			}
			event, executionErr := finalizeExecutionAudit(request, trace, denyErr)
			if event.EventID != "" {
				trace.AuditEvents = append(trace.AuditEvents, event)
			}
			return trace, executionErr
		}
	}

	executionErr := request.Execute(CommandExecutionContext{
		Authorization: authorizationTrace,
		SecretAccess:  cloneSecretAccessTraces(trace.SecretAccess),
	})
	if executionErr != nil {
		trace.Execution = ExecutionOutcome{
			Status:        ExecutionStatusFailed,
			FailureReason: strings.TrimSpace(executionErr.Error()),
		}
		event, combinedErr := finalizeExecutionAudit(request, trace, executionErr)
		if event.EventID != "" {
			trace.AuditEvents = append(trace.AuditEvents, event)
		}
		return trace, combinedErr
	}

	trace.Execution = ExecutionOutcome{Status: ExecutionStatusSucceeded}
	event, err := finalizeExecutionAudit(request, trace, nil)
	if event.EventID != "" {
		trace.AuditEvents = append(trace.AuditEvents, event)
	}
	return trace, err
}

func normalizeExecutionSecretRequirements(requirements []SecretExecutionRequirement) ([]SecretExecutionRequirement, error) {
	normalized := make([]SecretExecutionRequirement, 0, len(requirements))
	seen := map[string]struct{}{}
	for index, requirement := range requirements {
		secretName := strings.TrimSpace(requirement.Secret)
		if secretName == "" {
			return nil, fmt.Errorf("required_secret[%d] secret is required", index)
		}
		action, err := normalizeSecretAction(requirement.Action)
		if err != nil {
			return nil, fmt.Errorf("required_secret[%d] %w", index, err)
		}
		key := secretName + "\n" + action
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("required_secret[%d] secret/action %q:%q is duplicated", index, secretName, action)
		}
		seen[key] = struct{}{}
		normalized = append(normalized, SecretExecutionRequirement{
			Secret: secretName,
			Action: action,
		})
	}
	return normalized, nil
}

func finalizeExecutionAudit(
	request CommandExecutionRequest,
	trace CommandExecutionTrace,
	executionErr error,
) (AuditEvent, error) {
	if request.AuditPipeline == nil {
		return AuditEvent{}, executionErr
	}

	correlationID := strings.TrimSpace(trace.Authorization.CorrelationID)
	principal := strings.TrimSpace(trace.Authorization.Principal)
	command := strings.TrimSpace(trace.Authorization.NormalizedCommand)
	if command == "" {
		command = strings.TrimSpace(trace.Authorization.Command)
	}
	status := strings.TrimSpace(trace.Execution.Status)
	if status == "" {
		status = ExecutionStatusFailed
	}
	failureReason := strings.TrimSpace(trace.Execution.FailureReason)

	event, auditErr := request.AuditPipeline.RecordExecution(ExecutionAuditRecord{
		CorrelationID: correlationID,
		Principal:     principal,
		Command:       command,
		Capability:    strings.TrimSpace(trace.Authorization.RequiredCapability),
		OrgName:       strings.TrimSpace(trace.Authorization.OrgName),
		WorkspaceName: strings.TrimSpace(trace.Authorization.WorkspaceName),
		Status:        status,
		FailureReason: failureReason,
	})
	if auditErr != nil {
		if executionErr != nil {
			return AuditEvent{}, errors.Join(executionErr, auditErr)
		}
		return AuditEvent{}, auditErr
	}
	return event, executionErr
}

func cloneAuditEvents(events []AuditEvent) []AuditEvent {
	cloned := make([]AuditEvent, 0, len(events))
	for _, event := range events {
		cloned = append(cloned, cloneAuditEvent(event))
	}
	return cloned
}

func cloneSecretAccessTraces(traces []SecretAccessTrace) []SecretAccessTrace {
	cloned := make([]SecretAccessTrace, 0, len(traces))
	for _, trace := range traces {
		value := trace
		value.MatchedPolicies = append([]SecretPolicyDecisionTrace(nil), trace.MatchedPolicies...)
		value.HookDecisions = append([]SecretHookDecisionTrace(nil), trace.HookDecisions...)
		cloned = append(cloned, value)
	}
	return cloned
}
