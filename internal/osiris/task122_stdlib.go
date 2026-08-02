package osiris

import (
	"context"
	"encoding/json"
	"fmt"
)

type task122SpanHandle struct{ span *Span }

func (h *task122SpanHandle) Close() error {
	if h == nil || h.span == nil {
		return nil
	}
	return h.span.End(context.Background())
}

func task122RuntimeTelemetry(rt *Runtime) (*Telemetry, error) {
	if rt == nil {
		return nil, ErrTelemetryInvalid
	}
	if telemetry := rt.Telemetry(); telemetry != nil {
		return telemetry, nil
	}
	cfg := DefaultTelemetryConfig()
	cfg.Clock = rt.clockNow
	telemetry, err := NewTelemetry(cfg)
	if err != nil {
		return nil, err
	}
	rt.SetTelemetry(telemetry)
	return telemetry, nil
}

func task122Any(value Value) any {
	switch value.kind {
	case kindNull:
		return nil
	case kindBool:
		return value.boo
	case kindString:
		return value.str
	case kindNumber:
		return value.num
	case kindInt:
		if value.i != nil {
			return value.i.String()
		}
		return "0"
	case kindUint:
		if value.ub != nil {
			return value.ub.String()
		}
		return value.u
	case kindList:
		items := make([]any, len(value.list))
		for i, item := range value.list {
			items[i] = task122Any(item)
		}
		return items
	case kindMap:
		entries := make(map[string]any, len(value.mapValue.entries))
		for _, entry := range value.mapValue.entries {
			if entry.key.kind == kindString {
				entries[entry.key.str] = task122Any(entry.value)
			}
		}
		return entries
	default:
		return value.String()
	}
}

func task122Fields(value Value) ([]TelemetryField, error) {
	if value.kind == kindNull {
		return nil, nil
	}
	if value.kind != kindMap || value.mapValue == nil {
		return nil, fmt.Errorf("telemetry fields expect a map")
	}
	fields := make([]TelemetryField, 0, len(value.mapValue.entries))
	for _, entry := range value.mapValue.entries {
		key, err := taskStringArg(entry.key, "telemetry field key")
		if err != nil {
			return nil, err
		}
		fields = append(fields, TelemetryField{Key: key, Value: task122Any(entry.value)})
	}
	return fields, nil
}

func task122Labels(value Value) ([]TelemetryLabel, error) {
	if value.kind == kindNull {
		return nil, nil
	}
	if value.kind != kindMap || value.mapValue == nil {
		return nil, fmt.Errorf("telemetry labels expect a map")
	}
	labels := make([]TelemetryLabel, 0, len(value.mapValue.entries))
	for _, entry := range value.mapValue.entries {
		key, err := taskStringArg(entry.key, "telemetry label key")
		if err != nil {
			return nil, err
		}
		value, err := taskStringArg(entry.value, "telemetry label value")
		if err != nil {
			return nil, err
		}
		labels = append(labels, TelemetryLabel{Key: key, Value: value})
	}
	return labels, nil
}

func task122JSONValue(value any) (Value, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nullValue(), err
	}
	return jsonToValue(json.RawMessage(raw))
}

func task122MemorySnapshot(telemetry *Telemetry) (Value, error) {
	memory := telemetry.MemoryExporter()
	if memory == nil {
		return listValue(nil), nil
	}
	return task122JSONValue(memory.Snapshot())
}

func task122MetricLabels(args []Value) ([]TelemetryLabel, error) {
	if len(args) < 3 {
		return nil, nil
	}
	return task122Labels(args[2])
}

