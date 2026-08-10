package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestNativeLibraryTextParity covers the bounded, host-independent text
// members added to the Native C inventory in this slice. The differential
// harness compares interpreter, VM, generated C, and the generated executable
// using the same source and canonical value rendering.
func TestNativeLibraryTextParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-native-text-members",
		Source: `import "std/text" as text
	emit text.trim("  Osiris  ")
	emit text.trim_space("\tOsiris\n")
emit text.upper("Straße")
	emit text.lower("OSIRIS")
	emit text.contains("Osiris language", "language")
	emit text.has_prefix("Osiris", "Osi")
	emit text.has_suffix("Osiris", "ris")
	emit text.slug("Hello, Osiris!")
	emit text.cat("value/", 7u32, "/", complex(2, -3))
emit text.fields("  α β	")
emit text.join("|", ["a", "b", "c"])
emit text.compare("z", "aa")
emit text.compare("same", "same")
emit text.equal_fold("Σ", "ς")
emit text.equal_fold("K", "K")
emit text.contains_any("rhythm世", "ae世")
emit text.contains_any("rhythm", "ae")
emit text.count("aaaa", "aa")
emit text.count("abc", "x")
emit text.count("a世", "")
emit text.index("a世b", "b")
emit text.index("a世b", "")
emit text.last_index("é-é", "é")
emit text.last_index("a世b", "")
emit text.index_any("αβç", "çx")
emit text.last_index_any("a世b世", "世")
emit text.cut("a=b", "=")
emit text.cut_prefix("prefix-value", "prefix")
emit text.cut_suffix("value-suffix", "suffix")
emit text.trim_prefix("prefix-value", "prefix")
emit text.trim_suffix("value-suffix", "suffix")
emit text.replace("a-b-c", "-", ":")
emit text.replace_n("a-b-c", "-", ":", 1)
emit text.split("a,,世,", ",")
emit text.split_after("a,,世,", ",")
emit text.split_n("a,b,c", ",", 2)
	emit text.split_after_n("a,b,c", ",", 2)
	emit text.chunks("aβ世d", 2)
	emit text.repeat("世", 3)
`,
	})
}

// TestNativeLibraryTextOwnershipCapabilityAndErrorContract checks the C
// helper boundary independently of output parity. Every result-producing
// operation either returns an owned value or a scalar; no OS capability macro
// is introduced by this pure-text slice.
func TestNativeLibraryTextOwnershipCapabilityAndErrorContract(t *testing.T) {
	program, err := Parse("<nativeLibrary-native-text-contract>", `import "std/text" as text
	emit text.trim("  OSIRIS  ")
	emit text.trim_space("\tOSIRIS\n")
	emit text.upper("OSIRIS")
	emit text.lower("osiris")
	emit text.contains("osiris", "sir")
	emit text.has_prefix("osiris", "osi")
	emit text.has_suffix("osiris", "ris")
	emit text.slug("Hello, Osiris!")
	emit text.cat("value/", 7u32)
emit text.fields("a b")
emit text.join(",", ["a", "b"])
emit text.compare("a", "b")
emit text.equal_fold("a", "A")
emit text.contains_any("rhythm世", "ae世")
emit text.count("banana", "an")
emit text.index("a世b", "b")
emit text.last_index("a世b世", "世")
emit text.index_any("αβç", "çx")
emit text.last_index_any("a世b世", "世")
emit text.cut("a=b", "=")
emit text.cut_prefix("prefix-value", "prefix")
emit text.cut_suffix("value-suffix", "suffix")
emit text.trim_prefix("prefix-value", "prefix")
emit text.trim_suffix("value-suffix", "suffix")
emit text.replace("a-b", "-", ":")
emit text.replace_n("a-b-c", "-", ":", 1)
emit text.split("a,,世,", ",")
emit text.split_after("a,,世,", ",")
emit text.split_n("a,b,c", ",", 2)
	emit text.split_after_n("a,b,c", ",", 2)
	emit text.chunks("aβ世d", 2)
	emit text.repeat("x", 2)
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure std/text members unexpectedly define a host capability")
	}
	for _, symbol := range []string{
		"ostd_text_trim", "ostd_text_case", "ostd_text_fields", "ostd_text_join",
		"ostd_text_equal_fold", "ostd_text_count", "ostd_text_index",
		"oownedstring", "ooptionSome", "ostd_dispatch",
	} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated text artifact does not contain ownership boundary %q", symbol)
		}
	}
	for _, message := range []string{
		"text expects valid UTF-8", "count expects strings", "repeat result is too large",
	} {
		if !strings.Contains(artifact.Source, message) {
			t.Fatalf("generated text artifact does not preserve error contract %q", message)
		}
	}
	for _, member := range []string{
		"trim", "trim_space", "upper", "lower", "replace", "repeat", "split", "join", "chunks", "slug", "cat", "compare", "equal_fold", "contains", "contains_any", "has_prefix", "has_suffix", "count", "index", "index_any", "last_index", "last_index_any", "cut", "cut_prefix", "cut_suffix", "fields", "split_after", "split_n", "split_after_n", "trim_left", "trim_right", "trim_prefix", "trim_suffix", "replace_n",
	} {
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/text/"+member)
		if !ok || entry.Native != BackendImplemented {
			t.Fatalf("std/text.%s inventory entry = %+v, want implemented native capability", member, entry)
		}
	}
}
