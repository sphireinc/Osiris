package osiris

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func runTimeProgram(t *testing.T, rt *Runtime, vm bool, source string) ([]Value, error) {
	t.Helper()
	prog, err := Parse("<time-test>", source)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if vm {
		return rt.RunVM(context.Background(), prog)
	}
	return rt.Run(context.Background(), prog)
}

func TestStdTimeInterpreterVMParity(t *testing.T) {
	const source = `
import "std/time" as time
let stamp datetime = time.date(2024, 2, 29, 23, 59, 58, 123456789, time.utc_location)
let later = time.add(stamp, time.add(time.second, time.millisecond))
emit time.duration_string(time.add(time.second, time.millisecond))
emit time.format(stamp, time.rfc3339_nano)
emit time.format(later, time.rfc3339_nano)
emit time.year(stamp), time.month(stamp), time.day(stamp), time.weekday(stamp), time.year_day(stamp)
emit time.zone(stamp)
emit time.equal(time.sub(later, stamp), time.add(time.second, time.millisecond))
emit time.compare(stamp, later), time.before(stamp, later), time.after(later, stamp)
emit time.format(time.add_date(time.date(2024, 1, 31, 0, 0, 0, 0, time.utc_location), 0, 1, 0), "2006-01-02")
`
	var outputs []string
	for _, vm := range []bool{false, true} {
		out, err := runTimeProgram(t, NewRuntime(), vm, source)
		if err != nil {
			t.Fatalf("vm=%v: %v", vm, err)
		}
		outputs = append(outputs, valuesOutput(out))
	}
	if outputs[0] != outputs[1] {
		t.Fatalf("interpreter/VM mismatch:\ninterpreter:\n%s\nVM:\n%s", outputs[0], outputs[1])
	}
	for _, want := range []string{
		`"1.001s"`,
		`"2024-02-29T23:59:58.123456789Z"`,
		`"2024-02-29T23:59:59.124456789Z"`,
		`"2024u64, 2u64, 29u64, Thursday, 60u64"`,
		`["UTC", 0i64]`,
		"true",
		`"-1i64, true, true"`,
		`"2024-03-02"`,
	} {
		if !strings.Contains(outputs[0], want+"\n") {
			t.Fatalf("output missing %q:\n%s", want, outputs[0])
		}
	}
}

