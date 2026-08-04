package osiris

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func validateWASMArtifact(artifact []byte, limits WASMExecutionLimits) error {
	limits = normalizeWASMLimits(limits)
	if uint64(len(artifact)) > limits.MaxArtifactBytes {
		return wasmLimitError("artifact", limits.MaxArtifactBytes)
	}
	if len(artifact) < 8 || !bytes.Equal(artifact[:8], []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}) {
		return fmt.Errorf("malformed WASM header")
	}
	offset := 8
	lastStandard := byte(0)
	seenCustom := false
	var customPayload []byte
	for offset < len(artifact) {
		id := artifact[offset]
		offset++
		length, next, err := readWASMULEB(artifact, offset)
		if err != nil {
			return fmt.Errorf("malformed section length: %w", err)
		}
		offset = next
		if length > uint64(len(artifact)-offset) {
			return fmt.Errorf("section extends beyond artifact")
		}
		end := offset + int(length)
		section := artifact[offset:end]
		if id != 0 {
			if id > 11 || id == 2 || id == 4 || id == 8 || id == 9 {
				return fmt.Errorf("unsupported or malformed section id %d", id)
			}
			if id < lastStandard {
				return fmt.Errorf("standard sections out of order")
			}
			lastStandard = id
		}
		if id == 0 {
			name, payload, err := readWASMCustomSection(section)
			if err != nil {
				return err
			}
			if name == wasmCustomSection {
				if seenCustom {
					return fmt.Errorf("duplicate Osiris ABI section")
				}
				seenCustom = true
				customPayload = payload
			}
		}
		offset = end
	}
	if !seenCustom {
		return fmt.Errorf("missing Osiris ABI section")
	}
	if _, err := unmarshalWASMPayload(customPayload); err != nil {
		return err
	}
	return nil
}

func parseWASMArtifact(artifact []byte, limits WASMExecutionLimits) (*wasmProgram, error) {
	if err := validateWASMArtifact(artifact, limits); err != nil {
		return nil, fmt.Errorf("WASM backend: artifact validation failed: %w", err)
	}
	offset := 8
	for offset < len(artifact) {
		id := artifact[offset]
		offset++
		length, next, _ := readWASMULEB(artifact, offset)
		offset = next
		end := offset + int(length)
		if id == 0 {
			name, payload, _ := readWASMCustomSection(artifact[offset:end])
			if name == wasmCustomSection {
				program, err := unmarshalWASMPayload(payload)
				if err != nil {
					return nil, err
				}
				if err := validateWASMProgramProfile(program); err != nil {
					return nil, err
				}
				return program, nil
			}
		}
		offset = end
	}
	return nil, fmt.Errorf("WASM backend: missing Osiris ABI payload")
}

func validateWASMProgramProfile(program *wasmProgram) error {
	if program == nil || program.ABI.Profile == "" || !program.ABI.Deterministic {
		return fmt.Errorf("WASM backend: incompatible profile metadata")
	}
	profile := normalizeWASMProfile(WASMCapabilityProfile{
		Name:         program.ABI.Profile,
		Modules:      program.ABI.Modules,
		Capabilities: program.ABI.Capabilities,
	})
	for _, fn := range program.Functions {
		moduleValues := make(map[string]string)
		for _, block := range fn.Blocks {
			for _, in := range block.Instrs {
				switch in.Op {
				case "import":
					module := strings.Trim(in.Text, "\"")
					if !wasmProfileHasModule(profile, module) {
						return fmt.Errorf("WASM backend: artifact imports module %q outside profile", module)
					}
					moduleValues[in.Dest] = module
				case "member":
					if module := moduleValues[firstWASMArg(in)]; module != "" && !wasmProfileAllowsMember(profile, module, in.Text) {
						return fmt.Errorf("WASM backend: artifact uses %s.%s outside profile", module, in.Text)
					}
				}
			}
		}
	}
	return nil
}

func readWASMCustomSection(section []byte) (string, []byte, error) {
	nameLen, offset, err := readWASMULEB(section, 0)
	if err != nil || nameLen > uint64(len(section)-offset) {
		return "", nil, fmt.Errorf("malformed custom section name")
	}
	name := string(section[offset : offset+int(nameLen)])
	return name, section[offset+int(nameLen):], nil
}

