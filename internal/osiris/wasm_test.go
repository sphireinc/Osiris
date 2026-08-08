package osiris

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestWASMScalarArtifactExecutesThroughProvider(t *testing.T) {
	program, err := Parse("<wasm-scalar>", `
let base = 7
emit base + 3
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileWASM(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.ABI.Version != WASMABIVersion || artifact.ABI.ResultEncoding != "osiris-json-v1" {
		t.Fatalf("ABI = %#v", artifact.ABI)
	}
	if artifact.ABI.ExecutionMode != "scalar-i64-v1" {
		t.Fatalf("execution mode = %q, want scalar-i64-v1", artifact.ABI.ExecutionMode)
	}
	if !bytes.HasPrefix(artifact.Binary, []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}) {
		t.Fatalf("not a WASM v1 module: %x", artifact.Binary[:8])
	}
	if !bytes.Contains(artifact.Binary, []byte{0x7c}) {
		t.Fatalf("generated module has no i64.add opcode: %x", artifact.Binary)
	}
	result, err := ExecuteWASM(context.Background(), artifact, WASMExecutionLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if result.JSON != `[10]` || len(result.Outputs) != 1 || result.Outputs[0].String() != "10u64" {
		t.Fatalf("result = %#v, want [10]", result)
	}
	if result.Instructions == 0 || result.MemoryBytes == 0 {
		t.Fatalf("missing execution accounting: %#v", result)
	}
}

func TestWASMBranchAndRepeatedRunsAreDeterministic(t *testing.T) {
	program, err := Parse("<wasm-branch>", `
let flag = true
emit if(flag, 11, 22)
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileWASM(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	first, err := ExecuteWASM(context.Background(), artifact, WASMExecutionLimits{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ExecuteWASM(context.Background(), artifact, WASMExecutionLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if first.JSON != `[11]` || second.JSON != first.JSON || first.Instructions != second.Instructions {
		t.Fatalf("runs differ: first=%#v second=%#v", first, second)
	}
}

func TestWASMScalarLoopExecutesWithGeneratedBudget(t *testing.T) {
	program, err := Parse("<wasm-loop>", `
var total = 0
while total < 3 { total = total + 1 }
emit total
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileWASM(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.ABI.ExecutionMode != "scalar-i64-v1" {
		t.Fatalf("execution mode = %q", artifact.ABI.ExecutionMode)
	}
	result, err := ExecuteWASM(context.Background(), artifact, WASMExecutionLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if result.JSON != `[3]` || result.Instructions < 10 {
		t.Fatalf("result = %#v, want loop result and dynamic accounting", result)
	}
	if _, err := ExecuteWASM(context.Background(), artifact, WASMExecutionLimits{MaxInstructions: result.Instructions - 1}); err == nil || !strings.Contains(err.Error(), "instructions limit exceeded") {
		t.Fatalf("loop budget error = %v", err)
	}
}

func TestWASMScalarInstructionResults(t *testing.T) {
	fixtures := []struct {
		name   string
		source string
		want   string
	}{
		{name: "unary", source: "emit -7\n", want: `[{"bits":64,"int":"-7"}]`},
		{name: "comparison", source: "emit 9 > 3\n", want: `[true]`},
		{name: "boolean", source: "emit !false\n", want: `[true]`},
		{name: "float", source: "emit 1.5 + 2.25\n", want: `[{"bits":64,"float":"3.75f64"}]`},
		{name: "float32", source: "emit 1.5f32 + 2.25f32\n", want: `[{"bits":32,"float":"3.75f32"}]`},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			program, err := Parse("<wasm-scalar-"+fixture.name+">", fixture.source)
			if err != nil {
				t.Fatal(err)
			}
			artifact, err := CompileWASM(context.Background(), NewRuntime(), program)
			if err != nil {
				t.Fatal(err)
			}
			if artifact.ABI.ExecutionMode != "scalar-i64-v1" {
				t.Fatalf("execution mode = %q", artifact.ABI.ExecutionMode)
			}
			result, err := ExecuteWASM(context.Background(), artifact, WASMExecutionLimits{})
			if err != nil {
				t.Fatal(err)
			}
			if result.JSON != fixture.want {
				t.Fatalf("result = %s, want %s", result.JSON, fixture.want)
			}
			if result.Instructions < 2 {
				t.Fatalf("instruction accounting = %d, want lowered instruction count", result.Instructions)
			}
			if _, err := ExecuteWASM(context.Background(), artifact, WASMExecutionLimits{MaxInstructions: 1}); err == nil || !strings.Contains(err.Error(), "instructions limit exceeded") {
				t.Fatalf("low instruction limit error = %v", err)
			}
		})
	}
}

func TestWASMScalarWidthsAndTrapDiagnostics(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		source string
		want   string
		bits   uint16
	}{
		{name: "u8-wrap", source: "emit 255u8 + 1u8\n", want: "0u8", bits: 8},
		{name: "i8-wrap", source: "emit 127i8 + 1i8\n", want: "-128i8", bits: 8},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			program, err := Parse("<wasm-width-"+fixture.name+">", fixture.source)
			if err != nil {
				t.Fatal(err)
			}
			artifact, err := CompileWASM(context.Background(), NewRuntime(), program)
			if err != nil {
				t.Fatal(err)
			}
			if artifact.ABI.ExecutionMode != "scalar-i64-v1" {
				t.Fatalf("execution mode = %q", artifact.ABI.ExecutionMode)
			}
			result, err := ExecuteWASM(context.Background(), artifact, WASMExecutionLimits{})
			if err != nil {
				t.Fatal(err)
			}
			if result.Outputs[0].String() != fixture.want || (result.Outputs[0].bits != uint8(fixture.bits) && result.Outputs[0].ibits != fixture.bits) {
				t.Fatalf("result = %#v JSON=%s, want %s", result.Outputs[0], result.JSON, fixture.want)
			}
		})
	}

	program, err := Parse("wasm-divzero.osr", "emit 1u64 / 0u64\n")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileWASM(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteWASM(context.Background(), artifact, WASMExecutionLimits{}); err == nil || !strings.Contains(err.Error(), "wasm-divzero.osr") || !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("division trap = %v", err)
	}
}

func TestWASMAggregateMemoryRecordAndCodecRoundTrip(t *testing.T) {
	program, err := Parse("<wasm-aggregate>", "emit [1u8, 2u8, 3u8]\n")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileWASM(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.ABI.ExecutionMode != "aggregate-record-v1" {
		t.Fatalf("execution mode = %q", artifact.ABI.ExecutionMode)
	}
	result, err := ExecuteWASM(context.Background(), artifact, WASMExecutionLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outputs[0].String() != "[1u8, 2u8, 3u8]" || result.MemoryBytes <= 8 {
		t.Fatalf("aggregate result = %#v, want module-owned codec record", result)
	}
}

func TestWASMStaticFunctionMemoryRecord(t *testing.T) {
	program, err := Parse("<wasm-function>", "func add(value u64) u64 => value + 1u64\nemit add(2u64)\n")
	if err != nil {
		t.Fatal(err)
	}
	ir, err := NewRuntime().BuildIR(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if body, data, ok := emitWASMAggregateBody(ir); !ok {
		t.Fatalf("aggregate body rejected body=%x data=%d", body, len(data))
	}
	artifact, err := CompileWASM(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.ABI.ExecutionMode != "aggregate-record-v1" {
		t.Fatalf("execution mode = %q", artifact.ABI.ExecutionMode)
	}
	result, err := ExecuteWASM(context.Background(), artifact, WASMExecutionLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Outputs) != 1 || result.Outputs[0].String() != "3u64" || result.MemoryBytes <= 8 {
		t.Fatalf("function result = %#v", result)
	}
}

func TestWASMRejectsUnsupportedHostBuiltinBeforeEmission(t *testing.T) {
	program, err := Parse("<wasm-unsupported>", "emit read(\"input.txt\")\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileWASM(context.Background(), NewRuntime(), program); err == nil || !strings.Contains(err.Error(), "deterministic WASM profile") {
		t.Fatalf("error = %v, want deterministic profile rejection", err)
	}
}

func TestWASMRunnerRejectsMalformedArtifactAndEnforcesLimits(t *testing.T) {
	program, err := Parse("<wasm-limits>", "emit 1\nemit 2\n")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileWASM(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	malformed := append([]byte(nil), artifact.Binary...)
	malformed[0] = 0x7f
	if _, err := NewDeterministicWASMProvider().Execute(context.Background(), malformed, WASMExecutionLimits{}); err == nil || !strings.Contains(err.Error(), "malformed WASM header") {
		t.Fatalf("malformed error = %v", err)
	}
	if _, err := ExecuteWASM(context.Background(), artifact, WASMExecutionLimits{MaxOutputs: 1}); err == nil || !strings.Contains(err.Error(), "outputs limit exceeded") {
		t.Fatalf("output limit error = %v", err)
	}
	if _, err := ExecuteWASM(context.Background(), artifact, WASMExecutionLimits{MaxInstructions: 1}); err == nil || !strings.Contains(err.Error(), "instructions limit exceeded") {
		t.Fatalf("instruction limit error = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ExecuteWASM(canceled, artifact, WASMExecutionLimits{}); err != context.Canceled {
		t.Fatalf("cancellation error = %v, want context canceled", err)
	}
}

func TestWASMLanguageAndNumericParityCorpus(t *testing.T) {
	fixtures := []struct {
		name   string
		source string
	}{
		{
			name: "numeric-families-and-aggregates",
			source: `emit u8(255) + u8(1)
emit i8(-128)
emit 1.5f32 + 0.25f32
emit [1, 2, 3]
emit range(3)
emit tensor([1, 2]) + tensor([3, 4])
`,
		},
		{
			name: "mutation-functions-and-closures",
			source: `let base = 2
func add(value u64) u64 => value + base
var total = 0
while total < 3 { total = total + 1 }
emit add(total)
`,
		},
		{
			name: "results-options-and-patterns",
			source: `enum Result { Ok(value), Err(message) }
let result = Result.Ok([2, 3])
emit match result { Result.Ok([left, right]) => left + right, Result.Err(message) => 0 }
emit ok(41)
let? value = ok(42)
emit value
`,
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			program, err := Parse("wasm-"+fixture.name+".osr", fixture.source)
			if err != nil {
				t.Fatal(err)
			}
			rt := NewRuntime()
			interpreter, interpreterErr := rt.Run(context.Background(), program)
			vm, vmErr := rt.RunVM(context.Background(), program)
			artifact, err := CompileWASM(context.Background(), NewRuntime(), program)
			if err != nil {
				t.Fatalf("compile WASM: %v", err)
			}
			wasm, wasmErr := ExecuteWASM(context.Background(), artifact, WASMExecutionLimits{})
			want := differentialResult{Backend: "interpreter", Phase: "run", Output: valuesOutput(interpreter), Err: interpreterErr}
			assertDifferentialEqual(t, fixtureForWASM(fixture.name, fixture.source), want, differentialResult{Backend: "wasm", Phase: "execute", Output: valuesOutput(wasm.Outputs), Err: wasmErr})
			assertDifferentialEqual(t, fixtureForWASM(fixture.name, fixture.source), want, differentialResult{Backend: "vm", Phase: "run", Output: valuesOutput(vm), Err: vmErr})
		})
	}
}

func fixtureForWASM(name, source string) differentialFixture {
	return differentialFixture{Name: name, Source: source}
}

func TestWASMSourceSpanForRejectedCapability(t *testing.T) {
	program, err := Parse("wasm-capability.osr", "emit read(\"input.txt\")\n")
	if err != nil {
		t.Fatal(err)
	}
	_, err = CompileWASM(context.Background(), NewRuntime(), program)
	if err == nil || !strings.Contains(err.Error(), "wasm-capability.osr:1:") || !strings.Contains(err.Error(), "deterministic WASM profile") {
		t.Fatalf("error = %v, want source-spanned capability rejection", err)
	}
}

func TestWASMDeterministicStdlibAndCapabilityProfile(t *testing.T) {
	program, err := Parse("<wasm-stdlib-profile>", `import "std/text" as text
import "std/math" as math
import "std/crypto" as crypto
import "std/list" as list
import "std/signal" as signal
import "std/time" as time
import "std/regex" as regex
import "std/json" as json
import "std/i18n" as i18n
import "std/db" as db
import "std/os" as os
import "std/testing" as testing

emit text.upper("wasm")
emit math.sqrt(81)
emit crypto.digest("hello", crypto.sha256)
emit list.sum([1, 2, 3])
emit signal.mean([1, 2, 3])
emit time.format(time.parse(time.rfc3339, "2020-01-02T03:04:05Z"), time.rfc3339)
emit regex.is_match("a+", "aaa")
emit json.type_of([true])
emit i18n.format_currency("en-US", "12.50", "USD")
let connection = db.open()
emit db.exec(connection, "CREATE TABLE items (id INTEGER)", [])
emit os.write_file("fixture.txt", [65u8, 66u8], 0o600)
emit os.read_file("fixture.txt")
emit testing.assert(1 == 1)
emit testing.failed()
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileWASM(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.ABI.Profile != WASMProfileName || len(artifact.ABI.Modules) != len(DefaultWASMCapabilityProfile.Modules) {
		t.Fatalf("profile ABI = %#v", artifact.ABI)
	}
	result, err := ExecuteWASM(context.Background(), artifact, WASMExecutionLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Outputs) != 14 {
		t.Fatalf("outputs = %#v", result.Outputs)
	}
	want := []string{"WASM", "9", "2cf24dba5fb0a30e26e83b", "6", "2", "2020-01-02T03:04:05Z", "true", "array", "$12.50", "map{", "true", "[65u8, 66u8]", "true", "false"}
	for i, prefix := range want {
		got := strings.Trim(result.Outputs[i].String(), "\"")
		if !strings.HasPrefix(got, prefix) {
			t.Fatalf("output %d = %q, want prefix %q", i, got, prefix)
		}
	}
}

func TestWASMProfileRejectsUnlistedCapabilityAndAllowsExplicitUnsafe(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
		want   string
	}{
		{name: "network", source: "import \"std/net\" as net\nemit net.dns_lookup(\"example.test\")\n", want: "module is outside the deterministic WASM profile"},
		{name: "ambient identity", source: "import \"std/os\" as os\nemit os.hostname()\n", want: "outside the deterministic WASM profile"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			program, err := Parse("<wasm-profile-reject>", tc.source)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := CompileWASM(context.Background(), NewRuntime(), program); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("compile error = %v, want %q", err, tc.want)
			}
		})
	}

	profile := DefaultWASMCapabilityProfile
	profile.Modules = append(append([]string(nil), profile.Modules...), "std/unsafe")
	profile.Capabilities = append(append([]string(nil), profile.Capabilities...), UnsafeMemoryCapability)
	program, err := Parse("<wasm-explicit-unsafe>", "import \"std/unsafe\" as unsafe\nemit unsafe.capability\n")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	rt.GrantCapability(UnsafeMemoryCapability)
	artifact, err := CompileWASMWithProfile(context.Background(), rt, program, profile, WASMExecutionLimits{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := ExecuteWASM(context.Background(), artifact, WASMExecutionLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Outputs) != 1 || result.Outputs[0].str != UnsafeMemoryCapability {
		t.Fatalf("unsafe output = %#v", result.Outputs)
	}
}

func TestWASMHostImportManifestIsExplicitAndValidated(t *testing.T) {
	program, err := Parse("<wasm-host-imports>", `
import "std/list" as list
import "std/os" as os
emit list.sum([1u64, 2u64])
emit os.getenv("OSIRIS_TEST_MISSING")
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileWASM(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	want := []WASMHostImport{
		{Module: "std/list", Member: "sum", Encoding: WASMHostImportEncoding},
		{Module: "std/os", Member: "getenv", Capability: "os.environment", Encoding: WASMHostImportEncoding},
	}
	if !reflect.DeepEqual(artifact.ABI.Imports, want) {
		t.Fatalf("host imports = %#v, want %#v", artifact.ABI.Imports, want)
	}
	payload, err := parseWASMArtifact(artifact.Binary, WASMExecutionLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateWASMProgramProfile(payload); err != nil {
		t.Fatalf("generated host-import manifest rejected: %v", err)
	}

	tampered := *payload
	tampered.ABI.Imports = append([]WASMHostImport(nil), payload.ABI.Imports...)
	tampered.ABI.Imports[1].Capability = "os.file_read"
	if err := validateWASMProgramProfile(&tampered); err == nil || !strings.Contains(err.Error(), "declares capability") {
		t.Fatalf("tampered capability error = %v, want capability mismatch", err)
	}

	tampered = *payload
	tampered.ABI.Imports = append([]WASMHostImport(nil), payload.ABI.Imports...)
	tampered.ABI.Imports = append(tampered.ABI.Imports, tampered.ABI.Imports[1])
	if err := validateWASMProgramProfile(&tampered); err == nil || !strings.Contains(err.Error(), "duplicate host-import") {
		t.Fatalf("duplicate import error = %v, want duplicate rejection", err)
	}

	tampered = *payload
	tampered.ABI.Imports = append([]WASMHostImport(nil), payload.ABI.Imports[:1]...)
	if err := validateWASMProgramProfile(&tampered); err == nil || !strings.Contains(err.Error(), "manifest count") {
		t.Fatalf("missing import error = %v, want manifest/reference mismatch", err)
	}
}

func TestWASMHostImportValidationIsConcurrentSafe(t *testing.T) {
	program, err := Parse("<wasm-host-imports-race>", `
import "std/os" as os
emit os.getenv("OSIRIS_TEST_MISSING")
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileWASM(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	const workers = 8
	errs := make(chan error, workers)
	var group sync.WaitGroup
	group.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer group.Done()
			_, err := ExecuteWASM(context.Background(), artifact, WASMExecutionLimits{})
			errs <- err
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent WASM execution failed: %v", err)
		}
	}
}
