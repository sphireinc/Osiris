package osiris

import (
	"reflect"
	"strings"
	"testing"
)

// TestFormatterLanguageCorpus keeps the formatter contract tied to the
// parser's current accepted surface. Each row is deliberately named after a
// language construct so a formatter gap points to the missing matrix cell
// instead of looking like an incidental snapshot mismatch.
func TestFormatterLanguageCorpus(t *testing.T) {
	cases := []struct {
		name     string
		source   string
		contains []string
	}{
		{
			name: "declarations-and-metadata",
			source: `package demo 1.2.3
export alpha, beta
import "std/text\\n" as text
ffi "lib/demo" as native
enum Status: u8 { Ready = 1, Done = 2 }
enum Label: string { Short = "short", Long = "long" }
enum Result { Ok(value), Err(message) }
struct Point { x u64 ` + "`json:\"x\" db:\"point_x\"`" + `, label string ` + "`json:\"label,omitempty\"`" + ` }
type PointAlias = Point
newtype UserID u64
const count = 1
let? result = ok(count)
let typed list<map<string, option>> = []
let (left, right) = (1, 2)
var mutable = 3
mutable = 4
macro identity(value) => value
func typed<T: number>(value T, point Point, callback func(u64) string) string => callback(value)
func block(value u64) u64 { let local = value; return local }
func noop() { return }
trait Render<T: string> { func render(value T) string; func empty() }
interface Iterable<T: list<u64>> { func next() option }
impl Render for Point { func render() string => "point" }
while mutable < 5 { mutable = mutable + 1; break }
for item in [1, 2] { continue }
defer noop()
emit text.upper("hello"), Status.Ready
`,
			contains: []string{
				"package demo 1.2.3",
				"let native = ffi_load(\"lib/demo\")",
				"struct Point",
				"`json:\"x\" db:\"point_x\"`",
				"type PointAlias = Point",
				"newtype UserID u64",
				"const count = 1u64",
				"let? result = ok(count)",
				"macro identity(value) => value",
			},
		},
		{
			name: "reserved-types-and-generic-types",
			source: `func all(
	a number, b u8, c u16, d u32, e u64,
	f i8, g i16, h i32, i i64, j f32, k f64,
	l string, m bool, n null, o list, p tuple, q tensor,
	r enum, s function, t error, u result, v option, w stream,
	x task, y map, z set, aa module, ab decimal, ac fixed,
	ad interval, ae modint, af complex, ag bigint, ah bigfloat,
	ai quantity, aj dual, ak duration, al datetime, am location,
	callback func(u32, string) bool, nested list<map<string, option>>
) string => "types"
func generic<T: list<u64>, U: func(u64) string>(value T, callback U) T => value
`,
			contains: []string{
				"func all(",
				"callback func(u32, string) bool",
				"nested list<map<string, option>>",
				"func generic<T: list<u64>, U: func(u64) string>",
			},
		},
		{
			name: "expressions-and-operators",
			source: `struct Point { x u64, y u64 }
enum Result { Ok(value), Err(message) }
let point = Point { x: 1, y: 2 }
let updated = point with { y: 3 }
let value = Result.Ok(1)
emit -1 + 2 * 3
emit !false && true || false
emit 1 | 2 ^ 3 & 4 << 1 == 8
emit (1, 2), (), (3,)
emit [1, [2, 3]]
emit point.x, point[0]
emit identity<u64>(1)
emit {}, some(1), none(), err("nope")
emit some(1)?
emit try { err("bad") } catch failure { failure }
emit match value { Result.Ok(payload) when payload > 0 => payload, Result.Err(message) => 0, default => 0 }
emit switch [1, 2] { [first, second] => first + second, _ => 0 }
`,
			contains: []string{
				"let point = Point { x: 1u64, y: 2u64 }",
				"let updated = point with { y: 3u64 }",
				"emit -(1u64 + 2u64 * 3u64)",
				"emit identity<u64>(1u64)",
				"emit {}, some(1u64), none(), err(\"nope\")",
				"emit some(1u64)?",
				"emit try { err(\"bad\") } catch failure { failure }",
				"match value { Result.Ok(payload) when payload > 0u64 => payload",
				"switch [1u64, 2u64] { [first, second] => first + second",
			},
		},
		{
			name: "patterns-and-nested-bindings",
			source: `enum Result { Ok(value), Err(message) }
let value = Result.Ok(7)
emit match value {
number n and >= 0 => n,
Result.Ok(x) | Result.Err(x) => x,
not null => 0,
[head, [inner, tail]] => head + inner + tail,
true => 1,
_ => 0
}
`,
			contains: []string{
				"number n and >= 0u64 => n",
				"Result.Ok(x) | Result.Err(x) => x",
				"not null => 0u64",
				"[head, [inner, tail]] => head + inner + tail",
			},
		},
		{
			name: "numeric-literals-and-stdlib-expressions",
			source: `let values = [0x2a, 0b101, 0o7, 1_000u64, 2i32, 3.5f32, 4bf16, 6d, 7i, 8uf32]
emit std.text.upper("hello")
emit std.math.abs(-3)
emit option_map(some(1), identity)
emit tensor([2, 2], [1, 2, 3, 4])
`,
			contains: []string{
				"42u64",
				"5u64",
				"3.5f32",
				"4bf16",
				"6d",
				"7i",
				"8uf32",
				"std.text.upper(\"hello\")",
				"tensor([2u64, 2u64], [1u64, 2u64, 3u64, 4u64])",
			},
		},
		{
			name:   "comments-semicolons-and-whitespace",
			source: "# leading comment\nlet\tx=1;\n// another comment\nemit\n  x\n",
			contains: []string{
				"let x = 1u64",
				"emit x",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original, err := Parse("formatter-"+tc.name+".osr", tc.source)
			if err != nil {
				t.Fatalf("corpus source does not parse: %v\nsource:\n%s", err, tc.source)
			}

			formatted, err := FormatSource("formatter-"+tc.name+".osr", tc.source)
			if err != nil {
				t.Fatalf("format failed: %v\nsource:\n%s", err, tc.source)
			}
			for _, fragment := range tc.contains {
				if !strings.Contains(formatted, fragment) {
					t.Errorf("formatted output missing %q:\n%s", fragment, formatted)
				}
			}

			formattedAgain, err := FormatSource("formatter-"+tc.name+".osr", formatted)
			if err != nil {
				t.Errorf("format(format(source)) failed: %v\nformatted:\n%s", err, formatted)
			} else if formattedAgain != formatted {
				t.Errorf("format is not idempotent\nfirst:\n%s\nsecond:\n%s", formatted, formattedAgain)
			}

			roundTripped, err := Parse("formatter-"+tc.name+"-roundtrip.osr", formatted)
			if err != nil {
				t.Errorf("formatted source does not parse: %v\nformatted:\n%s", err, formatted)
				return
			}
			if got, want := formatterSemanticSnapshot(roundTripped), formatterSemanticSnapshot(original); !reflect.DeepEqual(got, want) {
				t.Errorf("parse/format/parse changed semantic structure\noriginal: %#v\nround-trip: %#v", want, got)
			}
		})
	}
}

