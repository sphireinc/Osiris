package osiris

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestParseAndPrecedence exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestParseAndPrecedence(t *testing.T) {
	prog, err := Parse("<test>", `emit 1 + 2 * 3`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 1; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), "7u64"; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}
}

// TestBooleanLiteralsAndLogic exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestBooleanLiteralsAndLogic(t *testing.T) {
	prog, err := Parse("<test>", `
emit true
emit false
emit !false
emit true && false
emit true || false
emit if(true, "yes", "no")
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 6; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	want := []string{"true", "false", "true", "false", "true", `"yes"`}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestStructuredErrorModel exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestStructuredErrorModel(t *testing.T) {
	prog, err := Parse("<test>", `
let base = error(InvalidArgument, "bad input")
let base = withmeta(base, "request_id", "abc-123")
let base = withdebug(base, "secret", "token")
let wrapped = wrap(base, Unavailable, "while loading dataset")
emit wrapped
emit wrapped.kind
emit wrapped.code
emit wrapped.message
emit wrapped.operation
emit wrapped.resource
emit wrapped.retryable
emit wrapped.severity
emit wrapped.cause.kind
emit wrapped.cause.metadata
emit wrapped.cause.debug
emit wrapped.redacted
emit errmeta(wrapped)
emit decode(encode(wrapped))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 14; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got := out[0].String(); !strings.Contains(got, `error[unavailable]("while loading dataset")`) || !strings.Contains(got, `error[invalid_argument]("bad input")`) {
		t.Fatalf("wrapped summary = %s, want wrapped cause chain", got)
	}
	wantStrings := []string{
		`"unavailable"`,
		`"unavailable"`,
		`"while loading dataset"`,
		`null`,
		`null`,
		`null`,
		`"error"`,
		`"invalid_argument"`,
		`[["request_id", "abc-123"]]`,
		`[["secret", "token"]]`,
	}
	for i, want := range wantStrings {
		if got := out[i+1].String(); got != want {
			t.Fatalf("output[%d] = %s, want %s", i+1, got, want)
		}
	}
	if got := out[11].String(); strings.Contains(got, "token") {
		t.Fatalf("redacted output leaked debug metadata: %s", got)
	}
	if !out[0].Equal(out[13]) {
		t.Fatalf("codec round-trip changed structured error: %s vs %s", out[0].String(), out[13].String())
	}
}

