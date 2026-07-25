package osiris

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ErrorKind string

const (
	ErrorKindUnknown           ErrorKind = "unknown_error"
	ErrorKindInvalidArgument   ErrorKind = "invalid_argument"
	ErrorKindNotFound          ErrorKind = "not_found"
	ErrorKindAlreadyExists     ErrorKind = "already_exists"
	ErrorKindPermissionDenied  ErrorKind = "permission_denied"
	ErrorKindUnauthenticated   ErrorKind = "unauthenticated"
	ErrorKindUnavailable       ErrorKind = "unavailable"
	ErrorKindTimeout           ErrorKind = "timeout"
	ErrorKindCancelled         ErrorKind = "cancelled"
	ErrorKindConflict          ErrorKind = "conflict"
	ErrorKindResourceExhausted ErrorKind = "resource_exhausted"
	ErrorKindOutOfMemory       ErrorKind = "out_of_memory"
	ErrorKindOutOfRange        ErrorKind = "out_of_range"
	ErrorKindUnsupported       ErrorKind = "unsupported"
	ErrorKindNotImplemented    ErrorKind = "not_implemented"
	ErrorKindParse             ErrorKind = "parse_error"
	ErrorKindType              ErrorKind = "type_error"
	ErrorKindEncoding          ErrorKind = "encoding_error"
	ErrorKindIO                ErrorKind = "io_error"
	ErrorKindNetwork           ErrorKind = "network_error"
	ErrorKindDatabase          ErrorKind = "database_error"
	ErrorKindInternal          ErrorKind = "internal_error"
)

type ErrorSeverity string

const (
	ErrorSeverityInfo    ErrorSeverity = "info"
	ErrorSeverityWarning ErrorSeverity = "warning"
	ErrorSeverityError   ErrorSeverity = "error"
	ErrorSeverityFatal   ErrorSeverity = "fatal"
)

// ErrorLocation captures where an error originated when source information is
// available. The source name is typically the Osiris file or module name and
// the span points at the source range that most directly explains the failure.
type ErrorLocation struct {
	Source string
	Span   sourceSpan
}

// String renders the location into a compact source:line:col form that is
// stable enough for logs and human inspection.
func (l ErrorLocation) String() string {
	if l.Source == "" && l.Span.start.line == 0 && l.Span.start.col == 0 && l.Span.end.line == 0 && l.Span.end.col == 0 {
		return ""
	}
	var b strings.Builder
	if l.Source != "" {
		b.WriteString(l.Source)
		if l.Span.start.line > 0 {
			b.WriteByte(':')
		}
	}
	if l.Span.start.line > 0 {
		b.WriteString(strconv.Itoa(l.Span.start.line))
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(l.Span.start.col))
		if l.Span.end.line > 0 || l.Span.end.col > 0 {
			b.WriteByte('-')
			b.WriteString(strconv.Itoa(l.Span.end.line))
			b.WriteByte(':')
			b.WriteString(strconv.Itoa(l.Span.end.col))
		}
	}
	if b.Len() == 0 {
		b.WriteString(l.Source)
	}
	return b.String()
}

// ErrorFrame captures one frame in a captured error stack trace. Frames are
// intentionally small and immutable so they can be copied across task and FFI
// boundaries without extra bookkeeping.
type ErrorFrame struct {
	Name     string
	Location ErrorLocation
}

// String renders the frame into a compact human-readable form.
func (f ErrorFrame) String() string {
	if loc := f.Location.String(); loc != "" {
		if f.Name != "" {
			return f.Name + " @ " + loc
		}
		return loc
	}
	return f.Name
}

// OsirisError is the structured error record used by the language runtime.
// It keeps the stable user-facing message, machine-readable kind/code, cause
// chain, source location, stack frames, and metadata in one immutable record.
type OsirisError struct {
	kind ErrorKind
	code string

	message     string
	userMessage string

	cause *OsirisError

	location *ErrorLocation
	stack    []ErrorFrame

	metadata      map[string]Value
	debugMetadata map[string]Value

	operation     string
	resource      string
	retryable     *bool
	severity      ErrorSeverity
	correlationID string
	timestamp     time.Time
	component     string
	remediation   string
}

// newOsirisError constructs a structured error record with a stable kind/code
// pair and message. The returned record is intentionally unexported and should
// be treated as immutable by callers.
func newOsirisError(kind ErrorKind, message string) *OsirisError {
	if kind == "" {
		kind = ErrorKindUnknown
	}
	return &OsirisError{
		kind:     kind,
		code:     string(kind),
		message:  message,
		severity: ErrorSeverityError,
	}
}

