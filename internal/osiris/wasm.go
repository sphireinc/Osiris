package osiris

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// WASMABIVersion is the version of the host/module contract. The version is
// carried in both the exported metadata and the artifact custom section.
const WASMABIVersion uint32 = 1

// WASMProfileName identifies the host-import policy carried by an artifact.
// The profile is intentionally narrow: it names deterministic providers and
// does not imply access to the browser, Node, process, or ambient filesystem.
const WASMProfileName = "deterministic-v1"

// WASMHostImportEncoding identifies the v1 logical host-import record. The
// record is carried in the Osiris ABI custom section; it describes the
// module/member and capability that a generated instruction may resolve. It
// is deliberately distinct from a raw wazero import signature: v1 still
// routes the call through the bounded Osiris value ABI.
const WASMHostImportEncoding = "osiris-host-import-v1"

const (
	wasmPageSize      = 64 * 1024
	wasmCustomSection = "osiris.wasm.v1"
	wasmPayloadMagic  = "OSWB"
)

// WASMABI describes the stable boundary between a generated module and its
// deterministic host. Values are returned as owned Osiris values; the module
// never retains a host pointer. Memory is linear, page-sized, and bounded by
// the execution limits supplied to the runner.
type WASMABI struct {
	Version        uint32           `json:"version"`
	Profile        string           `json:"profile"`
	ExecutionMode  string           `json:"execution_mode"`
	MemoryPageSize uint32           `json:"memory_page_size"`
	ResultEncoding string           `json:"result_encoding"`
	Cancellation   string           `json:"cancellation"`
	Ownership      string           `json:"ownership"`
	Deterministic  bool             `json:"deterministic"`
	Modules        []string         `json:"modules,omitempty"`
	Capabilities   []string         `json:"capabilities,omitempty"`
	Imports        []WASMHostImport `json:"imports,omitempty"`
}

// WASMHostImport is one explicit logical host import referenced by lowered
// IR. Module and Member identify the standard-library operation. Capability
// is the exact profile grant required for that operation, or empty for a
// pure deterministic operation. Encoding is fixed to
// WASMHostImportEncoding in ABI v1 so a host cannot reinterpret the record as
// an ambient pointer or an unbounded native call.
type WASMHostImport struct {
	Module     string `json:"module"`
	Member     string `json:"member"`
	Capability string `json:"capability,omitempty"`
	Encoding   string `json:"encoding"`
}

// WASMCapabilityProfile is the explicit import policy used while lowering a
// module. Modules and capabilities are copied into the artifact ABI in sorted
// order, so hosts can inspect the contract without reading source code.
type WASMCapabilityProfile struct {
	Name         string
	Modules      []string
	Capabilities []string
}

// DefaultWASMCapabilityProfile is the bounded deterministic profile. The
// listed std/os operations use NewTestRuntime's in-memory providers; no host
// filesystem, process table, clock, or network is consulted.
var DefaultWASMCapabilityProfile = WASMCapabilityProfile{
	Name: WASMProfileName,
	Modules: []string{
		"std/crypto", "std/db", "std/i18n", "std/json", "std/list", "std/math",
		"std/os", "std/regex", "std/signal", "std/telemetry", "std/testing", "std/text", "std/time",
	},
	Capabilities: []string{
		"clock.virtual", "db.embedded", "i18n.data", "os.directory_mutate", "os.directory_read",
		"os.environment", "os.environment_mutate", "os.file_metadata", "os.file_mutate", "os.file_read",
		"os.file_write", "regex.engine", "telemetry.memory", "testing.assertions",
	},
}

// WASMExecutionLimits are deterministic resource limits. Zero fields select
// DefaultWASMExecutionLimits. No limit is enforced by a background goroutine;
// cancellation is checked at each lowered instruction and block transition.
type WASMExecutionLimits struct {
	MaxInstructions  uint64
	MaxMemoryBytes   uint64
	MaxOutputs       uint32
	MaxArtifactBytes uint64
}

// DefaultWASMExecutionLimits is intentionally small enough for embedded use.
var DefaultWASMExecutionLimits = WASMExecutionLimits{
	MaxInstructions:  100_000,
	MaxMemoryBytes:   1 << 20,
	MaxOutputs:       1_024,
	MaxArtifactBytes: 1 << 20,
}

// WASMArtifact is an immutable generated module. Binary is the authoritative
// artifact; WAT is a deterministic inspection rendering. JSON is retained as
// a compatibility preview only for programs whose emitted values are fully
// compile-time scalar literals. Runtime results must be obtained through
// ExecuteWASM and are never manufactured by CompileWASM.
type WASMArtifact struct {
	ABI    WASMABI
	WAT    string
	Binary []byte
	JSON   string
}

// WASMExecution contains owned results from one module invocation.
type WASMExecution struct {
	Outputs      []Value
	JSON         string
	Instructions uint64
	MemoryBytes  uint64
}

// CompileWASM lowers checked Osiris IR into a bounded WASM artifact. It does
// not create a Session or evaluate source code. Unsupported IR is rejected
// before the module is emitted.
func CompileWASM(ctx context.Context, rt *Runtime, prog *Program) (*WASMArtifact, error) {
	return CompileWASMWithProfile(ctx, rt, prog, DefaultWASMCapabilityProfile, DefaultWASMExecutionLimits)
}

// CompileWASMWithLimits compiles with explicit artifact resource ceilings.
// The limits are part of the generated ABI payload and therefore part of the
// module's deterministic execution contract.
func CompileWASMWithLimits(ctx context.Context, rt *Runtime, prog *Program, limits WASMExecutionLimits) (*WASMArtifact, error) {
	return CompileWASMWithProfile(ctx, rt, prog, DefaultWASMCapabilityProfile, limits)
}

