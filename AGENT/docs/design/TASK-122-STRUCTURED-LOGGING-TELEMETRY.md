# Task 122: structured logging and telemetry

Status: implemented as an additive Go backend in `internal/osiris/task122_*`.
The language module table, CLI command table, shared documentation data, and
assembly are deliberately unchanged because task 122 explicitly limits edits
to disjoint task-122 files. The integration boundary and remaining blocker are
recorded below.

## Contract and ownership

`Telemetry` owns one dispatcher, one logger, one metric registry, and one
tracer. `NewTelemetry` starts one dispatcher goroutine. Callers own the
`context.Context`, exporter, and writer supplied to the constructor. The
backend never closes a caller-owned writer or exporter. `Close` drains queued
items and stops the dispatcher; it is safe to call more than once. `Flush`
drains without stopping the dispatcher.

Every captured item gets a monotonically increasing `Sequence` from its
Telemetry instance. Memory snapshots and JSON output are therefore ordered by
sequence. Concurrent calls are race-safe; the sequence reflects the order in
which the capture lock assigns it, not an unsupported claim about wall-clock
ordering between goroutines. A configured clock is called at capture time and
is converted to UTC.

Ordinary Osiris output is untouched. Telemetry only runs when a caller creates
and uses a `Telemetry`, attaches it with `Runtime.SetTelemetry`, or calls
`Runtime.RunWithTelemetry`. Exporter errors do not alter the wrapped program;
they are returned by the logging/metric/span operation when enqueueing fails or
by `Flush`/`Close` under a returning failure policy.

## Public identity and limits

### `type TelemetryLevel string`

`TelemetryLevel` accepts `TelemetryLevelDebug`,
`TelemetryLevelInfo`, `TelemetryLevelWarn`, and `TelemetryLevelError`. Any
other value passed to `(*StructuredLogger).Log` returns `ErrTelemetryInvalid`.

### `type BackpressurePolicy string`

`BackpressurePolicy` accepts `BackpressureBlock`,
`BackpressureDropNewest`, and `BackpressureDropOldest`. Block waits for queue
space and honors the operation context. Drop-newest returns
`ErrTelemetryQueueFull`; drop-oldest removes the oldest pending item and
accepts the new item. Both drop policies increment `(*Telemetry).Dropped`.

### `type ExporterFailurePolicy string`

`ExporterFailurePolicy` accepts `ExporterFailureIgnore`,
`ExporterFailureReturn`, and `ExporterFailureDisable`. Ignore records the
failure and continues. Return preserves the first failure for `Flush` and
`Close`. Disable records the failure, disables later submissions, and returns
the failure from later submissions and drain operations.

### `type TelemetryField struct { Key string; Value any }`

`TelemetryField` is caller input. The implementation copies it, sorts captured
fields by `Key`, converts values to JSON-compatible values, and never retains a
caller-owned map or slice reference.

### `type TelemetryLabel struct { Key string; Value string }`

`TelemetryLabel` identifies one metric series. Labels are copied and sorted by
key. Duplicate keys are invalid.

### `type TelemetryLimits struct { MaxFields int; MaxFieldKeyBytes int; MaxFieldValueBytes int; MaxLabels int; MaxLabelKeyBytes int; MaxLabelValueBytes int; MaxSeries int; MaxAttributes int; MaxEvents int; MaxEventFields int; MaxMemoryItems int; MaxJSONBytes int }`

`TelemetryLimits` bounds fields, labels, series, span state, memory retention,
and JSON records. Zero members receive defaults; negative members make
`NewTelemetry` or `NewMetrics` return `ErrTelemetryInvalid`. The defaults are
32 fields, 128-byte field keys, 1024-byte field values, 8 labels, 64-byte label
keys, 128-byte label values, 1024 series, 32 span attributes, 64 span events,
16 event fields, 4096 memory items, and 64 KiB JSON records.

### `func DefaultTelemetryLimits() TelemetryLimits`

Returns a new value containing the limits above. It has no side effects.

### `type TelemetryConfig struct { Clock func() time.Time; Component string; Source string; Limits TelemetryLimits; QueueSize int; Backpressure BackpressurePolicy; ExporterFailure ExporterFailurePolicy; Exporter TelemetryExporter }`

