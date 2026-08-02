package osiris

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestManifestPinnedNativeFFIScalarCalls(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("fixture build is maintained on macOS and Linux")
	}
	compiler, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("C compiler is unavailable")
	}
	root := t.TempDir()
	fixture := filepath.Join(root, "libosiris_ffi_fixture")
	args := []string{"-shared", "-fPIC", "-o", fixture, filepath.Join("testdata", "ffi_fixture.c")}
	if runtime.GOOS == "darwin" {
		fixture += ".dylib"
		args = []string{"-dynamiclib", "-o", fixture, filepath.Join("testdata", "ffi_fixture.c")}
	}
	command := exec.Command(compiler, args...)
	command.Dir, _ = os.Getwd()
	if output, err := command.CombinedOutput(); err != nil {
		t.Skipf("fixture compiler unavailable: %v (%s)", err, output)
	}
	libraryBytes, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(libraryBytes)
	manifestPath := filepath.Join(root, "ffi.json")
	manifest := FFIManifest{
		ABIVersion: 1, Library: fixture, Version: "1.0.0", Platform: runtime.GOOS,
		Architecture: runtime.GOARCH, SHA256: hex.EncodeToString(digest[:]),
		Capabilities: []string{"ffi.test"}, SearchPolicy: "absolute",
		Functions: []FFIFunction{
			{Name: "add_i64", Symbol: "osiris_ffi_add_i64", Args: []string{"i64", "i64"}, Result: "i64", Ownership: "borrowed", CallingConvention: "c", ThreadAffinity: "any"},
			{Name: "mul_f64", Symbol: "osiris_ffi_mul_f64", Args: []string{"f64", "f64"}, Result: "f64", Ownership: "borrowed", CallingConvention: "c", ThreadAffinity: "any"},
			{Name: "join_strings", Symbol: "osiris_ffi_join_strings", Args: []string{"string", "string"}, Result: "string", Ownership: "borrowed", CallingConvention: "c", ThreadAffinity: "any"},
			{Name: "sum_buffer", Symbol: "osiris_ffi_sum_buffer", Args: []string{"const_buffer"}, Result: "i64", Ownership: "borrowed", CallingConvention: "c", ThreadAffinity: "any"},
			{Name: "increment_buffer", Symbol: "osiris_ffi_increment_buffer", Args: []string{"buffer"}, Result: "i64", Ownership: "borrowed", CallingConvention: "c", ThreadAffinity: "any"},
			{Name: "sum_point", Symbol: "osiris_ffi_sum_point", Args: []string{"struct:Point"}, Result: "i64", Ownership: "borrowed", CallingConvention: "c", ThreadAffinity: "any"},
			{Name: "create_handle", Symbol: "osiris_ffi_create_handle", Args: []string{}, Result: "handle:fixture", Ownership: "owned", CallingConvention: "c", ThreadAffinity: "any"},
			{Name: "invoke_callback", Symbol: "osiris_ffi_invoke_callback", Args: []string{"callback:Unary"}, Result: "i64", Ownership: "borrowed", CallingConvention: "c", ThreadAffinity: "any"},
		},
		Constants: []FFIConstant{{Name: "answer", Type: "u32", Value: "42"}},
		Structs:   []FFIStruct{{Name: "Point", Size: 8, Alignment: 4, ByteOrder: "little", Fields: []FFIField{{Name: "x", Type: "i32", Offset: 0}, {Name: "y", Type: "i32", Offset: 4}}}},
		Handles:   []FFIHandleType{{Name: "fixture", ReleaseSymbol: "osiris_ffi_release_handle"}},
		Callbacks: []FFICallback{{Name: "Unary", Args: []string{"i64"}, Result: "i64", CallingConvention: "c"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	library, err := LoadFFILibrary(manifestPath, map[string]bool{"ffi.test": true})
	if err != nil {
		t.Fatal(err)
	}
	defer library.Close()
	integer, err := library.Call("add_i64", []Value{intValue(bigIntForTest(20), 64), intValue(bigIntForTest(22), 64)})
	if err != nil || integer.String() != "42i64" {
		t.Fatalf("integer call = %s, err = %v", integer.String(), err)
	}
	product, err := library.Call("mul_f64", []Value{numberValue(1.5), numberValue(2.0)})
	if err != nil || math.Abs(product.num-3.0) > 1e-12 {
		t.Fatalf("float call = %s, err = %v", product.String(), err)
	}
	joined, err := library.Call("join_strings", []Value{stringValue("osiris"), stringValue(" ffi")})
	if err != nil || joined.String() != `"osiris ffi"` {
		t.Fatalf("string call = %s, err = %v", joined.String(), err)
	}
	sum, err := library.Call("sum_buffer", []Value{listValue([]Value{uintValue(1, 8), uintValue(2, 8), uintValue(3, 8)})})
	if err != nil || sum.String() != "6i64" {
		t.Fatalf("const_buffer call = %s, err = %v", sum.String(), err)
	}
	mutable := listValue([]Value{uintValue(1, 8), uintValue(2, 8), uintValue(3, 8)})
	mutated, err := library.Call("increment_buffer", []Value{mutable})
	if err != nil || mutated.String() != "3i64" {
		t.Fatalf("mutable buffer call = %s, err = %v", mutated.String(), err)
	}
	if got := mutable.String(); got != "[2u8, 3u8, 4u8]" {
		t.Fatalf("mutable buffer = %s", got)
	}
	pointType := &structTypeValue{name: "Point", fields: []structFieldValue{{name: "x", typeName: "i32"}, {name: "y", typeName: "i32"}}, byName: map[string]int{"x": 0, "y": 1}}
	point := structInstanceValueOf(pointType, []Value{intValue(bigIntForTest(20), 32), intValue(bigIntForTest(22), 32)})
	sumPoint, err := library.Call("sum_point", []Value{point})
	if err != nil || sumPoint.String() != "42i64" {
		t.Fatalf("struct call = %s, err = %v", sumPoint.String(), err)
	}
	constant, err := library.Constant("answer")
	if err != nil || constant.String() != "42u32" {
		t.Fatalf("constant = %s, err = %v", constant.String(), err)
	}
	handle, err := library.Call("create_handle", nil)
	if err != nil || handle.Kind() != "handle:fixture" {
		t.Fatalf("handle = %s, err = %v", handle.String(), err)
	}
	if closed, err := closeBuiltinValue([]Value{handle}); err != nil || !closed.Truthy() {
		t.Fatalf("handle close = %s, err = %v", closed.String(), err)
	}
	liveHandle, err := library.Call("create_handle", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := library.Close(); err == nil {
		t.Fatal("library close succeeded with an active opaque handle")
	}
	if _, err := closeBuiltinValue([]Value{liveHandle}); err != nil {
		t.Fatal(err)
	}
	callback := functionValueOf("callback", func(args []Value) (Value, error) {
		value, err := args[0].Int()
		if err != nil {
			return nullValue(), err
		}
		return intValue(bigIntForTest(int64(value+1)), 64), nil
	})
	callbackResult, err := library.Call("invoke_callback", []Value{callback})
	if err != nil || callbackResult.String() != "42i64" {
		t.Fatalf("callback = %s, err = %v", callbackResult.String(), err)
	}

	nativeRuntime := NewRuntime()
	nativeRuntime.GrantCapability("ffi.test")
	source := fmt.Sprintf("ffi %q as lib\nemit ffi_call(lib, %q, 20i64, 22i64)\n", manifestPath, "add_i64")
	program, err := Parse("ffi-native.osr", source)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), nativeRuntime, program)
	if err != nil {
		t.Fatalf("native FFI compile = %v", err)
	}
	executable := filepath.Join(root, "ffi-native")
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatalf("native FFI executable = %v", err)
	}
	output, err := exec.Command(executable).CombinedOutput()
	if err != nil {
		t.Fatalf("native FFI execution = %v (%s)", err, output)
	}
	if got := strings.TrimSpace(string(output)); got != "42i64" {
		t.Fatalf("native FFI output = %q", got)
	}
	for _, test := range []struct {
		name string
		src  string
		want string
	}{
		{name: "string", src: fmt.Sprintf("ffi %q as lib\nemit ffi_call(lib, %q, %q, %q)\n", manifestPath, "join_strings", "osiris", " ffi"), want: `"osiris ffi"`},
		{name: "constant", src: fmt.Sprintf("ffi %q as lib\nemit ffi_const(lib, %q)\n", manifestPath, "answer"), want: "42u32"},
		{name: "close", src: fmt.Sprintf("ffi %q as lib\nemit ffi_close(lib)\n", manifestPath), want: "true"},
		{name: "const buffer", src: fmt.Sprintf("ffi %q as lib\nemit ffi_call(lib, %q, [1u8, 2u8, 3u8])\n", manifestPath, "sum_buffer"), want: "6i64"},
		{name: "mutable buffer", src: fmt.Sprintf("ffi %q as lib\nlet data = [1u8, 2u8, 3u8]\nemit ffi_call(lib, %q, data)\nemit data\n", manifestPath, "increment_buffer"), want: "3i64\n[2u8, 3u8, 4u8]"},
	} {
		t.Run(test.name, func(t *testing.T) {
			program, err := Parse("ffi-native.osr", test.src)
			if err != nil {
				t.Fatal(err)
			}
			artifact, err := CompileC(context.Background(), nativeRuntime, program)
			if err != nil {
				t.Fatalf("native FFI compile = %v", err)
			}
			executable := filepath.Join(root, "ffi-native-"+strings.ReplaceAll(test.name, " ", "-"))
			if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
				t.Fatalf("native FFI executable = %v", err)
			}
			output, err := exec.Command(executable).CombinedOutput()
			if err != nil {
				t.Fatalf("native FFI execution = %v (%s)", err, output)
			}
			if got := strings.TrimSpace(string(output)); got != test.want {
				t.Fatalf("native FFI output = %q, want %q", got, test.want)
			}
		})
	}
}

func bigIntForTest(value int64) *big.Int { return big.NewInt(value) }