// CompileWASMWithProfile lowers using an explicit deterministic host-import
// profile. A profile cannot add an operation to the WASM runner; it can only
// select a declared subset of the adapters implemented by this package.
func CompileWASMWithProfile(ctx context.Context, rt *Runtime, prog *Program, profile WASMCapabilityProfile, limits WASMExecutionLimits) (*WASMArtifact, error) {
	if err := rejectReservedTypeFutureBackend(prog, "WASM"); err != nil {
		return nil, err
	}
	profile = normalizeWASMProfile(profile)
	limits = normalizeWASMLimits(limits)
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if rt == nil {
		rt = NewRuntime()
	}
	ir, err := rt.BuildIR(ctx, prog)
	if err != nil {
		return nil, err
	}
	if _, err := VerifySSAProgram(ir); err != nil {
		return nil, fmt.Errorf("WASM backend: invalid SSA: %w", err)
	}
	compiled, err := lowerWASMProgram(ir, limits, profile)
	if err != nil {
		return nil, err
	}
	body, executionMode, resultData := emitWASMInstructionBody(ir)
	compiled.ABI.ExecutionMode = executionMode
	payload, err := marshalWASMPayload(compiled)
	if err != nil {
		return nil, fmt.Errorf("WASM backend: encode ABI payload: %w", err)
	}
	if uint64(len(payload)) > limits.MaxArtifactBytes {
		return nil, wasmLimitError("artifact", limits.MaxArtifactBytes)
	}
	if uint64(len(resultData)) > limits.MaxMemoryBytes {
		return nil, wasmLimitError("memory", limits.MaxMemoryBytes)
	}
	binary := buildWASMModule(payload, body, resultData, limits)
	if uint64(len(binary)) > limits.MaxArtifactBytes {
		return nil, wasmLimitError("artifact", limits.MaxArtifactBytes)
	}
	if err := validateWASMArtifact(binary, limits); err != nil {
		return nil, fmt.Errorf("WASM backend: generated artifact validation: %w", err)
	}
	preview := wasmLiteralPreview(compiled)
	return &WASMArtifact{
		ABI: WASMABI{
			Version:        WASMABIVersion,
			Profile:        profile.Name,
			ExecutionMode:  compiled.ABI.ExecutionMode,
			MemoryPageSize: wasmPageSize,
			ResultEncoding: "osiris-json-v1",
			Cancellation:   "context-check-per-instruction",
			Ownership:      "returned-values-owned; linear-memory-borrowed-during-call",
			Deterministic:  true,
			Modules:        append([]string(nil), compiled.ABI.Modules...),
			Capabilities:   append([]string(nil), compiled.ABI.Capabilities...),
			Imports:        append([]WASMHostImport(nil), compiled.ABI.Imports...),
		},
		WAT:    buildWASMText(ir.SourceName, body, preview, limits),
		Binary: binary,
		JSON:   preview,
	}, nil
}

// ExecuteWASM validates and runs the generated artifact through the bounded
// deterministic runner. It consumes the artifact binary, not the source
// program, and has no interpreter fallback.
func ExecuteWASM(ctx context.Context, artifact *WASMArtifact, limits WASMExecutionLimits) (WASMExecution, error) {
	if artifact == nil {
		return WASMExecution{}, fmt.Errorf("WASM backend: nil artifact")
	}
	return NewDeterministicWASMProvider().Execute(ctx, artifact.Binary, limits)
}

func normalizeWASMLimits(limits WASMExecutionLimits) WASMExecutionLimits {
	defaults := DefaultWASMExecutionLimits
	if limits.MaxInstructions == 0 {
		limits.MaxInstructions = defaults.MaxInstructions
	}
	if limits.MaxMemoryBytes == 0 {
		limits.MaxMemoryBytes = defaults.MaxMemoryBytes
	}
	if limits.MaxOutputs == 0 {
		limits.MaxOutputs = defaults.MaxOutputs
	}
	if limits.MaxArtifactBytes == 0 {
		limits.MaxArtifactBytes = defaults.MaxArtifactBytes
	}
	return limits
}

func wasmLimitError(resource string, limit uint64) error {
	return fmt.Errorf("WASM backend: %s limit exceeded (limit=%d)", resource, limit)
}

func buildWASMText(sourceName string, body []byte, preview string, limits WASMExecutionLimits) string {
	var b strings.Builder
	b.WriteString("(module\n")
	b.WriteString("  ;; Osiris WASM ABI v1; source ")
	b.WriteString(strconv.Quote(sourceName))
	b.WriteString("\n")
	b.WriteString("  ;; result encoding: osiris-json-v1; memory record: kind:u8 bits:u8 payload:u64 at result_ptr\n")
	b.WriteString("  (memory (export \"memory\") 1 ")
	b.WriteString(strconv.FormatUint((limits.MaxMemoryBytes+wasmPageSize-1)/wasmPageSize, 10))
	b.WriteString(")\n")
	b.WriteString("  (global (export \"abi_version\") i32 (i32.const 1))\n")
	b.WriteString("  (global (export \"result_ptr\") i32 (i32.const 0))\n")
	b.WriteString("  (global (export \"result_len\") (mut i32) (i32.const 16))\n")
	b.WriteString("  (global (export \"result_kind\") (mut i32) (i32.const 0))\n")
	b.WriteString("  (global (export \"result_value\") (mut i64) (i64.const 0))\n")
	b.WriteString("  (global (export \"result_f64\") (mut f64) (f64.const 0))\n")
	b.WriteString("  (global (export \"result_bits\") (mut i32) (i32.const 64))\n")
	b.WriteString("  (global (export \"instruction_count\") (mut i64) (i64.const 0))\n")
	b.WriteString("  (global (export \"instruction_budget\") (mut i64) (i64.const ")
	b.WriteString(strconv.FormatUint(limits.MaxInstructions, 10))
	b.WriteString("))\n")
	b.WriteString("  (func (export \"run\") (result i32)\n")
	b.WriteString("    ;; lowered scalar/control-flow body; status 0=ok, 1=instruction limit\n")
	for _, op := range body {
		b.WriteString("    ;; opcode ")
		b.WriteString(fmt.Sprintf("0x%02x", op))
		b.WriteByte('\n')
	}
	b.WriteString("    i32.const 0\n")
	b.WriteString("  )\n")
	if preview != "" {
		b.WriteString("  ;; literal compatibility preview ")
		b.WriteString(strconv.Quote(preview))
		b.WriteByte('\n')
	}
	b.WriteString(")\n")
	return b.String()
}

func buildWASMModule(payload, body, resultData []byte, limits WASMExecutionLimits) []byte {
	var out bytes.Buffer
	out.Write([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00})
	writeWASMSection(&out, 1, func(buf *bytes.Buffer) {
		writeULEB(buf, 1)
		buf.WriteByte(0x60)
		writeULEB(buf, 0)
		writeULEB(buf, 1)
		buf.WriteByte(0x7f)
	})
	writeWASMSection(&out, 3, func(buf *bytes.Buffer) {
		writeULEB(buf, 1)
		writeULEB(buf, 0)
	})
	writeWASMSection(&out, 5, func(buf *bytes.Buffer) {
		writeULEB(buf, 1)
		buf.WriteByte(0x01)
		writeULEB(buf, 1)
		writeULEB(buf, (limits.MaxMemoryBytes+wasmPageSize-1)/wasmPageSize)
	})
	writeWASMSection(&out, 6, func(buf *bytes.Buffer) {
		writeULEB(buf, 9)
		writeWASMGlobal(buf, 1, false)
		writeWASMGlobal(buf, 0, false)
		writeWASMGlobal(buf, 16, true)
		writeWASMGlobal(buf, 0, true)
		writeWASMI64Global(buf, 0, true)
		writeWASMF64Global(buf, 0, true)
		writeWASMGlobal(buf, 64, true)
		writeWASMI64Global(buf, 0, true)
		writeWASMI64Global(buf, int64(limits.MaxInstructions), true)
	})
	writeWASMSection(&out, 7, func(buf *bytes.Buffer) {
		writeULEB(buf, 11)
		writeWASMExport(buf, "memory", 0x02, 0)
		writeWASMExport(buf, "abi_version", 0x03, 0)
		writeWASMExport(buf, "result_ptr", 0x03, 1)
		writeWASMExport(buf, "result_len", 0x03, 2)
		writeWASMExport(buf, "result_kind", 0x03, 3)
		writeWASMExport(buf, "result_value", 0x03, 4)
		writeWASMExport(buf, "result_f64", 0x03, 5)
		writeWASMExport(buf, "result_bits", 0x03, 6)
		writeWASMExport(buf, "instruction_count", 0x03, 7)
		writeWASMExport(buf, "instruction_budget", 0x03, 8)
		writeWASMExport(buf, "run", 0x00, 0)
	})
	writeWASMSection(&out, 10, func(buf *bytes.Buffer) {
		writeULEB(buf, 1)
		var fn bytes.Buffer
		fn.Write(body)
		writeULEB(buf, uint64(fn.Len()))
		buf.Write(fn.Bytes())
	})
	if len(resultData) > 0 {
		writeWASMSection(&out, 11, func(buf *bytes.Buffer) {
			writeULEB(buf, 1)
			buf.WriteByte(0)
			buf.WriteByte(0x41)
			writeSLEB(buf, 0)
			buf.WriteByte(0x0b)
			writeULEB(buf, uint64(len(resultData)))
			buf.Write(resultData)
		})
	}
	writeWASMSection(&out, 0, func(buf *bytes.Buffer) {
		writeULEB(buf, uint64(len(wasmCustomSection)))
		buf.WriteString(wasmCustomSection)
		buf.Write(payload)
	})
	return out.Bytes()
}

