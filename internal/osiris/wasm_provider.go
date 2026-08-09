package osiris

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

var errWASMInstructionLimit = errors.New("WASM backend: instructions limit exceeded")

type wasmProgram struct {
	ABI        WASMABI             `json:"abi"`
	Limits     WASMExecutionLimits `json:"limits"`
	SourceName string              `json:"source_name"`
	Functions  []wasmFunction      `json:"functions"`
}

type wasmFunction struct {
	Name          string      `json:"name"`
	Params        []string    `json:"params"`
	ParamValues   []string    `json:"param_values,omitempty"`
	Captures      []string    `json:"captures,omitempty"`
	CaptureValues []string    `json:"capture_values,omitempty"`
	Blocks        []wasmBlock `json:"blocks"`
}

type wasmBlock struct {
	Name   string      `json:"name"`
	Params []string    `json:"params,omitempty"`
	Instrs []wasmInstr `json:"instrs,omitempty"`
	Term   wasmTerm    `json:"term"`
}

type wasmInstr struct {
	Dest  string       `json:"dest,omitempty"`
	Op    string       `json:"op"`
	Args  []string     `json:"args,omitempty"`
	Text  string       `json:"text,omitempty"`
	Value *wasmLiteral `json:"value,omitempty"`
}

type wasmTerm struct {
	Kind        string   `json:"kind"`
	Value       string   `json:"value,omitempty"`
	Target      string   `json:"target,omitempty"`
	Args        []string `json:"args,omitempty"`
	Cond        string   `json:"cond,omitempty"`
	TrueTarget  string   `json:"true_target,omitempty"`
	TrueArgs    []string `json:"true_args,omitempty"`
	FalseTarget string   `json:"false_target,omitempty"`
	FalseArgs   []string `json:"false_args,omitempty"`
}

type wasmLiteral struct {
	Kind           string         `json:"kind"`
	Number         float64        `json:"number,omitempty"`
	Bits           uint16         `json:"bits,omitempty"`
	BFloat         bool           `json:"bfloat,omitempty"`
	Bool           bool           `json:"bool,omitempty"`
	Uint           uint64         `json:"uint,omitempty"`
	Int            string         `json:"int,omitempty"`
	Text           string         `json:"text,omitempty"`
	Packed         bool           `json:"packed,omitempty"`
	Native         bool           `json:"native,omitempty"`
	Children       []*wasmLiteral `json:"children,omitempty"`
	MapKeys        []*wasmLiteral `json:"map_keys,omitempty"`
	Shape          []int          `json:"shape,omitempty"`
	TypeName       string         `json:"type_name,omitempty"`
	CaseName       string         `json:"case_name,omitempty"`
	Present        bool           `json:"present,omitempty"`
	ResultOK       bool           `json:"result_ok,omitempty"`
	ErrorKind      string         `json:"error_kind,omitempty"`
	ErrorText      string         `json:"error_text,omitempty"`
	Decimal        string         `json:"decimal,omitempty"`
	BigFloat       string         `json:"bigfloat,omitempty"`
	ComplexReal    float64        `json:"complex_real,omitempty"`
	ComplexImag    float64        `json:"complex_imag,omitempty"`
	FixedWidth     uint16         `json:"fixed_width,omitempty"`
	FixedFrac      uint16         `json:"fixed_frac,omitempty"`
	FixedRaw       string         `json:"fixed_raw,omitempty"`
	IntervalLo     string         `json:"interval_lo,omitempty"`
	IntervalHi     string         `json:"interval_hi,omitempty"`
	Modulus        string         `json:"modulus,omitempty"`
	ModValue       string         `json:"mod_value,omitempty"`
	QuantityDims   map[string]int `json:"quantity_dims,omitempty"`
	Duration       int64          `json:"duration,omitempty"`
	DateTimeUnix   int64          `json:"datetime_unix,omitempty"`
	DateTimeNano   int32          `json:"datetime_nano,omitempty"`
	LocationName   string         `json:"location_name,omitempty"`
	LocationKind   uint8          `json:"location_kind,omitempty"`
	LocationOffset int            `json:"location_offset,omitempty"`
}

// WASMProvider is the narrow execution boundary used by CompileWASM clients.
// Implementations must consume a validated artifact and return owned values.
type WASMProvider interface {
	Execute(context.Context, []byte, WASMExecutionLimits) (WASMExecution, error)
}

// DeterministicWASMProvider is the portable in-repository runner. It executes
// scalar instruction bodies through the WASM engine and uses the validated
// Osiris ABI payload only for forms outside the scalar lowering boundary, with
// no source or interpreter access and no unbounded host allocation.
type DeterministicWASMProvider struct{}

func NewDeterministicWASMProvider() WASMProvider {
	return DeterministicWASMProvider{}
}

func (DeterministicWASMProvider) Execute(ctx context.Context, artifact []byte, limits WASMExecutionLimits) (WASMExecution, error) {
	limits = normalizeWASMLimits(limits)
	if ctx == nil {
		ctx = context.Background()
	}
	program, err := parseWASMArtifact(artifact, limits)
	if err != nil {
		return WASMExecution{}, err
	}
	effective := wasmMinimumLimits(program.Limits, limits)
	if program.ABI.ExecutionMode == "scalar-i64-v1" {
		engineResult, err := executeWASMModule(ctx, artifact, effective.MaxInstructions, program.SourceName)
		if err != nil {
			return WASMExecution{}, err
		}
		if engineResult.instructions == 0 {
			return WASMExecution{}, fmt.Errorf("WASM backend: scalar module did not execute an instruction")
		}
		return executeWASMScalarResult(engineResult, effective, engineResult.instructions)
	}
	if program.ABI.ExecutionMode == "aggregate-record-v1" {
		engineResult, err := executeWASMModule(ctx, artifact, effective.MaxInstructions, program.SourceName)
		if err != nil {
			return WASMExecution{}, err
		}
		return executeWASMAggregateResult(engineResult.record, effective, engineResult.instructions)
	}
	_, err = executeWASMModule(ctx, artifact, 0, program.SourceName)
	if err != nil {
		return WASMExecution{}, err
	}
	return executeWASMProgram(ctx, program, effective)
}

