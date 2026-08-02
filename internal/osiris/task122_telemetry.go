package osiris

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TelemetryLevel is the severity of a structured log record.
type TelemetryLevel string

const (
	TelemetryLevelDebug TelemetryLevel = "debug"
	TelemetryLevelInfo  TelemetryLevel = "info"
	TelemetryLevelWarn  TelemetryLevel = "warn"
	TelemetryLevelError TelemetryLevel = "error"
)

// BackpressurePolicy controls what a producer does when the exporter queue is full.
type BackpressurePolicy string

const (
	BackpressureBlock      BackpressurePolicy = "block"
	BackpressureDropNewest BackpressurePolicy = "drop_newest"
	BackpressureDropOldest BackpressurePolicy = "drop_oldest"
)

// ExporterFailurePolicy controls how exporter errors affect later submissions.
type ExporterFailurePolicy string

const (
	ExporterFailureIgnore  ExporterFailurePolicy = "ignore"
	ExporterFailureReturn  ExporterFailurePolicy = "return"
	ExporterFailureDisable ExporterFailurePolicy = "disable"
)

var (
	// ErrTelemetryQueueFull reports a record rejected by a full bounded queue.
	ErrTelemetryQueueFull = errors.New("telemetry exporter queue is full")
	// ErrTelemetryClosed reports a submission made after Close.
	ErrTelemetryClosed = errors.New("telemetry is closed")
	// ErrTelemetryExporter reports a failure returned by an exporter.
	ErrTelemetryExporter = errors.New("telemetry exporter failed")
	// ErrTelemetryLimit reports a field, label, series, record, or output limit violation.
	ErrTelemetryLimit = errors.New("telemetry limit exceeded")
	// ErrTelemetryInvalid reports malformed configuration or an invalid public argument.
	ErrTelemetryInvalid = errors.New("invalid telemetry argument")
)

// TelemetryField is an immutable-at-capture key/value pair. Values are
// normalized to JSON-compatible data when a record is captured.
type TelemetryField struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

// TelemetryLabel identifies one metric series. Labels are sorted by Key at
// capture time; duplicate keys are rejected.
type TelemetryLabel struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// TelemetryLimits contains all finite limits enforced by the backend. Zero
// values are replaced by DefaultTelemetryLimits; negative values are invalid.
type TelemetryLimits struct {
	MaxFields          int
	MaxFieldKeyBytes   int
	MaxFieldValueBytes int
	MaxLabels          int
	MaxLabelKeyBytes   int
	MaxLabelValueBytes int
	MaxSeries          int
	MaxAttributes      int
	MaxEvents          int
	MaxEventFields     int
	MaxMemoryItems     int
	MaxJSONBytes       int
}

// DefaultTelemetryLimits returns the bounded defaults used by NewTelemetry.
func DefaultTelemetryLimits() TelemetryLimits {
	return TelemetryLimits{
		MaxFields: 32, MaxFieldKeyBytes: 128, MaxFieldValueBytes: 1024,
		MaxLabels: 8, MaxLabelKeyBytes: 64, MaxLabelValueBytes: 128,
		MaxSeries: 1024, MaxAttributes: 32, MaxEvents: 64, MaxEventFields: 16,
		MaxMemoryItems: 4096, MaxJSONBytes: 64 << 10,
	}
}

// TelemetryConfig defines clocks, identity, limits, exporter delivery, and
// failure behavior for one Telemetry instance.
type TelemetryConfig struct {
	Clock           func() time.Time
	Component       string
	Source          string
	Limits          TelemetryLimits
	QueueSize       int
	Backpressure    BackpressurePolicy
	ExporterFailure ExporterFailurePolicy
	Exporter        TelemetryExporter
}

// DefaultTelemetryConfig returns a deterministic-safe configuration using an
// in-memory exporter, a bounded queue, drop-newest overflow, and ignored
// exporter failures.
func DefaultTelemetryConfig() TelemetryConfig {
	return TelemetryConfig{
		Clock:           time.Now,
		Component:       "osiris",
		Source:          "runtime",
		Limits:          DefaultTelemetryLimits(),
		QueueSize:       256,
		Backpressure:    BackpressureDropNewest,
		ExporterFailure: ExporterFailureIgnore,
	}
}