func emitWASMInstructionBody(ir *IRProgram) ([]byte, string, []byte) {
	if body, data, ok := emitWASMAggregateBody(ir); ok {
		return body, "aggregate-record-v1", data
	}
	if body, ok := emitWASMScalarBody(ir); ok {
		return body, "scalar-i64-v1", nil
	}
	var body bytes.Buffer
	writeULEB(&body, 0)
	body.Write(emitWASMInspectionBody(ir))
	body.WriteByte(0x41)
	writeSLEB(&body, 0)
	body.WriteByte(0x0b)
	return body.Bytes(), "payload-v1", nil
}

// emitWASMAggregateBody covers deterministic aggregate construction whose
// inputs are already known at compile time. The values are encoded with the
// same bounded codec used by the public runtime, placed in the module's data
// segment, and returned through the module-owned result memory record. This is
// deliberately separate from scalar lowering and never evaluates a source
// program during ExecuteWASM.
func emitWASMAggregateBody(ir *IRProgram) ([]byte, []byte, bool) {
	if ir == nil || len(ir.Functions) == 0 || ir.Functions[0] == nil || len(ir.Functions[0].Blocks) != 1 {
		return nil, nil, false
	}
	block := ir.Functions[0].Blocks[0]
	values := make(map[string]Value)
	functionRefs := make(map[string]string)
	hasStaticCall := false
	var outputs []Value
	for _, in := range block.Instrs {
		switch in.Op {
		case "const", "global":
			values[in.Dest] = in.Value
		case "copy", "assign":
			if len(in.Args) != 1 {
				return nil, nil, false
			}
			value, ok := values[in.Args[0]]
			if !ok {
				return nil, nil, false
			}
			values[in.Dest] = cloneWASMValue(value)
		case "asserttype":
			if len(in.Args) != 1 {
				return nil, nil, false
			}
			value, ok := values[in.Args[0]]
			if !ok {
				return nil, nil, false
			}
			checked, err := checkBindingTypeID(ir.Types, in.TypeID, value, false)
			if err != nil {
				return nil, nil, false
			}
			values[in.Dest] = cloneWASMValue(checked)
		case "makefunc":
			if in.Dest == "" || in.Text == "" {
				return nil, nil, false
			}
			functionRefs[in.Dest] = in.Text
		case "unary":
			if len(in.Args) != 1 {
				return nil, nil, false
			}
			value, ok := values[in.Args[0]]
			if !ok {
				return nil, nil, false
			}
			if in.Text == "+" {
				values[in.Dest] = cloneWASMValue(value)
				continue
			}
			result, err := applyUnary(wasmToken(in.Text), value)
			if err != nil {
				return nil, nil, false
			}
			values[in.Dest] = result
		case "binary":
			if len(in.Args) != 2 {
				return nil, nil, false
			}
			left, leftOK := values[in.Args[0]]
			right, rightOK := values[in.Args[1]]
			if !leftOK || !rightOK {
				return nil, nil, false
			}
			result, err := vmBinary(context.Background(), wasmToken(in.Text), left, right)
			if err != nil {
				return nil, nil, false
			}
			values[in.Dest] = result
		case "list":
			items, ok := wasmStaticArgs(values, in.Args)
			if !ok {
				return nil, nil, false
			}
			values[in.Dest] = listValue(items)
		case "tuple":
			items, ok := wasmStaticArgs(values, in.Args)
			if !ok {
				return nil, nil, false
			}
			values[in.Dest] = tupleValue(items)
		case "enum":
			items, ok := wasmStaticArgs(values, in.Args)
			if !ok {
				return nil, nil, false
			}
			parts := strings.SplitN(in.Text, ".", 2)
			if len(parts) != 2 {
				return nil, nil, false
			}
			values[in.Dest] = enumCaseValue(parts[0], parts[1], items...)
		case "call":
			functionName := functionRefs[in.Text]
			fn := findWASMIRFunction(ir.Functions, functionName)
			args, ok := wasmStaticArgs(values, in.Args)
			if !ok || fn == nil {
				return nil, nil, false
			}
			value, ok := evaluateWASMStaticFunction(fn, args)
			if !ok {
				return nil, nil, false
			}
			values[in.Dest] = value
			hasStaticCall = true
		case "emit":
			if len(in.Args) != 1 {
				return nil, nil, false
			}
			value, ok := values[in.Args[0]]
			if !ok {
				return nil, nil, false
			}
			outputs = append(outputs, cloneWASMValue(value))
		default:
			return nil, nil, false
		}
	}
	if len(outputs) == 0 || block.Term == nil {
		return nil, nil, false
	}
	if len(outputs) == 1 && wasmScalarKindForValue(outputs[0]) != 0 && !hasStaticCall {
		return nil, nil, false
	}
	if _, ok := block.Term.(*irReturn); !ok {
		return nil, nil, false
	}
	data, err := marshalWASMResultRecords(outputs)
	if err != nil {
		return nil, nil, false
	}
	var body bytes.Buffer
	writeULEB(&body, 0)
	for range outputs {
		writeWASMStep(&body)
	}
	body.WriteByte(0x41)
	writeSLEB(&body, int64(len(data)))
	body.WriteByte(0x24)
	writeULEB(&body, 2)
	body.WriteByte(0x41)
	writeSLEB(&body, 7)
	body.WriteByte(0x24)
	writeULEB(&body, 3)
	body.WriteByte(0x41)
	writeSLEB(&body, 0)
	body.WriteByte(0x24)
	writeULEB(&body, 6)
	body.WriteByte(0x41)
	writeSLEB(&body, 0)
	body.WriteByte(0x0b)
	return body.Bytes(), data, true
}

func findWASMIRFunction(functions []*IRFunction, name string) *IRFunction {
	for _, fn := range functions {
		if fn != nil && fn.Name == name {
			return fn
		}
	}
	return nil
}

