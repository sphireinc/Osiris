package osiris

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Task120JSONVersion identifies the JSON utility contract.
const Task120JSONVersion = "json-rfc6901-rfc6902-rfc7396-rfc8785-lite-1"

// Task120JSONLimits bounds JSON parsing, tree traversal, patch operations, and
// generated output. Zero fields select the documented defaults.
type Task120JSONLimits struct {
	MaxBytes         int64
	MaxDepth         int
	MaxNodes         int
	MaxStringBytes   int64
	MaxObjectFields  int
	MaxPointerTokens int
	MaxPatchOps      int
	MaxOutputBytes   int64
}

func (l Task120JSONLimits) normalized() Task120JSONLimits {
	base := (Task120ConfigLimits{}).normalized()
	if l.MaxBytes <= 0 {
		l.MaxBytes = base.MaxBytes
	}
	if l.MaxDepth <= 0 {
		l.MaxDepth = base.MaxDepth
	}
	if l.MaxNodes <= 0 {
		l.MaxNodes = base.MaxNodes
	}
	if l.MaxStringBytes <= 0 {
		l.MaxStringBytes = base.MaxStringBytes
	}
	if l.MaxObjectFields <= 0 {
		l.MaxObjectFields = base.MaxObjectFields
	}
	if l.MaxPointerTokens <= 0 {
		l.MaxPointerTokens = l.MaxDepth
	}
	if l.MaxPatchOps <= 0 {
		l.MaxPatchOps = l.MaxNodes
	}
	if l.MaxOutputBytes <= 0 {
		l.MaxOutputBytes = l.MaxBytes
	}
	return l
}

func task120JSONConfigLimits(limits Task120JSONLimits) Task120ConfigLimits {
	limits = limits.normalized()
	return Task120ConfigLimits{MaxBytes: limits.MaxBytes, MaxDepth: limits.MaxDepth, MaxNodes: limits.MaxNodes, MaxStringBytes: limits.MaxStringBytes, MaxObjectFields: limits.MaxObjectFields}
}

// Task120JSONParse parses one bounded JSON value into the compatible ordered
// Task120ConfigValue representation.
func Task120JSONParse(ctx context.Context, data []byte, limits Task120JSONLimits) (Task120ConfigValue, error) {
	limits = limits.normalized()
	value, err := Task120ParseJSONConfig(ctx, data, task120JSONConfigLimits(limits))
	if err != nil {
		return Task120ConfigValue{}, err
	}
	if err := task120JSONCheckValue(ctx, value, limits, 0, new(int)); err != nil {
		return Task120ConfigValue{}, err
	}
	return value, nil
}

// Task120ParseJSON is the adapter-facing spelling of Task120JSONParse.
func Task120ParseJSON(ctx context.Context, data []byte, limits Task120ConfigLimits) (Task120ConfigValue, error) {
	return Task120JSONParse(ctx, data, Task120JSONLimits{MaxBytes: limits.MaxBytes, MaxDepth: limits.MaxDepth, MaxNodes: limits.MaxNodes, MaxStringBytes: limits.MaxStringBytes, MaxObjectFields: limits.MaxObjectFields})
}

// Task120JSONStreamParse parses one bounded caller-owned JSON stream and does
// not close it.
func Task120JSONStreamParse(ctx context.Context, src io.Reader, limits Task120JSONLimits) (Task120ConfigValue, error) {
	limits = limits.normalized()
	value, err := Task120ParseJSONConfigStream(ctx, src, task120JSONConfigLimits(limits))
	if err != nil {
		return Task120ConfigValue{}, err
	}
	if err := task120JSONCheckValue(ctx, value, limits, 0, new(int)); err != nil {
		return Task120ConfigValue{}, err
	}
	return value, nil
}

// Task120ParseJSONStream is the adapter-facing spelling of
// Task120JSONStreamParse.
func Task120ParseJSONStream(ctx context.Context, src io.Reader, limits Task120ConfigLimits) (Task120ConfigValue, error) {
	return Task120JSONStreamParse(ctx, src, Task120JSONLimits{MaxBytes: limits.MaxBytes, MaxDepth: limits.MaxDepth, MaxNodes: limits.MaxNodes, MaxStringBytes: limits.MaxStringBytes, MaxObjectFields: limits.MaxObjectFields})
}

// Task120ParseJSONValue is a compatibility alias for Task120ParseJSON.
func Task120ParseJSONValue(ctx context.Context, data []byte, limits Task120ConfigLimits) (Task120ConfigValue, error) {
	return Task120ParseJSON(ctx, data, limits)
}

// Task120ParseJSONValueStream is a compatibility alias for Task120ParseJSONStream.
func Task120ParseJSONValueStream(ctx context.Context, src io.Reader, limits Task120ConfigLimits) (Task120ConfigValue, error) {
	return Task120ParseJSONStream(ctx, src, limits)
}

// Task120JSONValidate returns a detailed syntax, model, limit, or cancellation
// error. Invalid JSON wraps ErrTask120Invalid.
func Task120JSONValidate(ctx context.Context, data []byte, limits Task120JSONLimits) error {
	_, err := Task120JSONParse(ctx, data, limits)
	return err
}

// Task120JSONValid reports whether data contains exactly one valid bounded
// JSON value. It returns false for malformed or limit-rejected input.
func Task120JSONValid(ctx context.Context, data []byte, limits Task120JSONLimits) (bool, error) {
	err := Task120JSONValidate(ctx, data, limits)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrTask120Invalid) {
		return false, nil
	}
	return false, err
}