// TelemetryExporter receives one already-normalized item at a time. Exporters
// must not mutate the item or retain references to its slices.
type TelemetryExporter interface {
	Export(context.Context, TelemetryItem) error
}

// TelemetryItem is the deterministic exporter envelope. Exactly one payload
// pointer is non-nil according to Kind.
type TelemetryItem struct {
	Sequence uint64       `json:"sequence"`
	Kind     string       `json:"kind"`
	Log      *LogRecord   `json:"log,omitempty"`
	Metric   *MetricPoint `json:"metric,omitempty"`
	Span     *SpanRecord  `json:"span,omitempty"`
}

// LogRecord is a structured log with stable identity, source, clock, context,
// sorted fields, and an optional redacted structured error.
type LogRecord struct {
	Sequence      uint64           `json:"sequence"`
	Timestamp     time.Time        `json:"timestamp"`
	Level         TelemetryLevel   `json:"level"`
	Component     string           `json:"component"`
	Source        string           `json:"source"`
	Message       string           `json:"message"`
	CorrelationID string           `json:"correlation_id,omitempty"`
	TaskID        string           `json:"task_id,omitempty"`
	TraceID       string           `json:"trace_id,omitempty"`
	SpanID        string           `json:"span_id,omitempty"`
	Fields        []TelemetryField `json:"fields,omitempty"`
	Error         *ErrorRecord     `json:"error,omitempty"`
}

// ErrorRecord is the redacted, bounded error projection exported with a log.
type ErrorRecord struct {
	Type          string           `json:"type"`
	Kind          string           `json:"kind,omitempty"`
	Code          string           `json:"code,omitempty"`
	Message       string           `json:"message"`
	Operation     string           `json:"operation,omitempty"`
	Resource      string           `json:"resource,omitempty"`
	CorrelationID string           `json:"correlation_id,omitempty"`
	Component     string           `json:"component,omitempty"`
	Fields        []TelemetryField `json:"fields,omitempty"`
}

// MemoryExporter is a bounded, concurrency-safe exporter for deterministic
// tests and local diagnostics.
type MemoryExporter struct {
	mu    sync.Mutex
	limit int
	items []TelemetryItem
}

// NewMemoryExporter constructs a memory exporter retaining at most limit
// items. A non-positive limit uses the default memory-item limit.
func NewMemoryExporter(limit int) *MemoryExporter {
	if limit <= 0 {
		limit = DefaultTelemetryLimits().MaxMemoryItems
	}
	return &MemoryExporter{limit: limit}
}

// Export appends a deep-enough copy of item to the bounded memory snapshot.
func (m *MemoryExporter) Export(_ context.Context, item TelemetryItem) error {
	if m == nil {
		return ErrTelemetryInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.items) >= m.limit {
		return fmt.Errorf("%w: memory exporter capacity %d", ErrTelemetryLimit, m.limit)
	}
	m.items = append(m.items, cloneTelemetryItem(item))
	return nil
}

// Snapshot returns items in sequence order and never exposes exporter-owned
// slices to the caller.
func (m *MemoryExporter) Snapshot() []TelemetryItem {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]TelemetryItem, len(m.items))
	for i := range m.items {
		out[i] = cloneTelemetryItem(m.items[i])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
	return out
}

// Reset removes all retained memory items.
func (m *MemoryExporter) Reset() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.items = nil
	m.mu.Unlock()
}

// JSONExporter writes one canonical JSON object and newline per item.
type JSONExporter struct {
	mu    sync.Mutex
	w     io.Writer
	limit int
}

// NewJSONExporter constructs an exporter writing to w and rejecting encoded
// records larger than maxBytes. A non-positive maxBytes uses the default.
func NewJSONExporter(w io.Writer, maxBytes int) (*JSONExporter, error) {
	if w == nil {
		return nil, fmt.Errorf("%w: nil JSON writer", ErrTelemetryInvalid)
	}
	if maxBytes <= 0 {
		maxBytes = DefaultTelemetryLimits().MaxJSONBytes
	}
	return &JSONExporter{w: w, limit: maxBytes}, nil
}

