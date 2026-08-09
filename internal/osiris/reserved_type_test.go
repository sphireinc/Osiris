package osiris

import (
	"context"
	"strings"
	"testing"
)

func TestReservedTypeSyntaxAndInference(t *testing.T) {
	source := `
	let p poly<number> = poly([1])
	let q quat<f64> = quat([0, 0, 0, 0])
	let v simd<4, number> = simd(4, [0, 0, 0, 0])
	let m sparse_matrix<number> = sparse_matrix([1, 1], [])
	let s sparse_tensor<number> = sparse_tensor([1], [])
`
	program, err := Parse("<reservedType-types>", source)
	if err != nil {
		t.Fatal(err)
	}
	formatted := FormatProgram(program)
	reparsed, err := Parse("<reservedType-types-formatted>", formatted)
	if err != nil {
		t.Fatalf("formatted source did not parse: %v\n%s", err, formatted)
	}
	if FormatProgram(reparsed) != formatted {
		t.Fatalf("type syntax is not formatter-stable:\n%s", formatted)
	}
	if _, err := NewRuntime().CheckProgram(context.Background(), program); err != nil {
		t.Fatalf("static checker rejected reserved type syntax: %v", err)
	}
	inferred, err := NewRuntime().InferProgram(context.Background(), program)
	if err != nil {
		t.Fatalf("inference rejected reserved type syntax: %v", err)
	}
	for _, typeName := range []string{"poly", "quat", "simd", "sparse_matrix", "sparse_tensor"} {
		if !strings.Contains(inferred.String(), typeName) {
			t.Fatalf("inference missing %q:\n%s", typeName, inferred)
		}
	}
	for _, invalid := range []string{
		"let x simd<3, number> = 0\n",
		"let x quat<number, number> = 0\n",
		"let x sparse_matrix<string> = 0\n",
	} {
		invalidProgram, err := Parse("<reservedType-invalid>", invalid)
		if err != nil {
			continue
		}
		if _, err := NewRuntime().CheckProgram(context.Background(), invalidProgram); err == nil {
			t.Fatalf("invalid reserved type accepted: %q", invalid)
		}
	}
}

func TestReservedTypesInterpreterVMCodecParity(t *testing.T) {
	program, err := Parse("<reservedType-runtime>", `
import "std/math" as math
let p poly<number> = math.poly([1, 2, 3])
let q quat<number> = math.quat([1, 2, 3, 4])
let v simd<4, number> = math.simd(4, [1, 2, 3, 4])
let m sparse_matrix<number> = math.sparse_matrix([2, 2], [[0, 1, 2], [0, 1, 3], [1, 0, 4]], "csr")
let s sparse_tensor<number> = math.sparse_tensor([2, 2, 2], [[0, 1, 1, 7]])
emit p * p
emit math.poly_degree(p)
emit math.poly_eval(p, 2)
emit q * q
emit at(v, 2)
emit at(m, [0, 1])
emit setat(m, [1, 1], 9)
emit shape(s)
emit len(m)
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	interpreter, err := rt.Run(context.Background(), program)
	if err != nil {
		t.Fatalf("interpreter: %v", err)
	}
	vm, err := rt.RunVM(context.Background(), program)
	if err != nil {
		t.Fatalf("vm: %v", err)
	}
	if len(interpreter) != len(vm) {
		t.Fatalf("interpreter outputs=%d, VM outputs=%d", len(interpreter), len(vm))
	}
	for i := range interpreter {
		if !interpreter[i].Equal(vm[i]) || interpreter[i].Kind() != vm[i].Kind() {
			t.Fatalf("output %d differs: interpreter=%s (%s), vm=%s (%s)", i, interpreter[i], interpreter[i].Kind(), vm[i], vm[i].Kind())
		}
		encoded, err := encodeValueBytes(interpreter[i])
		if err != nil {
			t.Fatalf("codec encode output %d: %v", i, err)
		}
		decoded, err := decodeValueBytes(encoded)
		if err != nil {
			t.Fatalf("codec decode output %d: %v", i, err)
		}
		if !decoded.Equal(interpreter[i]) {
			t.Fatalf("codec changed output %d: %s -> %s", i, interpreter[i], decoded)
		}
	}
	if got := interpreter[0].String(); !strings.Contains(got, "poly<number>") {
		t.Fatalf("polynomial product lost type metadata: %s", got)
	}
	encoded, err := encodeValueBytes(interpreter[6])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeValueBytes(encoded[:len(encoded)-1]); err == nil {
		t.Fatal("truncated future codec value was accepted")
	}
}

func TestReservedTypeSparseDuplicateAndBoundsDiagnostics(t *testing.T) {
	if _, err := futureSparseValue("sparse_matrix", []Value{
		listValue([]Value{uintValue(2, 64), uintValue(2, 64)}),
		listValue([]Value{listValue([]Value{uintValue(2, 64), uintValue(0, 64), numberValue(1)})}),
	}); err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("sparse bounds diagnostic = %v", err)
	}
	value, err := futureSparseValue("sparse_matrix", []Value{
		listValue([]Value{uintValue(2, 64), uintValue(2, 64)}),
		listValue([]Value{
			listValue([]Value{uintValue(0, 64), uintValue(0, 64), numberValue(1)}),
			listValue([]Value{uintValue(0, 64), uintValue(0, 64), numberValue(-1)}),
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := value.String(); !strings.HasSuffix(got, "()") {
		t.Fatalf("zero duplicate was not removed: %s", got)
	}
}

func TestReservedTypeArtifactBackendsRejectReservedValues(t *testing.T) {
	program, err := Parse("<reservedType-backends>", `
import "std/math" as math
emit math.poly([1, 2])
`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileC(context.Background(), NewRuntime(), program); err == nil || !strings.Contains(err.Error(), "native C backend: unsupported reserved type \"poly\"") {
		t.Fatalf("native diagnostic = %v", err)
	}
	if _, err := CompileWASM(context.Background(), NewRuntime(), program); err == nil || !strings.Contains(err.Error(), "WASM backend: unsupported reserved type \"poly\"") {
		t.Fatalf("WASM diagnostic = %v", err)
	}
	if _, err := CompileGPU(context.Background(), program); err == nil || !strings.Contains(err.Error(), "GPU backend: unsupported reserved type \"poly\"") {
		t.Fatalf("GPU diagnostic = %v", err)
	}
}
