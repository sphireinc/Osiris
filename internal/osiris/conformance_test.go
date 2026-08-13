package osiris

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type conformanceGolden struct {
	Output            string `json:"output,omitempty"`
	ErrorContains     string `json:"error_contains,omitempty"`
	Native            bool   `json:"native,omitempty"`
	NativeUnsupported bool   `json:"native_unsupported,omitempty"`
	SkipNative        bool   `json:"skip_native,omitempty"`
}

// coreConformanceGolden is the public, backend-neutral fixture format for the
// language contract.  The legacy testdata/conformance corpus intentionally
// remains supported while specifications migrate to spec/core.
type coreConformanceGolden struct {
	Version              string                       `json:"version"`
	Rule                 string                       `json:"rule"`
	Category             string                       `json:"category"`
	Output               string                       `json:"output,omitempty"`
	ErrorContains        string                       `json:"error_contains,omitempty"`
	NativeOutputContains string                       `json:"native_output_contains,omitempty"`
	Phase                string                       `json:"phase,omitempty"`
	Backends             coreConformanceBackendMatrix `json:"backends"`
	Reason               string                       `json:"reason,omitempty"`
}

type coreConformanceBackendMatrix struct {
	Interpreter string `json:"interpreter"`
	VM          string `json:"vm"`
	Native      string `json:"native"`
	WASM        string `json:"wasm"`
}

const (
	backendRequired = "required"
	backendReject   = "reject"
	backendSkip     = "skip"
)

func TestCoreSemanticsConformance(t *testing.T) {
	root := filepath.Join("..", "..", "spec", "core")
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".osr") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("Core Semantics fixture corpus is empty")
	}
	for _, path := range paths {
		path := path
		t.Run(strings.TrimSuffix(filepath.ToSlash(path), ".osr"), func(t *testing.T) {
			runCoreSemanticsFixture(t, path)
		})
	}
}

func runCoreSemanticsFixture(t *testing.T, path string) {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(strings.TrimSuffix(path, ".osr") + ".golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden coreConformanceGolden
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}
	validateCoreConformanceGolden(t, golden)

	prog, parseErr := Parse(path, string(source))
	if golden.Phase == "parse" {
		if parseErr == nil || !strings.Contains(parseErr.Error(), golden.ErrorContains) {
			t.Fatalf("parse error = %v, want substring %q", parseErr, golden.ErrorContains)
		}
		return
	}
	if parseErr != nil {
		t.Fatal(parseErr)
	}

	rt := NewRuntime()
	if golden.Phase == "type" {
		if _, err := rt.CheckProgram(context.Background(), prog); err == nil || !strings.Contains(err.Error(), golden.ErrorContains) {
			t.Fatalf("type error = %v, want substring %q", err, golden.ErrorContains)
		}
		return
	}
	if golden.Backends.Interpreter == backendRequired {
		values, err := rt.Run(context.Background(), prog)
		assertCoreConformanceResult(t, "interpreter", valuesOutput(values), err, golden)
	}
	if golden.Backends.VM == backendRequired {
		values, err := rt.RunVM(context.Background(), prog)
		assertCoreConformanceResult(t, "VM", valuesOutput(values), err, golden)
	}
	if golden.Backends.VM == backendReject {
		if _, err := rt.RunVM(context.Background(), prog); err == nil {
			t.Fatal("VM unexpectedly accepted a fixture marked reject")
		}
	}
	switch golden.Backends.Native {
	case backendRequired:
		if golden.ErrorContains != "" {
			runCoreNativeErrorFixture(t, path, prog, golden)
			return
		}
		runDifferentialFixture(t, differentialFixture{Name: strings.TrimSuffix(filepath.Base(path), ".osr"), Source: string(source)})
	case backendReject:
		if _, err := CompileC(context.Background(), NewRuntime(), prog); err == nil {
			t.Fatal("native C unexpectedly accepted a fixture marked reject")
		}
	}
}

