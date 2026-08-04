package osiris

import (
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
)

// UnsafeMemoryCapability is the host capability required before std/unsafe
// can be imported. The capability is deliberately separate from ffi.*: a
// native-library grant does not confer authority to allocate or inspect memory.
const UnsafeMemoryCapability = "unsafe.memory"

const (
	unsafeMemoryModuleName = "std/unsafe"
	unsafeMemoryVersion    = "1.0.0"
	unsafeMemoryMax        = 64 << 20
)

// UnsafeMemoryCapabilityManifest is the auditable requirement for the
// std/unsafe module. Hosts should copy this requirement into their launch or
// package capability manifest and grant it through Runtime.GrantCapability.
// Language code cannot create or extend this grant.
var UnsafeMemoryCapabilityManifest = map[string][]string{
	unsafeMemoryModuleName: {UnsafeMemoryCapability},
}

func requireStdlibCapability(rt *Runtime, module, capability string) error {
	if rt == nil || !rt.hasCapability(capability) {
		return fmt.Errorf("cannot import %s: capability %q is not granted", module, capability)
	}
	return nil
}

func stdlibModuleCapability(module string) string {
	if module == unsafeMemoryModuleName {
		return UnsafeMemoryCapability
	}
	return ""
}

// unsafeMemoryBlock owns the bytes. It contains no address that can cross the
// language boundary. Each live FFIHandle holds one reference, so releasing a
// parent buffer cannot invalidate an outstanding view.
type unsafeMemoryBlock struct {
	mu       sync.RWMutex
	data     []byte
	refs     int
	released bool
}

// unsafeMemoryResource is the opaque payload stored behind an FFIHandle. All
// offsets are checked byte offsets into the block; element access performs a
// second checked conversion from element index to byte offset.
type unsafeMemoryResource struct {
	block       *unsafeMemoryBlock
	offset      int
	length      int
	elementSize int
	alignment   int
	order       binary.ByteOrder
	writable    bool
}

func unsafeMemoryBlockValue(data []byte) (Value, error) {
	block := &unsafeMemoryBlock{data: append([]byte(nil), data...), refs: 1}
	resource := &unsafeMemoryResource{
		block: block, length: len(data), elementSize: 1, alignment: 1,
		order: binary.LittleEndian, writable: true,
	}
	handle, err := NewFFIHandle("handle:unsafe_memory_buffer", resource)
	if err != nil {
		return nullValue(), err
	}
	return ffiHandleValueOf(handle), nil
}

func unsafeMemoryRelease(value any) error {
	resource, ok := value.(*unsafeMemoryResource)
	if !ok || resource == nil || resource.block == nil {
		return nil
	}
	block := resource.block
	block.mu.Lock()
	if block.refs > 0 {
		block.refs--
	}
	if block.refs == 0 {
		block.released = true
		block.data = nil
	}
	block.mu.Unlock()
	return nil
}

func unsafeMemoryRetainView(value Value, operation string, makeView func(*unsafeMemoryResource, []byte) (*unsafeMemoryResource, error)) (Value, error) {
	handle, err := unsafeMemoryHandle(value, operation)
	if err != nil {
		return nullValue(), err
	}
	handle.mu.Lock()
	defer handle.mu.Unlock()
	if handle.closed {
		return nullValue(), fmt.Errorf("unsafe.memory.%s: handle is released", operation)
	}
	parent, ok := handle.value.(*unsafeMemoryResource)
	if !ok || parent == nil || parent.block == nil {
		return nullValue(), fmt.Errorf("unsafe.memory.%s: invalid handle payload", operation)
	}
	block := parent.block
	block.mu.Lock()
	defer block.mu.Unlock()
	if block.released || parent.offset < 0 || parent.length < 0 || parent.offset > len(block.data) || parent.length > len(block.data)-parent.offset {
		return nullValue(), fmt.Errorf("unsafe.memory.%s: view lifetime has ended", operation)
	}
	child, err := makeView(parent, block.data[parent.offset:parent.offset+parent.length])
	if err != nil {
		return nullValue(), err
	}
	child.block = block
	block.refs++
	view, err := NewFFIHandle("handle:unsafe_memory_view", child)
	if err != nil {
		block.refs--
		return nullValue(), err
	}
	return ffiHandleValueOf(view), nil
}