// TestTryCatchPreservesStructuredErrors exercises the behavior described by
// its body and locks in the observable language, runtime, or CLI semantics
// that the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestTryCatchPreservesStructuredErrors(t *testing.T) {
	prog, err := Parse("<test>", `
emit try {
  read("/definitely/not-found-osiris.txt")
} catch err {
  [err.kind, err.code, err.message, err.resource, err.source]
}
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 1; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got := out[0].String(); !strings.Contains(got, `"not_found"`) ||
		!strings.Contains(got, `"read could not complete"`) ||
		!strings.Contains(got, `/definitely/not-found-osiris.txt`) ||
		!strings.Contains(got, `"<test>"`) {
		t.Fatalf("output = %s, want structured error capture", got)
	}
}

// TestResultAndOptionHelpers exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestResultAndOptionHelpers(t *testing.T) {
	prog, err := Parse("<test>", `
emit isok(ok(41))
emit iserr(err(error(InvalidArgument, "bad input")))
emit issome(some(41))
emit isnone(none())
emit unwrap(ok(7))
emit unwrap(some(8))
emit or_else(none(), 99)
emit to_option(ok(11))
emit to_option(err(error(InvalidArgument, "bad")))
emit to_result(some(12))
emit try { unwrap(to_result(none(), "missing option")) } catch err { [err.kind, err.message] }
emit collect([ok(1), some(2), ok(3)])
emit try { unwrap(collect([ok(1), err(error(InvalidArgument, "bad"))])) } catch err { err.kind }
func double(x) => x * 2
func recover(err) => wrap(err, NotFound, "while loading")
func next(x) => ok(x + 1)
emit map(ok(5), double)
emit map(err(error(InvalidArgument, "bad")), double)
emit map_error(err(error(NotFound, "missing")), recover)
emit flat_map(ok(8), next)
emit discard(err(error(InvalidArgument, "ignored")))
emit try { discard(42) } catch err { err.message }
emit is_must_handle(must_handle(ok(1)))
emit is_must_handle(map(must_handle(ok(2)), double))
emit decode(encode(ok(123)))
emit decode(encode(some(456)))
emit try { expect(none(), "missing value") } catch err { err.kind }
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	want := []string{
		"true",
		"true",
		"true",
		"true",
		"7u64",
		"8u64",
		"99u64",
		"some(11u64)",
		"none",
		"ok(12u64)",
		`["not_found", "missing option"]`,
		"ok([1u64, 2u64, 3u64])",
		`"invalid_argument"`,
		"ok(10u64)",
		`err(error[invalid_argument]("bad"))`,
		`err(error[not_found]("while loading") <- error[not_found]("missing"))`,
		"ok(9u64)",
		"null",
		`"discard expects a result, option, or error"`,
		"true",
		"true",
		"ok(123u64)",
		"some(456u64)",
		`"not_found"`,
	}
	if got, wantLen := len(out), len(want); got != wantLen {
		t.Fatalf("len(out) = %d, want %d", got, wantLen)
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

func TestResultBindingInterpreterVMAndFormatting(t *testing.T) {
	source := `
	let? value = ok(41)
emit value + 1
emit try { let? missing = none(); missing } catch err { err.kind }
emit try { let? failed = err(error(InvalidArgument, "bad")); failed } catch err { err.kind }
`
	prog, err := Parse("<result-binding>", source)
	if err != nil {
		t.Fatal(err)
	}
	if formatted, err := FormatSource("<result-binding>", source); err != nil || !strings.Contains(formatted, "let? value = ok(41u64)") {
		t.Fatalf("format = %q, %v", formatted, err)
	}
	for _, run := range []struct {
		name string
		fn   func(context.Context, *Program) ([]Value, error)
	}{
		{name: "interpreter", fn: NewRuntime().Run},
		{name: "vm", fn: NewRuntime().RunVM},
	} {
		t.Run(run.name, func(t *testing.T) {
			out, err := run.fn(context.Background(), prog)
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"42u64", `"not_found"`, `"invalid_argument"`}
			for i := range want {
				if i >= len(out) || out[i].String() != want[i] {
					t.Fatalf("output = %v, want %v", out, want)
				}
			}
		})
	}
}

func TestResultCombinatorInterpreterVMParity(t *testing.T) {
	prog, err := Parse("<result-combinators>", `
func double(x) => x * 2
func next(x) => ok(x + 1)
func recover(err) => wrap(err, NotFound, "while loading")
emit map(ok(5), double)
emit flat_map(ok(8), next)
emit map_error(err(error(NotFound, "missing")), recover)
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range []struct {
		name string
		fn   func(context.Context, *Program) ([]Value, error)
	}{
		{name: "interpreter", fn: NewRuntime().Run},
		{name: "vm", fn: NewRuntime().RunVM},
	} {
		t.Run(run.name, func(t *testing.T) {
			out, err := run.fn(context.Background(), prog)
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"ok(10u64)", "ok(9u64)", `err(error[not_found]("while loading") <- error[not_found]("missing"))`}
			for i := range want {
				if i >= len(out) || out[i].String() != want[i] {
					t.Fatalf("output = %v, want %v", out, want)
				}
			}
		})
	}
}

func TestResultBindingLiteralDiagnosticHasSourceSpan(t *testing.T) {
	_, err := Parse("<result-diagnostic>", "let? value = 42\n")
	if err == nil || !strings.Contains(err.Error(), "<result-diagnostic>:1:6") || !strings.Contains(err.Error(), "let? requires a result or option") {
		t.Fatalf("diagnostic = %v", err)
	}
	if _, err := Parse("<result-diagnostic>", "let? value = read(path)\n"); err != nil {
		t.Fatalf("dynamic fallible API was incorrectly rejected: %v", err)
	}
}

// TestPropagateAndFatalErrors exercises the behavior described by its body
// and locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestPropagateAndFatalErrors(t *testing.T) {
	prog, err := Parse("<test>", `
func load() => read("/definitely/not-found-osiris.txt")?
emit load()
emit try { panic("boom") } catch err { "caught" }
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err == nil {
		t.Fatalf("run succeeded unexpectedly: %#v", out)
	}
	msg := err.Error()
	if !strings.Contains(msg, "boom") && !strings.Contains(msg, "not_found") {
		t.Fatalf("unexpected fatal/propagation error: %v", err)
	}
}

// TestEmitInsideLoopBody exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestEmitInsideLoopBody(t *testing.T) {
	prog, err := Parse("<test>", `
let i = 0.0
while i < 3.0 {
  emit [i, i + 1.0, i + 2.0]
  let i = i + 1.0
}
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	want := []string{"[0, 1, 2]", "[1, 2, 3]", "[2, 3, 4]"}
	if got, wantLen := len(out), len(want); got != wantLen {
		t.Fatalf("len(out) = %d, want %d", got, wantLen)
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestDecimalLiteralsAndArithmetic exercises the behavior described by its
// body and locks in the observable language, runtime, or CLI semantics that
// the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestDecimalLiteralsAndArithmetic(t *testing.T) {
	prog, err := Parse("<test>", `
emit 0.1d + 0.2d == 0.3d
emit dec("1.25") + dec("0.75")
emit 1d / 3d
emit decode(encode([0.5d]))
emit decode(encode(1d / 3d))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 5; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), "true"; got != want {
		t.Fatalf("output[0] = %s, want %s", got, want)
	}
	if got, want := out[1].String(), "2d"; got != want {
		t.Fatalf("output[1] = %s, want %s", got, want)
	}
	if got, want := out[2].String(), "1/3d"; got != want {
		t.Fatalf("output[2] = %s, want %s", got, want)
	}
	if got, want := out[3].String(), `[0.5d]`; got != want {
		t.Fatalf("output[3] = %s, want %s", got, want)
	}
	if got, want := out[4].String(), "1/3d"; got != want {
		t.Fatalf("output[4] = %s, want %s", got, want)
	}
}

// TestFloatWidthsAndRoundTrip exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestFloatWidthsAndRoundTrip(t *testing.T) {
	prog, err := Parse("<test>", `
emit 1.5f32
emit f32(2.25)
emit 1.5f32 + 0.25f32
emit decode(encode([1.5f32]))
emit 3.5f64
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	want := []string{"1.5f32", "2.25f32", "1.75f32", "[1.5f32]", "3.5f64"}
	if got, wantLen := len(out), len(want); got != wantLen {
		t.Fatalf("len(out) = %d, want %d", got, wantLen)
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestBFloatWidthsAndRoundTrip exercises the behavior described by its body
// and locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestBFloatWidthsAndRoundTrip(t *testing.T) {
	prog, err := Parse("<test>", `
emit 1.5bf32
emit bf32(2.25)
emit 1.5bf32 + 0.25bf32
emit decode(encode([1.5bf32]))
emit 3.5bf64
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	want := []string{"1.5bf32", "2.25bf32", "1.75bf32", "[1.5bf32]", "3.5bf64"}
	if got, wantLen := len(out), len(want); got != wantLen {
		t.Fatalf("len(out) = %d, want %d", got, wantLen)
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestArrayConstructorsRangeGeneratorsAndStats exercises the behavior
// described by its body and locks in the observable language, runtime, or CLI
// semantics that the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestArrayConstructorsRangeGeneratorsAndStats(t *testing.T) {
	prog, err := Parse("<test>", `
emit shape(zeros(2, 3))
emit shape(ones([2, 3]))
emit shape(full(2, 3, 7))
emit shape(eye(3))
emit flatten(eye(3))
emit flatten(arange(5))
emit flatten(linspace(0, 1, 3))
emit flatten(logspace(0, 2, 3, 10))
emit sinh(0)
emit expm1(0)
emit relu([-1, 2])
emit sigmoid(0)
emit mean([1, 2, 3, 4])
emit median([1, 3, 2, 4])
emit quantile([1, 2, 3, 4], 0.25)
emit percentile([1, 2, 3, 4], 25)
emit argmin([3, 1, 2])
emit argmax([3, 1, 2])
emit any([false, 0, 1])
emit all([true, 1, "x"])
emit cumsum([1, 2, 3])
emit cumprod([2, 3, 4])
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	want := []string{
		"[2u64, 3u64]",
		"[2u64, 3u64]",
		"[2u64, 3u64]",
		"[3u64, 3u64]",
		"[1, 0, 0, 0, 1, 0, 0, 0, 1]",
		"[0, 1, 2, 3, 4]",
		"[0, 0.5, 1]",
		"[1, 10, 100]",
		"0",
		"0",
		"[0, 2]",
		"0.5",
		"2.5",
		"2.5",
		"1.75",
		"1.75",
		"1u64",
		"0u64",
		"true",
		"true",
		"[1, 3, 6]",
		"[2, 6, 24]",
	}
	if got, wantLen := len(out), len(want); got != wantLen {
		t.Fatalf("len(out) = %d, want %d", got, wantLen)
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestShapeManipulationAndLinearAlgebraSurface exercises the behavior
// described by its body and locks in the observable language, runtime, or CLI
// semantics that the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestShapeManipulationAndLinearAlgebraSurface(t *testing.T) {
	prog, err := Parse("<test>", `
emit squeeze(tensor([[1], [2]]))
emit expand_dims([1, 2, 3], 0)
emit concat([1, 2], [3])
emit stack([1, 2], [3, 4])
emit broadcast_to(7, 2, 3)
emit where([true, false], [1, 2], [3, 4])
emit clip([-1, 2, 10], 0, 5)
emit gather([10, 20, 30], [2, 0])
emit scatter([0, 0, 0], [1, 2], [4, 5])
emit pad([1, 2], 1, 2, 0)
emit outer([1, 2], [3, 4])
emit trace(matrix(2, 2, [1, 2, 3, 4]))
emit det(matrix(2, 2, [1, 2, 3, 4]))
emit solve(matrix(2, 2, [1, 2, 3, 4]), [5, 11])
emit inv(matrix(2, 2, [1, 2, 3, 4]))
emit qr(matrix(2, 2, [1, 0, 0, 1]))
emit cholesky(matrix(2, 2, [4, 0, 0, 9]))
emit eig(matrix(2, 2, [3, 0, 0, 2]))
emit eigh(matrix(2, 2, [5, 0, 0, 7]))
emit svd(matrix(2, 2, [3, 0, 0, 2]))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	want := []string{
		"tensor[2]([1u64, 2u64])",
		"tensor[1x3]([1u64, 2u64, 3u64])",
		"tensor[3]([1u64, 2u64, 3u64])",
		"tensor[2x2]([1u64, 2u64, 3u64, 4u64])",
		"tensor[2x3]([7u64, 7u64, 7u64, 7u64, 7u64, 7u64])",
		"tensor[2]([1u64, 4u64])",
		"[0, 2, 5]",
		"tensor[2]([30u64, 10u64])",
		"tensor[3]([0u64, 4u64, 5u64])",
		"tensor[5]([0u64, 1u64, 2u64, 0u64, 0u64])",
		"tensor[2x2]([3, 4, 6, 8])",
		"5",
		"-2",
		"tensor[2]([1, 2])",
		"",
		"[tensor[2x2]([1, 0, 0, 1]), tensor[2x2]([1, 0, 0, 1])]",
		"tensor[2x2]([2, 0, 0, 3])",
		"[tensor[2]([3+0i, 2+0i]), tensor[2x2]([1+0i, 0+0i, 0+0i, 1+0i])]",
		"[tensor[2]([5, 7]), tensor[2x2]([1, 0, 0, 1])]",
		"[tensor[2x2]([1, 0, 0, 1]), tensor[2]([3, 2]), tensor[2x2]([1, 0, 0, 1])]",
	}
	if got, wantLen := len(out), len(want); got != wantLen {
		t.Fatalf("len(out) = %d, want %d", got, wantLen)
	}
	for i := range want {
		if i == 14 {
			got := out[i].String()
			if got != "tensor[2x2]([-1.9999999999999996, 0.9999999999999998, 1.4999999999999998, -0.49999999999999994])" &&
				got != "tensor[2x2]([-1.9999999999999996, 0.9999999999999999, 1.4999999999999998, -0.49999999999999994])" {
				t.Fatalf("output[%d] = %s, want one of two close float variants", i, got)
			}
			continue
		}
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestExactIntegerDivisionAndExponentiation exercises the behavior described
// by its body and locks in the observable language, runtime, or CLI semantics
// that the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestExactIntegerDivisionAndExponentiation(t *testing.T) {
	prog, err := Parse("<test>", `
emit 7 / 25 * 25 == 7
emit 1 / 3 + 1 / 6 == 1 / 2
emit pow(bigint("999999999999"), 279)
emit expt(bigint("999999999999"), 279)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 4; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), "true"; got != want {
		t.Fatalf("output[0] = %s, want %s", got, want)
	}
	if got, want := out[1].String(), "true"; got != want {
		t.Fatalf("output[1] = %s, want %s", got, want)
	}
	base := new(big.Int)
	if _, ok := base.SetString("999999999999", 10); !ok {
		t.Fatal("failed to parse test base")
	}
	wantPow := new(big.Int).Exp(base, big.NewInt(279), nil).String() + "n"
	if got := out[2].String(); got != wantPow {
		t.Fatalf("output[2] mismatch: got len=%d, want len=%d", len(got), len(wantPow))
	}
	if got := out[3].String(); got != wantPow {
		t.Fatalf("output[3] mismatch: got len=%d, want len=%d", len(got), len(wantPow))
	}
}

// TestSignedIntegerLiteralsAndArithmetic exercises the behavior described by
// its body and locks in the observable language, runtime, or CLI semantics
// that the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestSignedIntegerLiteralsAndArithmetic(t *testing.T) {
	prog, err := Parse("<test>", `
emit 1i16 + 2i16
emit -5i16
emit 0xffi8
emit 7i16 / 2i16
emit abs(-5i16)
emit pow(2i8, 7)
emit decode(encode(-1i8))
emit i32(42)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 8; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	want := []string{"3i16", "-5i16", "-1i8", "3.5d", "5i16", "-128i8", "-1i8", "42i32"}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestComplexLiteralsAndArithmetic exercises the behavior described by its
// body and locks in the observable language, runtime, or CLI semantics that
// the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestComplexLiteralsAndArithmetic(t *testing.T) {
	prog, err := Parse("<test>", `
emit 5 + 3i
emit complex(2, 3) * complex(1, -1)
emit magnitude(3 + 4i)
emit real(5 + 3i)
emit imag(5 + 3i)
emit decode(encode(5 + 3i))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 6; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	want := []string{"5+3i", "5+1i", "5", "5", "3", "5+3i"}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestUnsignedIntegerLiteralsAndConstructor exercises the behavior described
// by its body and locks in the observable language, runtime, or CLI semantics
// that the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestUnsignedIntegerLiteralsAndConstructor(t *testing.T) {
	prog, err := Parse("<test>", `
emit 0xffu
emit uint(9)
emit 5u + 2u
emit decode(encode(uint(9)))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 4; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	want := []string{"255u64", "9u64", "7u64", "9u64"}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestWideUnsignedIntegers exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestWideUnsignedIntegers(t *testing.T) {
	prog, err := Parse("<test>", `
emit 1u128
emit u256(7)
emit 1u128 << 100
emit decode(encode(1u128 << 100))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 4; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	want := []string{
		"1u128",
		"7u256",
		"1267650600228229401496703205376u128",
		"1267650600228229401496703205376u128",
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestPackedUnsignedFamily exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestPackedUnsignedFamily(t *testing.T) {
	prog, err := Parse("<test>", `
emit 1uf8
emit ufnum(9)
emit 1uf16 + 2uf16
emit decode(encode([1uf16]))
emit decode(encode(1uf1024))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	want := []string{"1uf8", "9ufnum", "3uf16", "[1uf16]", "1uf1024"}
	if got, wantLen := len(out), len(want); got != wantLen {
		t.Fatalf("len(out) = %d, want %d", got, wantLen)
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestByteLiteralsAndConstructor exercises the behavior described by its body
// and locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestByteLiteralsAndConstructor(t *testing.T) {
	prog, err := Parse("<test>", `
emit 255b
emit byte(7)
emit decode(encode(byte(9)))
emit pack([byte(65), byte(66)])
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 4; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	want := []string{"255u8", "7u8", "9u8", `"AB"`}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestNativeBitwiseOperators exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestNativeBitwiseOperators(t *testing.T) {
	prog, err := Parse("<test>", `
emit 7u8 & 3u8
emit 1u8 << 2
emit 5u64 | 2u64
emit ~0u8
emit 255b ^ 15b
emit 8u8 >> 1
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	want := []string{"3u8", "4u8", "7u64", "255u8", "240u8", "4u8"}
	if got, wantLen := len(out), len(want); got != wantLen {
		t.Fatalf("len(out) = %d, want %d", got, wantLen)
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestStdMathLibrary exercises the behavior described by its body and locks in
// the observable language, runtime, or CLI semantics that the surrounding
// package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestStdMathLibrary(t *testing.T) {
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.sin(0)
emit math.cos(0)
emit math.tan(0)
emit math.asin(1)
emit math.exp(1)
emit math.log(1)
emit math.log10(1000)
emit math.log2(8)
emit math.hypot(3, 4)
emit math.norm(math.vec(3, 4))
emit math.dot(math.vec([1, 2, 3]), math.vec([4, 5, 6]))
emit math.matmul(math.matrix(2, 2, [1, 2, 3, 4]), math.matrix(2, 2, [5, 6, 7, 8]))
emit math.conjugate(complex(2, 3))
emit math.conjugate(math.tensor([complex(1, 2), complex(3, -4)]))
emit math.u128(5)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	want := []string{
		"0",
		"1",
		"0",
		"1.5707963267948966",
		"2.718281828459045",
		"0",
		"3",
		"3",
		"5",
		"5",
		"32",
		"tensor[2x2]([19, 22, 43, 50])",
		"2-3i",
		"tensor[2]([1-2i, 3+4i])",
		"5u128",
	}
	if got, wantLen := len(out), len(want); got != wantLen {
		t.Fatalf("len(out) = %d, want %d", got, wantLen)
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestUnitTypesAndDimensionalAnalysis exercises the behavior described by its
// body and locks in the observable language, runtime, or CLI semantics that
// the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestUnitTypesAndDimensionalAnalysis(t *testing.T) {
	prog, err := Parse("<test>", `
emit 10 * Meter / (2 * Second)
emit (3 * Meter) + (2 * Meter)
emit decode(encode(10 * Meter / (2 * Second)))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	want := []string{"5*Meter_Per_Second", "5*Meter", "5*Meter_Per_Second"}
	if got, wantLen := len(out), len(want); got != wantLen {
		t.Fatalf("len(out) = %d, want %d", got, wantLen)
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestUnitTypeMismatchFails exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestUnitTypeMismatchFails(t *testing.T) {
	prog, err := Parse("<test>", `emit Meter + Second`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	if out, err := rt.Run(context.Background(), prog); err == nil {
		t.Fatalf("run succeeded unexpectedly: %#v", out)
	} else if !strings.Contains(err.Error(), "cannot add") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestBigNumberFallbacks exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestBigNumberFallbacks(t *testing.T) {
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.bigint("9223372036854775808") + math.bigint("7")
emit math.bigfloat("1.5") * math.bigfloat("2")
emit decode(encode(math.bigint("9223372036854775808")))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	want := []string{"9223372036854775815n", "3bf", "9223372036854775808n"}
	if got, wantLen := len(out), len(want); got != wantLen {
		t.Fatalf("len(out) = %d, want %d", got, wantLen)
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestRationalConstructorExactness exercises the behavior described by its
// body and locks in the observable language, runtime, or CLI semantics that
// the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestRationalConstructorExactness(t *testing.T) {
	prog, err := Parse("<test>", `
emit rational("1/3") + rational("1/6")
emit rat("2/5") * 3
emit decode(encode(rational("1/7")))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	want := []string{"0.5d", "1.2d", "1/7d"}
	if got, wantLen := len(out), len(want); got != wantLen {
		t.Fatalf("len(out) = %d, want %d", got, wantLen)
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestIntervalArithmeticAndEncoding exercises the behavior described by its
// body and locks in the observable language, runtime, or CLI semantics that
// the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestIntervalArithmeticAndEncoding(t *testing.T) {
	prog, err := Parse("<test>", `
emit interval(1, 3) + interval(2, 4)
emit interval(1, 3) * interval(2, 4)
emit interval(5, 5) == 5
emit decode(encode(interval(5, 5)))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	want := []string{"interval[3, 7]", "interval[2, 12]", "true", "interval[5, 5]"}
	if got, wantLen := len(out), len(want); got != wantLen {
		t.Fatalf("len(out) = %d, want %d", got, wantLen)
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestModIntArithmeticAndEncoding exercises the behavior described by its body
// and locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestModIntArithmeticAndEncoding(t *testing.T) {
	prog, err := Parse("<test>", `
emit modint(17, 5) + modint(17, 14)
emit modint(17, 5) * 3
emit modint(17, 5) / modint(17, 3)
emit decode(encode(modint(17, 5)))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	want := []string{"2 mod 17", "15 mod 17", "13 mod 17", "5 mod 17"}
	if got, wantLen := len(out), len(want); got != wantLen {
		t.Fatalf("len(out) = %d, want %d", got, wantLen)
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestFixedPointArithmeticAndEncoding exercises the behavior described by its
// body and locks in the observable language, runtime, or CLI semantics that
// the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestFixedPointArithmeticAndEncoding(t *testing.T) {
	prog, err := Parse("<test>", `
emit fixed(32, 16, 1.5d) + fixed(32, 16, 0.25d)
emit fixed(32, 16, 1.5d) * fixed(32, 16, 2)
emit pow(fixed(32, 16, 1.5d), 2)
emit decode(encode(fixed(32, 16, 1.5d)))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	want := []string{"fixed[32,16](7/4)", "fixed[32,16](3)", "fixed[32,16](9/4)", "fixed[32,16](3/2)"}
	if got, wantLen := len(out), len(want); got != wantLen {
		t.Fatalf("len(out) = %d, want %d", got, wantLen)
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestBigIntExactnessAndFallback exercises the behavior described by its body
// and locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestBigIntExactnessAndFallback(t *testing.T) {
	prog, err := Parse("<test>", `
emit bigint("9223372036854775808") + 1
emit bigint("999999999999") * bigint("999999999999")
emit pow(bigint("12"), 17)
emit decode(encode(bigint("9223372036854775808")))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	want := []string{
		"9223372036854775809n",
		"999999999998000000000001n",
		"2218611106740436992n",
		"9223372036854775808n",
	}
	if got, wantLen := len(out), len(want); got != wantLen {
		t.Fatalf("len(out) = %d, want %d", got, wantLen)
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestTensorSIMDStyleVectorizationAndViews exercises the behavior described by
// its body and locks in the observable language, runtime, or CLI semantics
// that the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestTensorSIMDStyleVectorizationAndViews(t *testing.T) {
	prog, err := Parse("<test>", `
emit matrix(2, 2, [1, 2, 3, 4]) * 2
emit view(tensor([1, 2, 3]))
emit clone(view(tensor([1, 2, 3])))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	want := []string{
		`tensor[2x2]([2, 4, 6, 8])`,
		`tensor[3]([1u64, 2u64, 3u64])`,
		`tensor[3]([1u64, 2u64, 3u64])`,
	}
	if got, wantLen := len(out), len(want); got != wantLen {
		t.Fatalf("len(out) = %d, want %d", got, wantLen)
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestGPUCompilerArtifact exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestGPUCompilerArtifact(t *testing.T) {
	prog, err := Parse("<test>", `
emit matmul(matrix(2, 2, [1, 2, 3, 4]), matrix(2, 2, [5, 6, 7, 8]))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileGPU(context.Background(), prog)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	if !strings.Contains(artifact.Source, "osiris_matmul") {
		t.Fatalf("gpu source missing matmul kernel")
	}
	found := false
	for _, k := range artifact.Kernels {
		if k.Kind == "matmul" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("gpu artifact missing matmul kernel metadata: %#v", artifact.Kernels)
	}
}

// TestConcurrentTaskScheduler exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestConcurrentTaskScheduler(t *testing.T) {
	prog, err := Parse("<test>", `
emit [gate(1), gate(2)]
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	var started int32
	release := make(chan struct{})
	var once sync.Once
	rt.Builtins["gate"] = func(args []Value) (Value, error) {
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("gate expects 1 argument")
		}
		if atomic.AddInt32(&started, 1) == 2 {
			once.Do(func() { close(release) })
		}
		select {
		case <-release:
			return args[0], nil
		case <-time.After(time.Second):
			return nullValue(), fmt.Errorf("task scheduler did not run tasks concurrently")
		}
	}
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := atomic.LoadInt32(&started), int32(2); got != want {
		t.Fatalf("gate calls = %d, want %d", got, want)
	}
	if got, want := len(out), 1; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), `[1u64, 2u64]`; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}
}

// TestStringPrimitiveIsolation exercises the behavior described by its body
// and locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestStringPrimitiveIsolation(t *testing.T) {
	okProg, err := Parse("<test>", `
emit "osiris"
emit len("osiris")
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), okProg)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 2; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), `"osiris"`; got != want {
		t.Fatalf("output[0] = %s, want %s", got, want)
	}
	if got, want := out[1].String(), "6"; got != want {
		t.Fatalf("output[1] = %s, want %s", got, want)
	}

	badProg, err := Parse("<test>", `emit uint("7")`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if out, err := rt.Run(context.Background(), badProg); err == nil {
		t.Fatalf("run succeeded unexpectedly: %#v", out)
	}
}

// TestLanguageSyntaxLoopsAndBlocks exercises the behavior described by its
// body and locks in the observable language, runtime, or CLI semantics that
// the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestLanguageSyntaxLoopsAndBlocks(t *testing.T) {
	prog, err := Parse("<test>", `
emit { let x = 2; let y = 3; x + y }
let i = 0
let acc = 0
while i < 5 {
  let acc = acc + i
  let i = i + 1
}
emit acc
let sum = 0
for n in [1, 2, 3, 4] {
  let sum = sum + n
}
emit sum
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	want := []string{"5u64", "10u64", "10u64"}
	if got, wantLen := len(out), len(want); got != wantLen {
		t.Fatalf("len(out) = %d, want %d", got, wantLen)
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestTryCatchErrorHandling exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestTryCatchErrorHandling(t *testing.T) {
	prog, err := Parse("<test>", `
emit try { uint("7") } catch err { cat("caught: ", err) }
emit try { let x = 1; x + 1 } catch err { cat("bad: ", err) }
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	want := []string{`"caught: expected unsigned integer, got string"`, "2u64"}
	if got, wantLen := len(out), len(want); got != wantLen {
		t.Fatalf("len(out) = %d, want %d", got, wantLen)
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestEmitConcatenatesCommaSeparatedValues exercises the behavior described by
// its body and locks in the observable language, runtime, or CLI semantics
// that the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestEmitConcatenatesCommaSeparatedValues(t *testing.T) {
	prog, err := Parse("<test>", `
emit "Solved:", 42
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 1; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), `"Solved:, 42u64"`; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}
}

// TestTensorAndMatrixType exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestTensorAndMatrixType(t *testing.T) {
	prog, err := Parse("<test>", `
emit shape(tensor([[1, 2], [3, 4]]))
emit flatten(tensor([[1, 2], [3, 4]]))
emit transpose(matrix(2, 2, [1, 2, 3, 4]))
emit dot(tensor([1, 2, 3]), tensor([4, 5, 6]))
emit matmul(matrix(2, 2, [1, 2, 3, 4]), matrix(2, 2, [5, 6, 7, 8]))
emit decode(encode(matrix(2, 2, [1, 2, 3, 4])))
emit hex(tensor([byte(65), byte(66)]))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 7; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	want := []string{
		`[2u64, 2u64]`,
		`[1u64, 2u64, 3u64, 4u64]`,
		`tensor[2x2]([1u64, 3u64, 2u64, 4u64])`,
		`32`,
		`tensor[2x2]([19, 22, 43, 50])`,
		`tensor[2x2]([1u64, 2u64, 3u64, 4u64])`,
		`"4142"`,
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestTensorBroadcastSemantics exercises the behavior described by its body
// and locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestTensorBroadcastSemantics(t *testing.T) {
	prog, err := Parse("<test>", `
emit matrix(2, 1, [1, 2]) + matrix(1, 2, [10, 20])
emit matrix(2, 2, [1, 2, 3, 4]) * 2
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	want := []string{
		`tensor[2x2]([11, 21, 12, 22])`,
		`tensor[2x2]([2, 4, 6, 8])`,
	}
	if got, wantLen := len(out), len(want); got != wantLen {
		t.Fatalf("len(out) = %d, want %d", got, wantLen)
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestTensorShapeValidationAtCompileTime exercises the behavior described by
// its body and locks in the observable language, runtime, or CLI semantics
// that the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestTensorShapeValidationAtCompileTime(t *testing.T) {
	prog, err := Parse("<test>", `
emit matmul(matrix(3, 4, [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12]), matrix(2, 3, [1, 2, 3, 4, 5, 6]))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	if out, err := rt.RunVM(context.Background(), prog); err == nil {
		t.Fatalf("vm run succeeded unexpectedly: %#v", out)
	} else if !strings.Contains(err.Error(), "matmul shape mismatch") {
		t.Fatalf("vm error = %v, want matmul shape mismatch", err)
	}
}

// TestLexicalShadowing exercises the behavior described by its body and locks
// in the observable language, runtime, or CLI semantics that the surrounding
// package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestLexicalShadowing(t *testing.T) {
	prog, err := Parse("<test>", `
let a = "moon"
emit a
let a = "sun"
emit a
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 2; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), `"moon"`; got != want {
		t.Fatalf("output[0] = %s, want %s", got, want)
	}
	if got, want := out[1].String(), `"sun"`; got != want {
		t.Fatalf("output[1] = %s, want %s", got, want)
	}
}

// TestStructuralMemoization exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestStructuralMemoization(t *testing.T) {
	prog, err := Parse("<test>", `
let a = probe("x", "y")
let b = probe("x", "y")
emit cat(a, b)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	rt := NewRuntime()
	var calls int32
	rt.Builtins["probe"] = func(args []Value) (Value, error) {
		atomic.AddInt32(&calls, 1)
		return stringValue(textify(args[0]) + ":" + textify(args[1])), nil
	}

	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := atomic.LoadInt32(&calls), int32(1); got != want {
		t.Fatalf("probe calls = %d, want %d", got, want)
	}
	if got, want := len(out), 1; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), `"x:yx:y"`; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}
}

// TestStdlibImportAndMemberCall exercises the behavior described by its body
// and locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestStdlibImportAndMemberCall(t *testing.T) {
	prog, err := Parse("<test>", `
import "std/text" as text
emit text.slug("Hello, Weird World")
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 1; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), `"hello-weird-world"`; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}
}

// TestSessionPersistsState exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestSessionPersistsState(t *testing.T) {
	rt := NewRuntime()
	sess := NewSession(rt, context.Background(), "<repl>")
	if out, err := sess.ExecSource(`let x = 3`); err != nil {
		t.Fatalf("let failed: %v", err)
	} else if len(out) != 0 {
		t.Fatalf("let produced outputs: %v", out)
	}
	out, err := sess.ExecSource(`emit x + 4`)
	if err != nil {
		t.Fatalf("emit failed: %v", err)
	}
	if got, want := len(out), 1; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), "7u64"; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}
}

// TestBlockExpression exercises the behavior described by its body and locks
// in the observable language, runtime, or CLI semantics that the surrounding
// package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestBlockExpression(t *testing.T) {
	prog, err := Parse("<test>", `
emit { let x = 2; let y = 3; x + y }
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 1; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), "5u64"; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}
}

// TestHygienicMacroExpansion exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestHygienicMacroExpansion(t *testing.T) {
	prog, err := Parse("<test>", `
let suffix = "!"
macro shout(x) => cat(x, suffix)
let suffix = "?"
emit shout("go")
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter run failed: %v", err)
	}
	got, err := rt.RunVM(context.Background(), prog)
	if err != nil {
		t.Fatalf("vm run failed: %v", err)
	}
	if gotS, wantS := len(got), len(want); gotS != wantS {
		t.Fatalf("len(got) = %d, want %d", gotS, wantS)
	}
	for i := range want {
		if got[i].String() != want[i].String() {
			t.Fatalf("output[%d] = %s, want %s", i, got[i].String(), want[i].String())
		}
	}
	if got := want[0].String(); got != `"go!"` {
		t.Fatalf("output = %s, want %s", got, `"go!"`)
	}
}

// TestFunctionDeclarationsAndRecursion exercises the behavior described by its
// body and locks in the observable language, runtime, or CLI semantics that
// the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestFunctionDeclarationsAndRecursion(t *testing.T) {
	prog, err := Parse("<test>", `
func fact(n) => if(n == 0, 1, n * fact(n - 1))
emit fact(5)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 1; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), "120u64"; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}
	vmOut, err := rt.RunVM(context.Background(), prog)
	if err != nil {
		t.Fatalf("vm run failed: %v", err)
	}
	if got, want := len(vmOut), 1; got != want {
		t.Fatalf("vm len(out) = %d, want %d", got, want)
	}
	if got, want := vmOut[0].String(), "120u64"; got != want {
		t.Fatalf("vm output = %s, want %s", got, want)
	}
}

// TestLoopsAndFlowControl exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestLoopsAndFlowControl(t *testing.T) {
	prog, err := Parse("<test>", `
func loopbreak() {
	while true { break }
	return 7
}

func loopcontinue(xs) {
	for x in xs { continue }
	return 9
}

emit loopbreak()
emit loopcontinue([1, 2, 3])
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 2; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), "7u64"; got != want {
		t.Fatalf("output[0] = %s, want %s", got, want)
	}
	if got, want := out[1].String(), "9u64"; got != want {
		t.Fatalf("output[1] = %s, want %s", got, want)
	}
	vmOut, err := rt.RunVM(context.Background(), prog)
	if err != nil {
		t.Fatalf("vm run failed: %v", err)
	}
	if got, want := len(vmOut), 2; got != want {
		t.Fatalf("vm len(out) = %d, want %d", got, want)
	}
	if got, want := vmOut[0].String(), "7u64"; got != want {
		t.Fatalf("vm output[0] = %s, want %s", got, want)
	}
	if got, want := vmOut[1].String(), "9u64"; got != want {
		t.Fatalf("vm output[1] = %s, want %s", got, want)
	}
}

// TestTaskSpawnAwaitAndCancel exercises the behavior described by its body
// and locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestTaskSpawnAwaitAndCancel(t *testing.T) {
	prog, err := Parse("<test>", `
let task = spawn({
	let x = 5
	x + 7
})
emit await(task)
emit cancel(task)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 2; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), "12u64"; got != want {
		t.Fatalf("output[0] = %s, want %s", got, want)
	}
	if got, want := out[1].String(), "false"; got != want {
		t.Fatalf("output[1] = %s, want %s", got, want)
	}
	vmOut, err := rt.RunVM(context.Background(), prog)
	if err != nil {
		t.Fatalf("vm run failed: %v", err)
	}
	if got, want := len(vmOut), 2; got != want {
		t.Fatalf("vm len(out) = %d, want %d", got, want)
	}
	if got, want := vmOut[0].String(), "12u64"; got != want {
		t.Fatalf("vm output[0] = %s, want %s", got, want)
	}
	if got, want := vmOut[1].String(), "false"; got != want {
		t.Fatalf("vm output[1] = %s, want %s", got, want)
	}
}

// TestTaskCancellationStopsLoops exercises the behavior described by its body
// and locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestTaskCancellationStopsLoops(t *testing.T) {
	prog, err := Parse("<test>", `
let task = spawn({
	let i = 0
	while true {
		let i = i + 1
	}
	i
})
emit cancel(task)
emit await(task)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	_, err = rt.Run(context.Background(), prog)
	if err == nil {
		t.Fatalf("run succeeded, want cancellation error")
	}
	if !strings.Contains(err.Error(), "context canceled") && !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("error = %v, want cancellation-related error", err)
	}
}

// TestParallelAndTimeout exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestParallelAndTimeout(t *testing.T) {
	prog, err := Parse("<test>", `
emit parallel(1, 2, 3)
emit timeout(0, {
	while true {
		let keep = 1
	}
	99
})
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err == nil {
		t.Fatalf("run succeeded, want timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("error = %v, want timeout-related error", err)
	}
	if len(out) != 1 || out[0].String() != "[1u64, 2u64, 3u64]" {
		t.Fatalf("out = %v, want committed parallel output before timeout failure", out)
	}
}

// TestAsyncIOWrappers exercises the behavior described by its body and locks
// in the observable language, runtime, or CLI semantics that the surrounding
// package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestAsyncIOWrappers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "async.txt")
	prog, err := Parse("<test>", `
let path = `+strconv.Quote(path)+`
emit await(write_async(path, "alpha\nbeta"))
emit await(read_async(path))
emit await(readbytes_async(path))
emit await(lines_async("a\nb"))
emit await(csv_async("a,b\n1,2"))
emit await(jsonparse_async("{\"x\":1}"))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 6; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), "true"; got != want {
		t.Fatalf("output[0] = %s, want %s", got, want)
	}
	if got, want := out[1].String(), `"alpha\nbeta"`; got != want {
		t.Fatalf("output[1] = %s, want %s", got, want)
	}
	if got, want := out[2].String(), "[97u8, 108u8, 112u8, 104u8, 97u8, 10u8, 98u8, 101u8, 116u8, 97u8]"; got != want {
		t.Fatalf("output[2] = %s, want %s", got, want)
	}
	if got, want := out[3].String(), `["a", "b"]`; got != want {
		t.Fatalf("output[3] = %s, want %s", got, want)
	}
	if got, want := out[4].String(), `[["a", "b"], ["1", "2"]]`; got != want {
		t.Fatalf("output[4] = %s, want %s", got, want)
	}
	if got, want := out[5].String(), `[["x", 1u64]]`; got != want {
		t.Fatalf("output[5] = %s, want %s", got, want)
	}
}

func TestTransferStreamsPreservesBoundedBytesAndFlushes(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.txt")
	destinationPath := filepath.Join(t.TempDir(), "destination.txt")
	if err := os.WriteFile(sourcePath, []byte("abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	source, err := openReadStream(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer source.close()
	destination, err := openWriteStream(destinationPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer destination.close()
	value, err := transferContextValue(context.Background(), []Value{streamValueOf(source), streamValueOf(destination), uintValue(4, 64)})
	if err != nil || value.String() != "4u64" {
		t.Fatalf("transfer = %s, %v", value.String(), err)
	}
	if got, err := os.ReadFile(destinationPath); err != nil || string(got) != "abcd" {
		t.Fatalf("destination = %q, %v", got, err)
	}
	if _, err := transferContextValue(context.Background(), []Value{streamValueOf(source), streamValueOf(source)}); err == nil {
		t.Fatal("self transfer unexpectedly succeeded")
	}
}

func TestTransferBuiltinInterpreterVMParity(t *testing.T) {
	for _, run := range []struct {
		name string
		fn   func(context.Context, *Program) ([]Value, error)
	}{
		{name: "interpreter", fn: NewRuntime().Run},
		{name: "vm", fn: NewRuntime().RunVM},
	} {
		t.Run(run.name, func(t *testing.T) {
			dir := t.TempDir()
			sourcePath := filepath.Join(dir, "source.txt")
			destinationPath := filepath.Join(dir, "destination.txt")
			if err := os.WriteFile(sourcePath, []byte("abcdef"), 0o600); err != nil {
				t.Fatal(err)
			}
			prog, err := Parse("<transfer-parity>", `
let source = openread(`+strconv.Quote(sourcePath)+`)
let destination = openwrite(`+strconv.Quote(destinationPath)+`)
emit transfer(source, destination, 4)
emit close(source)
emit close(destination)
`)
			if err != nil {
				t.Fatal(err)
			}
			out, err := run.fn(context.Background(), prog)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := len(out), 3; got != want {
				t.Fatalf("output count = %d, want %d", got, want)
			}
			for index, want := range []string{"4u64", "true", "true"} {
				if got := out[index].String(); got != want {
					t.Fatalf("output[%d] = %q, want %q", index, got, want)
				}
			}
			if got, err := os.ReadFile(destinationPath); err != nil || string(got) != "abcd" {
				t.Fatalf("destination = %q, %v", got, err)
			}
		})
	}
}

func TestTCPNetworkLoopbackResources(t *testing.T) {
	listener, err := tcpListen("127.0.0.1:0")
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skip("loopback listeners are forbidden by this sandbox")
		}
		t.Fatal(err)
	}
	defer listener.close()
	accepted := make(chan *streamValue, 1)
	go func() { connection, _ := listener.accept(1000); accepted <- connection }()
	client, err := tcpDial(listener.address(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	server := <-accepted
	if server == nil {
		t.Fatal("accept returned no connection")
	}
	defer server.close()
	if _, err := client.writeChunk(stringValue("ping\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := client.flush(); err != nil {
		t.Fatal(err)
	}
	line, err := server.readLine()
	if err != nil || line.String() != "ping" {
		t.Fatalf("server read = %s, %v", line.String(), err)
	}
	if _, err := server.writeChunk(stringValue("pong\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := server.flush(); err != nil {
		t.Fatal(err)
	}
	line, err = client.readLine()
	if err != nil || line.String() != "pong" {
		t.Fatalf("client read = %s, %v", line.String(), err)
	}
}

func TestTCPCancellationTimeoutAndStructuredErrors(t *testing.T) {
	listener, err := tcpListen("127.0.0.1:0")
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skip("loopback listeners are forbidden by this sandbox")
		}
		t.Fatal(err)
	}
	defer listener.close()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := listener.acceptContext(cancelled, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled accept error = %v, want context.Canceled", err)
	}
	timed, cancelTimeout := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelTimeout()
	if _, err := listener.acceptContext(timed, 0); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timed accept error = %v, want context.DeadlineExceeded", err)
	}

	accepted := make(chan *streamValue, 1)
	acceptErr := make(chan error, 1)
	go func() {
		stream, err := listener.acceptContext(context.Background(), 1000)
		accepted <- stream
		acceptErr <- err
	}()
	client, err := tcpDialContext(context.Background(), listener.address(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	server := <-accepted
	if err := <-acceptErr; err != nil || server == nil {
		t.Fatalf("accept = %v, %v", server, err)
	}
	defer server.close()
	readCtx, cancelRead := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelRead()
	if _, err := server.readLineContext(readCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timed read error = %v, want context.DeadlineExceeded", err)
	}

	if got := classifyGoError(&net.OpError{Op: "dial", Err: errors.New("refused")}); got != ErrorKindUnavailable {
		t.Fatalf("dial error kind = %s, want %s", got, ErrorKindUnavailable)
	}
	if got := classifyGoError(net.ErrClosed); got != ErrorKindIO {
		t.Fatalf("closed network error kind = %s, want %s", got, ErrorKindIO)
	}
}

func TestTCPContextBuiltinsRunInInterpreterAndVM(t *testing.T) {
	prog, err := Parse("<tcp-context-builtins>", `emit close(tcp_listen("127.0.0.1:0"))`)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range []struct {
		name string
		fn   func(context.Context, *Program) ([]Value, error)
	}{
		{name: "interpreter", fn: NewRuntime().Run},
		{name: "vm", fn: NewRuntime().RunVM},
	} {
		t.Run(run.name, func(t *testing.T) {
			out, err := run.fn(context.Background(), prog)
			if err != nil {
				if strings.Contains(err.Error(), "operation not permitted") {
					t.Skip("loopback listeners are forbidden by this sandbox")
				}
				t.Fatal(err)
			}
			if len(out) != 1 || !out[0].Truthy() {
				t.Fatalf("output = %v, want true", out)
			}
		})
	}
}

// TestRaceSelectAndStreamingIO exercises the behavior described by its body
// and locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestRaceSelectAndStreamingIO(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stream.txt")
	prog, err := Parse("<test>", `
let out = openwrite(`+strconv.Quote(path)+`)
emit writechunk(out, "alpha\n")
emit writechunk(out, "beta\n")
emit flush(out)
emit close(out)
let inp = openread(`+strconv.Quote(path)+`)
emit readline(inp)
emit readline(inp)
emit readline(inp)
emit close(inp)
emit race(1, 2, 3)
emit select(4, 5, 6)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	rt.SetTaskDebug(true)
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 10; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	want := []string{
		"6u64",
		"5u64",
		"true",
		"true",
		`"alpha"`,
		`"beta"`,
		"null",
		"true",
		"1u64",
		"[0u64, 4u64]",
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

// TestTaskTraceAndDeterministicDebugMode exercises the behavior described by
// its body and locks in the observable language, runtime, or CLI semantics
// that the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestTaskTraceAndDeterministicDebugMode(t *testing.T) {
	prog, err := Parse("<test>", `
emit parallel(1, 2, 3)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	rt.SetTaskTrace(true)
	rt.SetTaskDebug(true)
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 1; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), `[1u64, 2u64, 3u64]`; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}
	trace := rt.TaskTrace()
	if got, want := len(trace), 6; got != want {
		t.Fatalf("len(trace) = %d, want %d", got, want)
	}
	want := []TaskTraceEvent{
		{Kind: "scheduler.start", TaskID: "00000000", Status: "running", Detail: "deterministic"},
		{Kind: "scheduler.finish", TaskID: "00000000", Status: "done", Detail: "uint"},
		{Kind: "scheduler.start", TaskID: "00000001", Status: "running", Detail: "deterministic"},
		{Kind: "scheduler.finish", TaskID: "00000001", Status: "done", Detail: "uint"},
		{Kind: "scheduler.start", TaskID: "00000002", Status: "running", Detail: "deterministic"},
		{Kind: "scheduler.finish", TaskID: "00000002", Status: "done", Detail: "uint"},
	}
	for i := range want {
		if trace[i].Kind != want[i].Kind || trace[i].TaskID != want[i].TaskID || trace[i].Status != want[i].Status || trace[i].Detail != want[i].Detail {
			t.Fatalf("trace[%d] = %#v, want %#v", i, trace[i], want[i])
		}
	}
}

// TestFormatSource exercises the behavior described by its body and locks in
// the observable language, runtime, or CLI semantics that the surrounding
// package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestFormatSource(t *testing.T) {
	formatted, err := FormatSource("<test>", `package demo; export alpha, beta; import "std/text" as text; let x=text.slug("Hello, World"); emit text.upper(x); emit "Solved:", 42`)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	const want = "package demo\nexport alpha, beta\nimport \"std/text\" as text\nlet x = text.slug(\"Hello, World\")\nemit text.upper(x)\nemit \"Solved:\", 42u64\n"
	if formatted != want {
		t.Fatalf("formatted = %q, want %q", formatted, want)
	}
}

func TestTypedLetBindingsHaveCheckedCanonicalKinds(t *testing.T) {
	source := "let a = u64(3)\nlet b u32 = 3\nlet name string = \"something\"\nemit a, b, name\n"
	prog, err := Parse("<typed-let>", source)
	if err != nil {
		t.Fatal(err)
	}
	formatted := FormatProgram(prog)
	if !strings.Contains(formatted, "let b u32 = 3u32") || !strings.Contains(formatted, "let name string = \"something\"") {
		t.Fatalf("formatted typed lets = %q", formatted)
	}
	for _, run := range []struct {
		name string
		fn   func(*Runtime, *Program) ([]Value, error)
	}{
		{"interpreter", func(rt *Runtime, p *Program) ([]Value, error) { return rt.Run(context.Background(), p) }},
		{"vm", func(rt *Runtime, p *Program) ([]Value, error) { return rt.RunVM(context.Background(), p) }},
	} {
		t.Run(run.name, func(t *testing.T) {
			out, err := run.fn(NewRuntime(), prog)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := valuesOutput(out), "\"3u64, 3u32, something\"\n"; got != want {
				t.Fatalf("output = %q, want %q", got, want)
			}
		})
	}
	for _, source := range []string{
		"let value u32 = 3u64\n",
		"let value u8 = 256\n",
		"let value string = 3\n",
		"let value uint32 = 3\n",
	} {
		if _, err := Parse("<bad-typed-let>", source); err == nil {
			t.Fatalf("Parse(%q) succeeded", source)
		}
	}
	bad, err := Parse("<typed-let-dynamic>", "let a = u64(3)\nlet b u32 = a\nemit b\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range []func(*Runtime, *Program) ([]Value, error){
		func(rt *Runtime, p *Program) ([]Value, error) { return rt.Run(context.Background(), p) },
		func(rt *Runtime, p *Program) ([]Value, error) { return rt.RunVM(context.Background(), p) },
	} {
		if _, err := run(NewRuntime(), bad); err == nil || !strings.Contains(err.Error(), "binding type u32 requires u32") {
			t.Fatalf("dynamic type mismatch error = %v", err)
		}
	}
}

func TestCBackendTypedLetBindingParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<native-typed-let>", "let value u32 = 3\nemit value\n")
	if err != nil {
		t.Fatal(err)
	}
	want, err := NewRuntime().Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(artifact.Source, "oasserttype(") {
		t.Fatalf("generated C omitted typed let check:\n%s", artifact.Source)
	}
	exe := filepath.Join(t.TempDir(), "typed-let")
	if err := CompileCExecutable(context.Background(), artifact.Source, exe); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(exe).CombinedOutput()
	if err != nil {
		t.Fatalf("run native: %v\n%s", err, out)
	}
	if got := string(out); got != valuesOutput(want) {
		t.Fatalf("native output = %q, want %q", got, valuesOutput(want))
	}
}

func TestNumberBindingAnnotationAndConstructorParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<number-binding>", "let foo number = 4\nlet bar = number(4)\nemit foo\nemit bar\n")
	if err != nil {
		t.Fatal(err)
	}
	interpreter, err := NewRuntime().Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := NewRuntime().RunVM(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := valuesOutput(vm), valuesOutput(interpreter); got != want {
		t.Fatalf("VM output = %q, want %q", got, want)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "number-binding")
	if err := CompileCExecutable(context.Background(), artifact.Source, exe); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(exe).CombinedOutput()
	if err != nil {
		t.Fatalf("run native: %v\n%s", err, out)
	}
	if got, want := string(out), valuesOutput(interpreter); got != want {
		t.Fatalf("native output = %q, want %q", got, want)
	}
}

func TestEnumDeclarationParseAndFormat(t *testing.T) {
	source := "enum Status { Ready, Running, Done }\n"
	prog, err := Parse("<enum>", source)
	if err != nil {
		t.Fatal(err)
	}
	stmt, ok := prog.Statements[0].(enumStmt)
	if !ok || stmt.Name != "Status" || len(stmt.Cases) != 3 || stmt.Cases[2].Name != "Done" {
		t.Fatalf("enum parse = %#v", prog.Statements[0])
	}
	formatted, err := FormatSource("<enum>", source)
	if err != nil || formatted != source {
		t.Fatalf("enum format = %q, %v", formatted, err)
	}
	if _, err := Parse("<enum>", "enum Empty { }"); err == nil {
		t.Fatal("empty enum unexpectedly parsed")
	}
	if _, err := Parse("<enum>", "enum Repeat { A, A }"); err == nil {
		t.Fatal("duplicate enum case unexpectedly parsed")
	}
}

func TestEnumCasesEvaluateWithNominalEqualityInInterpreterAndVM(t *testing.T) {
	source := "enum Status { Ready, Running, Done }\nemit Status.Ready == Status.Ready\nemit Status.Ready == Status.Done\nemit Status.Ready\n"
	prog, err := Parse("<enum>", source)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range []struct {
		name string
		fn   func(*Runtime, *Program) ([]Value, error)
	}{
		{name: "interpreter", fn: func(rt *Runtime, prog *Program) ([]Value, error) { return rt.Run(context.Background(), prog) }},
		{name: "vm", fn: func(rt *Runtime, prog *Program) ([]Value, error) { return rt.RunVM(context.Background(), prog) }},
	} {
		t.Run(run.name, func(t *testing.T) {
			out, err := run.fn(NewRuntime(), prog)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := valuesOutput(out), "true\nfalse\nStatus.Ready\n"; got != want {
				t.Fatalf("output = %q, want %q", got, want)
			}
		})
	}
}

func TestEnumCasesMatchInInterpreterAndVM(t *testing.T) {
	source := "enum Status { Ready, Running, Done }\nlet status = Status.Running\nemit match status { Status.Ready => 1, Status.Running => 2, Status.Done => 3 }\n"
	prog, err := Parse("<enum-match>", source)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range []struct {
		name string
		fn   func(*Runtime, *Program) ([]Value, error)
	}{
		{name: "interpreter", fn: func(rt *Runtime, prog *Program) ([]Value, error) { return rt.Run(context.Background(), prog) }},
		{name: "vm", fn: func(rt *Runtime, prog *Program) ([]Value, error) { return rt.RunVM(context.Background(), prog) }},
	} {
		t.Run(run.name, func(t *testing.T) {
			out, err := run.fn(NewRuntime(), prog)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := valuesOutput(out), "2u64\n"; got != want {
				t.Fatalf("output = %q, want %q", got, want)
			}
		})
	}
	formatted, err := FormatSource("<enum-match>", source)
	if err != nil || !strings.Contains(formatted, "Status.Running => 2u64") {
		t.Fatalf("formatted enum match = %q, %v", formatted, err)
	}
}

func TestPayloadEnumCasesConstructAndMatchInInterpreterAndVM(t *testing.T) {
	source := "enum Result { Ok(value), Err(message) }\nlet result = Result.Ok([2, 3])\nemit match result { Result.Ok([x, y]) => x + y, Result.Err(message) => 0 }\nemit Result.Ok(5) == Result.Ok(5)\nemit Result.Ok(5) == Result.Ok(6)\n"
	prog, err := Parse("<payload-enum>", source)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range []struct {
		name string
		fn   func(*Runtime, *Program) ([]Value, error)
	}{
		{name: "interpreter", fn: func(rt *Runtime, prog *Program) ([]Value, error) { return rt.Run(context.Background(), prog) }},
		{name: "vm", fn: func(rt *Runtime, prog *Program) ([]Value, error) { return rt.RunVM(context.Background(), prog) }},
	} {
		t.Run(run.name, func(t *testing.T) {
			out, err := run.fn(NewRuntime(), prog)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := valuesOutput(out), "5u64\ntrue\nfalse\n"; got != want {
				t.Fatalf("output = %q, want %q", got, want)
			}
		})
	}
	formatted, err := FormatSource("<payload-enum>", source)
	if err != nil || !strings.Contains(formatted, "enum Result { Ok(value), Err(message) }") || !strings.Contains(formatted, "Result.Ok([x, y])") {
		t.Fatalf("formatted payload enum = %q, %v", formatted, err)
	}
}

func TestPayloadEnumCaseDiagnostics(t *testing.T) {
	fixtures := []struct {
		source string
		want   string
	}{
		{source: "enum Result { Ok(value), Err(message) }\nemit Result.Ok()\n", want: "enum case Result.Ok expects 1 arguments"},
		{source: "enum Result { Ok(value), Err(message) }\nemit match Result.Ok(1) { Result.Ok => 1, Result.Err(message) => 2 }\n", want: "enum case Result.Ok pattern expects 1 values"},
		{source: "enum Result { Ok(value), Err(message) }\nemit match Result.Ok(1) { Result.Ok(value) => value }\n", want: "non-exhaustive match for enum Result; missing cases: Err"},
	}
	for _, fixture := range fixtures {
		prog, err := Parse("<payload-enum-error>", fixture.source)
		if err != nil {
			t.Fatal(err)
		}
		for _, run := range []struct {
			name string
			fn   func(*Runtime, *Program) ([]Value, error)
		}{
			{name: "interpreter", fn: func(rt *Runtime, prog *Program) ([]Value, error) { return rt.Run(context.Background(), prog) }},
			{name: "vm", fn: func(rt *Runtime, prog *Program) ([]Value, error) { return rt.RunVM(context.Background(), prog) }},
		} {
			t.Run(run.name, func(t *testing.T) {
				_, err := run.fn(NewRuntime(), prog)
				if err == nil || !strings.Contains(err.Error(), fixture.want) {
					t.Fatalf("error = %v, want %q", err, fixture.want)
				}
			})
		}
	}
}

func TestMatchGuardsAndDefaultHaveInterpreterVMParity(t *testing.T) {
	source := "enum Result { Ok(value), Err(message) }\nlet result = Result.Ok(-2)\nemit match result { Result.Ok(value) when value > 0 => value, Result.Ok(value) => -value, default => 0 }\n"
	prog, err := Parse("<match-guard>", source)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range []struct {
		name string
		fn   func(*Runtime, *Program) ([]Value, error)
	}{
		{name: "interpreter", fn: func(rt *Runtime, prog *Program) ([]Value, error) { return rt.Run(context.Background(), prog) }},
		{name: "vm", fn: func(rt *Runtime, prog *Program) ([]Value, error) { return rt.RunVM(context.Background(), prog) }},
	} {
		t.Run(run.name, func(t *testing.T) {
			out, err := run.fn(NewRuntime(), prog)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := valuesOutput(out), "2\n"; got != want {
				t.Fatalf("output = %q, want %q", got, want)
			}
		})
	}
	formatted, err := FormatSource("<match-guard>", source)
	if err != nil || !strings.Contains(formatted, "Result.Ok(value) when value > 0u64 => value") || !strings.Contains(formatted, "default => 0u64") {
		t.Fatalf("formatted guard/default = %q, %v", formatted, err)
	}
}

func TestGroupedPatternsHaveInterpreterVMParityAndStableBindings(t *testing.T) {
	source := "enum Result { Ok(value), Err(value) }\nlet result = Result.Err(7)\nemit match result { Result.Ok(value) | Result.Err(value) => value, default => 0 }\n"
	prog, err := Parse("<grouped-pattern>", source)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range []struct {
		name string
		fn   func(*Runtime, *Program) ([]Value, error)
	}{
		{name: "interpreter", fn: func(rt *Runtime, prog *Program) ([]Value, error) { return rt.Run(context.Background(), prog) }},
		{name: "vm", fn: func(rt *Runtime, prog *Program) ([]Value, error) { return rt.RunVM(context.Background(), prog) }},
	} {
		t.Run(run.name, func(t *testing.T) {
			out, err := run.fn(NewRuntime(), prog)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := valuesOutput(out), "7u64\n"; got != want {
				t.Fatalf("output = %q, want %q", got, want)
			}
		})
	}
	badProg, err := Parse("<bad-grouped-pattern>", "enum Result { Ok(value), Err(value) }\nemit match Result.Ok(1) { Result.Ok(x) | Result.Err(y) => x, default => 0 }\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRuntime().Run(context.Background(), badProg); err == nil || !strings.Contains(err.Error(), "must bind the same names") {
		t.Fatalf("grouped binding error = %v", err)
	}
	formatted, err := FormatSource("<grouped-pattern>", source)
	if err != nil || !strings.Contains(formatted, "Result.Ok(value) | Result.Err(value) => value") {
		t.Fatalf("formatted grouped pattern = %q, %v", formatted, err)
	}
}

func TestRelationalTypeAndLogicalPatternsHaveInterpreterVMParity(t *testing.T) {
	source := "let value = 7\nemit match value { number n and >= 0 => n, not null => 0 }\nemit match -3 { < 0 => 1, _ => 0 }\n"
	prog, err := Parse("<advanced-pattern>", source)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range []struct {
		name string
		fn   func(*Runtime, *Program) ([]Value, error)
	}{
		{name: "interpreter", fn: func(rt *Runtime, prog *Program) ([]Value, error) { return rt.Run(context.Background(), prog) }},
		{name: "vm", fn: func(rt *Runtime, prog *Program) ([]Value, error) { return rt.RunVM(context.Background(), prog) }},
	} {
		t.Run(run.name, func(t *testing.T) {
			out, err := run.fn(NewRuntime(), prog)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := valuesOutput(out), "7u64\n1u64\n"; got != want {
				t.Fatalf("output = %q, want %q", got, want)
			}
		})
	}
	formatted, err := FormatSource("<advanced-pattern>", source)
	if err != nil || !strings.Contains(formatted, "number n and >= 0u64 => n") || !strings.Contains(formatted, "not null => 0u64") {
		t.Fatalf("formatted advanced pattern = %q, %v", formatted, err)
	}
}

func TestCBackendRelationalTypeAndLogicalPatternParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	source := "let value = 7\nemit match value { number n and >= 0 => n, not null => 0 }\nemit match -3 { < 0 => 1, _ => 0 }\n"
	prog, err := Parse("<native-advanced-pattern>", source)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile C: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "advanced-pattern")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("run executable: %v\n%s", err, out)
	}
	if got, want := string(out), valuesOutput(want); got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
	if !strings.Contains(artifact.Source, "omatchtype(") || !strings.Contains(artifact.Source, "block_main_match_and") {
		t.Fatalf("generated C did not lower type/logical patterns:\n%s", artifact.Source)
	}
}

func TestEnumMatchRejectsRedundantArmsWithSourceLocations(t *testing.T) {
	fixtures := []string{
		"enum Status { Ready, Done }\nemit match Status.Ready { Status.Ready => 1, Status.Ready => 2, Status.Done => 3 }\n",
		"enum Status { Ready, Done }\nemit match Status.Ready { _ => 1, Status.Ready => 2 }\n",
	}
	for _, source := range fixtures {
		prog, err := Parse("<redundant-enum-match>", source)
		if err != nil {
			t.Fatal(err)
		}
		for _, run := range []struct {
			name string
			fn   func(*Runtime, *Program) ([]Value, error)
		}{
			{name: "interpreter", fn: func(rt *Runtime, prog *Program) ([]Value, error) { return rt.Run(context.Background(), prog) }},
			{name: "vm", fn: func(rt *Runtime, prog *Program) ([]Value, error) { return rt.RunVM(context.Background(), prog) }},
		} {
			t.Run(run.name, func(t *testing.T) {
				_, err := run.fn(NewRuntime(), prog)
				var diagnostic *OsirisError
				if err == nil || !strings.Contains(err.Error(), "unreachable match arm") || !errors.As(err, &diagnostic) || diagnostic.location == nil || diagnostic.location.Span.start.line == 0 {
					t.Fatalf("redundancy error = %#v", err)
				}
			})
		}
	}
}

func TestEnumMatchRejectsUnknownCaseAndReportsNonExhaustiveCase(t *testing.T) {
	unknownCase := "enum Status { Ready, Done }\nemit match Status.Ready { Status.Missing => 1, _ => 2 }\n"
	nonExhaustive := "enum Status { Ready, Done }\nemit match Status.Done { Status.Ready => 1 }\n"
	for _, source := range []struct {
		source string
		want   string
	}{
		{source: unknownCase, want: "enum Status has no case"},
		{source: nonExhaustive, want: "non-exhaustive match for enum Status"},
	} {
		prog, err := Parse("<enum-match>", source.source)
		if err != nil {
			t.Fatal(err)
		}
		for _, run := range []struct {
			name string
			fn   func(*Runtime, *Program) ([]Value, error)
		}{
			{name: "interpreter", fn: func(rt *Runtime, prog *Program) ([]Value, error) { return rt.Run(context.Background(), prog) }},
			{name: "vm", fn: func(rt *Runtime, prog *Program) ([]Value, error) { return rt.RunVM(context.Background(), prog) }},
		} {
			t.Run(run.name, func(t *testing.T) {
				_, err := run.fn(NewRuntime(), prog)
				if err == nil || !strings.Contains(err.Error(), source.want) {
					t.Fatalf("error = %v, want %q", err, source.want)
				}
				var diagnostic *OsirisError
				if !errors.As(err, &diagnostic) || diagnostic.location == nil || diagnostic.location.Span.start.line == 0 || diagnostic.location.Span.start.col == 0 {
					t.Fatalf("error has no stable source span: %#v", err)
				}
			})
		}
	}
	prog, err := Parse("<native-nonexhaustive-enum-match>", nonExhaustive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileC(context.Background(), NewRuntime(), prog); err == nil || !strings.Contains(err.Error(), "non-exhaustive match for enum Status") {
		t.Fatalf("native compiler non-exhaustive diagnostic = %v", err)
	}
}

func TestCBackendNestedListEnumMatchParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	source := "enum Result { Ok(value), Err(message) }\nlet result = Result.Ok([2, [3, 4]])\nemit match result { Result.Ok([left, [middle, right]]) => left + middle + right, Result.Err(_) => 0 }\n"
	prog, err := Parse("<native-nested-list-enum-match>", source)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := rt.RunVM(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if got := valuesOutput(vm); got != valuesOutput(want) {
		t.Fatalf("VM output = %q, want %q", got, valuesOutput(want))
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile C: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "nested-list-enum-match")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("run executable: %v\n%s", err, out)
	}
	if got, want := string(out), valuesOutput(want); got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
	if !strings.Contains(artifact.Source, "olistis(") || !strings.Contains(artifact.Source, "olistitem(") {
		t.Fatalf("generated C did not lower nested list patterns:\n%s", artifact.Source)
	}
}

func TestCBackendRepeatedPatternBindingParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	source := "enum Result { Ok(value), Err(message) }\nemit match Result.Ok([2, 2]) { Result.Ok([same, same]) => 1, default => 0 }\nemit match Result.Ok([2, 3]) { Result.Ok([same, same]) => 1, default => 0 }\n"
	prog, err := Parse("<native-repeated-pattern-binding>", source)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile C: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "repeated-pattern-binding")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("run executable: %v\n%s", err, out)
	}
	if got, want := string(out), valuesOutput(want); got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
	if !strings.Contains(artifact.Source, "oequal(") {
		t.Fatalf("generated C did not test repeated bindings:\n%s", artifact.Source)
	}
}

func TestBackedEnumDeclarationsRoundTripAndNativeParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	source := "enum Status: string { Draft = \"draft\", Published = \"published\" }\nenum Code: i32 { Ok = 200i32, Missing = 404i32 }\nemit Status.Draft\nemit match Code.Missing { Code.Ok => \"ok\", Code.Missing => \"missing\" }\n"
	prog, err := Parse("<backed-enum>", source)
	if err != nil {
		t.Fatal(err)
	}
	formatted := FormatProgram(prog)
	if !strings.Contains(formatted, "enum Status: string { Draft = \"draft\", Published = \"published\" }") || !strings.Contains(formatted, "enum Code: i32 { Ok = 200i32, Missing = 404i32 }") {
		t.Fatalf("formatted backed enums = %q", formatted)
	}
	if _, err := Parse("<backed-enum-formatted>", formatted); err != nil {
		t.Fatalf("formatted backed enums did not parse: %v\n%s", err, formatted)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := rt.RunVM(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if got := valuesOutput(vm); got != valuesOutput(want) {
		t.Fatalf("VM output = %q, want %q", got, valuesOutput(want))
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile C: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "backed-enum")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("run executable: %v\n%s", err, out)
	}
	if got, want := string(out), valuesOutput(want); got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
}

func TestBackedEnumDeclarationDiagnostics(t *testing.T) {
	for _, source := range []string{
		"enum Status: bool { Draft = true }\n",
		"enum Status: string { Draft = \"draft\", Again = \"draft\" }\n",
		"enum Status: string { Draft }\n",
		"enum Status: string { Draft(value) = \"draft\" }\n",
		"enum Status { Draft = \"draft\" }\n",
	} {
		if _, err := Parse("<invalid-backed-enum>", source); err == nil {
			t.Fatalf("Parse(%q) succeeded", source)
		}
	}
}

func TestCBackendEnumValueParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	source := "enum Status { Ready, Done }\nenum Result { Ok(value), Err(message) }\nemit Status.Ready\nemit Status.Ready == Status.Ready\nemit Status.Ready == Status.Done\nemit Result.Ok(7)\nemit Result.Ok(7) == Result.Ok(7)\nemit Result.Ok(7) == Result.Ok(8)\n"
	prog, err := Parse("<native-unit-enum>", source)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile C: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "unit-enum")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("run executable: %v\n%s", err, out)
	}
	if got, want := string(out), valuesOutput(want); got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
	if !strings.Contains(artifact.Source, "O_ENUM") || !strings.Contains(artifact.Source, "oenum(\"Status\", \"Ready\"") {
		t.Fatalf("generated C did not lower enum values:\n%s", artifact.Source)
	}
}

func TestCBackendEnumNestedPayloadAndFunctionKindParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	source := "enum Inner { Value }\nenum Box { Wrap(value) }\nfunc identity(value) => value\nemit Box.Wrap(Inner.Value)\nemit Box.Wrap(identity) == Box.Wrap(identity)\nemit match Box.Wrap(identity) { Box.Wrap(function fn) => true, default => false }\n"
	prog, err := Parse("<native-enum-kind-parity>", source)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := rt.RunVM(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if got := valuesOutput(vm); got != valuesOutput(want) {
		t.Fatalf("VM output = %q, want %q", got, valuesOutput(want))
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile C: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "enum-kind-parity")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("run executable: %v\n%s", err, out)
	}
	if got, want := string(out), valuesOutput(want); got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
	if !strings.Contains(artifact.Source, "case O_ENUM:") || !strings.Contains(artifact.Source, "a.as.closure.call==b.as.closure.call") {
		t.Fatalf("generated C lacks recursive enum rendering or closure equality:\n%s", artifact.Source)
	}
}

func TestEnumJSONSerializationIsNominalAndRecursive(t *testing.T) {
	prog, err := Parse("<enum-json>", "enum Inner { Value }\nenum Box { Wrap(value) }\nemit Box.Wrap(Inner.Value)\n")
	if err != nil {
		t.Fatal(err)
	}
	out, err := NewRuntime().Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(out[0])
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"enum":{"case":"Wrap","payload":[{"enum":{"case":"Value","payload":[],"type":"Inner"}}],"type":"Box"}}`
	if got := string(encoded); got != want {
		t.Fatalf("enum JSON = %s, want %s", got, want)
	}
}

func TestCBackendUnitEnumMatchParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	source := "enum Status { Ready, Running, Done }\nlet status = Status.Running\nemit match status { Status.Ready => 1, Status.Running => 2, default => 3 }\n"
	prog, err := Parse("<native-unit-enum-match>", source)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile C: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "unit-enum-match")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("run executable: %v\n%s", err, out)
	}
	if got, want := string(out), valuesOutput(want); got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
	if !strings.Contains(artifact.Source, "oenumis(") {
		t.Fatalf("generated C did not dispatch on enum tags:\n%s", artifact.Source)
	}
}

func TestCBackendPayloadEnumMatchBindingParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	source := "enum Result { Ok(value), Err(message) }\nlet result = Result.Ok(7)\nemit match result { Result.Ok(value) => value, Result.Err(message) => 0 }\n"
	prog, err := Parse("<native-payload-enum-match>", source)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile C: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "payload-enum-match")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("run executable: %v\n%s", err, out)
	}
	if got, want := string(out), valuesOutput(want); got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
	if !strings.Contains(artifact.Source, "oenumpayload(") {
		t.Fatalf("generated C did not bind enum payload values:\n%s", artifact.Source)
	}
}

func TestCBackendGuardedEnumMatchParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	source := "enum Result { Ok(value), Err(message) }\nlet result = Result.Ok(7)\nemit match result { Result.Ok(value) when value > 10 => value, Result.Ok(value) => value, default => 0 }\n"
	prog, err := Parse("<native-guarded-enum-match>", source)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile C: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "guarded-enum-match")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("run executable: %v\n%s", err, out)
	}
	if got, want := string(out), valuesOutput(want); got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
	if !strings.Contains(artifact.Source, "block_main_match_guard") {
		t.Fatalf("generated C did not keep a separate guarded-match block:\n%s", artifact.Source)
	}
}

func TestCBackendGroupedEnumMatchParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	source := "enum Result { Ok(value), Err(value) }\nlet result = Result.Err(7)\nemit match result { Result.Ok(value) | Result.Err(value) => value, default => 0 }\n"
	prog, err := Parse("<native-grouped-enum-match>", source)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile C: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "grouped-enum-match")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("run executable: %v\n%s", err, out)
	}
	if got, want := string(out), valuesOutput(want); got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
	if !strings.Contains(artifact.Source, "block_main_match_alternative") {
		t.Fatalf("generated C did not retain grouped-alternative branches:\n%s", artifact.Source)
	}
}

// TestFormatMacrosAndBlocks exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestFormatMacrosAndBlocks(t *testing.T) {
	formatted, err := FormatSource("<test>", `macro shout(x)=>cat(x,suffix); emit { let x=2; x+1 }`)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	const want = "macro shout(x) => cat(x, suffix)\nemit { let x = 2u64; x + 1u64 }\n"
	if formatted != want {
		t.Fatalf("formatted = %q, want %q", formatted, want)
	}
}

// TestModuleExportsAreExplicit exercises the behavior described by its body
// and locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestModuleExportsAreExplicit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sigil.osr")
	source := `
package sigil
let hidden = "secret"
let visible = slug("Hello, World")
export visible
`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("write module: %v", err)
	}

	prog, err := Parse("<test>", "import "+quoteString(path)+" as sigil\nemit sigil.visible\n")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 1; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), `"hello-world"`; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}

	prog, err = Parse("<test>", "import "+quoteString(path)+" as sigil\nemit sigil.hidden\n")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if _, err := rt.Run(context.Background(), prog); err == nil {
		t.Fatalf("expected hidden export access to fail")
	}
}

