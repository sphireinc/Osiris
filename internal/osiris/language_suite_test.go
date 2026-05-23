package osiris

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runLanguageProgram(t *testing.T, source string) []Value {
	t.Helper()
	prog, err := Parse("<suite>", source)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	return out
}

func assertLanguageOutputs(t *testing.T, source string, want []string) {
	t.Helper()
	out := runLanguageProgram(t, source)
	if got, wantLen := len(out), len(want); got != wantLen {
		t.Fatalf("len(out) = %d, want %d", got, wantLen)
	}
	for i := range want {
		if got := out[i].String(); got != want[i] {
			t.Fatalf("output[%d] = %s, want %s", i, got, want[i])
		}
	}
}

func runLanguageProgramVM(t *testing.T, source string) []Value {
	t.Helper()
	prog, err := Parse("<suite>", source)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.RunVM(context.Background(), prog)
	if err != nil {
		t.Fatalf("vm run failed: %v", err)
	}
	return out
}

func TestLanguageGeneralFeatureSuite(t *testing.T) {
	t.Run("expressions-booleans-and-precedence", func(t *testing.T) {
		assertLanguageOutputs(t, `
emit 1 + 2 * 3
emit true && false || !false
emit if(3 > 2, "yes", "no")
emit (1 + 2) * 3
`, []string{
			"7u64",
			"true",
			`"yes"`,
			"9u64",
		})
	})

	t.Run("functions-loops-and-recursion", func(t *testing.T) {
		assertLanguageOutputs(t, `
func fact(n) => if(n < 2, 1, n * fact(n - 1))
emit fact(6)

let i = 0
let acc = 0
while i < 5 {
  let acc = acc + i
  let i = i + 1
}
emit acc
`, []string{
			"720u64",
			"10u64",
		})
	})

	t.Run("exact-wide-and-high-precision-numerics", func(t *testing.T) {
		assertLanguageOutputs(t, `
emit 7 / 25 * 25 == 7
emit 1 / 3 + 1 / 6 == 1 / 2
emit pow(bigint("999999999999"), 3)
emit 1u128 << 100
emit 1.5f32 + 2.25f32
emit dec("1.25") + dec("0.75")
`, []string{
			"true",
			"true",
			"999999999997000000000002999999999999n",
			"1267650600228229401496703205376u128",
			"3.75f32",
			"2d",
		})
	})

	t.Run("tensors-shapes-and-linear-algebra", func(t *testing.T) {
		assertLanguageOutputs(t, `
emit shape(matrix(2, 3, [1, 2, 3, 4, 5, 6]))
emit trace(matrix(2, 2, [1, 2, 3, 4]))
emit det(matrix(2, 2, [1, 2, 3, 4]))
emit solve(matrix(2, 2, [1, 2, 3, 4]), [5, 11])
emit eig(matrix(2, 2, [3, 0, 0, 2]))
emit eigh(matrix(2, 2, [5, 0, 0, 7]))
emit svd(matrix(2, 2, [3, 0, 0, 2]))
`, []string{
			"[2u64, 3u64]",
			"5",
			"-2",
			"tensor[2]([1, 2])",
			"[tensor[2]([3, 2]), tensor[2x2]([1, 0, 0, 1])]",
			"[tensor[2]([5, 7]), tensor[2x2]([1, 0, 0, 1])]",
			"[tensor[2x2]([1, 0, 0, 1]), tensor[2]([3, 2]), tensor[2x2]([1, 0, 0, 1])]",
		})
	})

	t.Run("stdlib-and-modules", func(t *testing.T) {
		dir := t.TempDir()
		modulePath := filepath.Join(dir, "sigil.osr")
		if err := os.WriteFile(modulePath, []byte(`
package sigil
export answer
let answer = slug("Hello, World")
`), 0o644); err != nil {
			t.Fatalf("write module: %v", err)
		}
		mainPath := filepath.Join(dir, "main.osr")
		prog, err := Parse(mainPath, "import "+quoteString(modulePath)+" as sigil\nimport \"std/text\" as text\nemit text.upper(sigil.answer)\n")
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
		if got, want := out[0].String(), `"HELLO-WORLD"`; got != want {
			t.Fatalf("output = %s, want %s", got, want)
		}
	})

	t.Run("macros-blocks-and-errors", func(t *testing.T) {
		assertLanguageOutputs(t, `
let suffix = "!"
macro shout(x) => cat(x, suffix)
emit shout("osiris")
emit { let x = 2; let y = 3; x + y }
emit try { uint("7") } catch err { cat("caught: ", err) }
`, []string{
			`"osiris!"`,
			"5u64",
			`"caught: expected unsigned integer, got string"`,
		})
	})

	t.Run("codec-roundtrip-and-bytes", func(t *testing.T) {
		assertLanguageOutputs(t, `
emit decode(encode([1u8, 2u16, "go"]))
emit hex(bytes("AB"))
`, []string{
			"[1u8, 2u16, \"go\"]",
			`"4142"`,
		})
	})

	t.Run("format-roundtrip-match-and-switch", func(t *testing.T) {
		source := `
let suffix = "!"
macro shout(x) => cat(x, suffix)
emit match [1, 2] { [a, b] => a + b; _ => 0 }
emit switch 9 { 1 => "one"; _ => "other" }
emit shout("osiris")
`
		formatted, err := FormatSource("<suite>", source)
		if err != nil {
			t.Fatalf("format failed: %v", err)
		}
		if !strings.Contains(formatted, "match ") {
			t.Fatalf("formatted output missing match: %s", formatted)
		}
		if !strings.Contains(formatted, "switch ") {
			t.Fatalf("formatted output missing switch: %s", formatted)
		}
		assertLanguageOutputs(t, formatted, []string{
			"3u64",
			`"other"`,
			`"osiris!"`,
		})
	})

	t.Run("vm-parity-on-core-programs", func(t *testing.T) {
		source := `
func fib(n) => if(n < 2, n, fib(n - 1) + fib(n - 2))
emit fib(10)
emit sum([1, 2, 3, 4, 5])
emit matmul(matrix(2, 2, [1, 2, 3, 4]), matrix(2, 2, [5, 6, 7, 8]))
`
		out := runLanguageProgram(t, source)
		vmOut := runLanguageProgramVM(t, source)
		if got, want := len(out), len(vmOut); got != want {
			t.Fatalf("len(run) = %d, len(vm) = %d", got, want)
		}
		for i := range out {
			if got, want := out[i].String(), vmOut[i].String(); got != want {
				t.Fatalf("vm parity mismatch at %d: run=%s vm=%s", i, got, want)
			}
		}
		if got, want := out[0].String(), "55u64"; got != want {
			t.Fatalf("fib output = %s, want %s", got, want)
		}
		if got, want := out[1].String(), "15"; got != want {
			t.Fatalf("sum output = %s, want %s", got, want)
		}
		if got, want := out[2].String(), "tensor[2x2]([19, 22, 43, 50])"; got != want {
			t.Fatalf("matmul output = %s, want %s", got, want)
		}
	})
}
