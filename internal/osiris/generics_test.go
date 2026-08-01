package osiris

import (
	"context"
	"strings"
	"testing"
)

func TestGenericFunctionsAndTypedCollections(t *testing.T) {
	prog, err := Parse("<generics>", `
func identity<T>(value T) T => value
func numeric<T: number>(value T) T => value
func apply<T>(fn func(T) T, value T) T => fn(value)
func recursive<T>(value T, again bool) T => if(again, recursive(value, false), value)
let values list<u64> = [1, 2, 3]
emit identity(7)
emit identity<string>("typed")
emit numeric(9)
emit apply(numeric, 4)
emit recursive("recursive", true)
emit values
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	checked, err := rt.CheckProgram(context.Background(), prog)
	if err != nil {
		t.Fatalf("type-check failed: %v", err)
	}
	if len(checked.Functions) != 4 || checked.Functions[0].TypeParams[0] != "T" {
		t.Fatalf("generic metadata = %#v", checked.Functions)
	}
	for _, run := range []struct {
		name string
		fn   func(context.Context, *Program) ([]Value, error)
	}{
		{name: "interpreter", fn: rt.Run},
		{name: "vm", fn: rt.RunVM},
	} {
		t.Run(run.name, func(t *testing.T) {
			out, runErr := run.fn(context.Background(), prog)
			if runErr != nil {
				t.Fatal(runErr)
			}
			want := []string{"7u64", `"typed"`, "9u64", "4u64", `"recursive"`, "[1u64, 2u64, 3u64]"}
			if len(out) != len(want) {
				t.Fatalf("outputs = %#v, want %d values", out, len(want))
			}
			for i := range want {
				if got := out[i].String(); got != want[i] {
					t.Errorf("output[%d] = %s, want %s", i, got, want[i])
				}
			}
		})
	}
}

func TestGenericInferenceDiagnostics(t *testing.T) {
	tests := []string{
		"func identity<T>(value T) T => value\nemit identity()",
		"func numeric<T: number>(value T) T => value\nemit numeric(\"not numeric\")",
		"func identity<T>(value T) T => value\nemit identity<u64, string>(1)",
	}
	for _, source := range tests {
		prog, err := Parse("<generic-error>", source)
		if err != nil {
			t.Fatalf("parse failed for %q: %v", source, err)
		}
		_, err = NewRuntime().CheckProgram(context.Background(), prog)
		if err == nil || !strings.Contains(err.Error(), "generic") && !strings.Contains(err.Error(), "type argument") && !strings.Contains(err.Error(), "expects") {
			t.Errorf("source %q error = %v", source, err)
		}
	}
}

func TestTypedMapAndSetOrderingAndCodec(t *testing.T) {
	prog, err := Parse("<collections>", `
let values map<string, u64> = map_new("first", 1, "second", 2)
let values = map_set(values, "first", 3)
let numbers set<u64> = set_new(2, 1, 2)
let numbers = set_add(numbers, 3)
emit values
emit map_keys(values)
emit map_values(values)
emit numbers
emit set_values(numbers)
emit decode(encode(values))
emit decode(encode(numbers))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	for _, run := range []struct {
		name string
		fn   func(context.Context, *Program) ([]Value, error)
	}{
		{name: "interpreter", fn: rt.Run},
		{name: "vm", fn: rt.RunVM},
	} {
		t.Run(run.name, func(t *testing.T) {
			out, runErr := run.fn(context.Background(), prog)
			if runErr != nil {
				t.Fatal(runErr)
			}
			want := []string{
				`map{"first": 3u64, "second": 2u64}`,
				`["first", "second"]`,
				`[3u64, 2u64]`,
				"set{2u64, 1u64, 3u64}",
				"[2u64, 1u64, 3u64]",
				`map{"first": 3u64, "second": 2u64}`,
				"set{2u64, 1u64, 3u64}",
			}
			if len(out) != len(want) {
				t.Fatalf("outputs = %#v, want %d values", out, len(want))
			}
			for i := range want {
				if got := out[i].String(); got != want[i] {
					t.Errorf("output[%d] = %s, want %s", i, got, want[i])
				}
			}
		})
	}
}

func TestGenericNativeLoweringUsesSharedTaggedContract(t *testing.T) {
	prog, err := Parse("<generic-native>", `
func identity<T>(value T) T => value
emit identity<u64>(7)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("native compile failed: %v", err)
	}
	if !strings.Contains(artifact.Source, "Osiris native C lowering") || !strings.Contains(artifact.Source, "osiris_fn_") {
		t.Fatalf("native artifact does not contain generic call lowering")
	}
}
