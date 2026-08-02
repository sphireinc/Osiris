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

func TestTask120RegexUnicodeCapturesReplaceAndSplit(t *testing.T) {
	ctx := context.Background()
	r, err := Task120CompileRegex(ctx, `(?P<word>世界)`, Task120RegexLimits{MaxInputBytes: 64, MaxMatches: 8, MaxOutputBytes: 64})
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
	separator, err := Task120CompileRegex(ctx, `,`, Task120RegexLimits{})
	if err != nil {
		t.Fatal(err)
	}
	parts, err := separator.Split(ctx, "a,,b")
	if err != nil || !reflect.DeepEqual(parts, []string{"a", "", "b"}) {
		t.Fatalf("split = %#v, %v", parts, err)
	}
}

func TestTask120RegexRejectsReDoSFeaturesAndLimits(t *testing.T) {
	if _, err := Task120CompileRegex(context.Background(), `a(?=b)`, Task120RegexLimits{}); !errors.Is(err, ErrTask120Invalid) {
		t.Fatalf("lookahead error = %v", err)
	}
	r, err := Task120CompileRegex(context.Background(), `a+`, Task120RegexLimits{MaxInputBytes: 3})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Find(context.Background(), "aaaa"); !errors.Is(err, ErrTask120Limit) {
		t.Fatalf("input limit error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Match(ctx, "a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}

func TestTask120CompressionIsStreamingBoundedAndDeterministic(t *testing.T) {
	input := bytes.Repeat([]byte("unicode-世界\n"), 100)
	limits := Task120CompressionLimits{MaxInputBytes: int64(len(input)), MaxOutputBytes: 1 << 20, MaxExpansionRatio: 1000}
	var first, second bytes.Buffer
	if _, err := Task120Compress(context.Background(), Task120CompressionGZIP, bytes.NewReader(input), &first, limits); err != nil {
		t.Fatal(err)
	}
	if _, err := Task120Compress(context.Background(), Task120CompressionGZIP, bytes.NewReader(input), &second, limits); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("gzip output is not deterministic")
	}
	var output bytes.Buffer
	stats, err := Task120Decompress(context.Background(), Task120CompressionGZIP, bytes.NewReader(first.Bytes()), &output, Task120CompressionLimits{MaxInputBytes: int64(first.Len()), MaxOutputBytes: int64(len(input)), MaxExpansionRatio: 1000})
	if err != nil || !bytes.Equal(output.Bytes(), input) || stats.OutputBytes != int64(len(input)) {
		t.Fatalf("decompress = %d, %v, equal=%v", stats.OutputBytes, err, bytes.Equal(output.Bytes(), input))
	}
	var limited bytes.Buffer
	if _, err := Task120Decompress(context.Background(), Task120CompressionGZIP, bytes.NewReader(first.Bytes()), &limited, Task120CompressionLimits{MaxInputBytes: int64(first.Len()), MaxOutputBytes: 2, MaxExpansionRatio: 1000}); !errors.Is(err, ErrTask120Limit) {
		t.Fatalf("output limit error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Task120Compress(ctx, Task120CompressionZlib, bytes.NewReader(input), io.Discard, Task120CompressionLimits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("compression cancellation = %v", err)
	}
}

func TestTask120ArchiveSafeSubsetAndBombLimits(t *testing.T) {
	entries := []Task120ArchiveEntry{{Name: "dir", Type: "dir"}, {Name: "dir/file.txt", Type: "file", Data: []byte("safe")}}
	var archive bytes.Buffer
	if err := Task120CreateArchive(context.Background(), entries, &archive, Task120ArchiveLimits{}); err != nil {
		t.Fatal(err)
	}
	var got []Task120ArchiveEntry
	if err := Task120ExtractArchive(context.Background(), bytes.NewReader(archive.Bytes()), Task120ArchiveLimits{}, func(entry Task120ArchiveEntry) error {
		got = append(got, entry)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, entries) {
		t.Fatalf("entries = %#v", got)
	}
	if err := Task120CreateArchive(context.Background(), []Task120ArchiveEntry{{Name: "../escape", Type: "file"}}, io.Discard, Task120ArchiveLimits{}); !errors.Is(err, ErrTask120Path) {
		t.Fatalf("traversal error = %v", err)
	}
	if err := Task120CreateArchive(context.Background(), []Task120ArchiveEntry{{Name: "a", Type: "file"}, {Name: "a", Type: "file"}}, io.Discard, Task120ArchiveLimits{}); !errors.Is(err, ErrTask120Invalid) {
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
	if err := Task120ExtractArchive(context.Background(), bytes.NewReader(malicious.Bytes()), Task120ArchiveLimits{}, func(Task120ArchiveEntry) error { return nil }); !errors.Is(err, ErrTask120Invalid) {
		t.Fatalf("symlink error = %v", err)
	}
}

func TestTask120OrderedConfigAndSchemaPaths(t *testing.T) {
	value, err := Task120ParseJSONConfig(context.Background(), []byte(`{"b":1,"a":{"ok":true},"items":["x"]}`), Task120ConfigLimits{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := value.JSON()
	if err != nil || string(encoded) != `{"b":1,"a":{"ok":true},"items":["x"]}` {
		t.Fatalf("ordered JSON = %s, %v", encoded, err)
	}
	if _, err := Task120ParseJSONConfig(context.Background(), []byte(`{"a":1,"a":2}`), Task120ConfigLimits{}); !errors.Is(err, ErrTask120Invalid) {
		t.Fatalf("duplicate JSON error = %v", err)
	}
	props, err := Task120ParsePropertiesConfig(context.Background(), []byte("# comment\nname = \"Osiris\"\ncount = 2\n"), Task120ConfigLimits{})
	if err != nil || len(props.Object) != 2 {
		t.Fatalf("properties = %#v, %v", props, err)
	}
	err = Task120ValidateConfig(value, Task120ConfigSchema{Type: Task120SchemaObject, Required: []string{"name"}, Properties: map[string]Task120ConfigSchema{"items": {Type: Task120SchemaArray, Items: &Task120ConfigSchema{Type: Task120SchemaInteger}}}})
	var validation *Task120ConfigValidationError
	if !errors.As(err, &validation) || len(validation.Errors) != 4 || validation.Errors[0].Path != `$["name"]` || validation.Errors[1].Path != `$["b"]` || validation.Errors[2].Path != `$["a"]` || validation.Errors[3].Path != `$["items"][0]` {
		t.Fatalf("validation = %#v, %v", validation, err)
	}
}

func TestTask120LocaleNumberDateAndMessagesAreVersioned(t *testing.T) {
	ctx := context.Background()
	en, err := Task120LocaleFor(ctx, "en")
	if err != nil {
		t.Fatal(err)
	}
	de, err := Task120LocaleFor(ctx, "de-DE")
	if err != nil {
		t.Fatal(err)
	}
	if en.DataVersion != Task120LocaleDataVersion {
		t.Fatalf("locale version = %q", en.DataVersion)
	}
	if got, err := Task120FormatNumber(ctx, en, 1234.5, Task120NumberOptions{MinFractionDigits: 2, MaxFractionDigits: 2, UseGrouping: true}); err != nil || got != "1,234.50" {
		t.Fatalf("en number = %q, %v", got, err)
	}
	if got, err := Task120FormatNumber(ctx, de, 1234.5, Task120NumberOptions{MinFractionDigits: 2, MaxFractionDigits: 2, UseGrouping: true}); err != nil || got != "1.234,50" {
		t.Fatalf("de number = %q, %v", got, err)
	}
	date := time.Date(2024, time.March, 5, 23, 0, 0, 0, time.FixedZone("host-independent", -5*60*60))
	if got, err := Task120FormatDate(ctx, en, date, Task120DateOptions{Style: Task120DateLong}); err != nil || got != "March 5, 2024" {
		t.Fatalf("date = %q, %v", got, err)
	}
	if got, err := Task120FormatMessage(ctx, de, "items", 2, nil); err != nil || got != "2 Elemente" {
		t.Fatalf("message = %q, %v", got, err)
	}
	if _, err := Task120LocaleFor(ctx, "xx-YY"); !errors.Is(err, ErrTask120Invalid) {
		t.Fatalf("unsupported locale = %v", err)
	}
	if !strings.Contains(en.Tag, "en-") {
		t.Fatal("locale tag was not canonicalized")
	}
}

func FuzzTask120JSONConfigDoesNotPanic(f *testing.F) {
	f.Add([]byte(`{"key":[1,true,null]}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = Task120ParseJSONConfig(context.Background(), data, Task120ConfigLimits{MaxBytes: 4096, MaxDepth: 16, MaxNodes: 1000})
	})
}

func FuzzTask120RegexCompileDoesNotPanic(f *testing.F) {
	f.Add("(a+)+$")
	f.Fuzz(func(t *testing.T, pattern string) {
		_, _ = Task120CompileRegex(context.Background(), pattern, Task120RegexLimits{MaxPatternBytes: 256})
	})
}
