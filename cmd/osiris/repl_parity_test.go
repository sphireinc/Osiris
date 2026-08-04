package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// replTranscript executes the CLI loop with an in-memory input stream. It is
// intentionally separate from the runtime/session tests: the prompt protocol,
// input framing, and expression shortcut are part of the CLI contract.
func replTranscript(t *testing.T, source string) (string, error) {
	t.Helper()
	var output bytes.Buffer
	err := runReplWithInput(context.Background(), strings.NewReader(source+"quit\n"), &output)
	return output.String(), err
}

func stripREPLPrompts(output string) string {
	output = strings.ReplaceAll(output, "osiris> ", "")
	return strings.ReplaceAll(output, "....> ", "")
}

func runSourceFixture(t *testing.T, source string) (string, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "program.osr")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("write source fixture: %v", err)
	}
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), []string{"run", path}, &stdout, &stderr)
	if stderr.Len() != 0 {
		t.Logf("source stderr: %q", stderr.String())
	}
	return stdout.String(), err
}

func TestREPLConstLetVarMatchesSourceFile(t *testing.T) {
	const source = "const base = 2 + 3\n" +
		"let runtime = base + 1\n" +
		"var current = runtime\n" +
		"current = current + base\n" +
		"emit base, runtime, current\n"

	want, err := runSourceFixture(t, source)
	if err != nil {
		t.Fatalf("source fixture: %v", err)
	}
	got, err := replTranscript(t, source)
	if err != nil {
		t.Fatalf("REPL: %v; output=%q", err, got)
	}
	if got = stripREPLPrompts(got); got != want {
		t.Fatalf("REPL output = %q, source output = %q", got, want)
	}
}

func TestREPLDeclarationFormsMatchSourceFile(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{
			name: "function",
			source: "func double(value u64) u64 => value + 1u64\n" +
				"emit double(2u64)\n",
		},
		{
			name: "struct_type_newtype",
			source: "struct Point { value u64 }\n" +
				"type Count = u64\n" +
				"newtype PointID u64\n" +
				"let point = Point { value: Count(3u64) }\n" +
				"let id = PointID(7u64)\n" +
				"emit point.value, id\n",
		},
		{
			name: "enum",
			source: "enum Status { Ready, Done }\n" +
				"emit Status.Ready\n",
		},
		{
			name: "trait_interface_impl",
			source: "trait Renderable { func render() string; }\n" +
				"interface Named { func name() string; }\n" +
				"struct Point { value u64 }\n" +
				"impl Renderable for Point { func render() string => \"point\" }\n" +
				"impl Named for Point { func name() string => \"p\" }\n" +
				"let point = Point { value: 1 }\n" +
				"emit point.render(), point.name()\n",
		},
		{
			name: "macro",
			source: "macro twice(value) => value + value\n" +
				"emit twice(3u64)\n",
		},
		{
			name: "import",
			source: "import \"std/text\" as text\n" +
				"emit text.upper(\"hello\")\n",
		},
		{
			name: "package_export",
			source: "package demo 1.0.0\n" +
				"export answer\n" +
				"let answer = 42u64\n" +
				"emit answer\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want, err := runSourceFixture(t, tc.source)
			if err != nil {
				t.Fatalf("source fixture: %v", err)
			}
			got, err := replTranscript(t, tc.source)
			if err != nil {
				t.Fatalf("REPL: %v; output=%q", err, got)
			}
			if got = stripREPLPrompts(got); got != want {
				t.Fatalf("REPL output = %q, source output = %q", got, want)
			}
		})
	}
}

func TestREPLExpressionShortcutAndExplicitEmit(t *testing.T) {
	output, err := replTranscript(t, "1 + 2\nemit 4\n")
	if err != nil {
		t.Fatalf("REPL: %v; output=%q", err, output)
	}
	if got, want := stripREPLPrompts(output), "3u64\n4u64\n"; got != want {
		t.Fatalf("REPL values = %q, want %q", got, want)
	}
}

func TestREPLStatePersistsAcrossSubmissions(t *testing.T) {
	output, err := replTranscript(t, "let answer = 2\nemit answer + 3\n")
	if err != nil {
		t.Fatalf("REPL: %v; output=%q", err, output)
	}
	if got, want := stripREPLPrompts(output), "5u64\n"; got != want {
		t.Fatalf("REPL values = %q, want %q", got, want)
	}
}

func TestREPLMultilineSubmissionIsAtomic(t *testing.T) {
	output, err := replTranscript(t, "func add(value u64) u64 {\n"+
		"return value + 1u64\n"+
		"}\n"+
		"emit add(4u64)\n")
	if err != nil {
		t.Fatalf("REPL: %v; output=%q", err, output)
	}
	if got, want := stripREPLPrompts(output), "5u64\n"; got != want {
		t.Fatalf("REPL values = %q, want %q", got, want)
	}
}

func TestREPLIncompleteInputDoesNotPartiallyExecute(t *testing.T) {
	var output bytes.Buffer
	err := runReplWithInput(context.Background(), strings.NewReader("func broken(value u64) u64 {\nreturn value + 1u64\n"), &output)
	if err == nil || !strings.Contains(err.Error(), "incomplete input") {
		t.Fatalf("REPL error = %v, want deterministic incomplete-input error; output=%q", err, output.String())
	}
	if strings.Contains(output.String(), "broken") || strings.Contains(output.String(), "1u64") {
		t.Fatalf("incomplete declaration produced executable output: %q", output.String())
	}
}

func TestREPLDiagnosticsMatchSourceFile(t *testing.T) {
	const source = "const value = uint(1)\n"
	_, sourceErr := runSourceFixture(t, source)
	if sourceErr == nil {
		t.Fatal("source fixture unexpectedly succeeded")
	}

	output, replErr := replTranscript(t, source)
	if replErr != nil {
		t.Fatalf("REPL returned transport error: %v; output=%q", replErr, output)
	}
	message := `error[type_error]("const initializer must be statically evaluable")`
	if !strings.Contains(output, message) {
		t.Fatalf("REPL diagnostic lacks source diagnostic fragment %q; source=%q output=%q", message, sourceErr, output)
	}
	spanStart := strings.LastIndex(sourceErr.Error(), ":1:")
	if spanStart < 0 || !strings.Contains(output, sourceErr.Error()[spanStart:]) {
		t.Fatalf("REPL diagnostic lacks source span from source diagnostic; source=%q output=%q", sourceErr, output)
	}
}