`TelemetryConfig` selects the clock, stable component/source defaults, all
limits, queue capacity, overflow behavior, exporter failure behavior, and the
caller-owned exporter. A nil clock, zero queue, empty policy, or nil exporter
receives the documented defaults; negative limits and unknown policies fail.

### `func DefaultTelemetryConfig() TelemetryConfig`

Returns a new configuration using `time.Now`, component `osiris`, source
`runtime`, a queue of 256, drop-newest overflow, ignored exporter failures,
and the default limits. It does not create a goroutine or exporter.

### `type TelemetryExporter interface { Export(context.Context, TelemetryItem) error }`

`Export` receives one normalized item at a time on the dispatcher goroutine.
The exporter must not mutate or retain item slices. It may return an error;
the configured failure policy decides whether that error is ignored, returned
by draining, or disables future delivery. The exporter does not own the
Telemetry item.

### `type TelemetryItem struct { Sequence uint64; Kind string; Log *LogRecord; Metric *MetricPoint; Span *SpanRecord }`

`TelemetryItem` is the immutable exporter envelope. `Kind` is `log`, `metric`,
or `span`, and exactly its matching pointer is non-nil. Exporters receive a
deep-enough copy for the documented public slices.

## Exporters

### `func NewMemoryExporter(limit int) *MemoryExporter`

Creates a concurrency-safe in-memory exporter retaining at most `limit` items.
A non-positive limit uses 4096. The exporter owns its retained copies; callers
read them only through `Snapshot`.

### `func (m *MemoryExporter) Export(context.Context, TelemetryItem) error`

Appends one copied item. When capacity is reached it returns `ErrTelemetryLimit`
and does not evict an earlier item. The context is accepted for interface
compatibility and is not used to cancel an already synchronous memory copy.

### `func (m *MemoryExporter) Snapshot() []TelemetryItem`

Returns copied items sorted by sequence. A nil exporter returns nil. The caller
owns the returned slice and all copied public slices.

### `func (m *MemoryExporter) Reset()`

Drops all retained items. It is safe on nil and concurrent with `Snapshot` or
`Export`.

### `func NewJSONExporter(io.Writer, int) (*JSONExporter, error)`

Creates a line-oriented JSON exporter. A nil writer returns
`ErrTelemetryInvalid`; a non-positive byte limit uses 64 KiB. The caller owns
the writer, and the exporter never closes it.

### `func (j *JSONExporter) Export(context.Context, TelemetryItem) error`

Marshals one item with `encoding/json`, checks the complete object plus newline
against the byte limit, and performs one serialized write. Oversized items
return `ErrTelemetryLimit`; marshal, short-write, and writer failures return
`ErrTelemetryExporter`.

## Provider and runtime hooks

### `func NewTelemetry(TelemetryConfig) (*Telemetry, error)`

Validates limits and policies, selects the default memory exporter when no
exporter is supplied, and starts the bounded dispatcher. The returned provider
owns the dispatcher and its default memory exporter. Invalid configuration
returns `ErrTelemetryInvalid`.

### `func (t *Telemetry) Logger() *StructuredLogger`

Returns the provider's logger. It returns nil for a nil provider.

### `func (t *Telemetry) Metrics() *Metrics`

Returns the provider's bounded registry. It returns nil for a nil provider.

### `func (t *Telemetry) Tracer() *Tracer`

Returns the provider's parent-aware tracer. It returns nil for a nil provider.

### `func (t *Telemetry) MemoryExporter() *MemoryExporter`

Returns the default memory exporter when `TelemetryConfig.Exporter` was nil.
It returns nil for a custom exporter or nil provider.

### `func (t *Telemetry) Flush(context.Context) error`

Flushes current metric snapshots, then waits for all queued and in-flight
items. Under return/disable failure policy it returns the first exporter error;
under ignore it returns nil. A canceled context returns its cancellation error
while the provider remains usable.

### `func (t *Telemetry) Close(context.Context) error`

Marks the dispatcher closed, drains pending items, waits for its goroutine, and
returns according to the failure policy. It is idempotent. New submissions
after close return `ErrTelemetryClosed`.

### `func (t *Telemetry) Dropped() uint64`

Returns the number of records discarded by either queue drop policy. The
counter is provider-local and monotonic until the provider is discarded.

