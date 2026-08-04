package osiris

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestVMParityCorpus keeps the VM contract in a dedicated corpus instead of
// relying only on backend-specific unit tests. Each case is executed through
// the same Runtime first by the interpreter and then by the VM, which also
// catches accidental reuse of interpreter-owned runtime state.
func TestVMParityCorpus(t *testing.T) {
	for _, fixture := range []struct {
		name string
		path string
	}{
		{name: "language-contract", path: "testdata/vm-parity/language-contract.osr"},
		{name: "cleanup-and-concurrency", path: "testdata/vm-parity/cleanup-and-concurrency.osr"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			source, err := os.ReadFile(fixture.path)
			if err != nil {
				t.Fatal(err)
			}
			prog, err := Parse(fixture.path, string(source))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			rt := NewRuntime()
			interpreterValues, interpreterErr := rt.Run(context.Background(), prog)
			vmValues, vmErr := rt.RunVM(context.Background(), prog)
			assertDifferentialEqual(t, differentialFixture{Name: fixture.name, Source: string(source)},
				differentialResult{Backend: "interpreter", Phase: "run", Output: valuesOutput(interpreterValues), Err: interpreterErr},
				differentialResult{Backend: "vm", Phase: "run", Output: valuesOutput(vmValues), Err: vmErr})
		})
	}
}

func TestVMErrorParityRetainsSourceSpan(t *testing.T) {
	prog, err := Parse("vm-span.osr", "emit uint(\"not-a-number\")\n")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	_, interpreterErr := rt.Run(context.Background(), prog)
	_, vmErr := rt.RunVM(context.Background(), prog)
	if interpreterErr == nil || vmErr == nil {
		t.Fatalf("interpreter error = %v, vm error = %v; both must fail", interpreterErr, vmErr)
	}
	if interpreterErr.Error() != vmErr.Error() {
		t.Fatalf("error mismatch:\ninterpreter: %v\nvm: %v", interpreterErr, vmErr)
	}
	var diagnostic *OsirisError
	if !errors.As(vmErr, &diagnostic) || diagnostic.location == nil || diagnostic.location.Span.start.line != 1 {
		t.Fatalf("vm error lacks stable source span: %#v", vmErr)
	}
}

func TestVMClosableCleanupParity(t *testing.T) {
	source := `trait Closable { func close() bool; }
struct Resource { label string }
impl Closable for Resource { func close() bool => { emit self.label; true } }
let resource = Resource { label: "released" }
defer { close(resource) }
emit "body"
`
	prog, err := Parse("vm-closable.osr", source)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	interpreterValues, interpreterErr := rt.Run(context.Background(), prog)
	vmValues, vmErr := rt.RunVM(context.Background(), prog)
	assertDifferentialEqual(t, differentialFixture{Name: "closable-cleanup", Source: source},
		differentialResult{Backend: "interpreter", Phase: "run", Output: valuesOutput(interpreterValues), Err: interpreterErr},
		differentialResult{Backend: "vm", Phase: "run", Output: valuesOutput(vmValues), Err: vmErr})
}

func TestVMCancellationCleanupParity(t *testing.T) {
	prog, err := Parse("vm-cancellation.osr", `defer { emit "cleanup" }
emit cancel_now()
`)
	if err != nil {
		t.Fatal(err)
	}
	run := func(execute func(context.Context, *Runtime, *Program) ([]Value, error)) differentialResult {
		rt := NewRuntime()
		ctx, cancel := context.WithCancel(context.Background())
		rt.Builtins["cancel_now"] = func([]Value) (Value, error) {
			cancel()
			return nullValue(), nil
		}
		values, runErr := execute(ctx, rt, prog)
		return differentialResult{Output: valuesOutput(values), Err: runErr}
	}
	interpreter := run(func(ctx context.Context, rt *Runtime, prog *Program) ([]Value, error) {
		return rt.Run(ctx, prog)
	})
	vm := run(func(ctx context.Context, rt *Runtime, prog *Program) ([]Value, error) {
		return rt.RunVM(ctx, prog)
	})
	if interpreter.Output != vm.Output || interpreter.Output != "null\n\"cleanup\"\n" {
		t.Fatalf("cancellation outputs differ: interpreter=%q vm=%q", interpreter.Output, vm.Output)
	}
	if interpreter.Err == nil || vm.Err == nil || !strings.Contains(interpreter.Err.Error(), "context canceled") || !strings.Contains(vm.Err.Error(), "context canceled") {
		t.Fatalf("cancellation errors differ: interpreter=%v vm=%v", interpreter.Err, vm.Err)
	}
}

