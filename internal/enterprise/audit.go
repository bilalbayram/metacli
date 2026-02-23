package enterprise

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	AuditEventTypeDecision  = "decision"
	AuditEventTypeExecution = "execution"

	ExecutionStatusSucceeded = "succeeded"
	ExecutionStatusFailed    = "failed"
)

var ErrAuditInvariantViolation = errors.New("audit invariant violation")

type DecisionAuditRecord struct {
	CorrelationID string
	Principal     string
	Command       string
	Capability    string
	OrgName       string
	WorkspaceName string
	Allowed       bool
	DenyReason    string
}

type ExecutionAuditRecord struct {
	CorrelationID string
	Principal     string
	Command       string
	Capability    string
	OrgName       string
	WorkspaceName string
	Status        string
	FailureReason string
}

type AuditEvent struct {
	Sequence        int    `json:"sequence"`
	EventID         string `json:"event_id"`
	CorrelationID   string `json:"correlation_id"`
	EventType       string `json:"event_type"`
	Timestamp       string `json:"timestamp"`
	Principal       string `json:"principal"`
	Command         string `json:"command"`
	Capability      string `json:"capability,omitempty"`
	OrgName         string `json:"org_name"`
	WorkspaceName   string `json:"workspace_name"`
	Allowed         *bool  `json:"allowed,omitempty"`
	DenyReason      string `json:"deny_reason,omitempty"`
	ExecutionStatus string `json:"execution_status,omitempty"`
	ExecutionError  string `json:"execution_error,omitempty"`
	PreviousDigest  string `json:"previous_digest,omitempty"`
	Digest          string `json:"digest"`
}

type AuditPipeline struct {
	mu                     sync.RWMutex
	now                    func() time.Time
	events                 []AuditEvent
	lastDigest             string
	decisionByCorrelation  map[string]AuditEvent
	executionByCorrelation map[string]AuditEvent
}

func NewAuditPipeline() *AuditPipeline {
	return newAuditPipelineWithClock(time.Now)
}

func newAuditPipelineWithClock(now func() time.Time) *AuditPipeline {
	if now == nil {
		panic("audit pipeline clock function is required")
	}
	return &AuditPipeline{
		now:                    now,
		events:                 make([]AuditEvent, 0),
		decisionByCorrelation:  map[string]AuditEvent{},
		executionByCorrelation: map[string]AuditEvent{},
	}
}

