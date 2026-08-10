package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestNativeLibraryTimeZoneParity covers the UTC-only native location ABI.
// Native datetimes carry no location payload, so every representable value
// reports the same deterministic UTC zone as the interpreter's UTC fixture.
func TestNativeLibraryTimeZoneParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-native-time-zone",
		Source: `import "std/time" as time
emit time.zone(time.unix(0))
emit time.zone(time.date(2024, 2, 29, 12, 34, 56, 789, time.utc_location))
emit try { time.zone() } catch err { "caught-arity" }
emit try { time.zone("not-a-datetime") } catch err { "caught-kind" }
`,
	})
}

// TestNativeLibraryTimeZoneContract verifies direct admission, owned list
// construction, the exact UTC-only error boundary, and capability absence.
func TestNativeLibraryTimeZoneContract(t *testing.T) {
	program, err := Parse("<nativeLibrary-native-time-zone-contract>", `import "std/time" as time
emit time.zone(time.unix(0))
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure std/time.zone unexpectedly defines a host capability")
	}
	for _, symbol := range []string{"ostd_time_zone", "ostd_dispatch", "olist"} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated zone artifact does not contain %q", symbol)
		}
	}
	for _, message := range []string{
		"zone expects 1 datetime",
	} {
		if !strings.Contains(artifact.Source, message) {
			t.Fatalf("generated zone artifact does not preserve %q", message)
		}
	}
	entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/time/zone")
	if !ok || entry.Native != BackendImplemented {
		t.Fatalf("std/time.zone inventory entry = %+v, want implemented native capability", entry)
	}
}
