package osiris

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
)

// taskMapValue returns a named field from a language map.  Standard-library
// adapters use this small helper so map field spelling and missing-field
// behavior stay consistent across interpreter and VM calls.
func taskMapValue(value Value, name string) (Value, bool) {
	if value.kind != kindMap || value.mapValue == nil {
		return Value{}, false
	}
	for _, entry := range value.mapValue.entries {
		if entry.key.kind == kindString && entry.key.str == name {
			return entry.value, true
		}
	}
	return Value{}, false
}

func taskStringArg(value Value, name string) (string, error) {
	result, err := value.AsString()
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return result, nil
}

func taskBytesArg(value Value, name string) ([]byte, error) {
	result, err := bytesFromValue(value)
	if err != nil {
		return nil, fmt.Errorf("%s: expected byte-compatible list: %w", name, err)
	}
	return result, nil
}

func task120RegexValue(ctx context.Context, args []Value, operation func(context.Context, *Task120CompiledRegex, []Value) (Value, error)) (Value, error) {
	if len(args) < 2 {
		return nullValue(), fmt.Errorf("regex operation expects pattern and input")
	}
	pattern, err := taskStringArg(args[0], "regex pattern")
	if err != nil {
		return nullValue(), err
	}
	compiled, err := Task120CompileRegex(ctx, pattern, Task120RegexLimits{})
	if err != nil {
		return nullValue(), err
	}
	return operation(ctx, compiled, args[1:])
}

func task120RegexModule() map[string]Value {
	mod := map[string]Value{
		"version":        stringValue(task120Version),
		"engine_version": functionValueOf("std/regex.engine_version", func([]Value) (Value, error) { return stringValue(Task120RegexVersion), nil }),
		"compile": contextFunctionValueOf("std/regex.compile", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("regex.compile expects pattern")
			}
			pattern, err := taskStringArg(args[0], "regex pattern")
			if err != nil {
				return nullValue(), err
			}
			if _, err := Task120CompileRegex(ctx, pattern, Task120RegexLimits{}); err != nil {
				return nullValue(), err
			}
			return mapValueOf([]mapEntry{{key: stringValue("pattern"), value: stringValue(pattern)}, {key: stringValue("engine_version"), value: stringValue(Task120RegexVersion)}}), nil
		}),
		"match": contextFunctionValueOf("std/regex.match", func(ctx context.Context, args []Value) (Value, error) {
			return task120RegexValue(ctx, args, func(ctx context.Context, re *Task120CompiledRegex, rest []Value) (Value, error) {
				input, err := taskStringArg(rest[0], "regex input")
				if err != nil {
					return nullValue(), err
				}
				matched, err := re.Match(ctx, input)
				return boolValue(matched), err
			})
		}),
		"find": contextFunctionValueOf("std/regex.find", func(ctx context.Context, args []Value) (Value, error) {
			return task120RegexValue(ctx, args, func(ctx context.Context, re *Task120CompiledRegex, rest []Value) (Value, error) {
				input, err := taskStringArg(rest[0], "regex input")
				if err != nil {
					return nullValue(), err
				}
				matches, err := re.Find(ctx, input)
				if err != nil {
					return nullValue(), err
				}
				out := make([]Value, len(matches))
				for i, match := range matches {
					captures := make([]Value, len(match.Captures))
					for j, capture := range match.Captures {
						captures[j] = mapValueOf([]mapEntry{{key: stringValue("index"), value: intValue(bigIntFromInt64(int64(capture.Index)), 64)}, {key: stringValue("text"), value: stringValue(capture.Text)}, {key: stringValue("start_byte"), value: intValue(bigIntFromInt64(int64(capture.StartByte)), 64)}, {key: stringValue("end_byte"), value: intValue(bigIntFromInt64(int64(capture.EndByte)), 64)}})
					}
					out[i] = mapValueOf([]mapEntry{{key: stringValue("text"), value: stringValue(match.Text)}, {key: stringValue("start_byte"), value: intValue(bigIntFromInt64(int64(match.StartByte)), 64)}, {key: stringValue("end_byte"), value: intValue(bigIntFromInt64(int64(match.EndByte)), 64)}, {key: stringValue("captures"), value: listValue(captures)}})
				}
				return listValue(out), nil
			})
		}),
		"replace": contextFunctionValueOf("std/regex.replace", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 3 {
				return nullValue(), fmt.Errorf("regex.replace expects pattern, input, replacement")
			}
			return task120RegexValue(ctx, args[:2], func(ctx context.Context, re *Task120CompiledRegex, rest []Value) (Value, error) {
				input, err := taskStringArg(rest[0], "regex input")
				if err != nil {
					return nullValue(), err
				}
				replacement, err := taskStringArg(args[2], "regex replacement")
				if err != nil {
					return nullValue(), err
				}
				result, _, err := re.Replace(ctx, input, replacement)
				return stringValue(result), err
			})
		}),
		"split": contextFunctionValueOf("std/regex.split", func(ctx context.Context, args []Value) (Value, error) {
			return task120RegexValue(ctx, args, func(ctx context.Context, re *Task120CompiledRegex, rest []Value) (Value, error) {
				input, err := taskStringArg(rest[0], "regex input")
				if err != nil {
					return nullValue(), err
				}
				parts, err := re.Split(ctx, input)
				if err != nil {
					return nullValue(), err
				}
				out := make([]Value, len(parts))
				for i, part := range parts {
					out[i] = stringValue(part)
				}
				return listValue(out), nil
			})
		}),
	}
	// `match` is retained for API parity; `is_match` is the grammar-friendly
	// spelling for source files where match is parsed as a language keyword.
	mod["is_match"] = mod["match"]
	return mod
}

