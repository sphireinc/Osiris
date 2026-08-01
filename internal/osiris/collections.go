package osiris

import "fmt"

func mapKeyAllowed(v Value) bool {
	switch v.kind {
	case kindNull, kindBool, kindNumber, kindDecimal, kindFixed, kindInterval, kindModInt,
		kindComplex, kindBigInt, kindBigFloat, kindInt, kindQuantity, kindUint, kindString,
		kindEnum:
		return true
	default:
		return false
	}
}

func mapFind(entries []mapEntry, key Value) int {
	for i := range entries {
		if entries[i].key.Equal(key) {
			return i
		}
	}
	return -1
}

func mapNewValue(args []Value) (Value, error) {
	if len(args)%2 != 0 {
		return nullValue(), fmt.Errorf("map_new expects key/value pairs")
	}
	entries := make([]mapEntry, 0, len(args)/2)
	for i := 0; i < len(args); i += 2 {
		key := args[i]
		if !mapKeyAllowed(key) {
			return nullValue(), fmt.Errorf("map key %s is not hashable", key.Kind())
		}
		entry := mapEntry{key: key, value: args[i+1]}
		if at := mapFind(entries, key); at >= 0 {
			entries[at].value = entry.value
			continue
		}
		entries = append(entries, entry)
	}
	return mapValueOf(entries), nil
}

func mapSetValue(args []Value) (Value, error) {
	if len(args) != 3 || args[0].kind != kindMap || args[0].mapValue == nil {
		return nullValue(), fmt.Errorf("map_set expects map, key, and value")
	}
	if !mapKeyAllowed(args[1]) {
		return nullValue(), fmt.Errorf("map key %s is not hashable", args[1].Kind())
	}
	entries := append([]mapEntry(nil), args[0].mapValue.entries...)
	entry := mapEntry{key: args[1], value: args[2]}
	if at := mapFind(entries, args[1]); at >= 0 {
		entries[at] = entry
	} else {
		entries = append(entries, entry)
	}
	return mapValueOf(entries), nil
}

func mapGetValue(args []Value) (Value, error) {
	if len(args) != 2 || args[0].kind != kindMap || args[0].mapValue == nil {
		return nullValue(), fmt.Errorf("map_get expects map and key")
	}
	if at := mapFind(args[0].mapValue.entries, args[1]); at >= 0 {
		return optionSomeValue(args[0].mapValue.entries[at].value), nil
	}
	return optionNoneValue(), nil
}

func mapKeysValue(args []Value) (Value, error) {
	if len(args) != 1 || args[0].kind != kindMap || args[0].mapValue == nil {
		return nullValue(), fmt.Errorf("map_keys expects a map")
	}
	keys := make([]Value, len(args[0].mapValue.entries))
	for i := range args[0].mapValue.entries {
		keys[i] = args[0].mapValue.entries[i].key
	}
	return listValue(keys), nil
}

func mapValuesValue(args []Value) (Value, error) {
	if len(args) != 1 || args[0].kind != kindMap || args[0].mapValue == nil {
		return nullValue(), fmt.Errorf("map_values expects a map")
	}
	values := make([]Value, len(args[0].mapValue.entries))
	for i := range args[0].mapValue.entries {
		values[i] = args[0].mapValue.entries[i].value
	}
	return listValue(values), nil
}

func setNewValue(args []Value) (Value, error) {
	items := make([]Value, 0, len(args))
	for _, item := range args {
		if !mapKeyAllowed(item) {
			return nullValue(), fmt.Errorf("set value %s is not hashable", item.Kind())
		}
		if !containsValue(items, item) {
			items = append(items, item)
		}
	}
	return setValueOf(items), nil
}

func setAddValue(args []Value) (Value, error) {
	if len(args) != 2 || args[0].kind != kindSet || args[0].setValue == nil {
		return nullValue(), fmt.Errorf("set_add expects set and value")
	}
	if !mapKeyAllowed(args[1]) {
		return nullValue(), fmt.Errorf("set value %s is not hashable", args[1].Kind())
	}
	items := append([]Value(nil), args[0].setValue.items...)
	if !containsValue(items, args[1]) {
		items = append(items, args[1])
	}
	return setValueOf(items), nil
}

func setHasValue(args []Value) (Value, error) {
	if len(args) != 2 || args[0].kind != kindSet || args[0].setValue == nil {
		return nullValue(), fmt.Errorf("set_has expects set and value")
	}
	return boolValue(containsValue(args[0].setValue.items, args[1])), nil
}

func setValuesValue(args []Value) (Value, error) {
	if len(args) != 1 || args[0].kind != kindSet || args[0].setValue == nil {
		return nullValue(), fmt.Errorf("set_values expects a set")
	}
	return listValue(args[0].setValue.items), nil
}

func containsValue(items []Value, target Value) bool {
	for _, item := range items {
		if item.Equal(target) {
			return true
		}
	}
	return false
}
