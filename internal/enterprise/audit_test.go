package enterprise

import (
	"errors"
	"testing"
	"time"
)

func TestAuditPipelineAppendsDecisionThenExecution(t *testing.T) {
	t.Parallel()

	timestamps := []time.Time{
		time.Date(2026, time.January, 5, 10, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 5, 10, 0, 1, 0, time.UTC),
	}
	index := 0
	pipeline := newAuditPipelineWithClock(func() time.Time {
		ts := timestamps[index]
		index++
		return ts
	})

	decisionEvent, err := pipeline.RecordDecision(DecisionAuditRecord{
		CorrelationID: "corr-001",
		Principal:     "alice",
		Command:       "api get",
		Capability:    "graph.read",
		OrgName:       "acme",
		WorkspaceName: "prod",
		Allowed:       true,
	})
	if err != nil {
		t.Fatalf("record decision: %v", err)
	}
	executionEvent, err := pipeline.RecordExecution(ExecutionAuditRecord{
		CorrelationID: "corr-001",
		Principal:     "alice",
		Command:       "api get",
		Capability:    "graph.read",
		OrgName:       "acme",
		WorkspaceName: "prod",
		Status:        ExecutionStatusSucceeded,
	})
	if err != nil {
		t.Fatalf("record execution: %v", err)
	}

	if decisionEvent.EventType != AuditEventTypeDecision {
		t.Fatalf("unexpected decision event type: %q", decisionEvent.EventType)
	}
	if executionEvent.EventType != AuditEventTypeExecution {
		t.Fatalf("unexpected execution event type: %q", executionEvent.EventType)
	}
	if executionEvent.PreviousDigest != decisionEvent.Digest {
		t.Fatalf(
			"expected execution previous digest to match decision digest: got=%q want=%q",
			executionEvent.PreviousDigest,
			decisionEvent.Digest,
		)
	}
	if executionEvent.Digest == decisionEvent.Digest {
		t.Fatal("decision and execution digest must differ")
	}

	events := pipeline.Events()
	if len(events) != 2 {
		t.Fatalf("unexpected event count: %d", len(events))
	}
	events[0].Principal = "mallory"
	if events[0].Allowed == nil {
		t.Fatal("expected decision allowed state")
	}
	*events[0].Allowed = false

	reloaded := pipeline.Events()
	if reloaded[0].Principal != "alice" {
		t.Fatalf("expected immutable principal, got=%q", reloaded[0].Principal)
	}
	if reloaded[0].Allowed == nil || !*reloaded[0].Allowed {
		t.Fatalf("expected immutable allowed state, got=%v", reloaded[0].Allowed)
	}
}

func TestAuditPipelineRejectsExecutionWithoutDecision(t *testing.T) {
	t.Parallel()

	pipeline := NewAuditPipeline()
	_, err := pipeline.RecordExecution(ExecutionAuditRecord{
		CorrelationID: "corr-001",
		Principal:     "alice",
		Command:       "api get",
		OrgName:       "acme",
		WorkspaceName: "prod",
		Status:        ExecutionStatusSucceeded,
	})
	if err == nil {
		t.Fatal("expected execution recording to fail without decision event")
	}
	if !errors.Is(err, ErrAuditInvariantViolation) {
		t.Fatalf("expected ErrAuditInvariantViolation, got %v", err)
	}
}

func TestAuditPipelineRejectsDuplicateDecisionCorrelation(t *testing.T) {
	t.Parallel()

	pipeline := NewAuditPipeline()
	_, err := pipeline.RecordDecision(DecisionAuditRecord{
		CorrelationID: "corr-001",
		Principal:     "alice",
		Command:       "api get",
		Capability:    "graph.read",
		OrgName:       "acme",
		WorkspaceName: "prod",
		Allowed:       true,
	})
	if err != nil {
		t.Fatalf("record first decision: %v", err)
	}

	_, err = pipeline.RecordDecision(DecisionAuditRecord{
		CorrelationID: "corr-001",
		Principal:     "alice",
		Command:       "api get",
		Capability:    "graph.read",
		OrgName:       "acme",
		WorkspaceName: "prod",
		Allowed:       true,
	})
	if err == nil {
		t.Fatal("expected duplicate decision event to fail")
	}
	if !errors.Is(err, ErrAuditInvariantViolation) {
		t.Fatalf("expected ErrAuditInvariantViolation, got %v", err)
	}
}

func TestAuditPipelineRejectsExecutionForDeniedDecision(t *testing.T) {
	t.Parallel()

	pipeline := NewAuditPipeline()
	_, err := pipeline.RecordDecision(DecisionAuditRecord{
		CorrelationID: "corr-001",
		Principal:     "alice",
		Command:       "api post",
		Capability:    "graph.write",
		OrgName:       "acme",
		WorkspaceName: "prod",
		Allowed:       false,
		DenyReason:    "missing capability",
	})
	if err != nil {
		t.Fatalf("record denied decision: %v", err)
	}

	_, err = pipeline.RecordExecution(ExecutionAuditRecord{
		CorrelationID: "corr-001",
		Principal:     "alice",
		Command:       "api post",
		Capability:    "graph.write",
		OrgName:       "acme",
		WorkspaceName: "prod",
		Status:        ExecutionStatusFailed,
		FailureReason: "authorization denied",
	})
	if err == nil {
		t.Fatal("expected execution recording to fail for denied decision")
	}
	if !errors.Is(err, ErrAuditInvariantViolation) {
		t.Fatalf("expected ErrAuditInvariantViolation, got %v", err)
	}
}
