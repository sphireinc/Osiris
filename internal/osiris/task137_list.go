package osiris

import (
	"context"
	"fmt"
	"math/big"
)

// Task137ListVersion identifies the additive list API staged by task 137.
const Task137ListVersion = "1.1.0"

const (
	// Task137ListMaxItems is the hard upper bound for any list operation's
	// input or output. A caller may select a smaller limit, but cannot raise it.
	Task137ListMaxItems = 100_000
	// Task137ListMaxDepth bounds recursive flattening work.
	Task137ListMaxDepth = 64
)

// Task137ListLimits bounds allocations and recursive traversal. Zero values
// select the safe defaults; values above the hard limits are clamped.
type Task137ListLimits struct {
	MaxItems int
	MaxDepth int
}

func (l Task137ListLimits) normalized() Task137ListLimits {
	if l.MaxItems <= 0 || l.MaxItems > Task137ListMaxItems {
		l.MaxItems = Task137ListMaxItems
	}
	if l.MaxDepth <= 0 || l.MaxDepth > Task137ListMaxDepth {
		l.MaxDepth = Task137ListMaxDepth
	}
	return l
}

func task137ListInput(value Value, limits Task137ListLimits, name string) ([]Value, error) {
	items, err := value.AsList()
	if err != nil {
		return nil, fmt.Errorf("std/list.%s: %w", name, err)
	}
	limits = limits.normalized()
	if len(items) > limits.MaxItems {
		return nil, task137ListLimitError(name, "items", len(items), limits.MaxItems)
	}
	return items, nil
}

func task137ListLimitError(operation, subject string, got, limit int) error {
	return fmt.Errorf("std/list.%s: %s %d exceeds limit %d", operation, subject, got, limit)
}

func task137ListIndex(value Value, name string) (int, error) {
	index, err := value.Int()
	if err != nil || index < 0 {
		return 0, fmt.Errorf("std/list: %s must be a non-negative integer", name)
	}
	return index, nil
}

func task137ListCheckContext(ctx context.Context, operation string) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("std/list.%s: %w", operation, ctx.Err())
	default:
		return nil
	}
}

// Task137ListConcat concatenates lists without mutating any input.
func Task137ListConcat(limits Task137ListLimits, lists ...[]Value) ([]Value, error) {
	limits = limits.normalized()
	total := 0
	for _, items := range lists {
		if len(items) > limits.MaxItems || total > limits.MaxItems-len(items) {
			return nil, task137ListLimitError("concat", "output items", total+len(items), limits.MaxItems)
		}
		total += len(items)
	}
	out := make([]Value, 0, total)
	for _, items := range lists {
		out = append(out, items...)
	}
	return out, nil
}

// Task137ListRepeat returns count copies of item, subject to MaxItems.
func Task137ListRepeat(item Value, count int, limits Task137ListLimits) ([]Value, error) {
	limits = limits.normalized()
	if count < 0 {
		return nil, fmt.Errorf("std/list.repeat: count must be non-negative")
	}
	if count > limits.MaxItems {
		return nil, task137ListLimitError("repeat", "count", count, limits.MaxItems)
	}
	out := make([]Value, count)
	for i := range out {
		out[i] = item
	}
	return out, nil
}

// Task137ListSlice returns the owned half-open range [start,end].
func Task137ListSlice(items []Value, start, end int, limits Task137ListLimits) ([]Value, error) {
	limits = limits.normalized()
	if len(items) > limits.MaxItems {
		return nil, task137ListLimitError("slice", "input items", len(items), limits.MaxItems)
	}
	if start < 0 || end < 0 || start > end {
		return nil, fmt.Errorf("std/list.slice: require 0 <= start <= end")
	}
	if start > len(items) {
		start = len(items)
	}
	if end > len(items) {
		end = len(items)
	}
	return append([]Value(nil), items[start:end]...), nil
}

// Task137ListAt returns an element and rejects out-of-range indexes.
func Task137ListAt(items []Value, index int, limits Task137ListLimits) (Value, error) {
	limits = limits.normalized()
	if len(items) > limits.MaxItems {
		return nullValue(), task137ListLimitError("at", "input items", len(items), limits.MaxItems)
	}
	if index < 0 || index >= len(items) {
		return nullValue(), fmt.Errorf("std/list.at: index %d out of range for length %d", index, len(items))
	}
	return items[index], nil
}