// clone returns a deep copy of the error record so callers can safely attach
// additional metadata without mutating the original error value.
func (e *OsirisError) clone() *OsirisError {
	if e == nil {
		return nil
	}
	out := *e
	if e.location != nil {
		loc := *e.location
		out.location = &loc
	}
	if len(e.stack) > 0 {
		out.stack = append([]ErrorFrame(nil), e.stack...)
	}
	if len(e.metadata) > 0 {
		out.metadata = make(map[string]Value, len(e.metadata))
		for k, v := range e.metadata {
			out.metadata[k] = v
		}
	}
	if len(e.debugMetadata) > 0 {
		out.debugMetadata = make(map[string]Value, len(e.debugMetadata))
		for k, v := range e.debugMetadata {
			out.debugMetadata[k] = v
		}
	}
	if e.cause != nil {
		out.cause = e.cause.clone()
	}
	if e.retryable != nil {
		v := *e.retryable
		out.retryable = &v
	}
	return &out
}

// withCause returns a copy of the error with the supplied cause attached.
func (e *OsirisError) withCause(cause *OsirisError) *OsirisError {
	out := e.clone()
	if out == nil {
		out = newOsirisError(ErrorKindUnknown, "")
	}
	if cause != nil {
		out.cause = cause.clone()
	}
	return out
}

// withSource returns a copy of the error with the supplied source name and
// source span attached.
func (e *OsirisError) withSource(source string, span sourceSpan) *OsirisError {
	out := e.clone()
	if out == nil {
		out = newOsirisError(ErrorKindUnknown, "")
	}
	out.location = &ErrorLocation{Source: source, Span: span}
	return out
}

// withStack returns a copy of the error with the supplied stack trace
// attached. The trace is copied so later callers cannot mutate the original
// frames.
func (e *OsirisError) withStack(stack []ErrorFrame) *OsirisError {
	out := e.clone()
	if out == nil {
		out = newOsirisError(ErrorKindUnknown, "")
	}
	if len(stack) == 0 {
		out.stack = nil
		return out
	}
	out.stack = append([]ErrorFrame(nil), stack...)
	return out
}

// withMetadata returns a copy of the error with a new public metadata entry
// attached. Recognized keys update the canonical fields on the record; all
// other keys are stored in the generic metadata map.
func (e *OsirisError) withMetadata(key string, value Value) *OsirisError {
	out := e.clone()
	if out == nil {
		out = newOsirisError(ErrorKindUnknown, "")
	}
	if out.metadata == nil {
		out.metadata = make(map[string]Value)
	}
	switch canonicalErrorMetaKey(key) {
	case "operation":
		out.operation = errorStringValue(value)
	case "resource":
		out.resource = errorStringValue(value)
	case "retryable":
		if value.kind == kindNull {
			out.retryable = nil
		} else {
			v := value.Truthy()
			out.retryable = &v
		}
	case "severity":
		if value.kind == kindNull {
			out.severity = ""
		} else {
			out.severity = ErrorSeverity(strings.ToLower(errorStringValue(value)))
		}
	case "user_message", "user-message":
		out.userMessage = errorStringValue(value)
	case "correlation_id", "correlation-id":
		out.correlationID = errorStringValue(value)
	case "component":
		out.component = errorStringValue(value)
	case "remediation", "hint":
		out.remediation = errorStringValue(value)
	case "source", "location":
		if value.kind == kindNull {
			out.location = nil
		} else {
			out.location = &ErrorLocation{Source: errorStringValue(value)}
		}
	case "timestamp":
		switch {
		case value.kind == kindNull:
			out.timestamp = time.Time{}
		case value.kind == kindString:
			if ts, err := parseErrorTimestamp(value.str); err == nil {
				out.timestamp = ts
			} else {
				out.metadata[key] = value
			}
		case value.kind == kindNumber:
			out.timestamp = time.Unix(int64(value.num), 0).UTC()
		default:
			out.metadata[key] = value
		}
	default:
		out.metadata[key] = value
	}
	return out
}

// withDebugMetadata returns a copy of the error with a new debug-only
// metadata entry attached. Debug metadata is preserved for inspection but
// omitted from redacted output.
func (e *OsirisError) withDebugMetadata(key string, value Value) *OsirisError {
	out := e.clone()
	if out == nil {
		out = newOsirisError(ErrorKindUnknown, "")
	}
	if out.debugMetadata == nil {
		out.debugMetadata = make(map[string]Value)
	}
	out.debugMetadata[key] = value
	return out
}

// displayMessage returns the message that should appear in user-facing
// stringification. A user message overrides the technical message when it has
// been provided.
func (e *OsirisError) displayMessage() string {
	if e == nil {
		return ""
	}
	if e.userMessage != "" {
		return e.userMessage
	}
	return e.message
}

