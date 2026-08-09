package osiris

import (
	"context"
	"fmt"
)

func stdlibJSONValueArg(value Value, name string) (StdlibConfigValue, error) {
	converted, err := stdlibValueToConfig(value)
	if err != nil {
		return StdlibConfigValue{}, fmt.Errorf("%s: %w", name, err)
	}
	return converted, nil
}

func stdlibJSONBytesResult(value []byte) Value {
	return bytesListValue(value)
}

func stdlibJSONSchemaFromValue(value Value) (StdlibConfigSchema, error) {
	schema := StdlibConfigSchema{Type: StdlibSchemaAny}
	if raw, ok := taskMapValue(value, "type"); ok {
		text, err := taskStringArg(raw, "schema.type")
		if err != nil {
			return schema, err
		}
		schema.Type = StdlibConfigSchemaType(text)
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
		child, err := stdlibJSONSchemaFromValue(raw)
		if err != nil {
			return schema, err
		}
		schema.Items = &child
	}
	if raw, ok := taskMapValue(value, "properties"); ok && raw.kind == kindMap && raw.mapValue != nil {
		schema.Properties = make(map[string]StdlibConfigSchema, len(raw.mapValue.entries))
		for _, entry := range raw.mapValue.entries {
			name, err := taskStringArg(entry.key, "schema property")
			if err != nil {
				return schema, err
			}
			child, err := stdlibJSONSchemaFromValue(entry.value)
			if err != nil {
				return schema, err
			}
			schema.Properties[name] = child
		}
	}
	return schema, nil
}

func stdlibJSONModule() map[string]Value {
	parse := func(ctx context.Context, args []Value) (Value, error) {
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("json.parse expects one byte-compatible value")
		}
		data, err := taskBytesArg(args[0], "JSON data")
		if err != nil {
			return nullValue(), err
		}
		parsed, err := StdlibJSONParse(ctx, data, StdlibJSONLimits{})
		if err != nil {
			return nullValue(), err
		}
		return stdlibConfigToValue(parsed), nil
	}
	parseProperties := func(ctx context.Context, args []Value) (Value, error) {
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("json.parse_properties expects one byte-compatible value")
		}
		data, err := taskBytesArg(args[0], "properties data")
		if err != nil {
			return nullValue(), err
		}
		parsed, err := StdlibParsePropertiesConfig(ctx, data, StdlibConfigLimits{})
		if err != nil {
			return nullValue(), err
		}
		return stdlibConfigToValue(parsed), nil
	}
	serialize := func(args []Value, pretty bool, indent string) (Value, error) {
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("json serialization expects one value")
		}
		value, err := stdlibJSONValueArg(args[0], "JSON value")
		if err != nil {
			return nullValue(), err
		}
		var encoded []byte
		if pretty {
			encoded, err = StdlibJSONPrettyIndent(value, "", indent, StdlibJSONLimits{})
		} else {
			encoded, err = StdlibStringifyJSON(value, StdlibJSONLimits{})
		}
		if err != nil {
			return nullValue(), err
		}
		return stdlibJSONBytesResult(encoded), nil
	}
	pointer := func(args []Value, operation string) (StdlibConfigValue, string, error) {
		if len(args) < 2 {
			return StdlibConfigValue{}, "", fmt.Errorf("json.%s expects value and pointer", operation)
		}
		value, err := stdlibJSONValueArg(args[0], "JSON value")
		if err != nil {
			return StdlibConfigValue{}, "", err
		}
		path, err := taskStringArg(args[1], "JSON Pointer")
		if err != nil {
			return StdlibConfigValue{}, "", err
		}
		return value, path, nil
	}
	return map[string]Value{
		"version":          stringValue(StdlibJSONVersion),
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
			valid, err := StdlibJSONValid(ctx, data, StdlibJSONLimits{})
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
			value, err := stdlibJSONValueArg(args[0], "JSON value")
			if err != nil {
				return nullValue(), err
			}
			encoded, err := StdlibJSONCanonicalize(value, StdlibJSONLimits{})
			if err != nil {
				return nullValue(), err
			}
			return stdlibJSONBytesResult(encoded), nil
		}),
		"pointer_get": functionValueOf("std/json.pointer_get", func(args []Value) (Value, error) {
			value, path, err := pointer(args, "pointer_get")
			if err != nil {
				return nullValue(), err
			}
			result, err := StdlibJSONPointerGet(value, path)
			if err != nil {
				return nullValue(), err
			}
			return stdlibConfigToValue(result), nil
		}),
		"pointer_set": functionValueOf("std/json.pointer_set", func(args []Value) (Value, error) {
			if len(args) != 3 {
				return nullValue(), fmt.Errorf("json.pointer_set expects value, pointer, and replacement")
			}
			value, path, err := pointer(args, "pointer_set")
			if err != nil {
				return nullValue(), err
			}
			replacement, err := stdlibJSONValueArg(args[2], "replacement")
			if err != nil {
				return nullValue(), err
			}
			result, err := StdlibJSONPointerSet(value, path, replacement)
			if err != nil {
				return nullValue(), err
			}
			return stdlibConfigToValue(result), nil
		}),
		"pointer_remove": functionValueOf("std/json.pointer_remove", func(args []Value) (Value, error) {
			value, path, err := pointer(args, "pointer_remove")
			if err != nil {
				return nullValue(), err
			}
			result, err := StdlibJSONPointerRemove(value, path)
			if err != nil {
				return nullValue(), err
			}
			return stdlibConfigToValue(result), nil
		}),
		"patch": functionValueOf("std/json.patch", func(args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("json.patch expects document and patch")
			}
			document, err := stdlibJSONValueArg(args[0], "document")
			if err != nil {
				return nullValue(), err
			}
			patch, err := stdlibJSONValueArg(args[1], "patch")
			if err != nil {
				return nullValue(), err
			}
			result, err := StdlibJSONPatchApply(document, patch)
			if err != nil {
				return nullValue(), err
			}
			return stdlibConfigToValue(result), nil
		}),
		"merge_patch": functionValueOf("std/json.merge_patch", func(args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("json.merge_patch expects document and patch")
			}
			document, err := stdlibJSONValueArg(args[0], "document")
			if err != nil {
				return nullValue(), err
			}
			patch, err := stdlibJSONValueArg(args[1], "patch")
			if err != nil {
				return nullValue(), err
			}
			result, err := StdlibApplyJSONMergePatch(context.Background(), document, patch, StdlibJSONLimits{})
			if err != nil {
				return nullValue(), err
			}
			return stdlibConfigToValue(result), nil
		}),
		"validate": functionValueOf("std/json.validate", func(args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("json.validate expects value and schema")
			}
			value, err := stdlibJSONValueArg(args[0], "JSON value")
			if err != nil {
				return nullValue(), err
			}
			schema, err := stdlibJSONSchemaFromValue(args[1])
			if err != nil {
				return nullValue(), err
			}
			return boolValue(StdlibValidateConfig(value, schema) == nil), nil
		}),
		"schema_property_names": functionValueOf("std/json.schema_property_names", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("json.schema_property_names expects schema")
			}
			schema, err := stdlibJSONSchemaFromValue(args[0])
			if err != nil {
				return nullValue(), err
			}
			names := StdlibSchemaPropertyNames(schema)
			out := make([]Value, len(names))
			for index, name := range names {
				out[index] = stringValue(name)
			}
			return listValue(out), nil
		}),
	}
}