// Export writes item atomically with respect to other exporter calls.
func (j *JSONExporter) Export(_ context.Context, item TelemetryItem) error {
	if j == nil || j.w == nil {
		return ErrTelemetryInvalid
	}
	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrTelemetryExporter, err)
	}
	if len(data)+1 > j.limit {
		return fmt.Errorf("%w: JSON item is %d bytes, limit is %d", ErrTelemetryLimit, len(data)+1, j.limit)
	}
	data = append(data, '\n')
	j.mu.Lock()
	defer j.mu.Unlock()
	n, err := j.w.Write(data)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrTelemetryExporter, err)
	}
	if n != len(data) {
		return fmt.Errorf("%w: short JSON write", ErrTelemetryExporter)
	}
	return nil
}

// Telemetry owns the logger, metrics, tracer, sequence generator, and bounded
// exporter dispatcher for one host/runtime integration.
type Telemetry struct {
	clock     func() time.Time
	component string
	source    string
	limits    TelemetryLimits
	dispatch  *telemetryDispatcher
	logger    *StructuredLogger
	metrics   *Metrics
	tracer    *Tracer
	idsMu     sync.Mutex
	sequence  uint64
}

// NewTelemetry validates cfg and starts a bounded exporter dispatcher.
func NewTelemetry(cfg TelemetryConfig) (*Telemetry, error) {
	limits, err := normalizeTelemetryLimits(cfg.Limits)
	if err != nil {
		return nil, err
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 256
	}
	if cfg.Backpressure == "" {
		cfg.Backpressure = BackpressureDropNewest
	}
	if cfg.ExporterFailure == "" {
		cfg.ExporterFailure = ExporterFailureIgnore
	}
	if !validBackpressure(cfg.Backpressure) || !validFailurePolicy(cfg.ExporterFailure) {
		return nil, fmt.Errorf("%w: unknown queue or exporter failure policy", ErrTelemetryInvalid)
	}
	exporter := cfg.Exporter
	var memory *MemoryExporter
	if exporter == nil {
		memory = NewMemoryExporter(limits.MaxMemoryItems)
		exporter = memory
	}
	t := &Telemetry{clock: cfg.Clock, component: cfg.Component, source: cfg.Source, limits: limits}
	t.dispatch = newTelemetryDispatcher(exporter, cfg.QueueSize, cfg.Backpressure, cfg.ExporterFailure)
	t.logger = &StructuredLogger{telemetry: t}
	t.metrics = newTelemetryMetrics(t)
	t.tracer = &Tracer{telemetry: t}
	if memory != nil {
		t.dispatch.defaultMemory = memory
	}
	return t, nil
}

// Logger returns the structured logger bound to this Telemetry instance.
func (t *Telemetry) Logger() *StructuredLogger {
	if t == nil {
		return nil
	}
	return t.logger
}

// Metrics returns the bounded metrics registry bound to this Telemetry instance.
func (t *Telemetry) Metrics() *Metrics {
	if t == nil {
		return nil
	}
	return t.metrics
}

// Tracer returns the trace/span provider bound to this Telemetry instance.
func (t *Telemetry) Tracer() *Tracer {
	if t == nil {
		return nil
	}
	return t.tracer
}

// MemoryExporter returns the default in-memory exporter, or nil when a custom
// exporter was supplied in the configuration.
func (t *Telemetry) MemoryExporter() *MemoryExporter {
	if t == nil || t.dispatch == nil {
		return nil
	}
	return t.dispatch.defaultMemory
}

// Flush exports all queued records and returns the first exporter failure when
// the configured failure policy is ExporterFailureReturn or Disable.
func (t *Telemetry) Flush(ctx context.Context) error {
	if t == nil || t.dispatch == nil {
		return nil
	}
	if t.metrics != nil {
		if err := t.metrics.Flush(ctx); err != nil {
			return err
		}
	}
	return t.dispatch.flush(ctx)
}

// Close flushes queued records, stops the dispatcher, and is idempotent.
func (t *Telemetry) Close(ctx context.Context) error {
	if t == nil || t.dispatch == nil {
		return nil
	}
	return t.dispatch.close(ctx)
}