// summaryString formats the public error summary. It intentionally keeps stack
// traces and debug metadata out of the main display path so normal output
// stays readable.
func (e *OsirisError) summaryString() string {
	if e == nil {
		return "error[unknown_error](\"\")"
	}
	var b strings.Builder
	b.WriteString("error[")
	b.WriteString(string(e.kind))
	b.WriteString("](\"")
	b.WriteString(escapeErrorText(e.displayMessage()))
	b.WriteString("\")")
	if e.location != nil {
		if loc := e.location.String(); loc != "" {
			b.WriteString(" @ ")
			b.WriteString(loc)
		}
	}
	if e.cause != nil {
		b.WriteString(" <- ")
		b.WriteString(e.cause.summaryString())
	}
	return b.String()
}

// Error implements the Go error interface with the public, redacted summary
// form so direct stderr printing does not leak debug metadata or stack traces.
func (e *OsirisError) Error() string { return e.summaryString() }

// String renders the same public summary as Error so Osiris values and Go
// callers see consistent output.
func (e *OsirisError) String() string { return e.summaryString() }

// Equal compares two error records structurally while ignoring volatile
// details such as timestamps and stack traces. This makes error comparison
// useful in tests and across task boundaries without treating debug-only data
// as part of the semantic contract.
func (e *OsirisError) Equal(other *OsirisError) bool {
	if e == nil || other == nil {
		return e == nil && other == nil
	}
	if e.kind != other.kind || e.code != other.code || e.message != other.message || e.userMessage != other.userMessage || e.operation != other.operation || e.resource != other.resource || e.severity != other.severity || e.correlationID != other.correlationID || e.component != other.component || e.remediation != other.remediation {
		return false
	}
	switch {
	case e.retryable == nil && other.retryable == nil:
	case e.retryable == nil || other.retryable == nil:
		return false
	case *e.retryable != *other.retryable:
		return false
	}
	if !errorLocationEqual(e.location, other.location) {
		return false
	}
	if !errorMetadataEqual(e.metadata, other.metadata) {
		return false
	}
	return e.cause.Equal(other.cause)
}

// Redacted returns a copy of the error with debug metadata and stack traces
// removed. Public metadata and the cause chain are preserved, so the caller
// still has a useful error summary without the sensitive details.
func (e *OsirisError) Redacted() *OsirisError {
	if e == nil {
		return nil
	}
	out := e.clone()
	out.debugMetadata = nil
	out.stack = nil
	if out.cause != nil {
		out.cause = out.cause.Redacted()
	}
	return out
}

// publicPairs returns the public inspection representation as a stable list
// of key/value pairs. The list is sorted and suitable for logs, debugging, and
// user-facing inspection without exposing debug-only metadata.
func (e *OsirisError) publicPairs() Value {
	if e == nil {
		return listValue(nil)
	}
	pairs := []Value{
		errorPair("kind", stringValue(string(e.kind))),
		errorPair("code", stringValue(e.code)),
		errorPair("message", stringValue(e.message)),
	}
	if e.userMessage != "" {
		pairs = append(pairs, errorPair("user_message", stringValue(e.userMessage)))
	}
	if e.operation != "" {
		pairs = append(pairs, errorPair("operation", stringValue(e.operation)))
	}
	if e.resource != "" {
		pairs = append(pairs, errorPair("resource", stringValue(e.resource)))
	}
	if e.retryable != nil {
		pairs = append(pairs, errorPair("retryable", boolValue(*e.retryable)))
	}
	if e.severity != "" {
		pairs = append(pairs, errorPair("severity", stringValue(string(e.severity))))
	}
	if e.correlationID != "" {
		pairs = append(pairs, errorPair("correlation_id", stringValue(e.correlationID)))
	}
	if e.component != "" {
		pairs = append(pairs, errorPair("component", stringValue(e.component)))
	}
	if e.remediation != "" {
		pairs = append(pairs, errorPair("remediation", stringValue(e.remediation)))
	}
	if e.location != nil && e.location.Source != "" {
		pairs = append(pairs, errorPair("source", stringValue(e.location.Source)))
	}
	if e.location != nil && e.location.String() != "" {
		pairs = append(pairs, errorPair("location", stringValue(e.location.String())))
	}
	if e.cause != nil {
		pairs = append(pairs, errorPair("cause", errorValueFromRecord(e.cause)))
	}
	if !e.timestamp.IsZero() {
		pairs = append(pairs, errorPair("timestamp", stringValue(e.timestamp.UTC().Format(time.RFC3339Nano))))
	}
	if len(e.stack) > 0 {
		stack := make([]Value, len(e.stack))
		for i, frame := range e.stack {
			stack[i] = stringValue(frame.String())
		}
		pairs = append(pairs, errorPair("stack", listValue(stack)))
	}
	if len(e.metadata) > 0 {
		pairs = append(pairs, errorPair("metadata", metadataMapValue(e.metadata)))
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		return pairKey(pairs[i]) < pairKey(pairs[j])
	})
	return listValue(pairs)
}