// executeWASMModule validates the executable section with a standards-
// compliant WASM engine before the Osiris ABI payload is decoded. The custom
// section carries Osiris values and capabilities, while the engine owns
// validation and execution of the generated module itself; neither source nor
// an interpreter is passed to the engine.
type wasmEngineResult struct {
	kind         uint64
	value        uint64
	float        uint64
	bits         uint64
	instructions uint64
	record       []byte
}

func executeWASMModule(ctx context.Context, artifact []byte, instructionBudget uint64, sourceName string) (wasmEngineResult, error) {
	engine := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().WithCloseOnContextDone(true))
	defer engine.Close(ctx)
	compiled, err := engine.CompileModule(ctx, artifact)
	if err != nil {
		return wasmEngineResult{}, fmt.Errorf("WASM backend: engine compile: %w", err)
	}
	defer compiled.Close(ctx)
	module, err := engine.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
	if err != nil {
		return wasmEngineResult{}, fmt.Errorf("WASM backend: engine instantiate: %w", err)
	}
	defer module.Close(ctx)
	run := module.ExportedFunction("run")
	if run == nil {
		return wasmEngineResult{}, fmt.Errorf("WASM backend: engine artifact has no run export")
	}
	if budget := module.ExportedGlobal("instruction_budget"); budget != nil && instructionBudget != 0 {
		mutable, ok := budget.(api.MutableGlobal)
		if !ok {
			return wasmEngineResult{}, fmt.Errorf("WASM backend: instruction budget global is immutable")
		}
		mutable.Set(instructionBudget)
	}
	results, err := run.Call(ctx)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return wasmEngineResult{}, ctxErr
		}
		return wasmEngineResult{}, wasmTrapError(sourceName, err)
	}
	if len(results) != 1 {
		return wasmEngineResult{}, fmt.Errorf("WASM backend: engine run returned invalid status")
	}
	if results[0] == 1 {
		return wasmEngineResult{}, errWASMInstructionLimit
	}
	if results[0] != 0 {
		return wasmEngineResult{}, fmt.Errorf("WASM backend: engine run returned invalid status")
	}
	kind := module.ExportedGlobal("result_kind")
	value := module.ExportedGlobal("result_value")
	float := module.ExportedGlobal("result_f64")
	bits := module.ExportedGlobal("result_bits")
	resultPtr := module.ExportedGlobal("result_ptr")
	resultLen := module.ExportedGlobal("result_len")
	if kind == nil || value == nil || resultPtr == nil || resultLen == nil {
		return wasmEngineResult{}, fmt.Errorf("WASM backend: generated result ABI is incomplete")
	}
	var floatBits uint64
	if float != nil {
		floatBits = float.Get()
	}
	var instructions uint64
	if count := module.ExportedGlobal("instruction_count"); count != nil {
		instructions = count.Get()
	}
	var width uint64
	if bits != nil {
		width = bits.Get()
	}
	memory := module.Memory()
	if memory == nil {
		return wasmEngineResult{}, fmt.Errorf("WASM backend: generated result record is unavailable")
	}
	if resultLen.Get() == 0 || resultLen.Get() > uint64(^uint32(0)) || resultPtr.Get() > uint64(^uint32(0)) {
		return wasmEngineResult{}, fmt.Errorf("WASM backend: generated result bounds are invalid")
	}
	record, ok := memory.Read(uint32(resultPtr.Get()), uint32(resultLen.Get()))
	if !ok {
		return wasmEngineResult{}, fmt.Errorf("WASM backend: generated result record is unavailable")
	}
	if kind.Get() == 0 {
		// payload-v1 bodies intentionally expose no generated result record yet;
		// the validated compatibility runner owns those results.
		return wasmEngineResult{instructions: instructions}, nil
	}
	if kind.Get() > 6 {
		return wasmEngineResult{kind: kind.Get(), value: value.Get(), bits: width, instructions: instructions, record: append([]byte(nil), record...)}, nil
	}
	if len(record) < 16 {
		return wasmEngineResult{}, fmt.Errorf("WASM backend: generated scalar result record is truncated")
	}
	if uint64(record[0]) != kind.Get() || uint64(record[1]) != width {
		return wasmEngineResult{}, fmt.Errorf("WASM backend: generated result record does not match result globals")
	}
	recordValue := binary.LittleEndian.Uint64(record[8:])
	if recordValue != value.Get() && kind.Get() <= 6 && kind.Get() != 4 && kind.Get() != 5 {
		return wasmEngineResult{}, fmt.Errorf("WASM backend: generated result payload does not match result global")
	}
	if kind.Get() == 4 || kind.Get() == 5 {
		floatBits = recordValue
	}
	return wasmEngineResult{kind: kind.Get(), value: recordValue, float: floatBits, bits: width, instructions: instructions, record: append([]byte(nil), record...)}, nil
}

