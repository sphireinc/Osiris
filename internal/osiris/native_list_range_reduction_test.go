package osiris

import (
	"context"
	"strings"
	"testing"
)

func TestNativeListRangeAndReductionParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "native-list-range-reduction",
		Source: `import "std/list" as list
emit list.range(4)
emit list.sum([1, 2, 3])
emit list.product([2, 3, 4])
emit list.sum([1.5f64, 2.25f64])
emit list.product([complex(1, 2), complex(2, 0)])
emit list.sum([])
emit list.product([])
emit try { list.range(-1) } catch err { "caught-range" }
emit try { list.sum([1, "bad"]) } catch err { "caught-sum" }
`,
	})
}

func TestNativeListRangeAndReductionUseOwnedDirectHelpers(t *testing.T) {
	program, err := Parse("native-list-range-reduction-contract.osr", `import "std/list" as list
emit list.range(3)
emit list.sum([1, 2])
emit list.product([2, 3])
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure list range and reductions unexpectedly define a host capability")
	}
	for _, call := range []string{
		"ostd_list_range(v",
		"ostd_list_sum(v",
		"ostd_list_product(v",
	} {
		if !strings.Contains(artifact.Source, call) {
			t.Fatalf("generated artifact does not contain direct helper call %q", call)
		}
	}
	if strings.Contains(artifact.Source, `ostd_dispatch("std/list"`) {
		t.Fatal("qualified pure list operations unexpectedly use module dispatch")
	}
}
