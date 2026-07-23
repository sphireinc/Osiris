package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestBuildIRLowering exercises the behavior described by its body and locks
// in the observable language, runtime, or CLI semantics that the surrounding
// package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestBuildIRLowering(t *testing.T) {
	prog, err := Parse("<test>", `
let base = 3
func add1(x) => x + base
let i = 0
while i < 2 { let i = i + 1 }
emit if(base > 0, add1(i), 0)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	rt := NewRuntime()
	ir, err := rt.BuildIR(context.Background(), prog)
	if err != nil {
		t.Fatalf("build ir failed: %v", err)
	}

	got := FormatIRProgram(ir)
	for _, want := range []string{
		"program <test>",
		"fn main()",
		"makefunc @add1.fn0 captures [main.base.0]",
		"while.header",
		"if.join",
		"emit",
		"fn add1.fn0(x) captures [base]",
		"return add1.fn0.t0",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("IR output missing %q\n--- got ---\n%s", want, got)
		}
	}
}
