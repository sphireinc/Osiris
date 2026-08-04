package osiris

import (
	"context"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Task137JSONVersion identifies the additive JSON query, transform, JSON Lines,
// and extended-schema contract. The task-120 APIs remain unchanged.
const Task137JSONVersion = "json-query-jsonl-schema-1"

const (
	Task137JSONDefaultMaxQueryResults = 1024
	Task137JSONDefaultMaxQuerySteps   = 10000
	Task137JSONDefaultMaxLines        = 1024
	Task137JSONDefaultMaxErrors       = 256
)

// Task137JSONQueryLimits bounds path parsing and traversal. Zero values select
// the documented defaults. MaxPathBytes is deliberately small because paths
// are control data, not a transport for document content.
type Task137JSONQueryLimits struct {
	MaxPathBytes int
	MaxResults   int
	MaxSteps     int
}

func (l Task137JSONQueryLimits) normalized() Task137JSONQueryLimits {
	if l.MaxPathBytes <= 0 {
		l.MaxPathBytes = 4096
	}
	if l.MaxResults <= 0 {
		l.MaxResults = Task137JSONDefaultMaxQueryResults
	}
	if l.MaxSteps <= 0 {
		l.MaxSteps = Task137JSONDefaultMaxQuerySteps
	}
	return l
}

// Task137JSONQuery evaluates a deliberately bounded JSONPath subset. Supported
// paths are $, .member, ["member"], ['member'], [integer], and [*]. Results
// preserve object insertion order and array order. Every result is detached.
func Task137JSONQuery(ctx context.Context, value Task120ConfigValue, path string, limits ...Task137JSONQueryLimits) ([]Task120ConfigValue, error) {
	lim := task137JSONQueryLimits(limits...)
	if len(path) > lim.MaxPathBytes {
		return nil, task120LimitError("JSON query", "path bytes", int64(len(path)), int64(lim.MaxPathBytes))
	}
	if err := task120JSONCheckValue(ctx, value, task120JSONOptionalLimits(), 0, new(int)); err != nil {
		return nil, err
	}
	tokens, err := task137JSONPath(path)
	if err != nil {
		return nil, err
	}
	current := []Task120ConfigValue{value}
	steps := 0
	for _, token := range tokens {
		if err := task120CheckContext(ctx, "JSON query"); err != nil {
			return nil, err
		}
		next := make([]Task120ConfigValue, 0, len(current))
		for _, item := range current {
			steps++
			if steps > lim.MaxSteps {
				return nil, task120LimitError("JSON query", "steps", int64(steps), int64(lim.MaxSteps))
			}
			children, err := task137JSONSelect(item, token)
			if err != nil {
				return nil, err
			}
			if len(next)+len(children) > lim.MaxResults {
				return nil, task120LimitError("JSON query", "results", int64(len(next)+len(children)), int64(lim.MaxResults))
			}
			next = append(next, children...)
		}
		current = next
	}
	for i := range current {
		current[i] = task120JSONClone(current[i])
	}
	return current, nil
}

// Task137JSONQueryOne returns exactly one JSONPath result.
func Task137JSONQueryOne(ctx context.Context, value Task120ConfigValue, path string, limits ...Task137JSONQueryLimits) (Task120ConfigValue, error) {
	results, err := Task137JSONQuery(ctx, value, path, limits...)
	if err != nil {
		return Task120ConfigValue{}, err
	}
	if len(results) != 1 {
		return Task120ConfigValue{}, fmt.Errorf("JSON query: %w: expected one result, got %d", ErrTask120Invalid, len(results))
	}
	return results[0], nil
}

// Task137JSONPointerExists checks an RFC 6901 pointer without treating a
// missing member as a failed operation. Malformed pointers and limit errors
// remain errors.
func Task137JSONPointerExists(value Task120ConfigValue, pointer string, limits ...Task120JSONLimits) (bool, error) {
	_, err := Task120JSONPointerGet(value, pointer, limits...)
	if err == nil {
		return true, nil
	}
	if strings.Contains(err.Error(), "not found") {
		return false, nil
	}
	return false, err
}

// Task137JSONType returns the stable Task120ConfigKind spelling of a value.
func Task137JSONType(value Task120ConfigValue) (Task120ConfigKind, error) {
	switch value.Kind {
	case Task120ConfigNull, Task120ConfigBool, Task120ConfigNumber, Task120ConfigString, Task120ConfigArray, Task120ConfigObject:
		return value.Kind, nil
	default:
		return "", fmt.Errorf("JSON type: %w: unknown value kind %q", ErrTask120Invalid, value.Kind)
	}
}

// Task137JSONObjectKeys returns object keys in source order. It rejects
// non-objects rather than silently returning an empty list.
func Task137JSONObjectKeys(value Task120ConfigValue) ([]string, error) {
	if value.Kind != Task120ConfigObject {
		return nil, fmt.Errorf("JSON keys: %w: expected object", ErrTask120Invalid)
	}
	keys := make([]string, len(value.Object))
	for i, field := range value.Object {
		keys[i] = field.Key
	}
	return keys, nil
}

// Task137JSONLength returns the number of members or elements. Scalars are
// rejected to avoid an ambiguous string-byte versus rune-count convention.
func Task137JSONLength(value Task120ConfigValue) (int, error) {
	switch value.Kind {
	case Task120ConfigArray:
		return len(value.Array), nil
	case Task120ConfigObject:
		return len(value.Object), nil
	default:
		return 0, fmt.Errorf("JSON length: %w: expected array or object", ErrTask120Invalid)
	}
}

// Task137JSONEqual compares JSON values structurally. Numbers compare by
// exact rational value, so 1, 1.0, and 1e0 are equal.
func Task137JSONEqual(left, right Task120ConfigValue) (bool, error) {
	return task137JSONEqual(left, right)
}

func task137JSONEqual(left, right Task120ConfigValue) (bool, error) {
	if left.Kind != right.Kind {
		if left.Kind == Task120ConfigNumber && right.Kind == Task120ConfigNumber {
			// unreachable because kinds are equal by representation, retained for clarity.
		}
		return false, nil
	}
	switch left.Kind {
	case Task120ConfigNull:
		return true, nil
	case Task120ConfigBool:
		return left.Bool == right.Bool, nil
	case Task120ConfigNumber:
		l, err := task137JSONRat(left.Number)
		if err != nil {
			return false, err
		}
		r, err := task137JSONRat(right.Number)
		if err != nil {
			return false, err
		}
		return l.Cmp(r) == 0, nil
	case Task120ConfigString:
		return left.String == right.String, nil
	case Task120ConfigArray:
		if len(left.Array) != len(right.Array) {
			return false, nil
		}
		for i := range left.Array {
			equal, err := task137JSONEqual(left.Array[i], right.Array[i])
			if err != nil || !equal {
				return equal, err
			}
		}
		return true, nil
	case Task120ConfigObject:
		if len(left.Object) != len(right.Object) {
			return false, nil
		}
		for i := range left.Object {
			if left.Object[i].Key != right.Object[i].Key {
				return false, nil
			}
			equal, err := task137JSONEqual(left.Object[i].Value, right.Object[i].Value)
			if err != nil || !equal {
				return equal, err
			}
		}
		return true, nil
	default:
		return false, fmt.Errorf("JSON equal: %w: unknown value kind %q", ErrTask120Invalid, left.Kind)
	}
}

// Task137JSONMerge recursively merges objects. Object members from overlay
// replace base members; object/object pairs merge recursively; arrays and
// scalars replace. The result is detached and bounded by limits.
func Task137JSONMerge(base, overlay Task120ConfigValue, limits ...Task120JSONLimits) (Task120ConfigValue, error) {
	lim := task120JSONOptionalLimits(limits...)
	if err := task120JSONCheckValue(context.Background(), base, lim, 0, new(int)); err != nil {
		return Task120ConfigValue{}, err
	}
	if err := task120JSONCheckValue(context.Background(), overlay, lim, 0, new(int)); err != nil {
		return Task120ConfigValue{}, err
	}
	result, err := task137JSONMergeValues(base, overlay)
	if err != nil {
		return Task120ConfigValue{}, err
	}
	return result, task120JSONCheckValue(context.Background(), result, lim, 0, new(int))
}

func task137JSONMergeValues(base, overlay Task120ConfigValue) (Task120ConfigValue, error) {
	if base.Kind != Task120ConfigObject || overlay.Kind != Task120ConfigObject {
		return task120JSONClone(overlay), nil
	}
	result := task120JSONClone(base)
	for _, field := range overlay.Object {
		index, err := task120JSONObjectIndex(result.Object, field.Key)
		if err != nil {
			return Task120ConfigValue{}, err
		}
		if index < 0 {
			result.Object = append(result.Object, Task120ConfigField{Key: field.Key, Value: task120JSONClone(field.Value)})
			continue
		}
		merged, err := task137JSONMergeValues(result.Object[index].Value, field.Value)
		if err != nil {
			return Task120ConfigValue{}, err
		}
		result.Object[index].Value = merged
	}
	return result, nil
}

// Task137JSONParseLines parses newline-delimited JSON. Empty lines are ignored
// and each non-empty line is independently bounded and must contain one value.
func Task137JSONParseLines(ctx context.Context, data []byte, limits ...Task120JSONLimits) ([]Task120ConfigValue, error) {
	lim := task120JSONOptionalLimits(limits...)
	if int64(len(data)) > lim.MaxBytes {
		return nil, task120LimitError("JSON Lines parse", "input bytes", int64(len(data)), lim.MaxBytes)
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > Task137JSONDefaultMaxLines+1 {
		return nil, task120LimitError("JSON Lines parse", "lines", int64(len(lines)), Task137JSONDefaultMaxLines)
	}
	out := make([]Task120ConfigValue, 0, len(lines))
	for number, line := range lines {
		if err := task120CheckContext(ctx, "JSON Lines parse"); err != nil {
			return nil, err
		}
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		value, err := Task120JSONParse(ctx, []byte(line), lim)
		if err != nil {
			return nil, fmt.Errorf("JSON Lines parse line %d: %w", number+1, err)
		}
		out = append(out, value)
	}
	return out, nil
}

// Task137JSONStringifyLines emits one compact JSON value per line.
func Task137JSONStringifyLines(values []Task120ConfigValue, limits ...Task120JSONLimits) ([]byte, error) {
	if len(values) > Task137JSONDefaultMaxLines {
		return nil, task120LimitError("JSON Lines stringify", "lines", int64(len(values)), Task137JSONDefaultMaxLines)
	}
	lim := task120JSONOptionalLimits(limits...)
	var b strings.Builder
	for i, value := range values {
		encoded, err := Task120JSONStringify(value, lim)
		if err != nil {
			return nil, fmt.Errorf("JSON Lines stringify line %d: %w", i+1, err)
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		b.Write(encoded)
	}
	b.WriteByte('\n')
	if int64(b.Len()) > lim.MaxOutputBytes {
		return nil, task120LimitError("JSON Lines stringify", "output bytes", int64(b.Len()), lim.MaxOutputBytes)
	}
	return []byte(b.String()), nil
}

// Task137JSONSchema is an additive schema vocabulary. Base type, object, and
// array constraints mirror task 120; enum, const, pattern, numeric bounds,
// uniqueness, additionalProperties, and boolean combinators are new.
type Task137JSONSchema struct {
	Type                 Task120ConfigSchemaType
	Required             []string
	Properties           map[string]Task137JSONSchema
	Items                *Task137JSONSchema
	MinItems             int
	MaxItems             int
	MinProperties        int
	MaxProperties        int
	MinLength            int
	MaxLength            int
	Pattern              string
	Minimum              string
	Maximum              string
	ExclusiveMinimum     bool
	ExclusiveMaximum     bool
	UniqueItems          bool
	AdditionalProperties *bool
	Enum                 []Task120ConfigValue
	Const                *Task120ConfigValue
	AnyOf                []Task137JSONSchema
	AllOf                []Task137JSONSchema
	OneOf                []Task137JSONSchema
	Not                  *Task137JSONSchema
}

// Task137JSONValidationLimits bounds diagnostics and schema recursion.
type Task137JSONValidationLimits struct {
	MaxErrors int
	MaxDepth  int
}

func (l Task137JSONValidationLimits) normalized() Task137JSONValidationLimits {
	if l.MaxErrors <= 0 {
		l.MaxErrors = Task137JSONDefaultMaxErrors
	}
	if l.MaxDepth <= 0 {
		l.MaxDepth = 64
	}
	return l
}

// Task137JSONValidationError identifies one deterministic schema violation.
type Task137JSONValidationError struct {
	Path    string
	Keyword string
	Message string
}

// Task137JSONValidateSchema returns all bounded validation issues in document
// order. A nil slice means valid; malformed schema definitions return error.
func Task137JSONValidateSchema(ctx context.Context, value Task120ConfigValue, schema Task137JSONSchema, limits ...Task137JSONValidationLimits) ([]Task137JSONValidationError, error) {
	lim := task137JSONValidationLimits(limits...)
	if err := task120JSONCheckValue(ctx, value, task120JSONOptionalLimits(), 0, new(int)); err != nil {
		return nil, err
	}
	issues := make([]Task137JSONValidationError, 0)
	if err := task137JSONValidateNode(ctx, value, schema, "", 0, lim, &issues); err != nil {
		return nil, err
	}
	return issues, nil
}

// Task137JSONSchemaValid is the boolean convenience form of
// Task137JSONValidateSchema.
func Task137JSONSchemaValid(ctx context.Context, value Task120ConfigValue, schema Task137JSONSchema, limits ...Task137JSONValidationLimits) (bool, error) {
	issues, err := Task137JSONValidateSchema(ctx, value, schema, limits...)
	return len(issues) == 0, err
}

func task137JSONValidateNode(ctx context.Context, value Task120ConfigValue, schema Task137JSONSchema, path string, depth int, lim Task137JSONValidationLimits, issues *[]Task137JSONValidationError) error {
	if err := task120CheckContext(ctx, "JSON schema validation"); err != nil {
		return err
	}
	if depth > lim.MaxDepth {
		return task120LimitError("JSON schema validation", "depth", int64(depth), int64(lim.MaxDepth))
	}
	add := func(keyword, message string) {
		if len(*issues) < lim.MaxErrors {
			*issues = append(*issues, Task137JSONValidationError{Path: path, Keyword: keyword, Message: message})
		}
	}
	if schema.Type != "" && schema.Type != Task120SchemaAny && !task137JSONTypeMatches(value, schema.Type) {
		add("type", fmt.Sprintf("expected %s, got %s", schema.Type, value.Kind))
		return nil
	}
	if schema.Const != nil {
		equal, err := task137JSONEqual(value, *schema.Const)
		if err != nil {
			return err
		}
		if !equal {
			add("const", "value does not equal const")
		}
	}
	if len(schema.Enum) > 0 {
		matched := false
		for _, candidate := range schema.Enum {
			equal, err := task137JSONEqual(value, candidate)
			if err != nil {
				return err
			}
			if equal {
				matched = true
				break
			}
		}
		if !matched {
			add("enum", "value is not one of the permitted values")
		}
	}
	if value.Kind == Task120ConfigNumber {
		if err := task137JSONNumberBounds(value.Number, schema, add); err != nil {
			return err
		}
	}
	if value.Kind == Task120ConfigString {
		length := len([]rune(value.String))
		if schema.MinLength > 0 && length < schema.MinLength {
			add("minLength", fmt.Sprintf("length %d is below %d", length, schema.MinLength))
		}
		if schema.MaxLength > 0 && length > schema.MaxLength {
			add("maxLength", fmt.Sprintf("length %d exceeds %d", length, schema.MaxLength))
		}
		if schema.Pattern != "" {
			pattern, err := regexp.Compile(schema.Pattern)
			if err != nil {
				return fmt.Errorf("JSON schema pattern: %w: %v", ErrTask120Invalid, err)
			}
			if !pattern.MatchString(value.String) {
				add("pattern", "string does not match pattern")
			}
		}
	}
	if value.Kind == Task120ConfigArray {
		if schema.MinItems > 0 && len(value.Array) < schema.MinItems {
			add("minItems", fmt.Sprintf("length %d is below %d", len(value.Array), schema.MinItems))
		}
		if schema.MaxItems > 0 && len(value.Array) > schema.MaxItems {
			add("maxItems", fmt.Sprintf("length %d exceeds %d", len(value.Array), schema.MaxItems))
		}
		if schema.UniqueItems {
			for i := range value.Array {
				for j := 0; j < i; j++ {
					equal, err := task137JSONEqual(value.Array[i], value.Array[j])
					if err != nil {
						return err
					}
					if equal {
						add("uniqueItems", fmt.Sprintf("item %d duplicates item %d", i, j))
						break
					}
				}
			}
		}
		if schema.Items != nil {
			for i, item := range value.Array {
				if err := task137JSONValidateNode(ctx, item, *schema.Items, fmt.Sprintf("%s/%d", path, i), depth+1, lim, issues); err != nil {
					return err
				}
			}
		}
	}
	if value.Kind == Task120ConfigObject {
		if schema.MinProperties > 0 && len(value.Object) < schema.MinProperties {
			add("minProperties", fmt.Sprintf("count %d is below %d", len(value.Object), schema.MinProperties))
		}
		if schema.MaxProperties > 0 && len(value.Object) > schema.MaxProperties {
			add("maxProperties", fmt.Sprintf("count %d exceeds %d", len(value.Object), schema.MaxProperties))
		}
		seen := make(map[string]struct{}, len(value.Object))
		for _, field := range value.Object {
			seen[field.Key] = struct{}{}
			child, ok := schema.Properties[field.Key]
			if ok {
				if err := task137JSONValidateNode(ctx, field.Value, child, path+"/"+task137JSONPointerEscape(field.Key), depth+1, lim, issues); err != nil {
					return err
				}
			} else if schema.AdditionalProperties != nil && !*schema.AdditionalProperties {
				add("additionalProperties", fmt.Sprintf("property %q is not allowed", field.Key))
			}
		}
		for _, required := range schema.Required {
			if _, ok := seen[required]; !ok {
				add("required", fmt.Sprintf("property %q is required", required))
			}
		}
	}
	for _, child := range schema.AllOf {
		if err := task137JSONValidateNode(ctx, value, child, path, depth+1, lim, issues); err != nil {
			return err
		}
	}
	if len(schema.AnyOf) > 0 || len(schema.OneOf) > 0 {
		branches := schema.AnyOf
		keyword := "anyOf"
		if len(branches) == 0 {
			branches = schema.OneOf
			keyword = "oneOf"
		}
		matches := 0
		for _, branch := range branches {
			branchIssues, err := Task137JSONValidateSchema(ctx, value, branch, lim)
			if err != nil {
				return err
			}
			if len(branchIssues) == 0 {
				matches++
			}
		}
		valid := matches > 0
		if keyword == "oneOf" {
			valid = matches == 1
		}
		if !valid {
			add(keyword, fmt.Sprintf("matched %d of %d branches", matches, len(branches)))
		}
	}
	if schema.Not != nil {
		branchIssues, err := Task137JSONValidateSchema(ctx, value, *schema.Not, lim)
		if err != nil {
			return err
		}
		if len(branchIssues) == 0 {
			add("not", "value matches forbidden schema")
		}
	}
	return nil
}

func task137JSONNumberBounds(number string, schema Task137JSONSchema, add func(string, string)) error {
	if schema.Minimum == "" && schema.Maximum == "" {
		return nil
	}
	value, err := task137JSONRat(number)
	if err != nil {
		return err
	}
	if schema.Minimum != "" {
		minimum, err := task137JSONRat(schema.Minimum)
		if err != nil {
			return fmt.Errorf("JSON schema minimum: %w", err)
		}
		cmp := value.Cmp(minimum)
		if (schema.ExclusiveMinimum && cmp <= 0) || (!schema.ExclusiveMinimum && cmp < 0) {
			add("minimum", "number is below minimum")
		}
	}
	if schema.Maximum != "" {
		maximum, err := task137JSONRat(schema.Maximum)
		if err != nil {
			return fmt.Errorf("JSON schema maximum: %w", err)
		}
		cmp := value.Cmp(maximum)
		if (schema.ExclusiveMaximum && cmp >= 0) || (!schema.ExclusiveMaximum && cmp > 0) {
			add("maximum", "number is above maximum")
		}
	}
	return nil
}

func task137JSONTypeMatches(value Task120ConfigValue, typ Task120ConfigSchemaType) bool {
	if typ == Task120SchemaAny {
		return true
	}
	if typ == Task120SchemaInteger && value.Kind == Task120ConfigNumber {
		rat, err := task137JSONRat(value.Number)
		return err == nil && rat.IsInt()
	}
	return value.Kind == Task120ConfigKind(typ)
}

// task137JSONRat converts the complete JSON number grammar, including an
// exponent, to an exact rational without passing through float64.
func task137JSONRat(number string) (*big.Rat, error) {
	if !task120JSONNumberValid(number) {
		return nil, fmt.Errorf("JSON number: %w: invalid number %q", ErrTask120Invalid, number)
	}
	if len(number) > 1<<20 {
		return nil, task120LimitError("JSON number", "bytes", int64(len(number)), 1<<20)
	}
	text := number
	negative := false
	if text[0] == '-' {
		negative = true
		text = text[1:]
	}
	exponent := 0
	if index := strings.IndexAny(text, "eE"); index >= 0 {
		parsed, err := strconv.Atoi(text[index+1:])
		if err != nil {
			return nil, fmt.Errorf("JSON number: %w: invalid exponent", ErrTask120Invalid)
		}
		exponent = parsed
		if exponent > 4096 || exponent < -4096 {
			return nil, task120LimitError("JSON number", "exponent", int64(exponent), 4096)
		}
		text = text[:index]
	}
	fractionDigits := 0
	if index := strings.IndexByte(text, '.'); index >= 0 {
		fractionDigits = len(text) - index - 1
		text = text[:index] + text[index+1:]
	}
	numerator := new(big.Int)
	if _, ok := numerator.SetString(text, 10); !ok {
		return nil, fmt.Errorf("JSON number: %w: invalid mantissa", ErrTask120Invalid)
	}
	if negative {
		numerator.Neg(numerator)
	}
	scale := fractionDigits - exponent
	if scale <= 0 {
		numerator.Mul(numerator, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-scale)), nil))
		return new(big.Rat).SetInt(numerator), nil
	}
	denominator := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	return new(big.Rat).SetFrac(numerator, denominator), nil
}

