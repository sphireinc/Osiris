package osiris

import (
	"context"
	"math"
	"strings"
	"testing"
)

// TestNativeCoreMathClassificationAndAngleParity covers the scalar floating
// point classification, angle conversion, sign-copy, and nextafter slice
// through the interpreter, VM, generated C, and native executable. The
// f32 cases exercise the result-width boundary and the matching-width rule
// used by the direct scalar emitter.
func TestNativeCoreMathClassificationAndAngleParity(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		source string
	}{
		{name: "degrees-radians", source: `import "std/math" as math
emit degrees(180.0)
emit math.radians(180.0)
emit degrees(1.0f32)
emit math.radians(1.0f32)
`},
		{name: "copysign", source: `import "std/math" as math
emit copysign(3.0, -0.0)
emit math.copysign(-3.0, 0.0)
`},
		{name: "nextafter", source: `import "std/math" as math
emit nextafter(1.0, 2.0)
emit math.nextafter(1.0f32, 2.0f32)
`},
		{name: "classification", source: `import "std/math" as math
emit isfinite(1.0)
emit math.isfinite(1.0f32)
emit isinf(1.0)
emit math.isinf(1.0f32)
emit isnan(1.0)
emit math.isnan(1.0f32)
emit isnormal(1.0)
emit math.isnormal(1.0f32)
emit signbit(-0.0)
emit math.signbit(-0.0f32)
`},
		{name: "classification-boundaries", source: `import "std/math" as math
let overflow = 1e308 * 1e308
emit isfinite(overflow)
emit isinf(overflow)
emit isnan(overflow - overflow)
emit isnormal(1e-45f32)
emit math.isnormal(1e-45f32)
`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			runDifferentialFixture(t, differentialFixture{
				Name:   "nativeCore-native-math-" + fixture.name,
				Source: fixture.source,
			})
		})
	}
}

// TestNativeCoreMathClassificationLoweringContract verifies that the new
// operations remain borrowed scalar calls. They must not fall through the
// tagged module dispatcher, evaluator, or host capability boundary.
func TestNativeCoreMathClassificationLoweringContract(t *testing.T) {
	program, err := Parse("<native-math-classification-contract>", `import "std/math" as math
emit degrees(1.0)
emit math.radians(1.0)
emit copysign(3.0, -0.0)
emit math.nextafter(1.0f32, 2.0f32)
emit isfinite(1.0)
emit math.isinf(1.0f32)
emit isnan(1.0)
emit math.isnormal(1.0f32)
emit signbit(-0.0)
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	mainSource := artifact.Source
	if start := strings.Index(mainSource, "int main(void)"); start >= 0 {
		mainSource = mainSource[start:]
	}
	for _, marker := range []string{
		"onative_degrees(",
		"onative_radians(",
		"onative_copysign(",
		"onative_nextafterf(",
		"onative_isfinite(",
		"onative_isinf(",
		"onative_isnan(",
		"onative_isnormalf(",
		"onative_signbit(",
	} {
		if !strings.Contains(mainSource, marker) {
			t.Fatalf("generated scalar math main omitted direct helper call %q", marker)
		}
	}
	for _, forbidden := range []string{`ostd_dispatch("std/math"`, "ocall(", "#define OSIRIS_CAP_", "malloc(", "retain("} {
		if strings.Contains(mainSource, forbidden) {
			t.Fatalf("scalar math main crossed an indirect, allocating, or host boundary %q", forbidden)
		}
	}
}

func TestNativeCoreMathClassificationRuntimeBoundaries(t *testing.T) {
	cases := []struct {
		name string
		call func([]Value) (Value, error)
		arg  Value
		want bool
	}{
		{name: "finite rejects infinity", call: isFiniteValue, arg: numberValue(math.Inf(1)), want: false},
		{name: "inf recognizes infinity", call: isInfValue, arg: numberValue(math.Inf(-1)), want: true},
		{name: "nan recognizes nan", call: isNaNValue, arg: numberValue(math.NaN()), want: true},
		{name: "normal rejects f64 subnormal", call: isNormalValue, arg: numberValue(math.SmallestNonzeroFloat64), want: false},
		{name: "normal rejects f32 subnormal", call: isNormalValue, arg: floatValue(float64(math.SmallestNonzeroFloat32), 32), want: false},
		{name: "signbit recognizes negative zero", call: signBitValue, arg: numberValue(math.Copysign(0, -1)), want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.call([]Value{tc.arg})
			if err != nil {
				t.Fatal(err)
			}
			if got.kind != kindBool || got.boo != tc.want {
				t.Fatalf("got %#v, want bool(%v)", got, tc.want)
			}
		})
	}
}

func TestNativeCoreMathClassificationErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
	}{
		{name: "degrees", source: "emit degrees(1.0, 2.0)\n"},
		{name: "radians", source: "emit radians(1.0, 2.0)\n"},
		{name: "copysign", source: "emit copysign(1.0)\n"},
		{name: "nextafter", source: "emit nextafter(1.0)\n"},
		{name: "isfinite", source: "emit isfinite(1.0, 2.0)\n"},
		{name: "isnormal", source: "emit isnormal(\"not-a-number\")\n"},
	} {
		program, err := Parse("native-math-classification-error.osr", tc.source)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := CompileC(context.Background(), NewRuntime(), program); err == nil || !strings.Contains(err.Error(), tc.name) {
			t.Fatalf("native C error for %q = %v, want a diagnostic naming the builtin", strings.TrimSpace(tc.source), err)
		}
	}
}
