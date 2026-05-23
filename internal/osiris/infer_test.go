package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestInferProgram exercises the behavior described by its body and locks in
// the observable language, runtime, or CLI semantics that the surrounding
// package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestInferProgram(t *testing.T) {
	prog, err := Parse("<infer>", `
let base = 3
let speed = 10 * Meter / (2 * Second)
let grid = matrix(2, 2, [1, 2, 3, 4])
func add1(x) => x + base
emit add1(4)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	rt := NewRuntime()
	inferProg, err := rt.InferProgram(context.Background(), prog)
	if err != nil {
		t.Fatalf("infer failed: %v", err)
	}

	got := FormatInferProgram(inferProg)
	for _, want := range []string{
		"infer <infer>",
		"bindings",
		"main.base: uint64",
		"main.speed: quantity[Meter_Per_Second]",
		"main.grid: tensor[2x2]",
		"functions",
		"fn add1: result=number",
		"captures [base:uint64]",
		"escapes [base]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("infer output missing %q\n--- got ---\n%s", want, got)
		}
	}
}