func TestStdTimeParseFormatEpochAndZones(t *testing.T) {
	const source = `
import "std/time" as time
let parsed = time.parse(time.rfc3339_nano, "1969-12-31T23:59:59.123456789-05:00")
emit time.format(parsed, time.rfc3339_nano)
emit time.format(time.unix(-1, 500000000), time.rfc3339_nano)
emit time.format(time.unix_milli(-1), time.rfc3339_nano)
emit time.format(time.unix_micro(-1), time.rfc3339_nano)
let fixed = time.fixed_zone("TEST", -18000)
emit time.format(time.in_location(time.unix(0, 0), fixed), time.rfc3339)
emit time.zone(time.in_location(time.unix(0, 0), fixed))
`
	out, err := runTimeProgram(t, NewRuntime(), false, source)
	if err != nil {
		t.Fatal(err)
	}
	got := valuesOutput(out)
	for _, want := range []string{
		`"1969-12-31T23:59:59.123456789-05:00"`,
		`"1969-12-31T23:59:59.5Z"`,
		`"1969-12-31T23:59:59.999Z"`,
		`"1969-12-31T23:59:59.999999Z"`,
		`"1969-12-31T19:00:00-05:00"`,
		`["TEST", -18000i64]`,
	} {
		if !strings.Contains(got, want+"\n") {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestStdTimeDSTGapAndFoldPolicy(t *testing.T) {
	rt := NewRuntime()
	gap := `
import "std/time" as time
let ny = time.load_location("America/New_York")
emit time.date(2024, 3, 10, 2, 30, 0, 0, ny)
`
	if _, err := runTimeProgram(t, rt, false, gap); err == nil {
		t.Fatal("DST gap unexpectedly succeeded")
	} else {
		var rec *OsirisError
		if !errors.As(err, &rec) || rec.kind != ErrorKindInvalidArgument {
			t.Fatalf("gap error = %T %v, want InvalidArgument", err, err)
		}
	}
	parseGap := `
import "std/time" as time
let ny = time.load_location("America/New_York")
emit time.parse("2006-01-02 15:04", "2024-03-10 02:30", ny)
`
	if _, err := runTimeProgram(t, NewRuntime(), false, parseGap); err == nil {
		t.Fatal("parsed DST gap unexpectedly succeeded")
	}

	fold := `
import "std/time" as time
let ny = time.load_location("America/New_York")
let stamp = time.date(2024, 11, 3, 1, 30, 0, 0, ny)
emit time.format(stamp, time.rfc3339)
let parsed = time.parse("2006-01-02 15:04", "2024-11-03 01:30", ny)
emit time.format(parsed, time.rfc3339)
`
	out, err := runTimeProgram(t, NewRuntime(), false, fold)
	if err != nil {
		t.Fatal(err)
	}
	if got := valuesOutput(out); got != "\"2024-11-03T01:30:00-04:00\"\n\"2024-11-03T01:30:00-04:00\"\n" {
		t.Fatalf("fold chose wrong instant: %s", got)
	}
}

func TestStdTimeDurationRangeAndStructuredErrors(t *testing.T) {
	for _, fixture := range []struct {
		source string
		kind   ErrorKind
	}{
		{`import "std/time" as time
emit time.add(time.duration(9223372036854775807u64), time.nanosecond)`, ErrorKindOutOfRange},
		{`import "std/time" as time
emit time.date(2023, 2, 29, 0, 0, 0, 0, time.utc_location)`, ErrorKindInvalidArgument},
		{`import "std/time" as time
emit time.parse(time.rfc3339, "2016-12-31T23:59:60Z")`, ErrorKindParse},
		{`import "std/time" as time
emit time.fixed_zone("bad", 86400)`, ErrorKindOutOfRange},
		{`import "std/time" as time
emit time.load_location("Etc/Definitely_Missing")`, ErrorKindNotFound},
	} {
		_, err := runTimeProgram(t, NewRuntime(), false, fixture.source)
		if err == nil {
			t.Fatalf("invalid time program succeeded:\n%s", fixture.source)
		}
		var rec *OsirisError
		if !errors.As(err, &rec) {
			t.Fatalf("error is not structured: %T %v", err, err)
		}
		if rec.kind != fixture.kind {
			t.Fatalf("error kind = %s, want %s: %v", rec.kind, fixture.kind, err)
		}
		if rec.location == nil || rec.location.Span.start.line == 0 {
			t.Fatalf("structured error lacks source span: %v", err)
		}
	}
}

func TestStdTimeDurationArithmeticRoundingAndTruthiness(t *testing.T) {
	const source = `
import "std/time" as time
let negative = time.duration(-1500000000)
emit time.duration_string(negative)
emit time.duration_string(time.round(negative, time.second))
emit time.duration_string(time.truncate(negative, time.second))
emit time.duration_string(time.sub(time.duration(-9223372036854775808), time.duration(-9223372036854775808)))
emit if(time.duration(0), "bad", "zero")
emit if(time.nanosecond, "nonzero", "bad")
`
	for _, vm := range []bool{false, true} {
		out, err := runTimeProgram(t, NewRuntime(), vm, source)
		if err != nil {
			t.Fatalf("vm=%v: %v", vm, err)
		}
		if got, want := valuesOutput(out), "\"-1.5s\"\n\"-2s\"\n\"-1s\"\n\"0s\"\n\"zero\"\n\"nonzero\"\n"; got != want {
			t.Fatalf("vm=%v output = %q, want %q", vm, got, want)
		}
	}
}

func TestStdTimeSupportedCivilRange(t *testing.T) {
	const source = `
import "std/time" as time
emit time.format(time.date(1, 1, 1, 0, 0, 0, 0), "2006-01-02")
emit time.format(time.date(9999, 12, 31, 23, 59, 59, 999999999), time.rfc3339_nano)
emit time.format(time.date(2000, 2, 29, 0, 0, 0, 0), "2006-01-02")
emit time.format(time.date(1900, 2, 28, 0, 0, 0, 0), "2006-01-02")
`
	out, err := runTimeProgram(t, NewRuntime(), false, source)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := valuesOutput(out), "\"0001-01-01\"\n\"9999-12-31T23:59:59.999999999Z\"\n\"2000-02-29\"\n\"1900-02-28\"\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestStdTimeInjectedClock(t *testing.T) {
	rt := NewRuntime()
	rt.SetClock(func() time.Time {
		return time.Date(2030, time.July, 8, 9, 10, 11, 12, time.FixedZone("ignored", 3600))
	})
	const source = `
import "std/time" as time
emit time.format(time.now(), time.rfc3339_nano)
emit time.zone(time.now())
`
	for _, vm := range []bool{false, true} {
		out, err := runTimeProgram(t, rt, vm, source)
		if err != nil {
			t.Fatalf("vm=%v: %v", vm, err)
		}
		if got := valuesOutput(out); got != "\"2030-07-08T08:10:11.000000012Z\"\n[\"UTC\", 0i64]\n" {
			t.Fatalf("vm=%v output = %q", vm, got)
		}
	}
}

func TestStdTimeLocationLoaderIsInjectable(t *testing.T) {
	rt := NewRuntime()
	loads := 0
	rt.SetTimeLocationLoader(func(name string) (*time.Location, error) {
		loads++
		if name != "Test/Pinned" {
			return nil, errors.New("not in pinned database")
		}
		return time.FixedZone("PIN", 20700), nil
	})
	out, err := runTimeProgram(t, rt, false, `
import "std/time" as time
let place = time.load_location("Test/Pinned")
emit time.format(time.in_location(time.unix(0), place), time.rfc3339)
`)
	if err != nil {
		t.Fatal(err)
	}
	if loads != 1 || valuesOutput(out) != "\"1970-01-01T05:45:00+05:45\"\n" {
		t.Fatalf("loads/output = %d/%q", loads, valuesOutput(out))
	}

	rt.SetClock(func() time.Time { panic("pure time operation consulted the clock") })
	if _, err := runTimeProgram(t, rt, false, `
import "std/time" as time
emit time.format(time.unix(0), time.rfc3339)
`); err != nil {
		t.Fatal(err)
	}
}

func TestStdTimeDependencyReport(t *testing.T) {
	report, err := NewRuntime().BuildDependencyReport("<time-deps>", `import "std/time@1" as time
emit time.rfc3339
`)
	if err != nil {
		t.Fatal(err)
	}
	node, ok := report.Nodes["std/time@1"]
	if !ok {
		// The report may canonicalize the selector away while retaining it on
		// the edge. Accept that representation but still require std/time.
		node, ok = report.Nodes["std/time"]
	}
	if !ok || node.Kind != "stdlib" || node.Version != "1.0.0" {
		t.Fatalf("std/time dependency node = %#v, present=%v; report=%#v", node, ok, report)
	}
}

func TestTimeValueCodecAndJSONRoundTrip(t *testing.T) {
	loc := timeLocationValueOfFixed("TEST", -18000)
	stamp, err := dateTimeValueFromTime(time.Date(2024, time.February, 29, 12, 34, 56, 789, loc.goLocation()), loc)
	if err != nil {
		t.Fatal(err)
	}
	ianaGo, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	iana := timeLocationValueOfIANA("America/New_York", ianaGo)
	ianaStamp, err := dateTimeValueFromTime(time.Date(2024, time.July, 8, 9, 10, 11, 12, ianaGo), iana)
	if err != nil {
		t.Fatal(err)
	}
	values := []Value{durationValue(int64(-1 << 63)), timeLocationValueOf(loc), stamp, timeLocationValueOf(iana), ianaStamp}
	for _, value := range values {
		raw, err := encodeValueBytes(value)
		if err != nil {
			t.Fatalf("encode %s: %v", value.Kind(), err)
		}
		got, err := decodeValueBytes(raw)
		if err != nil {
			t.Fatalf("decode %s: %v", value.Kind(), err)
		}
		if !got.Equal(value) || got.Kind() != value.Kind() {
			t.Fatalf("round trip = %s (%s), want %s (%s)", got, got.Kind(), value, value.Kind())
		}
		if value.kind == kindDateTime && !equalTimeLocations(got.datetime.location, value.datetime.location) {
			t.Fatalf("datetime location identity = %s, want %s", formatTimeLocation(got.datetime.location), formatTimeLocation(value.datetime.location))
		}
		jsonBytes, err := value.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(jsonBytes), value.Kind()) {
			t.Fatalf("%s JSON lacks tagged kind: %s", value.Kind(), jsonBytes)
		}
	}
}

func TestStdTimeInferenceFormatterAndNativeGate(t *testing.T) {
	const source = `import "std/time" as time
let step duration = time.millisecond
let stamp datetime = time.unix(0, 0)
let place location = time.utc_location
let text = time.format(stamp, time.rfc3339)
let cmp = time.compare(stamp, time.add(stamp, step))
emit text, cmp, place
`
	prog, err := Parse("<time-static>", source)
	if err != nil {
		t.Fatal(err)
	}
	formatted := FormatProgram(prog)
	for _, want := range []string{"let step duration =", "let stamp datetime =", "let place location ="} {
		if !strings.Contains(formatted, want) {
			t.Fatalf("formatted source lacks %q:\n%s", want, formatted)
		}
	}
	reparsed, err := Parse("<time-static-formatted>", formatted)
	if err != nil || FormatProgram(reparsed) != formatted {
		t.Fatalf("parse-format-parse failed: %v", err)
	}
	inferred, err := NewRuntime().InferProgram(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	report := FormatInferProgram(inferred)
	for _, want := range []string{
		"main.step: duration",
		"main.stamp: datetime",
		"main.place: location",
		"main.text: string",
		"main.cmp: int64",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("inference lacks %q:\n%s", want, report)
		}
	}
	_, err = CompileC(context.Background(), NewRuntime(), prog)
	if err == nil || !strings.Contains(err.Error(), `<time-static>:2:26:`) || !strings.Contains(err.Error(), `static module member "std/time".millisecond is not supported`) {
		t.Fatalf("native error = %v, want explicit partial std/time gate", err)
	}
	versioned, err := Parse("<time-versioned-native>", "import \"std/time@1\" as time\nemit time.now()\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileC(context.Background(), NewRuntime(), versioned); err == nil || !strings.Contains(err.Error(), `static module`) {
		t.Fatalf("versioned native std/time error = %v", err)
	}
	if strings.Contains(formatted, "2030") {
		t.Fatal("formatting unexpectedly evaluated a time expression")
	}
}
