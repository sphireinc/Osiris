package osiris

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// StdlibConfigVersion identifies the ordered JSON/property representation.
const StdlibConfigVersion = "ordered-config-1"

// StdlibConfigLimits bounds parser work. Zero fields select 16 MiB, depth
// 64, 100,000 nodes, 1 MiB strings, and 10,000 fields per object.
type StdlibConfigLimits struct {
	MaxBytes        int64
	MaxDepth        int
	MaxNodes        int
	MaxStringBytes  int64
	MaxObjectFields int
}

func (l StdlibConfigLimits) normalized() StdlibConfigLimits {
	if l.MaxBytes <= 0 {
		l.MaxBytes = 16 << 20
	}
	if l.MaxDepth <= 0 {
		l.MaxDepth = 64
	}
	if l.MaxNodes <= 0 {
		l.MaxNodes = 100_000
	}
	if l.MaxStringBytes <= 0 {
		l.MaxStringBytes = 1 << 20
	}
	if l.MaxObjectFields <= 0 {
		l.MaxObjectFields = 10_000
	}
	return l
}

// StdlibConfigKind is the ordered value kind.
type StdlibConfigKind string

const (
	StdlibConfigNull   StdlibConfigKind = "null"
	StdlibConfigBool   StdlibConfigKind = "boolean"
	StdlibConfigNumber StdlibConfigKind = "number"
	StdlibConfigString StdlibConfigKind = "string"
	StdlibConfigArray  StdlibConfigKind = "array"
	StdlibConfigObject StdlibConfigKind = "object"
)

// StdlibConfigField is one object member in source order.
type StdlibConfigField struct {
	Key   string
	Value StdlibConfigValue
}

// StdlibConfigValue preserves object order and JSON number spelling. Object
// keys are unique; parser functions reject duplicate keys rather than making
// a hidden last-write-wins choice.
type StdlibConfigValue struct {
	Kind   StdlibConfigKind
	Bool   bool
	Number string
	String string
	Array  []StdlibConfigValue
	Object []StdlibConfigField
}

// MarshalJSON emits a deterministic JSON representation in preserved object
// order and original number spelling.
func (v StdlibConfigValue) MarshalJSON() ([]byte, error) {
	switch v.Kind {
	case StdlibConfigNull:
		return []byte("null"), nil
	case StdlibConfigBool:
		if v.Bool {
			return []byte("true"), nil
		}
		return []byte("false"), nil
	case StdlibConfigNumber:
		if !stdlibJSONNumberValid(v.Number) {
			return nil, fmt.Errorf("%w: invalid config number", ErrStdlibInvalid)
		}
		return []byte(v.Number), nil
	case StdlibConfigString:
		return json.Marshal(v.String)
	case StdlibConfigArray:
		var b bytes.Buffer
		b.WriteByte('[')
		for i, item := range v.Array {
			if i > 0 {
				b.WriteByte(',')
			}
			encoded, err := item.MarshalJSON()
			if err != nil {
				return nil, err
			}
			b.Write(encoded)
		}
		b.WriteByte(']')
		return b.Bytes(), nil
	case StdlibConfigObject:
		var b bytes.Buffer
		b.WriteByte('{')
		seen := make(map[string]struct{}, len(v.Object))
		for i, field := range v.Object {
			if _, ok := seen[field.Key]; ok {
				return nil, fmt.Errorf("%w: duplicate config key %q", ErrStdlibInvalid, field.Key)
			}
			seen[field.Key] = struct{}{}
			if i > 0 {
				b.WriteByte(',')
			}
			key, _ := json.Marshal(field.Key)
			value, err := field.Value.MarshalJSON()
			if err != nil {
				return nil, err
			}
			b.Write(key)
			b.WriteByte(':')
			b.Write(value)
		}
		b.WriteByte('}')
		return b.Bytes(), nil
	default:
		return nil, fmt.Errorf("%w: unknown config kind %q", ErrStdlibInvalid, v.Kind)
	}
}

// JSON returns the deterministic ordered JSON bytes.
func (v StdlibConfigValue) JSON() ([]byte, error) { return v.MarshalJSON() }

type stdlibJSONDecoder struct {
	limits StdlibConfigLimits
	nodes  int
}

func (d *stdlibJSONDecoder) node(depth int) error {
	if depth > d.limits.MaxDepth {
		return stdlibLimitError("config parse", "depth", int64(depth), int64(d.limits.MaxDepth))
	}
	d.nodes++
	if d.nodes > d.limits.MaxNodes {
		return stdlibLimitError("config parse", "nodes", int64(d.nodes), int64(d.limits.MaxNodes))
	}
	return nil
}