func task122TelemetryModule(rt *Runtime) map[string]Value {
	ensure := func() (*Telemetry, error) { return task122RuntimeTelemetry(rt) }
	log := func(level TelemetryLevel, ctx context.Context, args []Value) (Value, error) {
		if len(args) < 2 || len(args) > 3 {
			return nullValue(), fmt.Errorf("telemetry log expects message, source, and optional fields")
		}
		telemetry, err := ensure()
		if err != nil {
			return nullValue(), err
		}
		message, err := taskStringArg(args[0], "log message")
		if err != nil {
			return nullValue(), err
		}
		source, err := taskStringArg(args[1], "log source")
		if err != nil {
			return nullValue(), err
		}
		var fields []TelemetryField
		if len(args) == 3 {
			fields, err = task122Fields(args[2])
			if err != nil {
				return nullValue(), err
			}
		}
		err = telemetry.Logger().Log(ctx, level, source, message, fields...)
		return boolValue(err == nil), err
	}
	return map[string]Value{
		"version": stringValue("1.0.0"),
		"new":     functionValueOf("std/telemetry.new", func([]Value) (Value, error) { _, err := ensure(); return boolValue(err == nil), err }),
		"debug":   contextFunctionValueOf("std/telemetry.debug", func(ctx context.Context, args []Value) (Value, error) { return log(TelemetryLevelDebug, ctx, args) }),
		"info":    contextFunctionValueOf("std/telemetry.info", func(ctx context.Context, args []Value) (Value, error) { return log(TelemetryLevelInfo, ctx, args) }),
		"warn":    contextFunctionValueOf("std/telemetry.warn", func(ctx context.Context, args []Value) (Value, error) { return log(TelemetryLevelWarn, ctx, args) }),
		"error":   contextFunctionValueOf("std/telemetry.error", func(ctx context.Context, args []Value) (Value, error) { return log(TelemetryLevelError, ctx, args) }),
		"counter_add": contextFunctionValueOf("std/telemetry.counter_add", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) < 2 || len(args) > 3 {
				return nullValue(), fmt.Errorf("counter_add expects name, delta, and optional labels")
			}
			telemetry, err := ensure()
			if err != nil {
				return nullValue(), err
			}
			name, err := taskStringArg(args[0], "counter name")
			if err != nil {
				return nullValue(), err
			}
			delta, err := args[1].Int()
			if err != nil || delta < 0 {
				return nullValue(), fmt.Errorf("counter delta must be a non-negative integer")
			}
			labels, err := task122MetricLabels(args)
			if err != nil {
				return nullValue(), err
			}
			counter, err := telemetry.Metrics().Counter(name, labels...)
			if err != nil {
				return nullValue(), err
			}
			return boolValue(true), counter.Add(ctx, uint64(delta))
		}),
		"gauge_set": contextFunctionValueOf("std/telemetry.gauge_set", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) < 2 || len(args) > 3 {
				return nullValue(), fmt.Errorf("gauge_set expects name, value, and optional labels")
			}
			telemetry, err := ensure()
			if err != nil {
				return nullValue(), err
			}
			name, err := taskStringArg(args[0], "gauge name")
			if err != nil {
				return nullValue(), err
			}
			number, err := args[1].AsNumber()
			if err != nil {
				return nullValue(), err
			}
			labels, err := task122MetricLabels(args)
			if err != nil {
				return nullValue(), err
			}
			gauge, err := telemetry.Metrics().Gauge(name, labels...)
			if err != nil {
				return nullValue(), err
			}
			return boolValue(true), gauge.Set(ctx, number)
		}),
		"histogram_observe": contextFunctionValueOf("std/telemetry.histogram_observe", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) < 2 || len(args) > 3 {
				return nullValue(), fmt.Errorf("histogram_observe expects name, value, and optional labels")
			}
			telemetry, err := ensure()
			if err != nil {
				return nullValue(), err
			}
			name, err := taskStringArg(args[0], "histogram name")
			if err != nil {
				return nullValue(), err
			}
			number, err := args[1].AsNumber()
			if err != nil {
				return nullValue(), err
			}
			labels, err := task122MetricLabels(args)
			if err != nil {
				return nullValue(), err
			}
			histogram, err := telemetry.Metrics().Histogram(name, labels...)
			if err != nil {
				return nullValue(), err
			}
			return boolValue(true), histogram.Observe(ctx, number)
		}),
		"metrics": functionValueOf("std/telemetry.metrics", func([]Value) (Value, error) {
			telemetry, err := ensure()
			if err != nil {
				return nullValue(), err
			}
			return task122JSONValue(telemetry.Metrics().Snapshot())
		}),
		"snapshot": functionValueOf("std/telemetry.snapshot", func([]Value) (Value, error) {
			telemetry, err := ensure()
			if err != nil {
				return nullValue(), err
			}
			return task122MemorySnapshot(telemetry)
		}),
		"flush": contextFunctionValueOf("std/telemetry.flush", func(ctx context.Context, _ []Value) (Value, error) {
			telemetry, err := ensure()
			if err != nil {
				return nullValue(), err
			}
			return boolValue(true), telemetry.Flush(ctx)
		}),
		"dropped": functionValueOf("std/telemetry.dropped", func([]Value) (Value, error) {
			telemetry, err := ensure()
			if err != nil {
				return nullValue(), err
			}
			return uintValue(telemetry.Dropped(), 64), nil
		}),
		"start_span": functionValueOf("std/telemetry.start_span", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("start_span expects name")
			}
			telemetry, err := ensure()
			if err != nil {
				return nullValue(), err
			}
			name, err := taskStringArg(args[0], "span name")
			if err != nil {
				return nullValue(), err
			}
			_, span := telemetry.Tracer().Start(context.Background(), name)
			return task121Handle("telemetry-span", "telemetry-span", &task122SpanHandle{span: span}), nil
		}),
		"span_attribute": functionValueOf("std/telemetry.span_attribute", func(args []Value) (Value, error) {
			if len(args) != 3 {
				return nullValue(), fmt.Errorf("span_attribute expects span, key, value")
			}
			resource, err := task121Resource(args[0])
			if err != nil {
				return nullValue(), err
			}
			handle, ok := resource.(*task122SpanHandle)
			if !ok {
				return nullValue(), fmt.Errorf("expected telemetry span")
			}
			key, err := taskStringArg(args[1], "span attribute key")
			if err != nil {
				return nullValue(), err
			}
			return boolValue(true), handle.span.SetAttribute(key, task122Any(args[2]))
		}),
		"span_event": functionValueOf("std/telemetry.span_event", func(args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("span_event expects span and name")
			}
			resource, err := task121Resource(args[0])
			if err != nil {
				return nullValue(), err
			}
			handle, ok := resource.(*task122SpanHandle)
			if !ok {
				return nullValue(), fmt.Errorf("expected telemetry span")
			}
			name, err := taskStringArg(args[1], "span event name")
			if err != nil {
				return nullValue(), err
			}
			return boolValue(true), handle.span.AddEvent(name)
		}),
		"span_end": functionValueOf("std/telemetry.span_end", closeBuiltinValue),
		"correlation_id": contextFunctionValueOf("std/telemetry.correlation_id", func(ctx context.Context, _ []Value) (Value, error) {
			state := telemetryStateFromContext(ctx)
			if state == nil {
				return stringValue(""), nil
			}
			return stringValue(state.correlationID), nil
		}),
		"task_id": contextFunctionValueOf("std/telemetry.task_id", func(ctx context.Context, _ []Value) (Value, error) {
			state := telemetryStateFromContext(ctx)
			if state == nil {
				return stringValue(""), nil
			}
			return stringValue(state.taskID), nil
		}),
		"span_context": functionValueOf("std/telemetry.span_context", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("span_context expects span")
			}
			resource, err := task121Resource(args[0])
			if err != nil {
				return nullValue(), err
			}
			handle, ok := resource.(*task122SpanHandle)
			if !ok {
				return nullValue(), fmt.Errorf("expected telemetry span")
			}
			return mapValueOf([]mapEntry{{key: stringValue("trace_id"), value: stringValue(handle.span.TraceID())}, {key: stringValue("span_id"), value: stringValue(handle.span.SpanID())}, {key: stringValue("parent_span_id"), value: stringValue(handle.span.ParentSpanID())}}), nil
		}),
		"close": functionValueOf("std/telemetry.close", func([]Value) (Value, error) {
			telemetry, err := ensure()
			if err != nil {
				return nullValue(), err
			}
			return boolValue(true), telemetry.Close(context.Background())
		}),
	}
}
