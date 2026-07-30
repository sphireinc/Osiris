package osiris

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func namedDefinitionNames(definition namedConstantDefinition) []string {
	return append([]string{definition.Canonical}, definition.Aliases...)
}

func TestNamedConstantCatalogAliasesInferenceAndCollisions(t *testing.T) {
	rt := NewRuntime()
	seen := make(map[string]string)
	for _, definition := range namedConstantDefinitions {
		canonical, ok := namedConstantValue(definition.Canonical)
		if !ok {
			t.Fatalf("canonical constant %q is not registered", definition.Canonical)
		}
		wantInfo := inferTypeFromValue(canonical)
		for _, name := range namedDefinitionNames(definition) {
			if previous := seen[name]; previous != "" {
				t.Fatalf("constant name %q is shared by %s and %s", name, previous, definition.Canonical)
			}
			seen[name] = definition.Canonical
			if _, exists := rt.Builtins[name]; exists {
				t.Fatalf("constant %q collides with a global builtin", name)
			}
			if _, exists := rt.ContextBuiltins[name]; exists {
				t.Fatalf("constant %q collides with a global context builtin", name)
			}
			if _, err := Parse("<constant-name>", "emit "+name); err != nil {
				t.Fatalf("constant %q cannot be used as an identifier: %v", name, err)
			}
			got, ok := namedConstantValue(name)
			if !ok || !got.Equal(canonical) || got.Kind() != canonical.Kind() {
				t.Fatalf("%s alias %q = %s (%s), want %s (%s)", definition.Canonical, name, got.String(), got.Kind(), canonical.String(), canonical.Kind())
			}
			info, ok := namedConstantInfo(name)
			if !ok || info.kind != wantInfo.kind || info.bits != wantInfo.bits || !dimsEqual(info.units, wantInfo.units) {
				t.Fatalf("%s inference = %#v, want %#v", name, info, wantInfo)
			}
		}
	}
	if got, want := len(rt.Constants), len(seen); got != want {
		t.Fatalf("runtime constants = %d, catalog names = %d", got, want)
	}
}

