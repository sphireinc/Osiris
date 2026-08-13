package osiris

import (
	"context"
	"strings"
	"testing"
)

func TestLazyStaticTypeAndForcePreserveElementType(t *testing.T) {
	prog, err := Parse("<lazy-types>", `
func relay<T>(value Lazy<T>) Lazy<T> => value
let delayed Lazy<u32> = lazy { u32(7) }
let copied Lazy<u32> = relay(delayed)
let value u32 = force(copied)
emit value
`)
	if err != nil {
		t.Fatal(err)
	}
	checked, err := NewRuntime().CheckProgram(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if len(checked.Bindings) != 3 {
		t.Fatalf("bindings = %#v, want three", checked.Bindings)
	}
	for _, binding := range checked.Bindings[:2] {
		if binding.Type != "Lazy<u32>" {
			t.Fatalf("lazy binding %s type = %q, want Lazy<u32>", binding.Name, binding.Type)
		}
	}
	if got := checked.Bindings[2].Type; got != "u32" {
		t.Fatalf("forced binding type = %q, want u32", got)
	}
	if got := checked.Functions[0].Signature; got != "func(Lazy<T>) Lazy<T>" {
		t.Fatalf("generic lazy signature = %q", got)
	}
}

func TestLazyStaticTypeRejectsImplicitForceAndMalformedLazyType(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "lazy is not its element",
			source: "let value u32 = lazy { u32(7) }\n",
			want:   "binding type u32 requires u32, got Lazy<u32>",
		},
		{
			name:   "element is not lazy",
			source: "let value Lazy<u32> = u32(7)\n",
			want:   "binding type Lazy<u32> requires Lazy<u32>, got u32",
		},
		{
			name:   "force requires lazy",
			source: "emit force(u32(7))\n",
			want:   "force expects Lazy<T>, got u32",
		},
		{
			name:   "force arity",
			source: "emit force()\n",
			want:   "force expects exactly one Lazy<T> argument",
		},
		{
			name:   "lazy arity",
			source: "let value Lazy<u32, u64> = lazy { u32(7) }\n",
			want:   "Lazy requires exactly one type argument",
		},
		{
			name:   "lazy requires its element type",
			source: "let value Lazy = lazy { u32(7) }\n",
			want:   "Lazy requires exactly one type argument",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prog, err := Parse("<lazy-type-error>", test.source)
			if err == nil {
				_, err = NewRuntime().CheckProgram(context.Background(), prog)
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}
