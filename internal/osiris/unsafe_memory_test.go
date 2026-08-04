package osiris

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func unsafeMemoryModuleMember(t *testing.T, rt *Runtime, name string) Value {
	t.Helper()
	mod, err := rt.loadModule(context.Background(), unsafeMemoryModuleName, "<unsafe-test>", nil)
	if err != nil {
		t.Fatal(err)
	}
	mv, err := mod.AsModule()
	if err != nil {
		t.Fatal(err)
	}
	member, ok := mv.members[name]
	if !ok {
		t.Fatalf("std/unsafe member %q is missing", name)
	}
	return member
}

func unsafeMemoryCall(t *testing.T, fn Value, args ...Value) Value {
	t.Helper()
	call, err := fn.AsFunction()
	if err != nil {
		t.Fatal(err)
	}
	value, err := call(args)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestUnsafeMemoryImportRequiresExplicitCapability(t *testing.T) {
	rt := NewRuntime()
	if _, err := rt.loadModule(context.Background(), unsafeMemoryModuleName, "<unsafe-gate>", nil); err == nil || !strings.Contains(err.Error(), UnsafeMemoryCapability) {
		t.Fatalf("import without capability error = %v", err)
	}
	rt.GrantCapability(UnsafeMemoryCapability)
	if _, err := rt.loadModule(context.Background(), unsafeMemoryModuleName, "<unsafe-gate>", nil); err != nil {
		t.Fatal(err)
	}
	rt.RevokeCapability(UnsafeMemoryCapability)
	if _, err := rt.loadModule(context.Background(), unsafeMemoryModuleName, "<unsafe-gate>", nil); err == nil {
		t.Fatal("cached std/unsafe import remained available after capability revocation")
	}
}

func TestUnsafeMemoryBoundedTypedViewAndOwnership(t *testing.T) {
	rt := NewRuntime()
	rt.GrantCapability(UnsafeMemoryCapability)
	buffer := unsafeMemoryCall(t, unsafeMemoryModuleMember(t, rt, "buffer"), uintValue(16, 64))
	view := unsafeMemoryCall(t, unsafeMemoryModuleMember(t, rt, "view"), buffer,
		uintValue(0, 64), uintValue(16, 64), uintValue(2, 8), uintValue(2, 8), stringValue("big"), boolValue(true))
	metadata := unsafeMemoryCall(t, unsafeMemoryModuleMember(t, rt, "metadata"), view)
	if metadata.kind != kindMap || len(metadata.mapValue.entries) != 6 {
		t.Fatalf("metadata = %s", metadata.String())
	}
	unsafeMemoryCall(t, unsafeMemoryModuleMember(t, rt, "write_u16"), view, uintValue(2, 64), uintValue(0x1234, 16))
	got := unsafeMemoryCall(t, unsafeMemoryModuleMember(t, rt, "read_u16"), view, uintValue(2, 64))
	value, _, err := got.AsUint()
	if err != nil || value != 0x1234 {
		t.Fatalf("read_u16 = %s, %v", got.String(), err)
	}
	bytes := unsafeMemoryCall(t, unsafeMemoryModuleMember(t, rt, "bytes"), buffer)
	if raw, err := bytesFromValue(bytes); err != nil || len(raw) != 16 || raw[4] != 0x12 || raw[5] != 0x34 {
		t.Fatalf("buffer bytes = %v, %v", raw, err)
	}
	copyFn, err := unsafeMemoryModuleMember(t, rt, "copy").AsFunction()
	if err != nil {
		t.Fatal(err)
	}
	if copied, err := copyFn([]Value{buffer, uintValue(0, 64), buffer, uintValue(4, 64), uintValue(4, 64)}); err != nil {
		t.Fatalf("same-handle copy = %s, %v", copied.String(), err)
	} else if n, err := copied.Int(); err != nil || n != 4 {
		t.Fatalf("same-handle copy count = %d, %v", n, err)
	}

	// Releasing the parent leaves the retained view valid; releasing the view
	// twice is a no-op and subsequent use is rejected.
	released := unsafeMemoryCall(t, unsafeMemoryModuleMember(t, rt, "release"), buffer)
	if !released.Truthy() {
		t.Fatal("first buffer release did not report ownership transition")
	}
	still, err := unsafeMemoryModuleMember(t, rt, "read_u16").AsFunction()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := still([]Value{view, uintValue(2, 64)}); err != nil {
		t.Fatalf("retained view became invalid after parent release: %v", err)
	}
	if !unsafeMemoryCall(t, unsafeMemoryModuleMember(t, rt, "release"), view).Truthy() {
		t.Fatal("view release did not report ownership transition")
	}
	if unsafeMemoryCall(t, unsafeMemoryModuleMember(t, rt, "release"), view).Truthy() {
		t.Fatal("second view release reported a transition")
	}
	if _, err := still([]Value{view, uintValue(2, 64)}); err == nil || !strings.Contains(err.Error(), "released") {
		t.Fatalf("use after release error = %v", err)
	}
}

func TestUnsafeMemoryRejectsBoundsAlignmentAndSerializationEscape(t *testing.T) {
	rt := NewRuntime()
	rt.GrantCapability(UnsafeMemoryCapability)
	buffer := unsafeMemoryCall(t, unsafeMemoryModuleMember(t, rt, "buffer"), uintValue(8, 64))
	viewFn, err := unsafeMemoryModuleMember(t, rt, "view").AsFunction()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := viewFn([]Value{buffer, uintValue(1, 64), uintValue(4, 64), uintValue(4, 8), uintValue(4, 8), stringValue("little"), boolValue(true)}); err == nil {
		t.Fatal("misaligned view unexpectedly succeeded")
	}
	if _, err := viewFn([]Value{buffer, uintValue(4, 64), uintValue(8, 64), uintValue(1, 8), uintValue(1, 8), stringValue("little"), boolValue(true)}); err == nil {
		t.Fatal("out-of-bounds view unexpectedly succeeded")
	}
	immutable, err := viewFn([]Value{buffer, uintValue(0, 64), uintValue(4, 64), uintValue(1, 8), uintValue(1, 8), stringValue("little"), boolValue(false)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := viewFn([]Value{immutable, uintValue(0, 64), uintValue(1, 64), uintValue(1, 8), uintValue(1, 8), stringValue("little"), boolValue(true)}); err == nil {
		t.Fatal("writable child of immutable view unexpectedly succeeded")
	}
	encoded, err := json.Marshal(buffer)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "0x") || strings.Contains(string(encoded), "address") || strings.Contains(string(encoded), "unsafe_memory_buffer") == false {
		t.Fatalf("opaque handle serialization = %s", encoded)
	}
	if _, err := task120ValueToConfig(buffer); err == nil {
		t.Fatal("unsafe handle unexpectedly became a JSON/config value")
	}
	release, err := unsafeMemoryModuleMember(t, rt, "release").AsFunction()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = release([]Value{immutable})
	_, _ = release([]Value{buffer})
}

func TestUnsafeMemoryConcurrentReadAndRelease(t *testing.T) {
	rt := NewRuntime()
	rt.GrantCapability(UnsafeMemoryCapability)
	buffer := unsafeMemoryCall(t, unsafeMemoryModuleMember(t, rt, "buffer"), uintValue(8, 64))
	write := unsafeMemoryModuleMember(t, rt, "write_u8")
	read := unsafeMemoryModuleMember(t, rt, "read_u8")
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			fn, _ := write.AsFunction()
			_, _ = fn([]Value{buffer, uintValue(uint64(index%8), 64), uintValue(uint64(index), 8)})
			fn, _ = read.AsFunction()
			_, _ = fn([]Value{buffer, uintValue(uint64(index%8), 64)})
		}(i)
	}
	wg.Wait()
	release, err := unsafeMemoryModuleMember(t, rt, "release").AsFunction()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := release([]Value{buffer}); err != nil {
		t.Fatal(err)
	}
}
