package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestNativeLibraryTextTrimSetParity covers Unicode code-point-set trimming
// through the interpreter, VM, generated C, and native executable.
func TestNativeLibraryTextTrimSetParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-native-text-trimset",
		Source: `import "std/text" as text
emit text.trim_left("¡¡hola!!", "¡")
emit text.trim_right("¡¡hola!!", "!")
emit text.trim_left("¡¡", "¡")
emit text.trim_right("!!", "!")
emit text.trim_left("αβγ", "αβ")
emit text.trim_right("αβγ", "βγ")
emit text.trim_left("unchanged", "")
emit text.trim_right("unchanged", "")
emit try { text.trim_left(1u64, "x") } catch err { "caught-left" }
emit try { text.trim_right("x", 1u64) } catch err { "caught-right" }
`,
	})
}

// TestNativeLibraryTextTrimSetOwnershipErrorAndCapabilityContract checks the
// direct dispatcher boundary: output strings are owned, malformed input and
// wrong kinds retain structured errors, and no host capability is introduced.
func TestNativeLibraryTextTrimSetOwnershipErrorAndCapabilityContract(t *testing.T) {
	program, err := Parse("<nativeLibrary-native-text-trimset-contract>", `import "std/text" as text
emit text.trim_left("¡¡hola!!", "¡")
emit text.trim_right("¡¡hola!!", "!")
emit text.trim_left("¡¡", "¡")
emit text.trim_right("!!", "!")
emit text.trim_left("αβγ", "αβ")
emit text.trim_right("αβγ", "βγ")
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure std/text trimming unexpectedly defines a host capability")
	}
	for _, symbol := range []string{
		"ostd_text_set_contains",
		"ostd_text_trim_set",
		"oownedstring",
		"ostd_dispatch",
	} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated text trim artifact does not contain ownership boundary %q", symbol)
		}
	}
	for _, message := range []string{
		"trim character set expects strings",
		"trim character set expects valid UTF-8",
	} {
		if !strings.Contains(artifact.Source, message) {
			t.Fatalf("generated text trim artifact does not preserve error contract %q", message)
		}
	}
	for _, member := range []string{"trim_left", "trim_right"} {
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/text/"+member)
		if !ok || entry.Native != BackendImplemented {
			t.Fatalf("std/text.%s inventory entry = %+v, want implemented native capability", member, entry)
		}
	}
}
