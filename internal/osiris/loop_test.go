package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestLoopCanonicalization exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestLoopCanonicalization(t *testing.T) {
	prog, err := Parse("<test>", `
func sweep(xs) {
  let total = 0
  for x in xs {
    let total = total + x
  }
  while total < 10 {
    break
  }
}
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	lp, err := NewRuntime().BuildLoopProgram(context.Background(), prog)
	if err != nil {
		t.Fatalf("loop analysis failed: %v", err)
	}
	got := FormatLoopProgram(lp)
	for _, want := range []string{
		"for loop header=",
		"while loop header=",
		"canonical",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("loop report missing %q\n%s", want, got)
		}
	}
}

func TestLoopInductionAnalysis(t *testing.T) {
	prog, err := Parse("<test>", `
let i = 0
while i < 8 {
  let i = i + 2
}
emit i
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	lp, err := NewRuntime().BuildLoopProgram(context.Background(), prog)
	if err != nil {
		t.Fatalf("loop analysis failed: %v", err)
	}
	got := FormatLoopProgram(lp)
	for _, want := range []string{"canonical", "induction=", "step=2", "bound="} {
		if !strings.Contains(got, want) {
			t.Fatalf("induction report missing %q\n%s", want, got)
		}
	}
}
