package osiris

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeCLowersLazyToCompiledOnceThunk(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("C compiler unavailable")
	}
	program, err := Parse("<native-lazy>", `
let delayed Lazy<u32> = lazy { emit "once"; u32(40) + u32(2) }
emit force(delayed)
emit force(delayed)
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"olazy(", "oforce(", "lazy_value"} {
		if !strings.Contains(artifact.Source, required) {
			t.Fatalf("native artifact is missing %q", required)
		}
	}
	executable := filepath.Join(t.TempDir(), "native-lazy")
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(executable).CombinedOutput()
	if err != nil {
		t.Fatalf("native lazy executable: %v\n%s", err, output)
	}
	if got, want := string(output), "\"once\"\n42u32\n42u32\n"; got != want {
		t.Fatalf("native output = %q, want %q", got, want)
	}
}

func TestVMExplicitLazyCompilesAndForcesExactlyOnce(t *testing.T) {
	program, err := Parse("<vm-lazy>", `
let delayed = lazy { tick() }
emit force(delayed)
emit force(delayed)
`)
	if err != nil {
		t.Fatal(err)
	}

	runtime := NewRuntime()
	calls := 0
	runtime.Builtins["tick"] = func(args []Value) (Value, error) {
		calls++
		return uintValue(42, 64), nil
	}
	outputs, err := runtime.RunVM(context.Background(), program)
	if err != nil {
		t.Fatalf("RunVM: %v", err)
	}
	if got, want := valuesOutput(outputs), "42u64\n42u64\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if calls != 1 {
		t.Fatalf("lazy body calls = %d, want 1", calls)
	}
}

func TestVMLazySharesOneConcurrentForceAndCachesFailure(t *testing.T) {
	program, err := Parse("<vm-lazy-once>", `
let delayed = lazy { tick() }
emit parallel(force(delayed), force(delayed))
let broken = lazy { fail() }
emit try { force(broken) } catch err { "caught" }
emit try { force(broken) } catch err { "caught" }
`)
	if err != nil {
		t.Fatal(err)
	}

	runtime := NewRuntime()
	var tickCalls, failCalls int
	runtime.Builtins["tick"] = func(args []Value) (Value, error) {
		tickCalls++
		return uintValue(9, 64), nil
	}
	runtime.Builtins["fail"] = func(args []Value) (Value, error) {
		failCalls++
		return nullValue(), errors.New("lazy failure")
	}
	outputs, err := runtime.RunVM(context.Background(), program)
	if err != nil {
		t.Fatalf("RunVM: %v", err)
	}
	if got, want := valuesOutput(outputs), "[9u64, 9u64]\n\"caught\"\n\"caught\"\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if tickCalls != 1 || failCalls != 1 {
		t.Fatalf("lazy executions = tick %d, fail %d; want one each", tickCalls, failCalls)
	}
}