func Task137ListIndexOf(items []Value, target Value, last bool, limits Task137ListLimits) (int, error) {
	limits = limits.normalized()
	if len(items) > limits.MaxItems {
		return 0, task137ListLimitError("index_of", "input items", len(items), limits.MaxItems)
	}
	if last {
		for i := len(items) - 1; i >= 0; i-- {
			if items[i].Equal(target) {
				return i, nil
			}
		}
		return -1, nil
	}
	for i, item := range items {
		if item.Equal(target) {
			return i, nil
		}
	}
	return -1, nil
}

// Task137ListReverse returns a reversed owned copy.
func Task137ListReverse(items []Value, limits Task137ListLimits) ([]Value, error) {
	limits = limits.normalized()
	if len(items) > limits.MaxItems {
		return nil, task137ListLimitError("reverse", "input items", len(items), limits.MaxItems)
	}
	out := make([]Value, len(items))
	for i := range items {
		out[len(items)-1-i] = items[i]
	}
	return out, nil
}

// Task137ListSort performs a stable ascending sort using Value.Compare.
func Task137ListSort(items []Value, limits Task137ListLimits) ([]Value, error) {
	limits = limits.normalized()
	if len(items) > limits.MaxItems {
		return nil, task137ListLimitError("sort", "input items", len(items), limits.MaxItems)
	}
	out := append([]Value(nil), items...)
	// Insertion sort makes comparison errors observable while retaining stable
	// ordering and bounded allocation for the language value model.
	for i := 1; i < len(out); i++ {
		item := out[i]
		j := i
		for j > 0 {
			cmp, err := out[j-1].Compare(item)
			if err != nil {
				return nil, fmt.Errorf("std/list.sort: %w", err)
			}
			if cmp <= 0 {
				break
			}
			out[j] = out[j-1]
			j--
		}
		out[j] = item
	}
	return out, nil
}

// Task137ListUnique keeps the first occurrence of every value.
func Task137ListUnique(items []Value, limits Task137ListLimits) ([]Value, error) {
	limits = limits.normalized()
	if len(items) > limits.MaxItems {
		return nil, task137ListLimitError("unique", "input items", len(items), limits.MaxItems)
	}
	out := make([]Value, 0, len(items))
	for _, item := range items {
		seen := false
		for _, prior := range out {
			if prior.Equal(item) {
				seen = true
				break
			}
		}
		if !seen {
			out = append(out, item)
		}
	}
	return out, nil
}

// Task137ListPartition splits at index and returns (left,right).
func Task137ListPartition(items []Value, index int, limits Task137ListLimits) (Value, error) {
	left, err := Task137ListSlice(items, 0, index, limits)
	if err != nil {
		return nullValue(), fmt.Errorf("std/list.partition: %w", err)
	}
	right, err := Task137ListSlice(items, index, len(items), limits)
	if err != nil {
		return nullValue(), fmt.Errorf("std/list.partition: %w", err)
	}
	return tupleValue([]Value{listValue(left), listValue(right)}), nil
}

// Task137ListZip pairs elements through the shorter input.
func Task137ListZip(left, right []Value, limits Task137ListLimits) ([]Value, error) {
	limits = limits.normalized()
	if len(left) > limits.MaxItems || len(right) > limits.MaxItems {
		return nil, task137ListLimitError("zip", "input items", task137MaxInt(len(left), len(right)), limits.MaxItems)
	}
	n := len(left)
	if len(right) < n {
		n = len(right)
	}
	out := make([]Value, n)
	for i := range out {
		out[i] = tupleValue([]Value{left[i], right[i]})
	}
	return out, nil
}

// Task137ListWindows returns overlapping windows using a positive step.
func Task137ListWindows(items []Value, width, step int, limits Task137ListLimits) ([]Value, error) {
	limits = limits.normalized()
	if width <= 0 || step <= 0 || width > limits.MaxItems || step > limits.MaxItems {
		return nil, fmt.Errorf("std/list.windows: width and step must be positive and within the item limit")
	}
	if len(items) > limits.MaxItems {
		return nil, task137ListLimitError("windows", "input items", len(items), limits.MaxItems)
	}
	out := make([]Value, 0)
	for start := 0; start+width <= len(items); start += step {
		if len(out) == limits.MaxItems {
			return nil, task137ListLimitError("windows", "output windows", len(out)+1, limits.MaxItems)
		}
		out = append(out, listValue(append([]Value(nil), items[start:start+width]...)))
	}
	return out, nil
}

