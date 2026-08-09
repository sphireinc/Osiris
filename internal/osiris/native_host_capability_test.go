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

func TestNativeHostHostIndependentOSHelpers(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	source := `import "std/os" as os
emit os.is_path_separator(47u8)
`
	program, err := Parse("nativeHost-native-host-independent-os-helpers.osr", source)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	rt.GrantCapability("os.system_identity")
	want, err := rt.Run(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileCWithOptions(context.Background(), rt, program, CCompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "nativeHost-native-os")
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatal(err)
	}
	got, err := exec.Command(executable).CombinedOutput()
	if err != nil {
		t.Fatalf("native executable: %v (%s)", err, got)
	}
	if string(got) != valuesOutput(want) {
		t.Fatalf("native output = %q, want %q", got, valuesOutput(want))
	}
}

func TestNativeHostCapabilityContractIsExplicit(t *testing.T) {
	program, err := Parse("nativeHost-capabilities.osr", `import "std/os" as os
	emit os.getenv("OSIRIS_NATIVE_HOST_MISSING")
`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileC(context.Background(), NewRuntime(), program); err == nil || !strings.Contains(err.Error(), "native capability") {
		t.Fatalf("CompileC error = %v, want source-spanned unsupported host capability diagnostic", err)
	}
	options := CCompileOptions{NativeCapabilities: []string{"os.environment", "os.environment"}}
	if got := normalizedNativeCapabilities(options.NativeCapabilities); len(got) != 1 || got[0] != "os.environment" {
		t.Fatalf("normalized capabilities = %#v, want one sorted unique capability", got)
	}
	if nativeCapabilityMacro("os.environment") != "OS_ENVIRONMENT" {
		t.Fatalf("capability macro = %q", nativeCapabilityMacro("os.environment"))
	}
}

func TestNativeHostBoundedFileCapabilityParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "input.bin")
	output := filepath.Join(dir, "output.bin")
	if err := os.WriteFile(input, []byte("seed"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := fmt.Sprintf(`import "std/os" as os
emit os.read_file(%q)
emit os.write_file(%q, [65u8, 66u8], 420)
emit os.read_file(%q)
`, input, output, output)
	program, err := Parse("nativeHost-native-file.osr", source)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	rt.GrantCapability("os.file_read")
	rt.GrantCapability("os.file_write")
	want, err := rt.Run(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	options := CCompileOptions{NativeCapabilities: []string{"os.file_read", "os.file_write"}}
	artifact, err := CompileCWithOptions(context.Background(), rt, program, options)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(dir, "nativeHost-native-file")
	if err := CompileCExecutableWithOptions(context.Background(), artifact.Source, executable, options); err != nil {
		t.Fatal(err)
	}
	got, err := exec.Command(executable).CombinedOutput()
	if err != nil {
		t.Fatalf("native executable: %v (%s)", err, got)
	}
	if string(got) != valuesOutput(want) {
		t.Fatalf("native output = %q, want %q", got, valuesOutput(want))
	}
}

func TestNativeHostEnvironmentCapabilityParityForMissingValue(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	const name = "OSIRIS_NATIVE_HOST_DETERMINISTIC_MISSING"
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
	program, err := Parse("nativeHost-native-env.osr", fmt.Sprintf("import \"std/os\" as os\nemit os.getenv(%q)\n", name))
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	rt.GrantCapability("os.environment")
	want, err := rt.Run(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	options := CCompileOptions{NativeCapabilities: []string{"os.environment"}}
	artifact, err := CompileCWithOptions(context.Background(), rt, program, options)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "nativeHost-native-env")
	if err := CompileCExecutableWithOptions(context.Background(), artifact.Source, executable, options); err != nil {
		t.Fatal(err)
	}
	got, err := exec.Command(executable).CombinedOutput()
	if err != nil {
		t.Fatalf("native executable: %v (%s)", err, got)
	}
	if string(got) != valuesOutput(want) {
		t.Fatalf("native output = %q, want %q", got, valuesOutput(want))
	}
}

func TestNativeHostIdentityCapabilityPredicates(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	program, err := Parse("nativeHost-native-identity.osr", `import "std/os" as os
emit os.getpagesize() > 0
emit os.getpid() > 0
emit os.getppid() > 0
emit os.hostname() != ""
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	rt.GrantCapability("os.system_identity")
	want, err := rt.Run(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	options := CCompileOptions{NativeCapabilities: []string{"os.system_identity"}}
	artifact, err := CompileCWithOptions(context.Background(), rt, program, options)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "nativeHost-native-identity")
	if err := CompileCExecutableWithOptions(context.Background(), artifact.Source, executable, options); err != nil {
		t.Fatal(err)
	}
	got, err := exec.Command(executable).CombinedOutput()
	if err != nil {
		t.Fatalf("native executable: %v (%s)", err, got)
	}
	if string(got) != valuesOutput(want) {
		t.Fatalf("native output = %q, want %q", got, valuesOutput(want))
	}
}

func TestNativeHostEnvironmentListingAndMutation(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	const name = "OSIRIS_NATIVE_HOST_MUTATION"
	_ = os.Unsetenv(name)
	program, err := Parse("nativeHost-native-env-mutation.osr", fmt.Sprintf(`import "std/os" as os
emit len(os.env_names()) > 0
emit len(os.environ()) > 0
emit os.setenv(%q, "value")
emit os.getenv(%q)
`, name, name))
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	rt.GrantCapability("os.environment")
	rt.GrantCapability("os.environment_mutate")
	want, err := rt.Run(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
	options := CCompileOptions{NativeCapabilities: []string{"os.environment", "os.environment_mutate"}}
	artifact, err := CompileCWithOptions(context.Background(), rt, program, options)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "nativeHost-native-env-mutation")
	if err := CompileCExecutableWithOptions(context.Background(), artifact.Source, executable, options); err != nil {
		t.Fatal(err)
	}
	got, err := exec.Command(executable).CombinedOutput()
	if err != nil {
		t.Fatalf("native executable: %v (%s)", err, got)
	}
	if string(got) != valuesOutput(want) {
		t.Fatalf("native output = %q, want %q", got, valuesOutput(want))
	}
}
