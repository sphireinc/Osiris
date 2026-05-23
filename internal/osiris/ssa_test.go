package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestBuildSSA exercises the behavior described by its body and locks in the
// observable language, runtime, or CLI semantics that the surrounding package
// depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestBuildSSA(t *testing.T) {
	prog, err := Parse("<ssa>", `
let base = 3
func add1(x) => x + base
emit add1(4)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	rt := NewRuntime()
	ssaProg, err := rt.BuildSSA(context.Background(), prog)
	if err != nil {
		t.Fatalf("build ssa failed: %v", err)
	}

	got := FormatSSAProgram(ssaProg)
	for _, want := range []string{
		"ssa <ssa>",
		"fn main: ",
		"fn add1.fn0 (add1)",
		"captures [base -> main.base.0]",
		"defs=",
		"uses=",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("SSA output missing %q\n--- got ---\n%s", want, got)
		}
	}
}

// TestVerifySSARejectsUnknownValue exercises the behavior described by its
// body and locks in the observable language, runtime, or CLI semantics that
// the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestVerifySSARejectsUnknownValue(t *testing.T) {
	prog := &IRProgram{
		SourceName: "<bad>",
		Functions: []*IRFunction{
			{
				Name: "main",
				Blocks: []*IRBlock{
					{
						Name: "main.entry",
						Instrs: []IRInstr{
							{Dest: "main.t0", Op: "copy", Args: []string{"missing"}},
						},
						Term: &irReturn{Value: "main.t0", HasValue: true},
					},
				},
			},
		},
	}

	if _, err := VerifySSAProgram(prog); err == nil {
		t.Fatalf("expected SSA verification to fail")
	} else if !strings.Contains(err.Error(), "undefined name \"missing\"") {
		t.Fatalf("unexpected error: %v", err)
	}
}
