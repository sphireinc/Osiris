package osiris

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"
)

func task137ListTestValue(value int64) Value {
	return intValue(big.NewInt(value), 64)
}

func task137ListTestValues(values ...int64) Value {
	items := make([]Value, len(values))
	for i, value := range values {
		items[i] = task137ListTestValue(value)
	}
	return listValue(items)
}

func task137ListCall(t *testing.T, name string, args ...Value) Value {
	t.Helper()
	fn, ok := task137ListModule()[name]
	if !ok || fn.fn == nil || fn.fn.builtin == nil {
		t.Fatalf("std/list.%s is not a plain builtin", name)
	}
	value, err := fn.fn.builtin(args)
	if err != nil {
		t.Fatalf("std/list.%s: %v", name, err)
	}
	return value
}

func TestTask137ListModuleSurface(t *testing.T) {
	module := task137ListModule()
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
	if got, _ := module["version"].AsString(); got != Task137ListVersion {
		t.Fatalf("version = %q, want %q", got, Task137ListVersion)
	}
}

func TestTask137ListDeterministicOperations(t *testing.T) {
	input := task137ListTestValues(3, 1, 3, 2)
	if got := task137ListCall(t, "sort", input); !got.Equal(task137ListTestValues(1, 2, 3, 3)) {
		t.Fatalf("sort = %s", got)
	}
	if got := task137ListCall(t, "reverse", input); !got.Equal(task137ListTestValues(2, 3, 1, 3)) {
		t.Fatalf("reverse = %s", got)
	}
	if got := task137ListCall(t, "unique", input); !got.Equal(task137ListTestValues(3, 1, 2)) {
		t.Fatalf("unique = %s", got)
	}
	if got := task137ListCall(t, "index_of", input, task137ListTestValue(3)); task137ListTestInt(t, got) != 0 {
		t.Fatalf("index_of = %s", got)
	}
	if got := task137ListCall(t, "last_index_of", input, task137ListTestValue(3)); task137ListTestInt(t, got) != 2 {
		t.Fatalf("last_index_of = %s", got)
	}
	if got := task137ListCall(t, "count", input, task137ListTestValue(3)); task137ListTestInt(t, got) != 2 {
		t.Fatalf("count = %s", got)
	}
	if got := task137ListCall(t, "slice", input, task137ListTestValue(1), task137ListTestValue(3)); !got.Equal(task137ListTestValues(1, 3)) {
		t.Fatalf("slice = %s", got)
	}
	if got := task137ListCall(t, "concat", task137ListTestValues(1, 2), task137ListTestValues(3)); !got.Equal(task137ListTestValues(1, 2, 3)) {
		t.Fatalf("concat = %s", got)
	}
	if got := task137ListCall(t, "repeat", task137ListTestValue(7), task137ListTestValue(3)); !got.Equal(task137ListTestValues(7, 7, 7)) {
		t.Fatalf("repeat = %s", got)
	}
}

func TestTask137ListShapeOperations(t *testing.T) {
	input := task137ListTestValues(1, 2, 3, 4, 5)
	windows := task137ListCall(t, "windows", input, task137ListTestValue(3), task137ListTestValue(2))
	wantWindows := listValue([]Value{
		task137ListTestValues(1, 2, 3),
		task137ListTestValues(3, 4, 5),
	})
	if !windows.Equal(wantWindows) {
		t.Fatalf("windows = %s, want %s", windows, wantWindows)
	}
	chunks := task137ListCall(t, "chunks", input, task137ListTestValue(2))
	wantChunks := listValue([]Value{task137ListTestValues(1, 2), task137ListTestValues(3, 4), task137ListTestValues(5)})
	if !chunks.Equal(wantChunks) {
		t.Fatalf("chunks = %s, want %s", chunks, wantChunks)
	}
	partition := task137ListCall(t, "partition", input, task137ListTestValue(2))
	if !partition.Equal(tupleValue([]Value{task137ListTestValues(1, 2), task137ListTestValues(3, 4, 5)})) {
		t.Fatalf("partition = %s", partition)
	}
	zip := task137ListCall(t, "zip", task137ListTestValues(1, 2, 3), task137ListTestValues(4, 5))
	wantZip := listValue([]Value{tupleValue([]Value{task137ListTestValue(1), task137ListTestValue(4)}), tupleValue([]Value{task137ListTestValue(2), task137ListTestValue(5)})})
	if !zip.Equal(wantZip) {
		t.Fatalf("zip = %s, want %s", zip, wantZip)
	}
	flat := task137ListCall(t, "flatten", listValue([]Value{task137ListTestValues(1, 2), listValue([]Value{task137ListTestValue(3)})}), task137ListTestValue(2))
	if !flat.Equal(task137ListTestValues(1, 2, 3)) {
		t.Fatalf("flatten = %s", flat)
	}
	all := task137ListCall(t, "all", listValue([]Value{boolValue(true), boolValue(true)}))
	any := task137ListCall(t, "any", listValue([]Value{boolValue(false), boolValue(true)}))
	none := task137ListCall(t, "none", listValue([]Value{boolValue(false), boolValue(false)}))
	if !all.Truthy() || !any.Truthy() || !none.Truthy() {
		t.Fatalf("boolean predicates = %s, %s, %s", all, any, none)
	}
}

func TestTask137ListCallbacksLimitsAndCancellation(t *testing.T) {
	items := []Value{task137ListTestValue(1), task137ListTestValue(2), task137ListTestValue(3)}
	mapped, err := Task137ListMap(context.Background(), items, func(value Value) (Value, error) {
		n, _ := value.Int()
		return task137ListTestValue(int64(n * 2)), nil
	}, Task137ListLimits{MaxItems: 3})
	if err != nil || !listValue(mapped).Equal(task137ListTestValues(2, 4, 6)) {
		t.Fatalf("map = %s, %v", listValue(mapped), err)
	}
	filtered, err := Task137ListFilter(context.Background(), items, func(value Value) (Value, error) {
		n, _ := value.Int()
		return boolValue(n%2 == 1), nil
	}, Task137ListLimits{})
	if err != nil || !listValue(filtered).Equal(task137ListTestValues(1, 3)) {
		t.Fatalf("filter = %s, %v", listValue(filtered), err)
	}
	folded, err := Task137ListFold(context.Background(), items, task137ListTestValue(0), func(acc, value Value) (Value, error) {
		a, _ := acc.Int()
		b, _ := value.Int()
		return task137ListTestValue(int64(a + b)), nil
	}, Task137ListLimits{})
	if err != nil || task137ListTestInt(t, folded) != 6 {
		t.Fatalf("fold = %s, %v", folded, err)
	}
	if _, err := Task137ListRepeat(task137ListTestValue(1), 4, Task137ListLimits{MaxItems: 3}); err == nil {
		t.Fatal("repeat exceeded limit without error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Task137ListMap(ctx, items, func(value Value) (Value, error) { return value, nil }, Task137ListLimits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled map error = %v", err)
	}
	if _, err := Task137ListFlatten(context.Background(), []Value{listValue([]Value{stringValue(strings.Repeat("x", 1))})}, Task137ListMaxDepth+1, Task137ListLimits{}); err == nil {
		t.Fatal("flatten exceeded depth without error")
	}
}

func task137ListTestInt(t *testing.T, value Value) int {
	t.Helper()
	result, err := value.Int()
	if err != nil {
		t.Fatalf("%s is not an integer: %v", value, err)
	}
	return result
}