func unsafeMemoryHandle(value Value, operation string) (*FFIHandle, error) {
	if value.kind != kindFFIHandle || value.ffiHandle == nil {
		return nil, fmt.Errorf("unsafe.memory.%s expects an unsafe memory handle", operation)
	}
	typeID := value.ffiHandle.TypeID()
	if typeID != "handle:unsafe_memory_buffer" && typeID != "handle:unsafe_memory_view" {
		return nil, fmt.Errorf("unsafe.memory.%s received %s, want an unsafe memory handle", operation, value.Kind())
	}
	return value.ffiHandle, nil
}

// unsafeMemoryWithResource holds both the handle mutex and a block read lock
// while an operation runs. This closes the race between an operation and a
// concurrent release: release waits, and the block cannot be reclaimed until
// the operation has finished.
func unsafeMemoryWithResource(value Value, operation string, fn func(*unsafeMemoryResource, []byte) (Value, error)) (Value, error) {
	handle, err := unsafeMemoryHandle(value, operation)
	if err != nil {
		return nullValue(), err
	}
	handle.mu.Lock()
	defer handle.mu.Unlock()
	if handle.closed {
		return nullValue(), fmt.Errorf("unsafe.memory.%s: handle is released", operation)
	}
	resource, ok := handle.value.(*unsafeMemoryResource)
	if !ok || resource == nil || resource.block == nil {
		return nullValue(), fmt.Errorf("unsafe.memory.%s: invalid handle payload", operation)
	}
	block := resource.block
	block.mu.RLock()
	defer block.mu.RUnlock()
	if block.released || resource.offset < 0 || resource.length < 0 || resource.offset > len(block.data) || resource.length > len(block.data)-resource.offset {
		return nullValue(), fmt.Errorf("unsafe.memory.%s: view lifetime has ended", operation)
	}
	return fn(resource, block.data[resource.offset:resource.offset+resource.length])
}

func unsafeMemoryInt(value Value, name string) (int, error) {
	if value.kind == kindInt && value.i != nil && !value.i.IsInt64() {
		return 0, fmt.Errorf("%s is outside the host integer range", name)
	}
	n, err := value.Int()
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	return n, nil
}

func unsafeMemoryNonNegative(value Value, name string) (int, error) {
	n, err := unsafeMemoryInt(value, name)
	if err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, fmt.Errorf("%s must be non-negative", name)
	}
	return n, nil
}

func unsafeMemoryPowerOfTwo(value Value, name string) (int, error) {
	n, err := unsafeMemoryNonNegative(value, name)
	if err != nil {
		return 0, err
	}
	if n == 0 || n > 8 || n&(n-1) != 0 {
		return 0, fmt.Errorf("%s must be one of 1, 2, 4, or 8", name)
	}
	return n, nil
}

func unsafeMemoryByteOrder(value Value) (binary.ByteOrder, string, error) {
	name, err := value.AsString()
	if err != nil {
		return nil, "", fmt.Errorf("byte_order: %w", err)
	}
	switch strings.ToLower(name) {
	case "little", "le":
		return binary.LittleEndian, "little", nil
	case "big", "be":
		return binary.BigEndian, "big", nil
	default:
		return nil, "", fmt.Errorf("byte_order must be \"little\" or \"big\"")
	}
}

func unsafeMemoryNewBuffer(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("unsafe.memory.buffer expects size")
	}
	size, err := unsafeMemoryNonNegative(args[0], "size")
	if err != nil {
		return nullValue(), err
	}
	if size > unsafeMemoryMax {
		return nullValue(), fmt.Errorf("unsafe.memory.buffer exceeds %d-byte limit", unsafeMemoryMax)
	}
	return unsafeMemoryBlockValue(make([]byte, size))
}

