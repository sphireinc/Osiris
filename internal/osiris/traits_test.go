package osiris

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTraitsMethodsAndIterableParity(t *testing.T) {
	source := `trait Printable { func render() string; }
struct Point { value u64 }
impl Printable for Point { func render() string => "point" }
let point = Point { value: 3 }
let printable Printable = point
emit point.render()
emit printable.render()
for item in "go" { emit item }
`
	rt := NewRuntime()
	for _, run := range []struct {
		name string
		fn   func() ([]Value, error)
	}{
		{"interpreter", func() ([]Value, error) {
			return NewSession(rt, context.Background(), "traits.osr").ExecSource(source)
		}},
		{"vm-entry", func() ([]Value, error) {
			prog, err := Parse("traits.osr", source)
			if err != nil {
				return nil, err
			}
			return rt.RunVM(context.Background(), prog)
		}},
	} {
		t.Run(run.name, func(t *testing.T) {
			values, err := run.fn()
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, len(values))
			for i, value := range values {
				got[i] = value.String()
			}
			want := []string{`"point"`, `"point"`, `"g"`, `"o"`}
			if strings.Join(got, "|") != strings.Join(want, "|") {
				t.Fatalf("outputs %v, want %v", got, want)
			}
		})
	}
}

func TestTraitConformanceDiagnostics(t *testing.T) {
	source := `trait Renderable { func render() string; }
struct Point { value u64 }
impl Renderable for Point { func other() string => "wrong" }
`
	prog, err := Parse("traits.osr", source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRuntime().CheckProgram(context.Background(), prog); err == nil || !strings.Contains(err.Error(), "not declared") {
		t.Fatalf("error %v, want deterministic trait method diagnostic", err)
	}
}

func TestReceiverMethodsAndTraitGenericBounds(t *testing.T) {
	source := `trait Renderable { func render() string; }
struct Point { value u64 }
func (point Point) render() string => "receiver"
func show<T: Renderable>(value T) string => value.render()
let point = Point { value: 4 }
emit show< Point >(point)
`
	prog, err := Parse("receiver.osr", source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRuntime().CheckProgram(context.Background(), prog); err == nil {
		t.Fatal("unimplemented receiver method should not satisfy a nominal trait bound")
	}

	source = `trait Renderable { func render() string; }
struct Point { value u64 }
impl Renderable for Point { func render() string => "receiver" }
func show<T: Renderable>(value T) string => value.render()
let point = Point { value: 4 }
emit show<Point>(point)
`
	values, err := NewSession(NewRuntime(), context.Background(), "receiver.osr").ExecSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].String() != `"receiver"` {
		t.Fatalf("outputs %v, want receiver", values)
	}
}

func TestStandaloneReceiverMethodDispatch(t *testing.T) {
	source := `struct Point { value u64 }
func (point Point) render() string => "receiver"
let point = Point { value: 4 }
emit point.render()
`
	values, err := NewSession(NewRuntime(), context.Background(), "receiver.osr").ExecSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].String() != `"receiver"` {
		t.Fatalf("outputs %v, want receiver", values)
	}
}

func TestTraitImplementationSurvivesModuleLoad(t *testing.T) {
	dir := t.TempDir()
	modulePath := filepath.Join(dir, "shapes.osr")
	module := `package shapes 1.0.0
export point
trait Printable { func render() string; }
struct Point { value u64 }
impl Printable for Point { func render() string => "module-point" }
let point = Point { value: 1 }
`
	if err := os.WriteFile(modulePath, []byte(module), 0o644); err != nil {
		t.Fatal(err)
	}
	main := "import " + quoteString(modulePath) + " as shapes\nemit shapes.point.render()\n"
	values, err := NewSession(NewRuntime(), context.Background(), filepath.Join(dir, "main.osr")).ExecSource(main)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].String() != `"module-point"` {
		t.Fatalf("module method output %v", values)
	}
}

func TestUserIteratorProtocolIsLazyAndTerminates(t *testing.T) {
	source := `struct Cursor { value u64 }
struct Source { value u64 }
func (source Source) iter() => Cursor { value: source.value }
func (cursor Cursor) next() => none()
let source = Source { value: 1 }
for item in source { emit item }
`
	values, err := NewSession(NewRuntime(), context.Background(), "iterator.osr").ExecSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 0 {
		t.Fatalf("empty user iterator emitted %v", values)
	}
}

func TestTraitIRMetadataAndNativeBoundary(t *testing.T) {
	source := `trait Incrementable { func next() u64; }
impl Incrementable for u64 { func next() u64 => u64(2) }
emit (u64(1)).next()
let incrementable Incrementable = u64(1)
emit incrementable.next()
`
	prog, err := Parse("native-trait.osr", source)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	ir, err := rt.BuildIR(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, fn := range ir.Functions {
		if fn != nil && fn.ReceiverType == "u64" {
			found = true
		}
	}
	if !found {
		t.Fatalf("IR functions %v did not retain receiver metadata", len(ir.Functions))
	}
	interpreter, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatalf("interpreter witness dispatch: %v", err)
	}
	if got := valuesOutput(interpreter); got != "2u64\n2u64\n" {
		t.Fatalf("interpreter witness output = %q", got)
	}
	artifact, err := CompileC(context.Background(), rt, prog)
	if err != nil {
		t.Fatalf("native result %v", err)
	}
	if !strings.Contains(artifact.Source, "oregistermethod(\"u64\", \"next\"") || !strings.Contains(artifact.Source, "omethodtype") {
		t.Fatalf("native artifact does not contain witness registration:\n%s", artifact.Source)
	}
	executable := filepath.Join(t.TempDir(), "native-trait")
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatalf("native witness artifact does not compile: %v", err)
	}
	output, err := exec.Command(executable).CombinedOutput()
	if err != nil {
		t.Fatalf("native witness executable: %v\n%s", err, output)
	}
	if got := string(output); got != "2u64\n2u64\n" {
		t.Fatalf("native witness output = %q, want %q", got, "2u64\\n2u64\\n")
	}
}