// publicInterface converts the public portion of the error into a JSON-safe
// representation. The result intentionally excludes debug-only metadata so
// serializing an error for logs or APIs does not leak sensitive details.
func (e *OsirisError) publicInterface() map[string]any {
	if e == nil {
		return map[string]any{"error": ""}
	}
	out := map[string]any{
		"error":   e.displayMessage(),
		"kind":    string(e.kind),
		"code":    e.code,
		"message": e.message,
	}
	if e.userMessage != "" {
		out["user_message"] = e.userMessage
	}
	if e.operation != "" {
		out["operation"] = e.operation
	}
	if e.resource != "" {
		out["resource"] = e.resource
	}
	if e.retryable != nil {
		out["retryable"] = *e.retryable
	}
	if e.severity != "" {
		out["severity"] = string(e.severity)
	}
	if e.correlationID != "" {
		out["correlation_id"] = e.correlationID
	}
	if e.component != "" {
		out["component"] = e.component
	}
	if e.remediation != "" {
		out["remediation"] = e.remediation
	}
	if e.location != nil && e.location.String() != "" {
		out["source"] = e.location.Source
		out["location"] = e.location.String()
	}
	if e.cause != nil {
		out["cause"] = e.cause.publicInterface()
	}
	if !e.timestamp.IsZero() {
		out["timestamp"] = e.timestamp.UTC().Format(time.RFC3339Nano)
	}
	if len(e.stack) > 0 {
		stack := make([]string, len(e.stack))
		for i, frame := range e.stack {
			stack[i] = frame.String()
		}
		out["stack"] = stack
	}
	if len(e.metadata) > 0 {
		meta := make(map[string]any, len(e.metadata))
		for k, v := range e.metadata {
			meta[k] = v.toInterface()
		}
		out["metadata"] = meta
	}
	return out
}

// member resolves structured fields on error values so Osiris code can inspect
// errors with ordinary member access.
func (e *OsirisError) member(name string) (Value, bool) {
	if e == nil {
		return nullValue(), false
	}
	switch canonicalErrorMetaKey(name) {
	case "kind", "code":
		return stringValue(string(e.kind)), true
	case "message":
		return stringValue(e.message), true
	case "user_message", "user-message":
		if e.userMessage == "" {
			return nullValue(), true
		}
		return stringValue(e.userMessage), true
	case "cause", "unwrap":
		if e.cause == nil {
			return nullValue(), true
		}
		return errorValueFromRecord(e.cause), true
	case "source":
		if e.location == nil || e.location.Source == "" {
			return nullValue(), true
		}
		return stringValue(e.location.Source), true
	case "location":
		if e.location == nil || e.location.String() == "" {
			return nullValue(), true
		}
		return stringValue(e.location.String()), true
	case "stack":
		stack := make([]Value, len(e.stack))
		for i, frame := range e.stack {
			stack[i] = stringValue(frame.String())
		}
		return listValue(stack), true
	case "metadata":
		return metadataMapValue(e.metadata), true
	case "debug":
		return metadataMapValue(e.debugMetadata), true
	case "operation":
		if e.operation == "" {
			return nullValue(), true
		}
		return stringValue(e.operation), true
	case "resource":
		if e.resource == "" {
			return nullValue(), true
		}
		return stringValue(e.resource), true
	case "retryable":
		if e.retryable == nil {
			return nullValue(), true
		}
		return boolValue(*e.retryable), true
	case "severity":
		if e.severity == "" {
			return nullValue(), true
		}
		return stringValue(string(e.severity)), true
	case "correlation_id", "correlation-id":
		if e.correlationID == "" {
			return nullValue(), true
		}
		return stringValue(e.correlationID), true
	case "component":
		if e.component == "" {
			return nullValue(), true
		}
		return stringValue(e.component), true
	case "remediation", "hint":
		if e.remediation == "" {
			return nullValue(), true
		}
		return stringValue(e.remediation), true
	case "timestamp":
		if e.timestamp.IsZero() {
			return nullValue(), true
		}
		return stringValue(e.timestamp.UTC().Format(time.RFC3339Nano)), true
	case "redacted":
		return errorValueFromRecord(e.Redacted()), true
	default:
		return nullValue(), false
	}
}

// errorValueFromRecord converts a structured error record into an Osiris value
// while preserving the record so the runtime can inspect, compare, and
// serialize it later.
func errorValueFromRecord(rec *OsirisError) Value {
	if rec == nil {
		rec = newOsirisError(ErrorKindUnknown, "")
	}
	return Value{kind: kindError, err: rec.clone(), errMsg: rec.displayMessage()}
}

// errorValueFromGoError converts any Go error into an Osiris error value while
// preserving structured Osiris errors and classifying common runtime failures
// into the standard taxonomy.
func errorValueFromGoError(err error) Value {
	if err == nil {
		return nullValue()
	}
	var oe *OsirisError
	if errors.As(err, &oe) {
		return errorValueFromRecord(oe)
	}
	kind := classifyGoError(err)
	return errorValueFromRecord(newOsirisError(kind, err.Error()))
}

