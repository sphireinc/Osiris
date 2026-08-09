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

// ExtendedJSONVersion identifies the additive JSON query, transform, JSON Lines,
// and extended-schema contract. The stdlib APIs remain unchanged.
const ExtendedJSONVersion = "json-query-jsonl-schema-1"

const (
	ExtendedJSONDefaultMaxQueryResults = 1024
	ExtendedJSONDefaultMaxQuerySteps   = 10000
	ExtendedJSONDefaultMaxLines        = 1024
	ExtendedJSONDefaultMaxErrors       = 256
)

// ExtendedJSONQueryLimits bounds path parsing and traversal. Zero values select
// the documented defaults. MaxPathBytes is deliberately small because paths
// are control data, not a transport for document content.
type ExtendedJSONQueryLimits struct {
	MaxPathBytes int
	MaxResults   int
	MaxSteps     int
}

func (l ExtendedJSONQueryLimits) normalized() ExtendedJSONQueryLimits {
	if l.MaxPathBytes <= 0 {
		l.MaxPathBytes = 4096
	}
	if l.MaxResults <= 0 {
		l.MaxResults = ExtendedJSONDefaultMaxQueryResults
	}
	if l.MaxSteps <= 0 {
		l.MaxSteps = ExtendedJSONDefaultMaxQuerySteps
	}
	return l
}

// ExtendedJSONQuery evaluates a deliberately bounded JSONPath subset. Supported
// paths are $, .member, ["member"], ['member'], [integer], and [*]. Results
// preserve object insertion order and array order. Every result is detached.
func ExtendedJSONQuery(ctx context.Context, value StdlibConfigValue, path string, limits ...ExtendedJSONQueryLimits) ([]StdlibConfigValue, error) {
	lim := extendedJSONQueryLimits(limits...)
	if len(path) > lim.MaxPathBytes {
		return nil, stdlibLimitError("JSON query", "path bytes", int64(len(path)), int64(lim.MaxPathBytes))
	}
	if err := stdlibJSONCheckValue(ctx, value, stdlibJSONOptionalLimits(), 0, new(int)); err != nil {
		return nil, err
	}
	tokens, err := extendedJSONPath(path)
	if err != nil {
		return nil, err
	}
	current := []StdlibConfigValue{value}
	steps := 0
	for _, token := range tokens {
		if err := stdlibCheckContext(ctx, "JSON query"); err != nil {
			return nil, err
		}
		next := make([]StdlibConfigValue, 0, len(current))
		for _, item := range current {
			steps++
			if steps > lim.MaxSteps {
				return nil, stdlibLimitError("JSON query", "steps", int64(steps), int64(lim.MaxSteps))
			}
			children, err := extendedJSONSelect(item, token)
			if err != nil {
				return nil, err
			}
			if len(next)+len(children) > lim.MaxResults {
				return nil, stdlibLimitError("JSON query", "results", int64(len(next)+len(children)), int64(lim.MaxResults))
			}
			next = append(next, children...)
		}
		current = next
	}
	for i := range current {
		current[i] = stdlibJSONClone(current[i])
	}
	return current, nil
}

// ExtendedJSONQueryOne returns exactly one JSONPath result.
func ExtendedJSONQueryOne(ctx context.Context, value StdlibConfigValue, path string, limits ...ExtendedJSONQueryLimits) (StdlibConfigValue, error) {
	results, err := ExtendedJSONQuery(ctx, value, path, limits...)
	if err != nil {
		return StdlibConfigValue{}, err
	}
	if len(results) != 1 {
		return StdlibConfigValue{}, fmt.Errorf("JSON query: %w: expected one result, got %d", ErrStdlibInvalid, len(results))
	}
	return results[0], nil
}

// ExtendedJSONPointerExists checks an RFC 6901 pointer without treating a
// missing member as a failed operation. Malformed pointers and limit errors
// remain errors.
func ExtendedJSONPointerExists(value StdlibConfigValue, pointer string, limits ...StdlibJSONLimits) (bool, error) {
	_, err := StdlibJSONPointerGet(value, pointer, limits...)
	if err == nil {
		return true, nil
	}
	if strings.Contains(err.Error(), "not found") {
		return false, nil
	}
	return false, err
}