// TestPackageVersioning exercises the behavior described by its body and locks
// in the observable language, runtime, or CLI semantics that the surrounding
// package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestPackageVersioning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gizmo.osr")
	source := `
package gizmo 2.1.0
export answer
let answer = 42
`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("write module: %v", err)
	}

	mainPath := filepath.Join(dir, "main.osr")
	prog, err := Parse(mainPath, "import "+quoteString(path+"@2.1.0")+" as gizmo\nimport \"std/text@1\" as text\nemit gizmo.answer\nemit text.slug(\"Versioned Text\")\n")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 2; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), "42u64"; got != want {
		t.Fatalf("output[0] = %s, want %s", got, want)
	}
	if got, want := out[1].String(), `"versioned-text"`; got != want {
		t.Fatalf("output[1] = %s, want %s", got, want)
	}

	lockPath := filepath.Join(dir, ".osiris.lock")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read lockfile: %v", err)
	}
	if !strings.Contains(string(data), `"package": "gizmo"`) || !strings.Contains(string(data), `"version": "2.1.0"`) {
		t.Fatalf("lockfile missing package metadata: %s", string(data))
	}

	bad, err := Parse(mainPath, "import "+quoteString(path+"@2.1.1")+" as gizmo\nemit gizmo.answer\n")
	if err != nil {
		t.Fatalf("parse mismatch import failed: %v", err)
	}
	if _, err := rt.Run(context.Background(), bad); err == nil {
		t.Fatalf("expected mismatched version import to fail")
	}
}

