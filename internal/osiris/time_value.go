package osiris

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

type timeLocationClass uint8

const (
	timeLocationUTC timeLocationClass = iota
	timeLocationFixed
	timeLocationIANA
)

type timeLocationValue struct {
	class  timeLocationClass
	name   string
	offset int
	loc    *time.Location
}

type dateTimeValue struct {
	unixSeconds int64
	nanosecond  int32
	location    *timeLocationValue
}

func utcTimeLocation() *timeLocationValue {
	return &timeLocationValue{class: timeLocationUTC, name: "UTC", loc: time.UTC}
}

func timeLocationValueOfFixed(name string, offset int) *timeLocationValue {
	return &timeLocationValue{class: timeLocationFixed, name: name, offset: offset, loc: time.FixedZone(name, offset)}
}

func timeLocationValueOfIANA(name string, loc *time.Location) *timeLocationValue {
	return &timeLocationValue{class: timeLocationIANA, name: name, loc: loc}
}

func (v *timeLocationValue) clone() *timeLocationValue {
	if v == nil {
		return utcTimeLocation()
	}
	return &timeLocationValue{class: v.class, name: v.name, offset: v.offset, loc: v.goLocation()}
}

func (v *timeLocationValue) goLocation() *time.Location {
	if v == nil || v.loc == nil {
		return time.UTC
	}
	return v.loc
}

func (v *dateTimeValue) clone() *dateTimeValue {
	if v == nil {
		return nil
	}
	return &dateTimeValue{
		unixSeconds: v.unixSeconds,
		nanosecond:  v.nanosecond,
		location:    v.location.clone(),
	}
}

func timeFailure(kind ErrorKind, format string, args ...any) error {
	return newOsirisError(kind, fmt.Sprintf(format, args...))
}

func structuredErrorAt(err error, source string, span sourceSpan) error {
	if err == nil {
		return nil
	}
	var record *OsirisError
	if !errors.As(err, &record) || record == nil {
		return err
	}
	if record.location != nil && record.location.Span.start.line != 0 {
		return err
	}
	return record.withSource(source, span)
}

func compareInt64(left, right int64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func compareDateTimeValues(left, right *dateTimeValue) int {
	if left == nil || right == nil {
		switch {
		case left == nil && right == nil:
			return 0
		case left == nil:
			return -1
		default:
			return 1
		}
	}
	if cmp := compareInt64(left.unixSeconds, right.unixSeconds); cmp != 0 {
		return cmp
	}
	return compareInt64(int64(left.nanosecond), int64(right.nanosecond))
}

func equalTimeLocations(left, right *timeLocationValue) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.class == right.class && left.name == right.name && left.offset == right.offset
}

func formatDuration(nanoseconds int64) string {
	return time.Duration(nanoseconds).String()
}

func formatTimeLocation(location *timeLocationValue) string {
	if location == nil || location.class == timeLocationUTC {
		return `location("UTC")`
	}
	if location.class == timeLocationFixed {
		return `fixed_zone(` + strconv.Quote(location.name) + `, ` + strconv.Itoa(location.offset) + `)`
	}
	return `location(` + strconv.Quote(location.name) + `)`
}

func (v *dateTimeValue) asTime() time.Time {
	if v == nil {
		return time.Unix(0, 0).UTC()
	}
	return time.Unix(v.unixSeconds, int64(v.nanosecond)).In(v.location.goLocation())
}

func formatDateTimeText(value *dateTimeValue) string {
	return value.asTime().Format(time.RFC3339Nano)
}

func formatDateTimeValue(value *dateTimeValue) string {
	if value == nil {
		return `datetime("1970-01-01T00:00:00Z")`
	}
	text := strconv.Quote(formatDateTimeText(value))
	if value.location != nil && value.location.class == timeLocationIANA {
		return `datetime(` + text + `, ` + strconv.Quote(value.location.name) + `)`
	}
	if value.location != nil && value.location.class == timeLocationFixed {
		return `datetime(` + text + `, ` + formatTimeLocation(value.location) + `)`
	}
	return `datetime(` + text + `)`
}

func timeLocationInterface(location *timeLocationValue) map[string]any {
	if location == nil {
		location = utcTimeLocation()
	}
	body := map[string]any{"name": location.name}
	switch location.class {
	case timeLocationUTC:
		body["kind"] = "utc"
	case timeLocationFixed:
		body["kind"] = "fixed"
		body["offset_seconds"] = strconv.Itoa(location.offset)
	case timeLocationIANA:
		body["kind"] = "iana"
	}
	return map[string]any{"location": body}
}

