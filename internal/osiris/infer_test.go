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

func TestInferTensorBinaryUsesBroadcastShape(t *testing.T) {
	prog, err := Parse("<infer-broadcast>", `
let vector = tensor([10, 20])
let matrix = tensor([[1, 2], [3, 4]])
let sum = vector + matrix
let reshaped = reshape(sum, [4])
let transposed = transpose(matrix)
`)
	if err != nil {
		t.Fatal(err)
	}
	inferProg, err := NewRuntime().InferProgram(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	got := FormatInferProgram(inferProg)
	for _, want := range []string{
		"main.vector: tensor[2]",
		"main.matrix: tensor[2x2]",
		"main.sum: tensor[2x2]",
		"main.reshaped: tensor[4]",
		"main.transposed: tensor[2x2]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("infer output missing %q\n--- got ---\n%s", want, got)
		}
	}
}
