package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestBuildCFG exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
func TestBuildCFG(t *testing.T) {
	prog, err := Parse("<cfg>", `
let x = 0
while x < 2 { let x = x + 1 }
emit if(x > 0, x, 0)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	rt := NewRuntime()
	cfgProg, err := rt.BuildCFG(context.Background(), prog)
	if err != nil {
		t.Fatalf("build cfg failed: %v", err)
	}

	got := FormatCFGProgram(cfgProg)
	for _, want := range []string{
		"cfg <cfg>",
		"fn main: entry=main.entry",
		"block main.while.header",
		"succs [main.while.body",
		"main.while.exit",
		"block main.if.join",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("CFG output missing %q\n--- got ---\n%s", want, got)
		}
	}
}