func (d *stdlibJSONDecoder) value(ctx context.Context, dec *json.Decoder, depth int) (StdlibConfigValue, error) {
	if err := stdlibCheckContext(ctx, "config parse"); err != nil {
		return StdlibConfigValue{}, err
	}
	if err := d.node(depth); err != nil {
		return StdlibConfigValue{}, err
	}
	token, err := dec.Token()
	if err != nil {
		return StdlibConfigValue{}, fmt.Errorf("config parse: %w: %v", ErrStdlibInvalid, err)
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '[':
			out := StdlibConfigValue{Kind: StdlibConfigArray}
			for dec.More() {
				item, err := d.value(ctx, dec, depth+1)
				if err != nil {
					return StdlibConfigValue{}, err
				}
				out.Array = append(out.Array, item)
			}
			closeToken, err := dec.Token()
			if err != nil || closeToken != json.Delim(']') {
				return StdlibConfigValue{}, fmt.Errorf("config array close: %w", ErrStdlibInvalid)
			}
			return out, nil
		case '{':
			out := StdlibConfigValue{Kind: StdlibConfigObject}
			seen := make(map[string]struct{})
			for dec.More() {
				keyToken, err := dec.Token()
				if err != nil {
					return StdlibConfigValue{}, fmt.Errorf("config object key: %w", err)
				}
				key, ok := keyToken.(string)
				if !ok || !utf8.ValidString(key) {
					return StdlibConfigValue{}, fmt.Errorf("%w: object key is not UTF-8 string", ErrStdlibInvalid)
				}
				if _, duplicate := seen[key]; duplicate {
					return StdlibConfigValue{}, fmt.Errorf("%w: duplicate config key %q", ErrStdlibInvalid, key)
				}
				seen[key] = struct{}{}
				if len(out.Object) >= d.limits.MaxObjectFields {
					return StdlibConfigValue{}, stdlibLimitError("config parse", "object fields", int64(len(out.Object)+1), int64(d.limits.MaxObjectFields))
				}
				child, err := d.value(ctx, dec, depth+1)
				if err != nil {
					return StdlibConfigValue{}, err
				}
				out.Object = append(out.Object, StdlibConfigField{Key: key, Value: child})
			}
			closeToken, err := dec.Token()
			if err != nil || closeToken != json.Delim('}') {
				return StdlibConfigValue{}, fmt.Errorf("config object close: %w", ErrStdlibInvalid)
			}
			return out, nil
		default:
			return StdlibConfigValue{}, fmt.Errorf("%w: unexpected delimiter %q", ErrStdlibInvalid, value)
		}
	case nil:
		return StdlibConfigValue{Kind: StdlibConfigNull}, nil
	case bool:
		return StdlibConfigValue{Kind: StdlibConfigBool, Bool: value}, nil
	case string:
		if int64(len(value)) > d.limits.MaxStringBytes {
			return StdlibConfigValue{}, stdlibLimitError("config parse", "string bytes", int64(len(value)), d.limits.MaxStringBytes)
		}
		return StdlibConfigValue{Kind: StdlibConfigString, String: value}, nil
	case json.Number:
		return StdlibConfigValue{Kind: StdlibConfigNumber, Number: string(value)}, nil
	default:
		return StdlibConfigValue{}, fmt.Errorf("%w: unsupported JSON token", ErrStdlibInvalid)
	}
}

// StdlibParseJSONConfig parses JSON into an ordered, duplicate-rejecting
// representation without consulting host configuration or map iteration.
func StdlibParseJSONConfig(ctx context.Context, data []byte, limits StdlibConfigLimits) (StdlibConfigValue, error) {
	limits = limits.normalized()
	if int64(len(data)) > limits.MaxBytes {
		return StdlibConfigValue{}, stdlibLimitError("config parse", "input bytes", int64(len(data)), limits.MaxBytes)
	}
	if !utf8.Valid(data) {
		return StdlibConfigValue{}, fmt.Errorf("config parse: %w: input is not valid UTF-8", ErrStdlibInvalid)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	d := &stdlibJSONDecoder{limits: limits}
	value, err := d.value(ctx, dec, 0)
	if err != nil {
		return StdlibConfigValue{}, err
	}
	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			return StdlibConfigValue{}, fmt.Errorf("%w: trailing JSON value", ErrStdlibInvalid)
		}
		return StdlibConfigValue{}, fmt.Errorf("%w: trailing JSON: %v", ErrStdlibInvalid, err)
	}
	return value, nil
}

