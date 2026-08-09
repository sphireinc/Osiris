package osiris

import (
	"context"
	"fmt"
	"math/big"
)

// ExtendedListVersion identifies the additive list API staged by extended standard library.
const ExtendedListVersion = "1.1.0"

const (
	// ExtendedListMaxItems is the hard upper bound for any list operation's
	// input or output. A caller may select a smaller limit, but cannot raise it.
	ExtendedListMaxItems = 100_000
	// ExtendedListMaxDepth bounds recursive flattening work.
	ExtendedListMaxDepth = 64
)

// ExtendedListLimits bounds allocations and recursive traversal. Zero values
// select the safe defaults; values above the hard limits are clamped.
type ExtendedListLimits struct {
	MaxItems int
	MaxDepth int
}

func (l ExtendedListLimits) normalized() ExtendedListLimits {
	if l.MaxItems <= 0 || l.MaxItems > ExtendedListMaxItems {
		l.MaxItems = ExtendedListMaxItems
	}
	if l.MaxDepth <= 0 || l.MaxDepth > ExtendedListMaxDepth {
		l.MaxDepth = ExtendedListMaxDepth
	}
	return l
}

func extendedListInput(value Value, limits ExtendedListLimits, name string) ([]Value, error) {
	items, err := value.AsList()
	if err != nil {
		return nil, fmt.Errorf("std/list.%s: %w", name, err)
	}
	limits = limits.normalized()
	if len(items) > limits.MaxItems {
		return nil, extendedListLimitError(name, "items", len(items), limits.MaxItems)
	}
	return items, nil
}

func extendedListLimitError(operation, subject string, got, limit int) error {
	return fmt.Errorf("std/list.%s: %s %d exceeds limit %d", operation, subject, got, limit)
}

func extendedListIndex(value Value, name string) (int, error) {
	index, err := value.Int()
	if err != nil || index < 0 {
		return 0, fmt.Errorf("std/list: %s must be a non-negative integer", name)
	}
	return index, nil
}