// Task137ListChunks returns non-overlapping windows of at most width items.
func Task137ListChunks(items []Value, width int, limits Task137ListLimits) ([]Value, error) {
	limits = limits.normalized()
	if width <= 0 || width > limits.MaxItems {
		return nil, fmt.Errorf("std/list.chunks: width must be positive and within the item limit")
	}
	if len(items) > limits.MaxItems {
		return nil, task137ListLimitError("chunks", "input items", len(items), limits.MaxItems)
	}
	out := make([]Value, 0, (len(items)+width-1)/width)
	for start := 0; start < len(items); start += width {
		if len(out) == limits.MaxItems {
			return nil, task137ListLimitError("chunks", "output chunks", len(out)+1, limits.MaxItems)
		}
		end := start + width
		if end > len(items) {
			end = len(items)
		}
		out = append(out, listValue(append([]Value(nil), items[start:end]...)))
	}
	return out, nil
}

func task137ListFlattenAppend(ctx context.Context, operation string, dst *[]Value, value Value, depth int, limits Task137ListLimits) error {
	if err := task137ListCheckContext(ctx, operation); err != nil {
		return err
	}
	if len(*dst) == limits.MaxItems {
		return task137ListLimitError(operation, "output items", len(*dst)+1, limits.MaxItems)
	}
	if value.kind != kindList || depth == 0 {
		*dst = append(*dst, value)
		return nil
	}
	if depth < 0 {
		return fmt.Errorf("std/list.flatten: depth must be non-negative")
	}
	if depth > limits.MaxDepth {
		return task137ListLimitError(operation, "depth", depth, limits.MaxDepth)
	}
	for _, item := range value.list {
		if err := task137ListFlattenAppend(ctx, operation, dst, item, depth-1, limits); err != nil {
			return err
		}
	}
	return nil
}