func evaluateWASMStaticFunction(fn *IRFunction, args []Value) (Value, bool) {
	if fn == nil || len(fn.Blocks) != 1 || len(fn.Params) != len(args) || len(fn.Captures) != 0 {
		return nullValue(), false
	}
	values := make(map[string]Value, len(args))
	for i, param := range fn.Params {
		values[param] = cloneWASMValue(args[i])
	}
	// Parameter SSA names are qualified by the function builder (for example
	// add.fn0.value.0), while the public function signature keeps `value`.
	// Bind both forms so static direct-call lowering follows the same ABI name
	// resolution as the ordinary IR evaluator.
	for _, in := range fn.Blocks[0].Instrs {
		for _, arg := range in.Args {
			for i, param := range fn.Params {
				if strings.HasSuffix(arg, "."+param+".0") {
					values[arg] = cloneWASMValue(args[i])
				}
			}
		}
	}
	for _, in := range fn.Blocks[0].Instrs {
		switch in.Op {
		case "const", "global":
			values[in.Dest] = in.Value
		case "copy", "assign":
			if len(in.Args) != 1 {
				return nullValue(), false
			}
			value, ok := values[in.Args[0]]
			if !ok {
				return nullValue(), false
			}
			values[in.Dest] = cloneWASMValue(value)
		case "asserttype":
			if len(in.Args) != 1 {
				return nullValue(), false
			}
			value, ok := values[in.Args[0]]
			if !ok {
				return nullValue(), false
			}
			checked, err := checkBindingTypeID(fn.Types, in.TypeID, value, false)
			if err != nil {
				return nullValue(), false
			}
			values[in.Dest] = cloneWASMValue(checked)
		case "unary":
			if len(in.Args) != 1 {
				return nullValue(), false
			}
			value, ok := values[in.Args[0]]
			if !ok {
				return nullValue(), false
			}
			if in.Text == "+" {
				values[in.Dest] = cloneWASMValue(value)
				continue
			}
			result, err := applyUnary(wasmToken(in.Text), value)
			if err != nil {
				return nullValue(), false
			}
			values[in.Dest] = result
		case "binary":
			if len(in.Args) != 2 {
				return nullValue(), false
			}
			left, leftOK := values[in.Args[0]]
			right, rightOK := values[in.Args[1]]
			if !leftOK || !rightOK {
				return nullValue(), false
			}
			result, err := vmBinary(context.Background(), wasmToken(in.Text), left, right)
			if err != nil {
				return nullValue(), false
			}
			values[in.Dest] = result
		default:
			return nullValue(), false
		}
	}
	term, ok := fn.Blocks[0].Term.(*irReturn)
	if !ok || !term.HasValue {
		return nullValue(), false
	}
	value, ok := values[term.Value]
	return cloneWASMValue(value), ok
}

func wasmStaticArgs(values map[string]Value, args []string) ([]Value, bool) {
	items := make([]Value, len(args))
	for i, arg := range args {
		value, ok := values[arg]
		if !ok {
			return nil, false
		}
		items[i] = cloneWASMValue(value)
	}
	return items, true
}

func marshalWASMResultRecords(outputs []Value) ([]byte, error) {
	if uint64(len(outputs)) > uint64(^uint32(0)) {
		return nil, fmt.Errorf("too many WASM outputs")
	}
	data := make([]byte, 8)
	copy(data[:4], []byte("OSMR"))
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(outputs)))
	for _, value := range outputs {
		encoded, err := encodeValueBytes(value)
		if err != nil {
			return nil, err
		}
		if uint64(len(encoded)) > uint64(^uint32(0)) {
			return nil, fmt.Errorf("WASM result record is too large")
		}
		var length [4]byte
		binary.LittleEndian.PutUint32(length[:], uint32(len(encoded)))
		data = append(data, length[:]...)
		data = append(data, encoded...)
	}
	return data, nil
}

type wasmScalarKind byte

const (
	wasmScalarUint    wasmScalarKind = 1
	wasmScalarInt     wasmScalarKind = 2
	wasmScalarBool    wasmScalarKind = 3
	wasmScalarFloat   wasmScalarKind = 4
	wasmScalarFloat32 wasmScalarKind = 5
	wasmScalarNull    wasmScalarKind = 6
)

func emitWASMScalarBody(ir *IRProgram) ([]byte, bool) {
	if ir == nil || len(ir.Functions) != 1 || ir.Functions[0] == nil || len(ir.Functions[0].Blocks) == 0 {
		return nil, false
	}
	if len(ir.Functions[0].Blocks) > 1 {
		return emitWASMScalarCFGBody(ir)
	}
	block := ir.Functions[0].Blocks[0]
	locals := make(map[string]uint32)
	kinds := make(map[string]wasmScalarKind)
	widths := make(map[string]uint16)
	var emitted string
	var emittedKind wasmScalarKind
	var emittedBits uint16
	for _, in := range block.Instrs {
		if in.Op == "emit" {
			if emitted != "" || len(in.Args) != 1 {
				return nil, false
			}
			emitted = in.Args[0]
			emittedKind = kinds[emitted]
			emittedBits = widths[emitted]
			continue
		}
		if in.Dest == "" {
			return nil, false
		}
		if _, ok := kinds[in.Dest]; !ok {
			kinds[in.Dest] = wasmScalarKind(0)
			locals[in.Dest] = uint32(len(locals))
		}
		switch in.Op {
		case "const", "global":
			kinds[in.Dest] = wasmScalarKindForValue(in.Value)
			widths[in.Dest] = wasmScalarWidthForValue(in.Value)
		case "copy":
			if len(in.Args) != 1 {
				return nil, false
			}
			kinds[in.Dest] = kinds[in.Args[0]]
			widths[in.Dest] = widths[in.Args[0]]
		case "unary":
			if len(in.Args) != 1 || !wasmScalarUnary(in.Text) {
				return nil, false
			}
			if in.Text == "!" {
				kinds[in.Dest] = wasmScalarBool
				widths[in.Dest] = 1
			} else if in.Text == "-" && wasmScalarIsFloat(kinds[in.Args[0]]) {
				kinds[in.Dest] = kinds[in.Args[0]]
				widths[in.Dest] = widths[in.Args[0]]
			} else if in.Text == "-" {
				kinds[in.Dest] = wasmScalarInt
				widths[in.Dest] = widths[in.Args[0]]
			} else {
				kinds[in.Dest] = kinds[in.Args[0]]
				widths[in.Dest] = widths[in.Args[0]]
			}
		case "binary":
			if len(in.Args) != 2 || !wasmScalarBinary(in.Text) {
				return nil, false
			}
			if (wasmScalarIsFloat(kinds[in.Args[0]]) || wasmScalarIsFloat(kinds[in.Args[1]])) && in.Text == "%" {
				return nil, false
			}
			if wasmScalarComparison(in.Text) {
				kinds[in.Dest] = wasmScalarBool
				widths[in.Dest] = 1
			} else if wasmScalarIsFloat(kinds[in.Args[0]]) || wasmScalarIsFloat(kinds[in.Args[1]]) {
				if kinds[in.Args[0]] == wasmScalarFloat32 && kinds[in.Args[1]] == wasmScalarFloat32 {
					kinds[in.Dest] = wasmScalarFloat32
					widths[in.Dest] = 32
				} else {
					kinds[in.Dest] = wasmScalarFloat
					widths[in.Dest] = 64
				}
			} else {
				kinds[in.Dest] = kinds[in.Args[0]]
				widths[in.Dest] = widths[in.Args[0]]
			}
		default:
			return nil, false
		}
		if kinds[in.Dest] == 0 {
			return nil, false
		}
	}
	if emitted == "" || emittedKind == 0 || block.Term == nil {
		return nil, false
	}
	if _, ok := block.Term.(*irReturn); !ok {
		return nil, false
	}
	var code bytes.Buffer
	for _, in := range block.Instrs {
		if in.Op == "emit" {
			continue
		}
		if !writeWASMScalarInstruction(&code, in, locals, kinds, widths) {
			return nil, false
		}
	}
	writeWASMStep(&code)
	if !writeWASMScalarResult(&code, locals, emittedKind, emittedBits, emitted) {
		return nil, false
	}
	code.WriteByte(0x41)
	writeSLEB(&code, 0)
	code.WriteByte(0x0b)
	var body bytes.Buffer
	if len(locals) == 0 {
		writeULEB(&body, 0)
	} else {
		writeULEB(&body, 1)
		writeULEB(&body, uint64(len(locals)))
		body.WriteByte(0x7e)
	}
	body.Write(code.Bytes())
	return body.Bytes(), true
}