// Task137JSONSchemaFromValue decodes the extended schema vocabulary from the
// same ordered map representation accepted by std/json.
func Task137JSONSchemaFromValue(value Task120ConfigValue) (Task137JSONSchema, error) {
	if value.Kind != Task120ConfigObject {
		return Task137JSONSchema{}, fmt.Errorf("JSON schema: %w: expected object", ErrTask120Invalid)
	}
	schema := Task137JSONSchema{Type: Task120SchemaAny}
	field := func(name string) (Task120ConfigValue, bool) {
		for _, item := range value.Object {
			if item.Key == name {
				return item.Value, true
			}
		}
		return Task120ConfigValue{}, false
	}
	if raw, ok := field("type"); ok {
		if raw.Kind != Task120ConfigString {
			return schema, fmt.Errorf("JSON schema.type: %w: expected string", ErrTask120Invalid)
		}
		schema.Type = Task120ConfigSchemaType(raw.String)
	}
	if raw, ok := field("required"); ok {
		if raw.Kind != Task120ConfigArray {
			return schema, fmt.Errorf("JSON schema.required: %w: expected array", ErrTask120Invalid)
		}
		for _, item := range raw.Array {
			if item.Kind != Task120ConfigString {
				return schema, fmt.Errorf("JSON schema.required: %w: expected strings", ErrTask120Invalid)
			}
			schema.Required = append(schema.Required, item.String)
		}
	}
	stringFields := map[string]*string{"pattern": &schema.Pattern, "minimum": &schema.Minimum, "maximum": &schema.Maximum}
	for name, target := range stringFields {
		if raw, ok := field(name); ok {
			if raw.Kind != Task120ConfigString && raw.Kind != Task120ConfigNumber {
				return schema, fmt.Errorf("JSON schema.%s: %w: expected string or number", name, ErrTask120Invalid)
			}
			if raw.Kind == Task120ConfigString {
				*target = raw.String
			} else {
				*target = raw.Number
			}
		}
	}
	boolFields := map[string]*bool{"exclusive_minimum": &schema.ExclusiveMinimum, "exclusive_maximum": &schema.ExclusiveMaximum, "unique_items": &schema.UniqueItems}
	for name, target := range boolFields {
		if raw, ok := field(name); ok {
			if raw.Kind != Task120ConfigBool {
				return schema, fmt.Errorf("JSON schema.%s: %w: expected boolean", name, ErrTask120Invalid)
			}
			*target = raw.Bool
		}
	}
	if raw, ok := field("additional_properties"); ok {
		if raw.Kind != Task120ConfigBool {
			return schema, fmt.Errorf("JSON schema.additional_properties: %w: expected boolean", ErrTask120Invalid)
		}
		schema.AdditionalProperties = new(bool)
		*schema.AdditionalProperties = raw.Bool
	}
	for name, target := range map[string]*int{"min_items": &schema.MinItems, "max_items": &schema.MaxItems, "min_properties": &schema.MinProperties, "max_properties": &schema.MaxProperties, "min_length": &schema.MinLength, "max_length": &schema.MaxLength} {
		if raw, ok := field(name); ok {
			if raw.Kind != Task120ConfigNumber {
				return schema, fmt.Errorf("JSON schema.%s: %w: expected integer", name, ErrTask120Invalid)
			}
			n, err := strconv.Atoi(raw.Number)
			if err != nil || n < 0 {
				return schema, fmt.Errorf("JSON schema.%s: %w: invalid integer", name, ErrTask120Invalid)
			}
			*target = n
		}
	}
	if raw, ok := field("enum"); ok {
		if raw.Kind != Task120ConfigArray {
			return schema, fmt.Errorf("JSON schema.enum: %w: expected array", ErrTask120Invalid)
		}
		schema.Enum = make([]Task120ConfigValue, len(raw.Array))
		for i := range raw.Array {
			schema.Enum[i] = task120JSONClone(raw.Array[i])
		}
	}
	if raw, ok := field("const"); ok {
		clone := task120JSONClone(raw)
		schema.Const = &clone
	}
	if raw, ok := field("items"); ok {
		child, err := Task137JSONSchemaFromValue(raw)
		if err != nil {
			return schema, err
		}
		schema.Items = &child
	}
	if raw, ok := field("properties"); ok {
		if raw.Kind != Task120ConfigObject {
			return schema, fmt.Errorf("JSON schema.properties: %w: expected object", ErrTask120Invalid)
		}
		schema.Properties = make(map[string]Task137JSONSchema, len(raw.Object))
		for _, item := range raw.Object {
			child, err := Task137JSONSchemaFromValue(item.Value)
			if err != nil {
				return schema, err
			}
			schema.Properties[item.Key] = child
		}
	}
	return schema, nil
}

