package osiris

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDependencyReportAndCycleError exercises the behavior described by its
// body and locks in the observable language, runtime, or CLI semantics that
// the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestDependencyReportAndCycleError(t *testing.T) {
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.osr")
	bPath := filepath.Join(dir, "b.osr")
	if err := os.WriteFile(aPath, []byte(`package a
import "./b.osr" as b
emit 1
`), 0o644); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(bPath, []byte(`package b
import "./a.osr" as a
emit 2
`), 0o644); err != nil {
		t.Fatalf("write b: %v", err)
	}

	rt := NewRuntime()
	src, err := os.ReadFile(aPath)
	if err != nil {
		t.Fatalf("read a: %v", err)
	}
	report, err := rt.BuildDependencyReport(aPath, string(src))
	if err != nil {
		t.Fatalf("dependency report failed: %v", err)
	}
	if len(report.Cycles) == 0 {
		t.Fatalf("expected cycle report, got %#v", report)
	}
	if got := strings.Join(report.Cycles[0], " -> "); !strings.Contains(got, "a.osr") || !strings.Contains(got, "b.osr") {
		t.Fatalf("cycle path missing modules: %s", got)
	}
	if len(report.Order) == 0 || report.Order[len(report.Order)-1] != aPath {
		t.Fatalf("resolution order unexpected: %#v", report.Order)
	}

	prog, err := Parse(aPath, `import "./b.osr" as b
emit 1
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if _, err := rt.Run(context.Background(), prog); err == nil || !strings.Contains(err.Error(), "import cycle:") || !strings.Contains(err.Error(), "a.osr") || !strings.Contains(err.Error(), "b.osr") {
		t.Fatalf("unexpected cycle error: %v", err)
	}
}
