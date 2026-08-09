package osiris

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestExtendedI18nLocalesPluralListsAndCurrency(t *testing.T) {
	ctx := context.Background()
	locale, err := ExtendedLocaleFor(ctx, "en")
	if err != nil {
		t.Fatal(err)
	}
	if locale.Tag != "en-US" || locale.Direction != "ltr" {
		t.Fatalf("locale alias = %#v", locale)
	}
	category, err := ExtendedPluralCategory(ctx, locale, "1")
	if err != nil || category != "one" {
		t.Fatalf("one category = %q, %v", category, err)
	}
	category, err = ExtendedPluralCategory(ctx, locale, "1.0")
	if err != nil || category != "other" {
		t.Fatalf("visible fraction category = %q, %v", category, err)
	}
	fr, _ := ExtendedLocaleFor(ctx, "fr-FR")
	category, err = ExtendedPluralCategory(ctx, fr, "0")
	if err != nil || category != "one" {
		t.Fatalf("French zero category = %q, %v", category, err)
	}
	ar, _ := ExtendedLocaleFor(ctx, "ar-EG")
	for number, want := range map[string]string{"0": "zero", "1": "one", "2": "two", "7": "few", "15": "many", "101": "other"} {
		category, err := ExtendedPluralCategory(ctx, ar, number)
		if err != nil || category != want {
			t.Fatalf("Arabic %s category = %q, %v; want %s", number, category, err, want)
		}
	}
	list, err := ExtendedFormatList(ctx, locale, []string{"Ada", "Lin", "Sol"}, ExtendedListStandard)
	if err != nil || list != "Ada, Lin, and Sol" {
		t.Fatalf("English list = %q, %v", list, err)
	}
	ja, _ := ExtendedLocaleFor(ctx, "ja")
	list, err = ExtendedFormatList(ctx, ja, []string{"A", "B", "C"}, "")
	if err != nil || list != "A、B、C" {
		t.Fatalf("Japanese list = %q, %v", list, err)
	}
	currency, err := ExtendedFormatCurrency(ctx, locale, "1234.5", "USD")
	if err != nil || currency != "$1,234.50" {
		t.Fatalf("US currency = %q, %v", currency, err)
	}
	de, _ := ExtendedLocaleFor(ctx, "de")
	currency, err = ExtendedFormatCurrency(ctx, de, "1234.5", "EUR")
	if err != nil || currency != "1.234,50 €" {
		t.Fatalf("German currency = %q, %v", currency, err)
	}
	percent, err := ExtendedFormatPercent(ctx, locale, "0.125")
	if err != nil || percent != "12.50%" {
		t.Fatalf("US percent = %q, %v", percent, err)
	}
	relative, err := ExtendedFormatRelative(ctx, locale, -2, "day")
	if err != nil || relative != "2 days ago" {
		t.Fatalf("relative time = %q, %v", relative, err)
	}
	date, err := ExtendedFormatDateStyle(ctx, locale, "2024-01-02T03:04:05Z", "long")
	if err != nil || date != "Tuesday, January 2, 2024" {
		t.Fatalf("long date = %q, %v", date, err)
	}
}

func TestExtendedI18nCatalogAndLimits(t *testing.T) {
	ctx := context.Background()
	locale, _ := ExtendedLocaleFor(ctx, "en-US")
	catalog := ExtendedCatalog{Locale: locale, Messages: map[string]ExtendedMessage{
		"items": {Plurals: map[string]string{"one": "{name} has {count} item", "other": "{name} has {count} items"}},
		"hello": {Text: "Hello, {name}!"},
	}}
	formatted, err := ExtendedCatalogFormat(ctx, catalog, "items", "2", map[string]string{"name": "Ada"})
	if err != nil || formatted != "Ada has 2 items" {
		t.Fatalf("plural catalog = %q, %v", formatted, err)
	}
	formatted, err = ExtendedCatalogFormat(ctx, catalog, "hello", "1", map[string]string{"name": "Ada"})
	if err != nil || formatted != "Hello, Ada!" {
		t.Fatalf("text catalog = %q, %v", formatted, err)
	}
	if _, err := ExtendedCatalogFormat(ctx, catalog, "hello", "1", nil); !errors.Is(err, ErrStdlibInvalid) {
		t.Fatalf("missing catalog argument error = %v", err)
	}
	value, err := StdlibJSONParse(ctx, []byte(`{"items":{"one":"one item","other":"many items"}}`), StdlibJSONLimits{})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ExtendedCatalogFromValue(locale, value)
	if err != nil {
		t.Fatal(err)
	}
	formatted, err = ExtendedCatalogFormat(ctx, decoded, "items", "1", nil)
	if err != nil || formatted != "one item" {
		t.Fatalf("decoded catalog = %q, %v", formatted, err)
	}
	overlay := ExtendedCatalog{Locale: locale, Messages: map[string]ExtendedMessage{"hello": {Text: "Hi, {name}!"}}}
	merged, err := ExtendedCatalogMerge(catalog, overlay)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Messages["hello"].Text != "Hi, {name}!" || merged.Messages["items"].Plurals["one"] != "{name} has {count} item" {
		t.Fatalf("catalog merge = %#v", merged.Messages)
	}
	if _, err := ExtendedFormatList(ctx, locale, make([]string, ExtendedI18nMaxItems+1), ExtendedListStandard); err == nil {
		t.Fatal("expected list item limit")
	}
	if _, err := ExtendedLocaleFor(ctx, "xx"); !errors.Is(err, ErrStdlibInvalid) {
		t.Fatalf("unsupported locale error = %v", err)
	}
	if got := ExtendedSupportedLocaleTags(); !reflect.DeepEqual(got, []string{"ar-EG", "de-DE", "en-US", "es-ES", "fr-FR", "ja-JP"}) {
		t.Fatalf("supported locale order = %#v", got)
	}
}

func TestExtendedI18nModuleHelperIsAdditive(t *testing.T) {
	module := ExtendedI18nModule()
	for _, name := range []string{"extensions_version", "supported_locales", "plural_category", "format_list", "format_currency", "format_percent", "format_relative", "format_date_style", "catalog_format"} {
		if _, ok := module[name]; !ok {
			t.Fatalf("i18n module missing %q", name)
		}
	}
}