func executeWASMAggregateResult(record []byte, limits WASMExecutionLimits, instructions uint64) (WASMExecution, error) {
	if len(record) < 8 || string(record[:4]) != "OSMR" {
		return WASMExecution{}, fmt.Errorf("WASM backend: malformed aggregate result record")
	}
	count := binary.LittleEndian.Uint32(record[4:8])
	if count > limits.MaxOutputs {
		return WASMExecution{}, wasmLimitError("outputs", uint64(limits.MaxOutputs))
	}
	outputs := make([]Value, 0, count)
	offset := 8
	for i := uint32(0); i < count; i++ {
		if offset+4 > len(record) {
			return WASMExecution{}, fmt.Errorf("WASM backend: truncated aggregate result length")
		}
		length := int(binary.LittleEndian.Uint32(record[offset : offset+4]))
		offset += 4
		if length > len(record)-offset {
			return WASMExecution{}, fmt.Errorf("WASM backend: truncated aggregate result value")
		}
		value, err := decodeValueBytes(record[offset : offset+length])
		if err != nil {
			return WASMExecution{}, fmt.Errorf("WASM backend: decode aggregate result %d: %w", i, err)
		}
		outputs = append(outputs, value)
		offset += length
	}
	if offset != len(record) {
		return WASMExecution{}, fmt.Errorf("WASM backend: aggregate result has trailing bytes")
	}
	encoded, err := json.Marshal(outputs)
	if err != nil {
		return WASMExecution{}, fmt.Errorf("WASM backend: encode aggregate result: %w", err)
	}
	if uint64(len(encoded)) > limits.MaxMemoryBytes || uint64(len(record)) > limits.MaxMemoryBytes {
		return WASMExecution{}, wasmLimitError("memory", limits.MaxMemoryBytes)
	}
	return WASMExecution{Outputs: outputs, JSON: string(encoded), Instructions: instructions, MemoryBytes: uint64(len(record))}, nil
}

func wasmTrapError(sourceName string, err error) error {
	reason := err.Error()
	switch {
	case strings.Contains(reason, "integer divide by zero"):
		reason = "integer division by zero"
	case strings.Contains(reason, "integer overflow"):
		reason = "signed integer overflow"
	case strings.Contains(reason, "unreachable"):
		reason = "unreachable trap"
	case strings.Contains(reason, "out of bounds"):
		reason = "linear-memory bounds trap"
	default:
		reason = "execution trap: " + reason
	}
	if sourceName == "" {
		sourceName = "<wasm>"
	}
	return fmt.Errorf("WASM backend: %s: %s", sourceName, reason)
}

func executeWASMScalarResult(result wasmEngineResult, limits WASMExecutionLimits, instructions uint64) (WASMExecution, error) {
	if result.kind == 0 {
		return WASMExecution{}, fmt.Errorf("WASM backend: scalar module did not produce a result")
	}
	var value Value
	switch result.kind {
	case 1:
		bits := uint8(result.bits)
		if bits == 0 {
			bits = 64
		}
		value = uintValue(result.value, bits)
	case 2:
		bits := uint16(result.bits)
		if bits == 0 {
			bits = 64
		}
		value = intValue(new(big.Int).SetInt64(int64(result.value)), bits)
	case 3:
		value = boolValue(result.value != 0)
	case 4:
		value = floatValue(math.Float64frombits(result.float), 64)
	case 5:
		value = floatValue(math.Float64frombits(result.float), 32)
	case 6:
		value = nullValue()
	default:
		return WASMExecution{}, fmt.Errorf("WASM backend: unknown scalar result kind %d", result.kind)
	}
	encoded, err := json.Marshal([]Value{value})
	if err != nil {
		return WASMExecution{}, fmt.Errorf("WASM backend: encode scalar result: %w", err)
	}
	if uint64(len(encoded)) > limits.MaxMemoryBytes {
		return WASMExecution{}, wasmLimitError("memory", limits.MaxMemoryBytes)
	}
	return WASMExecution{Outputs: []Value{value}, JSON: string(encoded), Instructions: instructions, MemoryBytes: uint64(len(encoded))}, nil
}

func wasmMinimumLimits(left, right WASMExecutionLimits) WASMExecutionLimits {
	left = normalizeWASMLimits(left)
	right = normalizeWASMLimits(right)
	if right.MaxInstructions < left.MaxInstructions {
		left.MaxInstructions = right.MaxInstructions
	}
	if right.MaxMemoryBytes < left.MaxMemoryBytes {
		left.MaxMemoryBytes = right.MaxMemoryBytes
	}
	if right.MaxOutputs < left.MaxOutputs {
		left.MaxOutputs = right.MaxOutputs
	}
	if right.MaxArtifactBytes < left.MaxArtifactBytes {
		left.MaxArtifactBytes = right.MaxArtifactBytes
	}
	return left
}

func normalizeWASMProfile(profile WASMCapabilityProfile) WASMCapabilityProfile {
	if profile.Name == "" {
		profile = DefaultWASMCapabilityProfile
	}
	profile.Modules = append([]string(nil), profile.Modules...)
	profile.Capabilities = append([]string(nil), profile.Capabilities...)
	sort.Strings(profile.Modules)
	sort.Strings(profile.Capabilities)
	profile.Modules = uniqueStrings(profile.Modules)
	profile.Capabilities = uniqueStrings(profile.Capabilities)
	return profile
}

func uniqueStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

func wasmProfileHasModule(profile WASMCapabilityProfile, name string) bool {
	for _, module := range profile.Modules {
		if module == name {
			return true
		}
	}
	return false
}

func wasmProfileHasCapability(profile WASMCapabilityProfile, capability string) bool {
	for _, granted := range profile.Capabilities {
		if granted == capability {
			return true
		}
	}
	return false
}