// StdlibParseJSONConfigStream parses a bounded caller-owned stream and does
// not close it.
func StdlibParseJSONConfigStream(ctx context.Context, src io.Reader, limits StdlibConfigLimits) (StdlibConfigValue, error) {
	if src == nil {
		return StdlibConfigValue{}, fmt.Errorf("config parse: %w: nil source", ErrStdlibInvalid)
	}
	limits = limits.normalized()
	readLimit := limits.MaxBytes
	if readLimit < int64(^uint64(0)>>1) {
		readLimit++
	}
	data, err := io.ReadAll(io.LimitReader(&stdlibContextReader{ctx: ctx, r: src}, readLimit))
	if err != nil {
		return StdlibConfigValue{}, fmt.Errorf("config stream: %w", err)
	}
	return StdlibParseJSONConfig(ctx, data, limits)
}

// StdlibParsePropertiesConfig parses a small human-authored key=value
// format. Blank lines and # comments are ignored; values are JSON scalars,
// arrays, or objects, and source order is retained.
func StdlibParsePropertiesConfig(ctx context.Context, data []byte, limits StdlibConfigLimits) (StdlibConfigValue, error) {
	limits = limits.normalized()
	if int64(len(data)) > limits.MaxBytes {
		return StdlibConfigValue{}, stdlibLimitError("properties parse", "input bytes", int64(len(data)), limits.MaxBytes)
	}
	if !utf8.Valid(data) {
		return StdlibConfigValue{}, fmt.Errorf("properties parse: %w: input is not valid UTF-8", ErrStdlibInvalid)
	}
	root := StdlibConfigValue{Kind: StdlibConfigObject}
	seen := map[string]struct{}{}
	for lineNumber, line := range strings.Split(string(data), "\n") {
		if err := stdlibCheckContext(ctx, "properties parse"); err != nil {
			return StdlibConfigValue{}, err
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		separator := strings.IndexByte(line, '=')
		if separator <= 0 {
			return StdlibConfigValue{}, fmt.Errorf("%w: properties line %d needs key = JSON-value", ErrStdlibInvalid, lineNumber+1)
		}
		key := strings.TrimSpace(line[:separator])
		raw := strings.TrimSpace(line[separator+1:])
		if key == "" || !utf8.ValidString(key) {
			return StdlibConfigValue{}, fmt.Errorf("%w: properties line %d has invalid key", ErrStdlibInvalid, lineNumber+1)
		}
		if _, ok := seen[key]; ok {
			return StdlibConfigValue{}, fmt.Errorf("%w: duplicate property %q", ErrStdlibInvalid, key)
		}
		seen[key] = struct{}{}
		value, err := StdlibParseJSONConfig(ctx, []byte(raw), limits)
		if err != nil {
			return StdlibConfigValue{}, fmt.Errorf("properties line %d: %w", lineNumber+1, err)
		}
		root.Object = append(root.Object, StdlibConfigField{Key: key, Value: value})
		if len(root.Object) > limits.MaxObjectFields {
			return StdlibConfigValue{}, stdlibLimitError("properties parse", "object fields", int64(len(root.Object)), int64(limits.MaxObjectFields))
		}
	}
	return root, nil
}

// StdlibConfigSchemaType identifies one schema node type.
type StdlibConfigSchemaType string

const (
	StdlibSchemaAny     StdlibConfigSchemaType = "any"
	StdlibSchemaNull    StdlibConfigSchemaType = "null"
	StdlibSchemaBool    StdlibConfigSchemaType = "boolean"
	StdlibSchemaNumber  StdlibConfigSchemaType = "number"
	StdlibSchemaInteger StdlibConfigSchemaType = "integer"
	StdlibSchemaString  StdlibConfigSchemaType = "string"
	StdlibSchemaArray   StdlibConfigSchemaType = "array"
	StdlibSchemaObject  StdlibConfigSchemaType = "object"
)

// StdlibConfigSchema is a deliberately small deterministic schema language.
// Properties are selected by name, required order is preserved for reporting,
// and additional keys are rejected unless explicitly allowed.
type StdlibConfigSchema struct {
	Type            StdlibConfigSchemaType
	Required        []string
	Properties      map[string]StdlibConfigSchema
	Items           *StdlibConfigSchema
	AllowAdditional bool
	MinItems        int
	MaxItems        int
	MinProperties   int
	MaxProperties   int
	MinLength       int
	MaxLength       int
}

// StdlibConfigError is one path-aware schema violation.
type StdlibConfigError struct {
	Path    string
	Code    string
	Message string
}

func (e StdlibConfigError) Error() string { return e.Path + ": " + e.Message }

// StdlibConfigValidationError groups stable, source-order/schema-order
// validation errors.
type StdlibConfigValidationError struct{ Errors []StdlibConfigError }

func (e *StdlibConfigValidationError) Error() string {
	if e == nil || len(e.Errors) == 0 {
		return "config validation failed"
	}
	return fmt.Sprintf("config validation failed: %s", e.Errors[0].Error())
}

func stdlibConfigPath(parent, key string) string {
	if parent == "$" {
		return "$[" + strconv.Quote(key) + "]"
	}
	return parent + "[" + strconv.Quote(key) + "]"
}

func stdlibValidateConfig(path string, value StdlibConfigValue, schema StdlibConfigSchema, errorsOut *[]StdlibConfigError) {
	if schema.Type == "" {
		schema.Type = StdlibSchemaAny
	}
	valid := schema.Type == StdlibSchemaAny || (schema.Type == StdlibSchemaNull && value.Kind == StdlibConfigNull) || (schema.Type == StdlibSchemaBool && value.Kind == StdlibConfigBool) || (schema.Type == StdlibSchemaNumber && value.Kind == StdlibConfigNumber) || (schema.Type == StdlibSchemaInteger && value.Kind == StdlibConfigNumber && isStdlibInteger(value.Number)) || (schema.Type == StdlibSchemaString && value.Kind == StdlibConfigString) || (schema.Type == StdlibSchemaArray && value.Kind == StdlibConfigArray) || (schema.Type == StdlibSchemaObject && value.Kind == StdlibConfigObject)
	if !valid {
		*errorsOut = append(*errorsOut, StdlibConfigError{Path: path, Code: "type", Message: "expected " + string(schema.Type) + ", got " + string(value.Kind)})
		return
	}
	switch value.Kind {
	case StdlibConfigString:
		length := len([]rune(value.String))
		if schema.MinLength > 0 && length < schema.MinLength {
			*errorsOut = append(*errorsOut, StdlibConfigError{Path: path, Code: "min_length", Message: "string is shorter than minimum"})
		}
		if schema.MaxLength > 0 && length > schema.MaxLength {
			*errorsOut = append(*errorsOut, StdlibConfigError{Path: path, Code: "max_length", Message: "string is longer than maximum"})
		}
	case StdlibConfigArray:
		if schema.MinItems > 0 && len(value.Array) < schema.MinItems {
			*errorsOut = append(*errorsOut, StdlibConfigError{Path: path, Code: "min_items", Message: "array has too few items"})
		}
		if schema.MaxItems > 0 && len(value.Array) > schema.MaxItems {
			*errorsOut = append(*errorsOut, StdlibConfigError{Path: path, Code: "max_items", Message: "array has too many items"})
		}
		if schema.Items != nil {
			for i, item := range value.Array {
				stdlibValidateConfig(path+"["+strconv.Itoa(i)+"]", item, *schema.Items, errorsOut)
			}
		}
	case StdlibConfigObject:
		if schema.MinProperties > 0 && len(value.Object) < schema.MinProperties {
			*errorsOut = append(*errorsOut, StdlibConfigError{Path: path, Code: "min_properties", Message: "object has too few properties"})
		}
		if schema.MaxProperties > 0 && len(value.Object) > schema.MaxProperties {
			*errorsOut = append(*errorsOut, StdlibConfigError{Path: path, Code: "max_properties", Message: "object has too many properties"})
		}
		present := make(map[string]StdlibConfigValue, len(value.Object))
		for _, field := range value.Object {
			present[field.Key] = field.Value
		}
		for _, required := range schema.Required {
			if _, ok := present[required]; !ok {
				*errorsOut = append(*errorsOut, StdlibConfigError{Path: stdlibConfigPath(path, required), Code: "required", Message: "required property is missing"})
			}
		}
		for _, field := range value.Object {
			child, ok := schema.Properties[field.Key]
			if !ok {
				if !schema.AllowAdditional {
					*errorsOut = append(*errorsOut, StdlibConfigError{Path: stdlibConfigPath(path, field.Key), Code: "additional_property", Message: "additional property is not allowed"})
				}
				continue
			}
			stdlibValidateConfig(stdlibConfigPath(path, field.Key), field.Value, child, errorsOut)
		}
	}
}

func isStdlibInteger(number string) bool {
	if number == "" || number[0] == '+' || strings.ContainsAny(number, ".eE") {
		return false
	}
	if number[0] == '-' {
		number = number[1:]
	}
	if number == "" || (len(number) > 1 && number[0] == '0') {
		return false
	}
	for _, r := range number {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// StdlibValidateConfig validates an ordered value and returns every stable
// path-aware violation in source order.
func StdlibValidateConfig(value StdlibConfigValue, schema StdlibConfigSchema) error {
	var violations []StdlibConfigError
	stdlibValidateConfig("$", value, schema, &violations)
	if len(violations) == 0 {
		return nil
	}
	return &StdlibConfigValidationError{Errors: violations}
}

// StdlibSchemaPropertyNames returns schema names in lexical order, useful to
// callers that need deterministic documentation or diagnostics.
func StdlibSchemaPropertyNames(schema StdlibConfigSchema) []string {
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