func dateTimeInterface(value *dateTimeValue) map[string]any {
	if value == nil {
		value = &dateTimeValue{location: utcTimeLocation()}
	}
	return map[string]any{"datetime": map[string]any{
		"unix_seconds": value.unixSecondsString(),
		"nanosecond":   value.nanosecond,
		"location":     timeLocationInterface(value.location)["location"],
	}}
}

func (v *dateTimeValue) unixSecondsString() string {
	if v == nil {
		return "0"
	}
	return strconv.FormatInt(v.unixSeconds, 10)
}

func dateTimeValueFromTime(value time.Time, location *timeLocationValue) (Value, error) {
	if location == nil {
		location = utcTimeLocation()
	}
	local := value.In(location.goLocation())
	if local.Year() < 1 || local.Year() > 9999 {
		return nullValue(), timeFailure(ErrorKindOutOfRange, "datetime year %d is outside 1..9999", local.Year())
	}
	return dateTimeValueOf(&dateTimeValue{
		unixSeconds: value.Unix(),
		nanosecond:  int32(value.Nanosecond()),
		location:    location,
	}), nil
}

func expectDuration(value Value, operation string) (int64, error) {
	if value.kind != kindDuration {
		return 0, timeFailure(ErrorKindType, "%s expects duration, got %s", operation, value.Kind())
	}
	return value.duration, nil
}

func expectDateTime(value Value, operation string) (*dateTimeValue, error) {
	if value.kind != kindDateTime || value.datetime == nil {
		return nil, timeFailure(ErrorKindType, "%s expects datetime, got %s", operation, value.Kind())
	}
	return value.datetime, nil
}

func expectTimeLocation(value Value, operation string) (*timeLocationValue, error) {
	if value.kind != kindTimeLocation || value.location == nil {
		return nil, timeFailure(ErrorKindType, "%s expects location, got %s", operation, value.Kind())
	}
	return value.location, nil
}

func exactInteger(value Value, name string) (*big.Int, error) {
	switch value.kind {
	case kindInt:
		if value.i == nil {
			return new(big.Int), nil
		}
		return new(big.Int).Set(value.i), nil
	case kindUint:
		if value.ub != nil {
			return new(big.Int).Set(value.ub), nil
		}
		return new(big.Int).SetUint64(value.u), nil
	case kindBigInt:
		if value.bi == nil {
			return new(big.Int), nil
		}
		return new(big.Int).Set(value.bi), nil
	case kindDecimal:
		if value.dec == nil {
			return new(big.Int), nil
		}
		if value.dec.Denom().Cmp(big.NewInt(1)) != 0 {
			return nil, timeFailure(ErrorKindType, "%s must be an exact integer, got %s", name, value.String())
		}
		return new(big.Int).Set(value.dec.Num()), nil
	case kindFixed:
		rational := fixedToRat(value.fx)
		if rational.Denom().Cmp(big.NewInt(1)) != 0 {
			return nil, timeFailure(ErrorKindType, "%s must be an exact integer, got %s", name, value.String())
		}
		return new(big.Int).Set(rational.Num()), nil
	case kindBigFloat:
		if value.bf == nil {
			return new(big.Int), nil
		}
		integer, accuracy := value.bf.Int(nil)
		if accuracy != big.Exact {
			return nil, timeFailure(ErrorKindType, "%s must be an exact integer, got %s", name, value.String())
		}
		return integer, nil
	case kindNumber:
		if math.IsNaN(value.num) || math.IsInf(value.num, 0) || math.Trunc(value.num) != value.num {
			return nil, timeFailure(ErrorKindType, "%s must be an exact integer, got %s", name, value.String())
		}
		integer, _ := new(big.Float).SetFloat64(value.num).Int(nil)
		return integer, nil
	default:
		return nil, timeFailure(ErrorKindType, "%s must be an integer, got %s", name, value.Kind())
	}
}

func exactInt64(value Value, name string) (int64, error) {
	integer, err := exactInteger(value, name)
	if err != nil {
		return 0, err
	}
	if !integer.IsInt64() {
		return 0, timeFailure(ErrorKindOutOfRange, "%s is outside signed 64-bit range", name)
	}
	return integer.Int64(), nil
}

