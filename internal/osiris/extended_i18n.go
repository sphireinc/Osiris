package osiris

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// ExtendedI18nVersion identifies the additive locale, plural, list, currency,
// and catalog contract. Stdlib locale handles and functions are unchanged.
const ExtendedI18nVersion = "locale-plural-list-catalog-1"

const (
	ExtendedI18nMaxItems          = 128
	ExtendedI18nMaxMessageBytes   = 4096
	ExtendedI18nMaxCatalogEntries = 1024
	ExtendedI18nMaxArguments      = 64
)

// ExtendedLocale is immutable embedded locale metadata. It contains no host
// locale state, timezone lookup, or process-dependent formatting behavior.
type ExtendedLocale struct {
	Tag              string
	Language         string
	Region           string
	Direction        string
	DecimalSeparator string
	GroupSeparator   string
	FirstDayOfWeek   int
	PluralCategories []string
	CurrencySymbols  map[string]string
}

var extendedLocales = map[string]ExtendedLocale{
	"en-US": {Tag: "en-US", Language: "en", Region: "US", Direction: "ltr", DecimalSeparator: ".", GroupSeparator: ",", FirstDayOfWeek: 0, PluralCategories: []string{"one", "other"}, CurrencySymbols: map[string]string{"USD": "$", "EUR": "€", "GBP": "£"}},
	"de-DE": {Tag: "de-DE", Language: "de", Region: "DE", Direction: "ltr", DecimalSeparator: ",", GroupSeparator: ".", FirstDayOfWeek: 1, PluralCategories: []string{"one", "other"}, CurrencySymbols: map[string]string{"EUR": "€", "USD": "$"}},
	"fr-FR": {Tag: "fr-FR", Language: "fr", Region: "FR", Direction: "ltr", DecimalSeparator: ",", GroupSeparator: " ", FirstDayOfWeek: 1, PluralCategories: []string{"one", "other"}, CurrencySymbols: map[string]string{"EUR": "€", "USD": "$"}},
	"es-ES": {Tag: "es-ES", Language: "es", Region: "ES", Direction: "ltr", DecimalSeparator: ",", GroupSeparator: ".", FirstDayOfWeek: 1, PluralCategories: []string{"one", "other"}, CurrencySymbols: map[string]string{"EUR": "€", "USD": "$"}},
	"ja-JP": {Tag: "ja-JP", Language: "ja", Region: "JP", Direction: "ltr", DecimalSeparator: ".", GroupSeparator: ",", FirstDayOfWeek: 0, PluralCategories: []string{"other"}, CurrencySymbols: map[string]string{"JPY": "￥", "USD": "$"}},
	"ar-EG": {Tag: "ar-EG", Language: "ar", Region: "EG", Direction: "rtl", DecimalSeparator: ".", GroupSeparator: ",", FirstDayOfWeek: 6, PluralCategories: []string{"zero", "one", "two", "few", "many", "other"}, CurrencySymbols: map[string]string{"EGP": "ج.م", "USD": "$"}},
}

// ExtendedLocaleFor resolves canonical tags and language aliases against the
// embedded deterministic data. Supported aliases are en, de, fr, es, ja, and
// ar; an unsupported region never falls back to host locale data.
func ExtendedLocaleFor(ctx context.Context, tag string) (ExtendedLocale, error) {
	if err := stdlibCheckContext(ctx, "locale lookup"); err != nil {
		return ExtendedLocale{}, err
	}
	normalized := extendedLocaleTag(tag)
	locale, ok := extendedLocales[normalized]
	if !ok {
		return ExtendedLocale{}, fmt.Errorf("locale lookup: %w: unsupported locale %q", ErrStdlibInvalid, tag)
	}
	locale.PluralCategories = append([]string(nil), locale.PluralCategories...)
	locale.CurrencySymbols = cloneExtendedStringMap(locale.CurrencySymbols)
	return locale, nil
}

// ExtendedSupportedLocaleTags returns the embedded tags in lexical order.
func ExtendedSupportedLocaleTags() []string {
	result := make([]string, 0, len(extendedLocales))
	for tag := range extendedLocales {
		result = append(result, tag)
	}
	sort.Strings(result)
	return result
}