// emitWASMScalarCFGBody lowers a scalar CFG into a small dispatcher
// loop. WASM's structured control flow does not have arbitrary gotos, while
// Osiris SSA blocks do, so the dispatcher keeps the current block in an i32
// local and copies block arguments into their parameter locals before the next
// dispatch. Each block visit and lowered instruction is budgeted, so cycles
// cannot escape the generated execution limit.
func emitWASMScalarCFGBody(ir *IRProgram) ([]byte, bool) {
	if ir == nil || len(ir.Functions) != 1 || ir.Functions[0] == nil {
		return nil, false
	}
	fn := ir.Functions[0]
	if len(fn.Blocks) < 2 {
		return nil, false
	}
	blockIndex := make(map[string]uint32, len(fn.Blocks))
	for i, block := range fn.Blocks {
		if block == nil || block.Name == "" {
			return nil, false
		}
		blockIndex[block.Name] = uint32(i)
	}
	locals := make(map[string]uint32)
	kinds := make(map[string]wasmScalarKind)
	widths := make(map[string]uint16)
	addValue := func(name string) {
		if name == "" {
			return
		}
		if _, ok := locals[name]; !ok {
			locals[name] = uint32(len(locals))
		}
	}
	for _, block := range fn.Blocks {
		for _, param := range block.Params {
			addValue(param)
		}
		for _, in := range block.Instrs {
			addValue(in.Dest)
		}
	}
	if len(locals) == 0 {
		return nil, false
	}

	// Infer block parameters and instruction result kinds to a fixed point,
	// because a join parameter is defined by incoming edge arguments.
	for pass := 0; pass <= len(fn.Blocks)*2; pass++ {
		changed := false
		for _, block := range fn.Blocks {
			for _, in := range block.Instrs {
				kind := wasmScalarInstructionKind(in, kinds)
				if kind != 0 && kinds[in.Dest] != kind {
					kinds[in.Dest] = kind
					changed = true
				}
				width := wasmScalarInstructionWidth(in, kinds, widths)
				if width != 0 && widths[in.Dest] != width {
					widths[in.Dest] = width
					changed = true
				}
			}
			propagate := func(target string, args []string) bool {
				targetBlock := findWASMBlock(fn.Blocks, target)
				if targetBlock == nil || len(targetBlock.Params) != len(args) {
					return false
				}
				for i, arg := range args {
					kind := kinds[arg]
					width := widths[arg]
					if kind != 0 && kinds[targetBlock.Params[i]] != kind {
						kinds[targetBlock.Params[i]] = kind
						changed = true
					}
					if width != 0 && widths[targetBlock.Params[i]] != width {
						widths[targetBlock.Params[i]] = width
						changed = true
					}
				}
				return true
			}
			switch term := block.Term.(type) {
			case *irJump:
				if !propagate(term.Target, term.Args) {
					return nil, false
				}
			case *irBranch:
				if !propagate(term.TrueTarget, term.TrueArgs) || !propagate(term.FalseTarget, term.FalseArgs) {
					return nil, false
				}
			case *irReturn:
			case *irUnreachable:
			default:
				return nil, false
			}
		}
		if !changed {
			break
		}
	}

	var emitted string
	var emittedKind wasmScalarKind
	for _, block := range fn.Blocks {
		for _, in := range block.Instrs {
			if in.Op == "emit" {
				if emitted != "" || len(in.Args) != 1 {
					return nil, false
				}
				emitted = in.Args[0]
				emittedKind = kinds[emitted]
				continue
			}
			if in.Dest == "" || kinds[in.Dest] == 0 || (kinds[in.Dest] != wasmScalarNull && widths[in.Dest] == 0) || !wasmScalarInstructionKindSupported(in, kinds) {
				return nil, false
			}
		}
		for _, param := range block.Params {
			if kinds[param] == 0 || (kinds[param] != wasmScalarNull && widths[param] == 0) {
				return nil, false
			}
		}
	}
	if emitted == "" || emittedKind == 0 {
		return nil, false
	}

	stateLocal := uint32(len(locals))
	var code bytes.Buffer
	if len(locals) > 0 {
		writeULEB(&code, 2)
		writeULEB(&code, uint64(len(locals)))
		code.WriteByte(0x7e)
		writeULEB(&code, 1)
		code.WriteByte(0x7f)
	} else {
		writeULEB(&code, 1)
		writeULEB(&code, 1)
		code.WriteByte(0x7f)
	}
	code.WriteByte(0x41)
	writeSLEB(&code, 0)
	code.WriteByte(0x21)
	writeULEB(&code, uint64(stateLocal))
	code.WriteByte(0x02)
	code.WriteByte(0x40)
	code.WriteByte(0x03)
	code.WriteByte(0x40)
	for index, block := range fn.Blocks {
		code.WriteByte(0x20)
		writeULEB(&code, uint64(stateLocal))
		code.WriteByte(0x41)
		writeSLEB(&code, int64(index))
		code.WriteByte(0x46)
		code.WriteByte(0x04)
		code.WriteByte(0x40)
		writeWASMStep(&code)
		for _, in := range block.Instrs {
			if in.Op == "emit" {
				if len(in.Args) != 1 || !writeWASMScalarResult(&code, locals, kinds[in.Args[0]], widths[in.Args[0]], in.Args[0]) {
					return nil, false
				}
				writeWASMStep(&code)
				continue
			}
			if !writeWASMScalarInstruction(&code, in, locals, kinds, widths) {
				return nil, false
			}
		}
		switch term := block.Term.(type) {
		case *irJump:
			if !writeWASMScalarJump(&code, fn.Blocks, blockIndex, locals, term.Target, term.Args, stateLocal) {
				return nil, false
			}
			code.WriteByte(0x0c)
			writeULEB(&code, 1)
		case *irBranch:
			if !writeWASMScalarCondition(&code, locals, term.Cond) {
				return nil, false
			}
			code.WriteByte(0x04)
			code.WriteByte(0x40)
			if !writeWASMScalarJump(&code, fn.Blocks, blockIndex, locals, term.TrueTarget, term.TrueArgs, stateLocal) {
				return nil, false
			}
			code.WriteByte(0x0c)
			writeULEB(&code, 2)
			code.WriteByte(0x05)
			if !writeWASMScalarJump(&code, fn.Blocks, blockIndex, locals, term.FalseTarget, term.FalseArgs, stateLocal) {
				return nil, false
			}
			code.WriteByte(0x0c)
			writeULEB(&code, 2)
			code.WriteByte(0x0b)
		case *irReturn:
			if term.HasValue {
				if !writeWASMScalarResult(&code, locals, kinds[term.Value], widths[term.Value], term.Value) {
					return nil, false
				}
			}
			code.WriteByte(0x0c)
			writeULEB(&code, 2)
		case *irUnreachable:
			code.WriteByte(0x00)
		default:
			return nil, false
		}
		code.WriteByte(0x0b)
	}
	code.WriteByte(0x0c)
	writeULEB(&code, 0)
	code.WriteByte(0x0b)
	code.WriteByte(0x0b)
	code.WriteByte(0x41)
	writeSLEB(&code, 0)
	code.WriteByte(0x0b)
	return code.Bytes(), true
}