func exactInt(value Value, name string) (int, error) {
	integer, err := exactInt64(value, name)
	if err != nil {
		return 0, err
	}
	if int64(int(integer)) != integer {
		return 0, timeFailure(ErrorKindOutOfRange, "%s is outside host integer range", name)
	}
	return int(integer), nil
}

func durationNanoseconds(value Value) (int64, error) {
	if value.kind == kindDuration {
		return value.duration, nil
	}
	if value.kind == kindQuantity {
		if value.qty == nil || !dimsEqual(value.qty.dims, map[string]int{"Second": 1}) {
			return 0, timeFailure(ErrorKindType, "duration quantity must have exactly one Second dimension")
		}
		scaled := new(big.Rat).Mul(value.qty.mag, new(big.Rat).SetInt64(int64(time.Second)))
		if scaled.Denom().Cmp(big.NewInt(1)) != 0 {
			return 0, timeFailure(ErrorKindInvalidArgument, "duration quantity is not an exact nanosecond count")
		}
		if !scaled.Num().IsInt64() {
			return 0, timeFailure(ErrorKindOutOfRange, "duration is outside signed 64-bit nanoseconds")
		}
		return scaled.Num().Int64(), nil
	}
	return exactInt64(value, "duration nanoseconds")
}

func timeDuration(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), timeFailure(ErrorKindInvalidArgument, "duration expects 1 argument")
	}
	nanoseconds, err := durationNanoseconds(args[0])
	if err != nil {
		return nullValue(), err
	}
	return durationValue(nanoseconds), nil
}

func timeParseDuration(args []Value) (Value, error) {
	if len(args) != 1 || args[0].kind != kindString {
		return nullValue(), timeFailure(ErrorKindType, "parse_duration expects 1 string argument")
	}
	value, err := time.ParseDuration(args[0].str)
	if err != nil {
		return nullValue(), timeFailure(ErrorKindParse, "cannot parse duration %q: %v", args[0].str, err)
	}
	return durationValue(int64(value)), nil
}

func timeDurationString(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), timeFailure(ErrorKindInvalidArgument, "duration_string expects 1 argument")
	}
	value, err := expectDuration(args[0], "duration_string")
	if err != nil {
		return nullValue(), err
	}
	return stringValue(formatDuration(value)), nil
}

