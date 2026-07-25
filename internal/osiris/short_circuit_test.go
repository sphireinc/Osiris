package osiris

import (
	"context"
	"testing"
)

func TestLogicalShortCircuitDoesNotResolveDeferredName(t *testing.T) {
	for _, source := range []string{"emit 0 && missing", "emit 1 || missing"} {
		prog, err := Parse("<short-circuit>", source)
		if err != nil {
			t.Fatal(err)
		}
		interpreter, err := NewRuntime().Run(context.Background(), prog)
		if err != nil {
			t.Fatalf("interpreter %q: %v", source, err)
		}
		vm, err := NewRuntime().RunVM(context.Background(), prog)
		if err != nil {
			t.Fatalf("VM %q: %v", source, err)
		}
		if valuesOutput(interpreter) != valuesOutput(vm) {
			t.Fatalf("output mismatch for %q: interpreter %q, VM %q", source, valuesOutput(interpreter), valuesOutput(vm))
		}
	}
}
