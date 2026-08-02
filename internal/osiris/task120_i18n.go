package osiris

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Task120LocaleDataVersion pins the embedded locale data. No host locale or
// CLDR installation is consulted.
const Task120LocaleDataVersion = "osiris-locale-lite-1"

// Task120Locale is an immutable locale handle containing embedded separators,
// month names, and message templates for one supported BCP-47 tag.
type Task120Locale struct {
	Tag         string
	DataVersion string
	decimal     string
	group       string
	months      [12]string
	shortMonths [12]string
	messages    map[string]task120Message
}

type task120Message struct {
	one   string
	other string
}

var task120Locales = map[string]Task120Locale{
	"en-US": {Tag: "en-US", DataVersion: Task120LocaleDataVersion, decimal: ".", group: ",", months: [12]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}, shortMonths: [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}, messages: map[string]task120Message{"greeting": {one: "Hello, {name}!", other: "Hello, {name}!"}, "items": {one: "{count} item", other: "{count} items"}}},
	"de-DE": {Tag: "de-DE", DataVersion: Task120LocaleDataVersion, decimal: ",", group: ".", months: [12]string{"Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"}, shortMonths: [12]string{"Jan", "Feb", "Mär", "Apr", "Mai", "Jun", "Jul", "Aug", "Sep", "Okt", "Nov", "Dez"}, messages: map[string]task120Message{"greeting": {one: "Hallo, {name}!", other: "Hallo, {name}!"}, "items": {one: "{count} Element", other: "{count} Elemente"}}},
	"fr-FR": {Tag: "fr-FR", DataVersion: Task120LocaleDataVersion, decimal: ",", group: " ", months: [12]string{"janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"}, shortMonths: [12]string{"janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."}, messages: map[string]task120Message{"greeting": {one: "Bonjour, {name} !", other: "Bonjour, {name} !"}, "items": {one: "{count} article", other: "{count} articles"}}},
}

// Task120DateStyle controls the fixed date pattern.
type Task120DateStyle string

const (
	Task120DateShort  Task120DateStyle = "short"
	Task120DateMedium Task120DateStyle = "medium"
	Task120DateLong   Task120DateStyle = "long"
)

// Task120NumberOptions specifies fixed decimal rounding and grouping. Zero
// MaxFractionDigits means zero fractional digits, not an ambient default.
type Task120NumberOptions struct {
	MinFractionDigits int
	MaxFractionDigits int
	UseGrouping       bool
	MaxOutputBytes    int
}

// Task120DateOptions specifies a locale date style and output bound. The
// supplied time's own location/date fields are used; no host timezone lookup
// occurs.
type Task120DateOptions struct {
	Style          Task120DateStyle
	MaxOutputBytes int
}

const defaultTask120I18nOutputBytes = 4096

