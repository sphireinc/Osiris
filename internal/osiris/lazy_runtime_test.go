package osiris

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestExplicitLazyDefersAndForcesExactlyOnce(t *testing.T) {
	var calls atomic.Int64
	rt := NewRuntime()
	rt.Builtins["expensive"] = func(args []Value) (Value, error) {
		if len(args) != 0 {
			return nullValue(), fmt.Errorf("expensive expects no arguments")
		}
		return uintValue(uint64(calls.Add(1)), 64), nil
	}
	prog, err := Parse("<explicit-lazy>", `
let delayed = lazy { expensive() }
emit 7
let first = force(delayed)
let second = force(delayed)
emit [first, second]
`)
	if err != nil {
		t.Fatal(err)
	}
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := valuesOutput(out), "7u64\n[1u64, 1u64]\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("body calls = %d, want 1", got)
	}
}

func TestExplicitLazyMemoizesLanguageFailureAndDetectsCycle(t *testing.T) {
	prog, err := Parse("<lazy-failure>", "let x = lazy { 1 / 0 }\nemit force(x)\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = NewRuntime().Run(context.Background(), prog); err == nil || !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("failure error = %v", err)
	}
	// The declaration itself is intentionally not in scope while its body is
	// built. Construct the only possible recursive graph directly to verify the
	// runtime state machine rather than weakening lexical visibility.
	session := NewSession(NewRuntime(), context.Background(), "<lazy-cycle>")
	cycleScope := &scope{items: make(map[string]binding)}
	value := lazyValueOf(&callExpr{callee: &identExpr{name: "force"}, args: []Expr{&identExpr{name: "x"}}}, cycleScope, session.ev)
	cycleScope.items["x"] = eagerBinding(value)
	if _, err := forceLazyValue(session.ev, value); err == nil || !strings.Contains(err.Error(), "LazyCycle") {
		t.Fatalf("cycle error = %v", err)
	}
}

func TestExplicitLazyCaptureAndConcurrentForce(t *testing.T) {
	var calls atomic.Int64
	rt := NewRuntime()
	rt.Builtins["once"] = func([]Value) (Value, error) {
		calls.Add(1)
		return uintValue(42, 64), nil
	}
	prog, err := Parse("<lazy-capture>", `
var n = 1
let delayed = lazy { n }
n = 2
let shared = lazy { once() }
`)
	if err != nil {
		t.Fatal(err)
	}
	session := NewSession(rt, context.Background(), prog.SourceName)
	if _, err := session.ExecProgram(prog, false, nil); err != nil {
		t.Fatal(err)
	}
	value, err := session.ev.eval(&identExpr{name: "delayed"}, session.scope)
	if err != nil {
		t.Fatal(err)
	}
	got, err := forceLazyValue(session.ev, value)
	if err != nil || got.String() != "2u64" {
		t.Fatalf("captured value/error = %s / %v, want 2u64", got.String(), err)
	}
	shared, err := session.ev.eval(&identExpr{name: "shared"}, session.scope)
	if err != nil {
		t.Fatal(err)
	}
	const workers = 8
	start := make(chan struct{})
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			// Separate evaluators represent separate task executions; the shared
			// Lazy object must coordinate rather than mistake them for recursion.
			taskSession := NewSession(rt, context.Background(), "<lazy-concurrent-force>")
			v, err := forceLazyValue(taskSession.ev, shared)
			if err != nil || v.String() != "42u64" {
				errs <- fmt.Errorf("force = %s / %v", v.String(), err)
			}
		}()
	}
	close(start)
	group.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("concurrent body calls = %d, want 1", got)
	}
}

func TestExplicitLazyCancellationResetsPendingState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	session := NewSession(NewRuntime(), ctx, "<lazy-cancel>")
	value := lazyValueOf(&literalExpr{value: uintValue(1, 64)}, session.scope, session.ev)
	cancel()
	_, err := forceLazyValue(session.ev, value)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("first force = %v, want cancellation", err)
	}
	value.lazy.mu.Lock()
	state := value.lazy.state
	value.lazy.mu.Unlock()
	if state != lazyUnforced {
		t.Fatalf("state after cancellation = %d, want unforced", state)
	}
}
