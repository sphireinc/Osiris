package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestNativeLibraryTimeDurationUnixParity compares the exact duration and Unix
// constructor slice across the interpreter, VM, generated C, and native
// executable. The cases include negative sub-second epochs, all six duration
// constants, and the four dual-use component names.
func TestNativeLibraryTimeDurationUnixParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-native-time-duration-unix",
		Source: `import "std/time" as time
emit time.duration_string(time.duration(1500000000))
emit time.duration_string(time.duration(2 * Second))
emit time.duration_string(time.nanosecond), time.duration_string(time.microsecond)
emit time.duration_string(time.millisecond), time.duration_string(time.second)
emit time.duration_string(time.minute), time.duration_string(time.hour)
emit time.format(time.unix(-1, 500000000), time.rfc3339_nano)
emit time.format(time.unix_milli(-1), time.rfc3339_nano)
emit time.format(time.unix_micro(-1), time.rfc3339_nano)
emit time.hour(time.unix(0, 123000000)), time.minute(time.unix(0, 123000000))
emit time.second(time.unix(0, 123000000)), time.nanosecond(time.unix(0, 123000000))
emit try { time.unix(1.5) } catch err { "fractional-seconds" }
emit try { time.unix_milli(1.5) } catch err { "fractional-milliseconds" }
emit time.duration_string(time.duration(0.5 * Second))
`,
	})
}

// TestNativeLibraryTimeDurationUnixContract verifies that the new members use
// direct value-owned C helpers, preserve the repaired ODatetime payload, and
// do not introduce a host capability or retained argument storage.
func TestNativeLibraryTimeDurationUnixContract(t *testing.T) {
	program, err := Parse("<nativeLibrary-native-time-duration-unix-contract>", `import "std/time" as time
emit time.duration(1), time.format(time.unix(0), time.rfc3339)
emit time.format(time.unix_milli(1), time.rfc3339_nano), time.duration_string(time.hour)
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure std/time duration and Unix operations unexpectedly define a host capability")
	}
	for _, symbol := range []string{
		"ostd_time_duration_value", "ostd_time_duration", "ostd_time_unix",
		"ostd_time_unix_scaled", "ODatetime", "ostd_time_datetime",
	} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated duration/Unix artifact does not contain %q", symbol)
		}
	}
	for _, call := range []string{
		"ostd_time_duration(", "ostd_time_unix((OValue[]){",
		"ostd_time_unix_scaled(", "ostd_time_duration_value(INT64_C(3600000000000))",
	} {
		if !strings.Contains(artifact.Source, call) {
			t.Fatalf("generated artifact does not contain direct lowering %q", call)
		}
	}
	if !strings.Contains(artifact.Source, "typedef struct { int64_t seconds; int32_t nanosecond; } ODatetime") {
		t.Fatal("duration/Unix slice changed the repaired ODatetime seconds/nanosecond ABI")
	}
	for _, member := range []string{
		"duration", "unix", "unix_milli", "unix_micro",
		"nanosecond", "microsecond", "millisecond", "second", "minute", "hour",
	} {
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/time/"+member)
		if !ok || entry.Native != BackendImplemented {
			t.Fatalf("std/time.%s inventory entry = %+v, want implemented native capability", member, entry)
		}
	}
}
