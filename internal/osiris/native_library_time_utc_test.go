package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestNativeLibraryTimeUTCParity compares the pure UTC conversion through the
// interpreter, VM, generated C, and generated executable. Native datetimes
// carry no non-UTC location slot, so the operation must preserve the exact
// instant and fractional nanosecond while accepting the same error cases.
func TestNativeLibraryTimeUTCParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-native-time-utc",
		Source: `import "std/time" as time
let negative = time.unix(-1, 250000000)
let leap = time.date(2024, 2, 29, 23, 59, 58, 123456789)
emit time.format(time.utc(negative), time.rfc3339_nano)
emit time.format(time.utc(leap), "2006-01-02 15:04:05.999999999Z07:00")
emit time.compare(time.utc(negative), negative) == 0
emit try { time.utc() } catch err { "caught-arity" }
emit try { time.utc(1u64) } catch err { "caught-type" }
`,
	})
}

// TestNativeLibraryTimeUTCContract verifies direct generated-C lowering,
// value ownership, exact validation, and the absence of host capabilities.
// The same focused test is suitable for normal, race, and LLVM AddressSanitizer
// runs because it builds and executes the generated C artifact.
func TestNativeLibraryTimeUTCContract(t *testing.T) {
	program, err := Parse("<nativeLibrary-native-time-utc-contract>", `import "std/time" as time
emit time.format(time.utc(time.unix(-1, 250000000)), time.rfc3339_nano)
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure std/time.utc lowering unexpectedly defines a host capability")
	}
	if strings.Count(artifact.Source, "ostd_time_utc(") < 2 {
		t.Fatal("generated UTC artifact does not contain the helper and direct call")
	}
	if strings.Contains(artifact.Source, `ostd_dispatch("std/time", "utc"`) {
		t.Fatal("valid std/time.utc call unexpectedly uses the generic dispatcher")
	}
	for _, marker := range []string{
		"utc expects 1 datetime",
		"typedef struct { int64_t seconds; int32_t nanosecond; } ODatetime",
		"O_DATETIME",
	} {
		if !strings.Contains(artifact.Source, marker) {
			t.Fatalf("generated UTC artifact does not preserve %q", marker)
		}
	}
	entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/time/utc")
	if !ok || entry.Native != BackendImplemented {
		t.Fatalf("std/time.utc inventory entry = %+v, want implemented native capability", entry)
	}
}
