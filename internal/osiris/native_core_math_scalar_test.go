package osiris

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNativeCoreMathScalarParity covers the width-preserving real scalar
// subset of atan2, exp2, and trunc through the interpreter, VM, generated C,
// and native executable. Qualified std/math calls and their builtin spellings
// deliberately share the same direct emitter path.
func TestNativeCoreMathScalarParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeCore-native-math-scalars",
		Source: `import "std/math" as math
emit atan2(0.0, -1.0)
emit math.atan2(1.0f32, 2.0f32)
emit exp2(10.0)
emit math.exp2(3.0f32)
emit trunc(-3.75)
emit math.trunc(-3.75f32)
`,
	})
}

// TestNativeCoreMathScalarLoweringOwnershipAndCapabilityContract checks that
// the three scalar operations are emitted as borrowed, allocation-free C
// expressions with no module dispatcher or host capability boundary.
func TestNativeCoreMathScalarLoweringOwnershipAndCapabilityContract(t *testing.T) {
	program, err := Parse("<native-math-scalars-contract>", `import "std/math" as math
emit math.atan2(1.0, 2.0)
emit math.exp2(3.0)
emit math.trunc(-3.75)
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"atan2(", "exp2(", "trunc("} {
		if !strings.Contains(artifact.Source, marker) {
			t.Fatalf("generated scalar math artifact omitted direct C call %q", marker)
		}
	}
	if strings.Contains(artifact.Source, `ostd_dispatch("std/math"`) || strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("scalar math unexpectedly crossed a dispatcher or host capability boundary")
	}
	mainSource := artifact.Source
	if start := strings.Index(mainSource, "int main(void)"); start >= 0 {
		mainSource = mainSource[start:]
	}
	if strings.Contains(mainSource, "malloc(") || strings.Contains(mainSource, "retain(") {
		t.Fatal("scalar math main retains or allocates input storage")
	}
	for _, member := range []string{"atan2", "exp2", "trunc"} {
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/math/"+member)
		if !ok || entry.Native != BackendImplemented {
			t.Fatalf("std/math.%s inventory entry = %+v, want implemented native status", member, entry)
		}
	}
}

func TestNativeCoreMathScalarErrorsRemainStructured(t *testing.T) {
	program, err := Parse("<native-math-scalar-errors>", "emit exp2(128.0f32)\n")
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
			if err := run.fn(); err == nil || !strings.Contains(err.Error(), "exp2 overflow") {
				t.Fatalf("%s error = %v, want structured exp2 overflow", run.name, err)
			}
		})
	}
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available for native scalar error evidence")
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "native-math-scalar-errors")
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(executable).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "osiris native error: exp2 overflow") {
		t.Fatalf("native scalar overflow = %v, output %q; want structured exp2 overflow", err, out)
	}
}

func TestNativeCoreMathScalarArityDiagnostic(t *testing.T) {
	program, err := Parse("native-math-scalar-arity.osr", "emit atan2(1.0)\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileC(context.Background(), NewRuntime(), program); err == nil || !strings.Contains(err.Error(), "atan2 expects 2 argument") {
		t.Fatalf("invalid atan2 arity compile error = %v, want native arity diagnostic", err)
	}
}
