package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestTask162NativeCoreDataParityExpanded keeps a small, executable witness
// for the core/data slice that task 162 owns. The fixture deliberately crosses
// the public interpreter, VM, and generated-C executable paths; it does not
// treat successful C emission alone as parity evidence.
func TestTask162NativeCoreDataParityExpanded(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "task162-native-core-data-expanded",
		Source: `emit ok(7)
emit unwrap(some(9))
emit decode(encode([7u32, 1.25f64, complex(2.5f64, -3.0f64)]))
let values = tensor([2, 2], [1.0f64, 2.0f64, 3.0f64, 4.0f64])
emit [sum(values), dot(values, values), shape(values)]
let tuple = (7u32, "native", 2.5f64)
emit tuple
emit tuple[0]
emit tuple[1]
emit len(tuple)
emit ((1, 2), 3)[0][1]
emit ((1, 2), 3)
`,
	})
}

// TestTask163NativeListReductionParity covers the previously missing native
// std/list.sum and std/list.product dispatch. The cases include ordinary
// numbers, fixed-width floats, and complex values so the C helper must retain
// the hosted result-family contract rather than merely return a C scalar.
func TestTask163NativeListReductionParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "task163-native-list-reductions",
		Source: `import "std/list" as list
emit list.sum([1, 2, 3])
emit list.product([2, 3, 4])
emit list.sum([1.5f64, 2.25f64])
emit list.product([complex(1, 2), complex(2, 0)])
`,
	})
}

func TestTask163NativeListFlattenParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "task163-native-list-flatten",
		Source: `import "std/list" as xs
let nested = [[1, 2], [3, [4, 5]]]
emit xs.flatten(nested, 1)
emit xs.flatten(nested, 2)
emit xs.flatten(nested, 0)
`,
	})
}

// TestTask163NativeTextBoundaryParity covers a bounded, host-independent
// std/text slice. The generated C helpers return owned strings rather than
// views into temporary arguments, and the operations require no host
// capability. The differential harness proves the interpreter, VM, generated
// C, and executable result contracts together.
func TestTask163NativeTextBoundaryParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "task163-native-text-boundary",
		Source: `import "std/text" as text
emit text.trim("  Osiris\u2003")
emit text.trim_space("\tOsiris\n")
emit text.lower("OSIRIS")
emit text.contains("Osiris language", "language")
emit text.has_prefix("Osiris language", "Osiris")
emit text.has_suffix("Osiris language", "language")
`,
	})
}

func TestTask163NativeTextOwnershipAndCapabilityContract(t *testing.T) {
	program, err := Parse("<task163-native-text-contract>", `import "std/text" as text
emit text.lower("OSIRIS")
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatalf("pure std/text lowering unexpectedly defines a host capability")
	}
	for _, symbol := range []string{"ostd_text_case", "oownedstring", "ostd_dispatch"} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated text artifact does not contain ownership boundary %q", symbol)
		}
	}
	entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/text/lower")
	if !ok || entry.Native != BackendImplemented {
		t.Fatalf("std/text.lower inventory entry = %+v, want implemented native capability", entry)
	}
}