// ExtendedJSONType returns the stable StdlibConfigKind spelling of a value.
func ExtendedJSONType(value StdlibConfigValue) (StdlibConfigKind, error) {
	switch value.Kind {
	case StdlibConfigNull, StdlibConfigBool, StdlibConfigNumber, StdlibConfigString, StdlibConfigArray, StdlibConfigObject:
		return value.Kind, nil
	default:
		return "", fmt.Errorf("JSON type: %w: unknown value kind %q", ErrStdlibInvalid, value.Kind)
	}
}

// ExtendedJSONObjectKeys returns object keys in source order. It rejects
// non-objects rather than silently returning an empty list.
func ExtendedJSONObjectKeys(value StdlibConfigValue) ([]string, error) {
	if value.Kind != StdlibConfigObject {
		return nil, fmt.Errorf("JSON keys: %w: expected object", ErrStdlibInvalid)
	}
	keys := make([]string, len(value.Object))
	for i, field := range value.Object {
		keys[i] = field.Key
	}
	return keys, nil
}

// ExtendedJSONLength returns the number of members or elements. Scalars are
// rejected to avoid an ambiguous string-byte versus rune-count convention.
func ExtendedJSONLength(value StdlibConfigValue) (int, error) {
	switch value.Kind {
	case StdlibConfigArray:
		return len(value.Array), nil
	case StdlibConfigObject:
		return len(value.Object), nil
	default:
		return 0, fmt.Errorf("JSON length: %w: expected array or object", ErrStdlibInvalid)
	}
}

// ExtendedJSONEqual compares JSON values structurally. Numbers compare by
// exact rational value, so 1, 1.0, and 1e0 are equal.
func ExtendedJSONEqual(left, right StdlibConfigValue) (bool, error) {
	return extendedJSONEqual(left, right)
}

func extendedJSONEqual(left, right StdlibConfigValue) (bool, error) {
	if left.Kind != right.Kind {
		if left.Kind == StdlibConfigNumber && right.Kind == StdlibConfigNumber {
			// unreachable because kinds are equal by representation, retained for clarity.
		}
		return false, nil
	}
	switch left.Kind {
	case StdlibConfigNull:
		return true, nil
	case StdlibConfigBool:
		return left.Bool == right.Bool, nil
	case StdlibConfigNumber:
		l, err := extendedJSONRat(left.Number)
		if err != nil {
			return false, err
		}
		r, err := extendedJSONRat(right.Number)
		if err != nil {
			return false, err
		}
		return l.Cmp(r) == 0, nil
	case StdlibConfigString:
		return left.String == right.String, nil
	case StdlibConfigArray:
		if len(left.Array) != len(right.Array) {
			return false, nil
		}
		for i := range left.Array {
			equal, err := extendedJSONEqual(left.Array[i], right.Array[i])
			if err != nil || !equal {
				return equal, err
			}
		}
		return true, nil
	case StdlibConfigObject:
		if len(left.Object) != len(right.Object) {
			return false, nil
		}
		for i := range left.Object {
			if left.Object[i].Key != right.Object[i].Key {
				return false, nil
			}
			equal, err := extendedJSONEqual(left.Object[i].Value, right.Object[i].Value)
			if err != nil || !equal {
				return equal, err
			}
		}
		return true, nil
	default:
		return false, fmt.Errorf("JSON equal: %w: unknown value kind %q", ErrStdlibInvalid, left.Kind)
	}
}

