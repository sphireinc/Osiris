package osiris

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNativeCoreUnsignedDivmodParity covers the fixed-width unsigned portion
// of std/math.divmod. The returned list is observed directly and by index so
// the witness checks both aggregate construction and the wider result width.
func TestNativeCoreUnsignedDivmodParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeCore-native-unsigned-divmod",
		Source: `import "std/math" as math
let pair = math.divmod(u8(17), u16(5))
emit pair
emit at(pair, 0)
emit at(pair, 1)
emit math.divmod(u64(100), u8(9))
`,
	})
}

// TestNativeCoreUnsignedDivmodLoweringContract checks direct helper selection,
// inventory status, and the owned aggregate boundary. The helper copies only
// scalar inputs into a two-item list and introduces no host capability.
func TestNativeCoreUnsignedDivmodLoweringContract(t *testing.T) {
	program, err := Parse("<nativeCore-native-unsigned-divmod-contract>", `import "std/math" as math
emit math.divmod(u8(17), u16(5))
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"static OValue ostd_math_divmod(OValue dividend, OValue divisor)",
		"ostd_math_divmod(",
		"return olist(parts,2);",
	} {
		if !strings.Contains(artifact.Source, marker) {
			t.Fatalf("generated divmod artifact omitted %q", marker)
		}
	}
	mainSource := artifact.Source
	if mainStart := strings.Index(mainSource, "int main(void)"); mainStart >= 0 {
		mainSource = mainSource[mainStart:]
	}
	if strings.Contains(mainSource, "ostd_dispatch(\"std/math\"") || strings.Contains(mainSource, " = ocall(") {
		t.Fatalf("divmod crossed an indirect native boundary:\n%s", artifact.Source)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure divmod lowering unexpectedly defines a host capability")
	}
	helperStart := strings.Index(artifact.Source, "static OValue ostd_math_divmod(")
	helperEnd := strings.Index(artifact.Source[helperStart+1:], "\nstatic ")
	if helperStart < 0 || helperEnd < 0 {
		t.Fatal("generated divmod helper region is incomplete")
	}
	helper := artifact.Source[helperStart : helperStart+1+helperEnd]
	if strings.Contains(helper, "malloc(") || strings.Contains(helper, "free(") {
		t.Fatalf("divmod helper retains or allocates input storage:\n%s", helper)
	}
	for _, id := range []string{"builtin/divmod", "module/std/math/divmod"} {
		entry, ok := findBackendCapability(BackendCapabilityInventory(), id)
		if !ok || entry.Native != BackendImplemented {
			t.Fatalf("inventory entry %s = %+v, want implemented", id, entry)
		}
	}
}

// TestNativeCoreUnsignedDivmodErrorContract keeps the bounded ABI failures
// aligned across the reference runtime and generated executable. Wider signed
// and arbitrary-precision inputs remain outside this native slice.
func TestNativeCoreUnsignedDivmodErrorContract(t *testing.T) {
	for _, test := range []struct {
		name        string
		source      string
		interpreter string
		native      string
	}{
		{
			name: "zero divisor",
			source: `import "std/math" as math
emit math.divmod(u8(7), u16(0))
`,
			interpreter: "divmod divisor must be non-zero",
			native:      "divmod divisor must be non-zero",
		},
		{
			name: "unsupported kind",
			source: `import "std/math" as math
emit math.divmod(i8(7), u8(2))
`,
			interpreter: "",
			native:      "divmod expects fixed-width unsigned integers",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			program, err := Parse("<nativeCore-native-unsigned-divmod-error>", test.source)
			if err != nil {
				t.Fatal(err)
			}
			values, interpreterErr := NewRuntime().Run(context.Background(), program)
			if test.interpreter == "" {
				if interpreterErr != nil {
					t.Fatalf("interpreter error = %v, want successful reference execution", interpreterErr)
				}
				if len(values) != 1 {
					t.Fatalf("interpreter values = %d, want one list result", len(values))
				}
			} else if interpreterErr == nil || !strings.Contains(interpreterErr.Error(), test.interpreter) {
				t.Fatalf("interpreter error = %v, want %q", interpreterErr, test.interpreter)
			}
			artifact, err := CompileC(context.Background(), NewRuntime(), program)
			if err != nil {
				t.Fatal(err)
			}
			executable := filepath.Join(t.TempDir(), "native-unsigned-divmod-error")
			if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
				t.Fatal(err)
			}
			output, nativeErr := exec.Command(executable).CombinedOutput()
			if nativeErr == nil || !strings.Contains(string(output), test.native) {
				t.Fatalf("native error = %v, output %q, want message containing %q", nativeErr, output, test.native)
			}
		})
	}
}
