package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestNativeLibraryTimeClockComponentsParity covers the deterministic UTC
// component accessors that were public in std/time but absent from the native
// admission set. The fixture deliberately avoids clocks, locations, and host
// providers while comparing all four execution paths.
func TestNativeLibraryTimeClockComponentsParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-native-time-clock-components",
		Source: `import "std/time" as time
let stamp = time.date(2024, 2, 29, 23, 42, 17, 123456789, time.utc_location)
emit time.hour(stamp), time.minute(stamp), time.second(stamp), time.nanosecond(stamp)
emit try { time.hour("not a datetime") } catch err { "caught" }
emit try { time.nanosecond() } catch err { "caught" }
`,
	})
}

// TestNativeLibraryTimeClockComponentsContract records the individual C
// symbols, structured argument errors, ownership boundary, capability set,
// and backend-inventory status for each accessor.
func TestNativeLibraryTimeClockComponentsContract(t *testing.T) {
	program, err := Parse("<nativeLibrary-native-time-clock-components-contract>", `import "std/time" as time
let stamp = time.date(2024, 2, 29, 23, 42, 17, 123456789, time.utc_location)
emit time.hour(stamp), time.minute(stamp), time.second(stamp), time.nanosecond(stamp)
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure std/time component accessors unexpectedly define a host capability")
	}
	for _, symbol := range []string{
		"ostd_time_component", "ostd_time_civil", "ocivil_from_days", "ostd_dispatch",
	} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated time component artifact does not contain %q", symbol)
		}
	}
	for _, message := range []string{
		"hour expects 1 datetime", "minute expects 1 datetime",
		"second expects 1 datetime", "nanosecond expects 1 datetime",
	} {
		if !strings.Contains(artifact.Source, message) {
			t.Fatalf("generated time component artifact does not preserve error contract %q", message)
		}
	}
	for _, member := range []string{"hour", "minute", "second", "nanosecond"} {
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/time/"+member)
		if !ok || entry.Native != BackendImplemented {
			t.Fatalf("std/time.%s inventory entry = %+v, want implemented native capability", member, entry)
		}
	}
}
