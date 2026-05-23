package osiris

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestParseAndPrecedence exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestBooleanLiteralsAndLogic exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestEmitInsideLoopBody exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestDecimalLiteralsAndArithmetic exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestFloatWidthsAndRoundTrip exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestBFloatWidthsAndRoundTrip exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestArrayConstructorsRangeGeneratorsAndStats exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestShapeManipulationAndLinearAlgebraSurface exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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
		"[tensor[2]([3, 2]), tensor[2x2]([1, 0, 0, 1])]",
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

// TestExactIntegerDivisionAndExponentiation exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestSignedIntegerLiteralsAndArithmetic exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestComplexLiteralsAndArithmetic exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestUnsignedIntegerLiteralsAndConstructor exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestWideUnsignedIntegers exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestPackedUnsignedFamily exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestByteLiteralsAndConstructor exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestNativeBitwiseOperators exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestStdMathLibrary exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestUnitTypesAndDimensionalAnalysis exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestUnitTypeMismatchFails exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestBigNumberFallbacks exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestRationalConstructorExactness exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestIntervalArithmeticAndEncoding exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestModIntArithmeticAndEncoding exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestFixedPointArithmeticAndEncoding exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestBigIntExactnessAndFallback exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestTensorSIMDStyleVectorizationAndViews exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestGPUCompilerArtifact exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestConcurrentTaskScheduler exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestStringPrimitiveIsolation exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestLanguageSyntaxLoopsAndBlocks exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestTryCatchErrorHandling exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestEmitConcatenatesCommaSeparatedValues exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestTensorAndMatrixType exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestTensorBroadcastSemantics exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestTensorShapeValidationAtCompileTime exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestLexicalShadowing exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestStructuralMemoization exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestStdlibImportAndMemberCall exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestSessionPersistsState exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestBlockExpression exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestHygienicMacroExpansion exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestFunctionDeclarationsAndRecursion exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestLoopsAndFlowControl exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestFormatSource exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestFormatMacrosAndBlocks exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestModuleExportsAreExplicit exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestPackageVersioning exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestBitwiseAndByteHelpers exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestBinaryCodecRoundTrip exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestMatchExpression exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestSwitchExpression exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestVizProgramDOT exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestVizImportsDOT exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestDataPipelineBuiltins exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestWASMBackendSnapshot exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestCBackendSnapshot exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
func TestCBackendSnapshot(t *testing.T) {
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
	if !strings.Contains(artifact.Source, "Osiris C snapshot") || !strings.Contains(artifact.Source, "osiris_outputs") {
		t.Fatalf("unexpected c source:\n%s", artifact.Source)
	}
	if !strings.Contains(artifact.Source, `puts(osiris_outputs[i]);`) {
		t.Fatalf("c source missing puts loop:\n%s", artifact.Source)
	}
}

// TestCBackendObjectAndExecutable exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestBytecodeVMParity exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestDeterministicSchedulerOrder exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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

// TestModuleRegistryCacheFallback exercises the behavior described by its body and locks in the observable language, runtime, or CLI semantics that the surrounding package depends on.
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
