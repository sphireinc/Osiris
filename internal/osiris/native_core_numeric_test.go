package osiris

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNativeCoreComplexF32Parity closes the scalar complex<f32> boundary.
// The witness starts with a suffixed literal, constructs the same family from
// typed components, performs a binary operation, and observes projections.
// It therefore crosses Value construction/rendering, IR literal lowering,
// interpreter execution, VM execution, generated-C execution, and the native
// executable rather than treating C emission alone as parity evidence.
func TestNativeCoreComplexF32Parity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeCore-native-complex-f32",
		Source: `let literal = 1.25f32i
let constructed = complex(f32(1.25), f32(2.5))
let sum = literal + constructed
emit literal
emit constructed
emit sum
emit real(constructed)
emit imag(constructed)
`,
	})
}

// TestNativeCoreIntegerMathParity closes the fixed-width unsigned gcd/lcm
// slice. These operations are pure numeric reductions: they must preserve the
// wider input width, stay inside the tagged Native C ABI, and never dispatch
// through the host-independent stdlib fallback.
func TestNativeCoreIntegerMathParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeCore-native-integer-math",
		Source: `import "std/math" as math
emit math.gcd(u8(48), u16(18))
emit math.lcm(u8(12), u16(18))
emit math.gcd(u64(0), u64(9))
`,
	})
}

// TestNativeCoreIntegerMathLoweringContract verifies the exact direct
// helper boundary and keeps unsupported shapes explicit at runtime. The
// helpers validate fixed-width unsigned values and report overflow instead of
// silently wrapping an lcm result into the declared width.
func TestNativeCoreIntegerMathLoweringContract(t *testing.T) {
	if got := nativeModuleMemberStatus("std/math", "gcd"); got != BackendImplemented {
		t.Fatalf("std/math.gcd inventory status = %q, want implemented", got)
	}
	if got := nativeModuleMemberStatus("std/math", "lcm"); got != BackendImplemented {
		t.Fatalf("std/math.lcm inventory status = %q, want implemented", got)
	}
	program, err := Parse("<nativeCore-native-integer-math-contract>", `import "std/math" as math
emit math.gcd(u8(48), u16(18))
emit math.lcm(u8(12), u16(18))
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"static OValue ostd_math_gcd(OValue left, OValue right)",
		"static OValue ostd_math_lcm(OValue left, OValue right)",
		"ostd_math_gcd(",
		"ostd_math_lcm(",
	} {
		if !strings.Contains(artifact.Source, marker) {
			t.Fatalf("generated integer math artifact omitted %q", marker)
		}
	}
	mainSource := artifact.Source
	if mainStart := strings.Index(mainSource, "int main(void)"); mainStart >= 0 {
		mainSource = mainSource[mainStart:]
	}
	if strings.Contains(mainSource, "ostd_dispatch(\"std/math\"") || strings.Contains(mainSource, " = ocall(") {
		t.Fatalf("integer math crossed an indirect native boundary:\n%s", artifact.Source)
	}
}

// TestNativeCoreComplexF32LoweringContract checks that an explicit width
// is retained in the IR and lowered to the tagged runtime constructor. The
// constructor stores the already-rounded f32 components in the existing
// OComplex payload; no separate allocation or host capability is introduced.
func TestNativeCoreComplexF32LoweringContract(t *testing.T) {
	program, err := Parse("<nativeCore-native-complex-f32-ir>", `let literal = 1.25f32i
let constructed = complex(f32(1.25), f32(2.5))
emit literal
emit constructed
`)
	if err != nil {
		t.Fatal(err)
	}
	ir, err := NewRuntime().BuildIR(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	var constants int
	for _, fn := range ir.Functions {
		for _, block := range fn.Blocks {
			for _, instr := range block.Instrs {
				if instr.Op == "const" && instr.Value.kind == kindComplex && instr.Value.fbits == 32 {
					constants++
				}
			}
		}
	}
	if constants != 1 {
		t.Fatalf("IR complex<f32> literal constants = %d, want 1", constants)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("scalar complex<f32> lowering unexpectedly defines a host capability")
	}
	if !strings.Contains(artifact.Source, "ocomplexbits(0, 1.25, 32)") || !strings.Contains(artifact.Source, "ocomplexcall((OValue[]){") {
		t.Fatalf("generated C omitted width-preserving complex constructors:\n%s", artifact.Source)
	}
}

// TestNativeCoreComplexUnsupportedShapeDiagnostic keeps a neighboring
// unsupported complex shape explicit. Complex arguments to scalar-only native
// math are rejected during C lowering with the source span of the offending
// call; they do not fall back to the interpreter or to a subprocess.
func TestNativeCoreComplexUnsupportedShapeDiagnostic(t *testing.T) {
	program, err := Parse("nativeCore-native-complex-shape.osr", "import \"std/math\" as math\nemit math.sqrt(1.0f32i)\n")
	if err != nil {
		t.Fatal(err)
	}
	_, err = CompileC(context.Background(), NewRuntime(), program)
	if err == nil {
		t.Fatal("native complex scalar-math shape unexpectedly compiled")
	}
	for _, want := range []string{
		"nativeCore-native-complex-shape.osr:2:6",
		"complex sqrt is not supported yet",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("diagnostic %q missing %q", err, want)
		}
	}
}

// TestNativeCoreDualScalarParity closes the inline dual-number ABI for
// construction, mixed-scalar arithmetic, unary negation, rendering, and
// projections. The differential harness also proves that generated C and its
// executable agree with the interpreter and VM without an evaluator fallback.
func TestNativeCoreDualScalarParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeCore-native-dual-scalar",
		Source: `let x = dual(3, 1)
let y = dual(2, 0.5)
emit x
emit x + y
emit x * y
emit x / y
emit -x
emit 2 + x
emit x / 2
emit primal(x * y)
emit tangent(x * y)
`,
	})
}

// TestNativeCoreDualScalarLoweringOwnershipAndCapabilityContract checks
// the exact tagged ABI boundary. A dual is two inline doubles: construction,
// projection, arithmetic, and unary negation do not allocate, retain input
// pointers, or define a host capability.
func TestNativeCoreDualScalarLoweringOwnershipAndCapabilityContract(t *testing.T) {
	program, err := Parse("<nativeCore-native-dual-contract>", `emit dual(3, 1) + 2
emit -dual(3, 1)
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"static OValue odual(double primal, double tangent)",
		"static OValue odualcall(const OValue *args, size_t count)",
		"static OValue odoubledualbin(const char *op, OValue a, OValue b)",
		"static OValue ounary(const char *op, OValue x)",
		`case O_DUAL: fputs("dual(",stdout);`,
		"oadd(",
		"ounary(\"-\"",
	} {
		if !strings.Contains(artifact.Source, marker) {
			t.Fatalf("generated dual ABI omitted %q", marker)
		}
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("scalar dual lowering unexpectedly defines a host capability")
	}
	for _, helper := range []string{
		"static OValue odual(double primal, double tangent)",
		"static OValue ounary(const char *op, OValue x)",
	} {
		start := strings.Index(artifact.Source, helper)
		if start < 0 {
			t.Fatalf("generated dual helper %q is missing", helper)
		}
		end := strings.Index(artifact.Source[start+len(helper):], "\nstatic OValue ")
		if end < 0 {
			t.Fatalf("generated dual helper boundary after %q is missing", helper)
		}
		if strings.Contains(artifact.Source[start:start+len(helper)+end], "malloc(") {
			t.Fatalf("dual helper %q allocates or retains input storage", helper)
		}
	}
}