func validateCivilFields(year, month, day, hour, minute, second, nanosecond int) error {
	if year < 1 || year > 9999 {
		return timeFailure(ErrorKindOutOfRange, "year %d is outside 1..9999", year)
	}
	if month < 1 || month > 12 {
		return timeFailure(ErrorKindInvalidArgument, "month %d is outside 1..12", month)
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 || second < 0 || second > 59 || nanosecond < 0 || nanosecond > 999999999 {
		return timeFailure(ErrorKindInvalidArgument, "invalid civil clock fields")
	}
	days := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if day < 1 || day > days {
		return timeFailure(ErrorKindInvalidArgument, "day %d is invalid for %04d-%02d", day, year, month)
	}
	return nil
}

func sameCivil(value time.Time, year, month, day, hour, minute, second, nanosecond int) bool {
	return value.Year() == year && int(value.Month()) == month && value.Day() == day &&
		value.Hour() == hour && value.Minute() == minute && value.Second() == second &&
		value.Nanosecond() == nanosecond
}

func resolveCivil(year, month, day, hour, minute, second, nanosecond int, location *timeLocationValue) (time.Time, error) {
	if err := validateCivilFields(year, month, day, hour, minute, second, nanosecond); err != nil {
		return time.Time{}, err
	}
	if location == nil {
		location = utcTimeLocation()
	}
	goLocation := location.goLocation()
	if location.class != timeLocationIANA {
		value := time.Date(year, time.Month(month), day, hour, minute, second, nanosecond, goLocation)
		if !sameCivil(value, year, month, day, hour, minute, second, nanosecond) {
			return time.Time{}, timeFailure(ErrorKindInvalidArgument, "civil time is not representable in %s", location.name)
		}
		return value, nil
	}

	// Derive all offsets near the requested wall time, then retain the
	// matching instant with the smallest Unix time. This rejects gaps and
	// defines folds as the earlier occurrence without relying on time.Date's
	// undocumented transition choice.
	naive := time.Date(year, time.Month(month), day, hour, minute, second, nanosecond, time.UTC)
	offsets := make(map[int]struct{})
	for delta := -48; delta <= 48; delta++ {
		_, offset := naive.Add(time.Duration(delta) * time.Hour).In(goLocation).Zone()
		offsets[offset] = struct{}{}
	}
	var chosen time.Time
	found := false
	for offset := range offsets {
		candidate := naive.Add(-time.Duration(offset) * time.Second).In(goLocation)
		if !sameCivil(candidate, year, month, day, hour, minute, second, nanosecond) {
			continue
		}
		if !found || candidate.Before(chosen) {
			chosen = candidate
			found = true
		}
	}
	if !found {
		return time.Time{}, timeFailure(ErrorKindInvalidArgument, "civil time falls in a timezone gap in %s", location.name)
	}
	return chosen, nil
}

func timeDate(args []Value) (Value, error) {
	if len(args) != 7 && len(args) != 8 {
		return nullValue(), timeFailure(ErrorKindInvalidArgument, "date expects 7 fields and an optional location")
	}
	fields := make([]int, 7)
	names := []string{"year", "month", "day", "hour", "minute", "second", "nanosecond"}
	for index := range fields {
		value, err := exactInt(args[index], names[index])
		if err != nil {
			return nullValue(), err
		}
		fields[index] = value
	}
	location := utcTimeLocation()
	if len(args) == 8 {
		var err error
		location, err = expectTimeLocation(args[7], "date")
		if err != nil {
			return nullValue(), err
		}
	}
	value, err := resolveCivil(fields[0], fields[1], fields[2], fields[3], fields[4], fields[5], fields[6], location)
	if err != nil {
		return nullValue(), err
	}
	return dateTimeValueFromTime(value, location)
}

func normalizedUnix(seconds, nanoseconds *big.Int) (int64, int64, error) {
	total := new(big.Int).Mul(seconds, big.NewInt(int64(time.Second)))
	total.Add(total, nanoseconds)
	quotient, remainder := new(big.Int).QuoRem(total, big.NewInt(int64(time.Second)), new(big.Int))
	if remainder.Sign() < 0 {
		quotient.Sub(quotient, big.NewInt(1))
		remainder.Add(remainder, big.NewInt(int64(time.Second)))
	}
	if !quotient.IsInt64() {
		return 0, 0, timeFailure(ErrorKindOutOfRange, "Unix timestamp is outside signed 64-bit seconds")
	}
	return quotient.Int64(), remainder.Int64(), nil
}

func timeUnix(args []Value) (Value, error) {
	if len(args) != 1 && len(args) != 2 {
		return nullValue(), timeFailure(ErrorKindInvalidArgument, "unix expects seconds and optional nanoseconds")
	}
	seconds, err := exactInteger(args[0], "Unix seconds")
	if err != nil {
		return nullValue(), err
	}
	nanoseconds := new(big.Int)
	if len(args) == 2 {
		nanoseconds, err = exactInteger(args[1], "Unix nanoseconds")
		if err != nil {
			return nullValue(), err
		}
	}
	second, nano, err := normalizedUnix(seconds, nanoseconds)
	if err != nil {
		return nullValue(), err
	}
	return dateTimeValueFromTime(time.Unix(second, nano).UTC(), utcTimeLocation())
}

func timeUnixScaled(args []Value, scale int64, name string) (Value, error) {
	if len(args) != 1 {
		return nullValue(), timeFailure(ErrorKindInvalidArgument, "%s expects 1 integer argument", name)
	}
	value, err := exactInteger(args[0], name)
	if err != nil {
		return nullValue(), err
	}
	total := new(big.Int).Mul(value, big.NewInt(scale))
	second, nano, err := normalizedUnix(new(big.Int), total)
	if err != nil {
		return nullValue(), err
	}
	return dateTimeValueFromTime(time.Unix(second, nano).UTC(), utcTimeLocation())
}

func timeUnixMilli(args []Value) (Value, error) {
	return timeUnixScaled(args, int64(time.Millisecond), "unix_milli")
}

func timeUnixMicro(args []Value) (Value, error) {
	return timeUnixScaled(args, int64(time.Microsecond), "unix_micro")
}

func parsedTimeLocation(value time.Time, preferred *timeLocationValue) *timeLocationValue {
	if preferred != nil {
		name, offset := value.Zone()
		preferredName, preferredOffset := value.In(preferred.goLocation()).Zone()
		if name == preferredName && offset == preferredOffset {
			return preferred
		}
	}
	if value.Location() == time.UTC {
		return utcTimeLocation()
	}
	name, offset := value.Zone()
	if name == "UTC" && offset == 0 {
		return utcTimeLocation()
	}
	if name == "" {
		name = formatNumericOffset(offset)
	}
	return timeLocationValueOfFixed(name, offset)
}

func formatNumericOffset(offset int) string {
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	return fmt.Sprintf("%s%02d:%02d", sign, offset/3600, (offset%3600)/60)
}

func timeParse(args []Value) (Value, error) {
	if len(args) != 2 && len(args) != 3 {
		return nullValue(), timeFailure(ErrorKindInvalidArgument, "parse expects layout, text, and optional location")
	}
	if args[0].kind != kindString || args[1].kind != kindString {
		return nullValue(), timeFailure(ErrorKindType, "parse layout and text must be strings")
	}
	location := utcTimeLocation()
	if len(args) == 3 {
		var err error
		location, err = expectTimeLocation(args[2], "parse")
		if err != nil {
			return nullValue(), err
		}
	}
	value, err := time.ParseInLocation(args[0].str, args[1].str, location.goLocation())
	if err != nil {
		return nullValue(), timeFailure(ErrorKindParse, "cannot parse datetime %q: %v", args[1].str, err)
	}
	if location.class == timeLocationIANA && value.Format(args[0].str) != args[1].str {
		return nullValue(), timeFailure(ErrorKindParse, "datetime %q is not canonical for layout %q or falls in a timezone gap", args[1].str, args[0].str)
	}
	resultLocation := parsedTimeLocation(value, location)
	if resultLocation.class == timeLocationIANA {
		value, err = resolveCivil(value.Year(), int(value.Month()), value.Day(), value.Hour(), value.Minute(), value.Second(), value.Nanosecond(), resultLocation)
		if err != nil {
			return nullValue(), err
		}
	}
	return dateTimeValueFromTime(value, resultLocation)
}

func timeFormat(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), timeFailure(ErrorKindInvalidArgument, "format expects datetime and layout")
	}
	value, err := expectDateTime(args[0], "format")
	if err != nil {
		return nullValue(), err
	}
	if args[1].kind != kindString {
		return nullValue(), timeFailure(ErrorKindType, "format layout must be string")
	}
	return stringValue(value.asTime().Format(args[1].str)), nil
}

