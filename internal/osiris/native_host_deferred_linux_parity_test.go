package osiris

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestNativeHostDeferredLinuxGeneratedCDeterministicParity is the executable evidence
// boundary for 148B-4. It runs deterministic fixtures through the interpreter,
// VM, and generated C executable, then checks all three against fixed output.
// The fixtures intentionally avoid host IDs, clocks, environment variables,
// network services, and host-dependent paths.
func TestNativeHostDeferredLinuxGeneratedCDeterministicParity(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux generated-C evidence runs in the Linux runner")
	}
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available in the Linux runner")
	}

	fixtures := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "scalar-control-flow",
			source: `var total = 0
for value in [1, 2, 3, 4] {
    total = total + value
}
emit total
emit total == 10
`,
			want: "10u64\ntrue\n",
		},
		{
			name: "standard-library-values",
			source: `import "std/text" as text
import "std/list" as list
emit text.trim("  Linux parity  ")
emit list.reverse([1, 2, 3])
emit len(text.split("a,b,c", ","))
`,
			want: "\"Linux parity\"\n[3u64, 2u64, 1u64]\n3\n",
		},
		{
			name: "codec-and-numeric-values",
			source: `emit dec("0.5")
emit bigint("92233720368547758081234567890")
emit decode(encode([7u32, 1.25f64, complex(2.5f64, -3.0f64)]))
`,
			want: "0.5d\n92233720368547758081234567890n\n[7u32, 1.25f64, 2.5f64-3f64i]\n",
		},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			got := runNativeHostDeferredLinuxDifferentialFixture(t, fixture.name, fixture.source)
			if got != fixture.want {
				t.Fatalf("deterministic output = %q, want %q", got, fixture.want)
			}
		})
	}
}

func runNativeHostDeferredLinuxDifferentialFixture(t *testing.T, name, source string) string {
	t.Helper()
	program, err := Parse("nativeHostDeferred-linux-"+name+".osr", source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	rt := NewRuntime()
	interpreterValues, interpreterErr := rt.Run(context.Background(), program)
	if interpreterErr != nil {
		t.Fatalf("interpreter: %v", interpreterErr)
	}
	vmValues, vmErr := rt.RunVM(context.Background(), program)
	if vmErr != nil {
		t.Fatalf("VM: %v", vmErr)
	}
	want := valuesOutput(interpreterValues)
	if got := valuesOutput(vmValues); got != want {
		t.Fatalf("VM output = %q, want %q", got, want)
	}

	artifact, err := CompileCWithOptions(context.Background(), NewRuntime(), program, CCompileOptions{})
	if err != nil {
		t.Fatalf("generated-C compile phase: %v", err)
	}
	executable := filepath.Join(t.TempDir(), "nativeHostDeferred-linux-"+name)
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatalf("generated-C executable phase: %v", err)
	}
	out, err := exec.Command(executable).CombinedOutput()
	if err != nil {
		t.Fatalf("generated-C execution: %v (%s)", err, out)
	}
	if got := string(out); got != want {
		t.Fatalf("generated-C output = %q, want interpreter output %q\nsource:\n%s", got, want, source)
	}
	return string(out)
}

func TestNativeHostDeferredLinuxSanitizerContract(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux sanitizer evidence runs in the Linux runner")
	}
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available in the Linux runner")
	}
	t.Setenv("OSIRIS_C_SANITIZERS", "address")
	if got := nativeSanitizerFlags(); len(got) != 1 || got[0] != "-fsanitize=address" {
		t.Fatalf("address sanitizer flags = %v", got)
	}
	// Execute a complete generated-C fixture under the sanitizer setting so
	// this test covers compile, link, process execution, and ASan startup.
	runNativeHostDeferredLinuxDifferentialFixture(t, "sanitized-scalars", `emit 7u64 * 6u64
emit [1, 2, 3, 4] == [1, 2, 3, 4]
`)
}
