package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestNativeCoreDataParityExpanded keeps a small, executable witness
// for the core/data slice. The fixture deliberately crosses
// the public interpreter, VM, and generated-C executable paths; it does not
// treat successful C emission alone as parity evidence.
func TestNativeCoreDataParityExpanded(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeCore-native-core-data-expanded",
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

// TestNativeCoreTupleDestructuringParity covers the next native core/data
// slice. Tuple destructuring is lowered to the existing tuple index ABI, so
// binding order, nested indexing, and the generated-C bounds/type path remain
// identical to the interpreter and VM contracts.
func TestNativeCoreTupleDestructuringParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeCore-native-tuple-destructuring",
		// The generated artifact is checked below. This fixture remains artifact-only
		// because tuple lowering is covered separately from the shared host runtime.
		SkipNative: true,
		Source: `let (left, right) = (2u64, 3u64)
let (nested, tail) = ((4u64, 5u64), 6u64)
emit left + right
emit nested[1] + tail
`,
	})
	program, err := Parse("<nativeCore-native-tuple-destructuring>", `let (left, right) = (2u64, 3u64)
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
	if got := strings.Count(artifact.Source, " = oat_product_or_value("); got < 4 {
		t.Fatalf("generated tuple-destructuring artifact emitted %d tuple index operations, want at least 4", got)
	}
}

// TestNativeCoreNamedProductParity proves the first nominal product ABI
// slice: declaration-order fields, member reads, and copy-on-write updates
// agree across the interpreter, VM, and generated native executable.
func TestNativeCoreNamedProductParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeCore-native-named-product",
		Source: `struct Point { x u64, y u64 }
let point = Point { x: 1u64, y: 2u64 }
let revised = point with { y: 9u64 }
emit point
emit point.x
emit point.y
emit point["x"]
emit point["y"]
emit revised.x
emit revised.y
emit try { point["missing"] } catch err { "caught" }
emit try { point[0] } catch err { "caught-index" }
`,
	})
}

// TestNativeCoreNamedProductRenderingAndIndexContract checks the generated
// product ABI directly. The product is rendered as a nominal value, while a
// string index resolves the same ordered field map as dot-member access. The
// source and field values are copied into tracked product storage; no emitted
// index path requires a host capability.
func TestNativeCoreNamedProductRenderingAndIndexContract(t *testing.T) {
	program, err := Parse("<nativeCore-native-product-render-index>", `struct Point { x u64, y u64 }
let point = Point { x: 1u64, y: 2u64 }
emit point
emit point["x"]
emit try { point["missing"] } catch err { "caught" }
emit try { point[0] } catch err { "caught-index" }
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure product construction/rendering/indexing unexpectedly defines a host capability")
	}
	for _, symbol := range []string{
		"O_PRODUCT",
		"oproduct(\"Point\"",
		"oat_product_or_value",
		"oproductupdate",
		"product field not found",
		"product index expects a string field name",
		"oprint_product_value",
		"malloc(index.as.s.len + 1)",
		"free(key)",
	} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated product artifact does not contain ownership/access boundary %q", symbol)
		}
	}
}

// TestNativeCoreNamedProductDiagnostic keeps the unsupported boundary
// source-spanned: product construction is supported, while statically invalid
// field values must still be rejected before C emission.
func TestNativeCoreNamedProductDiagnostic(t *testing.T) {
	program, err := Parse("nativeCore-native-product-error.osr", `struct Point { x u64, y u64 }
let point = Point { x: "wrong", y: 2u64 }
emit point.x
`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileC(context.Background(), NewRuntime(), program); err == nil || !strings.Contains(err.Error(), "nativeCore-native-product-error.osr:2:") || !strings.Contains(err.Error(), "type_error") {
		t.Fatalf("named product diagnostic = %v", err)
	}
}

// TestNativeCoreAliasesAndNewtypesParity proves the remaining declaration
// slice without adding a second runtime representation. Transparent aliases
// lower to a value copy, while newtypes lower to the existing nominal product
// ABI with one field named value.
func TestNativeCoreAliasesAndNewtypesParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeCore-native-aliases-newtypes",
		Source: `type Count = u64
newtype MissionID u64
let count = Count(7u64)
let id = MissionID(9u64)
emit count
emit id
emit id.value
emit id["value"]
`,
	})
}