// Dropped returns the number of records discarded by queue overflow.
func (t *Telemetry) Dropped() uint64 {
	if t == nil || t.dispatch == nil {
		return 0
	}
	t.dispatch.mu.Lock()
	dropped := t.dispatch.dropped
	t.dispatch.mu.Unlock()
	return dropped
}

func (t *Telemetry) nextSequence() uint64 {
	t.idsMu.Lock()
	t.sequence++
	n := t.sequence
	t.idsMu.Unlock()
	return n
}

func (t *Telemetry) nextID(prefix string) string {
	return prefix + "-" + strconv.FormatUint(t.nextSequence(), 10)
}

func (t *Telemetry) now() time.Time {
	if t == nil || t.clock == nil {
		return time.Now().UTC()
	}
	return t.clock().UTC()
}

type telemetryDispatcher struct {
	mu            sync.Mutex
	cond          *sync.Cond
	exporter      TelemetryExporter
	queue         []TelemetryItem
	queueSize     int
	backpressure  BackpressurePolicy
	failurePolicy ExporterFailurePolicy
	closed        bool
	disabled      bool
	inFlight      bool
	lastErr       error
	dropped       uint64
	done          chan struct{}
	defaultMemory *MemoryExporter
}

func newTelemetryDispatcher(exporter TelemetryExporter, size int, backpressure BackpressurePolicy, failure ExporterFailurePolicy) *telemetryDispatcher {
	d := &telemetryDispatcher{exporter: exporter, queueSize: size, backpressure: backpressure, failurePolicy: failure, done: make(chan struct{})}
	d.cond = sync.NewCond(&d.mu)
	go d.run()
	return d
}

func (d *telemetryDispatcher) run() {
	defer close(d.done)
	for {
		d.mu.Lock()
		for len(d.queue) == 0 && !d.closed {
			d.cond.Wait()
		}
		if len(d.queue) == 0 && d.closed {
			d.mu.Unlock()
			return
		}
		item := d.queue[0]
		d.queue = d.queue[1:]
		d.inFlight = true
		d.cond.Broadcast()
		d.mu.Unlock()

		err := d.exporter.Export(context.Background(), item)

		d.mu.Lock()
		d.inFlight = false
		if err != nil {
			d.lastErr = fmt.Errorf("%w: %v", ErrTelemetryExporter, err)
			if d.failurePolicy == ExporterFailureDisable {
				d.disabled = true
			}
		}
		d.cond.Broadcast()
		d.mu.Unlock()
	}
}