func readWASMULEB(data []byte, offset int) (uint64, int, error) {
	var value uint64
	for shift := uint(0); ; shift += 7 {
		if shift >= 64 || offset >= len(data) {
			return 0, offset, fmt.Errorf("invalid unsigned LEB128")
		}
		part := data[offset]
		offset++
		if shift == 63 && part > 1 {
			return 0, offset, fmt.Errorf("unsigned LEB128 overflow")
		}
		value |= uint64(part&0x7f) << shift
		if part&0x80 == 0 {
			return value, offset, nil
		}
	}
}

type wasmExecutionState struct {
	ctx          context.Context
	limits       WASMExecutionLimits
	rt           *Runtime
	program      *wasmProgram
	instructions uint64
	memoryBytes  uint64
	outputs      []Value
}

type wasmExecutionFrame struct {
	values   map[string]Value
	bindings map[string]string
}

func executeWASMProgram(ctx context.Context, program *wasmProgram, limits WASMExecutionLimits) (WASMExecution, error) {
	if err := ctx.Err(); err != nil {
		return WASMExecution{}, err
	}
	if program == nil || len(program.Functions) == 0 {
		return WASMExecution{}, fmt.Errorf("WASM backend: empty executable payload")
	}
	rt := NewTestRuntime()
	for _, capability := range program.ABI.Capabilities {
		rt.GrantCapability(capability)
	}
	if wasmProfileHasModule(WASMCapabilityProfile{Modules: program.ABI.Modules}, "std/testing") {
		rt.languageTest = newLanguageTestState(program.SourceName)
	}
	state := &wasmExecutionState{ctx: ctx, limits: limits, rt: rt, program: program}
	if _, err := executeWASMFunction(state, &program.Functions[0], nil, nil); err != nil {
		return WASMExecution{}, fmt.Errorf("WASM backend: %s: %w", program.SourceName, err)
	}
	jsonBytes, err := json.Marshal(state.outputs)
	if err != nil {
		return WASMExecution{}, fmt.Errorf("WASM backend: encode results: %w", err)
	}
	if uint64(len(jsonBytes)) > limits.MaxMemoryBytes {
		return WASMExecution{}, wasmLimitError("memory", limits.MaxMemoryBytes)
	}
	owned := make([]Value, len(state.outputs))
	for i := range state.outputs {
		owned[i] = cloneWASMValue(state.outputs[i])
	}
	return WASMExecution{Outputs: owned, JSON: string(jsonBytes), Instructions: state.instructions, MemoryBytes: uint64(len(jsonBytes))}, nil
}