func TestVMSourceModuleDoesNotReuseInterpreterModule(t *testing.T) {
	dir := t.TempDir()
	modulePath := filepath.Join(dir, "helper.osr")
	mainPath := filepath.Join(dir, "main.osr")
	moduleSource := `package helper 1.0.0
export value
func value() u64 => 42
`
	mainSource := "import " + quoteString(modulePath) + " as helper\nemit helper.value()\n"
	if err := os.WriteFile(modulePath, []byte(moduleSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainPath, []byte(mainSource), 0o644); err != nil {
		t.Fatal(err)
	}
	prog, err := Parse(mainPath, mainSource)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	interpreterValues, err := rt.Run(context.Background(), prog)
	if err != nil || valuesOutput(interpreterValues) != "42u64\n" {
		t.Fatalf("interpreter output = %v, error = %v", interpreterValues, err)
	}

	// Exercise the VM module loader after the interpreter has populated the
	// Runtime module cache. A VM source module must contain VM bodies even when
	// the same path has already been interpreted.
	runner := &vmRunner{
		rt:          rt,
		ctx:         context.Background(),
		source:      mainPath,
		vmModules:   make(map[string]Value),
		vmModuleMu:  &sync.Mutex{},
		frame:       &vmFrame{},
		emitAllowed: true,
	}
	module, err := runner.loadModuleVM(context.Background(), modulePath, mainPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	mod, err := module.AsModule()
	if err != nil {
		t.Fatal(err)
	}
	function, ok := mod.members["value"]
	if !ok || function.kind != kindFunction || function.fn == nil || function.fn.vmBody == nil {
		t.Fatalf("module function was not compiled for VM: %#v", function)
	}

	vmValues, vmErr := rt.RunVM(context.Background(), prog)
	if vmErr != nil || valuesOutput(vmValues) != valuesOutput(interpreterValues) {
		t.Fatalf("vm output = %v, error = %v; interpreter output = %v", vmValues, vmErr, interpreterValues)
	}
}

func TestVMConcurrentModuleCacheIsRaceSafe(t *testing.T) {
	dir := t.TempDir()
	modulePath := filepath.Join(dir, "helper.osr")
	mainPath := filepath.Join(dir, "main.osr")
	moduleSource := `package helper 1.0.0
export value
func value() u64 => 42
`
	mainSource := "emit parallel({ import " + quoteString(modulePath) + " as helper; helper.value() }, { import " + quoteString(modulePath) + " as helper; helper.value() })\n"
	if err := os.WriteFile(modulePath, []byte(moduleSource), 0o644); err != nil {
		t.Fatal(err)
	}
	prog, err := Parse(mainPath, mainSource)
	if err != nil {
		t.Fatal(err)
	}
	values, err := NewRuntime().RunVM(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || !strings.Contains(values[0].String(), "42u64") {
		t.Fatalf("parallel module output = %v", values)
	}
}

func TestVMSpawnSharesSourceModuleCacheWithParent(t *testing.T) {
	dir := t.TempDir()
	modulePath := filepath.Join(dir, "counter.osr")
	mainPath := filepath.Join(dir, "main.osr")
	moduleSource := `package counter 1.0.0
export next
var count = 0
func next() u64 => { count = count + 1; count }
`
	mainSource := "import " + quoteString(modulePath) + " as counter\n" +
		"emit counter.next()\n" +
		"emit await(spawn({ import " + quoteString(modulePath) + " as taskCounter; taskCounter.next() }))\n"
	if err := os.WriteFile(modulePath, []byte(moduleSource), 0o644); err != nil {
		t.Fatal(err)
	}
	prog, err := Parse(mainPath, mainSource)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	interpreterValues, interpreterErr := rt.Run(context.Background(), prog)
	vmValues, vmErr := rt.RunVM(context.Background(), prog)
	assertDifferentialEqual(t, differentialFixture{Name: "spawn-shared-module-cache", Source: mainSource},
		differentialResult{Backend: "interpreter", Phase: "run", Output: valuesOutput(interpreterValues), Err: interpreterErr},
		differentialResult{Backend: "vm", Phase: "run", Output: valuesOutput(vmValues), Err: vmErr})
	if vmErr != nil {
		t.Fatal(vmErr)
	}
	if got, want := valuesOutput(vmValues), "1u64\n2u64\n"; got != want {
		t.Fatalf("VM output = %q, want %q", got, want)
	}
}