func wasmModuleMemberCapability(module, member string) string {
	switch module {
	case "std/unsafe":
		return UnsafeMemoryCapability
	case "std/db":
		return "db.embedded"
	case "std/regex":
		return "regex.engine"
	case "std/i18n":
		return "i18n.data"
	case "std/telemetry":
		return "telemetry.memory"
	case "std/testing":
		return "testing.assertions"
	case "std/time":
		if member == "now" {
			return "clock.virtual"
		}
	case "std/os":
		switch member {
		case "args", "args_all", "platform":
			return "clock.virtual"
		case "env", "env_names", "getenv", "lookup_env", "environ", "expand_env":
			return "os.environment"
		case "setenv", "unsetenv", "clearenv":
			return "os.environment_mutate"
		case "read_file", "read", "read_at":
			return "os.file_read"
		case "write_file", "write", "write_at":
			return "os.file_write"
		case "stat", "lstat", "path_exists", "path_not_exists", "is_exist", "is_not_exist":
			return "os.file_metadata"
		case "read_dir", "read_dir_entries":
			return "os.directory_read"
		case "mkdir", "mkdir_all", "remove", "remove_all", "rename":
			return "os.directory_mutate"
		}
	}
	return ""
}

func wasmProfileAllowsMember(profile WASMCapabilityProfile, module, member string) bool {
	if !wasmProfileHasModule(profile, module) {
		return false
	}
	if module == "std/net" {
		return false
	}
	if module == "std/os" && !wasmOSMemberAllowed(member) {
		return false
	}
	// The deterministic profile exposes every member of pure modules, while
	// future math families remain protected by the existing reserved-type gate.
	if module == "std/math" {
		switch member {
		case "poly", "poly_degree", "poly_eval", "quat", "quat_conjugate", "quat_norm", "quat_inverse",
			"simd", "simd_mask", "simd_select", "sparse_matrix", "sparse_tensor", "nnz", "to_dense", "to_sparse":
			return false
		}
	}
	capability := wasmModuleMemberCapability(module, member)
	return capability == "" || wasmProfileHasCapability(profile, capability)
}

func wasmHostImportFor(module, member string) WASMHostImport {
	return WASMHostImport{
		Module:     module,
		Member:     member,
		Capability: wasmModuleMemberCapability(module, member),
		Encoding:   WASMHostImportEncoding,
	}
}

func sortedWASMHostImports(imports map[WASMHostImport]struct{}) []WASMHostImport {
	out := make([]WASMHostImport, 0, len(imports))
	for importDecl := range imports {
		out = append(out, importDecl)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Module != out[j].Module {
			return out[i].Module < out[j].Module
		}
		return out[i].Member < out[j].Member
	})
	return out
}

func wasmOSMemberAllowed(member string) bool {
	switch member {
	case "args", "args_all", "env", "env_names", "getenv", "lookup_env", "environ", "expand_env",
		"setenv", "unsetenv", "clearenv", "read_file", "write_file", "stat", "lstat", "path_exists",
		"path_not_exists", "is_exist", "is_not_exist", "read_dir", "read_dir_entries", "mkdir", "mkdir_all",
		"remove", "remove_all", "rename", "platform":
		return true
	default:
		return false
	}
}

func lowerWASMProgram(ir *IRProgram, limits WASMExecutionLimits, profile WASMCapabilityProfile) (*wasmProgram, error) {
	if ir == nil || len(ir.Functions) == 0 {
		return nil, fmt.Errorf("WASM backend: IR has no entry function")
	}
	profile = normalizeWASMProfile(profile)
	program := &wasmProgram{
		ABI: WASMABI{
			Version:        WASMABIVersion,
			Profile:        profile.Name,
			MemoryPageSize: wasmPageSize,
			ResultEncoding: "osiris-json-v1",
			Cancellation:   "context-check-per-instruction",
			Ownership:      "returned-values-owned; linear-memory-borrowed-during-call",
			Deterministic:  true,
			Modules:        append([]string(nil), profile.Modules...),
			Capabilities:   append([]string(nil), profile.Capabilities...),
		},
		Limits:     limits,
		SourceName: ir.SourceName,
	}
	hostImports := make(map[WASMHostImport]struct{})
	var instructionCount uint64
	for _, fn := range ir.Functions {
		if fn == nil {
			return nil, fmt.Errorf("WASM backend: nil function")
		}
		outFn := wasmFunction{
			Name:          fn.Name,
			Params:        append([]string(nil), fn.Params...),
			Captures:      append([]string(nil), fn.Captures...),
			CaptureValues: append([]string(nil), fn.CaptureValues...),
		}
		moduleValues := make(map[string]string)
		for _, param := range fn.Params {
			outFn.ParamValues = append(outFn.ParamValues, fmt.Sprintf("%s.%s.0", fn.Name, param))
		}
		for _, block := range fn.Blocks {
			if block == nil {
				return nil, wasmUnsupported(ir.SourceName, fn.SourceName, "nil basic block")
			}
			outBlock := wasmBlock{Name: block.Name, Params: append([]string(nil), block.Params...)}
			for _, in := range block.Instrs {
				instructionCount++
				if instructionCount > limits.MaxInstructions {
					return nil, wasmLimitError("static instructions", limits.MaxInstructions)
				}
				lowered, err := lowerWASMInstr(ir.SourceName, fn, block, in, moduleValues, profile)
				if err != nil {
					return nil, err
				}
				if in.Op == "member" {
					if module := moduleValues[firstIRArg(in)]; module != "" {
						hostImports[wasmHostImportFor(module, in.Text)] = struct{}{}
					}
				}
				outBlock.Instrs = append(outBlock.Instrs, lowered)
			}
			term, err := lowerWASMTerm(ir.SourceName, fn, block.Term)
			if err != nil {
				return nil, err
			}
			outBlock.Term = term
			outFn.Blocks = append(outFn.Blocks, outBlock)
		}
		if len(outFn.Blocks) == 0 {
			return nil, wasmUnsupported(ir.SourceName, fn.SourceName, "function has no basic blocks")
		}
		program.Functions = append(program.Functions, outFn)
	}
	program.ABI.Imports = sortedWASMHostImports(hostImports)
	return program, nil
}

