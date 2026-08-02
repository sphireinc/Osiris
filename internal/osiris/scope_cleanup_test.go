package osiris

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDeferRunsInLIFOOrderAtScopeExit(t *testing.T) {
	source := `
defer { emit "outer" }
defer { emit "inner" }
emit "body"
`
	want := []string{`"body"`, `"inner"`, `"outer"`}
	for _, run := range []struct {
		name string
		fn   func(*Runtime, *Program) ([]Value, error)
	}{
		{name: "interpreter", fn: func(rt *Runtime, prog *Program) ([]Value, error) {
			return rt.Run(context.Background(), prog)
		}},
		{name: "vm", fn: func(rt *Runtime, prog *Program) ([]Value, error) {
			return rt.RunVM(context.Background(), prog)
		}},
	} {
		t.Run(run.name, func(t *testing.T) {
			prog, err := Parse("<defer>", source)
			if err != nil {
				t.Fatal(err)
			}
			outputs, err := run.fn(NewRuntime(), prog)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, len(outputs))
			for i, output := range outputs {
				got[i] = output.String()
			}
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Fatalf("outputs = %v, want %v", got, want)
			}
		})
	}
}

func TestDeferRunsWhenTheBodyIsCancelled(t *testing.T) {
	rt := NewRuntime()
	prog, err := Parse("<defer-cancel>", `
defer { emit "cleanup" }
emit cancel_now()
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range []struct {
		name string
		fn   func(context.Context, *Program) ([]Value, error)
	}{
		{name: "interpreter", fn: rt.Run},
		{name: "vm", fn: rt.RunVM},
	} {
		t.Run(run.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			rt.Builtins["cancel_now"] = func(args []Value) (Value, error) {
				cancel()
				return nullValue(), nil
			}
			outputs, err := run.fn(ctx, prog)
			if err == nil || !strings.Contains(err.Error(), "context canceled") {
				t.Fatalf("error = %v, want context cancellation", err)
			}
			if len(outputs) == 0 || outputs[len(outputs)-1].String() != `"cleanup"` {
				t.Fatalf("outputs = %v, want cleanup as the final output after cancellation", outputs)
			}
		})
	}
}

func TestDeferPreservesBodyErrorWhenCleanupAlsoFails(t *testing.T) {
	prog, err := Parse("<defer-error>", `
defer { missing_cleanup() }
emit missing_body()
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range []struct {
		name string
		fn   func(*Runtime, *Program) ([]Value, error)
	}{
		{name: "interpreter", fn: func(rt *Runtime, p *Program) ([]Value, error) {
			return rt.Run(context.Background(), p)
		}},
		{name: "vm", fn: func(rt *Runtime, p *Program) ([]Value, error) {
			return rt.RunVM(context.Background(), p)
		}},
	} {
		t.Run(run.name, func(t *testing.T) {
			_, err := run.fn(NewRuntime(), prog)
			if err == nil || !strings.Contains(err.Error(), "missing_body") || !strings.Contains(err.Error(), "missing_cleanup") {
				t.Fatalf("error = %v, want body and cleanup failures", err)
			}
		})
	}
}

func TestDeferRunsWhenAChildTaskTerminates(t *testing.T) {
	source := `
let task = spawn({ defer { release_resource() }; fail_task() })
emit try { await(task) } catch err { "task-failed" }
`
	prog, err := Parse("<defer-task>", source)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range []struct {
		name string
		fn   func(*Runtime, *Program) ([]Value, error)
	}{
		{name: "interpreter", fn: func(rt *Runtime, p *Program) ([]Value, error) {
			return rt.Run(context.Background(), p)
		}},
		{name: "vm", fn: func(rt *Runtime, p *Program) ([]Value, error) {
			return rt.RunVM(context.Background(), p)
		}},
	} {
		t.Run(run.name, func(t *testing.T) {
			rt := NewRuntime()
			var released atomic.Int32
			rt.Builtins["release_resource"] = func(args []Value) (Value, error) {
				released.Add(1)
				return nullValue(), nil
			}
			rt.Builtins["fail_task"] = func(args []Value) (Value, error) {
				return nullValue(), fmt.Errorf("task failed")
			}
			values, err := run.fn(rt, prog)
			if err != nil {
				t.Fatal(err)
			}
			if len(values) != 1 || values[0].String() != `"task-failed"` {
				t.Fatalf("outputs = %v, want task-failed", values)
			}
			if got := released.Load(); got != 1 {
				t.Fatalf("cleanup calls = %d, want exactly one", got)
			}
		})
	}
}

func TestDeferControlFlowMatrixAndCapturedValues(t *testing.T) {
	source := `
func returned() {
  defer { emit "return-cleanup" }
  return "return-value"
}
var count = 0
while count < 2 {
  defer { emit "continue-cleanup" }
  count = count + 1
  continue
}
while true {
  defer { emit "break-cleanup" }
  break
}
var captured = "before"
defer { emit captured }
captured = "after"
emit returned()
emit try { defer { emit "try-cleanup" }; missing_body() } catch err { "caught" }
`
	prog, err := Parse("<defer-matrix>", source)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`"continue-cleanup"`, `"continue-cleanup"`, `"break-cleanup"`,
		`"return-cleanup"`, `"return-value"`, `"try-cleanup"`, `"caught"`, `"before"`,
	}
	for _, run := range []struct {
		name string
		fn   func(*Runtime, *Program) ([]Value, error)
	}{
		{name: "interpreter", fn: func(rt *Runtime, p *Program) ([]Value, error) {
			return rt.Run(context.Background(), p)
		}},
		{name: "vm", fn: func(rt *Runtime, p *Program) ([]Value, error) {
			return rt.RunVM(context.Background(), p)
		}},
	} {
		t.Run(run.name, func(t *testing.T) {
			values, err := run.fn(NewRuntime(), prog)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, len(values))
			for i, value := range values {
				got[i] = value.String()
			}
			if strings.Join(got, "|") != strings.Join(want, "|") {
				t.Fatalf("outputs = %v, want %v", got, want)
			}
		})
	}
}

