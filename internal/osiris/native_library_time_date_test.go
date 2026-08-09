package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestNativeLibraryTimeDateAddDateFormatParity covers the smallest complete
// UTC date family represented by the native ABI: exact civil construction,
// calendar normalization, negative Unix-era instants, and the documented
// reference-layout tokens used by std/time.format.
func TestNativeLibraryTimeDateAddDateFormatParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-native-time-date-add-date-format",
		Source: `import "std/time" as time
let leap = time.date(dec("2024.0"), bigint("2"), 29, 23, 59, 58, 123456789, time.utc_location)
let negative = time.date(1969, 12, 31, 23, 59, 59, 500000000, time.utc_location)
let normalized = time.add_date(time.date(2024, 1, 31, 0, 0, 0, 0), 0, 1, 0)
emit time.format(leap, "2006-01-02 15:04:05.999999999 Z07:00")
emit time.format(leap, "Monday, January 2, 2006 3:04:05 PM MST")
emit time.format(negative, time.rfc3339_nano)
emit time.format(normalized, "2006-01-02")
emit time.format(time.add_date(normalized, 0, -1, 0), "2006-01-02")
emit time.format(time.date(1, 1, 1, 0, 0, 0, 0), "2006-01-02")
emit time.format(time.date(9999, 12, 31, 23, 59, 59, 999999999), time.rfc3339_nano)
emit try { time.date(2024, 2, 29, 24, 0, 0, 0) } catch err { "invalid-civil" }
emit try { time.date(2024, 2, 29, 0.5, 0, 0, 0) } catch err { "fractional-civil" }
emit try { time.add_date(time.date(9999, 12, 31, 0, 0, 0, 0), 1, 0, 0) } catch err { "date-overflow" }
`,
	})
}

// TestNativeLibraryTimeDateAddDateFormatContract checks the ABI boundary and
// ownership rules separately from value parity. Datetimes are value-owned
// seconds plus normalized nanoseconds; only format allocates an owned string.
// The current native ABI is intentionally UTC-only because it has no location
// slot, so non-UTC location providers remain a source-spanned unsupported
// boundary rather than being silently treated as UTC.
func TestNativeLibraryTimeDateAddDateFormatContract(t *testing.T) {
	program, err := Parse("<nativeLibrary-native-time-date-contract>", `import "std/time" as time
let stamp = time.date(2024, 7, 8, 9, 10, 11, 125000000, time.utc_location)
emit time.format(stamp, "2006-01-02 15:04:05.000 Z07:00")
emit time.format(time.add_date(stamp, 1, -2, 3), "2006-01-02")
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure std/time date family unexpectedly defines a host capability")
	}
	for _, symbol := range []string{
		"ODatetime", "ostd_time_date", "ostd_time_add_date", "ostd_time_format",
		"ostd_time_datetime", "ocivil_from_days", "otime_append_fraction",
	} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated date-family artifact does not contain %q", symbol)
		}
	}
	for _, message := range []string{
		"date expects integer civil fields", "invalid civil datetime",
		"date location is not representable by the native UTC datetime ABI",
		"add_date result is outside years 1..9999", "format expects datetime and layout strings",
	} {
		if !strings.Contains(artifact.Source, message) {
			t.Fatalf("generated date-family artifact does not preserve %q", message)
		}
	}
	for _, member := range []string{"date", "add_date", "format", "rfc3339", "rfc3339_nano", "utc_location"} {
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/time/"+member)
		if !ok || entry.Native != BackendImplemented {
			t.Fatalf("std/time.%s inventory entry = %+v, want implemented native capability", member, entry)
		}
	}

	start := strings.Index(artifact.Source, "static OValue ostd_time_date(")
	end := strings.Index(artifact.Source[start:], "static OValue ostd_time_parse_duration(")
	if start < 0 || end < 0 {
		t.Fatal("generated date-family helper boundary is incomplete")
	}
	dateHelpers := artifact.Source[start : start+end]
	if strings.Contains(dateHelpers, "malloc(") || strings.Contains(dateHelpers, "realloc(") {
		t.Fatal("date and add_date retain or allocate input storage")
	}
	if !strings.Contains(artifact.Source, "memcpy(copy,data,len)") {
		t.Fatal("format result is not copied into tracked owned string storage")
	}
}

func TestNativeLibraryTimeDateRejectsNonUTCLocationAtSourceBoundary(t *testing.T) {
	program, err := Parse("nativeLibrary-native-time-location-boundary.osr", `import "std/time" as time
let place = time.fixed_zone("TEST", -18000)
emit time.date(2024, 1, 1, 0, 0, 0, 0, place)
`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileC(context.Background(), NewRuntime(), program); err == nil || !strings.Contains(err.Error(), "static module member") {
		t.Fatalf("non-UTC native location error = %v, want source-spanned static module boundary", err)
	}
}
