package osiris

import (
	"context"
	"fmt"
)

func task120JSONValueArg(value Value, name string) (Task120ConfigValue, error) {
	converted, err := task120ValueToConfig(value)
	if err != nil {
		return Task120ConfigValue{}, fmt.Errorf("%s: %w", name, err)
	}
	return converted, nil
}

func task120JSONBytesResult(value []byte) Value {
	return bytesListValue(value)
}

func task120JSONSchemaFromValue(value Value) (Task120ConfigSchema, error) {
	schema := Task120ConfigSchema{Type: Task120SchemaAny}
	if raw, ok := taskMapValue(value, "type"); ok {
		text, err := taskStringArg(raw, "schema.type")
		if err != nil {
			return schema, err
		}
		schema.Type = Task120ConfigSchemaType(text)
	}
	if raw, ok := taskMapValue(value, "required"); ok {
		items, err := raw.AsList()
		if err != nil {
			return schema, fmt.Errorf("schema.required: %w", err)
		}
		for _, item := range items {
			text, err := taskStringArg(item, "schema.required item")
			if err != nil {
				return schema, err
			}
			schema.Required = append(schema.Required, text)
		}
	}
	if raw, ok := taskMapValue(value, "allow_additional"); ok && raw.kind == kindBool {
		schema.AllowAdditional = raw.boo
	}
	for name, target := range map[string]*int{
		"min_items": &schema.MinItems, "max_items": &schema.MaxItems,
		"min_properties": &schema.MinProperties, "max_properties": &schema.MaxProperties,
		"min_length": &schema.MinLength, "max_length": &schema.MaxLength,
	} {
		if raw, ok := taskMapValue(value, name); ok {
			number, err := raw.Int()
			if err != nil {
				return schema, fmt.Errorf("schema.%s: %w", name, err)
			}
			*target = number
		}
	}
	if raw, ok := taskMapValue(value, "items"); ok {
		child, err := task120JSONSchemaFromValue(raw)
		if err != nil {
			return schema, err
		}
		schema.Items = &child
	}
	if raw, ok := taskMapValue(value, "properties"); ok && raw.kind == kindMap && raw.mapValue != nil {
		schema.Properties = make(map[string]Task120ConfigSchema, len(raw.mapValue.entries))
		for _, entry := range raw.mapValue.entries {
			name, err := taskStringArg(entry.key, "schema property")
			if err != nil {
				return schema, err
			}
			child, err := task120JSONSchemaFromValue(entry.value)
			if err != nil {
				return schema, err
			}
			schema.Properties[name] = child
		}
	}
	return schema, nil
}

