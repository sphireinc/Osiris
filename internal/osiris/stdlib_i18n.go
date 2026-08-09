package osiris

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// StdlibLocaleDataVersion pins the embedded locale data. No host locale or
// CLDR installation is consulted.
const StdlibLocaleDataVersion = "osiris-locale-lite-1"

// StdlibLocale is an immutable locale handle containing embedded separators,
// month names, and message templates for one supported BCP-47 tag.
type StdlibLocale struct {
	Tag         string
	DataVersion string
	decimal     string
	group       string
	months      [12]string
	shortMonths [12]string
	messages    map[string]stdlibMessage
}

type stdlibMessage struct {
	one   string
	other string
}

var stdlibLocales = map[string]StdlibLocale{
	"en-US": {Tag: "en-US", DataVersion: StdlibLocaleDataVersion, decimal: ".", group: ",", months: [12]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}, shortMonths: [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}, messages: map[string]stdlibMessage{"greeting": {one: "Hello, {name}!", other: "Hello, {name}!"}, "items": {one: "{count} item", other: "{count} items"}}},
	"de-DE": {Tag: "de-DE", DataVersion: StdlibLocaleDataVersion, decimal: ",", group: ".", months: [12]string{"Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"}, shortMonths: [12]string{"Jan", "Feb", "Mär", "Apr", "Mai", "Jun", "Jul", "Aug", "Sep", "Okt", "Nov", "Dez"}, messages: map[string]stdlibMessage{"greeting": {one: "Hallo, {name}!", other: "Hallo, {name}!"}, "items": {one: "{count} Element", other: "{count} Elemente"}}},
	"fr-FR": {Tag: "fr-FR", DataVersion: StdlibLocaleDataVersion, decimal: ",", group: " ", months: [12]string{"janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"}, shortMonths: [12]string{"janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."}, messages: map[string]stdlibMessage{"greeting": {one: "Bonjour, {name} !", other: "Bonjour, {name} !"}, "items": {one: "{count} article", other: "{count} articles"}}},
}

// StdlibDateStyle controls the fixed date pattern.
type StdlibDateStyle string

const (
	StdlibDateShort  StdlibDateStyle = "short"
	StdlibDateMedium StdlibDateStyle = "medium"
	StdlibDateLong   StdlibDateStyle = "long"
)

// StdlibNumberOptions specifies fixed decimal rounding and grouping. Zero
// MaxFractionDigits means zero fractional digits, not an ambient default.
type StdlibNumberOptions struct {
	MinFractionDigits int
	MaxFractionDigits int
	UseGrouping       bool
	MaxOutputBytes    int
}

// StdlibDateOptions specifies a locale date style and output bound. The
// supplied time's own location/date fields are used; no host timezone lookup
// occurs.
type StdlibDateOptions struct {
	Style          StdlibDateStyle
	MaxOutputBytes int
}

const defaultStdlibI18nOutputBytes = 4096

func stdlibLocaleTag(tag string) string {
	if tag == "en" {
		return "en-US"
	}
	if tag == "de" {
		return "de-DE"
	}
	if tag == "fr" {
		return "fr-FR"
	}
	return tag
}

// StdlibLocaleFor returns an embedded locale by exact supported tag (or the
// documented language alias en/de/fr) and never falls back to host data.
func StdlibLocaleFor(ctx context.Context, tag string) (StdlibLocale, error) {
	if err := stdlibCheckContext(ctx, "locale lookup"); err != nil {
		return StdlibLocale{}, err
	}
	normalized := stdlibLocaleTag(tag)
	locale, ok := stdlibLocales[normalized]
	if !ok {
		return StdlibLocale{}, fmt.Errorf("locale lookup: %w: unsupported locale %q", ErrStdlibInvalid, tag)
	}
	return locale, nil
}

// StdlibFormatNumber formats a finite IEEE-754 number with deterministic
// round-to-nearest formatting, locale separators, and a bounded output.
func StdlibFormatNumber(ctx context.Context, locale StdlibLocale, value float64, options StdlibNumberOptions) (string, error) {
	if err := stdlibCheckContext(ctx, "number format"); err != nil {
		return "", err
	}
	if locale.DataVersion != StdlibLocaleDataVersion || locale.decimal == "" {
		return "", fmt.Errorf("number format: %w: invalid locale handle", ErrStdlibInvalid)
	}
	if math.IsNaN(value) {
		return "", fmt.Errorf("number format: %w: NaN is not supported", ErrStdlibInvalid)
	}
	if options.MinFractionDigits < 0 || options.MaxFractionDigits < options.MinFractionDigits || options.MaxFractionDigits > 18 {
		return "", fmt.Errorf("number format: %w: fraction digits must satisfy 0 <= min <= max <= 18", ErrStdlibInvalid)
	}
	maxOutput := options.MaxOutputBytes
	if maxOutput <= 0 {
		maxOutput = defaultStdlibI18nOutputBytes
	}
	var result string
	switch {
	case math.IsInf(value, 1):
		result = "∞"
	case math.IsInf(value, -1):
		result = "-∞"
	default:
		if value == 0 {
			value = 0
		}
		raw := strconv.FormatFloat(value, 'f', options.MaxFractionDigits, 64)
		negative := strings.HasPrefix(raw, "-")
		if negative {
			raw = raw[1:]
		}
		parts := strings.SplitN(raw, ".", 2)
		integer, fraction := parts[0], ""
		if len(parts) == 2 {
			fraction = strings.TrimRight(parts[1], "0")
		}
		if len(fraction) < options.MinFractionDigits {
			fraction += strings.Repeat("0", options.MinFractionDigits-len(fraction))
		}
		if options.UseGrouping {
			integer = stdlibGroupInteger(integer, locale.group)
		}
		result = integer
		if fraction != "" {
			result += locale.decimal + fraction
		}
		if negative {
			result = "-" + result
		}
	}
	if len(result) > maxOutput {
		return "", stdlibLimitError("number format", "output bytes", int64(len(result)), int64(maxOutput))
	}
	if err := stdlibCheckContext(ctx, "number format"); err != nil {
		return "", err
	}
	return result, nil
}

