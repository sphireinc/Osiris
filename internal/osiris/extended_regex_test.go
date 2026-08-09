package osiris

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func extendedRegexCall(t *testing.T, name string, args ...Value) Value {
	t.Helper()
	fn, ok := extendedRegexModule()[name]
	if !ok || fn.fn == nil || fn.fn.contextBuiltin == nil {
		t.Fatalf("std/regex.%s is not a context builtin", name)
	}
	value, err := fn.fn.contextBuiltin(context.Background(), args)
	if err != nil {
		t.Fatalf("std/regex.%s: %v", name, err)
	}
	return value
}

func TestExtendedRegexModuleSurface(t *testing.T) {
	module := extendedRegexModule()
	want := []string{
		"version", "engine_version", "max_pattern_bytes", "max_input_bytes", "max_matches",
		"max_output_bytes", "max_split_parts", "capture_count", "capture_names", "pattern_info",
		"find_indices", "find_named", "match_count", "replace_first", "replace_n", "split_n", "quote_meta", "escape",
	}
	for _, name := range want {
		if _, ok := module[name]; !ok {
			t.Errorf("missing staged std/regex symbol %q", name)
		}
	}
	if got, _ := module["version"].AsString(); got != ExtendedRegexVersion {
		t.Fatalf("version = %q, want %q", got, ExtendedRegexVersion)
	}
}

func TestExtendedRegexMetadataAndUnicodeOffsets(t *testing.T) {
	compiled, err := ExtendedCompileRegex(context.Background(), `(?P<word>é+)`, StdlibRegexLimits{})
	if err != nil {
		t.Fatal(err)
	}
	info, err := compiled.Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.CaptureCount != 1 || len(info.CaptureNames) != 1 || info.CaptureNames[0] != "word" {
		t.Fatalf("pattern info = %+v", info)
	}
	input := "x éé y"
	indices, err := compiled.FindIndexes(context.Background(), input)
	if err != nil || len(indices) != 1 || indices[0] != (ExtendedRegexIndex{StartByte: 2, EndByte: 6}) {
		t.Fatalf("indices = %#v, %v", indices, err)
	}
	named, err := compiled.FindNamed(context.Background(), input)
	if err != nil || len(named) != 1 || named[0].Captures[0].Name != "word" || named[0].Captures[0].Text != "éé" {
		t.Fatalf("named = %#v, %v", named, err)
	}
	if got := extendedRegexCall(t, "capture_count", stringValue(`(?P<word>é+)`)); extendedRegexTestInt(t, got) != 1 {
		t.Fatalf("capture_count = %s", got)
	}
	if got := extendedRegexCall(t, "match_count", stringValue("a+"), stringValue("baaa ca")); extendedRegexTestInt(t, got) != 2 {
		t.Fatalf("match_count = %s", got)
	}
}

func TestExtendedRegexBoundedVariants(t *testing.T) {
	compiled, err := ExtendedCompileRegex(context.Background(), "a", StdlibRegexLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if got, count, err := compiled.ReplaceN(context.Background(), "a-a-a", "x", 2); err != nil || got != "x-x-a" || count != 2 {
		t.Fatalf("replace_n = %q, %d, %v", got, count, err)
	}
	if got, count, err := compiled.ReplaceN(context.Background(), "a-a", "x", 0); err != nil || got != "a-a" || count != 0 {
		t.Fatalf("replace_n zero = %q, %d, %v", got, count, err)
	}
	parts, err := compiled.SplitN(context.Background(), "a-b-a-c", 3)
	if err != nil || strings.Join(parts, "|") != "|-b-|-c" {
		t.Fatalf("split_n = %#v, %v", parts, err)
	}
	quoted, err := ExtendedRegexQuoteMeta(context.Background(), "a+b?", StdlibRegexLimits{})
	if err != nil || quoted != `a\+b\?` {
		t.Fatalf("quote_meta = %q, %v", quoted, err)
	}
	if got := extendedRegexCall(t, "replace_first", stringValue("a"), stringValue("a-a"), stringValue("x")); func() string { value, _ := got.AsString(); return value }() != "x-a" {
		t.Fatalf("replace_first = %s", got)
	}
	if got := extendedRegexCall(t, "split_n", stringValue("a"), stringValue("a-b-a"), intValue(bigIntFromInt64(2), 64)); !got.Equal(listValue([]Value{stringValue(""), stringValue("-b-a")})) {
		t.Fatalf("split_n module = %s", got)
	}
}

func extendedRegexTestInt(t *testing.T, value Value) int {
	t.Helper()
	result, err := value.Int()
	if err != nil {
		t.Fatalf("%s is not an integer: %v", value, err)
	}
	return result
}

func TestExtendedRegexLimitsCancellationAndInvalidPatterns(t *testing.T) {
	if _, err := ExtendedCompileRegex(context.Background(), "[", StdlibRegexLimits{}); err == nil {
		t.Fatal("invalid pattern compiled")
	}
	if _, err := ExtendedCompileRegex(context.Background(), "abcd", StdlibRegexLimits{MaxPatternBytes: 3}); err == nil {
		t.Fatal("pattern limit was not enforced")
	}
	compiled, err := ExtendedCompileRegex(context.Background(), "a", StdlibRegexLimits{MaxInputBytes: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compiled.FindIndexes(context.Background(), "aaa"); err == nil {
		t.Fatal("input limit was not enforced")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := compiled.FindIndexes(ctx, "a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled find error = %v", err)
	}
	if _, err := ExtendedRegexQuoteMeta(context.Background(), strings.Repeat("+", 20), StdlibRegexLimits{MaxOutputBytes: 10}); err == nil {
		t.Fatal("quote output limit was not enforced")
	}
}