// TestBitwiseAndByteHelpers exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestBitwiseAndByteHelpers(t *testing.T) {
	prog, err := Parse("<test>", `
emit 0xff & 0b1010
emit rotl(u8(1), 1)
emit hex("A")
emit pack(bytes("Hi"))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 4; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), "10u64"; got != want {
		t.Fatalf("output[0] = %s, want %s", got, want)
	}
	if got, want := out[1].String(), "2u8"; got != want {
		t.Fatalf("output[1] = %s, want %s", got, want)
	}
	if got, want := out[2].String(), `"41"`; got != want {
		t.Fatalf("output[2] = %s, want %s", got, want)
	}
	if got, want := out[3].String(), `"Hi"`; got != want {
		t.Fatalf("output[3] = %s, want %s", got, want)
	}
}

// TestBinaryCodecRoundTrip exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestBinaryCodecRoundTrip(t *testing.T) {
	prog, err := Parse("<test>", `
emit decode(encode([1, u8(2), "go"]))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 1; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), `[1u64, 2u8, "go"]`; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}
}

// TestMatchExpression exercises the behavior described by its body and locks
// in the observable language, runtime, or CLI semantics that the surrounding
// package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestMatchExpression(t *testing.T) {
	prog, err := Parse("<test>", `
emit match [1, 2] { [a, b] => a + b; _ => 0 }
emit match [7, 7] { [x, x] => x; _ => 0 }
emit match 9 { 1 => "nope"; _ => "fallback" }
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 3; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), "3u64"; got != want {
		t.Fatalf("output[0] = %s, want %s", got, want)
	}
	if got, want := out[1].String(), "7u64"; got != want {
		t.Fatalf("output[1] = %s, want %s", got, want)
	}
	if got, want := out[2].String(), `"fallback"`; got != want {
		t.Fatalf("output[2] = %s, want %s", got, want)
	}
}

// TestSwitchExpression exercises the behavior described by its body and locks
// in the observable language, runtime, or CLI semantics that the surrounding
// package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestSwitchExpression(t *testing.T) {
	prog, err := Parse("<test>", `
emit switch [1, 2] { [a, b] => a + b; _ => 0 }
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 1; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), "3u64"; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}
	formatted := FormatProgram(prog)
	if !strings.Contains(formatted, "switch ") {
		t.Fatalf("formatted output missing switch keyword: %s", formatted)
	}
}