func (d *telemetryDispatcher) submit(ctx context.Context, item TelemetryItem) error {
	if d == nil {
		return ErrTelemetryInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for len(d.queue) >= d.queueSize && !d.closed && !d.disabled {
		switch d.backpressure {
		case BackpressureDropNewest:
			d.dropped++
			return ErrTelemetryQueueFull
		case BackpressureDropOldest:
			d.dropped++
			d.queue = d.queue[1:]
		case BackpressureBlock:
			d.cond.Wait()
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		default:
			return ErrTelemetryInvalid
		}
	}
	if d.closed {
		return ErrTelemetryClosed
	}
	if d.disabled {
		return d.lastErr
	}
	d.queue = append(d.queue, item)
	d.cond.Signal()
	return nil
}

func (d *telemetryDispatcher) flush(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for len(d.queue) > 0 || d.inFlight {
		if err := ctx.Err(); err != nil {
			return err
		}
		d.cond.Wait()
	}
	if d.failurePolicy != ExporterFailureIgnore {
		return d.lastErr
	}
	return nil
}

func (d *telemetryDispatcher) close(ctx context.Context) error {
	d.mu.Lock()
	if !d.closed {
		d.closed = true
		d.cond.Broadcast()
	}
	d.mu.Unlock()
	if err := d.flush(ctx); err != nil {
		return err
	}
	<-d.done
	if d.failurePolicy != ExporterFailureIgnore {
		return d.lastErr
	}
	return nil
}

// StructuredLogger emits bounded structured records through Telemetry.
type StructuredLogger struct {
	telemetry *Telemetry
	fields    []TelemetryField
}

// With returns a logger retaining additional fields for each subsequent log.
func (l *StructuredLogger) With(fields ...TelemetryField) *StructuredLogger {
	if l == nil {
		return nil
	}
	out := &StructuredLogger{telemetry: l.telemetry, fields: append([]TelemetryField(nil), l.fields...)}
	out.fields = append(out.fields, fields...)
	return out
}

// Log captures and exports one structured record using context correlation,
// task, trace, and span identifiers when present.
func (l *StructuredLogger) Log(ctx context.Context, level TelemetryLevel, source, message string, fields ...TelemetryField) error {
	if l == nil || l.telemetry == nil {
		return ErrTelemetryInvalid
	}
	if !validTelemetryLevel(level) {
		return fmt.Errorf("%w: log level %q", ErrTelemetryInvalid, level)
	}
	t := l.telemetry
	state := telemetryStateFromContext(ctx)
	if source == "" {
		source = t.source
	}
	all := append(append([]TelemetryField(nil), l.fields...), fields...)
	normalized, err := normalizeFields(all, t.limits.MaxFields, t.limits.MaxFieldKeyBytes, t.limits.MaxFieldValueBytes)
	if err != nil {
		return err
	}
	record := &LogRecord{Sequence: t.nextSequence(), Timestamp: t.now(), Level: level, Component: t.component, Source: source, Message: redactTelemetryText(message), Fields: normalized}
	if state != nil {
		record.CorrelationID, record.TaskID = state.correlationID, state.taskID
		if state.span != nil {
			record.TraceID, record.SpanID = state.span.TraceID(), state.span.SpanID()
		}
	}
	return t.dispatch.submit(ctx, TelemetryItem{Sequence: record.Sequence, Kind: "log", Log: record})
}

// Debug emits a debug-level structured record.
func (l *StructuredLogger) Debug(ctx context.Context, source, message string, fields ...TelemetryField) error {
	return l.Log(ctx, TelemetryLevelDebug, source, message, fields...)
}

// Info emits an info-level structured record.
func (l *StructuredLogger) Info(ctx context.Context, source, message string, fields ...TelemetryField) error {
	return l.Log(ctx, TelemetryLevelInfo, source, message, fields...)
}

// Warn emits a warning-level structured record.
func (l *StructuredLogger) Warn(ctx context.Context, source, message string, fields ...TelemetryField) error {
	return l.Log(ctx, TelemetryLevelWarn, source, message, fields...)
}

// Error emits an error-level structured record with a bounded redacted error projection.
func (l *StructuredLogger) Error(ctx context.Context, source, message string, cause error, fields ...TelemetryField) error {
	if l == nil || l.telemetry == nil {
		return ErrTelemetryInvalid
	}
	state := telemetryStateFromContext(ctx)
	if cause == nil {
		return l.Log(ctx, TelemetryLevelError, source, message, fields...)
	}
	// The error projection is carried through a reserved field and removed by
	// buildErrorRecord; this preserves the common bounded-field path.
	fields = append(append([]TelemetryField(nil), l.fields...), fields...)
	fields = append(fields, TelemetryField{Key: "_error", Value: cause})
	return l.logErrorRecord(ctx, source, message, state, fields)
}

func (l *StructuredLogger) logErrorRecord(ctx context.Context, source, message string, state *telemetryContextState, fields []TelemetryField) error {
	t := l.telemetry
	if source == "" {
		source = t.source
	}
	normalized, err := normalizeFields(fields, t.limits.MaxFields, t.limits.MaxFieldKeyBytes, t.limits.MaxFieldValueBytes)
	if err != nil {
		return err
	}
	var errorRecord *ErrorRecord
	filtered := normalized[:0]
	for _, field := range normalized {
		if field.Key == "_error" {
			errorRecord = makeErrorRecord(field.Value)
			continue
		}
		filtered = append(filtered, field)
	}
	seq := t.nextSequence()
	record := &LogRecord{Sequence: seq, Timestamp: t.now(), Level: TelemetryLevelError, Component: t.component, Source: source, Message: redactTelemetryText(message), Fields: filtered, Error: errorRecord}
	if state != nil {
		record.CorrelationID, record.TaskID = state.correlationID, state.taskID
		if state.span != nil {
			record.TraceID, record.SpanID = state.span.TraceID(), state.span.SpanID()
		}
	}
	return t.dispatch.submit(ctx, TelemetryItem{Sequence: seq, Kind: "log", Log: record})
}

func makeErrorRecord(value any) *ErrorRecord {
	if err, ok := value.(error); ok {
		if structured, ok := err.(*OsirisError); ok && structured != nil {
			structured = structured.Redacted()
			record := &ErrorRecord{Type: reflect.TypeOf(err).String(), Kind: string(structured.kind), Code: structured.code, Message: redactTelemetryText(structured.displayMessage()), Operation: redactTelemetryText(structured.operation), Resource: redactTelemetryText(structured.resource), CorrelationID: redactTelemetryText(structured.correlationID), Component: redactTelemetryText(structured.component)}
			for key, val := range structured.metadata {
				record.Fields = append(record.Fields, TelemetryField{Key: key, Value: canonicalTelemetryValue(key, val, DefaultTelemetryLimits().MaxFieldValueBytes)})
			}
			if len(record.Fields) > DefaultTelemetryLimits().MaxFields {
				record.Fields = record.Fields[:DefaultTelemetryLimits().MaxFields]
			}
			sort.SliceStable(record.Fields, func(i, j int) bool { return record.Fields[i].Key < record.Fields[j].Key })
			return record
		}
		return &ErrorRecord{Type: reflect.TypeOf(err).String(), Message: redactTelemetryText(err.Error())}
	}
	return &ErrorRecord{Type: "value", Message: redactTelemetryText(fmt.Sprint(value))}
}

type telemetryContextState struct {
	telemetry     *Telemetry
	correlationID string
	taskID        string
	span          *Span
}

type telemetryContextKey struct{}

func telemetryStateFromContext(ctx context.Context) *telemetryContextState {
	if ctx == nil {
		return nil
	}
	state, _ := ctx.Value(telemetryContextKey{}).(*telemetryContextState)
	return state
}

// WithTelemetryContext attaches telemetry and stable correlation/task identity
// to ctx. Child task contexts inherit this value through context.WithCancel.
func WithTelemetryContext(ctx context.Context, telemetry *Telemetry, correlationID, taskID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if telemetry == nil {
		return ctx
	}
	if correlationID == "" {
		correlationID = telemetry.nextID("corr")
	}
	return context.WithValue(ctx, telemetryContextKey{}, &telemetryContextState{telemetry: telemetry, correlationID: correlationID, taskID: taskID})
}

// WithTaskTelemetry derives a context with taskID while preserving telemetry
// and the parent correlation ID.
func WithTaskTelemetry(ctx context.Context, taskID string) context.Context {
	state := telemetryStateFromContext(ctx)
	if state == nil {
		return ctx
	}
	return context.WithValue(ctx, telemetryContextKey{}, &telemetryContextState{telemetry: state.telemetry, correlationID: state.correlationID, taskID: taskID, span: state.span})
}

// TelemetryFromContext returns the telemetry provider attached to ctx.
func TelemetryFromContext(ctx context.Context) *Telemetry {
	state := telemetryStateFromContext(ctx)
	if state == nil {
		return nil
	}
	return state.telemetry
}

// CorrelationIDFromContext returns the inherited correlation ID, if any.
func CorrelationIDFromContext(ctx context.Context) string {
	state := telemetryStateFromContext(ctx)
	if state == nil {
		return ""
	}
	return state.correlationID
}

// TaskIDFromContext returns the inherited task ID, if any.
func TaskIDFromContext(ctx context.Context) string {
	state := telemetryStateFromContext(ctx)
	if state == nil {
		return ""
	}
	return state.taskID
}

// SetTelemetry attaches a provider for additive Runtime integration. Existing
// Runtime execution remains unchanged unless RunWithTelemetry is called.
func (r *Runtime) SetTelemetry(telemetry *Telemetry) {
	if r == nil {
		return
	}
	runtimeTelemetryRegistry.Store(r, telemetry)
}

// Telemetry returns the provider previously attached with SetTelemetry.
func (r *Runtime) Telemetry() *Telemetry {
	if r == nil {
		return nil
	}
	value, _ := runtimeTelemetryRegistry.Load(r)
	telemetry, _ := value.(*Telemetry)
	return telemetry
}

// TelemetryContext derives a task-compatible context using the provider
// attached to r and the runtime source name.
func (r *Runtime) TelemetryContext(ctx context.Context, taskID string) context.Context {
	telemetry := r.Telemetry()
	if telemetry == nil {
		return ctx
	}
	return WithTaskTelemetry(WithTelemetryContext(ctx, telemetry, "", taskID), taskID)
}

// RunWithTelemetry wraps Runtime.Run in a root span and records returned errors.
// It is an additive integration hook because Runtime.Run itself is intentionally
// unchanged by task-122's disjoint-file scope.
func (r *Runtime) RunWithTelemetry(ctx context.Context, prog *Program) ([]Value, error) {
	if r == nil {
		return nil, ErrTelemetryInvalid
	}
	telemetry := r.Telemetry()
	if telemetry == nil {
		return r.Run(ctx, prog)
	}
	ctx = telemetryContextForRun(ctx, telemetry)
	spanCtx, span := telemetry.Tracer().Start(ctx, "runtime.run", TelemetryField{Key: "source", Value: programSourceName(prog)})
	values, err := r.Run(spanCtx, prog)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(SpanStatusError, err.Error())
	} else {
		span.SetStatus(SpanStatusOK, "")
	}
	endErr := span.End(spanCtx)
	if err != nil {
		return values, err
	}
	return values, endErr
}