func runCoreNativeErrorFixture(t *testing.T, path string, prog *Program, golden coreConformanceGolden) {
	t.Helper()
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available for native Core error fixture")
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), prog)
	if err != nil {
		t.Fatalf("native C compile: %v", err)
	}
	executable := filepath.Join(t.TempDir(), filepath.Base(strings.TrimSuffix(path, ".osr")))
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatalf("native executable compile: %v", err)
	}
	output, runErr := exec.Command(executable).CombinedOutput()
	if runErr == nil {
		t.Fatalf("native executable unexpectedly succeeded: %q", output)
	}
	got := string(output)
	if !strings.Contains(got, golden.ErrorContains) {
		t.Fatalf("native error output = %q, want substring %q", got, golden.ErrorContains)
	}
	if golden.NativeOutputContains != "" && !strings.Contains(got, golden.NativeOutputContains) {
		t.Fatalf("native error output = %q, want prior output %q", got, golden.NativeOutputContains)
	}
}

func validateCoreConformanceGolden(t *testing.T, golden coreConformanceGolden) {
	t.Helper()
	if golden.Version != CoreSemanticsVersion || golden.Rule == "" || golden.Category == "" {
		t.Fatalf("fixture metadata must set version %q, rule, and category: %#v", CoreSemanticsVersion, golden)
	}
	if (golden.Output == "") == (golden.ErrorContains == "") {
		t.Fatal("fixture metadata must set exactly one of output or error_contains")
	}
	for name, status := range map[string]string{
		"interpreter": golden.Backends.Interpreter,
		"vm":          golden.Backends.VM,
		"native":      golden.Backends.Native,
		"wasm":        golden.Backends.WASM,
	} {
		if status != backendRequired && status != backendReject && status != backendSkip {
			t.Fatalf("backend %s has invalid status %q", name, status)
		}
	}
	if golden.Backends.Interpreter != backendRequired {
		t.Fatal("Core Semantics fixtures require interpreter coverage")
	}
	if golden.Backends.VM == backendSkip {
		t.Fatal("Core Semantics fixtures require VM execution or explicit rejection")
	}
}

func assertCoreConformanceResult(t *testing.T, backend, output string, err error, golden coreConformanceGolden) {
	t.Helper()
	if golden.ErrorContains != "" {
		if err == nil || !strings.Contains(err.Error(), golden.ErrorContains) {
			t.Fatalf("%s error = %v, want substring %q", backend, err, golden.ErrorContains)
		}
		return
	}
	if err != nil {
		t.Fatalf("%s error: %v", backend, err)
	}
	if output != golden.Output {
		t.Fatalf("%s output = %q, want %q", backend, output, golden.Output)
	}
}

func TestConformanceCorpus(t *testing.T) {
	var paths []string
	err := filepath.WalkDir("testdata/conformance", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".osr") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("conformance corpus is empty")
	}
	for _, path := range paths {
		t.Run(strings.TrimSuffix(path, ".osr"), func(t *testing.T) {
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(strings.TrimSuffix(path, ".osr") + ".golden.json")
			if err != nil {
				t.Fatal(err)
			}
			var golden conformanceGolden
			if err := json.Unmarshal(data, &golden); err != nil {
				t.Fatal(err)
			}
			prog, err := Parse(path, string(source))
			if golden.ErrorContains != "" {
				if err == nil || !strings.Contains(err.Error(), golden.ErrorContains) {
					t.Fatalf("parse error = %v, want substring %q", err, golden.ErrorContains)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			rt := NewRuntime()
			values, runErr := rt.Run(context.Background(), prog)
			if runErr != nil {
				t.Fatalf("interpreter error: %v", runErr)
			}
			if os.Getenv("OSIRIS_UPDATE_GOLDENS") == "1" {
				golden.Output = valuesOutput(values)
				updated, err := json.MarshalIndent(golden, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(strings.TrimSuffix(path, ".osr")+".golden.json", append(updated, '\n'), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := valuesOutput(values); got != golden.Output {
				t.Fatalf("interpreter output = %q, want %q", got, golden.Output)
			}
			vmValues, vmErr := rt.RunVM(context.Background(), prog)
			if vmErr != nil || valuesOutput(vmValues) != golden.Output {
				t.Fatalf("VM output/error = %q / %v, want %q", valuesOutput(vmValues), vmErr, golden.Output)
			}
			if golden.NativeUnsupported {
				if _, err := CompileC(context.Background(), NewRuntime(), prog); err == nil {
					t.Fatal("native C unexpectedly accepted unsupported corpus program")
				}
			}
			if golden.Native {
				runDifferentialFixture(t, differentialFixture{Name: strings.TrimSuffix(filepath.Base(path), ".osr"), Source: string(source)})
			}
		})
	}
}
