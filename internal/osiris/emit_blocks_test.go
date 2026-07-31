package osiris

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runEmitParity(t *testing.T, source string, want []string) {
	t.Helper()
	prog, err := Parse("<emit-test>", source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, runner := range []struct {
		name string
		run  func(context.Context, *Program) ([]Value, error)
	}{
		{name: "interpreter", run: NewRuntime().Run},
		{name: "vm", run: NewRuntime().RunVM},
	} {
		t.Run(runner.name, func(t *testing.T) {
			got, err := runner.run(context.Background(), prog)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			rendered := make([]string, len(got))
			for i := range got {
				rendered[i] = got[i].String()
			}
			if strings.Join(rendered, "|") != strings.Join(want, "|") {
				t.Fatalf("output = %q, want %q", rendered, want)
			}
		})
	}
}

func TestEmitSequentialBlockParity(t *testing.T) {
	runEmitParity(t, `
emit "top-before"
let seed = 2
emit seed

func announce(value) {
	emit value
	return value + 1
}

let called = announce(3)
emit called
let selected = if(true, { emit "selected"; 5 }, { emit "unselected"; 0 })
emit selected
emit match 2 { 1 => { emit "wrong"; 0 }; 2 => { emit "match"; 6 }; _ => 0 }
emit switch 9 { 9 => { emit "switch"; 7 }; _ => 0 }
emit try { emit "try"; uint("bad") } catch err { emit "catch"; 8 }
let i = 0
while i < 3 {
	emit i
	let i = i + 1
}
for item in [10, 11] {
	emit item
}
emit "top-after"
`, []string{
		`"top-before"`, "2u64", "3u64", "4u64", `"selected"`, "5u64",
		`"match"`, "6u64", `"switch"`, "7u64", `"try"`, `"catch"`,
		"8u64", "0u64", "1u64", "2u64", "10u64", "11u64", `"top-after"`,
	})
}

func TestEmitCommaFormIsOneOrderedValue(t *testing.T) {
	runEmitParity(t, `
emit "left", 3u32, "right"
emit "after"
`, []string{`"left, 3u32, right"`, `"after"`})
}

func TestEmitFlowControlClosureCaptureAndSingleEvaluation(t *testing.T) {
	runEmitParity(t, `
func early(value) {
	emit value
	return value + 1
	emit "unreachable"
}
let captured = 30
func inner(offset) {
	emit captured + offset
	return captured + offset + 1
}
func once() {
	emit "evaluated"
	return 1
}
let i = 0
while i < 5 {
	let i = i + 1
	emit if(i == 2, { continue }, i)
	emit if(i == 4, { break }, i + 10)
}
emit early(20)
emit inner(2)
emit once(), once()
`, []string{
		"1u64", "11u64", "3u64", "13u64", "4u64",
		"20u64", "21u64", "32u64", "33u64",
		`"evaluated"`, `"evaluated"`, `"1u64, 1u64"`,
	})
}

func TestEmitCommittedPrefixOnError(t *testing.T) {
	prog, err := Parse("prefix.osr", `
emit "committed"
emit uint("bad")
emit "unreachable"
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, runner := range []struct {
		name string
		run  func(context.Context, *Program) ([]Value, error)
	}{
		{name: "interpreter", run: NewRuntime().Run},
		{name: "vm", run: NewRuntime().RunVM},
	} {
		t.Run(runner.name, func(t *testing.T) {
			out, err := runner.run(context.Background(), prog)
			if err == nil {
				t.Fatal("run succeeded")
			}
			if got := valuesOutput(out); got != "\"committed\"\n" {
				t.Fatalf("committed output = %q", got)
			}
			if !strings.Contains(err.Error(), "prefix.osr:3:") {
				t.Fatalf("error lacks emit source span: %v", err)
			}
		})
	}
}

func TestEmitCommittedPrefixOnCancellation(t *testing.T) {
	prog, err := Parse("cancel-prefix.osr", `
emit "committed"
emit "cancelled-before-this"
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name        string
		cancelAfter int64
		run         func(context.Context, *Program) ([]Value, error)
	}{
		{name: "interpreter", cancelAfter: 4, run: NewRuntime().Run},
		{name: "vm", cancelAfter: 3, run: NewRuntime().RunVM},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &cancelAfterChecksContext{cancelAfter: tc.cancelAfter}
			out, err := tc.run(ctx, prog)
			if err == nil || !strings.Contains(err.Error(), "canceled") {
				t.Fatalf("out = %v, err = %v", out, err)
			}
			if got := valuesOutput(out); got != "\"committed\"\n" {
				t.Fatalf("committed output = %q", got)
			}
		})
	}
}

