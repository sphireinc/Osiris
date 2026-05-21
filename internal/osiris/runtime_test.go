package osiris

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseAndPrecedence(t *testing.T) {
	prog, err := Parse("<test>", `emit 1 + 2 * 3`)
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
	if got, want := out[0].String(), "7"; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}
}

func TestLexicalShadowing(t *testing.T) {
	prog, err := Parse("<test>", `
let a = "moon"
emit a
let a = "sun"
emit a
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 2; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), `"moon"`; got != want {
		t.Fatalf("output[0] = %s, want %s", got, want)
	}
	if got, want := out[1].String(), `"sun"`; got != want {
		t.Fatalf("output[1] = %s, want %s", got, want)
	}
}

func TestStructuralMemoization(t *testing.T) {
	prog, err := Parse("<test>", `
let a = probe("x", "y")
let b = probe("x", "y")
emit cat(a, b)
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	rt := NewRuntime()
	var calls int32
	rt.Builtins["probe"] = func(args []Value) (Value, error) {
		atomic.AddInt32(&calls, 1)
		return stringValue(textify(args[0]) + ":" + textify(args[1])), nil
	}

	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := atomic.LoadInt32(&calls), int32(1); got != want {
		t.Fatalf("probe calls = %d, want %d", got, want)
	}
	if got, want := len(out), 1; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), `"x:yx:y"`; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}
}

func TestStdlibImportAndMemberCall(t *testing.T) {
	prog, err := Parse("<test>", `
import "std/text" as text
emit text.slug("Hello, Weird World")
`)
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
	if got, want := out[0].String(), `"hello-weird-world"`; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}
}

func TestSessionPersistsState(t *testing.T) {
	rt := NewRuntime()
	sess := NewSession(rt, context.Background(), "<repl>")
	if out, err := sess.ExecSource(`let x = 3`); err != nil {
		t.Fatalf("let failed: %v", err)
	} else if len(out) != 0 {
		t.Fatalf("let produced outputs: %v", out)
	}
	out, err := sess.ExecSource(`emit x + 4`)
	if err != nil {
		t.Fatalf("emit failed: %v", err)
	}
	if got, want := len(out), 1; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), "7"; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}
}

func TestBlockExpression(t *testing.T) {
	prog, err := Parse("<test>", `
emit { let x = 2; let y = 3; x + y }
`)
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
	if got, want := out[0].String(), "5"; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}
}

func TestHygienicMacroExpansion(t *testing.T) {
	prog, err := Parse("<test>", `
let suffix = "!"
macro shout(x) => cat(x, suffix)
let suffix = "?"
emit shout("go")
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter run failed: %v", err)
	}
	got, err := rt.RunVM(context.Background(), prog)
	if err != nil {
		t.Fatalf("vm run failed: %v", err)
	}
	if gotS, wantS := len(got), len(want); gotS != wantS {
		t.Fatalf("len(got) = %d, want %d", gotS, wantS)
	}
	for i := range want {
		if got[i].String() != want[i].String() {
			t.Fatalf("output[%d] = %s, want %s", i, got[i].String(), want[i].String())
		}
	}
	if got := want[0].String(); got != `"go!"` {
		t.Fatalf("output = %s, want %s", got, `"go!"`)
	}
}

func TestFormatSource(t *testing.T) {
	formatted, err := FormatSource("<test>", `package demo; export alpha, beta; import "std/text" as text; let x=text.slug("Hello, World"); emit text.upper(x)`)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	const want = "package demo\nexport alpha, beta\nimport \"std/text\" as text\nlet x = text.slug(\"Hello, World\")\nemit text.upper(x)\n"
	if formatted != want {
		t.Fatalf("formatted = %q, want %q", formatted, want)
	}
}

func TestFormatMacrosAndBlocks(t *testing.T) {
	formatted, err := FormatSource("<test>", `macro shout(x)=>cat(x,suffix); emit { let x=2; x+1 }`)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	const want = "macro shout(x) => cat(x, suffix)\nemit { let x = 2u64; x + 1u64 }\n"
	if formatted != want {
		t.Fatalf("formatted = %q, want %q", formatted, want)
	}
}

func TestModuleExportsAreExplicit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sigil.osr")
	source := `
package sigil
let hidden = "secret"
let visible = slug("Hello, World")
export visible
`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("write module: %v", err)
	}

	prog, err := Parse("<test>", "import "+quoteString(path)+" as sigil\nemit sigil.visible\n")
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
	if got, want := out[0].String(), `"hello-world"`; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}

	prog, err = Parse("<test>", "import "+quoteString(path)+" as sigil\nemit sigil.hidden\n")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if _, err := rt.Run(context.Background(), prog); err == nil {
		t.Fatalf("expected hidden export access to fail")
	}
}

func TestPackageVersioning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gizmo.osr")
	source := `
package gizmo 2.1.0
export answer
let answer = 42
`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("write module: %v", err)
	}

	mainPath := filepath.Join(dir, "main.osr")
	prog, err := Parse(mainPath, "import "+quoteString(path+"@2.1.0")+" as gizmo\nimport \"std/text@1\" as text\nemit gizmo.answer\nemit text.slug(\"Versioned Text\")\n")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 2; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), "42u64"; got != want {
		t.Fatalf("output[0] = %s, want %s", got, want)
	}
	if got, want := out[1].String(), `"versioned-text"`; got != want {
		t.Fatalf("output[1] = %s, want %s", got, want)
	}

	lockPath := filepath.Join(dir, ".osiris.lock")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read lockfile: %v", err)
	}
	if !strings.Contains(string(data), `"package": "gizmo"`) || !strings.Contains(string(data), `"version": "2.1.0"`) {
		t.Fatalf("lockfile missing package metadata: %s", string(data))
	}

	bad, err := Parse(mainPath, "import "+quoteString(path+"@2.1.1")+" as gizmo\nemit gizmo.answer\n")
	if err != nil {
		t.Fatalf("parse mismatch import failed: %v", err)
	}
	if _, err := rt.Run(context.Background(), bad); err == nil {
		t.Fatalf("expected mismatched version import to fail")
	}
}

func TestBitwiseAndByteHelpers(t *testing.T) {
	prog, err := Parse("<test>", `
emit 0xff & 0b1010
emit rotl(u8(1), 1)
emit hex("A")
emit pack(bytes("Hi"))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 4; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), "10u64"; got != want {
		t.Fatalf("output[0] = %s, want %s", got, want)
	}
	if got, want := out[1].String(), "2u8"; got != want {
		t.Fatalf("output[1] = %s, want %s", got, want)
	}
	if got, want := out[2].String(), `"41"`; got != want {
		t.Fatalf("output[2] = %s, want %s", got, want)
	}
	if got, want := out[3].String(), `"Hi"`; got != want {
		t.Fatalf("output[3] = %s, want %s", got, want)
	}
}

