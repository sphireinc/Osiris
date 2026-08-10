package osiris

import (
	"strings"
	"testing"
)

func TestNativeCUnreachableTerminatorHasExplicitFailureLowering(t *testing.T) {
	var function strings.Builder
	if err := emitCTerm(&function, &irUnreachable{}, nil, map[string]string{}, false, nil); err != nil {
		t.Fatal(err)
	}
	if got := function.String(); got != "  return ofail(\"unreachable\");\n" {
		t.Fatalf("function unreachable lowering = %q", got)
	}

	var main strings.Builder
	if err := emitCTerm(&main, &irUnreachable{}, nil, map[string]string{}, true, nil); err != nil {
		t.Fatal(err)
	}
	if got := main.String(); got != "  return 1;\n" {
		t.Fatalf("main unreachable lowering = %q", got)
	}
}