func task120LocaleTag(tag string) string {
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

// Task120LocaleFor returns an embedded locale by exact supported tag (or the
// documented language alias en/de/fr) and never falls back to host data.
func Task120LocaleFor(ctx context.Context, tag string) (Task120Locale, error) {
	if err := task120CheckContext(ctx, "locale lookup"); err != nil {
		return Task120Locale{}, err
	}
	normalized := task120LocaleTag(tag)
	locale, ok := task120Locales[normalized]
	if !ok {
		return Task120Locale{}, fmt.Errorf("locale lookup: %w: unsupported locale %q", ErrTask120Invalid, tag)
	}
	return locale, nil
}

// Task120FormatNumber formats a finite IEEE-754 number with deterministic
// round-to-nearest formatting, locale separators, and a bounded output.
func Task120FormatNumber(ctx context.Context, locale Task120Locale, value float64, options Task120NumberOptions) (string, error) {
	if err := task120CheckContext(ctx, "number format"); err != nil {
		return "", err
	}
	if locale.DataVersion != Task120LocaleDataVersion || locale.decimal == "" {
		return "", fmt.Errorf("number format: %w: invalid locale handle", ErrTask120Invalid)
	}
	if math.IsNaN(value) {
		return "", fmt.Errorf("number format: %w: NaN is not supported", ErrTask120Invalid)
	}
	if options.MinFractionDigits < 0 || options.MaxFractionDigits < options.MinFractionDigits || options.MaxFractionDigits > 18 {
		return "", fmt.Errorf("number format: %w: fraction digits must satisfy 0 <= min <= max <= 18", ErrTask120Invalid)
	}
	maxOutput := options.MaxOutputBytes
	if maxOutput <= 0 {
		maxOutput = defaultTask120I18nOutputBytes
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
			integer = task120GroupInteger(integer, locale.group)
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
		return "", task120LimitError("number format", "output bytes", int64(len(result)), int64(maxOutput))
	}
	if err := task120CheckContext(ctx, "number format"); err != nil {
		return "", err
	}
	return result, nil
}

func task120GroupInteger(integer, separator string) string {
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

// Task120FormatDate formats the year/month/day from t with a fixed embedded
// locale pattern. It is independent of the process locale and timezone.
func Task120FormatDate(ctx context.Context, locale Task120Locale, t time.Time, options Task120DateOptions) (string, error) {
	if err := task120CheckContext(ctx, "date format"); err != nil {
		return "", err
	}
	if locale.DataVersion != Task120LocaleDataVersion || locale.decimal == "" {
		return "", fmt.Errorf("date format: %w: invalid locale handle", ErrTask120Invalid)
	}
	if options.Style == "" {
		options.Style = Task120DateMedium
	}
	if options.MaxOutputBytes <= 0 {
		options.MaxOutputBytes = defaultTask120I18nOutputBytes
	}
	year, month, day := t.Date()
	var result string
	switch options.Style {
	case Task120DateShort:
		switch locale.Tag {
		case "en-US":
			result = fmt.Sprintf("%02d/%02d/%04d", int(month), day, year)
		default:
			result = fmt.Sprintf("%02d/%02d/%04d", day, int(month), year)
		}
	case Task120DateMedium:
		if locale.Tag == "en-US" {
			result = fmt.Sprintf("%s %d, %04d", locale.shortMonths[month-1], day, year)
		} else {
			result = fmt.Sprintf("%02d %s %04d", day, locale.shortMonths[month-1], year)
		}
	case Task120DateLong:
		if locale.Tag == "en-US" {
			result = fmt.Sprintf("%s %d, %04d", locale.months[month-1], day, year)
		} else {
			result = fmt.Sprintf("%d %s %04d", day, locale.months[month-1], year)
		}
	default:
		return "", fmt.Errorf("date format: %w: unsupported style %q", ErrTask120Invalid, options.Style)
	}
	if len(result) > options.MaxOutputBytes {
		return "", task120LimitError("date format", "output bytes", int64(len(result)), int64(options.MaxOutputBytes))
	}
	return result, nil
}

// Task120FormatMessage formats an embedded versioned message by id. Supported
// placeholders are {name} and {count}; count selects one for exactly 1 and
// other otherwise. Missing placeholders are errors and map iteration never
// affects output order.
func Task120FormatMessage(ctx context.Context, locale Task120Locale, id string, count int64, args map[string]string) (string, error) {
	if err := task120CheckContext(ctx, "message format"); err != nil {
		return "", err
	}
	if locale.DataVersion != Task120LocaleDataVersion || locale.messages == nil {
		return "", fmt.Errorf("message format: %w: invalid locale handle", ErrTask120Invalid)
	}
	message, ok := locale.messages[id]
	if !ok {
		return "", fmt.Errorf("message format: %w: unknown message %q", ErrTask120Invalid, id)
	}
	template := message.other
	if count == 1 {
		template = message.one
	}
	if len(args) > 64 {
		return "", task120LimitError("message format", "arguments", int64(len(args)), 64)
	}
	args = cloneTask120MessageArgs(args)
	args["count"] = strconv.FormatInt(count, 10)
	var b strings.Builder
	for i := 0; i < len(template); {
		if err := task120CheckContext(ctx, "message format"); err != nil {
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
			return "", fmt.Errorf("message format: %w: unterminated placeholder", ErrTask120Invalid)
		}
		close += open + 1
		name := template[open+1 : close]
		value, ok := args[name]
		if !ok {
			return "", fmt.Errorf("message format: %w: missing argument %q", ErrTask120Invalid, name)
		}
		if b.Len()+len(value) > defaultTask120I18nOutputBytes {
			return "", task120LimitError("message format", "output bytes", int64(b.Len()+len(value)), defaultTask120I18nOutputBytes)
		}
		b.WriteString(value)
		i = close + 1
	}
	if b.Len() > defaultTask120I18nOutputBytes {
		return "", task120LimitError("message format", "output bytes", int64(b.Len()), defaultTask120I18nOutputBytes)
	}
	return b.String(), nil
}

func cloneTask120MessageArgs(args map[string]string) map[string]string {
	copyOf := make(map[string]string, len(args)+1)
	for key, value := range args {
		copyOf[key] = value
	}
	return copyOf
}