func TestFormatterMalformedCorpusPreservesDiagnosticLocation(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   string
	}{
		{name: "missing-expression", source: "let broken =\n", want: "expected expression"},
		{name: "unexpected-character", source: "let value = 1\n@\n", want: "unexpected character"},
		{name: "malformed-tag", source: "struct User { id u64 `json:bad` }\n", want: "requires a quoted value"},
		{name: "missing-enum-value", source: "enum Status: u8 { Ready }\n", want: "requires a value"},
		{name: "guarded-default", source: "emit match true { default when true => 1 }\n", want: "default match arm cannot have a guard"},
		{name: "unterminated-string", source: "emit \"unterminated\n", want: "unterminated string"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := FormatSource("formatter-malformed-"+tc.name+".osr", tc.source)
			if err == nil {
				t.Fatal("malformed source unexpectedly formatted")
			}
			message := err.Error()
			if !strings.Contains(message, tc.want) {
				t.Errorf("diagnostic = %q, want substring %q", message, tc.want)
			}
			if !strings.Contains(message, ":1:") && !strings.Contains(message, ":2:1") && !strings.Contains(message, "at 1:") && !strings.Contains(message, "at 2:1") {
				t.Errorf("diagnostic lost source location: %q", message)
			}
		})
	}
}

func TestFormatterMetadataSpansAreRecordedBeforeRoundTrip(t *testing.T) {
	source := "struct User { id u64 `json:\"id\"` }\nenum Status: u8 { Ready = 1 }\n"
	prog, err := Parse("formatter-metadata.osr", source)
	if err != nil {
		t.Fatal(err)
	}
	if len(prog.Statements) != 2 {
		t.Fatalf("statement count = %d, want 2", len(prog.Statements))
	}
	for i, stmt := range prog.Statements {
		span := formatterStatementSpan(stmt)
		if span.start.line == 0 || span.start.col == 0 || span.end.line == 0 || span.end.col == 0 {
			t.Errorf("statement %d has empty source span: %#v", i, span)
		}
	}
	structDecl := prog.Statements[0].(structStmt)
	if structDecl.Fields[0].Tag != `json:"id"` || structDecl.Fields[0].TagValues["json"] != "id" {
		t.Fatalf("struct tag metadata = %#v, want raw tag and decoded namespace", structDecl.Fields[0])
	}
	enumDecl := prog.Statements[1].(enumStmt)
	if enumDecl.Cases[0].BackingValue == nil || enumDecl.Cases[0].BackingValue.String() != "1u64" {
		t.Fatalf("enum backing metadata = %#v", enumDecl.Cases[0].BackingValue)
	}
	formatted, err := FormatSource("formatter-metadata.osr", source)
	if err != nil {
		t.Fatal(err)
	}
	roundTripped, err := Parse("formatter-metadata-roundtrip.osr", formatted)
	if err != nil {
		t.Fatalf("formatted metadata source does not parse: %v\n%s", err, formatted)
	}
	roundStruct := roundTripped.Statements[0].(structStmt)
	roundEnum := roundTripped.Statements[1].(enumStmt)
	if roundStruct.Fields[0].Span.start.line == 0 || roundStruct.Fields[0].Span.end.col == 0 {
		t.Errorf("formatted struct field lost source span: %#v", roundStruct.Fields[0].Span)
	}
	if roundEnum.Cases[0].Span.start.line == 0 || roundEnum.Cases[0].Span.end.col == 0 {
		t.Errorf("formatted enum case lost source span: %#v", roundEnum.Cases[0].Span)
	}
	if roundStruct.Fields[0].Tag != structDecl.Fields[0].Tag || roundStruct.Fields[0].TagValues["json"] != "id" {
		t.Errorf("formatted struct tag metadata = %#v", roundStruct.Fields[0])
	}
	if roundEnum.Cases[0].BackingValue == nil || roundEnum.Cases[0].BackingValue.String() != "1u64" {
		t.Errorf("formatted enum backing metadata = %#v", roundEnum.Cases[0].BackingValue)
	}
}