var runtimeTelemetryRegistry sync.Map

func telemetryContextForRun(ctx context.Context, telemetry *Telemetry) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if TelemetryFromContext(ctx) == telemetry {
		return ctx
	}
	return WithTelemetryContext(ctx, telemetry, "", "runtime")
}

func programSourceName(prog *Program) string {
	if prog == nil {
		return ""
	}
	return prog.SourceName
}

func normalizeTelemetryLimits(in TelemetryLimits) (TelemetryLimits, error) {
	defaults := DefaultTelemetryLimits()
	values := []*int{&in.MaxFields, &in.MaxFieldKeyBytes, &in.MaxFieldValueBytes, &in.MaxLabels, &in.MaxLabelKeyBytes, &in.MaxLabelValueBytes, &in.MaxSeries, &in.MaxAttributes, &in.MaxEvents, &in.MaxEventFields, &in.MaxMemoryItems, &in.MaxJSONBytes}
	defaultValues := []int{defaults.MaxFields, defaults.MaxFieldKeyBytes, defaults.MaxFieldValueBytes, defaults.MaxLabels, defaults.MaxLabelKeyBytes, defaults.MaxLabelValueBytes, defaults.MaxSeries, defaults.MaxAttributes, defaults.MaxEvents, defaults.MaxEventFields, defaults.MaxMemoryItems, defaults.MaxJSONBytes}
	for i, value := range values {
		if *value < 0 {
			return TelemetryLimits{}, fmt.Errorf("%w: negative telemetry limit", ErrTelemetryInvalid)
		}
		if *value == 0 {
			*value = defaultValues[i]
		}
	}
	return in, nil
}