func lowerWASMInstr(source string, fn *IRFunction, block *IRBlock, in IRInstr, moduleValues map[string]string, profile WASMCapabilityProfile) (wasmInstr, error) {
	out := wasmInstr{Dest: in.Dest, Op: in.Op, Args: append([]string(nil), in.Args...), Text: in.Text}
	switch in.Op {
	case "const", "global":
		literal, err := wasmLiteralFromValue(in.Value)
		if err != nil {
			return wasmInstr{}, wasmInstrUnsupported(source, fn, block, in, err.Error())
		}
		out.Value = literal
	case "copy", "asserttype", "emit", "unary", "binary", "assign", "list", "tuple", "enum", "resultbind", "cleanup", "builtin", "call", "matchenum", "enumpayload", "matchlist", "listitem", "matchtype", "matchfail":
		if in.Op == "builtin" && !wasmBuiltinAllowed(in.Text) {
			return wasmInstr{}, wasmInstrUnsupported(source, fn, block, in, "builtin is outside the deterministic WASM profile")
		}
		if in.Op == "unary" {
			if wasmToken(in.Text) == tokenEOF {
				return wasmInstr{}, wasmInstrUnsupported(source, fn, block, in, "unsupported unary operator")
			}
		}
		if in.Op == "binary" {
			if wasmToken(in.Text) == tokenEOF {
				return wasmInstr{}, wasmInstrUnsupported(source, fn, block, in, "unsupported binary operator")
			}
		}
		if in.Value.kind != kindNull && (in.Op == "matchenum" || in.Op == "enumpayload" || in.Op == "matchlist" || in.Op == "listitem") {
			literal, err := wasmLiteralFromValue(in.Value)
			if err != nil {
				return wasmInstr{}, wasmInstrUnsupported(source, fn, block, in, err.Error())
			}
			out.Value = literal
		}
	case "makefunc":
		// User functions are represented by an in-runner callable reference. The
		// reference carries only immutable captured values across the ABI.
	case "import":
		module := strings.Trim(in.Text, "\"")
		if !wasmProfileHasModule(profile, module) {
			return wasmInstr{}, wasmInstrUnsupported(source, fn, block, in, "module is outside the deterministic WASM profile")
		}
		moduleValues[in.Dest] = module
	case "member":
		if module := moduleValues[firstIRArg(in)]; module != "" && !wasmProfileAllowsMember(profile, module, in.Text) {
			capability := wasmModuleMemberCapability(module, in.Text)
			if capability != "" {
				return wasmInstr{}, wasmInstrUnsupported(source, fn, block, in, fmt.Sprintf("member %s.%s requires WASM capability %q", module, in.Text, capability))
			}
			return wasmInstr{}, wasmInstrUnsupported(source, fn, block, in, fmt.Sprintf("member %s.%s is outside the deterministic WASM profile", module, in.Text))
		}
	case "undef":
		return wasmInstr{}, wasmInstrUnsupported(source, fn, block, in, "undefined values are not in the WASM ABI")
	case "self":
		return wasmInstr{}, wasmInstrUnsupported(source, fn, block, in, "self references are not in the WASM ABI")
	case "async", "try":
		return wasmInstr{}, wasmInstrUnsupported(source, fn, block, in, "operation is outside the deterministic WASM profile")
	default:
		return wasmInstr{}, wasmInstrUnsupported(source, fn, block, in, "unknown IR operation")
	}
	return out, nil
}

func lowerWASMTerm(source string, fn *IRFunction, term IRTerminator) (wasmTerm, error) {
	switch t := term.(type) {
	case *irJump:
		return wasmTerm{Kind: "jump", Target: t.Target, Args: append([]string(nil), t.Args...)}, nil
	case *irBranch:
		return wasmTerm{Kind: "branch", Cond: t.Cond, TrueTarget: t.TrueTarget, TrueArgs: append([]string(nil), t.TrueArgs...), FalseTarget: t.FalseTarget, FalseArgs: append([]string(nil), t.FalseArgs...)}, nil
	case *irReturn:
		return wasmTerm{Kind: "return", Value: t.Value}, nil
	case *irUnreachable:
		return wasmTerm{Kind: "unreachable"}, nil
	default:
		return wasmTerm{}, fmt.Errorf("WASM backend: %s: function %s has unknown terminator %T", source, fn.Name, term)
	}
}

func wasmUnsupported(source, fn, message string) error {
	if fn == "" {
		fn = "<unknown>"
	}
	return fmt.Errorf("WASM backend: %s: function %s: unsupported %s", source, fn, message)
}

func wasmInstrUnsupported(source string, fn *IRFunction, block *IRBlock, in IRInstr, message string) error {
	where := source
	if in.Span.start.line > 0 {
		where = fmt.Sprintf("%s:%d:%d", source, in.Span.start.line, in.Span.start.col)
	}
	return fmt.Errorf("WASM backend: %s: function %s block %s op %s: %s", where, fn.Name, block.Name, in.Op, message)
}

