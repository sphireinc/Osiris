package osiris

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// readValue performs the read value step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func readValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("read expects 1 argument")
	}
	path, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nullValue(), err
	}
	return stringValue(string(data)), nil
}

// readBytesValue performs the read bytes value step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func readBytesValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("readbytes expects 1 argument")
	}
	path, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nullValue(), err
	}
	out := make([]Value, len(data))
	for i, b := range data {
		out[i] = uintValue(uint64(b), 8)
	}
	return listValue(out), nil
}

// writeValue performs the write value step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func writeValue(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("write expects 2 arguments")
	}
	path, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	data, err := bytesFromValue(args[1])
	if err != nil {
		return nullValue(), err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nullValue(), err
	}
	return boolValue(true), nil
}

// linesValue performs the lines value step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func linesValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("lines expects 1 argument")
	}
	data, err := bytesFromValue(args[0])
	if err != nil {
		return nullValue(), err
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return listValue(nil), nil
	}
	parts := strings.Split(text, "\n")
	out := make([]Value, len(parts))
	for i := range parts {
		out[i] = stringValue(parts[i])
	}
	return listValue(out), nil
}

// csvValue performs the csv value step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func csvValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("csv expects 1 argument")
	}
	data, err := bytesFromValue(args[0])
	if err != nil {
		return nullValue(), err
	}
	reader := csv.NewReader(bytes.NewReader(data))
	rows, err := reader.ReadAll()
	if err != nil {
		return nullValue(), err
	}
	out := make([]Value, len(rows))
	for i, row := range rows {
		cells := make([]Value, len(row))
		for j, cell := range row {
			cells[j] = stringValue(cell)
		}
		out[i] = listValue(cells)
	}
	return listValue(out), nil
}

// jsonParseValue performs the json parse value step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func jsonParseValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("jsonparse expects 1 argument")
	}
	data, err := bytesFromValue(args[0])
	if err != nil {
		return nullValue(), err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return nullValue(), err
	}
	return jsonToValue(raw)
}

// jsonToValue performs the json to value step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func jsonToValue(raw any) (Value, error) {
	switch v := raw.(type) {
	case nil:
		return nullValue(), nil
	case bool:
		return boolValue(v), nil
	case string:
		return stringValue(v), nil
	case json.Number:
		return jsonNumberValue(v)
	case float64:
		return numberValue(v), nil
	case []any:
		items := make([]Value, len(v))
		for i := range v {
			item, err := jsonToValue(v[i])
			if err != nil {
				return nullValue(), err
			}
			items[i] = item
		}
		return listValue(items), nil
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		rows := make([]Value, len(keys))
		for i, key := range keys {
			item, err := jsonToValue(v[key])
			if err != nil {
				return nullValue(), err
			}
			rows[i] = listValue([]Value{stringValue(key), item})
		}
		return listValue(rows), nil
	default:
		return nullValue(), fmt.Errorf("unsupported JSON value %T", raw)
	}
}

// jsonNumberValue performs the json number value step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func jsonNumberValue(n json.Number) (Value, error) {
	text := n.String()
	if strings.ContainsAny(text, ".eE") {
		f, err := n.Float64()
		if err != nil {
			return nullValue(), err
		}
		return numberValue(f), nil
	}
	u, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		f, ferr := n.Float64()
		if ferr != nil {
			return nullValue(), err
		}
		return numberValue(f), nil
	}
	return uintValue(u, 64), nil
}