func formatterStatementSpan(stmt Stmt) sourceSpan {
	switch value := stmt.(type) {
	case structStmt:
		return value.Span
	case enumStmt:
		return value.Span
	default:
		return sourceSpan{}
	}
}

func formatterSemanticSnapshot(program *Program) any {
	statements := make([]any, len(program.Statements))
	for i, statement := range program.Statements {
		statements[i] = formatterSemanticStmt(statement)
	}
	return statements
}

func formatterSemanticStmt(statement Stmt) any {
	switch stmt := statement.(type) {
	case packageStmt:
		return map[string]any{"kind": "package", "name": stmt.Name, "version": stmt.Version}
	case exportStmt:
		return map[string]any{"kind": "export", "names": stmt.Names}
	case importStmt:
		return map[string]any{"kind": "import", "path": stmt.Path, "alias": stmt.Alias}
	case enumStmt:
		cases := make([]any, len(stmt.Cases))
		for i, item := range stmt.Cases {
			var backing any
			if item.BackingValue != nil {
				backing = formatterSemanticValue(*item.BackingValue)
			}
			cases[i] = map[string]any{"name": item.Name, "params": item.Params, "backing": backing}
		}
		return map[string]any{"kind": "enum", "name": stmt.Name, "backing": stmt.Backing, "cases": cases}
	case structStmt:
		fields := make([]any, len(stmt.Fields))
		for i, field := range stmt.Fields {
			fields[i] = map[string]any{"name": field.Name, "type": field.Type, "tag": field.Tag, "tagValues": field.TagValues}
		}
		return map[string]any{"kind": "struct", "name": stmt.Name, "fields": fields}
	case typeDeclStmt:
		return map[string]any{"kind": "type", "name": stmt.Name, "target": stmt.Target, "alias": stmt.Alias}
	case newtypeStmt:
		return map[string]any{"kind": "newtype", "name": stmt.Name, "target": stmt.Target}
	case traitStmt:
		methods := make([]any, len(stmt.Methods))
		for i, method := range stmt.Methods {
			methods[i] = formatterSemanticFunction(method)
		}
		return map[string]any{"kind": "trait", "name": stmt.Name, "params": formatterSemanticTypeParams(stmt.TypeParams), "methods": methods}
	case implStmt:
		methods := make([]any, len(stmt.Methods))
		for i, method := range stmt.Methods {
			methods[i] = formatterSemanticFunction(method)
		}
		return map[string]any{"kind": "impl", "trait": stmt.Trait, "target": stmt.Target, "methods": methods}
	case letStmt:
		return map[string]any{"kind": "let", "name": stmt.Name, "names": stmt.Names, "type": stmt.Type, "expr": formatterSemanticExpr(stmt.Expr), "unwrap": stmt.Unwrap, "mutable": stmt.Mutable, "const": stmt.Const}
	case assignStmt:
		return map[string]any{"kind": "assign", "name": stmt.Name, "expr": formatterSemanticExpr(stmt.Expr)}
	case emitStmt:
		return map[string]any{"kind": "emit", "expr": formatterSemanticExpr(stmt.Expr)}
	case deferStmt:
		return map[string]any{"kind": "defer", "body": formatterSemanticExpr(stmt.Body)}
	case macroStmt:
		return map[string]any{"kind": "macro", "name": stmt.Name, "params": stmt.Params, "body": formatterSemanticExpr(stmt.Body)}
	case funcStmt:
		return formatterSemanticFunction(stmt)
	case returnStmt:
		return map[string]any{"kind": "return", "expr": formatterSemanticExpr(stmt.Expr)}
	case whileStmt:
		return map[string]any{"kind": "while", "cond": formatterSemanticExpr(stmt.Cond), "body": formatterSemanticExpr(stmt.Body)}
	case forStmt:
		return map[string]any{"kind": "for", "name": stmt.Name, "iter": formatterSemanticExpr(stmt.Iter), "body": formatterSemanticExpr(stmt.Body)}
	case breakStmt:
		return map[string]any{"kind": "break"}
	case continueStmt:
		return map[string]any{"kind": "continue"}
	default:
		return map[string]any{"kind": "unknown", "value": statement}
	}
}

