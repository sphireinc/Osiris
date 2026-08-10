package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestNativeTensorShapeDataParity covers the next disjoint real-tensor
// shape/data slice. These operations copy their inputs and return either an
// owned tensor or an owned list of tensors; the fixture crosses interpreter,
// VM, generated C, and the executable native path.
func TestNativeTensorShapeDataParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "native-tensor-shape-data",
		Source: `let value = matrix(2, 2, [1.0f64, 2.0f64, 3.0f64, 4.0f64])
emit flatten(repeat(value, 2))
emit flatten(tile(value, 2))
emit split(value, 3)
emit flatten(gather(value, [3, 0, 2]))
emit flatten(scatter(value, [0, 3], [9.0f64, 8.0f64]))
emit flatten(scatter(value, [1, 2], 7.0f64))
emit flatten(pad(value, 1, 2, -1.0f64))
`,
	})
}

// TestNativeTensorShapeDataInvalidShapes keeps value and shape errors
// in the same structured runtime path. No invalid operation is allowed to
// fall back to the interpreter or VM after C emission.
func TestNativeTensorShapeDataInvalidShapes(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "native-tensor-shape-data-errors",
		Source: `let value = matrix(2, 2, [1, 2, 3, 4])
emit try { repeat(value, -1) } catch err { "repeat-error" }
emit try { split(value, 0) } catch err { "split-error" }
emit try { gather(value, [4]) } catch err { "gather-error" }
emit try { scatter(value, [0, 1], [9, 8, 7]) } catch err { "scatter-values-error" }
emit try { scatter(value, [4], 7) } catch err { "scatter-index-error" }
emit try { pad(value, -1, 0) } catch err { "pad-error" }
`,
	})
}

// TestNativeTensorShapeDataLoweringOwnershipAndNoDispatch checks the
// direct helper and ownership boundary. The helpers allocate result storage,
// copy source data, and never retain input pointers or invoke a dispatcher or
// callable fallback.
func TestNativeTensorShapeDataLoweringOwnershipAndNoDispatch(t *testing.T) {
	program, err := Parse("<native-tensor-shape-data-contract>", `let value = matrix(2, 2, [1, 2, 3, 4])
emit repeat(value, 2)
emit tile(value, 2)
emit split(value, 3)
emit gather(value, [3, 0])
emit scatter(value, [0], 9)
emit pad(value, 1, 1, 0)
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	for _, helper := range []string{
		"otensor_repeat", "otensor_tile", "otensor_split", "otensor_gather", "otensor_scatter", "otensor_pad",
	} {
		if !strings.Contains(artifact.Source, "static OValue "+helper+"(") {
			t.Fatalf("generated artifact omitted direct helper %q", helper)
		}
	}
	mainSource := artifact.Source
	if start := strings.Index(mainSource, "int main(void)"); start >= 0 {
		mainSource = mainSource[start:]
	}
	for _, forbidden := range []string{`ostd_dispatch("std/math"`, " = ocall(", "retain("} {
		if strings.Contains(mainSource, forbidden) {
			t.Fatalf("tensor shape/data lowering crossed forbidden boundary %q", forbidden)
		}
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("tensor shape/data lowering unexpectedly defined a host capability")
	}
	for _, marker := range []string{"otensordatawidth", "memcpy", "free(items)", "olist(items"} {
		if !strings.Contains(artifact.Source, marker) {
			t.Fatalf("generated artifact omitted ownership marker %q", marker)
		}
	}
	for _, name := range []string{"repeat", "split", "tile", "gather", "scatter", "pad"} {
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "builtin/"+name)
		if !ok || entry.Native != BackendImplemented {
			t.Fatalf("builtin/%s inventory entry = %+v, want implemented", name, entry)
		}
	}
}
