package osiris

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const lazyLoopStackFixture = `
let i = 0
let total = 0
while i < 6000 {
  let total = total + i
  let i = i + 1
}
emit total
`

func TestLazyLoopBindingBoundedStackSubprocess(t *testing.T) {
	if mode := os.Getenv("OSIRIS_LAZY_STACK_HELPER"); mode != "" {
		debug.SetMaxStack(512 << 10)
		prog, err := Parse("<lazy-stack-helper>", lazyLoopStackFixture)
		if err != nil {
			t.Fatal(err)
		}
		var out []Value
		switch mode {
		case "interpreter":
			out, err = NewRuntime().Run(context.Background(), prog)
		case "vm-fallback":
			// RunVM intentionally delegates control-flow programs to the
			// interpreter session. This protects that public path until VM
			// control-flow bytecode is implemented independently.
			out, err = NewRuntime().RunVM(context.Background(), prog)
		default:
			t.Fatalf("unknown helper mode %q", mode)
		}
		if err != nil {
			t.Fatal(err)
		}
		if got, want := valuesOutput(out), "17997000u64\n"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
		return
	}

	for _, mode := range []string{"interpreter", "vm-fallback"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestLazyLoopBindingBoundedStackSubprocess$")
			cmd.Env = append(os.Environ(), "OSIRIS_LAZY_STACK_HELPER="+mode)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("bounded-stack helper failed: %v\n%s", err, output)
			}
		})
	}
}

func TestLazyLoopBindingParityAndScopeRetention(t *testing.T) {
	fixtures := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "default integer and two independent bindings",
			source: `
let i = 0
let sum = 0
let squares = 0
while i < 1000 {
  let sum = sum + i
  let squares = squares + i * i
  let i = i + 1
}
emit [sum, squares]
`,
			want: "[499500u64, 332833500u64]\n",
		},
		{
			name: "wrapping u32",
			source: `
let i = 0u32
let total = 4294967000u32
while i < 1000u32 {
  let total = total + i
  let i = i + 1u32
}
emit total
`,
			want: "499204u32\n",
		},
		{
			name: "f64 intermediate",
			source: `
let i = 0.0f64
let total = 0.0f64
while i < 1000.0f64 {
  let ratio = (i + 1.0f64) / 4.0f64
  let total = total + ratio
  let i = i + 1.0f64
}
emit total
`,
			want: "125125f64\n",
		},
		{
			name: "closure retains preceding iteration scope",
			source: `
let i = 0
let total = 0
func snapshot() => total
while i < 300 {
  func snapshot() => total
  let total = total + i
  let i = i + 1
}
emit snapshot()
`,
			want: "44551u64\n",
		},
		{
			name: "nested loops retain loop-carried scope",
			source: `
let outer = 0
let total = 0
while outer < 20 {
  let inner = 0
  while inner < 30 {
    let total = total + outer + inner
    let inner = inner + 1
  }
  let outer = outer + 1
}
emit total
`,
			want: "14400u64\n",
		},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			prog, err := Parse("<lazy-loop-parity>", fixture.source)
			if err != nil {
				t.Fatal(err)
			}
			for _, runner := range []struct {
				name string
				run  func(context.Context, *Program) ([]Value, error)
			}{
				{name: "interpreter", run: NewRuntime().Run},
				{name: "vm-control-flow-fallback", run: NewRuntime().RunVM},
			} {
				t.Run(runner.name, func(t *testing.T) {
					out, err := runner.run(context.Background(), prog)
					if err != nil {
						t.Fatal(err)
					}
					if got := valuesOutput(out); got != fixture.want {
						t.Fatalf("output = %q, want %q", got, fixture.want)
					}
				})
			}
		})
	}
}

func TestLazyUnusedExpressionsRemainUnevaluated(t *testing.T) {
	rt := NewRuntime()
	rt.SetTaskTrace(true)
	prog, err := Parse("<lazy-unused>", `
let unused_error = 1 / 0
let unused_task = spawn(41)
let unused_cycle = missing_cycle_name
func unused_recursive_cycle() => unused_recursive_cycle()
emit 7
`)
	if err != nil {
		t.Fatal(err)
	}
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if got := valuesOutput(out); got != "7u64\n" {
		t.Fatalf("output = %q", got)
	}
	if trace := rt.TaskTrace(); len(trace) != 0 {
		t.Fatalf("unused task became observable: %#v", trace)
	}
}