func TestCloseDispatchesTheClosableResourceProtocol(t *testing.T) {
	prog, err := Parse("<closable-resource>", `
trait Closable { func close() bool; }
struct Resource { label string }
impl Closable for Resource { func close() bool => { emit self.label; true } }
let resource = Resource { label: "released" }
defer { close(resource) }
emit "body"
`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{`"body"`, `"released"`}
	for _, run := range []struct {
		name string
		fn   func(*Runtime, *Program) ([]Value, error)
	}{
		{name: "interpreter", fn: func(rt *Runtime, p *Program) ([]Value, error) {
			return rt.Run(context.Background(), p)
		}},
		{name: "vm", fn: func(rt *Runtime, p *Program) ([]Value, error) {
			return rt.RunVM(context.Background(), p)
		}},
	} {
		t.Run(run.name, func(t *testing.T) {
			values, err := run.fn(NewRuntime(), prog)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, len(values))
			for i, value := range values {
				got[i] = value.String()
			}
			if strings.Join(got, "|") != strings.Join(want, "|") {
				t.Fatalf("outputs = %v, want %v", got, want)
			}
		})
	}
}

func TestNativeDeferLowersCleanupEdge(t *testing.T) {
	prog, err := Parse("native-defer.osr", `defer { emit "cleanup" }
emit "body"
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(artifact.Source, "cleanup") || !strings.Contains(artifact.Source, "ocall(") {
		t.Fatalf("generated C does not contain cleanup edge:\n%s", artifact.Source)
	}
}

func TestNativeDeferExecutesOnNormalAndReturnEdges(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("native-cleanup-edges.osr", `
func finish() { defer { emit "return-cleanup" }; return "done" }
defer { emit "scope-cleanup" }
emit finish()
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "native-cleanup-edges")
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatalf("compile executable: %v\n%s", err, artifact.Source)
	}
	output, err := exec.Command(executable).CombinedOutput()
	if err != nil {
		t.Fatalf("native cleanup executable: %v\n%s", err, output)
	}
	want := "\"return-cleanup\"\n\"done\"\n\"scope-cleanup\"\n"
	if !bytes.Equal(output, []byte(want)) {
		t.Fatalf("native cleanup output = %q, want %q", output, want)
	}
}

func TestCleanupIRAndOptimizerPreserveThunkOwnership(t *testing.T) {
	prog, err := Parse("<cleanup-ir>", `
let marker = "registered"
defer { emit marker }
emit "body"
`)
	if err != nil {
		t.Fatal(err)
	}
	optimized, _ := OptimizeProgramWithReport(prog)
	for name, p := range map[string]*Program{"source": prog, "optimized": optimized} {
		ir, err := NewRuntime().BuildIR(context.Background(), p)
		if err != nil {
			t.Fatalf("%s IR: %v", name, err)
		}
		if _, err := VerifySSAProgram(ir); err != nil {
			t.Fatalf("%s SSA: %v", name, err)
		}
		cleanupCount := 0
		for _, fn := range ir.Functions {
			for _, block := range fn.Blocks {
				for _, instr := range block.Instrs {
					if instr.Op == "cleanup" {
						cleanupCount++
					}
				}
			}
		}
		if cleanupCount != 1 {
			t.Fatalf("%s cleanup count = %d, want exactly one\n%s", name, cleanupCount, FormatIRProgram(ir))
		}
	}
}