func TestNativeCoreAliasesAndNewtypesIRContract(t *testing.T) {
	program, err := Parse("<nativeCore-native-aliases-newtypes-ir>", `type Count = u64
newtype MissionID u64
let count = Count(7u64)
let id = MissionID(9u64)
emit count
emit id
`)
	if err != nil {
		t.Fatal(err)
	}
	ir, err := NewRuntime().BuildIR(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if got := ir.Aliases["Count"]; got != "u64" {
		t.Fatalf("alias target = %q, want u64", got)
	}
	var foundAliasCopy, foundNewtype bool
	for _, fn := range ir.Functions {
		for _, block := range fn.Blocks {
			for _, instr := range block.Instrs {
				if instr.Op == "copy" && instr.Text == "Count" {
					foundAliasCopy = true
				}
				if instr.Op == "newtype" && instr.Text == "MissionID" && len(instr.Args) == 1 && len(instr.FieldNames) == 1 && instr.FieldNames[0] == "value" {
					foundNewtype = true
				}
			}
		}
	}
	if !foundAliasCopy || !foundNewtype {
		t.Fatalf("IR alias/newtype lowering = alias copy %v, newtype %v", foundAliasCopy, foundNewtype)
	}
}

func TestNativeCoreAliasesAndNewtypesOwnershipContract(t *testing.T) {
	program, err := Parse("<nativeCore-native-aliases-newtypes-contract>", `type Count = u64
newtype MissionID u64
let count = Count(7u64)
let id = MissionID(count)
emit id
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("alias/newtype lowering unexpectedly defines a host capability")
	}
	for _, symbol := range []string{
		"onewtype(\"MissionID\"",
		"\"value\"",
		"omember_any",
		"oat_product_or_value",
		"oproduct",
		"free(key)",
	} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated alias/newtype artifact does not contain %q", symbol)
		}
	}
	if strings.Contains(artifact.Source, "native C backend: product-type declarations are not yet supported") {
		t.Fatal("generated alias/newtype artifact retained the old unsupported declaration boundary")
	}
}

func TestNativeCoreNewtypeDiagnostic(t *testing.T) {
	program, err := Parse("nativeCore-native-newtype-error.osr", `newtype MissionID u64
let bad = MissionID("wrong")
emit bad
`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileC(context.Background(), NewRuntime(), program); err == nil ||
		!strings.Contains(err.Error(), "nativeCore-native-newtype-error.osr:2:") ||
		!strings.Contains(err.Error(), "type_error") {
		t.Fatalf("newtype diagnostic = %v", err)
	}
}

// TestNativeLibraryListReductionParity covers the previously missing native
// std/list.sum and std/list.product dispatch. The cases include ordinary
// numbers, fixed-width floats, and complex values so the C helper must retain
// the hosted result-family contract rather than merely return a C scalar.
func TestNativeLibraryListReductionParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-native-list-reductions",
		Source: `import "std/list" as list
emit list.sum([1, 2, 3])
emit list.product([2, 3, 4])
emit list.sum([1.5f64, 2.25f64])
emit list.product([complex(1, 2), complex(2, 0)])
`,
	})
}

func TestNativeLibraryListFlattenParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-native-list-flatten",
		Source: `import "std/list" as xs
let nested = [[1, 2], [3, [4, 5]]]
emit xs.flatten(nested, 1)
emit xs.flatten(nested, 2)
emit xs.flatten(nested, 0)
`,
	})
}

// TestNativeLibraryListPartitionParity proves that the staged list partition
// contract reaches the generated C dispatcher rather than being accepted only
// by the interpreter/VM. Both returned lists and the tuple wrapper are copied
// through the native allocation tracker.
func TestNativeLibraryListPartitionParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-native-list-partition",
		Source: `import "std/list" as list
let parts = list.partition([1u64, 2u64, 3u64], 2)
emit parts
emit parts[0]
emit parts[1]
emit try { list.partition([1u64, 2u64], 99) } catch err { "caught" }
`,
	})
}

func TestNativeLibraryListPartitionOwnershipAndErrorContract(t *testing.T) {
	program, err := Parse("<nativeLibrary-native-list-partition-contract>", `import "std/list" as list
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

// TestNativeLibraryTextBoundaryParity covers a bounded, host-independent
// std/text slice. The generated C helpers return owned strings rather than
// views into temporary arguments, and the operations require no host
// capability. The differential harness proves the interpreter, VM, generated
// C, and executable result contracts together.
func TestNativeLibraryTextBoundaryParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-native-text-boundary",
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

func TestNativeLibraryTextOwnershipAndCapabilityContract(t *testing.T) {
	program, err := Parse("<nativeLibrary-native-text-contract>", `import "std/text" as text
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

// TestNativeLibraryCryptoSHA512Parity proves the portable generated-C
// implementation of the canonical SHA-512 and SHA-512/256 members. The
// digest bytes are newly allocated Osiris list values; the hexadecimal form
// is an owned string. Both algorithms are pure and require no host grant.
func TestNativeLibraryCryptoSHA512Parity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-native-crypto-sha512",
		Source: `import "std/crypto" as crypto
emit crypto.digest("hello", "sha512")
emit crypto.digest("hello", "sha512_256")
emit crypto.digest_bytes("hello", "sha512")
emit crypto.algorithm_supported("sha512")
`,
	})
}

func TestNativeLibraryCryptoSHA512OwnershipCapabilityAndErrorContract(t *testing.T) {
	program, err := Parse("<nativeLibrary-native-crypto-contract>", `import "std/crypto" as crypto
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

// TestNativeLibraryTimePureValueParity covers the next portable std/time
// family: instant/duration ordering and UTC civil accessors. It intentionally
// avoids clock reads and timezone providers; the differential harness still
// executes the interpreter, VM, generated C, and native executable.
func TestNativeLibraryTimePureValueParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-native-time-pure-values",
		Source: `import "std/time" as time
let stamp = time.date(2024, 2, 29, 23, 59, 58, 123456789, time.utc_location)
emit time.compare(time.parse_duration("1s"), time.parse_duration("2s"))
emit time.equal(stamp, time.add(stamp, time.parse_duration("0s")))
emit time.before(time.date(2024, 1, 1, 0, 0, 0, 0, time.utc_location), stamp)
emit time.after(stamp, time.date(2024, 1, 1, 0, 0, 0, 0, time.utc_location))
emit time.year(stamp), time.month(stamp), time.day(stamp), time.year_day(stamp)
emit time.weekday(stamp)
emit try { time.year("not a datetime") } catch err { "caught" }
emit try { time.compare(time.parse_duration("1s"), stamp) } catch err { "caught" }
`,
	})
}

func TestNativeLibraryTimePureValueOwnershipCapabilityAndErrorContract(t *testing.T) {
	program, err := Parse("<nativeLibrary-native-time-contract>", `import "std/time" as time
let stamp = time.date(2024, 2, 29, 23, 59, 58, 123456789, time.utc_location)
emit time.weekday(stamp)
emit time.year(stamp)
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure std/time value operations unexpectedly define a host capability")
	}
	for _, symbol := range []string{"ostd_time_compare", "ostd_time_predicate", "ostd_time_civil", "ocivil_from_days", "ostd_dispatch"} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated time artifact does not contain %q", symbol)
		}
	}
	for _, message := range []string{"time comparison expects two durations or two datetimes", "datetime component expects datetime", "year expects 1 datetime", "weekday expects 1 datetime"} {
		if !strings.Contains(artifact.Source, message) {
			t.Fatalf("generated time artifact does not preserve error contract %q", message)
		}
	}
	for _, member := range []string{"compare", "equal", "before", "after", "year", "month", "day", "weekday", "year_day"} {
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/time/"+member)
		if !ok || entry.Native != BackendImplemented {
			t.Fatalf("std/time.%s inventory entry = %+v, want implemented native capability", member, entry)
		}
	}
}

// TestNativeLibrarySignalPureParity covers the host-independent signal
// window, filtering, statistics, and normalization contracts. Every result is
// either a scalar returned by value or a newly owned rank-one tensor; the
// fixture deliberately crosses interpreter, VM, generated C, and execution.
func TestNativeLibrarySignalPureParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-native-signal-pure",
		Source: `import "std/signal" as signal
emit signal.rectangular(3)
emit signal.hamming(1)
emit signal.blackman(1)
emit signal.bartlett(1)
emit signal.convolve([1, 2, 3], [1, 1])
emit signal.filter_fir([1, 2, 3], [1, 1])
emit signal.correlate([1, 2, 3], [1, 1])
emit signal.mean([1, 2, 3, 4])
emit signal.variance([1, 2, 3, 4])
emit signal.energy([1, 2, 3, 4])
emit signal.rms([1, 2, 3, 4])
emit signal.normalize([1, 2, 3], "minmax")
emit signal.normalize([1, 2, 3], "zscore")
emit signal.normalize([0, 0], "peak")
emit signal.fft([1, 0, 0, 0])
emit signal.ifft(signal.fft([1, 0, 0, 0]))
emit signal.fft([complex(1, 0), complex(0, 1), complex(0, 0), complex(0, 0)])
emit try { signal.fft([1, 2], "extra") } catch err { "caught-arity" }
emit try { signal.fft("not-a-signal") } catch err { "caught-kind" }
`,
	})
}

func TestNativeLibrarySignalOwnershipCapabilityAndErrorContract(t *testing.T) {
	program, err := Parse("<nativeLibrary-native-signal-contract>", `import "std/signal" as signal
	emit signal.fft([1, 0, 0, 0])
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure std/signal lowering unexpectedly defines a host capability")
	}
	for _, symbol := range []string{"ostd_signal_window", "ostd_signal_conv", "ostd_signal_fir", "ostd_signal_stats", "ostd_signal_normalize", "ostd_signal_data", "ostd_signal_complex_data", "ostd_signal_fft", "ocomplextensordata", "ostd_dispatch"} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated signal artifact does not contain ownership boundary %q", symbol)
		}
	}
	for _, message := range []string{"signal window length is out of range", "signal expects rank-one numeric data", "signal statistics expects non-empty numeric data", "signal.normalize mode must be peak, minmax, or zscore", "fft expects a rank-one numeric signal within the 4096-sample limit"} {
		if !strings.Contains(artifact.Source, message) {
			t.Fatalf("generated signal artifact does not preserve error contract %q", message)
		}
	}
	for _, member := range []string{"rectangular", "hann", "hamming", "blackman", "bartlett", "convolve", "correlate", "filter_fir", "mean", "variance", "energy", "rms", "normalize", "fft", "ifft"} {
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/signal/"+member)
		if !ok || entry.Native != BackendImplemented {
			t.Fatalf("std/signal.%s inventory entry = %+v, want implemented native capability", member, entry)
		}
	}
}