func unsafeMemoryFromBytes(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("unsafe.memory.from_bytes expects bytes")
	}
	data, err := bytesFromValue(args[0])
	if err != nil {
		return nullValue(), fmt.Errorf("unsafe.memory.from_bytes: %w", err)
	}
	if len(data) > unsafeMemoryMax {
		return nullValue(), fmt.Errorf("unsafe.memory.from_bytes exceeds %d-byte limit", unsafeMemoryMax)
	}
	return unsafeMemoryBlockValue(data)
}

func unsafeMemoryView(args []Value) (Value, error) {
	if len(args) != 7 {
		return nullValue(), fmt.Errorf("unsafe.memory.view expects handle, offset, length, element_size, alignment, byte_order, and writable")
	}
	offset, err := unsafeMemoryNonNegative(args[1], "offset")
	if err != nil {
		return nullValue(), err
	}
	length, err := unsafeMemoryNonNegative(args[2], "length")
	if err != nil {
		return nullValue(), err
	}
	elementSize, err := unsafeMemoryNonNegative(args[3], "element_size")
	if err != nil {
		return nullValue(), err
	}
	if elementSize != 1 && elementSize != 2 && elementSize != 4 && elementSize != 8 {
		return nullValue(), fmt.Errorf("element_size must be one of 1, 2, 4, or 8")
	}
	alignment, err := unsafeMemoryPowerOfTwo(args[4], "alignment")
	if err != nil {
		return nullValue(), err
	}
	order, _, err := unsafeMemoryByteOrder(args[5])
	if err != nil {
		return nullValue(), err
	}
	if length%elementSize != 0 || offset%alignment != 0 || length%alignment != 0 {
		return nullValue(), fmt.Errorf("unsafe.memory.view has incompatible bounds or alignment")
	}
	if args[6].kind != kindBool {
		return nullValue(), fmt.Errorf("writable must be bool")
	}
	writable := args[6].boo
	// The public contract has seven arguments; retain this explicit check so
	// malformed calls never silently choose mutability.
	return unsafeMemoryRetainView(args[0], "view", func(parent *unsafeMemoryResource, data []byte) (*unsafeMemoryResource, error) {
		if writable && !parent.writable {
			return nil, fmt.Errorf("unsafe.memory.view cannot make an immutable view writable")
		}
		if offset > len(data) || length > len(data)-offset {
			return nil, fmt.Errorf("unsafe.memory.view is outside the parent bounds")
		}
		child := &unsafeMemoryResource{block: parent.block, offset: parent.offset + offset, length: length, elementSize: elementSize, alignment: alignment, order: order, writable: writable}
		return child, nil
	})
}

func unsafeMemorySlice(args []Value) (Value, error) {
	if len(args) != 3 {
		return nullValue(), fmt.Errorf("unsafe.memory.slice expects handle, offset, and length")
	}
	offset, err := unsafeMemoryNonNegative(args[1], "offset")
	if err != nil {
		return nullValue(), err
	}
	length, err := unsafeMemoryNonNegative(args[2], "length")
	if err != nil {
		return nullValue(), err
	}
	return unsafeMemoryRetainView(args[0], "slice", func(parent *unsafeMemoryResource, data []byte) (*unsafeMemoryResource, error) {
		if offset%parent.elementSize != 0 || length%parent.elementSize != 0 || offset%parent.alignment != 0 || length%parent.alignment != 0 || offset > len(data) || length > len(data)-offset {
			return nil, fmt.Errorf("unsafe.memory.slice has incompatible bounds or alignment")
		}
		child := *parent
		child.offset += offset
		child.length = length
		return &child, nil
	})
}

func unsafeMemoryLength(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("unsafe.memory.len expects handle")
	}
	return unsafeMemoryWithResource(args[0], "len", func(resource *unsafeMemoryResource, _ []byte) (Value, error) {
		return uintValue(uint64(resource.length), 64), nil
	})
}

func unsafeMemoryElements(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("unsafe.memory.elements expects handle")
	}
	return unsafeMemoryWithResource(args[0], "elements", func(resource *unsafeMemoryResource, _ []byte) (Value, error) {
		return uintValue(uint64(resource.length/resource.elementSize), 64), nil
	})
}