// normalizeError turns a Go error into an OsirisError when possible and
// annotates it with the provided source name. Callers should use this at
// public runtime boundaries so ordinary Go errors become structured Osiris
// errors before they leave the runtime.
func normalizeError(err error, source string) error {
	if err == nil {
		return nil
	}
	var oe *OsirisError
	if errors.As(err, &oe) {
		if source != "" && (oe.location == nil || oe.location.Source == "") {
			return oe.withSource(source, sourceSpan{})
		}
		return oe
	}
	rec := newOsirisError(classifyGoError(err), err.Error())
	if source != "" {
		rec = rec.withSource(source, sourceSpan{})
	}
	return rec
}

// isFatalError reports whether the supplied error record is marked fatal.
// Fatal errors are treated as unrecoverable and should not be swallowed by
// ordinary try/catch handling.
func isFatalError(err error) bool {
	var oe *OsirisError
	if !errors.As(err, &oe) || oe == nil {
		return false
	}
	return oe.severity == ErrorSeverityFatal
}

// classifyGoError maps a Go error into the standard Osiris taxonomy when a
// better kind is obvious from the sentinel error value.
func classifyGoError(err error) ErrorKind {
	switch {
	case errors.Is(err, context.Canceled):
		return ErrorKindCancelled
	case errors.Is(err, context.DeadlineExceeded):
		return ErrorKindTimeout
	case errors.Is(err, os.ErrNotExist):
		return ErrorKindNotFound
	case errors.Is(err, os.ErrPermission):
		return ErrorKindPermissionDenied
	case errors.Is(err, os.ErrExist):
		return ErrorKindAlreadyExists
	case errors.Is(err, io.EOF):
		return ErrorKindOutOfRange
	default:
		return ErrorKindUnknown
	}
}

// errorStringValue returns the best user-facing string for a Value that is
// expected to contain an error.
func errorStringValue(v Value) string {
	if v.kind == kindError {
		if rec := v.errorRecord(); rec != nil {
			return rec.displayMessage()
		}
	}
	s, err := v.AsString()
	if err != nil {
		return v.String()
	}
	return s
}

// errorRecord returns the structured error record stored in the value. When
// the value only has the legacy string payload, the helper reconstructs a
// minimal structured record on the fly.
func (v Value) errorRecord() *OsirisError {
	if v.kind != kindError {
		return nil
	}
	if v.err != nil {
		return v.err.clone()
	}
	return newOsirisError(ErrorKindUnknown, v.errMsg)
}

// errorPair builds a stable two-item list used by the error metadata helpers.
func errorPair(key string, value Value) Value {
	return listValue([]Value{stringValue(key), value})
}

// metadataMapValue converts a metadata map into a deterministic list of
// key/value pairs sorted by key name.
func metadataMapValue(m map[string]Value) Value {
	if len(m) == 0 {
		return listValue(nil)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Value, 0, len(keys))
	for _, k := range keys {
		out = append(out, errorPair(k, m[k]))
	}
	return listValue(out)
}

// errorMetadataEqual compares two metadata maps for semantic equality.
func errorMetadataEqual(a, b map[string]Value) bool {
	if len(a) != len(b) {
		return len(a) == 0 && len(b) == 0
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || !av.Equal(bv) {
			return false
		}
	}
	return true
}

// errorLocationEqual compares two optional source locations.
func errorLocationEqual(a, b *ErrorLocation) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	}
	return a.Source == b.Source && a.Span == b.Span
}

// pairKey returns the string key from a two-item error pair list.
func pairKey(v Value) string {
	if v.kind != kindList || len(v.list) == 0 {
		return ""
	}
	s, err := v.list[0].AsString()
	if err != nil {
		return v.list[0].String()
	}
	return s
}

// canonicalErrorMetaKey normalizes a metadata key so the runtime can accept a
// small family of spellings without losing the canonical field mapping.
func canonicalErrorMetaKey(key string) string {
	key = strings.TrimSpace(strings.ToLower(key))
	key = strings.ReplaceAll(key, " ", "_")
	key = strings.ReplaceAll(key, "-", "_")
	return key
}

// parseErrorTimestamp parses a timestamp string in the formats that error
// metadata is expected to use.
func parseErrorTimestamp(text string) (time.Time, error) {
	if text == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}
	if ts, err := time.Parse(time.RFC3339Nano, text); err == nil {
		return ts, nil
	}
	if ts, err := time.Parse(time.RFC3339, text); err == nil {
		return ts, nil
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp format")
}

// escapeErrorText escapes error message text for stable summary rendering.
func escapeErrorText(text string) string {
	quoted := strconv.Quote(text)
	if len(quoted) < 2 {
		return quoted
	}
	return quoted[1 : len(quoted)-1]
}

