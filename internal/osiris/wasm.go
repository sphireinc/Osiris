package osiris

import (
	"bytes"
	"context"
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
	Version        uint32   `json:"version"`
	Profile        string   `json:"profile"`
	MemoryPageSize uint32   `json:"memory_page_size"`
	ResultEncoding string   `json:"result_encoding"`
	Cancellation   string   `json:"cancellation"`
	Ownership      string   `json:"ownership"`
	Deterministic  bool     `json:"deterministic"`
	Modules        []string `json:"modules,omitempty"`
	Capabilities   []string `json:"capabilities,omitempty"`
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
	if err := rejectTask134FutureBackend(prog, "WASM"); err != nil {
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
	payload, err := marshalWASMPayload(compiled)
	if err != nil {
		return nil, fmt.Errorf("WASM backend: encode ABI payload: %w", err)
	}
	if uint64(len(payload)) > limits.MaxArtifactBytes {
		return nil, wasmLimitError("artifact", limits.MaxArtifactBytes)
	}
	body := emitWASMInspectionBody(ir)
	binary := buildWASMModule(payload, body, limits)
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
			MemoryPageSize: wasmPageSize,
			ResultEncoding: "osiris-json-v1",
			Cancellation:   "context-check-per-instruction",
			Ownership:      "returned-values-owned; linear-memory-borrowed-during-call",
			Deterministic:  true,
			Modules:        append([]string(nil), compiled.ABI.Modules...),
			Capabilities:   append([]string(nil), compiled.ABI.Capabilities...),
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
	b.WriteString("  ;; result encoding: osiris-json-v1; owned results; bounded linear memory\n")
	b.WriteString("  (memory (export \"memory\") 1 ")
	b.WriteString(strconv.FormatUint((limits.MaxMemoryBytes+wasmPageSize-1)/wasmPageSize, 10))
	b.WriteString(")\n")
	b.WriteString("  (global (export \"abi_version\") i32 (i32.const 1))\n")
	b.WriteString("  (global (export \"result_ptr\") i32 (i32.const 0))\n")
	b.WriteString("  (global (export \"result_len\") i32 (i32.const 0))\n")
	b.WriteString("  (func (export \"run\") (result i32)\n")
	b.WriteString("    ;; lowered scalar/control-flow inspection body\n")
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

func buildWASMModule(payload, body []byte, limits WASMExecutionLimits) []byte {
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
		writeULEB(buf, 3)
		writeWASMGlobal(buf, 1)
		writeWASMGlobal(buf, 0)
		writeWASMGlobal(buf, 0)
	})
	writeWASMSection(&out, 7, func(buf *bytes.Buffer) {
		writeULEB(buf, 5)
		writeWASMExport(buf, "memory", 0x02, 0)
		writeWASMExport(buf, "abi_version", 0x03, 0)
		writeWASMExport(buf, "result_ptr", 0x03, 1)
		writeWASMExport(buf, "result_len", 0x03, 2)
		writeWASMExport(buf, "run", 0x00, 0)
	})
	writeWASMSection(&out, 10, func(buf *bytes.Buffer) {
		writeULEB(buf, 1)
		var fn bytes.Buffer
		writeULEB(&fn, 0)
		fn.Write(body)
		fn.WriteByte(0x41)
		writeSLEB(&fn, 0)
		fn.WriteByte(0x0b)
		writeULEB(buf, uint64(fn.Len()))
		buf.Write(fn.Bytes())
	})
	writeWASMSection(&out, 0, func(buf *bytes.Buffer) {
		writeULEB(buf, uint64(len(wasmCustomSection)))
		buf.WriteString(wasmCustomSection)
		buf.Write(payload)
	})
	return out.Bytes()
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

func writeWASMGlobal(buf *bytes.Buffer, value int32) {
	buf.WriteByte(0x7f)
	buf.WriteByte(0x00)
	writeWASMConstExpr(buf, value)
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