func executeWASMFunction(state *wasmExecutionState, fn *wasmFunction, args []Value, captures map[string]Value) (Value, error) {
	if fn == nil || len(fn.Blocks) == 0 {
		return nullValue(), fmt.Errorf("empty function")
	}
	blocks := make(map[string]*wasmBlock, len(fn.Blocks))
	for i := range fn.Blocks {
		if _, exists := blocks[fn.Blocks[i].Name]; exists {
			return nullValue(), fmt.Errorf("duplicate block %q", fn.Blocks[i].Name)
		}
		blocks[fn.Blocks[i].Name] = &fn.Blocks[i]
	}
	frame := &wasmExecutionFrame{values: make(map[string]Value), bindings: make(map[string]string)}
	if len(args) != len(fn.ParamValues) {
		return nullValue(), fmt.Errorf("function %s expects %d arguments, got %d", fn.Name, len(fn.ParamValues), len(args))
	}
	for i, name := range fn.ParamValues {
		frame.values[name] = cloneWASMValue(args[i])
	}
	for i, name := range fn.CaptureValues {
		if i >= len(fn.Captures) {
			return nullValue(), fmt.Errorf("function %s has an invalid capture layout", fn.Name)
		}
		value, ok := captures[fn.Captures[i]]
		if !ok {
			return nullValue(), fmt.Errorf("function %s is missing capture %q", fn.Name, fn.Captures[i])
		}
		frame.values[name] = cloneWASMValue(value)
	}
	current := fn.Blocks[0].Name
	blockArgs := []Value(nil)
	for {
		if err := state.step(); err != nil {
			return nullValue(), err
		}
		block, ok := blocks[current]
		if !ok {
			return nullValue(), fmt.Errorf("jump to unknown block %q", current)
		}
		if len(block.Params) != len(blockArgs) {
			return nullValue(), fmt.Errorf("block %s expects %d parameters, got %d", block.Name, len(block.Params), len(blockArgs))
		}
		for i, name := range block.Params {
			frame.values[name] = cloneWASMValue(blockArgs[i])
		}
		blockArgs = nil
		for _, in := range block.Instrs {
			if err := state.step(); err != nil {
				return nullValue(), err
			}
			if err := executeWASMInstr(state, frame, in); err != nil {
				return nullValue(), err
			}
		}
		var err error
		switch block.Term.Kind {
		case "jump":
			blockArgs, err = wasmArgs(frame.values, block.Term.Args)
			if err != nil {
				return nullValue(), err
			}
			current = block.Term.Target
		case "branch":
			condition, ok := frame.values[block.Term.Cond]
			if !ok {
				return nullValue(), fmt.Errorf("missing branch value %q", block.Term.Cond)
			}
			if condition.Truthy() {
				blockArgs, err = wasmArgs(frame.values, block.Term.TrueArgs)
				current = block.Term.TrueTarget
			} else {
				blockArgs, err = wasmArgs(frame.values, block.Term.FalseArgs)
				current = block.Term.FalseTarget
			}
			if err != nil {
				return nullValue(), err
			}
		case "return":
			if block.Term.Value == "" {
				return nullValue(), nil
			}
			value, ok := frame.values[block.Term.Value]
			if !ok {
				return nullValue(), fmt.Errorf("missing return value %q", block.Term.Value)
			}
			return cloneWASMValue(value), nil
		case "unreachable":
			return nullValue(), fmt.Errorf("unreachable instruction")
		default:
			return nullValue(), fmt.Errorf("unknown terminator %q", block.Term.Kind)
		}
	}
}

func (s *wasmExecutionState) step() error {
	s.instructions++
	if s.instructions > s.limits.MaxInstructions {
		return wasmLimitError("instructions", s.limits.MaxInstructions)
	}
	if err := s.ctx.Err(); err != nil {
		return err
	}
	return nil
}

