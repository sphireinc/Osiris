package osiris

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func task122TelemetryForTest(t *testing.T, exporter TelemetryExporter) *Telemetry {
	t.Helper()
	cfg := DefaultTelemetryConfig()
	cfg.Clock = func() time.Time { return time.Unix(123, 456).UTC() }
	cfg.Exporter = exporter
	cfg.QueueSize = 128
	cfg.Backpressure = BackpressureBlock
	cfg.Limits.MaxMemoryItems = 1 << 16
	tel, err := NewTelemetry(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tel.Close(context.Background()) })
	return tel
}

func task122Flush(t *testing.T, tel *Telemetry) {
	t.Helper()
	if err := tel.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestTask122StructuredLogOrderingClockContextAndRedaction(t *testing.T) {
	memory := NewMemoryExporter(32)
	tel := task122TelemetryForTest(t, memory)
	ctx := WithTelemetryContext(context.Background(), tel, "request-7", "task-2")
	logger := tel.Logger().With(TelemetryField{Key: "z", Value: "last"})
	if err := logger.Info(ctx, "source.go:12", "password=leak", TelemetryField{Key: "b", Value: 2}, TelemetryField{Key: "a", Value: 1}, TelemetryField{Key: "api-key", Value: "secret-value"}); err != nil {
		t.Fatal(err)
	}
	task122Flush(t, tel)
	items := memory.Snapshot()
	if len(items) != 1 || items[0].Log == nil {
		t.Fatalf("unexpected memory snapshot: %#v", items)
	}
	log := items[0].Log
	if log.Timestamp != time.Unix(123, 456).UTC() || log.Source != "source.go:12" || log.CorrelationID != "request-7" || log.TaskID != "task-2" {
		t.Fatalf("log identity mismatch: %#v", log)
	}
	if log.Message != "password[REDACTED]" {
		t.Fatalf("message was not redacted: %q", log.Message)
	}
	if got := []string{log.Fields[0].Key, log.Fields[1].Key, log.Fields[2].Key, log.Fields[3].Key}; fmt.Sprint(got) != "[a api-key b z]" {
		t.Fatalf("fields are not stable: %v", got)
	}
	if log.Fields[1].Value != "[REDACTED]" {
		t.Fatalf("sensitive field leaked: %#v", log.Fields[1])
	}
}

func TestTask122StructuredErrorRedaction(t *testing.T) {
	memory := NewMemoryExporter(8)
	tel := task122TelemetryForTest(t, memory)
	ctx := WithTelemetryContext(context.Background(), tel, "corr", "task")
	err := newOsirisError(ErrorKindPermissionDenied, "authorization=top-secret")
	err = err.withMetadata("password", stringValue("hidden"))
	err = err.withMetadata("safe", stringValue("kept"))
	if got := tel.Logger().Error(ctx, "errors.go", "operation failed", err); got != nil {
		t.Fatal(got)
	}
	task122Flush(t, tel)
	record := memory.Snapshot()[0].Log.Error
	if record == nil || strings.Contains(fmt.Sprintf("%#v", record), "hidden") || strings.Contains(record.Message, "top-secret") {
		t.Fatalf("error leaked: %#v", record)
	}
	if len(record.Fields) != 1 || record.Fields[0].Key != "safe" {
		t.Fatalf("unexpected redacted fields: %#v", record.Fields)
	}
}

func TestTask122MetricsLimitsOrderingAndHistogram(t *testing.T) {
	memory := NewMemoryExporter(32)
	tel := task122TelemetryForTest(t, memory)
	tel.Metrics().limits.MaxSeries = 2
	counter, err := tel.Metrics().Counter("requests", TelemetryLabel{Key: "zone", Value: "b"}, TelemetryLabel{Key: "method", Value: "get"})
	if err != nil {
		t.Fatal(err)
	}
	if err := counter.Add(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	if _, err := tel.Metrics().Gauge("load"); err != nil {
		t.Fatal(err)
	}
	if _, err := tel.Metrics().Histogram("too-many"); !errors.Is(err, ErrTelemetryLimit) {
		t.Fatalf("expected cardinality limit, got %v", err)
	}
	hist, err := tel.Metrics().Histogram("latency", TelemetryLabel{Key: "method", Value: "get"})
	if !errors.Is(err, ErrTelemetryLimit) {
		t.Fatalf("expected series limit for histogram, got %v", err)
	}
	if hist != nil {
		t.Fatal("histogram unexpectedly created")
	}
	points := tel.Metrics().Snapshot()
	if len(points) != 2 || points[0].Name != "load" || points[1].Name != "requests" {
		t.Fatalf("metric ordering mismatch: %#v", points)
	}
	if points[1].Labels[0].Key != "method" || points[1].Counter != 3 {
		t.Fatalf("metric labels/value mismatch: %#v", points[1])
	}
}

func TestTask122MetricsHistogramStandaloneAndCancellation(t *testing.T) {
	metrics, err := NewMetrics(MetricsConfig{Limits: TelemetryLimits{MaxSeries: 4}, HistogramBuckets: []float64{1, 2, 4}})
	if err != nil {
		t.Fatal(err)
	}
	hist, err := metrics.Histogram("latency")
	if err != nil {
		t.Fatal(err)
	}
	if err := hist.Observe(context.Background(), 1.5); err != nil {
		t.Fatal(err)
	}
	if hist.Count() != 1 {
		t.Fatal(hist.Count())
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := hist.Observe(canceled, 2); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	point := metrics.Snapshot()[0]
	if point.Histogram == nil || point.Histogram.Buckets[0].Count != 0 || point.Histogram.Buckets[1].Count != 1 {
		t.Fatalf("histogram mismatch: %#v", point)
	}
}

func TestTask122TraceParentEventsCancellationAndPropagation(t *testing.T) {
	memory := NewMemoryExporter(32)
	tel := task122TelemetryForTest(t, memory)
	rootBase := WithTelemetryContext(context.Background(), tel, "corr-1", "task-1")
	rootCtx, root := tel.Tracer().Start(rootBase, "root", TelemetryField{Key: "z", Value: 2})
	childCtx, child := tel.Tracer().Start(rootCtx, "child")
	if child.ParentSpanID() != root.SpanID() || child.TraceID() != root.TraceID() {
		t.Fatalf("parent propagation failed: root=%#v child=%#v", root, child)
	}
	if got := TaskIDFromContext(childCtx); got != "task-1" {
		t.Fatalf("task context lost: %q", got)
	}
	if err := child.AddEvent("step", TelemetryField{Key: "b", Value: 2}, TelemetryField{Key: "a", Value: 1}); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(childCtx)
	cancel()
	if err := child.End(canceled); err != nil {
		t.Fatal(err)
	}
	root.SetStatus(SpanStatusOK, "done")
	if err := root.End(rootCtx); err != nil {
		t.Fatal(err)
	}
	task122Flush(t, tel)
	items := memory.Snapshot()
	if len(items) != 2 || items[0].Span == nil || items[1].Span == nil {
		t.Fatalf("span snapshot mismatch: %#v", items)
	}
	if items[0].Span.Status != SpanStatusCancelled || !items[0].Span.Cancelled {
		t.Fatalf("cancellation not recorded: %#v", items[0].Span)
	}
	if items[0].Span.Events[0].Fields[0].Key != "a" {
		t.Fatalf("event fields not ordered: %#v", items[0].Span.Events)
	}
	if items[1].Span.Status != SpanStatusOK {
		t.Fatalf("root status mismatch: %#v", items[1].Span)
	}
}

type task122BlockingExporter struct {
	started chan struct{}
	release chan struct{}
	mu      sync.Mutex
	items   int
	err     error
}

func (e *task122BlockingExporter) Export(_ context.Context, _ TelemetryItem) error {
	e.mu.Lock()
	e.items++
	first := e.items == 1
	e.mu.Unlock()
	if first {
		close(e.started)
		<-e.release
	}
	return e.err
}

func TestTask122BackpressureOverflowAndFailurePolicies(t *testing.T) {
	exporter := &task122BlockingExporter{started: make(chan struct{}), release: make(chan struct{})}
	cfg := DefaultTelemetryConfig()
	cfg.Exporter = exporter
	cfg.QueueSize = 1
	cfg.Backpressure = BackpressureDropNewest
	cfg.ExporterFailure = ExporterFailureReturn
	tel, err := NewTelemetry(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Close(context.Background())
	if err := tel.Logger().Info(context.Background(), "test", "first"); err != nil {
		t.Fatal(err)
	}
	<-exporter.started
	if err := tel.Logger().Info(context.Background(), "test", "queued"); err != nil {
		t.Fatal(err)
	}
	if err := tel.Logger().Info(context.Background(), "test", "overflow"); !errors.Is(err, ErrTelemetryQueueFull) {
		t.Fatalf("expected queue overflow, got %v", err)
	}
	if tel.Dropped() != 1 {
		t.Fatalf("drop count mismatch: %d", tel.Dropped())
	}
	close(exporter.release)
	if err := tel.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	failing := &task122BlockingExporter{started: make(chan struct{}), release: make(chan struct{}), err: errors.New("sink down")}
	failCfg := DefaultTelemetryConfig()
	failCfg.Exporter, failCfg.ExporterFailure = failing, ExporterFailureReturn
	failTel, err := NewTelemetry(failCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := failTel.Logger().Info(context.Background(), "test", "failure"); err != nil {
		t.Fatal(err)
	}
	<-failing.started
	close(failing.release)
	if err := failTel.Flush(context.Background()); !errors.Is(err, ErrTelemetryExporter) {
		t.Fatalf("expected exporter failure, got %v", err)
	}
	_ = failTel.Close(context.Background())
}

func TestTask122JSONExporterDeterministic(t *testing.T) {
	var output bytes.Buffer
	jsonExporter, err := NewJSONExporter(&output, 4096)
	if err != nil {
		t.Fatal(err)
	}
	tel := task122TelemetryForTest(t, jsonExporter)
	ctx := WithTelemetryContext(context.Background(), tel, "corr", "task")
	if err := tel.Logger().Info(ctx, "source", "hello", TelemetryField{Key: "b", Value: 2}, TelemetryField{Key: "a", Value: 1}); err != nil {
		t.Fatal(err)
	}
	task122Flush(t, tel)
	if !strings.Contains(output.String(), `"level":"info"`) || !strings.Contains(output.String(), `"fields":[{"key":"a"`) {
		t.Fatalf("unexpected JSON: %s", output.String())
	}
	if !strings.HasSuffix(output.String(), "\n") {
		t.Fatal("JSON exporter did not write line boundary")
	}
}

func TestTask122TelemetryConcurrent(t *testing.T) {
	memory := NewMemoryExporter(4096)
	tel := task122TelemetryForTest(t, memory)
	counter, err := tel.Metrics().Counter("concurrent")
	if err != nil {
		t.Fatal(err)
	}
	const workers = 8
	const iterations = 32
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				ctx := WithTaskTelemetry(WithTelemetryContext(context.Background(), tel, "corr", "task"), fmt.Sprintf("task-%d", worker))
				if err := counter.Add(ctx, 1); err != nil {
					t.Errorf("counter: %v", err)
				}
				if err := tel.Logger().Debug(ctx, "concurrent", "event", TelemetryField{Key: "worker", Value: worker}); err != nil {
					t.Errorf("log: %v", err)
				}
			}
		}(worker)
	}
	wg.Wait()
	task122Flush(t, tel)
	if counter.Value() != workers*iterations {
		t.Fatalf("counter mismatch: %d", counter.Value())
	}
	if got := len(memory.Snapshot()); got != workers*iterations+1 {
		t.Fatalf("record count mismatch: %d", got)
	}
}

func ExampleNewTelemetry() {
	memory := NewMemoryExporter(4)
	cfg := DefaultTelemetryConfig()
	cfg.Exporter = memory
	tel, _ := NewTelemetry(cfg)
	_ = tel.Logger().Info(context.Background(), "example", "ready")
	_ = tel.Flush(context.Background())
	_ = tel.Close(context.Background())
	fmt.Println(len(memory.Snapshot()))
	// Output: 1
}

func BenchmarkTask122TelemetryLog(b *testing.B) {
	memory := NewMemoryExporter(b.N + 1)
	cfg := DefaultTelemetryConfig()
	cfg.Exporter = memory
	cfg.QueueSize = b.N + 1
	tel, err := NewTelemetry(cfg)
	if err != nil {
		b.Fatal(err)
	}
	logger := tel.Logger()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := logger.Info(context.Background(), "benchmark", "event", TelemetryField{Key: "index", Value: i}); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if err := tel.Close(context.Background()); err != nil {
		b.Fatal(err)
	}
}