// ExtendedJSONMerge recursively merges objects. Object members from overlay
// replace base members; object/object pairs merge recursively; arrays and
// scalars replace. The result is detached and bounded by limits.
func ExtendedJSONMerge(base, overlay StdlibConfigValue, limits ...StdlibJSONLimits) (StdlibConfigValue, error) {
	lim := stdlibJSONOptionalLimits(limits...)
	if err := stdlibJSONCheckValue(context.Background(), base, lim, 0, new(int)); err != nil {
		return StdlibConfigValue{}, err
	}
	if err := stdlibJSONCheckValue(context.Background(), overlay, lim, 0, new(int)); err != nil {
		return StdlibConfigValue{}, err
	}
	result, err := extendedJSONMergeValues(base, overlay)
	if err != nil {
		return StdlibConfigValue{}, err
	}
	return result, stdlibJSONCheckValue(context.Background(), result, lim, 0, new(int))
}

func extendedJSONMergeValues(base, overlay StdlibConfigValue) (StdlibConfigValue, error) {
	if base.Kind != StdlibConfigObject || overlay.Kind != StdlibConfigObject {
		return stdlibJSONClone(overlay), nil
	}
	result := stdlibJSONClone(base)
	for _, field := range overlay.Object {
		index, err := stdlibJSONObjectIndex(result.Object, field.Key)
		if err != nil {
			return StdlibConfigValue{}, err
		}
		if index < 0 {
			result.Object = append(result.Object, StdlibConfigField{Key: field.Key, Value: stdlibJSONClone(field.Value)})
			continue
		}
		merged, err := extendedJSONMergeValues(result.Object[index].Value, field.Value)
		if err != nil {
			return StdlibConfigValue{}, err
		}
		result.Object[index].Value = merged
	}
	return result, nil
}

