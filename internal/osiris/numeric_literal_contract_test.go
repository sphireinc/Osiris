package osiris

import (
	"context"
	"math"
	"strings"
	"testing"
)

func TestNumericLiteralContractAcrossParserInferenceAndCodec(t *testing.T) {
	cases := []struct {
		literal string
		kind    string
		text    string
	}{
		{literal: "42", kind: "uint64", text: "42u64"},
		{literal: "0xffu8", kind: "uint8", text: "255u8"},
		{literal: "-7i16", kind: "int16", text: "-7i16"},
		{literal: "1.25", kind: "number", text: "1.25"},
		{literal: "1.25f32", kind: "f32", text: "1.25f32"},
		{literal: "1.25bf16", kind: "bf16", text: "1.25bf16"},
		{literal: "1.25d", kind: "decimal", text: "1.25d"},
		{literal: "2i", kind: "complex", text: "2i"},
	}
	for _, item := range cases {
		t.Run(item.literal, func(t *testing.T) {
			prog, err := Parse("<literal>", "let value = "+item.literal+"\nemit value\n")
			if err != nil {
				t.Fatal(err)
			}
			out, err := NewRuntime().Run(context.Background(), prog)
			if err != nil {
				t.Fatal(err)
			}
			if got := out[0].String(); got != item.text {
				t.Fatalf("value = %q, want %q", got, item.text)
			}
			inferProg, err := NewRuntime().InferProgram(context.Background(), prog)
			if err != nil {
				t.Fatal(err)
			}
			if got := FormatInferProgram(inferProg); !strings.Contains(got, "main.value: "+item.kind) {
				t.Fatalf("inference missing kind %q:\n%s", item.kind, got)
			}
			encoded, err := encodeValueBytes(out[0])
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := decodeValueBytes(encoded)
			if err != nil {
				t.Fatal(err)
			}
			if !decoded.Equal(out[0]) || decoded.Kind() != out[0].Kind() || decoded.String() != out[0].String() {
				t.Fatalf("codec round trip = %s (%s), want %s (%s)", decoded.String(), decoded.Kind(), out[0].String(), out[0].Kind())
			}
		})
	}
}

func TestInvalidNumericSuffixHasParserLocation(t *testing.T) {
	_, err := Parse("<literal>", "emit 1u7\n")
	if err == nil || !strings.Contains(err.Error(), "<literal>:1:6") || !strings.Contains(err.Error(), "invalid numeric literal") {
		t.Fatalf("parse error = %v", err)
	}
}