func wasmScalarInstructionKind(in IRInstr, kinds map[string]wasmScalarKind) wasmScalarKind {
	switch in.Op {
	case "const", "global":
		return wasmScalarKindForValue(in.Value)
	case "copy", "assign":
		if len(in.Args) == 1 {
			return kinds[in.Args[0]]
		}
	case "unary":
		if len(in.Args) != 1 || !wasmScalarUnary(in.Text) {
			return 0
		}
		arg := kinds[in.Args[0]]
		if arg == 0 {
			return 0
		}
		if in.Text == "!" {
			return wasmScalarBool
		}
		if in.Text == "-" && wasmScalarIsFloat(arg) {
			return arg
		}
		if in.Text == "-" {
			return wasmScalarInt
		}
		return arg
	case "binary":
		if len(in.Args) != 2 || !wasmScalarBinary(in.Text) {
			return 0
		}
		left, right := kinds[in.Args[0]], kinds[in.Args[1]]
		if left == 0 || right == 0 || (wasmScalarIsFloat(left) || wasmScalarIsFloat(right)) && in.Text == "%" {
			return 0
		}
		if wasmScalarComparison(in.Text) {
			return wasmScalarBool
		}
		if wasmScalarIsFloat(left) || wasmScalarIsFloat(right) {
			if left == wasmScalarFloat32 && right == wasmScalarFloat32 {
				return wasmScalarFloat32
			}
			return wasmScalarFloat
		}
		return left
	}
	return 0
}

func wasmScalarInstructionKindSupported(in IRInstr, kinds map[string]wasmScalarKind) bool {
	return wasmScalarInstructionKind(in, kinds) != 0
}

func wasmScalarWidthForValue(value Value) uint16 {
	switch value.kind {
	case kindUint:
		if value.ub != nil {
			return value.ubits
		}
		if value.bits == 0 {
			return 64
		}
		return uint16(value.bits)
	case kindInt:
		if value.ibits == 0 {
			return 64
		}
		return value.ibits
	case kindBool:
		return 1
	case kindNumber:
		if value.fbits == 32 {
			return 32
		}
		return 64
	default:
		return 0
	}
}

func wasmScalarInstructionWidth(in IRInstr, kinds map[string]wasmScalarKind, widths map[string]uint16) uint16 {
	switch in.Op {
	case "const", "global":
		return wasmScalarWidthForValue(in.Value)
	case "copy", "assign":
		if len(in.Args) == 1 {
			return widths[in.Args[0]]
		}
	case "unary":
		if len(in.Args) == 1 {
			if in.Text == "!" {
				return 1
			}
			return widths[in.Args[0]]
		}
	case "binary":
		if len(in.Args) == 2 {
			if wasmScalarComparison(in.Text) {
				return 1
			}
			left, right := kinds[in.Args[0]], kinds[in.Args[1]]
			if wasmScalarIsFloat(left) || wasmScalarIsFloat(right) {
				if left == wasmScalarFloat32 && right == wasmScalarFloat32 {
					return 32
				}
				return 64
			}
			return widths[in.Args[0]]
		}
	}
	return 0
}

func writeWASMScalarInstruction(code *bytes.Buffer, in IRInstr, locals map[string]uint32, kinds map[string]wasmScalarKind, widths map[string]uint16) bool {
	writeWASMStep(code)
	switch in.Op {
	case "const", "global":
		value, ok := wasmScalarValue(in.Value)
		if !ok {
			return false
		}
		code.WriteByte(0x42)
		writeSLEB(code, value)
	case "copy", "assign":
		if len(in.Args) != 1 {
			return false
		}
		code.WriteByte(0x20)
		writeULEB(code, uint64(locals[in.Args[0]]))
	case "unary":
		if len(in.Args) != 1 {
			return false
		}
		argKind := kinds[in.Args[0]]
		if wasmScalarIsFloat(argKind) {
			code.WriteByte(0x20)
			writeULEB(code, uint64(locals[in.Args[0]]))
			code.WriteByte(0xbf)
			if argKind == wasmScalarFloat32 {
				code.WriteByte(0xb6)
			}
			if in.Text == "-" {
				if argKind == wasmScalarFloat32 {
					code.WriteByte(0x91)
				} else {
					code.WriteByte(0x9a)
				}
			}
			if argKind == wasmScalarFloat32 {
				code.WriteByte(0xbb)
			}
			code.WriteByte(0xbd)
		} else if in.Text == "-" {
			code.WriteByte(0x42)
			writeSLEB(code, 0)
			code.WriteByte(0x20)
			writeULEB(code, uint64(locals[in.Args[0]]))
			code.WriteByte(0x7d)
		} else {
			code.WriteByte(0x20)
			writeULEB(code, uint64(locals[in.Args[0]]))
			if in.Text == "!" {
				code.WriteByte(0x50)
				code.WriteByte(0xad)
			} else if in.Text == "~" {
				code.WriteByte(0x42)
				writeSLEB(code, -1)
				code.WriteByte(0x85)
			} else if in.Text != "+" {
				return false
			}
		}
	case "binary":
		if len(in.Args) != 2 {
			return false
		}
		leftKind, rightKind := kinds[in.Args[0]], kinds[in.Args[1]]
		if wasmScalarIsFloat(leftKind) || wasmScalarIsFloat(rightKind) {
			narrow := leftKind == wasmScalarFloat32 && rightKind == wasmScalarFloat32
			wasmScalarFloatOperand(code, locals[in.Args[0]], leftKind, narrow)
			wasmScalarFloatOperand(code, locals[in.Args[1]], rightKind, narrow)
			code.WriteByte(wasmScalarFloatBinaryOpcode(in.Text, narrow))
			if wasmScalarComparison(in.Text) {
				code.WriteByte(0xad)
			} else {
				if narrow {
					code.WriteByte(0xbb)
				}
				code.WriteByte(0xbd)
			}
		} else {
			code.WriteByte(0x20)
			writeULEB(code, uint64(locals[in.Args[0]]))
			code.WriteByte(0x20)
			writeULEB(code, uint64(locals[in.Args[1]]))
			code.WriteByte(wasmScalarBinaryOpcode(in.Text, leftKind == wasmScalarUint))
			if wasmScalarComparison(in.Text) {
				code.WriteByte(0xad)
			}
		}
	default:
		return false
	}
	writeWASMScalarNormalize(code, kinds[in.Dest], widths[in.Dest])
	code.WriteByte(0x21)
	writeULEB(code, uint64(locals[in.Dest]))
	return true
}

