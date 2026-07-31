package osiris

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTypedFunctionSignaturesParseFormatParse(t *testing.T) {
	source := `func increment(value u32) u32 => value + u32(1)
func apply(value u32, transform func(u32) u32) u32 {
  return transform(value)
}
let callback func(u32) u32 = increment
emit apply(u32(2), callback)
`
	formatted, err := FormatSource("<typed-functions>", source)
	if err != nil {
		t.Fatal(err)
	}
	const want = `func increment(value u32) u32 => value + u32(1u64)
func apply(value u32, transform func(u32) u32) u32 { return transform(value) }
let callback func(u32) u32 = increment
emit apply(u32(2u64), callback)
`
	if formatted != want {
		t.Fatalf("formatted = %q, want %q", formatted, want)
	}
	reparsed, err := Parse("<typed-functions-formatted>", formatted)
	if err != nil {
		t.Fatal(err)
	}
	if got := FormatProgram(reparsed); got != want {
		t.Fatalf("second format = %q, want %q", got, want)
	}
	optimized := OptimizeProgram(reparsed)
	optimizedSource := FormatProgram(optimized)
	for _, signature := range []string{
		"func increment(value u32) u32",
		"func apply(value u32, transform func(u32) u32) u32",
	} {
		if !strings.Contains(optimizedSource, signature) {
			t.Fatalf("optimized source lost %q:\n%s", signature, optimizedSource)
		}
	}
	if _, err := NewRuntime().CheckProgram(context.Background(), optimized); err != nil {
		t.Fatalf("optimized typed program: %v", err)
	}
}

func TestTypedFunctionSignatureSyntaxDiagnostics(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{"func bad(value: u32) u32 => value\n", "expected )"},
		{"let callback func(u32) = missing\n", "function type requires a return type"},
		{"func bad(value u32) func(u32) => value\n", "function type requires a return type"},
		{"func bad(value Unknown) u32 => value\n", "unknown type"},
	}
	for _, test := range tests {
		if _, err := Parse("<typed-syntax>", test.source); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("Parse(%q) error = %v, want %q", test.source, err, test.want)
		}
	}
}

func TestTypedFunctionCheckerRejectsKnownMismatchesBeforeExecution(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
		line   int
	}{
		{
			name:   "argument width",
			source: "func accept(value u32) u32 => value\nemit accept(u64(3))\n",
			want:   "argument 1 to accept requires u32, got u64",
			line:   2,
		},
		{
			name:   "expression return",
			source: "func bad(value u32) string => value\nemit \"unreachable\"\n",
			want:   "function bad returns u32, requires string",
			line:   1,
		},
		{
			name:   "explicit return",
			source: "func bad(value u32) string { return value }\nemit \"unreachable\"\n",
			want:   "function bad returns u32, requires string",
			line:   1,
		},
		{
			name:   "fallthrough",
			source: "func bad(value u32) u32 { let copy = value }\nemit \"unreachable\"\n",
			want:   "function bad can fall through with null, requires u32",
			line:   1,
		},
		{
			name:   "callback",
			source: "func use(value u32, callback func(u32) string) string => callback(value)\nfunc wrong(value u32) u32 => value\nemit use(u32(1), wrong)\n",
			want:   "argument 2 to use requires func(u32) string, got func(u32) u32",
			line:   3,
		},
		{
			name:   "known branch return",
			source: "func bad(flag bool) u32 => if(flag, u32(1), \"bad\")\nemit \"unreachable\"\n",
			want:   "function bad returns string, requires u32",
			line:   1,
		},
		{
			name:   "result and option are distinct",
			source: "func accept(value result) result => value\nemit accept(none())\n",
			want:   "argument 1 to accept requires result, got option",
			line:   2,
		},
		{
			name:   "null is not nullable option",
			source: "func accept(value null) null => value\nemit accept(none())\n",
			want:   "argument 1 to accept requires null, got option",
			line:   2,
		},
		{
			name:   "generic placeholder",
			source: "func identity(value T) T => value\n",
			want:   "unknown type",
			line:   1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prog, err := Parse("<typed-error>", test.source)
			if err != nil {
				if !strings.Contains(err.Error(), test.want) {
					t.Fatalf("Parse error = %v, want %q", err, test.want)
				}
				return
			}
			outputs, err := NewRuntime().Run(context.Background(), prog)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Run error = %v, want %q", err, test.want)
			}
			if len(outputs) != 0 {
				t.Fatalf("outputs = %v, want checker failure before execution", outputs)
			}
			if !strings.Contains(err.Error(), "<typed-error>:"+itoa(test.line)+":") {
				t.Fatalf("error = %v, want source line %d", err, test.line)
			}
		})
	}
}