func executeWASMInstr(state *wasmExecutionState, frame *wasmExecutionFrame, in wasmInstr) error {
	values := frame.values
	lookup := func(name string) (Value, error) {
		value, ok := values[name]
		if !ok {
			return nullValue(), fmt.Errorf("missing value %q", name)
		}
		return value, nil
	}
	switch in.Op {
	case "const", "global":
		value, err := in.Value.value()
		if err != nil {
			return err
		}
		values[in.Dest] = value
	case "copy":
		value, err := lookup(firstWASMArg(in))
		if err != nil {
			return err
		}
		values[in.Dest] = cloneWASMValue(value)
		if in.Text != "" {
			frame.bindings[in.Text] = in.Dest
		}
	case "assign":
		value, err := lookup(firstWASMArg(in))
		if err != nil {
			return err
		}
		values[in.Dest] = cloneWASMValue(value)
		frame.bindings[in.Text] = in.Dest
	case "list", "tuple":
		items := make([]Value, len(in.Args))
		for i, name := range in.Args {
			value, err := lookup(name)
			if err != nil {
				return err
			}
			items[i] = cloneWASMValue(value)
		}
		if in.Op == "list" {
			values[in.Dest] = listValue(items)
		} else {
			values[in.Dest] = tupleValue(items)
		}
	case "enum":
		items := make([]Value, len(in.Args))
		for i, name := range in.Args {
			value, err := lookup(name)
			if err != nil {
				return err
			}
			items[i] = value
		}
		parts := strings.SplitN(in.Text, ".", 2)
		if len(parts) != 2 {
			return fmt.Errorf("malformed enum constructor %q", in.Text)
		}
		values[in.Dest] = enumCaseValue(parts[0], parts[1], items...)
	case "resultbind":
		value, err := lookup(firstWASMArg(in))
		if err != nil {
			return err
		}
		unwrapped, err := unwrapBinding(value)
		if err != nil {
			return err
		}
		values[in.Dest] = unwrapped
	case "builtin":
		fn, ok := state.rt.Builtins[in.Text]
		if !ok {
			return fmt.Errorf("WASM profile does not expose builtin %q", in.Text)
		}
		values[in.Dest] = functionValueOf(in.Text, fn)
	case "import":
		module := strings.Trim(in.Text, "\"")
		value, err := stdlibModule(state.rt, module, "")
		if err != nil {
			return err
		}
		values[in.Dest] = value
	case "member":
		object, err := lookup(firstWASMArg(in))
		if err != nil {
			return err
		}
		value, err := resolveMember(state.ctx, object, in.Text, state.rt)
		if err != nil {
			return err
		}
		values[in.Dest] = value
	case "call":
		callee, err := lookup(in.Text)
		if err != nil {
			return err
		}
		args := make([]Value, len(in.Args))
		for i, name := range in.Args {
			args[i], err = lookup(name)
			if err != nil {
				return err
			}
		}
		if callee.kind == kindStruct && callee.structType != nil {
			value, err := constructStruct(callee.structType, args)
			if err != nil {
				return err
			}
			values[in.Dest] = value
			return nil
		}
		if callee.kind != kindFunction || callee.fn == nil {
			return fmt.Errorf("value %s is not callable in WASM", callee.Kind())
		}
		var value Value
		if callee.fn.builtin != nil {
			value, err = callee.fn.builtin(args)
		} else if callee.fn.contextBuiltin != nil {
			value, err = callee.fn.contextBuiltin(state.ctx, args)
		} else {
			return fmt.Errorf("function %s has no executable WASM body", callee.fn.name)
		}
		if err != nil {
			return err
		}
		values[in.Dest] = value
	case "makefunc":
		fn := wasmFunctionByName(state.program, in.Text)
		if fn == nil {
			return fmt.Errorf("unknown WASM function %q", in.Text)
		}
		captured := make(map[string]Value, len(fn.Captures))
		for i, name := range fn.Captures {
			if i >= len(in.Args) {
				return fmt.Errorf("function %s missing capture %q", in.Text, name)
			}
			value, err := lookup(in.Args[i])
			if err != nil {
				return err
			}
			captured[name] = cloneWASMValue(value)
		}
		values[in.Dest] = functionValueOf(in.Text, func(args []Value) (Value, error) { return executeWASMFunction(state, fn, args, captured) })
	case "matchenum":
		value, err := lookup(firstWASMArg(in))
		if err != nil {
			return err
		}
		parts := strings.SplitN(in.Text, ".", 2)
		values[in.Dest] = boolValue(value.kind == kindEnum && value.enum != nil && len(parts) == 2 && value.enum.typeName == parts[0] && value.enum.caseName == parts[1])
	case "enumpayload":
		value, err := lookup(firstWASMArg(in))
		if err != nil {
			return err
		}
		if value.kind != kindEnum || value.enum == nil {
			return fmt.Errorf("enum payload requires an enum")
		}
		index, err := wasmIndex(in.Value)
		if err != nil {
			return err
		}
		if index < 0 || index >= len(value.enum.payload) {
			return fmt.Errorf("enum payload index %d out of range", index)
		}
		values[in.Dest] = cloneWASMValue(value.enum.payload[index])
	case "matchlist":
		value, err := lookup(firstWASMArg(in))
		if err != nil {
			return err
		}
		items, err := value.AsList()
		if err != nil {
			return err
		}
		index, err := wasmIndex(in.Value)
		if err != nil {
			return err
		}
		values[in.Dest] = boolValue(len(items) == index)
	case "listitem":
		value, err := lookup(firstWASMArg(in))
		if err != nil {
			return err
		}
		index, err := wasmIndex(in.Value)
		if err != nil {
			return err
		}
		items, err := value.AsList()
		if err != nil {
			return err
		}
		if index < 0 || index >= len(items) {
			return fmt.Errorf("list item index %d out of range", index)
		}
		values[in.Dest] = cloneWASMValue(items[index])
	case "matchtype":
		value, err := lookup(firstWASMArg(in))
		if err != nil {
			return err
		}
		values[in.Dest] = boolValue(bindingTypeMatches(in.Text, value))
	case "matchfail":
		return fmt.Errorf("match failed: %s", in.Text)
	case "cleanup":
		cleanup, err := lookup(firstWASMArg(in))
		if err != nil {
			return err
		}
		if cleanup.kind != kindFunction || cleanup.fn == nil || cleanup.fn.builtin == nil {
			return fmt.Errorf("cleanup is not callable")
		}
		_, err = cleanup.fn.builtin(nil)
		return err
	case "asserttype":
		value, err := lookup(firstWASMArg(in))
		if err != nil {
			return err
		}
		if !bindingTypeMatches(in.Text, value) {
			return fmt.Errorf("type assertion failed: expected %s, got %s", in.Text, value.Kind())
		}
		values[in.Dest] = cloneWASMValue(value)
	case "unary":
		value, err := lookup(firstWASMArg(in))
		if err != nil {
			return err
		}
		result, err := applyUnary(wasmToken(in.Text), value)
		if err != nil {
			return err
		}
		values[in.Dest] = result
	case "binary":
		if len(in.Args) != 2 {
			return fmt.Errorf("binary operation expects two operands")
		}
		left, err := lookup(in.Args[0])
		if err != nil {
			return err
		}
		right, err := lookup(in.Args[1])
		if err != nil {
			return err
		}
		result, err := vmBinary(state.ctx, wasmToken(in.Text), left, right)
		if err != nil {
			return err
		}
		values[in.Dest] = result
	case "emit":
		value, err := lookup(firstWASMArg(in))
		if err != nil {
			return err
		}
		if uint32(len(state.outputs)) >= state.limits.MaxOutputs {
			return wasmLimitError("outputs", uint64(state.limits.MaxOutputs))
		}
		candidate := cloneWASMValue(value)
		if _, err := wasmLiteralFromValue(candidate); err != nil {
			return fmt.Errorf("result is outside WASM ABI: %w", err)
		}
		encoded, err := json.Marshal(candidate)
		if err != nil {
			return fmt.Errorf("encode output: %w", err)
		}
		state.memoryBytes += uint64(len(encoded))
		if state.memoryBytes > state.limits.MaxMemoryBytes {
			return wasmLimitError("memory", state.limits.MaxMemoryBytes)
		}
		state.outputs = append(state.outputs, candidate)
	default:
		return fmt.Errorf("unsupported runtime operation %q", in.Op)
	}
	return nil
}