func writeWASMScalarCondition(code *bytes.Buffer, locals map[string]uint32, value string) bool {
	local, ok := locals[value]
	if !ok {
		return false
	}
	code.WriteByte(0x20)
	writeULEB(code, uint64(local))
	code.WriteByte(0x42)
	writeSLEB(code, 0)
	code.WriteByte(0x52)
	return true
}

func writeWASMScalarJump(code *bytes.Buffer, blocks []*IRBlock, indexes map[string]uint32, locals map[string]uint32, target string, args []string, stateLocal uint32) bool {
	targetBlock := findWASMBlock(blocks, target)
	if targetBlock == nil || len(targetBlock.Params) != len(args) {
		return false
	}
	for i, arg := range args {
		argLocal, ok := locals[arg]
		if !ok {
			return false
		}
		code.WriteByte(0x20)
		writeULEB(code, uint64(argLocal))
		code.WriteByte(0x21)
		writeULEB(code, uint64(locals[targetBlock.Params[i]]))
	}
	index, ok := indexes[target]
	if !ok {
		return false
	}
	code.WriteByte(0x41)
	writeSLEB(code, int64(index))
	code.WriteByte(0x21)
	writeULEB(code, uint64(stateLocal))
	return true
}

func writeWASMScalarNormalize(code *bytes.Buffer, kind wasmScalarKind, width uint16) {
	if kind == wasmScalarUint && width > 0 && width < 64 {
		code.WriteByte(0x42)
		writeSLEB(code, int64((uint64(1)<<width)-1))
		code.WriteByte(0x83)
	} else if kind == wasmScalarInt {
		switch width {
		case 8:
			code.WriteByte(0xc2)
		case 16:
			code.WriteByte(0xc3)
		case 32:
			code.WriteByte(0xc4)
		}
	}
}

func writeWASMScalarResult(code *bytes.Buffer, locals map[string]uint32, kind wasmScalarKind, width uint16, value string) bool {
	local, ok := locals[value]
	if !ok || kind == 0 {
		return false
	}
	code.WriteByte(0x41)
	writeSLEB(code, 16)
	code.WriteByte(0x24)
	writeULEB(code, 2)
	code.WriteByte(0x20)
	writeULEB(code, uint64(local))
	if wasmScalarIsFloat(kind) {
		code.WriteByte(0xbf)
		code.WriteByte(0x24)
		writeULEB(code, 5)
	} else {
		code.WriteByte(0x24)
		writeULEB(code, 4)
	}
	code.WriteByte(0x41)
	writeSLEB(code, int64(kind))
	code.WriteByte(0x24)
	writeULEB(code, 3)
	code.WriteByte(0x41)
	writeSLEB(code, int64(width))
	code.WriteByte(0x24)
	writeULEB(code, 6)
	// The fixed result record is owned by the module. Its first two bytes are
	// kind and width; the payload is the i64-backed scalar bits at byte eight.
	for _, offset := range []int64{0, 1} {
		code.WriteByte(0x41)
		writeSLEB(code, offset)
		if offset == 0 {
			code.WriteByte(0x41)
			writeSLEB(code, int64(kind))
		} else {
			code.WriteByte(0x41)
			writeSLEB(code, int64(width))
		}
		code.WriteByte(0x3a)
		writeULEB(code, 0)
		writeULEB(code, 0)
	}
	code.WriteByte(0x41)
	writeSLEB(code, 8)
	code.WriteByte(0x20)
	writeULEB(code, uint64(local))
	code.WriteByte(0x37)
	writeULEB(code, 3)
	writeULEB(code, 0)
	return true
}

func findWASMBlock(blocks []*IRBlock, name string) *IRBlock {
	for _, block := range blocks {
		if block != nil && block.Name == name {
			return block
		}
	}
	return nil
}

func writeWASMStep(code *bytes.Buffer) {
	// Every lowered IR operation consumes one budget unit and increments the
	// exported execution counter. Returning a status from the generated
	// function keeps the limit failure deterministic and avoids relying on a
	// host-side replay of the IR.
	// Reject before consuming the next operation. This makes a budget of N
	// permit exactly N lowered steps instead of N+1.
	code.WriteByte(0x23)
	writeULEB(code, 8)
	code.WriteByte(0x42)
	writeSLEB(code, 0)
	code.WriteByte(0x57)
	code.WriteByte(0x04)
	code.WriteByte(0x40)
	code.WriteByte(0x41)
	writeSLEB(code, 1)
	code.WriteByte(0x0f)
	code.WriteByte(0x0b)
	code.WriteByte(0x23)
	writeULEB(code, 8)
	code.WriteByte(0x42)
	writeSLEB(code, 1)
	code.WriteByte(0x7d)
	code.WriteByte(0x24)
	writeULEB(code, 8)
	code.WriteByte(0x23)
	writeULEB(code, 7)
	code.WriteByte(0x42)
	writeSLEB(code, 1)
	code.WriteByte(0x7c)
	code.WriteByte(0x24)
	writeULEB(code, 7)
}

func wasmScalarKindForValue(value Value) wasmScalarKind {
	switch value.kind {
	case kindUint:
		return wasmScalarUint
	case kindInt:
		return wasmScalarInt
	case kindBool:
		return wasmScalarBool
	case kindNull:
		return wasmScalarNull
	case kindNumber:
		if value.bfloat {
			return 0
		}
		if value.fbits == 32 {
			return wasmScalarFloat32
		}
		return wasmScalarFloat
	default:
		return 0
	}
}

func wasmScalarValue(value Value) (int64, bool) {
	switch value.kind {
	case kindUint:
		if value.ub != nil || value.u > math.MaxInt64 {
			return int64(value.u), value.ub == nil
		}
		return int64(value.u), true
	case kindInt:
		if value.i == nil || !value.i.IsInt64() {
			return 0, false
		}
		return value.i.Int64(), true
	case kindBool:
		if value.boo {
			return 1, true
		}
		return 0, true
	case kindNull:
		return 0, true
	case kindNumber:
		if value.bfloat || math.IsNaN(value.num) || math.IsInf(value.num, 0) {
			return 0, false
		}
		return int64(math.Float64bits(value.num)), true
	default:
		return 0, false
	}
}