func TestBinaryCodecRoundTrip(t *testing.T) {
	prog, err := Parse("<test>", `
emit decode(encode([1, u8(2), "go"]))
`)
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
	if got, want := out[0].String(), `[1u64, 2u8, "go"]`; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}
}

func TestMatchExpression(t *testing.T) {
	prog, err := Parse("<test>", `
emit match [1, 2] { [a, b] => a + b; _ => 0 }
emit match [7, 7] { [x, x] => x; _ => 0 }
emit match 9 { 1 => "nope"; _ => "fallback" }
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 3; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), "3"; got != want {
		t.Fatalf("output[0] = %s, want %s", got, want)
	}
	if got, want := out[1].String(), "7u64"; got != want {
		t.Fatalf("output[1] = %s, want %s", got, want)
	}
	if got, want := out[2].String(), `"fallback"`; got != want {
		t.Fatalf("output[2] = %s, want %s", got, want)
	}
}

func TestVizProgramDOT(t *testing.T) {
	prog, err := Parse("<test>", `emit match [1, 2] { [a, b] => a + b; _ => 0 }`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	dot := VizProgramDOT(prog)
	if !strings.Contains(dot, "digraph osiris-ast") {
		t.Fatalf("dot missing graph header: %s", dot)
	}
	if !strings.Contains(dot, "match") || !strings.Contains(dot, "binary +") {
		t.Fatalf("dot missing match structure: %s", dot)
	}
}

func TestVizImportsDOT(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "child.osr")
	parent := filepath.Join(dir, "parent.osr")
	if err := os.WriteFile(child, []byte(`package child
export value
let value = 7
`), 0o644); err != nil {
		t.Fatalf("write child: %v", err)
	}
	if err := os.WriteFile(parent, []byte(`import "./child.osr" as child
emit child.value
`), 0o644); err != nil {
		t.Fatalf("write parent: %v", err)
	}
	src, err := os.ReadFile(parent)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	dot, err := VizImportsDOT(parent, string(src))
	if err != nil {
		t.Fatalf("viz imports: %v", err)
	}
	if !strings.Contains(dot, "digraph osiris-imports") {
		t.Fatalf("dot missing graph header: %s", dot)
	}
	if !strings.Contains(dot, "child.osr") {
		t.Fatalf("dot missing child import: %s", dot)
	}
}

func TestDataPipelineBuiltins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "payload.txt")
	prog, err := Parse("<test>", `
emit write(PATH, "alpha\nbeta")
emit read(PATH)
emit lines(read(PATH))
emit csv("a,b\n1,2")
emit jsonparse("{\"x\":1,\"y\":[true,false]}")
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	replaced := strings.ReplaceAll(strings.ReplaceAll(FormatProgram(prog), "PATH", quoteString(path)), "\"alpha\\nbeta\"", "\"alpha\\nbeta\"")
	prog, err = Parse("<test>", replaced)
	if err != nil {
		t.Fatalf("reparse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 5; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), "true"; got != want {
		t.Fatalf("output[0] = %s, want %s", got, want)
	}
	if got, want := out[1].String(), `"alpha\nbeta"`; got != want {
		t.Fatalf("output[1] = %s, want %s", got, want)
	}
	if got, want := out[2].String(), `["alpha", "beta"]`; got != want {
		t.Fatalf("output[2] = %s, want %s", got, want)
	}
	if got, want := out[3].String(), `[["a", "b"], ["1", "2"]]`; got != want {
		t.Fatalf("output[3] = %s, want %s", got, want)
	}
	if got, want := out[4].String(), `[["x", 1u64], ["y", [true, false]]]`; got != want {
		t.Fatalf("output[4] = %s, want %s", got, want)
	}
}

func TestWASMBackendSnapshot(t *testing.T) {
	prog, err := Parse("<test>", `
let seed = "osiris"
emit seed
emit 7 + 3
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	artifact, err := CompileWASM(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("compile wasm failed: %v", err)
	}
	if !strings.Contains(artifact.WAT, "(module") || !strings.Contains(artifact.WAT, "result_len") {
		t.Fatalf("unexpected wat: %s", artifact.WAT)
	}
	if !strings.Contains(artifact.WAT, `"[\"osiris\",10]"`) {
		t.Fatalf("wat missing payload: %s", artifact.WAT)
	}
	if len(artifact.Binary) < 8 || !bytes.Equal(artifact.Binary[:8], []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}) {
		t.Fatalf("unexpected wasm header: %x", artifact.Binary[:8])
	}
	if artifact.JSON != `["osiris",10]` {
		t.Fatalf("json = %s, want %s", artifact.JSON, `["osiris",10]`)
	}
}

func TestBytecodeVMParity(t *testing.T) {
	prog, err := Parse("<test>", `
import "std/text" as text
let seed = "osiris"
emit text.slug(seed + " engine")
emit band(u8(7), 3)
emit match [1, 2] { [a, b] => a + b; _ => 0 }
emit decode(encode([1u8, 2u16, "go"]))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	want, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter run failed: %v", err)
	}
	got, err := rt.RunVM(context.Background(), prog)
	if err != nil {
		t.Fatalf("vm run failed: %v", err)
	}
	if gotS, wantS := len(got), len(want); gotS != wantS {
		t.Fatalf("len(got) = %d, want %d", gotS, wantS)
	}
	for i := range want {
		if got[i].String() != want[i].String() {
			t.Fatalf("output[%d] = %s, want %s", i, got[i].String(), want[i].String())
		}
	}
}

func TestDeterministicSchedulerOrder(t *testing.T) {
	prog, err := Parse("<test>", `
emit [tap("a"), tap("b"), tap("c")]
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	var mu sync.Mutex
	var calls []string
	rt.Builtins["tap"] = func(args []Value) (Value, error) {
		label, err := args[0].AsString()
		if err != nil {
			return nullValue(), err
		}
		switch label {
		case "a":
			time.Sleep(20 * time.Millisecond)
		case "b":
			time.Sleep(5 * time.Millisecond)
		}
		mu.Lock()
		calls = append(calls, label)
		mu.Unlock()
		return stringValue(label), nil
	}
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got, want := len(out), 1; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), `["a", "b", "c"]`; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}
	if got, want := strings.Join(calls, ""), "abc"; got != want {
		t.Fatalf("call order = %s, want %s", got, want)
	}
}

func TestModuleRegistryCacheFallback(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "atom.osr")
	root := filepath.Join(dir, "main.osr")
	if err := os.WriteFile(child, []byte(`package atom
export value
let value = 13
`), 0o644); err != nil {
		t.Fatalf("write child: %v", err)
	}
	prog, err := Parse(root, `import "./atom.osr" as atom
emit atom.value
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rt := NewRuntime()
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("initial run failed: %v", err)
	}
	if got, want := len(out), 1; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), "13u64"; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}
	lockPath := filepath.Join(dir, ".osiris.lock")
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lockfile missing: %v", err)
	}
	cacheDir := filepath.Join(dir, ".osiris", "cache", "modules")
	if entries, err := os.ReadDir(cacheDir); err != nil {
		t.Fatalf("cache dir missing: %v", err)
	} else if len(entries) == 0 {
		t.Fatalf("cache dir empty")
	}
	if err := os.Remove(child); err != nil {
		t.Fatalf("remove child: %v", err)
	}
	rt2 := NewRuntime()
	out, err = rt2.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("cached run failed: %v", err)
	}
	if got, want := len(out), 1; got != want {
		t.Fatalf("len(out) = %d, want %d", got, want)
	}
	if got, want := out[0].String(), "13u64"; got != want {
		t.Fatalf("cached output = %s, want %s", got, want)
	}
}
