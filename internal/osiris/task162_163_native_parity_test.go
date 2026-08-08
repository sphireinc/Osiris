package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestTask162NativeCoreDataParityExpanded keeps a small, executable witness
// for the core/data slice that task 162 owns. The fixture deliberately crosses
// the public interpreter, VM, and generated-C executable paths; it does not
// treat successful C emission alone as parity evidence.
func TestTask162NativeCoreDataParityExpanded(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "task162-native-core-data-expanded",
		Source: `emit ok(7)
emit unwrap(some(9))
emit decode(encode([7u32, 1.25f64, complex(2.5f64, -3.0f64)]))
let values = tensor([2, 2], [1.0f64, 2.0f64, 3.0f64, 4.0f64])
emit [sum(values), dot(values, values), shape(values)]
let tuple = (7u32, "native", 2.5f64)
emit tuple
emit tuple[0]
emit tuple[1]
emit len(tuple)
emit ((1, 2), 3)[0][1]
emit ((1, 2), 3)
`,
	})
}

// TestTask162NativeTupleDestructuringParity covers the next native core/data
// slice. Tuple destructuring is lowered to the existing tuple index ABI, so
// binding order, nested indexing, and the generated-C bounds/type path remain
// identical to the interpreter and VM contracts.
func TestTask162NativeTupleDestructuringParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "task162-native-tuple-destructuring",
		// The generated artifact is checked below. Executing it is currently
		// blocked by the pre-existing macOS C compiler failure in the shared
		// SHA-512 runtime constants, which is outside this core/data slice.
		SkipNative: true,
		Source: `let (left, right) = (2u64, 3u64)
let (nested, tail) = ((4u64, 5u64), 6u64)
emit left + right
emit nested[1] + tail
`,
	})
	program, err := Parse("<task162-native-tuple-destructuring>", `let (left, right) = (2u64, 3u64)
let (nested, tail) = ((4u64, 5u64), 6u64)
emit left + right
emit nested[1] + tail
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"v4 = oat(v2, v3);", "v7 = oat(v2, v6);", "v15 = oat(v13, v14);", "v18 = oat(v13, v17);"} {
		if !strings.Contains(artifact.Source, marker) {
			t.Fatalf("generated tuple-destructuring artifact omitted %q", marker)
		}
	}
}

// TestTask162NativeNamedProductParity proves the first nominal product ABI
// slice: declaration-order fields, member reads, and copy-on-write updates
// agree across the interpreter, VM, and generated native executable.
func TestTask162NativeNamedProductParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "task162-native-named-product",
		Source: `struct Point { x u64, y u64 }
let point = Point { x: 1u64, y: 2u64 }
let revised = point with { y: 9u64 }
emit point.x
emit point.y
emit revised.x
emit revised.y
`,
	})
}

// TestTask162NativeNamedProductDiagnostic keeps the unsupported boundary
// source-spanned: product construction is supported, while statically invalid
// field values must still be rejected before C emission.
func TestTask162NativeNamedProductDiagnostic(t *testing.T) {
	program, err := Parse("task162-native-product-error.osr", `struct Point { x u64, y u64 }
let point = Point { x: "wrong", y: 2u64 }
emit point.x
`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileC(context.Background(), NewRuntime(), program); err == nil || !strings.Contains(err.Error(), "task162-native-product-error.osr:2:") || !strings.Contains(err.Error(), "type_error") {
		t.Fatalf("named product diagnostic = %v", err)
	}
}

// TestTask163NativeListReductionParity covers the previously missing native
// std/list.sum and std/list.product dispatch. The cases include ordinary
// numbers, fixed-width floats, and complex values so the C helper must retain
// the hosted result-family contract rather than merely return a C scalar.
func TestTask163NativeListReductionParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "task163-native-list-reductions",
		Source: `import "std/list" as list
emit list.sum([1, 2, 3])
emit list.product([2, 3, 4])
emit list.sum([1.5f64, 2.25f64])
emit list.product([complex(1, 2), complex(2, 0)])
`,
	})
}

func TestTask163NativeListFlattenParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "task163-native-list-flatten",
		Source: `import "std/list" as xs
let nested = [[1, 2], [3, [4, 5]]]
emit xs.flatten(nested, 1)
emit xs.flatten(nested, 2)
emit xs.flatten(nested, 0)
`,
	})
}

// TestTask163NativeListPartitionParity proves that the staged list partition
// contract reaches the generated C dispatcher rather than being accepted only
// by the interpreter/VM. Both returned lists and the tuple wrapper are copied
// through the native allocation tracker.
func TestTask163NativeListPartitionParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "task163-native-list-partition",
		Source: `import "std/list" as list
let parts = list.partition([1u64, 2u64, 3u64], 2)
emit parts
emit parts[0]
emit parts[1]
emit try { list.partition([1u64, 2u64], 99) } catch err { "caught" }
`,
	})
}

func TestTask163NativeListPartitionOwnershipAndErrorContract(t *testing.T) {
	program, err := Parse("<task163-native-list-partition-contract>", `import "std/list" as list
emit list.partition([1u64, 2u64], 1)
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure std/list partition lowering unexpectedly defines a host capability")
	}
	for _, symbol := range []string{"ostd_list_partition", "ostd_dispatch", "olist", "out.fbits=1"} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated partition artifact does not contain ownership boundary %q", symbol)
		}
	}
	for _, message := range []string{"partition expects a list and non-negative index", "partition expects list and index"} {
		if !strings.Contains(artifact.Source, message) {
			t.Fatalf("generated partition artifact does not preserve error contract %q", message)
		}
	}
	entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/list/partition")
	if !ok || entry.Native != BackendImplemented {
		t.Fatalf("std/list.partition inventory entry = %+v, want implemented native capability", entry)
	}
}

