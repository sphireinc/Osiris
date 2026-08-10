package osiris

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeResultOptionErrorConstructorContract(t *testing.T) {
	program, err := Parse("<native-result-option-error-constructors>", `let failure = error("invalid_argument", "bad")
let successful = ok(41u64)
let failed = err(failure)
let present = some(41u64)
let absent = none()
emit decode(encode(failure)), decode(encode(successful)), decode(encode(failed)), decode(encode(present)), decode(encode(absent))
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"oerror(", "oresultok(", "oresultfail(", "ooptionSome(",
		"ooptionNone(", "oencode(", "odecode(", "oresultbox(",
		"ooptionbox(", "otrack(",
	} {
		if !strings.Contains(artifact.Source, marker) {
			t.Fatalf("generated constructor/codec artifact omitted %q", marker)
		}
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") || strings.Contains(artifact.Source, "ostd_dispatch(\"std/") {
		t.Fatal("pure constructor/codec surface introduced a host capability or standard-library dispatch")
	}

	for _, test := range []struct {
		name   string
		source string
		want   string
	}{
		{name: "error arity", source: `emit error("invalid_argument")`, want: "error expects 2 arguments"},
		{name: "ok arity", source: `emit ok()`, want: "ok expects 1 argument"},
		{name: "err arity", source: `emit err()`, want: "err expects 1 argument"},
		{name: "some arity", source: `emit some()`, want: "some expects 1 argument"},
		{name: "none arity", source: `emit none(1u64)`, want: "none expects 0 arguments"},
		{name: "encode arity", source: `emit encode()`, want: "encode expects 1 argument"},
		{name: "decode arity", source: `emit decode()`, want: "decode expects 1 argument"},
	} {
		t.Run(test.name, func(t *testing.T) {
			program, err := Parse("<native-result-option-error-arity>", test.source)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := CompileC(context.Background(), NewRuntime(), program); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("compile error = %v, want %q", err, test.want)
			}
		})
	}

	bad, err := Parse("<native-result-option-error-kind>", `emit error(1u64, "bad")`)
	if err != nil {
		t.Fatal(err)
	}
	badArtifact, err := CompileC(context.Background(), NewRuntime(), bad)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "native-result-option-error-kind")
	if err := CompileCExecutable(context.Background(), badArtifact.Source, executable); err != nil {
		t.Fatal(err)
	}
	output, runErr := exec.Command(executable).CombinedOutput()
	if runErr == nil || !strings.Contains(string(output), "error expects a kind and message string") {
		t.Fatalf("wrong-kind execution error = %v, output %q", runErr, output)
	}
}

func TestNativeResultOptionCombinatorAndTryParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "native-result-option-combinators-try",
		Source: `func double(value) => value * 2
func recover_error(err) => error("wrapped", err.message)
func next(value) => ok(value + 1)
func some_next(value) => some(value + 2)
func none_next(value) => none()
func return_text(err) => "text failure"
func fail_callback(value) => unwrap(none())
emit isok(ok(1u64))
emit iserr(err(error("invalid_argument", "bad")))
emit map(ok(5u64), double)
emit map(err(error("invalid_argument", "bad")), double)
emit map(err(error("invalid_argument", "bad")), fail_callback)
emit map_error(err(error("not_found", "missing")), recover_error)
emit map_error(err(error("not_found", "missing")), return_text)
emit map_error(ok(3u64), fail_callback)
emit flat_map(ok(8u64), next)
emit flat_map(ok(8u64), some_next)
emit flat_map(ok(8u64), none_next)
emit flat_map(err(error("invalid_argument", "bad")), next)
emit flat_map(err(error("invalid_argument", "bad")), fail_callback)
emit try { unwrap(err(error("invalid_argument", "bad"))) } catch err { [err.kind, err.message] }
emit try { let? value = none(); value } catch err { [err.kind, err.message] }
emit try { 7u64 } catch err { "unexpected" }
`,
	})
}

func TestNativeResultOptionCombinatorLoweringOwnershipAndNoFallback(t *testing.T) {
	program, err := Parse("<native-result-option-combinators>", `func double(value) => value * 2
func recover_error(err) => error("wrapped", err.message)
func next(value) => ok(value + 1)
func return_text(err) => "text failure"
emit map(ok(5u64), double)
emit map_error(err(error("not_found", "missing")), recover_error)
emit flat_map(ok(8u64), next)
emit isok(ok(1u64)), iserr(err(error("invalid_argument", "bad")))
emit try { unwrap(err(error("invalid_argument", "bad"))) } catch err { err.kind }
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"omapresult(", "omaperror(", "oflatmap(", "oresultbox(", "ooptionbox(",
		"otrack(", "otry(", "oresultbind(",
	} {
		if !strings.Contains(artifact.Source, marker) {
			t.Fatalf("generated combinator artifact omitted %q", marker)
		}
	}
	generatedMain := artifact.Source
	if start := strings.Index(generatedMain, "// Osiris native C lowering from"); start >= 0 {
		generatedMain = generatedMain[start:]
	}
	if strings.Contains(generatedMain, "ostd_dispatch(") || strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure Result/Option combinators unexpectedly introduced a host fallback or capability")
	}
}

func TestNativeResultOptionCombinatorMalformedAndCallbackErrors(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
		want   string
	}{
		{name: "map arity", source: `emit map(ok(1u64))`, want: "map expects result and function"},
		{name: "map error arity", source: `emit map_error(ok(1u64), 1u64, 2u64)`, want: "map_error expects result and function"},
		{name: "flat map arity", source: `emit flat_map(ok(1u64), 1u64, 2u64)`, want: "flat_map expects result and function"},
	} {
		t.Run(test.name, func(t *testing.T) {
			program, err := Parse("<native-result-option-combinator-arity>", test.source)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := CompileC(context.Background(), NewRuntime(), program); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("compile error = %v, want %q", err, test.want)
			}
		})
	}

	runDifferentialFixture(t, differentialFixture{
		Name: "native-result-option-combinator-callback-error",
		Source: `func bad(value) => unwrap(none())
func return_number(err) => 1u64
func bad_flat(value) => 1u64
emit try { map(ok(1u64), bad) } catch err { [err.kind, err.message] }
emit try { map_error(err(error("not_found", "missing")), return_number) } catch err { err.message }
emit try { flat_map(ok(1u64), bad_flat) } catch err { err.message }
emit try { map(1u64, bad_flat) } catch err { err.message }
`,
	})
}