func checkedDurationAdd(left, right int64) (int64, error) {
	result := new(big.Int).Add(big.NewInt(left), big.NewInt(right))
	if !result.IsInt64() {
		return 0, timeFailure(ErrorKindOutOfRange, "duration arithmetic overflow")
	}
	return result.Int64(), nil
}

func timeAdd(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), timeFailure(ErrorKindInvalidArgument, "add expects 2 arguments")
	}
	switch args[0].kind {
	case kindDuration:
		right, err := expectDuration(args[1], "add")
		if err != nil {
			return nullValue(), err
		}
		result, err := checkedDurationAdd(args[0].duration, right)
		if err != nil {
			return nullValue(), err
		}
		return durationValue(result), nil
	case kindDateTime:
		duration, err := expectDuration(args[1], "add")
		if err != nil {
			return nullValue(), err
		}
		value := args[0].datetime
		return dateTimeValueFromTime(value.asTime().Add(time.Duration(duration)), value.location)
	default:
		return nullValue(), timeFailure(ErrorKindType, "add expects duration+duration or datetime+duration")
	}
}

func datetimeDifference(left, right *dateTimeValue) (int64, error) {
	seconds := new(big.Int).Sub(big.NewInt(left.unixSeconds), big.NewInt(right.unixSeconds))
	total := new(big.Int).Mul(seconds, big.NewInt(int64(time.Second)))
	total.Add(total, big.NewInt(int64(left.nanosecond)-int64(right.nanosecond)))
	if !total.IsInt64() {
		return 0, timeFailure(ErrorKindOutOfRange, "datetime difference is outside duration range")
	}
	return total.Int64(), nil
}

func timeSub(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), timeFailure(ErrorKindInvalidArgument, "sub expects 2 arguments")
	}
	switch args[0].kind {
	case kindDuration:
		right, err := expectDuration(args[1], "sub")
		if err != nil {
			return nullValue(), err
		}
		result := new(big.Int).Sub(big.NewInt(args[0].duration), big.NewInt(right))
		if !result.IsInt64() {
			return nullValue(), timeFailure(ErrorKindOutOfRange, "duration subtraction overflow")
		}
		return durationValue(result.Int64()), nil
	case kindDateTime:
		right, err := expectDateTime(args[1], "sub")
		if err != nil {
			return nullValue(), err
		}
		result, err := datetimeDifference(args[0].datetime, right)
		if err != nil {
			return nullValue(), err
		}
		return durationValue(result), nil
	default:
		return nullValue(), timeFailure(ErrorKindType, "sub expects duration-duration or datetime-datetime")
	}
}