func task137JSONPath(path string) ([]string, error) {
	if path == "$" || path == "" {
		return nil, nil
	}
	if !strings.HasPrefix(path, "$") {
		return nil, fmt.Errorf("JSON query: %w: path must start with $", ErrTask120Invalid)
	}
	var tokens []string
	for i := 1; i < len(path); {
		switch path[i] {
		case '.':
			i++
			if i < len(path) && path[i] == '*' {
				tokens = append(tokens, "*")
				i++
				continue
			}
			start := i
			for i < len(path) && (path[i] == '_' || path[i] == '-' || path[i] >= 'a' && path[i] <= 'z' || path[i] >= 'A' && path[i] <= 'Z' || path[i] >= '0' && path[i] <= '9') {
				i++
			}
			if start == i {
				return nil, fmt.Errorf("JSON query: %w: empty member", ErrTask120Invalid)
			}
			tokens = append(tokens, "k:"+path[start:i])
		case '[':
			i++
			if i >= len(path) {
				return nil, fmt.Errorf("JSON query: %w: unterminated selector", ErrTask120Invalid)
			}
			if path[i] == '*' {
				i++
				if i >= len(path) || path[i] != ']' {
					return nil, fmt.Errorf("JSON query: %w: expected ]", ErrTask120Invalid)
				}
				i++
				tokens = append(tokens, "*")
				continue
			}
			if path[i] == '\'' || path[i] == '"' {
				quote := path[i]
				i++
				var b strings.Builder
				for i < len(path) && path[i] != quote {
					if path[i] == '\\' {
						i++
						if i >= len(path) {
							return nil, fmt.Errorf("JSON query: %w: incomplete escape", ErrTask120Invalid)
						}
					}
					b.WriteByte(path[i])
					i++
				}
				if i >= len(path) || path[i] != quote {
					return nil, fmt.Errorf("JSON query: %w: unterminated member", ErrTask120Invalid)
				}
				i++
				if i >= len(path) || path[i] != ']' {
					return nil, fmt.Errorf("JSON query: %w: expected ]", ErrTask120Invalid)
				}
				i++
				tokens = append(tokens, "k:"+b.String())
				continue
			}
			start := i
			for i < len(path) && path[i] >= '0' && path[i] <= '9' {
				i++
			}
			if start == i || i >= len(path) || path[i] != ']' {
				return nil, fmt.Errorf("JSON query: %w: expected array index", ErrTask120Invalid)
			}
			i++
			tokens = append(tokens, "i:"+path[start:i-1])
		default:
			return nil, fmt.Errorf("JSON query: %w: unexpected path character %q", ErrTask120Invalid, path[i])
		}
	}
	return tokens, nil
}

