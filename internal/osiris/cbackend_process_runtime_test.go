package osiris

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestNativeHostResourceManifestIsExplicit(t *testing.T) {
	if got, ok := nativeHostResourceCapability("std/os", "process_spawn"); !ok || got != "os.process_spawn" {
		t.Fatalf("process_spawn capability = %q, %v", got, ok)
	}
	if got, ok := nativeHostResourceCapability("std/os", "set_deadline"); !ok || got != "os.file_metadata" {
		t.Fatalf("set_deadline capability = %q, %v", got, ok)
	}
	if _, ok := nativeHostResourceCapability("std/os", "unknown"); ok {
		t.Fatal("unknown host resource unexpectedly has an ABI")
	}
	for name, expression := range nativeHostResourceRuntimeSymbols {
		if strings.TrimSpace(expression) == "" {
			t.Fatalf("empty runtime expression for %s", name)
		}
	}
}

func TestNativeRootResourceSubsetAdmissionIsExplicit(t *testing.T) {
	for _, member := range []string{
		"root_create", "root_open", "root_open_file", "root_read_file", "root_write_file",
		"root_stat", "root_lstat", "root_readlink", "root_mkdir", "root_mkdir_all", "root_remove", "root_remove_all", "root_rename", "root_link", "root_symlink", "root_chmod", "root_close",
	} {
		if _, ok := nativeStaticModuleMember("std/os", member); !ok {
			t.Fatalf("supported native root member %q is not admitted", member)
		}
		if _, ok := nativeHostResourceRuntimeSymbols[member]; !ok {
			t.Fatalf("supported native root member %q has no runtime symbol", member)
		}
	}
	if got, ok := nativeHostResourceCapability("std/os", "root_open_file"); !ok || got != "os.file_write" {
		t.Fatalf("root_open_file capability = %q, %v; want conservative write grant", got, ok)
	}
	for _, member := range []string{"root_chown", "root_lchown", "root_chtimes", "root_fs"} {
		if _, ok := nativeStaticModuleMember("std/os", member); !ok {
			t.Fatalf("supported native root member %q is not admitted", member)
		}
		if _, ok := nativeHostResourceRuntimeSymbols[member]; !ok {
			t.Fatalf("supported native root member %q has no runtime symbol", member)
		}
	}
}

func TestNativeHostResourceComponentHasOwnershipAndCancellationBoundaries(t *testing.T) {
	component := nativeHostResourceRuntimeComponent
	for _, marker := range []string{
		"OSIRIS_HOST_RESOURCE_MAGIC",
		"osiris_host_resource_cleanup",
		"osiris_host_resource_close",
		"fork()",
		"execvp(command_copy, argv)",
		"waitpid(resource->pid",
		"kill(resource->pid",
		"poll(&pfd",
		"process_release requires a waited process",
		"OSIRIS_HOST_MAX_IO",
		"command_copy[command_length] = 0",
		"timeout.kind == O_NULL ? -1",
		"osiris_host_root_open_followed",
		"O_NOFOLLOW",
		"root symlink target is absolute, escaping, or too large",
		"root_open requires a relative path without ..",
	} {
		if !strings.Contains(component, marker) {
			t.Fatalf("native resource component missing %q", marker)
		}
	}
}

func TestNativeHostResourceComponentCompilesAsC(t *testing.T) {
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("cc not available")
	}

	// The component is intentionally compiled against the current generated
	// prelude without wiring it into the shared emitter. This catches ABI drift
	// while preserving the integrator-only native host boundary.
	source := cRuntimePrelude() + "\n" + nativeHostResourceRuntimeComponent + "\n"
	path := t.TempDir() + "/host_resources.c"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	// The existing prelude has two legacy enum cases without printer output;
	// suppress that unrelated warning while treating component diagnostics as
	// errors in the focused ABI smoke test.
	cmd := exec.Command(cc, "-std=c11", "-Werror", "-Wno-switch", "-fsyntax-only", path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("native host-resource ABI does not compile: %v\n%s", err, output)
	}
}

func TestNativeHostResourceComponentsCompileAsNonPOSIXC(t *testing.T) {
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("cc not available")
	}
	source := cRuntimePrelude() + "\n" + cRuntimeNetworkComponent + "\n" + nativeHostResourceRuntimeComponent + "\n" + cRuntimeTLSHTTPComponent + "\n" + cRuntimeNativeHTTPBridgeComponent + "\n"
	path := t.TempDir() + "/host_resources_windows.c"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(cc, "-std=c11", "-Werror", "-Wno-switch", "-fsyntax-only", "-DOSIRIS_FORCE_NONPOSIX", path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("non-POSIX native host-resource ABI does not compile: %v\n%s", err, output)
	}
}
