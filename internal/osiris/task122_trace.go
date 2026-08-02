package osiris

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// SpanStatus is the terminal status of a span.
type SpanStatus string

const (
	SpanStatusUnset     SpanStatus = "unset"
	SpanStatusOK        SpanStatus = "ok"
	SpanStatusError     SpanStatus = "error"
	SpanStatusCancelled SpanStatus = "cancelled"
)

// SpanEvent is a bounded timestamped event attached to a span.
type SpanEvent struct {
	Name      string           `json:"name"`
	Timestamp time.Time        `json:"timestamp"`
	Fields    []TelemetryField `json:"fields,omitempty"`
}

// SpanRecord is the immutable exported form of a completed span.
type SpanRecord struct {
	Sequence      uint64           `json:"sequence"`
	TraceID       string           `json:"trace_id"`
	SpanID        string           `json:"span_id"`
	ParentSpanID  string           `json:"parent_span_id,omitempty"`
	Name          string           `json:"name"`
	Start         time.Time        `json:"start"`
	End           time.Time        `json:"end"`
	Status        SpanStatus       `json:"status"`
	StatusMessage string           `json:"status_message,omitempty"`
	Cancelled     bool             `json:"cancelled,omitempty"`
	Attributes    []TelemetryField `json:"attributes,omitempty"`
	Events        []SpanEvent      `json:"events,omitempty"`
	Error         *ErrorRecord     `json:"error,omitempty"`
}

// SpanContext is the propagation-safe identity of a span.
type SpanContext struct {
	TraceID      string `json:"trace_id"`
	SpanID       string `json:"span_id"`
	ParentSpanID string `json:"parent_span_id,omitempty"`
}

// Tracer starts bounded spans associated with one Telemetry provider.
type Tracer struct{ telemetry *Telemetry }

// Start creates a child span when ctx carries a parent span, or a new root
// trace otherwise. The returned context propagates the new span to tasks.
func (t *Tracer) Start(ctx context.Context, name string, attributes ...TelemetryField) (context.Context, *Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	if t == nil || t.telemetry == nil {
		return ctx, &Span{invalid: true}
	}
	if name == "" {
		name = "unnamed"
	}
	state := telemetryStateFromContext(ctx)
	traceID, parentID, correlationID, taskID := t.telemetry.nextID("trace"), "", "", ""
	if state != nil {
		correlationID, taskID = state.correlationID, state.taskID
		if state.span != nil {
			traceID, parentID = state.span.TraceID(), state.span.SpanID()
		} else if state.telemetry == t.telemetry && state.correlationID != "" {
			// Correlation and trace IDs remain separate; a context without a
			// span starts a fresh trace while retaining the request identity.
		}
	}
	normalized, err := normalizeFields(attributes, t.telemetry.limits.MaxAttributes, t.telemetry.limits.MaxFieldKeyBytes, t.telemetry.limits.MaxFieldValueBytes)
	if err != nil {
		normalized = nil
	}
	span := &Span{tracer: t, traceID: traceID, spanID: t.telemetry.nextID("span"), parentSpanID: parentID, name: redactTelemetryText(name), start: t.telemetry.now(), attributes: normalized, correlationID: correlationID, taskID: taskID}
	spanCtx := context.WithValue(ctx, telemetryContextKey{}, &telemetryContextState{telemetry: t.telemetry, correlationID: correlationID, taskID: taskID, span: span})
	return spanCtx, span
}

// Span represents a live or completed trace span. Mutations are serialized;
// End is idempotent and exports exactly one record.
type Span struct {
	mu            sync.Mutex
	tracer        *Tracer
	invalid       bool
	traceID       string
	spanID        string
	parentSpanID  string
	correlationID string
	taskID        string
	name          string
	start         time.Time
	end           time.Time
	status        SpanStatus
	statusMessage string
	cancelled     bool
	attributes    []TelemetryField
	events        []SpanEvent
	errorRecord   *ErrorRecord
	ended         bool
}

