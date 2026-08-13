package osiris

import (
	"context"
	"strings"
	"testing"
)

func TestFrontendStagesExpandOnceResolveAndTypeBeforeBackends(t *testing.T) {
	parsed, err := ParseSurface("<frontend-stages>", `
macro twice(x) => x + x
let answer u32 = twice(u32(21))
emit answer
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	typed, err := rt.Frontend(context.Background(), parsed)
	if err != nil {
		t.Fatal(err)
	}
	if typed.Checked() == nil {
		t.Fatal("frontend did not retain checked result")
	}
	if got := len(typed.resolved.Symbols); got < 2 {
		t.Fatalf("symbols = %d, want macro and binding identities", got)
	}
	for index, symbol := range typed.resolved.Symbols {
		if symbol.ID != SymbolID(index+1) {
			t.Fatalf("symbol %q ID = %d, want %d", symbol.Name, symbol.ID, index+1)
		}
	}
	if len(typed.Checked().Bindings) == 0 || typed.Checked().Bindings[0].Symbol == 0 {
		t.Fatal("checked binding did not retain its resolved SymbolID")
	}
	if len(typed.resolved.References) == 0 {
		t.Fatal("resolver did not assign identifier-reference identities")
	}
	if strings.Contains(FormatProgram(typed.resolved.expanded.program), "twice(") {
		t.Fatal("macro invocation survived the single expansion phase")
	}
	vm, err := rt.RunVMFromTyped(context.Background(), typed)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := valuesOutput(vm), "42u32\n"; got != want {
		t.Fatalf("VM output = %q, want %q", got, want)
	}
	ir, err := rt.BuildIRFromTyped(context.Background(), typed)
	if err != nil {
		t.Fatal(err)
	}
	if ir.SourceName != "<frontend-stages>" {
		t.Fatalf("IR source = %q", ir.SourceName)
	}
	artifact, err := CompileC(context.Background(), rt, parsed.program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "twice(") {
		t.Fatal("native backend did not receive the frontend-expanded program")
	}
}

func TestFrontendReportsMacroGeneratedTypeErrorAfterExpansion(t *testing.T) {
	parsed, err := ParseSurface("<frontend-macro-error>", `
macro wrong() => "not a u32"
let value u32 = wrong()
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewRuntime().Frontend(context.Background(), parsed)
	if err == nil || !strings.Contains(err.Error(), "binding type u32") {
		t.Fatalf("frontend error = %v, want post-expansion type diagnostic", err)
	}
}

func TestLSPDeclarationUsesFrontendSymbolIdentityAndType(t *testing.T) {
	program, err := Parse("<frontend-lsp>", "let answer u32 = u32(42)\nemit answer\n")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	typed, err := rt.frontendProgram(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	analysis := analyzeDocumentContext(context.Background(), rt, "file:///frontend-lsp.osr", program)
	var definition *symbolDef
	for _, def := range analysis.defs {
		if def.name == "answer" {
			definition = def
			break
		}
	}
	if definition == nil || definition.symbolID == 0 || definition.typeName != "u32" {
		t.Fatalf("LSP definition = %#v, want frontend SymbolID and u32 type", definition)
	}
	if definition.symbolID != typed.Checked().Bindings[0].Symbol {
		t.Fatalf("LSP SymbolID = %d, compiler SymbolID = %d", definition.symbolID, typed.Checked().Bindings[0].Symbol)
	}
}
