package osiris

import (
	"context"
	"testing"
)

// TestBuildSourceMap exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
func TestBuildSourceMap(t *testing.T) {
	prog, err := Parse("<test>", `
let outer = { let inner = 3; inner }
emit outer
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	sm := BuildSourceMap(prog)
	if got, want := len(sm.Entries), 3; got != want {
		t.Fatalf("entries = %d, want %d", got, want)
	}
	if sm.Entries[0].Kind != "let" || sm.Entries[0].Range.Start.Line != 2 {
		t.Fatalf("unexpected outer entry: %#v", sm.Entries[0])
	}
	inner := sm.Entries[1]
	if inner.Parent != 0 || inner.Kind != "let" || inner.Range.Start.Line != 2 {
		t.Fatalf("unexpected inner entry: %#v", inner)
	}
}

// TestTraceFrameCarriesSourceRange exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
func TestTraceFrameCarriesSourceRange(t *testing.T) {
	prog, err := Parse("<test>", `
let x = 1
emit x
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	frames, err := rt.Trace(context.Background(), prog)
	if err != nil {
		t.Fatalf("trace failed: %v", err)
	}
	if got := frames[0].Range.Start.Line; got != 2 {
		t.Fatalf("frame line = %d, want 2", got)
	}
}