func TestTypedFunctionContextFitsEveryDefaultIntegerOutcome(t *testing.T) {
	prog, err := Parse("<typed-context>", `func choose(flag bool) u32 => if(flag, 1, match flag { true => 2, false => 3 })
emit choose(false)
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range []struct {
		name string
		call func() ([]Value, error)
	}{
		{"interpreter", func() ([]Value, error) { return NewRuntime().Run(context.Background(), prog) }},
		{"vm", func() ([]Value, error) { return NewRuntime().RunVM(context.Background(), prog) }},
	} {
		t.Run(run.name, func(t *testing.T) {
			outputs, err := run.call()
			if err != nil {
				t.Fatal(err)
			}
			if got, want := valuesOutput(outputs), "3u32\n"; got != want {
				t.Fatalf("output = %q, want %q", got, want)
			}
		})
	}
}

func TestTypedFunctionCanonicalContainerAndRuntimeKinds(t *testing.T) {
	prog, err := Parse("<typed-kinds>", `func keep_result(value result) result => value
func keep_option(value option) option => value
func keep_task(value task) task => value
func keep_null(value null) null => value
func keep_tensor(value tensor) tensor => value
func keep_enum(value enum) enum => value
`)
	if err != nil {
		t.Fatal(err)
	}
	checked, err := NewRuntime().CheckProgram(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"func(result) result",
		"func(option) option",
		"func(task) task",
		"func(null) null",
		"func(tensor) tensor",
		"func(enum) enum",
	}
	if len(checked.Functions) != len(want) {
		t.Fatalf("checked functions = %#v", checked.Functions)
	}
	for i, signature := range want {
		if got := checked.Functions[i].Signature; got != signature {
			t.Errorf("signature %d = %q, want %q", i, got, signature)
		}
	}
}

func TestTypedFunctionNestedClosureAndShadowing(t *testing.T) {
	prog, err := Parse("<typed-closure>", `let offset string = "outer"
func make_adder(offset u32) func(u32) u32 {
  func add(value u32) u32 => value + offset
  add
}
let add_two func(u32) u32 = make_adder(u32(2))
emit add_two(u32(40))
`)
	if err != nil {
		t.Fatal(err)
	}
	checked, err := NewRuntime().CheckProgram(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if len(checked.Functions) != 2 ||
		checked.Functions[0].Signature != "func(u32) func(u32) u32" ||
		checked.Functions[1].Signature != "func(u32) u32" {
		t.Fatalf("checked functions = %#v", checked.Functions)
	}
	interpreter, err := NewRuntime().Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := NewRuntime().RunVM(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := valuesOutput(interpreter), "42u32\n"; got != want {
		t.Fatalf("interpreter output = %q, want %q", got, want)
	}
	if got, want := valuesOutput(vm), valuesOutput(interpreter); got != want {
		t.Fatalf("VM output = %q, want %q", got, want)
	}
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "typed-closure")
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(executable).CombinedOutput()
	if err != nil {
		t.Fatalf("native run: %v\n%s", err, output)
	}
	if got, want := string(output), valuesOutput(interpreter); got != want {
		t.Fatalf("native output = %q, want %q", got, want)
	}
}

func TestTypedFunctionDiagnosticHasArgumentSpan(t *testing.T) {
	prog, err := Parse("<typed-span>", "func accept(value u32) u32 => value\nemit accept(u64(3))\n")
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewRuntime().CheckProgram(context.Background(), prog)
	typed, ok := err.(*OsirisError)
	if !ok || typed.location == nil {
		t.Fatalf("error = %#v, want located OsirisError", err)
	}
	if got, want := typed.location.Span.start, (sourcePos{line: 2, col: 13}); got != want {
		t.Fatalf("diagnostic start = %#v, want %#v", got, want)
	}
}

func TestTypedFunctionRecursionCallbacksAndBackendParity(t *testing.T) {
	source := `func countdown(value u32) u32 => if(value == u32(0), u32(0), countdown(value - u32(1)))
func apply(value u32, callback func(u32) u32) u32 => callback(value)
let callback func(u32) u32 = countdown
emit apply(u32(10), callback)
`
	prog, err := Parse("<typed-parity>", source)
	if err != nil {
		t.Fatal(err)
	}
	checked, err := NewRuntime().CheckProgram(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := checked.Functions[1].Signature, "func(u32, func(u32) u32) u32"; got != want {
		t.Fatalf("signature = %q, want %q", got, want)
	}

	interpreter, err := NewRuntime().Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := NewRuntime().RunVM(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := valuesOutput(vm), valuesOutput(interpreter); got != want {
		t.Fatalf("VM output = %q, want %q", got, want)
	}

	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(artifact.Source, "countdown") || !strings.Contains(artifact.Source, "apply") {
		t.Fatalf("generated C omitted typed functions")
	}
	executable := filepath.Join(t.TempDir(), "typed-parity")
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(executable).CombinedOutput()
	if err != nil {
		t.Fatalf("native run: %v\n%s", err, output)
	}
	if got, want := string(output), valuesOutput(interpreter); got != want {
		t.Fatalf("native output = %q, want %q", got, want)
	}
}

func TestTypedFunctionMutualRecursionIsCheckedWithoutExecution(t *testing.T) {
	source := `func even(value u32) bool => if(value == u32(0), true, odd(value - u32(1)))
func odd(value u32) bool => if(value == u32(0), false, even(value - u32(1)))
emit even(u32(10))
`
	prog, err := Parse("<typed-mutual>", source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRuntime().CheckProgram(context.Background(), prog); err != nil {
		t.Fatal(err)
	}
	interpreter, err := NewRuntime().Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := NewRuntime().RunVM(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := valuesOutput(vm), valuesOutput(interpreter); got != want {
		t.Fatalf("VM output = %q, want %q", got, want)
	}
}

func TestTypedFunctionIllTypedProgramIsRejectedByEveryArtifactPath(t *testing.T) {
	prog, err := Parse("<typed-artifact-error>", `func bad(value u32) string => value
emit bad(u32(1))
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	checks := []struct {
		name string
		run  func() error
	}{
		{"vm", func() error { _, err := rt.RunVM(context.Background(), prog); return err }},
		{"ir", func() error { _, err := rt.BuildIR(context.Background(), prog); return err }},
		{"native", func() error { _, err := CompileC(context.Background(), rt, prog); return err }},
		{"wasm", func() error { _, err := CompileWASM(context.Background(), rt, prog); return err }},
		{"gpu", func() error { _, err := CompileGPU(context.Background(), prog); return err }},
		{"infer", func() error { _, err := rt.InferProgram(context.Background(), prog); return err }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			err := check.run()
			if err == nil || !strings.Contains(err.Error(), "function bad returns u32, requires string") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestTypedFunctionCheckerRunsAfterMacroExpansionBeforeExecution(t *testing.T) {
	prog, err := Parse("<typed-macro>", `macro broken() => {
  func bad(value u32) string => value
  bad(u32(1))
}
emit "must not run"
emit broken()
`)
	if err != nil {
		t.Fatal(err)
	}
	outputs, err := NewRuntime().Run(context.Background(), prog)
	if err == nil || !strings.Contains(err.Error(), "returns u32, requires string") {
		t.Fatalf("error = %v", err)
	}
	if len(outputs) != 0 {
		t.Fatalf("outputs = %v, want expanded checker failure before execution", outputs)
	}
}

func TestTypedFunctionPrecheckPreservesIncrementalSessionDeclarations(t *testing.T) {
	session := NewSession(NewRuntime(), context.Background(), "<typed-session>")
	if _, err := session.ExecSource(`enum Status { Ready }
macro pass(value) => value
`); err != nil {
		t.Fatal(err)
	}
	outputs, err := session.ExecSource(`emit pass(match Status.Ready { Status.Ready => u32(1) })`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := valuesOutput(outputs), "1u32\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestTypedFunctionNativeRejectsUnsupportedRuntimeKindsBeforeEmission(t *testing.T) {
	prog, err := Parse("<typed-native-boundary>", `func keep(value option) option => value
emit keep(some(1))
`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRuntime().Run(context.Background(), prog); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRuntime().RunVM(context.Background(), prog); err != nil {
		t.Fatal(err)
	}
	_, err = CompileC(context.Background(), NewRuntime(), prog)
	if err == nil ||
		!strings.Contains(err.Error(), `<typed-native-boundary>:1:`) ||
		!strings.Contains(err.Error(), `type assertion "option" is not supported`) {
		t.Fatalf("native error = %v", err)
	}
}

func TestTypedFunctionMetadataFlowsThroughValueInferenceIRAndLSP(t *testing.T) {
	prog, err := Parse("<typed-metadata>", `func increment(value u32) u32 => value + u32(1)
emit increment
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	outputs, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if len(outputs) != 1 || outputs[0].FunctionType() != "func(u32) u32" {
		t.Fatalf("function metadata = %#v, type %q", outputs, outputs[0].FunctionType())
	}

	inferred, err := rt.InferProgram(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if len(inferred.Functions) != 1 {
		t.Fatalf("inferred functions = %#v", inferred.Functions)
	}
	fn := inferred.Functions[0]
	if fn.Signature != "func(u32) u32" || fn.Result != "u32" || len(fn.Params) != 1 || fn.Params[0].Kind != "u32" {
		t.Fatalf("inferred function = %#v", fn)
	}

	ir, err := rt.BuildIR(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	var irfn *IRFunction
	for _, candidate := range ir.Functions {
		if candidate.SourceName == "increment" {
			irfn = candidate
			break
		}
	}
	if irfn == nil || len(irfn.ParamTypes) != 1 || irfn.ParamTypes[0] != "u32" || irfn.ResultType != "u32" {
		t.Fatalf("IR function = %#v", irfn)
	}
	if rendered := ir.String(); !strings.Contains(rendered, "fn increment.fn0(value u32) u32") || strings.Count(rendered, "asserttype u32") < 2 {
		t.Fatalf("typed IR metadata/assertions missing:\n%s", rendered)
	}

	sourceMap := BuildSourceMap(prog)
	if len(sourceMap.Entries) == 0 ||
		sourceMap.Entries[0].Statement != "func increment(value u32) u32 => value + u32(1u64)" ||
		sourceMap.Entries[0].Range.End.Column <= sourceMap.Entries[0].Range.Start.Column {
		t.Fatalf("typed source map entry = %#v", sourceMap.Entries)
	}

	analysis := analyzeDocument(rt, "file:///typed-metadata.osr", prog)
	var detail string
	for _, def := range analysis.defs {
		if def.name == "increment" {
			detail = def.detail
			break
		}
	}
	if detail != "func increment(u32) u32" {
		t.Fatalf("LSP function detail = %q", detail)
	}

	bad, err := Parse("<typed-lsp>", "func bad(value u32) string => value\n")
	if err != nil {
		t.Fatal(err)
	}
	analysis = analyzeDocument(rt, "file:///typed-lsp.osr", bad)
	if len(analysis.diagnostics) == 0 || !strings.Contains(analysis.diagnostics[len(analysis.diagnostics)-1].Message, "function bad returns u32, requires string") {
		t.Fatalf("LSP diagnostics = %#v", analysis.diagnostics)
	}
}

func TestTypedFunctionLSPHoverAndMutualRecursionFixture(t *testing.T) {
	const source = `func even(value bool) bool => odd(value)
func odd(value bool) bool => even(value)
emit even(true)
`
	const path = "/tmp/typed-lsp-fixture.osr"
	prog, err := Parse(path, source)
	if err != nil {
		t.Fatal(err)
	}
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI(path)
	server.upsertDocument(uri, source)
	state := server.docs[uri]
	if state == nil {
		t.Fatal("typed LSP fixture was not opened")
	}
	for _, diagnostic := range state.diagnostics {
		if strings.Contains(diagnostic.Message, "undefined name") {
			t.Fatalf("mutual recursion diagnostic = %#v", diagnostic)
		}
	}
	emit := prog.Statements[2].(emitStmt)
	callee := emit.Expr.(*callExpr).callee.(*identExpr)
	hover, err := server.hover(uri, lspPosition{
		Line:      callee.Span.start.line - 1,
		Character: callee.Span.start.col - 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hover == nil || !strings.Contains(hover.Contents.Value, "func even(bool) bool") {
		t.Fatalf("typed function hover = %#v", hover)
	}
	paramUse := prog.Statements[0].(funcStmt).Body.(*callExpr).args[0].(*identExpr).Span
	hover, err = server.hover(uri, lspPosition{
		Line:      paramUse.start.line - 1,
		Character: paramUse.start.col - 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hover == nil || !strings.Contains(hover.Contents.Value, "function parameter bool") {
		t.Fatalf("typed parameter hover = %#v", hover)
	}
}

func TestTypedFunctionDynamicBoundaryKeepsRuntimeGuards(t *testing.T) {
	prog, err := Parse("<typed-dynamic-guard>", `func text_value(value u32) string => cat(value)
func numeric_value(value u32) u32 => value
let selected = if(false, text_value, numeric_value)
let callback func(u32) string = selected
emit callback(u32(1))
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range []struct {
		name string
		call func() ([]Value, error)
	}{
		{"interpreter", func() ([]Value, error) { return NewRuntime().Run(context.Background(), prog) }},
		{"vm", func() ([]Value, error) { return NewRuntime().RunVM(context.Background(), prog) }},
	} {
		t.Run(run.name, func(t *testing.T) {
			outputs, err := run.call()
			if err == nil || !strings.Contains(err.Error(), "requires func(u32) string, got func(u32) u32") {
				t.Fatalf("error = %v", err)
			}
			if len(outputs) != 0 {
				t.Fatalf("outputs = %v, want no execution past type guard", outputs)
			}
		})
	}

	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(artifact.Source, `strcmp(value.as.closure.signature,name)==0`) {
		t.Fatalf("generated C omitted exact function-signature guard")
	}
	executable := filepath.Join(t.TempDir(), "typed-dynamic-guard")
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(executable).CombinedOutput()
	if err == nil || !strings.Contains(string(output), "binding type assertion failed") {
		t.Fatalf("native output/error = %q, %v", output, err)
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	at := len(digits)
	for value > 0 {
		at--
		digits[at] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[at:])
}