// TestNativeCoreDualScalarArityDiagnostic keeps the source-level error
// contract explicit. Arity is known during lowering, so malformed dual calls
// are rejected with the call span instead of reaching a native subprocess.
func TestNativeCoreDualScalarArityDiagnostic(t *testing.T) {
	program, err := Parse("nativeCore-native-dual-arity.osr", "emit dual(1)\n")
	if err != nil {
		t.Fatal(err)
	}
	_, err = CompileC(context.Background(), NewRuntime(), program)
	if err == nil {
		t.Fatal("invalid dual arity unexpectedly compiled")
	}
	for _, want := range []string{
		"nativeCore-native-dual-arity.osr:1:6",
		"dual expects 2 arguments",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("diagnostic %q missing %q", err, want)
		}
	}
}

// TestNativeCoreDualScalarRuntimeErrorContract checks that a runtime dual
// failure remains a native structured failure with the same public message as
// the reference runtime. The source is valid, so the division-by-zero branch
// must be represented by the generated executable rather than rejected or
// evaluated by another backend.
func TestNativeCoreDualScalarRuntimeErrorContract(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available for native dual error evidence")
	}
	program, err := Parse("nativeCore-native-dual-runtime-error.osr", `let x = dual(1, 1)
emit x / dual(0, 1)
`)
	if err != nil {
		t.Fatal(err)
	}
	_, interpreterErr := NewRuntime().Run(t.Context(), program)
	if interpreterErr == nil || !strings.Contains(interpreterErr.Error(), "division by zero") {
		t.Fatalf("interpreter dual runtime error = %v", interpreterErr)
	}
	artifact, err := CompileC(t.Context(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "nativeCore-native-dual-runtime-error")
	if err := CompileCExecutable(t.Context(), artifact.Source, executable); err != nil {
		t.Fatal(err)
	}
	output, nativeErr := exec.Command(executable).CombinedOutput()
	if nativeErr == nil || !strings.Contains(string(output), "division by zero") {
		t.Fatalf("native dual runtime error = %v, output %q", nativeErr, output)
	}
}
