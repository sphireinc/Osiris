package osiris

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func nativeHostDeferredEncodedBytes(value string) string {
	parts := make([]string, 0, len(value))
	for i := 0; i < len(value); i++ {
		parts = append(parts, fmt.Sprintf("%du8", value[i]))
	}
	return strings.Join(parts, ", ")
}

func nativeHostDeferredShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func TestNativeHostDeferredProcessEnvironmentOverlayExecutes(t *testing.T) {
	t.Setenv("OSIRIS_148B_INHERITED", "parent")
	t.Setenv("OSIRIS_148B_DELETED", "remove-me")
	script := `printf '%s|%s|%s' "$OSIRIS_148B_INHERITED" "$OSIRIS_148B_CHILD" "$OSIRIS_148B_DELETED"`
	source := fmt.Sprintf(`import "std/os" as os
let options = map_new("env", map_new("OSIRIS_148B_CHILD", "child", "OSIRIS_148B_DELETED", null), "inherit_env", true)
let process = os.process_spawn("/bin/sh", ["-c", %q], options)
emit os.process_wait(process.process, 1000)
emit os.read(process.stdout, 4096, 1000)
emit os.process_release(process.process)
`, script)
	got := runNativeHostResourceProgram(t, source, "os.process_spawn", "os.process", "os.process_inspect", "os.file_read")
	if !strings.Contains(got, nativeHostDeferredEncodedBytes("parent|child|")) {
		t.Fatalf("inherited overlay output = %q, want parent|child| bytes with explicit deletion", got)
	}
}

func TestNativeHostDeferredProcessReplacementEnvironmentIsDeterministic(t *testing.T) {
	t.Setenv("OSIRIS_148B_PARENT_ONLY", "must-not-inherit")
	source := `import "std/os" as os
let options = map_new("env", map_new("Z_OSIRIS_148B", "last", "A_OSIRIS_148B", "first"), "inherit_env", false)
let process = os.process_spawn("/usr/bin/env", [], options)
emit os.process_wait(process.process, 1000)
emit os.read(process.stdout, 4096, 1000)
emit os.process_release(process.process)
`
	got := runNativeHostResourceProgram(t, source, "os.process_spawn", "os.process", "os.process_inspect", "os.file_read")
	if strings.Contains(got, nativeHostDeferredEncodedBytes("OSIRIS_148B_PARENT_ONLY=")) {
		t.Fatalf("replacement environment inherited parent variable: %q", got)
	}
	a := strings.Index(got, nativeHostDeferredEncodedBytes("A_OSIRIS_148B=first\n"))
	z := strings.Index(got, nativeHostDeferredEncodedBytes("Z_OSIRIS_148B=last\n"))
	if a < 0 || z < 0 || a > z {
		t.Fatalf("replacement environment order = %q, want A before Z", got)
	}
}

func TestNativeHostDeferredProcessEnvironmentInheritanceCombinations(t *testing.T) {
	t.Setenv("OSIRIS_148B_COMBINATION", "inherited")
	script := `printf '%s' "$OSIRIS_148B_COMBINATION"`
	variants := []struct {
		name    string
		options string
		want    string
	}{
		{name: "default", options: "null", want: "inherited"},
		{name: "inherit-only", options: `map_new("inherit_env", true)`, want: "inherited"},
		{name: "empty-only", options: `map_new("inherit_env", false)`, want: ""},
		{name: "overlay", options: `map_new("env", map_new("OSIRIS_148B_COMBINATION", "overlay"), "inherit_env", true)`, want: "overlay"},
		{name: "replacement", options: `map_new("env", map_new("OSIRIS_148B_COMBINATION", "replacement"), "inherit_env", false)`, want: "replacement"},
	}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			source := fmt.Sprintf(`import "std/os" as os
let process = os.process_spawn("/bin/sh", ["-c", %q], %s)
emit os.process_wait(process.process, 1000)
emit os.read(process.stdout, 4096, 1000)
emit os.process_release(process.process)
`, script, variant.options)
			got := runNativeHostResourceProgram(t, source, "os.process_spawn", "os.process", "os.process_inspect", "os.file_read")
			if variant.want == "" {
				if !strings.Contains(got, "[]") {
					t.Fatalf("environment mode output = %q, want an empty byte result", got)
				}
			} else if !strings.Contains(got, nativeHostDeferredEncodedBytes(variant.want)) {
				t.Fatalf("environment mode output = %q, want bytes for %q", got, variant.want)
			}
		})
	}
}