func timeAddDate(args []Value) (Value, error) {
	if len(args) != 4 {
		return nullValue(), timeFailure(ErrorKindInvalidArgument, "add_date expects datetime, years, months, and days")
	}
	value, err := expectDateTime(args[0], "add_date")
	if err != nil {
		return nullValue(), err
	}
	deltas := make([]int, 3)
	for index, name := range []string{"years", "months", "days"} {
		deltas[index], err = exactInt(args[index+1], name)
		if err != nil {
			return nullValue(), err
		}
	}
	return dateTimeValueFromTime(value.asTime().AddDate(deltas[0], deltas[1], deltas[2]), value.location)
}

func roundDuration(value, quantum int64, truncate bool) (int64, error) {
	if quantum < 0 {
		return 0, timeFailure(ErrorKindInvalidArgument, "rounding quantum must not be negative")
	}
	if quantum == 0 {
		return value, nil
	}
	divisor := big.NewInt(quantum)
	quotient, remainder := new(big.Int).QuoRem(big.NewInt(value), divisor, new(big.Int))
	if !truncate {
		twice := new(big.Int).Lsh(new(big.Int).Abs(new(big.Int).Set(remainder)), 1)
		if twice.Cmp(divisor) >= 0 {
			if value < 0 {
				quotient.Sub(quotient, big.NewInt(1))
			} else {
				quotient.Add(quotient, big.NewInt(1))
			}
		}
	}
	result := new(big.Int).Mul(quotient, divisor)
	if !result.IsInt64() {
		return 0, timeFailure(ErrorKindOutOfRange, "duration rounding overflow")
	}
	return result.Int64(), nil
}

func timeRoundLike(args []Value, truncate bool) (Value, error) {
	name := "round"
	if truncate {
		name = "truncate"
	}
	if len(args) != 2 {
		return nullValue(), timeFailure(ErrorKindInvalidArgument, "%s expects a value and duration quantum", name)
	}
	quantum, err := expectDuration(args[1], name)
	if err != nil {
		return nullValue(), err
	}
	if quantum < 0 {
		return nullValue(), timeFailure(ErrorKindInvalidArgument, "%s quantum must not be negative", name)
	}
	switch args[0].kind {
	case kindDuration:
		result, err := roundDuration(args[0].duration, quantum, truncate)
		if err != nil {
			return nullValue(), err
		}
		return durationValue(result), nil
	case kindDateTime:
		value := args[0].datetime
		current := value.asTime()
		if truncate {
			current = current.Truncate(time.Duration(quantum))
		} else {
			current = current.Round(time.Duration(quantum))
		}
		return dateTimeValueFromTime(current, value.location)
	default:
		return nullValue(), timeFailure(ErrorKindType, "%s expects duration or datetime", name)
	}
}

func timeRound(args []Value) (Value, error) {
	return timeRoundLike(args, false)
}

func timeTruncate(args []Value) (Value, error) {
	return timeRoundLike(args, true)
}

func timeCompareValues(args []Value) (int, error) {
	if len(args) != 2 {
		return 0, timeFailure(ErrorKindInvalidArgument, "time comparison expects 2 arguments")
	}
	switch args[0].kind {
	case kindDuration:
		right, err := expectDuration(args[1], "comparison")
		if err != nil {
			return 0, err
		}
		return compareInt64(args[0].duration, right), nil
	case kindDateTime:
		right, err := expectDateTime(args[1], "comparison")
		if err != nil {
			return 0, err
		}
		return compareDateTimeValues(args[0].datetime, right), nil
	default:
		return 0, timeFailure(ErrorKindType, "time comparison expects two durations or two datetimes")
	}
}

func timeEqual(args []Value) (Value, error) {
	comparison, err := timeCompareValues(args)
	return boolValue(comparison == 0), err
}

func timeBefore(args []Value) (Value, error) {
	comparison, err := timeCompareValues(args)
	return boolValue(comparison < 0), err
}

func timeAfter(args []Value) (Value, error) {
	comparison, err := timeCompareValues(args)
	return boolValue(comparison > 0), err
}