func task120CompressionLimitsValue(_ Value) Task120CompressionLimits {
	return Task120CompressionLimits{}
}

func task120CompressionModule() map[string]Value {
	compress := func(ctx context.Context, args []Value, decompress bool) (Value, error) {
		if len(args) != 2 {
			return nullValue(), fmt.Errorf("compression operation expects data and kind")
		}
		data, err := taskBytesArg(args[0], "compression data")
		if err != nil {
			return nullValue(), err
		}
		kindText, err := taskStringArg(args[1], "compression kind")
		if err != nil {
			return nullValue(), err
		}
		kind := Task120CompressionKind(kindText)
		if kind != Task120CompressionGZIP && kind != Task120CompressionZlib {
			return nullValue(), fmt.Errorf("unsupported compression kind %q", kindText)
		}
		var output bytes.Buffer
		if decompress {
			_, err = Task120Decompress(ctx, kind, bytes.NewReader(data), &output, task120CompressionLimitsValue(Value{}))
		} else {
			_, err = Task120Compress(ctx, kind, bytes.NewReader(data), &output, task120CompressionLimitsValue(Value{}))
		}
		if err != nil {
			return nullValue(), err
		}
		return bytesListValue(output.Bytes()), nil
	}
	codec := func(kind Task120CompressionKind, decompress bool) contextCallable {
		return func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("compression shorthand expects one byte-compatible value")
			}
			return compress(ctx, []Value{args[0], stringValue(string(kind))}, decompress)
		}
	}
	return map[string]Value{
		"version":    stringValue(task120Version),
		"gzip":       contextFunctionValueOf("std/compress.gzip", codec(Task120CompressionGZIP, false)),
		"gunzip":     contextFunctionValueOf("std/compress.gunzip", codec(Task120CompressionGZIP, true)),
		"zlib":       contextFunctionValueOf("std/compress.zlib", codec(Task120CompressionZlib, false)),
		"unzlib":     contextFunctionValueOf("std/compress.unzlib", codec(Task120CompressionZlib, true)),
		"compress":   contextFunctionValueOf("std/compress.compress", func(ctx context.Context, args []Value) (Value, error) { return compress(ctx, args, false) }),
		"decompress": contextFunctionValueOf("std/compress.decompress", func(ctx context.Context, args []Value) (Value, error) { return compress(ctx, args, true) }),
	}
}

func task120ConfigToValue(config Task120ConfigValue) Value {
	switch config.Kind {
	case Task120ConfigNull:
		return nullValue()
	case Task120ConfigBool:
		return boolValue(config.Bool)
	case Task120ConfigNumber:
		if strings.ContainsAny(config.Number, ".eE") {
			if n, err := strconv.ParseFloat(config.Number, 64); err == nil {
				return numberValue(n)
			}
		}
		if n, ok := new(big.Int).SetString(config.Number, 10); ok {
			return intValue(n, 64)
		}
		if n, err := strconv.ParseFloat(config.Number, 64); err == nil {
			return numberValue(n)
		}
		return stringValue(config.Number)
	case Task120ConfigString:
		return stringValue(config.String)
	case Task120ConfigArray:
		items := make([]Value, len(config.Array))
		for i, item := range config.Array {
			items[i] = task120ConfigToValue(item)
		}
		return listValue(items)
	case Task120ConfigObject:
		entries := make([]mapEntry, len(config.Object))
		for i, field := range config.Object {
			entries[i] = mapEntry{key: stringValue(field.Key), value: task120ConfigToValue(field.Value)}
		}
		return mapValueOf(entries)
	default:
		return nullValue()
	}
}

