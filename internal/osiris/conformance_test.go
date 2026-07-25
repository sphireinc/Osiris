package osiris

import (
	"context"
	"encoding/json"
	"os"
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
