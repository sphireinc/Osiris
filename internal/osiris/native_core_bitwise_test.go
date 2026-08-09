package osiris

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNativeCoreBitOperationsParity crosses the interpreter, VM, generated C,
// and native executable for the complete fixed-width unsigned bit family.
// The fixture includes width-sensitive zero behavior, same-width results, and
// byte-order reversal; it does not depend on a host capability or provider.
func TestNativeCoreBitOperationsParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeCore-native-bit-operations",
		Source: `import "std/math" as math
emit math.count_ones(0xf0u8)
emit math.count_zeros(0xf0u8)
emit math.leading_zeros(0x0fu8)
emit math.trailing_zeros(0xf0u8)
emit math.bit_width(0xf0u8)
emit math.bit_floor(129u8)
emit math.bit_ceil(65u8)
emit math.has_single_bit(128u8)
emit math.has_single_bit(129u8)
emit math.byte_swap(0x1234u16)
emit math.leading_zeros(0u64), math.trailing_zeros(0u64), math.bit_width(0u64)
emit math.byte_swap(0x0102030405060708u64)
`,
	})
}

// TestNativeCoreBitOperationsLoweringOwnershipAndCapabilityContract checks
// that every member has a direct generated-C helper, remains allocation-free,
// and never crosses the std/math dispatcher or a callable fallback. The
// inventory assertions keep the public Native C claim synchronized with the
// implementation boundary.
func TestNativeCoreBitOperationsLoweringOwnershipAndCapabilityContract(t *testing.T) {
	program, err := Parse("<nativeCore-native-bit-operations-contract>", `import "std/math" as math
emit math.count_ones(0xf0u8)
emit math.count_zeros(0xf0u8)
emit math.leading_zeros(0x0fu8)
emit math.trailing_zeros(0xf0u8)
emit math.bit_width(0xf0u8)
emit math.bit_floor(129u8)
emit math.bit_ceil(65u8)
emit math.has_single_bit(128u8)
emit math.byte_swap(0x1234u16)
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	for _, helper := range []string{
		"ostd_math_count_ones",
		"ostd_math_count_zeros",
		"ostd_math_leading_zeros",
		"ostd_math_trailing_zeros",
		"ostd_math_bit_width",
		"ostd_math_bit_floor",
		"ostd_math_bit_ceil",
		"ostd_math_has_single_bit",
		"ostd_math_byte_swap",
	} {
		if !strings.Contains(artifact.Source, "static OValue "+helper+"(") {
			t.Fatalf("generated bit-operation artifact omitted helper %q", helper)
		}
	}
	mainSource := artifact.Source
	if mainStart := strings.Index(mainSource, "int main(void)"); mainStart >= 0 {
		mainSource = mainSource[mainStart:]
	}
	if strings.Contains(mainSource, "ostd_dispatch(\"std/math\"") || strings.Contains(mainSource, " = ocall(") {
		t.Fatalf("bit operations crossed an indirect native boundary:\n%s", artifact.Source)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure bit operations unexpectedly define a host capability")
	}
	for _, helper := range []string{
		"ostd_math_count_ones",
		"ostd_math_count_zeros",
		"ostd_math_leading_zeros",
		"ostd_math_trailing_zeros",
		"ostd_math_bit_width",
		"ostd_math_bit_floor",
		"ostd_math_bit_ceil",
		"ostd_math_has_single_bit",
		"ostd_math_byte_swap",
	} {
		start := strings.Index(artifact.Source, "static OValue "+helper+"(")
		if start < 0 {
			t.Fatalf("generated bit-operation helper %q is missing", helper)
		}
		end := strings.Index(artifact.Source[start+1:], "\nstatic ")
		if end < 0 {
			end = len(artifact.Source) - start - 1
		}
		if strings.Contains(artifact.Source[start:start+1+end], "malloc(") || strings.Contains(artifact.Source[start:start+1+end], "free(") {
			t.Fatalf("bit-operation helper %q allocates or releases input storage", helper)
		}
	}
	for _, member := range []string{
		"count_ones", "count_zeros", "leading_zeros", "trailing_zeros",
		"bit_width", "bit_floor", "bit_ceil", "has_single_bit", "byte_swap",
	} {
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/math/"+member)
		if !ok {
			t.Fatalf("inventory omitted std/math.%s", member)
		}
		if entry.Native != BackendImplemented {
			t.Fatalf("inventory status for std/math.%s = %q, want implemented", member, entry.Native)
		}
	}
}

// TestNativeCoreBitOperationsErrorContract keeps the native failures explicit:
// invalid input remains a runtime structured failure, while an unrepresentable
// bit ceiling is rejected instead of wrapping into the declared width.
func TestNativeCoreBitOperationsErrorContract(t *testing.T) {
	for _, test := range []struct {
		name        string
		source      string
		interpreter string
		native      string
	}{
		{
			name: "input kind",
			source: `import "std/math" as math
emit math.count_ones(1i8)
`,
			interpreter: "count_ones expects a fixed-width unsigned integer",
			native:      "count_ones expects a fixed-width unsigned integer",
		},
		{
			name: "ceiling overflow",
			source: `import "std/math" as math
emit math.bit_ceil(129u8)
`,
			interpreter: "bit_ceil result is not representable in u8",
			native:      "bit_ceil result is not representable in unsigned width",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			program, err := Parse("<nativeCore-native-bit-error>", test.source)
			if err != nil {
				t.Fatal(err)
			}
			_, interpreterErr := NewRuntime().Run(context.Background(), program)
			if interpreterErr == nil || !strings.Contains(interpreterErr.Error(), test.interpreter) {
				t.Fatalf("interpreter error = %v, want %q", interpreterErr, test.interpreter)
			}
			artifact, err := CompileC(context.Background(), NewRuntime(), program)
			if err != nil {
				t.Fatal(err)
			}
			executable := filepath.Join(t.TempDir(), "native-bit-error")
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
