package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestClosureAnalysis exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
func TestClosureAnalysis(t *testing.T) {
	prog, err := Parse("<test>", `
let base = 3
func add1(x) => x + base
emit add1(4)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.AnalyzeClosure(context.Background(), prog)
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	got := FormatClosureProgram(out)
	for _, want := range []string{
		"closure <test>",
		"environments",
		"env add1.fn0.env @ add1.fn0 fields [base -> main.base.0]",
		"fn add1.fn0 (add1): params [x] env=add1.fn0.env captures [base]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("closure report missing %q\n%s", want, got)
		}
	}
}
