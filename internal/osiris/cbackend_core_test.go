package osiris

import (
	"strings"
	"testing"
)

func TestCBackendMutableClosureCellParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "native-mutable-closure-cell",
		Source: `var value = [1]
func read() => value
value = [2, 3]
emit read()
`,
	})
}

func TestCBackendMutableClosureObservesLaterMutation(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "native-mutable-closure-later-mutation",
		Source: `var value = [1]
func read() => value
emit read()
value = [2, 3]
emit read()
`,
	})
}

func TestCBackendMutableClosureMutationParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "native-mutable-closure-mutation",
		Source: `var value = 1
func bump() { value = value + 1; value }
emit bump()
emit value
`,
	})
}

func TestCBackendMutableClosureWritesSharedCellParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "native-mutable-closure-writes-cell",
		Source: `var value = 0
func bump() { value = value + 1; return value }
emit bump()
emit bump()
`,
	})
}

func TestCBackendNestedMutableClosureCellParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "native-nested-mutable-closure-cell",
		Source: `var value = 1
func factory() {
  func read() => value
  return read
}
let read = factory()
value = 2
emit read()
`,
	})
}

func TestCBackendMutableStateGeneratedCUsesOwnedCells(t *testing.T) {
	prog, err := Parse("native-cell-source.osr", `var value = [1]
func read() => value
value = [2, 3]
emit read()
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(t.Context(), NewRuntime(), prog)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"O_CELL", "ocell(", "ocellvalue(", "ocellset("} {
		if !strings.Contains(artifact.Source, marker) {
			t.Fatalf("generated C omitted mutable-state helper %q", marker)
		}
	}
}

func TestCBackendMutableStateAcrossControlFlowParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "native-mutable-state-control-flow",
		Source: `var total = 0
while total < 4 { total = total + 1 }
emit total
`,
	})
}

func TestCBackendBreakContinueStateParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "native-break-continue-state",
		Source: `var total = 0
while total < 5 {
  total = total + 1
  emit if(total == 2, { continue }, total)
  emit if(total == 4, { break }, total)
}
emit total
`,
	})
}

func TestCBackendTaskMutableCaptureSnapshotParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "native-task-mutable-capture-snapshot",
		Source: `var value = 1
let task = spawn({ value = 3; value })
emit value
emit await(task)
emit value
`,
	})
}

func TestCBackendBlockBindingStateParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "native-block-binding-state",
		Source: `let value = 1
emit { let value = 2; value }
emit value
`,
	})
}

func TestCBackendImmutableAssignmentDiagnostic(t *testing.T) {
	for _, declaration := range []string{"let", "const"} {
		t.Run(declaration, func(t *testing.T) {
			prog, err := Parse("immutable-native.osr", declaration+" value = 1\nvalue = 2\n")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := CompileC(t.Context(), NewRuntime(), prog); err == nil || !strings.Contains(err.Error(), "immutable") {
				t.Fatalf("immutable native assignment error = %v", err)
			}
		})
	}
}

func TestCBackendAggregateAndGenericCallParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "native-aggregate-generic-calls",
		Source: `func pair(value) => [value, value + 1]
func identity<T>(value T) T => value
emit pair(4)
emit identity([1, 2])
`,
	})
}

func TestCBackendTypedCallDiagnosticIsSourceSpanned(t *testing.T) {
	prog, err := Parse("typed-native.osr", `func accepts(value u32) u32 => value
emit accepts("wrong")
`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileC(t.Context(), NewRuntime(), prog); err == nil {
		t.Fatal("native compiler accepted a statically invalid typed call")
	} else if got := err.Error(); !strings.Contains(got, "typed-native.osr") || !strings.Contains(got, "type_error") {
		t.Fatalf("typed native diagnostic = %q", got)
	}
}