### `func (r *Runtime) SetTelemetry(*Telemetry)`

Associates a provider with an existing Runtime through an additive registry.
Passing nil clears the association. It does not change `Runtime.Run` or
ordinary program output.

### `func (r *Runtime) Telemetry() *Telemetry`

Returns the provider associated with `r`, or nil. The registry is safe for
concurrent lookup and replacement.

### `func (r *Runtime) TelemetryContext(context.Context, string) context.Context`

Derives a context carrying the associated provider and task ID. If no provider
is attached it returns the input context. Child contexts created by existing
task APIs inherit the value through the standard context parent chain.

### `func (r *Runtime) RunWithTelemetry(context.Context, *Program) ([]Value, error)`

Runs the existing Runtime path under a root `runtime.run` span when a provider
is attached, records returned errors, and ends the span. With no provider it
delegates directly to `Runtime.Run`. It does not alter program values or emit
ordinary output.

## Logging

### `type LogRecord struct { Sequence uint64; Timestamp time.Time; Level TelemetryLevel; Component string; Source string; Message string; CorrelationID string; TaskID string; TraceID string; SpanID string; Fields []TelemetryField; Error *ErrorRecord }`

`LogRecord` is a captured record. Timestamp, component, source, level,
message, IDs, sorted fields, and the optional redacted error are bounded by the
provider. The exporter owns its copy after delivery.

### `type ErrorRecord struct { Type string; Kind string; Code string; Message string; Operation string; Resource string; CorrelationID string; Component string; Fields []TelemetryField }`

`ErrorRecord` is a bounded error projection. Structured Osiris errors use their
redacted public fields and omit sensitive metadata; ordinary Go errors use
their concrete type and redacted `Error()` text. Debug stacks and sensitive
metadata never cross the exporter boundary.

### `func (l *StructuredLogger) With(...TelemetryField) *StructuredLogger`

Returns a logger copy with additional fields. The original logger is unchanged;
fields are captured, sorted, normalized, and bounded at log time.

### `func (l *StructuredLogger) Log(context.Context, TelemetryLevel, string, string, ...TelemetryField) error`

Captures one record with level, source, message, fields, and IDs inherited from
context. Empty source uses the provider source. Invalid levels, empty/oversized
field keys, too many fields, queue overflow, closure, and disabled exporters
return the corresponding documented errors. Messages and values are redacted
before enqueueing.

### `func (l *StructuredLogger) Debug(context.Context, string, string, ...TelemetryField) error`

Calls `Log` with `TelemetryLevelDebug`.

### `func (l *StructuredLogger) Info(context.Context, string, string, ...TelemetryField) error`

Calls `Log` with `TelemetryLevelInfo`.

### `func (l *StructuredLogger) Warn(context.Context, string, string, ...TelemetryField) error`

Calls `Log` with `TelemetryLevelWarn`.

### `func (l *StructuredLogger) Error(context.Context, string, string, error, ...TelemetryField) error`

Captures an error-level record and a structured redacted error projection. A
nil error behaves like an error-level `Log`. Error metadata is bounded and
sorted; the logger does not take ownership of the supplied error.

## Context propagation

### `func WithTelemetryContext(context.Context, *Telemetry, string, string) context.Context`

Attaches a provider, correlation ID, and task ID. An empty correlation ID gets
a deterministic provider-local `corr-N` ID. The returned context is the
parent for task and span propagation.

### `func WithTaskTelemetry(context.Context, string) context.Context`

Copies the inherited telemetry and correlation ID while replacing the task ID.
It is a no-op when the input has no telemetry state.

### `func TelemetryFromContext(context.Context) *Telemetry`

Returns the attached provider or nil.

### `func CorrelationIDFromContext(context.Context) string`

Returns the inherited correlation ID or empty string.

### `func TaskIDFromContext(context.Context) string`

Returns the inherited task ID or empty string.

## Metrics

### `type MetricKind string`

The values are `MetricKindCounter`, `MetricKindGauge`, and
`MetricKindHistogram`.

### `type HistogramBucket struct { UpperBound float64; Count uint64 }`

Represents one cumulative histogram bucket. Bounds are finite and strictly
increasing.

### `type HistogramSnapshot struct { Count uint64; Sum float64; Buckets []HistogramBucket }`