func wasmLiteralFromValue(v Value) (*wasmLiteral, error) {
	switch v.kind {
	case kindNull:
		return &wasmLiteral{Kind: "null"}, nil
	case kindBool:
		return &wasmLiteral{Kind: "bool", Bool: v.boo}, nil
	case kindString:
		return &wasmLiteral{Kind: "string", Text: v.str}, nil
	case kindNumber:
		if math.IsNaN(v.num) || math.IsInf(v.num, 0) {
			return nil, fmt.Errorf("non-finite number is not supported")
		}
		return &wasmLiteral{Kind: "number", Number: v.num, Bits: v.fbits, BFloat: v.bfloat}, nil
	case kindUint:
		if v.ub != nil {
			return &wasmLiteral{Kind: "biguint", Int: v.ub.String(), Bits: v.ubits, Packed: v.packed, Native: v.native}, nil
		}
		return &wasmLiteral{Kind: "uint", Uint: v.u, Bits: uint16(v.bits), Packed: v.packed, Native: v.native}, nil
	case kindInt:
		if v.i == nil {
			return &wasmLiteral{Kind: "int", Int: "0", Bits: v.ibits}, nil
		}
		return &wasmLiteral{Kind: "int", Int: v.i.String(), Bits: v.ibits}, nil
	case kindBigInt:
		if v.bi == nil {
			return &wasmLiteral{Kind: "bigint", Int: "0"}, nil
		}
		return &wasmLiteral{Kind: "bigint", Int: v.bi.String()}, nil
	case kindBigFloat:
		if v.bf == nil {
			return &wasmLiteral{Kind: "bigfloat", BigFloat: "0"}, nil
		}
		r, _ := v.bf.Rat(nil)
		return &wasmLiteral{Kind: "bigfloat", BigFloat: r.RatString()}, nil
	case kindDecimal:
		if v.dec == nil {
			return &wasmLiteral{Kind: "decimal", Decimal: "0"}, nil
		}
		return &wasmLiteral{Kind: "decimal", Decimal: v.dec.RatString()}, nil
	case kindFixed:
		if v.fx == nil {
			return &wasmLiteral{Kind: "fixed", FixedWidth: 64}, nil
		}
		return &wasmLiteral{Kind: "fixed", FixedWidth: v.fx.width, FixedFrac: v.fx.frac, FixedRaw: v.fx.raw.String()}, nil
	case kindInterval:
		if v.iv == nil {
			return &wasmLiteral{Kind: "interval", IntervalLo: "0", IntervalHi: "0"}, nil
		}
		return &wasmLiteral{Kind: "interval", IntervalLo: v.iv.lo.RatString(), IntervalHi: v.iv.hi.RatString()}, nil
	case kindModInt:
		if v.mi == nil {
			return &wasmLiteral{Kind: "modint", Modulus: "1", ModValue: "0"}, nil
		}
		return &wasmLiteral{Kind: "modint", Modulus: v.mi.mod.String(), ModValue: v.mi.val.String()}, nil
	case kindComplex:
		return &wasmLiteral{Kind: "complex", ComplexReal: real(v.cx), ComplexImag: imag(v.cx), Bits: v.fbits}, nil
	case kindList, kindTuple, kindTensor:
		literal := &wasmLiteral{Kind: v.kind.String()}
		if v.kind == kindTensor {
			literal.Shape = append([]int(nil), v.shape...)
			for _, item := range v.data {
				child, err := wasmLiteralFromValue(item)
				if err != nil {
					return nil, err
				}
				literal.Children = append(literal.Children, child)
			}
		} else {
			items := v.list
			if v.kind == kindTuple {
				items = v.tuple
			}
			for _, item := range items {
				child, err := wasmLiteralFromValue(item)
				if err != nil {
					return nil, err
				}
				literal.Children = append(literal.Children, child)
			}
		}
		return literal, nil
	case kindEnum:
		if v.enum == nil {
			return &wasmLiteral{Kind: "enum"}, nil
		}
		literal := &wasmLiteral{Kind: "enum", TypeName: v.enum.typeName, CaseName: v.enum.caseName}
		for _, item := range v.enum.payload {
			child, err := wasmLiteralFromValue(item)
			if err != nil {
				return nil, err
			}
			literal.Children = append(literal.Children, child)
		}
		return literal, nil
	case kindResult:
		if v.res == nil {
			return &wasmLiteral{Kind: "result"}, nil
		}
		literal := &wasmLiteral{Kind: "result", ResultOK: v.res.ok}
		if v.res.ok {
			child, err := wasmLiteralFromValue(v.res.val)
			if err != nil {
				return nil, err
			}
			literal.Children = []*wasmLiteral{child}
		} else if v.res.err != nil {
			literal.ErrorKind, literal.ErrorText = string(v.res.err.kind), v.res.err.message
		}
		return literal, nil
	case kindOption:
		literal := &wasmLiteral{Kind: "option"}
		if v.opt != nil {
			literal.Present = v.opt.present
			if v.opt.present {
				child, err := wasmLiteralFromValue(v.opt.val)
				if err != nil {
					return nil, err
				}
				literal.Children = []*wasmLiteral{child}
			}
		}
		return literal, nil
	case kindQuantity:
		if v.qty == nil {
			return &wasmLiteral{Kind: "quantity"}, nil
		}
		return &wasmLiteral{Kind: "quantity", Decimal: v.qty.mag.RatString(), QuantityDims: v.qty.dims}, nil
	case kindMap:
		literal := &wasmLiteral{Kind: "map"}
		if v.mapValue != nil {
			for _, entry := range v.mapValue.entries {
				key, err := wasmLiteralFromValue(entry.key)
				if err != nil {
					return nil, err
				}
				value, err := wasmLiteralFromValue(entry.value)
				if err != nil {
					return nil, err
				}
				literal.MapKeys = append(literal.MapKeys, key)
				literal.Children = append(literal.Children, value)
			}
		}
		return literal, nil
	case kindDuration:
		return &wasmLiteral{Kind: "duration", Duration: v.duration}, nil
	case kindDateTime:
		if v.datetime == nil {
			return &wasmLiteral{Kind: "datetime"}, nil
		}
		location := v.datetime.location
		literal := &wasmLiteral{Kind: "datetime", DateTimeUnix: v.datetime.unixSeconds, DateTimeNano: v.datetime.nanosecond}
		if location != nil {
			literal.LocationName, literal.LocationKind, literal.LocationOffset = location.name, uint8(location.class), location.offset
		}
		return literal, nil
	case kindTimeLocation:
		if v.location == nil {
			return &wasmLiteral{Kind: "location"}, nil
		}
		return &wasmLiteral{Kind: "location", LocationName: v.location.name, LocationKind: uint8(v.location.class), LocationOffset: v.location.offset}, nil
	case kindError:
		if record := v.errorRecord(); record != nil {
			return &wasmLiteral{Kind: "error", ErrorKind: string(record.kind), ErrorText: record.message}, nil
		}
		return &wasmLiteral{Kind: "error", ErrorText: v.errMsg}, nil
	default:
		return nil, fmt.Errorf("value kind %s is not supported", v.Kind())
	}
}

