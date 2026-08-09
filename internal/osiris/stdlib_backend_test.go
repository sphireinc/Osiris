package osiris

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestStdlibRegexUnicodeCapturesReplaceAndSplit(t *testing.T) {
	ctx := context.Background()
	r, err := StdlibCompileRegex(ctx, `(?P<word>世界)`, StdlibRegexLimits{MaxInputBytes: 64, MaxMatches: 8, MaxOutputBytes: 64})
	if err != nil {
		t.Fatal(err)
	}
	matched, err := r.Match(ctx, "你好世界")
	if err != nil || !matched {
		t.Fatalf("match = %v, %v", matched, err)
	}
	matches, err := r.Find(ctx, "你好世界世界")
	if err != nil || len(matches) != 2 {
		t.Fatalf("find = %#v, %v", matches, err)
	}
	if got := matches[0].Captures[1]; got.Text != "世界" || got.StartByte != len("你好") {
		t.Fatalf("capture = %#v", got)
	}
	replaced, count, err := r.Replace(ctx, "世界", "<$word>")
	if err != nil || count != 1 || replaced != "<世界>" {
		t.Fatalf("replace = %q, %d, %v", replaced, count, err)
	}
	separator, err := StdlibCompileRegex(ctx, `,`, StdlibRegexLimits{})
	if err != nil {
		t.Fatal(err)
	}
	parts, err := separator.Split(ctx, "a,,b")
	if err != nil || !reflect.DeepEqual(parts, []string{"a", "", "b"}) {
		t.Fatalf("split = %#v, %v", parts, err)
	}
}