func task120ValueToConfig(value Value) (Task120ConfigValue, error) {
	switch value.kind {
	case kindNull:
		return Task120ConfigValue{Kind: Task120ConfigNull}, nil
	case kindBool:
		return Task120ConfigValue{Kind: Task120ConfigBool, Bool: value.boo}, nil
	case kindNumber, kindInt, kindUint:
		return Task120ConfigValue{Kind: Task120ConfigNumber, Number: value.String()}, nil
	case kindString:
		return Task120ConfigValue{Kind: Task120ConfigString, String: value.str}, nil
	case kindList:
		items := make([]Task120ConfigValue, len(value.list))
		for i, item := range value.list {
			converted, err := task120ValueToConfig(item)
			if err != nil {
				return Task120ConfigValue{}, err
			}
			items[i] = converted
		}
		return Task120ConfigValue{Kind: Task120ConfigArray, Array: items}, nil
	case kindMap:
		fields := make([]Task120ConfigField, len(value.mapValue.entries))
		for i, entry := range value.mapValue.entries {
			key, err := taskStringArg(entry.key, "config key")
			if err != nil {
				return Task120ConfigValue{}, err
			}
			converted, err := task120ValueToConfig(entry.value)
			if err != nil {
				return Task120ConfigValue{}, err
			}
			fields[i] = Task120ConfigField{Key: key, Value: converted}
		}
		return Task120ConfigValue{Kind: Task120ConfigObject, Object: fields}, nil
	default:
		return Task120ConfigValue{}, fmt.Errorf("unsupported configuration value %s", value.Kind())
	}
}

func task120ConfigModule() map[string]Value {
	var schemaFromValue func(Value) (Task120ConfigSchema, error)
	schemaFromValue = func(value Value) (Task120ConfigSchema, error) {
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
		for name, target := range map[string]*int{"min_items": &schema.MinItems, "max_items": &schema.MaxItems, "min_properties": &schema.MinProperties, "max_properties": &schema.MaxProperties, "min_length": &schema.MinLength, "max_length": &schema.MaxLength} {
			if raw, ok := taskMapValue(value, name); ok {
				number, err := raw.Int()
				if err != nil {
					return schema, fmt.Errorf("schema.%s: %w", name, err)
				}
				*target = number
			}
		}
		if raw, ok := taskMapValue(value, "items"); ok {
			child, err := schemaFromValue(raw)
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
				child, err := schemaFromValue(entry.value)
				if err != nil {
					return schema, err
				}
				schema.Properties[name] = child
			}
		}
		return schema, nil
	}
	parse := func(ctx context.Context, args []Value, properties bool) (Value, error) {
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("config parser expects one byte-compatible value")
		}
		data, err := taskBytesArg(args[0], "config data")
		if err != nil {
			return nullValue(), err
		}
		var parsed Task120ConfigValue
		if properties {
			parsed, err = Task120ParsePropertiesConfig(ctx, data, Task120ConfigLimits{})
		} else {
			parsed, err = Task120ParseJSONConfig(ctx, data, Task120ConfigLimits{})
		}
		if err != nil {
			return nullValue(), err
		}
		return task120ConfigToValue(parsed), nil
	}
	return map[string]Value{
		"version":          stringValue(task120Version),
		"parse_json":       contextFunctionValueOf("std/config.parse_json", func(ctx context.Context, args []Value) (Value, error) { return parse(ctx, args, false) }),
		"parse_properties": contextFunctionValueOf("std/config.parse_properties", func(ctx context.Context, args []Value) (Value, error) { return parse(ctx, args, true) }),
		"validate": functionValueOf("std/config.validate", func(args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("config.validate expects value and schema")
			}
			value, err := task120ValueToConfig(args[0])
			if err != nil {
				return nullValue(), err
			}
			schema, err := schemaFromValue(args[1])
			if err != nil {
				return nullValue(), err
			}
			return boolValue(Task120ValidateConfig(value, schema) == nil), nil
		}),
		"schema_property_names": functionValueOf("std/config.schema_property_names", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("config.schema_property_names expects schema")
			}
			schema, err := schemaFromValue(args[0])
			if err != nil {
				return nullValue(), err
			}
			names := Task120SchemaPropertyNames(schema)
			out := make([]Value, len(names))
			for i, name := range names {
				out[i] = stringValue(name)
			}
			return listValue(out), nil
		}),
	}
}