func timeCompare(args []Value) (Value, error) {
	comparison, err := timeCompareValues(args)
	if err != nil {
		return nullValue(), err
	}
	return intValue(big.NewInt(int64(comparison)), 64), nil
}

func timeComponent(args []Value, name string, extract func(time.Time) int) (Value, error) {
	if len(args) != 1 {
		return nullValue(), timeFailure(ErrorKindInvalidArgument, "%s expects 1 datetime", name)
	}
	value, err := expectDateTime(args[0], name)
	if err != nil {
		return nullValue(), err
	}
	return uintValue(uint64(extract(value.asTime())), 64), nil
}

func timeYear(args []Value) (Value, error) {
	return timeComponent(args, "year", func(value time.Time) int { return value.Year() })
}

func timeMonth(args []Value) (Value, error) {
	return timeComponent(args, "month", func(value time.Time) int { return int(value.Month()) })
}

func timeDay(args []Value) (Value, error) {
	return timeComponent(args, "day", func(value time.Time) int { return value.Day() })
}

func timeHour(args []Value) (Value, error) {
	return timeComponent(args, "hour", func(value time.Time) int { return value.Hour() })
}

func timeMinute(args []Value) (Value, error) {
	return timeComponent(args, "minute", func(value time.Time) int { return value.Minute() })
}

func timeSecond(args []Value) (Value, error) {
	return timeComponent(args, "second", func(value time.Time) int { return value.Second() })
}

func timeNanosecond(args []Value) (Value, error) {
	return timeComponent(args, "nanosecond", func(value time.Time) int { return value.Nanosecond() })
}

func timeYearDay(args []Value) (Value, error) {
	return timeComponent(args, "year_day", func(value time.Time) int { return value.YearDay() })
}

func timeWeekday(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), timeFailure(ErrorKindInvalidArgument, "weekday expects 1 datetime")
	}
	value, err := expectDateTime(args[0], "weekday")
	if err != nil {
		return nullValue(), err
	}
	return stringValue(value.asTime().Weekday().String()), nil
}

func timeZone(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), timeFailure(ErrorKindInvalidArgument, "zone expects 1 datetime")
	}
	value, err := expectDateTime(args[0], "zone")
	if err != nil {
		return nullValue(), err
	}
	name, offset := value.asTime().Zone()
	return listValue([]Value{stringValue(name), intValue(big.NewInt(int64(offset)), 64)}), nil
}

func timeUTC(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), timeFailure(ErrorKindInvalidArgument, "utc expects 1 datetime")
	}
	value, err := expectDateTime(args[0], "utc")
	if err != nil {
		return nullValue(), err
	}
	return dateTimeValueFromTime(value.asTime().UTC(), utcTimeLocation())
}

func timeInLocation(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), timeFailure(ErrorKindInvalidArgument, "in_location expects datetime and location")
	}
	value, err := expectDateTime(args[0], "in_location")
	if err != nil {
		return nullValue(), err
	}
	location, err := expectTimeLocation(args[1], "in_location")
	if err != nil {
		return nullValue(), err
	}
	return dateTimeValueFromTime(value.asTime().In(location.goLocation()), location)
}

func timeFixedZone(args []Value) (Value, error) {
	if len(args) != 2 || args[0].kind != kindString {
		return nullValue(), timeFailure(ErrorKindType, "fixed_zone expects a non-empty name string and offset seconds")
	}
	if strings.TrimSpace(args[0].str) == "" {
		return nullValue(), timeFailure(ErrorKindInvalidArgument, "fixed_zone name must not be empty")
	}
	offset, err := exactInt(args[1], "fixed_zone offset")
	if err != nil {
		return nullValue(), err
	}
	if offset <= -24*60*60 || offset >= 24*60*60 {
		return nullValue(), timeFailure(ErrorKindOutOfRange, "fixed_zone offset must be strictly between -86400 and 86400 seconds")
	}
	return timeLocationValueOf(timeLocationValueOfFixed(args[0].str, offset)), nil
}

func timeLoadLocation(rt *Runtime, args []Value) (Value, error) {
	if len(args) != 1 || args[0].kind != kindString {
		return nullValue(), timeFailure(ErrorKindType, "load_location expects 1 string argument")
	}
	name := args[0].str
	if name == "" {
		return nullValue(), timeFailure(ErrorKindInvalidArgument, "location name must not be empty")
	}
	if name == "UTC" {
		return timeLocationValueOf(utcTimeLocation()), nil
	}
	location, err := rt.loadTimeLocation(name)
	if err != nil {
		return nullValue(), timeFailure(ErrorKindNotFound, "timezone %q is unavailable: %v", name, err)
	}
	return timeLocationValueOf(timeLocationValueOfIANA(name, location)), nil
}