Represents a copied, deterministic histogram snapshot.

### `type MetricPoint struct { Name string; Kind MetricKind; Labels []TelemetryLabel; Counter uint64; Gauge float64; Histogram *HistogramSnapshot }`

Represents one series. `Kind` selects the meaningful value field. Points are
sorted by kind, name, and canonical label order.

### `type MetricsConfig struct { Limits TelemetryLimits; HistogramBuckets []float64 }`

Configures a standalone registry. Empty buckets receive stable default bounds;
invalid non-finite or non-increasing bounds return `ErrTelemetryInvalid`.

### `func DefaultMetricsConfig() MetricsConfig`

Returns default limits and the fixed latency buckets from 1 ms through 10 s.

### `func NewMetrics(MetricsConfig) (*Metrics, error)`

Creates a standalone bounded registry. It owns copied bucket definitions and
supports handles and snapshots without an exporter.

### `type Counter struct` and `func (m *Metrics) Counter(string, ...TelemetryLabel) (*Counter, error)`

The operation validates name, labels, label limits, duplicate keys, and global
series cardinality. The returned handle remains valid for the registry's
lifetime. A new kind/name/label combination consumes one bounded series.

### `func (c *Counter) Add(context.Context, uint64) error`

Atomically increments a counter after checking cancellation. It returns context
cancellation or invalid-handle errors.

### `func (c *Counter) Value() uint64`

Returns the current counter value with a race-safe read; a nil handle returns
zero.

### `type Gauge struct` and `func (m *Metrics) Gauge(string, ...TelemetryLabel) (*Gauge, error)`

Creates or returns a bounded float64 gauge series after the same label and
cardinality validation as `Counter`.

### `func (g *Gauge) Set(context.Context, float64) error`

Atomically replaces the gauge. NaN, infinity, cancellation, and invalid
handles are rejected.

### `func (g *Gauge) Add(context.Context, float64) error`

Atomically adds a finite delta and rejects overflow, cancellation, and invalid
handles.

### `func (g *Gauge) Value() float64`

Returns the current value with a race-safe read; a nil handle returns zero.

### `type Histogram struct` and `func (m *Metrics) Histogram(string, ...TelemetryLabel) (*Histogram, error)`

Creates or returns a bounded cumulative-bucket histogram series after label and
cardinality validation.

### `func (h *Histogram) Observe(context.Context, float64) error`

Atomically records one finite value, increments count and sum, and increments
every bucket whose upper bound contains the value. Cancellation and non-finite
values are errors.

### `func (h *Histogram) Count() uint64`

Returns the race-safe observation count; a nil handle returns zero.

### `func (m *Metrics) Snapshot() []MetricPoint`

Copies every series into stable kind/name/label order. The caller owns the
returned slices.

### `func (m *Metrics) Flush(context.Context) error`

For a Telemetry-owned registry, enqueues one metric item per snapshot point.
For a standalone registry it is a no-op. Queue, closure, cancellation, and
exporter errors are returned.

## Traces and spans

### `type SpanStatus string`

The values are `SpanStatusUnset`, `SpanStatusOK`, `SpanStatusError`, and
`SpanStatusCancelled`.

### `type SpanEvent struct { Name string; Timestamp time.Time; Fields []TelemetryField }`

Represents one bounded event in insertion order with sorted fields.

### `type SpanRecord struct { Sequence uint64; TraceID string; SpanID string; ParentSpanID string; Name string; Start time.Time; End time.Time; Status SpanStatus; StatusMessage string; Cancelled bool; Attributes []TelemetryField; Events []SpanEvent; Error *ErrorRecord }`

Represents one completed immutable span. Attributes and events are copied and
bounded at mutation time.

### `type SpanContext struct { TraceID string; SpanID string; ParentSpanID string }`

Carries only propagation identity and is safe to serialize as JSON.

### `func (t *Tracer) Start(context.Context, string, ...TelemetryField) (context.Context, *Span)`

Creates a root span or a child of the active span. It assigns deterministic
provider-local trace/span IDs, captures a clock timestamp, normalizes initial
attributes, and returns a context carrying the new span. Calls do not start a
per-span goroutine.

### `type Span struct`

