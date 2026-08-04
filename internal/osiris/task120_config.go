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

// Task120ConfigVersion identifies the ordered JSON/property representation.
const Task120ConfigVersion = "ordered-config-1"

// Task120ConfigLimits bounds parser work. Zero fields select 16 MiB, depth
// 64, 100,000 nodes, 1 MiB strings, and 10,000 fields per object.
type Task120ConfigLimits struct {
	MaxBytes        int64
	MaxDepth        int
	MaxNodes        int
	MaxStringBytes  int64
	MaxObjectFields int
}

func (l Task120ConfigLimits) normalized() Task120ConfigLimits {
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

// Task120ConfigKind is the ordered value kind.
type Task120ConfigKind string

const (
	Task120ConfigNull   Task120ConfigKind = "null"
	Task120ConfigBool   Task120ConfigKind = "boolean"
	Task120ConfigNumber Task120ConfigKind = "number"
	Task120ConfigString Task120ConfigKind = "string"
	Task120ConfigArray  Task120ConfigKind = "array"
	Task120ConfigObject Task120ConfigKind = "object"
)

// Task120ConfigField is one object member in source order.
type Task120ConfigField struct {
	Key   string
	Value Task120ConfigValue
}

// Task120ConfigValue preserves object order and JSON number spelling. Object
// keys are unique; parser functions reject duplicate keys rather than making
// a hidden last-write-wins choice.
type Task120ConfigValue struct {
	Kind   Task120ConfigKind
	Bool   bool
	Number string
	String string
	Array  []Task120ConfigValue
	Object []Task120ConfigField
}

// MarshalJSON emits a deterministic JSON representation in preserved object
// order and original number spelling.
func (v Task120ConfigValue) MarshalJSON() ([]byte, error) {
	switch v.Kind {
	case Task120ConfigNull:
		return []byte("null"), nil
	case Task120ConfigBool:
		if v.Bool {
			return []byte("true"), nil
		}
		return []byte("false"), nil
	case Task120ConfigNumber:
		if !task120JSONNumberValid(v.Number) {
			return nil, fmt.Errorf("%w: invalid config number", ErrTask120Invalid)
		}
		return []byte(v.Number), nil
	case Task120ConfigString:
		return json.Marshal(v.String)
	case Task120ConfigArray:
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
	case Task120ConfigObject:
		var b bytes.Buffer
		b.WriteByte('{')
		seen := make(map[string]struct{}, len(v.Object))
		for i, field := range v.Object {
			if _, ok := seen[field.Key]; ok {
				return nil, fmt.Errorf("%w: duplicate config key %q", ErrTask120Invalid, field.Key)
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
		return nil, fmt.Errorf("%w: unknown config kind %q", ErrTask120Invalid, v.Kind)
	}
}

// JSON returns the deterministic ordered JSON bytes.
func (v Task120ConfigValue) JSON() ([]byte, error) { return v.MarshalJSON() }

type task120JSONDecoder struct {
	limits Task120ConfigLimits
	nodes  int
}

func (d *task120JSONDecoder) node(depth int) error {
	if depth > d.limits.MaxDepth {
		return task120LimitError("config parse", "depth", int64(depth), int64(d.limits.MaxDepth))
	}
	d.nodes++
	if d.nodes > d.limits.MaxNodes {
		return task120LimitError("config parse", "nodes", int64(d.nodes), int64(d.limits.MaxNodes))
	}
	return nil
}

func (d *task120JSONDecoder) value(ctx context.Context, dec *json.Decoder, depth int) (Task120ConfigValue, error) {
	if err := task120CheckContext(ctx, "config parse"); err != nil {
		return Task120ConfigValue{}, err
	}
	if err := d.node(depth); err != nil {
		return Task120ConfigValue{}, err
	}
	token, err := dec.Token()
	if err != nil {
		return Task120ConfigValue{}, fmt.Errorf("config parse: %w: %v", ErrTask120Invalid, err)
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '[':
			out := Task120ConfigValue{Kind: Task120ConfigArray}
			for dec.More() {
				item, err := d.value(ctx, dec, depth+1)
				if err != nil {
					return Task120ConfigValue{}, err
				}
				out.Array = append(out.Array, item)
			}
			closeToken, err := dec.Token()
			if err != nil || closeToken != json.Delim(']') {
				return Task120ConfigValue{}, fmt.Errorf("config array close: %w", ErrTask120Invalid)
			}
			return out, nil
		case '{':
			out := Task120ConfigValue{Kind: Task120ConfigObject}
			seen := make(map[string]struct{})
			for dec.More() {
				keyToken, err := dec.Token()
				if err != nil {
					return Task120ConfigValue{}, fmt.Errorf("config object key: %w", err)
				}
				key, ok := keyToken.(string)
				if !ok || !utf8.ValidString(key) {
					return Task120ConfigValue{}, fmt.Errorf("%w: object key is not UTF-8 string", ErrTask120Invalid)
				}
				if _, duplicate := seen[key]; duplicate {
					return Task120ConfigValue{}, fmt.Errorf("%w: duplicate config key %q", ErrTask120Invalid, key)
				}
				seen[key] = struct{}{}
				if len(out.Object) >= d.limits.MaxObjectFields {
					return Task120ConfigValue{}, task120LimitError("config parse", "object fields", int64(len(out.Object)+1), int64(d.limits.MaxObjectFields))
				}
				child, err := d.value(ctx, dec, depth+1)
				if err != nil {
					return Task120ConfigValue{}, err
				}
				out.Object = append(out.Object, Task120ConfigField{Key: key, Value: child})
			}
			closeToken, err := dec.Token()
			if err != nil || closeToken != json.Delim('}') {
				return Task120ConfigValue{}, fmt.Errorf("config object close: %w", ErrTask120Invalid)
			}
			return out, nil
		default:
			return Task120ConfigValue{}, fmt.Errorf("%w: unexpected delimiter %q", ErrTask120Invalid, value)
		}
	case nil:
		return Task120ConfigValue{Kind: Task120ConfigNull}, nil
	case bool:
		return Task120ConfigValue{Kind: Task120ConfigBool, Bool: value}, nil
	case string:
		if int64(len(value)) > d.limits.MaxStringBytes {
			return Task120ConfigValue{}, task120LimitError("config parse", "string bytes", int64(len(value)), d.limits.MaxStringBytes)
		}
		return Task120ConfigValue{Kind: Task120ConfigString, String: value}, nil
	case json.Number:
		return Task120ConfigValue{Kind: Task120ConfigNumber, Number: string(value)}, nil
	default:
		return Task120ConfigValue{}, fmt.Errorf("%w: unsupported JSON token", ErrTask120Invalid)
	}
}

// Task120ParseJSONConfig parses JSON into an ordered, duplicate-rejecting
// representation without consulting host configuration or map iteration.
func Task120ParseJSONConfig(ctx context.Context, data []byte, limits Task120ConfigLimits) (Task120ConfigValue, error) {
	limits = limits.normalized()
	if int64(len(data)) > limits.MaxBytes {
		return Task120ConfigValue{}, task120LimitError("config parse", "input bytes", int64(len(data)), limits.MaxBytes)
	}
	if !utf8.Valid(data) {
		return Task120ConfigValue{}, fmt.Errorf("config parse: %w: input is not valid UTF-8", ErrTask120Invalid)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	d := &task120JSONDecoder{limits: limits}
	value, err := d.value(ctx, dec, 0)
	if err != nil {
		return Task120ConfigValue{}, err
	}
	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			return Task120ConfigValue{}, fmt.Errorf("%w: trailing JSON value", ErrTask120Invalid)
		}
		return Task120ConfigValue{}, fmt.Errorf("%w: trailing JSON: %v", ErrTask120Invalid, err)
	}
	return value, nil
}

// Task120ParseJSONConfigStream parses a bounded caller-owned stream and does
// not close it.
func Task120ParseJSONConfigStream(ctx context.Context, src io.Reader, limits Task120ConfigLimits) (Task120ConfigValue, error) {
	if src == nil {
		return Task120ConfigValue{}, fmt.Errorf("config parse: %w: nil source", ErrTask120Invalid)
	}
	limits = limits.normalized()
	readLimit := limits.MaxBytes
	if readLimit < int64(^uint64(0)>>1) {
		readLimit++
	}
	data, err := io.ReadAll(io.LimitReader(&task120ContextReader{ctx: ctx, r: src}, readLimit))
	if err != nil {
		return Task120ConfigValue{}, fmt.Errorf("config stream: %w", err)
	}
	return Task120ParseJSONConfig(ctx, data, limits)
}

// Task120ParsePropertiesConfig parses a small human-authored key=value
// format. Blank lines and # comments are ignored; values are JSON scalars,
// arrays, or objects, and source order is retained.
func Task120ParsePropertiesConfig(ctx context.Context, data []byte, limits Task120ConfigLimits) (Task120ConfigValue, error) {
	limits = limits.normalized()
	if int64(len(data)) > limits.MaxBytes {
		return Task120ConfigValue{}, task120LimitError("properties parse", "input bytes", int64(len(data)), limits.MaxBytes)
	}
	if !utf8.Valid(data) {
		return Task120ConfigValue{}, fmt.Errorf("properties parse: %w: input is not valid UTF-8", ErrTask120Invalid)
	}
	root := Task120ConfigValue{Kind: Task120ConfigObject}
	seen := map[string]struct{}{}
	for lineNumber, line := range strings.Split(string(data), "\n") {
		if err := task120CheckContext(ctx, "properties parse"); err != nil {
			return Task120ConfigValue{}, err
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		separator := strings.IndexByte(line, '=')
		if separator <= 0 {
			return Task120ConfigValue{}, fmt.Errorf("%w: properties line %d needs key = JSON-value", ErrTask120Invalid, lineNumber+1)
		}
		key := strings.TrimSpace(line[:separator])
		raw := strings.TrimSpace(line[separator+1:])
		if key == "" || !utf8.ValidString(key) {
			return Task120ConfigValue{}, fmt.Errorf("%w: properties line %d has invalid key", ErrTask120Invalid, lineNumber+1)
		}
		if _, ok := seen[key]; ok {
			return Task120ConfigValue{}, fmt.Errorf("%w: duplicate property %q", ErrTask120Invalid, key)
		}
		seen[key] = struct{}{}
		value, err := Task120ParseJSONConfig(ctx, []byte(raw), limits)
		if err != nil {
			return Task120ConfigValue{}, fmt.Errorf("properties line %d: %w", lineNumber+1, err)
		}
		root.Object = append(root.Object, Task120ConfigField{Key: key, Value: value})
		if len(root.Object) > limits.MaxObjectFields {
			return Task120ConfigValue{}, task120LimitError("properties parse", "object fields", int64(len(root.Object)), int64(limits.MaxObjectFields))
		}
	}
	return root, nil
}

// Task120ConfigSchemaType identifies one schema node type.
type Task120ConfigSchemaType string

const (
	Task120SchemaAny     Task120ConfigSchemaType = "any"
	Task120SchemaNull    Task120ConfigSchemaType = "null"
	Task120SchemaBool    Task120ConfigSchemaType = "boolean"
	Task120SchemaNumber  Task120ConfigSchemaType = "number"
	Task120SchemaInteger Task120ConfigSchemaType = "integer"
	Task120SchemaString  Task120ConfigSchemaType = "string"
	Task120SchemaArray   Task120ConfigSchemaType = "array"
	Task120SchemaObject  Task120ConfigSchemaType = "object"
)

// Task120ConfigSchema is a deliberately small deterministic schema language.
// Properties are selected by name, required order is preserved for reporting,
// and additional keys are rejected unless explicitly allowed.
type Task120ConfigSchema struct {
	Type            Task120ConfigSchemaType
	Required        []string
	Properties      map[string]Task120ConfigSchema
	Items           *Task120ConfigSchema
	AllowAdditional bool
	MinItems        int
	MaxItems        int
	MinProperties   int
	MaxProperties   int
	MinLength       int
	MaxLength       int
}

// Task120ConfigError is one path-aware schema violation.
type Task120ConfigError struct {
	Path    string
	Code    string
	Message string
}

func (e Task120ConfigError) Error() string { return e.Path + ": " + e.Message }

// Task120ConfigValidationError groups stable, source-order/schema-order
// validation errors.
type Task120ConfigValidationError struct{ Errors []Task120ConfigError }

func (e *Task120ConfigValidationError) Error() string {
	if e == nil || len(e.Errors) == 0 {
		return "config validation failed"
	}
	return fmt.Sprintf("config validation failed: %s", e.Errors[0].Error())
}

func task120ConfigPath(parent, key string) string {
	if parent == "$" {
		return "$[" + strconv.Quote(key) + "]"
	}
	return parent + "[" + strconv.Quote(key) + "]"
}

func task120ValidateConfig(path string, value Task120ConfigValue, schema Task120ConfigSchema, errorsOut *[]Task120ConfigError) {
	if schema.Type == "" {
		schema.Type = Task120SchemaAny
	}
	valid := schema.Type == Task120SchemaAny || (schema.Type == Task120SchemaNull && value.Kind == Task120ConfigNull) || (schema.Type == Task120SchemaBool && value.Kind == Task120ConfigBool) || (schema.Type == Task120SchemaNumber && value.Kind == Task120ConfigNumber) || (schema.Type == Task120SchemaInteger && value.Kind == Task120ConfigNumber && isTask120Integer(value.Number)) || (schema.Type == Task120SchemaString && value.Kind == Task120ConfigString) || (schema.Type == Task120SchemaArray && value.Kind == Task120ConfigArray) || (schema.Type == Task120SchemaObject && value.Kind == Task120ConfigObject)
	if !valid {
		*errorsOut = append(*errorsOut, Task120ConfigError{Path: path, Code: "type", Message: "expected " + string(schema.Type) + ", got " + string(value.Kind)})
		return
	}
	switch value.Kind {
	case Task120ConfigString:
		length := len([]rune(value.String))
		if schema.MinLength > 0 && length < schema.MinLength {
			*errorsOut = append(*errorsOut, Task120ConfigError{Path: path, Code: "min_length", Message: "string is shorter than minimum"})
		}
		if schema.MaxLength > 0 && length > schema.MaxLength {
			*errorsOut = append(*errorsOut, Task120ConfigError{Path: path, Code: "max_length", Message: "string is longer than maximum"})
		}
	case Task120ConfigArray:
		if schema.MinItems > 0 && len(value.Array) < schema.MinItems {
			*errorsOut = append(*errorsOut, Task120ConfigError{Path: path, Code: "min_items", Message: "array has too few items"})
		}
		if schema.MaxItems > 0 && len(value.Array) > schema.MaxItems {
			*errorsOut = append(*errorsOut, Task120ConfigError{Path: path, Code: "max_items", Message: "array has too many items"})
		}
		if schema.Items != nil {
			for i, item := range value.Array {
				task120ValidateConfig(path+"["+strconv.Itoa(i)+"]", item, *schema.Items, errorsOut)
			}
		}
	case Task120ConfigObject:
		if schema.MinProperties > 0 && len(value.Object) < schema.MinProperties {
			*errorsOut = append(*errorsOut, Task120ConfigError{Path: path, Code: "min_properties", Message: "object has too few properties"})
		}
		if schema.MaxProperties > 0 && len(value.Object) > schema.MaxProperties {
			*errorsOut = append(*errorsOut, Task120ConfigError{Path: path, Code: "max_properties", Message: "object has too many properties"})
		}
		present := make(map[string]Task120ConfigValue, len(value.Object))
		for _, field := range value.Object {
			present[field.Key] = field.Value
		}
		for _, required := range schema.Required {
			if _, ok := present[required]; !ok {
				*errorsOut = append(*errorsOut, Task120ConfigError{Path: task120ConfigPath(path, required), Code: "required", Message: "required property is missing"})
			}
		}
		for _, field := range value.Object {
			child, ok := schema.Properties[field.Key]
			if !ok {
				if !schema.AllowAdditional {
					*errorsOut = append(*errorsOut, Task120ConfigError{Path: task120ConfigPath(path, field.Key), Code: "additional_property", Message: "additional property is not allowed"})
				}
				continue
			}
			task120ValidateConfig(task120ConfigPath(path, field.Key), field.Value, child, errorsOut)
		}
	}
}

func isTask120Integer(number string) bool {
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

// Task120ValidateConfig validates an ordered value and returns every stable
// path-aware violation in source order.
func Task120ValidateConfig(value Task120ConfigValue, schema Task120ConfigSchema) error {
	var violations []Task120ConfigError
	task120ValidateConfig("$", value, schema, &violations)
	if len(violations) == 0 {
		return nil
	}
	return &Task120ConfigValidationError{Errors: violations}
}

// Task120SchemaPropertyNames returns schema names in lexical order, useful to
// callers that need deterministic documentation or diagnostics.
func Task120SchemaPropertyNames(schema Task120ConfigSchema) []string {
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