// ExtendedPluralCategory implements the embedded locale plural rules for a
// bounded decimal spelling. The spelling is retained for visible-fraction
// handling; floating-point conversion is deliberately avoided.
func ExtendedPluralCategory(ctx context.Context, locale ExtendedLocale, number string) (string, error) {
	if err := stdlibCheckContext(ctx, "plural category"); err != nil {
		return "", err
	}
	if err := extendedValidateLocale(locale); err != nil {
		return "", err
	}
	parts, err := extendedDecimalParts(number)
	if err != nil {
		return "", err
	}
	integer := new(big.Int)
	if _, ok := integer.SetString(parts.integer, 10); !ok {
		return "", fmt.Errorf("plural category: %w: invalid number %q", ErrStdlibInvalid, number)
	}
	if integer.Sign() < 0 {
		integer.Abs(integer)
	}
	hasFraction := parts.fraction != ""
	last := new(big.Int).Mod(new(big.Int).Set(integer), big.NewInt(100)).Int64()
	value := integer.Int64()
	switch locale.Language {
	case "ja":
		return "other", nil
	case "fr":
		if !hasFraction && (value == 0 || value == 1) {
			return "one", nil
		}
	case "ar":
		if !hasFraction {
			switch value {
			case 0:
				return "zero", nil
			case 1:
				return "one", nil
			case 2:
				return "two", nil
			}
			if last >= 3 && last <= 10 {
				return "few", nil
			}
			if last >= 11 && last <= 99 {
				return "many", nil
			}
		}
	default:
		if !hasFraction && value == 1 {
			return "one", nil
		}
	}
	return "other", nil
}

// ExtendedListStyle selects deterministic list punctuation.
type ExtendedListStyle string

const (
	ExtendedListStandard ExtendedListStyle = "standard"
	ExtendedListShort    ExtendedListStyle = "short"
)

// ExtendedFormatList formats a bounded list with locale conjunctions. Items
// are treated as already localized text and are not escaped or reinterpreted.
func ExtendedFormatList(ctx context.Context, locale ExtendedLocale, items []string, style ExtendedListStyle) (string, error) {
	if err := stdlibCheckContext(ctx, "list format"); err != nil {
		return "", err
	}
	if err := extendedValidateLocale(locale); err != nil {
		return "", err
	}
	if len(items) > ExtendedI18nMaxItems {
		return "", stdlibLimitError("list format", "items", int64(len(items)), ExtendedI18nMaxItems)
	}
	if style == "" {
		style = ExtendedListStandard
	}
	if style != ExtendedListStandard && style != ExtendedListShort {
		return "", fmt.Errorf("list format: %w: unsupported style %q", ErrStdlibInvalid, style)
	}
	for _, item := range items {
		if !utf8.ValidString(item) || len(item) > ExtendedI18nMaxMessageBytes {
			return "", fmt.Errorf("list format: %w: invalid or oversized item", ErrStdlibInvalid)
		}
	}
	if len(items) == 0 {
		return "", nil
	}
	if len(items) == 1 {
		return items[0], nil
	}
	if locale.Language == "ja" {
		return strings.Join(items, "、"), nil
	}
	conjunction := extendedListConjunction(locale.Language, style)
	if len(items) == 2 {
		return items[0] + " " + conjunction + " " + items[1], nil
	}
	var b strings.Builder
	for i, item := range items {
		if i > 0 {
			if i == len(items)-1 {
				if locale.Language == "en" && style == ExtendedListStandard {
					b.WriteString(",")
				}
				b.WriteByte(' ')
				b.WriteString(conjunction)
				b.WriteByte(' ')
			} else {
				b.WriteString(", ")
			}
		}
		b.WriteString(item)
	}
	if b.Len() > ExtendedI18nMaxMessageBytes {
		return "", stdlibLimitError("list format", "output bytes", int64(b.Len()), ExtendedI18nMaxMessageBytes)
	}
	return b.String(), nil
}

func extendedListConjunction(language string, style ExtendedListStyle) string {
	switch language {
	case "de":
		return "und"
	case "fr":
		return "et"
	case "es":
		return "y"
	case "ar":
		return "و"
	default:
		if style == ExtendedListShort {
			return "&"
		}
		return "and"
	}
}