// Task137ListFlatten recursively flattens at most depth list layers.
func Task137ListFlatten(ctx context.Context, items []Value, depth int, limits Task137ListLimits) ([]Value, error) {
	limits = limits.normalized()
	if depth < 0 || depth > limits.MaxDepth {
		return nil, task137ListLimitError("flatten", "depth", depth, limits.MaxDepth)
	}
	if len(items) > limits.MaxItems {
		return nil, task137ListLimitError("flatten", "input items", len(items), limits.MaxItems)
	}
	out := make([]Value, 0, len(items))
	for _, item := range items {
		if err := task137ListFlattenAppend(ctx, "flatten", &out, item, depth, limits); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Task137ListMap applies a host callback in input order.
func Task137ListMap(ctx context.Context, items []Value, callback func(Value) (Value, error), limits Task137ListLimits) ([]Value, error) {
	limits = limits.normalized()
	if callback == nil {
		return nil, fmt.Errorf("std/list.map: callback is required")
	}
	if len(items) > limits.MaxItems {
		return nil, task137ListLimitError("map", "input items", len(items), limits.MaxItems)
	}
	out := make([]Value, len(items))
	for i, item := range items {
		if err := task137ListCheckContext(ctx, "map"); err != nil {
			return nil, err
		}
		mapped, err := callback(item)
		if err != nil {
			return nil, fmt.Errorf("std/list.map at index %d: %w", i, err)
		}
		out[i] = mapped
	}
	return out, nil
}

// Task137ListFilter keeps items for which the host callback returns a truthy value.
func Task137ListFilter(ctx context.Context, items []Value, callback func(Value) (Value, error), limits Task137ListLimits) ([]Value, error) {
	limits = limits.normalized()
	if callback == nil {
		return nil, fmt.Errorf("std/list.filter: callback is required")
	}
	if len(items) > limits.MaxItems {
		return nil, task137ListLimitError("filter", "input items", len(items), limits.MaxItems)
	}
	out := make([]Value, 0, len(items))
	for i, item := range items {
		if err := task137ListCheckContext(ctx, "filter"); err != nil {
			return nil, err
		}
		keep, err := callback(item)
		if err != nil {
			return nil, fmt.Errorf("std/list.filter at index %d: %w", i, err)
		}
		if keep.Truthy() {
			out = append(out, item)
		}
	}
	return out, nil
}

// Task137ListFold reduces items in input order.
func Task137ListFold(ctx context.Context, items []Value, initial Value, callback func(Value, Value) (Value, error), limits Task137ListLimits) (Value, error) {
	limits = limits.normalized()
	if callback == nil {
		return nullValue(), fmt.Errorf("std/list.fold: callback is required")
	}
	if len(items) > limits.MaxItems {
		return nullValue(), task137ListLimitError("fold", "input items", len(items), limits.MaxItems)
	}
	acc := initial
	for i, item := range items {
		if err := task137ListCheckContext(ctx, "fold"); err != nil {
			return nullValue(), err
		}
		var err error
		acc, err = callback(acc, item)
		if err != nil {
			return nullValue(), fmt.Errorf("std/list.fold at index %d: %w", i, err)
		}
	}
	return acc, nil
}

func task137ListBool(items []Value, mode string, limits Task137ListLimits) (Value, error) {
	limits = limits.normalized()
	if len(items) > limits.MaxItems {
		return nullValue(), task137ListLimitError(mode, "input items", len(items), limits.MaxItems)
	}
	result := mode == "all" || mode == "none"
	for _, item := range items {
		truthy := item.Truthy()
		switch mode {
		case "all":
			result = result && truthy
		case "any":
			result = result || truthy
		case "none":
			result = result && !truthy
		}
	}
	return boolValue(result), nil
}

func task137ListEnumerate(items []Value, limits Task137ListLimits) ([]Value, error) {
	limits = limits.normalized()
	if len(items) > limits.MaxItems {
		return nil, task137ListLimitError("enumerate", "input items", len(items), limits.MaxItems)
	}
	out := make([]Value, len(items))
	for i, item := range items {
		out[i] = tupleValue([]Value{uintValue(uint64(i), 64), item})
	}
	return out, nil
}

func task137ListModule() map[string]Value {
	limits := Task137ListLimits{}
	listArg := func(args []Value, index int, name string) ([]Value, error) {
		if index >= len(args) {
			return nil, fmt.Errorf("std/list.%s: missing list argument", name)
		}
		return task137ListInput(args[index], limits, name)
	}
	mod := map[string]Value{
		"version":   stringValue(Task137ListVersion),
		"max_items": uintValue(Task137ListMaxItems, 64),
		"max_depth": uintValue(Task137ListMaxDepth, 64),
		"empty": functionValueOf("std/list.empty", func(args []Value) (Value, error) {
			if len(args) != 0 {
				return nullValue(), fmt.Errorf("std/list.empty expects no arguments")
			}
			return listValue(nil), nil
		}),
		"repeat": functionValueOf("std/list.repeat", func(args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("std/list.repeat expects item and count")
			}
			count, err := args[1].Int()
			if err != nil {
				return nullValue(), err
			}
			items, err := Task137ListRepeat(args[0], count, limits)
			return listValue(items), err
		}),
		"concat": functionValueOf("std/list.concat", func(args []Value) (Value, error) {
			lists := make([][]Value, len(args))
			for i := range args {
				var err error
				lists[i], err = listArg(args, i, "concat")
				if err != nil {
					return nullValue(), err
				}
			}
			items, err := Task137ListConcat(limits, lists...)
			return listValue(items), err
		}),
		"slice": functionValueOf("std/list.slice", func(args []Value) (Value, error) {
			if len(args) != 3 {
				return nullValue(), fmt.Errorf("std/list.slice expects list, start, end")
			}
			items, err := listArg(args, 0, "slice")
			if err != nil {
				return nullValue(), err
			}
			start, err := task137ListIndex(args[1], "slice start")
			if err != nil {
				return nullValue(), err
			}
			end, err := task137ListIndex(args[2], "slice end")
			if err != nil {
				return nullValue(), err
			}
			out, err := Task137ListSlice(items, start, end, limits)
			return listValue(out), err
		}),
		"at": functionValueOf("std/list.at", func(args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("std/list.at expects list and index")
			}
			items, err := listArg(args, 0, "at")
			if err != nil {
				return nullValue(), err
			}
			index, err := task137ListIndex(args[1], "index")
			if err != nil {
				return nullValue(), err
			}
			return Task137ListAt(items, index, limits)
		}),
		"index_of": functionValueOf("std/list.index_of", func(args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("std/list.index_of expects list and value")
			}
			items, err := listArg(args, 0, "index_of")
			if err != nil {
				return nullValue(), err
			}
			index, err := Task137ListIndexOf(items, args[1], false, limits)
			return intValue(big.NewInt(int64(index)), 64), err
		}),
		"last_index_of": functionValueOf("std/list.last_index_of", func(args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("std/list.last_index_of expects list and value")
			}
			items, err := listArg(args, 0, "last_index_of")
			if err != nil {
				return nullValue(), err
			}
			index, err := Task137ListIndexOf(items, args[1], true, limits)
			return intValue(big.NewInt(int64(index)), 64), err
		}),
		"contains": functionValueOf("std/list.contains", func(args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("std/list.contains expects list and value")
			}
			items, err := listArg(args, 0, "contains")
			if err != nil {
				return nullValue(), err
			}
			index, err := Task137ListIndexOf(items, args[1], false, limits)
			return boolValue(index >= 0), err
		}),
		"count": functionValueOf("std/list.count", func(args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("std/list.count expects list and value")
			}
			items, err := listArg(args, 0, "count")
			if err != nil {
				return nullValue(), err
			}
			count := 0
			for _, item := range items {
				if item.Equal(args[1]) {
					count++
				}
			}
			return uintValue(uint64(count), 64), nil
		}),
		"reverse": functionValueOf("std/list.reverse", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("std/list.reverse expects list")
			}
			items, err := listArg(args, 0, "reverse")
			out, err := Task137ListReverse(items, limits)
			return listValue(out), err
		}),
		"sort": functionValueOf("std/list.sort", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("std/list.sort expects list")
			}
			items, err := listArg(args, 0, "sort")
			if err != nil {
				return nullValue(), err
			}
			out, err := Task137ListSort(items, limits)
			return listValue(out), err
		}),
		"unique": functionValueOf("std/list.unique", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("std/list.unique expects list")
			}
			items, err := listArg(args, 0, "unique")
			if err != nil {
				return nullValue(), err
			}
			out, err := Task137ListUnique(items, limits)
			return listValue(out), err
		}),
		"partition": functionValueOf("std/list.partition", func(args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("std/list.partition expects list and index")
			}
			items, err := listArg(args, 0, "partition")
			if err != nil {
				return nullValue(), err
			}
			index, err := task137ListIndex(args[1], "partition index")
			if err != nil {
				return nullValue(), err
			}
			return Task137ListPartition(items, index, limits)
		}),
		"zip": functionValueOf("std/list.zip", func(args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("std/list.zip expects two lists")
			}
			left, err := listArg(args, 0, "zip")
			if err != nil {
				return nullValue(), err
			}
			right, err := listArg(args, 1, "zip")
			if err != nil {
				return nullValue(), err
			}
			out, err := Task137ListZip(left, right, limits)
			return listValue(out), err
		}),
		"windows": functionValueOf("std/list.windows", func(args []Value) (Value, error) {
			if len(args) != 3 {
				return nullValue(), fmt.Errorf("std/list.windows expects list, width, step")
			}
			items, err := listArg(args, 0, "windows")
			if err != nil {
				return nullValue(), err
			}
			width, err := task137ListIndex(args[1], "window width")
			if err != nil {
				return nullValue(), err
			}
			step, err := task137ListIndex(args[2], "window step")
			if err != nil {
				return nullValue(), err
			}
			out, err := Task137ListWindows(items, width, step, limits)
			return listValue(out), err
		}),
		"chunks": functionValueOf("std/list.chunks", func(args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("std/list.chunks expects list and width")
			}
			items, err := listArg(args, 0, "chunks")
			if err != nil {
				return nullValue(), err
			}
			width, err := task137ListIndex(args[1], "chunk width")
			if err != nil {
				return nullValue(), err
			}
			out, err := Task137ListChunks(items, width, limits)
			return listValue(out), err
		}),
		"flatten": functionValueOf("std/list.flatten", func(args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("std/list.flatten expects list and depth")
			}
			items, err := listArg(args, 0, "flatten")
			if err != nil {
				return nullValue(), err
			}
			depth, err := task137ListIndex(args[1], "flatten depth")
			if err != nil {
				return nullValue(), err
			}
			out, err := Task137ListFlatten(context.Background(), items, depth, limits)
			return listValue(out), err
		}),
		"all": functionValueOf("std/list.all", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("std/list.all expects list")
			}
			items, err := listArg(args, 0, "all")
			if err != nil {
				return nullValue(), err
			}
			return task137ListBool(items, "all", limits)
		}),
		"any": functionValueOf("std/list.any", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("std/list.any expects list")
			}
			items, err := listArg(args, 0, "any")
			if err != nil {
				return nullValue(), err
			}
			return task137ListBool(items, "any", limits)
		}),
		"none": functionValueOf("std/list.none", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("std/list.none expects list")
			}
			items, err := listArg(args, 0, "none")
			if err != nil {
				return nullValue(), err
			}
			return task137ListBool(items, "none", limits)
		}),
		"enumerate": functionValueOf("std/list.enumerate", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("std/list.enumerate expects list")
			}
			items, err := listArg(args, 0, "enumerate")
			if err != nil {
				return nullValue(), err
			}
			out, err := task137ListEnumerate(items, limits)
			return listValue(out), err
		}),
	}
	return mod
}

func task137MaxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