func (p *AuditPipeline) RecordDecision(record DecisionAuditRecord) (AuditEvent, error) {
	if p == nil {
		return AuditEvent{}, errors.New("audit pipeline is nil")
	}

	correlationID, err := normalizeCorrelationID(record.CorrelationID)
	if err != nil {
		return AuditEvent{}, err
	}
	principal, err := requiredAuditField(record.Principal, "principal")
	if err != nil {
		return AuditEvent{}, err
	}
	command, err := requiredAuditField(record.Command, "command")
	if err != nil {
		return AuditEvent{}, err
	}
	orgName, err := requiredAuditField(record.OrgName, "org_name")
	if err != nil {
		return AuditEvent{}, err
	}
	workspaceName, err := requiredAuditField(record.WorkspaceName, "workspace_name")
	if err != nil {
		return AuditEvent{}, err
	}
	capability := strings.TrimSpace(record.Capability)
	denyReason := strings.TrimSpace(record.DenyReason)
	if !record.Allowed && denyReason == "" {
		return AuditEvent{}, errors.New("deny_reason is required when decision is denied")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.decisionByCorrelation[correlationID]; exists {
		return AuditEvent{}, fmt.Errorf(
			"%w: decision event already recorded for correlation_id %q",
			ErrAuditInvariantViolation,
			correlationID,
		)
	}

	allowed := record.Allowed
	event := AuditEvent{
		Sequence:       len(p.events) + 1,
		EventID:        fmt.Sprintf("audit-%06d", len(p.events)+1),
		CorrelationID:  correlationID,
		EventType:      AuditEventTypeDecision,
		Timestamp:      p.now().UTC().Format(time.RFC3339Nano),
		Principal:      principal,
		Command:        command,
		Capability:     capability,
		OrgName:        orgName,
		WorkspaceName:  workspaceName,
		Allowed:        &allowed,
		DenyReason:     denyReason,
		PreviousDigest: p.lastDigest,
	}

	event, err = withDigest(event)
	if err != nil {
		return AuditEvent{}, err
	}

	p.events = append(p.events, event)
	p.lastDigest = event.Digest
	p.decisionByCorrelation[correlationID] = event
	return cloneAuditEvent(event), nil
}

func (p *AuditPipeline) RecordExecution(record ExecutionAuditRecord) (AuditEvent, error) {
	if p == nil {
		return AuditEvent{}, errors.New("audit pipeline is nil")
	}

	correlationID, err := normalizeCorrelationID(record.CorrelationID)
	if err != nil {
		return AuditEvent{}, err
	}
	principal, err := requiredAuditField(record.Principal, "principal")
	if err != nil {
		return AuditEvent{}, err
	}
	command, err := requiredAuditField(record.Command, "command")
	if err != nil {
		return AuditEvent{}, err
	}
	orgName, err := requiredAuditField(record.OrgName, "org_name")
	if err != nil {
		return AuditEvent{}, err
	}
	workspaceName, err := requiredAuditField(record.WorkspaceName, "workspace_name")
	if err != nil {
		return AuditEvent{}, err
	}
	capability := strings.TrimSpace(record.Capability)
	status := strings.ToLower(strings.TrimSpace(record.Status))
	failureReason := strings.TrimSpace(record.FailureReason)
	if status != ExecutionStatusSucceeded && status != ExecutionStatusFailed {
		return AuditEvent{}, fmt.Errorf("execution status %q is not supported", record.Status)
	}
	if status == ExecutionStatusFailed && failureReason == "" {
		return AuditEvent{}, errors.New("failure reason is required when execution status is failed")
	}
	if status == ExecutionStatusSucceeded && failureReason != "" {
		return AuditEvent{}, errors.New("failure reason is only allowed when execution status is failed")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	decisionEvent, exists := p.decisionByCorrelation[correlationID]
	if !exists {
		return AuditEvent{}, fmt.Errorf(
			"%w: execution event requires a prior decision event for correlation_id %q",
			ErrAuditInvariantViolation,
			correlationID,
		)
	}
	if _, exists := p.executionByCorrelation[correlationID]; exists {
		return AuditEvent{}, fmt.Errorf(
			"%w: execution event already recorded for correlation_id %q",
			ErrAuditInvariantViolation,
			correlationID,
		)
	}
	if decisionEvent.Allowed == nil {
		return AuditEvent{}, fmt.Errorf(
			"%w: decision event for correlation_id %q has missing allowed state",
			ErrAuditInvariantViolation,
			correlationID,
		)
	}
	if !*decisionEvent.Allowed {
		return AuditEvent{}, fmt.Errorf(
			"%w: execution event cannot be recorded for denied decision correlation_id %q",
			ErrAuditInvariantViolation,
			correlationID,
		)
	}
	if decisionEvent.Principal != principal ||
		decisionEvent.Command != command ||
		decisionEvent.OrgName != orgName ||
		decisionEvent.WorkspaceName != workspaceName {
		return AuditEvent{}, fmt.Errorf(
			"%w: execution event identity does not match decision event for correlation_id %q",
			ErrAuditInvariantViolation,
			correlationID,
		)
	}
	if decisionEvent.Capability != "" && capability != "" && capability != decisionEvent.Capability {
		return AuditEvent{}, fmt.Errorf(
			"%w: execution capability %q does not match decision capability %q for correlation_id %q",
			ErrAuditInvariantViolation,
			capability,
			decisionEvent.Capability,
			correlationID,
		)
	}
	if capability == "" {
		capability = decisionEvent.Capability
	}

	event := AuditEvent{
		Sequence:        len(p.events) + 1,
		EventID:         fmt.Sprintf("audit-%06d", len(p.events)+1),
		CorrelationID:   correlationID,
		EventType:       AuditEventTypeExecution,
		Timestamp:       p.now().UTC().Format(time.RFC3339Nano),
		Principal:       principal,
		Command:         command,
		Capability:      capability,
		OrgName:         orgName,
		WorkspaceName:   workspaceName,
		ExecutionStatus: status,
		ExecutionError:  failureReason,
		PreviousDigest:  p.lastDigest,
	}

	event, err = withDigest(event)
	if err != nil {
		return AuditEvent{}, err
	}

	p.events = append(p.events, event)
	p.lastDigest = event.Digest
	p.executionByCorrelation[correlationID] = event
	return cloneAuditEvent(event), nil
}

func (p *AuditPipeline) Events() []AuditEvent {
	if p == nil {
		return nil
	}
	p.mu.RLock()
	defer p.mu.RUnlock()

	events := make([]AuditEvent, 0, len(p.events))
	for _, event := range p.events {
		events = append(events, cloneAuditEvent(event))
	}
	return events
}

func normalizeCorrelationID(value string) (string, error) {
	correlationID := strings.TrimSpace(value)
	if correlationID == "" {
		return "", errors.New("correlation_id is required")
	}
	if strings.ContainsAny(correlationID, " \t\n\r") {
		return "", fmt.Errorf("correlation_id %q cannot contain whitespace", value)
	}
	return correlationID, nil
}

func requiredAuditField(value, fieldName string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", fmt.Errorf("%s is required", fieldName)
	}
	return normalized, nil
}

type auditDigestPayload struct {
	Sequence        int    `json:"sequence"`
	EventID         string `json:"event_id"`
	CorrelationID   string `json:"correlation_id"`
	EventType       string `json:"event_type"`
	Timestamp       string `json:"timestamp"`
	Principal       string `json:"principal"`
	Command         string `json:"command"`
	Capability      string `json:"capability,omitempty"`
	OrgName         string `json:"org_name"`
	WorkspaceName   string `json:"workspace_name"`
	Allowed         *bool  `json:"allowed,omitempty"`
	DenyReason      string `json:"deny_reason,omitempty"`
	ExecutionStatus string `json:"execution_status,omitempty"`
	ExecutionError  string `json:"execution_error,omitempty"`
	PreviousDigest  string `json:"previous_digest,omitempty"`
}

func withDigest(event AuditEvent) (AuditEvent, error) {
	payload, err := json.Marshal(auditDigestPayload{
		Sequence:        event.Sequence,
		EventID:         event.EventID,
		CorrelationID:   event.CorrelationID,
		EventType:       event.EventType,
		Timestamp:       event.Timestamp,
		Principal:       event.Principal,
		Command:         event.Command,
		Capability:      event.Capability,
		OrgName:         event.OrgName,
		WorkspaceName:   event.WorkspaceName,
		Allowed:         event.Allowed,
		DenyReason:      event.DenyReason,
		ExecutionStatus: event.ExecutionStatus,
		ExecutionError:  event.ExecutionError,
		PreviousDigest:  event.PreviousDigest,
	})
	if err != nil {
		return AuditEvent{}, fmt.Errorf("encode audit event digest payload: %w", err)
	}

	input := make([]byte, 0, len(event.PreviousDigest)+len(payload))
	input = append(input, []byte(event.PreviousDigest)...)
	input = append(input, payload...)

	sum := sha256.Sum256(input)
	event.Digest = hex.EncodeToString(sum[:])
	return event, nil
}

func cloneAuditEvent(event AuditEvent) AuditEvent {
	cloned := event
	if event.Allowed != nil {
		value := *event.Allowed
		cloned.Allowed = &value
	}
	return cloned
}