// ExtendedFormatCurrency formats an exact decimal amount with two fractional
// digits, locale separators, and an embedded symbol. Unknown currency codes
// are rendered as codes; no host currency database is consulted.
func ExtendedFormatCurrency(ctx context.Context, locale ExtendedLocale, amount, currency string) (string, error) {
	if err := stdlibCheckContext(ctx, "currency format"); err != nil {
		return "", err
	}
	if err := extendedValidateLocale(locale); err != nil {
		return "", err
	}
	if len(currency) != 3 || strings.ToUpper(currency) != currency {
		return "", fmt.Errorf("currency format: %w: currency must be an uppercase ISO-4217 code", ErrStdlibInvalid)
	}
	parts, err := extendedDecimalParts(amount)
	if err != nil {
		return "", err
	}
	rational, ok := new(big.Rat).SetString(amount)
	if !ok {
		return "", fmt.Errorf("currency format: %w: invalid amount", ErrStdlibInvalid)
	}
	formatted := rational.FloatString(2)
	negative := strings.HasPrefix(formatted, "-")
	if negative {
		formatted = formatted[1:]
	}
	numberParts := strings.SplitN(formatted, ".", 2)
	integer := extendedGroupDigits(numberParts[0], locale.GroupSeparator)
	fraction := "00"
	if len(numberParts) == 2 {
		fraction = numberParts[1]
	}
	text := integer + locale.DecimalSeparator + fraction
	if negative {
		text = "-" + text
	}
	symbol := currency
	if known, ok := locale.CurrencySymbols[currency]; ok {
		symbol = known
	}
	if locale.Language == "de" || locale.Language == "fr" || locale.Language == "es" {
		text += " " + symbol
	} else {
		text = symbol + text
	}
	if len(text) > ExtendedI18nMaxMessageBytes || len(parts.integer) > ExtendedI18nMaxMessageBytes {
		return "", stdlibLimitError("currency format", "output bytes", int64(len(text)), ExtendedI18nMaxMessageBytes)
	}
	return text, nil
}

// ExtendedFormatPercent formats an exact decimal ratio as a localized percent
// with two fractional digits. The input 0.125 therefore renders as 12.50% in
// en-US. No floating-point conversion or host locale database is consulted.
func ExtendedFormatPercent(ctx context.Context, locale ExtendedLocale, amount string) (string, error) {
	if err := stdlibCheckContext(ctx, "percent format"); err != nil {
		return "", err
	}
	if err := extendedValidateLocale(locale); err != nil {
		return "", err
	}
	rational, ok := new(big.Rat).SetString(amount)
	if !ok || len(amount) > ExtendedI18nMaxMessageBytes {
		return "", fmt.Errorf("percent format: %w: invalid amount", ErrStdlibInvalid)
	}
	rational.Mul(rational, big.NewRat(100, 1))
	text := extendedFormatDecimal(rational.FloatString(2), locale)
	if locale.Language == "de" || locale.Language == "fr" || locale.Language == "es" {
		text += " %"
	} else {
		text += "%"
	}
	if len(text) > ExtendedI18nMaxMessageBytes {
		return "", stdlibLimitError("percent format", "output bytes", int64(len(text)), ExtendedI18nMaxMessageBytes)
	}
	return text, nil
}

// ExtendedFormatRelative formats a signed integer distance using fixed
// embedded phrases. Positive values are future-oriented; negative values are
// past-oriented. Supported units are minute, hour, day, week, month, and year.
func ExtendedFormatRelative(ctx context.Context, locale ExtendedLocale, value int64, unit string) (string, error) {
	if err := stdlibCheckContext(ctx, "relative format"); err != nil {
		return "", err
	}
	if err := extendedValidateLocale(locale); err != nil {
		return "", err
	}
	unit = strings.ToLower(strings.TrimSpace(unit))
	switch unit {
	case "minute", "minutes", "hour", "hours", "day", "days", "week", "weeks", "month", "months", "year", "years":
	default:
		return "", fmt.Errorf("relative format: %w: unsupported unit %q", ErrStdlibInvalid, unit)
	}
	if value == 0 {
		switch locale.Language {
		case "de":
			return "gerade jetzt", nil
		case "fr":
			return "maintenant", nil
		case "es":
			return "ahora", nil
		case "ja":
			return "今", nil
		case "ar":
			return "الآن", nil
		default:
			return "now", nil
		}
	}
	quantity := value
	future := quantity > 0
	if quantity < 0 {
		quantity = -quantity
	}
	base := strings.TrimSuffix(strings.TrimSuffix(unit, "s"), "es")
	if locale.Language == "ja" {
		return fmt.Sprintf("%d%s", quantity, extendedJapaneseRelativeUnit(base, future)), nil
	}
	if locale.Language == "ar" {
		return extendedArabicRelative(quantity, base, future), nil
	}
	word := extendedRelativeWord(locale.Language, base, quantity == 1)
	switch locale.Language {
	case "de":
		if future {
			return fmt.Sprintf("in %d %s", quantity, word), nil
		}
		return fmt.Sprintf("vor %d %s", quantity, word), nil
	case "fr":
		if future {
			return fmt.Sprintf("dans %d %s", quantity, word), nil
		}
		return fmt.Sprintf("il y a %d %s", quantity, word), nil
	case "es":
		if future {
			return fmt.Sprintf("en %d %s", quantity, word), nil
		}
		return fmt.Sprintf("hace %d %s", quantity, word), nil
	default:
		if future {
			return fmt.Sprintf("in %d %s", quantity, word), nil
		}
		return fmt.Sprintf("%d %s ago", quantity, word), nil
	}
}

