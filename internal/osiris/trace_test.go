package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestTraceProgram exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
func TestTraceProgram(t *testing.T) {
	prog, err := Parse("<test>", `
let x = 3
emit x + 4
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	frames, err := rt.Trace(context.Background(), prog)
	if err != nil {
		t.Fatalf("trace failed: %v", err)
	}
	if got, want := len(frames), 2; got != want {
		t.Fatalf("frames = %d, want %d", got, want)
	}
	if frames[0].Kind != "let" || frames[0].Statement != "let x = 3u64" {
		t.Fatalf("unexpected first frame: %#v", frames[0])
	}
	if len(frames[0].Scope) == 0 || frames[0].Scope[0].Name != "x" || frames[0].Scope[0].Value != "3u64" {
		t.Fatalf("scope snapshot missing binding: %#v", frames[0].Scope)
	}
	if frames[1].Kind != "emit" || len(frames[1].Outputs) != 1 || frames[1].Outputs[0] != "7u64" {
		t.Fatalf("unexpected second frame: %#v", frames[1])
	}
}

// TestEvalAt exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
func TestEvalAt(t *testing.T) {
	prog, err := Parse("<test>", `
let x = 3
let y = 4
emit x + y
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	v, err := rt.EvalAt(context.Background(), prog, 1, "x + y")
	if err != nil {
		t.Fatalf("eval at failed: %v", err)
	}
	if got, want := v.String(), "7u64"; got != want {
		t.Fatalf("watch = %s, want %s", got, want)
	}
}

// TestTraceCarriesCallStack exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
func TestTraceCarriesCallStack(t *testing.T) {
	prog, err := Parse("<test>", `
emit cat("a", "b")
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	frames, err := rt.Trace(context.Background(), prog)
	if err != nil {
		t.Fatalf("trace failed: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("frames = %d, want 1", len(frames))
	}
	if len(frames[0].Stack) == 0 || !strings.Contains(frames[0].Stack[len(frames[0].Stack)-1], "cat") {
		t.Fatalf("stack missing cat call: %#v", frames[0].Stack)
	}
}
