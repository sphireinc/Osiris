package osiris

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
)

// MetricKind identifies a counter, gauge, or histogram series.
type MetricKind string

const (
	MetricKindCounter   MetricKind = "counter"
	MetricKindGauge     MetricKind = "gauge"
	MetricKindHistogram MetricKind = "histogram"
)

// HistogramBucket is one cumulative upper-bound bucket.
type HistogramBucket struct {
	UpperBound float64 `json:"upper_bound"`
	Count      uint64  `json:"count"`
}

// HistogramSnapshot is the bounded deterministic state of a histogram.
type HistogramSnapshot struct {
	Count   uint64            `json:"count"`
	Sum     float64           `json:"sum"`
	Buckets []HistogramBucket `json:"buckets"`
}

// MetricPoint is one deterministic metric series snapshot. Only the value
// matching Kind is meaningful.
type MetricPoint struct {
	Name      string             `json:"name"`
	Kind      MetricKind         `json:"kind"`
	Labels    []TelemetryLabel   `json:"labels,omitempty"`
	Counter   uint64             `json:"counter,omitempty"`
	Gauge     float64            `json:"gauge,omitempty"`
	Histogram *HistogramSnapshot `json:"histogram,omitempty"`
}

// MetricsConfig controls metric labels, series cardinality, and histogram
// bucket boundaries. Limits are inherited from TelemetryConfig when Metrics is
// obtained through Telemetry.Metrics.
type MetricsConfig struct {
	Limits           TelemetryLimits
	HistogramBuckets []float64
}

// DefaultMetricsConfig returns the default metric limits and stable latency
// buckets in seconds.
func DefaultMetricsConfig() MetricsConfig {
	return MetricsConfig{Limits: DefaultTelemetryLimits(), HistogramBuckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}}
}

// Metrics is a concurrency-safe bounded metric registry.
type Metrics struct {
	mu        sync.RWMutex
	telemetry *Telemetry
	limits    TelemetryLimits
	buckets   []float64
	series    map[metricSeriesKey]*metricSeries
}

// NewMetrics constructs a standalone bounded registry. Standalone registries
// provide Snapshot and handle operations but have no exporter until obtained
// from Telemetry.Metrics.
func NewMetrics(cfg MetricsConfig) (*Metrics, error) {
	limits, err := normalizeTelemetryLimits(cfg.Limits)
	if err != nil {
		return nil, err
	}
	buckets := cfg.HistogramBuckets
	if len(buckets) == 0 {
		buckets = DefaultMetricsConfig().HistogramBuckets
	}
	previous := math.Inf(-1)
	for _, bucket := range buckets {
		if math.IsNaN(bucket) || math.IsInf(bucket, 0) || bucket <= previous {
			return nil, fmt.Errorf("%w: histogram buckets must be finite and strictly increasing", ErrTelemetryInvalid)
		}
		previous = bucket
	}
	return &Metrics{limits: limits, buckets: append([]float64(nil), buckets...), series: make(map[metricSeriesKey]*metricSeries)}, nil
}

type metricSeriesKey struct {
	kind   MetricKind
	name   string
	labels string
}

type metricSeries struct {
	name      string
	kind      MetricKind
	labels    []TelemetryLabel
	counter   uint64
	gauge     float64
	histCount uint64
	histSum   float64
	hist      []HistogramBucket
}

// newTelemetryMetrics constructs the registry with Telemetry's shared limits.
func newTelemetryMetrics(t *Telemetry) *Metrics {
	limits := DefaultTelemetryLimits()
	if t != nil {
		limits = t.limits
	}
	metrics, _ := NewMetrics(MetricsConfig{Limits: limits})
	metrics.telemetry = t
	return metrics
}

func metricName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: empty metric name", ErrTelemetryInvalid)
	}
	return nil
}

