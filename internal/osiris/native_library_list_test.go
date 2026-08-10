package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestNativeLibraryListPureParity covers the host-independent std/list
// operations that are already expressible in the shared interpreter/VM
// contract. The same source is executed by the interpreter, VM, generated C,
// and the generated native executable.
func TestNativeLibraryListPureParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-native-list-pure",
		Source: `import "std/list" as list
emit list.range(4)
emit list.len([3u64, 1u64, 3u64])
emit list.take([3u64, 1u64, 3u64], 2)
emit list.sum([1u64, 2u64, 3u64])
emit list.product([2u64, 3u64, 4u64])
let values = [3u64, 1u64, 3u64, 2u64, 1u64]
emit list.reverse(values)
emit list.at(values, 1)
emit list.index_of(values, 3u64)
emit list.last_index_of(values, 1u64)
emit list.contains(values, 2u64)
emit list.count(values, 3u64)
emit list.sort(values)
emit list.unique(values)
emit list.zip([1u64, 2u64], ["a", "b", "c"])
emit list.concat([1u64], [2u64, 3u64], [])
emit list.concat()
emit list.repeat("x", 3)
emit list.empty()
emit list.all([true, true])
emit list.all([])
emit list.any([false, true])
emit list.any([])
emit list.none([false, false])
emit list.none([false, true])
emit list.enumerate(["a", "b"])
emit list.slice(values, 1, 4)
emit list.chunks(values, 2)
emit list.windows(values, 3, 2)
emit list.flatten([[1u64, 2u64], [3u64]], 1)
emit try { list.at(values, 99) } catch err { "caught-at" }
emit try { list.repeat("x", -1) } catch err { "caught-repeat" }
emit try { list.sort([1u64, "bad"]) } catch err { "caught-sort" }
emit try { list.zip([1u64], "bad") } catch err { "caught-zip" }
emit try { list.windows(values, 0, 1) } catch err { "caught-windows" }
`,
	})
}

// TestNativeLibraryListCompleteOwnershipAndCapabilityContract proves the
// complete exported std/list surface one member at a time. Every operation
// returns an owned value or scalar, copies nested list storage where the
// language contract requires an owned result, and remains capability-free.
func TestNativeLibraryListCompleteOwnershipAndCapabilityContract(t *testing.T) {
	program, err := Parse("<nativeLibrary-native-list-complete-contract>", `import "std/list" as list
emit list.range(3)
emit list.len([1u64, 2u64])
emit list.take([1u64, 2u64], 1)
emit list.sum([1u64, 2u64])
emit list.product([2u64, 3u64])
emit list.reverse([1u64, 2u64])
emit list.at([1u64], 0)
emit list.index_of([1u64], 1u64)
emit list.last_index_of([1u64], 1u64)
emit list.contains([1u64], 1u64)
emit list.count([1u64], 1u64)
emit list.sort([2u64, 1u64])
emit list.unique([1u64, 1u64])
emit list.partition([1u64, 2u64], 1)
emit list.slice([1u64, 2u64], 0, 1)
emit list.chunks([1u64, 2u64], 1)
emit list.windows([1u64, 2u64], 1, 1)
emit list.zip([1u64], [2u64])
emit list.concat([1u64], [2u64])
emit list.repeat(1u64, 2)
emit list.empty()
emit list.all([true])
emit list.any([true])
emit list.none([false])
emit list.enumerate([1u64])
emit list.flatten([[1u64]], 1)
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure std/list lowering unexpectedly defines a host capability")
	}
	for _, symbol := range []string{
		"ostd_list_range", "ostd_list_take", "ostd_list_sum",
		"ostd_list_product", "ostd_list_reverse", "ostd_list_at",
		"ostd_list_index", "ostd_list_sort", "ostd_list_unique",
		"ostd_list_partition", "ostd_list_shape", "ostd_list_zip",
		"ostd_list_flatten", "ostd_dispatch",
	} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated list artifact does not contain %q", symbol)
		}
	}
	for _, member := range []string{
		"range", "len", "take", "sum", "product", "reverse", "at",
		"index_of", "last_index_of", "contains", "count", "sort", "unique",
		"partition", "slice", "chunks", "windows", "zip", "concat", "repeat",
		"empty", "all", "any", "none", "enumerate", "flatten",
	} {
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/list/"+member)
		if !ok || entry.Native != BackendImplemented {
			t.Fatalf("std/list.%s inventory entry = %+v, want implemented native capability", member, entry)
		}
	}
}

// TestNativeLibraryListPureOwnershipErrorAndCapabilityContract checks every
// member in the bounded pure slice individually. Each result is materialized
// through the tracked C list constructor; no input pointer or host capability
// is exposed by these operations.
func TestNativeLibraryListPureOwnershipErrorAndCapabilityContract(t *testing.T) {
	program, err := Parse("<nativeLibrary-native-list-pure-contract>", `import "std/list" as list
emit list.reverse([1u64, 2u64])
emit list.at([1u64], 0)
emit list.index_of([1u64], 1u64)
emit list.last_index_of([1u64], 1u64)
emit list.contains([1u64], 1u64)
emit list.count([1u64], 1u64)
emit list.sort([2u64, 1u64])
emit list.unique([1u64, 1u64])
emit list.zip([1u64], [2u64])
emit list.concat([1u64], [2u64])
emit list.repeat(1u64, 2)
emit list.empty()
emit list.all([true])
emit list.any([true])
emit list.none([false])
emit list.enumerate([1u64])
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure std/list lowering unexpectedly defines a host capability")
	}
	for _, symbol := range []string{
		"ostd_list_reverse",
		"ostd_list_at",
		"ostd_list_index",
		"ostd_list_sort",
		"ostd_list_unique",
		"ostd_list_zip",
		"olist",
		"ostd_dispatch",
		"list index out of range",
		"sort expects a list",
		"list predicate expects a list",
	} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated list artifact does not contain %q", symbol)
		}
	}
	for _, member := range []string{
		"reverse", "at", "index_of", "last_index_of", "contains", "count",
		"sort", "unique", "zip", "concat", "repeat", "empty", "all", "any",
		"none", "enumerate",
	} {
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/list/"+member)
		if !ok || entry.Native != BackendImplemented {
			t.Fatalf("std/list.%s inventory entry = %+v, want implemented native capability", member, entry)
		}
	}
}