// errorBuiltin constructs a new structured Osiris error from a kind code and
// human-readable message.
func errorBuiltin(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("error expects 2 arguments")
	}
	kind, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	message, err := args[1].AsString()
	if err != nil {
		return nullValue(), err
	}
	return errorValueFromRecord(newOsirisError(ErrorKind(kind), message)), nil
}

// wrapErrorBuiltin constructs a new structured error and attaches a cause to
// it. The first argument is the cause, followed by the kind and message of the
// wrapper error.
func wrapErrorBuiltin(args []Value) (Value, error) {
	if len(args) != 3 {
		return nullValue(), fmt.Errorf("wrap expects 3 arguments")
	}
	cause, err := args[0].AsError()
	if err != nil {
		if args[0].kind != kindNull {
			return nullValue(), err
		}
	}
	kind, err := args[1].AsString()
	if err != nil {
		return nullValue(), err
	}
	message, err := args[2].AsString()
	if err != nil {
		return nullValue(), err
	}
	rec := newOsirisError(ErrorKind(kind), message)
	if cause != nil {
		rec = rec.withCause(cause)
	}
	return errorValueFromRecord(rec), nil
}

// causeErrorBuiltin returns the cause attached to a structured error or null
// when the error is not wrapped.
func causeErrorBuiltin(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("cause expects 1 argument")
	}
	rec, err := args[0].AsError()
	if err != nil {
		return nullValue(), err
	}
	if rec == nil || rec.cause == nil {
		return nullValue(), nil
	}
	return errorValueFromRecord(rec.cause), nil
}

// unwrapErrorBuiltin is an alias of causeErrorBuiltin.
func unwrapErrorBuiltin(args []Value) (Value, error) { return causeErrorBuiltin(args) }

// errMetaBuiltin returns the public inspection record for an error. The
// record includes the stable human-readable fields and the public metadata
// entries, but not debug-only details.
func errMetaBuiltin(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("errmeta expects 1 argument")
	}
	rec, err := args[0].AsError()
	if err != nil {
		return nullValue(), err
	}
	if rec == nil {
		return listValue(nil), nil
	}
	return rec.publicPairs(), nil
}

// redactErrorBuiltin strips debug-only metadata and stack traces from an
// error while preserving the public cause chain and user-facing content.
func redactErrorBuiltin(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("redact expects 1 argument")
	}
	rec, err := args[0].AsError()
	if err != nil {
		return nullValue(), err
	}
	return errorValueFromRecord(rec.Redacted()), nil
}

// withMetaBuiltin returns a copy of the supplied error with one public
// metadata entry attached or updated. Recognized keys update canonical fields;
// other keys are stored in the generic metadata map.
func withMetaBuiltin(args []Value) (Value, error) {
	if len(args) != 3 {
		return nullValue(), fmt.Errorf("withmeta expects 3 arguments")
	}
	rec, err := args[0].AsError()
	if err != nil {
		return nullValue(), err
	}
	key, err := args[1].AsString()
	if err != nil {
		return nullValue(), err
	}
	return errorValueFromRecord(rec.withMetadata(key, args[2])), nil
}

// withDebugMetaBuiltin returns a copy of the supplied error with a new
// debug-only metadata entry attached.
func withDebugMetaBuiltin(args []Value) (Value, error) {
	if len(args) != 3 {
		return nullValue(), fmt.Errorf("withdebug expects 3 arguments")
	}
	rec, err := args[0].AsError()
	if err != nil {
		return nullValue(), err
	}
	key, err := args[1].AsString()
	if err != nil {
		return nullValue(), err
	}
	return errorValueFromRecord(rec.withDebugMetadata(key, args[2])), nil
}