func firstWASMArg(in wasmInstr) string {
	if len(in.Args) == 0 {
		return ""
	}
	return in.Args[0]
}

func wasmIndex(literal *wasmLiteral) (int, error) {
	if literal == nil {
		return 0, fmt.Errorf("missing WASM index literal")
	}
	value, err := literal.value()
	if err != nil {
		return 0, err
	}
	index, err := value.Int()
	if err != nil {
		return 0, fmt.Errorf("WASM index must be an integer: %w", err)
	}
	return index, nil
}

func wasmArgs(values map[string]Value, names []string) ([]Value, error) {
	args := make([]Value, len(names))
	for i, name := range names {
		value, ok := values[name]
		if !ok {
			return nil, fmt.Errorf("WASM backend: missing block argument %q", name)
		}
		args[i] = value
	}
	return args, nil
}

func evaluateWASMConstantPreview(program *wasmProgram) ([]Value, bool) {
	if program == nil || len(program.Functions) != 1 {
		return nil, false
	}
	for _, block := range program.Functions[0].Blocks {
		if block.Term.Kind != "return" {
			return nil, false
		}
		for _, in := range block.Instrs {
			switch in.Op {
			case "const", "global", "copy", "asserttype", "emit", "unary", "binary":
			default:
				return nil, false
			}
		}
	}
	previewLimits := program.Limits
	previewLimits.MaxInstructions = 10_000
	previewLimits.MaxOutputs = 10_000
	previewLimits.MaxMemoryBytes = 1 << 20
	result, err := executeWASMProgram(context.Background(), program, previewLimits)
	if err != nil {
		return nil, false
	}
	return result.Outputs, true
}
