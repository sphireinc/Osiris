package osiris

import (
	"strings"
	"testing"
)

// TestConstantFoldingAndSimplification exercises the behavior described by its
// body and locks in the observable language, runtime, or CLI semantics that
// the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestConstantFoldingAndSimplification(t *testing.T) {
	prog, err := Parse("<test>", `
let a = 1 + 2 * 3
let b = cat("os", "iris")
let c = 7u8 & 3u8
let d = 10 * Meter / (2 * Second)
let e = if(true, 1 + 2, 0)
let f = matrix(2, 2, [1, 2, 3, 4]) + matrix(2, 2, [1, 2, 3, 4])
let dead = 1 + 2
let g = hash("osiris")
let h = hash("osiris")
func cse(x) {
	let a = cat(x, "!")
	let b = cat(x, "!")
	return [a, b]
}
func simplify(x) {
	let y = x + 0
	let z = y * 1
	return z
}
emit a
emit b
emit c
emit d
emit e
emit f
emit h
emit cse("go")
emit simplify(9)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	opt := OptimizeProgram(prog)
	got := FormatProgram(opt)
	for _, want := range []string{
		"emit 7u64",
		`emit "osiris"`,
		"emit 3u8",
		"emit 5*Meter_Per_Second",
		"emit 3u64",
		"emit tensor([2, 2], [2, 4, 6, 8])",
		`emit "a71d6f42cb1e038c2222b9c1942e3c58c34cc699af0bfc63d59ca51f120b0cce"`,
		"func simplify(x) { let y = x; let z = x; return x }",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("optimized program missing %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "dead") {
		t.Fatalf("dead binding was not eliminated\n%s", got)
	}
}

// TestCommonSubexpressionElimination exercises the behavior described by its
// body and locks in the observable language, runtime, or CLI semantics that
// the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestCommonSubexpressionElimination(t *testing.T) {
	prog, err := Parse("<test>", `
let a = hash("osiris")
let b = hash("osiris")
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	stmts, _, _ := optimizeStmtSequence(prog.Statements, nil)
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements after CSE, got %d", len(stmts))
	}
	first, ok := stmts[0].(letStmt)
	if !ok {
		t.Fatalf("expected first statement to be let, got %T", stmts[0])
	}
	second, ok := stmts[1].(letStmt)
	if !ok {
		t.Fatalf("expected second statement to be let, got %T", stmts[1])
	}
	if _, ok := first.Expr.(*literalExpr); !ok {
		t.Fatalf("expected first expression to fold to a literal, got %T", first.Expr)
	}
	ref, ok := second.Expr.(*identExpr)
	if !ok {
		t.Fatalf("expected second expression to reuse the first binding, got %T", second.Expr)
	}
	if ref.name != first.Name {
		t.Fatalf("expected second expression to reference %q, got %q", first.Name, ref.name)
	}
}

// TestNamedValueSimplification exercises the behavior described by its body
// and locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestNamedValueSimplification(t *testing.T) {
	prog, err := Parse("<test>", `
let distance = 2 * kilo * Meter
let duration = 30 * minute
let circle = 2 * Pi
emit distance
emit duration
emit circle
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	got := FormatProgram(OptimizeProgram(prog))
	for _, want := range []string{
		"emit 2000*Meter",
		"emit 1800*Second",
		"emit 6.283185307179586",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("optimized program missing %q\n%s", want, got)
		}
	}
}

// TestStrengthReduction exercises the behavior described by its body and locks
// in the observable language, runtime, or CLI semantics that the surrounding
// package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestStrengthReduction(t *testing.T) {
	prog, err := Parse("<test>", `
func scale(x) {
	return uint(x) * 8u64
}
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn, ok := prog.Statements[0].(funcStmt)
	if !ok {
		t.Fatalf("expected func statement, got %T", prog.Statements[0])
	}
	got := formatStmt(optimizeStmt(fn))
	if !strings.Contains(got, "return uint(x) << 3u64") {
		t.Fatalf("strength reduction did not fire\n%s", got)
	}
}

// TestLoopOptimizationDoesNotFreezeMutableState exercises the behavior
// described by its body and locks in the observable language, runtime, or CLI
// semantics that the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestLoopOptimizationDoesNotFreezeMutableState(t *testing.T) {
	prog, err := Parse("<test>", `
func sweep(n) {
	let i = 0.0
	let total = 0.0
	while i < n {
		let total = total + i
		let i = i + 1.0
	}
	return total
}
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	got := FormatProgram(OptimizeProgram(prog))
	if strings.Contains(got, "while 0 < n") {
		t.Fatalf("optimizer froze loop condition into a constant\n%s", got)
	}
	if strings.Contains(got, "let i = 0.0") && strings.Contains(got, "while i < n") {
		// acceptable, but keep the test focused on the constant-freeze regression
	}
}