func TestComplexWidthContractAcrossRuntimeVMInferenceAndCodec(t *testing.T) {
	prog, err := Parse("<complex-width>", `
let narrow = 1.25f32i
let wide = 1.25f64i
let constructed = complex(f32(1.25), f32(2.5))
let promoted = constructed + complex(f64(0.5), f64(0))
emit narrow
emit wide
emit constructed
emit promoted
emit real(constructed)
emit imag(constructed)
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	interpreter, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := rt.RunVM(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	for i := range interpreter {
		if !interpreter[i].Equal(vm[i]) || interpreter[i].Kind() != vm[i].Kind() {
			t.Fatalf("interpreter/VM output %d differs: %s (%s) vs %s (%s)", i, interpreter[i].String(), interpreter[i].Kind(), vm[i].String(), vm[i].Kind())
		}
	}
	for index, want := range []string{"complex<f32>", "complex<f64>", "complex<f32>", "complex<f64>", "f32", "f32"} {
		if got := interpreter[index].Kind(); got != want {
			t.Fatalf("output %d kind = %s, want %s", index, got, want)
		}
		encoded, err := encodeValueBytes(interpreter[index])
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeValueBytes(encoded)
		if err != nil {
			t.Fatal(err)
		}
		if !decoded.Equal(interpreter[index]) || decoded.Kind() != interpreter[index].Kind() {
			t.Fatalf("codec output %d = %s (%s), want %s (%s)", index, decoded.String(), decoded.Kind(), interpreter[index].String(), interpreter[index].Kind())
		}
	}
	inferProg, err := rt.InferProgram(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	gotInfer := FormatInferProgram(inferProg)
	for _, want := range []string{
		"main.narrow: complex<f32>",
		"main.wide: complex<f64>",
		"main.constructed: complex<f32>",
		"main.promoted: complex<f64>",
	} {
		if !strings.Contains(gotInfer, want) {
			t.Fatalf("inference missing %q:\n%s", want, gotInfer)
		}
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("native complex width compile = %v", err)
	}
	if !strings.Contains(artifact.Source, "ocomplexbits(") {
		t.Fatal("native complex width artifact does not use the width-preserving constructor")
	}
}

func TestDualScalarContractAcrossRuntimeVMInferenceAndCodec(t *testing.T) {
	const source = `
let x = dual(3, 1)
let y = dual(2, 0.5)
let sum = x + y
let product = x * y
let quotient = x / y
let negated = -x
let mixed_left = x + 2
let mixed_right = 2 + x
let mixed_quotient = x / 2
emit sum
emit product
emit quotient
emit negated
emit mixed_left
emit mixed_right
emit mixed_quotient
emit primal(product)
emit tangent(product)

`
	prog, err := Parse("<dual>", source)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	interpreter, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := rt.RunVM(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"dual(5, 1.5)", "dual(6, 3.5)", "dual(1.5, 0.125)",
		"dual(-3, -1)", "dual(5, 1)", "dual(5, 1)", "dual(1.5, 0.5)",
		"6", "3.5",
	}
	for i := range want {
		if got := interpreter[i].String(); got != want[i] {
			t.Fatalf("interpreter output %d = %s, want %s", i, got, want[i])
		}
		if !interpreter[i].Equal(vm[i]) || interpreter[i].Kind() != vm[i].Kind() {
			t.Fatalf("interpreter/VM output %d differs", i)
		}
		encoded, err := encodeValueBytes(interpreter[i])
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeValueBytes(encoded)
		if err != nil {
			t.Fatal(err)
		}
		if !decoded.Equal(interpreter[i]) || decoded.Kind() != interpreter[i].Kind() {
			t.Fatalf("codec output %d changed", i)
		}
	}
	inferred, err := rt.InferProgram(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if got := FormatInferProgram(inferred); !strings.Contains(got, "main.product: dual") {
		t.Fatalf("inference missing dual binding:\n%s", got)
	}
	runDifferentialFixture(t, differentialFixture{Name: "dual-scalar-contract", Source: source})
}

func TestSeededRandomIsReproducibleAcrossRuntimeAndVM(t *testing.T) {
	prog, err := Parse("<random>", `
let first = random("osiris", 0)
let second = random("osiris", 1)
let repeat = random("osiris", 0)
emit first
emit second
emit repeat
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	interpreter, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := rt.RunVM(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if !interpreter[0].Equal(interpreter[2]) || interpreter[0].Equal(interpreter[1]) {
		t.Fatalf("seed/counter sequence is not deterministic")
	}
	for i := range interpreter {
		if !interpreter[i].Equal(vm[i]) || interpreter[i].num < 0 || interpreter[i].num >= 1 {
			t.Fatalf("random output %d = %s", i, interpreter[i].String())
		}
	}
}

func TestSignalModuleReferenceKernelsAcrossRuntimeAndVM(t *testing.T) {
	prog, err := Parse("<signal>", `
import "std/signal" as signal
let window = signal.hann(4)
let filtered = signal.convolve([1, 2, 3], [1, 1])
emit window
emit filtered
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	interpreter, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := rt.RunVM(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]float64{{0, 0.75, 0.75, 0}, {1, 3, 5, 3}}
	for i := range want {
		if len(interpreter[i].data) != len(want[i]) {
			t.Fatalf("output %d has wrong length", i)
		}
		for j, expected := range want[i] {
			actual, err := interpreter[i].data[j].AsNumber()
			if err != nil || math.Abs(actual-expected) > 1e-12 {
				t.Fatalf("output %d[%d] = %v, want %v", i, j, actual, expected)
			}
		}
		if !interpreter[i].Equal(vm[i]) {
			t.Fatalf("interpreter/VM output %d differs", i)
		}
	}
}
