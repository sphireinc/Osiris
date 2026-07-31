package osiris

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// differentialFixture is a reusable backend-parity case. NativeUnsupported
// means that the C backend must reject the program explicitly; it is never a
// successful native result.
type differentialFixture struct {
	Name              string
	Source            string
	SourcePath        string
	NativeUnsupported bool
	SkipNative        bool
}

type differentialResult struct {
	Backend string
	Phase   string
	Output  string
	Err     error
}

func runDifferentialFixture(t *testing.T, fixture differentialFixture) {
	t.Helper()
	if fixture.SourcePath != "" {
		data, err := os.ReadFile(fixture.SourcePath)
		if err != nil {
			t.Fatalf("fixture %s: read source file %s: %v", fixture.Name, fixture.SourcePath, err)
		}
		fixture.Source = string(data)
	}
	prog, err := Parse(fixture.Name+".osr", fixture.Source)
	if err != nil {
		t.Fatalf("fixture %s parse failed: %v\n--- source ---\n%s", fixture.Name, err, fixture.Source)
	}

	rt := NewRuntime()
	interpreterValues, interpreterErr := rt.Run(context.Background(), prog)
	interpreter := differentialResult{Backend: "interpreter", Phase: "run", Output: valuesOutput(interpreterValues), Err: interpreterErr}
	vmValues, vmErr := rt.RunVM(context.Background(), prog)
	vm := differentialResult{Backend: "vm", Phase: "run", Output: valuesOutput(vmValues), Err: vmErr}
	assertDifferentialEqual(t, fixture, interpreter, vm)

	if fixture.SkipNative {
		return
	}
	if fixture.NativeUnsupported {
		if _, err := CompileC(context.Background(), NewRuntime(), prog); err == nil {
			t.Fatalf("fixture %s: native C unexpectedly accepted unsupported program\n--- source ---\n%s", fixture.Name, fixture.Source)
		}
		return
	}
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available for native differential fixture")
	}
	artifact, compileErr := CompileC(context.Background(), NewRuntime(), prog)
	if compileErr != nil {
		t.Fatalf("fixture %s: native compile phase failed: %v\n--- source ---\n%s", fixture.Name, compileErr, fixture.Source)
	}
	executable := filepath.Join(t.TempDir(), fixture.Name)
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatalf("fixture %s: C executable phase failed: %v\n--- source ---\n%s\n--- C ---\n%s", fixture.Name, err, fixture.Source, artifact.Source)
	}
	out, nativeErr := exec.Command(executable).CombinedOutput()
	native := differentialResult{Backend: "native-c", Phase: "execute", Output: string(out), Err: nativeErr}
	assertDifferentialEqual(t, fixture, interpreter, native)
}

func valuesOutput(values []Value) string {
	var output strings.Builder
	for _, value := range values {
		output.WriteString(value.String())
		output.WriteByte('\n')
	}
	return output.String()
}

func assertDifferentialEqual(t *testing.T, fixture differentialFixture, want, got differentialResult) {
	t.Helper()
	if message := differentialMismatch(fixture, want, got); message != "" {
		t.Fatal(message)
	}
}

func differentialMismatch(fixture differentialFixture, want, got differentialResult) string {
	if (want.Err == nil) == (got.Err == nil) && (want.Err == nil || want.Err.Error() == got.Err.Error()) && want.Output == got.Output {
		return ""
	}
	return fmt.Sprintf("differential mismatch in fixture %s: %s/%s versus %s/%s\nwant output: %q\nwant error: %v\ngot output: %q\ngot error: %v\n--- source ---\n%s", fixture.Name, want.Backend, want.Phase, got.Backend, got.Phase, want.Output, want.Err, got.Output, got.Err, fixture.Source)
}

func TestDifferentialHarness(t *testing.T) {
	for _, fixture := range []differentialFixture{
		{Name: "scalar-control-flow", SourcePath: "testdata/differential/scalar-control-flow.osr"},
		{Name: "native-static-math", Source: "import \"std/math\" as math\nemit math.sqrt(81)\nemit math.clamp(12, 0, 10)\n"},
		{Name: "numeric-boundaries", Source: "emit u8(255) + u8(1)\nemit i8(-128)\nemit 1.5f32 + 0.25f32\nemit -0.0f64\n"},
		{Name: "native-bigint", Source: "emit bigint(\"9223372036854775808\")\n"},
		{Name: "structured-error", Source: "emit uint(\"not-a-number\")\n", SkipNative: true},
		{Name: "unsupported-host-io", Source: "emit read(\"input.txt\")\n", NativeUnsupported: true},
	} {
		t.Run(fixture.Name, func(t *testing.T) { runDifferentialFixture(t, fixture) })
	}
}

func TestDifferentialMismatchDiagnostic(t *testing.T) {
	fixture := differentialFixture{Name: "diagnostic", Source: "emit 1"}
	message := differentialMismatch(fixture, differentialResult{Backend: "interpreter", Phase: "run", Output: "1\n"}, differentialResult{Backend: "vm", Phase: "run", Output: "2\n"})
	for _, part := range []string{fixture.Name, "interpreter/run", "vm/run", fixture.Source} {
		if !strings.Contains(message, part) {
			t.Fatalf("diagnostic %q does not identify %q", message, part)
		}
	}
}
