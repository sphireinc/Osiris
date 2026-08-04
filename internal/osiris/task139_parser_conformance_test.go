package osiris

import (
	"strings"
	"testing"
)

func TestTask139LanguageSurfaceParseFixtures(t *testing.T) {
	fixtures := map[string]string{
		"bindings": `let answer = 42
const limit = 2 + 3
var total = 0
total = total + 2
let? value = ok(41)
emit answer, limit, total, value`,
		"functions-and-generics": `func double<T: number>(value T) T => value
func apply(value u32, callback func(u32) u32) u32 => callback(value)
func announce(value) { emit value; return value + 1 }`,
		"modules-and-ffi": `package demo "1"
export alpha, beta
import "std/text" as text
ffi "./math.ffi.json" as native_math`,
		"control-flow": `while true { break }
for letter in "go" { continue }
defer { emit "cleanup" }
return`,
		"match-and-patterns": `emit match [1, 2] { [x, y] => x + y, _ => 0 }
emit switch 0 { 0 => "zero", default => "other" }
emit match value { number n when n > 0 => n, _ => 0 }`,
		"types-and-data": `enum Status { Ready, Done }
struct Point { x u64 ` + "`json:\"x\"`" + `, y u64 }
type Count = u64
newtype UserID u64
let pair = ("ready", u64(3))
let point = Point { x: u64(1), y: u64(2) }
let moved = point with { x: u64(3) }`,
		"traits-and-interfaces": `trait Printable { func render(value u64) string; }
interface Named { func name() string; }
struct Point { value u64 }
impl Printable for Point { func render(value u64) string => "point" }`,
		"macros-and-try": `macro shout(x) => text.upper(x)
emit try { read("/definitely/missing.osiris") } catch err { err.kind }`,
	}

	for name, source := range fixtures {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse("<task139-"+name+">", source); err != nil {
				t.Fatalf("parse failed: %v\nsource:\n%s", err, source)
			}
			formatted, err := FormatSource("<task139-format-"+name+">", source)
			if err != nil {
				t.Fatalf("format failed: %v", err)
			}
			if _, err := Parse("<task139-roundtrip-"+name+">", formatted); err != nil {
				t.Fatalf("formatted source did not parse: %v\nformatted:\n%s", err, formatted)
			}
		})
	}
}

func TestTask139InvalidAndBoundaryDiagnostics(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   string
	}{
		{name: "unterminated-string", source: "emit \"unterminated", want: "<task139-invalid>:1:6"},
		{name: "missing-expression", source: "let value =", want: "<task139-invalid>:1:12"},
		{name: "missing-match-arm", source: "emit match 1 {}", want: "match requires at least one arm"},
		{name: "invalid-number-suffix", source: "emit 1u7", want: "<task139-invalid>:1:6"},
		{name: "unclosed-list", source: "emit [1, 2", want: "<task139-invalid>:1:11"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse("<task139-invalid>", tc.source)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}

	for _, source := range []string{"emit .5", "emit 0.5e-2f32", "emit \"😀\"", "emit []", "emit {}"} {
		t.Run("boundary/"+source, func(t *testing.T) {
			if _, err := Parse("<task139-boundary>", source); err != nil {
				t.Fatalf("parse failed: %v", err)
			}
		})
	}
}

func TestTask139GenericTraitSignatureParses(t *testing.T) {
	source := `trait Comparable<T: number> { func compare(value T) bool; }`
	if _, err := Parse("<task139-generic-trait>", source); err != nil {
		t.Fatalf("generic trait parse failed: %v", err)
	}
}

func TestTask139NestedGenericTypeAndCallBoundaries(t *testing.T) {
	fixtures := []string{
		`let values map<string, list<u64>> = map_new("values", [1, 2])`,
		`func wrap<T>(value T) T => value
emit wrap<list<u64>>([1, 2])`,
	}
	for _, source := range fixtures {
		t.Run(source, func(t *testing.T) {
			if _, err := Parse("<task139-nested-generic>", source); err != nil {
				t.Fatalf("nested generic source failed to parse: %v\nsource:\n%s", err, source)
			}
			formatted, err := FormatSource("<task139-nested-generic-format>", source)
			if err != nil {
				t.Fatalf("nested generic source failed to format: %v\nsource:\n%s", err, source)
			}
			if _, err := Parse("<task139-nested-generic-roundtrip>", formatted); err != nil {
				t.Fatalf("formatted nested generic source failed to parse: %v\nformatted:\n%s", err, formatted)
			}
		})
	}
}

func TestTask139SourceSpansAndFormatParseRoundTrip(t *testing.T) {
	source := "let value = [1, (2, 3)]\nemit value\n"
	prog, err := Parse("<task139-span>", source)
	if err != nil {
		t.Fatal(err)
	}
	let := prog.Statements[0].(letStmt)
	if let.Span.start != (sourcePos{line: 1, col: 1}) || let.Span.end.line != 1 || let.Span.end.col <= let.Span.start.col {
		t.Fatalf("let span = %#v", let.Span)
	}
	list := let.Expr.(*listExpr)
	if list.Span.start != (sourcePos{line: 1, col: 13}) || list.Span.end.col <= list.Span.start.col {
		t.Fatalf("list span = %#v", list.Span)
	}

	formatted, err := FormatSource("<task139-format>", source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse("<task139-format-roundtrip>", formatted); err != nil {
		t.Fatalf("formatted source did not parse: %v\nformatted:\n%s", err, formatted)
	}
}
