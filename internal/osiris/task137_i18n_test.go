package osiris

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestTask137I18nLocalesPluralListsAndCurrency(t *testing.T) {
	ctx := context.Background()
	locale, err := Task137LocaleFor(ctx, "en")
	if err != nil {
		t.Fatal(err)
	}
	if locale.Tag != "en-US" || locale.Direction != "ltr" {
		t.Fatalf("locale alias = %#v", locale)
	}
	category, err := Task137PluralCategory(ctx, locale, "1")
	if err != nil || category != "one" {
		t.Fatalf("one category = %q, %v", category, err)
	}
	category, err = Task137PluralCategory(ctx, locale, "1.0")
	if err != nil || category != "other" {
		t.Fatalf("visible fraction category = %q, %v", category, err)
	}
	fr, _ := Task137LocaleFor(ctx, "fr-FR")
	category, err = Task137PluralCategory(ctx, fr, "0")
	if err != nil || category != "one" {
		t.Fatalf("French zero category = %q, %v", category, err)
	}
	ar, _ := Task137LocaleFor(ctx, "ar-EG")
	for number, want := range map[string]string{"0": "zero", "1": "one", "2": "two", "7": "few", "15": "many", "101": "other"} {
		category, err := Task137PluralCategory(ctx, ar, number)
		if err != nil || category != want {
			t.Fatalf("Arabic %s category = %q, %v; want %s", number, category, err, want)
		}
	}
	list, err := Task137FormatList(ctx, locale, []string{"Ada", "Lin", "Sol"}, Task137ListStandard)
	if err != nil || list != "Ada, Lin, and Sol" {
		t.Fatalf("English list = %q, %v", list, err)
	}
	ja, _ := Task137LocaleFor(ctx, "ja")
	list, err = Task137FormatList(ctx, ja, []string{"A", "B", "C"}, "")
	if err != nil || list != "A、B、C" {
		t.Fatalf("Japanese list = %q, %v", list, err)
	}
	currency, err := Task137FormatCurrency(ctx, locale, "1234.5", "USD")
	if err != nil || currency != "$1,234.50" {
		t.Fatalf("US currency = %q, %v", currency, err)
	}
	de, _ := Task137LocaleFor(ctx, "de")
	currency, err = Task137FormatCurrency(ctx, de, "1234.5", "EUR")
	if err != nil || currency != "1.234,50 €" {
		t.Fatalf("German currency = %q, %v", currency, err)
	}
	percent, err := Task137FormatPercent(ctx, locale, "0.125")
	if err != nil || percent != "12.50%" {
		t.Fatalf("US percent = %q, %v", percent, err)
	}
	relative, err := Task137FormatRelative(ctx, locale, -2, "day")
	if err != nil || relative != "2 days ago" {
		t.Fatalf("relative time = %q, %v", relative, err)
	}
	date, err := Task137FormatDateStyle(ctx, locale, "2024-01-02T03:04:05Z", "long")
	if err != nil || date != "Tuesday, January 2, 2024" {
		t.Fatalf("long date = %q, %v", date, err)
	}
}

func TestTask137I18nCatalogAndLimits(t *testing.T) {
	ctx := context.Background()
	locale, _ := Task137LocaleFor(ctx, "en-US")
	catalog := Task137Catalog{Locale: locale, Messages: map[string]Task137Message{
		"items": {Plurals: map[string]string{"one": "{name} has {count} item", "other": "{name} has {count} items"}},
		"hello": {Text: "Hello, {name}!"},
	}}
	formatted, err := Task137CatalogFormat(ctx, catalog, "items", "2", map[string]string{"name": "Ada"})
	if err != nil || formatted != "Ada has 2 items" {
		t.Fatalf("plural catalog = %q, %v", formatted, err)
	}
	formatted, err = Task137CatalogFormat(ctx, catalog, "hello", "1", map[string]string{"name": "Ada"})
	if err != nil || formatted != "Hello, Ada!" {
		t.Fatalf("text catalog = %q, %v", formatted, err)
	}
	if _, err := Task137CatalogFormat(ctx, catalog, "hello", "1", nil); !errors.Is(err, ErrTask120Invalid) {
		t.Fatalf("missing catalog argument error = %v", err)
	}
	value, err := Task120JSONParse(ctx, []byte(`{"items":{"one":"one item","other":"many items"}}`), Task120JSONLimits{})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Task137CatalogFromValue(locale, value)
	if err != nil {
		t.Fatal(err)
	}
	formatted, err = Task137CatalogFormat(ctx, decoded, "items", "1", nil)
	if err != nil || formatted != "one item" {
		t.Fatalf("decoded catalog = %q, %v", formatted, err)
	}
	overlay := Task137Catalog{Locale: locale, Messages: map[string]Task137Message{"hello": {Text: "Hi, {name}!"}}}
	merged, err := Task137CatalogMerge(catalog, overlay)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Messages["hello"].Text != "Hi, {name}!" || merged.Messages["items"].Plurals["one"] != "{name} has {count} item" {
		t.Fatalf("catalog merge = %#v", merged.Messages)
	}
	if _, err := Task137FormatList(ctx, locale, make([]string, Task137I18nMaxItems+1), Task137ListStandard); err == nil {
		t.Fatal("expected list item limit")
	}
	if _, err := Task137LocaleFor(ctx, "xx"); !errors.Is(err, ErrTask120Invalid) {
		t.Fatalf("unsupported locale error = %v", err)
	}
	if got := Task137SupportedLocaleTags(); !reflect.DeepEqual(got, []string{"ar-EG", "de-DE", "en-US", "es-ES", "fr-FR", "ja-JP"}) {
		t.Fatalf("supported locale order = %#v", got)
	}
}

func TestTask137I18nModuleHelperIsAdditive(t *testing.T) {
	module := Task137I18nModule()
	for _, name := range []string{"task137_version", "supported_locales", "plural_category", "format_list", "format_currency", "format_percent", "format_relative", "format_date_style", "catalog_format"} {
		if _, ok := module[name]; !ok {
			t.Fatalf("i18n module missing %q", name)
		}
	}
}
