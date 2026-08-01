package osiris

import (
	"context"
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

func TestTraitIRMetadataAndNativeBoundary(t *testing.T) {
	source := `trait Incrementable { func next() u64; }
impl Incrementable for u64 { func next() u64 => 2u64 }
emit 1u64
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
	if _, err := CompileC(context.Background(), rt, prog); err == nil || !strings.Contains(err.Error(), "vtable ABI") {
		t.Fatalf("native result %v, want explicit dispatch boundary", err)
	}
}