// ExtendedFormatDateStyle formats an RFC3339 instant using a fixed UTC view
// and embedded short, medium, or long date patterns.
func ExtendedFormatDateStyle(ctx context.Context, locale ExtendedLocale, rfc3339, style string) (string, error) {
	if err := stdlibCheckContext(ctx, "date format"); err != nil {
		return "", err
	}
	if err := extendedValidateLocale(locale); err != nil {
		return "", err
	}
	instant, err := time.Parse(time.RFC3339Nano, rfc3339)
	if err != nil {
		return "", fmt.Errorf("date format: %w: invalid RFC3339 date", ErrStdlibInvalid)
	}
	style = strings.ToLower(strings.TrimSpace(style))
	if style == "" {
		style = "medium"
	}
	if style != "short" && style != "medium" && style != "long" {
		return "", fmt.Errorf("date format: %w: unsupported style %q", ErrStdlibInvalid, style)
	}
	date := instant.UTC()
	if style == "short" {
		switch locale.Language {
		case "en":
			return date.Format("01/02/2006"), nil
		case "ja":
			return date.Format("2006/01/02"), nil
		default:
			return date.Format("02/01/2006"), nil
		}
	}
	month := extendedLocalizedMonth(locale.Language, int(date.Month()))
	if locale.Language == "ja" {
		return fmt.Sprintf("%d年%d月%d日", date.Year(), date.Month(), date.Day()), nil
	}
	if locale.Language == "de" {
		return fmt.Sprintf("%d. %s %d", date.Day(), month, date.Year()), nil
	}
	if locale.Language == "fr" || locale.Language == "es" || locale.Language == "ar" {
		return fmt.Sprintf("%d %s %d", date.Day(), month, date.Year()), nil
	}
	if style == "long" {
		return fmt.Sprintf("%s, %s %d, %d", extendedLocalizedWeekday(locale.Language, int(date.Weekday())), month, date.Day(), date.Year()), nil
	}
	return fmt.Sprintf("%s %d, %d", month, date.Day(), date.Year()), nil
}

func extendedFormatDecimal(formatted string, locale ExtendedLocale) string {
	negative := strings.HasPrefix(formatted, "-")
	if negative {
		formatted = formatted[1:]
	}
	parts := strings.SplitN(formatted, ".", 2)
	text := extendedGroupDigits(parts[0], locale.GroupSeparator)
	if len(parts) == 2 {
		text += locale.DecimalSeparator + parts[1]
	}
	if negative {
		text = "-" + text
	}
	return text
}

func extendedRelativeWord(language, unit string, singular bool) string {
	words := map[string]map[string][2]string{
		"en": {"minute": {"minute", "minutes"}, "hour": {"hour", "hours"}, "day": {"day", "days"}, "week": {"week", "weeks"}, "month": {"month", "months"}, "year": {"year", "years"}},
		"de": {"minute": {"Minute", "Minuten"}, "hour": {"Stunde", "Stunden"}, "day": {"Tag", "Tagen"}, "week": {"Woche", "Wochen"}, "month": {"Monat", "Monaten"}, "year": {"Jahr", "Jahren"}},
		"fr": {"minute": {"minute", "minutes"}, "hour": {"heure", "heures"}, "day": {"jour", "jours"}, "week": {"semaine", "semaines"}, "month": {"mois", "mois"}, "year": {"an", "ans"}},
		"es": {"minute": {"minuto", "minutos"}, "hour": {"hora", "horas"}, "day": {"día", "días"}, "week": {"semana", "semanas"}, "month": {"mes", "meses"}, "year": {"año", "años"}},
	}
	choices := words[language]
	if choices == nil {
		choices = words["en"]
	}
	choice := choices[unit]
	if singular {
		return choice[0]
	}
	return choice[1]
}

func extendedJapaneseRelativeUnit(unit string, future bool) string {
	units := map[string]string{"minute": "分", "hour": "時間", "day": "日", "week": "週間", "month": "か月", "year": "年"}
	if future {
		return units[unit] + "後"
	}
	return units[unit] + "前"
}

