package plugin

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type TraceEvent struct {
	PluginID  string
	Namespace string
	Command   string
	Timestamp time.Time
}

type Tracer interface {
	Trace(event TraceEvent) error
}

type Runtime struct {
	tracer Tracer
}

func NewRuntime(tracer Tracer) (Runtime, error) {
	if tracer == nil {
		return Runtime{}, errors.New("plugin tracer is required")
	}
	return Runtime{tracer: tracer}, nil
}

func (r Runtime) Trace(event TraceEvent) error {
	if r.tracer == nil {
		return errors.New("plugin tracer is required")
	}
	if err := validateNameToken("trace event plugin id", event.PluginID); err != nil {
		return err
	}
	if err := validateNameToken("trace event namespace", event.Namespace); err != nil {
		return err
	}
	if err := validateNameToken("trace event command", event.Command); err != nil {
		return err
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	return r.tracer.Trace(event)
}

type NamespaceTracer struct {
	namespace string
	mu        sync.Mutex
	events    []TraceEvent
}

func NewNamespaceTracer(namespace string) (*NamespaceTracer, error) {
	if err := validateNameToken("namespace", namespace); err != nil {
		return nil, err
	}
	return &NamespaceTracer{
		namespace: namespace,
		events:    make([]TraceEvent, 0, 4),
	}, nil
}

func (t *NamespaceTracer) Trace(event TraceEvent) error {
	if event.Namespace != t.namespace {
		return fmt.Errorf("namespace tracer mismatch: expected %q got %q", t.namespace, event.Namespace)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, event)
	return nil
}

func (t *NamespaceTracer) Events() []TraceEvent {
	t.mu.Lock()
	defer t.mu.Unlock()
	copied := make([]TraceEvent, len(t.events))
	copy(copied, t.events)
	return copied
}

type NopTracer struct{}

func (NopTracer) Trace(_ TraceEvent) error {
	return nil
}
