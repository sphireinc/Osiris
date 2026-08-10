package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestNativeTimeComparisonParity compares the value-only std/time relation
// family across the interpreter, VM, generated C, and native execution. Both
// duration and UTC datetime values are covered, along with the language error
// boundary for mixed kinds and malformed arity.
func TestNativeTimeComparisonParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "native-time-comparison",
		Source: `import "std/time" as time
let zero = time.duration(0)
let one = time.duration(1000000000)
let start = time.date(2024, 2, 29, 23, 59, 58, 123, time.utc_location)
let finish = time.add(start, one)
emit time.compare(zero, one)
emit time.compare(one, zero)
emit time.compare(start, finish)
emit time.equal(zero, zero)
emit time.equal(start, finish)
emit time.before(zero, one)
emit time.before(finish, start)
emit time.after(one, zero)
emit time.after(start, finish)
emit try { time.equal(zero, start) } catch err { "mixed-kind" }
emit try { time.compare(zero) } catch err { "wrong-arity" }
`,
	})
}

// TestNativeTimeComparisonContract verifies direct valid-call lowering,
// value-only ownership, and the absence of a host capability or generic
// dispatcher call for the comparison family.
func TestNativeTimeComparisonContract(t *testing.T) {
	program, err := Parse("<native-time-comparison-contract>", `import "std/time" as time
let zero = time.duration(0)
let one = time.duration(1000000000)
emit time.compare(zero, one), time.equal(zero, zero), time.before(zero, one), time.after(one, zero)
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure std/time comparison lowering unexpectedly defines a host capability")
	}
	for _, symbol := range []string{
		"ostd_time_compare_values", "ostd_time_compare(", "ostd_time_predicate(",
	} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated comparison artifact does not contain %q", symbol)
		}
	}
	for _, member := range []string{"compare", "equal", "before", "after"} {
		if strings.Contains(artifact.Source, `ostd_dispatch("std/time", "`+member+`"`) {
			t.Fatalf("valid std/time.%s call unexpectedly uses the generic dispatcher", member)
		}
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/time/"+member)
		if !ok || entry.Native != BackendImplemented {
			t.Fatalf("std/time.%s inventory entry = %+v, want implemented native capability", member, entry)
		}
	}
	start := strings.Index(artifact.Source, "static int ostd_time_compare_values(")
	end := strings.Index(artifact.Source[start:], "static int oleapyear(")
	if start < 0 || end < 0 {
		t.Fatal("generated comparison helper boundary is incomplete")
	}
	if strings.Contains(artifact.Source[start:start+end], "malloc(") || strings.Contains(artifact.Source[start:start+end], "realloc(") {
		t.Fatal("time comparison helper allocates or retains input storage")
	}
}