func extendedListCheckContext(ctx context.Context, operation string) error {
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

// ExtendedListConcat concatenates lists without mutating any input.
func ExtendedListConcat(limits ExtendedListLimits, lists ...[]Value) ([]Value, error) {
	limits = limits.normalized()
	total := 0
	for _, items := range lists {
		if len(items) > limits.MaxItems || total > limits.MaxItems-len(items) {
			return nil, extendedListLimitError("concat", "output items", total+len(items), limits.MaxItems)
		}
		total += len(items)
	}
	out := make([]Value, 0, total)
	for _, items := range lists {
		out = append(out, items...)
	}
	return out, nil
}

// ExtendedListRepeat returns count copies of item, subject to MaxItems.
func ExtendedListRepeat(item Value, count int, limits ExtendedListLimits) ([]Value, error) {
	limits = limits.normalized()
	if count < 0 {
		return nil, fmt.Errorf("std/list.repeat: count must be non-negative")
	}
	if count > limits.MaxItems {
		return nil, extendedListLimitError("repeat", "count", count, limits.MaxItems)
	}
	out := make([]Value, count)
	for i := range out {
		out[i] = item
	}
	return out, nil
}

// ExtendedListSlice returns the owned half-open range [start,end].
func ExtendedListSlice(items []Value, start, end int, limits ExtendedListLimits) ([]Value, error) {
	limits = limits.normalized()
	if len(items) > limits.MaxItems {
		return nil, extendedListLimitError("slice", "input items", len(items), limits.MaxItems)
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

// ExtendedListAt returns an element and rejects out-of-range indexes.
func ExtendedListAt(items []Value, index int, limits ExtendedListLimits) (Value, error) {
	limits = limits.normalized()
	if len(items) > limits.MaxItems {
		return nullValue(), extendedListLimitError("at", "input items", len(items), limits.MaxItems)
	}
	if index < 0 || index >= len(items) {
		return nullValue(), fmt.Errorf("std/list.at: index %d out of range for length %d", index, len(items))
	}
	return items[index], nil
}

func ExtendedListIndexOf(items []Value, target Value, last bool, limits ExtendedListLimits) (int, error) {
	limits = limits.normalized()
	if len(items) > limits.MaxItems {
		return 0, extendedListLimitError("index_of", "input items", len(items), limits.MaxItems)
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

// ExtendedListReverse returns a reversed owned copy.
func ExtendedListReverse(items []Value, limits ExtendedListLimits) ([]Value, error) {
	limits = limits.normalized()
	if len(items) > limits.MaxItems {
		return nil, extendedListLimitError("reverse", "input items", len(items), limits.MaxItems)
	}
	out := make([]Value, len(items))
	for i := range items {
		out[len(items)-1-i] = items[i]
	}
	return out, nil
}

// ExtendedListSort performs a stable ascending sort using Value.Compare.
func ExtendedListSort(items []Value, limits ExtendedListLimits) ([]Value, error) {
	limits = limits.normalized()
	if len(items) > limits.MaxItems {
		return nil, extendedListLimitError("sort", "input items", len(items), limits.MaxItems)
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

// ExtendedListUnique keeps the first occurrence of every value.
func ExtendedListUnique(items []Value, limits ExtendedListLimits) ([]Value, error) {
	limits = limits.normalized()
	if len(items) > limits.MaxItems {
		return nil, extendedListLimitError("unique", "input items", len(items), limits.MaxItems)
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

// ExtendedListPartition splits at index and returns (left,right).
func ExtendedListPartition(items []Value, index int, limits ExtendedListLimits) (Value, error) {
	left, err := ExtendedListSlice(items, 0, index, limits)
	if err != nil {
		return nullValue(), fmt.Errorf("std/list.partition: %w", err)
	}
	right, err := ExtendedListSlice(items, index, len(items), limits)
	if err != nil {
		return nullValue(), fmt.Errorf("std/list.partition: %w", err)
	}
	return tupleValue([]Value{listValue(left), listValue(right)}), nil
}

// ExtendedListZip pairs elements through the shorter input.
func ExtendedListZip(left, right []Value, limits ExtendedListLimits) ([]Value, error) {
	limits = limits.normalized()
	if len(left) > limits.MaxItems || len(right) > limits.MaxItems {
		return nil, extendedListLimitError("zip", "input items", extendedMaxInt(len(left), len(right)), limits.MaxItems)
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

// ExtendedListWindows returns overlapping windows using a positive step.
func ExtendedListWindows(items []Value, width, step int, limits ExtendedListLimits) ([]Value, error) {
	limits = limits.normalized()
	if width <= 0 || step <= 0 || width > limits.MaxItems || step > limits.MaxItems {
		return nil, fmt.Errorf("std/list.windows: width and step must be positive and within the item limit")
	}
	if len(items) > limits.MaxItems {
		return nil, extendedListLimitError("windows", "input items", len(items), limits.MaxItems)
	}
	out := make([]Value, 0)
	for start := 0; start+width <= len(items); start += step {
		if len(out) == limits.MaxItems {
			return nil, extendedListLimitError("windows", "output windows", len(out)+1, limits.MaxItems)
		}
		out = append(out, listValue(append([]Value(nil), items[start:start+width]...)))
	}
	return out, nil
}

// ExtendedListChunks returns non-overlapping windows of at most width items.
func ExtendedListChunks(items []Value, width int, limits ExtendedListLimits) ([]Value, error) {
	limits = limits.normalized()
	if width <= 0 || width > limits.MaxItems {
		return nil, fmt.Errorf("std/list.chunks: width must be positive and within the item limit")
	}
	if len(items) > limits.MaxItems {
		return nil, extendedListLimitError("chunks", "input items", len(items), limits.MaxItems)
	}
	out := make([]Value, 0, (len(items)+width-1)/width)
	for start := 0; start < len(items); start += width {
		if len(out) == limits.MaxItems {
			return nil, extendedListLimitError("chunks", "output chunks", len(out)+1, limits.MaxItems)
		}
		end := start + width
		if end > len(items) {
			end = len(items)
		}
		out = append(out, listValue(append([]Value(nil), items[start:end]...)))
	}
	return out, nil
}

func extendedListFlattenAppend(ctx context.Context, operation string, dst *[]Value, value Value, depth int, limits ExtendedListLimits) error {
	if err := extendedListCheckContext(ctx, operation); err != nil {
		return err
	}
	if len(*dst) == limits.MaxItems {
		return extendedListLimitError(operation, "output items", len(*dst)+1, limits.MaxItems)
	}
	if value.kind != kindList || depth == 0 {
		*dst = append(*dst, value)
		return nil
	}
	if depth < 0 {
		return fmt.Errorf("std/list.flatten: depth must be non-negative")
	}
	if depth > limits.MaxDepth {
		return extendedListLimitError(operation, "depth", depth, limits.MaxDepth)
	}
	for _, item := range value.list {
		if err := extendedListFlattenAppend(ctx, operation, dst, item, depth-1, limits); err != nil {
			return err
		}
	}
	return nil
}

// ExtendedListFlatten recursively flattens at most depth list layers.
func ExtendedListFlatten(ctx context.Context, items []Value, depth int, limits ExtendedListLimits) ([]Value, error) {
	limits = limits.normalized()
	if depth < 0 || depth > limits.MaxDepth {
		return nil, extendedListLimitError("flatten", "depth", depth, limits.MaxDepth)
	}
	if len(items) > limits.MaxItems {
		return nil, extendedListLimitError("flatten", "input items", len(items), limits.MaxItems)
	}
	out := make([]Value, 0, len(items))
	for _, item := range items {
		if err := extendedListFlattenAppend(ctx, "flatten", &out, item, depth, limits); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ExtendedListMap applies a host callback in input order.
func ExtendedListMap(ctx context.Context, items []Value, callback func(Value) (Value, error), limits ExtendedListLimits) ([]Value, error) {
	limits = limits.normalized()
	if callback == nil {
		return nil, fmt.Errorf("std/list.map: callback is required")
	}
	if len(items) > limits.MaxItems {
		return nil, extendedListLimitError("map", "input items", len(items), limits.MaxItems)
	}
	out := make([]Value, len(items))
	for i, item := range items {
		if err := extendedListCheckContext(ctx, "map"); err != nil {
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

// ExtendedListFilter keeps items for which the host callback returns a truthy value.
func ExtendedListFilter(ctx context.Context, items []Value, callback func(Value) (Value, error), limits ExtendedListLimits) ([]Value, error) {
	limits = limits.normalized()
	if callback == nil {
		return nil, fmt.Errorf("std/list.filter: callback is required")
	}
	if len(items) > limits.MaxItems {
		return nil, extendedListLimitError("filter", "input items", len(items), limits.MaxItems)
	}
	out := make([]Value, 0, len(items))
	for i, item := range items {
		if err := extendedListCheckContext(ctx, "filter"); err != nil {
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

// ExtendedListFold reduces items in input order.
func ExtendedListFold(ctx context.Context, items []Value, initial Value, callback func(Value, Value) (Value, error), limits ExtendedListLimits) (Value, error) {
	limits = limits.normalized()
	if callback == nil {
		return nullValue(), fmt.Errorf("std/list.fold: callback is required")
	}
	if len(items) > limits.MaxItems {
		return nullValue(), extendedListLimitError("fold", "input items", len(items), limits.MaxItems)
	}
	acc := initial
	for i, item := range items {
		if err := extendedListCheckContext(ctx, "fold"); err != nil {
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

func extendedListBool(items []Value, mode string, limits ExtendedListLimits) (Value, error) {
	limits = limits.normalized()
	if len(items) > limits.MaxItems {
		return nullValue(), extendedListLimitError(mode, "input items", len(items), limits.MaxItems)
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

func extendedListEnumerate(items []Value, limits ExtendedListLimits) ([]Value, error) {
	limits = limits.normalized()
	if len(items) > limits.MaxItems {
		return nil, extendedListLimitError("enumerate", "input items", len(items), limits.MaxItems)
	}
	out := make([]Value, len(items))
	for i, item := range items {
		out[i] = tupleValue([]Value{uintValue(uint64(i), 64), item})
	}
	return out, nil
}

func extendedListModule() map[string]Value {
	limits := ExtendedListLimits{}
	listArg := func(args []Value, index int, name string) ([]Value, error) {
		if index >= len(args) {
			return nil, fmt.Errorf("std/list.%s: missing list argument", name)
		}
		return extendedListInput(args[index], limits, name)
	}
	mod := map[string]Value{
		"version":   stringValue(ExtendedListVersion),
		"max_items": uintValue(ExtendedListMaxItems, 64),
		"max_depth": uintValue(ExtendedListMaxDepth, 64),
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
			items, err := ExtendedListRepeat(args[0], count, limits)
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
			items, err := ExtendedListConcat(limits, lists...)
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
			start, err := extendedListIndex(args[1], "slice start")
			if err != nil {
				return nullValue(), err
			}
			end, err := extendedListIndex(args[2], "slice end")
			if err != nil {
				return nullValue(), err
			}
			out, err := ExtendedListSlice(items, start, end, limits)
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
			index, err := extendedListIndex(args[1], "index")
			if err != nil {
				return nullValue(), err
			}
			return ExtendedListAt(items, index, limits)
		}),
		"index_of": functionValueOf("std/list.index_of", func(args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("std/list.index_of expects list and value")
			}
			items, err := listArg(args, 0, "index_of")
			if err != nil {
				return nullValue(), err
			}
			index, err := ExtendedListIndexOf(items, args[1], false, limits)
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
			index, err := ExtendedListIndexOf(items, args[1], true, limits)
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
			index, err := ExtendedListIndexOf(items, args[1], false, limits)
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
			out, err := ExtendedListReverse(items, limits)
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
			out, err := ExtendedListSort(items, limits)
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
			out, err := ExtendedListUnique(items, limits)
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
			index, err := extendedListIndex(args[1], "partition index")
			if err != nil {
				return nullValue(), err
			}
			return ExtendedListPartition(items, index, limits)
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
			out, err := ExtendedListZip(left, right, limits)
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
			width, err := extendedListIndex(args[1], "window width")
			if err != nil {
				return nullValue(), err
			}
			step, err := extendedListIndex(args[2], "window step")
			if err != nil {
				return nullValue(), err
			}
			out, err := ExtendedListWindows(items, width, step, limits)
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
			width, err := extendedListIndex(args[1], "chunk width")
			if err != nil {
				return nullValue(), err
			}
			out, err := ExtendedListChunks(items, width, limits)
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
			depth, err := extendedListIndex(args[1], "flatten depth")
			if err != nil {
				return nullValue(), err
			}
			out, err := ExtendedListFlatten(context.Background(), items, depth, limits)
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
			return extendedListBool(items, "all", limits)
		}),
		"any": functionValueOf("std/list.any", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("std/list.any expects list")
			}
			items, err := listArg(args, 0, "any")
			if err != nil {
				return nullValue(), err
			}
			return extendedListBool(items, "any", limits)
		}),
		"none": functionValueOf("std/list.none", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("std/list.none expects list")
			}
			items, err := listArg(args, 0, "none")
			if err != nil {
				return nullValue(), err
			}
			return extendedListBool(items, "none", limits)
		}),
		"enumerate": functionValueOf("std/list.enumerate", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("std/list.enumerate expects list")
			}
			items, err := listArg(args, 0, "enumerate")
			if err != nil {
				return nullValue(), err
			}
			out, err := extendedListEnumerate(items, limits)
			return listValue(out), err
		}),
	}
	return mod
}

func extendedMaxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