// ExtendedJSONParseLines parses newline-delimited JSON. Empty lines are ignored
// and each non-empty line is independently bounded and must contain one value.
func ExtendedJSONParseLines(ctx context.Context, data []byte, limits ...StdlibJSONLimits) ([]StdlibConfigValue, error) {
	lim := stdlibJSONOptionalLimits(limits...)
	if int64(len(data)) > lim.MaxBytes {
		return nil, stdlibLimitError("JSON Lines parse", "input bytes", int64(len(data)), lim.MaxBytes)
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > ExtendedJSONDefaultMaxLines+1 {
		return nil, stdlibLimitError("JSON Lines parse", "lines", int64(len(lines)), ExtendedJSONDefaultMaxLines)
	}
	out := make([]StdlibConfigValue, 0, len(lines))
	for number, line := range lines {
		if err := stdlibCheckContext(ctx, "JSON Lines parse"); err != nil {
			return nil, err
		}
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		value, err := StdlibJSONParse(ctx, []byte(line), lim)
		if err != nil {
			return nil, fmt.Errorf("JSON Lines parse line %d: %w", number+1, err)
		}
		out = append(out, value)
	}
	return out, nil
}

// ExtendedJSONStringifyLines emits one compact JSON value per line.
func ExtendedJSONStringifyLines(values []StdlibConfigValue, limits ...StdlibJSONLimits) ([]byte, error) {
	if len(values) > ExtendedJSONDefaultMaxLines {
		return nil, stdlibLimitError("JSON Lines stringify", "lines", int64(len(values)), ExtendedJSONDefaultMaxLines)
	}
	lim := stdlibJSONOptionalLimits(limits...)
	var b strings.Builder
	for i, value := range values {
		encoded, err := StdlibJSONStringify(value, lim)
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
		return nil, stdlibLimitError("JSON Lines stringify", "output bytes", int64(b.Len()), lim.MaxOutputBytes)
	}
	return []byte(b.String()), nil
}

// ExtendedJSONSchema is an additive schema vocabulary. Base type, object, and
// array constraints mirror standard library; enum, const, pattern, numeric bounds,
// uniqueness, additionalProperties, and boolean combinators are new.
type ExtendedJSONSchema struct {
	Type                 StdlibConfigSchemaType
	Required             []string
	Properties           map[string]ExtendedJSONSchema
	Items                *ExtendedJSONSchema
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
	Enum                 []StdlibConfigValue
	Const                *StdlibConfigValue
	AnyOf                []ExtendedJSONSchema
	AllOf                []ExtendedJSONSchema
	OneOf                []ExtendedJSONSchema
	Not                  *ExtendedJSONSchema
}

// ExtendedJSONValidationLimits bounds diagnostics and schema recursion.
type ExtendedJSONValidationLimits struct {
	MaxErrors int
	MaxDepth  int
}

func (l ExtendedJSONValidationLimits) normalized() ExtendedJSONValidationLimits {
	if l.MaxErrors <= 0 {
		l.MaxErrors = ExtendedJSONDefaultMaxErrors
	}
	if l.MaxDepth <= 0 {
		l.MaxDepth = 64
	}
	return l
}

// ExtendedJSONValidationError identifies one deterministic schema violation.
type ExtendedJSONValidationError struct {
	Path    string
	Keyword string
	Message string
}

// ExtendedJSONValidateSchema returns all bounded validation issues in document
// order. A nil slice means valid; malformed schema definitions return error.
func ExtendedJSONValidateSchema(ctx context.Context, value StdlibConfigValue, schema ExtendedJSONSchema, limits ...ExtendedJSONValidationLimits) ([]ExtendedJSONValidationError, error) {
	lim := extendedJSONValidationLimits(limits...)
	if err := stdlibJSONCheckValue(ctx, value, stdlibJSONOptionalLimits(), 0, new(int)); err != nil {
		return nil, err
	}
	issues := make([]ExtendedJSONValidationError, 0)
	if err := extendedJSONValidateNode(ctx, value, schema, "", 0, lim, &issues); err != nil {
		return nil, err
	}
	return issues, nil
}

// ExtendedJSONSchemaValid is the boolean convenience form of
// ExtendedJSONValidateSchema.
func ExtendedJSONSchemaValid(ctx context.Context, value StdlibConfigValue, schema ExtendedJSONSchema, limits ...ExtendedJSONValidationLimits) (bool, error) {
	issues, err := ExtendedJSONValidateSchema(ctx, value, schema, limits...)
	return len(issues) == 0, err
}

func extendedJSONValidateNode(ctx context.Context, value StdlibConfigValue, schema ExtendedJSONSchema, path string, depth int, lim ExtendedJSONValidationLimits, issues *[]ExtendedJSONValidationError) error {
	if err := stdlibCheckContext(ctx, "JSON schema validation"); err != nil {
		return err
	}
	if depth > lim.MaxDepth {
		return stdlibLimitError("JSON schema validation", "depth", int64(depth), int64(lim.MaxDepth))
	}
	add := func(keyword, message string) {
		if len(*issues) < lim.MaxErrors {
			*issues = append(*issues, ExtendedJSONValidationError{Path: path, Keyword: keyword, Message: message})
		}
	}
	if schema.Type != "" && schema.Type != StdlibSchemaAny && !extendedJSONTypeMatches(value, schema.Type) {
		add("type", fmt.Sprintf("expected %s, got %s", schema.Type, value.Kind))
		return nil
	}
	if schema.Const != nil {
		equal, err := extendedJSONEqual(value, *schema.Const)
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
			equal, err := extendedJSONEqual(value, candidate)
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
	if value.Kind == StdlibConfigNumber {
		if err := extendedJSONNumberBounds(value.Number, schema, add); err != nil {
			return err
		}
	}
	if value.Kind == StdlibConfigString {
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
				return fmt.Errorf("JSON schema pattern: %w: %v", ErrStdlibInvalid, err)
			}
			if !pattern.MatchString(value.String) {
				add("pattern", "string does not match pattern")
			}
		}
	}
	if value.Kind == StdlibConfigArray {
		if schema.MinItems > 0 && len(value.Array) < schema.MinItems {
			add("minItems", fmt.Sprintf("length %d is below %d", len(value.Array), schema.MinItems))
		}
		if schema.MaxItems > 0 && len(value.Array) > schema.MaxItems {
			add("maxItems", fmt.Sprintf("length %d exceeds %d", len(value.Array), schema.MaxItems))
		}
		if schema.UniqueItems {
			for i := range value.Array {
				for j := 0; j < i; j++ {
					equal, err := extendedJSONEqual(value.Array[i], value.Array[j])
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
				if err := extendedJSONValidateNode(ctx, item, *schema.Items, fmt.Sprintf("%s/%d", path, i), depth+1, lim, issues); err != nil {
					return err
				}
			}
		}
	}
	if value.Kind == StdlibConfigObject {
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
				if err := extendedJSONValidateNode(ctx, field.Value, child, path+"/"+extendedJSONPointerEscape(field.Key), depth+1, lim, issues); err != nil {
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
		if err := extendedJSONValidateNode(ctx, value, child, path, depth+1, lim, issues); err != nil {
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
			branchIssues, err := ExtendedJSONValidateSchema(ctx, value, branch, lim)
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
		branchIssues, err := ExtendedJSONValidateSchema(ctx, value, *schema.Not, lim)
		if err != nil {
			return err
		}
		if len(branchIssues) == 0 {
			add("not", "value matches forbidden schema")
		}
	}
	return nil
}

func extendedJSONNumberBounds(number string, schema ExtendedJSONSchema, add func(string, string)) error {
	if schema.Minimum == "" && schema.Maximum == "" {
		return nil
	}
	value, err := extendedJSONRat(number)
	if err != nil {
		return err
	}
	if schema.Minimum != "" {
		minimum, err := extendedJSONRat(schema.Minimum)
		if err != nil {
			return fmt.Errorf("JSON schema minimum: %w", err)
		}
		cmp := value.Cmp(minimum)
		if (schema.ExclusiveMinimum && cmp <= 0) || (!schema.ExclusiveMinimum && cmp < 0) {
			add("minimum", "number is below minimum")
		}
	}
	if schema.Maximum != "" {
		maximum, err := extendedJSONRat(schema.Maximum)
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

func extendedJSONTypeMatches(value StdlibConfigValue, typ StdlibConfigSchemaType) bool {
	if typ == StdlibSchemaAny {
		return true
	}
	if typ == StdlibSchemaInteger && value.Kind == StdlibConfigNumber {
		rat, err := extendedJSONRat(value.Number)
		return err == nil && rat.IsInt()
	}
	return value.Kind == StdlibConfigKind(typ)
}

// extendedJSONRat converts the complete JSON number grammar, including an
// exponent, to an exact rational without passing through float64.
func extendedJSONRat(number string) (*big.Rat, error) {
	if !stdlibJSONNumberValid(number) {
		return nil, fmt.Errorf("JSON number: %w: invalid number %q", ErrStdlibInvalid, number)
	}
	if len(number) > 1<<20 {
		return nil, stdlibLimitError("JSON number", "bytes", int64(len(number)), 1<<20)
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
			return nil, fmt.Errorf("JSON number: %w: invalid exponent", ErrStdlibInvalid)
		}
		exponent = parsed
		if exponent > 4096 || exponent < -4096 {
			return nil, stdlibLimitError("JSON number", "exponent", int64(exponent), 4096)
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
		return nil, fmt.Errorf("JSON number: %w: invalid mantissa", ErrStdlibInvalid)
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

// ExtendedJSONSchemaFromValue decodes the extended schema vocabulary from the
// same ordered map representation accepted by std/json.
func ExtendedJSONSchemaFromValue(value StdlibConfigValue) (ExtendedJSONSchema, error) {
	if value.Kind != StdlibConfigObject {
		return ExtendedJSONSchema{}, fmt.Errorf("JSON schema: %w: expected object", ErrStdlibInvalid)
	}
	schema := ExtendedJSONSchema{Type: StdlibSchemaAny}
	field := func(name string) (StdlibConfigValue, bool) {
		for _, item := range value.Object {
			if item.Key == name {
				return item.Value, true
			}
		}
		return StdlibConfigValue{}, false
	}
	if raw, ok := field("type"); ok {
		if raw.Kind != StdlibConfigString {
			return schema, fmt.Errorf("JSON schema.type: %w: expected string", ErrStdlibInvalid)
		}
		schema.Type = StdlibConfigSchemaType(raw.String)
	}
	if raw, ok := field("required"); ok {
		if raw.Kind != StdlibConfigArray {
			return schema, fmt.Errorf("JSON schema.required: %w: expected array", ErrStdlibInvalid)
		}
		for _, item := range raw.Array {
			if item.Kind != StdlibConfigString {
				return schema, fmt.Errorf("JSON schema.required: %w: expected strings", ErrStdlibInvalid)
			}
			schema.Required = append(schema.Required, item.String)
		}
	}
	stringFields := map[string]*string{"pattern": &schema.Pattern, "minimum": &schema.Minimum, "maximum": &schema.Maximum}
	for name, target := range stringFields {
		if raw, ok := field(name); ok {
			if raw.Kind != StdlibConfigString && raw.Kind != StdlibConfigNumber {
				return schema, fmt.Errorf("JSON schema.%s: %w: expected string or number", name, ErrStdlibInvalid)
			}
			if raw.Kind == StdlibConfigString {
				*target = raw.String
			} else {
				*target = raw.Number
			}
		}
	}
	boolFields := map[string]*bool{"exclusive_minimum": &schema.ExclusiveMinimum, "exclusive_maximum": &schema.ExclusiveMaximum, "unique_items": &schema.UniqueItems}
	for name, target := range boolFields {
		if raw, ok := field(name); ok {
			if raw.Kind != StdlibConfigBool {
				return schema, fmt.Errorf("JSON schema.%s: %w: expected boolean", name, ErrStdlibInvalid)
			}
			*target = raw.Bool
		}
	}
	if raw, ok := field("additional_properties"); ok {
		if raw.Kind != StdlibConfigBool {
			return schema, fmt.Errorf("JSON schema.additional_properties: %w: expected boolean", ErrStdlibInvalid)
		}
		schema.AdditionalProperties = new(bool)
		*schema.AdditionalProperties = raw.Bool
	}
	for name, target := range map[string]*int{"min_items": &schema.MinItems, "max_items": &schema.MaxItems, "min_properties": &schema.MinProperties, "max_properties": &schema.MaxProperties, "min_length": &schema.MinLength, "max_length": &schema.MaxLength} {
		if raw, ok := field(name); ok {
			if raw.Kind != StdlibConfigNumber {
				return schema, fmt.Errorf("JSON schema.%s: %w: expected integer", name, ErrStdlibInvalid)
			}
			n, err := strconv.Atoi(raw.Number)
			if err != nil || n < 0 {
				return schema, fmt.Errorf("JSON schema.%s: %w: invalid integer", name, ErrStdlibInvalid)
			}
			*target = n
		}
	}
	if raw, ok := field("enum"); ok {
		if raw.Kind != StdlibConfigArray {
			return schema, fmt.Errorf("JSON schema.enum: %w: expected array", ErrStdlibInvalid)
		}
		schema.Enum = make([]StdlibConfigValue, len(raw.Array))
		for i := range raw.Array {
			schema.Enum[i] = stdlibJSONClone(raw.Array[i])
		}
	}
	if raw, ok := field("const"); ok {
		clone := stdlibJSONClone(raw)
		schema.Const = &clone
	}
	if raw, ok := field("items"); ok {
		child, err := ExtendedJSONSchemaFromValue(raw)
		if err != nil {
			return schema, err
		}
		schema.Items = &child
	}
	if raw, ok := field("properties"); ok {
		if raw.Kind != StdlibConfigObject {
			return schema, fmt.Errorf("JSON schema.properties: %w: expected object", ErrStdlibInvalid)
		}
		schema.Properties = make(map[string]ExtendedJSONSchema, len(raw.Object))
		for _, item := range raw.Object {
			child, err := ExtendedJSONSchemaFromValue(item.Value)
			if err != nil {
				return schema, err
			}
			schema.Properties[item.Key] = child
		}
	}
	return schema, nil
}

func extendedJSONPath(path string) ([]string, error) {
	if path == "$" || path == "" {
		return nil, nil
	}
	if !strings.HasPrefix(path, "$") {
		return nil, fmt.Errorf("JSON query: %w: path must start with $", ErrStdlibInvalid)
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
				return nil, fmt.Errorf("JSON query: %w: empty member", ErrStdlibInvalid)
			}
			tokens = append(tokens, "k:"+path[start:i])
		case '[':
			i++
			if i >= len(path) {
				return nil, fmt.Errorf("JSON query: %w: unterminated selector", ErrStdlibInvalid)
			}
			if path[i] == '*' {
				i++
				if i >= len(path) || path[i] != ']' {
					return nil, fmt.Errorf("JSON query: %w: expected ]", ErrStdlibInvalid)
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
							return nil, fmt.Errorf("JSON query: %w: incomplete escape", ErrStdlibInvalid)
						}
					}
					b.WriteByte(path[i])
					i++
				}
				if i >= len(path) || path[i] != quote {
					return nil, fmt.Errorf("JSON query: %w: unterminated member", ErrStdlibInvalid)
				}
				i++
				if i >= len(path) || path[i] != ']' {
					return nil, fmt.Errorf("JSON query: %w: expected ]", ErrStdlibInvalid)
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
				return nil, fmt.Errorf("JSON query: %w: expected array index", ErrStdlibInvalid)
			}
			i++
			tokens = append(tokens, "i:"+path[start:i-1])
		default:
			return nil, fmt.Errorf("JSON query: %w: unexpected path character %q", ErrStdlibInvalid, path[i])
		}
	}
	return tokens, nil
}

func extendedJSONSelect(value StdlibConfigValue, token string) ([]StdlibConfigValue, error) {
	if token == "*" {
		switch value.Kind {
		case StdlibConfigArray:
			return append([]StdlibConfigValue(nil), value.Array...), nil
		case StdlibConfigObject:
			out := make([]StdlibConfigValue, len(value.Object))
			for i, field := range value.Object {
				out[i] = field.Value
			}
			return out, nil
		default:
			return nil, nil
		}
	}
	if strings.HasPrefix(token, "k:") {
		if value.Kind != StdlibConfigObject {
			return nil, nil
		}
		key := strings.TrimPrefix(token, "k:")
		for _, field := range value.Object {
			if field.Key == key {
				return []StdlibConfigValue{field.Value}, nil
			}
		}
		return nil, nil
	}
	if strings.HasPrefix(token, "i:") {
		if value.Kind != StdlibConfigArray {
			return nil, nil
		}
		index, err := strconv.Atoi(strings.TrimPrefix(token, "i:"))
		if err != nil || index < 0 || index >= len(value.Array) {
			return nil, nil
		}
		return []StdlibConfigValue{value.Array[index]}, nil
	}
	return nil, fmt.Errorf("JSON query: %w: unknown selector", ErrStdlibInvalid)
}

func extendedJSONPointerEscape(value string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(value)
}

func extendedJSONQueryLimits(limits ...ExtendedJSONQueryLimits) ExtendedJSONQueryLimits {
	if len(limits) == 0 {
		return (ExtendedJSONQueryLimits{}).normalized()
	}
	return limits[0].normalized()
}

func extendedJSONValidationLimits(limits ...ExtendedJSONValidationLimits) ExtendedJSONValidationLimits {
	if len(limits) == 0 {
		return (ExtendedJSONValidationLimits{}).normalized()
	}
	return limits[0].normalized()
}

func extendedJSONStringList(values []string) Value {
	out := make([]Value, len(values))
	for i, value := range values {
		out[i] = stringValue(value)
	}
	return listValue(out)
}

func extendedJSONIssueList(issues []ExtendedJSONValidationError) Value {
	out := make([]Value, len(issues))
	for i, issue := range issues {
		out[i] = mapValueOf([]mapEntry{{key: stringValue("path"), value: stringValue(issue.Path)}, {key: stringValue("keyword"), value: stringValue(issue.Keyword)}, {key: stringValue("message"), value: stringValue(issue.Message)}})
	}
	return listValue(out)
}

// ExtendedJSONModuleExtensions returns additive std/json members for the
// module registration layer. It is intentionally separate from stdlib's
// module map so integration can merge this map without altering stdlib code.
func ExtendedJSONModuleExtensions() map[string]Value {
	return map[string]Value{
		"extensions_version": stringValue(ExtendedJSONVersion),
		"type_of": functionValueOf("std/json.type_of", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("json.type_of expects one value")
			}
			value, err := stdlibValueToConfig(args[0])
			if err != nil {
				return nullValue(), err
			}
			kind, err := ExtendedJSONType(value)
			return stringValue(string(kind)), err
		}),
		"keys": functionValueOf("std/json.keys", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("json.keys expects one object")
			}
			value, err := stdlibValueToConfig(args[0])
			if err != nil {
				return nullValue(), err
			}
			keys, err := ExtendedJSONObjectKeys(value)
			return extendedJSONStringList(keys), err
		}),
		"length": functionValueOf("std/json.length", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("json.length expects one array or object")
			}
			value, err := stdlibValueToConfig(args[0])
			if err != nil {
				return nullValue(), err
			}
			length, err := ExtendedJSONLength(value)
			return intValue(big.NewInt(int64(length)), 64), err
		}),
		"equal": functionValueOf("std/json.equal", func(args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("json.equal expects two values")
			}
			left, err := stdlibValueToConfig(args[0])
			if err != nil {
				return nullValue(), err
			}
			right, err := stdlibValueToConfig(args[1])
			if err != nil {
				return nullValue(), err
			}
			equal, err := ExtendedJSONEqual(left, right)
			return boolValue(equal), err
		}),
		"merge": functionValueOf("std/json.merge", func(args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("json.merge expects base and overlay")
			}
			base, err := stdlibValueToConfig(args[0])
			if err != nil {
				return nullValue(), err
			}
			overlay, err := stdlibValueToConfig(args[1])
			if err != nil {
				return nullValue(), err
			}
			merged, err := ExtendedJSONMerge(base, overlay)
			return stdlibConfigToValue(merged), err
		}),
		"query": contextFunctionValueOf("std/json.query", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("json.query expects value and path")
			}
			value, err := stdlibValueToConfig(args[0])
			if err != nil {
				return nullValue(), err
			}
			path, err := taskStringArg(args[1], "JSON query path")
			if err != nil {
				return nullValue(), err
			}
			results, err := ExtendedJSONQuery(ctx, value, path)
			if err != nil {
				return nullValue(), err
			}
			out := make([]Value, len(results))
			for i, item := range results {
				out[i] = stdlibConfigToValue(item)
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
			values, err := ExtendedJSONParseLines(ctx, data)
			if err != nil {
				return nullValue(), err
			}
			out := make([]Value, len(values))
			for i, item := range values {
				out[i] = stdlibConfigToValue(item)
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
			values := make([]StdlibConfigValue, len(items))
			for i, item := range items {
				values[i], err = stdlibValueToConfig(item)
				if err != nil {
					return nullValue(), err
				}
			}
			encoded, err := ExtendedJSONStringifyLines(values)
			return stdlibJSONBytesResult(encoded), err
		}),
	}
}

// ExtendedJSONModule is the named integration helper for the additive JSON
// members. It is separate from stdlibJSONModule so a caller can merge maps
// without editing either implementation file.
func ExtendedJSONModule() map[string]Value {
	return ExtendedJSONModuleExtensions()
}

// extendedJSONModule is the package-local spelling used by module registration
// code that prefers the existing task-prefixed helper convention.
func extendedJSONModule() map[string]Value {
	return ExtendedJSONModule()
}

// extendedJSONSortedKeys is retained for deterministic future catalog/schema
// adapters that need map-backed input while the public representation remains
// ordered.
func extendedJSONSortedKeys(values map[string]ExtendedJSONSchema) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