func task120JSONModule() map[string]Value {
	parse := func(ctx context.Context, args []Value) (Value, error) {
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("json.parse expects one byte-compatible value")
		}
		data, err := taskBytesArg(args[0], "JSON data")
		if err != nil {
			return nullValue(), err
		}
		parsed, err := Task120JSONParse(ctx, data, Task120JSONLimits{})
		if err != nil {
			return nullValue(), err
		}
		return task120ConfigToValue(parsed), nil
	}
	parseProperties := func(ctx context.Context, args []Value) (Value, error) {
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("json.parse_properties expects one byte-compatible value")
		}
		data, err := taskBytesArg(args[0], "properties data")
		if err != nil {
			return nullValue(), err
		}
		parsed, err := Task120ParsePropertiesConfig(ctx, data, Task120ConfigLimits{})
		if err != nil {
			return nullValue(), err
		}
		return task120ConfigToValue(parsed), nil
	}
	serialize := func(args []Value, pretty bool, indent string) (Value, error) {
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("json serialization expects one value")
		}
		value, err := task120JSONValueArg(args[0], "JSON value")
		if err != nil {
			return nullValue(), err
		}
		var encoded []byte
		if pretty {
			encoded, err = Task120JSONPrettyIndent(value, "", indent, Task120JSONLimits{})
		} else {
			encoded, err = Task120StringifyJSON(value, Task120JSONLimits{})
		}
		if err != nil {
			return nullValue(), err
		}
		return task120JSONBytesResult(encoded), nil
	}
	pointer := func(args []Value, operation string) (Task120ConfigValue, string, error) {
		if len(args) < 2 {
			return Task120ConfigValue{}, "", fmt.Errorf("json.%s expects value and pointer", operation)
		}
		value, err := task120JSONValueArg(args[0], "JSON value")
		if err != nil {
			return Task120ConfigValue{}, "", err
		}
		path, err := taskStringArg(args[1], "JSON Pointer")
		if err != nil {
			return Task120ConfigValue{}, "", err
		}
		return value, path, nil
	}
	return map[string]Value{
		"version":          stringValue(Task120JSONVersion),
		"parse":            contextFunctionValueOf("std/json.parse", parse),
		"parse_json":       contextFunctionValueOf("std/json.parse_json", parse),
		"parse_properties": contextFunctionValueOf("std/json.parse_properties", parseProperties),
		"valid": contextFunctionValueOf("std/json.valid", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("json.valid expects one byte-compatible value")
			}
			data, err := taskBytesArg(args[0], "JSON data")
			if err != nil {
				return nullValue(), err
			}
			valid, err := Task120JSONValid(ctx, data, Task120JSONLimits{})
			return boolValue(valid), err
		}),
		"stringify": functionValueOf("std/json.stringify", func(args []Value) (Value, error) { return serialize(args, false, "") }),
		"pretty": functionValueOf("std/json.pretty", func(args []Value) (Value, error) {
			if len(args) == 1 {
				return serialize(args, true, "  ")
			}
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("json.pretty expects value and optional indent string")
			}
			indent, err := taskStringArg(args[1], "JSON indent")
			if err != nil {
				return nullValue(), err
			}
			if indent == "" || len(indent) > 16 {
				return nullValue(), fmt.Errorf("json.pretty indent must contain 1 to 16 characters")
			}
			return serialize(args[:1], true, indent)
		}),
		"canonicalize": functionValueOf("std/json.canonicalize", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("json.canonicalize expects one value")
			}
			value, err := task120JSONValueArg(args[0], "JSON value")
			if err != nil {
				return nullValue(), err
			}
			encoded, err := Task120JSONCanonicalize(value, Task120JSONLimits{})
			if err != nil {
				return nullValue(), err
			}
			return task120JSONBytesResult(encoded), nil
		}),
		"pointer_get": functionValueOf("std/json.pointer_get", func(args []Value) (Value, error) {
			value, path, err := pointer(args, "pointer_get")
			if err != nil {
				return nullValue(), err
			}
			result, err := Task120JSONPointerGet(value, path)
			if err != nil {
				return nullValue(), err
			}
			return task120ConfigToValue(result), nil
		}),
		"pointer_set": functionValueOf("std/json.pointer_set", func(args []Value) (Value, error) {
			if len(args) != 3 {
				return nullValue(), fmt.Errorf("json.pointer_set expects value, pointer, and replacement")
			}
			value, path, err := pointer(args, "pointer_set")
			if err != nil {
				return nullValue(), err
			}
			replacement, err := task120JSONValueArg(args[2], "replacement")
			if err != nil {
				return nullValue(), err
			}
			result, err := Task120JSONPointerSet(value, path, replacement)
			if err != nil {
				return nullValue(), err
			}
			return task120ConfigToValue(result), nil
		}),
		"pointer_remove": functionValueOf("std/json.pointer_remove", func(args []Value) (Value, error) {
			value, path, err := pointer(args, "pointer_remove")
			if err != nil {
				return nullValue(), err
			}
			result, err := Task120JSONPointerRemove(value, path)
			if err != nil {
				return nullValue(), err
			}
			return task120ConfigToValue(result), nil
		}),
		"patch": functionValueOf("std/json.patch", func(args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("json.patch expects document and patch")
			}
			document, err := task120JSONValueArg(args[0], "document")
			if err != nil {
				return nullValue(), err
			}
			patch, err := task120JSONValueArg(args[1], "patch")
			if err != nil {
				return nullValue(), err
			}
			result, err := Task120JSONPatchApply(document, patch)
			if err != nil {
				return nullValue(), err
			}
			return task120ConfigToValue(result), nil
		}),
		"merge_patch": functionValueOf("std/json.merge_patch", func(args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("json.merge_patch expects document and patch")
			}
			document, err := task120JSONValueArg(args[0], "document")
			if err != nil {
				return nullValue(), err
			}
			patch, err := task120JSONValueArg(args[1], "patch")
			if err != nil {
				return nullValue(), err
			}
			result, err := Task120ApplyJSONMergePatch(context.Background(), document, patch, Task120JSONLimits{})
			if err != nil {
				return nullValue(), err
			}
			return task120ConfigToValue(result), nil
		}),
		"validate": functionValueOf("std/json.validate", func(args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("json.validate expects value and schema")
			}
			value, err := task120JSONValueArg(args[0], "JSON value")
			if err != nil {
				return nullValue(), err
			}
			schema, err := task120JSONSchemaFromValue(args[1])
			if err != nil {
				return nullValue(), err
			}
			return boolValue(Task120ValidateConfig(value, schema) == nil), nil
		}),
		"schema_property_names": functionValueOf("std/json.schema_property_names", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("json.schema_property_names expects schema")
			}
			schema, err := task120JSONSchemaFromValue(args[0])
			if err != nil {
				return nullValue(), err
			}
			names := Task120SchemaPropertyNames(schema)
			out := make([]Value, len(names))
			for index, name := range names {
				out[index] = stringValue(name)
			}
			return listValue(out), nil
		}),
	}
}