func normalizeLabels(labels []TelemetryLabel, limits TelemetryLimits) ([]TelemetryLabel, string, error) {
	if len(labels) > limits.MaxLabels {
		return nil, "", fmt.Errorf("%w: %d labels, limit is %d", ErrTelemetryLimit, len(labels), limits.MaxLabels)
	}
	out := append([]TelemetryLabel(nil), labels...)
	seen := make(map[string]struct{}, len(out))
	for _, label := range out {
		if strings.TrimSpace(label.Key) == "" {
			return nil, "", fmt.Errorf("%w: empty metric label key", ErrTelemetryInvalid)
		}
		if len([]byte(label.Key)) > limits.MaxLabelKeyBytes || len([]byte(label.Value)) > limits.MaxLabelValueBytes {
			return nil, "", fmt.Errorf("%w: metric label %q exceeds byte limit", ErrTelemetryLimit, label.Key)
		}
		if _, ok := seen[label.Key]; ok {
			return nil, "", fmt.Errorf("%w: duplicate metric label %q", ErrTelemetryInvalid, label.Key)
		}
		seen[label.Key] = struct{}{}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	var key strings.Builder
	for _, label := range out {
		key.WriteString(label.Key)
		key.WriteByte(0)
		key.WriteString(label.Value)
		key.WriteByte(0)
	}
	return out, key.String(), nil
}

func (m *Metrics) get(kind MetricKind, name string, labels []TelemetryLabel) (*metricSeries, error) {
	if m == nil {
		return nil, ErrTelemetryInvalid
	}
	if err := metricName(name); err != nil {
		return nil, err
	}
	normalized, labelKey, err := normalizeLabels(labels, m.limits)
	if err != nil {
		return nil, err
	}
	key := metricSeriesKey{kind: kind, name: name, labels: labelKey}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing := m.series[key]; existing != nil {
		return existing, nil
	}
	if len(m.series) >= m.limits.MaxSeries {
		return nil, fmt.Errorf("%w: metric series limit is %d", ErrTelemetryLimit, m.limits.MaxSeries)
	}
	series := &metricSeries{name: name, kind: kind, labels: normalized}
	if kind == MetricKindHistogram {
		series.hist = make([]HistogramBucket, len(m.buckets))
		for i, upper := range m.buckets {
			series.hist[i].UpperBound = upper
		}
	}
	m.series[key] = series
	return series, nil
}

// Counter is a monotonic uint64 metric handle.
type Counter struct {
	metrics *Metrics
	series  *metricSeries
}

// Counter returns the named bounded counter series.
func (m *Metrics) Counter(name string, labels ...TelemetryLabel) (*Counter, error) {
	series, err := m.get(MetricKindCounter, name, labels)
	if err != nil {
		return nil, err
	}
	return &Counter{metrics: m, series: series}, nil
}

// Add increases the counter by delta after checking ctx cancellation.
func (c *Counter) Add(ctx context.Context, delta uint64) error {
	if c == nil || c.metrics == nil || c.series == nil {
		return ErrTelemetryInvalid
	}
	if err := telemetryContextError(ctx); err != nil {
		return err
	}
	c.metrics.mu.Lock()
	c.series.counter += delta
	c.metrics.mu.Unlock()
	return nil
}

// Value returns the current counter value.
func (c *Counter) Value() uint64 {
	if c == nil || c.metrics == nil || c.series == nil {
		return 0
	}
	c.metrics.mu.RLock()
	value := c.series.counter
	c.metrics.mu.RUnlock()
	return value
}

// Gauge is a settable float64 metric handle.
type Gauge struct {
	metrics *Metrics
	series  *metricSeries
}

// Gauge returns the named bounded gauge series.
func (m *Metrics) Gauge(name string, labels ...TelemetryLabel) (*Gauge, error) {
	series, err := m.get(MetricKindGauge, name, labels)
	if err != nil {
		return nil, err
	}
	return &Gauge{metrics: m, series: series}, nil
}

// Set replaces the gauge value after checking ctx cancellation. NaN and
// infinities are rejected so snapshots remain portable JSON numbers.
func (g *Gauge) Set(ctx context.Context, value float64) error {
	if g == nil || g.metrics == nil || g.series == nil {
		return ErrTelemetryInvalid
	}
	if err := telemetryContextError(ctx); err != nil {
		return err
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("%w: non-finite gauge", ErrTelemetryInvalid)
	}
	g.metrics.mu.Lock()
	g.series.gauge = value
	g.metrics.mu.Unlock()
	return nil
}

// Add changes the gauge value by delta after checking ctx cancellation.
func (g *Gauge) Add(ctx context.Context, delta float64) error {
	if g == nil || g.metrics == nil || g.series == nil {
		return ErrTelemetryInvalid
	}
	if err := telemetryContextError(ctx); err != nil {
		return err
	}
	if math.IsNaN(delta) || math.IsInf(delta, 0) {
		return fmt.Errorf("%w: non-finite gauge delta", ErrTelemetryInvalid)
	}
	g.metrics.mu.Lock()
	next := g.series.gauge + delta
	if math.IsInf(next, 0) {
		g.metrics.mu.Unlock()
		return fmt.Errorf("%w: gauge overflow", ErrTelemetryLimit)
	}
	g.series.gauge = next
	g.metrics.mu.Unlock()
	return nil
}

// Value returns the current gauge value.
func (g *Gauge) Value() float64 {
	if g == nil || g.metrics == nil || g.series == nil {
		return 0
	}
	g.metrics.mu.RLock()
	value := g.series.gauge
	g.metrics.mu.RUnlock()
	return value
}

// Histogram is a bounded cumulative-bucket distribution handle.
type Histogram struct {
	metrics *Metrics
	series  *metricSeries
}

// Histogram returns the named bounded histogram series.
func (m *Metrics) Histogram(name string, labels ...TelemetryLabel) (*Histogram, error) {
	series, err := m.get(MetricKindHistogram, name, labels)
	if err != nil {
		return nil, err
	}
	return &Histogram{metrics: m, series: series}, nil
}

// Observe records a finite value and updates all matching cumulative buckets.
func (h *Histogram) Observe(ctx context.Context, value float64) error {
	if h == nil || h.metrics == nil || h.series == nil {
		return ErrTelemetryInvalid
	}
	if err := telemetryContextError(ctx); err != nil {
		return err
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("%w: non-finite histogram value", ErrTelemetryInvalid)
	}
	h.metrics.mu.Lock()
	h.series.histCount++
	h.series.histSum += value
	for i := range h.series.hist {
		if value <= h.series.hist[i].UpperBound {
			h.series.hist[i].Count++
		}
	}
	h.metrics.mu.Unlock()
	return nil
}

// Count returns the number of observations.
func (h *Histogram) Count() uint64 {
	if h == nil || h.metrics == nil || h.series == nil {
		return 0
	}
	h.metrics.mu.RLock()
	value := h.series.histCount
	h.metrics.mu.RUnlock()
	return value
}

// Snapshot returns all metric series sorted by kind, name, and labels.
func (m *Metrics) Snapshot() []MetricPoint {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	out := make([]MetricPoint, 0, len(m.series))
	for _, series := range m.series {
		point := MetricPoint{Name: series.name, Kind: series.kind, Labels: append([]TelemetryLabel(nil), series.labels...), Counter: series.counter, Gauge: series.gauge}
		if series.kind == MetricKindHistogram {
			hist := &HistogramSnapshot{Count: series.histCount, Sum: series.histSum, Buckets: append([]HistogramBucket(nil), series.hist...)}
			point.Histogram = hist
		}
		out = append(out, point)
	}
	m.mu.RUnlock()
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return labelsKey(out[i].Labels) < labelsKey(out[j].Labels)
	})
	return out
}

// Flush exports a deterministic point for every current series.
func (m *Metrics) Flush(ctx context.Context) error {
	if m == nil || m.telemetry == nil {
		return nil
	}
	for _, point := range m.Snapshot() {
		seq := m.telemetry.nextSequence()
		if err := m.telemetry.dispatch.submit(ctx, TelemetryItem{Sequence: seq, Kind: "metric", Metric: &point}); err != nil {
			return err
		}
	}
	return nil
}

func labelsKey(labels []TelemetryLabel) string {
	var b strings.Builder
	for _, label := range labels {
		b.WriteString(label.Key)
		b.WriteByte(0)
		b.WriteString(label.Value)
		b.WriteByte(0)
	}
	return b.String()
}

func telemetryContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
