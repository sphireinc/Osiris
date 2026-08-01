package osiris

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBackendCapabilityInventoryIsCompleteAndUnique(t *testing.T) {
	entries := BackendCapabilityInventory()
	if len(entries) == 0 {
		t.Fatal("backend capability inventory is empty")
	}
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.ID == "" || entry.Surface == "" || entry.Evidence == "" || entry.Note == "" {
			t.Fatalf("incomplete backend capability entry: %+v", entry)
		}
		if entry.VM != BackendImplemented && entry.VM != BackendUnsupported {
			t.Fatalf("invalid VM status for %s: %q", entry.ID, entry.VM)
		}
		if entry.Native != BackendImplemented && entry.Native != BackendUnsupported {
			t.Fatalf("invalid native status for %s: %q", entry.ID, entry.Native)
		}
		if _, duplicate := seen[entry.ID]; duplicate {
			t.Fatalf("duplicate backend capability %s", entry.ID)
		}
		seen[entry.ID] = struct{}{}
	}
}

func TestRunVMPublicPathHasNoInterpreterDelegation(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "bytecode.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	if strings.Contains(body, "ExecProgram(") || strings.Contains(body, ".eval(") {
		t.Fatal("public VM implementation contains an interpreter delegation")
	}
}

func TestNativeCompilerHasNoEvaluatorFallback(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(filename)
	for _, name := range []string{"cbackend.go", "cbackend_native.go"} {
		source, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		body := string(source)
		for _, forbidden := range []string{"ExecProgram(", "RunVM(", "snapshot output", "interpreter fallback"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("native compiler %s contains forbidden fallback marker %q", name, forbidden)
			}
		}
	}
}