func unsafeMemoryBytes(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("unsafe.memory.bytes expects handle")
	}
	return unsafeMemoryWithResource(args[0], "bytes", func(_ *unsafeMemoryResource, data []byte) (Value, error) {
		return bytesListValue(data), nil
	})
}

func unsafeMemoryMetadata(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("unsafe.memory.metadata expects handle")
	}
	return unsafeMemoryWithResource(args[0], "metadata", func(resource *unsafeMemoryResource, _ []byte) (Value, error) {
		order := "little"
		if resource.order == binary.BigEndian {
			order = "big"
		}
		return mapValueOf([]mapEntry{
			{key: stringValue("bytes"), value: uintValue(uint64(resource.length), 64)},
			{key: stringValue("elements"), value: uintValue(uint64(resource.length/resource.elementSize), 64)},
			{key: stringValue("element_size"), value: uintValue(uint64(resource.elementSize), 8)},
			{key: stringValue("alignment"), value: uintValue(uint64(resource.alignment), 8)},
			{key: stringValue("byte_order"), value: stringValue(order)},
			{key: stringValue("writable"), value: boolValue(resource.writable)},
		}), nil
	})
}

func unsafeMemoryIndex(args []Value, width int, write bool) (Value, error) {
	name := "read_u" + fmt.Sprint(width*8)
	if write {
		name = "write_u" + fmt.Sprint(width*8)
	}
	want := 3
	if !write {
		want = 2
	}
	if len(args) != want {
		return nullValue(), fmt.Errorf("unsafe.memory.%s expects %d arguments", name, want)
	}
	index, err := unsafeMemoryNonNegative(args[1], "index")
	if err != nil {
		return nullValue(), err
	}
	return unsafeMemoryWithResource(args[0], name, func(resource *unsafeMemoryResource, data []byte) (Value, error) {
		if resource.elementSize != width {
			return nullValue(), fmt.Errorf("unsafe.memory.%s requires an element_size-%d view", name, width)
		}
		if index >= len(data)/width {
			return nullValue(), fmt.Errorf("unsafe.memory.%s index %d is out of bounds", name, index)
		}
		start := index * width
		if write {
			if !resource.writable {
				return nullValue(), fmt.Errorf("unsafe.memory.%s: view is immutable", name)
			}
			n, _, err := args[2].AsUint()
			if err != nil {
				return nullValue(), fmt.Errorf("value: %w", err)
			}
			max := uint64(^uint8(0))
			if width == 2 {
				max = uint64(^uint16(0))
			} else if width == 4 {
				max = uint64(^uint32(0))
			} else if width == 8 {
				max = ^uint64(0)
			}
			if n > max {
				return nullValue(), fmt.Errorf("value does not fit in u%d", width*8)
			}
			switch width {
			case 1:
				data[start] = byte(n)
			case 2:
				resource.order.PutUint16(data[start:start+width], uint16(n))
			case 4:
				resource.order.PutUint32(data[start:start+width], uint32(n))
			case 8:
				resource.order.PutUint64(data[start:start+width], n)
			}
			return boolValue(true), nil
		}
		var n uint64
		switch width {
		case 1:
			n = uint64(data[start])
		case 2:
			n = uint64(resource.order.Uint16(data[start : start+width]))
		case 4:
			n = uint64(resource.order.Uint32(data[start : start+width]))
		case 8:
			n = resource.order.Uint64(data[start : start+width])
		}
		return uintValue(n, uint8(width*8)), nil
	})
}