func wasmScalarUnary(op string) bool { return op == "+" || op == "-" || op == "!" || op == "~" }

func wasmScalarIsFloat(kind wasmScalarKind) bool {
	return kind == wasmScalarFloat || kind == wasmScalarFloat32
}

func wasmScalarBinary(op string) bool {
	switch op {
	case "+", "-", "*", "/", "%", "==", "!=", "<", "<=", ">", ">=":
		return true
	default:
		return false
	}
}

func wasmScalarComparison(op string) bool {
	switch op {
	case "==", "!=", "<", "<=", ">", ">=":
		return true
	default:
		return false
	}
}

func wasmScalarBinaryOpcode(op string, unsigned bool) byte {
	switch op {
	case "+":
		return 0x7c
	case "-":
		return 0x7d
	case "*":
		return 0x7e
	case "/":
		if unsigned {
			return 0x80
		}
		return 0x7f
	case "%":
		if unsigned {
			return 0x82
		}
		return 0x81
	case "==":
		return 0x51
	case "!=":
		return 0x52
	case "<":
		if unsigned {
			return 0x54
		}
		return 0x53
	case "<=":
		if unsigned {
			return 0x58
		}
		return 0x57
	case ">":
		if unsigned {
			return 0x56
		}
		return 0x55
	case ">=":
		if unsigned {
			return 0x5a
		}
		return 0x59
	default:
		return 0x7c
	}
}

func wasmScalarFloatOperand(code *bytes.Buffer, local uint32, kind wasmScalarKind, narrow bool) {
	code.WriteByte(0x20)
	writeULEB(code, uint64(local))
	if wasmScalarIsFloat(kind) {
		code.WriteByte(0xbf)
		if narrow {
			code.WriteByte(0xb6)
		}
	} else if kind == wasmScalarUint {
		code.WriteByte(0xb8)
	} else {
		code.WriteByte(0xb7)
	}
}

func wasmScalarFloatBinaryOpcode(op string, narrow bool) byte {
	if narrow {
		switch op {
		case "+":
			return 0x92
		case "-":
			return 0x93
		case "*":
			return 0x94
		case "/":
			return 0x95
		case "==":
			return 0x5b
		case "!=":
			return 0x5c
		case "<":
			return 0x5d
		case ">":
			return 0x5e
		case "<=":
			return 0x5f
		case ">=":
			return 0x60
		}
	}
	switch op {
	case "+":
		return 0xa0
	case "-":
		return 0xa1
	case "*":
		return 0xa2
	case "/":
		return 0xa3
	case "==":
		return 0x61
	case "!=":
		return 0x62
	case "<":
		return 0x63
	case ">":
		return 0x64
	case "<=":
		return 0x65
	case ">=":
		return 0x66
	default:
		return 0xa0
	}
}

func emitWASMInspectionBody(ir *IRProgram) []byte {
	var body bytes.Buffer
	if ir == nil {
		return nil
	}
	for _, fn := range ir.Functions {
		for _, block := range fn.Blocks {
			for _, in := range block.Instrs {
				switch in.Op {
				case "const":
					if in.Value.kind == kindNumber && !in.Value.bfloat {
						body.WriteByte(0x41)
						writeSLEB(&body, 0)
						body.WriteByte(0x1a)
					} else {
						body.WriteByte(0x41)
						writeSLEB(&body, 0)
						body.WriteByte(0x1a)
					}
				case "binary":
					body.WriteByte(0x41)
					writeSLEB(&body, 0)
					body.WriteByte(0x41)
					writeSLEB(&body, 0)
					switch in.Text {
					case "+":
						body.WriteByte(0x6a)
					case "-":
						body.WriteByte(0x6b)
					case "*":
						body.WriteByte(0x6c)
					case "==":
						body.WriteByte(0x46)
					case "<":
						body.WriteByte(0x48)
					case ">":
						body.WriteByte(0x4a)
					default:
						body.WriteByte(0x6a)
					}
					body.WriteByte(0x1a)
				case "unary":
					body.WriteByte(0x41)
					writeSLEB(&body, 0)
					body.WriteByte(0x1a)
				}
			}
		}
	}
	return body.Bytes()
}

func writeWASMGlobal(buf *bytes.Buffer, value int32, mutable bool) {
	buf.WriteByte(0x7f)
	if mutable {
		buf.WriteByte(0x01)
	} else {
		buf.WriteByte(0x00)
	}
	writeWASMConstExpr(buf, value)
}

func writeWASMI64Global(buf *bytes.Buffer, value int64, mutable bool) {
	buf.WriteByte(0x7e)
	if mutable {
		buf.WriteByte(0x01)
	} else {
		buf.WriteByte(0x00)
	}
	buf.WriteByte(0x42)
	writeSLEB(buf, value)
	buf.WriteByte(0x0b)
}

func writeWASMF64Global(buf *bytes.Buffer, value float64, mutable bool) {
	buf.WriteByte(0x7c)
	if mutable {
		buf.WriteByte(0x01)
	} else {
		buf.WriteByte(0x00)
	}
	buf.WriteByte(0x44)
	var bits [8]byte
	binary.LittleEndian.PutUint64(bits[:], math.Float64bits(value))
	buf.Write(bits[:])
	buf.WriteByte(0x0b)
}

func writeWASMConstExpr(buf *bytes.Buffer, value int32) {
	buf.WriteByte(0x41)
	writeSLEB(buf, int64(value))
	buf.WriteByte(0x0b)
}

func writeWASMExport(buf *bytes.Buffer, name string, kind byte, index uint64) {
	writeULEB(buf, uint64(len(name)))
	buf.WriteString(name)
	buf.WriteByte(kind)
	writeULEB(buf, index)
}

func writeWASMSection(buf *bytes.Buffer, id byte, build func(*bytes.Buffer)) {
	var payload bytes.Buffer
	build(&payload)
	buf.WriteByte(id)
	writeULEB(buf, uint64(payload.Len()))
	buf.Write(payload.Bytes())
}

func writeULEB(buf *bytes.Buffer, v uint64) {
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		buf.WriteByte(b)
		if v == 0 {
			return
		}
	}
}

func writeSLEB(buf *bytes.Buffer, v int64) {
	for {
		b := byte(v & 0x7f)
		v >>= 7
		more := !((v == 0 && b&0x40 == 0) || (v == -1 && b&0x40 != 0))
		if more {
			b |= 0x80
		}
		buf.WriteByte(b)
		if !more {
			return
		}
	}
}

func wasmLiteralPreview(program *wasmProgram) string {
	values, ok := evaluateWASMConstantPreview(program)
	if !ok {
		return ""
	}
	payload, err := json.Marshal(values)
	if err != nil {
		return ""
	}
	return string(payload)
}

func wasmPages(n int) int {
	if n <= 0 {
		return 1
	}
	return (n + wasmPageSize - 1) / wasmPageSize
}

// Keep math imported in this file's compile-time boundary: the generated
// module rejects non-finite literals before JSON or WASM encoding can accept
// an implementation-dependent representation.
var _ = math.MaxFloat64