type cancelAfterChecksContext struct {
	checks      atomic.Int64
	cancelAfter int64
}

func (ctx *cancelAfterChecksContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (ctx *cancelAfterChecksContext) Done() <-chan struct{}       { return nil }
func (ctx *cancelAfterChecksContext) Value(any) any               { return nil }
func (ctx *cancelAfterChecksContext) Err() error {
	if ctx.checks.Add(1) >= ctx.cancelAfter {
		return context.Canceled
	}
	return nil
}

func TestLazyDeepEvaluationCancellationIsBoundedAndDeterministic(t *testing.T) {
	const source = `
let i = 0
let total = 0
while i < 5000 {
  let total = total + i
  let i = i + 1
}
`
	prog, err := Parse("<lazy-cancellation>", source)
	if err != nil {
		t.Fatal(err)
	}
	session := NewSession(NewRuntime(), context.Background(), prog.SourceName)
	if _, err := session.ExecProgram(prog, false, nil); err != nil {
		t.Fatal(err)
	}
	expression := &identExpr{name: "total"}
	key, err := session.ev.fingerprint(expression, session.scope)
	if err != nil {
		t.Fatal(err)
	}

	cancelContext := &cancelAfterChecksContext{cancelAfter: 4}
	session.ev.ctx = cancelContext
	if _, err := session.ev.evalWithKey(expression, session.scope, key); !errors.Is(err, context.Canceled) {
		t.Fatalf("deep evaluation error = %v, want context cancellation", err)
	}
	if checks := cancelContext.checks.Load(); checks != 4 {
		t.Fatalf("cancellation checks = %d, want deterministic fourth check", checks)
	}
}

func TestLazyDeepMemoizationEvaluatesPureLeafOnce(t *testing.T) {
	var calls atomic.Int64
	rt := NewRuntime()
	rt.Builtins["seed"] = func(args []Value) (Value, error) {
		if len(args) != 0 {
			return nullValue(), fmt.Errorf("seed expects no arguments")
		}
		calls.Add(1)
		return uintValue(1, 64), nil
	}
	prog, err := Parse("<lazy-memoization>", `
let i = 0
let total = seed()
while i < 1000 {
  let total = total + i
  let i = i + 1
}
emit total + total
emit total
`)
	if err != nil {
		t.Fatal(err)
	}
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := valuesOutput(out), "999002u64\n499501u64\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("pure leaf calls = %d, want 1", got)
	}
}

func TestLazyDeepConcurrentDemandSharesMemoizedValue(t *testing.T) {
	prog, err := Parse("<lazy-concurrent>", `
let i = 0
let total = 0
while i < 2000 {
  let total = total + i
  let i = i + 1
}
`)
	if err != nil {
		t.Fatal(err)
	}
	session := NewSession(NewRuntime(), context.Background(), prog.SourceName)
	if _, err := session.ExecProgram(prog, false, nil); err != nil {
		t.Fatal(err)
	}
	expression := &identExpr{name: "total"}
	const readers = 8
	start := make(chan struct{})
	results := make(chan Value, readers)
	errs := make(chan error, readers)
	var group sync.WaitGroup
	for range readers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			value, err := session.ev.eval(expression, session.scope)
			if err != nil {
				errs <- err
				return
			}
			results <- value
		}()
	}
	close(start)
	group.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Errorf("concurrent demand failed: %v", err)
	}
	for value := range results {
		if got := value.String(); got != "1999000u64" {
			t.Errorf("concurrent value = %s", got)
		}
	}
}

func TestLazyBindingCycleHasStableFingerprintAndDemandedError(t *testing.T) {
	expression := &identExpr{name: "cycle"}
	cycleScope := &scope{items: make(map[string]binding)}
	cycleScope.items["cycle"] = binding{expr: expression, env: cycleScope}
	evaluator := NewSession(NewRuntime(), context.Background(), "<lazy-cycle>").ev

	first, err := evaluator.fingerprint(expression, cycleScope)
	if err != nil {
		t.Fatal(err)
	}
	second, err := evaluator.fingerprint(expression, cycleScope)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("cycle fingerprints differ: %q, %q", first, second)
	}
	if _, err := evaluator.eval(expression, cycleScope); err == nil || !strings.Contains(err.Error(), "cyclic lazy binding dependency") {
		t.Fatalf("demanded cycle error = %v", err)
	}
}