func unsafeMemoryCopy(args []Value) (Value, error) {
	if len(args) != 5 {
		return nullValue(), fmt.Errorf("unsafe.memory.copy expects destination, destination_offset, source, source_offset, and length")
	}
	dstOffset, err := unsafeMemoryNonNegative(args[1], "destination_offset")
	if err != nil {
		return nullValue(), err
	}
	srcOffset, err := unsafeMemoryNonNegative(args[3], "source_offset")
	if err != nil {
		return nullValue(), err
	}
	length, err := unsafeMemoryNonNegative(args[4], "length")
	if err != nil {
		return nullValue(), err
	}
	// Snapshot the source before locking the destination. Besides keeping the
	// ownership contract simple, this makes copy(dst, ..., dst, ...) safe and
	// avoids a same-handle nested-lock deadlock.
	var source []byte
	_, err = unsafeMemoryWithResource(args[2], "copy", func(_ *unsafeMemoryResource, srcData []byte) (Value, error) {
		if srcOffset > len(srcData) || length > len(srcData)-srcOffset {
			return nullValue(), fmt.Errorf("unsafe.memory.copy source is out of bounds")
		}
		source = append([]byte(nil), srcData[srcOffset:srcOffset+length]...)
		return nullValue(), nil
	})
	if err != nil {
		return nullValue(), err
	}
	return unsafeMemoryWithResource(args[0], "copy", func(dst *unsafeMemoryResource, dstData []byte) (Value, error) {
		if !dst.writable || dstOffset > len(dstData) || length > len(dstData)-dstOffset {
			return nullValue(), fmt.Errorf("unsafe.memory.copy destination is immutable or out of bounds")
		}
		copy(dstData[dstOffset:dstOffset+length], source)
		return uintValue(uint64(length), 64), nil
	})
}

func unsafeMemoryReleaseValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("unsafe.memory.release expects handle")
	}
	handle, err := unsafeMemoryHandle(args[0], "release")
	if err != nil {
		return nullValue(), err
	}
	handle.mu.Lock()
	wasClosed := handle.closed
	handle.mu.Unlock()
	if err := handle.Close(unsafeMemoryRelease); err != nil {
		return nullValue(), err
	}
	return boolValue(!wasClosed), nil
}

func unsafeMemoryModule() map[string]Value {
	return map[string]Value{
		"version":    stringValue(unsafeMemoryVersion),
		"capability": stringValue(UnsafeMemoryCapability),
		"max_bytes":  uintValue(unsafeMemoryMax, 64),
		"buffer":     functionValueOf("std/unsafe.buffer", unsafeMemoryNewBuffer),
		"from_bytes": functionValueOf("std/unsafe.from_bytes", unsafeMemoryFromBytes),
		"view":       functionValueOf("std/unsafe.view", unsafeMemoryView),
		"slice":      functionValueOf("std/unsafe.slice", unsafeMemorySlice),
		"len":        functionValueOf("std/unsafe.len", unsafeMemoryLength),
		"elements":   functionValueOf("std/unsafe.elements", unsafeMemoryElements),
		"bytes":      functionValueOf("std/unsafe.bytes", unsafeMemoryBytes),
		"metadata":   functionValueOf("std/unsafe.metadata", unsafeMemoryMetadata),
		"read_u8":    functionValueOf("std/unsafe.read_u8", func(a []Value) (Value, error) { return unsafeMemoryIndex(a, 1, false) }),
		"read_u16":   functionValueOf("std/unsafe.read_u16", func(a []Value) (Value, error) { return unsafeMemoryIndex(a, 2, false) }),
		"read_u32":   functionValueOf("std/unsafe.read_u32", func(a []Value) (Value, error) { return unsafeMemoryIndex(a, 4, false) }),
		"read_u64":   functionValueOf("std/unsafe.read_u64", func(a []Value) (Value, error) { return unsafeMemoryIndex(a, 8, false) }),
		"write_u8":   functionValueOf("std/unsafe.write_u8", func(a []Value) (Value, error) { return unsafeMemoryIndex(a, 1, true) }),
		"write_u16":  functionValueOf("std/unsafe.write_u16", func(a []Value) (Value, error) { return unsafeMemoryIndex(a, 2, true) }),
		"write_u32":  functionValueOf("std/unsafe.write_u32", func(a []Value) (Value, error) { return unsafeMemoryIndex(a, 4, true) }),
		"write_u64":  functionValueOf("std/unsafe.write_u64", func(a []Value) (Value, error) { return unsafeMemoryIndex(a, 8, true) }),
		"copy":       functionValueOf("std/unsafe.copy", unsafeMemoryCopy),
		"release":    functionValueOf("std/unsafe.release", unsafeMemoryReleaseValue),
	}
}