func (l *wasmLiteral) value() (Value, error) {
	if l == nil {
		return nullValue(), fmt.Errorf("missing WASM literal")
	}
	switch l.Kind {
	case "null":
		return nullValue(), nil
	case "bool":
		return boolValue(l.Bool), nil
	case "string":
		return stringValue(l.Text), nil
	case "number":
		if math.IsNaN(l.Number) || math.IsInf(l.Number, 0) {
			return nullValue(), fmt.Errorf("invalid number literal")
		}
		if l.BFloat {
			return bfloatValue(l.Number, l.Bits), nil
		}
		return floatValue(l.Number, l.Bits), nil
	case "uint":
		if l.Bits == 0 || l.Bits > 64 {
			return nullValue(), fmt.Errorf("invalid unsigned width %d", l.Bits)
		}
		if l.Packed {
			return packedUintValue(l.Uint, uint8(l.Bits), l.Native), nil
		}
		return uintValue(l.Uint, uint8(l.Bits)), nil
	case "int":
		if l.Bits == 0 || l.Bits > 64 {
			return nullValue(), fmt.Errorf("invalid signed width %d", l.Bits)
		}
		value, ok := new(big.Int).SetString(l.Int, 10)
		if !ok {
			return nullValue(), fmt.Errorf("invalid signed literal")
		}
		return intValue(value, l.Bits), nil
	case "biguint":
		value, ok := new(big.Int).SetString(l.Int, 10)
		if !ok {
			return nullValue(), fmt.Errorf("invalid unsigned literal")
		}
		if l.Packed {
			return packedWideUintValue(value, l.Bits, l.Native), nil
		}
		return wideUintValue(value, l.Bits), nil
	case "bigint":
		value, ok := new(big.Int).SetString(l.Int, 10)
		if !ok {
			return nullValue(), fmt.Errorf("invalid bigint literal")
		}
		return bigintValue(value), nil
	case "bigfloat":
		value, ok := new(big.Rat).SetString(l.BigFloat)
		if !ok {
			return nullValue(), fmt.Errorf("invalid bigfloat literal")
		}
		return bigfloatValue(new(big.Float).SetRat(value)), nil
	case "decimal":
		value, ok := new(big.Rat).SetString(l.Decimal)
		if !ok {
			return nullValue(), fmt.Errorf("invalid decimal literal")
		}
		return decimalValue(value), nil
	case "fixed":
		raw, ok := new(big.Int).SetString(l.FixedRaw, 10)
		if !ok {
			raw = new(big.Int)
		}
		return fixedValueOfValue(&fixedValue{width: l.FixedWidth, frac: l.FixedFrac, raw: raw}), nil
	case "interval":
		lo, ok := new(big.Rat).SetString(l.IntervalLo)
		if !ok {
			return nullValue(), fmt.Errorf("invalid interval lower bound")
		}
		hi, ok := new(big.Rat).SetString(l.IntervalHi)
		if !ok {
			return nullValue(), fmt.Errorf("invalid interval upper bound")
		}
		return intervalValueOf(newIntervalValue(lo, hi)), nil
	case "modint":
		mod, ok := new(big.Int).SetString(l.Modulus, 10)
		if !ok {
			return nullValue(), fmt.Errorf("invalid modulus")
		}
		val, ok := new(big.Int).SetString(l.ModValue, 10)
		if !ok {
			return nullValue(), fmt.Errorf("invalid modular value")
		}
		return modIntValueOfValue(&modIntValue{mod: mod, val: val}), nil
	case "complex":
		return complexValueStyled(complex(l.ComplexReal, l.ComplexImag), l.Bits), nil
	case "list", "tuple", "tensor":
		items := make([]Value, len(l.Children))
		for i, child := range l.Children {
			value, err := child.value()
			if err != nil {
				return nullValue(), err
			}
			items[i] = value
		}
		if l.Kind == "tensor" {
			return tensorValue(l.Shape, items), nil
		}
		if l.Kind == "tuple" {
			return tupleValue(items), nil
		}
		return listValue(items), nil
	case "enum":
		items := make([]Value, len(l.Children))
		for i, child := range l.Children {
			value, err := child.value()
			if err != nil {
				return nullValue(), err
			}
			items[i] = value
		}
		return enumCaseValue(l.TypeName, l.CaseName, items...), nil
	case "result":
		if l.ResultOK && len(l.Children) == 1 {
			value, err := l.Children[0].value()
			if err != nil {
				return nullValue(), err
			}
			return resultOkValue(value), nil
		}
		kind := ErrorKind(l.ErrorKind)
		if kind == "" {
			kind = ErrorKindUnknown
		}
		return resultErrValue(newOsirisError(kind, l.ErrorText)), nil
	case "option":
		if l.Present && len(l.Children) == 1 {
			value, err := l.Children[0].value()
			if err != nil {
				return nullValue(), err
			}
			return optionSomeValue(value), nil
		}
		return optionNoneValue(), nil
	case "quantity":
		ratio, ok := new(big.Rat).SetString(l.Decimal)
		if !ok {
			return nullValue(), fmt.Errorf("invalid quantity magnitude")
		}
		return quantityValueOf(ratio, l.QuantityDims), nil
	case "map":
		if len(l.MapKeys) != len(l.Children) {
			return nullValue(), fmt.Errorf("invalid map literal")
		}
		entries := make([]mapEntry, len(l.MapKeys))
		for i := range l.MapKeys {
			key, err := l.MapKeys[i].value()
			if err != nil {
				return nullValue(), err
			}
			value, err := l.Children[i].value()
			if err != nil {
				return nullValue(), err
			}
			entries[i] = mapEntry{key: key, value: value}
		}
		return mapValueOf(entries), nil
	case "duration":
		return durationValue(l.Duration), nil
	case "datetime":
		location := wasmLocationValue(l.LocationKind, l.LocationName, l.LocationOffset)
		return dateTimeValueOf(&dateTimeValue{unixSeconds: l.DateTimeUnix, nanosecond: l.DateTimeNano, location: location}), nil
	case "location":
		location := wasmLocationValue(l.LocationKind, l.LocationName, l.LocationOffset)
		return timeLocationValueOf(location), nil
	case "error":
		kind := ErrorKind(l.ErrorKind)
		if kind == "" {
			kind = ErrorKindUnknown
		}
		return errorValueFromRecord(newOsirisError(kind, l.ErrorText)), nil
	default:
		return nullValue(), fmt.Errorf("unknown WASM literal kind %q", l.Kind)
	}
}

