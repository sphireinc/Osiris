package osiris

import (
	"context"
	"os"
	"os/exec"
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
	for _, module := range []string{"std/time", "std/text", "std/math", "std/crypto", "std/signal", "std/net", "std/list"} {
		value, err := stdlibModule(NewRuntime(), module, "")
		if err != nil {
			t.Fatalf("stdlib module %s: %v", module, err)
		}
		loaded, err := value.AsModule()
		if err != nil {
			t.Fatalf("stdlib module %s value: %v", module, err)
		}
		for member := range loaded.members {
			id := "module/" + module + "/" + member
			if _, ok := seen[id]; !ok {
				t.Fatalf("public stdlib member %s is missing from backend inventory", id)
			}
		}
	}
	if entry, ok := findBackendCapability(entries, "syntax/defer"); !ok || entry.Native != BackendImplemented {
		t.Fatalf("defer inventory entry = %+v, want native cleanup-edge support", entry)
	}
}

func findBackendCapability(entries []BackendCapability, id string) (BackendCapability, bool) {
	for _, entry := range entries {
		if entry.ID == id {
			return entry, true
		}
	}
	return BackendCapability{}, false
}

func TestCBackendPortableCompilationFlags(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	program, err := Parse("<portable-flags>", "emit 1\n")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := CompileCExecutableCached(context.Background(), artifact.Source, filepath.Join(t.TempDir(), "portable"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range manifest.Flags {
		if flag == "-march=native" {
			t.Fatalf("native cache manifest contains host-specific flag %q: %#v", flag, manifest.Flags)
		}
	}
}

func TestCBackendSanitizerFlagsAreCacheKeyed(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	t.Setenv("OSIRIS_C_SANITIZERS", "undefined")
	program, err := Parse("<sanitizer-flags>", "emit 1\n")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := CompileCExecutableCached(context.Background(), artifact.Source, filepath.Join(t.TempDir(), "sanitized"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(manifest.Flags, " ")
	if !strings.Contains(joined, "-fsanitize=undefined") || strings.Contains(joined, "-flto") {
		t.Fatalf("sanitizer manifest flags = %q, want UBSan and no LTO", joined)
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