func TestNativeHostDeferredProcessEnvironmentInterpreterVMDifferential(t *testing.T) {
	t.Setenv("OSIRIS_148B_DIFFERENTIAL", "parent")
	source := `import "std/os" as os
let launched = os.process_spawn("/bin/sh", ["-c", "test \"$OSIRIS_148B_DIFFERENTIAL\" = overlay"], map_new("env", map_new("OSIRIS_148B_DIFFERENTIAL", "overlay"), "inherit_env", true))
let process = unwrap(map_get(launched, "process"))
let status = os.process_wait(process, 1000)
emit unwrap(map_get(status, "success"))
`
	program, err := Parse("nativeHostDeferred-process-differential.osr", source)
	if err != nil {
		t.Fatal(err)
	}
	interpreterRuntime := NewRuntime()
	interpreterRuntime.GrantCapability("os.process")
	interpreter, err := interpreterRuntime.Run(context.Background(), program)
	if err != nil {
		t.Fatalf("interpreter: %v", err)
	}
	vmRuntime := NewRuntime()
	vmRuntime.GrantCapability("os.process")
	vm, err := vmRuntime.RunVM(context.Background(), program)
	if err != nil {
		t.Fatalf("VM: %v", err)
	}
	if got, want := valuesOutput(vm), valuesOutput(interpreter); got != want {
		t.Fatalf("VM output = %q, want interpreter output %q", got, want)
	}
	nativeSource := `import "std/os" as os
let launched = os.process_spawn("/bin/sh", ["-c", "test \"$OSIRIS_148B_DIFFERENTIAL\" = overlay"], map_new("env", map_new("OSIRIS_148B_DIFFERENTIAL", "overlay"), "inherit_env", true))
let process = launched.process
let status = os.process_wait(process, 1000)
emit status.success
`
	if got := runNativeHostResourceProgram(t, nativeSource, "os.process_spawn", "os.process"); got != "true\n" {
		t.Fatalf("native output = %q, want %q", got, "true\\n")
	}
}

func TestNativeHostDeferredProcessGroupKillCleansDescendants(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "descendant-survived")
	if err := os.WriteFile(marker, []byte("not-survived"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf("trap '' TERM; (sleep 2; printf survived > %s) & wait", nativeHostDeferredShellQuote(marker))
	source := fmt.Sprintf(`import "std/os" as os
let process = os.process_spawn("/bin/sh", ["-c", %q], null)
emit os.process_kill(process.process, null)
emit os.process_wait(process.process, 1000)
emit os.process_release(process.process)
`, script)
	got := runNativeHostResourceProgram(t, source, "os.process_spawn", "os.process", "os.process_kill", "os.process_inspect")
	if !strings.Contains(got, "signal") {
		t.Fatalf("group kill output = %q, want signal termination", got)
	}
	time.Sleep(150 * time.Millisecond)
	if data, err := os.ReadFile(marker); err != nil || string(data) != "not-survived" {
		t.Fatalf("descendant survived process-group kill: data=%q err=%v", data, err)
	}
}

func TestNativeHostDeferredProcessTimeoutEscalatesAndReapsGroup(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "timeout-descendant-survived")
	if err := os.WriteFile(marker, []byte("not-survived"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf("trap '' TERM; (sleep 2; printf survived > %s) & wait", nativeHostDeferredShellQuote(marker))
	source := fmt.Sprintf(`import "std/os" as os
let process = os.process_spawn("/bin/sh", ["-c", %q], null)
emit os.process_wait(process.process, 20)
`, script)
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	program, err := Parse("nativeHostDeferred-timeout.osr", source)
	if err != nil {
		t.Fatal(err)
	}
	options := CCompileOptions{NativeCapabilities: []string{"os.process_spawn", "os.process"}}
	artifact, err := CompileCWithOptions(context.Background(), NewRuntime(), program, options)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "nativeHostDeferred-timeout")
	if err := CompileCExecutableWithOptions(context.Background(), artifact.Source, executable, options); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	output, runErr := exec.Command(executable).CombinedOutput()
	if runErr != nil {
		t.Fatalf("timeout program failed before returning its error value: %v (%s)", runErr, output)
	}
	if !strings.Contains(string(output), "process_wait deadline exceeded") {
		t.Fatalf("timeout process output = %s, want deadline error", output)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("timeout cleanup took %s; output=%s", elapsed, output)
	}
	time.Sleep(150 * time.Millisecond)
	if data, err := os.ReadFile(marker); err != nil || string(data) != "not-survived" {
		t.Fatalf("descendant survived timeout group cleanup: data=%q err=%v", data, err)
	}
}

func TestNativeHostDeferredProcessResourceExitCleanupKillsOwnedGroup(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "exit-descendant-survived")
	if err := os.WriteFile(marker, []byte("not-survived"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf("(sleep 2; printf survived > %s) & wait", nativeHostDeferredShellQuote(marker))
	source := fmt.Sprintf(`import "std/os" as os
let process = os.process_spawn("/bin/sh", ["-c", %q], null)
emit process.pid
`, script)
	runNativeHostResourceProgram(t, source, "os.process_spawn")
	time.Sleep(150 * time.Millisecond)
	if data, err := os.ReadFile(marker); err != nil || string(data) != "not-survived" {
		t.Fatalf("owned process group survived native resource exit cleanup: data=%q err=%v", data, err)
	}
}
