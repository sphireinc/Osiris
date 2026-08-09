package osiris

import (
	"context"
	"errors"
	"math"
	"math/big"
	"strings"
	"testing"
)

func runMathProgram(t *testing.T, vm bool, source string) ([]Value, error) {
	t.Helper()
	prog, err := Parse("<math-test>", source)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if vm {
		return NewRuntime().RunVM(context.Background(), prog)
	}
	return NewRuntime().Run(context.Background(), prog)
}

func TestExpandedMathInterpreterVMParity(t *testing.T) {
	const source = `
import "std/math" as math
emit atan2(0.0, -1.0), math.atan2(0.0, -1.0)
emit exp2(3.0), math.exp2(3.0)
emit trunc(-3.75), math.trunc(-3.75)
emit fmod(5.5, 2.0), math.fmod(5.5, 2.0)
emit remainder(5.5, 2.0), math.remainder(5.5, 2.0)
emit fma(2.0, 3.0, 4.0), math.fma(2.0, 3.0, 4.0)
emit positive_difference(3.0, 5.0), math.positive_difference(3.0, 5.0)
emit copysign(3.0, -0.0), math.copysign(3.0, -0.0)
emit nextafter(1.0, 2.0), math.nextafter(1.0, 2.0)
emit frexp(8.0), math.frexp(8.0)
emit ldexp(0.5, 4), math.ldexp(0.5, 4)
emit modf(-3.25), math.modf(-3.25)
emit isfinite(1.0), math.isfinite(1.0)
emit isinf(1.0), math.isinf(1.0)
emit isnan(1.0), math.isnan(1.0)
emit isnormal(1.0), math.isnormal(1.0)
emit signbit(-0.0), math.signbit(-0.0)
emit degrees(Pi), math.degrees(Pi)
emit radians(180.0), math.radians(180.0)
emit sinpi(0.5), math.sinpi(0.5)
emit cospi(1.0), math.cospi(1.0)
emit tanpi(0.25), math.tanpi(0.25)
emit divmod(17, 5), math.divmod(17, 5)
emit gcd(48, 18), math.gcd(48, 18)
emit lcm(48, 18), math.lcm(48, 18)
emit count_ones(240u8), math.count_ones(240u8)
emit count_zeros(240u8), math.count_zeros(240u8)
emit leading_zeros(15u8), math.leading_zeros(15u8)
emit trailing_zeros(240u8), math.trailing_zeros(240u8)
emit bit_width(240u8), math.bit_width(240u8)
emit bit_floor(129u8), math.bit_floor(129u8)
emit bit_ceil(65u8), math.bit_ceil(65u8)
emit has_single_bit(128u8), math.has_single_bit(128u8)
emit byte_swap(0x1234u16), math.byte_swap(0x1234u16)
emit phase(complex(0.0, 1.0)), math.phase(complex(0.0, 1.0))
`
	var outputs []string
	for _, vm := range []bool{false, true} {
		out, err := runMathProgram(t, vm, source)
		if err != nil {
			t.Fatalf("vm=%v: %v", vm, err)
		}
		outputs = append(outputs, valuesOutput(out))
	}
	if outputs[0] != outputs[1] {
		t.Fatalf("interpreter/VM mismatch:\ninterpreter:\n%s\nVM:\n%s", outputs[0], outputs[1])
	}
	for _, want := range []string{
		"3.141592653589793, 3.141592653589793",
		"8, 8",
		"-3, -3",
		"1.5, 1.5",
		"-0.5, -0.5",
		"10, 10",
		"0, 0",
		"-3, -3",
		"1.0000000000000002, 1.0000000000000002",
		"0.5 4i64, 0.5 4i64",
		"8, 8",
		"-3 -0.25, -3 -0.25",
		"true, true",
		"false, false",
		"false, false",
		"true, true",
		"true, true",
		"180, 180",
		"3.141592653589793, 3.141592653589793",
		"1, 1",
		"-1, -1",
		"1, 1",
		"3u64 2u64, 3u64 2u64",
		"6u64, 6u64",
		"144u64, 144u64",
		"4u64, 4u64",
		"4u64, 4u64",
		"4u64, 4u64",
		"4u64, 4u64",
		"8u64, 8u64",
		"128u8, 128u8",
		"128u8, 128u8",
		"true, true",
		"13330u16, 13330u16",
		"1.5707963267948966, 1.5707963267948966",
	} {
		if !strings.Contains(outputs[0], `"`+want+`"`+"\n") {
			t.Fatalf("output missing %q:\n%s", want, outputs[0])
		}
	}
}

