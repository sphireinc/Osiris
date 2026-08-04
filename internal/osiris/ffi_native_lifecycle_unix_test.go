//go:build cgo && (darwin || linux || freebsd || netbsd || openbsd)

package osiris

import (
	"strings"
	"testing"
	"unsafe"
)

func TestFFINativeLibraryCloseRejectsActiveOperation(t *testing.T) {
	sentinel := new(byte)
	state := &ffiNativeState{
		handle: unsafe.Pointer(sentinel),
		cache:  &ffiSymbolCache{symbols: make(map[string]unsafe.Pointer)},
	}
	library := ffiNativeLibrary{state: state}
	active, err := library.begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := library.close(); err == nil || !strings.Contains(err.Error(), "active operations") {
		t.Fatalf("close with active operation = %v, want active-operation error", err)
	}
	library.end(active)
	// Avoid calling dlclose with the sentinel handle; this checks the final
	// state transition without requiring a second dynamic-library fixture.
	state.mu.Lock()
	state.handle = nil
	state.mu.Unlock()
	if err := library.close(); err != nil {
		t.Fatal(err)
	}
	if _, err := library.begin(); err == nil {
		t.Fatal("closed native adapter admitted a new operation")
	}
}