func validTelemetryLevel(level TelemetryLevel) bool {
	return level == TelemetryLevelDebug || level == TelemetryLevelInfo || level == TelemetryLevelWarn || level == TelemetryLevelError
}

func validBackpressure(policy BackpressurePolicy) bool {
	return policy == BackpressureBlock || policy == BackpressureDropNewest || policy == BackpressureDropOldest
}

func validFailurePolicy(policy ExporterFailurePolicy) bool {
	return policy == ExporterFailureIgnore || policy == ExporterFailureReturn || policy == ExporterFailureDisable
}

func normalizeFields(fields []TelemetryField, maxFields, maxKeyBytes, maxValueBytes int) ([]TelemetryField, error) {
	if len(fields) > maxFields {
		return nil, fmt.Errorf("%w: %d fields, limit is %d", ErrTelemetryLimit, len(fields), maxFields)
	}
	out := make([]TelemetryField, 0, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field.Key) == "" {
			return nil, fmt.Errorf("%w: empty field key", ErrTelemetryInvalid)
		}
		if len([]byte(field.Key)) > maxKeyBytes {
			return nil, fmt.Errorf("%w: field key %q is too long", ErrTelemetryLimit, field.Key)
		}
		out = append(out, TelemetryField{Key: field.Key, Value: canonicalTelemetryValue(field.Key, field.Value, maxValueBytes)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func canonicalTelemetryValue(key string, value any, maxBytes int) any {
	if isSensitiveTelemetryKey(key) {
		return "[REDACTED]"
	}
	if key != "_error" {
		if err, ok := value.(error); ok {
			return makeErrorRecord(err)
		}
	}
	if err, ok := value.(error); ok {
		return err
	}
	if value == nil {
		return nil
	}
	var encoded []byte
	if raw, err := json.Marshal(value); err == nil {
		encoded = raw
	} else {
		encoded = []byte(fmt.Sprint(value))
	}
	if len(encoded) > maxBytes {
		return string(encoded[:maxBytes]) + "…"
	}
	var normalized any
	if err := json.Unmarshal(encoded, &normalized); err == nil {
		return redactTelemetryValue(normalized)
	}
	return redactTelemetryText(string(encoded))
}

func redactTelemetryValue(value any) any {
	switch v := value.(type) {
	case string:
		return redactTelemetryText(v)
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = redactTelemetryValue(v[i])
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			if isSensitiveTelemetryKey(key) {
				out[key] = "[REDACTED]"
			} else {
				out[key] = redactTelemetryValue(item)
			}
		}
		return out
	default:
		return value
	}
}

func isSensitiveTelemetryKey(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("-", "_", " ", "_").Replace(key))
	for _, sensitive := range []string{"token", "secret", "password", "passwd", "api_key", "authorization", "credential", "private_key"} {
		if strings.Contains(normalized, sensitive) {
			return true
		}
	}
	return false
}

func redactTelemetryText(value string) string {
	for _, marker := range []string{"token=", "token:", "secret=", "secret:", "password=", "password:", "api_key=", "api_key:"} {
		for {
			idx := strings.Index(strings.ToLower(value), marker)
			if idx < 0 {
				break
			}
			start := idx + len(marker)
			end := start
			for end < len(value) && !strings.ContainsRune(" \t\r\n,;", rune(value[end])) {
				end++
			}
			value = value[:idx] + marker[:len(marker)-1] + "[REDACTED]" + value[end:]
		}
	}
	return value
}

func cloneTelemetryItem(item TelemetryItem) TelemetryItem {
	out := item
	if item.Log != nil {
		logCopy := *item.Log
		logCopy.Fields = append([]TelemetryField(nil), item.Log.Fields...)
		if item.Log.Error != nil {
			errCopy := *item.Log.Error
			errCopy.Fields = append([]TelemetryField(nil), item.Log.Error.Fields...)
			logCopy.Error = &errCopy
		}
		out.Log = &logCopy
	}
	if item.Metric != nil {
		metricCopy := *item.Metric
		metricCopy.Labels = append([]TelemetryLabel(nil), item.Metric.Labels...)
		if item.Metric.Histogram != nil {
			histCopy := *item.Metric.Histogram
			histCopy.Buckets = append([]HistogramBucket(nil), item.Metric.Histogram.Buckets...)
			metricCopy.Histogram = &histCopy
		}
		out.Metric = &metricCopy
	}
	if item.Span != nil {
		spanCopy := *item.Span
		spanCopy.Attributes = append([]TelemetryField(nil), item.Span.Attributes...)
		spanCopy.Events = append([]SpanEvent(nil), item.Span.Events...)
		for i := range spanCopy.Events {
			spanCopy.Events[i].Fields = append([]TelemetryField(nil), spanCopy.Events[i].Fields...)
		}
		out.Span = &spanCopy
	}
	return out
}
