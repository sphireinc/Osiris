package osiris

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
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
  [err.kind, err.code, err.message, err.source]
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
	if got := out[0].String(); !strings.Contains(got, `"not_found"`) || !strings.Contains(got, `/definitely/not-found-osiris.txt`) || !strings.Contains(got, `"<test>"`) {
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
	if len(out) != 0 {
		t.Fatalf("len(out) = %d, want 0 after timeout failure", len(out))
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
}

func TestNativeLoweringRejectsEnumsExplicitly(t *testing.T) {
	for _, source := range []string{
		"enum Status { Ready, Done }\nemit Status.Ready\n",
		"enum Result { Ok(value), Err(message) }\nemit match Result.Ok(1) { Result.Ok(value) => value, Result.Err(message) => 0 }\n",
	} {
		prog, err := Parse("<enum>", source)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := CompileC(context.Background(), NewRuntime(), prog); err == nil || !strings.Contains(err.Error(), "native lowering does not yet support enum") {
			t.Fatalf("native error = %v, want explicit enum lowering rejection", err)
		}
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
	const want = "53f213f6070bf8445fd8945e896d41af39dfeab9816f0ec40e2f39a5ce23d819"
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
	if err == nil || !strings.Contains(err.Error(), `unsupported IR operation "global"`) {
		t.Fatalf("CompileC error = %v, want unsupported global IR diagnostic", err)
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

func TestCBackendUnboxedFloatWidthParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("<test>", "emit 1.25f32 + 2.5f32\nemit 1.25f64 + 2.5f64")
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
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); !strings.Contains(got, "osiris native error: division by zero") || !strings.Contains(got, "osiris native error: remainder by zero") {
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
import "std/text" as text
let selected = if(true, math, text)
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
	if err == nil || !strings.Contains(err.Error(), `builtin "pow" is not supported yet`) {
		t.Fatalf("CompileC error = %v, want unsupported static builtin diagnostic", err)
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
	emit m.log(100.0f64, 10.0f64)
emit math.log2(8.0f64)
emit math.log10(1000.0f64)
emit math.hypot(3.0f64, 4.0f64)
emit math.sin(0.0f64)
emit math.asin(0.0f64)
emit math.acos(1.0f64)
emit math.atan(0.0f64)
emit math.sinh(0.0f64)
emit math.cosh(0.0f64)
emit math.tanh(0.0f64)
emit math.asinh(0.0f64)
emit math.acosh(1.0f64)
emit math.atanh(0.0f64)
emit math.expm1(0.0f64)
emit math.log1p(0.0f64)
emit math.cbrt(8.0f64)
emit math.erf(0.0f64)
emit math.erfc(0.0f64)
emit math.gamma(1.0f64)
emit math.lgamma(1.0f64)
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
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got, want := string(out), "osiris native error: eigh numerical provider failed\nnull\n"; got != want {
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
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
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
