package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestNativeLibraryListChunksParity compares the bounded chunking operation
// through the interpreter, VM, generated C, and native executable paths.
func TestNativeLibraryListChunksParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "native-list-chunks",
		Source: `import "std/list" as list
let values = [0u64, 1u64, 2u64, 3u64, 4u64]
emit list.chunks(values, 2)
emit list.chunks(values, 3)
emit list.chunks([], 2)
emit try { list.chunks(values, 0) } catch err { "caught-zero" }
emit try { list.chunks(values, -1) } catch err { "caught-negative" }
emit try { list.chunks(values, 100001) } catch err { "caught-width-limit" }
emit try { list.chunks("bad", 2) } catch err { "caught-input" }
`,
	})
}

// TestNativeLibraryListChunksOwnershipErrorAndCapabilityContract checks the
// generated helper, independent ownership of every chunk, structured invalid
// input errors, and the absence of host capability definitions.
func TestNativeLibraryListChunksOwnershipErrorAndCapabilityContract(t *testing.T) {
	program, err := Parse("<native-list-chunks-contract>", `import "std/list" as list
emit list.chunks([1u64, 2u64, 3u64], 2)
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure std/list.chunks lowering unexpectedly defines a host capability")
	}
	for _, symbol := range []string{
		"ostd_list_shape",
		"ostd_dispatch",
		"olist",
		"chunks width must be positive and within the item limit",
		"list input exceeds limit 100000",
	} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated chunks artifact does not contain %q", symbol)
		}
	}
	entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/list/chunks")
	if !ok || entry.Native != BackendImplemented {
		t.Fatalf("std/list.chunks inventory entry = %+v, want implemented native capability", entry)
	}
}