// TestVizProgramDOT exercises the behavior described by its body and locks in
// the observable language, runtime, or CLI semantics that the surrounding
// package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestVizProgramDOT(t *testing.T) {
	prog, err := Parse("<test>", `emit match [1, 2] { [a, b] => a + b; _ => 0 }`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	dot := VizProgramDOT(prog)
	if !strings.Contains(dot, "digraph osiris-ast") {
		t.Fatalf("dot missing graph header: %s", dot)
	}
	if !strings.Contains(dot, "match") || !strings.Contains(dot, "binary +") {
		t.Fatalf("dot missing match structure: %s", dot)
	}
}

// TestVizImportsDOT exercises the behavior described by its body and locks in
// the observable language, runtime, or CLI semantics that the surrounding
// package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestVizImportsDOT(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "child.osr")
	parent := filepath.Join(dir, "parent.osr")
	if err := os.WriteFile(child, []byte(`package child
export value
let value = 7
`), 0o644); err != nil {
		t.Fatalf("write child: %v", err)
	}
	if err := os.WriteFile(parent, []byte(`import "./child.osr" as child
emit child.value
`), 0o644); err != nil {
		t.Fatalf("write parent: %v", err)
	}
	src, err := os.ReadFile(parent)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	dot, err := VizImportsDOT(parent, string(src))
	if err != nil {
		t.Fatalf("viz imports: %v", err)
	}
	if !strings.Contains(dot, "digraph osiris-imports") {
		t.Fatalf("dot missing graph header: %s", dot)
	}
	if !strings.Contains(dot, "child.osr") {
		t.Fatalf("dot missing child import: %s", dot)
	}
}

// TestDataPipelineBuiltins exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestDataPipelineBuiltins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "payload.txt")
	prog, err := Parse("<test>", `
emit write(PATH, "alpha\nbeta")
emit read(PATH)
emit lines(read(PATH))
emit csv("a,b\n1,2")
emit jsonparse("{\"x\":1,\"y\":[true,false]}")
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	replaced := strings.ReplaceAll(strings.ReplaceAll(FormatProgram(prog), "PATH", quoteString(path)), "\"alpha\\nbeta\"", "\"alpha\\nbeta\"")
	prog, err = Parse("<test>", replaced)
	if err != nil {
		t.Fatalf("reparse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 5; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), "true"; got != want {
		t.Fatalf("output[0] = %s, want %s", got, want)
	}
	if got, want := out[1].String(), `"alpha\nbeta"`; got != want {
		t.Fatalf("output[1] = %s, want %s", got, want)
	}
	if got, want := out[2].String(), `["alpha", "beta"]`; got != want {
		t.Fatalf("output[2] = %s, want %s", got, want)
	}
	if got, want := out[3].String(), `[["a", "b"], ["1", "2"]]`; got != want {
		t.Fatalf("output[3] = %s, want %s", got, want)
	}
	if got, want := out[4].String(), `[["x", 1u64], ["y", [true, false]]]`; got != want {
		t.Fatalf("output[4] = %s, want %s", got, want)
	}
}

// TestWASMBackendSnapshot exercises the behavior described by its body and
// locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestWASMBackendSnapshot(t *testing.T) {
	prog, err := Parse("<test>", `
let seed = "osiris"
emit seed
emit 7 + 3
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	artifact, err := CompileWASM(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile wasm failed: %v", err)
	}
	if !strings.Contains(artifact.WAT, "(module") || !strings.Contains(artifact.WAT, "result_len") {
		t.Fatalf("unexpected wat: %s", artifact.WAT)
	}
	if !strings.Contains(artifact.WAT, `"[\"osiris\",10]"`) {
		t.Fatalf("wat missing payload: %s", artifact.WAT)
	}
	if len(artifact.Binary) < 8 || !bytes.Equal(artifact.Binary[:8], []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}) {
		t.Fatalf("unexpected wasm header: %x", artifact.Binary[:8])
	}
	if artifact.JSON != `["osiris",10]` {
		t.Fatalf("json = %s, want %s", artifact.JSON, `["osiris",10]`)
	}
}

// TestCBackendLowering exercises the behavior described by its body and locks
// in the observable language, runtime, or CLI semantics that the surrounding
// package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestCBackendLowering(t *testing.T) {
	prog, err := Parse("<test>", `
emit "osiris"
emit 7 + 3
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	if !strings.Contains(artifact.Source, "Osiris native C lowering") || !strings.Contains(artifact.Source, "= oadd(") {
		t.Fatalf("unexpected c source:\n%s", artifact.Source)
	}
	if strings.Contains(artifact.Source, "osiris_outputs") {
		t.Fatalf("c source still contains snapshot output table:\n%s", artifact.Source)
	}
}

func TestCBackendDeterministicSource(t *testing.T) {
	prog, err := Parse("<test>", `
func twice_after_one(x) {
  let y = x + 1
  return y * 2
}
let values = [1, 2, 3]
let tensor_values = tensor([1.0, 2.0, 3.0])
emit twice_after_one(4)
emit len(values)
emit tensor_values + tensor_values
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	first, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("first compile failed: %v", err)
	}
	second, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("second compile failed: %v", err)
	}
	if first.Source != second.Source {
		t.Fatalf("generated C is not deterministic\n--- first ---\n%s\n--- second ---\n%s", first.Source, second.Source)
	}
	if strings.Contains(first.Source, "osiris_outputs") || !strings.Contains(first.Source, "Osiris native C lowering") {
		t.Fatalf("generated C does not describe real native lowering:\n%s", first.Source)
	}
}

// TestCBackendRuntimeComponentGolden pins the complete generated C source for
// a representative complex-tensor program. The digest is an intentionally
// compact golden: changing component order, whitespace, or an emitted helper
// requires an explicit review of the generated artifact and this value.
func TestCBackendRuntimeComponentGolden(t *testing.T) {
	prog, err := Parse("<runtime-component-golden>", `
let values = tensor([[complex(1, 2), complex(3, -4)]])
emit transpose(values)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	first, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("first C compile failed: %v", err)
	}
	second, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("second C compile failed: %v", err)
	}
	if first.Source != second.Source {
		t.Fatal("runtime-component C source is not deterministic")
	}
	if !strings.Contains(first.Source, "static OValue oat_real(") ||
		!strings.Contains(first.Source, "static OValue oat(OValue x, OValue index)") ||
		strings.Index(first.Source, "static OValue oat_real(") > strings.Index(first.Source, "static OValue oat(OValue x, OValue index)") {
		t.Fatal("runtime components were not emitted in the required base-then-complex order")
	}
	got := fmt.Sprintf("%x", sha256.Sum256([]byte(first.Source)))
	const want = "e001629a58ac1e2a83c6fd623f696bf2f9d4833d34814ebc3ad5013d374be16c"
	if got != want {
		t.Fatalf("runtime-component generated-C golden changed: got %s, want %s", got, want)
	}
}

func TestCBackendReportsTaggedFallbackReason(t *testing.T) {
	prog, err := Parse("<test>", `
let values = [1, 2]
emit len(values)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	if !strings.Contains(artifact.Source, "Osiris native fallback: list values require the tagged ABI") {
		t.Fatalf("generated C did not report its tagged fallback:\n%s", artifact.Source)
	}
}

func TestCBackendRejectsUnsupportedIROperation(t *testing.T) {
	prog, err := Parse("<test>", "emit Meter")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	_, err = CompileC(context.Background(), NewRuntime(), prog)
	if err == nil || !strings.Contains(err.Error(), `named quantity "Meter"`) {
		t.Fatalf("CompileC error = %v, want unsupported named-quantity diagnostic", err)
	}
}

func FuzzCBackendSource(f *testing.F) {
	for _, source := range []string{
		"emit 1 + 2",
		"let values = [1, 2]; emit len(values)",
		"func add(a, b) => a + b\nemit add(2, 3)",
		"emit tensor([1.0, 2.0]) + tensor([3.0, 4.0])",
	} {
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 16<<10 {
			t.Skip()
		}
		prog, err := Parse("<fuzz>", source)
		if err != nil {
			return
		}
		artifact, err := CompileC(context.Background(), NewRuntime(), prog)
		if err != nil {
			return
		}
		if artifact == nil || artifact.Source == "" {
			t.Fatal("successful C compile returned no source")
		}
		if strings.Contains(artifact.Source, "osiris_outputs") {
			t.Fatalf("C backend regressed to a snapshot output table:\n%s", artifact.Source)
		}
	})
}

