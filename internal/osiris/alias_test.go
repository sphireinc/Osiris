package osiris

import (
	"context"
	"strings"
	"testing"
)

func TestAnalyzeAliasProgram(t *testing.T) {
	prog, err := Parse("<alias>", `
let base = tensor([1, 2, 3])
let same = view(base)
let copy = clone(base)
func id(x) => x
func makecopy(x) => clone(x)
func capture() => base
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	rt := NewRuntime()
	aliasProg, err := rt.AnalyzeAlias(context.Background(), prog)
	if err != nil {
		t.Fatalf("alias analysis failed: %v", err)
	}

	got := FormatAliasProgram(aliasProg)
	for _, want := range []string{
		"alias <alias>",
		"classes",
		"main.base [tensor] fresh",
		"members [main.base, main.same]",
		"main.same: tensor shared with main.base",
		"main.copy: tensor fresh",
		"functions",
		"fn capture: result=aliases capture base",
		"fn id: params [x] result=aliases param x",
		"fn makecopy: params [x] result=fresh",
		"escapes [base]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("alias output missing %q\n--- got ---\n%s", want, got)
		}
	}
}