func TestLazyDeepDemandedErrorRetainsStructuredLocation(t *testing.T) {
	const source = `
let i = 0
let total = 0
while i < 1000 {
  let total = total + i
  let i = i + 1
}
emit fail_after(total)
`
	for _, runner := range []struct {
		name string
		run  func(*Runtime, context.Context, *Program) ([]Value, error)
	}{
		{name: "interpreter", run: func(rt *Runtime, ctx context.Context, prog *Program) ([]Value, error) {
			return rt.Run(ctx, prog)
		}},
		{name: "vm-control-flow-fallback", run: func(rt *Runtime, ctx context.Context, prog *Program) ([]Value, error) {
			return rt.RunVM(ctx, prog)
		}},
	} {
		t.Run(runner.name, func(t *testing.T) {
			rt := NewRuntime()
			rt.Builtins["fail_after"] = func(args []Value) (Value, error) {
				if len(args) != 1 || args[0].String() != "499500u64" {
					return nullValue(), fmt.Errorf("unexpected argument")
				}
				return nullValue(), newOsirisError(ErrorKindInvalidArgument, "demanded lazy failure")
			}
			prog, err := Parse("<lazy-demanded-error>", source)
			if err != nil {
				t.Fatal(err)
			}
			_, err = runner.run(rt, context.Background(), prog)
			var record *OsirisError
			if !errors.As(err, &record) {
				t.Fatalf("error is not structured: %T %v", err, err)
			}
			if record.kind != ErrorKindInvalidArgument || !strings.Contains(record.message, "demanded lazy failure") {
				t.Fatalf("structured error = %#v", record)
			}
			if record.location == nil || record.location.Source != "<lazy-demanded-error>" || record.location.Span.start.line == 0 {
				t.Fatalf("structured location = %#v", record.location)
			}
		})
	}
}

func TestLazyLoopNativeIRAndGeneratedCStayRuntimeDriven(t *testing.T) {
	fixtures := []struct {
		name            string
		source          string
		minHeaderParams int
	}{
		{
			name: "u32",
			source: `
let i = 0u32
let total = 0u32
while i < 1000u32 {
  let total = total + i
  let i = i + 1u32
}
emit total
`,
			minHeaderParams: 2,
		},
		{
			name: "f64 with intermediate",
			source: `
let i = 0.0f64
let total = 0.0f64
while i < 1000.0f64 {
  let ratio = (i + 1.0f64) / 3.0f64
  let total = total + ratio % 2.0f64
  let i = i + 1.0f64
}
emit total
`,
			minHeaderParams: 3,
		},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			prog, err := Parse("<lazy-native-"+fixture.name+">", fixture.source)
			if err != nil {
				t.Fatal(err)
			}
			rt := NewRuntime()
			want, err := rt.Run(context.Background(), prog)
			if err != nil {
				t.Fatal(err)
			}
			ir, err := rt.BuildIR(context.Background(), prog)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := VerifySSAProgram(ir); err != nil {
				t.Fatalf("SSA verification: %v", err)
			}
			var header *IRBlock
			for _, block := range ir.Functions[0].Blocks {
				if strings.Contains(block.Name, "while.header") {
					header = block
					break
				}
			}
			if header == nil || len(header.Params) < fixture.minHeaderParams {
				t.Fatalf("loop header params = %#v, want at least %d\n%s", header, fixture.minHeaderParams, FormatIRProgram(ir))
			}

			artifact, err := CompileC(context.Background(), rt, prog)
			if err != nil {
				t.Fatal(err)
			}
			for _, fragment := range []string{"block_main_while_header", "block_main_while_body", "goto block_main_while_header"} {
				if !strings.Contains(artifact.Source, fragment) {
					t.Fatalf("generated C lacks runtime loop fragment %q:\n%s", fragment, artifact.Source)
				}
			}
			checksum := strings.TrimSpace(valuesOutput(want))
			if checksum != "" && strings.Contains(artifact.Source, checksum) {
				t.Fatalf("generated C captured interpreter checksum %q instead of computing it at runtime", checksum)
			}
		})
	}
}
