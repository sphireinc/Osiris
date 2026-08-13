package osiris

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestFormerLazyLetContradictionCannotRecur is the regression gate for the
// former specification/interpreter disagreement. Every claimed execution
// path must execute the initializer at the declaration, preserve preceding
// output, and never reach the following statement.
func TestFormerLazyLetContradictionCannotRecur(t *testing.T) {
	source := "emit \"before\"\nlet value = 1 / 0\nemit \"after\"\n"
	prog, err := Parse("<eager-let-parity>", source)
	if err != nil {
		t.Fatal(err)
	}
	for _, backend := range []struct {
		name string
		run  func(*Runtime, context.Context, *Program) ([]Value, error)
	}{
		{name: "interpreter", run: func(rt *Runtime, ctx context.Context, program *Program) ([]Value, error) { return rt.Run(ctx, program) }},
		{name: "vm", run: func(rt *Runtime, ctx context.Context, program *Program) ([]Value, error) {
			return rt.RunVM(ctx, program)
		}},
	} {
		t.Run(backend.name, func(t *testing.T) {
			values, runErr := backend.run(NewRuntime(), context.Background(), prog)
			if runErr == nil || !strings.Contains(runErr.Error(), "division by zero") {
				t.Fatalf("error = %v, want eager division failure", runErr)
			}
			if got, want := valuesOutput(values), "\"before\"\n"; got != want {
				t.Fatalf("output = %q, want %q", got, want)
			}
		})
	}

	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available for native eager-let parity")
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "eager-let-parity")
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatal(err)
	}
	output, runErr := exec.Command(executable).CombinedOutput()
	if runErr == nil {
		t.Fatalf("native executable unexpectedly succeeded: %s", output)
	}
	if got := string(output); !strings.Contains(got, "\"before\"\n") || strings.Contains(got, "\"after\"") || !strings.Contains(got, "division by zero") {
		t.Fatalf("native output = %q; want before, no after, and division failure", got)
	}
}