// Task120ValidJSON is a context-free validity convenience wrapper.
func Task120ValidJSON(data []byte, limits ...Task120JSONLimits) bool {
	ok, _ := Task120JSONValid(context.Background(), data, task120JSONOptionalLimits(limits...))
	return ok
}

type task120JSONValueChecker struct {
	ctx    context.Context
	limits Task120JSONLimits
	nodes  *int
}

func task120JSONCheckValue(ctx context.Context, value Task120ConfigValue, limits Task120JSONLimits, depth int, nodes *int) error {
	if err := task120CheckContext(ctx, "JSON value"); err != nil {
		return err
	}
	if depth > limits.MaxDepth {
		return task120LimitError("JSON value", "depth", int64(depth), int64(limits.MaxDepth))
	}
	*nodes++
	if *nodes > limits.MaxNodes {
		return task120LimitError("JSON value", "nodes", int64(*nodes), int64(limits.MaxNodes))
	}
	switch value.Kind {
	case Task120ConfigNull, Task120ConfigBool:
		return nil
	case Task120ConfigNumber:
		if !task120JSONNumberValid(value.Number) {
			return fmt.Errorf("JSON value: %w: invalid number %q", ErrTask120Invalid, value.Number)
		}
	case Task120ConfigString:
		if !utf8.ValidString(value.String) {
			return fmt.Errorf("JSON value: %w: invalid UTF-8 string", ErrTask120Invalid)
		}
		if int64(len(value.String)) > limits.MaxStringBytes {
			return task120LimitError("JSON value", "string bytes", int64(len(value.String)), limits.MaxStringBytes)
		}
	case Task120ConfigArray:
		for _, item := range value.Array {
			if err := task120JSONCheckValue(ctx, item, limits, depth+1, nodes); err != nil {
				return err
			}
		}
	case Task120ConfigObject:
		if len(value.Object) > limits.MaxObjectFields {
			return task120LimitError("JSON value", "object fields", int64(len(value.Object)), int64(limits.MaxObjectFields))
		}
		seen := make(map[string]struct{}, len(value.Object))
		for _, field := range value.Object {
			if !utf8.ValidString(field.Key) {
				return fmt.Errorf("JSON value: %w: invalid UTF-8 object key", ErrTask120Invalid)
			}
			if _, ok := seen[field.Key]; ok {
				return fmt.Errorf("JSON value: %w: duplicate object key %q", ErrTask120Invalid, field.Key)
			}
			seen[field.Key] = struct{}{}
			if err := task120JSONCheckValue(ctx, field.Value, limits, depth+1, nodes); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("JSON value: %w: unknown kind %q", ErrTask120Invalid, value.Kind)
	}
	return nil
}

func task120JSONOptionalLimits(limits ...Task120JSONLimits) Task120JSONLimits {
	if len(limits) == 0 {
		return Task120JSONLimits{}.normalized()
	}
	return limits[0].normalized()
}

func task120JSONNumberValid(number string) bool {
	if number == "" {
		return false
	}
	decoder := json.NewDecoder(strings.NewReader(number))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return false
	}
	if _, ok := value.(json.Number); !ok {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}

func task120JSONBoundOutput(data []byte, limits Task120JSONLimits) ([]byte, error) {
	if int64(len(data)) > limits.MaxOutputBytes {
		return nil, task120LimitError("JSON output", "output bytes", int64(len(data)), limits.MaxOutputBytes)
	}
	return append([]byte(nil), data...), nil
}

// Task120JSONOptions controls optional JSON serialization.
type Task120JSONOptions struct {
	Pretty    bool
	Indent    string
	Prefix    string
	Canonical bool
	Limits    Task120JSONLimits
}

// Task120JSONStringify emits compact deterministic JSON preserving object
// insertion order and number spelling.
func Task120JSONStringify(value Task120ConfigValue, limits ...Task120JSONLimits) ([]byte, error) {
	return task120JSONSerialize(value, task120JSONOptionalLimits(limits...), false)
}

// Task120StringifyJSON is the adapter-facing spelling of Task120JSONStringify.
// It accepts both the context-aware adapter form (ctx, value, limits) and the
// compact Go form (value[, limits]) for compatibility with existing Task 120
// callers.
func Task120StringifyJSON(args ...interface{}) ([]byte, error) {
	if len(args) == 3 {
		ctx, ctxOK := args[0].(context.Context)
		value, valueOK := args[1].(Task120ConfigValue)
		limits, limitsOK := args[2].(Task120JSONLimits)
		if ctxOK && valueOK && limitsOK {
			if err := task120CheckContext(ctx, "JSON stringify"); err != nil {
				return nil, err
			}
			return Task120JSONStringify(value, limits)
		}
	}
	if len(args) >= 1 {
		value, valueOK := args[0].(Task120ConfigValue)
		if valueOK {
			if len(args) == 1 {
				return Task120JSONStringify(value)
			}
			if limits, limitsOK := args[1].(Task120JSONLimits); limitsOK {
				return Task120JSONStringify(value, limits)
			}
		}
	}
	return nil, fmt.Errorf("JSON stringify: %w: expected value and optional limits", ErrTask120Invalid)
}

// Task120JSONPretty emits deterministic two-space-indented JSON.
func Task120JSONPretty(value Task120ConfigValue, limits ...Task120JSONLimits) ([]byte, error) {
	return Task120JSONPrettyIndent(value, "", "  ", limits...)
}

// Task120PrettyJSON accepts the adapter form (ctx, value, indent, limits) and
// the compact Go form (value[, limits]).
func Task120PrettyJSON(args ...interface{}) ([]byte, error) {
	if len(args) == 4 {
		ctx, ctxOK := args[0].(context.Context)
		value, valueOK := args[1].(Task120ConfigValue)
		indent, indentOK := args[2].(string)
		limits, limitsOK := args[3].(Task120JSONLimits)
		if ctxOK && valueOK && indentOK && limitsOK {
			if err := task120CheckContext(ctx, "JSON pretty"); err != nil {
				return nil, err
			}
			return Task120JSONPrettyIndent(value, "", indent, limits)
		}
	}
	if len(args) >= 1 {
		value, valueOK := args[0].(Task120ConfigValue)
		if valueOK {
			if len(args) == 1 {
				return Task120JSONPretty(value)
			}
			if limits, limitsOK := args[1].(Task120JSONLimits); limitsOK {
				return Task120JSONPretty(value, limits)
			}
		}
	}
	return nil, fmt.Errorf("JSON pretty: %w: expected value, indent, and limits", ErrTask120Invalid)
}

// Task120JSONPrettyIndent emits deterministic JSON with caller-selected
// encoding/json-compatible prefix and indentation.
func Task120JSONPrettyIndent(value Task120ConfigValue, prefix, indent string, limits ...Task120JSONLimits) ([]byte, error) {
	lim := task120JSONOptionalLimits(limits...)
	compact, err := Task120JSONStringify(value, lim)
	if err != nil {
		return nil, err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, compact, prefix, indent); err != nil {
		return nil, fmt.Errorf("JSON pretty: %w", err)
	}
	return task120JSONBoundOutput(pretty.Bytes(), lim)
}

// Task120StringifyJSONWithOptions serializes according to options.
func Task120StringifyJSONWithOptions(value Task120ConfigValue, options Task120JSONOptions) ([]byte, error) {
	if options.Canonical {
		return Task120JSONCanonicalize(value, options.Limits)
	}
	if options.Pretty {
		indent := options.Indent
		if indent == "" {
			indent = "  "
		}
		return Task120JSONPrettyIndent(value, options.Prefix, indent, options.Limits)
	}
	return Task120JSONStringify(value, options.Limits)
}

func task120JSONSerialize(value Task120ConfigValue, limits Task120JSONLimits, canonical bool) ([]byte, error) {
	if err := task120JSONCheckValue(context.Background(), value, limits, 0, new(int)); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := task120JSONEncode(&out, value, canonical); err != nil {
		return nil, err
	}
	return task120JSONBoundOutput(out.Bytes(), limits)
}

func task120JSONEncode(out *bytes.Buffer, value Task120ConfigValue, canonical bool) error {
	switch value.Kind {
	case Task120ConfigNull:
		out.WriteString("null")
	case Task120ConfigBool:
		if value.Bool {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
	case Task120ConfigNumber:
		if canonical {
			number, err := task120JSONCanonicalNumber(value.Number)
			if err != nil {
				return err
			}
			out.WriteString(number)
		} else {
			out.WriteString(value.Number)
		}
	case Task120ConfigString:
		task120JSONWriteString(out, value.String)
	case Task120ConfigArray:
		out.WriteByte('[')
		for i, item := range value.Array {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := task120JSONEncode(out, item, canonical); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case Task120ConfigObject:
		fields := append([]Task120ConfigField(nil), value.Object...)
		if canonical {
			sort.SliceStable(fields, func(i, j int) bool { return task120JSONUTF16Less(fields[i].Key, fields[j].Key) })
		}
		out.WriteByte('{')
		for i, field := range fields {
			if i > 0 {
				out.WriteByte(',')
			}
			task120JSONWriteString(out, field.Key)
			out.WriteByte(':')
			if err := task120JSONEncode(out, field.Value, canonical); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	default:
		return fmt.Errorf("JSON encode: %w: unknown kind %q", ErrTask120Invalid, value.Kind)
	}
	return nil
}

// Task120JSONCanonicalize accepts the adapter form (ctx, value, limits) and
// the compact Go form (value[, limits]).
func Task120JSONCanonicalize(args ...interface{}) ([]byte, error) {
	if len(args) == 3 {
		ctx, ctxOK := args[0].(context.Context)
		value, valueOK := args[1].(Task120ConfigValue)
		limits, limitsOK := args[2].(Task120JSONLimits)
		if ctxOK && valueOK && limitsOK {
			if err := task120CheckContext(ctx, "JSON canonicalize"); err != nil {
				return nil, err
			}
			return task120JSONSerialize(value, limits, true)
		}
	}
	if len(args) >= 1 {
		value, valueOK := args[0].(Task120ConfigValue)
		if valueOK {
			if len(args) == 1 {
				return task120JSONSerialize(value, task120JSONOptionalLimits(), true)
			}
			if limits, limitsOK := args[1].(Task120JSONLimits); limitsOK {
				return task120JSONSerialize(value, limits, true)
			}
		}
	}
	return nil, fmt.Errorf("JSON canonicalize: %w: expected value and optional limits", ErrTask120Invalid)
}

// Task120JSONCanonical is a compatibility alias for Task120JSONCanonicalize.
func Task120JSONCanonical(value Task120ConfigValue, limits ...Task120JSONLimits) ([]byte, error) {
	args := []interface{}{value}
	if len(limits) != 0 {
		args = append(args, limits[0])
	}
	return Task120JSONCanonicalize(args...)
}

// Task120CanonicalizeJSON is a compatibility alias for Task120JSONCanonicalize.
func Task120CanonicalizeJSON(value Task120ConfigValue) ([]byte, error) {
	return Task120JSONCanonicalize(value)
}

func task120JSONCanonicalNumber(number string) (string, error) {
	if !task120JSONNumberValid(number) {
		return "", fmt.Errorf("JSON canonical: %w: invalid number %q", ErrTask120Invalid, number)
	}
	f, err := strconv.ParseFloat(number, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return "", fmt.Errorf("JSON canonical: %w: number %q is outside finite binary64", ErrTask120Invalid, number)
	}
	if f == 0 {
		return "0", nil
	}
	abs := math.Abs(f)
	if abs >= 1e-6 && abs < 1e21 {
		return strconv.FormatFloat(f, 'f', -1, 64), nil
	}
	raw := strconv.FormatFloat(f, 'e', -1, 64)
	parts := strings.SplitN(raw, "e", 2)
	exponent, _ := strconv.Atoi(parts[1])
	mantissa := strings.TrimRight(strings.TrimRight(parts[0], "0"), ".")
	exponentText := strconv.Itoa(exponent)
	if exponent >= 0 {
		exponentText = "+" + exponentText
	}
	return mantissa + "e" + exponentText, nil
}

func task120JSONWriteString(out *bytes.Buffer, value string) {
	const hex = "0123456789abcdef"
	out.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"', '\\':
			out.WriteByte('\\')
			out.WriteRune(r)
		case '\b':
			out.WriteString(`\b`)
		case '\f':
			out.WriteString(`\f`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			if r < 0x20 {
				out.WriteString(`\u00`)
				out.WriteByte(hex[(r>>4)&0xf])
				out.WriteByte(hex[r&0xf])
			} else {
				out.WriteRune(r)
			}
		}
	}
	out.WriteByte('"')
}

func task120JSONUTF16Less(a, b string) bool {
	aa, bb := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(aa) && i < len(bb); i++ {
		if aa[i] != bb[i] {
			return aa[i] < bb[i]
		}
	}
	return len(aa) < len(bb)
}

func task120JSONPointerTokens(pointer string, limits Task120JSONLimits) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("JSON Pointer: %w: pointer must be empty or begin with '/'", ErrTask120Invalid)
	}
	raw := strings.Split(pointer[1:], "/")
	if len(raw) > limits.MaxPointerTokens {
		return nil, task120LimitError("JSON Pointer", "tokens", int64(len(raw)), int64(limits.MaxPointerTokens))
	}
	tokens := make([]string, len(raw))
	for i, part := range raw {
		var b strings.Builder
		for j := 0; j < len(part); j++ {
			if part[j] != '~' {
				b.WriteByte(part[j])
				continue
			}
			if j+1 >= len(part) || (part[j+1] != '0' && part[j+1] != '1') {
				return nil, fmt.Errorf("JSON Pointer: %w: invalid escape in token %q", ErrTask120Invalid, part)
			}
			if part[j+1] == '0' {
				b.WriteByte('~')
			} else {
				b.WriteByte('/')
			}
			j++
		}
		if !utf8.ValidString(b.String()) {
			return nil, fmt.Errorf("JSON Pointer: %w: invalid UTF-8 token", ErrTask120Invalid)
		}
		tokens[i] = b.String()
	}
	return tokens, nil
}

func task120JSONArrayIndex(token string, length int, allowAppend bool) (int, bool, error) {
	if token == "-" {
		if allowAppend {
			return length, true, nil
		}
		return 0, false, fmt.Errorf("JSON Pointer: %w: '-' is only valid for add", ErrTask120Invalid)
	}
	if token == "" || (len(token) > 1 && token[0] == '0') {
		return 0, false, fmt.Errorf("JSON Pointer: %w: invalid array index %q", ErrTask120Invalid, token)
	}
	for _, r := range token {
		if r < '0' || r > '9' {
			return 0, false, fmt.Errorf("JSON Pointer: %w: invalid array index %q", ErrTask120Invalid, token)
		}
	}
	index, err := strconv.Atoi(token)
	if err != nil || index > length || (!allowAppend && index == length) {
		return 0, false, fmt.Errorf("JSON Pointer: %w: array index %q out of range", ErrTask120Invalid, token)
	}
	return index, index == length, nil
}

func task120JSONClone(value Task120ConfigValue) Task120ConfigValue {
	clone := value
	if value.Array != nil {
		clone.Array = make([]Task120ConfigValue, len(value.Array))
		for i := range value.Array {
			clone.Array[i] = task120JSONClone(value.Array[i])
		}
	}
	if value.Object != nil {
		clone.Object = make([]Task120ConfigField, len(value.Object))
		for i, field := range value.Object {
			clone.Object[i] = Task120ConfigField{Key: field.Key, Value: task120JSONClone(field.Value)}
		}
	}
	return clone
}

func task120JSONObjectIndex(object []Task120ConfigField, key string) (int, error) {
	index := -1
	for i, field := range object {
		if field.Key == key {
			if index >= 0 {
				return -1, fmt.Errorf("JSON object: %w: duplicate key %q", ErrTask120Invalid, key)
			}
			index = i
		}
	}
	return index, nil
}

func task120JSONPointerGetValue(value Task120ConfigValue, tokens []string) (Task120ConfigValue, error) {
	for _, token := range tokens {
		switch value.Kind {
		case Task120ConfigObject:
			index, err := task120JSONObjectIndex(value.Object, token)
			if err != nil || index < 0 {
				return Task120ConfigValue{}, fmt.Errorf("JSON Pointer: %w: member %q not found", ErrTask120Invalid, token)
			}
			value = value.Object[index].Value
		case Task120ConfigArray:
			index, appendItem, err := task120JSONArrayIndex(token, len(value.Array), false)
			if err != nil || appendItem || index >= len(value.Array) {
				return Task120ConfigValue{}, fmt.Errorf("JSON Pointer: %w: array item %q not found", ErrTask120Invalid, token)
			}
			value = value.Array[index]
		default:
			return Task120ConfigValue{}, fmt.Errorf("JSON Pointer: %w: scalar cannot be traversed", ErrTask120Invalid)
		}
	}
	return value, nil
}

// Task120JSONPointerGet returns a detached value at an RFC 6901 pointer.
func Task120JSONPointerGet(value Task120ConfigValue, pointer string, limits ...Task120JSONLimits) (Task120ConfigValue, error) {
	lim := task120JSONOptionalLimits(limits...)
	if err := task120JSONCheckValue(context.Background(), value, lim, 0, new(int)); err != nil {
		return Task120ConfigValue{}, err
	}
	tokens, err := task120JSONPointerTokens(pointer, lim)
	if err != nil {
		return Task120ConfigValue{}, err
	}
	resolved, err := task120JSONPointerGetValue(value, tokens)
	if err != nil {
		return Task120ConfigValue{}, err
	}
	return task120JSONClone(resolved), nil
}

func task120JSONPointerAdd(value Task120ConfigValue, tokens []string, replacement Task120ConfigValue) (Task120ConfigValue, error) {
	if len(tokens) == 0 {
		return task120JSONClone(replacement), nil
	}
	clone := task120JSONClone(value)
	token := tokens[0]
	switch clone.Kind {
	case Task120ConfigObject:
		index, err := task120JSONObjectIndex(clone.Object, token)
		if err != nil {
			return Task120ConfigValue{}, err
		}
		if len(tokens) == 1 {
			if index < 0 {
				clone.Object = append(clone.Object, Task120ConfigField{Key: token, Value: task120JSONClone(replacement)})
			} else {
				clone.Object[index].Value = task120JSONClone(replacement)
			}
			return clone, nil
		}
		if index < 0 {
			return Task120ConfigValue{}, fmt.Errorf("JSON Pointer: %w: intermediate member %q not found", ErrTask120Invalid, token)
		}
		child, err := task120JSONPointerAdd(clone.Object[index].Value, tokens[1:], replacement)
		if err != nil {
			return Task120ConfigValue{}, err
		}
		clone.Object[index].Value = child
		return clone, nil
	case Task120ConfigArray:
		index, appendItem, err := task120JSONArrayIndex(token, len(clone.Array), true)
		if err != nil {
			return Task120ConfigValue{}, err
		}
		if len(tokens) == 1 {
			if appendItem {
				clone.Array = append(clone.Array, task120JSONClone(replacement))
				return clone, nil
			}
			clone.Array = append(clone.Array, Task120ConfigValue{})
			copy(clone.Array[index+1:], clone.Array[index:])
			clone.Array[index] = task120JSONClone(replacement)
			return clone, nil
		}
		if index >= len(clone.Array) {
			return Task120ConfigValue{}, fmt.Errorf("JSON Pointer: %w: intermediate array index out of range", ErrTask120Invalid)
		}
		child, err := task120JSONPointerAdd(clone.Array[index], tokens[1:], replacement)
		if err != nil {
			return Task120ConfigValue{}, err
		}
		clone.Array[index] = child
		return clone, nil
	default:
		return Task120ConfigValue{}, fmt.Errorf("JSON Pointer: %w: parent is scalar", ErrTask120Invalid)
	}
}

// Task120JSONPointerSet returns a detached document with an RFC 6902 add-style
// value at the pointer. Object members are created or replaced; array indices
// insert and '-' appends.
func Task120JSONPointerSet(value Task120ConfigValue, pointer string, replacement Task120ConfigValue, limits ...Task120JSONLimits) (Task120ConfigValue, error) {
	lim := task120JSONOptionalLimits(limits...)
	if err := task120JSONCheckValue(context.Background(), value, lim, 0, new(int)); err != nil {
		return Task120ConfigValue{}, err
	}
	if err := task120JSONCheckValue(context.Background(), replacement, lim, 0, new(int)); err != nil {
		return Task120ConfigValue{}, err
	}
	tokens, err := task120JSONPointerTokens(pointer, lim)
	if err != nil {
		return Task120ConfigValue{}, err
	}
	result, err := task120JSONPointerAdd(value, tokens, replacement)
	if err != nil {
		return Task120ConfigValue{}, err
	}
	return result, task120JSONCheckValue(context.Background(), result, lim, 0, new(int))
}

func task120JSONPointerRemove(value Task120ConfigValue, tokens []string) (Task120ConfigValue, error) {
	if len(tokens) == 0 {
		return Task120ConfigValue{}, fmt.Errorf("JSON Pointer: %w: cannot remove document root", ErrTask120Invalid)
	}
	clone := task120JSONClone(value)
	token := tokens[0]
	if len(tokens) == 1 {
		switch clone.Kind {
		case Task120ConfigObject:
			index, err := task120JSONObjectIndex(clone.Object, token)
			if err != nil || index < 0 {
				return Task120ConfigValue{}, fmt.Errorf("JSON Pointer: %w: member %q not found", ErrTask120Invalid, token)
			}
			clone.Object = append(clone.Object[:index], clone.Object[index+1:]...)
			return clone, nil
		case Task120ConfigArray:
			index, _, err := task120JSONArrayIndex(token, len(clone.Array), false)
			if err != nil || index >= len(clone.Array) {
				return Task120ConfigValue{}, fmt.Errorf("JSON Pointer: %w: array item %q not found", ErrTask120Invalid, token)
			}
			clone.Array = append(clone.Array[:index], clone.Array[index+1:]...)
			return clone, nil
		}
		return Task120ConfigValue{}, fmt.Errorf("JSON Pointer: %w: parent is scalar", ErrTask120Invalid)
	}
	switch clone.Kind {
	case Task120ConfigObject:
		index, err := task120JSONObjectIndex(clone.Object, token)
		if err != nil || index < 0 {
			return Task120ConfigValue{}, fmt.Errorf("JSON Pointer: %w: intermediate member %q not found", ErrTask120Invalid, token)
		}
		child, err := task120JSONPointerRemove(clone.Object[index].Value, tokens[1:])
		if err != nil {
			return Task120ConfigValue{}, err
		}
		clone.Object[index].Value = child
		return clone, nil
	case Task120ConfigArray:
		index, _, err := task120JSONArrayIndex(token, len(clone.Array), false)
		if err != nil || index >= len(clone.Array) {
			return Task120ConfigValue{}, fmt.Errorf("JSON Pointer: %w: intermediate array item %q not found", ErrTask120Invalid, token)
		}
		child, err := task120JSONPointerRemove(clone.Array[index], tokens[1:])
		if err != nil {
			return Task120ConfigValue{}, err
		}
		clone.Array[index] = child
		return clone, nil
	default:
		return Task120ConfigValue{}, fmt.Errorf("JSON Pointer: %w: parent is scalar", ErrTask120Invalid)
	}
}

// Task120JSONRemove is the patch engine's explicit remove helper.
func Task120JSONRemove(value Task120ConfigValue, tokens []string) (Task120ConfigValue, error) {
	return task120JSONPointerRemove(value, tokens)
}

// Task120JSONPointerRemove returns a detached document with the pointer target
// removed. Removing the document root is rejected because this API returns one
// value and cannot represent an absent document separately from JSON null.
func Task120JSONPointerRemove(value Task120ConfigValue, pointer string, limits ...Task120JSONLimits) (Task120ConfigValue, error) {
	lim := task120JSONOptionalLimits(limits...)
	if err := task120JSONCheckValue(context.Background(), value, lim, 0, new(int)); err != nil {
		return Task120ConfigValue{}, err
	}
	tokens, err := task120JSONPointerTokens(pointer, lim)
	if err != nil {
		return Task120ConfigValue{}, err
	}
	result, err := task120JSONPointerRemove(value, tokens)
	if err != nil {
		return Task120ConfigValue{}, err
	}
	return result, task120JSONCheckValue(context.Background(), result, lim, 0, new(int))
}

// Task120JSONPatchOperation is one typed RFC 6902 operation for Go callers.
// A zero Value means that the operation omitted its value member.
type Task120JSONPatchOperation struct {
	Op    string
	Path  string
	From  string
	Value Task120ConfigValue
}

func task120JSONPatchValue(operations []Task120JSONPatchOperation) Task120ConfigValue {
	patch := Task120ConfigValue{Kind: Task120ConfigArray, Array: make([]Task120ConfigValue, len(operations))}
	for i, operation := range operations {
		fields := []Task120ConfigField{{Key: "op", Value: Task120ConfigValue{Kind: Task120ConfigString, String: operation.Op}}, {Key: "path", Value: Task120ConfigValue{Kind: Task120ConfigString, String: operation.Path}}}
		if operation.From != "" {
			fields = append(fields, Task120ConfigField{Key: "from", Value: Task120ConfigValue{Kind: Task120ConfigString, String: operation.From}})
		}
		if operation.Value.Kind != "" {
			fields = append(fields, Task120ConfigField{Key: "value", Value: operation.Value})
		}
		patch.Array[i] = Task120ConfigValue{Kind: Task120ConfigObject, Object: fields}
	}
	return patch
}

// Task120ApplyJSONPatch applies typed RFC 6902 operations transactionally.
func Task120ApplyJSONPatch(ctx context.Context, document Task120ConfigValue, operations []Task120JSONPatchOperation, limits Task120JSONLimits) (Task120ConfigValue, error) {
	return task120JSONPatchApply(ctx, document, task120JSONPatchValue(operations), limits)
}

// Task120JSONPatchApply applies a JSON Patch document with default limits.
func Task120JSONPatchApply(document, patch Task120ConfigValue) (Task120ConfigValue, error) {
	return task120JSONPatchApply(context.Background(), document, patch, Task120JSONLimits{})
}

func task120JSONPatchApply(ctx context.Context, document, patch Task120ConfigValue, limits Task120JSONLimits) (Task120ConfigValue, error) {
	limits = limits.normalized()
	if err := task120CheckContext(ctx, "JSON Patch"); err != nil {
		return Task120ConfigValue{}, err
	}
	if err := task120JSONCheckValue(ctx, document, limits, 0, new(int)); err != nil {
		return Task120ConfigValue{}, err
	}
	if err := task120JSONCheckValue(ctx, patch, limits, 0, new(int)); err != nil {
		return Task120ConfigValue{}, err
	}
	if patch.Kind != Task120ConfigArray {
		return Task120ConfigValue{}, fmt.Errorf("JSON Patch: %w: patch must be an array", ErrTask120Invalid)
	}
	if len(patch.Array) > limits.MaxPatchOps {
		return Task120ConfigValue{}, task120LimitError("JSON Patch", "operations", int64(len(patch.Array)), int64(limits.MaxPatchOps))
	}
	result := task120JSONClone(document)
	for i, operation := range patch.Array {
		if err := task120CheckContext(ctx, "JSON Patch"); err != nil {
			return Task120ConfigValue{}, err
		}
		if err := task120JSONApplyOperation(result, operation, i, limits, &result); err != nil {
			return Task120ConfigValue{}, err
		}
		if err := task120JSONCheckValue(ctx, result, limits, 0, new(int)); err != nil {
			return Task120ConfigValue{}, err
		}
	}
	return result, nil
}

func task120JSONApplyOperation(document, operation Task120ConfigValue, index int, limits Task120JSONLimits, result *Task120ConfigValue) error {
	if operation.Kind != Task120ConfigObject {
		return fmt.Errorf("JSON Patch operation %d: %w: operation must be object", index, ErrTask120Invalid)
	}
	op, ok := task120JSONObjectField(operation, "op")
	pathValue, pathOK := task120JSONObjectField(operation, "path")
	if !ok || op.Kind != Task120ConfigString || !pathOK || pathValue.Kind != Task120ConfigString {
		return fmt.Errorf("JSON Patch operation %d: %w: op and path are required strings", index, ErrTask120Invalid)
	}
	path := pathValue.String
	tokens, err := task120JSONPointerTokens(path, limits)
	if err != nil {
		return fmt.Errorf("JSON Patch operation %d: %w", index, err)
	}
	value, hasValue := task120JSONObjectField(operation, "value")
	switch op.String {
	case "add":
		if !hasValue {
			return fmt.Errorf("JSON Patch operation %d: %w: add requires value", index, ErrTask120Invalid)
		}
		updated, err := task120JSONPointerAdd(*result, tokens, value)
		if err != nil {
			return fmt.Errorf("JSON Patch operation %d: %w", index, err)
		}
		*result = updated
	case "remove":
		updated, err := task120JSONPointerRemove(*result, tokens)
		if err != nil {
			return fmt.Errorf("JSON Patch operation %d: %w", index, err)
		}
		*result = updated
	case "replace":
		if !hasValue {
			return fmt.Errorf("JSON Patch operation %d: %w: replace requires value", index, ErrTask120Invalid)
		}
		if len(tokens) > 0 {
			if _, err := task120JSONPointerGetValue(*result, tokens); err != nil {
				return fmt.Errorf("JSON Patch operation %d: %w", index, err)
			}
		}
		updated, err := task120JSONPointerReplace(*result, tokens, value)
		if err != nil {
			return fmt.Errorf("JSON Patch operation %d: %w", index, err)
		}
		*result = updated
	case "move", "copy":
		fromValue, ok := task120JSONObjectField(operation, "from")
		if !ok || fromValue.Kind != Task120ConfigString {
			return fmt.Errorf("JSON Patch operation %d: %w: from is required", index, ErrTask120Invalid)
		}
		fromTokens, err := task120JSONPointerTokens(fromValue.String, limits)
		if err != nil {
			return fmt.Errorf("JSON Patch operation %d: %w", index, err)
		}
		if op.String == "move" && task120JSONPointerDescendant(fromTokens, tokens) {
			return fmt.Errorf("JSON Patch operation %d: %w: move target is a descendant", index, ErrTask120Invalid)
		}
		moved, err := task120JSONPointerGetValue(*result, fromTokens)
		if err != nil {
			return fmt.Errorf("JSON Patch operation %d: %w", index, err)
		}
		if op.String == "move" {
			*result, err = task120JSONPointerRemove(*result, fromTokens)
			if err != nil {
				return fmt.Errorf("JSON Patch operation %d: %w", index, err)
			}
		}
		*result, err = task120JSONPointerAdd(*result, tokens, moved)
		if err != nil {
			return fmt.Errorf("JSON Patch operation %d: %w", index, err)
		}
	case "test":
		if !hasValue {
			return fmt.Errorf("JSON Patch operation %d: %w: test requires value", index, ErrTask120Invalid)
		}
		actual, err := task120JSONPointerGetValue(*result, tokens)
		if err != nil || !task120JSONEqual(actual, value) {
			return fmt.Errorf("JSON Patch operation %d: %w: test failed", index, ErrTask120Invalid)
		}
	default:
		return fmt.Errorf("JSON Patch operation %d: %w: unsupported op %q", index, ErrTask120Invalid, op.String)
	}
	return nil
}

func task120JSONObjectField(object Task120ConfigValue, key string) (Task120ConfigValue, bool) {
	for _, field := range object.Object {
		if field.Key == key {
			return field.Value, true
		}
	}
	return Task120ConfigValue{}, false
}

func task120JSONPointerReplace(value Task120ConfigValue, tokens []string, replacement Task120ConfigValue) (Task120ConfigValue, error) {
	if len(tokens) == 0 {
		return task120JSONClone(replacement), nil
	}
	clone := task120JSONClone(value)
	if len(tokens) == 1 {
		switch clone.Kind {
		case Task120ConfigObject:
			index, err := task120JSONObjectIndex(clone.Object, tokens[0])
			if err == nil && index >= 0 {
				clone.Object[index].Value = task120JSONClone(replacement)
				return clone, nil
			}
		case Task120ConfigArray:
			index, _, err := task120JSONArrayIndex(tokens[0], len(clone.Array), false)
			if err == nil && index < len(clone.Array) {
				clone.Array[index] = task120JSONClone(replacement)
				return clone, nil
			}
		}
		return Task120ConfigValue{}, fmt.Errorf("JSON Pointer: %w: replace target not found", ErrTask120Invalid)
	}
	child, err := task120JSONPointerGetValue(clone, tokens[:1])
	if err != nil {
		return Task120ConfigValue{}, err
	}
	updated, err := task120JSONPointerReplace(child, tokens[1:], replacement)
	if err != nil {
		return Task120ConfigValue{}, err
	}
	return task120JSONPointerReplace(clone, tokens[:1], updated)
}

func task120JSONPointerDescendant(parent, child []string) bool {
	if len(child) <= len(parent) {
		return false
	}
	for i := range parent {
		if parent[i] != child[i] {
			return false
		}
	}
	return true
}

// Task120JSONMergePatch applies RFC 7396 with default limits.
func Task120JSONMergePatch(target, patch Task120ConfigValue) Task120ConfigValue {
	result, err := Task120ApplyJSONMergePatch(context.Background(), target, patch, Task120JSONLimits{})
	if err != nil {
		return Task120ConfigValue{}
	}
	return result
}

// Task120ApplyJSONMergePatch applies RFC 7396 transactionally with context and
// explicit limits.
func Task120ApplyJSONMergePatch(ctx context.Context, target, patch Task120ConfigValue, limits Task120JSONLimits) (Task120ConfigValue, error) {
	limits = limits.normalized()
	if err := task120JSONCheckValue(ctx, target, limits, 0, new(int)); err != nil {
		return Task120ConfigValue{}, err
	}
	if err := task120JSONCheckValue(ctx, patch, limits, 0, new(int)); err != nil {
		return Task120ConfigValue{}, err
	}
	result, err := task120JSONMergeValue(ctx, target, patch, limits, 0)
	if err != nil {
		return Task120ConfigValue{}, err
	}
	return result, task120JSONCheckValue(ctx, result, limits, 0, new(int))
}

// Task120JSONMergePatchContext is the error-returning context-aware spelling
// of Task120JSONMergePatch.
func Task120JSONMergePatchContext(ctx context.Context, target, patch Task120ConfigValue, limits Task120JSONLimits) (Task120ConfigValue, error) {
	return Task120ApplyJSONMergePatch(ctx, target, patch, limits)
}

func task120JSONMergeValue(ctx context.Context, target, patch Task120ConfigValue, limits Task120JSONLimits, depth int) (Task120ConfigValue, error) {
	if err := task120CheckContext(ctx, "JSON Merge Patch"); err != nil {
		return Task120ConfigValue{}, err
	}
	if depth > limits.MaxDepth {
		return Task120ConfigValue{}, task120LimitError("JSON Merge Patch", "depth", int64(depth), int64(limits.MaxDepth))
	}
	if patch.Kind != Task120ConfigObject {
		return task120JSONClone(patch), nil
	}
	result := task120JSONClone(target)
	if result.Kind != Task120ConfigObject {
		result = Task120ConfigValue{Kind: Task120ConfigObject}
	}
	for _, field := range patch.Object {
		index, err := task120JSONObjectIndex(result.Object, field.Key)
		if err != nil {
			return Task120ConfigValue{}, err
		}
		if field.Value.Kind == Task120ConfigNull {
			if index >= 0 {
				result.Object = append(result.Object[:index], result.Object[index+1:]...)
			}
			continue
		}
		base := Task120ConfigValue{Kind: Task120ConfigNull}
		if index >= 0 {
			base = result.Object[index].Value
		}
		merged, err := task120JSONMergeValue(ctx, base, field.Value, limits, depth+1)
		if err != nil {
			return Task120ConfigValue{}, err
		}
		if index >= 0 {
			result.Object[index].Value = merged
		} else {
			result.Object = append(result.Object, Task120ConfigField{Key: field.Key, Value: merged})
		}
	}
	return result, nil
}

func task120JSONMergePatchValue(ctx context.Context, target, patch Task120ConfigValue, limits Task120JSONLimits) (Task120ConfigValue, error) {
	return Task120ApplyJSONMergePatch(ctx, target, patch, limits)
}

func task120JSONEqual(a, b Task120ConfigValue) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case Task120ConfigNull:
		return true
	case Task120ConfigBool:
		return a.Bool == b.Bool
	case Task120ConfigNumber:
		return task120JSONNumberEqual(a.Number, b.Number)
	case Task120ConfigString:
		return a.String == b.String
	case Task120ConfigArray:
		if len(a.Array) != len(b.Array) {
			return false
		}
		for i := range a.Array {
			if !task120JSONEqual(a.Array[i], b.Array[i]) {
				return false
			}
		}
		return true
	case Task120ConfigObject:
		if len(a.Object) != len(b.Object) {
			return false
		}
		for _, field := range a.Object {
			other, ok := task120JSONObjectField(b, field.Key)
			if !ok || !task120JSONEqual(field.Value, other) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func task120JSONNumberEqual(a, b string) bool {
	ra, oka := task120JSONExactRat(a)
	rb, okb := task120JSONExactRat(b)
	return oka && okb && ra.Cmp(rb) == 0
}

func task120JSONExactRat(number string) (*big.Rat, bool) {
	if !task120JSONNumberValid(number) {
		return nil, false
	}
	f, _, err := big.ParseFloat(number, 10, 256, big.ToNearestEven)
	if err != nil || f == nil {
		return nil, false
	}
	r, _ := f.Rat(nil)
	return r, true
}
