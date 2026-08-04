package osiris

import (
	"context"
	"strings"
	"testing"
)

func TestReservedTypesExplicitConstructorsAndConversions(t *testing.T) {
	program, err := Parse("<reserved-types-contract>", `
let p = poly<f64>([1, 2, 0])
let q = quat<f64>([1, 2, 3, 4])
let mask = simd<4, bool>([true, false, true, false])
let left = simd<4, f64>([1, 2, 3, 4])
let right = simd<4, f64>([5, 6, 7, 8])
let sparse = sparse_matrix<f64>([2, 2], [[0, 0, 2], [1, 1, 3]], "csr")
emit p
emit poly_degree(p)
emit quat_inverse(q)
emit simd_select(mask, left, right)
emit nnz(sparse)
emit to_dense(sparse)
emit to_sparse(to_dense(sparse))
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
		if !interpreter[i].Equal(vm[i]) {
			t.Fatalf("output %d differs: interpreter=%s, VM=%s", i, interpreter[i], vm[i])
		}
	}
	if got := interpreter[0].Kind(); got != "poly<f64>" {
		t.Fatalf("explicit polynomial type = %s", got)
	}
	if got := interpreter[3].Kind(); got != "simd<4, f64>" {
		t.Fatalf("selected SIMD type = %s", got)
	}
}

func TestReservedTypeCodecUsesFamilyTags(t *testing.T) {
	sparse, err := futureSparseValue("sparse_matrix", []Value{
		listValue([]Value{uintValue(1, 64), uintValue(1, 64)}),
		listValue([]Value{listValue([]Value{uintValue(0, 64), uintValue(0, 64), numberValue(1)})}),
	})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		want  byte
		value Value
	}{
		{name: "poly", want: 0x22, value: mustFutureTestValue(t, futurePolyValue, []Value{listValue([]Value{numberValue(1)})})},
		{name: "quat", want: 0x23, value: mustFutureTestValue(t, futureQuatValue, []Value{listValue([]Value{numberValue(1), numberValue(0), numberValue(0), numberValue(0)})})},
		{name: "simd", want: 0x24, value: mustFutureTestValue(t, futureSIMDValue, []Value{listValue([]Value{numberValue(1), numberValue(2)})})},
		{name: "sparse_matrix", want: 0x25, value: sparse},
	}
	for _, tc := range cases {
		encoded, err := encodeValueBytes(tc.value)
		if err != nil {
			t.Fatalf("%s encode: %v", tc.name, err)
		}
		if len(encoded) < 5 || encoded[4] != tc.want {
			t.Fatalf("%s tag = %#x, want %#x", tc.name, encoded[4], tc.want)
		}
		decoded, err := decodeValueBytes(encoded)
		if err != nil || !decoded.Equal(tc.value) {
			t.Fatalf("%s codec round trip: value=%s decoded=%s err=%v", tc.name, tc.value, decoded, err)
		}
	}
}

func TestReservedTypeDiagnostics(t *testing.T) {
	program, err := Parse("<reserved-type-diagnostics>", "emit simd<3, number>([1, 2, 3])\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRuntime().CheckProgram(context.Background(), program); err == nil || !strings.Contains(err.Error(), "power of two") {
		t.Fatalf("invalid SIMD type diagnostic = %v", err)
	}
}

func mustFutureTestValue(t *testing.T, fn func([]Value) (Value, error), args []Value) Value {
	t.Helper()
	value, err := fn(args)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