`Span` is a race-safe live lifecycle object. The caller owns its lifecycle and
must call `End`; `End` is idempotent and exports at most one record.

### `func (s *Span) Context(context.Context) context.Context`

Returns a context carrying the span while preserving inherited correlation and
task IDs.

### `func (s *Span) TraceID() string`

Returns the span trace ID.

### `func (s *Span) SpanID() string`

Returns the span ID.

### `func (s *Span) ParentSpanID() string`

Returns the parent ID or empty for a root.

### `func (s *Span) SetAttribute(string, any) error`

Adds or replaces one sorted bounded attribute. It rejects mutation after End,
invalid keys, sensitive values are redacted, and the attribute limit is
enforced.

### `func (s *Span) AddEvent(string, ...TelemetryField) error`

Appends one timestamped event in call order. It enforces event and event-field
limits, sorts fields, redacts values, and rejects mutation after End.

### `func (s *Span) SetStatus(SpanStatus, string)`

Sets a valid status and redacted bounded status message while the span is live.
Invalid status values and post-End updates are ignored because this operation
has no error return.

### `func (s *Span) RecordError(error)`

Stores a redacted error projection and sets error status while live. Nil errors
are ignored; ownership remains with the caller.

### `func (s *Span) End(context.Context) error`

Captures the end clock, marks a canceled context as cancelled unless an
explicit error status exists, and enqueues exactly one span record. It returns
queue, closure, cancellation, or exporter-dispatch errors.

### `func SpanContextFromContext(context.Context) (SpanContext, bool)`

Returns the active span identity and true, or a zero context and false.

### `func WithSpanContext(context.Context, *Telemetry, SpanContext, string, string) context.Context`

Attaches a remote/serialized parent identity to a context. The next
`Tracer.Start` creates a child under that identity. Empty identity or nil
provider leaves the context unchanged.

## Redaction rules

Field keys containing `token`, `secret`, `password`, `passwd`, `api_key`,
`authorization`, `credential`, or `private_key` are replaced by
`[REDACTED]`. Nested JSON objects apply the same key rule. Text markers such
as `token=...`, `secret:...`, `password=...`, and `api_key:...` redact their
value. Structured Osiris error metadata is first passed through its existing
redacted projection, then normalized again for telemetry. Redaction occurs
before queueing, so memory, JSON, and failing/custom exporters see the same
safe representation.

## Tests and operational evidence

`internal/osiris/task122_telemetry_test.go` covers stable field ordering,
injected clocks, IDs and task context, structured error redaction, metric
label/cardinality limits, histogram overflow/cancellation, span parent/event
ordering, span cancellation, JSON output, queue overflow, exporter failure,
concurrent updates, and a benchmark. Run:

```sh
GOCACHE=/tmp/osiris-go-cache go test ./internal/osiris -run '^TestTask122' -count=1
GOCACHE=/tmp/osiris-go-cache go test -race ./internal/osiris -run '^TestTask122' -count=1
GOCACHE=/tmp/osiris-go-cache go test ./internal/osiris -run '^ExampleNewTelemetry$' -count=1
GOCACHE=/tmp/osiris-go-cache go test ./internal/osiris -run '^$' -bench '^BenchmarkTask122TelemetryLog$' -benchtime=1x
```

The current checkout has unrelated in-progress task files that prevent the
package from compiling: task 121's embedded statement does not implement the
declared `DatabaseStatement` interface, and task 123 has an unused `sort`
import. Earlier package attempts also exposed missing task-120/language-test
symbols. These files are outside task 122's allowed edit set and were not
changed.

## Integration boundary

The additive hooks implemented here are `(*Runtime).SetTelemetry`,
`(*Runtime).Telemetry`, `(*Runtime).TelemetryContext`, and
`(*Runtime).RunWithTelemetry`. Existing `Session.spawnValueTask`, VM task
contexts, and standard Go context cancellation remain compatible because the
provider is carried by ordinary context values. Automatic task lifecycle
events, a `std/telemetry` module, CLI flags/diagnostics, and FFI exporter
registration require edits to `task.go`, scheduler/runtime paths,
`internal/osiris/module.go`, `cmd/osiris/main.go`, or shared assembly/data.
Those edits are explicitly forbidden by the task request, so they are not
claimed as complete here.