func task137JSONSelect(value Task120ConfigValue, token string) ([]Task120ConfigValue, error) {
	if token == "*" {
		switch value.Kind {
		case Task120ConfigArray:
			return append([]Task120ConfigValue(nil), value.Array...), nil
		case Task120ConfigObject:
			out := make([]Task120ConfigValue, len(value.Object))
			for i, field := range value.Object {
				out[i] = field.Value
			}
			return out, nil
		default:
			return nil, nil
		}
	}
	if strings.HasPrefix(token, "k:") {
		if value.Kind != Task120ConfigObject {
			return nil, nil
		}
		key := strings.TrimPrefix(token, "k:")
		for _, field := range value.Object {
			if field.Key == key {
				return []Task120ConfigValue{field.Value}, nil
			}
		}
		return nil, nil
	}
	if strings.HasPrefix(token, "i:") {
		if value.Kind != Task120ConfigArray {
			return nil, nil
		}
		index, err := strconv.Atoi(strings.TrimPrefix(token, "i:"))
		if err != nil || index < 0 || index >= len(value.Array) {
			return nil, nil
		}
		return []Task120ConfigValue{value.Array[index]}, nil
	}
	return nil, fmt.Errorf("JSON query: %w: unknown selector", ErrTask120Invalid)
}

func task137JSONPointerEscape(value string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(value)
}

