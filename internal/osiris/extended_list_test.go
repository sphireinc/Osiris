package osiris

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"
)

func extendedListTestValue(value int64) Value {
	return intValue(big.NewInt(value), 64)
}

func extendedListTestValues(values ...int64) Value {
	items := make([]Value, len(values))
	for i, value := range values {
		items[i] = extendedListTestValue(value)
	}
	return listValue(items)
}

func extendedListCall(t *testing.T, name string, args ...Value) Value {
	t.Helper()
	fn, ok := extendedListModule()[name]
	if !ok || fn.fn == nil || fn.fn.builtin == nil {
		t.Fatalf("std/list.%s is not a plain builtin", name)
	}
	value, err := fn.fn.builtin(args)
	if err != nil {
		t.Fatalf("std/list.%s: %v", name, err)
	}
	return value
}

func TestExtendedListModuleSurface(t *testing.T) {
	module := extendedListModule()
	want := []string{
		"version", "max_items", "max_depth", "empty", "repeat", "concat", "slice", "at",
		"index_of", "last_index_of", "contains", "count", "reverse", "sort", "unique",
		"partition", "zip", "windows", "chunks", "flatten", "all", "any", "none", "enumerate",
	}
	for _, name := range want {
		if _, ok := module[name]; !ok {
			t.Errorf("missing staged std/list symbol %q", name)
		}
	}
	if got, _ := module["version"].AsString(); got != ExtendedListVersion {
		t.Fatalf("version = %q, want %q", got, ExtendedListVersion)
	}
}

func TestExtendedListDeterministicOperations(t *testing.T) {
	input := extendedListTestValues(3, 1, 3, 2)
	if got := extendedListCall(t, "sort", input); !got.Equal(extendedListTestValues(1, 2, 3, 3)) {
		t.Fatalf("sort = %s", got)
	}
	if got := extendedListCall(t, "reverse", input); !got.Equal(extendedListTestValues(2, 3, 1, 3)) {
		t.Fatalf("reverse = %s", got)
	}
	if got := extendedListCall(t, "unique", input); !got.Equal(extendedListTestValues(3, 1, 2)) {
		t.Fatalf("unique = %s", got)
	}
	if got := extendedListCall(t, "index_of", input, extendedListTestValue(3)); extendedListTestInt(t, got) != 0 {
		t.Fatalf("index_of = %s", got)
	}
	if got := extendedListCall(t, "last_index_of", input, extendedListTestValue(3)); extendedListTestInt(t, got) != 2 {
		t.Fatalf("last_index_of = %s", got)
	}
	if got := extendedListCall(t, "count", input, extendedListTestValue(3)); extendedListTestInt(t, got) != 2 {
		t.Fatalf("count = %s", got)
	}
	if got := extendedListCall(t, "slice", input, extendedListTestValue(1), extendedListTestValue(3)); !got.Equal(extendedListTestValues(1, 3)) {
		t.Fatalf("slice = %s", got)
	}
	if got := extendedListCall(t, "concat", extendedListTestValues(1, 2), extendedListTestValues(3)); !got.Equal(extendedListTestValues(1, 2, 3)) {
		t.Fatalf("concat = %s", got)
	}
	if got := extendedListCall(t, "repeat", extendedListTestValue(7), extendedListTestValue(3)); !got.Equal(extendedListTestValues(7, 7, 7)) {
		t.Fatalf("repeat = %s", got)
	}
}

