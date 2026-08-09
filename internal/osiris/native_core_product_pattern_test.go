package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestNativeCoreProductPatternParity proves that named-product patterns
// use the same nominal type and named-field contract in the interpreter, VM,
// generated C, and native executable. A pattern may constrain a subset of the
// fields; every constrained field is recursively matched and bound.
func TestNativeCoreProductPatternParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeCore-native-product-pattern",
		Source: `struct Point { x u64, y u64 }
struct Box { point Point }
let point = Point { x: 1u64, y: 9u64 }
let box = Box { point: point }
emit match point {
  Point { x: x, y: y } => x + y,
  _ => 0u64
}
emit match point {
  Point { x: 1u64 } => "x-only",
  _ => "wrong"
}
emit match point {
  Point { y: 9u64 } => "y-only",
  _ => "wrong"
}
emit match point {
  Point { x: 2u64 } => "wrong",
  Point { x: x } => x
}
emit match box {
  Box { point: Point { y: y } } => y,
  _ => 0u64
}
`,
	})
}

func TestNativeCoreProductPatternFormatAndClone(t *testing.T) {
	source := `struct Point { x u64, y u64 }
let point = Point { x: 1u64, y: 9u64 }
emit match point { Point { x: x, y: y } => x + y, _ => 0u64 }
`
	formatted, err := FormatSource("<nativeCore-product-pattern-format>", source)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(formatted, "Point { x: x, y: y } => x + y") {
		t.Fatalf("formatted product pattern = %q", formatted)
	}
	program, err := Parse("<nativeCore-product-pattern-clone>", source)
	if err != nil {
		t.Fatal(err)
	}
	if len(program.Statements) != 3 {
		t.Fatalf("parsed product-pattern statement count = %d", len(program.Statements))
	}
	clone := cloneStmt(program.Statements[2])
	if got := formatStmt(clone); !strings.Contains(got, "Point { x: x, y: y } => x + y") {
		t.Fatalf("cloned product pattern format = %q", got)
	}
}

func TestNativeCoreProductPatternIRContract(t *testing.T) {
	program, err := Parse("<nativeCore-product-pattern-ir>", `struct Point { x u64, y u64 }
let point = Point { x: 1u64, y: 9u64 }
emit match point { Point { x: x, y: y } => x + y, _ => 0u64 }
`)
	if err != nil {
		t.Fatal(err)
	}
	ir, err := NewRuntime().BuildIR(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	var foundMatch, foundField bool
	for _, fn := range ir.Functions {
		for _, block := range fn.Blocks {
			for _, instr := range block.Instrs {
				if instr.Op == "matchproduct" && instr.Text == "Point" && len(instr.FieldNames) == 2 && instr.FieldNames[0] == "x" && instr.FieldNames[1] == "y" {
					foundMatch = true
				}
				if instr.Op == "productfield" && (instr.Text == "x" || instr.Text == "y") && len(instr.Args) == 1 {
					foundField = true
				}
			}
		}
	}
	if !foundMatch || !foundField {
		t.Fatalf("product pattern IR = matchproduct %v, productfield %v", foundMatch, foundField)
	}
}

func TestNativeCoreProductPatternDiagnostic(t *testing.T) {
	program, err := Parse("nativeCore-native-product-pattern-error.osr", `struct Point { x u64, y u64 }
let point = Point { x: 1u64, y: 9u64 }
emit match point { Point { missing: _ } => 1u64, _ => 0u64 }
`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileC(context.Background(), NewRuntime(), program); err == nil ||
		!strings.Contains(err.Error(), "nativeCore-native-product-pattern-error.osr:3:") ||
		!strings.Contains(err.Error(), "type_error") ||
		!strings.Contains(err.Error(), "struct Point has no field") ||
		!strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing product-pattern field diagnostic = %v", err)
	}
	duplicate, err := Parse("nativeCore-native-product-pattern-duplicate.osr", `struct Point { x u64, y u64 }
let point = Point { x: 1u64, y: 9u64 }
emit match point { Point { x: _, x: _ } => 1u64, _ => 0u64 }
`)
	if duplicate != nil || err == nil || !strings.Contains(err.Error(), "nativeCore-native-product-pattern-duplicate.osr:3:") || !strings.Contains(err.Error(), "repeats field") {
		t.Fatalf("duplicate product-pattern field diagnostic = program %v, error %v", duplicate, err)
	}
}

func TestNativeCoreProductPatternOwnershipAndCapabilityContract(t *testing.T) {
	program, err := Parse("<nativeCore-product-pattern-contract>", `struct Point { x u64, y u64 }
let point = Point { x: 1u64, y: 9u64 }
emit match point { Point { x: x } => x, _ => 0u64 }
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure product-pattern matching unexpectedly defines a host capability")
	}
	for _, symbol := range []string{
		"static OValue oproductis(",
		"static OValue oproductfield(",
		"oproductis(",
		"oproductfield(",
		", \"Point\", (const char*[]){\"x\"}",
	} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated product-pattern artifact lacks %q", symbol)
		}
	}
	fieldStart := strings.Index(artifact.Source, "static OValue oproductfield(")
	if fieldStart < 0 {
		t.Fatal("generated product-pattern artifact does not expose a product-field helper")
	}
	fieldEnd := strings.Index(artifact.Source[fieldStart:], "/* Named products")
	if fieldEnd < 0 {
		t.Fatal("generated product-pattern artifact does not expose a bounded product-field helper block")
	}
	fieldHelper := artifact.Source[fieldStart : fieldStart+fieldEnd]
	if strings.Contains(fieldHelper, "malloc") || strings.Contains(fieldHelper, "calloc") {
		t.Fatal("product pattern field helper unexpectedly allocates lookup storage")
	}
}