func TestStdlibRegexRejectsReDoSFeaturesAndLimits(t *testing.T) {
	if _, err := StdlibCompileRegex(context.Background(), `a(?=b)`, StdlibRegexLimits{}); !errors.Is(err, ErrStdlibInvalid) {
		t.Fatalf("lookahead error = %v", err)
	}
	r, err := StdlibCompileRegex(context.Background(), `a+`, StdlibRegexLimits{MaxInputBytes: 3})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Find(context.Background(), "aaaa"); !errors.Is(err, ErrStdlibLimit) {
		t.Fatalf("input limit error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Match(ctx, "a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}

func TestStdlibCompressionIsStreamingBoundedAndDeterministic(t *testing.T) {
	input := bytes.Repeat([]byte("unicode-世界\n"), 100)
	limits := StdlibCompressionLimits{MaxInputBytes: int64(len(input)), MaxOutputBytes: 1 << 20, MaxExpansionRatio: 1000}
	var first, second bytes.Buffer
	if _, err := StdlibCompress(context.Background(), StdlibCompressionGZIP, bytes.NewReader(input), &first, limits); err != nil {
		t.Fatal(err)
	}
	if _, err := StdlibCompress(context.Background(), StdlibCompressionGZIP, bytes.NewReader(input), &second, limits); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("gzip output is not deterministic")
	}
	var output bytes.Buffer
	stats, err := StdlibDecompress(context.Background(), StdlibCompressionGZIP, bytes.NewReader(first.Bytes()), &output, StdlibCompressionLimits{MaxInputBytes: int64(first.Len()), MaxOutputBytes: int64(len(input)), MaxExpansionRatio: 1000})
	if err != nil || !bytes.Equal(output.Bytes(), input) || stats.OutputBytes != int64(len(input)) {
		t.Fatalf("decompress = %d, %v, equal=%v", stats.OutputBytes, err, bytes.Equal(output.Bytes(), input))
	}
	var limited bytes.Buffer
	if _, err := StdlibDecompress(context.Background(), StdlibCompressionGZIP, bytes.NewReader(first.Bytes()), &limited, StdlibCompressionLimits{MaxInputBytes: int64(first.Len()), MaxOutputBytes: 2, MaxExpansionRatio: 1000}); !errors.Is(err, ErrStdlibLimit) {
		t.Fatalf("output limit error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := StdlibCompress(ctx, StdlibCompressionZlib, bytes.NewReader(input), io.Discard, StdlibCompressionLimits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("compression cancellation = %v", err)
	}
}

func TestStdlibArchiveSafeSubsetAndBombLimits(t *testing.T) {
	entries := []StdlibArchiveEntry{{Name: "dir", Type: "dir"}, {Name: "dir/file.txt", Type: "file", Data: []byte("safe")}}
	var archive bytes.Buffer
	if err := StdlibCreateArchive(context.Background(), entries, &archive, StdlibArchiveLimits{}); err != nil {
		t.Fatal(err)
	}
	var got []StdlibArchiveEntry
	if err := StdlibExtractArchive(context.Background(), bytes.NewReader(archive.Bytes()), StdlibArchiveLimits{}, func(entry StdlibArchiveEntry) error {
		got = append(got, entry)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, entries) {
		t.Fatalf("entries = %#v", got)
	}
	if err := StdlibCreateArchive(context.Background(), []StdlibArchiveEntry{{Name: "../escape", Type: "file"}}, io.Discard, StdlibArchiveLimits{}); !errors.Is(err, ErrStdlibPath) {
		t.Fatalf("traversal error = %v", err)
	}
	if err := StdlibCreateArchive(context.Background(), []StdlibArchiveEntry{{Name: "a", Type: "file"}, {Name: "a", Type: "file"}}, io.Discard, StdlibArchiveLimits{}); !errors.Is(err, ErrStdlibInvalid) {
		t.Fatalf("duplicate error = %v", err)
	}
	var malicious bytes.Buffer
	tw := tar.NewWriter(&malicious)
	if err := tw.WriteHeader(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/outside"}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := StdlibExtractArchive(context.Background(), bytes.NewReader(malicious.Bytes()), StdlibArchiveLimits{}, func(StdlibArchiveEntry) error { return nil }); !errors.Is(err, ErrStdlibInvalid) {
		t.Fatalf("symlink error = %v", err)
	}
}

func TestStdlibOrderedConfigAndSchemaPaths(t *testing.T) {
	value, err := StdlibParseJSONConfig(context.Background(), []byte(`{"b":1,"a":{"ok":true},"items":["x"]}`), StdlibConfigLimits{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := value.JSON()
	if err != nil || string(encoded) != `{"b":1,"a":{"ok":true},"items":["x"]}` {
		t.Fatalf("ordered JSON = %s, %v", encoded, err)
	}
	if _, err := StdlibParseJSONConfig(context.Background(), []byte(`{"a":1,"a":2}`), StdlibConfigLimits{}); !errors.Is(err, ErrStdlibInvalid) {
		t.Fatalf("duplicate JSON error = %v", err)
	}
	props, err := StdlibParsePropertiesConfig(context.Background(), []byte("# comment\nname = \"Osiris\"\ncount = 2\n"), StdlibConfigLimits{})
	if err != nil || len(props.Object) != 2 {
		t.Fatalf("properties = %#v, %v", props, err)
	}
	err = StdlibValidateConfig(value, StdlibConfigSchema{Type: StdlibSchemaObject, Required: []string{"name"}, Properties: map[string]StdlibConfigSchema{"items": {Type: StdlibSchemaArray, Items: &StdlibConfigSchema{Type: StdlibSchemaInteger}}}})
	var validation *StdlibConfigValidationError
	if !errors.As(err, &validation) || len(validation.Errors) != 4 || validation.Errors[0].Path != `$["name"]` || validation.Errors[1].Path != `$["b"]` || validation.Errors[2].Path != `$["a"]` || validation.Errors[3].Path != `$["items"][0]` {
		t.Fatalf("validation = %#v, %v", validation, err)
	}
}

func TestStdlibLocaleNumberDateAndMessagesAreVersioned(t *testing.T) {
	ctx := context.Background()
	en, err := StdlibLocaleFor(ctx, "en")
	if err != nil {
		t.Fatal(err)
	}
	de, err := StdlibLocaleFor(ctx, "de-DE")
	if err != nil {
		t.Fatal(err)
	}
	if en.DataVersion != StdlibLocaleDataVersion {
		t.Fatalf("locale version = %q", en.DataVersion)
	}
	if got, err := StdlibFormatNumber(ctx, en, 1234.5, StdlibNumberOptions{MinFractionDigits: 2, MaxFractionDigits: 2, UseGrouping: true}); err != nil || got != "1,234.50" {
		t.Fatalf("en number = %q, %v", got, err)
	}
	if got, err := StdlibFormatNumber(ctx, de, 1234.5, StdlibNumberOptions{MinFractionDigits: 2, MaxFractionDigits: 2, UseGrouping: true}); err != nil || got != "1.234,50" {
		t.Fatalf("de number = %q, %v", got, err)
	}
	date := time.Date(2024, time.March, 5, 23, 0, 0, 0, time.FixedZone("host-independent", -5*60*60))
	if got, err := StdlibFormatDate(ctx, en, date, StdlibDateOptions{Style: StdlibDateLong}); err != nil || got != "March 5, 2024" {
		t.Fatalf("date = %q, %v", got, err)
	}
	if got, err := StdlibFormatMessage(ctx, de, "items", 2, nil); err != nil || got != "2 Elemente" {
		t.Fatalf("message = %q, %v", got, err)
	}
	if _, err := StdlibLocaleFor(ctx, "xx-YY"); !errors.Is(err, ErrStdlibInvalid) {
		t.Fatalf("unsupported locale = %v", err)
	}
	if !strings.Contains(en.Tag, "en-") {
		t.Fatal("locale tag was not canonicalized")
	}
}

func FuzzStdlibJSONConfigDoesNotPanic(f *testing.F) {
	f.Add([]byte(`{"key":[1,true,null]}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = StdlibParseJSONConfig(context.Background(), data, StdlibConfigLimits{MaxBytes: 4096, MaxDepth: 16, MaxNodes: 1000})
	})
}

func FuzzStdlibRegexCompileDoesNotPanic(f *testing.F) {
	f.Add("(a+)+$")
	f.Fuzz(func(t *testing.T, pattern string) {
		_, _ = StdlibCompileRegex(context.Background(), pattern, StdlibRegexLimits{MaxPatternBytes: 256})
	})
}