func TestExtendedListShapeOperations(t *testing.T) {
	input := extendedListTestValues(1, 2, 3, 4, 5)
	windows := extendedListCall(t, "windows", input, extendedListTestValue(3), extendedListTestValue(2))
	wantWindows := listValue([]Value{
		extendedListTestValues(1, 2, 3),
		extendedListTestValues(3, 4, 5),
	})
	if !windows.Equal(wantWindows) {
		t.Fatalf("windows = %s, want %s", windows, wantWindows)
	}
	chunks := extendedListCall(t, "chunks", input, extendedListTestValue(2))
	wantChunks := listValue([]Value{extendedListTestValues(1, 2), extendedListTestValues(3, 4), extendedListTestValues(5)})
	if !chunks.Equal(wantChunks) {
		t.Fatalf("chunks = %s, want %s", chunks, wantChunks)
	}
	partition := extendedListCall(t, "partition", input, extendedListTestValue(2))
	if !partition.Equal(tupleValue([]Value{extendedListTestValues(1, 2), extendedListTestValues(3, 4, 5)})) {
		t.Fatalf("partition = %s", partition)
	}
	zip := extendedListCall(t, "zip", extendedListTestValues(1, 2, 3), extendedListTestValues(4, 5))
	wantZip := listValue([]Value{tupleValue([]Value{extendedListTestValue(1), extendedListTestValue(4)}), tupleValue([]Value{extendedListTestValue(2), extendedListTestValue(5)})})
	if !zip.Equal(wantZip) {
		t.Fatalf("zip = %s, want %s", zip, wantZip)
	}
	flat := extendedListCall(t, "flatten", listValue([]Value{extendedListTestValues(1, 2), listValue([]Value{extendedListTestValue(3)})}), extendedListTestValue(2))
	if !flat.Equal(extendedListTestValues(1, 2, 3)) {
		t.Fatalf("flatten = %s", flat)
	}
	all := extendedListCall(t, "all", listValue([]Value{boolValue(true), boolValue(true)}))
	any := extendedListCall(t, "any", listValue([]Value{boolValue(false), boolValue(true)}))
	none := extendedListCall(t, "none", listValue([]Value{boolValue(false), boolValue(false)}))
	if !all.Truthy() || !any.Truthy() || !none.Truthy() {
		t.Fatalf("boolean predicates = %s, %s, %s", all, any, none)
	}
}

func TestExtendedListCallbacksLimitsAndCancellation(t *testing.T) {
	items := []Value{extendedListTestValue(1), extendedListTestValue(2), extendedListTestValue(3)}
	mapped, err := ExtendedListMap(context.Background(), items, func(value Value) (Value, error) {
		n, _ := value.Int()
		return extendedListTestValue(int64(n * 2)), nil
	}, ExtendedListLimits{MaxItems: 3})
	if err != nil || !listValue(mapped).Equal(extendedListTestValues(2, 4, 6)) {
		t.Fatalf("map = %s, %v", listValue(mapped), err)
	}
	filtered, err := ExtendedListFilter(context.Background(), items, func(value Value) (Value, error) {
		n, _ := value.Int()
		return boolValue(n%2 == 1), nil
	}, ExtendedListLimits{})
	if err != nil || !listValue(filtered).Equal(extendedListTestValues(1, 3)) {
		t.Fatalf("filter = %s, %v", listValue(filtered), err)
	}
	folded, err := ExtendedListFold(context.Background(), items, extendedListTestValue(0), func(acc, value Value) (Value, error) {
		a, _ := acc.Int()
		b, _ := value.Int()
		return extendedListTestValue(int64(a + b)), nil
	}, ExtendedListLimits{})
	if err != nil || extendedListTestInt(t, folded) != 6 {
		t.Fatalf("fold = %s, %v", folded, err)
	}
	if _, err := ExtendedListRepeat(extendedListTestValue(1), 4, ExtendedListLimits{MaxItems: 3}); err == nil {
		t.Fatal("repeat exceeded limit without error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ExtendedListMap(ctx, items, func(value Value) (Value, error) { return value, nil }, ExtendedListLimits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled map error = %v", err)
	}
	if _, err := ExtendedListFlatten(context.Background(), []Value{listValue([]Value{stringValue(strings.Repeat("x", 1))})}, ExtendedListMaxDepth+1, ExtendedListLimits{}); err == nil {
		t.Fatal("flatten exceeded depth without error")
	}
}

func extendedListTestInt(t *testing.T, value Value) int {
	t.Helper()
	result, err := value.Int()
	if err != nil {
		t.Fatalf("%s is not an integer: %v", value, err)
	}
	return result
}