func TestExpandedMathComplexFamiliesAndBranchCuts(t *testing.T) {
	const source = `
import "std/math" as math
emit exp(complex(0.0, 0.0)), math.exp(complex(0.0, 0.0))
emit log(complex(1.0, 0.0)), math.log(complex(1.0, 0.0))
emit math.sqrt(complex(-1.0, 0.0)), math.sqrt(complex(-1.0, 0.0))
emit pow(complex(0.0, 1.0), 2.0), math.pow(complex(0.0, 1.0), 2.0)
emit sin(complex(0.0, 0.0)), math.sin(complex(0.0, 0.0))
emit cos(complex(0.0, 0.0)), math.cos(complex(0.0, 0.0))
emit tan(complex(0.0, 0.0)), math.tan(complex(0.0, 0.0))
emit sinh(complex(0.0, 0.0)), math.sinh(complex(0.0, 0.0))
emit cosh(complex(0.0, 0.0)), math.cosh(complex(0.0, 0.0))
emit tanh(complex(0.0, 0.0)), math.tanh(complex(0.0, 0.0))
emit math.sqrt(complex(-4.0, -0.0))
emit phase(complex(-1.0, 0.0)), phase(complex(-1.0, -0.0))
`
	var outputs []string
	for _, vm := range []bool{false, true} {
		out, err := runMathProgram(t, vm, source)
		if err != nil {
			t.Fatalf("vm=%v: %v", vm, err)
		}
		outputs = append(outputs, valuesOutput(out))
	}
	if outputs[0] != outputs[1] {
		t.Fatalf("interpreter/VM mismatch:\ninterpreter:\n%s\nVM:\n%s", outputs[0], outputs[1])
	}
	wants := []string{
		"1+0i, 1+0i",
		"0+0i, 0+0i",
		"1i, 1i",
		"-1+1.2246467991473515e-16i, -1+1.2246467991473515e-16i",
		"0+0i, 0+0i",
		"1+0i, 1+0i",
		"0+0i, 0+0i",
		"0+0i, 0+0i",
		"1+0i, 1+0i",
		"0+0i, 0+0i",
		"-2i",
		"3.141592653589793, -3.141592653589793",
	}
	for index, want := range wants {
		if index < 10 {
			want = `"` + want + `"`
		}
		if index == 11 {
			want = `"` + want + `"`
		}
		if !strings.Contains(outputs[0], want+"\n") {
			t.Fatalf("output missing %q:\n%s", want, outputs[0])
		}
	}
}

func TestExpandedMathBoundarySemantics(t *testing.T) {
	classification := []struct {
		name string
		call func([]Value) (Value, error)
		arg  Value
		want bool
	}{
		{name: "finite", call: isFiniteValue, arg: numberValue(1), want: true},
		{name: "infinity", call: isInfValue, arg: numberValue(math.Inf(1)), want: true},
		{name: "nan", call: isNaNValue, arg: numberValue(math.NaN()), want: true},
		{name: "subnormal", call: isNormalValue, arg: numberValue(math.SmallestNonzeroFloat64), want: false},
		{name: "complex infinity", call: isInfValue, arg: complexValue(complex(math.Inf(1), 0)), want: true},
	}
	for _, test := range classification {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.call([]Value{test.arg})
			if err != nil {
				t.Fatal(err)
			}
			if got.kind != kindBool || got.boo != test.want {
				t.Fatalf("got %s, want %v", got.String(), test.want)
			}
		})
	}

	t.Run("floating boundaries", func(t *testing.T) {
		if _, err := exp2Value([]Value{floatValue(128, 32)}); err == nil {
			t.Fatal("f32 exp2 overflow unexpectedly succeeded")
		} else {
			var record *OsirisError
			if !errors.As(err, &record) || record.kind != ErrorKindOutOfRange {
				t.Fatalf("f32 exp2 overflow = %T %v, want structured out_of_range", err, err)
			}
		}
		underflow, err := exp2Value([]Value{numberValue(-1075)})
		if err != nil || underflow.num != 0 || math.Signbit(underflow.num) {
			t.Fatalf("exp2 underflow = %s, %v; want +0", underflow.String(), err)
		}
		next, err := nextAfterValue([]Value{floatValue(1, 32), floatValue(2, 32)})
		if err != nil || next.fbits != 32 || next.num != float64(math.Nextafter32(1, 2)) {
			t.Fatalf("nextafter f32 = %#v, %v", next, err)
		}
		parts, err := modfValue([]Value{numberValue(math.Copysign(0, -1))})
		if err != nil || len(parts.list) != 2 || !math.Signbit(parts.list[0].num) || !math.Signbit(parts.list[1].num) {
			t.Fatalf("modf(-0) = %s, %v; want two negative zeros", parts.String(), err)
		}
		difference, err := positiveDifferenceValue([]Value{numberValue(-0), numberValue(0)})
		if err != nil || difference.num != 0 || math.Signbit(difference.num) {
			t.Fatalf("positive_difference(-0, +0) = %s, %v; want +0", difference.String(), err)
		}
		fused, err := fmaValue([]Value{numberValue(math.Inf(1)), numberValue(0), numberValue(1)})
		if err != nil || !math.IsNaN(fused.num) {
			t.Fatalf("fma(+Inf, 0, 1) = %s, %v; want NaN", fused.String(), err)
		}
	})

	t.Run("exact integer boundaries", func(t *testing.T) {
		negative, err := divmodValue([]Value{intValue(big.NewInt(-17), 8), intValue(big.NewInt(5), 8)})
		if err != nil || negative.String() != "[-3i8, -2i8]" {
			t.Fatalf("divmod(-17i8, 5i8) = %s, %v", negative.String(), err)
		}
		widened, err := lcmValue([]Value{uintValue(200, 8), uintValue(3, 8)})
		if err != nil || widened.kind != kindBigInt || widened.String() != "600n" {
			t.Fatalf("lcm(200u8, 3u8) = %s/%s, %v; want exact 600n", widened.Kind(), widened.String(), err)
		}
	})

	for _, test := range []struct {
		name       string
		expression string
		kind       ErrorKind
		message    string
	}{
		{name: "fmod zero", expression: "fmod(1.0, 0.0)", kind: ErrorKindInvalidArgument, message: "fmod divisor must be non-zero"},
		{name: "remainder zero", expression: "remainder(1.0, 0.0)", kind: ErrorKindInvalidArgument, message: "remainder divisor must be non-zero"},
		{name: "tanpi pole", expression: "tanpi(0.5)", kind: ErrorKindInvalidArgument, message: "tanpi is undefined at half-integers"},
		{name: "division zero", expression: "divmod(1, 0)", kind: ErrorKindInvalidArgument, message: "divmod divisor must be non-zero"},
		{name: "bit input kind", expression: "count_ones(1i8)", kind: ErrorKindType, message: "count_ones expects a fixed-width unsigned integer"},
		{name: "bit ceiling overflow", expression: "bit_ceil(129u8)", kind: ErrorKindOutOfRange, message: "bit_ceil result is not representable in u8"},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, vm := range []bool{false, true} {
				_, err := runMathProgram(t, vm, "emit "+test.expression)
				if err == nil {
					t.Fatalf("vm=%v: expected error", vm)
				}
				var record *OsirisError
				if !errors.As(err, &record) {
					t.Fatalf("vm=%v: error is not structured: %T %v", vm, err, err)
				}
				if record.kind != test.kind || !strings.Contains(record.message, test.message) {
					t.Fatalf("vm=%v: error = %s/%q, want %s containing %q", vm, record.kind, record.message, test.kind, test.message)
				}
			}
		})
	}
}

