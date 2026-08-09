package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestNativeTimeRoundTruncateParity covers the pure duration and UTC
// datetime subset of std/time.round and std/time.truncate. The seven-nanosecond
// quantum keeps datetime rounding anchored to the language's year-1 zero time,
// rather than accidentally making Unix epoch the native reference point.
func TestNativeTimeRoundTruncateParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "native-time-round-truncate",
		Source: `import "std/time" as time
let epoch = time.date(1970, 1, 1, 0, 0, 0, 0, time.utc_location)
let one = time.date(1970, 1, 1, 0, 0, 1, 0, time.utc_location)
let one_half = time.date(1970, 1, 1, 0, 0, 1, 500000000, time.utc_location)
let quantum = time.sub(one, epoch)
let half = time.sub(one_half, epoch)
let negative = time.sub(epoch, one_half)
let seven = time.sub(time.date(1970, 1, 1, 0, 0, 0, 7, time.utc_location), epoch)
let negative_quantum = time.sub(epoch, time.date(1970, 1, 1, 0, 0, 0, 1, time.utc_location))
emit time.duration_string(time.round(half, quantum))
emit time.duration_string(time.truncate(negative, quantum))
emit time.duration_string(time.sub(time.round(one_half, quantum), epoch))
emit time.duration_string(time.sub(time.truncate(one_half, quantum), epoch))
emit time.duration_string(time.sub(time.round(epoch, seven), epoch))
emit time.duration_string(time.sub(time.truncate(epoch, seven), epoch))
emit time.duration_string(time.round(half, time.sub(epoch, epoch)))
emit try { time.round("not a duration or datetime", quantum) } catch err { "wrong-value" }
emit try { time.truncate(half, "not a duration") } catch err { "wrong-quantum" }
emit try { time.round(half, negative_quantum) } catch err { "negative-quantum" }
emit try { time.truncate(half) } catch err { "wrong-arity" }
`,
	})
}

// TestNativeTimeRoundTruncateOwnershipCapabilityAndErrorContract checks
// that both functions are admitted individually, return scalar values without
// retaining input storage, and do not introduce a host capability.
func TestNativeTimeRoundTruncateOwnershipCapabilityAndErrorContract(t *testing.T) {
	program, err := Parse("<native-time-round-truncate-contract>", `import "std/time" as time
let epoch = time.date(1970, 1, 1, 0, 0, 0, 0, time.utc_location)
let one = time.date(1970, 1, 1, 0, 0, 1, 0, time.utc_location)
let half = time.sub(time.date(1970, 1, 1, 0, 0, 1, 500000000, time.utc_location), epoch)
let quantum = time.sub(one, epoch)
emit time.round(half, quantum), time.truncate(half, quantum)
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("std/time.round/truncate unexpectedly define a host capability")
	}
	for _, symbol := range []string{
		"ostd_time_round", "ostd_time_round_duration", "ostd_time_round_datetime",
		"ostd_time_zero_remainder", "ostd_dispatch",
	} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated time round/truncate artifact does not contain %q", symbol)
		}
	}
	for _, message := range []string{
		"round quantum must not be negative", "truncate quantum must not be negative",
		"round expects duration or datetime", "truncate expects duration or datetime",
		"duration rounding overflow", "datetime rounding result is outside native range",
	} {
		if !strings.Contains(artifact.Source, message) {
			t.Fatalf("generated time round/truncate artifact does not preserve error contract %q", message)
		}
	}
	start := strings.Index(artifact.Source, "static OValue ostd_time_round(")
	end := strings.Index(artifact.Source[start:], "static int ostd_time_civil")
	if start < 0 || end < 0 {
		t.Fatal("generated time round/truncate helper boundary is incomplete")
	}
	if strings.Contains(artifact.Source[start:start+end], "malloc(") {
		t.Fatal("time round/truncate helper allocates or retains input storage")
	}
	for _, member := range []string{"round", "truncate"} {
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/time/"+member)
		if !ok || entry.Native != BackendImplemented {
			t.Fatalf("std/time.%s inventory entry = %+v, want implemented native capability", member, entry)
		}
		if _, ok := nativeStaticModuleMember("std/time", member); !ok {
			t.Fatalf("std/time.%s is not individually admitted by nativeStaticModuleMember", member)
		}
	}
}
