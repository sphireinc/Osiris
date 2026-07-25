package osiris

import (
	"context"
	"encoding/json"
	"testing"
)

// FuzzFormatRoundTrip feeds randomized inputs into the target parser,
// formatter, or evaluator path so that malformed or surprising inputs are
// caught before they can become runtime panics.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func FuzzFormatRoundTrip(f *testing.F) {
	seeds := []string{
		`emit 1 + 2 * 3`,
		`emit match [1, 2] { [a, b] => a + b; _ => 0 }`,
		`package sigil; export value; let value = hash("seed")`,
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, src string) {
		prog, err := Parse("<fuzz>", src)
		if err != nil {
			return
		}
		formatted := FormatProgram(prog)
		roundTripped, err := Parse("<roundtrip>", formatted)
		if err != nil {
			t.Fatalf("format output did not parse: %v\n%s", err, formatted)
		}
		if got, want := FormatProgram(roundTripped), formatted; got != want {
			t.Fatalf("format round-trip mismatch\nwant:\n%s\ngot:\n%s", want, got)
		}
	})
}

// FuzzSourceMapAndErrorsNoPanic keeps source-map construction and structured
// parse-error normalization safe for malformed editor input.
func FuzzSourceMapAndErrorsNoPanic(f *testing.F) {
	for _, seed := range []string{`emit 1`, `let =`, `emit match [1, 2] { [a, b] => a + b }`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, src string) {
		if len(src) > 4096 {
			t.Skip()
		}
		prog, err := Parse("<fuzz-map>", src)
		if err != nil {
			_, marshalErr := json.Marshal(errorValueFromGoError(normalizeError(err, "<fuzz-map>")))
			if marshalErr != nil {
				t.Fatalf("marshal normalized parse error: %v", marshalErr)
			}
			return
		}
		if _, err := json.Marshal(BuildSourceMap(prog)); err != nil {
			t.Fatalf("marshal source map: %v", err)
		}
	})
}

// FuzzEvalNoPanic feeds randomized inputs into the target parser, formatter,
// or evaluator path so that malformed or surprising inputs are caught before
// they can become runtime panics.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func FuzzEvalNoPanic(f *testing.F) {
	seeds := []string{
		`1 + 2 * 3`,
		`match [1, 2] { [a, b] => a + b; _ => 0 }`,
		`decode(encode([1u8, 2u16, "go"]))`,
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, expr string) {
		prog, err := Parse("<fuzz>", "emit "+expr)
		if err != nil {
			return
		}
		rt := NewRuntime()
		_, _ = rt.Run(context.Background(), prog)
	})
}

// FuzzInterpreterVMParity exercises a deliberately bounded expression subset.
// Prefixing with emit prevents generated input from introducing statement-level
// loops/tasks while still covering parser, evaluator, and bytecode behavior.
func FuzzInterpreterVMParity(f *testing.F) {
	for _, seed := range []string{
		`1 + 2 * 3`,
		`u8(255) + u8(1)`,
		`if(3 > 2, "yes", "no")`,
		`decode(encode([1u8, 2u16, "go"]))`,
		`match [1, 2] { [a, b] => a + b; _ => 0 }`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, expr string) {
		if len(expr) > 1024 {
			t.Skip()
		}
		prog, err := Parse("<fuzz-vm>", "emit "+expr)
		if err != nil {
			return
		}
		rt := NewRuntime()
		interpreter, interpreterErr := rt.Run(context.Background(), prog)
		vm, vmErr := rt.RunVM(context.Background(), prog)
		if (interpreterErr == nil) != (vmErr == nil) {
			t.Fatalf("interpreter error %v does not match VM error %v for %q", interpreterErr, vmErr, expr)
		}
		if interpreterErr != nil {
			return
		}
		if got, want := valuesOutput(vm), valuesOutput(interpreter); got != want {
			t.Fatalf("VM output %q, want %q for %q", got, want, expr)
		}
	})
}
