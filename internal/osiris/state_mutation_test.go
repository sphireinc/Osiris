package osiris

import (
	"context"
	"strings"
	"testing"
)

func TestMutableLocalsAreDeterministicAcrossInterpreterAndVM(t *testing.T) {
	source := `var total = 0
total = total + 1
total = total + 2
emit total
`
	prog, err := Parse("<mutation>", source)
	if err != nil {
		t.Fatal(err)
	}
	if got := FormatProgram(prog); !strings.Contains(got, "var total = 0") || !strings.Contains(got, "total = total + 2") {
		t.Fatalf("formatted mutation lost its statement: %s", got)
	}
	interpreter, err := NewRuntime().Run(context.Background(), prog)
	if err != nil || valuesOutput(interpreter) != "3u64\n" {
		t.Fatalf("interpreter = %v, %v", interpreter, err)
	}
	vm, err := NewRuntime().RunVM(context.Background(), prog)
	if err != nil || valuesOutput(vm) != "3u64\n" {
		t.Fatalf("vm = %v, %v", vm, err)
	}
	ir, err := NewRuntime().BuildIR(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifySSAProgram(ir); err != nil {
		t.Fatalf("mutation IR is not SSA-valid: %v", err)
	}
	if !strings.Contains(FormatIRProgram(ir), "assign total") {
		t.Fatalf("IR does not retain the mutation barrier:\n%s", FormatIRProgram(ir))
	}
}

func TestMutableLocalsRejectImmutableWrites(t *testing.T) {
	prog, err := Parse("<immutable-write>", "let value = 1\nvalue = 2\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRuntime().CheckProgram(context.Background(), prog); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("immutable assignment error = %v", err)
	}
}

func TestMutableClosureCapturesCellAndTasksSnapshotIt(t *testing.T) {
	source := `var value = 1
func read() => value
value = 2
let task = spawn({ value = 3; value })
emit read()
emit await(task)
emit value
`
	values, err := NewSession(NewRuntime(), context.Background(), "<mutation-closure>").ExecSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if got := valuesOutput(values); got != "2u64\n3u64\n2u64\n" {
		t.Fatalf("closure/task mutation output = %q", got)
	}
}

func TestMutableLoopStateAndAggregateUpdatesRemainValueSemantic(t *testing.T) {
	source := `var total = 0
while total < 3 { total = total + 1 }
struct Box { value u64 }
let original = Box { value: 1 }
var changed = original
changed = original with { value: 9 }
emit total
emit original
emit changed
`
	values, err := NewSession(NewRuntime(), context.Background(), "<mutation-loop>").ExecSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if got := valuesOutput(values); got != "3u64\nBox { value: 1u64 }\nBox { value: 9u64 }\n" {
		t.Fatalf("loop/value update output = %q", got)
	}
}

func TestMutableAssignmentDoesNotCommitFailedRHS(t *testing.T) {
	session := NewSession(NewRuntime(), context.Background(), "<mutation-error>")
	if _, err := session.ExecSource("var value = 3\nvalue = uint(\"bad\")\n"); err == nil {
		t.Fatal("failed assignment unexpectedly succeeded")
	}
	values, err := session.ExecSource("emit value\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := valuesOutput(values); got != "3u64\n" {
		t.Fatalf("failed assignment committed value: %q", got)
	}
}

func TestMutableNativeParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name:   "mutable-native",
		Source: "var value = 1\nvalue = value + 2\nemit value\n",
	})
}

func TestConstLetAndVarHaveDistinctContracts(t *testing.T) {
	source := `const base = 2 + 3
let runtime = base + 1
var current = runtime
current = current + base
emit base, runtime, current
`
	prog, err := Parse("<binding-contracts>", source)
	if err != nil {
		t.Fatal(err)
	}
	checked, err := NewRuntime().CheckProgram(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if len(checked.Bindings) < 3 || !checked.Bindings[0].Constant || checked.Bindings[1].Mutable || !checked.Bindings[2].Mutable {
		t.Fatalf("binding metadata = %+v", checked.Bindings)
	}
	values, err := NewRuntime().Run(context.Background(), prog)
	if err != nil || valuesOutput(values) != "\"5u64, 6u64, 11u64\"\n" {
		t.Fatalf("interpreter = %v, %v", values, err)
	}
	vm, err := NewRuntime().RunVM(context.Background(), prog)
	if err != nil || valuesOutput(vm) != "\"5u64, 6u64, 11u64\"\n" {
		t.Fatalf("vm = %v, %v", vm, err)
	}
}

func TestConstRequiresStaticInitializerAndPreservesVarBuiltin(t *testing.T) {
	prog, err := Parse("<bad-const>", "const value = uint(1)\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRuntime().CheckProgram(context.Background(), prog); err == nil || !strings.Contains(err.Error(), "statically evaluable") {
		t.Fatalf("const call error = %v", err)
	}
	values, err := NewSession(NewRuntime(), context.Background(), "<var-builtin>").ExecSource("emit var([1, 2, 3])\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 {
		t.Fatalf("var builtin output = %v", values)
	}
}

func TestConstNativeParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name:   "const-native",
		Source: "const base = 2 + 3\nlet runtime = base + 1\nvar current = runtime\ncurrent = current + base\nemit current\n",
	})
}
