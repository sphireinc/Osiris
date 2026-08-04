package osiris

import (
	"context"
	"strings"
	"testing"
)

func TestSessionExecSourcePersistsMutableBindings(t *testing.T) {
	session := NewSession(NewRuntime(), context.Background(), "session-mutable.osr")
	if _, err := session.ExecSource("var total = 5\n"); err != nil {
		t.Fatalf("declare mutable binding: %v", err)
	}
	if _, err := session.ExecSource("total = total + 4\n"); err != nil {
		t.Fatalf("assign persisted mutable binding: %v", err)
	}
	values, err := session.ExecSource("emit total\n")
	if err != nil {
		t.Fatalf("read persisted mutable binding: %v", err)
	}
	if got := valuesOutput(values); got != "9u64\n" {
		t.Fatalf("persisted mutable binding = %q, want %q", got, "9u64\\n")
	}
}

func TestInterpreterConformanceAndDeterminism(t *testing.T) {
	tests := []struct {
		name          string
		source        string
		wantOutput    string
		errorContains string
	}{
		{
			name: "bindings-typed-generic-calls",
			source: `const seed = 2
func identity<T>(value T) T => value
var total = seed
total = total + 3
emit identity<u64>(total)
`,
			wantOutput: "5u64\n",
		},
		{
			name: "traits-interfaces-impls-and-iterators",
			source: `trait Printable { func render() string; }
struct Point { value u64 }
impl Printable for Point { func render() string => "point" }
let point = Point { value: 1 }
let printable Printable = point
emit printable.render()
for item in "go" { emit item }
`,
			wantOutput: "\"point\"\n\"g\"\n\"o\"\n",
		},
		{
			name: "results-options-and-patterns",
			source: `emit try { let? value = ok(41); value + 1 } catch err { err.kind }
emit match [2, 3] { [a, b] => a * b; _ => 0 }
emit try { let? missing = none(); missing } catch err { err.kind }
`,
			wantOutput: "42u64\n6u64\n\"not_found\"\n",
		},
		{
			name: "modules-macros-and-stdlib",
			source: `import "std/text" as text
let suffix = "!"
macro shout(value) => cat(text.upper(value), suffix)
emit shout("go")
`,
			wantOutput: "\"GO!\"\n",
		},
		{
			name: "tasks-cancellation-cleanup",
			source: `defer { emit "cleanup" }
let task = spawn({ 7 })
emit await(task)
`,
			wantOutput: "7u64\n\"cleanup\"\n",
		},
		{
			name: "committed-output-errors-and-source-spans",
			source: `emit "committed"
emit uint("bad")
`,
			wantOutput:    "\"committed\"\n",
			errorContains: "interpreter-committed-output-errors-and-source-spans.osr:2:6-2:17",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prog, err := Parse("interpreter-"+test.name+".osr", test.source)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			var firstOutput, firstError string
			for run := 0; run < 3; run++ {
				values, runErr := NewRuntime().Run(context.Background(), prog)
				output := valuesOutput(values)
				errorText := ""
				if runErr != nil {
					errorText = runErr.Error()
				}
				if run == 0 {
					firstOutput, firstError = output, errorText
				} else if output != firstOutput || errorText != firstError {
					t.Fatalf("run %d changed result: output=%q error=%q; first output=%q error=%q", run, output, errorText, firstOutput, firstError)
				}
				if test.wantOutput != output {
					t.Fatalf("output = %q, want %q", output, test.wantOutput)
				}
				if test.errorContains == "" && runErr != nil {
					t.Fatalf("run error: %v", runErr)
				}
				if test.errorContains != "" && (runErr == nil || !strings.Contains(errorText, test.errorContains)) {
					t.Fatalf("error = %q, want substring %q", errorText, test.errorContains)
				}
			}
		})
	}
}

func TestInterpreterBlockMutableBindingsUseCells(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "assignment", source: `emit { var total = 5; total = total + 4; total }`, want: "9u64\n"},
		{name: "loop", source: `emit { var total = 0; while total < 3 { total = total + 1 }; total }`, want: "3u64\n"},
		{name: "tuple-destructuring", source: `emit { let (left, right) = (2, 3); left + right }`, want: "5u64\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			values, err := NewSession(NewRuntime(), context.Background(), "block-mutable.osr").ExecSource(tc.source)
			if err != nil {
				t.Fatalf("execute block mutation: %v", err)
			}
			if got := valuesOutput(values); got != tc.want {
				t.Fatalf("block mutation = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSessionExecSourceCarriesInterpreterDeclarationsIntoLaterChecks(t *testing.T) {
	session := NewSession(NewRuntime(), context.Background(), "session-declarations.osr")
	if _, err := session.ExecSource("func add(value u64) u64 => value + 2\n"); err != nil {
		t.Fatalf("declare function: %v", err)
	}
	values, err := session.ExecSource("emit add(3u64)\n")
	if err != nil {
		t.Fatalf("call function from later source fragment: %v", err)
	}
	if got, want := valuesOutput(values), "5u64\n"; got != want {
		t.Fatalf("later function call = %q, want %q", got, want)
	}
}