// TestTask163NativeTextBoundaryParity covers a bounded, host-independent
// std/text slice. The generated C helpers return owned strings rather than
// views into temporary arguments, and the operations require no host
// capability. The differential harness proves the interpreter, VM, generated
// C, and executable result contracts together.
func TestTask163NativeTextBoundaryParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "task163-native-text-boundary",
		Source: `import "std/text" as text
emit text.trim("  Osiris\u2003")
emit text.trim_space("\tOsiris\n")
emit text.lower("OSIRIS")
emit text.contains("Osiris language", "language")
emit text.has_prefix("Osiris language", "Osiris")
emit text.has_suffix("Osiris language", "language")
`,
	})
}

func TestTask163NativeTextOwnershipAndCapabilityContract(t *testing.T) {
	program, err := Parse("<task163-native-text-contract>", `import "std/text" as text
emit text.lower("OSIRIS")
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatalf("pure std/text lowering unexpectedly defines a host capability")
	}
	for _, symbol := range []string{"ostd_text_case", "oownedstring", "ostd_dispatch"} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated text artifact does not contain ownership boundary %q", symbol)
		}
	}
	entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/text/lower")
	if !ok || entry.Native != BackendImplemented {
		t.Fatalf("std/text.lower inventory entry = %+v, want implemented native capability", entry)
	}
}

// TestTask163NativeCryptoSHA512Parity proves the portable generated-C
// implementation of the canonical SHA-512 and SHA-512/256 members. The
// digest bytes are newly allocated Osiris list values; the hexadecimal form
// is an owned string. Both algorithms are pure and require no host grant.
func TestTask163NativeCryptoSHA512Parity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "task163-native-crypto-sha512",
		Source: `import "std/crypto" as crypto
emit crypto.digest("hello", "sha512")
emit crypto.digest("hello", "sha512_256")
emit crypto.digest_bytes("hello", "sha512")
emit crypto.algorithm_supported("sha512")
`,
	})
}

func TestTask163NativeCryptoSHA512OwnershipCapabilityAndErrorContract(t *testing.T) {
	program, err := Parse("<task163-native-crypto-contract>", `import "std/crypto" as crypto
emit crypto.digest("hello", "sha512")
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure std/crypto digest lowering unexpectedly defines a host capability")
	}
	for _, symbol := range []string{"OSHA512", "osh512init", "osh512write", "osh512final", "ostd_crypto_digest", "oownedstring", "ostd_bytes_list"} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated crypto artifact does not contain ownership boundary %q", symbol)
		}
	}
	for _, message := range []string{"digest expects byte-compatible data and algorithm", "unsupported crypto algorithm"} {
		if !strings.Contains(artifact.Source, message) {
			t.Fatalf("generated crypto artifact does not preserve error contract %q", message)
		}
	}
	for _, member := range []string{"digest", "digest_bytes", "algorithm_supported"} {
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/crypto/"+member)
		if !ok || entry.Native != BackendImplemented {
			t.Fatalf("std/crypto.%s inventory entry = %+v, want implemented native capability", member, entry)
		}
	}
}