// encodeErrorRecord writes the structured error record into the value codec.
// The format is versioned so future fields can be added without breaking the
// existing runtime.
func encodeErrorRecord(buf *bytes.Buffer, rec *OsirisError) error {
	if rec == nil {
		rec = newOsirisError(ErrorKindUnknown, "")
	}
	if err := buf.WriteByte(0x0a); err != nil {
		return err
	}
	buf.Write(appendUvarint(nil, 1))
	if err := writeCodecString(buf, string(rec.kind)); err != nil {
		return err
	}
	if err := writeCodecString(buf, rec.code); err != nil {
		return err
	}
	if err := writeCodecString(buf, rec.message); err != nil {
		return err
	}
	if err := writeCodecString(buf, rec.userMessage); err != nil {
		return err
	}
	if err := writeCodecString(buf, rec.operation); err != nil {
		return err
	}
	if err := writeCodecString(buf, rec.resource); err != nil {
		return err
	}
	if rec.retryable != nil {
		if err := buf.WriteByte(1); err != nil {
			return err
		}
		if *rec.retryable {
			if err := buf.WriteByte(1); err != nil {
				return err
			}
		} else if err := buf.WriteByte(0); err != nil {
			return err
		}
	} else if err := buf.WriteByte(0); err != nil {
		return err
	}
	if err := writeCodecString(buf, string(rec.severity)); err != nil {
		return err
	}
	if err := writeCodecString(buf, rec.correlationID); err != nil {
		return err
	}
	if err := writeCodecString(buf, rec.component); err != nil {
		return err
	}
	if err := writeCodecString(buf, rec.remediation); err != nil {
		return err
	}
	if rec.timestamp.IsZero() {
		if err := buf.WriteByte(0); err != nil {
			return err
		}
	} else {
		if err := buf.WriteByte(1); err != nil {
			return err
		}
		if err := writeCodecString(buf, rec.timestamp.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	if rec.location == nil || rec.location.String() == "" {
		if err := buf.WriteByte(0); err != nil {
			return err
		}
	} else {
		if err := buf.WriteByte(1); err != nil {
			return err
		}
		if err := writeCodecString(buf, rec.location.Source); err != nil {
			return err
		}
		buf.Write(appendUvarint(nil, uint64(rec.location.Span.start.line)))
		buf.Write(appendUvarint(nil, uint64(rec.location.Span.start.col)))
		buf.Write(appendUvarint(nil, uint64(rec.location.Span.end.line)))
		buf.Write(appendUvarint(nil, uint64(rec.location.Span.end.col)))
	}
	if rec.cause == nil {
		if err := buf.WriteByte(0); err != nil {
			return err
		}
	} else {
		if err := buf.WriteByte(1); err != nil {
			return err
		}
		if err := encodeErrorRecord(buf, rec.cause); err != nil {
			return err
		}
	}
	if err := writeCodecStack(buf, rec.stack); err != nil {
		return err
	}
	if err := writeCodecMap(buf, rec.metadata); err != nil {
		return err
	}
	if err := writeCodecMap(buf, rec.debugMetadata); err != nil {
		return err
	}
	return nil
}

// decodeErrorRecord reads a structured error record from the value codec.
func decodeErrorRecord(cur *byteCursor) (*OsirisError, error) {
	ver, err := cur.uvarint()
	if err != nil {
		return nil, err
	}
	if ver != 1 {
		return nil, fmt.Errorf("unsupported error encoding version %d", ver)
	}
	kind, err := readCodecString(cur)
	if err != nil {
		return nil, err
	}
	code, err := readCodecString(cur)
	if err != nil {
		return nil, err
	}
	message, err := readCodecString(cur)
	if err != nil {
		return nil, err
	}
	userMessage, err := readCodecString(cur)
	if err != nil {
		return nil, err
	}
	operation, err := readCodecString(cur)
	if err != nil {
		return nil, err
	}
	resource, err := readCodecString(cur)
	if err != nil {
		return nil, err
	}
	hasRetryable, err := cur.byte()
	if err != nil {
		return nil, err
	}
	var retryable *bool
	if hasRetryable != 0 {
		v, err := cur.byte()
		if err != nil {
			return nil, err
		}
		b := v != 0
		retryable = &b
	}
	severity, err := readCodecString(cur)
	if err != nil {
		return nil, err
	}
	correlationID, err := readCodecString(cur)
	if err != nil {
		return nil, err
	}
	component, err := readCodecString(cur)
	if err != nil {
		return nil, err
	}
	remediation, err := readCodecString(cur)
	if err != nil {
		return nil, err
	}
	hasTimestamp, err := cur.byte()
	if err != nil {
		return nil, err
	}
	var ts time.Time
	if hasTimestamp != 0 {
		text, err := readCodecString(cur)
		if err != nil {
			return nil, err
		}
		ts, err = parseErrorTimestamp(text)
		if err != nil {
			return nil, err
		}
	}
	hasLocation, err := cur.byte()
	if err != nil {
		return nil, err
	}
	var location *ErrorLocation
	if hasLocation != 0 {
		source, err := readCodecString(cur)
		if err != nil {
			return nil, err
		}
		startLine, err := cur.uvarint()
		if err != nil {
			return nil, err
		}
		startCol, err := cur.uvarint()
		if err != nil {
			return nil, err
		}
		endLine, err := cur.uvarint()
		if err != nil {
			return nil, err
		}
		endCol, err := cur.uvarint()
		if err != nil {
			return nil, err
		}
		location = &ErrorLocation{
			Source: source,
			Span: sourceSpan{
				start: sourcePos{line: int(startLine), col: int(startCol)},
				end:   sourcePos{line: int(endLine), col: int(endCol)},
			},
		}
	}
	hasCause, err := cur.byte()
	if err != nil {
		return nil, err
	}
	var cause *OsirisError
	if hasCause != 0 {
		cause, err = decodeTaggedErrorRecord(cur)
		if err != nil {
			return nil, err
		}
	}
	stack, err := readCodecStack(cur)
	if err != nil {
		return nil, err
	}
	metadata, err := readCodecMap(cur)
	if err != nil {
		return nil, err
	}
	debugMetadata, err := readCodecMap(cur)
	if err != nil {
		return nil, err
	}
	rec := newOsirisError(ErrorKind(kind), message)
	rec.code = code
	if rec.code == "" {
		rec.code = string(rec.kind)
	}
	rec.userMessage = userMessage
	rec.operation = operation
	rec.resource = resource
	rec.retryable = retryable
	rec.severity = ErrorSeverity(severity)
	if rec.severity == "" {
		rec.severity = ErrorSeverityError
	}
	rec.correlationID = correlationID
	rec.timestamp = ts
	rec.component = component
	rec.remediation = remediation
	rec.location = location
	rec.cause = cause
	rec.stack = stack
	rec.metadata = metadata
	rec.debugMetadata = debugMetadata
	return rec, nil
}

// decodeTaggedErrorRecord reads a nested structured error record that still
// carries its own outer codec tag. Nested error causes are encoded with the
// same tagged representation as top-level values, so the recursive decoder
// needs to consume and validate that outer tag before parsing the record body.
func decodeTaggedErrorRecord(cur *byteCursor) (*OsirisError, error) {
	tag, err := cur.byte()
	if err != nil {
		return nil, err
	}
	if tag != 0x0a {
		return nil, fmt.Errorf("invalid error encoding tag %d", tag)
	}
	return decodeErrorRecord(cur)
}

// writeCodecString writes a single string field into the codec stream.
func writeCodecString(buf *bytes.Buffer, text string) error {
	buf.Write(appendUvarint(nil, uint64(len(text))))
	_, err := buf.WriteString(text)
	return err
}

// readCodecString reads a single string field from the codec stream.
func readCodecString(cur *byteCursor) (string, error) {
	n, err := cur.uvarint()
	if err != nil {
		return "", err
	}
	raw, err := cur.take(int(n))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// writeCodecMap writes a deterministic map of string keys to values.
func writeCodecMap(buf *bytes.Buffer, m map[string]Value) error {
	if len(m) == 0 {
		buf.Write(appendUvarint(nil, 0))
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	buf.Write(appendUvarint(nil, uint64(len(keys))))
	for _, k := range keys {
		if err := writeCodecString(buf, k); err != nil {
			return err
		}
		if err := encodeInto(buf, m[k]); err != nil {
			return err
		}
	}
	return nil
}

// readCodecMap reads a deterministic map of string keys to values from the
// codec stream.
func readCodecMap(cur *byteCursor) (map[string]Value, error) {
	n, err := cur.uvarint()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	out := make(map[string]Value, n)
	for i := uint64(0); i < n; i++ {
		key, err := readCodecString(cur)
		if err != nil {
			return nil, err
		}
		v, err := decodeFrom(cur)
		if err != nil {
			return nil, err
		}
		out[key] = v
	}
	return out, nil
}

// writeCodecStack writes the structured error stack into the codec stream.
func writeCodecStack(buf *bytes.Buffer, stack []ErrorFrame) error {
	buf.Write(appendUvarint(nil, uint64(len(stack))))
	for _, frame := range stack {
		if err := writeCodecString(buf, frame.Name); err != nil {
			return err
		}
		if err := writeCodecString(buf, frame.Location.Source); err != nil {
			return err
		}
		buf.Write(appendUvarint(nil, uint64(frame.Location.Span.start.line)))
		buf.Write(appendUvarint(nil, uint64(frame.Location.Span.start.col)))
		buf.Write(appendUvarint(nil, uint64(frame.Location.Span.end.line)))
		buf.Write(appendUvarint(nil, uint64(frame.Location.Span.end.col)))
	}
	return nil
}

// readCodecStack reads structured error stack frames from the codec stream.
func readCodecStack(cur *byteCursor) ([]ErrorFrame, error) {
	n, err := cur.uvarint()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	stack := make([]ErrorFrame, 0, n)
	for i := uint64(0); i < n; i++ {
		name, err := readCodecString(cur)
		if err != nil {
			return nil, err
		}
		source, err := readCodecString(cur)
		if err != nil {
			return nil, err
		}
		startLine, err := cur.uvarint()
		if err != nil {
			return nil, err
		}
		startCol, err := cur.uvarint()
		if err != nil {
			return nil, err
		}
		endLine, err := cur.uvarint()
		if err != nil {
			return nil, err
		}
		endCol, err := cur.uvarint()
		if err != nil {
			return nil, err
		}
		stack = append(stack, ErrorFrame{
			Name: name,
			Location: ErrorLocation{
				Source: source,
				Span: sourceSpan{
					start: sourcePos{line: int(startLine), col: int(startCol)},
					end:   sourcePos{line: int(endLine), col: int(endCol)},
				},
			},
		})
	}
	return stack, nil
}