func formatterSemanticFunction(function funcStmt) any {
	return map[string]any{
		"kind":         "func",
		"name":         function.Name,
		"receiver":     function.Receiver,
		"receiverType": function.ReceiverType,
		"typeParams":   formatterSemanticTypeParams(function.TypeParams),
		"params":       function.Params,
		"paramTypes":   function.ParamTypes,
		"returnType":   function.ReturnType,
		"body":         formatterSemanticExpr(function.Body),
	}
}

func formatterSemanticTypeParams(params []typeParam) any {
	result := make([]any, len(params))
	for i, param := range params {
		result[i] = map[string]any{"name": param.Name, "bound": param.Bound}
	}
	return result
}

func formatterSemanticExpr(expression Expr) any {
	if expression == nil {
		return nil
	}
	switch expression := expression.(type) {
	case *literalExpr:
		return map[string]any{"kind": "literal", "value": formatterSemanticValue(expression.value)}
	case *identExpr:
		return map[string]any{"kind": "ident", "name": expression.name}
	case *unaryExpr:
		return map[string]any{"kind": "unary", "op": expression.op.String(), "expr": formatterSemanticExpr(expression.expr)}
	case *binaryExpr:
		return map[string]any{"kind": "binary", "op": expression.op.String(), "left": formatterSemanticExpr(expression.left), "right": formatterSemanticExpr(expression.right)}
	case *callExpr:
		args := make([]any, len(expression.args))
		for i, arg := range expression.args {
			args[i] = formatterSemanticExpr(arg)
		}
		return map[string]any{"kind": "call", "callee": formatterSemanticExpr(expression.callee), "args": args, "typeArgs": expression.typeArgs}
	case *blockExpr:
		statements := make([]any, len(expression.stmts))
		for i, statement := range expression.stmts {
			statements[i] = formatterSemanticStmt(statement)
		}
		return map[string]any{"kind": "block", "statements": statements, "tail": formatterSemanticExpr(expression.tail)}
	case *tryExpr:
		return map[string]any{"kind": "try", "body": formatterSemanticExpr(expression.body), "catch": expression.catchName, "handler": formatterSemanticExpr(expression.handler)}
	case *propagateExpr:
		return map[string]any{"kind": "propagate", "expr": formatterSemanticExpr(expression.expr)}
	case *memberExpr:
		return map[string]any{"kind": "member", "object": formatterSemanticExpr(expression.object), "name": expression.name}
	case *listExpr:
		return map[string]any{"kind": "list", "items": formatterSemanticExprs(expression.items)}
	case *tupleExpr:
		return map[string]any{"kind": "tuple", "items": formatterSemanticExprs(expression.items)}
	case *structExpr:
		return map[string]any{"kind": "struct", "type": expression.TypeName, "fields": formatterSemanticFields(expression.Fields)}
	case *structUpdateExpr:
		return map[string]any{"kind": "update", "base": formatterSemanticExpr(expression.Base), "fields": formatterSemanticFields(expression.Fields)}
	case *indexExpr:
		return map[string]any{"kind": "index", "object": formatterSemanticExpr(expression.Object), "index": formatterSemanticExpr(expression.Index)}
	case *matchExpr:
		arms := make([]any, len(expression.arms))
		for i, arm := range expression.arms {
			arms[i] = map[string]any{"pattern": formatterSemanticPattern(arm.pattern), "guard": formatterSemanticExpr(arm.guard), "body": formatterSemanticExpr(arm.body), "default": arm.isDefault}
		}
		return map[string]any{"kind": "match", "subject": formatterSemanticExpr(expression.subject), "arms": arms, "keyword": expression.keyword.String()}
	default:
		return map[string]any{"kind": "unknown", "value": expression}
	}
}

