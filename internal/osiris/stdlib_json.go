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

// StdlibJSONVersion identifies the JSON utility contract.
const StdlibJSONVersion = "json-rfc6901-rfc6902-rfc7396-rfc8785-lite-1"

// StdlibJSONLimits bounds JSON parsing, tree traversal, patch operations, and
// generated output. Zero fields select the documented defaults.
type StdlibJSONLimits struct {
	MaxBytes         int64
	MaxDepth         int
	MaxNodes         int
	MaxStringBytes   int64
	MaxObjectFields  int
	MaxPointerTokens int
	MaxPatchOps      int
	MaxOutputBytes   int64
}

func (l StdlibJSONLimits) normalized() StdlibJSONLimits {
	base := (StdlibConfigLimits{}).normalized()
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

func stdlibJSONConfigLimits(limits StdlibJSONLimits) StdlibConfigLimits {
	limits = limits.normalized()
	return StdlibConfigLimits{MaxBytes: limits.MaxBytes, MaxDepth: limits.MaxDepth, MaxNodes: limits.MaxNodes, MaxStringBytes: limits.MaxStringBytes, MaxObjectFields: limits.MaxObjectFields}
}

// StdlibJSONParse parses one bounded JSON value into the compatible ordered
// StdlibConfigValue representation.
func StdlibJSONParse(ctx context.Context, data []byte, limits StdlibJSONLimits) (StdlibConfigValue, error) {
	limits = limits.normalized()
	value, err := StdlibParseJSONConfig(ctx, data, stdlibJSONConfigLimits(limits))
	if err != nil {
		return StdlibConfigValue{}, err
	}
	if err := stdlibJSONCheckValue(ctx, value, limits, 0, new(int)); err != nil {
		return StdlibConfigValue{}, err
	}
	return value, nil
}

// StdlibParseJSON is the adapter-facing spelling of StdlibJSONParse.
func StdlibParseJSON(ctx context.Context, data []byte, limits StdlibConfigLimits) (StdlibConfigValue, error) {
	return StdlibJSONParse(ctx, data, StdlibJSONLimits{MaxBytes: limits.MaxBytes, MaxDepth: limits.MaxDepth, MaxNodes: limits.MaxNodes, MaxStringBytes: limits.MaxStringBytes, MaxObjectFields: limits.MaxObjectFields})
}

// StdlibJSONStreamParse parses one bounded caller-owned JSON stream and does
// not close it.
func StdlibJSONStreamParse(ctx context.Context, src io.Reader, limits StdlibJSONLimits) (StdlibConfigValue, error) {
	limits = limits.normalized()
	value, err := StdlibParseJSONConfigStream(ctx, src, stdlibJSONConfigLimits(limits))
	if err != nil {
		return StdlibConfigValue{}, err
	}
	if err := stdlibJSONCheckValue(ctx, value, limits, 0, new(int)); err != nil {
		return StdlibConfigValue{}, err
	}
	return value, nil
}

// StdlibParseJSONStream is the adapter-facing spelling of
// StdlibJSONStreamParse.
func StdlibParseJSONStream(ctx context.Context, src io.Reader, limits StdlibConfigLimits) (StdlibConfigValue, error) {
	return StdlibJSONStreamParse(ctx, src, StdlibJSONLimits{MaxBytes: limits.MaxBytes, MaxDepth: limits.MaxDepth, MaxNodes: limits.MaxNodes, MaxStringBytes: limits.MaxStringBytes, MaxObjectFields: limits.MaxObjectFields})
}

// StdlibParseJSONValue is a compatibility alias for StdlibParseJSON.
func StdlibParseJSONValue(ctx context.Context, data []byte, limits StdlibConfigLimits) (StdlibConfigValue, error) {
	return StdlibParseJSON(ctx, data, limits)
}

// StdlibParseJSONValueStream is a compatibility alias for StdlibParseJSONStream.
func StdlibParseJSONValueStream(ctx context.Context, src io.Reader, limits StdlibConfigLimits) (StdlibConfigValue, error) {
	return StdlibParseJSONStream(ctx, src, limits)
}

// StdlibJSONValidate returns a detailed syntax, model, limit, or cancellation
// error. Invalid JSON wraps ErrStdlibInvalid.
func StdlibJSONValidate(ctx context.Context, data []byte, limits StdlibJSONLimits) error {
	_, err := StdlibJSONParse(ctx, data, limits)
	return err
}

// StdlibJSONValid reports whether data contains exactly one valid bounded
// JSON value. It returns false for malformed or limit-rejected input.
func StdlibJSONValid(ctx context.Context, data []byte, limits StdlibJSONLimits) (bool, error) {
	err := StdlibJSONValidate(ctx, data, limits)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrStdlibInvalid) {
		return false, nil
	}
	return false, err
}

// StdlibValidJSON is a context-free validity convenience wrapper.
func StdlibValidJSON(data []byte, limits ...StdlibJSONLimits) bool {
	ok, _ := StdlibJSONValid(context.Background(), data, stdlibJSONOptionalLimits(limits...))
	return ok
}

type stdlibJSONValueChecker struct {
	ctx    context.Context
	limits StdlibJSONLimits
	nodes  *int
}