// Context returns the propagation context containing this span.
func (s *Span) Context(ctx context.Context) context.Context {
	if s == nil || s.invalid || s.tracer == nil || s.tracer.telemetry == nil {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	state := telemetryStateFromContext(ctx)
	correlationID, taskID := "", ""
	if state != nil {
		correlationID, taskID = state.correlationID, state.taskID
	}
	return context.WithValue(ctx, telemetryContextKey{}, &telemetryContextState{telemetry: s.tracer.telemetry, correlationID: correlationID, taskID: taskID, span: s})
}

// TraceID returns the immutable trace ID.
func (s *Span) TraceID() string {
	if s == nil {
		return ""
	}
	return s.traceID
}

// SpanID returns the immutable span ID.
func (s *Span) SpanID() string {
	if s == nil {
		return ""
	}
	return s.spanID
}

// ParentSpanID returns the parent span ID, or empty for a root span.
func (s *Span) ParentSpanID() string {
	if s == nil {
		return ""
	}
	return s.parentSpanID
}

// SetAttribute adds or replaces one bounded span attribute.
func (s *Span) SetAttribute(key string, value any) error {
	if s == nil || s.invalid || s.tracer == nil || s.tracer.telemetry == nil {
		return ErrTelemetryInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return fmt.Errorf("%w: span already ended", ErrTelemetryInvalid)
	}
	field, err := normalizeFields([]TelemetryField{{Key: key, Value: value}}, s.tracer.telemetry.limits.MaxAttributes, s.tracer.telemetry.limits.MaxFieldKeyBytes, s.tracer.telemetry.limits.MaxFieldValueBytes)
	if err != nil {
		return err
	}
	for i := range s.attributes {
		if s.attributes[i].Key == key {
			s.attributes[i] = field[0]
			return nil
		}
	}
	if len(s.attributes) >= s.tracer.telemetry.limits.MaxAttributes {
		return fmt.Errorf("%w: span attribute limit is %d", ErrTelemetryLimit, s.tracer.telemetry.limits.MaxAttributes)
	}
	s.attributes = append(s.attributes, field[0])
	sort.SliceStable(s.attributes, func(i, j int) bool { return s.attributes[i].Key < s.attributes[j].Key })
	return nil
}

// AddEvent appends one bounded, ordered span event.
func (s *Span) AddEvent(name string, fields ...TelemetryField) error {
	if s == nil || s.invalid || s.tracer == nil || s.tracer.telemetry == nil {
		return ErrTelemetryInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return fmt.Errorf("%w: span already ended", ErrTelemetryInvalid)
	}
	if len(s.events) >= s.tracer.telemetry.limits.MaxEvents {
		return fmt.Errorf("%w: span event limit is %d", ErrTelemetryLimit, s.tracer.telemetry.limits.MaxEvents)
	}
	normalized, err := normalizeFields(fields, s.tracer.telemetry.limits.MaxEventFields, s.tracer.telemetry.limits.MaxFieldKeyBytes, s.tracer.telemetry.limits.MaxFieldValueBytes)
	if err != nil {
		return err
	}
	s.events = append(s.events, SpanEvent{Name: redactTelemetryText(name), Timestamp: s.tracer.telemetry.now(), Fields: normalized})
	return nil
}

// SetStatus updates the span status and bounded status message.
func (s *Span) SetStatus(status SpanStatus, message string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if !s.ended && (status == SpanStatusUnset || status == SpanStatusOK || status == SpanStatusError || status == SpanStatusCancelled) {
		s.status, s.statusMessage = status, redactTelemetryText(message)
	}
	s.mu.Unlock()
}

// RecordError stores a redacted error projection and marks the span as failed.
func (s *Span) RecordError(err error) {
	if s == nil || err == nil {
		return
	}
	s.mu.Lock()
	if !s.ended {
		s.errorRecord = makeErrorRecord(err)
		s.status = SpanStatusError
		s.statusMessage = redactTelemetryText(err.Error())
	}
	s.mu.Unlock()
}

// End closes and exports the span exactly once. A canceled ctx marks the span
// canceled unless it already has an explicit error status.
func (s *Span) End(ctx context.Context) error {
	if s == nil || s.invalid || s.tracer == nil || s.tracer.telemetry == nil {
		return ErrTelemetryInvalid
	}
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		return nil
	}
	s.ended = true
	s.end = s.tracer.telemetry.now()
	if err := telemetryContextError(ctx); err != nil {
		s.cancelled = true
		if s.status != SpanStatusError {
			s.status, s.statusMessage = SpanStatusCancelled, err.Error()
		}
	}
	if s.status == "" {
		s.status = SpanStatusUnset
	}
	record := &SpanRecord{Sequence: s.tracer.telemetry.nextSequence(), TraceID: s.traceID, SpanID: s.spanID, ParentSpanID: s.parentSpanID, Name: s.name, Start: s.start, End: s.end, Status: s.status, StatusMessage: s.statusMessage, Cancelled: s.cancelled, Attributes: append([]TelemetryField(nil), s.attributes...), Events: append([]SpanEvent(nil), s.events...), Error: s.errorRecord}
	for i := range record.Events {
		record.Events[i].Fields = append([]TelemetryField(nil), record.Events[i].Fields...)
	}
	s.mu.Unlock()
	return s.tracer.telemetry.dispatch.submit(ctx, TelemetryItem{Sequence: record.Sequence, Kind: "span", Span: record})
}

// SpanContextFromContext returns the active span identity, if present.
func SpanContextFromContext(ctx context.Context) (SpanContext, bool) {
	state := telemetryStateFromContext(ctx)
	if state == nil || state.span == nil {
		return SpanContext{}, false
	}
	return SpanContext{TraceID: state.span.TraceID(), SpanID: state.span.SpanID(), ParentSpanID: state.span.ParentSpanID()}, true
}

// WithSpanContext attaches a remote or previously serialized parent context.
// The next Tracer.Start call creates a child span under that identity.
func WithSpanContext(ctx context.Context, telemetry *Telemetry, spanContext SpanContext, correlationID, taskID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if telemetry == nil || spanContext.TraceID == "" || spanContext.SpanID == "" {
		return ctx
	}
	parent := &Span{tracer: &Tracer{telemetry: telemetry}, traceID: spanContext.TraceID, spanID: spanContext.SpanID, parentSpanID: spanContext.ParentSpanID}
	if correlationID == "" {
		correlationID = telemetry.nextID("corr")
	}
	return context.WithValue(ctx, telemetryContextKey{}, &telemetryContextState{telemetry: telemetry, correlationID: correlationID, taskID: taskID, span: parent})
}