func BenchmarkCBackendUnboxedScalarLoop(b *testing.B) {
	if _, err := exec.LookPath("cc"); err != nil {
		b.Skip("cc not available")
	}
	prog, err := Parse("<benchmark>", `
let i = 0
let total = 0
while i < 1000000 {
  let total = total + i
  let i = i + 1
}
emit total
`)
	if err != nil {
		b.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		b.Fatal(err)
	}
	exePath := filepath.Join(b.TempDir(), "loop")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := exec.Command(exePath).Run(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCBackendSmallStaticForListUnrolled(b *testing.B) {
	if _, err := exec.LookPath("cc"); err != nil {
		b.Skip("cc not available")
	}
	prog, err := Parse("<benchmark>", "let values = [2, 3, 5, 7]\nlet total = 0\nfor value in values { let total = total + value }\nemit total\n")
	if err != nil {
		b.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		b.Fatal(err)
	}
	exePath := filepath.Join(b.TempDir(), "small-static-for")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := exec.Command(exePath).Run(); err != nil {
			b.Fatal(err)
		}
	}
}

// TestCBackendObjectAndExecutable exercises the behavior described by its body
// and locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestCBackendObjectAndExecutable(t *testing.T) {
	compiler, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("cc not available")
	}
	_ = compiler
	prog, err := Parse("<test>", `
emit "osiris"
emit 7 + 3
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	dir := t.TempDir()
	objPath := filepath.Join(dir, "snapshot.o")
	exePath := filepath.Join(dir, "snapshot")
	if err := CompileCObject(context.Background(), artifact.Source, objPath); err != nil {
		t.Fatalf("compile object failed: %v", err)
	}
	if info, err := os.Stat(objPath); err != nil {
		t.Fatalf("stat object failed: %v", err)
	} else if info.Size() == 0 {
		t.Fatalf("object file is empty")
	}
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v", err)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got, want := string(out), "\"osiris\"\n10u64\n"; got != want {
		t.Fatalf("executable output = %q, want %q", got, want)
	}
}

func TestCBackendExecutableCache(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit 7 + 3")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	firstPath := filepath.Join(dir, "first")
	first, err := CompileCExecutableCached(context.Background(), artifact.Source, firstPath, cacheDir)
	if err != nil {
		t.Fatalf("first cached compile failed: %v", err)
	}
	if first.CacheHit || first.Key == "" || first.SourceSHA256 == "" {
		t.Fatalf("first cache manifest = %#v, want a miss with identity", first)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, first.Key+".json")); err != nil {
		t.Fatalf("cache manifest missing: %v", err)
	}
	secondPath := filepath.Join(dir, "second")
	second, err := CompileCExecutableCached(context.Background(), artifact.Source, secondPath, cacheDir)
	if err != nil {
		t.Fatalf("second cached compile failed: %v", err)
	}
	if !second.CacheHit || second.Key != first.Key {
		t.Fatalf("second cache manifest = %#v, want hit for %s", second, first.Key)
	}
	out, err := exec.Command(secondPath).CombinedOutput()
	if err != nil {
		t.Fatalf("cached executable failed: %v\n%s", err, out)
	}
	if got, want := string(out), "10u64\n"; got != want {
		t.Fatalf("cached executable output = %q, want %q", got, want)
	}
}

func TestCBackendExecutableCacheIncludesProviderLinkFlags(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<provider-cache-flags>", "emit 7 + 3")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	first, err := CompileCExecutableCachedWithOptions(context.Background(), artifact.Source, filepath.Join(dir, "first"), filepath.Join(dir, "cache"), CCompileOptions{LinalgProviderFlags: []string{"-D__OSIRIS_PROVIDER_LINK_A=1"}})
	if err != nil {
		t.Fatalf("first cached compile failed: %v", err)
	}
	second, err := CompileCExecutableCachedWithOptions(context.Background(), artifact.Source, filepath.Join(dir, "second"), filepath.Join(dir, "cache"), CCompileOptions{LinalgProviderFlags: []string{"-D__OSIRIS_PROVIDER_LINK_B=1"}})
	if err != nil {
		t.Fatalf("second cached compile failed: %v", err)
	}
	if first.Key == second.Key || first.SourceSHA256 != second.SourceSHA256 {
		t.Fatalf("provider link flags did not participate in cache identity: first=%#v second=%#v", first, second)
	}
}

func TestCBackendExecutableCacheIncludesProviderSourceContent(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<provider-cache-source>", "emit 7 + 3")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	providerPath := filepath.Join(dir, "provider.c")
	writeProvider := func(marker string) {
		t.Helper()
		if err := os.WriteFile(providerPath, []byte("int osiris_provider_"+marker+"(void) { return 0; }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeProvider("first")
	options := CCompileOptions{LinalgProviderSource: providerPath}
	first, err := CompileCExecutableCachedWithOptions(context.Background(), artifact.Source, filepath.Join(dir, "first"), filepath.Join(dir, "cache"), options)
	if err != nil {
		t.Fatalf("first cached compile failed: %v", err)
	}
	writeProvider("second")
	second, err := CompileCExecutableCachedWithOptions(context.Background(), artifact.Source, filepath.Join(dir, "second"), filepath.Join(dir, "cache"), options)
	if err != nil {
		t.Fatalf("second cached compile failed: %v", err)
	}
	if first.Key == second.Key || first.LinalgProviderSHA256 == second.LinalgProviderSHA256 {
		t.Fatalf("provider source content did not participate in cache identity: first=%#v second=%#v", first, second)
	}
}

func TestCBackendControlFlowParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
let n = 0
while n < 3 { let n = n + 1 }
emit if(n == 3, n, 0)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "control-flow")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v", err)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendUnboxedScalarLoop(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
let n = 0
let total = 0
while n < 100 {
  let total = total + n
  let n = n + 1
}
emit total
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	mainAt := strings.LastIndex(artifact.Source, "int main(void) {")
	if mainAt < 0 {
		t.Fatalf("generated C has no main:\n%s", artifact.Source)
	}
	mainSource := artifact.Source[mainAt:]
	if !strings.Contains(mainSource, "uint64_t s") || strings.Contains(mainSource, "OValue s") || strings.Contains(mainSource, "oadd(") {
		t.Fatalf("scalar loop did not use unboxed lowering:\n%s", mainSource)
	}
	headerAt := strings.Index(mainSource, "block_main_while_header")
	bodyAt := strings.Index(mainSource, "block_main_while_body")
	if headerAt < 0 || bodyAt < headerAt || strings.Contains(mainSource[headerAt:bodyAt], "100uLL") {
		t.Fatalf("loop-invariant bound was not hoisted from the loop header:\n%s", mainSource)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-loop")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got, want := string(out), "4950u64\n"; got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
}

func TestCBackendProofGatedLoopInvariantCodeMotionParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	source := "let i = 0\nlet total = 0\nwhile i < 4 {\n  let scale = 2 + 3\n  let total = total + scale\n  let i = i + 1\n}\nemit total\n"
	prog, err := Parse("<native-licm>", source)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := rt.RunVM(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if got := valuesOutput(vm); got != valuesOutput(want) {
		t.Fatalf("VM output = %q, want %q", got, valuesOutput(want))
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile C: %v", err)
	}
	mainAt := strings.LastIndex(artifact.Source, "int main(void) {")
	if mainAt < 0 {
		t.Fatalf("generated C has no main:\n%s", artifact.Source)
	}
	mainSource := artifact.Source[mainAt:]
	entryAt := strings.Index(mainSource, "block_main_entry")
	headerAt := strings.Index(mainSource, "block_main_while_header")
	bodyAt := strings.Index(mainSource, "block_main_while_body")
	if entryAt < 0 || headerAt < entryAt || bodyAt < headerAt {
		t.Fatalf("generated C lost canonical loop shape:\n%s", mainSource)
	}
	if strings.Contains(mainSource[bodyAt:], "ouintbits(2uLL, 64)") || strings.Contains(mainSource[bodyAt:], "ouintbits(3uLL, 64)") {
		t.Fatalf("numeric invariant literals remained in loop body:\n%s", mainSource)
	}
	exePath := filepath.Join(t.TempDir(), "native-licm")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("run executable: %v\n%s", err, out)
	}
	if got, want := string(out), valuesOutput(want); got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
}

func TestCBackendProofGatedForListBoundsEliminationParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	source := "let values = [2, 3, 5, 7]\nlet total = 0\nfor value in values {\n  let total = total + value\n}\nemit total\n"
	prog, err := Parse("<native-for-bounds>", source)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := rt.RunVM(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if got := valuesOutput(vm); got != valuesOutput(want) {
		t.Fatalf("VM output = %q, want %q", got, valuesOutput(want))
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile C: %v", err)
	}
	mainAt := strings.LastIndex(artifact.Source, "int main(void) {")
	if mainAt < 0 {
		t.Fatalf("generated C has no main:\n%s", artifact.Source)
	}
	bodyAt := strings.Index(artifact.Source[mainAt:], "block_main_for_body")
	if bodyAt >= 0 {
		body := artifact.Source[mainAt+bodyAt:]
		if !strings.Contains(body, "olistatunchecked(") {
			t.Fatalf("proved list iteration did not remove dynamic bounds check:\n%s", body)
		}
	} else if strings.Contains(artifact.Source[mainAt:], " = oat(") {
		t.Fatalf("unrolled list iteration retained a dynamic bounds check:\n%s", artifact.Source[mainAt:])
	}
	exePath := filepath.Join(t.TempDir(), "native-for-bounds")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("run executable: %v\n%s", err, out)
	}
	if got, want := string(out), valuesOutput(want); got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
}

func TestCBackendSmallStaticForListUnrollingParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	source := "let values = [2, 3, 5, 7]\nlet total = 0\nfor value in values {\n  let total = total + value\n}\nemit total\n"
	prog, err := Parse("<native-for-unroll>", source)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := rt.RunVM(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if got := valuesOutput(vm); got != valuesOutput(want) {
		t.Fatalf("VM output = %q, want %q", got, valuesOutput(want))
	}
	ir, err := rt.BuildIR(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	optimizeNativeLoops(ir)
	optimized := FormatIRProgram(ir)
	if !strings.Contains(optimized, ".unroll0") || !strings.Contains(optimized, "jump main.for.exit") {
		t.Fatalf("small fixed-trip loop was not unrolled:\n%s", optimized)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile C: %v", err)
	}
	mainAt := strings.LastIndex(artifact.Source, "int main(void) {")
	if mainAt < 0 {
		t.Fatalf("generated C lost canonical for labels:\n%s", artifact.Source)
	}
	headerAt := strings.Index(artifact.Source[mainAt:], "block_main_for_header")
	if headerAt >= 0 {
		t.Fatalf("generated C retained the unreachable unrolled loop header:\n%s", artifact.Source[mainAt:])
	}
	entry := artifact.Source[mainAt:]
	if !strings.Contains(entry, "goto block_main_for_exit") || strings.Contains(entry, "goto block_main_for_header") {
		t.Fatalf("generated C entry did not bypass the unrolled loop header:\n%s", entry)
	}
	exePath := filepath.Join(t.TempDir(), "native-for-unroll")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("run executable: %v\n%s", err, out)
	}
	if got, want := string(out), valuesOutput(want); got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
}

func TestCBackendSmallStaticForListUnrollingIsProfitabilityGated(t *testing.T) {
	prog, err := Parse("<native-for-unroll-gate>", "let values = [1, 2, 3, 4, 5]\nlet total = 0\nfor value in values { let total = total + value }\nemit total\n")
	if err != nil {
		t.Fatal(err)
	}
	ir, err := NewRuntime().BuildIR(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	optimizeNativeLoops(ir)
	optimized := FormatIRProgram(ir)
	if strings.Contains(optimized, ".unroll") || !strings.Contains(optimized, "jump main.for.header") {
		t.Fatalf("unrolling ignored the bounded profitability gate:\n%s", optimized)
	}
}

func TestCBackendBoundsEliminationLeavesDirectAtChecked(t *testing.T) {
	prog, err := Parse("<native-direct-at>", "let values = [2, 3]\nemit at(values, 4)\n")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile C: %v", err)
	}
	mainAt := strings.LastIndex(artifact.Source, "int main(void) {")
	if mainAt < 0 {
		t.Fatalf("generated C has no main:\n%s", artifact.Source)
	}
	mainSource := artifact.Source[mainAt:]
	if !strings.Contains(mainSource, "= oat(") || strings.Contains(mainSource, "olistatunchecked(") {
		t.Fatalf("direct at lost its checked lowering:\n%s", mainSource)
	}
}

func TestCBackendUnboxedFloatWidthParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit 1.25f32 + 2.5f32\nemit 2.0f32\nemit 1.25f64 + 2.5f64")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	mainAt := strings.LastIndex(artifact.Source, "int main(void) {")
	if mainAt < 0 || strings.Contains(artifact.Source[mainAt:], "OValue s") {
		t.Fatalf("float program did not use unboxed lowering:\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-floats")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendUnboxedLogicalBranchParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit true && false\nemit false && true\nemit true || false\nemit false || true")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	mainAt := strings.LastIndex(artifact.Source, "int main(void) {")
	if mainAt < 0 || strings.Contains(artifact.Source[mainAt:], "Osiris native fallback") || strings.Contains(artifact.Source[mainAt:], "OValue s") {
		t.Fatalf("logical branch program did not use unboxed lowering\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-logical-branches")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatDivisionParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit 7.5f32 / 2.0f32\nemit -22.0f64 / 7.0f64")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float division unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-division")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatDivisionZeroIsReported(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit 1.0f64 / 0.0f64")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float division unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-division-zero")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "osiris native error: division by zero") {
		t.Fatalf("native float division result = err %v, output %q\n--- C ---\n%s", err, out, artifact.Source)
	}
}

func TestCBackendUnboxedFloatSqrtParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "import \"std/math\" as math\nemit math.sqrt(2.25f32)\nemit math.sqrt(81.0f64)")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float sqrt unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-sqrt")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatSqrtDomainIsReported(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "import \"std/math\" as math\nemit math.sqrt(-1.0f64)")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float sqrt unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-sqrt-domain")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "osiris native error: sqrt domain error") {
		t.Fatalf("native float sqrt result = err %v, output %q\n--- C ---\n%s", err, out, artifact.Source)
	}
}

func TestCBackendUnboxedFloatCbrtParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "import \"std/math\" as math\nemit math.cbrt(-8.0f32)\nemit math.cbrt(27.0f64)")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float cbrt unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-cbrt")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatExpm1Parity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "import \"std/math\" as math\nemit math.expm1(0.5f32)\nemit math.expm1(-0.5f64)")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float expm1 unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-expm1")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatLog1pParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "import \"std/math\" as math\nemit math.log1p(0.5f32)\nemit math.log1p(-0.5f64)")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float log1p unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-log1p")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatErfParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "import \"std/math\" as math\nemit math.erf(0.5f32)\nemit math.erf(-0.5f64)")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float erf unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-erf")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatErfcParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "import \"std/math\" as math\nemit math.erfc(0.5f32)\nemit math.erfc(-0.5f64)")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float erfc unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-erfc")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatRoundParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "import \"std/math\" as math\nemit math.round(1.5f32)\nemit math.round(-1.5f64)")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float round unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-round")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatAbsParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "import \"std/math\" as math\nemit math.abs(-2.5f32)\nemit math.abs(-2.5f64)")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float abs unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-abs")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatMathLoopParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
let i = 1.0f64
let total = 0.0f64
while i < 5.0f64 {
  let total = total + math.sqrt(i)
  let i = i + 1.0f64
}
emit total
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	mainAt := strings.LastIndex(artifact.Source, "int main(void) {")
	if mainAt < 0 || strings.Contains(artifact.Source[mainAt:], "Osiris native fallback") || strings.Contains(artifact.Source[mainAt:], "OValue s") {
		t.Fatalf("float math loop unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	if !strings.Contains(artifact.Source[mainAt:], "sqrt(") {
		t.Fatalf("generated loop does not call direct sqrt\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-math-loop")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatClampParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.clamp(-2.0f32, 0.0f32, 10.0f32)
emit math.clamp(12.0f32, 0.0f32, 10.0f32)
emit math.clamp(4.0f64, 0.0f64, 10.0f64)
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float clamp unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-clamp")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatExtremaParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.min(4.0f32, -2.0f32, 3.0f32)
emit math.max(4.0f32, -2.0f32, 3.0f32)
emit math.min(4.0f64, -2.0f64, 3.0f64)
emit math.max(4.0f64, -2.0f64, 3.0f64)
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float extrema unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-extrema")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatFloorCeilParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.floor(1.75f32)
emit math.ceil(-1.25f32)
emit math.floor(-1.25f64)
emit math.ceil(1.75f64)
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float floor/ceil unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-floor-ceil")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatHypotParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.hypot(3.0f32, 4.0f32)
emit math.hypot(2.0f64, 3.0f64, 6.0f64)
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float hypot unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-hypot")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatTrigonometricParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.sin(0.5f32)
emit math.cos(0.5f32)
emit math.tan(0.5f32)
emit math.sin(-0.5f64)
emit math.cos(-0.5f64)
emit math.tan(-0.5f64)
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float trigonometry unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-trigonometry")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatInverseTrigonometricParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.asin(0.5f32)
emit math.acos(-0.5f32)
emit math.atan(0.5f32)
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float inverse trigonometry unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-inverse-trigonometry")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatInverseTrigonometricDomainIsReported(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	for _, test := range []struct {
		name    string
		source  string
		message string
	}{
		{name: "asin", source: "emit math.asin(2.0f32)", message: "asin domain error"},
		{name: "acos", source: "emit math.acos(-2.0f32)", message: "acos domain error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			prog, err := Parse("<test>", "import \"std/math\" as math\n"+test.source)
			if err != nil {
				t.Fatal(err)
			}
			artifact, err := CompileC(context.Background(), NewRuntime(), prog)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(artifact.Source, "Osiris native fallback") {
				t.Fatalf("float %s unexpectedly used tagged fallback\n--- C ---\n%s", test.name, artifact.Source)
			}
			exePath := filepath.Join(t.TempDir(), "unboxed-float-"+test.name+"-domain")
			if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
				t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
			}
			out, err := exec.Command(exePath).CombinedOutput()
			if err == nil || !strings.Contains(string(out), "osiris native error: "+test.message) {
				t.Fatalf("native %s domain = err %v, output %q\n--- C ---\n%s", test.name, err, out, artifact.Source)
			}
		})
	}
}

func TestCBackendFloatLibmF64IsRejected(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
	}{
		{name: "inverse", source: "emit math.asin(0.5f64)\nemit math.atan(-0.5f64)"},
		{name: "hyperbolic", source: "emit math.sinh(0.5f64)\nemit math.acosh(2.0f64)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			prog, err := Parse("<test>", "import \"std/math\" as math\n"+test.source)
			if err != nil {
				t.Fatal(err)
			}
			_, err = CompileC(context.Background(), NewRuntime(), prog)
			if err == nil || !strings.Contains(err.Error(), "requires a proven f32 scalar input") {
				t.Fatalf("CompileC error = %v, want f64 libm diagnostic", err)
			}
		})
	}
}

func TestCBackendUnboxedFloatHyperbolicParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.sinh(0.5f32)
emit math.cosh(0.5f32)
emit math.tanh(-0.5f32)
emit math.asinh(-0.5f32)
emit math.acosh(2.0f32)
emit math.atanh(0.5f32)
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float hyperbolic functions unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-hyperbolic")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatHyperbolicDomainIsReported(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	for _, test := range []struct {
		name    string
		source  string
		message string
	}{
		{name: "acosh", source: "emit math.acosh(0.5f32)", message: "acosh domain error"},
		{name: "atanh", source: "emit math.atanh(1.0f32)", message: "atanh domain error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			prog, err := Parse("<test>", "import \"std/math\" as math\n"+test.source)
			if err != nil {
				t.Fatal(err)
			}
			artifact, err := CompileC(context.Background(), NewRuntime(), prog)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(artifact.Source, "Osiris native fallback") {
				t.Fatalf("float %s unexpectedly used tagged fallback\n--- C ---\n%s", test.name, artifact.Source)
			}
			exePath := filepath.Join(t.TempDir(), "unboxed-float-"+test.name+"-domain")
			if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
				t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
			}
			out, err := exec.Command(exePath).CombinedOutput()
			if err == nil || !strings.Contains(string(out), "osiris native error: "+test.message) {
				t.Fatalf("native %s domain = err %v, output %q\n--- C ---\n%s", test.name, err, out, artifact.Source)
			}
		})
	}
}

func TestCBackendUnboxedFloatComparisonMathParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.sign(-2.0f32)
emit math.sign(0.0f64)
emit math.step(-2.0f32)
emit math.step(2.0f64)
emit math.heaviside(-2.0f32)
emit math.heaviside(0.0f64)
emit math.relu(-2.0f32)
emit math.relu(2.0f64)
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("comparison math unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-comparison-math")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedIntegerComparisonMathParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.sign(-2i16)
emit math.step(2u8)
emit math.heaviside(0u32)
emit math.relu(-2i32)
emit math.relu(2u64)
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("integer comparison math unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-integer-comparison-math")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedExactSignedLiteralDivisionParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
emit 12i32 / 3i32
emit 8i64 / 2i64
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("exact signed literal division unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-exact-signed-division")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendNonExactSignedLiteralDivisionRetainsTaggedDecimalPath(t *testing.T) {
	prog, err := Parse("<test>", "emit 7i32 / 3i32")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("non-exact signed literal division unexpectedly used unboxed lowering\n--- C ---\n%s", artifact.Source)
	}
}

func TestCBackendUnboxedFloatSigmoidParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.sigmoid(-8.0f32)
emit math.sigmoid(0.5f32)
emit math.sigmoid(80.0f32)
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("f32 sigmoid unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-sigmoid")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatSoftplusParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.softplus(-8.0f32)
emit math.softplus(0.5f32)
emit math.softplus(20.0f32)
emit math.softplus(21.0f32)
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("f32 softplus unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-softplus")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatGammaParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.gamma(0.5f32)
emit math.gamma(4.5f32)
emit math.gamma(-0.5f32)
emit math.gamma(-1.0f32)
emit math.lgamma(0.5f32)
emit math.lgamma(4.5f32)
emit math.lgamma(-0.5f32)
emit math.lgamma(-1.0f32)
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("f32 gamma functions unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-gamma")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v, output %q\n--- C ---\n%s", err, out, artifact.Source)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendFloatGammaF64IsRejected(t *testing.T) {
	for _, name := range []string{"gamma", "lgamma"} {
		t.Run(name, func(t *testing.T) {
			prog, err := Parse("<test>", "import \"std/math\" as math\nemit math."+name+"(0.5f64)")
			if err != nil {
				t.Fatal(err)
			}
			_, err = CompileC(context.Background(), NewRuntime(), prog)
			if err == nil || !strings.Contains(err.Error(), "requires a proven f32 scalar input") {
				t.Fatalf("%s f64 compile error = %v, want parity-safe rejection", name, err)
			}
		})
	}
}

func TestCBackendUnboxedFloatPowParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.pow(2.0f32, 5.75f32)
emit math.pow(8.5f32, 3.5f32)
emit math.pow(8.0f32, 0.5f32)
emit math.expt(3.0f32, 0.5f32)
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("f32 pow functions unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-pow")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v, output %q\n--- C ---\n%s", err, out, artifact.Source)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendFloatPowF64IsRejected(t *testing.T) {
	for _, name := range []string{"pow", "expt"} {
		t.Run(name, func(t *testing.T) {
			prog, err := Parse("<test>", "import \"std/math\" as math\nemit math."+name+"(2.0f64, 3.0f64)")
			if err != nil {
				t.Fatal(err)
			}
			_, err = CompileC(context.Background(), NewRuntime(), prog)
			if err == nil || !strings.Contains(err.Error(), "requires proven f32 scalar inputs") {
				t.Fatalf("%s f64 compile error = %v, want parity-safe rejection", name, err)
			}
		})
	}
}

func TestCBackendExactFloatPowIsRejected(t *testing.T) {
	for _, name := range []string{"pow", "expt"} {
		t.Run(name, func(t *testing.T) {
			prog, err := Parse("<test>", "import \"std/math\" as math\nemit math."+name+"(2.0f32, 3.0f32)")
			if err != nil {
				t.Fatal(err)
			}
			_, err = CompileC(context.Background(), NewRuntime(), prog)
			if err == nil || !strings.Contains(err.Error(), "requires proven f32 scalar inputs") {
				t.Fatalf("%s exact f32 compile error = %v, want result-kind-safe rejection", name, err)
			}
		})
	}
}

func TestCBackendUnboxedFloatComplexProjectionParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.real(-3.5f32)
emit math.imag(-3.5f32)
emit math.conjugate(-3.5f32)
emit math.magnitude(-3.5f32)
emit math.conjugate(-7i16)
emit math.conjugate(255u8)
emit math.real(7.25f64)
emit math.imag(7.25f64)
emit math.conjugate(7.25f64)
emit math.magnitude(-7.25f64)
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("scalar complex projections unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-complex-projections")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v, output %q\n--- C ---\n%s", err, out, artifact.Source)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatCastParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.f32(1.234567890123f64)
emit math.f32(-7.25f32)
emit math.f64(1.234567f32)
emit math.f64(-7.25f64)
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float casts unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-casts")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v, output %q\n--- C ---\n%s", err, out, artifact.Source)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedUnsignedMagnitudeParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.abs(255u8)
emit math.magnitude(255u8)
emit math.abs(18446744073709551615u64)
emit math.magnitude(18446744073709551615u64)
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("unsigned magnitude functions unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-unsigned-magnitude")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v, output %q\n--- C ---\n%s", err, out, artifact.Source)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedSignedMagnitudeParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.abs(-7i16)
emit math.magnitude(-7i16)
emit math.abs(-128i8)
emit math.magnitude(-128i8)
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("signed magnitude functions unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-signed-magnitude")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v, output %q\n--- C ---\n%s", err, out, artifact.Source)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatExpParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "import \"std/math\" as math\nemit math.exp(1.0f32)\nemit math.exp(1.0f64)")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float exp unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-exp")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatExpOverflowIsReported(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "import \"std/math\" as math\nemit math.exp(1000.0f64)")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float exp unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-exp-overflow")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "osiris native error: exp overflow") {
		t.Fatalf("native exp overflow = err %v, output %q\n--- C ---\n%s", err, out, artifact.Source)
	}
}

func TestCBackendUnboxedFloatLogarithmParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "import \"std/math\" as math\nemit math.log(2.0f32)\nemit math.log(81.0f32, 3.0f32)\nemit math.log10(100.0f64)\nemit math.log2(8.0f64)")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float logarithms unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-logarithms")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("native executable failed: %v, output %q\n--- C ---\n%s", err, out, artifact.Source)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatLogarithmDomainIsReported(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	for _, test := range []struct {
		name    string
		source  string
		message string
	}{
		{name: "log", source: "emit math.log(0.0f64)", message: "log domain error"},
		{name: "log-base", source: "emit math.log(1.0f32, 1.0f32)", message: "log domain error"},
		{name: "log10", source: "emit math.log10(-1.0f64)", message: "log10 domain error"},
		{name: "log2", source: "emit math.log2(0.0f64)", message: "log2 domain error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			prog, err := Parse("<test>", "import \"std/math\" as math\n"+test.source)
			if err != nil {
				t.Fatal(err)
			}
			artifact, err := CompileC(context.Background(), NewRuntime(), prog)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(artifact.Source, "Osiris native fallback") {
				t.Fatalf("float %s unexpectedly used tagged fallback\n--- C ---\n%s", test.name, artifact.Source)
			}
			exePath := filepath.Join(t.TempDir(), "unboxed-float-"+test.name+"-domain")
			if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
				t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
			}
			out, err := exec.Command(exePath).CombinedOutput()
			if err == nil || !strings.Contains(string(out), "osiris native error: "+test.message) {
				t.Fatalf("native %s domain = err %v, output %q\n--- C ---\n%s", test.name, err, out, artifact.Source)
			}
		})
	}
}

func TestCBackendFloatBaseLogF64IsRejected(t *testing.T) {
	prog, err := Parse("<test>", "import \"std/math\" as math\nemit math.log(81.0f64, 3.0f64)")
	if err != nil {
		t.Fatal(err)
	}
	_, err = CompileC(context.Background(), NewRuntime(), prog)
	if err == nil || !strings.Contains(err.Error(), "two-argument log requires proven f32 scalar inputs") {
		t.Fatalf("base-log f64 compile error = %v, want parity-safe rejection", err)
	}
}

func TestCBackendUnboxedFloatRemainderParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit 5.5f32 % 2.0f32\nemit -22.0f64 % 7.0f64")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float remainder unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-remainder")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedFloatRemainderZeroIsReported(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit 1.0f32 % 0.0f32")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("float remainder unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-float-remainder-zero")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "osiris native error: remainder by zero") {
		t.Fatalf("native float remainder result = err %v, output %q\n--- C ---\n%s", err, out, artifact.Source)
	}
}

func TestCBackendTaggedFloatRemainderParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
let values = [5.5f64]
emit at(values, 0) % 2.0f64
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("tagged float remainder unexpectedly used scalar lowering\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "tagged-float-remainder")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got, expected := string(out), want[0].String()+"\n"; got != expected {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected, artifact.Source)
	}
}

func TestCBackendTaggedFloatRemainderZeroIsReported(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
let values = [1.0f32]
emit at(values, 0) % 0.0f32
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("tagged float remainder unexpectedly used scalar lowering\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "tagged-float-remainder-zero")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err == nil {
		t.Fatalf("running executable succeeded, want runtime failure\n%s", out)
	}
	if !strings.Contains(string(out), "osiris native error: remainder by zero") {
		t.Fatalf("native tagged float zero remainder = %q\n--- C ---\n%s", out, artifact.Source)
	}
}

func TestCBackendUnboxedScalarLoopIgnoresDeadUndefCarry(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
let i = 0.0f64
let total = 0.0f64
while i < 8.0f64 {
  let transient = (i + 1.0f64) / 3.0f64
  let total = total + i
  let i = i + 1.0f64
}
emit total
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("dead loop carry unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-dead-loop-carry")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got, expected := string(out), want[0].String()+"\n"; got != expected {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected, artifact.Source)
	}
}

func TestCBackendUnboxedUintOverflowIsReported(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit 18446744073709551615 + 1")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-overflow")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "osiris native error: unsigned addition overflow") {
		t.Fatalf("native overflow result = err %v, output %q\n--- C ---\n%s", err, out, artifact.Source)
	}
}

func TestCBackendUnboxedIntArithmeticParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit -17i64 + 5i64\nemit -1234567i64 * 33i64")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("i64 arithmetic unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-i64-arithmetic")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedIntOverflowIsReported(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit 9223372036854775807i64 + 1i64\nemit 3037000500i64 * 3037000500i64")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("i64 arithmetic unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-i64-overflow")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "osiris native error: signed addition overflow") {
		t.Fatalf("native i64 overflow result = err %v, output %q\n--- C ---\n%s", err, out, artifact.Source)
	}
}

func TestCBackendUnboxedUintRemainderParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit 18446744073709551615 % 97")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("u64 remainder unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-u64-remainder")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got, expected := string(out), want[0].String()+"\n"; got != expected {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected, artifact.Source)
	}
}

func TestCBackendUnboxedIntRemainderParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit -17i64 % 5i64\nemit -9223372036854775808i64 % -1i64")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("i64 remainder unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-i64-remainder")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedIntRemainderZeroIsReported(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit 1i64 % 0i64")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("i64 remainder unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-i64-remainder-zero")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "osiris native error: signed remainder by zero") {
		t.Fatalf("native zero remainder result = err %v, output %q\n--- C ---\n%s", err, out, artifact.Source)
	}
}

func TestCBackendUnboxedIntBitwiseParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit -1i64 & 255i64\nemit -256i64 | 15i64\nemit -1i64 ^ 0x55i64")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("i64 bitwise operations unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-i64-bitwise")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedIntShiftParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit 1i64 << 62i64\nemit -1i64 >> 1i64\nemit -4i64 >> 2i64")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("i64 shifts unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-i64-shifts")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var expected strings.Builder
	for _, value := range want {
		expected.WriteString(value.String())
		expected.WriteByte('\n')
	}
	if got := string(out); got != expected.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, expected.String(), artifact.Source)
	}
}

func TestCBackendUnboxedIntShiftRangeIsReported(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit 1i64 << 64i64")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("i64 shift unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-i64-shift-range")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "osiris native error: shift count exceeds signed width") {
		t.Fatalf("native i64 shift result = err %v, output %q\n--- C ---\n%s", err, out, artifact.Source)
	}
}

func TestCBackendUnboxedShiftRangeIsReported(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit 1 << 64")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-shift-range")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "osiris native error: shift count exceeds u64 width") {
		t.Fatalf("native shift result = err %v, output %q\n--- C ---\n%s", err, out, artifact.Source)
	}
}

func TestCBackendUnboxedNarrowSignedShiftRangeIsReported(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit 1i8 << 8i8")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-narrow-signed-shift-range")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "osiris native error: shift count exceeds signed width") {
		t.Fatalf("native shift result = err %v, output %q\n--- C ---\n%s", err, out, artifact.Source)
	}
}

func TestCBackendUnboxedNarrowSignedRemainderZeroIsReported(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit 1i8 % 0i8")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-narrow-signed-remainder-zero")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "osiris native error: signed remainder by zero") {
		t.Fatalf("native remainder result = err %v, output %q\n--- C ---\n%s", err, out, artifact.Source)
	}
}

func TestCBackendDirectFunctionParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
func add(a, b) => a + b
emit add(2, 3)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	mainAt := strings.LastIndex(artifact.Source, "int main(void) {")
	if mainAt < 0 || strings.Contains(artifact.Source[mainAt:], "OValue s") || strings.Contains(artifact.Source[mainAt:], "osiris_fn_") {
		t.Fatalf("direct scalar call was not inlined into the unboxed main:\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "direct-function")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v", err)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got, want := string(out), "5u64\n"; got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
}

func TestCBackendDirectClosureParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
let offset = 3
func plus_offset(x) => x + offset
emit plus_offset(4)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "direct-closure")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got, want := string(out), "7u64\n"; got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
}

func TestCBackendIndirectFunctionValueParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
let offset = 3
func identity(f) => f
func add(x) => x + offset
let f = identity(add)
emit f(4)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "indirect-function")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got, want := string(out), "7u64\n"; got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
}

func TestCBackendDirectFunctionLocalsParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
func twice_after_one(x) {
  let y = x + 1
  return y * 2
}

emit twice_after_one(4)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	mainAt := strings.LastIndex(artifact.Source, "int main(void) {")
	if mainAt < 0 || strings.Contains(artifact.Source[mainAt:], "OValue s") || strings.Contains(artifact.Source[mainAt:], "osiris_fn_") {
		t.Fatalf("straight-line scalar function was not inlined into the unboxed main:\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "direct-function-locals")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v", err)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got, want := string(out), "10u64\n"; got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
}

func TestCBackendComplexScalarParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
func make_complex(real, imag) => complex(real, imag)
func components(z) => real(z) + imag(z)
emit 5 + 3i
emit complex(2, 3) * complex(1, -1)
emit make_complex(2, 3)
emit components(make_complex(2, 3))
emit real(complex(2, 3))
emit imag(complex(2, 3))
emit magnitude(complex(3, 4))
emit abs(complex(3, 4))
emit complex(5, 0) == 5
emit complex(5, 0)
emit -complex(2, 3)
emit conjugate(complex(2, 3))
let complex_values = tensor([complex(1, 2), complex(3, 4)])
emit complex_values
emit shape(complex_values)
emit flatten(complex_values)
emit at(complex_values, 1)
emit sum(complex_values)
emit product(complex_values)
emit dot(complex_values, complex_values)
emit norm(complex_values)
emit outer(complex_values, complex_values)
let complex_matrix = tensor([[complex(1, 2), complex(3, 4)], [complex(5, 6), complex(7, 8)]])
emit at(complex_matrix, [1, 0])
emit at(complex_matrix, [0])
emit reshape(complex_matrix, [4])
emit setat(complex_matrix, [1, 0], complex(9, 10))
emit setat(complex_matrix, [0], tensor([complex(11, 12), complex(13, 14)]))
emit complex_matrix + complex(1, -1)
emit tensor([[complex(1, 1)], [complex(2, 2)]]) + tensor([complex(10, 0), complex(20, 0)])
emit take(complex_matrix, 3)
emit slice(complex_matrix, 0, 1)
emit slice(complex_matrix, 0, 1, 1)
emit conjugate(complex_matrix)
emit transpose(complex_matrix)
emit trace(complex_matrix)
emit det(tensor([[complex(1, 2), complex(0, 0)], [complex(0, 0), complex(7, 8)]]))
emit solve(tensor([[complex(1, 1), complex(0, 0)], [complex(0, 0), complex(2, -1)]]), tensor([complex(2, 2), complex(4, -2)]))
emit inv(tensor([[complex(1, 1), complex(0, 0)], [complex(0, 0), complex(2, -1)]]))
emit qr(tensor([[complex(1, 0), complex(0, 1)], [complex(0, 0), complex(0, 0)]]))
emit matmul(complex_matrix, tensor([[1, 2], [3, 4]]))`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	if !strings.Contains(artifact.Source, "= ocomplexcall(") {
		t.Fatalf("complex builtin was not lowered into generated C:\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "complex-scalars")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v", err)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got, want := string(out), "5+3i\n5+1i\n2+3i\n5\n2\n3\n5\n5\ntrue\n5+0i\n-2-3i\n2-3i\ntensor[2]([1+2i, 3+4i])\n[2u64]\n[1+2i, 3+4i]\n3+4i\n4+6i\n-5+10i\n-10+28i\n5.477225575051661\ntensor[2x2]([-3+4i, -5+10i, -5+10i, -7+24i])\n5+6i\ntensor[2]([1+2i, 3+4i])\ntensor[4]([1+2i, 3+4i, 5+6i, 7+8i])\ntensor[2x2]([1+2i, 3+4i, 9+10i, 7+8i])\ntensor[2x2]([11+12i, 13+14i, 5+6i, 7+8i])\ntensor[2x2]([2+1i, 4+3i, 6+5i, 8+7i])\ntensor[2x2]([11+1i, 21+1i, 12+2i, 22+2i])\ntensor[3]([1+2i, 3+4i, 5+6i])\ntensor[1x2]([1+2i, 3+4i])\ntensor[2x1]([1+2i, 5+6i])\ntensor[2x2]([1-2i, 3-4i, 5-6i, 7-8i])\ntensor[2x2]([1+2i, 5+6i, 3+4i, 7+8i])\n8+10i\n-9+22i\ntensor[2]([2+0i, 2+0i])\ntensor[2x2]([0.5-0.5i, 0+0i, 0+0i, 0.4+0.2i])\n[tensor[2x2]([1+0i, 0+0i, 0+0i, 0+0i]), tensor[2x2]([1+0i, 1i, 0+0i, 0+0i])]\ntensor[2x2]([10+14i, 14+20i, 26+30i, 38+44i])\n"; got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
}

func TestComplexTensorReductions(t *testing.T) {
	prog, err := Parse("<test>", `
let values = tensor([complex(1, 2), complex(3, 4)])
emit sum(values)
emit product(values)
emit dot(values, values)
emit norm(values)
emit outer(values, values)
emit solve(tensor([[complex(1, 1), complex(0, 0)], [complex(0, 0), complex(2, -1)]]), tensor([complex(2, 2), complex(4, -2)]))
emit inv(tensor([[complex(1, 1), complex(0, 0)], [complex(0, 0), complex(2, -1)]]))
emit qr(tensor([[complex(1, 0), complex(0, 1)], [complex(0, 0), complex(0, 0)]]))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	out, err := NewRuntime().Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	want := []string{"4+6i", "-5+10i", "-10+28i", "5.477225575051661", "tensor[2x2]([-3+4i, -5+10i, -5+10i, -7+24i])", "tensor[2]([2+0i, 2+0i])", "tensor[2x2]([0.5-0.5i, 0+0i, 0+0i, 0.4+0.2i])", "[tensor[2x2]([1+0i, 0+0i, 0+0i, 0+0i]), tensor[2x2]([1+0i, 1i, 0+0i, 0+0i])]"}
	if got := len(out); got != len(want) {
		t.Fatalf("output length = %d, want %d", got, len(want))
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %q, want %q", i, got, want[i])
		}
	}
}

func TestCBackendComplexCholeskyParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `emit cholesky(tensor([[complex(4, 0), complex(0, 2)], [complex(0, -2), complex(5, 0)]]))`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	interpreted, err := NewRuntime().Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter run failed: %v", err)
	}
	if got, want := interpreted[0].String(), "tensor[2x2]([2+0i, 0+0i, -1i, 2+0i])"; got != want {
		t.Fatalf("interpreter output = %q, want %q", got, want)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile C failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "complex-cholesky")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v", err)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got, want := string(out), "tensor[2x2]([2+0i, 0+0i, -1i, 2+0i])\n"; got != want {
		t.Fatalf("native output = %q, want %q", got, want)
	}
}

// TestCBackendComplexTensorResiduals checks numerical complex kernels by their
// mathematical residuals rather than by incidental factor/vector layouts.
func TestCBackendComplexTensorResiduals(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<complex-residuals>", `
let a = tensor([[complex(4, 0), complex(0, 2)], [complex(0, -2), complex(5, 0)]])
let rhs = tensor([complex(2, 2), complex(4, -2)])
let solution = solve(a, rhs)
let factor = cholesky(a)
let decomposition = qr(tensor([[complex(1, 0), complex(0, 1)], [complex(0, 1), complex(1, 0)]]))
emit norm(matmul(a, reshape(solution, [2, 1])) - reshape(rhs, [2, 1]))
emit norm(matmul(factor, transpose(conjugate(factor))) - a)
emit norm(matmul(at(decomposition, 0), at(decomposition, 1)) - tensor([[complex(1, 0), complex(0, 1)], [complex(0, 1), complex(1, 0)]]))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile C failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "complex-residuals")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	lines := strings.Fields(string(out))
	if got, want := len(lines), 3; got != want {
		t.Fatalf("residual output = %q, want %d values", out, want)
	}
	for i, text := range lines {
		residual, err := strconv.ParseFloat(text, 64)
		if err != nil {
			t.Fatalf("residual[%d] %q is not a float: %v", i, text, err)
		}
		if residual > 1e-10 {
			t.Fatalf("residual[%d] = %.17g, want <= 1e-10\n--- C ---\n%s", i, residual, artifact.Source)
		}
	}
}

func TestCBackendFunctionBranchParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `func floor_at_two(n) => if(n > 2, n, 2)
emit floor_at_two(1)
emit floor_at_two(5)`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	mainAt := strings.LastIndex(artifact.Source, "int main(void) {")
	if mainAt < 0 || strings.Contains(artifact.Source[mainAt:], "OValue s") || strings.Contains(artifact.Source[mainAt:], "osiris_fn_") {
		t.Fatalf("branching scalar function was not inlined into the unboxed main:\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "function-branch")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v", err)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got, want := string(out), "2u64\n5u64\n"; got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
}

func TestCBackendFunctionLoopParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `func count_to(n) {
  let i = 0
  while i < n { let i = i + 1 }
  return i
}

emit count_to(4)`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "function-loop")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v", err)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got, want := string(out), "4u64\n"; got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
}

func TestCBackendBitwiseParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit (13 & 7) | (2 << 3)\nemit 17 % 5")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "bitwise")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v", err)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got, want := string(out), "21u64\n2u64\n"; got != want {
		t.Fatalf("native output = %q, want %q", got, want)
	}
}

func TestCBackendZeroDivisionIsDefined(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit 1 / 0\nemit 1 % 0")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "zero-division")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v", err)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err == nil {
		t.Fatalf("running executable succeeded, want runtime failure\n%s", out)
	}
	if got := string(out); !strings.Contains(got, "osiris native error: division by zero") || strings.Contains(got, "remainder by zero") {
		t.Fatalf("native zero diagnostics = %q", got)
	}
}

func TestCBackendAbsBuiltinParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit abs(-7)\nemit abs(-2.5f64)")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "abs")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v", err)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got, want := string(out), "7i64\n2.5f64\n"; got != want {
		t.Fatalf("native output = %q, want %q", got, want)
	}
}

func TestCBackendScalarCastAndStringLenParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit uint(7.0f64)\nemit f32(1.25)\nemit f64(2)\nemit len(\"h\u00e9llo \U0001f680\")")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "scalar-casts-len")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendUnicodeStringAtParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
let greeting = "héllo 🚀"
emit at(greeting, 1)
emit at(greeting, 6)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "unicode-string-at")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendFixedWidthIntegerParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
emit 255u8
emit u8(260)
emit u16(65537)
emit u32(4294967297)
emit i8(255)
emit i16(65535)
emit i32(4294967295)
emit i64(7)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "fixed-width-integers")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendFixedWidthIntegerArithmeticParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
emit 250u8 + 10u8
emit 1u8 + 1u16
emit 2u8 * 130u8
emit 7u8 & 3u8
emit 1u8 << 2
emit 1i16 + 2i16
emit 2i8 * 70i8
emit 7i16 % 3i16
emit 8i16 / 2i16
emit -5i16
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "fixed-width-arithmetic")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendUnboxedNarrowUnsignedIntegerParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
emit 250u8 + 10u8
emit 65535u16 + 2u16
emit 4294967295u32 * 2u32
emit 1u8 << 7u8
emit 250u8 + 10u16
emit 0xffu8 | 0xff00u16
emit 1u8 << 2
emit 255u8 % 10u16
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("narrow unsigned program unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	for _, declaration := range []string{"uint8_t s", "uint16_t s", "uint32_t s"} {
		if !strings.Contains(artifact.Source, declaration) {
			t.Fatalf("generated C does not contain unboxed %s local\n--- C ---\n%s", declaration, artifact.Source)
		}
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-narrow-unsigned")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendUnboxedNarrowSignedIntegerParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
emit 120i8 + 10i8
emit 120i8 - 240i8
emit 30000i16 * 2i16
emit 5i32 - 15i32
emit -(120i8)
emit 0x7fi8 ^ 0x80i8
emit 0x7fi16 & 0xff00i16
emit 1i8 << 6i8
emit 0x80i8 >> 1i8
emit 0x80i8 % 3i8
emit 120i8 + 10i16
emit 0x7fi8 | 0x0100i16
emit 1i8 << 6i16
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	if strings.Contains(artifact.Source, "Osiris native fallback") {
		t.Fatalf("narrow signed program unexpectedly used tagged fallback\n--- C ---\n%s", artifact.Source)
	}
	for _, declaration := range []string{"int8_t s", "int16_t s", "int32_t s"} {
		if !strings.Contains(artifact.Source, declaration) {
			t.Fatalf("generated C does not contain unboxed %s local\n--- C ---\n%s", declaration, artifact.Source)
		}
	}
	exePath := filepath.Join(t.TempDir(), "unboxed-narrow-signed")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendStaticMathImportParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.abs(i8(-7))
emit math.u8(260)
emit math.f32(1.25)
emit math.pi
emit math.tau
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	if strings.Contains(artifact.Source, " = ocall(") {
		t.Fatalf("static math import lowered through an indirect call:\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "static-math-import")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendStaticMathCapturedModuleParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<captured-static-module>", `
import "std/math" as math
func root(value) => math.sqrt(value)
emit root(81.0)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile C failed: %v", err)
	}
	if strings.Contains(artifact.Source, " = ocall(") {
		t.Fatalf("captured static module lowered through a dynamic call:\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "captured-static-module")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got, want := string(out), "9\n"; got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
}

func TestCBackendRejectsConflictingModuleMerge(t *testing.T) {
	prog, err := Parse("<conflicting-static-module>", `
import "std/math" as math
import "std/crypto" as crypto
let selected = if(true, math, crypto)
emit selected.sqrt(81)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	_, err = CompileC(context.Background(), NewRuntime(), prog)
	if err == nil || !strings.Contains(err.Error(), "dynamic module member \"sqrt\" cannot be lowered") {
		t.Fatalf("CompileC error = %v, want a dynamic module member diagnostic", err)
	}
}

func TestCBackendReportsUnsupportedStaticMathMember(t *testing.T) {
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.pow(2, 3)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	_, err = CompileC(context.Background(), NewRuntime(), prog)
	if err == nil || !strings.Contains(err.Error(), "requires proven f32 scalar inputs") {
		t.Fatalf("CompileC error = %v, want parity-safe pow diagnostic", err)
	}
}

func TestCBackendStaticMathScalarParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
let m = math
	emit math.sqrt(4.0f64)
	emit m.log(100.0f32, 10.0f32)
emit math.log2(8.0f64)
emit math.log10(1000.0f64)
emit math.hypot(3.0f64, 4.0f64)
emit math.sin(0.0f64)
emit math.expm1(0.0f64)
emit math.log1p(0.0f64)
emit math.cbrt(8.0f64)
emit math.erf(0.0f64)
emit math.erfc(0.0f64)
emit math.gamma(1.0f32)
emit math.lgamma(1.0f32)
emit math.floor(2.9f32)
emit math.ceil(2.1f32)
emit math.round(2.5f64)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "static-math-scalar")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendStaticMathVectorParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.vec()
emit math.vector([1.0, 2.0, 3.0])
emit math.vec(4.0, 5.0)
let original = tensor([6.0, 7.0])
emit math.vector(original)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "static-math-vector")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendStaticMathFillTensorParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.zeros(3)
emit math.ones([2])
emit math.full(2, 3.5)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "static-math-fill-tensor")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendStaticMathMultidimensionalFillTensorParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
let zeroes = math.zeros(2, 3)
let filled = math.full([2, 3], 1.5)
emit zeroes
emit math.shape(zeroes)
emit filled + 2.0
emit math.shape(filled)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "static-math-multidimensional-fill-tensor")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendNumericalProviderDiagnostics(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "eigh",
			src:  "import \"std/math\" as math\nemit math.eigh(math.identity(2))",
			want: "eigh requires a configured native numerical provider",
		},
		{
			name: "svd",
			src:  "import \"std/math\" as math\nemit math.svd(math.identity(2))",
			want: "svd requires a configured native numerical provider",
		},
		{
			name: "eig",
			src:  "import \"std/math\" as math\nemit math.eig(math.identity(2))",
			want: "eig requires a configured ABI v2 native numerical provider",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prog, err := Parse("<test>", test.src)
			if err != nil {
				t.Fatal(err)
			}
			_, err = CompileC(context.Background(), NewRuntime(), prog)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("CompileC error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCBackendEighProviderParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.eigh(math.matrix(2, 2, [2.0, 0.0, 0.0, 3.0]))
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	providerPath := filepath.Join("testdata", "linalg_provider_diagonal.c")
	artifact, err := CompileCWithOptions(context.Background(), rt, prog, CCompileOptions{LinalgProviderSource: providerPath})
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "eigh-provider")
	if err := CompileCExecutableWithOptions(context.Background(), artifact.Source, exePath, CCompileOptions{LinalgProviderSource: providerPath}); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got, want := string(out), wantValues[0].String()+"\n"; got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
}

