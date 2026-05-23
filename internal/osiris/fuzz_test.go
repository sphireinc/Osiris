package osiris

import (
	"context"
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
