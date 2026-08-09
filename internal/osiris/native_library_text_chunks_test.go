package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestNativeLibraryTextChunksParity compares Unicode code-point chunking
// through the interpreter, VM, generated C, and native executable paths.
func TestNativeLibraryTextChunksParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-native-text-chunks",
		Source: `import "std/text" as text
emit text.chunks("a世界b", 2)
emit text.chunks("αβγ", 1u64)
emit text.chunks("", 3)
emit try { text.chunks("abc", 0) } catch err { "caught-zero" }
emit try { text.chunks("abc", -1) } catch err { "caught-negative" }
emit try { text.chunks(1u64, 2) } catch err { "caught-input" }
emit try { text.chunks("abc", 1.5) } catch err { "caught-width" }
`,
	})
}

// TestNativeLibraryTextChunksOwnershipErrorAndCapabilityContract checks the
// direct generated-C call, copied result strings, bounded input validation,
// structured invalid-width errors, and absence of host capabilities.
func TestNativeLibraryTextChunksOwnershipErrorAndCapabilityContract(t *testing.T) {
	program, err := Parse("<nativeLibrary-native-text-chunks-contract>", `import "std/text" as text
emit text.chunks("a世界b", 2)
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure std/text.chunks lowering unexpectedly defines a host capability")
	}
	if strings.Count(artifact.Source, "ostd_text_chunks(") < 2 {
		t.Fatal("generated text chunks artifact does not contain both the helper and direct call")
	}
	for _, symbol := range []string{"ostd_text_arg", "oownedstring", "olist"} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated text chunks artifact does not contain ownership boundary %q", symbol)
		}
	}
	for _, marker := range []string{
		"chunks expects text and positive size",
		"chunks expects valid UTF-8",
		"text input exceeds the 1048576-byte limit",
	} {
		if !strings.Contains(artifact.Source, marker) {
			t.Fatalf("generated text chunks artifact does not preserve error contract %q", marker)
		}
	}
	entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/text/chunks")
	if !ok || entry.Native != BackendImplemented {
		t.Fatalf("std/text.chunks inventory entry = %+v, want implemented native capability", entry)
	}
}
