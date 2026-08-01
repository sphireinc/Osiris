package osiris

import (
	"context"
	"strings"
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
	_, err = NewRuntime().Run(context.Background(), prog)
	if err == nil || !strings.Contains(err.Error(), "missing_body") || !strings.Contains(err.Error(), "missing_cleanup") {
		t.Fatalf("error = %v, want body and cleanup failures", err)
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