func TestEmitRejectedDuringConcurrentExecution(t *testing.T) {
	tests := map[string]string{
		"spawn-direct": `
let task = spawn({ emit "bad"; 1 })
emit await(task)
`,
		"spawn-indirect": `
func noisy() { emit "bad"; return 1 }
let task = spawn({ noisy() })
emit await(task)
`,
		"parallel": `emit parallel({ emit "bad"; 1 }, 2)`,
		"race":     `emit race({ emit "bad"; 1 }, 2)`,
		"select":   `emit select({ emit "bad"; 1 }, 2)`,
		"timeout":  `emit timeout(10, { emit "bad"; 1 })`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			prog, err := Parse(name+".osr", source)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			for _, run := range []func(context.Context, *Program) ([]Value, error){
				NewRuntime().Run,
				NewRuntime().RunVM,
			} {
				out, err := run(context.Background(), prog)
				if err == nil || !strings.Contains(err.Error(), "emit is not allowed during concurrent execution") {
					t.Fatalf("out = %v, err = %v", out, err)
				}
				if len(out) != 0 {
					t.Fatalf("concurrent output escaped: %v", out)
				}
			}
			if _, err := CompileC(context.Background(), NewRuntime(), prog); err == nil ||
				!strings.Contains(err.Error(), "emit is not allowed during concurrent execution") {
				t.Fatalf("native compile diagnostic = %v", err)
			}
		})
	}
}

func TestEmitAfterAwaitRemainsValid(t *testing.T) {
	runEmitParity(t, `
let task = spawn({ let value = 5; value + 7 })
emit await(task)
`, []string{"12u64"})
}

func TestEmitMacroBlockRoundTripAndExpansion(t *testing.T) {
	source := `
macro announce(value) => { emit value; value + 1 }
let result = announce(4)
emit result
`
	formatted, err := FormatSource("macro-emit.osr", source)
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	if _, err := Parse("macro-emit.osr", formatted); err != nil {
		t.Fatalf("parse formatted source: %v\n%s", err, formatted)
	}
	runEmitParity(t, source, []string{"4u64", "5u64"})
}

func TestEmitSupportedContextsFormatRoundTrip(t *testing.T) {
	sources := []string{
		`emit 1; let value = 2; emit value`,
		`while false { emit 1 }`,
		`for value in [] { emit value }`,
		`func show(value) { emit value; return value }`,
		`emit if(true, { emit 1; 2 }, { emit 3; 4 })`,
		`emit match 1 { 1 => { emit 2; 3 }; _ => 0 }`,
		`emit switch 1 { 1 => { emit 2; 3 }; _ => 0 }`,
		`emit try { emit 1; uint("bad") } catch err { emit err; 2 }`,
		`macro show(value) => { emit value; value }; emit show(1)`,
	}
	for i, source := range sources {
		formatted, err := FormatSource("emit-format.osr", source)
		if err != nil {
			t.Fatalf("case %d format: %v", i, err)
		}
		if _, err := Parse("emit-format.osr", formatted); err != nil {
			t.Fatalf("case %d parse formatted: %v\n%s", i, err, formatted)
		}
	}
}

func TestEmitNativeIRAndGeneratedCPlacement(t *testing.T) {
	source := `
func announce(value) {
	emit value
	return value + 1
}
let i = 0u32
while i < 3u32 {
	emit i
	let i = i + 1u32
}
emit announce(7u32)
`
	prog, err := Parse("native-emit.osr", source)
	if err != nil {
		t.Fatal(err)
	}
	ir, err := NewRuntime().BuildIR(context.Background(), prog)
	if err != nil {
		t.Fatalf("build IR: %v", err)
	}
	renderedIR := FormatIRProgram(ir)
	for _, fragment := range []string{"main.while.body", "emit", "fn announce"} {
		if !strings.Contains(renderedIR, fragment) {
			t.Fatalf("IR lacks %q:\n%s", fragment, renderedIR)
		}
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile C: %v", err)
	}
	for _, fragment := range []string{"block_main_while_body", "printf(", "announce"} {
		if !strings.Contains(artifact.Source, fragment) {
			t.Fatalf("generated C lacks %q", fragment)
		}
	}
	runDifferentialFixture(t, differentialFixture{Name: "emit-native-cfg", Source: source})
}

func TestEmitNativeNestedValueRenderingParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "emit-native-values",
		Source: `
enum Status { Ready }
let result = {
	emit 7u32
	emit 1.5f32
	emit 2.5f64
	emit "text"
	emit [1, 2]
	emit Status.Ready
	9
}
emit result
`,
	})
}

func TestEmitPurityAndOptimizationBarrier(t *testing.T) {
	source := `
func noisy(value) {
	emit value
	return value + 1
}
let result = noisy(4)
emit result
`
	prog, err := Parse("emit-purity.osr", source)
	if err != nil {
		t.Fatal(err)
	}
	report, err := NewRuntime().AnalyzePurity(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	rendered := FormatPurityProgram(report)
	for _, fragment := range []string{"fn noisy @ main params [value] impure", "main.result: function impure"} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("purity report lacks %q:\n%s", fragment, rendered)
		}
	}
	optimized := OptimizeProgram(prog)
	before, beforeErr := NewRuntime().Run(context.Background(), prog)
	after, afterErr := NewRuntime().Run(context.Background(), optimized)
	if beforeErr != nil || afterErr != nil || valuesOutput(before) != valuesOutput(after) {
		t.Fatalf("optimization crossed emit barrier: before=%q/%v after=%q/%v\n%s",
			valuesOutput(before), beforeErr, valuesOutput(after), afterErr, FormatProgram(optimized))
	}
	if got := strings.Count(FormatProgram(optimized), "emit "); got != 2 {
		t.Fatalf("optimized emit count = %d:\n%s", got, FormatProgram(optimized))
	}
}

func TestEmitModuleBoundary(t *testing.T) {
	dir := t.TempDir()
	modulePath := filepath.Join(dir, "speaker.osr")
	if err := os.WriteFile(modulePath, []byte(`
package speaker 1.0.0
export speak
func speak(value) {
	emit value
	return value + 1
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.osr")
	source := "import " + quoteString(modulePath) + " as speaker\nemit speaker.speak(9)\n"
	prog, err := Parse(mainPath, source)
	if err != nil {
		t.Fatal(err)
	}
	for _, runner := range []struct {
		name string
		run  func(context.Context, *Program) ([]Value, error)
	}{
		{name: "interpreter", run: NewRuntime().Run},
		{name: "vm", run: NewRuntime().RunVM},
	} {
		t.Run(runner.name, func(t *testing.T) {
			out, err := runner.run(context.Background(), prog)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if got := valuesOutput(out); got != "9u64\n10u64\n" {
				t.Fatalf("output = %q", got)
			}
		})
	}
	if _, err := CompileC(context.Background(), NewRuntime(), prog); err == nil || !strings.Contains(err.Error(), "static module") {
		t.Fatalf("native local-module boundary was not rejected explicitly: %v", err)
	}

	badModulePath := filepath.Join(dir, "noisy.osr")
	if err := os.WriteFile(badModulePath, []byte(`
package noisy 1.0.0
export answer
emit "forbidden"
let answer = 1
`), 0o644); err != nil {
		t.Fatal(err)
	}
	bad, err := Parse(mainPath, "import "+quoteString(badModulePath)+" as noisy\nemit noisy.answer\n")
	if err != nil {
		t.Fatal(err)
	}
	out, err := NewRuntime().Run(context.Background(), bad)
	if err == nil || !strings.Contains(err.Error(), "module initialization cannot emit") {
		t.Fatalf("out = %v, err = %v", out, err)
	}
	if len(out) != 0 {
		t.Fatalf("module initialization leaked output: %v", out)
	}
}

func TestEmitNativeCommittedPrefixBeforeRuntimeFailure(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	prog, err := Parse("native-prefix.osr", `
emit "committed"
emit 1 / 0
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("compile C: %v", err)
	}
	executable := filepath.Join(t.TempDir(), "native-prefix")
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatalf("compile executable: %v", err)
	}
	var stdout, stderr strings.Builder
	command := exec.Command(executable)
	command.Stdout = &stdout
	command.Stderr = &stderr
	err = command.Run()
	if err == nil {
		t.Fatalf("native execution succeeded: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if stdout.String() != "\"committed\"\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() == 0 {
		t.Fatal("native runtime failure wrote no diagnostic to stderr")
	}
}
