package osiris

import (
	"context"
	"strings"
	"testing"
)

func TestLoopCanonicalization(t *testing.T) {
	prog, err := Parse("<test>", `
func sweep(xs) {
  let total = 0
  for x in xs {
    let total = total + x
  }
  while total < 10 {
    break
  }
}
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	lp, err := NewRuntime().BuildLoopProgram(context.Background(), prog)
	if err != nil {
		t.Fatalf("loop analysis failed: %v", err)
	}
	got := FormatLoopProgram(lp)
	for _, want := range []string{
		"for loop header=",
		"while loop header=",
		"canonical",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("loop report missing %q\n%s", want, got)
		}
	}
}
