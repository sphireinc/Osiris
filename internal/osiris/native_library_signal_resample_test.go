package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestNativeLibrarySignalResampleParity compares the deterministic resampler
// across the interpreter, VM, generated C, and generated executable. The
// cases cover interpolation, endpoint preservation, empty and singleton input,
// and zero- and one-element output tensors.
func TestNativeLibrarySignalResampleParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-native-signal-resample",
		Source: `import "std/signal" as signal
import "std/list" as list
emit signal.resample([1, 2, 3], 5)
emit signal.resample([1, 2], 1)
emit signal.resample([1, 2], 0)
emit signal.resample([], 3)
emit signal.resample([7], 4)
emit try { signal.resample([1, 2], -1) } catch err { "caught-negative-length" }
emit try { signal.resample([1, 2], 1.5) } catch err { "caught-fractional-length" }
emit try { signal.resample([1, 2], 65537) } catch err { "caught-oversized-length" }
emit try { signal.resample(list.range(65537), 1) } catch err { "caught-oversized-input" }
emit try { signal.resample(["not numeric"], 2) } catch err { "caught-invalid-signal" }
emit try { signal.resample([[1, "not numeric"]], 2) } catch err { "caught-invalid-nested-signal" }
`,
	})
}

// TestNativeLibrarySignalResampleOwnershipErrorAndCapabilityContract checks
// the individual admission, staging, allocation, cleanup, and error boundary
// rather than relying only on equal emitted values.
func TestNativeLibrarySignalResampleOwnershipErrorAndCapabilityContract(t *testing.T) {
	program, err := Parse("<nativeLibrary-native-signal-resample-contract>", `import "std/signal" as signal
emit signal.resample([1, 2, 3], 5)
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure std/signal.resample lowering unexpectedly defines a host capability")
	}
	for _, symbol := range []string{
		"ostd_signal_resample", "ostd_signal_data", "otensordata", "ostd_dispatch",
	} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated resample artifact does not contain ownership boundary %q", symbol)
		}
	}
	for _, message := range []string{
		"signal.resample expects a one-dimensional numeric signal",
		"signal.resample exceeds the 65536-sample limit",
		"signal.resample length must be an integer from 0 through 65536",
	} {
		if !strings.Contains(artifact.Source, message) {
			t.Fatalf("generated resample artifact does not preserve error contract %q", message)
		}
	}
	entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/signal/resample")
	if !ok || entry.Native != BackendImplemented {
		t.Fatalf("std/signal.resample inventory entry = %+v, want implemented native capability", entry)
	}
}
