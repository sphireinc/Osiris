package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestBuildDom exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
func TestBuildDom(t *testing.T) {
	prog, err := Parse("<dom>", `
let x = 0
while x < 2 { let x = x + 1 }
emit if(x > 0, x, 0)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	rt := NewRuntime()
	domProg, err := rt.BuildDom(context.Background(), prog)
	if err != nil {
		t.Fatalf("build dom failed: %v", err)
	}

	got := FormatDomProgram(domProg)
	for _, want := range []string{
		"dom <dom>",
		"fn main: entry=main.entry",
		"idom=main.entry",
		"block main.while.header",
		"children [main.while.body",
		"block main.if.join",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("dominator output missing %q\n--- got ---\n%s", want, got)
		}
	}
}
