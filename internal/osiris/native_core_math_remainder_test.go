package osiris

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNativeCoreMathRemainderParity compares the pure floating remainder
// family through the interpreter, VM, generated C, and native executable.
// The default and explicit f32 forms exercise both result-width boundaries;
// builtin and qualified std/math calls share the same direct emitter path.
func TestNativeCoreMathRemainderParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeCore-native-math-remainder",
		Source: `import "std/math" as math
emit fmod(5.5, 2.0)
emit math.fmod(5.5f32, 2.0f32)
emit remainder(5.5, 2.0)
emit math.remainder(5.5f32, 2.0f32)
emit fma(2.0, 3.0, 4.0)
emit math.fma(1.25f32, 2.0f32, -0.5f32)
`,
	})
}

// TestNativeCoreMathRemainderLoweringOwnershipAndCapabilityContract checks
// that each operation is emitted as a borrowed, allocation-free scalar C
// call, with no module dispatcher, evaluator fallback, or host capability.
func TestNativeCoreMathRemainderLoweringOwnershipAndCapabilityContract(t *testing.T) {
	program, err := Parse("<native-math-remainder-contract>", `import "std/math" as math
emit math.fmod(5.5, 2.0)
emit math.remainder(5.5, 2.0)
emit math.fma(2.0, 3.0, 4.0)
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"fmod(", "remainder(", "fma("} {
		if !strings.Contains(artifact.Source, marker) {
			t.Fatalf("generated scalar math artifact omitted direct C call %q", marker)
		}
	}
	mainSource := artifact.Source
	if start := strings.Index(mainSource, "int main(void)"); start >= 0 {
		mainSource = mainSource[start:]
	}
	if strings.Contains(mainSource, `ostd_dispatch("std/math"`) || strings.Contains(mainSource, "ocall(") || strings.Contains(mainSource, "#define OSIRIS_CAP_") {
		t.Fatal("scalar remainder math crossed an indirect or host capability boundary")
	}
	if strings.Contains(mainSource, "malloc(") || strings.Contains(mainSource, "retain(") {
		t.Fatal("scalar remainder math main retains or allocates input storage")
	}
	for _, member := range []string{"fmod", "remainder", "fma"} {
		if got := nativeBuiltinStatus(member); got != BackendImplemented {
			t.Fatalf("builtin %s inventory status = %q, want implemented", member, got)
		}
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/math/"+member)
		if !ok || entry.Native != BackendImplemented {
			t.Fatalf("std/math.%s inventory entry = %+v, want implemented native status", member, entry)
		}
	}
}

func TestNativeCoreMathRemainderZeroDivisorsRemainStructured(t *testing.T) {
	for _, expression := range []string{
		"fmod(1.0, 0.0)",
		"remainder(1.0, 0.0)",
	} {
		t.Run(strings.ReplaceAll(expression, "(", "-"), func(t *testing.T) {
			program, err := Parse("<native-math-remainder-error>", "emit "+expression+"\n")
			if err != nil {
				t.Fatal(err)
			}
			rt := NewRuntime()
			for _, run := range []struct {
				name string
				fn   func() error
			}{
				{name: "interpreter", fn: func() error { _, err := rt.Run(context.Background(), program); return err }},
				{name: "vm", fn: func() error { _, err := rt.RunVM(context.Background(), program); return err }},
			} {
				t.Run(run.name, func(t *testing.T) {
					if err := run.fn(); err == nil {
						t.Fatal("zero divisor unexpectedly succeeded")
					} else {
						var record *OsirisError
						if !errors.As(err, &record) || record.kind != ErrorKindInvalidArgument || !strings.Contains(record.message, "divisor must be non-zero") {
							t.Fatalf("error = %T %v, want structured zero-divisor error", err, err)
						}
					}
				})
			}
			if _, err := exec.LookPath("cc"); err != nil {
				t.Skip("cc not available for native zero-divisor evidence")
			}
			artifact, err := CompileC(context.Background(), NewRuntime(), program)
			if err != nil {
				t.Fatal(err)
			}
			executable := filepath.Join(t.TempDir(), "native-math-remainder-error")
			if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command(executable).CombinedOutput()
			operation := strings.Split(expression, "(")[0]
			want := operation + " divisor must be non-zero"
			if err == nil || !strings.Contains(string(out), "osiris native error: "+want) {
				t.Fatalf("native zero-divisor result = %v, output %q; want structured %s error", err, out, want)
			}
		})
	}
}

func TestNativeCoreMathRemainderArityDiagnostic(t *testing.T) {
	for _, call := range []string{
		"fmod(1.0)",
		"remainder(1.0)",
		"fma(1.0, 2.0)",
	} {
		program, err := Parse("native-math-remainder-arity.osr", "emit "+call+"\n")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := CompileC(context.Background(), NewRuntime(), program); err == nil || !strings.Contains(err.Error(), "expects") {
			t.Fatalf("invalid %s compile error = %v, want native arity diagnostic", call, err)
		}
	}
}

func TestNativeCoreMathRemainderShapeDiagnostic(t *testing.T) {
	for _, call := range []string{
		"fmod(1.0f32, 2.0)",
		"remainder(1.0f32, 2.0)",
		"fma(1.0f32, 2.0f32, 3.0)",
	} {
		program, err := Parse("native-math-remainder-shape.osr", "emit "+call+"\n")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := CompileC(context.Background(), NewRuntime(), program); err == nil || !strings.Contains(err.Error(), "matching-width floating scalar inputs") {
			t.Fatalf("unsupported %s compile error = %v, want explicit scalar-shape diagnostic", call, err)
		}
	}
}
