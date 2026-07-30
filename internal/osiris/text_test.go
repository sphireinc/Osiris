package osiris

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func runTextProgram(t *testing.T, vm bool, expression string) (Value, error) {
	t.Helper()
	source := "import \"std/text\" as text\nemit " + expression + "\n"
	prog, err := Parse("<text-test>", source)
	if err != nil {
		t.Fatalf("parse failed for %s: %v", expression, err)
	}
	var output []Value
	if vm {
		output, err = NewRuntime().RunVM(context.Background(), prog)
	} else {
		output, err = NewRuntime().Run(context.Background(), prog)
	}
	if err != nil {
		return Value{}, err
	}
	if len(output) != 1 {
		t.Fatalf("%s produced %d values, want 1", expression, len(output))
	}
	return output[0], nil
}

func TestStdTextFunctionsInterpreterVMParity(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		want       string
	}{
		{name: "compare less", expression: `text.compare("alpha", "beta")`, want: `-1i64`},
		{name: "compare equal", expression: `text.compare("", "")`, want: `0i64`},
		{name: "equal fold unicode", expression: `text.equal_fold("Σ", "ς")`, want: `true`},
		{name: "equal fold false", expression: `text.equal_fold("resume", "résumé")`, want: `false`},
		{name: "contains", expression: `text.contains("naïve café", "café")`, want: `true`},
		{name: "contains empty", expression: `text.contains("", "")`, want: `true`},
		{name: "contains any unicode", expression: `text.contains_any("rhythm世", "ae世")`, want: `true`},
		{name: "contains any empty set", expression: `text.contains_any("osiris", "")`, want: `false`},
		{name: "has prefix", expression: `text.has_prefix("世界", "世")`, want: `true`},
		{name: "has prefix empty", expression: `text.has_prefix("world", "")`, want: `true`},
		{name: "has suffix", expression: `text.has_suffix("café", "fé")`, want: `true`},
		{name: "has suffix absent", expression: `text.has_suffix("café", "tea")`, want: `false`},
		{name: "count nonoverlapping", expression: `text.count("aaaa", "aa")`, want: `2u64`},
		{name: "count empty unicode", expression: `text.count("a世", "")`, want: `3u64`},
		{name: "index code points", expression: `text.index("a世b", "b")`, want: `some(2u64)`},
		{name: "index absent", expression: `text.index("a世b", "x")`, want: `none`},
		{name: "index empty", expression: `text.index("a世b", "")`, want: `some(0u64)`},
		{name: "index any code points", expression: `text.index_any("αβç", "çx")`, want: `some(2u64)`},
		{name: "index any empty set", expression: `text.index_any("abc", "")`, want: `none`},
		{name: "last index code points", expression: `text.last_index("é-é", "é")`, want: `some(2u64)`},
		{name: "last index empty", expression: `text.last_index("a世b", "")`, want: `some(3u64)`},
		{name: "last index any code points", expression: `text.last_index_any("a世b世", "世")`, want: `some(3u64)`},
		{name: "last index any absent", expression: `text.last_index_any("abc", "世")`, want: `none`},
		{name: "cut", expression: `text.cut("key=value=tail", "=")`, want: `some(["key", "value=tail"])`},
		{name: "cut absent", expression: `text.cut("key", "=")`, want: `none`},
		{name: "cut empty", expression: `text.cut("abc", "")`, want: `some(["", "abc"])`},
		{name: "cut prefix", expression: `text.cut_prefix("unhappy", "un")`, want: `some("happy")`},
		{name: "cut prefix absent", expression: `text.cut_prefix("happy", "un")`, want: `none`},
		{name: "cut suffix", expression: `text.cut_suffix("report.txt", ".txt")`, want: `some("report")`},
		{name: "cut suffix empty", expression: `text.cut_suffix("report", "")`, want: `some("report")`},
		{name: "fields unicode space", expression: "text.fields(\"  alpha\u2003β\\t\")", want: `["alpha", "β"]`},
		{name: "fields empty", expression: `text.fields("")`, want: `[]`},
		{name: "split after repeated", expression: `text.split_after("a,,b", ",")`, want: `["a,", ",", "b"]`},
		{name: "split after empty separator", expression: `text.split_after("世a", "")`, want: `["世", "a"]`},
		{name: "split n negative", expression: `text.split_n("a,,b", ",", -1)`, want: `["a", "", "b"]`},
		{name: "split n zero", expression: `text.split_n("a,,b", ",", 0)`, want: `[]`},
		{name: "split n positive", expression: `text.split_n("a,,b", ",", 2)`, want: `["a", ",b"]`},
		{name: "split n empty separator", expression: `text.split_n("世ab", "", 2)`, want: `["世", "ab"]`},
		{name: "split after n negative", expression: `text.split_after_n("a,,b", ",", -1)`, want: `["a,", ",", "b"]`},
		{name: "split after n zero", expression: `text.split_after_n("a,,b", ",", 0)`, want: `[]`},
		{name: "split after n positive", expression: `text.split_after_n("a,,b", ",", 2)`, want: `["a,", ",b"]`},
		{name: "trim space unicode", expression: "text.trim_space(\"\u2003osiris\u00a0\")", want: `"osiris"`},
		{name: "trim left rune set", expression: `text.trim_left("¡¡hola!!", "¡")`, want: `"hola!!"`},
		{name: "trim right rune set", expression: `text.trim_right("¡¡hola!!", "!")`, want: `"¡¡hola"`},
		{name: "trim prefix", expression: `text.trim_prefix("foobar", "foo")`, want: `"bar"`},
		{name: "trim prefix absent", expression: `text.trim_prefix("foobar", "bar")`, want: `"foobar"`},
		{name: "trim suffix", expression: `text.trim_suffix("foobar", "bar")`, want: `"foo"`},
		{name: "trim suffix empty", expression: `text.trim_suffix("foobar", "")`, want: `"foobar"`},
		{name: "replace n", expression: `text.replace_n("aaa", "a", "世", 2)`, want: `"世世a"`},
		{name: "replace n zero", expression: `text.replace_n("aaa", "a", "b", 0)`, want: `"aaa"`},
		{name: "replace n negative", expression: `text.replace_n("aaa", "a", "b", -1)`, want: `"bbb"`},
		{name: "replace n empty needle", expression: `text.replace_n("ab", "", "_", 2)`, want: `"_a_b"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, vm := range []bool{false, true} {
				got, err := runTextProgram(t, vm, test.expression)
				if err != nil {
					t.Fatalf("vm=%v: %v", vm, err)
				}
				if got.String() != test.want {
					t.Fatalf("vm=%v: %s = %s, want %s", vm, test.expression, got.String(), test.want)
				}
			}
		})
	}
}

func TestStdTextStructuredErrors(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		kind       ErrorKind
		message    string
	}{
		{name: "arity", expression: `text.compare("one")`, kind: ErrorKindInvalidArgument, message: "compare expects 2 arguments"},
		{name: "kind", expression: `text.contains("one", true)`, kind: ErrorKindType, message: "contains argument 2 expects string, got bool"},
		{name: "limit kind", expression: `text.split_n("a,b", ",", 1.5)`, kind: ErrorKindType, message: "split_n argument 3 expects an integer, got number"},
		{name: "limit overflow", expression: `text.split_after_n("a,b", ",", 18446744073709551615u64)`, kind: ErrorKindOutOfRange, message: "split_after_n argument 3 exceeds the host allocation limit"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, vm := range []bool{false, true} {
				_, err := runTextProgram(t, vm, test.expression)
				if err == nil {
					t.Fatalf("vm=%v: expected error", vm)
				}
				var record *OsirisError
				if !errors.As(err, &record) {
					t.Fatalf("vm=%v: error is not structured: %T %v", vm, err, err)
				}
				if record.kind != test.kind || !strings.Contains(record.message, test.message) {
					t.Fatalf("vm=%v: error = kind %s message %q, want %s containing %q", vm, record.kind, record.message, test.kind, test.message)
				}
				if record.location == nil || record.location.Source != "<text-test>" || record.location.Span.start.line != 2 {
					t.Fatalf("vm=%v: error lacks call source location: %#v", vm, record.location)
				}
			}
		})
	}
}

func TestStdTextInferenceAndNativeGate(t *testing.T) {
	const source = `import "std/text" as text
let ordering = text.compare("a", "b")
let predicate = text.contains("abc", "b")
let counted = text.count("banana", "an")
let found = text.index("a世b", "b")
let cut = text.cut("a=b", "=")
let tokens = text.fields("a b")
let transformed = text.replace_n("aaa", "a", "b", 2)
emit ordering, predicate, counted, found, cut, tokens, transformed
`
	prog, err := Parse("<text-static>", source)
	if err != nil {
		t.Fatal(err)
	}
	inferred, err := NewRuntime().InferProgram(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	report := FormatInferProgram(inferred)
	for _, want := range []string{
		"main.ordering: int64",
		"main.predicate: bool",
		"main.counted: uint64",
		"main.found: option",
		"main.cut: option",
		"main.tokens: list",
		"main.transformed: string",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("inference lacks %q:\n%s", want, report)
		}
	}
	ir, err := NewRuntime().BuildIR(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	irText := FormatIRProgram(ir)
	for _, want := range []string{`import "std/text"`, "member ", ".index", "call "} {
		if !strings.Contains(irText, want) {
			t.Errorf("IR lacks %q:\n%s", want, irText)
		}
	}
	if _, err := CompileC(context.Background(), NewRuntime(), prog); err == nil {
		t.Fatal("native compilation unexpectedly accepted std/text")
	} else {
		for _, want := range []string{`native C backend: static module "std/text" is not supported`, "<text-static>:1:8"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("native diagnostic %q lacks %q", err, want)
			}
		}
	}
}

func FuzzStdTextCutReconstruction(f *testing.F) {
	f.Add("key=value=tail", "=")
	f.Add("世界", "世")
	f.Add("", "")
	f.Fuzz(func(t *testing.T, text, separator string) {
		got, err := textCutValue([]Value{stringValue(text), stringValue(separator)})
		if err != nil {
			t.Fatal(err)
		}
		record := got.optionRecord()
		if !strings.Contains(text, separator) {
			if record == nil || record.present {
				t.Fatalf("cut(%q, %q) = %s, want none", text, separator, got.String())
			}
			return
		}
		if record == nil || !record.present || record.val.kind != kindList || len(record.val.list) != 2 {
			t.Fatalf("cut(%q, %q) has invalid shape: %s", text, separator, got.String())
		}
		before, err := record.val.list[0].AsString()
		if err != nil {
			t.Fatal(err)
		}
		after, err := record.val.list[1].AsString()
		if err != nil {
			t.Fatal(err)
		}
		if reconstructed := before + separator + after; reconstructed != text {
			t.Fatalf("cut reconstruction = %q, want %q", reconstructed, text)
		}
	})
}

func FuzzStdTextSplitJoinRoundTrip(f *testing.F) {
	f.Add("a,,b", ",")
	f.Add("世・界", "・")
	f.Add("", "|")
	f.Fuzz(func(t *testing.T, text, separator string) {
		if separator == "" {
			t.Skip()
		}
		parts, err := splitValue([]Value{stringValue(text), stringValue(separator)})
		if err != nil {
			t.Fatal(err)
		}
		joined, err := joinValue([]Value{stringValue(separator), parts})
		if err != nil {
			t.Fatal(err)
		}
		got, err := joined.AsString()
		if err != nil {
			t.Fatal(err)
		}
		if got != text {
			t.Fatalf("join(split(%q, %q)) = %q", text, separator, got)
		}
	})
}