func stdlibGroupInteger(integer, separator string) string {
	if len(integer) <= 3 {
		return integer
	}
	first := len(integer) % 3
	if first == 0 {
		first = 3
	}
	var b strings.Builder
	b.WriteString(integer[:first])
	for i := first; i < len(integer); i += 3 {
		b.WriteString(separator)
		b.WriteString(integer[i : i+3])
	}
	return b.String()
}

// StdlibFormatDate formats the year/month/day from t with a fixed embedded
// locale pattern. It is independent of the process locale and timezone.
func StdlibFormatDate(ctx context.Context, locale StdlibLocale, t time.Time, options StdlibDateOptions) (string, error) {
	if err := stdlibCheckContext(ctx, "date format"); err != nil {
		return "", err
	}
	if locale.DataVersion != StdlibLocaleDataVersion || locale.decimal == "" {
		return "", fmt.Errorf("date format: %w: invalid locale handle", ErrStdlibInvalid)
	}
	if options.Style == "" {
		options.Style = StdlibDateMedium
	}
	if options.MaxOutputBytes <= 0 {
		options.MaxOutputBytes = defaultStdlibI18nOutputBytes
	}
	year, month, day := t.Date()
	var result string
	switch options.Style {
	case StdlibDateShort:
		switch locale.Tag {
		case "en-US":
			result = fmt.Sprintf("%02d/%02d/%04d", int(month), day, year)
		default:
			result = fmt.Sprintf("%02d/%02d/%04d", day, int(month), year)
		}
	case StdlibDateMedium:
		if locale.Tag == "en-US" {
			result = fmt.Sprintf("%s %d, %04d", locale.shortMonths[month-1], day, year)
		} else {
			result = fmt.Sprintf("%02d %s %04d", day, locale.shortMonths[month-1], year)
		}
	case StdlibDateLong:
		if locale.Tag == "en-US" {
			result = fmt.Sprintf("%s %d, %04d", locale.months[month-1], day, year)
		} else {
			result = fmt.Sprintf("%d %s %04d", day, locale.months[month-1], year)
		}
	default:
		return "", fmt.Errorf("date format: %w: unsupported style %q", ErrStdlibInvalid, options.Style)
	}
	if len(result) > options.MaxOutputBytes {
		return "", stdlibLimitError("date format", "output bytes", int64(len(result)), int64(options.MaxOutputBytes))
	}
	return result, nil
}

// StdlibFormatMessage formats an embedded versioned message by id. Supported
// placeholders are {name} and {count}; count selects one for exactly 1 and
// other otherwise. Missing placeholders are errors and map iteration never
// affects output order.
func StdlibFormatMessage(ctx context.Context, locale StdlibLocale, id string, count int64, args map[string]string) (string, error) {
	if err := stdlibCheckContext(ctx, "message format"); err != nil {
		return "", err
	}
	if locale.DataVersion != StdlibLocaleDataVersion || locale.messages == nil {
		return "", fmt.Errorf("message format: %w: invalid locale handle", ErrStdlibInvalid)
	}
	message, ok := locale.messages[id]
	if !ok {
		return "", fmt.Errorf("message format: %w: unknown message %q", ErrStdlibInvalid, id)
	}
	template := message.other
	if count == 1 {
		template = message.one
	}
	if len(args) > 64 {
		return "", stdlibLimitError("message format", "arguments", int64(len(args)), 64)
	}
	args = cloneStdlibMessageArgs(args)
	args["count"] = strconv.FormatInt(count, 10)
	var b strings.Builder
	for i := 0; i < len(template); {
		if err := stdlibCheckContext(ctx, "message format"); err != nil {
			return "", err
		}
		open := strings.IndexByte(template[i:], '{')
		if open < 0 {
			b.WriteString(template[i:])
			break
		}
		open += i
		b.WriteString(template[i:open])
		close := strings.IndexByte(template[open+1:], '}')
		if close < 0 {
			return "", fmt.Errorf("message format: %w: unterminated placeholder", ErrStdlibInvalid)
		}
		close += open + 1
		name := template[open+1 : close]
		value, ok := args[name]
		if !ok {
			return "", fmt.Errorf("message format: %w: missing argument %q", ErrStdlibInvalid, name)
		}
		if b.Len()+len(value) > defaultStdlibI18nOutputBytes {
			return "", stdlibLimitError("message format", "output bytes", int64(b.Len()+len(value)), defaultStdlibI18nOutputBytes)
		}
		b.WriteString(value)
		i = close + 1
	}
	if b.Len() > defaultStdlibI18nOutputBytes {
		return "", stdlibLimitError("message format", "output bytes", int64(b.Len()), defaultStdlibI18nOutputBytes)
	}
	return b.String(), nil
}

func cloneStdlibMessageArgs(args map[string]string) map[string]string {
	copyOf := make(map[string]string, len(args)+1)
	for key, value := range args {
		copyOf[key] = value
	}
	return copyOf
}