func formatterSemanticExprs(expressions []Expr) []any {
	result := make([]any, len(expressions))
	for i, expression := range expressions {
		result[i] = formatterSemanticExpr(expression)
	}
	return result
}

func formatterSemanticFields(fields []structFieldExpr) []any {
	result := make([]any, len(fields))
	for i, field := range fields {
		result[i] = map[string]any{"name": field.Name, "expr": formatterSemanticExpr(field.Expr)}
	}
	return result
}

func formatterSemanticPattern(pattern Pattern) any {
	if pattern == nil {
		return nil
	}
	switch pattern := pattern.(type) {
	case wildcardPattern:
		return map[string]any{"kind": "wildcard"}
	case bindPattern:
		return map[string]any{"kind": "bind", "name": pattern.name}
	case enumPattern:
		items := make([]any, len(pattern.items))
		for i, item := range pattern.items {
			items[i] = formatterSemanticPattern(item)
		}
		return map[string]any{"kind": "enum", "type": pattern.typeName, "case": pattern.caseName, "items": items}
	case orPattern:
		items := make([]any, len(pattern.alternatives))
		for i, item := range pattern.alternatives {
			items[i] = formatterSemanticPattern(item)
		}
		return map[string]any{"kind": "or", "items": items}
	case andPattern:
		return map[string]any{"kind": "and", "left": formatterSemanticPattern(pattern.left), "right": formatterSemanticPattern(pattern.right)}
	case notPattern:
		return map[string]any{"kind": "not", "inner": formatterSemanticPattern(pattern.inner)}
	case typePattern:
		return map[string]any{"kind": "type", "type": pattern.typeName, "binding": pattern.binding}
	case relationalPattern:
		return map[string]any{"kind": "relational", "op": pattern.op.String(), "value": formatterSemanticValue(pattern.value)}
	case literalPattern:
		return map[string]any{"kind": "literal", "value": formatterSemanticValue(pattern.value)}
	case listPattern:
		items := make([]any, len(pattern.items))
		for i, item := range pattern.items {
			items[i] = formatterSemanticPattern(item)
		}
		return map[string]any{"kind": "list", "items": items}
	default:
		return map[string]any{"kind": "unknown", "value": pattern}
	}
}

func formatterSemanticValue(value Value) any {
	return []string{value.Kind(), value.String()}
}