func task137JSONQueryLimits(limits ...Task137JSONQueryLimits) Task137JSONQueryLimits {
	if len(limits) == 0 {
		return (Task137JSONQueryLimits{}).normalized()
	}
	return limits[0].normalized()
}

func task137JSONValidationLimits(limits ...Task137JSONValidationLimits) Task137JSONValidationLimits {
	if len(limits) == 0 {
		return (Task137JSONValidationLimits{}).normalized()
	}
	return limits[0].normalized()
}

func task137JSONStringList(values []string) Value {
	out := make([]Value, len(values))
	for i, value := range values {
		out[i] = stringValue(value)
	}
	return listValue(out)
}

func task137JSONIssueList(issues []Task137JSONValidationError) Value {
	out := make([]Value, len(issues))
	for i, issue := range issues {
		out[i] = mapValueOf([]mapEntry{{key: stringValue("path"), value: stringValue(issue.Path)}, {key: stringValue("keyword"), value: stringValue(issue.Keyword)}, {key: stringValue("message"), value: stringValue(issue.Message)}})
	}
	return listValue(out)
}

// Task137JSONModuleExtensions returns additive std/json members for the
// module registration layer. It is intentionally separate from task-120's
// module map so integration can merge this map without altering task-120 code.
func Task137JSONModuleExtensions() map[string]Value {
	return map[string]Value{
		"task137_version": stringValue(Task137JSONVersion),
		"type_of": functionValueOf("std/json.type_of", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("json.type_of expects one value")
			}
			value, err := task120ValueToConfig(args[0])
			if err != nil {
				return nullValue(), err
			}
			kind, err := Task137JSONType(value)
			return stringValue(string(kind)), err
		}),
		"keys": functionValueOf("std/json.keys", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("json.keys expects one object")
			}
			value, err := task120ValueToConfig(args[0])
			if err != nil {
				return nullValue(), err
			}
			keys, err := Task137JSONObjectKeys(value)
			return task137JSONStringList(keys), err
		}),
		"length": functionValueOf("std/json.length", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("json.length expects one array or object")
			}
			value, err := task120ValueToConfig(args[0])
			if err != nil {
				return nullValue(), err
			}
			length, err := Task137JSONLength(value)
			return intValue(big.NewInt(int64(length)), 64), err
		}),
		"equal": functionValueOf("std/json.equal", func(args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("json.equal expects two values")
			}
			left, err := task120ValueToConfig(args[0])
			if err != nil {
				return nullValue(), err
			}
			right, err := task120ValueToConfig(args[1])
			if err != nil {
				return nullValue(), err
			}
			equal, err := Task137JSONEqual(left, right)
			return boolValue(equal), err
		}),
		"merge": functionValueOf("std/json.merge", func(args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("json.merge expects base and overlay")
			}
			base, err := task120ValueToConfig(args[0])
			if err != nil {
				return nullValue(), err
			}
			overlay, err := task120ValueToConfig(args[1])
			if err != nil {
				return nullValue(), err
			}
			merged, err := Task137JSONMerge(base, overlay)
			return task120ConfigToValue(merged), err
		}),
		"query": contextFunctionValueOf("std/json.query", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("json.query expects value and path")
			}
			value, err := task120ValueToConfig(args[0])
			if err != nil {
				return nullValue(), err
			}
			path, err := taskStringArg(args[1], "JSON query path")
			if err != nil {
				return nullValue(), err
			}
			results, err := Task137JSONQuery(ctx, value, path)
			if err != nil {
				return nullValue(), err
			}
			out := make([]Value, len(results))
			for i, item := range results {
				out[i] = task120ConfigToValue(item)
			}
			return listValue(out), nil
		}),
		"parse_lines": contextFunctionValueOf("std/json.parse_lines", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("json.parse_lines expects byte-compatible data")
			}
			data, err := taskBytesArg(args[0], "JSON Lines data")
			if err != nil {
				return nullValue(), err
			}
			values, err := Task137JSONParseLines(ctx, data)
			if err != nil {
				return nullValue(), err
			}
			out := make([]Value, len(values))
			for i, item := range values {
				out[i] = task120ConfigToValue(item)
			}
			return listValue(out), nil
		}),
		"stringify_lines": functionValueOf("std/json.stringify_lines", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("json.stringify_lines expects an array")
			}
			items, err := args[0].AsList()
			if err != nil {
				return nullValue(), err
			}
			values := make([]Task120ConfigValue, len(items))
			for i, item := range items {
				values[i], err = task120ValueToConfig(item)
				if err != nil {
					return nullValue(), err
				}
			}
			encoded, err := Task137JSONStringifyLines(values)
			return task120JSONBytesResult(encoded), err
		}),
	}
}

// Task137JSONModule is the named integration helper for the additive JSON
// members. It is separate from task120JSONModule so a caller can merge maps
// without editing either implementation file.
func Task137JSONModule() map[string]Value {
	return Task137JSONModuleExtensions()
}

// task137JSONModule is the package-local spelling used by module registration
// code that prefers the existing task-prefixed helper convention.
func task137JSONModule() map[string]Value {
	return Task137JSONModule()
}

// task137JSONSortedKeys is retained for deterministic future catalog/schema
// adapters that need map-backed input while the public representation remains
// ordered.
func task137JSONSortedKeys(values map[string]Task137JSONSchema) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