func task120I18nModule() map[string]Value {
	locale := func(ctx context.Context, args []Value) (Task120Locale, error) {
		if len(args) != 1 {
			return Task120Locale{}, fmt.Errorf("i18n operation expects locale")
		}
		tag, err := taskStringArg(args[0], "locale")
		if err != nil {
			return Task120Locale{}, err
		}
		return Task120LocaleFor(ctx, tag)
	}
	return map[string]Value{
		"data_version": stringValue(Task120LocaleDataVersion),
		"locale": contextFunctionValueOf("std/i18n.locale", func(ctx context.Context, args []Value) (Value, error) {
			l, err := locale(ctx, args)
			if err != nil {
				return nullValue(), err
			}
			return mapValueOf([]mapEntry{{key: stringValue("tag"), value: stringValue(l.Tag)}, {key: stringValue("data_version"), value: stringValue(l.DataVersion)}}), nil
		}),
		"format_number": contextFunctionValueOf("std/i18n.format_number", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("i18n.format_number expects locale and number")
			}
			l, err := locale(ctx, args[:1])
			if err != nil {
				return nullValue(), err
			}
			n, err := args[1].AsNumber()
			if err != nil {
				return nullValue(), err
			}
			result, err := Task120FormatNumber(ctx, l, n, Task120NumberOptions{UseGrouping: true, MaxFractionDigits: 2})
			return stringValue(result), err
		}),
		"format_date": contextFunctionValueOf("std/i18n.format_date", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("i18n.format_date expects locale and RFC3339 date")
			}
			l, err := locale(ctx, args[:1])
			if err != nil {
				return nullValue(), err
			}
			text, err := taskStringArg(args[1], "date")
			if err != nil {
				return nullValue(), err
			}
			stamp, err := time.Parse(time.RFC3339Nano, text)
			if err != nil {
				return nullValue(), err
			}
			result, err := Task120FormatDate(ctx, l, stamp, Task120DateOptions{})
			return stringValue(result), err
		}),
		"format_message": contextFunctionValueOf("std/i18n.format_message", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) < 3 {
				return nullValue(), fmt.Errorf("i18n.format_message expects locale, id, and count")
			}
			l, err := locale(ctx, args[:1])
			if err != nil {
				return nullValue(), err
			}
			id, err := taskStringArg(args[1], "message id")
			if err != nil {
				return nullValue(), err
			}
			count, err := args[2].Int()
			if err != nil {
				return nullValue(), err
			}
			result, err := Task120FormatMessage(ctx, l, id, int64(count), map[string]string{})
			return stringValue(result), err
		}),
	}
}

func task120ArchiveModule() map[string]Value {
	return map[string]Value{
		"version": stringValue(Task120ArchiveVersion),
		"archive_create": contextFunctionValueOf("std/compress.archive_create", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("archive_create expects entry list")
			}
			items, err := args[0].AsList()
			if err != nil {
				return nullValue(), err
			}
			entries := make([]Task120ArchiveEntry, len(items))
			for i, item := range items {
				name, ok := taskMapValue(item, "name")
				if !ok {
					return nullValue(), fmt.Errorf("archive entry %d has no name", i)
				}
				entries[i].Name, err = taskStringArg(name, "archive name")
				if err != nil {
					return nullValue(), err
				}
				typ, ok := taskMapValue(item, "type")
				if !ok {
					return nullValue(), fmt.Errorf("archive entry %d has no type", i)
				}
				entries[i].Type, err = taskStringArg(typ, "archive type")
				if err != nil {
					return nullValue(), err
				}
				if data, ok := taskMapValue(item, "data"); ok {
					entries[i].Data, err = taskBytesArg(data, "archive data")
					if err != nil {
						return nullValue(), err
					}
				}
			}
			var output bytes.Buffer
			if err := Task120CreateArchive(ctx, entries, &output, Task120ArchiveLimits{}); err != nil {
				return nullValue(), err
			}
			return bytesListValue(output.Bytes()), nil
		}),
		"archive_extract": contextFunctionValueOf("std/compress.archive_extract", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("archive_extract expects archive bytes")
			}
			data, err := taskBytesArg(args[0], "archive data")
			if err != nil {
				return nullValue(), err
			}
			var entries []Value
			err = Task120ExtractArchive(ctx, bytes.NewReader(data), Task120ArchiveLimits{}, func(entry Task120ArchiveEntry) error {
				entries = append(entries, mapValueOf([]mapEntry{{key: stringValue("name"), value: stringValue(entry.Name)}, {key: stringValue("type"), value: stringValue(entry.Type)}, {key: stringValue("data"), value: bytesListValue(entry.Data)}}))
				return nil
			})
			if err != nil {
				return nullValue(), err
			}
			return listValue(entries), nil
		}),
	}
}