func extendedArabicRelative(quantity int64, unit string, future bool) string {
	words := map[string]string{"minute": "دقيقة", "hour": "ساعة", "day": "يوم", "week": "أسبوع", "month": "شهر", "year": "سنة"}
	if future {
		return fmt.Sprintf("خلال %d %s", quantity, words[unit])
	}
	return fmt.Sprintf("منذ %d %s", quantity, words[unit])
}

func extendedLocalizedMonth(language string, month int) string {
	sets := map[string][]string{
		"en": {"", "January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
		"de": {"", "Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"},
		"fr": {"", "janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"},
		"es": {"", "enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"},
	}
	set := sets[language]
	if set == nil {
		set = sets["en"]
	}
	return set[month]
}

func extendedLocalizedWeekday(language string, weekday int) string {
	sets := map[string][]string{
		"en": {"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"},
		"de": {"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"},
		"fr": {"dimanche", "lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi"},
		"es": {"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"},
	}
	set := sets[language]
	if set == nil {
		set = sets["en"]
	}
	return set[weekday]
}

// ExtendedMessage stores a singular text template and optional plural
// templates. Plural templates take precedence over Text.
type ExtendedMessage struct {
	Text    string
	Plurals map[string]string
}

// ExtendedCatalog is a bounded immutable-by-convention message catalog. Public
// constructors clone maps before returning so callers can retain ownership of
// their input data.
type ExtendedCatalog struct {
	Locale   ExtendedLocale
	Messages map[string]ExtendedMessage
}

// ExtendedCatalogFormat selects a plural template and expands {count} plus
// caller-provided {name} placeholders. Unknown placeholders are errors.
func ExtendedCatalogFormat(ctx context.Context, catalog ExtendedCatalog, key, number string, args map[string]string) (string, error) {
	if err := stdlibCheckContext(ctx, "catalog format"); err != nil {
		return "", err
	}
	if err := extendedValidateLocale(catalog.Locale); err != nil {
		return "", err
	}
	if len(catalog.Messages) > ExtendedI18nMaxCatalogEntries || len(args) > ExtendedI18nMaxArguments {
		return "", stdlibLimitError("catalog format", "entries or arguments", int64(len(catalog.Messages)+len(args)), ExtendedI18nMaxCatalogEntries)
	}
	message, ok := catalog.Messages[key]
	if !ok {
		return "", fmt.Errorf("catalog format: %w: unknown message %q", ErrStdlibInvalid, key)
	}
	category, err := ExtendedPluralCategory(ctx, catalog.Locale, number)
	if err != nil {
		return "", err
	}
	template := message.Text
	if message.Plurals != nil {
		if selected, ok := message.Plurals[category]; ok {
			template = selected
		} else if selected, ok := message.Plurals["other"]; ok {
			template = selected
		} else if template == "" {
			return "", fmt.Errorf("catalog format: %w: message %q has no %s or other plural", ErrStdlibInvalid, key, category)
		}
	}
	if len(template) > ExtendedI18nMaxMessageBytes || !utf8.ValidString(template) {
		return "", stdlibLimitError("catalog format", "template bytes", int64(len(template)), ExtendedI18nMaxMessageBytes)
	}
	values := cloneExtendedStringMap(args)
	values["count"] = number
	return extendedExpandTemplate(template, values)
}

// ExtendedCatalogMerge overlays messages by key and clones all maps.
func ExtendedCatalogMerge(base, overlay ExtendedCatalog) (ExtendedCatalog, error) {
	if err := extendedValidateLocale(base.Locale); err != nil {
		return ExtendedCatalog{}, err
	}
	if err := extendedValidateLocale(overlay.Locale); err != nil {
		return ExtendedCatalog{}, err
	}
	if base.Locale.Tag != overlay.Locale.Tag {
		return ExtendedCatalog{}, fmt.Errorf("catalog merge: %w: locale mismatch", ErrStdlibInvalid)
	}
	if len(base.Messages)+len(overlay.Messages) > ExtendedI18nMaxCatalogEntries {
		return ExtendedCatalog{}, stdlibLimitError("catalog merge", "entries", int64(len(base.Messages)+len(overlay.Messages)), ExtendedI18nMaxCatalogEntries)
	}
	result := ExtendedCatalog{Locale: base.Locale, Messages: make(map[string]ExtendedMessage, len(base.Messages)+len(overlay.Messages))}
	result.Locale.PluralCategories = append([]string(nil), base.Locale.PluralCategories...)
	result.Locale.CurrencySymbols = cloneExtendedStringMap(base.Locale.CurrencySymbols)
	for key, message := range base.Messages {
		result.Messages[key] = cloneExtendedMessage(message)
	}
	for key, message := range overlay.Messages {
		result.Messages[key] = cloneExtendedMessage(message)
	}
	return result, nil
}

// ExtendedCatalogFromValue decodes a catalog object from the ordered JSON-like
// representation: "key": "text" or "key": {"one": "...", "other": "..."}.
func ExtendedCatalogFromValue(locale ExtendedLocale, value StdlibConfigValue) (ExtendedCatalog, error) {
	if value.Kind != StdlibConfigObject {
		return ExtendedCatalog{}, fmt.Errorf("catalog: %w: expected object", ErrStdlibInvalid)
	}
	if len(value.Object) > ExtendedI18nMaxCatalogEntries {
		return ExtendedCatalog{}, stdlibLimitError("catalog", "entries", int64(len(value.Object)), ExtendedI18nMaxCatalogEntries)
	}
	catalog := ExtendedCatalog{Locale: locale, Messages: make(map[string]ExtendedMessage, len(value.Object))}
	for _, field := range value.Object {
		if field.Value.Kind == StdlibConfigString {
			catalog.Messages[field.Key] = ExtendedMessage{Text: field.Value.String}
			continue
		}
		if field.Value.Kind != StdlibConfigObject {
			return ExtendedCatalog{}, fmt.Errorf("catalog message %q: %w: expected string or plural object", field.Key, ErrStdlibInvalid)
		}
		message := ExtendedMessage{Plurals: make(map[string]string, len(field.Value.Object))}
		for _, plural := range field.Value.Object {
			if plural.Value.Kind != StdlibConfigString {
				return ExtendedCatalog{}, fmt.Errorf("catalog message %q: %w: plural text must be string", field.Key, ErrStdlibInvalid)
			}
			message.Plurals[plural.Key] = plural.Value.String
		}
		catalog.Messages[field.Key] = message
	}
	return catalog, nil
}

func extendedExpandTemplate(template string, values map[string]string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(template); {
		open := strings.IndexByte(template[i:], '{')
		if open < 0 {
			b.WriteString(template[i:])
			break
		}
		open += i
		b.WriteString(template[i:open])
		close := strings.IndexByte(template[open+1:], '}')
		if close < 0 {
			return "", fmt.Errorf("catalog format: %w: unterminated placeholder", ErrStdlibInvalid)
		}
		close += open + 1
		name := template[open+1 : close]
		if name == "" {
			return "", fmt.Errorf("catalog format: %w: empty placeholder", ErrStdlibInvalid)
		}
		value, ok := values[name]
		if !ok {
			return "", fmt.Errorf("catalog format: %w: missing argument %q", ErrStdlibInvalid, name)
		}
		b.WriteString(value)
		i = close + 1
		if b.Len() > ExtendedI18nMaxMessageBytes {
			return "", stdlibLimitError("catalog format", "output bytes", int64(b.Len()), ExtendedI18nMaxMessageBytes)
		}
	}
	return b.String(), nil
}

type extendedDecimalPartsResult struct {
	integer  string
	fraction string
}

func extendedDecimalParts(value string) (extendedDecimalPartsResult, error) {
	if len(value) == 0 || len(value) > ExtendedI18nMaxMessageBytes {
		return extendedDecimalPartsResult{}, fmt.Errorf("decimal: %w: empty or oversized number", ErrStdlibInvalid)
	}
	if strings.HasPrefix(value, "+") {
		value = value[1:]
	}
	negative := strings.HasPrefix(value, "-")
	if negative {
		value = value[1:]
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || value == "" {
		return extendedDecimalPartsResult{}, fmt.Errorf("decimal: %w: invalid number %q", ErrStdlibInvalid, value)
	}
	integer := parts[0]
	if integer == "" {
		integer = "0"
	}
	for _, digit := range integer {
		if digit < '0' || digit > '9' {
			return extendedDecimalPartsResult{}, fmt.Errorf("decimal: %w: invalid number %q", ErrStdlibInvalid, value)
		}
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		for _, digit := range fraction {
			if digit < '0' || digit > '9' {
				return extendedDecimalPartsResult{}, fmt.Errorf("decimal: %w: invalid number %q", ErrStdlibInvalid, value)
			}
		}
	}
	integer = strings.TrimLeft(integer, "0")
	if integer == "" {
		integer = "0"
	}
	if negative && integer != "0" {
		integer = "-" + integer
	}
	return extendedDecimalPartsResult{integer: integer, fraction: fraction}, nil
}

func extendedGroupDigits(integer, separator string) string {
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

func extendedLocaleTag(tag string) string {
	tag = strings.TrimSpace(tag)
	if len(tag) == 2 {
		switch strings.ToLower(tag) {
		case "en":
			return "en-US"
		case "de":
			return "de-DE"
		case "fr":
			return "fr-FR"
		case "es":
			return "es-ES"
		case "ja":
			return "ja-JP"
		case "ar":
			return "ar-EG"
		}
	}
	parts := strings.Split(tag, "-")
	if len(parts) == 2 {
		return strings.ToLower(parts[0]) + "-" + strings.ToUpper(parts[1])
	}
	return tag
}

func extendedValidateLocale(locale ExtendedLocale) error {
	known, ok := extendedLocales[locale.Tag]
	if !ok || locale.Language != known.Language || locale.DecimalSeparator == "" || locale.GroupSeparator == "" {
		return fmt.Errorf("i18n operation: %w: invalid locale handle", ErrStdlibInvalid)
	}
	return nil
}

func cloneExtendedStringMap(values map[string]string) map[string]string {
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneExtendedMessage(message ExtendedMessage) ExtendedMessage {
	return ExtendedMessage{Text: message.Text, Plurals: cloneExtendedStringMap(message.Plurals)}
}

func extendedI18nStringList(values []string) Value {
	out := make([]Value, len(values))
	for i, value := range values {
		out[i] = stringValue(value)
	}
	return listValue(out)
}

func extendedI18nStringMap(value Value) (map[string]string, error) {
	config, err := stdlibValueToConfig(value)
	if err != nil || config.Kind != StdlibConfigObject {
		return nil, fmt.Errorf("i18n catalog arguments: %w: expected object", ErrStdlibInvalid)
	}
	result := make(map[string]string, len(config.Object))
	for _, field := range config.Object {
		if field.Value.Kind != StdlibConfigString {
			return nil, fmt.Errorf("i18n catalog arguments: %w: value for %q must be string", ErrStdlibInvalid, field.Key)
		}
		result[field.Key] = field.Value.String
	}
	return result, nil
}

// ExtendedI18nModule returns additive std/i18n members. Integration can merge
// this map into the existing stdlib module without editing this file.
func ExtendedI18nModule() map[string]Value {
	return map[string]Value{
		"extensions_version": stringValue(ExtendedI18nVersion),
		"supported_locales": functionValueOf("std/i18n.supported_locales", func(args []Value) (Value, error) {
			if len(args) != 0 {
				return nullValue(), fmt.Errorf("i18n.supported_locales expects no arguments")
			}
			return extendedI18nStringList(ExtendedSupportedLocaleTags()), nil
		}),
		"plural_category": contextFunctionValueOf("std/i18n.plural_category", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("i18n.plural_category expects locale and number")
			}
			tag, err := taskStringArg(args[0], "locale")
			if err != nil {
				return nullValue(), err
			}
			number, err := taskStringArg(args[1], "number")
			if err != nil {
				return nullValue(), err
			}
			locale, err := ExtendedLocaleFor(ctx, tag)
			if err != nil {
				return nullValue(), err
			}
			category, err := ExtendedPluralCategory(ctx, locale, number)
			return stringValue(category), err
		}),
		"format_list": contextFunctionValueOf("std/i18n.format_list", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) < 2 || len(args) > 3 {
				return nullValue(), fmt.Errorf("i18n.format_list expects locale, list, and optional style")
			}
			tag, err := taskStringArg(args[0], "locale")
			if err != nil {
				return nullValue(), err
			}
			items, err := args[1].AsList()
			if err != nil {
				return nullValue(), err
			}
			texts := make([]string, len(items))
			for i, item := range items {
				texts[i], err = taskStringArg(item, "list item")
				if err != nil {
					return nullValue(), err
				}
			}
			style := ExtendedListStandard
			if len(args) == 3 {
				text, err := taskStringArg(args[2], "list style")
				if err != nil {
					return nullValue(), err
				}
				style = ExtendedListStyle(text)
			}
			locale, err := ExtendedLocaleFor(ctx, tag)
			if err != nil {
				return nullValue(), err
			}
			formatted, err := ExtendedFormatList(ctx, locale, texts, style)
			return stringValue(formatted), err
		}),
		"format_currency": contextFunctionValueOf("std/i18n.format_currency", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 3 {
				return nullValue(), fmt.Errorf("i18n.format_currency expects locale, amount, and currency")
			}
			tag, err := taskStringArg(args[0], "locale")
			if err != nil {
				return nullValue(), err
			}
			amount, err := taskStringArg(args[1], "amount")
			if err != nil {
				return nullValue(), err
			}
			currency, err := taskStringArg(args[2], "currency")
			if err != nil {
				return nullValue(), err
			}
			locale, err := ExtendedLocaleFor(ctx, tag)
			if err != nil {
				return nullValue(), err
			}
			formatted, err := ExtendedFormatCurrency(ctx, locale, amount, currency)
			return stringValue(formatted), err
		}),
		"format_percent": contextFunctionValueOf("std/i18n.format_percent", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("i18n.format_percent expects locale and decimal amount")
			}
			tag, err := taskStringArg(args[0], "locale")
			if err != nil {
				return nullValue(), err
			}
			amount, err := taskStringArg(args[1], "percent amount")
			if err != nil {
				return nullValue(), err
			}
			locale, err := ExtendedLocaleFor(ctx, tag)
			if err != nil {
				return nullValue(), err
			}
			formatted, err := ExtendedFormatPercent(ctx, locale, amount)
			return stringValue(formatted), err
		}),
		"format_relative": contextFunctionValueOf("std/i18n.format_relative", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 3 {
				return nullValue(), fmt.Errorf("i18n.format_relative expects locale, integer value, and unit")
			}
			tag, err := taskStringArg(args[0], "locale")
			if err != nil {
				return nullValue(), err
			}
			value, err := args[1].Int()
			if err != nil {
				return nullValue(), fmt.Errorf("i18n.format_relative: %w: value must be integer", ErrStdlibInvalid)
			}
			unit, err := taskStringArg(args[2], "relative unit")
			if err != nil {
				return nullValue(), err
			}
			locale, err := ExtendedLocaleFor(ctx, tag)
			if err != nil {
				return nullValue(), err
			}
			formatted, err := ExtendedFormatRelative(ctx, locale, int64(value), unit)
			return stringValue(formatted), err
		}),
		"format_date_style": contextFunctionValueOf("std/i18n.format_date_style", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 3 {
				return nullValue(), fmt.Errorf("i18n.format_date_style expects locale, RFC3339 date, and style")
			}
			tag, err := taskStringArg(args[0], "locale")
			if err != nil {
				return nullValue(), err
			}
			date, err := taskStringArg(args[1], "date")
			if err != nil {
				return nullValue(), err
			}
			style, err := taskStringArg(args[2], "date style")
			if err != nil {
				return nullValue(), err
			}
			locale, err := ExtendedLocaleFor(ctx, tag)
			if err != nil {
				return nullValue(), err
			}
			formatted, err := ExtendedFormatDateStyle(ctx, locale, date, style)
			return stringValue(formatted), err
		}),
		"catalog_format": contextFunctionValueOf("std/i18n.catalog_format", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 4 && len(args) != 5 {
				return nullValue(), fmt.Errorf("i18n.catalog_format expects locale, catalog, key, number, and optional arguments")
			}
			tag, err := taskStringArg(args[0], "locale")
			if err != nil {
				return nullValue(), err
			}
			key, err := taskStringArg(args[2], "message key")
			if err != nil {
				return nullValue(), err
			}
			number, err := taskStringArg(args[3], "message number")
			if err != nil {
				return nullValue(), err
			}
			locale, err := ExtendedLocaleFor(ctx, tag)
			if err != nil {
				return nullValue(), err
			}
			catalogValue, err := stdlibValueToConfig(args[1])
			if err != nil {
				return nullValue(), err
			}
			catalog, err := ExtendedCatalogFromValue(locale, catalogValue)
			if err != nil {
				return nullValue(), err
			}
			arguments := map[string]string{}
			if len(args) == 5 {
				arguments, err = extendedI18nStringMap(args[4])
				if err != nil {
					return nullValue(), err
				}
			}
			formatted, err := ExtendedCatalogFormat(ctx, catalog, key, number, arguments)
			return stringValue(formatted), err
		}),
	}
}

// extendedI18nModule is the package-local spelling used by module registration
// code that prefers the existing task-prefixed helper convention.
func extendedI18nModule() map[string]Value {
	return ExtendedI18nModule()
}
