package osiris

import (
	"context"
	"strings"
	"testing"
)

func TestNativeCoreMathExtremaInventoryAndLoweringContract(t *testing.T) {
	program, err := Parse("<native-core-math-extrema>", `import "std/math" as math
emit math.min(3, -2, 4.5)
emit math.max(3, -2, 4.5)
emit math.clamp(-2, 0, 10)
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	for _, helper := range []string{"ominmax", "oclamp"} {
		if !strings.Contains(artifact.Source, helper+"(") {
			t.Fatalf("generated extrema artifact omitted direct helper %q", helper)
		}
	}
	if strings.Contains(artifact.Source, `ostd_dispatch("std/math"`) {
		t.Fatal("std/math extrema unexpectedly used module dispatch")
	}
	for _, member := range []string{"min", "max", "clamp"} {
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/math/"+member)
		if !ok || entry.Native != BackendImplemented {
			t.Fatalf("std/math.%s inventory entry = %+v, want implemented native status", member, entry)
		}
	}
}

func TestNativeCoreMathScalarInventoryCoversDirectLowerings(t *testing.T) {
	for _, member := range []string{
		"sqrt", "exp", "log", "log10", "log2", "pow", "expt", "sin", "cos", "tan",
		"asin", "acos", "atan", "sinh", "cosh", "tanh", "asinh", "acosh", "atanh",
		"expm1", "log1p", "cbrt", "erf", "erfc", "gamma", "lgamma", "floor", "ceil", "round",
	} {
		if got := nativeModuleMemberStatus("std/math", member); got != BackendImplemented {
			t.Fatalf("std/math.%s inventory status = %q, want implemented", member, got)
		}
	}
}