func wasmLocationValue(kind uint8, name string, offset int) *timeLocationValue {
	location := &timeLocationValue{class: timeLocationClass(kind), name: name, offset: offset}
	switch location.class {
	case timeLocationUTC:
		return utcTimeLocation()
	case timeLocationFixed:
		location.loc = time.FixedZone(name, offset)
	case timeLocationIANA:
		location.loc, _ = time.LoadLocation(name)
		if location.loc == nil {
			location.loc = time.UTC
		}
	default:
		return utcTimeLocation()
	}
	return location
}

func marshalWASMPayload(program *wasmProgram) ([]byte, error) {
	var out []byte
	out = append(out, wasmPayloadMagic...)
	out = append(out, byte(WASMABIVersion))
	payload, err := json.Marshal(program)
	if err != nil {
		return nil, err
	}
	var length [4]byte
	binary.LittleEndian.PutUint32(length[:], uint32(len(payload)))
	out = append(out, length[:]...)
	out = append(out, payload...)
	return out, nil
}

func unmarshalWASMPayload(payload []byte) (*wasmProgram, error) {
	if len(payload) < len(wasmPayloadMagic)+1+4 || string(payload[:4]) != wasmPayloadMagic {
		return nil, fmt.Errorf("WASM backend: malformed ABI payload")
	}
	if payload[4] != byte(WASMABIVersion) {
		return nil, fmt.Errorf("WASM backend: unsupported ABI version %d", payload[4])
	}
	length := int(binary.LittleEndian.Uint32(payload[5:9]))
	if length < 0 || length != len(payload)-9 {
		return nil, fmt.Errorf("WASM backend: malformed ABI payload length")
	}
	var program wasmProgram
	if err := json.Unmarshal(payload[9:], &program); err != nil {
		return nil, fmt.Errorf("WASM backend: malformed ABI JSON: %w", err)
	}
	if program.ABI.Version != WASMABIVersion || !program.ABI.Deterministic {
		return nil, fmt.Errorf("WASM backend: incompatible ABI metadata")
	}
	return &program, nil
}

func wasmToken(op string) tokenKind {
	for k := tokenEOF; k <= tokenOrOr; k++ {
		if k.String() == op {
			return k
		}
	}
	return tokenEOF
}

func wasmModuleAllowed(name string) bool {
	switch name {
	case "std/math", "std/list", "std/text", "std/signal", "std/crypto":
		return true
	default:
		return false
	}
}

func firstIRArg(in IRInstr) string {
	if len(in.Args) == 0 {
		return ""
	}
	return in.Args[0]
}

func wasmBuiltinAllowed(name string) bool {
	switch name {
	case "read", "readbytes", "write", "openread", "openwrite", "readline", "readchunk", "writechunk", "transfer", "flush", "close",
		"tcp_listen", "tcp_accept", "tcp_dial", "udp_bind", "udp_connect", "udp_send", "udp_receive", "dns_lookup", "tls_dial", "tls_dial_config",
		"http_get", "http_request", "http_request_stream", "http_serve", "ffi_load", "ffi_call", "ffi_const", "ffi_close",
		"now", "sleep", "spawn", "await", "cancel", "parallel", "timeout", "race", "select":
		return false
	default:
		return true
	}
}

func wasmFunctionByName(program *wasmProgram, name string) *wasmFunction {
	if program == nil {
		return nil
	}
	for i := range program.Functions {
		if program.Functions[i].Name == name {
			return &program.Functions[i]
		}
	}
	return nil
}

func cloneWASMValue(value Value) Value {
	// Callable and module values are execution-local references. They never
	// cross the returned-value ABI, so preserve the reference while values move
	// between SSA blocks or function frames.
	if value.kind == kindFunction || value.kind == kindModule || value.kind == kindStream {
		return value
	}
	// ABI v1 values are scalar and immutable. Reconstructing through the
	// literal form gives the runner an explicit ownership boundary.
	literal, err := wasmLiteralFromValue(value)
	if err != nil {
		return nullValue()
	}
	copy, err := literal.value()
	if err != nil {
		return nullValue()
	}
	return copy
}
