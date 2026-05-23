package osiris

import (
	"context"
	"strings"
	"testing"
)

func TestPurityAnalysis(t *testing.T) {
	prog, err := Parse("<test>", `
let base = tensor([1, 2, 3])
let same = view(base)
let copy = clone(base)
let digest = hash("osiris")
let imp = read("missing.txt")
func id(x) => x
func recurse(n) => if(n == 0, 0, recurse(n - 1))
func impure() => read("missing.txt")
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.AnalyzePurity(context.Background(), prog)
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	got := FormatPurityProgram(out)
	for _, want := range []string{
		"purity <test>",
		"main.base: tensor pure",
		"main.same: tensor pure",
		"main.copy: tensor pure",
		"main.digest: string pure",
		"main.imp: string impure",
		"fn id @ main params [x] pure",
		"fn recurse @ main params [n] pure",
		"calls [recurse]",
		"fn impure @ main impure",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("purity report missing %q\n%s", want, got)
		}
	}
}