func stdlibJSONCheckValue(ctx context.Context, value StdlibConfigValue, limits StdlibJSONLimits, depth int, nodes *int) error {
	if err := stdlibCheckContext(ctx, "JSON value"); err != nil {
		return err
	}
	if depth > limits.MaxDepth {
		return stdlibLimitError("JSON value", "depth", int64(depth), int64(limits.MaxDepth))
	}
	*nodes++
	if *nodes > limits.MaxNodes {
		return stdlibLimitError("JSON value", "nodes", int64(*nodes), int64(limits.MaxNodes))
	}
	switch value.Kind {
	case StdlibConfigNull, StdlibConfigBool:
		return nil
	case StdlibConfigNumber:
		if !stdlibJSONNumberValid(value.Number) {
			return fmt.Errorf("JSON value: %w: invalid number %q", ErrStdlibInvalid, value.Number)
		}
	case StdlibConfigString:
		if !utf8.ValidString(value.String) {
			return fmt.Errorf("JSON value: %w: invalid UTF-8 string", ErrStdlibInvalid)
		}
		if int64(len(value.String)) > limits.MaxStringBytes {
			return stdlibLimitError("JSON value", "string bytes", int64(len(value.String)), limits.MaxStringBytes)
		}
	case StdlibConfigArray:
		for _, item := range value.Array {
			if err := stdlibJSONCheckValue(ctx, item, limits, depth+1, nodes); err != nil {
				return err
			}
		}
	case StdlibConfigObject:
		if len(value.Object) > limits.MaxObjectFields {
			return stdlibLimitError("JSON value", "object fields", int64(len(value.Object)), int64(limits.MaxObjectFields))
		}
		seen := make(map[string]struct{}, len(value.Object))
		for _, field := range value.Object {
			if !utf8.ValidString(field.Key) {
				return fmt.Errorf("JSON value: %w: invalid UTF-8 object key", ErrStdlibInvalid)
			}
			if _, ok := seen[field.Key]; ok {
				return fmt.Errorf("JSON value: %w: duplicate object key %q", ErrStdlibInvalid, field.Key)
			}
			seen[field.Key] = struct{}{}
			if err := stdlibJSONCheckValue(ctx, field.Value, limits, depth+1, nodes); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("JSON value: %w: unknown kind %q", ErrStdlibInvalid, value.Kind)
	}
	return nil
}

func stdlibJSONOptionalLimits(limits ...StdlibJSONLimits) StdlibJSONLimits {
	if len(limits) == 0 {
		return StdlibJSONLimits{}.normalized()
	}
	return limits[0].normalized()
}

func stdlibJSONNumberValid(number string) bool {
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

func stdlibJSONBoundOutput(data []byte, limits StdlibJSONLimits) ([]byte, error) {
	if int64(len(data)) > limits.MaxOutputBytes {
		return nil, stdlibLimitError("JSON output", "output bytes", int64(len(data)), limits.MaxOutputBytes)
	}
	return append([]byte(nil), data...), nil
}

// StdlibJSONOptions controls optional JSON serialization.
type StdlibJSONOptions struct {
	Pretty    bool
	Indent    string
	Prefix    string
	Canonical bool
	Limits    StdlibJSONLimits
}

// StdlibJSONStringify emits compact deterministic JSON preserving object
// insertion order and number spelling.
func StdlibJSONStringify(value StdlibConfigValue, limits ...StdlibJSONLimits) ([]byte, error) {
	return stdlibJSONSerialize(value, stdlibJSONOptionalLimits(limits...), false)
}

// StdlibStringifyJSON is the adapter-facing spelling of StdlibJSONStringify.
// It accepts both the context-aware adapter form (ctx, value, limits) and the
// compact Go form (value[, limits]) for compatibility with existing
// callers.
func StdlibStringifyJSON(args ...interface{}) ([]byte, error) {
	if len(args) == 3 {
		ctx, ctxOK := args[0].(context.Context)
		value, valueOK := args[1].(StdlibConfigValue)
		limits, limitsOK := args[2].(StdlibJSONLimits)
		if ctxOK && valueOK && limitsOK {
			if err := stdlibCheckContext(ctx, "JSON stringify"); err != nil {
				return nil, err
			}
			return StdlibJSONStringify(value, limits)
		}
	}
	if len(args) >= 1 {
		value, valueOK := args[0].(StdlibConfigValue)
		if valueOK {
			if len(args) == 1 {
				return StdlibJSONStringify(value)
			}
			if limits, limitsOK := args[1].(StdlibJSONLimits); limitsOK {
				return StdlibJSONStringify(value, limits)
			}
		}
	}
	return nil, fmt.Errorf("JSON stringify: %w: expected value and optional limits", ErrStdlibInvalid)
}

// StdlibJSONPretty emits deterministic two-space-indented JSON.
func StdlibJSONPretty(value StdlibConfigValue, limits ...StdlibJSONLimits) ([]byte, error) {
	return StdlibJSONPrettyIndent(value, "", "  ", limits...)
}

// StdlibPrettyJSON accepts the adapter form (ctx, value, indent, limits) and
// the compact Go form (value[, limits]).
func StdlibPrettyJSON(args ...interface{}) ([]byte, error) {
	if len(args) == 4 {
		ctx, ctxOK := args[0].(context.Context)
		value, valueOK := args[1].(StdlibConfigValue)
		indent, indentOK := args[2].(string)
		limits, limitsOK := args[3].(StdlibJSONLimits)
		if ctxOK && valueOK && indentOK && limitsOK {
			if err := stdlibCheckContext(ctx, "JSON pretty"); err != nil {
				return nil, err
			}
			return StdlibJSONPrettyIndent(value, "", indent, limits)
		}
	}
	if len(args) >= 1 {
		value, valueOK := args[0].(StdlibConfigValue)
		if valueOK {
			if len(args) == 1 {
				return StdlibJSONPretty(value)
			}
			if limits, limitsOK := args[1].(StdlibJSONLimits); limitsOK {
				return StdlibJSONPretty(value, limits)
			}
		}
	}
	return nil, fmt.Errorf("JSON pretty: %w: expected value, indent, and limits", ErrStdlibInvalid)
}

// StdlibJSONPrettyIndent emits deterministic JSON with caller-selected
// encoding/json-compatible prefix and indentation.
func StdlibJSONPrettyIndent(value StdlibConfigValue, prefix, indent string, limits ...StdlibJSONLimits) ([]byte, error) {
	lim := stdlibJSONOptionalLimits(limits...)
	compact, err := StdlibJSONStringify(value, lim)
	if err != nil {
		return nil, err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, compact, prefix, indent); err != nil {
		return nil, fmt.Errorf("JSON pretty: %w", err)
	}
	return stdlibJSONBoundOutput(pretty.Bytes(), lim)
}

// StdlibStringifyJSONWithOptions serializes according to options.
func StdlibStringifyJSONWithOptions(value StdlibConfigValue, options StdlibJSONOptions) ([]byte, error) {
	if options.Canonical {
		return StdlibJSONCanonicalize(value, options.Limits)
	}
	if options.Pretty {
		indent := options.Indent
		if indent == "" {
			indent = "  "
		}
		return StdlibJSONPrettyIndent(value, options.Prefix, indent, options.Limits)
	}
	return StdlibJSONStringify(value, options.Limits)
}

func stdlibJSONSerialize(value StdlibConfigValue, limits StdlibJSONLimits, canonical bool) ([]byte, error) {
	if err := stdlibJSONCheckValue(context.Background(), value, limits, 0, new(int)); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := stdlibJSONEncode(&out, value, canonical); err != nil {
		return nil, err
	}
	return stdlibJSONBoundOutput(out.Bytes(), limits)
}

func stdlibJSONEncode(out *bytes.Buffer, value StdlibConfigValue, canonical bool) error {
	switch value.Kind {
	case StdlibConfigNull:
		out.WriteString("null")
	case StdlibConfigBool:
		if value.Bool {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
	case StdlibConfigNumber:
		if canonical {
			number, err := stdlibJSONCanonicalNumber(value.Number)
			if err != nil {
				return err
			}
			out.WriteString(number)
		} else {
			out.WriteString(value.Number)
		}
	case StdlibConfigString:
		stdlibJSONWriteString(out, value.String)
	case StdlibConfigArray:
		out.WriteByte('[')
		for i, item := range value.Array {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := stdlibJSONEncode(out, item, canonical); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case StdlibConfigObject:
		fields := append([]StdlibConfigField(nil), value.Object...)
		if canonical {
			sort.SliceStable(fields, func(i, j int) bool { return stdlibJSONUTF16Less(fields[i].Key, fields[j].Key) })
		}
		out.WriteByte('{')
		for i, field := range fields {
			if i > 0 {
				out.WriteByte(',')
			}
			stdlibJSONWriteString(out, field.Key)
			out.WriteByte(':')
			if err := stdlibJSONEncode(out, field.Value, canonical); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	default:
		return fmt.Errorf("JSON encode: %w: unknown kind %q", ErrStdlibInvalid, value.Kind)
	}
	return nil
}

// StdlibJSONCanonicalize accepts the adapter form (ctx, value, limits) and
// the compact Go form (value[, limits]).
func StdlibJSONCanonicalize(args ...interface{}) ([]byte, error) {
	if len(args) == 3 {
		ctx, ctxOK := args[0].(context.Context)
		value, valueOK := args[1].(StdlibConfigValue)
		limits, limitsOK := args[2].(StdlibJSONLimits)
		if ctxOK && valueOK && limitsOK {
			if err := stdlibCheckContext(ctx, "JSON canonicalize"); err != nil {
				return nil, err
			}
			return stdlibJSONSerialize(value, limits, true)
		}
	}
	if len(args) >= 1 {
		value, valueOK := args[0].(StdlibConfigValue)
		if valueOK {
			if len(args) == 1 {
				return stdlibJSONSerialize(value, stdlibJSONOptionalLimits(), true)
			}
			if limits, limitsOK := args[1].(StdlibJSONLimits); limitsOK {
				return stdlibJSONSerialize(value, limits, true)
			}
		}
	}
	return nil, fmt.Errorf("JSON canonicalize: %w: expected value and optional limits", ErrStdlibInvalid)
}

// StdlibJSONCanonical is a compatibility alias for StdlibJSONCanonicalize.
func StdlibJSONCanonical(value StdlibConfigValue, limits ...StdlibJSONLimits) ([]byte, error) {
	args := []interface{}{value}
	if len(limits) != 0 {
		args = append(args, limits[0])
	}
	return StdlibJSONCanonicalize(args...)
}

// StdlibCanonicalizeJSON is a compatibility alias for StdlibJSONCanonicalize.
func StdlibCanonicalizeJSON(value StdlibConfigValue) ([]byte, error) {
	return StdlibJSONCanonicalize(value)
}

func stdlibJSONCanonicalNumber(number string) (string, error) {
	if !stdlibJSONNumberValid(number) {
		return "", fmt.Errorf("JSON canonical: %w: invalid number %q", ErrStdlibInvalid, number)
	}
	f, err := strconv.ParseFloat(number, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return "", fmt.Errorf("JSON canonical: %w: number %q is outside finite binary64", ErrStdlibInvalid, number)
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

func stdlibJSONWriteString(out *bytes.Buffer, value string) {
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

func stdlibJSONUTF16Less(a, b string) bool {
	aa, bb := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(aa) && i < len(bb); i++ {
		if aa[i] != bb[i] {
			return aa[i] < bb[i]
		}
	}
	return len(aa) < len(bb)
}

func stdlibJSONPointerTokens(pointer string, limits StdlibJSONLimits) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("JSON Pointer: %w: pointer must be empty or begin with '/'", ErrStdlibInvalid)
	}
	raw := strings.Split(pointer[1:], "/")
	if len(raw) > limits.MaxPointerTokens {
		return nil, stdlibLimitError("JSON Pointer", "tokens", int64(len(raw)), int64(limits.MaxPointerTokens))
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
				return nil, fmt.Errorf("JSON Pointer: %w: invalid escape in token %q", ErrStdlibInvalid, part)
			}
			if part[j+1] == '0' {
				b.WriteByte('~')
			} else {
				b.WriteByte('/')
			}
			j++
		}
		if !utf8.ValidString(b.String()) {
			return nil, fmt.Errorf("JSON Pointer: %w: invalid UTF-8 token", ErrStdlibInvalid)
		}
		tokens[i] = b.String()
	}
	return tokens, nil
}

func stdlibJSONArrayIndex(token string, length int, allowAppend bool) (int, bool, error) {
	if token == "-" {
		if allowAppend {
			return length, true, nil
		}
		return 0, false, fmt.Errorf("JSON Pointer: %w: '-' is only valid for add", ErrStdlibInvalid)
	}
	if token == "" || (len(token) > 1 && token[0] == '0') {
		return 0, false, fmt.Errorf("JSON Pointer: %w: invalid array index %q", ErrStdlibInvalid, token)
	}
	for _, r := range token {
		if r < '0' || r > '9' {
			return 0, false, fmt.Errorf("JSON Pointer: %w: invalid array index %q", ErrStdlibInvalid, token)
		}
	}
	index, err := strconv.Atoi(token)
	if err != nil || index > length || (!allowAppend && index == length) {
		return 0, false, fmt.Errorf("JSON Pointer: %w: array index %q out of range", ErrStdlibInvalid, token)
	}
	return index, index == length, nil
}

func stdlibJSONClone(value StdlibConfigValue) StdlibConfigValue {
	clone := value
	if value.Array != nil {
		clone.Array = make([]StdlibConfigValue, len(value.Array))
		for i := range value.Array {
			clone.Array[i] = stdlibJSONClone(value.Array[i])
		}
	}
	if value.Object != nil {
		clone.Object = make([]StdlibConfigField, len(value.Object))
		for i, field := range value.Object {
			clone.Object[i] = StdlibConfigField{Key: field.Key, Value: stdlibJSONClone(field.Value)}
		}
	}
	return clone
}

func stdlibJSONObjectIndex(object []StdlibConfigField, key string) (int, error) {
	index := -1
	for i, field := range object {
		if field.Key == key {
			if index >= 0 {
				return -1, fmt.Errorf("JSON object: %w: duplicate key %q", ErrStdlibInvalid, key)
			}
			index = i
		}
	}
	return index, nil
}

func stdlibJSONPointerGetValue(value StdlibConfigValue, tokens []string) (StdlibConfigValue, error) {
	for _, token := range tokens {
		switch value.Kind {
		case StdlibConfigObject:
			index, err := stdlibJSONObjectIndex(value.Object, token)
			if err != nil || index < 0 {
				return StdlibConfigValue{}, fmt.Errorf("JSON Pointer: %w: member %q not found", ErrStdlibInvalid, token)
			}
			value = value.Object[index].Value
		case StdlibConfigArray:
			index, appendItem, err := stdlibJSONArrayIndex(token, len(value.Array), false)
			if err != nil || appendItem || index >= len(value.Array) {
				return StdlibConfigValue{}, fmt.Errorf("JSON Pointer: %w: array item %q not found", ErrStdlibInvalid, token)
			}
			value = value.Array[index]
		default:
			return StdlibConfigValue{}, fmt.Errorf("JSON Pointer: %w: scalar cannot be traversed", ErrStdlibInvalid)
		}
	}
	return value, nil
}

// StdlibJSONPointerGet returns a detached value at an RFC 6901 pointer.
func StdlibJSONPointerGet(value StdlibConfigValue, pointer string, limits ...StdlibJSONLimits) (StdlibConfigValue, error) {
	lim := stdlibJSONOptionalLimits(limits...)
	if err := stdlibJSONCheckValue(context.Background(), value, lim, 0, new(int)); err != nil {
		return StdlibConfigValue{}, err
	}
	tokens, err := stdlibJSONPointerTokens(pointer, lim)
	if err != nil {
		return StdlibConfigValue{}, err
	}
	resolved, err := stdlibJSONPointerGetValue(value, tokens)
	if err != nil {
		return StdlibConfigValue{}, err
	}
	return stdlibJSONClone(resolved), nil
}

func stdlibJSONPointerAdd(value StdlibConfigValue, tokens []string, replacement StdlibConfigValue) (StdlibConfigValue, error) {
	if len(tokens) == 0 {
		return stdlibJSONClone(replacement), nil
	}
	clone := stdlibJSONClone(value)
	token := tokens[0]
	switch clone.Kind {
	case StdlibConfigObject:
		index, err := stdlibJSONObjectIndex(clone.Object, token)
		if err != nil {
			return StdlibConfigValue{}, err
		}
		if len(tokens) == 1 {
			if index < 0 {
				clone.Object = append(clone.Object, StdlibConfigField{Key: token, Value: stdlibJSONClone(replacement)})
			} else {
				clone.Object[index].Value = stdlibJSONClone(replacement)
			}
			return clone, nil
		}
		if index < 0 {
			return StdlibConfigValue{}, fmt.Errorf("JSON Pointer: %w: intermediate member %q not found", ErrStdlibInvalid, token)
		}
		child, err := stdlibJSONPointerAdd(clone.Object[index].Value, tokens[1:], replacement)
		if err != nil {
			return StdlibConfigValue{}, err
		}
		clone.Object[index].Value = child
		return clone, nil
	case StdlibConfigArray:
		index, appendItem, err := stdlibJSONArrayIndex(token, len(clone.Array), true)
		if err != nil {
			return StdlibConfigValue{}, err
		}
		if len(tokens) == 1 {
			if appendItem {
				clone.Array = append(clone.Array, stdlibJSONClone(replacement))
				return clone, nil
			}
			clone.Array = append(clone.Array, StdlibConfigValue{})
			copy(clone.Array[index+1:], clone.Array[index:])
			clone.Array[index] = stdlibJSONClone(replacement)
			return clone, nil
		}
		if index >= len(clone.Array) {
			return StdlibConfigValue{}, fmt.Errorf("JSON Pointer: %w: intermediate array index out of range", ErrStdlibInvalid)
		}
		child, err := stdlibJSONPointerAdd(clone.Array[index], tokens[1:], replacement)
		if err != nil {
			return StdlibConfigValue{}, err
		}
		clone.Array[index] = child
		return clone, nil
	default:
		return StdlibConfigValue{}, fmt.Errorf("JSON Pointer: %w: parent is scalar", ErrStdlibInvalid)
	}
}

// StdlibJSONPointerSet returns a detached document with an RFC 6902 add-style
// value at the pointer. Object members are created or replaced; array indices
// insert and '-' appends.
func StdlibJSONPointerSet(value StdlibConfigValue, pointer string, replacement StdlibConfigValue, limits ...StdlibJSONLimits) (StdlibConfigValue, error) {
	lim := stdlibJSONOptionalLimits(limits...)
	if err := stdlibJSONCheckValue(context.Background(), value, lim, 0, new(int)); err != nil {
		return StdlibConfigValue{}, err
	}
	if err := stdlibJSONCheckValue(context.Background(), replacement, lim, 0, new(int)); err != nil {
		return StdlibConfigValue{}, err
	}
	tokens, err := stdlibJSONPointerTokens(pointer, lim)
	if err != nil {
		return StdlibConfigValue{}, err
	}
	result, err := stdlibJSONPointerAdd(value, tokens, replacement)
	if err != nil {
		return StdlibConfigValue{}, err
	}
	return result, stdlibJSONCheckValue(context.Background(), result, lim, 0, new(int))
}

func stdlibJSONPointerRemove(value StdlibConfigValue, tokens []string) (StdlibConfigValue, error) {
	if len(tokens) == 0 {
		return StdlibConfigValue{}, fmt.Errorf("JSON Pointer: %w: cannot remove document root", ErrStdlibInvalid)
	}
	clone := stdlibJSONClone(value)
	token := tokens[0]
	if len(tokens) == 1 {
		switch clone.Kind {
		case StdlibConfigObject:
			index, err := stdlibJSONObjectIndex(clone.Object, token)
			if err != nil || index < 0 {
				return StdlibConfigValue{}, fmt.Errorf("JSON Pointer: %w: member %q not found", ErrStdlibInvalid, token)
			}
			clone.Object = append(clone.Object[:index], clone.Object[index+1:]...)
			return clone, nil
		case StdlibConfigArray:
			index, _, err := stdlibJSONArrayIndex(token, len(clone.Array), false)
			if err != nil || index >= len(clone.Array) {
				return StdlibConfigValue{}, fmt.Errorf("JSON Pointer: %w: array item %q not found", ErrStdlibInvalid, token)
			}
			clone.Array = append(clone.Array[:index], clone.Array[index+1:]...)
			return clone, nil
		}
		return StdlibConfigValue{}, fmt.Errorf("JSON Pointer: %w: parent is scalar", ErrStdlibInvalid)
	}
	switch clone.Kind {
	case StdlibConfigObject:
		index, err := stdlibJSONObjectIndex(clone.Object, token)
		if err != nil || index < 0 {
			return StdlibConfigValue{}, fmt.Errorf("JSON Pointer: %w: intermediate member %q not found", ErrStdlibInvalid, token)
		}
		child, err := stdlibJSONPointerRemove(clone.Object[index].Value, tokens[1:])
		if err != nil {
			return StdlibConfigValue{}, err
		}
		clone.Object[index].Value = child
		return clone, nil
	case StdlibConfigArray:
		index, _, err := stdlibJSONArrayIndex(token, len(clone.Array), false)
		if err != nil || index >= len(clone.Array) {
			return StdlibConfigValue{}, fmt.Errorf("JSON Pointer: %w: intermediate array item %q not found", ErrStdlibInvalid, token)
		}
		child, err := stdlibJSONPointerRemove(clone.Array[index], tokens[1:])
		if err != nil {
			return StdlibConfigValue{}, err
		}
		clone.Array[index] = child
		return clone, nil
	default:
		return StdlibConfigValue{}, fmt.Errorf("JSON Pointer: %w: parent is scalar", ErrStdlibInvalid)
	}
}

// StdlibJSONRemove is the patch engine's explicit remove helper.
func StdlibJSONRemove(value StdlibConfigValue, tokens []string) (StdlibConfigValue, error) {
	return stdlibJSONPointerRemove(value, tokens)
}

// StdlibJSONPointerRemove returns a detached document with the pointer target
// removed. Removing the document root is rejected because this API returns one
// value and cannot represent an absent document separately from JSON null.
func StdlibJSONPointerRemove(value StdlibConfigValue, pointer string, limits ...StdlibJSONLimits) (StdlibConfigValue, error) {
	lim := stdlibJSONOptionalLimits(limits...)
	if err := stdlibJSONCheckValue(context.Background(), value, lim, 0, new(int)); err != nil {
		return StdlibConfigValue{}, err
	}
	tokens, err := stdlibJSONPointerTokens(pointer, lim)
	if err != nil {
		return StdlibConfigValue{}, err
	}
	result, err := stdlibJSONPointerRemove(value, tokens)
	if err != nil {
		return StdlibConfigValue{}, err
	}
	return result, stdlibJSONCheckValue(context.Background(), result, lim, 0, new(int))
}

// StdlibJSONPatchOperation is one typed RFC 6902 operation for Go callers.
// A zero Value means that the operation omitted its value member.
type StdlibJSONPatchOperation struct {
	Op    string
	Path  string
	From  string
	Value StdlibConfigValue
}

func stdlibJSONPatchValue(operations []StdlibJSONPatchOperation) StdlibConfigValue {
	patch := StdlibConfigValue{Kind: StdlibConfigArray, Array: make([]StdlibConfigValue, len(operations))}
	for i, operation := range operations {
		fields := []StdlibConfigField{{Key: "op", Value: StdlibConfigValue{Kind: StdlibConfigString, String: operation.Op}}, {Key: "path", Value: StdlibConfigValue{Kind: StdlibConfigString, String: operation.Path}}}
		if operation.From != "" {
			fields = append(fields, StdlibConfigField{Key: "from", Value: StdlibConfigValue{Kind: StdlibConfigString, String: operation.From}})
		}
		if operation.Value.Kind != "" {
			fields = append(fields, StdlibConfigField{Key: "value", Value: operation.Value})
		}
		patch.Array[i] = StdlibConfigValue{Kind: StdlibConfigObject, Object: fields}
	}
	return patch
}

// StdlibApplyJSONPatch applies typed RFC 6902 operations transactionally.
func StdlibApplyJSONPatch(ctx context.Context, document StdlibConfigValue, operations []StdlibJSONPatchOperation, limits StdlibJSONLimits) (StdlibConfigValue, error) {
	return stdlibJSONPatchApply(ctx, document, stdlibJSONPatchValue(operations), limits)
}

// StdlibJSONPatchApply applies a JSON Patch document with default limits.
func StdlibJSONPatchApply(document, patch StdlibConfigValue) (StdlibConfigValue, error) {
	return stdlibJSONPatchApply(context.Background(), document, patch, StdlibJSONLimits{})
}

func stdlibJSONPatchApply(ctx context.Context, document, patch StdlibConfigValue, limits StdlibJSONLimits) (StdlibConfigValue, error) {
	limits = limits.normalized()
	if err := stdlibCheckContext(ctx, "JSON Patch"); err != nil {
		return StdlibConfigValue{}, err
	}
	if err := stdlibJSONCheckValue(ctx, document, limits, 0, new(int)); err != nil {
		return StdlibConfigValue{}, err
	}
	if err := stdlibJSONCheckValue(ctx, patch, limits, 0, new(int)); err != nil {
		return StdlibConfigValue{}, err
	}
	if patch.Kind != StdlibConfigArray {
		return StdlibConfigValue{}, fmt.Errorf("JSON Patch: %w: patch must be an array", ErrStdlibInvalid)
	}
	if len(patch.Array) > limits.MaxPatchOps {
		return StdlibConfigValue{}, stdlibLimitError("JSON Patch", "operations", int64(len(patch.Array)), int64(limits.MaxPatchOps))
	}
	result := stdlibJSONClone(document)
	for i, operation := range patch.Array {
		if err := stdlibCheckContext(ctx, "JSON Patch"); err != nil {
			return StdlibConfigValue{}, err
		}
		if err := stdlibJSONApplyOperation(result, operation, i, limits, &result); err != nil {
			return StdlibConfigValue{}, err
		}
		if err := stdlibJSONCheckValue(ctx, result, limits, 0, new(int)); err != nil {
			return StdlibConfigValue{}, err
		}
	}
	return result, nil
}

func stdlibJSONApplyOperation(document, operation StdlibConfigValue, index int, limits StdlibJSONLimits, result *StdlibConfigValue) error {
	if operation.Kind != StdlibConfigObject {
		return fmt.Errorf("JSON Patch operation %d: %w: operation must be object", index, ErrStdlibInvalid)
	}
	op, ok := stdlibJSONObjectField(operation, "op")
	pathValue, pathOK := stdlibJSONObjectField(operation, "path")
	if !ok || op.Kind != StdlibConfigString || !pathOK || pathValue.Kind != StdlibConfigString {
		return fmt.Errorf("JSON Patch operation %d: %w: op and path are required strings", index, ErrStdlibInvalid)
	}
	path := pathValue.String
	tokens, err := stdlibJSONPointerTokens(path, limits)
	if err != nil {
		return fmt.Errorf("JSON Patch operation %d: %w", index, err)
	}
	value, hasValue := stdlibJSONObjectField(operation, "value")
	switch op.String {
	case "add":
		if !hasValue {
			return fmt.Errorf("JSON Patch operation %d: %w: add requires value", index, ErrStdlibInvalid)
		}
		updated, err := stdlibJSONPointerAdd(*result, tokens, value)
		if err != nil {
			return fmt.Errorf("JSON Patch operation %d: %w", index, err)
		}
		*result = updated
	case "remove":
		updated, err := stdlibJSONPointerRemove(*result, tokens)
		if err != nil {
			return fmt.Errorf("JSON Patch operation %d: %w", index, err)
		}
		*result = updated
	case "replace":
		if !hasValue {
			return fmt.Errorf("JSON Patch operation %d: %w: replace requires value", index, ErrStdlibInvalid)
		}
		if len(tokens) > 0 {
			if _, err := stdlibJSONPointerGetValue(*result, tokens); err != nil {
				return fmt.Errorf("JSON Patch operation %d: %w", index, err)
			}
		}
		updated, err := stdlibJSONPointerReplace(*result, tokens, value)
		if err != nil {
			return fmt.Errorf("JSON Patch operation %d: %w", index, err)
		}
		*result = updated
	case "move", "copy":
		fromValue, ok := stdlibJSONObjectField(operation, "from")
		if !ok || fromValue.Kind != StdlibConfigString {
			return fmt.Errorf("JSON Patch operation %d: %w: from is required", index, ErrStdlibInvalid)
		}
		fromTokens, err := stdlibJSONPointerTokens(fromValue.String, limits)
		if err != nil {
			return fmt.Errorf("JSON Patch operation %d: %w", index, err)
		}
		if op.String == "move" && stdlibJSONPointerDescendant(fromTokens, tokens) {
			return fmt.Errorf("JSON Patch operation %d: %w: move target is a descendant", index, ErrStdlibInvalid)
		}
		moved, err := stdlibJSONPointerGetValue(*result, fromTokens)
		if err != nil {
			return fmt.Errorf("JSON Patch operation %d: %w", index, err)
		}
		if op.String == "move" {
			*result, err = stdlibJSONPointerRemove(*result, fromTokens)
			if err != nil {
				return fmt.Errorf("JSON Patch operation %d: %w", index, err)
			}
		}
		*result, err = stdlibJSONPointerAdd(*result, tokens, moved)
		if err != nil {
			return fmt.Errorf("JSON Patch operation %d: %w", index, err)
		}
	case "test":
		if !hasValue {
			return fmt.Errorf("JSON Patch operation %d: %w: test requires value", index, ErrStdlibInvalid)
		}
		actual, err := stdlibJSONPointerGetValue(*result, tokens)
		if err != nil || !stdlibJSONEqual(actual, value) {
			return fmt.Errorf("JSON Patch operation %d: %w: test failed", index, ErrStdlibInvalid)
		}
	default:
		return fmt.Errorf("JSON Patch operation %d: %w: unsupported op %q", index, ErrStdlibInvalid, op.String)
	}
	return nil
}

func stdlibJSONObjectField(object StdlibConfigValue, key string) (StdlibConfigValue, bool) {
	for _, field := range object.Object {
		if field.Key == key {
			return field.Value, true
		}
	}
	return StdlibConfigValue{}, false
}

func stdlibJSONPointerReplace(value StdlibConfigValue, tokens []string, replacement StdlibConfigValue) (StdlibConfigValue, error) {
	if len(tokens) == 0 {
		return stdlibJSONClone(replacement), nil
	}
	clone := stdlibJSONClone(value)
	if len(tokens) == 1 {
		switch clone.Kind {
		case StdlibConfigObject:
			index, err := stdlibJSONObjectIndex(clone.Object, tokens[0])
			if err == nil && index >= 0 {
				clone.Object[index].Value = stdlibJSONClone(replacement)
				return clone, nil
			}
		case StdlibConfigArray:
			index, _, err := stdlibJSONArrayIndex(tokens[0], len(clone.Array), false)
			if err == nil && index < len(clone.Array) {
				clone.Array[index] = stdlibJSONClone(replacement)
				return clone, nil
			}
		}
		return StdlibConfigValue{}, fmt.Errorf("JSON Pointer: %w: replace target not found", ErrStdlibInvalid)
	}
	child, err := stdlibJSONPointerGetValue(clone, tokens[:1])
	if err != nil {
		return StdlibConfigValue{}, err
	}
	updated, err := stdlibJSONPointerReplace(child, tokens[1:], replacement)
	if err != nil {
		return StdlibConfigValue{}, err
	}
	return stdlibJSONPointerReplace(clone, tokens[:1], updated)
}

func stdlibJSONPointerDescendant(parent, child []string) bool {
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

// StdlibJSONMergePatch applies RFC 7396 with default limits.
func StdlibJSONMergePatch(target, patch StdlibConfigValue) StdlibConfigValue {
	result, err := StdlibApplyJSONMergePatch(context.Background(), target, patch, StdlibJSONLimits{})
	if err != nil {
		return StdlibConfigValue{}
	}
	return result
}

// StdlibApplyJSONMergePatch applies RFC 7396 transactionally with context and
// explicit limits.
func StdlibApplyJSONMergePatch(ctx context.Context, target, patch StdlibConfigValue, limits StdlibJSONLimits) (StdlibConfigValue, error) {
	limits = limits.normalized()
	if err := stdlibJSONCheckValue(ctx, target, limits, 0, new(int)); err != nil {
		return StdlibConfigValue{}, err
	}
	if err := stdlibJSONCheckValue(ctx, patch, limits, 0, new(int)); err != nil {
		return StdlibConfigValue{}, err
	}
	result, err := stdlibJSONMergeValue(ctx, target, patch, limits, 0)
	if err != nil {
		return StdlibConfigValue{}, err
	}
	return result, stdlibJSONCheckValue(ctx, result, limits, 0, new(int))
}

// StdlibJSONMergePatchContext is the error-returning context-aware spelling
// of StdlibJSONMergePatch.
func StdlibJSONMergePatchContext(ctx context.Context, target, patch StdlibConfigValue, limits StdlibJSONLimits) (StdlibConfigValue, error) {
	return StdlibApplyJSONMergePatch(ctx, target, patch, limits)
}

func stdlibJSONMergeValue(ctx context.Context, target, patch StdlibConfigValue, limits StdlibJSONLimits, depth int) (StdlibConfigValue, error) {
	if err := stdlibCheckContext(ctx, "JSON Merge Patch"); err != nil {
		return StdlibConfigValue{}, err
	}
	if depth > limits.MaxDepth {
		return StdlibConfigValue{}, stdlibLimitError("JSON Merge Patch", "depth", int64(depth), int64(limits.MaxDepth))
	}
	if patch.Kind != StdlibConfigObject {
		return stdlibJSONClone(patch), nil
	}
	result := stdlibJSONClone(target)
	if result.Kind != StdlibConfigObject {
		result = StdlibConfigValue{Kind: StdlibConfigObject}
	}
	for _, field := range patch.Object {
		index, err := stdlibJSONObjectIndex(result.Object, field.Key)
		if err != nil {
			return StdlibConfigValue{}, err
		}
		if field.Value.Kind == StdlibConfigNull {
			if index >= 0 {
				result.Object = append(result.Object[:index], result.Object[index+1:]...)
			}
			continue
		}
		base := StdlibConfigValue{Kind: StdlibConfigNull}
		if index >= 0 {
			base = result.Object[index].Value
		}
		merged, err := stdlibJSONMergeValue(ctx, base, field.Value, limits, depth+1)
		if err != nil {
			return StdlibConfigValue{}, err
		}
		if index >= 0 {
			result.Object[index].Value = merged
		} else {
			result.Object = append(result.Object, StdlibConfigField{Key: field.Key, Value: merged})
		}
	}
	return result, nil
}

func stdlibJSONMergePatchValue(ctx context.Context, target, patch StdlibConfigValue, limits StdlibJSONLimits) (StdlibConfigValue, error) {
	return StdlibApplyJSONMergePatch(ctx, target, patch, limits)
}

func stdlibJSONEqual(a, b StdlibConfigValue) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case StdlibConfigNull:
		return true
	case StdlibConfigBool:
		return a.Bool == b.Bool
	case StdlibConfigNumber:
		return stdlibJSONNumberEqual(a.Number, b.Number)
	case StdlibConfigString:
		return a.String == b.String
	case StdlibConfigArray:
		if len(a.Array) != len(b.Array) {
			return false
		}
		for i := range a.Array {
			if !stdlibJSONEqual(a.Array[i], b.Array[i]) {
				return false
			}
		}
		return true
	case StdlibConfigObject:
		if len(a.Object) != len(b.Object) {
			return false
		}
		for _, field := range a.Object {
			other, ok := stdlibJSONObjectField(b, field.Key)
			if !ok || !stdlibJSONEqual(field.Value, other) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func stdlibJSONNumberEqual(a, b string) bool {
	ra, oka := stdlibJSONExactRat(a)
	rb, okb := stdlibJSONExactRat(b)
	return oka && okb && ra.Cmp(rb) == 0
}

func stdlibJSONExactRat(number string) (*big.Rat, bool) {
	if !stdlibJSONNumberValid(number) {
		return nil, false
	}
	f, _, err := big.ParseFloat(number, 10, 256, big.ToNearestEven)
	if err != nil || f == nil {
		return nil, false
	}
	r, _ := f.Rat(nil)
	return r, true
}
