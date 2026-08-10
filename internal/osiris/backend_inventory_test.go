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
	for _, module := range []string{"std/time", "std/text", "std/math", "std/crypto", "std/signal", "std/net", "std/os", "std/list"} {
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
	for module, members := range map[string][]string{
		"std/net": {"tcp_listen", "tcp_accept", "tcp_dial", "udp_bind", "udp_connect", "udp_send", "udp_receive", "dns_lookup", "tls_dial", "tls_dial_config", "tls_listen", "http_get", "http_request", "http_request_stream", "http_serve"},
		"std/os":  {"open", "create", "open_file", "pipe", "read", "write", "read_at", "write_at", "seek_file", "sync", "open_root", "root_open", "root_open_file", "root_read_file", "root_write_file", "root_stat", "root_lstat", "root_fs", "root_readlink", "root_mkdir", "root_mkdir_all", "root_remove", "root_remove_all", "root_rename", "root_link", "root_symlink", "root_chmod", "root_close", "stat", "lstat", "read_dir", "mkdir", "mkdir_all", "remove", "remove_all", "truncate", "rename", "link", "symlink", "chmod", "readlink", "watch"},
	} {
		for _, member := range members {
			entry, ok := findBackendCapability(entries, "module/"+module+"/"+member)
			if !ok || entry.Native != BackendImplemented {
				t.Fatalf("%s.%s inventory entry = %+v, want implemented native lowering", module, member, entry)
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

func TestBackendCapabilityInventoryMatchesNativeShapeAndClassificationSlices(t *testing.T) {
	for _, name := range []string{"squeeze", "expand_dims", "broadcast_to", "clip", "where", "concat", "stack", "copysign", "nextafter", "isfinite", "isinf", "isnan", "isnormal", "signbit", "degrees", "radians"} {
		if got := nativeBuiltinStatus(name); got != BackendImplemented {
			t.Errorf("native builtin %s = %q, want implemented", name, got)
		}
	}
	for _, name := range []string{"clone", "view"} {
		if got := nativeBuiltinStatus(name); got != BackendUnsupported {
			t.Errorf("native builtin %s = %q, want unsupported", name, got)
		}
	}
	for _, name := range []string{"tile", "gather", "scatter", "pad"} {
		if got := nativeBuiltinStatus(name); got != BackendImplemented {
			t.Errorf("native builtin %s = %q, want implemented", name, got)
		}
	}
	for _, name := range []string{"squeeze", "expand_dims", "broadcast_to", "clip", "where", "concat", "stack", "copysign", "nextafter", "isfinite", "isinf", "isnan", "isnormal", "signbit", "degrees", "radians"} {
		if got := nativeModuleMemberStatus("std/math", name); got != BackendImplemented {
			t.Errorf("std/math.%s = %q, want implemented", name, got)
		}
	}
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
	if !containsBackendFlag(manifest.Flags, "-ffp-contract=fast") {
		t.Fatalf("native cache manifest does not specify floating-point contraction: %#v", manifest.Flags)
	}
}

func containsBackendFlag(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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
	if !strings.Contains(joined, "-fsanitize=undefined") || !strings.Contains(joined, "-flto") {
		t.Fatalf("sanitizer manifest flags = %q, want UBSan and LTO for production floating-point parity", joined)
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
