package osiris

import (
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// TestHeapInspectorSkipsEmitAndTracksModules exercises the behavior described
// by its body and locks in the observable language, runtime, or CLI semantics
// that the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func TestHeapInspectorSkipsEmitAndTracksModules(t *testing.T) {
	dir := t.TempDir()
	sourceName := filepath.Join(dir, "main.osr")
	source := `
let x = 3
import "std/text" as text
emit probe(x)
`

	rt := NewRuntime()
	var probes int32
	rt.Builtins["probe"] = func(args []Value) (Value, error) {
		atomic.AddInt32(&probes, 1)
		return args[0], nil
	}

	insp, err := rt.OpenHeapInspector(context.Background(), sourceName, source)
	if err != nil {
		t.Fatalf("open heap inspector failed: %v", err)
	}
	if got := atomic.LoadInt32(&probes); got != 0 {
		t.Fatalf("probe calls = %d, want 0", got)
	}

	report := insp.Report()
	if got, want := len(report.Scope), 2; got != want {
		t.Fatalf("scope len = %d, want %d", got, want)
	}
	if report.Scope[0].Name != "x" || report.Scope[0].Kind != "uint" || report.Scope[0].Value != "3u64" {
		t.Fatalf("unexpected first binding: %#v", report.Scope[0])
	}
	if report.Scope[1].Name != "text" || report.Scope[1].Kind != "module" {
		t.Fatalf("unexpected module binding: %#v", report.Scope[1])
	}

	found := false
	for _, mod := range report.Modules {
		if strings.Contains(mod.Ref, "std/text") {
			found = true
			if mod.Package != "std/text" || mod.Version != "1.0.0" {
				t.Fatalf("unexpected stdlib metadata: %#v", mod)
			}
			if len(mod.Exports) == 0 {
				t.Fatalf("expected stdlib exports, got %#v", mod)
			}
			break
		}
	}
	if !found {
		t.Fatalf("stdlib module not reported: %#v", report.Modules)
	}

	v, err := insp.Query("x + 4")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if got, want := v.String(), "7u64"; got != want {
		t.Fatalf("query output = %s, want %s", got, want)
	}
}