func timeNow(rt *Runtime, args []Value) (Value, error) {
	if len(args) != 0 {
		return nullValue(), timeFailure(ErrorKindInvalidArgument, "now expects no arguments")
	}
	return dateTimeValueFromTime(rt.clockNow().UTC(), utcTimeLocation())
}

// callDurationComponent resolves the four std/time names that deliberately
// serve both as duration constants and component accessors. Member selection
// produces an ordinary duration; using that value in call position dispatches
// by its exact unit value. The rule survives codecs because it is derived only
// from the duration's public nanosecond count.
func callDurationComponent(callee Value, args []Value) (Value, error) {
	switch callee.duration {
	case int64(time.Nanosecond):
		return timeNanosecond(args)
	case int64(time.Second):
		return timeSecond(args)
	case int64(time.Minute):
		return timeMinute(args)
	case int64(time.Hour):
		return timeHour(args)
	default:
		return nullValue(), timeFailure(ErrorKindType, "duration %s is not a datetime component accessor", formatDuration(callee.duration))
	}
}

func stdlibTimeModule(rt *Runtime, selector string) (Value, error) {
	const (
		name    = "std/time"
		version = "1.0.0"
	)
	if !versionMatchesSelector(version, selector) {
		return nullValue(), fmt.Errorf("stdlib module %s version %s does not satisfy %q", name, version, selector)
	}
	return makeModule(name, version, map[string]Value{
		"duration":        functionValueOf("std/time.duration", timeDuration),
		"parse_duration":  functionValueOf("std/time.parse_duration", timeParseDuration),
		"duration_string": functionValueOf("std/time.duration_string", timeDurationString),
		"date":            functionValueOf("std/time.date", timeDate),
		"unix":            functionValueOf("std/time.unix", timeUnix),
		"unix_milli":      functionValueOf("std/time.unix_milli", timeUnixMilli),
		"unix_micro":      functionValueOf("std/time.unix_micro", timeUnixMicro),
		"parse":           functionValueOf("std/time.parse", timeParse),
		"format":          functionValueOf("std/time.format", timeFormat),
		"add":             functionValueOf("std/time.add", timeAdd),
		"sub":             functionValueOf("std/time.sub", timeSub),
		"add_date":        functionValueOf("std/time.add_date", timeAddDate),
		"round":           functionValueOf("std/time.round", timeRound),
		"truncate":        functionValueOf("std/time.truncate", timeTruncate),
		"equal":           functionValueOf("std/time.equal", timeEqual),
		"before":          functionValueOf("std/time.before", timeBefore),
		"after":           functionValueOf("std/time.after", timeAfter),
		"compare":         functionValueOf("std/time.compare", timeCompare),
		"year":            functionValueOf("std/time.year", timeYear),
		"month":           functionValueOf("std/time.month", timeMonth),
		"day":             functionValueOf("std/time.day", timeDay),
		"weekday":         functionValueOf("std/time.weekday", timeWeekday),
		"year_day":        functionValueOf("std/time.year_day", timeYearDay),
		"zone":            functionValueOf("std/time.zone", timeZone),
		"utc":             functionValueOf("std/time.utc", timeUTC),
		"in_location":     functionValueOf("std/time.in_location", timeInLocation),
		"fixed_zone":      functionValueOf("std/time.fixed_zone", timeFixedZone),
		"load_location": functionValueOf("std/time.load_location", func(args []Value) (Value, error) {
			return timeLoadLocation(rt, args)
		}),
		"now": functionValueOf("std/time.now", func(args []Value) (Value, error) {
			return timeNow(rt, args)
		}),
		"rfc3339":      stringValue(time.RFC3339),
		"rfc3339_nano": stringValue(time.RFC3339Nano),
		"utc_location": timeLocationValueOf(utcTimeLocation()),
		"nanosecond":   durationValue(int64(time.Nanosecond)),
		"microsecond":  durationValue(int64(time.Microsecond)),
		"millisecond":  durationValue(int64(time.Millisecond)),
		"second":       durationValue(int64(time.Second)),
		"minute":       durationValue(int64(time.Minute)),
		"hour":         durationValue(int64(time.Hour)),
	}), nil
}