func TestExpandedUnitExactIdentitiesAndInterpreterVMParity(t *testing.T) {
	source := `
emit 12 * inch == foot
emit 3 * foot == yard
emit 1760 * yard == mile
emit gallon == 231 * inch * inch * inch
emit ImperialGallon == 4.54609d * liter
emit pound == 0.45359237d * kilogram
emit atmosphere == 101325 * pascal
emit torr == atmosphere / 760
emit knot == NauticalMile / hour
emit KilowattHour == 3600000 * joule
emit milliliter == cubicCentimeter
emit lightYear == SpeedOfLight * JulianYear
`
	prog, err := Parse("<expanded-units>", source)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	interpreter, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := rt.RunVM(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(interpreter), 12; got != want {
		t.Fatalf("interpreter produced %d identities, want %d", got, want)
	}
	for index := range interpreter {
		if !interpreter[index].Equal(boolValue(true)) {
			t.Fatalf("identity %d is %s, want true", index, interpreter[index].String())
		}
		if !vm[index].Equal(interpreter[index]) {
			t.Fatalf("identity %d differs: interpreter=%s vm=%s", index, interpreter[index].String(), vm[index].String())
		}
	}
}

func TestNamedQuantityCodecRoundTripsEveryCatalogConcept(t *testing.T) {
	for _, definition := range namedConstantDefinitions {
		value := definition.Factory()
		if value.kind != kindQuantity {
			continue
		}
		t.Run(definition.Canonical, func(t *testing.T) {
			encoded, err := encodeValueBytes(value)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := decodeValueBytes(encoded)
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Kind() != value.Kind() || !decoded.Equal(value) || decoded.String() != value.String() {
				t.Fatalf("round trip = %s (%s), want %s (%s)", decoded.String(), decoded.Kind(), value.String(), value.Kind())
			}
		})
	}
}

func TestExpandedUnitIncompatibleDimensionsFail(t *testing.T) {
	prog, err := Parse("<expanded-unit-mismatch>", "emit gallon + pound")
	if err != nil {
		t.Fatal(err)
	}
	for _, runner := range []struct {
		name string
		run  func(context.Context, *Program) ([]Value, error)
	}{
		{"interpreter", NewRuntime().Run},
		{"vm", NewRuntime().RunVM},
	} {
		t.Run(runner.name, func(t *testing.T) {
			if _, err := runner.run(context.Background(), prog); err == nil || !strings.Contains(err.Error(), "cannot add") {
				t.Fatalf("error = %v, want incompatible-dimension failure", err)
			}
		})
	}
}

func TestExpandedConstantInferenceReport(t *testing.T) {
	prog, err := Parse("<constant-inference>", `
let length = foot
let volume = gallon
let mass = pound
let pressure = atmosphere
let energy = KilowattHour
let speed = knot
let angle = degree
let mathValue = Sqrt2
let physical = VacuumPermeability
`)
	if err != nil {
		t.Fatal(err)
	}
	inferred, err := NewRuntime().InferProgram(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	report := FormatInferProgram(inferred)
	for _, want := range []string{
		"main.length: quantity[Meter]",
		"main.volume: quantity[Meter^3]",
		"main.mass: quantity[Kilogram]",
		"main.pressure: quantity[Kilogram_Per_Meter_Times_Second^2]",
		"main.energy: quantity[Kilogram_Times_Meter^2_Per_Second^2]",
		"main.speed: quantity[Meter_Per_Second]",
		"main.angle: number",
		"main.mathValue: number",
		"main.physical: quantity[Kilogram_Times_Meter_Per_Ampere^2_Times_Second^2]",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("inference report lacks %q:\n%s", want, report)
		}
	}
}

func TestNamedConstantDocumentationIndexesMatchCatalog(t *testing.T) {
	type referenceEntry struct {
		Names       []string `json:"names"`
		Description string   `json:"description"`
		Value       string   `json:"value"`
		Example     string   `json:"example"`
	}
	type referenceSection struct {
		Entries []referenceEntry `json:"entries"`
	}
	type referencePage struct {
		Sections []referenceSection `json:"sections"`
	}
	var reference struct {
		Global referencePage `json:"global"`
		Errors referencePage `json:"errors"`
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "newdocs", "src", "constant-reference.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &reference); err != nil {
		t.Fatal(err)
	}
	documented := make(map[string]string)
	for _, page := range []referencePage{reference.Global, reference.Errors} {
		for _, section := range page.Sections {
			for _, entry := range section.Entries {
				if len(entry.Names) == 0 || entry.Description == "" {
					t.Fatalf("incomplete constant reference entry: %#v", entry)
				}
				for _, name := range entry.Names {
					if previous := documented[name]; previous != "" {
						t.Fatalf("documented constant %q appears in %s and %s", name, previous, entry.Names[0])
					}
					documented[name] = entry.Names[0]
				}
				if entry.Value == "" && entry.Example == "" {
					// Error constants synthesize their value/example from `code`;
					// global entries must provide both fields directly.
					continue
				}
				if entry.Value == "" || entry.Example == "" {
					t.Fatalf("constant %q lacks a value or example", entry.Names[0])
				}
			}
		}
	}
	implemented := make([]string, 0, len(namedConstantFactories))
	for name := range namedConstantFactories {
		implemented = append(implemented, name)
		if documented[name] == "" {
			t.Errorf("implemented constant %q has no detailed documentation", name)
		}
	}
	sort.Strings(implemented)
	var technical struct {
		NamedConstants []string `json:"namedConstants"`
	}
	data, err = os.ReadFile(filepath.Join("..", "..", "newdocs", "src", "technical-reference.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &technical); err != nil {
		t.Fatal(err)
	}
	got := append([]string(nil), technical.NamedConstants...)
	sort.Strings(got)
	if strings.Join(got, "\n") != strings.Join(implemented, "\n") {
		t.Fatalf("technical named-constant index differs from implementation")
	}
}

func TestNativeNumericGlobalParityAndQuantityDiagnostic(t *testing.T) {
	numeric, err := Parse("<native-number-global>", "emit Sqrt2")
	if err != nil {
		t.Fatal(err)
	}
	ir, err := NewRuntime().BuildIR(context.Background(), numeric)
	if err != nil {
		t.Fatal(err)
	}
	if text := FormatIRProgram(ir); !strings.Contains(text, "global Sqrt2") {
		t.Fatalf("IR does not retain named global identity:\n%s", text)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), numeric)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(artifact.Source, "1.4142135623730951") {
		t.Fatalf("generated C does not contain Sqrt2's binary64 value:\n%s", artifact.Source)
	}
	if _, err := exec.LookPath("cc"); err == nil {
		executable := filepath.Join(t.TempDir(), "named-number")
		if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
			t.Fatal(err)
		}
		native, err := exec.Command(executable).CombinedOutput()
		if err != nil {
			t.Fatalf("native execution: %v\n%s", err, native)
		}
		interpreted, err := NewRuntime().Run(context.Background(), numeric)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(native), valuesOutput(interpreted); got != want {
			t.Fatalf("native output = %q, want %q", got, want)
		}
	}

	quantity, err := Parse("<native-quantity-global>", "emit foot")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileC(context.Background(), NewRuntime(), quantity); err == nil || !strings.Contains(err.Error(), `named quantity "foot"`) {
		t.Fatalf("quantity compile error = %v, want explicit named-quantity diagnostic", err)
	}
}