func TestExpandedMathBitWidthAndPackedStyle(t *testing.T) {
	const source = `
emit bit_floor(129uf16)
emit bit_ceil(129uf16)
emit byte_swap(0x1234uf16)
emit bit_floor(u128(bigint("340282366920938463463374607431768211455")))
emit byte_swap(0x0102030405060708u64)
emit leading_zeros(0u8), trailing_zeros(0u8), count_zeros(0u8), bit_width(0u8)
`
	out, err := runMathProgram(t, false, source)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"128uf16",
		"256uf16",
		"13330uf16",
		"170141183460469231731687303715884105728u128",
		"578437695752307201u64",
		`"8u64, 8u64, 8u64, 0u64"`,
	}
	if got := strings.Split(strings.TrimSpace(valuesOutput(out)), "\n"); len(got) != len(want) {
		t.Fatalf("got %d outputs, want %d:\n%s", len(got), len(want), valuesOutput(out))
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("output[%d] = %s, want %s", i, got[i], want[i])
			}
		}
	}
}

func TestExpandedMathInferenceAndNativeDiagnostics(t *testing.T) {
	prog, err := Parse("<math-infer>", `
import "std/math" as math
let angle = atan2(1.0f32, 2.0f32)
let parts = frexp(8.0)
let split = divmod(17, 5)
let bits = bit_floor(129u16)
let class = isfinite(angle)
let z = math.sqrt(complex(-1.0, 0.0))
`)
	if err != nil {
		t.Fatal(err)
	}
	inferred, err := NewRuntime().InferProgram(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	got := FormatInferProgram(inferred)
	for _, want := range []string{
		"main.angle: f32",
		"main.parts: list",
		"main.split: list",
		"main.bits: uint16",
		"main.class: bool",
		"main.z: complex",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("infer output missing %q\n--- got ---\n%s", want, got)
		}
	}

	for _, source := range []string{
		"emit sin(complex(1.0, 2.0))",
		"func apply(z) => sin(z)\nemit apply(complex(1.0, 2.0))",
	} {
		prog, err := Parse("<math-native>", source)
		if err != nil {
			t.Fatal(err)
		}
		_, err = CompileC(context.Background(), NewRuntime(), prog)
		if err == nil || !strings.Contains(err.Error(), "native C backend") || !strings.Contains(err.Error(), "not supported") {
			t.Fatalf("CompileC(%q) error = %v, want explicit unsupported diagnostic", source, err)
		}
	}
}