func TestCBackendProviderABIMismatchDiagnostic(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<provider-abi-mismatch>", `
import "std/math" as math
emit math.eigh(math.identity(2))
`)
	if err != nil {
		t.Fatal(err)
	}
	providerPath := filepath.Join("testdata", "linalg_provider_incompatible.c")
	artifact, err := CompileCWithOptions(context.Background(), NewRuntime(), prog, CCompileOptions{LinalgProviderSource: providerPath})
	if err != nil {
		t.Fatalf("compile C failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "provider-abi-mismatch")
	if err := CompileCExecutableWithOptions(context.Background(), artifact.Source, exePath, CCompileOptions{LinalgProviderSource: providerPath}); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err == nil {
		t.Fatalf("running executable succeeded, want provider failure\n%s", out)
	}
	if got, want := string(out), "osiris native error: eigh numerical provider failed\n"; got != want {
		t.Fatalf("provider ABI mismatch output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
}

func TestCBackendSVDProviderParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.svd(math.matrix(2, 2, [3.0, 0.0, 0.0, 2.0]))
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	providerPath := filepath.Join("testdata", "linalg_provider_diagonal.c")
	artifact, err := CompileCWithOptions(context.Background(), rt, prog, CCompileOptions{LinalgProviderSource: providerPath})
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "svd-provider")
	if err := CompileCExecutableWithOptions(context.Background(), artifact.Source, exePath, CCompileOptions{LinalgProviderSource: providerPath}); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got, want := string(out), wantValues[0].String()+"\n"; got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
}

func TestCBackendEigProviderParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.eig(math.matrix(2, 2, [3.0, 0.0, 0.0, 2.0]))
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	providerPath := filepath.Join("testdata", "linalg_provider_diagonal.c")
	artifact, err := CompileCWithOptions(context.Background(), rt, prog, CCompileOptions{LinalgProviderSource: providerPath})
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "eig-provider")
	if err := CompileCExecutableWithOptions(context.Background(), artifact.Source, exePath, CCompileOptions{LinalgProviderSource: providerPath}); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got, want := string(out), wantValues[0].String()+"\n"; got != want {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
}

func TestCBackendEigProviderComplexResidual(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
let result = math.eig(math.matrix(2, 2, [0.0, -1.0, 1.0, 0.0]))
let values = at(result, 0)
let vectors = at(result, 1)
let plus = at(values, 0)
let minus = at(values, 1)
let plus0 = at(vectors, [0, 0])
let plus1 = at(vectors, [1, 0])
let minus0 = at(vectors, [0, 1])
let minus1 = at(vectors, [1, 1])
emit magnitude(0 - plus1 - plus * plus0)
emit magnitude(plus0 - plus * plus1)
emit magnitude(0 - minus1 - minus * minus0)
emit magnitude(minus0 - minus * minus1)
`)
	if err != nil {
		t.Fatal(err)
	}
	providerPath := filepath.Join("testdata", "linalg_provider_diagonal.c")
	artifact, err := CompileCWithOptions(context.Background(), NewRuntime(), prog, CCompileOptions{LinalgProviderSource: providerPath})
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "eig-provider-complex-residual")
	if err := CompileCExecutableWithOptions(context.Background(), artifact.Source, exePath, CCompileOptions{LinalgProviderSource: providerPath}); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got, want := string(out), "0\n0\n0\n0\n"; got != want {
		t.Fatalf("complex eig residuals = %q, want %q\n--- C ---\n%s", got, want, artifact.Source)
	}
}

func TestCBackendAccelerateProviderResiduals(t *testing.T) {
	providerPath := filepath.Join("providers", "accelerate_lapack_provider.c")
	if _, err := os.Stat(providerPath); err != nil {
		t.Skipf("Apple Accelerate provider is unavailable: %v", err)
	}
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<accelerate-provider>", `
import "std/math" as math
let symmetric = math.matrix(2, 2, [2.0, 1.0, 1.0, 2.0])
let eigen = math.eigh(symmetric)
let eigenvalues = at(eigen, 0)
let eigenvectors = at(eigen, 1)
let diagonal = math.matrix(2, 2, [at(eigenvalues, 0), 0.0, 0.0, at(eigenvalues, 1)])
let source = math.matrix(2, 2, [3.0, 1.0, 0.0, 2.0])
let singular = math.svd(source)
let u = at(singular, 0)
let values = at(singular, 1)
let vt = at(singular, 2)
let singular_diagonal = math.matrix(2, 2, [at(values, 0), 0.0, 0.0, at(values, 1)])
let rotation = math.matrix(2, 2, [0.0, -1.0, 1.0, 0.0])
let general = math.eig(rotation)
let general_values = at(general, 0)
let general_vectors = at(general, 1)
let plus = at(general_values, 0)
let minus = at(general_values, 1)
let plus0 = at(general_vectors, [0, 0])
let plus1 = at(general_vectors, [1, 0])
let minus0 = at(general_vectors, [0, 1])
let minus1 = at(general_vectors, [1, 1])
emit norm(matmul(matmul(eigenvectors, diagonal), transpose(eigenvectors)) - symmetric)
emit norm(matmul(transpose(eigenvectors), eigenvectors) - math.identity(2))
emit norm(matmul(matmul(u, singular_diagonal), vt) - source)
emit at(values, 0) >= at(values, 1)
emit magnitude(0 - plus1 - plus * plus0)
emit magnitude(plus0 - plus * plus1)
emit magnitude(0 - minus1 - minus * minus0)
emit magnitude(minus0 - minus * minus1)
`)
	if err != nil {
		t.Fatal(err)
	}
	options := CCompileOptions{LinalgProviderSource: providerPath, LinalgProviderFlags: []string{"-framework", "Accelerate"}}
	artifact, err := CompileCWithOptions(context.Background(), NewRuntime(), prog, options)
	if err != nil {
		t.Fatalf("compile C failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "accelerate-provider")
	if err := CompileCExecutableWithOptions(context.Background(), artifact.Source, exePath, options); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	lines := strings.Fields(string(out))
	if got, want := len(lines), 8; got != want {
		t.Fatalf("provider output = %q, want %d values", out, want)
	}
	if lines[3] != "true" {
		t.Fatalf("singular values are not non-increasing: %q", out)
	}
	for i, text := range append(lines[:3], lines[4:]...) {
		residual, err := strconv.ParseFloat(text, 64)
		if err != nil || residual > 1e-10 {
			t.Fatalf("residual[%d] = %q, want finite value <= 1e-10 (err=%v)\n--- C ---\n%s", i, text, err, artifact.Source)
		}
	}
}

func TestCBackendNestedTensorAndMatrixParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
let nested = math.tensor([[1.0, 2.0], [3.0, 4.0]])
let shaped = math.tensor([2, 2], [5.0, 6.0, 7.0, 8.0])
let matrix = math.matrix(2, 2, [9.0, 10.0, 11.0, 12.0])
let reshaped = math.reshape(matrix, [4, 1])
let transposed = math.transpose(math.matrix(2, 3, [1.0, 2.0, 3.0, 4.0, 5.0, 6.0]))
let product = math.matmul(math.matrix(2, 3, [1.0, 2.0, 3.0, 4.0, 5.0, 6.0]), math.matrix(3, 2, [7.0, 8.0, 9.0, 10.0, 11.0, 12.0]))
let broadcast = math.zeros(2, 1) + math.ones(1, 3)
let changed = setat(matrix, [1, 0], 42.0)
let rowChanged = setat(matrix, [1], math.tensor([42.0, 43.0]))
let row = at(matrix, [1])
let prefix = take(matrix, 3)
let window = slice(math.matrix(3, 2, [1.0, 2.0, 3.0, 4.0, 5.0, 6.0]), 1, 8)
let columns = slice(math.matrix(2, 3, [1.0, 2.0, 3.0, 4.0, 5.0, 6.0]), 1, 3, 1)
let identity = math.eye(2, 3)
let square = math.matrix(3, 3, [2.0, 1.0, 3.0, 0.0, -1.0, 4.0, 5.0, 2.0, 0.0])
let system = math.matrix(2, 2, [4.0, 7.0, 2.0, 6.0])
let positive = math.matrix(2, 2, [4.0, 2.0, 2.0, 3.0])
let qrInput = math.matrix(3, 2, [1.0, 0.0, 0.0, 1.0, 1.0, 1.0])
let outer = math.outer(math.matrix(1, 2, [2.0, 3.0]), math.vector(4.0, 5.0))
emit nested
emit math.shape(nested)
emit shaped + nested
emit math.shape(matrix)
emit matrix
emit at(matrix, [1, 0])
emit row
emit math.shape(row)
emit prefix
emit math.shape(prefix)
emit window
emit math.shape(window)
emit columns
emit math.shape(columns)
emit math.trace(square)
emit math.det(square)
emit math.solve(system, math.vector(1.0, 0.0))
emit math.solve(system, math.identity(2))
emit math.inv(system)
emit math.cholesky(positive)
emit math.qr(qrInput)
emit outer
emit identity
emit math.shape(identity)
emit changed
emit rowChanged
emit matrix
emit reshaped
emit math.shape(reshaped)
emit transposed
emit math.shape(transposed)
emit product
emit math.shape(product)
emit broadcast
emit math.shape(broadcast)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "nested-tensor-and-matrix")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendStaticMathRangeTensorParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.arange(4)
emit math.arange(1, 5, 1.5)
emit math.arange(5, 1, -1.5)
emit math.linspace(2, 6, 3)
emit math.linspace(4, 8)
emit math.logspace(0, 2, 3)
emit math.logspace(0, 2, 3, 2)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "static-math-range-tensor")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendStaticMathExtremaParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
import "std/math" as math
emit math.min(3, -2, 4.5)
emit math.max(3, -2, 4.5)
emit math.clamp(-2, 0, 10)
emit math.clamp(12, 0, 10)
emit math.clamp(4, 0, 10)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "static-math-extrema")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendStringConcatenationParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "let greeting = \"h\u00e9\" + \"llo \U0001f680\"\nemit greeting\nemit greeting == \"h\u00e9llo \U0001f680\"\nemit len(greeting)")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "string-concat")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendListLiteralParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "let values = [1, 2, 3]\nemit values\nemit len(values)\nemit values == [1, 2, 3]\nemit values == [1, 2]")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "list-literal")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendListIndexAndForParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
let values = [2, 3, 5]
emit at(values, 1)
let total = 0
for value in values { let total = total + value }
emit total
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "list-for")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendContiguousTensorKernelParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
let left = tensor([1.0, 2.0, 3.0])
let right = tensor([4.0, 5.0, 6.0])
let sum = left + right
emit sum
emit at(sum, 2)
emit len(sum)
emit sum == tensor([5.0, 7.0, 9.0])
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	if !strings.Contains(artifact.Source, "otensorbroadcastshape") || !strings.Contains(artifact.Source, "for(size_t output=0;output<n;output++)") {
		t.Fatalf("generated C did not contain the direct broadcast tensor-add kernel:\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "tensor-kernel")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendTensorShapeMismatchIsReported(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit tensor([1.0, 2.0]) + tensor([1.0, 2.0, 3.0])")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "tensor-shape-mismatch")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err == nil {
		t.Fatalf("running executable succeeded, want shape mismatch\n%s", out)
	}
	if !strings.Contains(string(out), "osiris native error: tensor shape mismatch") {
		t.Fatalf("native shape-mismatch output = %q\n--- C ---\n%s", out, artifact.Source)
	}
}

func TestCBackendTensorScalarKernelParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
let values = tensor([1.0, 2.0, 4.0])
emit values + 2
emit 10 - values
emit values * 3
emit 16 / values
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	if !strings.Contains(artifact.Source, "otensorscalar") {
		t.Fatalf("generated C did not contain the tensor-scalar kernel:\n%s", artifact.Source)
	}
	exePath := filepath.Join(t.TempDir(), "tensor-scalar-kernel")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendTensorShapeAndFlattenParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
let values = tensor([1.0, 2.5, 4.0])
emit shape(values)
emit flatten(values)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "tensor-shape-flatten")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendTensorReductionsParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
emit sum(tensor([1.5, 2.0, 3.5]))
emit product(tensor([1.5, 2.0, 3.5]))
emit sum(tensor([]))
emit product(tensor([]))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "tensor-reductions")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendTensorDotParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
emit dot(tensor([1.5, 2.0, 3.5]), tensor([2.0, 4.0, 1.0]))
emit dot(tensor([]), tensor([]))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "tensor-dot")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestCBackendTensorNormParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", `
emit norm(tensor([3.0, 4.0]))
emit norm(tensor([]))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	wantValues, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	exePath := filepath.Join(t.TempDir(), "tensor-norm")
	if err := CompileCExecutable(context.Background(), artifact.Source, exePath); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(exePath).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

// TestBytecodeVMParity exercises the behavior described by its body and locks
// in the observable language, runtime, or CLI semantics that the surrounding
// package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestBytecodeVMParity(t *testing.T) {
	prog, err := Parse("<test>", `
import "std/text" as text
let seed = "osiris"
emit text.slug(seed + " engine")
emit band(u8(7), 3)
emit match [1, 2] { [a, b] => a + b; _ => 0 }
emit decode(encode([1u8, 2u16, "go"]))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter run failed: %v", err)
	}
	got, err := rt.RunVM(context.Background(), prog)
	if err != nil {
		t.Fatalf("vm run failed: %v", err)
	}
	if gotS, wantS := len(got), len(want); gotS != wantS {
		t.Fatalf("len(got) = %d, want %d", gotS, wantS)
	}
	for i := range want {
		if got[i].String() != want[i].String() {
			t.Fatalf("output[%d] = %s, want %s", i, got[i].String(), want[i].String())
		}
	}
}

// TestDeterministicSchedulerOrder exercises the behavior described by its body
// and locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestDeterministicSchedulerOrder(t *testing.T) {
	prog, err := Parse("<test>", `
emit [tap("a"), tap("b"), tap("c")]
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	var mu sync.Mutex
	var calls []string
	rt.Builtins["tap"] = func(args []Value) (Value, error) {
		label, err := args[0].AsString()
		if err != nil {
			return nullValue(), err
		}
		switch label {
		case "a":
			time.Sleep(20 * time.Millisecond)
		case "b":
			time.Sleep(5 * time.Millisecond)
		}
		mu.Lock()
		calls = append(calls, label)
		mu.Unlock()
		return stringValue(label), nil
	}
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 1; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), `["a", "b", "c"]`; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}
	if got, want := len(calls), 3; got != want {
		t.Fatalf("call count = %d, want %d", got, want)
	}
	seen := map[string]bool{}
	for _, call := range calls {
		seen[call] = true
	}
	for _, label := range []string{"a", "b", "c"} {
		if !seen[label] {
			t.Fatalf("missing call %q in %v", label, calls)
		}
	}
}

// TestModuleRegistryCacheFallback exercises the behavior described by its body
// and locks in the observable language, runtime, or CLI semantics that the
// surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestModuleRegistryCacheFallback(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "atom.osr")
	root := filepath.Join(dir, "main.osr")
	if err := os.WriteFile(child, []byte(`package atom
export value
let value = 13
`), 0o644); err != nil {
		t.Fatalf("write child: %v", err)
	}
	prog, err := Parse(root, `import "./atom.osr" as atom
emit atom.value
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("initial run failed: %v", err)
	}
	if got, want := len(out), 1; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), "13u64"; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}
	lockPath := filepath.Join(dir, ".osiris.lock")
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lockfile missing: %v", err)
	}
	cacheDir := filepath.Join(dir, ".osiris", "cache", "modules")
	if entries, err := os.ReadDir(cacheDir); err != nil {
		t.Fatalf("cache dir missing: %v", err)
	} else if len(entries) == 0 {
		t.Fatalf("cache dir empty")
	}
	if err := os.Remove(child); err != nil {
		t.Fatalf("remove child: %v", err)
	}
	rt2 := NewRuntime()
	out, err = rt2.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("cached run failed: %v", err)
	}
	if got, want := len(out), 1; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), "13u64"; got != want {
		t.Fatalf("cached output = %s, want %s", got, want)
	}
}
