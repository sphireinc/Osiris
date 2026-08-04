package osiris

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
)

func writeCValueArray(b *strings.Builder, args []string, vars map[string]string) {
	if len(args) == 0 {
		b.WriteString("NULL")
		return
	}
	b.WriteString("(OValue[]){")
	for i, arg := range args {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(vars[arg])
	}
	b.WriteByte('}')
}

func cFFIConstantLiteral(constant FFIConstant) (string, error) {
	value, err := parseFFIConstant(constant.Type, constant.Value)
	if err != nil {
		return "", fmt.Errorf("native C backend: ffi constant %q: %w", constant.Name, err)
	}
	return cLiteral(value)
}

func cFFIIntegerResultSpec(result string) (kind, bits int, err error) {
	switch result {
	case "void":
		return 0, 0, nil
	case "bool":
		return 1, 1, nil
	case "u8", "u16", "u32", "u64":
		return 2, int(uintWidth(result)), nil
	case "i8", "i16", "i32", "i64":
		return 3, int(intWidth(result)), nil
	default:
		return 0, 0, fmt.Errorf("native C backend: unsupported ffi integer result type %q", result)
	}
}

// emitCIR emits the initial executable subset of the native backend. It uses a
// small tagged runtime so control flow can be lowered before scalar
// specialization replaces values in hot paths.
func emitCIR(prog *IRProgram) (string, error) { return emitCIRWithOptions(prog, CCompileOptions{}) }

func emitCIRWithOptions(prog *IRProgram, options CCompileOptions) (string, error) {
	return emitCIRWithOptionsAndFFI(prog, options, &nativeFFIPlan{calls: make(map[string]FFIFunction), constants: make(map[string]FFIConstant), loads: make(map[string]string)})
}

func emitCIRWithOptionsAndFFI(prog *IRProgram, options CCompileOptions, ffiPlan *nativeFFIPlan) (string, error) {
	if prog == nil || len(prog.Functions) == 0 || prog.Functions[0].Name != "main" {
		return "", fmt.Errorf("native C backend requires a top-level main function")
	}
	for _, fn := range prog.Functions {
		if fn == nil || fn.ReceiverType == "" {
			continue
		}
		if !nativeWitnessType(fn.ReceiverType) {
			return "", fmt.Errorf("native C backend: receiver type %s has no tagged witness representation", fn.ReceiverType)
		}
		if len(fn.Captures) != 0 {
			return "", fmt.Errorf("native C backend: receiver method %s captures lexical state; witness ownership requires an explicit environment ABI", fn.SourceName)
		}
	}
	functions := make(map[string]*IRFunction)
	functionNames := make(map[string]string)
	for index, candidate := range prog.Functions[1:] {
		functions[candidate.Name] = candidate
		functionNames[candidate.Name] = fmt.Sprintf("osiris_fn_%d", index)
	}
	for _, fn := range prog.Functions {
		for _, block := range fn.Blocks {
			for _, instruction := range block.Instrs {
				if instruction.Op == "makefunc" {
					if target := functions[instruction.Text]; target != nil {
						functions[instruction.Dest] = target
					}
				}
			}
		}
	}
	moduleIdentities := nativeModuleIdentitiesByFunction(prog)
	complexValues := nativeKnownComplexProgramValues(prog, moduleIdentities)
	var b strings.Builder
	if options.LinalgProviderSource != "" {
		b.WriteString("#define OSIRIS_LINALG_PROVIDER 1\n")
	}
	if options.PublicEmitSpacing {
		b.WriteString("#define OSIRIS_PUBLIC_EMIT_SPACING 1\n")
	}
	for _, capability := range normalizedNativeCapabilities(options.NativeCapabilities) {
		fmt.Fprintf(&b, "#define OSIRIS_CAP_%s 1\n", nativeCapabilityMacro(capability))
	}
	if nativeTLSProviderRequested(options.NativeCapabilities) {
		b.WriteString("#define OSIRIS_OPENSSL_PROVIDER 1\n")
	}
	if ffiPlan == nil {
		ffiPlan = &nativeFFIPlan{calls: make(map[string]FFIFunction), constants: make(map[string]FFIConstant), loads: make(map[string]string)}
	}
	b.WriteString(cRuntimePrelude())
	b.WriteString(cRuntimeWindowsComponent)
	b.WriteString(cRuntimeNetworkComponent)
	b.WriteString(nativeHostResourceRuntimeComponent)
	b.WriteString(cRuntimeTLSHTTPComponent)
	b.WriteString(cRuntimeOpenSSLProviderComponent)
	b.WriteString(cRuntimeNativeHTTPBridgeComponent)
	for _, fn := range prog.Functions[1:] {
		fmt.Fprintf(&b, "\nstatic OValue %s(", functionNames[fn.Name])
		for i := range fn.Params {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "OValue p%d", i)
		}
		for i := range fn.Captures {
			if len(fn.Params) > 0 || i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "OValue c%d", i)
		}
		b.WriteString(");\n")
		fmt.Fprintf(&b, "static OValue %s_closure(const OValue *args, size_t argc, const OValue *captures, size_t capture_count);\n", functionNames[fn.Name])
	}
	b.WriteString("\n// Osiris native C lowering from ")
	b.WriteString(strings.ReplaceAll(strings.TrimSpace(prog.SourceName), "\n", " "))
	b.WriteByte('\n')
	for _, fn := range prog.Functions[1:] {
		if len(fn.TypeParams) == 0 {
			continue
		}
		fmt.Fprintf(&b, "// generic %s<%s> uses the shared tagged ABI\n", fn.SourceName, strings.Join(fn.TypeParams, ", "))
	}
	for _, fn := range prog.Functions[1:] {
		if err := emitCFunction(&b, fn, false, functions, functionNames, moduleIdentities[fn.Name], complexValues, options.LinalgProviderSource != "", prog.SourceName, ffiPlan); err != nil {
			return "", err
		}
	}
	for _, fn := range prog.Functions[1:] {
		emitCClosureWrapper(&b, fn, functionNames[fn.Name])
	}
	for _, fn := range prog.Functions[1:] {
		if fn == nil || fn.ReceiverType == "" {
			continue
		}
		wrapper := functionNames[fn.Name] + "_witness"
		fmt.Fprintf(&b, "static OValue %s(const OValue *args, size_t argc, const OValue *captures, size_t capture_count) { if(capture_count!=1 || argc!=%d) return ofail(\"invalid method witness call\"); OValue all[%d]; all[0]=captures[0]; if(argc) memcpy(all+1,args,argc*sizeof(OValue)); return %s(all[0]%s); }\n", wrapper, len(fn.Params)-1, len(fn.Params), functionNames[fn.Name], nativeWitnessCallArgs(len(fn.Params)-1))
	}
	b.WriteString("int main(void) {\n")
	for _, fn := range prog.Functions[1:] {
		if fn == nil || fn.ReceiverType == "" {
			continue
		}
		fmt.Fprintf(&b, "  oregistermethod(\"%s\", \"%s\", %s_witness, \"%s\");\n", cEscapeString(fn.ReceiverType), cEscapeString(nativeWitnessMethodName(fn.SourceName)), functionNames[fn.Name], cEscapeString(irFunctionSignature(fn)))
	}
	if specialized, err := emitCScalarMain(&b, prog.Functions[0], functions); err != nil {
		return "", err
	} else if !specialized {
		fmt.Fprintf(&b, "  // Osiris native fallback: %s\n", nativeScalarFallbackReason(prog.Functions[0]))
		if err := emitCFunctionBody(&b, prog.Functions[0], true, functions, functionNames, moduleIdentities[prog.Functions[0].Name], complexValues, options.LinalgProviderSource != "", prog.SourceName, ffiPlan); err != nil {
			return "", err
		}
	}
	b.WriteString("}\n")
	return b.String(), nil
}

func nativeTLSProviderRequested(capabilities []string) bool {
	for _, capability := range normalizedNativeCapabilities(capabilities) {
		if strings.HasPrefix(capability, "net.tls") {
			return true
		}
	}
	return false
}

func nativeWitnessType(typeName string) bool {
	switch typeName {
	case "bool", "string", "number", "int", "uint", "i8", "i16", "i32", "i64", "u8", "u16", "u32", "u64", "f32", "f64":
		return true
	default:
		return false
	}
}

func nativeWitnessCallArgs(count int) string {
	if count == 0 {
		return ""
	}
	args := make([]string, count)
	for i := range args {
		args[i] = fmt.Sprintf(", all[%d]", i+1)
	}
	return strings.Join(args, "")
}

func nativeWitnessMethodName(name string) string {
	if _, method, ok := strings.Cut(name, "."); ok {
		return method
	}
	return name
}

func nativeScalarFallbackReason(fn *IRFunction) string {
	if fn == nil {
		return "missing main function"
	}
	for _, block := range fn.Blocks {
		for _, in := range block.Instrs {
			switch in.Op {
			case "list":
				return "list values require the tagged ABI"
			case "enum":
				return "enum values require the tagged ABI"
			case "makefunc", "call":
				return "function or builtin call requires the tagged ABI"
			case "builtin", "import", "member":
				return "dynamic lookup requires the tagged ABI"
			case "global":
				if in.Value.kind == kindQuantity {
					return "named quantities require an exact rational quantity ABI"
				}
				return "named global value requires the tagged ABI"
			case "const", "copy", "unary", "binary", "emit":
			default:
				return fmt.Sprintf("IR operation %q requires the tagged ABI", in.Op)
			}
		}
	}
	return "dynamic scalar types require the tagged ABI"
}

func emitCFunction(b *strings.Builder, fn *IRFunction, main bool, functions map[string]*IRFunction, functionNames map[string]string, moduleSeeds map[string]string, complexValues map[string]bool, hasProvider bool, programSource string, ffiPlan *nativeFFIPlan) error {
	fmt.Fprintf(b, "\nstatic OValue %s(", functionNames[fn.Name])
	for i := range fn.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(b, "OValue p%d", i)
	}
	for i := range fn.Captures {
		if len(fn.Params) > 0 || i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(b, "OValue c%d", i)
	}
	b.WriteString(") {\n")
	if err := emitCFunctionBody(b, fn, main, functions, functionNames, moduleSeeds, complexValues, hasProvider, programSource, ffiPlan); err != nil {
		return err
	}
	b.WriteString("}\n")
	return nil
}

func emitCFunctionBody(b *strings.Builder, fn *IRFunction, main bool, functions map[string]*IRFunction, functionNames map[string]string, moduleSeeds map[string]string, complexValues map[string]bool, hasProvider bool, programSource string, ffiPlan *nativeFFIPlan) error {
	reachable := nativeReachableBlocks(fn)
	vars := map[string]string{}
	names := make([]string, 0)
	for i, param := range fn.Params {
		vars[fmt.Sprintf("%s.%s.0", fn.Name, param)] = fmt.Sprintf("p%d", i)
	}
	for i, capture := range fn.CaptureValues {
		if i < len(fn.MutableCaptures) && fn.MutableCaptures[i] {
			vars[capture] = fmt.Sprintf("cap%d", i)
		} else {
			vars[capture] = fmt.Sprintf("c%d", i)
		}
	}
	mutableCells := make(map[string]string)
	cellByValue := make(map[string]string)
	mutableBindings := make(map[string]bool, len(fn.MutableBindings))
	for i, name := range fn.MutableBindings {
		mutableBindings[name] = true
		mutableCells[name] = fmt.Sprintf("cell%d", i)
	}
	for i, capture := range fn.Captures {
		if i >= len(fn.MutableCaptures) || !fn.MutableCaptures[i] || i >= len(fn.CaptureValues) {
			continue
		}
		cell := fmt.Sprintf("c%d", i)
		mutableCells[capture] = cell
		cellByValue[fn.CaptureValues[i]] = cell
	}
	add := func(name string) {
		if name != "" {
			if _, ok := vars[name]; !ok {
				vars[name] = fmt.Sprintf("v%d", len(names))
				names = append(names, name)
			}
		}
	}
	bindings := make(map[string]*IRFunction)
	for name, target := range functions {
		if target == fn && strings.Contains(name, "."+fn.SourceName+".") {
			bindings[name] = target
		}
	}
	closureArgs := make(map[string][]string)
	builtins := make(map[string]string)
	modules := nativeModuleIdentities(fn, moduleSeeds)
	moduleCandidates := nativeModuleCandidates(fn, moduleSeeds)
	for _, block := range fn.Blocks {
		if !reachable[block.Name] {
			continue
		}
		for _, p := range block.Params {
			add(p)
		}
		for _, in := range block.Instrs {
			add(in.Dest)
			for _, arg := range in.Args {
				add(arg)
			}
			if in.Op == "makefunc" {
				bindings[in.Dest] = functions[in.Text]
				closureArgs[in.Dest] = append([]string(nil), in.Args...)
			}
			if in.Op == "builtin" {
				builtins[in.Dest] = in.Text
			}
			if in.Op == "member" && len(in.Args) == 1 {
				if builtin, ok := nativeStaticModuleMember(modules[in.Args[0]], in.Text); ok {
					builtins[in.Dest] = builtin
				}
			}
		}
		switch term := block.Term.(type) {
		case *irJump:
			for _, arg := range term.Args {
				add(arg)
			}
		case *irBranch:
			add(term.Cond)
			for _, arg := range term.TrueArgs {
				add(arg)
			}
			for _, arg := range term.FalseArgs {
				add(arg)
			}
		case *irReturn:
			add(term.Value)
		}
	}
	for _, block := range fn.Blocks {
		if !reachable[block.Name] {
			continue
		}
		for _, in := range block.Instrs {
			if in.Op == "asserttype" && !nativeTypeAssertionSupported(in.Text) {
				return nativeCInstrError(programSource, in, "type assertion %q is not supported", in.Text)
			}
			if in.Op == "member" && len(in.Args) == 1 {
				path, ok := modules[in.Args[0]]
				if !ok {
					if moduleCandidates[in.Args[0]] {
						return nativeCInstrError(programSource, in, "dynamic module member %q cannot be lowered", in.Text)
					}
					continue
				}
				builtin, supported := nativeStaticModuleMember(path, in.Text)
				if !supported {
					return nativeCInstrError(programSource, in, "static module member %q.%s is not supported", path, in.Text)
				}
				if capability := nativeModuleCapability(path, in.Text); !nativeCapabilityGranted(ffiPlan, capability) {
					return nativeCInstrError(programSource, in, "native capability %q is not granted for %s.%s", capability, path, in.Text)
				}
				builtins[in.Dest] = builtin
			}
		}
	}
	resourceProducers := nativeResourceProducers(fn, builtins)
	sort.Slice(names, func(i, j int) bool { return vars[names[i]] < vars[names[j]] })
	for _, name := range names {
		fmt.Fprintf(b, "  OValue %s = onull();\n", vars[name])
	}
	for _, name := range fn.MutableBindings {
		fmt.Fprintf(b, "  OValue %s = onull();\n", mutableCells[name])
	}
	for i := range fn.CaptureValues {
		if i < len(fn.MutableCaptures) && fn.MutableCaptures[i] {
			fmt.Fprintf(b, "  OValue cap%d = c%d;\n", i, i)
		}
	}
	for _, block := range fn.Blocks {
		if !reachable[block.Name] {
			continue
		}
		fmt.Fprintf(b, "%s:\n", cLabel(block.Name))
		for _, in := range block.Instrs {
			if err := emitCInstr(b, in, vars, bindings, closureArgs, builtins, resourceProducers, complexValues, functionNames, hasProvider, fn.Name, ffiPlan, programSource, mutableCells, cellByValue, mutableBindings); err != nil {
				return err
			}
			if main {
				b.WriteString("  if (ohaderror) { fprintf(stderr, \"osiris native error: %s\\n\", oerrormessage); return 1; }\n")
			} else {
				b.WriteString("  if (ohaderror) return onull();\n")
			}
		}
		if err := emitCTerm(b, block.Term, fn.Blocks, vars, main, cellByValue); err != nil {
			return fmt.Errorf("function %s block %s: %w", fn.Name, block.Name, err)
		}
	}
	return nil
}

func nativeCInstrError(sourceName string, in IRInstr, format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	if sourceName != "" && in.Span.start.line > 0 {
		return fmt.Errorf("%s:%d:%d: native C backend: %s", sourceName, in.Span.start.line, in.Span.start.col, message)
	}
	return fmt.Errorf("native C backend: %s", message)
}

func nativeTypeAssertionSupported(name string) bool {
	if strings.HasPrefix(name, "func(") {
		return true
	}
	switch name {
	case "number", "u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64",
		"f32", "f64", "string", "bool", "null", "list", "tensor", "enum",
		"function", "complex", "decimal", "bigint", "quantity", "datetime",
		"duration", "error", "result", "task":
		return true
	default:
		// Named trait/interface assertions are proven by the shared checker;
		// native values retain their concrete tagged representation and the
		// witness table performs the dynamic method lookup.
		return len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z'
	}
}

func nativeReachableBlocks(fn *IRFunction) map[string]bool {
	reachable := make(map[string]bool)
	if fn == nil || len(fn.Blocks) == 0 {
		return reachable
	}
	byName := make(map[string]*IRBlock, len(fn.Blocks))
	for _, block := range fn.Blocks {
		if block != nil {
			byName[block.Name] = block
		}
	}
	work := []string{fn.Blocks[0].Name}
	for len(work) > 0 {
		name := work[len(work)-1]
		work = work[:len(work)-1]
		if reachable[name] {
			continue
		}
		reachable[name] = true
		if block := byName[name]; block != nil {
			work = append(work, cfgSuccessors(block.Term)...)
		}
	}
	return reachable
}

func emitCClosureWrapper(b *strings.Builder, fn *IRFunction, name string) {
	fmt.Fprintf(b, "\nstatic OValue %s_closure(const OValue *args, size_t argc, const OValue *captures, size_t capture_count) {\n", name)
	fmt.Fprintf(b, "  if (argc != %d || capture_count != %d) return ofail(\"closure call arity mismatch\");\n", len(fn.Params), len(fn.Captures))
	fmt.Fprintf(b, "  return %s(", name)
	for i := range fn.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(b, "args[%d]", i)
	}
	for i := range fn.Captures {
		if len(fn.Params) > 0 || i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(b, "captures[%d]", i)
	}
	b.WriteString(");\n}\n")
}

func nativeComplexMathBuiltin(name string) bool {
	switch name {
	case "exp", "log", "sqrt", "pow", "expt", "sin", "cos", "tan", "sinh", "cosh", "tanh":
		return true
	default:
		return false
	}
}

func nativeStaticModuleMember(path, member string) (string, bool) {
	if path == "std/math" {
		return member, true
	}
	supported := map[string]map[string]struct{}{
		"std/net":    {"tcp_listen": {}, "tcp_accept": {}, "tcp_dial": {}, "udp_bind": {}, "udp_connect": {}, "udp_send": {}, "udp_receive": {}, "dns_lookup": {}, "tls_dial": {}, "tls_dial_config": {}, "tls_listen": {}, "http_get": {}, "http_request": {}, "http_request_stream": {}, "http_serve": {}},
		"std/crypto": {"random": {}, "hash": {}, "digest": {}, "digest_bytes": {}, "hex_encode": {}, "hex_decode": {}, "compare_digest": {}, "algorithm_supported": {}},
		"std/list":   {"range": {}, "len": {}, "take": {}, "reverse": {}, "at": {}, "index_of": {}, "last_index_of": {}, "contains": {}, "count": {}, "sort": {}, "unique": {}, "slice": {}, "chunks": {}, "windows": {}, "zip": {}, "concat": {}, "repeat": {}, "empty": {}, "all": {}, "any": {}, "none": {}, "enumerate": {}},
		"std/signal": {"rectangular": {}, "hann": {}, "hamming": {}, "blackman": {}, "bartlett": {}, "convolve": {}, "correlate": {}, "filter_fir": {}, "mean": {}, "energy": {}, "variance": {}, "rms": {}, "fft": {}, "ifft": {}, "normalize": {}},
		"std/text":   {"trim": {}, "trim_space": {}, "upper": {}, "lower": {}, "fields": {}, "join": {}, "slug": {}, "cat": {}, "replace": {}, "replace_n": {}, "split": {}, "split_after": {}, "split_n": {}, "split_after_n": {}, "chunks": {}, "compare": {}, "contains": {}, "has_prefix": {}, "has_suffix": {}, "equal_fold": {}, "count": {}, "index": {}, "last_index": {}, "index_any": {}, "last_index_any": {}, "cut": {}, "cut_prefix": {}, "cut_suffix": {}, "trim_left": {}, "trim_right": {}, "trim_prefix": {}, "trim_suffix": {}, "repeat": {}},
		"std/time": {
			"add": {}, "add_date": {}, "date": {}, "duration_string": {},
			"format": {}, "parse_duration": {}, "rfc3339": {}, "sub": {},
			"utc_location": {},
		},
		"std/os": {"is_path_separator": {}, "read_file": {}, "write_file": {}, "getenv": {}, "env_names": {}, "environ": {}, "setenv": {}, "unsetenv": {}, "hostname": {}, "getpid": {}, "getppid": {}, "getuid": {}, "geteuid": {}, "getgid": {}, "getegid": {}, "getpagesize": {}, "process_spawn": {}, "start_process": {}, "find_process": {}, "process_wait": {}, "process_state": {}, "process_kill": {}, "process_signal": {}, "process_release": {}, "process_stdin": {}, "process_stdout": {}, "process_stderr": {}, "open": {}, "create": {}, "open_file": {}, "pipe": {}, "stdin": {}, "stdout": {}, "stderr": {}, "fd": {}, "name": {}, "new_file": {}, "signal_subscribe": {}, "close": {}, "read": {}, "write": {}, "write_string": {}, "read_at": {}, "write_at": {}, "seek_file": {}, "sync": {}, "set_deadline": {}, "set_read_deadline": {}, "set_write_deadline": {}, "open_root": {}, "root_create": {}, "root_open": {}, "root_open_file": {}, "root_read_file": {}, "root_write_file": {}, "root_stat": {}, "root_lstat": {}, "root_chown": {}, "root_lchown": {}, "root_chtimes": {}, "root_fs": {}, "root_readlink": {}, "root_mkdir": {}, "root_mkdir_all": {}, "root_remove": {}, "root_remove_all": {}, "root_rename": {}, "root_link": {}, "root_symlink": {}, "root_chmod": {}, "root_close": {}, "stat": {}, "lstat": {}, "read_dir": {}, "mkdir": {}, "mkdir_all": {}, "remove": {}, "remove_all": {}, "truncate": {}, "rename": {}, "link": {}, "symlink": {}, "chmod": {}, "readlink": {}, "watch": {}},
	}
	members, ok := supported[path]
	if !ok {
		return "", false
	}
	if _, ok := members[member]; !ok {
		return "", false
	}
	if path == "std/math" {
		return member, true
	}
	return path + "." + member, true
}

// nativeResourceProducer describes the capability family owned by a resource
// value. Generic readchunk/writechunk/close calls cannot use one ambient
// capability because the same builtin is shared by files, processes, sockets,
// TLS streams, and HTTP body streams.
type nativeResourceProducer struct {
	name  string
	read  string
	write string
	close string
}

func nativeResourceProducerForBuiltin(builtin string) (nativeResourceProducer, bool) {
	switch builtin {
	case "std/os.open", "std/os.open_file", "std/os.stdin":
		return nativeResourceProducer{name: builtin, read: "os.file_read", write: "os.file_write", close: "os.file_read"}, true
	case "std/os.create", "std/os.stdout", "std/os.stderr":
		return nativeResourceProducer{name: builtin, read: "os.file_read", write: "os.file_write", close: "os.file_write"}, true
	case "std/os.new_file":
		return nativeResourceProducer{name: builtin, read: "os.file_read", write: "os.file_write", close: "os.process_raw_handle"}, true
	case "std/os.pipe":
		// pipe returns a two-element list; listatunchecked below refines this
		// marker to the read or write endpoint before a generic operation.
		return nativeResourceProducer{name: builtin}, true
	case "std/os.process_spawn", "std/os.start_process", "std/os.find_process":
		return nativeResourceProducer{name: builtin, read: "os.process", write: "os.process", close: "os.process"}, true
	case "std/os.process_stdin", "std/os.process_stdout", "std/os.process_stderr":
		return nativeResourceProducer{name: builtin, read: "os.process", write: "os.process", close: "os.process"}, true
	case "std/os.open_root":
		return nativeResourceProducer{name: builtin, read: "os.file_read", write: "os.file_write", close: "os.file_read"}, true
	case "std/os.root_create":
		return nativeResourceProducer{name: builtin, read: "os.file_read", write: "os.file_write", close: "os.file_write"}, true
	case "std/os.root_open":
		return nativeResourceProducer{name: builtin, read: "os.file_read", write: "os.file_write", close: "os.file_read"}, true
	case "std/os.root_open_file":
		return nativeResourceProducer{name: builtin, read: "os.file_read", write: "os.file_write", close: "os.file_write"}, true
	case "std/os.signal_subscribe":
		return nativeResourceProducer{name: builtin, read: "os.signal", write: "os.signal", close: "os.signal"}, true
	case "std/os.watch":
		return nativeResourceProducer{name: builtin, read: "os.watch", write: "os.watch", close: "os.watch"}, true
	case "std/net.tcp_listen", "std/net.tcp_accept", "std/net.tcp_dial", "std/net.udp_bind", "std/net.udp_connect":
		return nativeResourceProducer{name: builtin, read: "net.tcp_read", write: "net.tcp_write", close: "net.close"}, true
	case "std/net.tls_dial":
		return nativeResourceProducer{name: builtin, read: "net.tls_dial", write: "net.tls_dial", close: "net.tls_dial"}, true
	case "std/net.tls_dial_config":
		return nativeResourceProducer{name: builtin, read: "net.tls_dial_config", write: "net.tls_dial_config", close: "net.tls_dial_config"}, true
	case "std/net.tls_listen":
		return nativeResourceProducer{name: builtin, read: "net.tls_listen", write: "net.tls_listen", close: "net.tls_listen"}, true
	case "std/net.http_request_stream":
		return nativeResourceProducer{name: builtin, read: "net.http_stream", write: "net.http_stream", close: "net.http_stream"}, true
	default:
		return nativeResourceProducer{}, false
	}
}

func nativeResourceProducerForValue(builtin string) (nativeResourceProducer, bool) {
	return nativeResourceProducerForBuiltin(builtin)
}

// nativeResourceProducers tracks producer identity through the SSA values
// emitted by one function. A missing value is deliberately treated as
// unknown; accepting it with net.tcp_* would reintroduce the old ambient
// capability leak for files and host streams.
func nativeResourceProducers(fn *IRFunction, builtins map[string]string) map[string]nativeResourceProducer {
	producers := make(map[string]nativeResourceProducer)
	constantIndices := make(map[string]uint64)
	if fn == nil {
		return producers
	}
	for _, block := range fn.Blocks {
		for _, in := range block.Instrs {
			switch in.Op {
			case "const":
				switch in.Value.kind {
				case kindUint:
					constantIndices[in.Dest] = in.Value.u
				case kindInt:
					if in.Value.i != nil && in.Value.i.Sign() >= 0 && in.Value.i.BitLen() <= 64 {
						constantIndices[in.Dest] = in.Value.i.Uint64()
					}
				}
				delete(producers, in.Dest)
			case "copy", "assign":
				if len(in.Args) == 1 {
					if producer, ok := producers[in.Args[0]]; ok {
						producers[in.Dest] = producer
					} else {
						delete(producers, in.Dest)
					}
				}
			case "call":
				if producer, ok := nativeResourceProducerForValue(builtins[in.Text]); ok {
					producers[in.Dest] = producer
				} else {
					delete(producers, in.Dest)
				}
			case "member":
				if producer, ok := nativeResourceProducerForValue(builtins[in.Dest]); ok {
					producers[in.Dest] = producer
				} else if len(in.Args) == 1 && in.Text == "process" {
					if process, ok := producers[in.Args[0]]; ok && (process.name == "std/os.process_spawn" || process.name == "std/os.start_process") {
						producers[in.Dest] = nativeResourceProducer{name: "std/os.process_spawn.process", read: "os.process", write: "os.process", close: "os.process"}
					} else {
						delete(producers, in.Dest)
					}
				} else if len(in.Args) == 1 && in.Text == "body_stream" {
					if response, ok := producers[in.Args[0]]; ok && response.name == "std/net.http_request_stream" {
						producers[in.Dest] = nativeResourceProducer{name: "std/net.http_request_stream.body_stream", read: "net.http_stream", write: "net.http_stream", close: "net.http_stream"}
					} else {
						delete(producers, in.Dest)
					}
				} else if len(in.Args) == 1 && in.Text == "root_fs" {
					if root, ok := producers[in.Args[0]]; ok && root.name == "std/os.open_root" {
						producers[in.Dest] = root
					} else {
						delete(producers, in.Dest)
					}
				} else {
					delete(producers, in.Dest)
				}
			case "listatunchecked":
				// A pipe's endpoints are the only native resource tuple whose
				// element producer is statically known by this backend.
				if len(in.Args) == 2 {
					if pipe, ok := producers[in.Args[0]]; ok && pipe.name == "std/os.pipe" {
						if index, isConstant := constantIndices[in.Args[1]]; isConstant && index == 0 {
							producers[in.Dest] = nativeResourceProducer{name: "std/os.pipe.read", read: "os.file_read", close: "os.file_read"}
						} else if isConstant && index == 1 {
							producers[in.Dest] = nativeResourceProducer{name: "std/os.pipe.write", write: "os.file_write", close: "os.file_write"}
						} else {
							delete(producers, in.Dest)
						}
					} else {
						delete(producers, in.Dest)
					}
				}
			default:
				// Other IR operations either do not produce resources or require
				// a typed operation-specific lowering before they can preserve
				// producer identity.
				delete(producers, in.Dest)
			}
		}
	}
	return producers
}

func nativeGenericResourceCapability(builtin string, resource string, producers map[string]nativeResourceProducer) (string, string, bool) {
	producer, ok := producers[resource]
	if !ok {
		return "", "", false
	}
	switch builtin {
	case "readchunk":
		return producer.read, producer.name, producer.read != ""
	case "writechunk":
		return producer.write, producer.name, producer.write != ""
	case "close":
		return producer.close, producer.name, producer.close != ""
	default:
		return "", "", false
	}
}

func nativeModuleCapability(path, member string) string {
	// Generic resource operations are authorized from the SSA producer at the
	// call site. They must not acquire a coarse capability merely because the
	// operation was selected through std/os or std/net member syntax.
	if (path == "std/os" || path == "std/net") && (member == "readchunk" || member == "writechunk" || member == "close") {
		return ""
	}
	if capability, ok := nativeHostResourceCapability(path, member); ok {
		return capability
	}
	if path == "std/net" {
		return nativeNetworkRuntimeCapabilities()[member]
	}
	if path != "std/os" {
		return ""
	}
	switch member {
	case "env_names", "environ":
		return "os.environment"
	case "setenv", "unsetenv":
		return "os.environment_mutate"
	case "hostname", "getpid", "getppid", "getuid", "geteuid", "getgid", "getegid", "getpagesize":
		return "os.system_identity"
	case "getenv":
		return "os.environment"
	case "read_file":
		return "os.file_read"
	case "write_file":
		return "os.file_write"
	default:
		return ""
	}
}

func nativeBuiltinCapability(builtin string) string {
	// These builtins are checked against the producer SSA value in
	// emitCFunctionBody. Returning a shared network capability here would make
	// a host file or process stream require an unrelated net grant.
	if builtin == "readchunk" || builtin == "writechunk" || builtin == "close" {
		return ""
	}
	if module, member, ok := strings.Cut(builtin, "."); ok && (module == "std/os" || module == "std/net") && (member == "readchunk" || member == "writechunk" || member == "close") {
		return ""
	}
	if capability := nativeNetworkRuntimeCapabilities()[builtin]; capability != "" {
		return capability
	}
	if module, member, ok := strings.Cut(builtin, "."); ok {
		if capability, ok := nativeHostResourceCapability(module, member); ok {
			return capability
		}
		if module == "std/net" {
			return nativeNetworkRuntimeCapabilities()[member]
		}
	}
	if capability, ok := nativeHostResourceCapability("std/os", builtin); ok {
		return capability
	}
	return ""
}

func emitNativeResourceCall(b *strings.Builder, dest, builtin string, args []string) (bool, error) {
	name := builtin
	if module, member, ok := strings.Cut(builtin, "."); ok && (module == "std/net" || module == "std/os") {
		name = member
	}
	arg := func(index int) string {
		if index < len(args) {
			return args[index]
		}
		return "onull()"
	}
	call := func(function string, values ...string) {
		fmt.Fprintf(b, "  %s = %s(", dest, function)
		for i, value := range values {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(value)
		}
		b.WriteString(");\n")
	}
	require := func(min, max int) error {
		if len(args) < min || (max >= 0 && len(args) > max) {
			if min == max {
				return fmt.Errorf("native C backend: %s expects %d argument(s), got %d", builtin, min, len(args))
			}
			return fmt.Errorf("native C backend: %s expects between %d and %d argument(s), got %d", builtin, min, max, len(args))
		}
		return nil
	}
	switch name {
	case "tcp_listen":
		if err := require(1, 1); err != nil {
			return true, err
		}
		call("onet_tcp_listen", args[0])
	case "tcp_accept":
		if err := require(1, 2); err != nil {
			return true, err
		}
		call("onet_tcp_accept", args[0], arg(1))
	case "tcp_dial":
		if err := require(1, 2); err != nil {
			return true, err
		}
		call("onet_tcp_dial", args[0], arg(1))
	case "udp_bind":
		if err := require(1, 1); err != nil {
			return true, err
		}
		call("onet_udp_bind", args[0])
	case "udp_connect":
		if err := require(1, 1); err != nil {
			return true, err
		}
		call("onet_udp_connect", args[0])
	case "udp_send":
		if err := require(2, 3); err != nil {
			return true, err
		}
		call("onet_udp_send", args[0], args[1], arg(2))
	case "udp_receive":
		if err := require(1, 3); err != nil {
			return true, err
		}
		call("onet_udp_receive", args[0], arg(1), arg(2))
	case "dns_lookup":
		if err := require(1, 2); err != nil {
			return true, err
		}
		call("onet_dns_lookup", args[0], arg(1))
	case "tls_dial":
		if err := require(1, 3); err != nil {
			return true, err
		}
		call("osiris_native_tls_dial", args[0], arg(1), "onull()", arg(2))
	case "tls_dial_config":
		if err := require(3, 4); err != nil {
			return true, err
		}
		call("osiris_native_tls_dial", args[0], args[1], args[2], arg(3))
	case "tls_listen":
		if err := require(3, 3); err != nil {
			return true, err
		}
		call("osiris_native_tls_listen", args[0], args[1], args[2])
	case "http_get":
		if err := require(1, 2); err != nil {
			return true, err
		}
		call("osiris_native_http_get_default", args[0], arg(1))
	case "http_request":
		if err := require(2, 5); err != nil {
			return true, err
		}
		call("osiris_native_http_request_default", args[0], args[1], arg(2), arg(3), arg(4))
	case "http_request_stream":
		if err := require(2, 5); err != nil {
			return true, err
		}
		call("osiris_native_http_request_stream_default", args[0], args[1], arg(2), arg(3), arg(4))
	case "http_serve":
		if err := require(2, 3); err != nil {
			return true, err
		}
		call("osiris_native_http_serve_default", args[0], args[1], arg(2))
	case "readchunk":
		if err := require(2, 3); err != nil {
			return true, err
		}
		call("osiris_native_resource_readchunk", args[0], args[1], arg(2))
	case "writechunk":
		if err := require(2, 3); err != nil {
			return true, err
		}
		call("osiris_native_resource_writechunk", args[0], args[1], arg(2))
	case "close":
		if err := require(1, 1); err != nil {
			return true, err
		}
		call("osiris_native_resource_close", args[0])
	case "process_spawn", "start_process":
		if err := require(2, 3); err != nil {
			return true, err
		}
		call("osiris_host_process_spawn", args[0], args[1], arg(2))
	case "find_process":
		if err := require(1, 1); err != nil {
			return true, err
		}
		call("osiris_host_process_find", args[0])
	case "process_wait":
		if err := require(1, 2); err != nil {
			return true, err
		}
		call("osiris_host_process_wait", args[0], arg(1))
	case "process_state":
		if err := require(1, 1); err != nil {
			return true, err
		}
		call("osiris_host_process_state", args[0])
	case "process_kill", "process_signal":
		if err := require(1, 2); err != nil {
			return true, err
		}
		call("osiris_host_process_kill", args[0], arg(1))
	case "process_release":
		if err := require(1, 1); err != nil {
			return true, err
		}
		call("osiris_host_process_release", args[0])
	case "process_stdin", "process_stdout", "process_stderr":
		if err := require(1, 1); err != nil {
			return true, err
		}
		which := map[string]string{"process_stdin": "0", "process_stdout": "1", "process_stderr": "2"}[name]
		call("osiris_host_process_stream", args[0], which)
	case "pipe":
		if err := require(0, 0); err != nil {
			return true, err
		}
		call("osiris_host_pipe")
	case "stdin", "stdout", "stderr":
		if err := require(0, 0); err != nil {
			return true, err
		}
		which := map[string]string{"stdin": "0", "stdout": "1", "stderr": "2"}[name]
		call("osiris_host_standard_stream", which)
	case "fd":
		if err := require(1, 1); err != nil {
			return true, err
		}
		call("osiris_host_fd", args[0])
	case "name":
		if err := require(1, 1); err != nil {
			return true, err
		}
		call("osiris_host_name", args[0])
	case "new_file":
		if err := require(2, 2); err != nil {
			return true, err
		}
		call("osiris_host_new_file", args[0], args[1])
	case "signal_subscribe":
		if err := require(1, 1); err != nil {
			return true, err
		}
		call("osiris_host_signal_subscribe", args[0])
	case "open":
		if err := require(1, 1); err != nil {
			return true, err
		}
		call("osiris_host_file_open", args[0], "ouint(O_RDONLY)", "ouint(0)")
	case "create":
		if err := require(1, 1); err != nil {
			return true, err
		}
		call("osiris_host_file_open", args[0], "ouint(O_RDWR|O_CREAT|O_TRUNC)", "ouint(0666)")
	case "open_file":
		if err := require(3, 3); err != nil {
			return true, err
		}
		call("osiris_host_file_open", args[0], args[1], args[2])
	case "stat":
		if err := require(1, 1); err != nil {
			return true, err
		}
		call("osiris_host_stat", args[0], "1")
	case "lstat":
		if err := require(1, 1); err != nil {
			return true, err
		}
		call("osiris_host_stat", args[0], "0")
	case "read_dir":
		if err := require(1, 1); err != nil {
			return true, err
		}
		call("osiris_host_read_dir", args[0])
	case "mkdir":
		if err := require(2, 2); err != nil {
			return true, err
		}
		call("osiris_host_mkdir", args[0], args[1], "0")
	case "mkdir_all":
		if err := require(2, 2); err != nil {
			return true, err
		}
		call("osiris_host_mkdir", args[0], args[1], "1")
	case "remove":
		if err := require(1, 1); err != nil {
			return true, err
		}
		call("osiris_host_remove", args[0], "0")
	case "remove_all":
		if err := require(1, 1); err != nil {
			return true, err
		}
		call("osiris_host_remove", args[0], "1")
	case "truncate":
		if err := require(2, 2); err != nil {
			return true, err
		}
		call("osiris_host_truncate", args[0], args[1])
	case "rename":
		if err := require(2, 2); err != nil {
			return true, err
		}
		call("osiris_host_rename", args[0], args[1])
	case "link", "symlink":
		if err := require(2, 2); err != nil {
			return true, err
		}
		symbolic := "0"
		if name == "symlink" {
			symbolic = "1"
		}
		call("osiris_host_link", args[0], args[1], symbolic)
	case "readlink":
		if err := require(1, 1); err != nil {
			return true, err
		}
		call("osiris_host_readlink", args[0])
	case "chmod":
		if err := require(2, 2); err != nil {
			return true, err
		}
		call("osiris_host_chmod", args[0], args[1])
	case "write_string":
		if err := require(2, 3); err != nil {
			return true, err
		}
		call("osiris_host_file_write_string", args[0], args[1], arg(2))
	case "watch":
		if err := require(1, 2); err != nil {
			return true, err
		}
		call("osiris_host_watch", args[0], arg(1))
	case "read":
		if err := require(2, 3); err != nil {
			return true, err
		}
		call("osiris_host_file_io", args[0], args[1], "onull()", arg(2), "0", "0")
	case "write":
		if err := require(2, 3); err != nil {
			return true, err
		}
		call("osiris_host_file_io", args[0], args[1], "onull()", arg(2), "1", "0")
	case "read_at":
		if err := require(3, 4); err != nil {
			return true, err
		}
		call("osiris_host_file_io", args[0], args[1], args[2], arg(3), "0", "1")
	case "write_at":
		if err := require(3, 4); err != nil {
			return true, err
		}
		call("osiris_host_file_io", args[0], args[1], args[2], arg(3), "1", "1")
	case "seek_file":
		if err := require(3, 3); err != nil {
			return true, err
		}
		call("osiris_host_file_seek", args[0], args[1], args[2])
	case "sync":
		if err := require(1, 1); err != nil {
			return true, err
		}
		call("osiris_host_file_sync", args[0])
	case "set_deadline", "set_read_deadline", "set_write_deadline":
		if err := require(2, 2); err != nil {
			return true, err
		}
		call("osiris_host_resource_deadline", args[0], args[1])
	case "open_root":
		if err := require(1, 1); err != nil {
			return true, err
		}
		call("osiris_host_root_open", args[0])
	case "root_create":
		if err := require(2, 2); err != nil {
			return true, err
		}
		call("osiris_host_root_create", args[0], args[1])
	case "root_open":
		if err := require(2, 2); err != nil {
			return true, err
		}
		call("osiris_host_root_open_file", args[0], args[1], "ouint(O_RDONLY)", "ouint(0)")
	case "root_open_file":
		if err := require(4, 4); err != nil {
			return true, err
		}
		call("osiris_host_root_open_file", args[0], args[1], args[2], args[3])
	case "root_read_file":
		if err := require(2, 2); err != nil {
			return true, err
		}
		call("osiris_host_root_read_file", args[0], args[1])
	case "root_write_file":
		if err := require(4, 4); err != nil {
			return true, err
		}
		call("osiris_host_root_write_file", args[0], args[1], args[2], args[3])
	case "root_stat", "root_lstat":
		if err := require(2, 2); err != nil {
			return true, err
		}
		follow := "1"
		if name == "root_lstat" {
			follow = "0"
		}
		call("osiris_host_root_stat", args[0], args[1], follow)
	case "root_chown", "root_lchown":
		if err := require(4, 4); err != nil {
			return true, err
		}
		link := "0"
		if name == "root_lchown" {
			link = "1"
		}
		call("osiris_host_root_chown", args[0], args[1], args[2], args[3], link)
	case "root_chtimes":
		if err := require(4, 4); err != nil {
			return true, err
		}
		call("osiris_host_root_chtimes", args[0], args[1], args[2], args[3])
	case "root_fs":
		if err := require(1, 1); err != nil {
			return true, err
		}
		call("osiris_host_root_fs", args[0])
	case "root_readlink":
		if err := require(2, 2); err != nil {
			return true, err
		}
		call("osiris_host_root_readlink", args[0], args[1])
	case "root_link", "root_symlink":
		if err := require(3, 3); err != nil {
			return true, err
		}
		symbolic := "0"
		if name == "root_symlink" {
			symbolic = "1"
		}
		call("osiris_host_root_link", args[0], args[1], args[2], symbolic)
	case "root_mkdir", "root_mkdir_all":
		if err := require(3, 3); err != nil {
			return true, err
		}
		if name == "root_mkdir_all" {
			call("osiris_host_root_mkdir_all", args[0], args[1], args[2])
		} else {
			call("osiris_host_root_mkdir", args[0], args[1], args[2])
		}
	case "root_remove", "root_remove_all":
		if err := require(2, 2); err != nil {
			return true, err
		}
		if name == "root_remove_all" {
			call("osiris_host_root_remove_all", args[0], args[1])
		} else {
			call("osiris_host_root_remove", args[0], args[1])
		}
	case "root_rename":
		if err := require(3, 3); err != nil {
			return true, err
		}
		call("osiris_host_root_rename", args[0], args[1], args[2])
	case "root_chmod":
		if err := require(3, 3); err != nil {
			return true, err
		}
		call("osiris_host_root_chmod", args[0], args[1], args[2])
	case "root_close":
		if err := require(1, 1); err != nil {
			return true, err
		}
		call("osiris_host_root_close", args[0])
	default:
		return false, nil
	}
	return true, nil
}

func nativeKnownComplexProgramValues(prog *IRProgram, moduleIdentities map[string]map[string]string) map[string]bool {
	complexValues := make(map[string]bool)
	builtinsByFunction := make(map[string]map[string]string)
	targetsByFunction := make(map[string]map[string]*IRFunction)
	functions := make(map[string]*IRFunction)
	for _, fn := range prog.Functions {
		functions[fn.Name] = fn
	}
	for _, fn := range prog.Functions {
		builtins := make(map[string]string)
		targets := make(map[string]*IRFunction)
		modules := nativeModuleIdentities(fn, moduleIdentities[fn.Name])
		for _, block := range fn.Blocks {
			for _, in := range block.Instrs {
				if in.Op == "builtin" {
					builtins[in.Dest] = in.Text
				}
				if in.Op == "member" && len(in.Args) == 1 && modules[in.Args[0]] == "std/math" {
					builtins[in.Dest] = in.Text
				}
				if in.Op == "makefunc" {
					targets[in.Dest] = functions[in.Text]
				}
			}
		}
		builtinsByFunction[fn.Name] = builtins
		targetsByFunction[fn.Name] = targets
	}
	changed := true
	for changed {
		changed = false
		mark := func(name string) {
			if name != "" && !complexValues[name] {
				complexValues[name] = true
				changed = true
			}
		}
		for _, fn := range prog.Functions {
			builtins := builtinsByFunction[fn.Name]
			targets := targetsByFunction[fn.Name]
			for _, block := range fn.Blocks {
				for _, in := range block.Instrs {
					switch in.Op {
					case "const":
						if in.Value.kind == kindComplex {
							mark(in.Dest)
						}
					case "copy":
						if len(in.Args) == 1 {
							if complexValues[in.Args[0]] {
								mark(in.Dest)
							}
							if targets[in.Args[0]] != nil {
								targets[in.Dest] = targets[in.Args[0]]
							}
						}
					case "unary":
						if len(in.Args) == 1 && complexValues[in.Args[0]] {
							mark(in.Dest)
						}
					case "binary":
						if len(in.Args) == 2 && (complexValues[in.Args[0]] || complexValues[in.Args[1]]) {
							mark(in.Dest)
						}
					case "makefunc":
						target := targets[in.Dest]
						if target != nil {
							for index, arg := range in.Args {
								if index < len(target.CaptureValues) && complexValues[arg] {
									mark(target.CaptureValues[index])
								}
							}
						}
					case "call":
						if target := targets[in.Text]; target != nil {
							for index, arg := range in.Args {
								if index < len(target.Params) && complexValues[arg] {
									mark(fmt.Sprintf("%s.%s.0", target.Name, target.Params[index]))
								}
							}
							for _, targetBlock := range target.Blocks {
								if returned, ok := targetBlock.Term.(*irReturn); ok && returned.HasValue && complexValues[returned.Value] {
									mark(in.Dest)
								}
							}
							continue
						}
						builtin := builtins[in.Text]
						if builtin == "" {
							builtin = in.Text
						}
						if builtin == "complex" || builtin == "conjugate" {
							mark(in.Dest)
							continue
						}
						if nativeComplexMathBuiltin(builtin) {
							for _, arg := range in.Args {
								if complexValues[arg] {
									mark(in.Dest)
									break
								}
							}
						}
					}
				}
				switch term := block.Term.(type) {
				case *irJump:
					propagateNativeComplexBlockArgs(fn, term.Target, term.Args, complexValues, mark)
				case *irBranch:
					propagateNativeComplexBlockArgs(fn, term.TrueTarget, term.TrueArgs, complexValues, mark)
					propagateNativeComplexBlockArgs(fn, term.FalseTarget, term.FalseArgs, complexValues, mark)
				}
			}
		}
	}
	return complexValues
}

func propagateNativeComplexBlockArgs(fn *IRFunction, target string, args []string, complexValues map[string]bool, mark func(string)) {
	for _, block := range fn.Blocks {
		if block.Name != target {
			continue
		}
		for index, arg := range args {
			if index < len(block.Params) && complexValues[arg] {
				mark(block.Params[index])
			}
		}
		return
	}
}

func emitCInstr(b *strings.Builder, in IRInstr, vars map[string]string, bindings map[string]*IRFunction, closureArgs map[string][]string, builtins map[string]string, resourceProducers map[string]nativeResourceProducer, complexValues map[string]bool, functionNames map[string]string, hasProvider bool, currentFunction string, ffiPlan *nativeFFIPlan, programSource string, mutableCells map[string]string, cellByValue map[string]string, mutableBindings map[string]bool) error {
	diagnostic := func(format string, args ...any) error {
		return nativeCInstrError(programSource, in, format, args...)
	}
	name := func(v string) (string, error) {
		n, ok := vars[v]
		if !ok {
			return "", fmt.Errorf("native C backend: unknown value %q", v)
		}
		// Mutable captures and mutable SSA bindings are represented by stable
		// OCell values. Reads must dereference the cell, while closure creation
		// below can still recover the raw cell through cellByValue and pass it
		// through to the nested closure.
		if cell, isCell := cellByValue[v]; isCell {
			return fmt.Sprintf("ocellvalue(%s)", cell), nil
		}
		return n, nil
	}
	switch in.Op {
	case "const":
		v, err := cLiteral(in.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  %s = %s;\n", vars[in.Dest], v)
	case "global":
		if in.Value.kind == kindQuantity && in.Text != "meter" {
			return fmt.Errorf("native C backend: named quantity %q is not supported", in.Text)
		}
		v, err := cLiteral(in.Value)
		if err != nil {
			return fmt.Errorf("native C backend: named constant %q: %w", in.Text, err)
		}
		fmt.Fprintf(b, "  %s = %s; /* global %s */\n", vars[in.Dest], v, in.Text)
	case "copy", "assign":
		if len(in.Args) != 1 {
			return fmt.Errorf("native C backend: copy expects one argument")
		}
		v, err := name(in.Args[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  %s = %s;\n", vars[in.Dest], v)
		if in.Op == "copy" {
			if mutableBindings[in.Text] {
				fmt.Fprintf(b, "  %s = ocell(%s);\n", mutableCells[in.Text], vars[in.Dest])
			}
		} else if cell := mutableCells[in.Text]; cell != "" {
			fmt.Fprintf(b, "  (void)ocellset(%s, %s);\n", cell, vars[in.Dest])
		}
		if cell := mutableCells[in.Text]; cell != "" {
			cellByValue[in.Dest] = cell
		}
	case "binary":
		if len(in.Args) != 2 {
			return fmt.Errorf("native C backend: binary expects two arguments")
		}
		a, err := name(in.Args[0])
		if err != nil {
			return err
		}
		c, err := name(in.Args[1])
		if err != nil {
			return err
		}
		helper, ok := map[string]string{
			"+": "oadd", "-": "osub", "*": "omul", "/": "odiv",
			"%": "omod", "&": "oband", "|": "obor", "^": "obxor",
			"<<": "oshiftl", ">>": "oshiftr", "<": "olt", "<=": "ole",
			">": "ogt", ">=": "oge", "==": "oequal", "!=": "onotequal",
		}[in.Text]
		if !ok {
			return fmt.Errorf("native C backend: unsupported binary operator %q", in.Text)
		}
		fmt.Fprintf(b, "  %s = %s(%s, %s);\n", vars[in.Dest], helper, a, c)
	case "unary":
		if len(in.Args) != 1 {
			return fmt.Errorf("native C backend: unary expects one argument")
		}
		a, err := name(in.Args[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  %s = ounary(%q, %s);\n", vars[in.Dest], in.Text, a)
	case "emit":
		for _, arg := range in.Args {
			a, err := name(arg)
			if err != nil {
				return err
			}
			fmt.Fprintf(b, "  oprint(%s);\n", a)
		}
	case "cleanup":
		if len(in.Args) != 1 {
			return fmt.Errorf("native C backend: cleanup expects one thunk")
		}
		fmt.Fprintf(b, "  (void)ocall(%s, NULL, 0);\n", vars[in.Args[0]])
	case "makefunc":
		wrapper, ok := functionNames[in.Text]
		if !ok {
			return fmt.Errorf("native C backend: unknown function %q", in.Text)
		}
		signature := ""
		if target := bindings[in.Dest]; target != nil {
			signature = irFunctionSignature(target)
		}
		target := bindings[in.Dest]
		captureValues := make([]string, len(in.Args))
		for i, arg := range in.Args {
			v, err := name(arg)
			if err != nil {
				return err
			}
			if target != nil && i < len(target.MutableCaptures) && target.MutableCaptures[i] {
				cell := cellByValue[arg]
				if cell == "" && i < len(target.Captures) {
					cell = mutableCells[target.Captures[i]]
				}
				if cell == "" {
					captureName := fmt.Sprintf("capture %d", i)
					if i < len(target.Captures) {
						captureName = target.Captures[i]
					}
					return diagnostic("native C backend: mutable capture %q has no stable cell", captureName)
				}
				v = cell
			}
			captureValues[i] = v
		}
		if len(in.Args) == 0 {
			fmt.Fprintf(b, "  %s = oclosure(%s_closure, NULL, 0, \"%s\");\n", vars[in.Dest], wrapper, cEscapeString(signature))
			return nil
		}
		fmt.Fprintf(b, "  %s = oclosure(%s_closure, (OValue[]){", vars[in.Dest], wrapper)
		for i, value := range captureValues {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(value)
		}
		fmt.Fprintf(b, "}, %d, \"%s\");\n", len(in.Args), cEscapeString(signature))
		return nil
	case "import":
		// Static stdlib imports and members are resolved while scanning the IR
		// block. Their values are represented by the builtin map at call sites.
		return nil
	case "member":
		switch builtins[in.Dest] {
		case "pi":
			fmt.Fprintf(b, "  %s = ofloat(0x1.921fb54442d18p+1);\n", vars[in.Dest])
		case "tau":
			fmt.Fprintf(b, "  %s = ofloat(0x1.921fb54442d18p+2);\n", vars[in.Dest])
		case "std/time.utc_location":
			fmt.Fprintf(b, "  %s = ostring(\"UTC\");\n", vars[in.Dest])
		case "std/time.rfc3339":
			fmt.Fprintf(b, "  %s = ostring(\"2006-01-02T15:04:05Z07:00\");\n", vars[in.Dest])
		case "std/os.stdin":
			fmt.Fprintf(b, "  %s = osiris_host_standard_stream(0);\n", vars[in.Dest])
		case "std/os.stdout":
			fmt.Fprintf(b, "  %s = osiris_host_standard_stream(1);\n", vars[in.Dest])
		case "std/os.stderr":
			fmt.Fprintf(b, "  %s = osiris_host_standard_stream(2);\n", vars[in.Dest])
		default:
			if builtins[in.Dest] != "" {
				return nil
			}
			if len(in.Args) != 1 {
				return fmt.Errorf("native C backend: member access expects one receiver")
			}
			fmt.Fprintf(b, "  %s = omember(%s, \"%s\");\n", vars[in.Dest], vars[in.Args[0]], cEscapeString(in.Text))
		}
		return nil
	case "builtin":
		return nil
	case "undef":
		fmt.Fprintf(b, "  %s = onull();\n", vars[in.Dest])
		return nil
	case "list":
		if len(in.Args) == 0 {
			fmt.Fprintf(b, "  %s = olist(NULL, 0);\n", vars[in.Dest])
			return nil
		}
		b.WriteString("  ")
		fmt.Fprintf(b, "%s = olist((OValue[]){", vars[in.Dest])
		for i, arg := range in.Args {
			if i > 0 {
				b.WriteString(", ")
			}
			v, err := name(arg)
			if err != nil {
				return err
			}
			b.WriteString(v)
		}
		fmt.Fprintf(b, "}, %d);\n", len(in.Args))
		return nil
	case "enum":
		typeName, caseName, ok := strings.Cut(in.Text, ".")
		if !ok || typeName == "" || caseName == "" {
			return fmt.Errorf("native C backend: invalid enum case %q", in.Text)
		}
		if len(in.Args) == 0 {
			fmt.Fprintf(b, "  %s = oenum(\"%s\", \"%s\", NULL, 0);\n", vars[in.Dest], cEscapeString(typeName), cEscapeString(caseName))
			return nil
		}
		fmt.Fprintf(b, "  %s = oenum(\"%s\", \"%s\", (OValue[]){", vars[in.Dest], cEscapeString(typeName), cEscapeString(caseName))
		for i, arg := range in.Args {
			if i > 0 {
				b.WriteString(", ")
			}
			v, err := name(arg)
			if err != nil {
				return err
			}
			b.WriteString(v)
		}
		fmt.Fprintf(b, "}, %d);\n", len(in.Args))
		return nil
	case "matchenum":
		if len(in.Args) != 1 {
			return fmt.Errorf("native C backend: enum match expects one subject")
		}
		typeName, caseName, ok := strings.Cut(in.Text, ".")
		if !ok || typeName == "" || caseName == "" {
			return fmt.Errorf("native C backend: invalid enum pattern %q", in.Text)
		}
		subject, err := name(in.Args[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  %s = oenumis(%s, \"%s\", \"%s\", %du);\n", vars[in.Dest], subject, cEscapeString(typeName), cEscapeString(caseName), in.Value.u)
		return nil
	case "enumpayload":
		if len(in.Args) != 1 {
			return fmt.Errorf("native C backend: enum payload read expects one subject")
		}
		subject, err := name(in.Args[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  %s = oenumpayload(%s, %du);\n", vars[in.Dest], subject, in.Value.u)
		return nil
	case "matchlist":
		if len(in.Args) != 1 {
			return fmt.Errorf("native C backend: list match expects one subject")
		}
		subject, err := name(in.Args[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  %s = olistis(%s, %du);\n", vars[in.Dest], subject, in.Value.u)
		return nil
	case "listitem":
		if len(in.Args) != 1 {
			return fmt.Errorf("native C backend: list item read expects one subject")
		}
		subject, err := name(in.Args[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  %s = olistitem(%s, %du);\n", vars[in.Dest], subject, in.Value.u)
		return nil
	case "matchtype":
		if len(in.Args) != 1 {
			return fmt.Errorf("native C backend: type match expects one subject")
		}
		subject, err := name(in.Args[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  %s = omatchtype(%s, \"%s\");\n", vars[in.Dest], subject, cEscapeString(in.Text))
		return nil
	case "asserttype":
		if len(in.Args) != 1 {
			return fmt.Errorf("native C backend: binding type check expects one value")
		}
		value, err := name(in.Args[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  %s = oasserttype(%s, \"%s\");\n", vars[in.Dest], value, cEscapeString(in.Text))
		return nil
	case "matchfail":
		fmt.Fprintf(b, "  %s = ofail(\"%s\");\n", vars[in.Dest], cEscapeString(in.Text))
		return nil
	case "async":
		switch in.Text {
		case "spawn":
			if len(in.Args) != 1 {
				return fmt.Errorf("native C backend: async.spawn expects one thunk")
			}
			fmt.Fprintf(b, "  %s = otask(%s);\n", vars[in.Dest], vars[in.Args[0]])
		case "await":
			if len(in.Args) != 1 {
				return fmt.Errorf("native C backend: async.await expects one task")
			}
			fmt.Fprintf(b, "  %s = oawait(%s);\n", vars[in.Dest], vars[in.Args[0]])
		case "cancel":
			if len(in.Args) != 1 {
				return fmt.Errorf("native C backend: async.cancel expects one task")
			}
			fmt.Fprintf(b, "  %s = ocancel(%s);\n", vars[in.Dest], vars[in.Args[0]])
		case "parallel":
			fmt.Fprintf(b, "  %s = oparallel((OValue[]){", vars[in.Dest])
			for i, arg := range in.Args {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(vars[arg])
			}
			fmt.Fprintf(b, "}, %d);\n", len(in.Args))
		default:
			return fmt.Errorf("native C backend: unsupported async operation %q", in.Text)
		}
		return nil
	case "try":
		if len(in.Args) != 2 {
			return fmt.Errorf("native C backend: try expects body and handler thunks")
		}
		fmt.Fprintf(b, "  %s = otry(%s, %s);\n", vars[in.Dest], vars[in.Args[0]], vars[in.Args[1]])
		return nil
	case "resultbind":
		if len(in.Args) != 1 {
			return fmt.Errorf("native C backend: resultbind expects one result or option")
		}
		fmt.Fprintf(b, "  %s = oresultbind(%s);\n", vars[in.Dest], vars[in.Args[0]])
		return nil
	case "listatunchecked":
		if len(in.Args) != 2 {
			return fmt.Errorf("native C backend: unchecked list access expects list and index")
		}
		list, err := name(in.Args[0])
		if err != nil {
			return err
		}
		index, err := name(in.Args[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  %s = olistatunchecked(%s, %s);\n", vars[in.Dest], list, index)
		return nil
	case "call":
		fn := bindings[in.Text]
		builtin := builtins[in.Text]
		if builtin == "" {
			builtin = in.Text
		}
		if builtin == "ffi_load" {
			if len(in.Args) != 1 {
				return fmt.Errorf("native C backend: ffi_load expects one manifest path")
			}
			if libraryPath := ffiPlan.loads[ffiPlan.loadKey(currentFunction, in.Dest)]; libraryPath != "" {
				fmt.Fprintf(b, "  %s = offi_load(ostring(\"%s\"));\n", vars[in.Dest], cEscapeString(libraryPath))
			} else {
				return fmt.Errorf("native C backend: ffi_load manifest was not resolved at compile time")
			}
			return nil
		}
		if builtin == "ffi_close" {
			if len(in.Args) != 1 {
				return fmt.Errorf("native C backend: ffi_close expects one library")
			}
			fmt.Fprintf(b, "  %s = offi_close(&%s);\n", vars[in.Dest], vars[in.Args[0]])
			return nil
		}
		if builtin == "ffi_const" {
			if len(in.Args) != 2 {
				return fmt.Errorf("native C backend: ffi_const expects a library and constant name")
			}
			constant, ok := ffiPlan.constants[ffiPlan.callKey(currentFunction, in.Args[0], in.Args[1])]
			if !ok {
				return fmt.Errorf("native C backend: ffi_const member is not a declared manifest constant")
			}
			literal, err := cFFIConstantLiteral(constant)
			if err != nil {
				return err
			}
			fmt.Fprintf(b, "  %s = %s;\n", vars[in.Dest], literal)
			return nil
		}
		if builtin == "ffi_call" {
			if len(in.Args) < 2 {
				return fmt.Errorf("native C backend: ffi_call expects a library and declared function name")
			}
			declaration, ok := ffiPlan.calls[ffiPlan.callKey(currentFunction, in.Args[0], in.Args[1])]
			if !ok {
				return fmt.Errorf("native C backend: ffi_call member is not a declared manifest function")
			}
			args := in.Args[2:]
			if len(args) != len(declaration.Args) {
				return fmt.Errorf("native C backend: ffi %s expects %d argument(s), got %d", declaration.Name, len(declaration.Args), len(args))
			}
			if declaration.CallingConvention != "c" {
				return fmt.Errorf("native C backend: ffi %s requires the C calling convention", declaration.Name)
			}
			if ffiScalarSignature(&declaration) {
				if declaration.Result == "f32" || declaration.Result == "f64" {
					fmt.Fprintf(b, "  %s = offi_f64(%s, \"%s\", ", vars[in.Dest], vars[in.Args[0]], cEscapeString(declaration.Symbol))
				} else {
					if declaration.Result == "void" {
						fmt.Fprintf(b, "  %s = offi_void(%s, \"%s\", %d, ", vars[in.Dest], vars[in.Args[0]], cEscapeString(declaration.Symbol), len(args))
						writeCValueArray(b, args, vars)
						b.WriteString(");\n")
						return nil
					}
					kind, bits, err := cFFIIntegerResultSpec(declaration.Result)
					if err != nil {
						return err
					}
					fmt.Fprintf(b, "  %s = offi_i64(%s, \"%s\", %d, %d, %d, ", vars[in.Dest], vars[in.Args[0]], cEscapeString(declaration.Symbol), kind, bits, len(args))
				}
				writeCValueArray(b, args, vars)
				if declaration.Result == "f32" || declaration.Result == "f64" {
					bits := 64
					if declaration.Result == "f32" {
						bits = 32
					}
					fmt.Fprintf(b, ", %d);\n", bits)
				} else {
					b.WriteString(");\n")
				}
				return nil
			}
			if ffiStringSignature(&declaration) {
				fmt.Fprintf(b, "  %s = offi_string(%s, \"%s\", ", vars[in.Dest], vars[in.Args[0]], cEscapeString(declaration.Symbol))
				writeCValueArray(b, args, vars)
				fmt.Fprintf(b, ", %d);\n", len(args))
				return nil
			}
			if ffiConstBufferSignature(&declaration) {
				kind, bits, err := cFFIIntegerResultSpec(declaration.Result)
				if err != nil {
					return err
				}
				fmt.Fprintf(b, "  %s = offi_const_buffer(%s, \"%s\", %d, %d, ", vars[in.Dest], vars[in.Args[0]], cEscapeString(declaration.Symbol), kind, bits)
				writeCValueArray(b, args, vars)
				fmt.Fprintf(b, ", %d);\n", len(args))
				return nil
			}
			if ffiMutableBufferSignature(&declaration) {
				kind, bits, err := cFFIIntegerResultSpec(declaration.Result)
				if err != nil {
					return err
				}
				fmt.Fprintf(b, "  %s = offi_mutable_buffer(%s, \"%s\", %d, %d, %s);\n", vars[in.Dest], vars[in.Args[0]], cEscapeString(declaration.Symbol), kind, bits, vars[args[0]])
				return nil
			}
			return fmt.Errorf("native C backend: ffi %s uses an ABI shape not supported by the generated-C adapter", declaration.Name)
		}
		genericBuiltin := builtin
		if module, member, ok := strings.Cut(builtin, "."); ok && (module == "std/os" || module == "std/net") {
			if member == "readchunk" || member == "writechunk" || member == "close" {
				genericBuiltin = member
			}
		}
		if genericBuiltin == "readchunk" || genericBuiltin == "writechunk" || genericBuiltin == "close" {
			if len(in.Args) == 0 {
				return diagnostic("%s requires a resource producer that can be proven statically", genericBuiltin)
			}
			capability, producer, known := nativeGenericResourceCapability(genericBuiltin, in.Args[0], resourceProducers)
			if !known {
				return diagnostic("%s resource producer for %q cannot be proven statically", genericBuiltin, in.Args[0])
			}
			if !nativeCapabilityGranted(ffiPlan, capability) {
				return diagnostic("native capability %q is not granted for %s producer %q", capability, genericBuiltin, producer)
			}
		} else if capability := nativeBuiltinCapability(builtin); capability != "" && !nativeCapabilityGranted(ffiPlan, capability) {
			return fmt.Errorf("native C backend: native capability %q is not granted for %s", capability, builtin)
		}
		if fn == nil && builtin == "map_new" {
			fmt.Fprintf(b, "  %s = omapnew((OValue[]){", vars[in.Dest])
			for i, arg := range in.Args {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(vars[arg])
			}
			fmt.Fprintf(b, "}, %d);\n", len(in.Args))
			return nil
		}
		resourceArgs := make([]string, len(in.Args))
		for index, argument := range in.Args {
			value, err := name(argument)
			if err != nil {
				return err
			}
			resourceArgs[index] = value
		}
		if emitted, err := emitNativeResourceCall(b, vars[in.Dest], builtin, resourceArgs); emitted {
			return err
		}
		if nativeComplexMathBuiltin(builtin) {
			for _, arg := range in.Args {
				if complexValues[arg] {
					return fmt.Errorf("native C backend: complex %s is not supported yet", builtin)
				}
			}
		}
		if fn == nil {
			if strings.HasPrefix(builtin, "std/") && !strings.HasPrefix(builtin, "std/math.") {
				module, member, ok := strings.Cut(builtin, ".")
				if ok {
					if _, supported := nativeStaticModuleMember(module, member); supported {
						if capability := nativeModuleCapability(module, member); !nativeCapabilityGranted(ffiPlan, capability) {
							return fmt.Errorf("native C backend: native capability %q is not granted for %s.%s", capability, module, member)
						}
						fmt.Fprintf(b, "  %s = ostd_dispatch(\"%s\", \"%s\", (OValue[]){", vars[in.Dest], cEscapeString(module), cEscapeString(member))
						for i, arg := range in.Args {
							if i > 0 {
								b.WriteString(", ")
							}
							b.WriteString(vars[arg])
						}
						fmt.Fprintf(b, "}, %d);\n", len(in.Args))
						return nil
					}
				}
			}
			switch builtin {
			case "std/crypto.random":
				if len(in.Args) != 2 {
					return fmt.Errorf("native C backend: std/crypto.random expects 2 arguments")
				}
				fmt.Fprintf(b, "  %s = ocrypto_random(%s, %s);\n", vars[in.Dest], vars[in.Args[0]], vars[in.Args[1]])
				return nil
			case "std/crypto.hash":
				fmt.Fprintf(b, "  %s = ocrypto_hash((OValue[]){", vars[in.Dest])
				for i, arg := range in.Args {
					if i > 0 {
						b.WriteString(", ")
					}
					b.WriteString(vars[arg])
				}
				fmt.Fprintf(b, "}, %d);\n", len(in.Args))
				return nil
			case "std/list.range", "std/signal.hann", "std/text.slug", "std/text.fields",
				"std/text.upper", "std/time.parse_duration", "std/time.duration_string":
				if len(in.Args) != 1 {
					return fmt.Errorf("native C backend: %s expects 1 argument", builtin)
				}
				helper := strings.NewReplacer("/", "_", ".", "_").Replace(builtin)
				fmt.Fprintf(b, "  %s = o%s(%s);\n", vars[in.Dest], helper, vars[in.Args[0]])
				return nil
			case "std/signal.convolve", "std/text.join", "std/time.format", "std/time.add", "std/time.sub":
				if len(in.Args) != 2 {
					return fmt.Errorf("native C backend: %s expects 2 arguments", builtin)
				}
				helper := strings.NewReplacer("/", "_", ".", "_").Replace(builtin)
				fmt.Fprintf(b, "  %s = o%s(%s, %s);\n", vars[in.Dest], helper, vars[in.Args[0]], vars[in.Args[1]])
				return nil
			case "std/time.add_date":
				if len(in.Args) != 4 {
					return fmt.Errorf("native C backend: std/time.add_date expects 4 arguments")
				}
				fmt.Fprintf(b, "  %s = ostd_time_add_date(%s, %s, %s, %s);\n", vars[in.Dest], vars[in.Args[0]], vars[in.Args[1]], vars[in.Args[2]], vars[in.Args[3]])
				return nil
			case "std/time.date":
				if len(in.Args) != 8 {
					return fmt.Errorf("native C backend: std/time.date expects 8 arguments")
				}
				fmt.Fprintf(b, "  %s = ostd_time_date((OValue[]){", vars[in.Dest])
				for i, arg := range in.Args {
					if i > 0 {
						b.WriteString(", ")
					}
					b.WriteString(vars[arg])
				}
				b.WriteString("});\n")
				return nil
			case "rotl", "bxor", "band":
				if len(in.Args) != 2 {
					return fmt.Errorf("native C backend: %s expects 2 arguments", builtin)
				}
				helper := map[string]string{"rotl": "orotl", "bxor": "obxor", "band": "oband"}[builtin]
				fmt.Fprintf(b, "  %s = %s(%s, %s);\n", vars[in.Dest], helper, vars[in.Args[0]], vars[in.Args[1]])
				return nil
			case "cat":
				fmt.Fprintf(b, "  %s = ocat((OValue[]){", vars[in.Dest])
				for i, arg := range in.Args {
					if i > 0 {
						b.WriteString(", ")
					}
					b.WriteString(vars[arg])
				}
				fmt.Fprintf(b, "}, %d);\n", len(in.Args))
				return nil
			case "dec", "bigint", "ok", "err", "some", "none", "encode", "decode":
				if builtin == "none" {
					if len(in.Args) != 0 {
						return fmt.Errorf("native C backend: none expects 0 arguments")
					}
					fmt.Fprintf(b, "  %s = ooptionNone();\n", vars[in.Dest])
					return nil
				}
				if len(in.Args) != 1 {
					return fmt.Errorf("native C backend: %s expects 1 argument", builtin)
				}
				helper := map[string]string{
					"dec": "odecimal", "bigint": "obigint", "ok": "oresultok",
					"err": "oresultfail", "some": "ooptionSome", "encode": "oencode", "decode": "odecode",
				}[builtin]
				fmt.Fprintf(b, "  %s = %s(%s);\n", vars[in.Dest], helper, vars[in.Args[0]])
				return nil
			case "error":
				if len(in.Args) != 2 {
					return fmt.Errorf("native C backend: error expects 2 arguments")
				}
				fmt.Fprintf(b, "  %s = oerror(%s, %s);\n", vars[in.Dest], vars[in.Args[0]], vars[in.Args[1]])
				return nil
			case "issome", "isnone":
				if len(in.Args) != 1 {
					return fmt.Errorf("native C backend: %s expects 1 argument", builtin)
				}
				if builtin == "issome" {
					fmt.Fprintf(b, "  %s = obool(%s.kind == O_OPTION && %s.fbits != 0);\n", vars[in.Dest], vars[in.Args[0]], vars[in.Args[0]])
				} else {
					fmt.Fprintf(b, "  %s = obool(%s.kind == O_OPTION && %s.fbits == 0);\n", vars[in.Dest], vars[in.Args[0]], vars[in.Args[0]])
				}
				return nil
			case "to_option", "to_result", "unwrap", "expect", "or_else", "collect":
				if builtin == "to_option" || builtin == "unwrap" || builtin == "collect" {
					if len(in.Args) != 1 {
						return fmt.Errorf("native C backend: %s expects 1 argument", builtin)
					}
				} else if builtin == "to_result" || builtin == "expect" || builtin == "or_else" {
					if len(in.Args) != 2 && !(builtin == "to_result" && len(in.Args) == 1) {
						return fmt.Errorf("native C backend: %s expects 2 arguments", builtin)
					}
				}
				switch builtin {
				case "to_option":
					fmt.Fprintf(b, "  %s = otoOption(%s);\n", vars[in.Dest], vars[in.Args[0]])
				case "to_result":
					message := "onull()"
					if len(in.Args) == 2 {
						message = vars[in.Args[1]]
					}
					fmt.Fprintf(b, "  %s = otoResult(%s, %s);\n", vars[in.Dest], vars[in.Args[0]], message)
				case "unwrap":
					fmt.Fprintf(b, "  %s = ounwrap(%s);\n", vars[in.Dest], vars[in.Args[0]])
				case "expect":
					fmt.Fprintf(b, "  %s = oexpect(%s, %s);\n", vars[in.Dest], vars[in.Args[0]], vars[in.Args[1]])
				case "or_else":
					fmt.Fprintf(b, "  %s = oorElse(%s, %s);\n", vars[in.Dest], vars[in.Args[0]], vars[in.Args[1]])
				case "collect":
					fmt.Fprintf(b, "  %s = ocollect(%s);\n", vars[in.Dest], vars[in.Args[0]])
				}
				return nil
			}
		}
		if fn == nil && (builtin == "var" || builtin == "median" || builtin == "percentile" || builtin == "argmin" || builtin == "argmax" || builtin == "any" || builtin == "all" || builtin == "cumsum" || builtin == "cumprod") {
			expected := 1
			if builtin == "percentile" {
				expected = 2
			}
			if len(in.Args) != expected {
				return fmt.Errorf("native C backend: %s expects %d argument(s)", builtin, expected)
			}
			helper := map[string]string{"var": "ovar", "median": "omedian", "percentile": "opercentile", "argmin": "oargmin", "argmax": "oargmax", "any": "oany", "all": "oall", "cumsum": "ocumsum", "cumprod": "ocumprod"}[builtin]
			fmt.Fprintf(b, "  %s = %s(", vars[in.Dest], helper)
			for i, arg := range in.Args {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(vars[arg])
			}
			b.WriteString(");\n")
			return nil
		}
		if fn == nil && (builtin == "dual" || builtin == "primal" || builtin == "tangent") {
			if builtin == "dual" {
				if len(in.Args) != 2 {
					return fmt.Errorf("native C backend: dual expects 2 arguments")
				}
				fmt.Fprintf(b, "  %s = odualcall((OValue[]){%s, %s}, 2);\n", vars[in.Dest], vars[in.Args[0]], vars[in.Args[1]])
				return nil
			}
			if len(in.Args) != 1 {
				return fmt.Errorf("native C backend: %s expects 1 argument", builtin)
			}
			helper := map[string]string{"primal": "oprimal", "tangent": "otangent"}[builtin]
			fmt.Fprintf(b, "  %s = %s(%s);\n", vars[in.Dest], helper, vars[in.Args[0]])
			return nil
		}
		if fn == nil && (builtin == "abs" || builtin == "uint" || builtin == "byte" || builtin == "bytes" || builtin == "u8" || builtin == "u16" || builtin == "u32" || builtin == "u64" || builtin == "i8" || builtin == "i16" || builtin == "i32" || builtin == "i64" || builtin == "f32" || builtin == "f64" || builtin == "number" || builtin == "complex" || builtin == "real" || builtin == "imag" || builtin == "conjugate" || builtin == "magnitude" || builtin == "sqrt" || builtin == "exp" || builtin == "pow" || builtin == "expt" || builtin == "log" || builtin == "log10" || builtin == "log2" || builtin == "sin" || builtin == "cos" || builtin == "tan" || builtin == "asin" || builtin == "acos" || builtin == "atan" || builtin == "sinh" || builtin == "cosh" || builtin == "tanh" || builtin == "asinh" || builtin == "acosh" || builtin == "atanh" || builtin == "expm1" || builtin == "log1p" || builtin == "cbrt" || builtin == "erf" || builtin == "erfc" || builtin == "gamma" || builtin == "lgamma" || builtin == "floor" || builtin == "ceil" || builtin == "round" || builtin == "hypot" || builtin == "min" || builtin == "max" || builtin == "clamp" || builtin == "mean" || builtin == "std" || builtin == "quantile" || builtin == "phase" || builtin == "vector" || builtin == "vec" || builtin == "zeros" || builtin == "ones" || builtin == "full" || builtin == "identity" || builtin == "eye" || builtin == "arange" || builtin == "linspace" || builtin == "logspace" || builtin == "len" || builtin == "take" || builtin == "slice" || builtin == "at" || builtin == "setat" || builtin == "tensor" || builtin == "matrix" || builtin == "reshape" || builtin == "transpose" || builtin == "matmul" || builtin == "outer" || builtin == "trace" || builtin == "det" || builtin == "solve" || builtin == "inv" || builtin == "cholesky" || builtin == "qr" || builtin == "eig" || builtin == "eigh" || builtin == "svd" || builtin == "shape" || builtin == "flatten" || builtin == "sum" || builtin == "product" || builtin == "dot" || builtin == "norm") {
			expected := 1
			if builtin == "at" || builtin == "take" || builtin == "dot" || builtin == "reshape" || builtin == "matmul" || builtin == "outer" || builtin == "solve" {
				expected = 2
			}
			if builtin == "slice" {
				expected = 3
			}
			if builtin == "clamp" {
				expected = 3
			}
			if builtin == "quantile" {
				expected = 2
			}
			if builtin == "setat" {
				expected = 3
			}
			if builtin == "matrix" {
				expected = 1
			}
			if builtin == "complex" {
				expected = 2
			}
			if builtin == "log" && len(in.Args) == 2 {
				expected = 2
			}
			if builtin == "pow" || builtin == "expt" {
				expected = 2
			}
			if len(in.Args) != expected {
				if builtin == "hypot" && len(in.Args) >= 2 {
					// hypot is intentionally variadic.
				} else if builtin == "vector" || builtin == "vec" {
					// vector construction accepts zero or more values.
				} else if (builtin == "zeros" || builtin == "ones") && len(in.Args) >= 1 {
					// Fill constructors accept one shape list or one or more dimensions.
				} else if builtin == "full" && len(in.Args) >= 2 {
					// full accepts one shape list or one or more dimensions plus fill.
				} else if (builtin == "identity" || builtin == "eye") && (len(in.Args) == 1 || len(in.Args) == 2) {
					// identity constructors accept a square size or rows and columns.
				} else if builtin == "complex" && (len(in.Args) == 1 || len(in.Args) == 2) {
					// complex accepts a complex-compatible value or real and imaginary parts.
				} else if builtin == "tensor" && (len(in.Args) == 1 || len(in.Args) == 2) {
					// tensor accepts nested data, optionally preceded by a shape.
				} else if builtin == "matrix" && len(in.Args) >= 1 && len(in.Args) <= 3 {
					// matrix accepts nested data, shape/data, or rows/columns/data.
				} else if builtin == "arange" && len(in.Args) >= 1 && len(in.Args) <= 3 {
					// arange accepts stop, start/stop, or start/stop/step.
				} else if (builtin == "min" || builtin == "max") && len(in.Args) >= 1 {
					// min and max are variadic numeric folds.
				} else if builtin == "linspace" && (len(in.Args) == 2 || len(in.Args) == 3) {
					// linspace accepts start/stop with an optional count.
				} else if builtin == "logspace" && (len(in.Args) == 3 || len(in.Args) == 4) {
					// logspace accepts start/stop/count with an optional base.
				} else if builtin == "slice" && len(in.Args) == 4 {
					// slice accepts an optional axis after start and end.
				} else {
					return fmt.Errorf("native C backend: %s expects %d argument(s)", builtin, expected)
				}
			}
			if builtin == "hypot" {
				fmt.Fprintf(b, "  %s = ohypot((OValue[]){", vars[in.Dest])
				for i, arg := range in.Args {
					if i > 0 {
						b.WriteString(", ")
					}
					b.WriteString(vars[arg])
				}
				fmt.Fprintf(b, "}, %d);\n", len(in.Args))
				return nil
			}
			if builtin == "asin" || builtin == "acos" || builtin == "atan" || builtin == "sinh" || builtin == "cosh" || builtin == "tanh" || builtin == "asinh" || builtin == "acosh" || builtin == "atanh" {
				// The scalar specializer admits the characterized f32 subset. Do
				// not route every other call through platform libm: Go's f64
				// implementation is not an exact cross-platform C-libm contract.
				return fmt.Errorf("native C backend: %s requires a proven f32 scalar input; f64 libm lowering is not parity-safe", builtin)
			}
			if builtin == "log" && len(in.Args) == 2 {
				// Go and C libm can differ in the final f64 bit after the two
				// logarithms and division. The scalar specializer proves the f32
				// rounding boundary; every other base-log call must reject rather
				// than use ologbase's incompatible host result.
				return fmt.Errorf("native C backend: two-argument log requires proven f32 scalar inputs; f64 libm lowering is not parity-safe")
			}
			if builtin == "gamma" || builtin == "lgamma" {
				// The scalar specializer admits only the characterized f32
				// narrowing boundary. Do not silently use the tagged C helper for
				// f64: Go and platform libm have no bit-exact shared contract.
				return fmt.Errorf("native C backend: %s requires a proven f32 scalar input; f64 libm lowering is not parity-safe", builtin)
			}
			if builtin == "pow" || builtin == "expt" {
				// Exact integer inputs have a different result kind, while f64
				// power relies on a libm contract that is not bit-exact with Go.
				// The scalar specializer admits only proven f32 operands.
				return fmt.Errorf("native C backend: %s requires proven f32 scalar inputs; exact and f64 lowering is not parity-safe", builtin)
			}
			if builtin == "min" || builtin == "max" {
				fmt.Fprintf(b, "  %s = ominmax((OValue[]){", vars[in.Dest])
				for i, arg := range in.Args {
					if i > 0 {
						b.WriteString(", ")
					}
					b.WriteString(vars[arg])
				}
				fmt.Fprintf(b, "}, %d, %d);\n", len(in.Args), map[string]int{"min": 0, "max": 1}[builtin])
				return nil
			}
			if builtin == "zeros" || builtin == "ones" || builtin == "full" {
				helper := map[string]string{"zeros": "ozeros", "ones": "oones", "full": "ofull"}[builtin]
				fmt.Fprintf(b, "  %s = %s((OValue[]){", vars[in.Dest], helper)
				for i, arg := range in.Args {
					if i > 0 {
						b.WriteString(", ")
					}
					b.WriteString(vars[arg])
				}
				fmt.Fprintf(b, "}, %d);\n", len(in.Args))
				return nil
			}
			if builtin == "identity" || builtin == "eye" {
				fmt.Fprintf(b, "  %s = oidentity((OValue[]){", vars[in.Dest])
				for i, arg := range in.Args {
					if i > 0 {
						b.WriteString(", ")
					}
					b.WriteString(vars[arg])
				}
				fmt.Fprintf(b, "}, %d);\n", len(in.Args))
				return nil
			}
			if builtin == "eigh" {
				if !hasProvider {
					return fmt.Errorf("native C backend: eigh requires a configured native numerical provider")
				}
				fmt.Fprintf(b, "  %s = oeigh(%s);\n", vars[in.Dest], vars[in.Args[0]])
				return nil
			}
			if builtin == "svd" {
				if !hasProvider {
					return fmt.Errorf("native C backend: svd requires a configured native numerical provider")
				}
				fmt.Fprintf(b, "  %s = osvd(%s);\n", vars[in.Dest], vars[in.Args[0]])
				return nil
			}
			if builtin == "eig" {
				if !hasProvider {
					return fmt.Errorf("native C backend: eig requires a configured ABI v2 native numerical provider")
				}
				fmt.Fprintf(b, "  %s = oeig(%s);\n", vars[in.Dest], vars[in.Args[0]])
				return nil
			}
			if builtin == "slice" && len(in.Args) == 4 {
				fmt.Fprintf(b, "  %s = osliceaxis(%s, %s, %s, %s);\n", vars[in.Dest], vars[in.Args[0]], vars[in.Args[1]], vars[in.Args[2]], vars[in.Args[3]])
				return nil
			}
			if builtin == "tensor" || builtin == "matrix" {
				helper := map[string]string{"tensor": "otensorcall", "matrix": "omatrixcall"}[builtin]
				fmt.Fprintf(b, "  %s = %s((OValue[]){", vars[in.Dest], helper)
				for i, arg := range in.Args {
					if i > 0 {
						b.WriteString(", ")
					}
					b.WriteString(vars[arg])
				}
				fmt.Fprintf(b, "}, %d);\n", len(in.Args))
				return nil
			}
			if builtin == "vector" || builtin == "vec" {
				if len(in.Args) == 0 {
					fmt.Fprintf(b, "  %s = ovector(NULL, 0);\n", vars[in.Dest])
					return nil
				}
				fmt.Fprintf(b, "  %s = ovector((OValue[]){", vars[in.Dest])
				for i, arg := range in.Args {
					if i > 0 {
						b.WriteString(", ")
					}
					b.WriteString(vars[arg])
				}
				fmt.Fprintf(b, "}, %d);\n", len(in.Args))
				return nil
			}
			if builtin == "arange" || builtin == "linspace" || builtin == "logspace" {
				helper := map[string]string{"arange": "oarange", "linspace": "olinspace", "logspace": "ologspace"}[builtin]
				fmt.Fprintf(b, "  %s = %s((OValue[]){", vars[in.Dest], helper)
				for i, arg := range in.Args {
					if i > 0 {
						b.WriteString(", ")
					}
					b.WriteString(vars[arg])
				}
				fmt.Fprintf(b, "}, %d);\n", len(in.Args))
				return nil
			}
			if builtin == "complex" {
				fmt.Fprintf(b, "  %s = ocomplexcall((OValue[]){", vars[in.Dest])
				for i, arg := range in.Args {
					if i > 0 {
						b.WriteString(", ")
					}
					b.WriteString(vars[arg])
				}
				fmt.Fprintf(b, "}, %d);\n", len(in.Args))
				return nil
			}
			helper := map[string]string{"abs": "oabs", "uint": "ouintcast", "byte": "obytecast", "bytes": "obytes", "u8": "ou8cast", "u16": "ou16cast", "u32": "ou32cast", "u64": "ou64cast", "i8": "oi8cast", "i16": "oi16cast", "i32": "oi32cast", "i64": "oi64cast", "f32": "of32cast", "f64": "of64cast", "number": "onumbercast", "real": "oreal", "imag": "oimag", "conjugate": "oconjugate", "magnitude": "omagnitude", "sqrt": "osqrt", "exp": "oexp", "log": "olog", "log10": "olog10", "log2": "olog2", "sin": "osin", "cos": "ocos", "tan": "otan", "asin": "oasin", "acos": "oacos", "atan": "oatan", "sinh": "osinh", "cosh": "ocosh", "tanh": "otanh", "asinh": "oasinh", "acosh": "oacosh", "atanh": "oatanh", "expm1": "oexpm1", "log1p": "olog1p", "cbrt": "ocbrt", "erf": "oerf", "erfc": "oerfc", "gamma": "ogamma", "lgamma": "olgamma", "floor": "ofloor", "ceil": "oceil", "round": "oround", "clamp": "oclamp", "mean": "omean", "std": "ostd", "quantile": "oquantile", "phase": "ophase", "len": "olen", "take": "otake", "slice": "oslice", "at": "oat", "setat": "osetat", "reshape": "oreshape", "transpose": "otranspose", "matmul": "omatmul", "outer": "oouter", "trace": "otrace", "det": "odet", "solve": "osolve", "inv": "oinv", "cholesky": "ocholesky", "qr": "oqr", "shape": "oshape", "flatten": "oflatten", "sum": "otensorsum", "product": "otensorproduct", "dot": "otensordot", "norm": "otensornorm"}[builtin]
			if builtin == "log" && len(in.Args) == 2 {
				helper = "ologbase"
			}
			fmt.Fprintf(b, "  %s = %s(", vars[in.Dest], helper)
			for i, arg := range in.Args {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(vars[arg])
			}
			b.WriteString(");\n")
			return nil
		}
		if fn == nil && builtins[in.Text] != "" {
			if builtin == "eigh" || builtin == "svd" {
				if hasProvider {
					return fmt.Errorf("native C backend: %s provider adapter is not implemented yet", builtin)
				}
				return fmt.Errorf("native C backend: %s requires a configured native numerical provider", builtin)
			}
			if builtin == "eig" {
				return fmt.Errorf("native C backend: eig requires a configured ABI v2 native numerical provider")
			}
			return fmt.Errorf("native C backend: builtin %q is not supported yet", builtins[in.Text])
		}
		if fn == nil {
			callee, err := name(in.Text)
			if err != nil {
				return fmt.Errorf("native C backend: unsupported call %q", in.Text)
			}
			if len(in.Args) == 0 {
				fmt.Fprintf(b, "  %s = ocall(%s, NULL, 0);\n", vars[in.Dest], callee)
				return nil
			}
			fmt.Fprintf(b, "  %s = ocall(%s, (OValue[]){", vars[in.Dest], callee)
			for i, arg := range in.Args {
				if i > 0 {
					b.WriteString(", ")
				}
				v, err := name(arg)
				if err != nil {
					return err
				}
				b.WriteString(v)
			}
			fmt.Fprintf(b, "}, %d);\n", len(in.Args))
			return nil
		}
		if len(fn.Params) != len(in.Args) {
			return fmt.Errorf("native C backend: call %q has %d argument(s), want %d", in.Text, len(in.Args), len(fn.Params))
		}
		fmt.Fprintf(b, "  %s = %s(", vars[in.Dest], functionNames[fn.Name])
		for i, arg := range in.Args {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(vars[arg])
		}
		target := fn
		for i, capture := range closureArgs[in.Text] {
			if len(in.Args) > 0 || i > 0 {
				b.WriteString(", ")
			}
			v, err := name(capture)
			if err != nil {
				return err
			}
			if target != nil && i < len(target.MutableCaptures) && target.MutableCaptures[i] {
				cell := cellByValue[capture]
				if cell == "" && i < len(target.Captures) {
					cell = mutableCells[target.Captures[i]]
				}
				if cell == "" {
					captureName := fmt.Sprintf("capture %d", i)
					if i < len(target.Captures) {
						captureName = target.Captures[i]
					}
					return diagnostic("native C backend: mutable capture %q has no stable cell", captureName)
				}
				v = cell
			}
			b.WriteString(v)
		}
		b.WriteString(");\n")
		return nil
	default:
		return fmt.Errorf("native C backend: unsupported IR operation %q", in.Op)
	}
	return nil
}

func irFunctionSignature(fn *IRFunction) string {
	if fn == nil {
		return ""
	}
	hasType := fn.ResultType != ""
	params := make([]staticType, len(fn.Params))
	for i := range params {
		params[i] = dynamicStaticType
		if i < len(fn.ParamTypes) && fn.ParamTypes[i] != "" {
			if parsed, err := parseStaticType(fn.ParamTypes[i]); err == nil {
				params[i] = parsed
				hasType = true
			}
		}
	}
	result := dynamicStaticType
	if fn.ResultType != "" {
		if parsed, err := parseStaticType(fn.ResultType); err == nil {
			result = parsed
		}
	}
	if !hasType {
		return ""
	}
	return staticType{name: "func", params: params, result: &result}.String()
}

func emitCTerm(b *strings.Builder, term IRTerminator, blocks []*IRBlock, vars map[string]string, main bool, cellByValue map[string]string) error {
	valueName := func(value string) string {
		name := vars[value]
		if cellByValue[value] != "" {
			return fmt.Sprintf("ocellvalue(%s)", name)
		}
		return name
	}
	lookup := func(label string) *IRBlock {
		for _, block := range blocks {
			if block.Name == label {
				return block
			}
		}
		return nil
	}
	jump := func(label string, args []string) error {
		target := lookup(label)
		if target == nil || len(target.Params) != len(args) {
			return fmt.Errorf("native C backend: invalid jump to %q", label)
		}
		for i, arg := range args {
			fmt.Fprintf(b, "  %s = %s;\n", vars[target.Params[i]], valueName(arg))
		}
		fmt.Fprintf(b, "  goto %s;\n", cLabel(label))
		return nil
	}
	switch t := term.(type) {
	case *irJump:
		return jump(t.Target, t.Args)
	case *irBranch:
		fmt.Fprintf(b, "  if (otruthy(%s)) {\n", valueName(t.Cond))
		if err := jump(t.TrueTarget, t.TrueArgs); err != nil {
			return err
		}
		b.WriteString("  } else {\n")
		if err := jump(t.FalseTarget, t.FalseArgs); err != nil {
			return err
		}
		b.WriteString("  }\n")
	case *irReturn:
		if main {
			b.WriteString("  return 0;\n")
		} else if t.HasValue {
			fmt.Fprintf(b, "  return %s;\n", valueName(t.Value))
		} else {
			b.WriteString("  return onull();\n")
		}
	default:
		return fmt.Errorf("native C backend: unsupported terminator %T", term)
	}
	return nil
}

func cLabel(s string) string { return "block_" + strings.NewReplacer(".", "_", "-", "_").Replace(s) }

func cLiteral(v Value) (string, error) {
	switch v.kind {
	case kindNull:
		return "onull()", nil
	case kindBool:
		return fmt.Sprintf("obool(%d)", map[bool]int{false: 0, true: 1}[v.boo]), nil
	case kindUint:
		if v.ub != nil || v.ubits > 64 {
			return "", fmt.Errorf("native C backend: wide unsigned literals are not supported")
		}
		return fmt.Sprintf("ouintbits(%duLL, %d)", v.u, nativeUintWidth(v)), nil
	case kindInt:
		if v.i == nil || !v.i.IsInt64() {
			return "", fmt.Errorf("native C backend: wide signed literals are not supported")
		}
		return fmt.Sprintf("ointbits(%dLL, %d)", v.i.Int64(), nativeIntWidth(v)), nil
	case kindNumber:
		if v.fbits != 0 {
			return fmt.Sprintf("ofloatbits(%.17g, %d)", v.num, v.fbits), nil
		}
		return fmt.Sprintf("ofloat(%.17g)", v.num), nil
	case kindComplex:
		if v.fbits == 32 {
			return "", fmt.Errorf("native C backend: complex<f32> lowering is not implemented; use complex<f64> or the default complex family")
		}
		return fmt.Sprintf("ocomplex(%.17g, %.17g)", real(v.cx), imag(v.cx)), nil
	case kindString:
		return fmt.Sprintf("ostringview(\"%s\", %d)", cEscapeString(v.str), len(v.str)), nil
	case kindQuantity:
		return "oquantityunit(\"" + cEscapeString(v.String()) + "\")", nil
	default:
		return "", fmt.Errorf("native C backend: unsupported literal kind %s", v.Kind())
	}
}

func nativeUintWidth(v Value) uint16 {
	if v.bits == 0 {
		return 64
	}
	return uint16(v.bits)
}

func nativeIntWidth(v Value) uint16 {
	if v.ibits == 0 {
		return 64
	}
	return v.ibits
}

// nativeScalarKind is the closed, allocation-free subset which can be emitted
// directly as C locals. Values outside this subset intentionally use the
// tagged ABI until their ownership and error rules are implemented.
type nativeScalarKind uint8

const (
	nativeScalarUnknown nativeScalarKind = iota
	nativeScalarBool
	nativeScalarUint
	nativeScalarU8
	nativeScalarU16
	nativeScalarU32
	nativeScalarInt
	nativeScalarI8
	nativeScalarI16
	nativeScalarI32
	nativeScalarFloat
	nativeScalarF32
	nativeScalarF64
)

func nativeScalarValueKind(v Value) nativeScalarKind {
	switch v.kind {
	case kindBool:
		return nativeScalarBool
	case kindUint:
		if v.ub == nil && v.ubits <= 64 {
			switch nativeUintWidth(v) {
			case 8:
				return nativeScalarU8
			case 16:
				return nativeScalarU16
			case 32:
				return nativeScalarU32
			case 64:
				return nativeScalarUint
			}
		}
	case kindInt:
		if v.i != nil && v.i.IsInt64() {
			switch nativeIntWidth(v) {
			case 8:
				return nativeScalarI8
			case 16:
				return nativeScalarI16
			case 32:
				return nativeScalarI32
			case 64:
				return nativeScalarInt
			}
		}
	case kindNumber:
		switch v.fbits {
		case 0:
			return nativeScalarFloat
		case 32:
			return nativeScalarF32
		case 64:
			return nativeScalarF64
		}
	}
	return nativeScalarUnknown
}

func nativeScalarUnsignedWidth(kind nativeScalarKind) uint {
	switch kind {
	case nativeScalarU8:
		return 8
	case nativeScalarU16:
		return 16
	case nativeScalarU32:
		return 32
	case nativeScalarUint:
		return 64
	default:
		return 0
	}
}

func nativeScalarIsUnsigned(kind nativeScalarKind) bool {
	return nativeScalarUnsignedWidth(kind) != 0
}

func nativeScalarNarrowUnsignedResult(a, b nativeScalarKind) nativeScalarKind {
	aw, bw := nativeScalarUnsignedWidth(a), nativeScalarUnsignedWidth(b)
	if aw == 0 || bw == 0 || aw >= 64 || bw >= 64 {
		return nativeScalarUnknown
	}
	if aw >= bw {
		return a
	}
	return b
}

func nativeScalarSignedWidth(kind nativeScalarKind) uint {
	switch kind {
	case nativeScalarI8:
		return 8
	case nativeScalarI16:
		return 16
	case nativeScalarI32:
		return 32
	case nativeScalarInt:
		return 64
	default:
		return 0
	}
}

func nativeScalarIsNarrowSigned(kind nativeScalarKind) bool {
	return nativeScalarSignedWidth(kind) != 0 && kind != nativeScalarInt
}

func nativeScalarNarrowSignedResult(a, b nativeScalarKind) nativeScalarKind {
	aw, bw := nativeScalarSignedWidth(a), nativeScalarSignedWidth(b)
	if aw == 0 || bw == 0 || aw >= 64 || bw >= 64 {
		return nativeScalarUnknown
	}
	if aw >= bw {
		return a
	}
	return b
}

func nativeScalarIsFloat(kind nativeScalarKind) bool {
	return kind == nativeScalarFloat || kind == nativeScalarF32 || kind == nativeScalarF64
}

func nativeScalarIsInteger(kind nativeScalarKind) bool {
	return nativeScalarIsUnsigned(kind) || nativeScalarSignedWidth(kind) != 0
}

func nativeScalarFloatResult(a, b nativeScalarKind) nativeScalarKind {
	if a == nativeScalarFloat || b == nativeScalarFloat {
		return nativeScalarFloat
	}
	if a == nativeScalarF64 || b == nativeScalarF64 {
		return nativeScalarF64
	}
	if a == nativeScalarF32 || b == nativeScalarF32 {
		return nativeScalarF32
	}
	return nativeScalarUnknown
}

func nativeScalarComparable(a, b nativeScalarKind) bool {
	if a == b {
		return true
	}
	return nativeScalarIsFloat(a) && nativeScalarIsFloat(b)
}

func nativeScalarResultKind(op string, a, b nativeScalarKind) nativeScalarKind {
	switch op {
	case "==", "!=", "<", "<=", ">", ">=":
		if nativeScalarComparable(a, b) {
			return nativeScalarBool
		}
	case "+", "*":
		if nativeScalarIsFloat(a) || nativeScalarIsFloat(b) {
			return nativeScalarFloatResult(a, b)
		}
		if kind := nativeScalarNarrowUnsignedResult(a, b); kind != nativeScalarUnknown {
			return kind
		}
		if a == nativeScalarUint && b == nativeScalarUint {
			return nativeScalarUint
		}
		if a == nativeScalarInt && b == nativeScalarInt {
			return nativeScalarInt
		}
		if kind := nativeScalarNarrowSignedResult(a, b); kind != nativeScalarUnknown {
			return kind
		}
	case "-":
		if nativeScalarIsFloat(a) || nativeScalarIsFloat(b) {
			return nativeScalarFloatResult(a, b)
		}
		if kind := nativeScalarNarrowSignedResult(a, b); kind != nativeScalarUnknown {
			return kind
		}
		if a == nativeScalarInt && b == nativeScalarInt {
			return nativeScalarInt
		}
		if kind := nativeScalarNarrowUnsignedResult(a, b); kind != nativeScalarUnknown {
			return kind
		}
		if a == nativeScalarUint && b == nativeScalarUint {
			return nativeScalarUint
		}
	case "/":
		if nativeScalarIsFloat(a) && nativeScalarIsFloat(b) {
			return nativeScalarFloatResult(a, b)
		}
	case "%":
		if nativeScalarIsFloat(a) && nativeScalarIsFloat(b) {
			return nativeScalarFloatResult(a, b)
		}
		if kind := nativeScalarNarrowSignedResult(a, b); kind != nativeScalarUnknown {
			return kind
		}
		if a == nativeScalarInt && b == nativeScalarInt {
			return nativeScalarInt
		}
		if kind := nativeScalarNarrowUnsignedResult(a, b); kind != nativeScalarUnknown {
			return kind
		}
		if a == nativeScalarUint && b == nativeScalarUint {
			return nativeScalarUint
		}
	// Division retains the tagged path until the direct emitter has the same
	// decimal-result and recoverable zero-divisor contract as the native ABI.
	case "&", "|", "^", "<<", ">>":
		if op == "<<" || op == ">>" {
			if nativeScalarUnsignedWidth(a) > 0 && nativeScalarUnsignedWidth(a) < 64 && nativeScalarUnsignedWidth(b) > 0 {
				return a
			}
			if a == nativeScalarUint && b == nativeScalarUint {
				return nativeScalarUint
			}
		} else if kind := nativeScalarNarrowUnsignedResult(a, b); kind != nativeScalarUnknown {
			return kind
		} else if a == nativeScalarUint && b == nativeScalarUint {
			return nativeScalarUint
		}
		if op == "<<" || op == ">>" {
			if nativeScalarIsNarrowSigned(a) && nativeScalarIsNarrowSigned(b) {
				return a
			}
			if a == nativeScalarInt && b == nativeScalarInt {
				return nativeScalarInt
			}
		} else if kind := nativeScalarNarrowSignedResult(a, b); kind != nativeScalarUnknown {
			return kind
		} else if a == nativeScalarInt && b == nativeScalarInt {
			return nativeScalarInt
		}
	}
	return nativeScalarUnknown
}

// nativeScalarBuiltinResultKind describes the small subset of static std/math
// calls whose result kind and failure contract are fully representable by the
// unboxed scalar emitter. A builtin is admitted only when its result cannot
// change into a tagged language value at runtime.
func nativeScalarBuiltinResultKind(name string, args []nativeScalarKind) nativeScalarKind {
	if (name == "abs" || name == "sqrt" || name == "cbrt" || name == "exp" || name == "log" || name == "log10" || name == "log2" || name == "expm1" || name == "log1p" || name == "erf" || name == "erfc" || name == "sin" || name == "cos" || name == "tan" || name == "floor" || name == "ceil" || name == "round") && len(args) == 1 && nativeScalarIsFloat(args[0]) {
		return args[0]
	}
	if name == "log" && len(args) == 2 && args[0] == nativeScalarF32 && args[1] == nativeScalarF32 {
		return nativeScalarF32
	}
	if name == "f32" && len(args) == 1 && nativeScalarIsFloat(args[0]) {
		return nativeScalarF32
	}
	if name == "f64" && len(args) == 1 && nativeScalarIsFloat(args[0]) {
		return nativeScalarF64
	}
	// Real scalar inputs are complex-compatible. Their projection and
	// conjugation contracts are representable without materializing a complex
	// OValue; magnitude is simply the width-preserving floating absolute value.
	if (name == "real" || name == "imag" || name == "magnitude") && len(args) == 1 && nativeScalarIsFloat(args[0]) {
		return args[0]
	}
	if (name == "abs" || name == "magnitude") && len(args) == 1 && nativeScalarIsUnsigned(args[0]) {
		return args[0]
	}
	if (name == "abs" || name == "magnitude") && len(args) == 1 && nativeScalarIsNarrowSigned(args[0]) {
		return args[0]
	}
	if name == "conjugate" && len(args) == 1 && (nativeScalarIsFloat(args[0]) || nativeScalarIsInteger(args[0])) {
		return args[0]
	}
	if (name == "sign" || name == "step" || name == "heaviside" || name == "relu") && len(args) == 1 {
		if nativeScalarIsFloat(args[0]) {
			return args[0]
		}
		if nativeScalarIsInteger(args[0]) {
			return nativeScalarFloat
		}
	}
	// Go's f64 inverse-trigonometric implementation can differ by one final
	// bit from the platform libm. The f32 result is explicitly narrowed by the
	// language and is characterized below; retain f64 on the tagged Go path.
	if (name == "asin" || name == "acos" || name == "atan") && len(args) == 1 && args[0] == nativeScalarF32 {
		return nativeScalarF32
	}
	if (name == "sinh" || name == "cosh" || name == "tanh" || name == "asinh" || name == "acosh" || name == "atanh") && len(args) == 1 && args[0] == nativeScalarF32 {
		return nativeScalarF32
	}
	if name == "sigmoid" && len(args) == 1 && args[0] == nativeScalarF32 {
		return nativeScalarF32
	}
	if name == "softplus" && len(args) == 1 && args[0] == nativeScalarF32 {
		return nativeScalarF32
	}
	// Gamma implementations are not specified to agree bit-for-bit between Go
	// and a platform C library. The language's explicit f32 narrowing is the
	// characterized contract; retain f64 on a rejecting path until equivalent
	// evidence exists.
	if (name == "gamma" || name == "lgamma") && len(args) == 1 && args[0] == nativeScalarF32 {
		return nativeScalarF32
	}
	if name == "clamp" && len(args) == 3 && nativeScalarIsFloat(args[0]) && args[0] == args[1] && args[1] == args[2] {
		return args[0]
	}
	if (name == "min" || name == "max") && len(args) > 0 && nativeScalarIsFloat(args[0]) {
		for _, kind := range args[1:] {
			if kind != args[0] {
				return nativeScalarUnknown
			}
		}
		return args[0]
	}
	if name == "hypot" && len(args) >= 2 && nativeScalarIsFloat(args[0]) {
		for _, kind := range args[1:] {
			if kind != args[0] {
				return nativeScalarUnknown
			}
		}
		return args[0]
	}
	return nativeScalarUnknown
}

// nativeScalarExactSignedDivisionKind admits only literal, same-width signed
// divisions whose quotient is integral. Osiris otherwise returns a decimal,
// which cannot be represented by an unboxed scalar local.
func nativeScalarExactSignedDivisionKind(left, right Value, leftKind, rightKind nativeScalarKind) nativeScalarKind {
	if leftKind != rightKind || nativeScalarSignedWidth(leftKind) == 0 || right.kind != kindInt || right.i == nil || right.i.Sign() == 0 || left.kind != kindInt || left.i == nil {
		return nativeScalarUnknown
	}
	if new(big.Int).Rem(left.i, right.i).Sign() != 0 {
		return nativeScalarUnknown
	}
	return leftKind
}

func nativeScalarLiteral(v Value) (string, bool) {
	switch nativeScalarValueKind(v) {
	case nativeScalarBool:
		if v.boo {
			return "1", true
		}
		return "0", true
	case nativeScalarUint:
		return fmt.Sprintf("%duLL", v.u), true
	case nativeScalarU8:
		return fmt.Sprintf("UINT8_C(%d)", v.u), true
	case nativeScalarU16:
		return fmt.Sprintf("UINT16_C(%d)", v.u), true
	case nativeScalarU32:
		return fmt.Sprintf("UINT32_C(%d)", v.u), true
	case nativeScalarInt:
		return fmt.Sprintf("%dLL", v.i.Int64()), true
	case nativeScalarI8:
		return fmt.Sprintf("((int8_t)%d)", v.i.Int64()), true
	case nativeScalarI16:
		return fmt.Sprintf("((int16_t)%d)", v.i.Int64()), true
	case nativeScalarI32:
		return fmt.Sprintf("((int32_t)%d)", v.i.Int64()), true
	case nativeScalarFloat:
		return fmt.Sprintf("%.17g", v.num), true
	case nativeScalarF32:
		literal := fmt.Sprintf("%.17g", v.num)
		// C requires a decimal point or exponent before the f suffix. Without
		// this, an integral f32 such as 2.0f32 became the invalid token `2f`.
		if !strings.ContainsAny(literal, ".eE") {
			literal += ".0"
		}
		return literal + "f", true
	case nativeScalarF64:
		return fmt.Sprintf("%.17g", v.num), true
	default:
		return "", false
	}
}

func nativeScalarCType(kind nativeScalarKind) string {
	switch kind {
	case nativeScalarBool:
		return "int"
	case nativeScalarUint:
		return "uint64_t"
	case nativeScalarU8:
		return "uint8_t"
	case nativeScalarU16:
		return "uint16_t"
	case nativeScalarU32:
		return "uint32_t"
	case nativeScalarInt:
		return "int64_t"
	case nativeScalarI8:
		return "int8_t"
	case nativeScalarI16:
		return "int16_t"
	case nativeScalarI32:
		return "int32_t"
	case nativeScalarFloat, nativeScalarF64:
		return "double"
	case nativeScalarF32:
		return "float"
	default:
		return ""
	}
}

type scalarInlinePlan struct {
	fn    *IRFunction
	kinds map[string]nativeScalarKind
	ret   string
}

type scalarTernaryPlan struct {
	fn        *IRFunction
	kinds     map[string]nativeScalarKind
	entry     *IRBlock
	then      *IRBlock
	otherwise *IRBlock
	cond      string
	thenVal   string
	elseVal   string
}

// scalarInlinePlanFor recognizes non-capturing, straight-line scalar helpers.
// Control flow and nested calls remain on the general native function ABI.
func scalarInlinePlanFor(fn *IRFunction, args []nativeScalarKind) *scalarInlinePlan {
	if fn == nil || len(fn.Captures) != 0 || len(fn.Params) != len(args) || len(fn.Blocks) != 1 {
		return nil
	}
	block := fn.Blocks[0]
	ret, ok := block.Term.(*irReturn)
	if !ok || !ret.HasValue {
		return nil
	}
	kinds := make(map[string]nativeScalarKind, len(args)+len(block.Instrs))
	for i, param := range fn.Params {
		kinds[fmt.Sprintf("%s.%s.0", fn.Name, param)] = args[i]
	}
	for _, in := range block.Instrs {
		var kind nativeScalarKind
		switch in.Op {
		case "const":
			kind = nativeScalarValueKind(in.Value)
		case "copy":
			if len(in.Args) != 1 {
				return nil
			}
			kind = kinds[in.Args[0]]
		case "unary":
			if len(in.Args) != 1 {
				return nil
			}
			if in.Text == "!" {
				kind = nativeScalarBool
			} else if in.Text == "-" {
				if nativeScalarIsFloat(kinds[in.Args[0]]) || nativeScalarSignedWidth(kinds[in.Args[0]]) != 0 {
					kind = kinds[in.Args[0]]
				}
			}
		case "binary":
			if len(in.Args) != 2 {
				return nil
			}
			kind = nativeScalarResultKind(in.Text, kinds[in.Args[0]], kinds[in.Args[1]])
		default:
			return nil
		}
		if in.Dest == "" || kind == nativeScalarUnknown {
			return nil
		}
		kinds[in.Dest] = kind
	}
	if kinds[ret.Value] == nativeScalarUnknown {
		return nil
	}
	return &scalarInlinePlan{fn: fn, kinds: kinds, ret: ret.Value}
}

func scalarInlineCallKind(fn *IRFunction, args []nativeScalarKind) nativeScalarKind {
	plan := scalarInlinePlanFor(fn, args)
	if plan != nil {
		return plan.kinds[plan.ret]
	}
	planIf := scalarTernaryPlanFor(fn, args)
	if planIf == nil {
		return nativeScalarUnknown
	}
	return planIf.kinds[planIf.thenVal]
}

func scalarTernaryPlanFor(fn *IRFunction, args []nativeScalarKind) *scalarTernaryPlan {
	if fn == nil || len(fn.Captures) != 0 || len(fn.Params) != len(args) || len(fn.Blocks) != 4 {
		return nil
	}
	entry, thenBlock, elseBlock, join := fn.Blocks[0], fn.Blocks[1], fn.Blocks[2], fn.Blocks[3]
	branch, ok := entry.Term.(*irBranch)
	if !ok || branch.TrueTarget != thenBlock.Name || branch.FalseTarget != elseBlock.Name || len(branch.TrueArgs) != 0 || len(branch.FalseArgs) != 0 || len(thenBlock.Params) != 0 || len(elseBlock.Params) != 0 || len(join.Params) != 1 {
		return nil
	}
	thenJump, ok := thenBlock.Term.(*irJump)
	if !ok || thenJump.Target != join.Name || len(thenJump.Args) != 1 {
		return nil
	}
	elseJump, ok := elseBlock.Term.(*irJump)
	if !ok || elseJump.Target != join.Name || len(elseJump.Args) != 1 {
		return nil
	}
	ret, ok := join.Term.(*irReturn)
	if !ok || !ret.HasValue || ret.Value != join.Params[0] || len(join.Instrs) != 0 {
		return nil
	}
	kinds := make(map[string]nativeScalarKind, len(args)+len(entry.Instrs)+len(thenBlock.Instrs)+len(elseBlock.Instrs))
	for i, param := range fn.Params {
		kinds[fmt.Sprintf("%s.%s.0", fn.Name, param)] = args[i]
	}
	infer := func(instrs []IRInstr) bool {
		for _, in := range instrs {
			var kind nativeScalarKind
			switch in.Op {
			case "const":
				kind = nativeScalarValueKind(in.Value)
			case "copy":
				if len(in.Args) != 1 {
					return false
				}
				kind = kinds[in.Args[0]]
			case "unary":
				if len(in.Args) != 1 {
					return false
				}
				if in.Text == "!" {
					kind = nativeScalarBool
				} else if in.Text == "-" && (nativeScalarIsFloat(kinds[in.Args[0]]) || nativeScalarSignedWidth(kinds[in.Args[0]]) != 0) {
					kind = kinds[in.Args[0]]
				}
			case "binary":
				if len(in.Args) != 2 {
					return false
				}
				kind = nativeScalarResultKind(in.Text, kinds[in.Args[0]], kinds[in.Args[1]])
			default:
				return false
			}
			if in.Dest == "" || kind == nativeScalarUnknown {
				return false
			}
			kinds[in.Dest] = kind
		}
		return true
	}
	if !infer(entry.Instrs) || kinds[branch.Cond] != nativeScalarBool {
		return nil
	}
	entryKinds := make(map[string]nativeScalarKind, len(kinds))
	for name, kind := range kinds {
		entryKinds[name] = kind
	}
	if !infer(thenBlock.Instrs) {
		return nil
	}
	thenKind := kinds[thenJump.Args[0]]
	for name := range kinds {
		if _, ok := entryKinds[name]; !ok {
			delete(kinds, name)
		}
	}
	if !infer(elseBlock.Instrs) {
		return nil
	}
	elseKind := kinds[elseJump.Args[0]]
	if thenKind == nativeScalarUnknown || thenKind != elseKind {
		return nil
	}
	// Re-run the then branch after restoring the complete map so both branch
	// value kinds remain available to the emitter.
	if !infer(thenBlock.Instrs) {
		return nil
	}
	return &scalarTernaryPlan{fn: fn, kinds: kinds, entry: entry, then: thenBlock, otherwise: elseBlock, cond: branch.Cond, thenVal: thenJump.Args[0], elseVal: elseJump.Args[0]}
}

func emitScalarBinary(b *strings.Builder, dest, op, left, right string, kind nativeScalarKind) {
	switch {
	case op == "%" && nativeScalarIsFloat(kind):
		fmt.Fprintf(b, "  if (%s == 0) return onativefail(\"remainder by zero\");\n", right)
		if kind == nativeScalarF32 {
			// The interpreter evaluates math.Mod in f64 and then applies f32
			// result-width rounding, so use fmod rather than fmodf here.
			fmt.Fprintf(b, "  %s = (%s)fmod((double)%s, (double)%s);\n", dest, nativeScalarCType(kind), left, right)
		} else {
			fmt.Fprintf(b, "  %s = fmod(%s, %s);\n", dest, left, right)
		}
	case op == "/" && nativeScalarIsFloat(kind):
		fmt.Fprintf(b, "  if (%s == 0) return onativefail(\"division by zero\");\n", right)
		fmt.Fprintf(b, "  %s = %s / %s;\n", dest, left, right)
	case op == "+" && kind == nativeScalarUint:
		fmt.Fprintf(b, "  if (__builtin_add_overflow(%s, %s, &%s)) return onativefail(\"unsigned addition overflow\");\n", left, right, dest)
	case op == "*" && kind == nativeScalarUint:
		fmt.Fprintf(b, "  if (__builtin_mul_overflow(%s, %s, &%s)) return onativefail(\"unsigned multiplication overflow\");\n", left, right, dest)
	case op == "+" && kind == nativeScalarInt:
		fmt.Fprintf(b, "  if (__builtin_add_overflow(%s, %s, &%s)) return onativefail(\"signed addition overflow\");\n", left, right, dest)
	case op == "*" && kind == nativeScalarInt:
		fmt.Fprintf(b, "  if (__builtin_mul_overflow(%s, %s, &%s)) return onativefail(\"signed multiplication overflow\");\n", left, right, dest)
	case op == "-" && kind == nativeScalarInt:
		fmt.Fprintf(b, "  if (__builtin_sub_overflow(%s, %s, &%s)) return onativefail(\"signed subtraction overflow\");\n", left, right, dest)
	case (op == "<<" || op == ">>") && kind == nativeScalarUint:
		fmt.Fprintf(b, "  if (%s >= 64) return onativefail(\"shift count exceeds u64 width\");\n", right)
		fmt.Fprintf(b, "  %s = %s %s %s;\n", dest, left, op, right)
	case (op == "<<" || op == ">>") && nativeScalarUnsignedWidth(kind) != 0:
		// C shifts at or above the operand width are undefined. The native ABI
		// reports such counts instead of relying on host behavior.
		fmt.Fprintf(b, "  if (%s >= %d) return onativefail(\"shift count exceeds unsigned width\");\n", right, nativeScalarUnsignedWidth(kind))
		fmt.Fprintf(b, "  %s = (%s)((uint64_t)%s %s %s);\n", dest, nativeScalarCType(kind), left, op, right)
	case (op == "+" || op == "-" || op == "*") && nativeScalarIsNarrowSigned(kind):
		// The widest narrow product fits in int64_t. Normalize through the
		// explicit ABI helper before assigning to a signed narrow C local, so
		// neither C signed overflow nor implementation-defined narrowing leaks
		// into the language result.
		fmt.Fprintf(b, "  %s = (%s)osignedbits((uint64_t)((int64_t)%s %s (int64_t)%s), %d);\n", dest, nativeScalarCType(kind), left, op, right, nativeScalarSignedWidth(kind))
	case (op == "&" || op == "|" || op == "^") && nativeScalarIsNarrowSigned(kind):
		fmt.Fprintf(b, "  %s = (%s)osignedbits((uint64_t)%s %s (uint64_t)%s, %d);\n", dest, nativeScalarCType(kind), left, op, right, nativeScalarSignedWidth(kind))
	case op == "%" && nativeScalarIsNarrowSigned(kind):
		fmt.Fprintf(b, "  if ((int64_t)%s == 0) return onativefail(\"signed remainder by zero\");\n", right)
		fmt.Fprintf(b, "  %s = (%s)osignedbits((uint64_t)((int64_t)%s %% (int64_t)%s), %d);\n", dest, nativeScalarCType(kind), left, right, nativeScalarSignedWidth(kind))
	case op == "%" && kind == nativeScalarInt:
		// INT64_MIN % -1 is undefined in C, although its mathematical and
		// Osiris result is zero. Handle it before emitting the host operation.
		fmt.Fprintf(b, "  if (%s == 0) return onativefail(\"signed remainder by zero\");\n", right)
		fmt.Fprintf(b, "  %s = (%s == INT64_MIN && %s == -1) ? 0 : %s %% %s;\n", dest, left, right, left, right)
	case op == "%" && nativeScalarUnsignedWidth(kind) > 0 && nativeScalarUnsignedWidth(kind) < 64:
		fmt.Fprintf(b, "  if ((uint64_t)%s == 0) return onativefail(\"unsigned remainder by zero\");\n", right)
		fmt.Fprintf(b, "  %s = (%s)((uint64_t)%s %% (uint64_t)%s);\n", dest, nativeScalarCType(kind), left, right)
	case op == "%" && kind == nativeScalarUint:
		fmt.Fprintf(b, "  if (%s == 0) return onativefail(\"unsigned remainder by zero\");\n", right)
		fmt.Fprintf(b, "  %s = %s %% %s;\n", dest, left, right)
	case (op == "<<" || op == ">>") && nativeScalarIsNarrowSigned(kind):
		width := nativeScalarSignedWidth(kind)
		fmt.Fprintf(b, "  if ((int64_t)%s < 0 || (uint64_t)%s >= %d) return onativefail(\"shift count exceeds signed width\");\n", right, right, width)
		if op == "<<" {
			fmt.Fprintf(b, "  %s = (%s)osignedbits((uint64_t)(int64_t)%s << %s, %d);\n", dest, nativeScalarCType(kind), left, right, width)
		} else {
			fmt.Fprintf(b, "  %s = (%s)osignedbits((int64_t)%s >= 0 ? ((uint64_t)(int64_t)%s >> %s) : ~((~(uint64_t)(int64_t)%s) >> %s), %d);\n", dest, nativeScalarCType(kind), left, left, right, left, right, width)
		}
	case (op == "<<" || op == ">>") && kind == nativeScalarInt:
		fmt.Fprintf(b, "  if ((int64_t)%s < 0 || (uint64_t)%s >= 64) return onativefail(\"shift count exceeds signed width\");\n", right, right)
		if op == "<<" {
			fmt.Fprintf(b, "  %s = osignedbits((uint64_t)%s << %s, 64);\n", dest, left, right)
		} else {
			fmt.Fprintf(b, "  %s = osignedbits((int64_t)%s >= 0 ? ((uint64_t)%s >> %s) : ~((~(uint64_t)%s) >> %s), 64);\n", dest, left, left, right, left, right)
		}
	case (op == "+" || op == "-" || op == "*") && nativeScalarUnsignedWidth(kind) > 0 && nativeScalarUnsignedWidth(kind) < 32:
		// uint8_t and uint16_t operands undergo C's signed integer promotion.
		// Widen explicitly before arithmetic, then narrow to Osiris's wrapping
		// result width so multiplication cannot invoke signed-overflow UB.
		fmt.Fprintf(b, "  %s = (%s)((uint64_t)%s %s (uint64_t)%s);\n", dest, nativeScalarCType(kind), left, op, right)
	default:
		fmt.Fprintf(b, "  %s = %s %s %s;\n", dest, left, op, right)
	}
}

func emitScalarExactSignedDivision(b *strings.Builder, dest, leftName, rightName string, left, right Value, kind nativeScalarKind) {
	// C defines every proven exact case except MIN / -1. Osiris wraps that
	// quotient back to the declared signed width, which is the original MIN.
	width := nativeScalarSignedWidth(kind)
	minimum := new(big.Int).Neg(new(big.Int).Lsh(big.NewInt(1), width-1))
	if right.i != nil && right.i.Cmp(big.NewInt(-1)) == 0 && left.i != nil && left.i.Cmp(minimum) == 0 {
		fmt.Fprintf(b, "  %s = %s;\n", dest, leftName)
		return
	}
	fmt.Fprintf(b, "  %s = %s / %s;\n", dest, leftName, rightName)
}

func emitScalarBuiltin(b *strings.Builder, dest, name string, args []string, kind nativeScalarKind) bool {
	switch name {
	case "f32":
		if len(args) != 1 || kind != nativeScalarF32 {
			return false
		}
		fmt.Fprintf(b, "  %s = (float)%s;\n", dest, args[0])
		return true
	case "f64":
		if len(args) != 1 || kind != nativeScalarF64 {
			return false
		}
		fmt.Fprintf(b, "  %s = (double)%s;\n", dest, args[0])
		return true
	case "real":
		if len(args) != 1 || !nativeScalarIsFloat(kind) {
			return false
		}
		fmt.Fprintf(b, "  %s = %s;\n", dest, args[0])
		return true
	case "conjugate":
		if len(args) != 1 || (!nativeScalarIsFloat(kind) && !nativeScalarIsInteger(kind)) {
			return false
		}
		fmt.Fprintf(b, "  %s = %s;\n", dest, args[0])
		return true
	case "imag":
		if len(args) != 1 || !nativeScalarIsFloat(kind) {
			return false
		}
		fmt.Fprintf(b, "  %s = 0;\n", dest)
		return true
	case "magnitude":
		if len(args) != 1 {
			return false
		}
		if nativeScalarIsUnsigned(kind) {
			fmt.Fprintf(b, "  %s = %s;\n", dest, args[0])
			return true
		}
		if nativeScalarIsNarrowSigned(kind) {
			fmt.Fprintf(b, "  %s = (%s)osignedbits(%s < 0 ? UINT64_C(0) - (uint64_t)(int64_t)%s : (uint64_t)(int64_t)%s, %d);\n", dest, nativeScalarCType(kind), args[0], args[0], args[0], nativeScalarSignedWidth(kind))
			return true
		}
		if !nativeScalarIsFloat(kind) {
			return false
		}
		if kind == nativeScalarF32 {
			fmt.Fprintf(b, "  %s = (float)fabs((double)%s);\n", dest, args[0])
		} else {
			fmt.Fprintf(b, "  %s = fabs(%s);\n", dest, args[0])
		}
		return true
	case "sign", "step", "heaviside", "relu":
		if len(args) != 1 || !nativeScalarIsFloat(kind) {
			return false
		}
		switch name {
		case "sign":
			fmt.Fprintf(b, "  %s = %s > 0 ? 1 : (%s < 0 ? -1 : 0);\n", dest, args[0], args[0])
		case "step":
			fmt.Fprintf(b, "  %s = %s > 0 ? 1 : 0;\n", dest, args[0])
		case "heaviside":
			fmt.Fprintf(b, "  %s = %s > 0 ? 1 : (%s < 0 ? 0 : 0.5);\n", dest, args[0], args[0])
		case "relu":
			fmt.Fprintf(b, "  %s = %s < 0 ? 0 : %s;\n", dest, args[0], args[0])
		}
		return true
	case "sigmoid":
		if len(args) != 1 || kind != nativeScalarF32 {
			return false
		}
		// Match the interpreter's branch before exp so large positive and
		// negative values retain its stable finite formulation.
		fmt.Fprintf(b, "  %s = %s >= 0 ? (float)(1 / (1 + exp(-(double)%s))) : (float)(exp((double)%s) / (1 + exp((double)%s)));\n", dest, args[0], args[0], args[0], args[0])
		return true
	case "softplus":
		if len(args) != 1 || kind != nativeScalarF32 {
			return false
		}
		// Retain the source runtime's strictly-greater-than 20 cutoff rather
		// than substituting a host-library softplus approximation.
		fmt.Fprintf(b, "  %s = %s > 20 ? %s : (float)log1p(exp((double)%s));\n", dest, args[0], args[0], args[0])
		return true
	case "sin", "cos", "tan":
		if len(args) != 1 || !nativeScalarIsFloat(kind) {
			return false
		}
		if kind == nativeScalarF32 {
			fmt.Fprintf(b, "  %s = (float)%s((double)%s);\n", dest, name, args[0])
		} else {
			fmt.Fprintf(b, "  %s = %s(%s);\n", dest, name, args[0])
		}
		return true
	case "atan":
		if len(args) != 1 || !nativeScalarIsFloat(kind) {
			return false
		}
		if kind == nativeScalarF32 {
			fmt.Fprintf(b, "  %s = (float)atan((double)%s);\n", dest, args[0])
		} else {
			fmt.Fprintf(b, "  %s = atan(%s);\n", dest, args[0])
		}
		return true
	case "asin", "acos":
		if len(args) != 1 || !nativeScalarIsFloat(kind) {
			return false
		}
		// As with exp, use a scoped temporary because a scalar CFG block may
		// begin at a C label. The interpreter rejects NaN and infinity from
		// these inverse functions, including a NaN input, before width rounding.
		b.WriteString("  {\n")
		fmt.Fprintf(b, "    double %s_%s = %s((double)%s);\n", dest, name, name, args[0])
		fmt.Fprintf(b, "    if (!isfinite(%s_%s)) return onativefail(\"%s domain error\");\n", dest, name, name)
		if kind == nativeScalarF32 {
			fmt.Fprintf(b, "    %s = (float)%s_%s;\n", dest, dest, name)
		} else {
			fmt.Fprintf(b, "    %s = %s_%s;\n", dest, dest, name)
		}
		b.WriteString("  }\n")
		return true
	case "sinh", "cosh", "tanh", "asinh":
		if len(args) != 1 || kind != nativeScalarF32 {
			return false
		}
		fmt.Fprintf(b, "  %s = (float)%s((double)%s);\n", dest, name, args[0])
		return true
	case "acosh", "atanh":
		if len(args) != 1 || kind != nativeScalarF32 {
			return false
		}
		// These helpers reject NaN and infinity in the interpreter. Keep the
		// check before f32 narrowing and scope the declaration for label-led CFGs.
		b.WriteString("  {\n")
		fmt.Fprintf(b, "    double %s_%s = %s((double)%s);\n", dest, name, name, args[0])
		fmt.Fprintf(b, "    if (!isfinite(%s_%s)) return onativefail(\"%s domain error\");\n", dest, name, name)
		fmt.Fprintf(b, "    %s = (float)%s_%s;\n", dest, dest, name)
		b.WriteString("  }\n")
		return true
	case "hypot":
		if len(args) < 2 || !nativeScalarIsFloat(kind) {
			return false
		}
		fmt.Fprintf(b, "  %s = ", dest)
		if kind == nativeScalarF32 {
			b.WriteString("(float)")
		}
		b.WriteString("sqrt(")
		for i, arg := range args {
			if i > 0 {
				b.WriteString(" + ")
			}
			fmt.Fprintf(b, "((double)%s * (double)%s)", arg, arg)
		}
		b.WriteString(");\n")
		return true
	case "floor", "ceil":
		if len(args) != 1 || !nativeScalarIsFloat(kind) {
			return false
		}
		if kind == nativeScalarF32 {
			fmt.Fprintf(b, "  %s = (float)%s((double)%s);\n", dest, name, args[0])
		} else {
			fmt.Fprintf(b, "  %s = %s(%s);\n", dest, name, args[0])
		}
		return true
	case "min", "max":
		if len(args) == 0 || !nativeScalarIsFloat(kind) {
			return false
		}
		fmt.Fprintf(b, "  %s = %s;\n", dest, args[0])
		for _, arg := range args[1:] {
			if name == "min" {
				fmt.Fprintf(b, "  if (%s < %s) %s = %s;\n", arg, dest, dest, arg)
			} else {
				fmt.Fprintf(b, "  if (%s > %s) %s = %s;\n", arg, dest, dest, arg)
			}
		}
		return true
	case "clamp":
		if len(args) != 3 || !nativeScalarIsFloat(kind) {
			return false
		}
		fmt.Fprintf(b, "  %s = %s;\n", dest, args[0])
		fmt.Fprintf(b, "  if (%s < %s) %s = %s; else if (%s > %s) %s = %s;\n", dest, args[1], dest, args[1], dest, args[2], dest, args[2])
		return true
	case "abs":
		if len(args) != 1 {
			return false
		}
		if nativeScalarIsUnsigned(kind) {
			fmt.Fprintf(b, "  %s = %s;\n", dest, args[0])
			return true
		}
		if nativeScalarIsNarrowSigned(kind) {
			fmt.Fprintf(b, "  %s = (%s)osignedbits(%s < 0 ? UINT64_C(0) - (uint64_t)(int64_t)%s : (uint64_t)(int64_t)%s, %d);\n", dest, nativeScalarCType(kind), args[0], args[0], args[0], nativeScalarSignedWidth(kind))
			return true
		}
		if !nativeScalarIsFloat(kind) {
			return false
		}
		if kind == nativeScalarF32 {
			fmt.Fprintf(b, "  %s = (float)fabs((double)%s);\n", dest, args[0])
		} else {
			fmt.Fprintf(b, "  %s = fabs(%s);\n", dest, args[0])
		}
		return true
	case "sqrt":
		if len(args) != 1 || !nativeScalarIsFloat(kind) {
			return false
		}
		fmt.Fprintf(b, "  if (%s < 0) return onativefail(\"sqrt domain error\");\n", args[0])
		if kind == nativeScalarF32 {
			fmt.Fprintf(b, "  %s = (float)sqrt((double)%s);\n", dest, args[0])
		} else {
			fmt.Fprintf(b, "  %s = sqrt(%s);\n", dest, args[0])
		}
		return true
	case "cbrt":
		if len(args) != 1 || !nativeScalarIsFloat(kind) {
			return false
		}
		if kind == nativeScalarF32 {
			fmt.Fprintf(b, "  %s = (float)cbrt((double)%s);\n", dest, args[0])
		} else {
			fmt.Fprintf(b, "  %s = cbrt(%s);\n", dest, args[0])
		}
		return true
	case "exp":
		if len(args) != 1 || !nativeScalarIsFloat(kind) {
			return false
		}
		// The interpreter checks the double-precision result before applying
		// float-width rounding. Keep that order so a finite result which only
		// overflows when narrowed to f32 retains its language value.
		// Blocks may start with a C label, which must be followed by a
		// statement rather than a declaration. Scope the temporary in a
		// compound statement so this emission is valid in every CFG block.
		b.WriteString("  {\n")
		fmt.Fprintf(b, "    double %s_exp = exp((double)%s);\n", dest, args[0])
		fmt.Fprintf(b, "    if (!isfinite(%s_exp)) return onativefail(\"exp overflow\");\n", dest)
		if kind == nativeScalarF32 {
			fmt.Fprintf(b, "    %s = (float)%s_exp;\n", dest, dest)
		} else {
			fmt.Fprintf(b, "    %s = %s_exp;\n", dest, dest)
		}
		b.WriteString("  }\n")
		return true
	case "log":
		if !nativeScalarIsFloat(kind) || (len(args) != 1 && len(args) != 2) {
			return false
		}
		if len(args) == 1 {
			fmt.Fprintf(b, "  if (%s <= 0) return onativefail(\"log domain error\");\n", args[0])
			if kind == nativeScalarF32 {
				fmt.Fprintf(b, "  %s = (float)log((double)%s);\n", dest, args[0])
			} else {
				fmt.Fprintf(b, "  %s = log(%s);\n", dest, args[0])
			}
			return true
		}
		if kind != nativeScalarF32 {
			return false
		}
		// The runtime defines base conversion as math.Log(value) / math.Log(base)
		// after validating both inputs. Keep that expression and f32 narrowing
		// order rather than substituting a platform-specific log-base helper.
		fmt.Fprintf(b, "  if (%s <= 0 || %s <= 0 || %s == 1) return onativefail(\"log domain error\");\n", args[0], args[1], args[1])
		fmt.Fprintf(b, "  %s = (float)(log((double)%s) / log((double)%s));\n", dest, args[0], args[1])
		return true
	case "pow", "expt":
		if len(args) != 2 || kind != nativeScalarF32 {
			return false
		}
		// Float operands bypass the runtime's exact-integer branch. Preserve
		// its math.Pow semantics and round only once at the f32 boundary.
		fmt.Fprintf(b, "  %s = (float)pow((double)%s, (double)%s);\n", dest, args[0], args[1])
		return true
	case "log10", "log2":
		if len(args) != 1 || !nativeScalarIsFloat(kind) {
			return false
		}
		fmt.Fprintf(b, "  if (%s <= 0) return onativefail(\"%s domain error\");\n", args[0], name)
		if kind == nativeScalarF32 {
			fmt.Fprintf(b, "  %s = (float)%s((double)%s);\n", dest, name, args[0])
		} else {
			fmt.Fprintf(b, "  %s = %s(%s);\n", dest, name, args[0])
		}
		return true
	case "expm1":
		if len(args) != 1 || !nativeScalarIsFloat(kind) {
			return false
		}
		if kind == nativeScalarF32 {
			fmt.Fprintf(b, "  %s = (float)expm1((double)%s);\n", dest, args[0])
		} else {
			fmt.Fprintf(b, "  %s = expm1(%s);\n", dest, args[0])
		}
		return true
	case "log1p":
		if len(args) != 1 || !nativeScalarIsFloat(kind) {
			return false
		}
		if kind == nativeScalarF32 {
			fmt.Fprintf(b, "  %s = (float)log1p((double)%s);\n", dest, args[0])
		} else {
			fmt.Fprintf(b, "  %s = log1p(%s);\n", dest, args[0])
		}
		return true
	case "erf":
		if len(args) != 1 || !nativeScalarIsFloat(kind) {
			return false
		}
		if kind == nativeScalarF32 {
			fmt.Fprintf(b, "  %s = (float)erf((double)%s);\n", dest, args[0])
		} else {
			fmt.Fprintf(b, "  %s = erf(%s);\n", dest, args[0])
		}
		return true
	case "erfc":
		if len(args) != 1 || !nativeScalarIsFloat(kind) {
			return false
		}
		if kind == nativeScalarF32 {
			fmt.Fprintf(b, "  %s = (float)erfc((double)%s);\n", dest, args[0])
		} else {
			fmt.Fprintf(b, "  %s = erfc(%s);\n", dest, args[0])
		}
		return true
	case "gamma", "lgamma":
		if len(args) != 1 || kind != nativeScalarF32 {
			return false
		}
		if name == "gamma" {
			fmt.Fprintf(b, "  %s = (float)tgamma((double)%s);\n", dest, args[0])
		} else {
			fmt.Fprintf(b, "  %s = (float)lgamma((double)%s);\n", dest, args[0])
		}
		return true
	case "round":
		if len(args) != 1 || !nativeScalarIsFloat(kind) {
			return false
		}
		if kind == nativeScalarF32 {
			fmt.Fprintf(b, "  %s = (float)round((double)%s);\n", dest, args[0])
		} else {
			fmt.Fprintf(b, "  %s = round(%s);\n", dest, args[0])
		}
		return true
	default:
		return false
	}
}

func emitScalarInlineInstrs(b *strings.Builder, instrs []IRInstr, kinds map[string]nativeScalarKind, values map[string]string, counter *int) bool {
	for _, in := range instrs {
		local := fmt.Sprintf("inl%d", *counter)
		*counter++
		fmt.Fprintf(b, "    %s %s = 0;\n", nativeScalarCType(kinds[in.Dest]), local)
		switch in.Op {
		case "const":
			literal, ok := nativeScalarLiteral(in.Value)
			if !ok {
				return false
			}
			fmt.Fprintf(b, "    %s = %s;\n", local, literal)
		case "copy":
			fmt.Fprintf(b, "    %s = %s;\n", local, values[in.Args[0]])
		case "unary":
			if in.Text == "-" && kinds[in.Dest] == nativeScalarInt {
				fmt.Fprintf(b, "    if (__builtin_sub_overflow((int64_t)0, %s, &%s)) return onativefail(\"signed negation overflow\");\n", values[in.Args[0]], local)
			} else if in.Text == "-" && nativeScalarIsNarrowSigned(kinds[in.Dest]) {
				fmt.Fprintf(b, "    %s = (%s)osignedbits((uint64_t)(-(int64_t)%s), %d);\n", local, nativeScalarCType(kinds[in.Dest]), values[in.Args[0]], nativeScalarSignedWidth(kinds[in.Dest]))
			} else {
				fmt.Fprintf(b, "    %s = %s%s;\n", local, in.Text, values[in.Args[0]])
			}
		case "binary":
			emitScalarBinary(b, local, in.Text, values[in.Args[0]], values[in.Args[1]], kinds[in.Dest])
		default:
			return false
		}
		values[in.Dest] = local
	}
	return true
}

// emitCScalarMain emits a proven scalar-only main CFG without OValue locals or
// generic helper calls. It is deliberately all-or-nothing: uncertainty falls
// back to the tagged ABI rather than guessing at language semantics.
func emitCScalarMain(b *strings.Builder, fn *IRFunction, functions map[string]*IRFunction) (bool, error) {
	if fn == nil || len(fn.Params) != 0 {
		return false, nil
	}
	blocks := make(map[string]*IRBlock, len(fn.Blocks))
	uses := make(map[string]bool)
	bindings := make(map[string]*IRFunction)
	builtins := make(map[string]string)
	modules := nativeModuleIdentities(fn, nil)
	for _, block := range fn.Blocks {
		blocks[block.Name] = block
		for _, in := range block.Instrs {
			if in.Op == "makefunc" {
				bindings[in.Dest] = functions[in.Text]
			}
			if in.Op == "builtin" {
				builtins[in.Dest] = in.Text
			}
			if in.Op == "member" && len(in.Args) == 1 && modules[in.Args[0]] == "std/math" {
				builtins[in.Dest] = in.Text
			}
			if in.Op == "emit" {
				for _, arg := range in.Args {
					uses[arg] = true
				}
			}
		}
		switch term := block.Term.(type) {
		case *irBranch:
			uses[term.Cond] = true
		case *irReturn:
			uses[term.Value] = term.HasValue
		}
	}
	// Compute value liveness rather than treating every SSA edge as a use.
	// A source-level loop local can create an undef-initialized block parameter
	// that is carried around a cycle but never observed. Such dead plumbing must
	// not prevent an otherwise proven scalar CFG from lowering, while a live
	// undef remains an immediate reason to retain the tagged ABI.
	for changed := true; changed; {
		changed = false
		mark := func(name string) {
			if name != "" && !uses[name] {
				uses[name] = true
				changed = true
			}
		}
		for _, block := range fn.Blocks {
			for _, in := range block.Instrs {
				if in.Dest != "" && uses[in.Dest] {
					for _, arg := range in.Args {
						mark(arg)
					}
				}
			}
			switch term := block.Term.(type) {
			case *irJump:
				target := blocks[term.Target]
				if target == nil || len(target.Params) != len(term.Args) {
					return false, nil
				}
				for i, arg := range term.Args {
					if uses[target.Params[i]] {
						mark(arg)
					}
				}
			case *irBranch:
				for _, edge := range []struct {
					target string
					args   []string
				}{{term.TrueTarget, term.TrueArgs}, {term.FalseTarget, term.FalseArgs}} {
					target := blocks[edge.target]
					if target == nil || len(target.Params) != len(edge.args) {
						return false, nil
					}
					for i, arg := range edge.args {
						if uses[target.Params[i]] {
							mark(arg)
						}
					}
				}
			}
		}
	}
	kinds := make(map[string]nativeScalarKind)
	constants := make(map[string]Value)
	for _, block := range fn.Blocks {
		for _, in := range block.Instrs {
			if (in.Op == "const" || in.Op == "global") && in.Dest != "" {
				constants[in.Dest] = in.Value
			}
		}
	}
	exactSignedDivisions := make(map[string]bool)
	nonExactF32Powers := make(map[string]bool)
	set := func(name string, kind nativeScalarKind) (bool, bool) {
		if name == "" || kind == nativeScalarUnknown {
			return false, true
		}
		current := kinds[name]
		if current == nativeScalarUnknown {
			kinds[name] = kind
			return true, true
		}
		return false, current == kind
	}
	propagate := func(target string, args []string) (bool, bool) {
		block := blocks[target]
		if block == nil || len(block.Params) != len(args) {
			return false, false
		}
		changed := false
		for i, arg := range args {
			if !uses[block.Params[i]] {
				continue
			}
			c, ok := set(block.Params[i], kinds[arg])
			if !ok {
				return false, false
			}
			changed = changed || c
		}
		return changed, true
	}
	for pass := 0; pass < len(fn.Blocks)*8+8; pass++ {
		changed := false
		for _, block := range fn.Blocks {
			for _, in := range block.Instrs {
				var kind nativeScalarKind
				switch in.Op {
				case "const", "global":
					kind = nativeScalarValueKind(in.Value)
				case "copy":
					// A lexical alias of a static module is compiler metadata, not a
					// scalar C value. Its member calls have already been resolved into
					// builtin handles above, so it must not be inferred or emitted as a
					// blank scalar assignment.
					if modules[in.Dest] != "" {
						continue
					}
					if len(in.Args) != 1 {
						return false, nil
					}
					kind = kinds[in.Args[0]]
				case "unary":
					if len(in.Args) != 1 {
						return false, nil
					}
					if in.Text == "!" {
						kind = nativeScalarBool
					} else if in.Text == "-" && (nativeScalarIsFloat(kinds[in.Args[0]]) || nativeScalarSignedWidth(kinds[in.Args[0]]) != 0) {
						kind = kinds[in.Args[0]]
					}
				case "binary":
					if len(in.Args) != 2 {
						return false, nil
					}
					if in.Text == "/" {
						left, leftOK := constants[in.Args[0]]
						right, rightOK := constants[in.Args[1]]
						if leftOK && rightOK {
							kind = nativeScalarExactSignedDivisionKind(left, right, kinds[in.Args[0]], kinds[in.Args[1]])
							if kind != nativeScalarUnknown {
								exactSignedDivisions[in.Dest] = true
							}
						}
					}
					if kind == nativeScalarUnknown {
						kind = nativeScalarResultKind(in.Text, kinds[in.Args[0]], kinds[in.Args[1]])
					}
				case "makefunc":
					if bindings[in.Dest] == nil {
						return false, nil
					}
					continue
				case "call":
					args := make([]nativeScalarKind, len(in.Args))
					for i, arg := range in.Args {
						args[i] = kinds[arg]
					}
					if builtin, ok := builtins[in.Text]; ok {
						if builtin == "pow" || builtin == "expt" {
							// pow/expt first attempt exact integer arithmetic, even when a
							// literal was written with an f32 suffix. Admit C pow only when
							// both static operands are f32 constants and that exact branch is
							// conclusively inapplicable; otherwise its result kind can become
							// an integer or bigint rather than an unboxed f32.
							if len(in.Args) != 2 || args[0] != nativeScalarF32 || args[1] != nativeScalarF32 {
								return false, nil
							}
							left, leftOK := constants[in.Args[0]]
							right, rightOK := constants[in.Args[1]]
							if !leftOK || !rightOK {
								return false, nil
							}
							_, exact, err := exactPowValue(left, right)
							if err != nil || exact {
								return false, nil
							}
							kind = nativeScalarF32
							nonExactF32Powers[in.Dest] = true
						} else {
							kind = nativeScalarBuiltinResultKind(builtin, args)
						}
						if kind == nativeScalarUnknown {
							return false, nil
						}
					} else {
						kind = scalarInlineCallKind(bindings[in.Text], args)
					}
				case "import", "member", "builtin":
					continue
				case "emit":
					continue
				case "undef":
					if !uses[in.Dest] {
						continue
					}
					return false, nil
				default:
					return false, nil
				}
				if in.Dest == "" {
					return false, nil
				}
				if kind == nativeScalarUnknown {
					if !uses[in.Dest] {
						continue
					}
					continue
				}
				c, ok := set(in.Dest, kind)
				if !ok {
					return false, nil
				}
				changed = changed || c
			}
			switch term := block.Term.(type) {
			case *irJump:
				c, ok := propagate(term.Target, term.Args)
				if !ok {
					return false, nil
				}
				changed = changed || c
			case *irBranch:
				if kinds[term.Cond] != nativeScalarBool {
					return false, nil
				}
				c, ok := propagate(term.TrueTarget, term.TrueArgs)
				if !ok {
					return false, nil
				}
				changed = changed || c
				c, ok = propagate(term.FalseTarget, term.FalseArgs)
				if !ok {
					return false, nil
				}
				changed = changed || c
			case *irReturn, nil:
			default:
				return false, nil
			}
		}
		if !changed {
			break
		}
	}
	for _, block := range fn.Blocks {
		for _, param := range block.Params {
			if uses[param] && kinds[param] == nativeScalarUnknown {
				return false, nil
			}
		}
		for _, in := range block.Instrs {
			switch in.Op {
			case "const", "global", "unary", "binary", "call":
				if kinds[in.Dest] == nativeScalarUnknown && uses[in.Dest] {
					return false, nil
				}
			case "copy":
				if modules[in.Dest] != "" {
					continue
				}
				if kinds[in.Dest] == nativeScalarUnknown && uses[in.Dest] {
					return false, nil
				}
			case "makefunc":
				if bindings[in.Dest] == nil {
					return false, nil
				}
			case "import", "member", "builtin":
				// Static module metadata and builtin handles do not become C
				// values in the unboxed path.
			case "emit":
				for _, arg := range in.Args {
					if kinds[arg] == nativeScalarUnknown {
						return false, nil
					}
				}
			case "undef":
				if uses[in.Dest] {
					return false, nil
				}
			default:
				return false, nil
			}
		}
	}
	names := make([]string, 0, len(kinds))
	initializers := make(map[string]string)
	for _, block := range fn.Blocks {
		for _, in := range block.Instrs {
			if (in.Op != "const" && in.Op != "global") || kinds[in.Dest] == nativeScalarUnknown {
				continue
			}
			literal, ok := nativeScalarLiteral(in.Value)
			if !ok {
				return false, nil
			}
			initializers[in.Dest] = literal
		}
	}
	for name, kind := range kinds {
		if kind == nativeScalarUnknown {
			return false, nil
		}
		names = append(names, name)
	}
	sort.Strings(names)
	vars := make(map[string]string, len(names))
	for i, name := range names {
		vars[name] = fmt.Sprintf("s%d", i)
		initial := "0"
		if literal, ok := initializers[name]; ok {
			initial = literal
		}
		fmt.Fprintf(b, "  %s %s = %s;\n", nativeScalarCType(kinds[name]), vars[name], initial)
	}
	value := func(name string) (string, bool) {
		v, ok := vars[name]
		return v, ok
	}
	emitPrint := func(name string) bool {
		v, ok := value(name)
		if !ok {
			return false
		}
		switch kinds[name] {
		case nativeScalarBool:
			fmt.Fprintf(b, "  puts(%s ? \"true\" : \"false\");\n", v)
		case nativeScalarUint:
			fmt.Fprintf(b, "  printf(\"%%lluu64\\n\", (unsigned long long)%s);\n", v)
		case nativeScalarU8:
			fmt.Fprintf(b, "  printf(\"%%uu8\\n\", (unsigned)%s);\n", v)
		case nativeScalarU16:
			fmt.Fprintf(b, "  printf(\"%%uu16\\n\", (unsigned)%s);\n", v)
		case nativeScalarU32:
			fmt.Fprintf(b, "  printf(\"%%uu32\\n\", (unsigned)%s);\n", v)
		case nativeScalarInt:
			fmt.Fprintf(b, "  printf(\"%%lldi64\\n\", (long long)%s);\n", v)
		case nativeScalarI8:
			fmt.Fprintf(b, "  printf(\"%%hhdi8\\n\", %s);\n", v)
		case nativeScalarI16:
			fmt.Fprintf(b, "  printf(\"%%hdi16\\n\", %s);\n", v)
		case nativeScalarI32:
			fmt.Fprintf(b, "  printf(\"%%di32\\n\", %s);\n", v)
		case nativeScalarFloat:
			fmt.Fprintf(b, "  oprintfloat(%s); putchar('\\n');\n", v)
		case nativeScalarF32:
			fmt.Fprintf(b, "  oprintfloat((double)%s); fputs(\"f32\\n\", stdout);\n", v)
		case nativeScalarF64:
			fmt.Fprintf(b, "  oprintfloat(%s); fputs(\"f64\\n\", stdout);\n", v)
		default:
			return false
		}
		return true
	}
	jump := func(target string, args []string) bool {
		block := blocks[target]
		if block == nil || len(block.Params) != len(args) {
			return false
		}
		for i, arg := range args {
			if !uses[block.Params[i]] {
				continue
			}
			if kinds[block.Params[i]] != kinds[arg] {
				return false
			}
			fmt.Fprintf(b, "  %s = %s;\n", vars[block.Params[i]], vars[arg])
		}
		fmt.Fprintf(b, "  goto %s;\n", cLabel(target))
		return true
	}
	inlineCounter := 0
	for _, block := range fn.Blocks {
		fmt.Fprintf(b, "%s:\n", cLabel(block.Name))
		for _, in := range block.Instrs {
			switch in.Op {
			case "import", "member", "builtin":
				continue
			case "undef":
				if uses[in.Dest] {
					return false, nil
				}
				continue
			case "const", "global":
				if kinds[in.Dest] == nativeScalarUnknown && !uses[in.Dest] {
					continue
				}
				// Pure scalar constants were initialized at function entry. This
				// hoists loop-header constants out of every iteration.
			case "copy":
				if modules[in.Dest] != "" {
					// Static module aliases are compile-time metadata only.
					continue
				}
				if vars[in.Dest] == "" || vars[in.Args[0]] == "" {
					return false, nil
				}
				fmt.Fprintf(b, "  %s = %s;\n", vars[in.Dest], vars[in.Args[0]])
			case "unary":
				if in.Text == "-" && kinds[in.Dest] == nativeScalarInt {
					fmt.Fprintf(b, "  if (__builtin_sub_overflow((int64_t)0, %s, &%s)) return onativefail(\"signed negation overflow\");\n", vars[in.Args[0]], vars[in.Dest])
				} else if in.Text == "-" && nativeScalarIsNarrowSigned(kinds[in.Dest]) {
					fmt.Fprintf(b, "  %s = (%s)osignedbits((uint64_t)(-(int64_t)%s), %d);\n", vars[in.Dest], nativeScalarCType(kinds[in.Dest]), vars[in.Args[0]], nativeScalarSignedWidth(kinds[in.Dest]))
				} else {
					fmt.Fprintf(b, "  %s = %s%s;\n", vars[in.Dest], in.Text, vars[in.Args[0]])
				}
			case "binary":
				left, right := vars[in.Args[0]], vars[in.Args[1]]
				if in.Text == "/" && exactSignedDivisions[in.Dest] {
					emitScalarExactSignedDivision(b, vars[in.Dest], left, right, constants[in.Args[0]], constants[in.Args[1]], kinds[in.Dest])
				} else {
					emitScalarBinary(b, vars[in.Dest], in.Text, left, right, kinds[in.Dest])
				}
			case "makefunc":
				// The recognized direct call is inlined below; no closure object is needed.
			case "call":
				if builtin, ok := builtins[in.Text]; ok {
					if (builtin == "pow" || builtin == "expt") && !nonExactF32Powers[in.Dest] {
						return false, nil
					}
					args := make([]string, len(in.Args))
					for i, arg := range in.Args {
						args[i] = vars[arg]
					}
					if !emitScalarBuiltin(b, vars[in.Dest], builtin, args, kinds[in.Dest]) {
						return false, nil
					}
					continue
				}
				callee := bindings[in.Text]
				argKinds := make([]nativeScalarKind, len(in.Args))
				for i, arg := range in.Args {
					argKinds[i] = kinds[arg]
				}
				plan := scalarInlinePlanFor(callee, argKinds)
				values := map[string]string{}
				for i, param := range callee.Params {
					values[fmt.Sprintf("%s.%s.0", callee.Name, param)] = vars[in.Args[i]]
				}
				b.WriteString("  {\n")
				if plan != nil {
					if !emitScalarInlineInstrs(b, callee.Blocks[0].Instrs, plan.kinds, values, &inlineCounter) {
						return false, nil
					}
					fmt.Fprintf(b, "    %s = %s;\n", vars[in.Dest], values[plan.ret])
				} else if ternary := scalarTernaryPlanFor(callee, argKinds); ternary != nil {
					if !emitScalarInlineInstrs(b, ternary.entry.Instrs, ternary.kinds, values, &inlineCounter) {
						return false, nil
					}
					thenValues := make(map[string]string, len(values))
					elseValues := make(map[string]string, len(values))
					for name, value := range values {
						thenValues[name] = value
						elseValues[name] = value
					}
					fmt.Fprintf(b, "    if (%s) {\n", values[ternary.cond])
					if !emitScalarInlineInstrs(b, ternary.then.Instrs, ternary.kinds, thenValues, &inlineCounter) {
						return false, nil
					}
					fmt.Fprintf(b, "      %s = %s;\n", vars[in.Dest], thenValues[ternary.thenVal])
					b.WriteString("    } else {\n")
					if !emitScalarInlineInstrs(b, ternary.otherwise.Instrs, ternary.kinds, elseValues, &inlineCounter) {
						return false, nil
					}
					fmt.Fprintf(b, "      %s = %s;\n", vars[in.Dest], elseValues[ternary.elseVal])
					b.WriteString("    }\n")
				} else {
					return false, nil
				}
				b.WriteString("  }\n")
			case "emit":
				for _, arg := range in.Args {
					if !emitPrint(arg) {
						return false, nil
					}
				}
			}
		}
		switch term := block.Term.(type) {
		case *irJump:
			if !jump(term.Target, term.Args) {
				return false, nil
			}
		case *irBranch:
			fmt.Fprintf(b, "  if (%s) {\n", vars[term.Cond])
			if !jump(term.TrueTarget, term.TrueArgs) {
				return false, nil
			}
			b.WriteString("  } else {\n")
			if !jump(term.FalseTarget, term.FalseArgs) {
				return false, nil
			}
			b.WriteString("  }\n")
		case *irReturn, nil:
			b.WriteString("  return 0;\n")
		default:
			return false, nil
		}
	}
	return true, nil
}

func cRuntimePrelude() string {
	return strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(`#include <stdio.h>
#include <stdint.h>
#include <string.h>
#include <stdlib.h>
#include <math.h>
#include <limits.h>
#if !defined(_WIN32)
#include <sys/stat.h>
#endif
#if defined(__unix__) || defined(__APPLE__)
#include <dlfcn.h>
#include <unistd.h>
#endif

typedef enum { O_NULL, O_BOOL, O_UINT, O_INT, O_FLOAT, O_COMPLEX, O_DUAL, O_STRING, O_LIST, O_TENSOR, O_COMPLEX_TENSOR, O_CLOSURE, O_CELL, O_ENUM, O_DECIMAL, O_BIGINT, O_QUANTITY, O_DATETIME, O_DURATION, O_ERROR, O_RESULT, O_OPTION, O_TASK, O_BYTES, O_MAP, O_FFI, O_RESOURCE } OKind;
typedef struct OValue OValue;
typedef struct OTask OTask;
typedef struct OCell OCell;
typedef OValue (*OCall)(const OValue *args, size_t argc, const OValue *captures, size_t capture_count);
typedef struct { const char *data; size_t len; } OString;
typedef struct { double real; double imag; } OComplex;
typedef struct { double primal; double tangent; } ODual;
typedef struct { OValue *items; size_t len; } OList;
typedef struct OMap OMap;
typedef struct { OString type_name; OString case_name; OList payload; } OEnum;
typedef struct {
  OString kind;
  OString message;
  OString user_message;
  OString operation;
  OString resource;
  OString source;
  OString location;
  OValue *cause;
  OValue *stack;
  OValue *metadata;
  OValue *debug;
  int retryable;
  int severity;
  int redacted;
} OError;
typedef struct { double *data; size_t len; size_t *shape; size_t rank; unsigned char element_kind; } OTensor;
typedef struct { OComplex *data; size_t len; size_t *shape; size_t rank; } OComplexTensor;
typedef struct { OCall call; OValue *captures; size_t capture_count; const char *signature; } OClosure;
typedef struct { double magnitude; OString unit; } OQuantity;
typedef struct { OValue *value; } OBox;
typedef struct { void *handle; } OFFILibrary;
typedef struct OAlloc { void *ptr; struct OAlloc *next; } OAlloc;
typedef enum { O_LINALG_OK = 0, O_LINALG_INVALID_ARGUMENT, O_LINALG_NOT_CONVERGED, O_LINALG_NO_MEMORY, O_LINALG_UNAVAILABLE } OLinalgStatus;
typedef struct { unsigned abi_version; OLinalgStatus (*eigh_f64)(size_t, double *, double *, double *); OLinalgStatus (*svd_f64)(size_t, size_t, double *, double *, double *, double *); } OLinalgProviderV1;
typedef struct { unsigned abi_version; OLinalgStatus (*eigh_f64)(size_t, double *, double *, double *); OLinalgStatus (*svd_f64)(size_t, size_t, double *, double *, double *, double *); OLinalgStatus (*eig_f64)(size_t, double *, OComplex *, OComplex *); } OLinalgProviderV2;
extern const OLinalgProviderV1 *osiris_linalg_provider_v1(void);
struct OValue { OKind kind; uint16_t fbits; union { int b; uint64_t u; int64_t i; double f; OComplex cx; ODual dual; OString s; OList list; OTensor tensor; OComplexTensor ctensor; OClosure closure; OCell *cell; OEnum en; OError error; OQuantity quantity; OBox box; OTask *task; OMap *map; OFFILibrary ffi; void *resource; } as; };
typedef struct { OString key; OValue value; } OMapEntry;
struct OMap { OMapEntry *entries; size_t len; };
struct OCell { OValue value; };
struct OTask { OValue thunk; OValue result; OValue error; int state; int failed; };
static OValue ofail(const char *message);
static OValue obin(const char *op, OValue a, OValue b);
static void oprintdatetime(OValue value);
static int otrack(void *ptr);
static double ogosin(double value);
static double ogocos(double value);
static int onum(OValue x, double *out);
static int ocomplexnum(OValue x, OComplex *out);
static int oshapeargs(const OValue *args, size_t argc, size_t **out_shape, size_t *out_rank, size_t *out_len);
static OAlloc *oallocs = NULL;
static int ohaderror = 0;
static int oprinted = 0;
static const char *oerrormessage = "unknown error";
static OValue olasterror;
static void ofreeallocs(void) { while(oallocs) { OAlloc *next=oallocs->next; free(oallocs->ptr); free(oallocs); oallocs=next; } }
static OValue onull(void) { return (OValue){O_NULL,0,{.u=0}}; }
static OValue ocell(OValue value) { OCell *cell=(OCell *)malloc(sizeof(OCell)); if(!cell||!otrack(cell)) { free(cell); return ofail("out of memory"); } cell->value=value; return (OValue){O_CELL,0,{.cell=cell}}; }
static OValue ocellvalue(OValue value) { return value.kind==O_CELL&&value.as.cell?value.as.cell->value:value; }
static int ocellset(OValue cell, OValue value) { if(cell.kind!=O_CELL||!cell.as.cell) { ofail("invalid mutable capture cell"); return 0; } cell.as.cell->value=value; return 1; }
static OValue obool(int x) { return (OValue){O_BOOL,0,{.b=!!x}}; }
static uint64_t omask(uint16_t bits) { return bits>=64?UINT64_MAX:((UINT64_C(1)<<bits)-1); }
static int64_t osignedbits(uint64_t raw, uint16_t bits) { if(bits>=64) { int64_t value; memcpy(&value,&raw,sizeof(value)); return value; } uint64_t masked=raw&omask(bits),sign=UINT64_C(1)<<(bits-1); return masked&sign?(int64_t)masked-(int64_t)(UINT64_C(1)<<bits):(int64_t)masked; }
static OValue ouintbits(uint64_t x, uint16_t bits) { if(bits==0||bits>64) bits=64; return (OValue){O_UINT,bits,{.u=x&omask(bits)}}; }
static OValue ointbits(int64_t x, uint16_t bits) { if(bits==0||bits>64) bits=64; uint64_t raw=(uint64_t)x&omask(bits); if(bits<64&&(raw&(UINT64_C(1)<<(bits-1)))) raw|=~omask(bits); return (OValue){O_INT,bits,{.i=(int64_t)raw}}; }
static OValue ouint(uint64_t x) { return ouintbits(x,64); }
static OValue oint(int64_t x) { return ointbits(x,64); }
static OValue ofloat(double x) { return (OValue){O_FLOAT,0,{.f=x}}; }
static OValue ofloatbits(double x, uint16_t bits) { if(bits==32) x=(double)(float)x; return (OValue){O_FLOAT,bits,{.f=x}}; }
static OValue ocomplexbits(double real, double imag, uint16_t fbits) { return (OValue){O_COMPLEX,fbits,{.cx={real,imag}}}; }
static OValue ocomplex(double real, double imag) { return ocomplexbits(real,imag,0); }
static OValue odual(double primal, double tangent) { return (OValue){O_DUAL,0,{.dual={primal,tangent}}}; }
static int odualparts(OValue value, double *primal, double *tangent) {
  if(value.kind==O_DUAL) { *primal=value.as.dual.primal; *tangent=value.as.dual.tangent; return 1; }
  if(!onum(value,primal)) return 0;
  *tangent=0; return 1;
}
static OValue oprimal(OValue value) { return value.kind==O_DUAL?ofloat(value.as.dual.primal):value; }
static OValue otangent(OValue value) { double primal,tangent; return odualparts(value,&primal,&tangent)?ofloat(tangent):ofail("tangent expects a numeric value"); }
static OValue odualcall(const OValue *args, size_t count) {
  if(count!=2) return ofail("dual expects 2 arguments");
  double primal,tangent; if(!onum(args[0],&primal)||!onum(args[1],&tangent)) return ofail("dual expects numeric arguments");
  return odual(primal,tangent);
}
static OValue ostring(const char *x) { return (OValue){O_STRING,0,{.s={x,strlen(x)}}}; }
static OValue ostringview(const char *data, size_t len) { return (OValue){O_STRING,0,{.s={data,len}}}; }
static OValue oconcat(OValue a, OValue b) { if(a.kind!=O_STRING||b.kind!=O_STRING) return ofail("string concatenation expects strings"); if(a.as.s.len>SIZE_MAX-b.as.s.len-1) return ofail("string concatenation is too large"); size_t n=a.as.s.len+b.as.s.len; char *data=(char *)malloc(n+1); OAlloc *node=(OAlloc *)malloc(sizeof(OAlloc)); if(!data||!node) { free(data); free(node); return ofail("out of memory"); } memcpy(data,a.as.s.data,a.as.s.len); memcpy(data+a.as.s.len,b.as.s.data,b.as.s.len); data[n]=0; node->ptr=data; node->next=oallocs; oallocs=node; static int registered=0; if(!registered) { atexit(ofreeallocs); registered=1; } return (OValue){O_STRING,0,{.s={data,n}}}; }
static OValue olist(const OValue *items, size_t len) { if(len==0) return (OValue){O_LIST,0,{.list={NULL,0}}}; if(len>SIZE_MAX/sizeof(OValue)) return ofail("list is too large"); OValue *data=(OValue *)malloc(len*sizeof(OValue)); OAlloc *node=(OAlloc *)malloc(sizeof(OAlloc)); if(!data||!node) { free(data); free(node); return ofail("out of memory"); } memcpy(data,items,len*sizeof(OValue)); node->ptr=data; node->next=oallocs; oallocs=node; static int registered=0; if(!registered) { atexit(ofreeallocs); registered=1; } return (OValue){O_LIST,0,{.list={data,len}}}; }
static OValue omap(const char **keys, const OValue *values, size_t len) {
  OMap *map=(OMap *)malloc(sizeof(OMap)); if(!map) return ofail("out of memory"); map->entries=len?(OMapEntry *)malloc(len*sizeof(OMapEntry)):NULL; if(len&&!map->entries) { free(map); return ofail("out of memory"); } map->len=len; for(size_t i=0;i<len;i++) { map->entries[i].key=(OString){keys[i],strlen(keys[i])}; map->entries[i].value=values[i]; } return (OValue){O_MAP,0,{.map=map}};
}
static OValue omapnew(const OValue *items, size_t len) {
  if (len % 2 != 0) return ofail("map_new expects key/value pairs");
  size_t count = len / 2;
  OMap *map = (OMap *)malloc(sizeof(OMap));
  if (!map) return ofail("out of memory");
  map->entries = count ? (OMapEntry *)calloc(count, sizeof(OMapEntry)) : NULL;
  if (count && !map->entries) { free(map); return ofail("out of memory"); }
  map->len = 0;
  for (size_t i = 0; i < count; i++) {
    OValue key = items[i * 2];
    if (key.kind != O_STRING) { free(map->entries); free(map); return ofail("native map_new requires string keys"); }
    size_t key_length = key.as.s.len;
    char *key_copy = (char *)malloc(key_length + 1);
    if (!key_copy) { free(map->entries); free(map); return ofail("out of memory"); }
    memcpy(key_copy, key.as.s.data, key_length); key_copy[key_length] = 0;
    map->entries[map->len].key = (OString){key_copy, key_length};
    map->entries[map->len].value = items[i * 2 + 1];
    map->len++;
  }
  return (OValue){O_MAP, 0, {.map = map}};
}
static OValue *omapget(OValue value, const char *key) { if(value.kind!=O_MAP||!value.as.map) return NULL; for(size_t i=0;i<value.as.map->len;i++) if(value.as.map->entries[i].key.len==strlen(key)&&memcmp(value.as.map->entries[i].key.data,key,value.as.map->entries[i].key.len)==0) return &value.as.map->entries[i].value; return NULL; }
static OValue oenum(const char *type_name, const char *case_name, const OValue *payload, size_t count) { OValue values=olist(payload,count); if(count&&values.kind==O_NULL) return values; return (OValue){O_ENUM,0,{.en={{type_name,strlen(type_name)},{case_name,strlen(case_name)},values.as.list}}}; }
static OValue oenumis(OValue value, const char *type_name, const char *case_name, size_t payload_count) { return obool(value.kind==O_ENUM&&value.as.en.type_name.len==strlen(type_name)&&value.as.en.case_name.len==strlen(case_name)&&value.as.en.payload.len==payload_count&&!memcmp(value.as.en.type_name.data,type_name,value.as.en.type_name.len)&&!memcmp(value.as.en.case_name.data,case_name,value.as.en.case_name.len)); }
static OValue oenumpayload(OValue value, size_t index) { return value.kind==O_ENUM&&index<value.as.en.payload.len?value.as.en.payload.items[index]:ofail("enum payload index out of range"); }
static OValue olistis(OValue value, size_t count) { return obool(value.kind==O_LIST&&value.as.list.len==count); }
static OValue olistitem(OValue value, size_t index) { return value.kind==O_LIST&&index<value.as.list.len?value.as.list.items[index]:ofail("list pattern index out of range"); }
static OValue olistatunchecked(OValue value, OValue index) { return value.as.list.items[(size_t)index.as.f]; }
static OValue omatchtype(OValue value, const char *name) { if(!strcmp(name,"number")) return obool(value.kind==O_FLOAT||value.kind==O_INT||value.kind==O_UINT); if(!strcmp(name,"integer")||!strcmp(name,"int")) return obool(value.kind==O_INT||value.kind==O_UINT); if(!strcmp(name,"string")) return obool(value.kind==O_STRING); if(!strcmp(name,"list")) return obool(value.kind==O_LIST); if(!strcmp(name,"tensor")) return obool(value.kind==O_TENSOR||value.kind==O_COMPLEX_TENSOR); if(!strcmp(name,"bool")) return obool(value.kind==O_BOOL); if(!strcmp(name,"null")) return obool(value.kind==O_NULL); if(!strcmp(name,"enum")) return obool(value.kind==O_ENUM); if(!strcmp(name,"function")) return obool(value.kind==O_CLOSURE); if(!strcmp(name,"result")) return obool(value.kind==O_RESULT); if(!strcmp(name,"option")) return obool(value.kind==O_OPTION); if(!strcmp(name,"task")) return obool(value.kind==O_TASK); return obool(0); }
static OValue oasserttype(OValue value, const char *name) { int ok=0, known=1; if(!strcmp(name,"number")) ok=value.kind==O_FLOAT; else if(!strcmp(name,"u8")) ok=value.kind==O_UINT&&value.fbits==8; else if(!strcmp(name,"u16")) ok=value.kind==O_UINT&&value.fbits==16; else if(!strcmp(name,"u32")) ok=value.kind==O_UINT&&value.fbits==32; else if(!strcmp(name,"u64")) ok=value.kind==O_UINT&&value.fbits==64; else if(!strcmp(name,"i8")) ok=value.kind==O_INT&&value.fbits==8; else if(!strcmp(name,"i16")) ok=value.kind==O_INT&&value.fbits==16; else if(!strcmp(name,"i32")) ok=value.kind==O_INT&&value.fbits==32; else if(!strcmp(name,"i64")) ok=value.kind==O_INT&&value.fbits==64; else if(!strcmp(name,"f32")) ok=value.kind==O_FLOAT&&value.fbits==32; else if(!strcmp(name,"f64")) ok=value.kind==O_FLOAT&&value.fbits==64; else if(!strcmp(name,"string")) ok=value.kind==O_STRING; else if(!strcmp(name,"bool")) ok=value.kind==O_BOOL; else if(!strcmp(name,"null")) ok=value.kind==O_NULL; else if(!strcmp(name,"list")) ok=value.kind==O_LIST; else if(!strcmp(name,"tensor")) ok=value.kind==O_TENSOR||value.kind==O_COMPLEX_TENSOR; else if(!strcmp(name,"enum")) ok=value.kind==O_ENUM; else if(!strcmp(name,"function")) ok=value.kind==O_CLOSURE; else if(!strcmp(name,"complex")) ok=value.kind==O_COMPLEX; else if(!strcmp(name,"decimal")) ok=value.kind==O_DECIMAL; else if(!strcmp(name,"bigint")) ok=value.kind==O_BIGINT; else if(!strcmp(name,"quantity")) ok=value.kind==O_QUANTITY; else if(!strcmp(name,"datetime")) ok=value.kind==O_DATETIME; else if(!strcmp(name,"duration")) ok=value.kind==O_DURATION; else if(!strcmp(name,"error")) ok=value.kind==O_ERROR; else if(!strcmp(name,"result")) ok=value.kind==O_RESULT; else if(!strcmp(name,"option")) ok=value.kind==O_OPTION; else if(!strcmp(name,"task")) ok=value.kind==O_TASK; else if(!strncmp(name,"func(",5)) ok=value.kind==O_CLOSURE&&value.as.closure.signature&&strcmp(value.as.closure.signature,name)==0; else known=0; return ok||(!known&&name[0]>='A'&&name[0]<='Z')?value:ofail("binding type assertion failed"); }
static OValue oclosure(OCall call, const OValue *captures, size_t capture_count, const char *signature) { if(!call) return ofail("invalid closure"); if(capture_count==0) return (OValue){O_CLOSURE,0,{.closure={call,NULL,0,signature}}}; if(capture_count>SIZE_MAX/sizeof(OValue)) return ofail("closure captures are too large"); OValue *data=(OValue *)malloc(capture_count*sizeof(OValue)); OAlloc *node=(OAlloc *)malloc(sizeof(OAlloc)); if(!data||!node) { free(data); free(node); return ofail("out of memory"); } memcpy(data,captures,capture_count*sizeof(OValue)); node->ptr=data; node->next=oallocs; oallocs=node; static int registered=0; if(!registered) { atexit(ofreeallocs); registered=1; } return (OValue){O_CLOSURE,0,{.closure={call,data,capture_count,signature}}}; }
static OValue ocall(OValue fn, const OValue *args, size_t argc) { if(fn.kind!=O_CLOSURE||!fn.as.closure.call) return ofail("value is not callable"); return fn.as.closure.call(args,argc,fn.as.closure.captures,fn.as.closure.capture_count); }
static int otrack(void *ptr) { if(!ptr) return 1; OAlloc *node=(OAlloc *)malloc(sizeof(OAlloc)); if(!node) return 0; node->ptr=ptr; node->next=oallocs; oallocs=node; static int registered=0; if(!registered) { atexit(ofreeallocs); registered=1; } return 1; }
static OValue oexacttext(OKind kind, OValue text) {
  if(text.kind!=O_STRING||text.as.s.len==0) return ofail(kind==O_DECIMAL?"dec expects a non-empty string":"bigint expects a non-empty string");
  size_t start=0,end=text.as.s.len;
  while(start<end&&(text.as.s.data[start]==' '||text.as.s.data[start]=='\t'||text.as.s.data[start]=='\n')) start++;
  while(end>start&&(text.as.s.data[end-1]==' '||text.as.s.data[end-1]=='\t'||text.as.s.data[end-1]=='\n')) end--;
  if(start==end) return ofail(kind==O_DECIMAL?"invalid decimal literal":"invalid bigint literal");
  if(kind==O_DECIMAL&&end>start&&text.as.s.data[end-1]=='d') end--;
  if(kind==O_BIGINT&&end>start&&text.as.s.data[end-1]=='n') end--;
  if(start==end) return ofail(kind==O_DECIMAL?"invalid decimal literal":"invalid bigint literal");
  size_t len=end-start; char *copy=(char *)malloc(len+1); if(!copy) return ofail("out of memory");
  memcpy(copy,text.as.s.data+start,len); copy[len]=0;
  if(!otrack(copy)) { free(copy); return ofail("out of memory"); }
  return (OValue){kind,0,{.s={copy,len}}};
}
static OValue odecimal(OValue text) { return oexacttext(O_DECIMAL,text); }
static OValue obigint(OValue text) { return oexacttext(O_BIGINT,text); }
static OValue oquantityunit(const char *text) { if(!strncmp(text,"1*",2)) text+=2; return (OValue){O_QUANTITY,0,{.quantity={1.0,{text,strlen(text)}}}}; }
static OValue oerrorvalue(const char *kind, const char *message) { return (OValue){O_ERROR,0,{.error={.kind={kind,strlen(kind)},.message={message,strlen(message)}}}}; }
static OValue oerror(OValue kind, OValue message) { if(kind.kind!=O_STRING||message.kind!=O_STRING) return ofail("error expects a kind and message string"); return (OValue){O_ERROR,0,{.error={.kind=kind.as.s,.message=message.as.s}}}; }
static OValue oresultbox(OValue value, int success) { OValue *copy=(OValue *)malloc(sizeof(OValue)); if(!copy||!otrack(copy)) { free(copy); return ofail("out of memory"); } *copy=value; return (OValue){O_RESULT,(uint16_t)(success?1:0),{.box={copy}}}; }
static OValue oresultok(OValue value) { return oresultbox(value,1); }
static OValue oresultfail(OValue error) { return error.kind==O_ERROR?oresultbox(error,0):ofail("err expects an error value"); }
/* Option uses the same owned single-value box as Result, with fbits as the
   presence bit.  Keeping the payload boxed makes absent options unambiguous
   and prevents a null payload from being confused with none(). */
static OValue ooptionbox(OValue value, int present) { OValue *copy=(OValue *)malloc(sizeof(OValue)); if(!copy||!otrack(copy)) { free(copy); return ofail("out of memory"); } *copy=value; return (OValue){O_OPTION,(uint16_t)(present?1:0),{.box={copy}}}; }
static OValue ooptionSome(OValue value) { return ooptionbox(value,1); }
static OValue ooptionNone(void) { return ooptionbox(onull(),0); }
static OValue ooptionbind(OValue option) { if(option.kind!=O_OPTION||!option.as.box.value) return ofail("let? expects a result or option"); if(option.fbits) return *option.as.box.value; OValue error=oerrorvalue("not_found","option is empty"); ohaderror=1; olasterror=error; oerrormessage=error.as.error.message.data; return onull(); }
static OValue otoOption(OValue result) { if(result.kind!=O_RESULT||!result.as.box.value) return ofail("to_option expects a result"); return result.fbits?ooptionSome(*result.as.box.value):ooptionNone(); }
static OValue otoResult(OValue option, OValue message) { if(option.kind!=O_OPTION||!option.as.box.value) return ofail("to_result expects an option"); if(option.fbits) return oresultok(*option.as.box.value); if(message.kind==O_STRING) return oresultfail(oerrorvalue("not_found",message.as.s.data)); return oresultfail(oerrorvalue("not_found","option is empty")); }
static OValue ounwrap(OValue value) { if(value.kind==O_RESULT) { if(value.fbits&&value.as.box.value) return *value.as.box.value; if(value.as.box.value&&value.as.box.value->kind==O_ERROR) { ohaderror=1; olasterror=*value.as.box.value; oerrormessage=olasterror.as.error.message.data; return onull(); } return ofail("result contains an invalid error"); } return value.kind==O_OPTION?ooptionbind(value):ofail("unwrap expects a result or option"); }
static OValue oexpect(OValue value, OValue message) { if(message.kind!=O_STRING) return ofail("expect message must be a string"); if(value.kind==O_OPTION) { if(value.fbits&&value.as.box.value) return *value.as.box.value; return ofail(message.as.s.data); } if(value.kind==O_RESULT) { if(value.fbits&&value.as.box.value) return *value.as.box.value; return ofail(message.as.s.data); } return ofail("expect expects a result or option"); }
static OValue oorElse(OValue value, OValue fallback) { if(value.kind==O_OPTION) return value.fbits&&value.as.box.value?*value.as.box.value:fallback; if(value.kind==O_RESULT) return value.fbits&&value.as.box.value?*value.as.box.value:fallback; return ofail("or_else expects a result or option"); }
static OValue ocollect(OValue values) { if(values.kind!=O_LIST) return ofail("collect expects a list"); OValue *items=values.as.list.len?(OValue *)malloc(values.as.list.len*sizeof(OValue)):NULL; if(values.as.list.len&&!items) return ofail("out of memory"); for(size_t i=0;i<values.as.list.len;i++) { OValue item=values.as.list.items[i]; if(item.kind==O_OPTION) { if(!item.fbits||!item.as.box.value) { free(items); return oresultfail(oerrorvalue("not_found","option is empty")); } items[i]=*item.as.box.value; } else if(item.kind==O_RESULT) { if(!item.fbits||!item.as.box.value) { OValue error=item.as.box.value?*item.as.box.value:oerrorvalue("unknown_error","result is missing error payload"); free(items); return oresultfail(error); } items[i]=*item.as.box.value; } else { free(items); return ofail("collect expects results or options"); } } OValue out=olist(items,values.as.list.len); free(items); return oresultok(out); }
typedef struct OMethodEntry { const char *receiver_type; const char *name; OCall call; const char *signature; struct OMethodEntry *next; } OMethodEntry;
static OMethodEntry *omethods = NULL;
static void oregistermethod(const char *receiver_type, const char *name, OCall call, const char *signature) { OMethodEntry *entry=(OMethodEntry *)malloc(sizeof(OMethodEntry)); if(!entry) { ofail("out of memory"); return; } entry->receiver_type=receiver_type; entry->name=name; entry->call=call; entry->signature=signature; entry->next=omethods; omethods=entry; }
static int omethodtype(OValue value, const char *type) { if(!strcmp(type,"bool")) return value.kind==O_BOOL; if(!strcmp(type,"string")) return value.kind==O_STRING; if(!strcmp(type,"number")) return value.kind==O_FLOAT; if(!strcmp(type,"f32")) return value.kind==O_FLOAT&&value.fbits==32; if(!strcmp(type,"f64")) return value.kind==O_FLOAT&&value.fbits==64; if(!strcmp(type,"uint")) return value.kind==O_UINT; if(!strcmp(type,"int")) return value.kind==O_INT; if(!strncmp(type,"u",1)) return value.kind==O_UINT&&value.fbits==(uint16_t)strtoul(type+1,NULL,10); if(!strncmp(type,"i",1)) return value.kind==O_INT&&value.fbits==(uint16_t)strtoul(type+1,NULL,10); return 0; }
static OValue omember(OValue value, const char *name) { if(value.kind==O_MAP) { OValue *found=omapget(value,name); if(found) return *found; } if(value.kind==O_ERROR) { if(!strcmp(name,"kind")||!strcmp(name,"code")) return (OValue){O_STRING,0,{.s=value.as.error.kind}}; if(!strcmp(name,"message")||!strcmp(name,"user_message")) return (OValue){O_STRING,0,{.s=value.as.error.message}}; if(!strcmp(name,"operation")) return (OValue){O_STRING,0,{.s=value.as.error.operation}}; if(!strcmp(name,"resource")) return (OValue){O_STRING,0,{.s=value.as.error.resource}}; if(!strcmp(name,"source")||!strcmp(name,"location")) return (OValue){O_STRING,0,{.s=value.as.error.source}}; if(!strcmp(name,"retryable")) return obool(value.as.error.retryable); if(!strcmp(name,"severity")) return oint(value.as.error.severity); if(!strcmp(name,"cause")) return value.as.error.cause?*value.as.error.cause:onull(); if(!strcmp(name,"stack")) return value.as.error.stack?*value.as.error.stack:olist(NULL,0); if(!strcmp(name,"metadata")) return value.as.error.metadata?*value.as.error.metadata:olist(NULL,0); } if(value.kind==O_OPTION) { if(!strcmp(name,"present")||!strcmp(name,"is_some")) return obool(value.fbits!=0); if(!strcmp(name,"is_none")) return obool(value.fbits==0); if(!strcmp(name,"value")) return ooptionbind(value); } for(OMethodEntry *entry=omethods; entry; entry=entry->next) { if(!strcmp(entry->name,name)&&omethodtype(value,entry->receiver_type)) return oclosure(entry->call,&value,1,entry->signature); } return ofail("member is not available on this value"); }
static OValue orotl(OValue value, OValue shift) { if(value.kind!=O_UINT||shift.kind!=O_UINT) return ofail("rotl expects unsigned integers"); uint16_t bits=value.fbits?value.fbits:64; uint64_t n=shift.as.u%bits, raw=value.as.u&omask(bits); return ouintbits(n?((raw<<n)|(raw>>(bits-n))):raw,bits); }
static OValue otask(OValue thunk) { if(thunk.kind!=O_CLOSURE) return ofail("spawn expects a callable thunk"); OTask *task=(OTask *)calloc(1,sizeof(OTask)); if(!task||!otrack(task)) { free(task); return ofail("out of memory"); } task->thunk=thunk; task->result=onull(); task->error=onull(); return (OValue){O_TASK,0,{.task=task}}; }
static OValue oawait(OValue value) { if(value.kind!=O_TASK||!value.as.task) return ofail("await expects a task"); OTask *task=value.as.task; if(!task->state) { int outer=ohaderror; const char *outer_message=oerrormessage; OValue outer_error=olasterror; ohaderror=0; task->result=ocall(task->thunk,NULL,0); task->failed=ohaderror; task->error=olasterror; task->state=1; ohaderror=outer; oerrormessage=outer_message; olasterror=outer_error; } if(task->failed) { ohaderror=1; olasterror=task->error; oerrormessage=task->error.kind==O_ERROR?task->error.as.error.message.data:"task failed"; return onull(); } return task->result; }
static OValue ocancel(OValue value) { if(value.kind!=O_TASK||!value.as.task) return ofail("cancel expects a task"); OTask *task=value.as.task; if(task->state==1) return obool(0); if(task->state==0) { task->state=2; task->failed=1; task->error=oerrorvalue("cancelled","context canceled"); } return obool(1); }
static OValue oparallel(const OValue *thunks, size_t count) { OValue *values=count?(OValue *)malloc(count*sizeof(OValue)):NULL; if(count&&!values) return ofail("out of memory"); for(size_t i=0;i<count;i++) { values[i]=ocall(thunks[i],NULL,0); if(ohaderror) { free(values); return onull(); } } OValue result=olist(values,count); free(values); return result; }
static OValue otry(OValue body, OValue handler) { int outer=ohaderror; const char *outer_message=oerrormessage; OValue outer_error=olasterror; ohaderror=0; OValue result=ocall(body,NULL,0); if(!ohaderror) { ohaderror=outer; oerrormessage=outer_message; olasterror=outer_error; return result; } OValue error=olasterror.kind==O_ERROR?olasterror:oerrorvalue("unknown_error",oerrormessage); ohaderror=0; result=ocall(handler,&error,1); if(!ohaderror) { ohaderror=outer; oerrormessage=outer_message; olasterror=outer_error; } return result; }
static OValue oresultbind(OValue result) { if(result.kind==O_OPTION) return ooptionbind(result); if(result.kind!=O_RESULT||!result.as.box.value) return ofail("let? expects a result or option"); if(result.fbits) return *result.as.box.value; OValue error=*result.as.box.value; if(error.kind!=O_ERROR) return ofail("result contains an invalid error"); ohaderror=1; olasterror=error; oerrormessage=error.as.error.message.data; return onull(); }
static OValue otensordata_kind(double *data, size_t n, const size_t *shape, size_t rank, uint16_t fbits, unsigned char element_kind) { size_t *dims=rank?(size_t *)malloc(rank*sizeof(size_t)):NULL; if(rank&&!dims) { free(data); return ofail("out of memory"); } if(rank) memcpy(dims,shape,rank*sizeof(size_t)); if(!otrack(data)||!otrack(dims)) return ofail("out of memory"); return (OValue){O_TENSOR,fbits,{.tensor={data,n,dims,rank,element_kind}}}; }
static OValue otensordatawidth(double *data, size_t n, const size_t *shape, size_t rank, uint16_t fbits) { return otensordata_kind(data,n,shape,rank,fbits,0); }
static OValue otensordata(double *data, size_t n, const size_t *shape, size_t rank) { return otensordatawidth(data,n,shape,rank,0); }
static OValue ocomplextensordata(OComplex *data, size_t n, const size_t *shape, size_t rank) { size_t *dims=rank?(size_t *)malloc(rank*sizeof(size_t)):NULL; if(rank&&!dims) { free(data); return ofail("out of memory"); } if(rank) memcpy(dims,shape,rank*sizeof(size_t)); if(!otrack(data)||!otrack(dims)) return ofail("out of memory"); return (OValue){O_COMPLEX_TENSOR,0,{.ctensor={data,n,dims,rank}}}; }
typedef struct { size_t *shape; size_t rank; double *data; size_t len; uint16_t fbits; unsigned char element_kind; } OTensorParts;
static void ofreeparts(OTensorParts part) { free(part.shape); free(part.data); }
static int oinfer_tensor(OValue x, OTensorParts *out) { memset(out,0,sizeof(*out)); if(x.kind==O_TENSOR) { out->rank=x.as.tensor.rank; out->len=x.as.tensor.len; out->fbits=x.fbits; out->shape=out->rank?(size_t *)malloc(out->rank*sizeof(size_t)):NULL; out->data=out->len?(double *)malloc(out->len*sizeof(double)):NULL; if((out->rank&&!out->shape)||(out->len&&!out->data)) { ofreeparts(*out); return 0; } if(out->rank) memcpy(out->shape,x.as.tensor.shape,out->rank*sizeof(size_t)); if(out->len) memcpy(out->data,x.as.tensor.data,out->len*sizeof(double)); return 1; } if(x.kind!=O_LIST) { double value; if(!onum(x,&value)) return 0; out->data=(double *)malloc(sizeof(double)); if(!out->data) return 0; out->data[0]=value; out->len=1; out->fbits=x.kind==O_FLOAT?x.fbits:0; return 1; } if(x.as.list.len==0) { out->shape=(size_t *)malloc(sizeof(size_t)); if(!out->shape) return 0; out->shape[0]=0; out->rank=1; return 1; } OTensorParts first; if(!oinfer_tensor(x.as.list.items[0],&first)) return 0; size_t total=first.len; double *data=first.data; first.data=NULL; for(size_t i=1;i<x.as.list.len;i++) { OTensorParts child; if(!oinfer_tensor(x.as.list.items[i],&child)) { free(data); ofreeparts(first); return 0; } if(child.rank!=first.rank||memcmp(child.shape,first.shape,first.rank*sizeof(size_t))) { ofreeparts(child); free(data); ofreeparts(first); return 0; } if(child.fbits>first.fbits) first.fbits=child.fbits; if(child.len>SIZE_MAX-total) { ofreeparts(child); free(data); ofreeparts(first); return 0; } double *grown=(double *)realloc(data,(total+child.len)*sizeof(double)); if(child.len&&!grown) { ofreeparts(child); free(data); ofreeparts(first); return 0; } data=grown; if(child.len) memcpy(data+total,child.data,child.len*sizeof(double)); total+=child.len; ofreeparts(child); } size_t *shape=(size_t *)malloc((first.rank+1)*sizeof(size_t)); if(!shape) { free(data); ofreeparts(first); return 0; } shape[0]=x.as.list.len; if(first.rank) memcpy(shape+1,first.shape,first.rank*sizeof(size_t)); free(first.shape); out->shape=shape; out->rank=first.rank+1; out->data=data; out->len=total; out->fbits=first.fbits; return 1; }
typedef struct { size_t *shape; size_t rank; OComplex *data; size_t len; } OComplexTensorParts;
static void ofreecomplexparts(OComplexTensorParts part) { free(part.shape); free(part.data); }
static int oinfer_complextensor(OValue x, OComplexTensorParts *out) { memset(out,0,sizeof(*out)); if(x.kind==O_COMPLEX_TENSOR) { out->rank=x.as.ctensor.rank; out->len=x.as.ctensor.len; out->shape=out->rank?(size_t *)malloc(out->rank*sizeof(size_t)):NULL; out->data=out->len?(OComplex *)malloc(out->len*sizeof(OComplex)):NULL; if((out->rank&&!out->shape)||(out->len&&!out->data)) { ofreecomplexparts(*out); return 0; } if(out->rank) memcpy(out->shape,x.as.ctensor.shape,out->rank*sizeof(size_t)); if(out->len) memcpy(out->data,x.as.ctensor.data,out->len*sizeof(OComplex)); return 1; } if(x.kind==O_TENSOR) { out->rank=x.as.tensor.rank; out->len=x.as.tensor.len; out->shape=out->rank?(size_t *)malloc(out->rank*sizeof(size_t)):NULL; out->data=out->len?(OComplex *)malloc(out->len*sizeof(OComplex)):NULL; if((out->rank&&!out->shape)||(out->len&&!out->data)) { ofreecomplexparts(*out); return 0; } if(out->rank) memcpy(out->shape,x.as.tensor.shape,out->rank*sizeof(size_t)); for(size_t i=0;i<out->len;i++) out->data[i]=(OComplex){x.as.tensor.data[i],0}; return 1; } if(x.kind!=O_LIST) { OComplex value; if(!ocomplexnum(x,&value)) return 0; out->data=(OComplex *)malloc(sizeof(OComplex)); if(!out->data) return 0; out->data[0]=value; out->len=1; return 1; } if(x.as.list.len==0) { out->shape=(size_t *)malloc(sizeof(size_t)); if(!out->shape) return 0; out->shape[0]=0; out->rank=1; return 1; } OComplexTensorParts first; if(!oinfer_complextensor(x.as.list.items[0],&first)) return 0; size_t total=first.len; OComplex *data=first.data; first.data=NULL; for(size_t i=1;i<x.as.list.len;i++) { OComplexTensorParts child; if(!oinfer_complextensor(x.as.list.items[i],&child)) { free(data); ofreecomplexparts(first); return 0; } if(child.rank!=first.rank||memcmp(child.shape,first.shape,first.rank*sizeof(size_t))) { ofreecomplexparts(child); free(data); ofreecomplexparts(first); return 0; } if(child.len>SIZE_MAX-total) { ofreecomplexparts(child); free(data); ofreecomplexparts(first); return 0; } OComplex *grown=(OComplex *)realloc(data,(total+child.len)*sizeof(OComplex)); if(child.len&&!grown) { ofreecomplexparts(child); free(data); ofreecomplexparts(first); return 0; } data=grown; if(child.len) memcpy(data+total,child.data,child.len*sizeof(OComplex)); total+=child.len; ofreecomplexparts(child); } size_t *shape=(size_t *)malloc((first.rank+1)*sizeof(size_t)); if(!shape) { free(data); ofreecomplexparts(first); return 0; } shape[0]=x.as.list.len; if(first.rank) memcpy(shape+1,first.shape,first.rank*sizeof(size_t)); free(first.shape); out->shape=shape; out->rank=first.rank+1; out->data=data; out->len=total; return 1; }
static unsigned char otensorelementkind(OValue x) { if(x.kind==O_TENSOR) return x.as.tensor.element_kind; if(x.kind==O_LIST&&x.as.list.len) { unsigned char kind=0; for(size_t i=0;i<x.as.list.len;i++) { OValue item=x.as.list.items[i]; unsigned char current=item.kind==O_UINT?1:item.kind==O_INT?2:item.kind==O_LIST?otensorelementkind(item):0; if(!current||(kind&&kind!=current)) return 0; kind=current; } return kind; } return 0; }
static OValue otensor(OValue x) { OTensorParts real; if(oinfer_tensor(x,&real)) { OValue out=otensordata_kind(real.data,real.len,real.shape,real.rank,real.fbits,otensorelementkind(x)); free(real.shape); return out; } OComplexTensorParts complex; if(!oinfer_complextensor(x,&complex)) return ofail("tensor expects rectangular numeric data"); OValue out=ocomplextensordata(complex.data,complex.len,complex.shape,complex.rank); free(complex.shape); return out; }
static OValue otensorwithshape(OValue shape_arg, OValue data_arg) { size_t *shape,n,rank; if(!oshapeargs(&shape_arg,1,&shape,&rank,&n)) return ofail("tensor shape must contain non-negative integer dimensions"); OTensorParts real; if(oinfer_tensor(data_arg,&real)) { if(real.len!=n) { free(shape); ofreeparts(real); return ofail("tensor shape does not match data"); } OValue out=otensordata_kind(real.data,real.len,shape,rank,real.fbits,otensorelementkind(data_arg)); free(shape); free(real.shape); return out; } OComplexTensorParts complex; if(!oinfer_complextensor(data_arg,&complex)) { free(shape); return ofail("tensor expects rectangular numeric data"); } if(complex.len!=n) { free(shape); ofreecomplexparts(complex); return ofail("tensor shape does not match data"); } OValue out=ocomplextensordata(complex.data,complex.len,shape,rank); free(shape); free(complex.shape); return out; }
static OValue otensorcall(const OValue *args, size_t argc) { if(argc==1) return otensor(args[0]); if(argc==2) return otensorwithshape(args[0],args[1]); return ofail("tensor expects 1 or 2 arguments"); }
static OValue omatrixcall(const OValue *args, size_t argc) { if(argc<=2) return otensorcall(args,argc); if(argc!=3) return ofail("matrix expects 1, 2, or 3 arguments"); OValue shape=olist(args,2); return otensorwithshape(shape,args[2]); }
static int odimension(OValue x, size_t *out) { double n; if(!onum(x,&n)||n<0||n>(double)SIZE_MAX||floor(n)!=n) return 0; *out=(size_t)n; return 1; }
static int oshapeargs(const OValue *args, size_t argc, size_t **out_shape, size_t *out_rank, size_t *out_len) { if(argc==0) return 0; const OValue *values=args; size_t count=argc; if(argc==1&&args[0].kind==O_LIST) { values=args[0].as.list.items; count=args[0].as.list.len; } if(count==0||count>SIZE_MAX/sizeof(size_t)) return 0; size_t *shape=(size_t *)malloc(count*sizeof(size_t)), total=1; if(!shape) return 0; for(size_t i=0;i<count;i++) { if(!odimension(values[i],&shape[i])|| (shape[i]&&total>SIZE_MAX/shape[i])) { free(shape); return 0; } total*=shape[i]; } *out_shape=shape; *out_rank=count; *out_len=total; return 1; }
static OValue otensorfill(const OValue *shape_args, size_t argc, double fill) { size_t *shape,n,rank; if(!oshapeargs(shape_args,argc,&shape,&rank,&n)) return ofail("tensor dimensions must be non-negative integers"); if(n>SIZE_MAX/sizeof(double)) { free(shape); return ofail("tensor is too large"); } double *data=n?(double *)malloc(n*sizeof(double)):NULL; if(n&&!data) { free(shape); return ofail("out of memory"); } for(size_t i=0;i<n;i++) data[i]=fill; OValue out=otensordata(data,n,shape,rank); free(shape); return out; }
static OValue ozeros(const OValue *args, size_t argc) { return otensorfill(args,argc,0); }
static OValue oones(const OValue *args, size_t argc) { return otensorfill(args,argc,1); }
static OValue ofull(const OValue *args, size_t argc) { double fill; if(argc<2||!onum(args[argc-1],&fill)) return ofail("full expects dimensions and a numeric fill value"); return otensorfill(args,argc-1,fill); }
static OValue oidentity(const OValue *args, size_t argc) { if(argc!=1&&argc!=2) return ofail("identity expects 1 or 2 arguments"); size_t rows,cols; if(!odimension(args[0],&rows)) return ofail("identity size must be non-negative"); cols=rows; if(argc==2&&!odimension(args[1],&cols)) return ofail("identity size must be non-negative"); if(rows&&cols>SIZE_MAX/rows) return ofail("tensor is too large"); size_t n=rows*cols; double *data=n?(double *)calloc(n,sizeof(double)):NULL; if(n&&!data) return ofail("out of memory"); size_t diagonal=rows<cols?rows:cols; for(size_t i=0;i<diagonal;i++) data[i*cols+i]=1; size_t shape[2]={rows,cols}; return otensordata(data,n,shape,2); }
static OValue oeigh(OValue tensor) { if(tensor.kind!=O_TENSOR||tensor.as.tensor.rank!=2||tensor.as.tensor.shape[0]!=tensor.as.tensor.shape[1]) return ofail("eigh expects a square matrix"); size_t n=tensor.as.tensor.shape[0]; for(size_t i=0;i<n;i++) for(size_t j=i+1;j<n;j++) if(fabs(tensor.as.tensor.data[i*n+j]-tensor.as.tensor.data[j*n+i])>1e-9) return ofail("eigh expects a symmetric matrix"); if(n&&n>SIZE_MAX/n) return ofail("matrix is too large"); size_t count=n*n; double *input=count?(double *)malloc(count*sizeof(double)):NULL,*values=n?(double *)malloc(n*sizeof(double)):NULL,*vectors=count?(double *)malloc(count*sizeof(double)):NULL; if((count&&!input)||(n&&!values)||(count&&!vectors)) { free(input); free(values); free(vectors); return ofail("out of memory"); } if(count) memcpy(input,tensor.as.tensor.data,count*sizeof(double)); const OLinalgProviderV1 *provider=osiris_linalg_provider_v1(); OLinalgStatus status=provider&&provider->abi_version==1&&provider->eigh_f64?provider->eigh_f64(n,input,values,vectors):O_LINALG_UNAVAILABLE; free(input); if(status!=O_LINALG_OK) { free(values); free(vectors); return ofail(status==O_LINALG_NOT_CONVERGED?"eigh did not converge":"eigh numerical provider failed"); } size_t value_shape[1]={n},vector_shape[2]={n,n}; OValue output[2]={otensordata(values,n,value_shape,1),otensordata(vectors,count,vector_shape,2)}; return olist(output,2); }
static OValue osvd(OValue tensor) { if(tensor.kind!=O_TENSOR||tensor.as.tensor.rank!=2) return ofail("svd expects a matrix"); size_t rows=tensor.as.tensor.shape[0],cols=tensor.as.tensor.shape[1]; if((rows&&cols>SIZE_MAX/rows)||(cols&&cols>SIZE_MAX/cols)) return ofail("matrix is too large"); size_t input_count=rows*cols,vt_count=cols*cols; double *input=input_count?(double *)malloc(input_count*sizeof(double)):NULL,*u=input_count?(double *)malloc(input_count*sizeof(double)):NULL,*values=cols?(double *)malloc(cols*sizeof(double)):NULL,*vt=vt_count?(double *)malloc(vt_count*sizeof(double)):NULL; if((input_count&&!input)||(input_count&&!u)||(cols&&!values)||(vt_count&&!vt)) { free(input); free(u); free(values); free(vt); return ofail("out of memory"); } if(input_count) memcpy(input,tensor.as.tensor.data,input_count*sizeof(double)); const OLinalgProviderV1 *provider=osiris_linalg_provider_v1(); OLinalgStatus status=provider&&provider->abi_version==1&&provider->svd_f64?provider->svd_f64(rows,cols,input,u,values,vt):O_LINALG_UNAVAILABLE; free(input); if(status!=O_LINALG_OK) { free(u); free(values); free(vt); return ofail(status==O_LINALG_NOT_CONVERGED?"svd did not converge":"svd numerical provider failed"); } size_t ushape[2]={rows,cols},vshape[1]={cols},vtshape[2]={cols,cols}; OValue output[3]={otensordata(u,input_count,ushape,2),otensordata(values,cols,vshape,1),otensordata(vt,vt_count,vtshape,2)}; return olist(output,3); }
static int ocount(OValue x, size_t *out) { double n; if(!onum(x,&n)||n<0||n>(double)SIZE_MAX||floor(n)!=n) return 0; *out=(size_t)n; return 1; }
static OValue oreshape(OValue tensor, OValue shape_arg) { if(tensor.kind!=O_TENSOR) return ofail("reshape expects a tensor"); size_t *shape,n,rank; if(!oshapeargs(&shape_arg,1,&shape,&rank,&n)) return ofail("reshape expects non-negative integer dimensions"); if(n!=tensor.as.tensor.len) { free(shape); return ofail("tensor shape does not match data"); } double *data=n?(double *)malloc(n*sizeof(double)):NULL; if(n&&!data) { free(shape); return ofail("out of memory"); } if(n) memcpy(data,tensor.as.tensor.data,n*sizeof(double)); OValue out=otensordata(data,n,shape,rank); free(shape); return out; }
static OValue otranspose(OValue tensor) { if(tensor.kind!=O_TENSOR||tensor.as.tensor.rank!=2) return ofail("transpose expects a 2D matrix"); size_t rows=tensor.as.tensor.shape[0], cols=tensor.as.tensor.shape[1], n=tensor.as.tensor.len; double *data=n?(double *)malloc(n*sizeof(double)):NULL; if(n&&!data) return ofail("out of memory"); for(size_t row=0;row<rows;row++) for(size_t col=0;col<cols;col++) data[col*rows+row]=tensor.as.tensor.data[row*cols+col]; size_t shape[2]={cols,rows}; return otensordata(data,n,shape,2); }
static OValue omatmul(OValue left, OValue right) { if(left.kind!=O_TENSOR||right.kind!=O_TENSOR||left.as.tensor.rank!=2||right.as.tensor.rank!=2) return ofail("matmul expects 2D matrices"); size_t rows=left.as.tensor.shape[0], mid=left.as.tensor.shape[1], right_rows=right.as.tensor.shape[0], cols=right.as.tensor.shape[1]; if(mid!=right_rows) return ofail("matmul shape mismatch"); if(rows&&cols>SIZE_MAX/rows) return ofail("tensor is too large"); size_t n=rows*cols; double *data=n?(double *)malloc(n*sizeof(double)):NULL; if(n&&!data) return ofail("out of memory"); for(size_t row=0;row<rows;row++) for(size_t col=0;col<cols;col++) { volatile double total=0; for(size_t k=0;k<mid;k++) { volatile double product=left.as.tensor.data[row*mid+k]*right.as.tensor.data[k*cols+col]; total=total+product; } data[row*cols+col]=total; } size_t shape[2]={rows,cols}; return otensordatawidth(data,n,shape,2,left.fbits>right.fbits?left.fbits:right.fbits); }
static OValue oouter(OValue left, OValue right) { if((left.kind!=O_TENSOR&&left.kind!=O_COMPLEX_TENSOR)||(right.kind!=O_TENSOR&&right.kind!=O_COMPLEX_TENSOR)) return ofail("outer expects tensors"); size_t rows=left.kind==O_TENSOR?left.as.tensor.len:left.as.ctensor.len,cols=right.kind==O_TENSOR?right.as.tensor.len:right.as.ctensor.len; if(rows&&cols>SIZE_MAX/rows) return ofail("tensor is too large"); size_t count=rows*cols; size_t shape[2]={rows,cols}; if(left.kind==O_TENSOR&&right.kind==O_TENSOR) { double *data=count?(double *)malloc(count*sizeof(double)):NULL; if(count&&!data) return ofail("out of memory"); for(size_t row=0;row<rows;row++) for(size_t col=0;col<cols;col++) data[row*cols+col]=left.as.tensor.data[row]*right.as.tensor.data[col]; return otensordata(data,count,shape,2); } OComplex *data=count?(OComplex *)malloc(count*sizeof(OComplex)):NULL; if(count&&!data) return ofail("out of memory"); for(size_t row=0;row<rows;row++) for(size_t col=0;col<cols;col++) { OComplex a=left.kind==O_TENSOR?(OComplex){left.as.tensor.data[row],0}:left.as.ctensor.data[row],b=right.kind==O_TENSOR?(OComplex){right.as.tensor.data[col],0}:right.as.ctensor.data[col]; data[row*cols+col]=(OComplex){a.real*b.real-a.imag*b.imag,a.real*b.imag+a.imag*b.real}; } return ocomplextensordata(data,count,shape,2); }
static OValue otrace(OValue tensor) { if(tensor.kind!=O_TENSOR||tensor.as.tensor.rank!=2) return ofail("trace expects a matrix"); size_t rows=tensor.as.tensor.shape[0],cols=tensor.as.tensor.shape[1],limit=rows<cols?rows:cols; double total=0; for(size_t i=0;i<limit;i++) total+=tensor.as.tensor.data[i*cols+i]; return ofloat(total); }
static OValue odet(OValue tensor) { if(tensor.kind!=O_TENSOR||tensor.as.tensor.rank!=2||tensor.as.tensor.shape[0]!=tensor.as.tensor.shape[1]) return ofail("det expects a square matrix"); size_t n=tensor.as.tensor.shape[0]; if(n==0) return ofloat(1); if(n>SIZE_MAX/n||n*n>SIZE_MAX/sizeof(double)) return ofail("matrix is too large"); double *data=(double *)malloc(n*n*sizeof(double)); if(!data) return ofail("out of memory"); memcpy(data,tensor.as.tensor.data,n*n*sizeof(double)); double sign=1,det=1; for(size_t i=0;i<n;i++) { size_t pivot=i; double max_abs=fabs(data[i*n+i]); for(size_t row=i+1;row<n;row++) { double cur=fabs(data[row*n+i]); if(cur>max_abs) { max_abs=cur; pivot=row; } } if(max_abs==0) { free(data); return ofloat(0); } if(pivot!=i) { for(size_t col=0;col<n;col++) { double tmp=data[i*n+col]; data[i*n+col]=data[pivot*n+col]; data[pivot*n+col]=tmp; } sign=-sign; } double piv=data[i*n+i]; det*=piv; for(size_t row=i+1;row<n;row++) { double factor=data[row*n+i]/piv; for(size_t col=i;col<n;col++) data[row*n+col]-=factor*data[i*n+col]; } } free(data); return ofloat(sign*det); }
static OValue osolve(OValue matrix, OValue rhs) { if(matrix.kind!=O_TENSOR||matrix.as.tensor.rank!=2||matrix.as.tensor.shape[0]!=matrix.as.tensor.shape[1]) return ofail("solve expects a square matrix"); if(rhs.kind!=O_TENSOR) return ofail("solve rhs must be a vector or matrix"); size_t n=matrix.as.tensor.shape[0],m; if(rhs.as.tensor.rank==1) { if(rhs.as.tensor.shape[0]!=n) return ofail("solve dimension mismatch"); m=1; } else if(rhs.as.tensor.rank==2) { if(rhs.as.tensor.shape[0]!=n) return ofail("solve dimension mismatch"); m=rhs.as.tensor.shape[1]; } else return ofail("solve rhs must be a vector or matrix"); if(n==0) return ofail("solve expects a non-empty matrix"); if(m>SIZE_MAX-n||n>(SIZE_MAX/(n+m))||n*(n+m)>SIZE_MAX/sizeof(double)) return ofail("matrix is too large"); size_t width=n+m; double *aug=(double *)malloc(n*width*sizeof(double)); if(!aug) return ofail("out of memory"); for(size_t row=0;row<n;row++) { memcpy(aug+row*width,matrix.as.tensor.data+row*n,n*sizeof(double)); for(size_t col=0;col<m;col++) aug[row*width+n+col]=rhs.as.tensor.data[row*m+col]; } for(size_t i=0;i<n;i++) { size_t pivot=i; double max_abs=fabs(aug[i*width+i]); for(size_t row=i+1;row<n;row++) { double cur=fabs(aug[row*width+i]); if(cur>max_abs) { max_abs=cur; pivot=row; } } if(max_abs==0) { free(aug); return ofail("solve matrix is singular"); } if(pivot!=i) for(size_t col=0;col<width;col++) { double tmp=aug[i*width+col]; aug[i*width+col]=aug[pivot*width+col]; aug[pivot*width+col]=tmp; } double piv=aug[i*width+i]; for(size_t col=i;col<width;col++) aug[i*width+col]/=piv; for(size_t row=0;row<n;row++) { if(row==i) continue; double factor=aug[row*width+i]; for(size_t col=i;col<width;col++) aug[row*width+col]-=factor*aug[i*width+col]; } } if(n&&m>SIZE_MAX/n) { free(aug); return ofail("matrix is too large"); } size_t count=n*m; double *data=count?(double *)malloc(count*sizeof(double)):NULL; if(count&&!data) { free(aug); return ofail("out of memory"); } for(size_t row=0;row<n;row++) if(m) memcpy(data+row*m,aug+row*width+n,m*sizeof(double)); free(aug); if(m==1) { size_t shape[1]={n}; return otensordata(data,count,shape,1); } size_t shape[2]={n,m}; return otensordata(data,count,shape,2); }
static OValue oinv(OValue matrix) { if(matrix.kind!=O_TENSOR||matrix.as.tensor.rank!=2||matrix.as.tensor.shape[0]!=matrix.as.tensor.shape[1]) return ofail("inv expects a square matrix"); size_t n=matrix.as.tensor.shape[0]; if(n==0) return ofail("inv expects a non-empty matrix"); if(n>SIZE_MAX/n) return ofail("matrix is too large"); size_t count=n*n; double *data=(double *)calloc(count,sizeof(double)); if(!data) return ofail("out of memory"); for(size_t i=0;i<n;i++) data[i*n+i]=1; size_t shape[2]={n,n}; OValue rhs=otensordata(data,count,shape,2); return osolve_real(matrix,rhs); }
static OValue ocholesky(OValue matrix) { if(matrix.kind!=O_TENSOR||matrix.as.tensor.rank!=2||matrix.as.tensor.shape[0]!=matrix.as.tensor.shape[1]) return ofail("cholesky expects a square matrix"); size_t n=matrix.as.tensor.shape[0]; if(n&&n>SIZE_MAX/n) return ofail("matrix is too large"); size_t count=n*n; double *data=count?(double *)calloc(count,sizeof(double)):NULL; if(count&&!data) return ofail("out of memory"); for(size_t i=0;i<n;i++) for(size_t j=0;j<=i;j++) { double sum=matrix.as.tensor.data[i*n+j]; for(size_t k=0;k<j;k++) sum-=data[i*n+k]*data[j*n+k]; if(i==j) { if(sum<=0) { free(data); return ofail("cholesky expects positive definite matrix"); } data[i*n+j]=sqrt(sum); } else data[i*n+j]=sum/data[j*n+j]; } size_t shape[2]={n,n}; return otensordata(data,count,shape,2); }
static OValue oqr(OValue matrix) { if(matrix.kind!=O_TENSOR||matrix.as.tensor.rank!=2) return ofail("qr expects a matrix"); size_t rows=matrix.as.tensor.shape[0],cols=matrix.as.tensor.shape[1]; if((rows&&cols>SIZE_MAX/rows)||(cols&&cols>SIZE_MAX/cols)) return ofail("matrix is too large"); size_t qcount=rows*cols,rcount=cols*cols; if(qcount>SIZE_MAX/sizeof(double)||rcount>SIZE_MAX/sizeof(double)||rows>SIZE_MAX/sizeof(double)) return ofail("matrix is too large"); double *q=qcount?(double *)calloc(qcount,sizeof(double)):NULL,*r=rcount?(double *)calloc(rcount,sizeof(double)):NULL,*v=rows?(double *)malloc(rows*sizeof(double)):NULL; if((qcount&&!q)||(rcount&&!r)||(rows&&!v)) { free(q); free(r); free(v); return ofail("out of memory"); } for(size_t j=0;j<cols;j++) { for(size_t row=0;row<rows;row++) v[row]=matrix.as.tensor.data[row*cols+j]; for(size_t i=0;i<j;i++) { double dot=0; for(size_t row=0;row<rows;row++) dot+=q[row*cols+i]*matrix.as.tensor.data[row*cols+j]; r[i*cols+j]=dot; for(size_t row=0;row<rows;row++) v[row]-=dot*q[row*cols+i]; } double norm=0; for(size_t row=0;row<rows;row++) norm+=v[row]*v[row]; norm=sqrt(norm); if(norm==0) continue; r[j*cols+j]=norm; for(size_t row=0;row<rows;row++) q[row*cols+j]=v[row]/norm; } free(v); size_t qshape[2]={rows,cols},rshape[2]={cols,cols}; OValue values[2]={otensordata(q,qcount,qshape,2),otensordata(r,rcount,rshape,2)}; return olist(values,2); }
static OValue oarange(const OValue *args, size_t argc) { double start=0,stop=0,step=1; if(argc<1||argc>3) return ofail("arange expects 1, 2, or 3 arguments"); if(!onum(args[0],&start)) return ofail("arange expects numeric arguments"); if(argc==1) { stop=start; start=0; } else if(!onum(args[1],&stop)) return ofail("arange expects numeric arguments"); if(argc==3&&!onum(args[2],&step)) return ofail("arange expects numeric arguments"); if(step==0) return ofail("arange step must not be zero"); size_t n=0,cap=0; double *data=NULL; for(double x=start; step>0?x<stop:x>stop; x+=step) { if(n>1000000) { free(data); return ofail("arange generated too many elements"); } if(n==cap) { size_t next=cap?cap*2:16; if(next>SIZE_MAX/sizeof(double)) { free(data); return ofail("tensor is too large"); } double *grown=(double *)realloc(data,next*sizeof(double)); if(!grown) { free(data); return ofail("out of memory"); } data=grown; cap=next; } data[n++]=x; } size_t shape[1]={n}; return otensordata(data,n,shape,1); }
static OValue olinearspace(const OValue *args, size_t argc, int logarithmic) { if(argc<(logarithmic?3:2)||argc>(logarithmic?4:3)) return ofail(logarithmic?"logspace expects 3 or 4 arguments":"linspace expects 2 or 3 arguments"); double start,stop,base=10; size_t count=logarithmic?0:50; if(!onum(args[0],&start)||!onum(args[1],&stop)) return ofail(logarithmic?"logspace expects numeric arguments":"linspace expects numeric arguments"); if(logarithmic) { if(!ocount(args[2],&count)) return ofail("logspace count must be a non-negative integer"); if(argc==4&&!onum(args[3],&base)) return ofail("logspace base must be numeric"); } else if(argc==3&&!ocount(args[2],&count)) return ofail("linspace count must be a non-negative integer"); if(count>SIZE_MAX/sizeof(double)) return ofail("tensor is too large"); double *data=count?(double *)malloc(count*sizeof(double)):NULL; if(count&&!data) return ofail("out of memory"); if(count==1) data[0]=logarithmic?pow(base,start):start; else if(count>1) { double step=(stop-start)/(double)(count-1); for(size_t i=0;i<count;i++) { double x=start+(double)i*step; data[i]=logarithmic?pow(base,x):x; } } size_t shape[1]={count}; return otensordata(data,count,shape,1); }
static OValue olinspace(const OValue *args, size_t argc) { return olinearspace(args,argc,0); }
static OValue ologspace(const OValue *args, size_t argc) { return olinearspace(args,argc,1); }
static uint16_t ofloatwidthmany(const OValue *values, size_t count) { uint16_t bits=0; int explicit=0,def=0; for(size_t i=0;i<count;i++) { if(values[i].kind==O_FLOAT&&values[i].fbits) { explicit=1; if(values[i].fbits>bits) bits=values[i].fbits; } else def=1; } return explicit&&def&&bits<64?64:bits; }
static OValue ominmax(const OValue *values, size_t count, int want_max) { if(count==0) return ofail("expected at least 1 argument"); double best; if(!onum(values[0],&best)) return ofail("min/max expects numeric arguments"); for(size_t i=1;i<count;i++) { double next; if(!onum(values[i],&next)) return ofail("min/max expects numeric arguments"); if((want_max&&next>best)||(!want_max&&next<best)) best=next; } return ofloatbits(best,ofloatwidthmany(values,count)); }
static OValue oclamp(OValue value, OValue low, OValue high) { double x,lo,hi; if(!onum(value,&x)||!onum(low,&lo)||!onum(high,&hi)) return ofail("clamp expects numeric arguments"); if(x<lo) x=lo; else if(x>hi) x=hi; OValue values[3]={value,low,high}; return ofloatbits(x,ofloatwidthmany(values,3)); }
static int otruthy(OValue x) { switch(x.kind) { case O_BOOL:return x.as.b; case O_UINT:return x.as.u!=0; case O_INT:return x.as.i!=0; case O_FLOAT:return x.as.f!=0; case O_COMPLEX:return x.as.cx.real!=0||x.as.cx.imag!=0; case O_DUAL:return x.as.dual.primal!=0||x.as.dual.tangent!=0; case O_STRING:return x.as.s.len!=0; case O_LIST:return x.as.list.len!=0; case O_TENSOR:return x.as.tensor.len!=0; case O_COMPLEX_TENSOR:return x.as.ctensor.len!=0; case O_CLOSURE:return 1; case O_ENUM:return 1; case O_OPTION:return x.fbits!=0; case O_RESULT:return x.fbits!=0; default:return 0; } }
static int onum(OValue x, double *out) { switch(x.kind) { case O_FLOAT:*out=x.as.f; return 1; case O_INT:*out=(double)x.as.i; return 1; case O_UINT:*out=(double)x.as.u; return 1; case O_BOOL:*out=(double)x.as.b; return 1; default:return 0; } }
static int ocomplexnum(OValue x, OComplex *out) { if(x.kind==O_COMPLEX) { *out=x.as.cx; return 1; } double real; if(!onum(x,&real)) return 0; out->real=real; out->imag=0; return 1; }
static OValue ocomplexcall(const OValue *args, size_t argc) { OComplex value; if(argc==1) { if(!ocomplexnum(args[0],&value)) return ofail("complex expects numeric arguments"); return ocomplexbits(value.real,value.imag,args[0].kind==O_FLOAT||args[0].kind==O_COMPLEX?args[0].fbits:0); } if(argc==2) { double real,imag; if(!onum(args[0],&real)||!onum(args[1],&imag)) return ofail("complex expects numeric arguments"); uint16_t width=args[0].kind==O_FLOAT?args[0].fbits:0; if(args[1].kind==O_FLOAT&&args[1].fbits>width) width=args[1].fbits; return ocomplexbits(real,imag,width); } return ofail("complex expects 1 or 2 arguments"); }
static OValue oreal(OValue x) { OComplex value; return ocomplexnum(x,&value)?ofloat(value.real):ofail("real expects a complex-compatible value"); }
static OValue oimag(OValue x) { OComplex value; return ocomplexnum(x,&value)?ofloat(value.imag):ofail("imag expects a complex-compatible value"); }
static OValue oconjugate(OValue x) { if(x.kind==O_COMPLEX) return ocomplexbits(x.as.cx.real,-x.as.cx.imag,x.fbits); if(x.kind==O_COMPLEX_TENSOR) { size_t n=x.as.ctensor.len; OComplex *data=n?(OComplex *)malloc(n*sizeof(OComplex)):NULL; if(n&&!data) return ofail("out of memory"); for(size_t i=0;i<n;i++) data[i]=(OComplex){x.as.ctensor.data[i].real,-x.as.ctensor.data[i].imag}; return ocomplextensordata(data,n,x.as.ctensor.shape,x.as.ctensor.rank); } double ignored; return onum(x,&ignored)?x:ofail("conjugate expects a complex-compatible value"); }
static OValue omagnitude(OValue x) { OComplex value; return ocomplexnum(x,&value)?ofloat(hypot(value.real,value.imag)):ofail("magnitude expects a complex-compatible value"); }
static OValue odoubledualbin(const char *op, OValue a, OValue b) {
  double ap,at,bp,bt; if(!odualparts(a,&ap,&at)||!odualparts(b,&bp,&bt)) return ofail("dual operator expects numeric values");
  if(!strcmp(op,"+")) return odual(ap+bp,at+bt);
  if(!strcmp(op,"-")) return odual(ap-bp,at-bt);
  if(!strcmp(op,"*")) return odual(ap*bp,at*bp+ap*bt);
  if(!strcmp(op,"/")) { if(bp==0) return ofail("division by zero"); return odual(ap/bp,(at*bp-ap*bt)/(bp*bp)); }
  return ofail("unsupported dual operator");
}
static OValue ofail(const char *message) { ohaderror=1; oerrormessage=message; olasterror=oerrorvalue("unknown_error",message); return onull(); }
static int onativefail(const char *message) { ohaderror=1; oerrormessage=message; olasterror=oerrorvalue("unknown_error",message); fprintf(stderr, "osiris native error: %s\n", message); return 1; }
static int otensorsameshape(OTensor a, OTensor b) { return a.rank==b.rank&&!memcmp(a.shape,b.shape,a.rank*sizeof(size_t)); }
static int otensorbroadcastshape(OTensor a, OTensor b, size_t **out_shape, size_t *out_rank, size_t *out_len) { size_t rank=a.rank>b.rank?a.rank:b.rank, total=1; size_t *shape=rank?(size_t *)malloc(rank*sizeof(size_t)):NULL; if(rank&&!shape) return 0; for(size_t axis=0;axis<rank;axis++) { size_t adim=1,bdim=1; if(axis>=rank-a.rank) adim=a.shape[axis-(rank-a.rank)]; if(axis>=rank-b.rank) bdim=b.shape[axis-(rank-b.rank)]; if(adim!=bdim&&adim!=1&&bdim!=1) { free(shape); return 0; } shape[axis]=adim>bdim?adim:bdim; if(shape[axis]&&total>SIZE_MAX/shape[axis]) { free(shape); return 0; } total*=shape[axis]; } *out_shape=shape; *out_rank=rank; *out_len=total; return 1; }
static OValue otensorbin(const char *op, OValue a, OValue b) { if(a.kind!=O_TENSOR||b.kind!=O_TENSOR) return ofail("tensor operation expects tensors"); size_t *shape,rank,n; if(!otensorbroadcastshape(a.as.tensor,b.as.tensor,&shape,&rank,&n)) return ofail("tensor shape mismatch"); size_t *astrides=a.as.tensor.rank?(size_t *)malloc(a.as.tensor.rank*sizeof(size_t)):NULL, *bstrides=b.as.tensor.rank?(size_t *)malloc(b.as.tensor.rank*sizeof(size_t)):NULL; if((a.as.tensor.rank&&!astrides)||(b.as.tensor.rank&&!bstrides)) { free(shape); free(astrides); free(bstrides); return ofail("out of memory"); } size_t stride=1; for(size_t i=a.as.tensor.rank;i>0;i--) { astrides[i-1]=stride; stride*=a.as.tensor.shape[i-1]; } stride=1; for(size_t i=b.as.tensor.rank;i>0;i--) { bstrides[i-1]=stride; stride*=b.as.tensor.shape[i-1]; } double *data=n?(double *)malloc(n*sizeof(double)):NULL; if(n&&!data) { free(shape); free(astrides); free(bstrides); return ofail("out of memory"); } for(size_t output=0;output<n;output++) { size_t rest=output, ai=0,bi=0; for(size_t axis=rank;axis>0;axis--) { size_t at=axis-1, coord=rest%shape[at]; rest/=shape[at]; if(at>=rank-a.as.tensor.rank) { size_t input=at-(rank-a.as.tensor.rank); if(a.as.tensor.shape[input]!=1) ai+=coord*astrides[input]; } if(at>=rank-b.as.tensor.rank) { size_t input=at-(rank-b.as.tensor.rank); if(b.as.tensor.shape[input]!=1) bi+=coord*bstrides[input]; } } double x=a.as.tensor.data[ai],y=b.as.tensor.data[bi]; if(!strcmp(op,"+")) data[output]=x+y; else if(!strcmp(op,"-")) data[output]=x-y; else if(!strcmp(op,"*")) data[output]=x*y; else if(!strcmp(op,"/")) { if(y==0) { free(shape); free(astrides); free(bstrides); free(data); return ofail("division by zero"); } data[output]=x/y; } else { free(shape); free(astrides); free(bstrides); free(data); return ofail("unsupported tensor operation"); } } free(astrides); free(bstrides); OValue out=otensordata(data,n,shape,rank); free(shape); return out; }
static OValue otensorscalar(const char *op, OValue tensor, OValue scalar, int scalar_left) { if(tensor.kind!=O_TENSOR) return ofail("tensor operation expects a tensor"); double s; if(!onum(scalar,&s)) return ofail("tensor scalar must be numeric"); size_t n=tensor.as.tensor.len; double *data=n?(double *)malloc(n*sizeof(double)):NULL; if(n&&!data) return ofail("out of memory"); if(!strcmp(op,"+")) { for(size_t i=0;i<n;i++) data[i]=tensor.as.tensor.data[i]+s; } else if(!strcmp(op,"-")) { if(scalar_left) { for(size_t i=0;i<n;i++) data[i]=s-tensor.as.tensor.data[i]; } else { for(size_t i=0;i<n;i++) data[i]=tensor.as.tensor.data[i]-s; } } else if(!strcmp(op,"*")) { for(size_t i=0;i<n;i++) data[i]=tensor.as.tensor.data[i]*s; } else if(!strcmp(op,"/")) { if(scalar_left) { for(size_t i=0;i<n;i++) { double x=tensor.as.tensor.data[i]; if(x==0) { free(data); return ofail("division by zero"); } data[i]=s/x; } } else { if(s==0) { free(data); return ofail("division by zero"); } for(size_t i=0;i<n;i++) data[i]=tensor.as.tensor.data[i]/s; } } else { free(data); return ofail("unsupported tensor operation"); } return otensordata(data,n,tensor.as.tensor.shape,tensor.as.tensor.rank); }
static int oeq(OValue a, OValue b) { if(a.kind==O_OPTION&&b.kind==O_OPTION) { if(a.fbits!=b.fbits) return 0; return !a.fbits||((a.as.box.value&&b.as.box.value)&&oeq(*a.as.box.value,*b.as.box.value)); } if(a.kind==O_RESULT&&b.kind==O_RESULT) { if(a.fbits!=b.fbits) return 0; return !a.as.box.value||!b.as.box.value?(!a.as.box.value&&!b.as.box.value):oeq(*a.as.box.value,*b.as.box.value); } if(a.kind==O_ENUM&&b.kind==O_ENUM) { if(a.as.en.type_name.len!=b.as.en.type_name.len||a.as.en.case_name.len!=b.as.en.case_name.len||memcmp(a.as.en.type_name.data,b.as.en.type_name.data,a.as.en.type_name.len)||memcmp(a.as.en.case_name.data,b.as.en.case_name.data,a.as.en.case_name.len)||a.as.en.payload.len!=b.as.en.payload.len) return 0; for(size_t i=0;i<a.as.en.payload.len;i++) if(!oeq(a.as.en.payload.items[i],b.as.en.payload.items[i])) return 0; return 1; } if(a.kind==O_STRING&&b.kind==O_STRING) return a.as.s.len==b.as.s.len&&!memcmp(a.as.s.data,b.as.s.data,a.as.s.len); if(a.kind==O_DECIMAL&&b.kind==O_DECIMAL) return a.as.s.len==b.as.s.len&&!memcmp(a.as.s.data,b.as.s.data,a.as.s.len); if(a.kind==O_BIGINT&&b.kind==O_BIGINT) return a.as.s.len==b.as.s.len&&!memcmp(a.as.s.data,b.as.s.data,a.as.s.len); if(a.kind==O_DUAL&&b.kind==O_DUAL) return a.as.dual.primal==b.as.dual.primal&&a.as.dual.tangent==b.as.dual.tangent; if(a.kind==O_LIST&&b.kind==O_LIST) { if(a.as.list.len!=b.as.list.len) return 0; for(size_t i=0;i<a.as.list.len;i++) if(!oeq(a.as.list.items[i],b.as.list.items[i])) return 0; return 1; } if(a.kind==O_TENSOR&&b.kind==O_TENSOR) { if(!otensorsameshape(a.as.tensor,b.as.tensor)) return 0; for(size_t i=0;i<a.as.tensor.len;i++) if(a.as.tensor.data[i]!=b.as.tensor.data[i]) return 0; return 1; } if(a.kind==O_COMPLEX_TENSOR&&b.kind==O_COMPLEX_TENSOR) { if(a.as.ctensor.rank!=b.as.ctensor.rank||memcmp(a.as.ctensor.shape,b.as.ctensor.shape,a.as.ctensor.rank*sizeof(size_t))) return 0; for(size_t i=0;i<a.as.ctensor.len;i++) if(a.as.ctensor.data[i].real!=b.as.ctensor.data[i].real||a.as.ctensor.data[i].imag!=b.as.ctensor.data[i].imag) return 0; return 1; } if(a.kind==O_CLOSURE&&b.kind==O_CLOSURE) return a.as.closure.call==b.as.closure.call; if(a.kind==O_NULL||b.kind==O_NULL) return a.kind==b.kind; if(a.kind==O_BOOL&&b.kind==O_BOOL) return a.as.b==b.as.b; if(a.kind==O_COMPLEX||b.kind==O_COMPLEX) { OComplex x,y; return ocomplexnum(a,&x)&&ocomplexnum(b,&y)&&x.real==y.real&&x.imag==y.imag; } double x,y; return onum(a,&x)&&onum(b,&y)&&x==y; }
static uint16_t ofloatwidth(OValue a, OValue b) { return a.kind==O_FLOAT&&a.fbits!=0?a.fbits:(b.kind==O_FLOAT?b.fbits:0); }
static uint16_t ointwidth(OValue a, OValue b, int shift) { uint16_t width=a.fbits?a.fbits:64; if(!shift&&b.fbits>width) width=b.fbits; return width; }
static OValue obin(const char *op, OValue a, OValue b) { if(!strcmp(op,"==")) return obool(oeq(a,b)); if(!strcmp(op,"!=")) return obool(!oeq(a,b)); if(!strcmp(op,"+")&&a.kind==O_STRING&&b.kind==O_STRING) return oconcat(a,b); if(!strcmp(op,"*")&&(a.kind==O_QUANTITY||b.kind==O_QUANTITY)) { OValue q=a.kind==O_QUANTITY?a:b,n=a.kind==O_QUANTITY?b:a; double scale; if(!onum(n,&scale)) return ofail("quantity multiplication expects a numeric scale"); q.as.quantity.magnitude*=scale; return q; } if(a.kind==O_COMPLEX||b.kind==O_COMPLEX) { OComplex x,y; uint16_t aw=(a.kind==O_FLOAT||a.kind==O_COMPLEX)?a.fbits:0,bw=(b.kind==O_FLOAT||b.kind==O_COMPLEX)?b.fbits:0,width=aw>bw?aw:bw; if(!ocomplexnum(a,&x)||!ocomplexnum(b,&y)) return ofail("complex operator expects numeric values"); if(!strcmp(op,"+")) return ocomplexbits(x.real+y.real,x.imag+y.imag,width); if(!strcmp(op,"-")) return ocomplexbits(x.real-y.real,x.imag-y.imag,width); if(!strcmp(op,"*")) return ocomplexbits(x.real*y.real-x.imag*y.imag,x.real*y.imag+x.imag*y.real,width); if(!strcmp(op,"/")) { double scale=y.real*y.real+y.imag*y.imag; return scale==0?ofail("division by zero"):ocomplexbits((x.real*y.real+x.imag*y.imag)/scale,(x.imag*y.real-x.real*y.imag)/scale,width); } return ofail("ordering is not defined for complex numbers"); } if((!strcmp(op,"+")||!strcmp(op,"-")||!strcmp(op,"*")||!strcmp(op,"/"))&&a.kind==O_TENSOR&&b.kind==O_TENSOR) return otensorbin(op,a,b); if((!strcmp(op,"+")||!strcmp(op,"-")||!strcmp(op,"*")||!strcmp(op,"/"))&&a.kind==O_TENSOR) return otensorscalar(op,a,b,0); if((!strcmp(op,"+")||!strcmp(op,"-")||!strcmp(op,"*")||!strcmp(op,"/"))&&b.kind==O_TENSOR) return otensorscalar(op,b,a,1); if(a.kind==O_UINT&&b.kind==O_UINT) { uint16_t width=ointwidth(a,b,0); if(!strcmp(op,"+")) return ouintbits(a.as.u+b.as.u,width); if(!strcmp(op,"-")) return ouintbits(a.as.u-b.as.u,width); if(!strcmp(op,"*")) return ouintbits(a.as.u*b.as.u,width); if(!strcmp(op,"/")&&b.as.u!=0&&a.as.u%b.as.u==0) return ouintbits(a.as.u/b.as.u,width); } if(a.kind==O_INT&&b.kind==O_INT) { uint16_t width=ointwidth(a,b,0); uint64_t x=(uint64_t)a.as.i,y=(uint64_t)b.as.i; if(!strcmp(op,"+")) return ointbits((int64_t)(x+y),width); if(!strcmp(op,"-")) return ointbits((int64_t)(x-y),width); if(!strcmp(op,"*")) return ointbits((int64_t)(x*y),width); if(!strcmp(op,"/")&&b.as.i!=0&&a.as.i%b.as.i==0) return ointbits(a.as.i/b.as.i,width); } double x,y; if(!onum(a,&x)||!onum(b,&y)) return ofail("numeric operator expects numeric values"); uint16_t w=ofloatwidth(a,b); if(!strcmp(op,"+")) return (a.kind==O_FLOAT||b.kind==O_FLOAT)?ofloatbits(x+y,w):ofail("mixed integer addition requires bigint lowering"); if(!strcmp(op,"-")) return (a.kind==O_FLOAT||b.kind==O_FLOAT)?ofloatbits(x-y,w):ofail("mixed integer subtraction requires bigint lowering"); if(!strcmp(op,"*")) return (a.kind==O_FLOAT||b.kind==O_FLOAT)?ofloatbits(x*y,w):ofail("mixed integer multiplication requires bigint lowering"); if(!strcmp(op,"/")) return y==0?ofail("division by zero"):ofloatbits(x/y,w); if(!strcmp(op,"<")) return obool(x<y); if(!strcmp(op,"<=")) return obool(x<=y); if(!strcmp(op,">")) return obool(x>y); if(!strcmp(op,">=")) return obool(x>=y); return onull(); }
static OValue obit(const char *op, OValue a, OValue b) { if(a.kind==O_UINT&&b.kind==O_UINT) { uint16_t width=ointwidth(a,b,!strcmp(op,"<<")||!strcmp(op,">>")); uint64_t x=a.as.u,y=b.as.u; if(!strcmp(op,"%")) return y==0?ofail("remainder by zero"):ouintbits(x%y,width); if(!strcmp(op,"&")) return ouintbits(x&y,width); if(!strcmp(op,"|")) return ouintbits(x|y,width); if(!strcmp(op,"^")) return ouintbits(x^y,width); if(!strcmp(op,"<<")) return ouintbits(x<<(y%64),width); if(!strcmp(op,">>")) return ouintbits(x>>(y%64),width); } if(a.kind==O_INT&&b.kind==O_INT) { uint16_t width=ointwidth(a,b,!strcmp(op,"<<")||!strcmp(op,">>")); int64_t x=a.as.i,y=b.as.i; if(!strcmp(op,"%")) return y==0?ofail("remainder by zero"):ointbits(x%y,width); if(!strcmp(op,"&")) return ointbits(x&y,width); if(!strcmp(op,"|")) return ointbits(x|y,width); if(!strcmp(op,"^")) return ointbits(x^y,width); if(!strcmp(op,"<<")) return ointbits(x<<((uint64_t)y%64),width); if(!strcmp(op,">>")) return ointbits(x>>((uint64_t)y%64),width); } return ofail("mixed integer bitwise operations require bigint lowering"); }
static OValue oadd(OValue a, OValue b) { return a.kind==O_DUAL||b.kind==O_DUAL?odoubledualbin("+",a,b):obin("+",a,b); }
static OValue osub(OValue a, OValue b) { return a.kind==O_DUAL||b.kind==O_DUAL?odoubledualbin("-",a,b):obin("-",a,b); }
static OValue omul(OValue a, OValue b) { return a.kind==O_DUAL||b.kind==O_DUAL?odoubledualbin("*",a,b):obin("*",a,b); }
static OValue odiv(OValue a, OValue b) { return a.kind==O_DUAL||b.kind==O_DUAL?odoubledualbin("/",a,b):obin("/",a,b); }
static OValue omod(OValue a, OValue b) { if(a.kind==O_FLOAT||b.kind==O_FLOAT) { double x,y; if(!onum(a,&x)||!onum(b,&y)) return ofail("numeric operator expects numeric values"); if(y==0) return ofail("remainder by zero"); return ofloatbits(fmod(x,y),ofloatwidth(a,b)); } return obit("%",a,b); }
static OValue oband(OValue a, OValue b) { return obit("&",a,b); }
static OValue obor(OValue a, OValue b) { return obit("|",a,b); }
static OValue obxor(OValue a, OValue b) { return obit("^",a,b); }
static OValue oshiftl(OValue a, OValue b) { return obit("<<",a,b); }
static OValue oshiftr(OValue a, OValue b) { return obit(">>",a,b); }
static OValue olt(OValue a, OValue b) { return obin("<",a,b); }
static OValue ole(OValue a, OValue b) { return obin("<=",a,b); }
static OValue ogt(OValue a, OValue b) { return obin(">",a,b); }
static OValue oge(OValue a, OValue b) { return obin(">=",a,b); }
static OValue oequal(OValue a, OValue b) { return obin("==",a,b); }
static OValue onotequal(OValue a, OValue b) { return obin("!=",a,b); }
static OValue ounary(const char *op, OValue x) { double n; if(!strcmp(op,"!")) return obool(!otruthy(x)); if(!strcmp(op,"-")) { if(x.kind==O_COMPLEX) return ocomplexbits(-x.as.cx.real,-x.as.cx.imag,x.fbits); if(x.kind==O_INT) return ointbits((int64_t)(UINT64_C(0)-(uint64_t)x.as.i),x.fbits); if(!onum(x,&n)) return ofail("unary - expects a numeric value"); return x.kind==O_FLOAT?ofloatbits(-x.as.f,x.fbits):oint(-(int64_t)n); } return x; }
static OValue oabs(OValue x) { if(x.kind==O_COMPLEX) return ofloatbits(hypot(x.as.cx.real,x.as.cx.imag),x.fbits); if(x.kind==O_FLOAT) return ofloatbits(x.as.f<0?-x.as.f:x.as.f,x.fbits); if(x.kind==O_INT) return ointbits(x.as.i<0?-x.as.i:x.as.i,x.fbits); return x; }
static OValue ouintcast(OValue x) { double n; return onum(x,&n)?ouintbits((uint64_t)n,64):ofail("expected unsigned integer, got string"); }
static OValue ouintwidthcast(OValue x, uint16_t bits, const char *name) { double n; return onum(x,&n)?ouintbits((uint64_t)n,bits):ofail(name); }
static OValue ointwidthcast(OValue x, uint16_t bits, const char *name) { double n; return onum(x,&n)?ointbits((int64_t)n,bits):ofail(name); }
static OValue obytecast(OValue x) { return ouintwidthcast(x,8,"byte expects a numeric value"); }
static OValue ou8cast(OValue x) { return ouintwidthcast(x,8,"u8 expects a numeric value"); }
static OValue ou16cast(OValue x) { return ouintwidthcast(x,16,"u16 expects a numeric value"); }
static OValue ou32cast(OValue x) { return ouintwidthcast(x,32,"u32 expects a numeric value"); }
static OValue ou64cast(OValue x) { return ouintwidthcast(x,64,"u64 expects a numeric value"); }
static OValue oi8cast(OValue x) { return ointwidthcast(x,8,"i8 expects a numeric value"); }
static OValue oi16cast(OValue x) { return ointwidthcast(x,16,"i16 expects a numeric value"); }
static OValue oi32cast(OValue x) { return ointwidthcast(x,32,"i32 expects a numeric value"); }
static OValue oi64cast(OValue x) { return ointwidthcast(x,64,"i64 expects a numeric value"); }
static OValue of32cast(OValue x) { double n; return onum(x,&n)?ofloatbits((double)(float)n,32):ofail("f32 expects a numeric value"); }
static OValue of64cast(OValue x) { double n; return onum(x,&n)?ofloatbits(n,64):ofail("f64 expects a numeric value"); }
static OValue onumbercast(OValue x) { OValue value=of64cast(x); if(value.kind==O_FLOAT) value.fbits=0; return value; }
static OValue omath1(OValue x, double (*fn)(double), const char *name) { double n; if(!onum(x,&n)) return ofail(name); return ofloatbits(fn(n),x.kind==O_FLOAT?x.fbits:0); }
static OValue omath1checked(OValue x, double (*fn)(double), const char *name) { double n; if(!onum(x,&n)) return ofail(name); double out=fn(n); return (isnan(out)||isinf(out))?ofail(name):ofloatbits(out,x.kind==O_FLOAT?x.fbits:0); }
static OValue osqrt(OValue x) { double n; if(!onum(x,&n)) return ofail("sqrt expects a numeric value"); return n<0?ofail("sqrt domain error"):ofloatbits(sqrt(n),x.kind==O_FLOAT?x.fbits:0); }
static OValue oexp(OValue x) { double n; if(!onum(x,&n)) return ofail("exp expects a numeric value"); double out=exp(n); return (isnan(out)||isinf(out))?ofail("exp overflow"):ofloatbits(out,x.kind==O_FLOAT?x.fbits:0); }
static OValue olog(OValue x) { double n; if(!onum(x,&n)) return ofail("log expects a numeric value"); return n<=0?ofail("log domain error"):ofloatbits(log(n),x.kind==O_FLOAT?x.fbits:0); }
static OValue ologbase(OValue x, OValue base) { double n,b; if(!onum(x,&n)||!onum(base,&b)) return ofail("log expects numeric values"); return n<=0||b<=0||b==1?ofail("log domain error"):ofloatbits(log(n)/log(b),ofloatwidth(x,base)); }
static OValue olog10(OValue x) { double n; if(!onum(x,&n)) return ofail("log10 expects a numeric value"); return n<=0?ofail("log10 domain error"):ofloatbits(log10(n),x.kind==O_FLOAT?x.fbits:0); }
static OValue olog2(OValue x) { double n; if(!onum(x,&n)) return ofail("log2 expects a numeric value"); return n<=0?ofail("log2 domain error"):ofloatbits(log2(n),x.kind==O_FLOAT?x.fbits:0); }
static OValue osin(OValue x) { return omath1(x,ogosin,"sin expects a numeric value"); }
static OValue ocos(OValue x) { return omath1(x,ogocos,"cos expects a numeric value"); }
static OValue otan(OValue x) { return omath1(x,tan,"tan expects a numeric value"); }
static OValue oasin(OValue x) { return omath1checked(x,asin,"asin domain error"); }
static OValue oacos(OValue x) { return omath1checked(x,acos,"acos domain error"); }
static OValue oatan(OValue x) { return omath1(x,atan,"atan expects a numeric value"); }
static OValue osinh(OValue x) { return omath1(x,sinh,"sinh expects a numeric value"); }
static OValue ocosh(OValue x) { return omath1(x,cosh,"cosh expects a numeric value"); }
static OValue otanh(OValue x) { return omath1(x,tanh,"tanh expects a numeric value"); }
static OValue oasinh(OValue x) { return omath1(x,asinh,"asinh expects a numeric value"); }
static OValue oacosh(OValue x) { return omath1checked(x,acosh,"acosh domain error"); }
static OValue oatanh(OValue x) { return omath1checked(x,atanh,"atanh domain error"); }
static OValue oexpm1(OValue x) { return omath1(x,expm1,"expm1 expects a numeric value"); }
static OValue olog1p(OValue x) { return omath1(x,log1p,"log1p expects a numeric value"); }
static OValue ocbrt(OValue x) { return omath1(x,cbrt,"cbrt expects a numeric value"); }
static OValue oerf(OValue x) { return omath1(x,erf,"erf expects a numeric value"); }
static OValue oerfc(OValue x) { return omath1(x,erfc,"erfc expects a numeric value"); }
static OValue ogamma(OValue x) { return omath1(x,tgamma,"gamma expects a numeric value"); }
static OValue olgamma(OValue x) { return omath1(x,lgamma,"lgamma expects a numeric value"); }
static OValue ofloor(OValue x) { return omath1(x,floor,"floor expects a numeric value"); }
static OValue oceil(OValue x) { return omath1(x,ceil,"ceil expects a numeric value"); }
static OValue oround(OValue x) { return omath1(x,round,"round expects a numeric value"); }
static OValue ohypot(const OValue *values, size_t count) { if(count<2) return ofail("hypot expects at least 2 arguments"); double total=0; uint16_t bits=0; for(size_t i=0;i<count;i++) { double n; if(!onum(values[i],&n)) return ofail("hypot expects numeric values"); total+=n*n; if(values[i].kind==O_FLOAT&&values[i].fbits>bits) bits=values[i].fbits; } return ofloatbits(sqrt(total),bits); }
static OValue ovector(const OValue *values, size_t count) { if(count==0) return otensor((OValue){O_LIST,0,{.list={NULL,0}}}); if(count==1) { if(values[0].kind==O_TENSOR) return values[0]; if(values[0].kind==O_LIST) return otensor(values[0]); } return otensor(olist(values,count)); }
static OValue olen(OValue x) { if(x.kind==O_LIST) return ofloat((double)x.as.list.len); if(x.kind==O_TENSOR) return ofloat((double)x.as.tensor.len); if(x.kind==O_COMPLEX_TENSOR) return ofloat((double)x.as.ctensor.len); if(x.kind!=O_STRING) return ofail("len expects a string, list, or tensor"); uint64_t n=0; const unsigned char *p=(const unsigned char *)x.as.s.data, *end=p+x.as.s.len; for(;p<end;p++) if((*p&0xc0)!=0x80) n++; return ofloat((double)n); }
static OValue oat(OValue x, OValue index) { if(x.kind==O_MAP&&index.kind==O_STRING) { char *key=(char *)malloc(index.as.s.len+1); if(!key) return ofail("out of memory"); memcpy(key,index.as.s.data,index.as.s.len); key[index.as.s.len]=0; OValue *found=omapget(x,key); free(key); return found?*found:ofail("map key not found"); } if(x.kind==O_TENSOR&&index.kind==O_LIST) { size_t count=index.as.list.len; if(count==0||count>x.as.tensor.rank) return ofail("tensor coordinate rank does not match tensor rank"); size_t offset=0,stride=1; for(size_t axis=x.as.tensor.rank;axis>count;axis--) stride*=x.as.tensor.shape[axis-1]; size_t block=stride; for(size_t axis=count;axis>0;axis--) { size_t at=axis-1, coordinate; if(!odimension(index.as.list.items[at],&coordinate)||coordinate>=x.as.tensor.shape[at]) return ofail("tensor coordinate out of range"); offset+=coordinate*stride; stride*=x.as.tensor.shape[at]; } if(count==x.as.tensor.rank) return ofloat(x.as.tensor.data[offset]); double *data=block?(double *)malloc(block*sizeof(double)):NULL; if(block&&!data) return ofail("out of memory"); if(block) memcpy(data,x.as.tensor.data+offset,block*sizeof(double)); return otensordata(data,block,x.as.tensor.shape+count,x.as.tensor.rank-count); } double n; if(!onum(index,&n)||n<0||n>=(double)UINT64_MAX||(double)(uint64_t)n!=n) return ofail("index must be a non-negative integer"); uint64_t i=(uint64_t)n; if(x.kind==O_LIST) return i<x.as.list.len?x.as.list.items[i]:ofail("list index out of range"); if(x.kind==O_TENSOR) return i<x.as.tensor.len?ofloat(x.as.tensor.data[i]):ofail("tensor index out of range"); if(x.kind==O_COMPLEX_TENSOR) return i<x.as.ctensor.len?ocomplex(x.as.ctensor.data[i].real,x.as.ctensor.data[i].imag):ofail("tensor index out of range"); if(x.kind==O_STRING) { size_t offset=0; uint64_t rune=0; while(offset<x.as.s.len) { size_t width=1; unsigned char lead=(unsigned char)x.as.s.data[offset]; if((lead&0x80)==0) width=1; else if((lead&0xe0)==0xc0&&offset+2<=x.as.s.len) width=2; else if((lead&0xf0)==0xe0&&offset+3<=x.as.s.len) width=3; else if((lead&0xf8)==0xf0&&offset+4<=x.as.s.len) width=4; if(rune==i) return ostringview(x.as.s.data+offset,width); offset+=width; rune++; } return ofail("at index out of range"); } return ofail("at expects a string, list, or tensor"); }
static OValue osetat(OValue tensor, OValue index, OValue value) { if(tensor.kind!=O_TENSOR) return ofail("native setat expects a tensor"); size_t offset,block=1,coordinate_rank=tensor.as.tensor.rank; if(index.kind==O_LIST) { coordinate_rank=index.as.list.len; if(coordinate_rank==0||coordinate_rank>tensor.as.tensor.rank) return ofail("tensor coordinate rank does not match tensor rank"); size_t stride=1; for(size_t axis=tensor.as.tensor.rank;axis>coordinate_rank;axis--) stride*=tensor.as.tensor.shape[axis-1]; block=stride; offset=0; for(size_t axis=coordinate_rank;axis>0;axis--) { size_t at=axis-1, coordinate; if(!odimension(index.as.list.items[at],&coordinate)||coordinate>=tensor.as.tensor.shape[at]) return ofail("tensor coordinate out of range"); offset+=coordinate*stride; stride*=tensor.as.tensor.shape[at]; } } else { if(!odimension(index,&offset)||offset>=tensor.as.tensor.len) return ofail("setat index out of range"); } size_t n=tensor.as.tensor.len; double *data=n?(double *)malloc(n*sizeof(double)):NULL; if(n&&!data) return ofail("out of memory"); if(n) memcpy(data,tensor.as.tensor.data,n*sizeof(double)); if(index.kind==O_LIST&&coordinate_rank<tensor.as.tensor.rank) { if(value.kind!=O_TENSOR||value.as.tensor.rank!=tensor.as.tensor.rank-coordinate_rank) { free(data); return ofail("tensor replacement shape does not match coordinate slice"); } for(size_t i=0;i<value.as.tensor.rank;i++) if(value.as.tensor.shape[i]!=tensor.as.tensor.shape[coordinate_rank+i]) { free(data); return ofail("tensor replacement shape does not match coordinate slice"); } if(block) memcpy(data+offset,value.as.tensor.data,block*sizeof(double)); } else { double replacement; if(!onum(value,&replacement)) { free(data); return ofail("tensor replacement must be numeric"); } data[offset]=replacement; } return otensordata(data,n,tensor.as.tensor.shape,tensor.as.tensor.rank); }
static OValue otake(OValue tensor, OValue count_arg) { if(tensor.kind!=O_TENSOR) return ofail("native take expects a tensor"); size_t count; if(!odimension(count_arg,&count)) return ofail("take count must be non-negative"); if(count>tensor.as.tensor.len) count=tensor.as.tensor.len; double *data=count?(double *)malloc(count*sizeof(double)):NULL; if(count&&!data) return ofail("out of memory"); if(count) memcpy(data,tensor.as.tensor.data,count*sizeof(double)); size_t shape[1]={count}; return otensordata(data,count,shape,1); }
static OValue oslice(OValue tensor, OValue start_arg, OValue end_arg) { if(tensor.kind!=O_TENSOR||tensor.as.tensor.rank==0) return ofail("slice expects a tensor with a leading dimension"); size_t start,end; if(!odimension(start_arg,&start)) return ofail("slice start must be a non-negative integer"); if(!odimension(end_arg,&end)) return ofail("slice end must be a non-negative integer"); if(start>end) return ofail("slice end must not precede start"); size_t limit=tensor.as.tensor.shape[0]; if(start>limit) start=limit; if(end>limit) end=limit; size_t block=1; for(size_t axis=tensor.as.tensor.rank;axis>1;axis--) block*=tensor.as.tensor.shape[axis-1]; size_t count=(end-start)*block; double *data=count?(double *)malloc(count*sizeof(double)):NULL; if(count&&!data) return ofail("out of memory"); if(count) memcpy(data,tensor.as.tensor.data+start*block,count*sizeof(double)); size_t *shape=(size_t *)malloc(tensor.as.tensor.rank*sizeof(size_t)); if(!shape) { free(data); return ofail("out of memory"); } memcpy(shape,tensor.as.tensor.shape,tensor.as.tensor.rank*sizeof(size_t)); shape[0]=end-start; OValue out=otensordata(data,count,shape,tensor.as.tensor.rank); free(shape); return out; }
static OValue osliceaxis(OValue tensor, OValue start_arg, OValue end_arg, OValue axis_arg) { if(tensor.kind!=O_TENSOR||tensor.as.tensor.rank==0) return ofail("slice expects a tensor with a leading dimension"); size_t start,end,axis; if(!odimension(start_arg,&start)) return ofail("slice start must be a non-negative integer"); if(!odimension(end_arg,&end)) return ofail("slice end must be a non-negative integer"); if(!odimension(axis_arg,&axis)||axis>=tensor.as.tensor.rank) return ofail("slice axis out of range"); if(start>end) return ofail("slice end must not precede start"); size_t limit=tensor.as.tensor.shape[axis]; if(start>limit) start=limit; if(end>limit) end=limit; size_t outer=1,inner=1; for(size_t i=0;i<axis;i++) outer*=tensor.as.tensor.shape[i]; for(size_t i=axis+1;i<tensor.as.tensor.rank;i++) inner*=tensor.as.tensor.shape[i]; size_t span=end-start,count=outer*span*inner; double *data=count?(double *)malloc(count*sizeof(double)):NULL; if(count&&!data) return ofail("out of memory"); for(size_t group=0;group<outer;group++) { size_t source=(group*limit+start)*inner,destination=group*span*inner; if(span&&inner) memcpy(data+destination,tensor.as.tensor.data+source,span*inner*sizeof(double)); } size_t *shape=(size_t *)malloc(tensor.as.tensor.rank*sizeof(size_t)); if(!shape) { free(data); return ofail("out of memory"); } memcpy(shape,tensor.as.tensor.shape,tensor.as.tensor.rank*sizeof(size_t)); shape[axis]=span; OValue out=otensordata(data,count,shape,tensor.as.tensor.rank); free(shape); return out; }
static OValue oshape(OValue x) { size_t n; const size_t *shape; if(x.kind==O_TENSOR) { n=x.as.tensor.rank; shape=x.as.tensor.shape; } else if(x.kind==O_COMPLEX_TENSOR) { n=x.as.ctensor.rank; shape=x.as.ctensor.shape; } else return ofail("shape expects a tensor"); OValue *dims=n?(OValue *)malloc(n*sizeof(OValue)):NULL; if(n&&!dims) return ofail("out of memory"); for(size_t i=0;i<n;i++) dims[i]=ouint((uint64_t)shape[i]); OValue out=olist(dims,n); free(dims); return out; }
static OValue oflatten(OValue x) { size_t n; OValue *items; if(x.kind==O_TENSOR) { n=x.as.tensor.len; items=n?(OValue *)malloc(n*sizeof(OValue)):NULL; if(n&&!items) return ofail("out of memory"); for(size_t i=0;i<n;i++) items[i]=ofloat(x.as.tensor.data[i]); } else if(x.kind==O_COMPLEX_TENSOR) { n=x.as.ctensor.len; items=n?(OValue *)malloc(n*sizeof(OValue)):NULL; if(n&&!items) return ofail("out of memory"); for(size_t i=0;i<n;i++) items[i]=ocomplex(x.as.ctensor.data[i].real,x.as.ctensor.data[i].imag); } else return ofail("flatten expects a tensor"); OValue out=olist(items,n); free(items); return out; }
static OValue otensorsum(OValue x) { if(x.kind==O_TENSOR) { double total=0; for(size_t i=0;i<x.as.tensor.len;i++) total+=x.as.tensor.data[i]; return ofloat(total); } if(x.kind==O_COMPLEX_TENSOR) { OComplex total={0,0}; for(size_t i=0;i<x.as.ctensor.len;i++) { total.real+=x.as.ctensor.data[i].real; total.imag+=x.as.ctensor.data[i].imag; } return ocomplex(total.real,total.imag); } return ofail("sum expects a tensor"); }
static OValue otensorproduct(OValue x) { if(x.kind==O_TENSOR) { double total=1; for(size_t i=0;i<x.as.tensor.len;i++) total*=x.as.tensor.data[i]; return ofloat(total); } if(x.kind==O_COMPLEX_TENSOR) { OComplex total={1,0}; for(size_t i=0;i<x.as.ctensor.len;i++) { OComplex value=x.as.ctensor.data[i]; total=(OComplex){total.real*value.real-total.imag*value.imag,total.real*value.imag+total.imag*value.real}; } return ocomplex(total.real,total.imag); } return ofail("product expects a tensor"); }
static OValue otensordot(OValue a, OValue b) { if((a.kind!=O_TENSOR&&a.kind!=O_COMPLEX_TENSOR)||(b.kind!=O_TENSOR&&b.kind!=O_COMPLEX_TENSOR)) return ofail("dot expects tensors"); size_t an=a.kind==O_TENSOR?a.as.tensor.len:a.as.ctensor.len,bn=b.kind==O_TENSOR?b.as.tensor.len:b.as.ctensor.len; if(an!=bn) return ofail("dot expects tensors with the same length"); if(a.kind==O_TENSOR&&b.kind==O_TENSOR) { double total=0; for(size_t i=0;i<an;i++) total+=a.as.tensor.data[i]*b.as.tensor.data[i]; return ofloat(total); } OComplex total={0,0}; for(size_t i=0;i<an;i++) { OComplex left=a.kind==O_TENSOR?(OComplex){a.as.tensor.data[i],0}:a.as.ctensor.data[i],right=b.kind==O_TENSOR?(OComplex){b.as.tensor.data[i],0}:b.as.ctensor.data[i]; total.real+=left.real*right.real-left.imag*right.imag; total.imag+=left.real*right.imag+left.imag*right.real; } return ocomplex(total.real,total.imag); }
static OValue otensornorm(OValue x) { if(x.kind==O_TENSOR) { double total=0; for(size_t i=0;i<x.as.tensor.len;i++) total+=x.as.tensor.data[i]*x.as.tensor.data[i]; return ofloat(sqrt(total)); } if(x.kind==O_COMPLEX_TENSOR) { double total=0; for(size_t i=0;i<x.as.ctensor.len;i++) total+=x.as.ctensor.data[i].real*x.as.ctensor.data[i].real+x.as.ctensor.data[i].imag*x.as.ctensor.data[i].imag; return ofloat(sqrt(total)); } return ofail("norm expects a tensor"); }
static void oprintfloat(double x) { if(isnan(x)) { fputs("NaN",stdout); return; } if(isinf(x)) { fputs(signbit(x)?"-Inf":"+Inf",stdout); return; } if(trunc(x)==x&&x>=-9223372036854775808.0&&x<=9223372036854775807.0) { printf("%lld",(long long)x); return; } char text[64], *end=NULL; for(int precision=1;precision<=17;precision++) { snprintf(text,sizeof(text),"%.*g",precision,x); double roundtrip=strtod(text,&end); if(end&&*end==0&&roundtrip==x&&!(x==0&&signbit(roundtrip)!=signbit(x))) { fputs(text,stdout); return; } } printf("%.17g",x); }
static void oprintvalue(OValue x) { switch(x.kind) { case O_NULL: fputs("null",stdout); break; case O_BOOL: fputs(x.as.b?"true":"false",stdout); break; case O_UINT: printf("%lluu%u",(unsigned long long)x.as.u,(unsigned)(x.fbits?x.fbits:64)); break; case O_INT: printf("%lldi%u",(long long)x.as.i,(unsigned)(x.fbits?x.fbits:64)); break; case O_FLOAT: oprintfloat(x.as.f); if(x.fbits) printf("f%u",(unsigned)x.fbits); break; case O_COMPLEX: if(x.as.cx.imag==0) { oprintfloat(x.as.cx.real); if(x.fbits) printf("f%u",(unsigned)x.fbits); fputs("+0",stdout); if(x.fbits) printf("f%u",(unsigned)x.fbits); fputs("i",stdout); } else if(x.as.cx.real==0) { oprintfloat(x.as.cx.imag); if(x.fbits) printf("f%u",(unsigned)x.fbits); fputs("i",stdout); } else { oprintfloat(x.as.cx.real); if(x.fbits) printf("f%u",(unsigned)x.fbits); if(signbit(x.as.cx.imag)) { putchar('-'); oprintfloat(-x.as.cx.imag); } else { putchar('+'); oprintfloat(x.as.cx.imag); } if(x.fbits) printf("f%u",(unsigned)x.fbits); fputs("i",stdout); } break; case O_DUAL: oprintfloat(x.as.dual.primal); fputs("+",stdout); oprintfloat(x.as.dual.tangent); fputs("ε",stdout); break; case O_STRING: printf("\"%.*s\"",(int)x.as.s.len,x.as.s.data); break; case O_LIST: putchar('['); for(size_t i=0;i<x.as.list.len;i++) { if(i) fputs(", ",stdout); oprintvalue(x.as.list.items[i]); } putchar(']'); break; case O_TENSOR: fputs("tensor[",stdout); for(size_t i=0;i<x.as.tensor.rank;i++) { if(i) putchar('x'); printf("%zu",x.as.tensor.shape[i]); } fputs("]([",stdout); for(size_t i=0;i<x.as.tensor.len;i++) { if(i) fputs(", ",stdout); oprintfloat(x.as.tensor.data[i]); if(x.fbits) printf("f%u",(unsigned)x.fbits); } fputs("])",stdout); break; case O_COMPLEX_TENSOR: fputs("tensor[",stdout); for(size_t i=0;i<x.as.ctensor.rank;i++) { if(i) putchar('x'); printf("%zu",x.as.ctensor.shape[i]); } fputs("]([",stdout); for(size_t i=0;i<x.as.ctensor.len;i++) { if(i) fputs(", ",stdout); oprintvalue(ocomplexbits(x.as.ctensor.data[i].real,x.as.ctensor.data[i].imag,x.fbits)); } fputs("])",stdout); break; case O_CLOSURE: fputs("<function>",stdout); break; case O_ENUM: printf("%.*s.%.*s",(int)x.as.en.type_name.len,x.as.en.type_name.data,(int)x.as.en.case_name.len,x.as.en.case_name.data); if(x.as.en.payload.len) { putchar('('); for(size_t i=0;i<x.as.en.payload.len;i++) { if(i) fputs(", ",stdout); oprintvalue(x.as.en.payload.items[i]); } putchar(')'); } break; case O_DECIMAL: printf("%.*sd",(int)x.as.s.len,x.as.s.data); break; case O_BIGINT: printf("%.*sn",(int)x.as.s.len,x.as.s.data); break; case O_QUANTITY: oprintfloat(x.as.quantity.magnitude); printf("*%.*s",(int)x.as.quantity.unit.len,x.as.quantity.unit.data); break; case O_DATETIME: oprintdatetime(x); break; case O_DURATION: printf("duration(%lld)",(long long)x.as.i); break; case O_ERROR: printf("error(%.*s, %.*s)",(int)x.as.error.kind.len,x.as.error.kind.data,(int)x.as.error.message.len,x.as.error.message.data); break; case O_RESULT: fputs(x.fbits?"ok":"err",stdout); break; case O_OPTION: if(!x.fbits) fputs("none",stdout); else { fputs("some(",stdout); if(x.as.box.value) oprintvalue(*x.as.box.value); fputs(")",stdout); } break; case O_TASK: fputs("<task>",stdout); break; case O_BYTES: fputs("bytes",stdout); break; case O_MAP: putchar('{'); for(size_t i=0;i<x.as.map->len;i++) { if(i) fputs(", ",stdout); printf("%.*s: ",(int)x.as.map->entries[i].key.len,x.as.map->entries[i].key.data); oprintvalue(x.as.map->entries[i].value); } putchar('}'); break; case O_CELL: fputs("<cell>",stdout); break; case O_FFI: fputs("<ffi>",stdout); break; case O_RESOURCE: fputs("<host-resource>",stdout); break; } }
static void oprint(OValue x) {
#ifdef OSIRIS_PUBLIC_EMIT_SPACING
if(oprinted) putchar('\n');
#endif
oprintvalue(x); putchar('\n'); oprinted=1;
}
`, "static OValue oat(", "static OValue oat_real(", 1), "static OValue oreshape(", "static OValue oreshape_real(", 1), "static OValue osetat(", "static OValue osetat_real(", 1), "static OValue obin(const char *op, OValue a, OValue b) {", "static OValue obin_real(const char *op, OValue a, OValue b) {", 1), "static OValue otake(", "static OValue otake_real(", 1), "static OValue oslice(", "static OValue oslice_real(", 1), "static OValue osliceaxis(", "static OValue osliceaxis_real(", 1), "static OValue otranspose(", "static OValue otranspose_real(", 1), "static OValue omatmul(", "static OValue omatmul_real(", 1), "static OValue otrace(", "static OValue otrace_real(", 1), "static OValue odet(", "static OValue odet_real(", 1), "static OValue osolve(", "static OValue osolve_real(", 1), "static OValue oinv(", "static OValue oinv_real(", 1), "static OValue ocholesky(", "static OValue ocholesky_real(", 1), "static OValue oqr(", "static OValue oqr_real(", 1), "provider&&provider->abi_version==1&&provider->eigh_f64", "provider&&provider->abi_version>=1&&provider->eigh_f64", 1), "provider&&provider->abi_version==1&&provider->svd_f64", "provider&&provider->abi_version>=1&&provider->svd_f64", 1) + cRuntimeComplexTensorComponent + cRuntimePortableComponent + strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(cRuntimeStdlibComponent, "ostd_text_join", "o147_text_join"), "ostd_text_fields", "o147_text_fields"), "ostd_list_range", "o147_list_range") + cRuntimeFFIComponent
}

const cRuntimeFFIComponent = `
static OValue offi_load(OValue path) {
#if defined(__unix__) || defined(__APPLE__)
  if(path.kind!=O_STRING || memchr(path.as.s.data, 0, path.as.s.len)) return ofail("ffi_load expects a NUL-free manifest path");
  char *copy=(char *)malloc(path.as.s.len+1); if(!copy) return ofail("out of memory");
  memcpy(copy,path.as.s.data,path.as.s.len); copy[path.as.s.len]=0;
  void *handle=dlopen(copy, RTLD_NOW|RTLD_LOCAL); free(copy);
  if(!handle) { const char *detail=dlerror(); return ofail(detail?detail:"unable to open foreign library"); }
  return (OValue){O_FFI,0,{.ffi={handle}}};
#else
  (void)path; return ofail("native C FFI is unavailable on this platform");
#endif
}
static void *offi_symbol(OValue library, const char *name) {
#if defined(__unix__) || defined(__APPLE__)
  if(library.kind!=O_FFI || !library.as.ffi.handle) { ofail("ffi call expects an open library"); return NULL; }
  dlerror(); void *symbol=dlsym(library.as.ffi.handle,name); const char *detail=dlerror();
  if(detail || !symbol) { ofail(detail?detail:"foreign symbol is unavailable"); return NULL; }
  return symbol;
#else
  (void)library; (void)name; ofail("native C FFI is unavailable on this platform"); return NULL;
#endif
}
static OValue offi_close(OValue *library) {
#if defined(__unix__) || defined(__APPLE__)
  if(!library || library->kind!=O_FFI || !library->as.ffi.handle) return ofail("ffi_close expects an open library");
  if(dlclose(library->as.ffi.handle)!=0) { const char *detail=dlerror(); return ofail(detail?detail:"unable to close foreign library"); }
  library->as.ffi.handle=NULL;
  return obool(1);
#else
  (void)library; return ofail("native C FFI is unavailable on this platform");
#endif
}
static OValue offi_i64(OValue library, const char *name, int result_kind, int result_bits, size_t count, const OValue *values) {
  void *symbol=offi_symbol(library,name); if(!symbol) return onull(); if(count>4) return ofail("ffi integer ABI arity exceeds four");
  int64_t a[4]={0,0,0,0}; for(size_t i=0;i<count;i++) { double number; if(!onum(values[i],&number)) return ofail("ffi integer call expects integer arguments"); a[i]=(int64_t)number; }
  int64_t result=0; switch(count) {
  case 0: result=((int64_t (*)(void))symbol)(); break;
  case 1: result=((int64_t (*)(int64_t))symbol)(a[0]); break;
  case 2: result=((int64_t (*)(int64_t,int64_t))symbol)(a[0],a[1]); break;
  case 3: result=((int64_t (*)(int64_t,int64_t,int64_t))symbol)(a[0],a[1],a[2]); break;
  case 4: result=((int64_t (*)(int64_t,int64_t,int64_t,int64_t))symbol)(a[0],a[1],a[2],a[3]); break;
  }
  if(result_kind==0) return onull(); if(result_kind==1) return obool(result!=0); if(result_kind==2) return ouintbits((uint64_t)result,(uint16_t)result_bits); return ointbits(result,(uint16_t)result_bits);
}
static OValue offi_void(OValue library, const char *name, size_t count, const OValue *values) {
  void *symbol=offi_symbol(library,name); if(!symbol) return onull(); if(count>4) return ofail("ffi void ABI arity exceeds four");
  int64_t a[4]={0,0,0,0}; for(size_t i=0;i<count;i++) { double number; if(!onum(values[i],&number)) return ofail("ffi void call expects integer arguments"); a[i]=(int64_t)number; }
  switch(count) { case 0: ((void (*)(void))symbol)(); break; case 1: ((void (*)(int64_t))symbol)(a[0]); break; case 2: ((void (*)(int64_t,int64_t))symbol)(a[0],a[1]); break; case 3: ((void (*)(int64_t,int64_t,int64_t))symbol)(a[0],a[1],a[2]); break; case 4: ((void (*)(int64_t,int64_t,int64_t,int64_t))symbol)(a[0],a[1],a[2],a[3]); break; }
  return onull();
}
static OValue offi_f64(OValue library, const char *name, const OValue *values, size_t count, int bits) {
  void *symbol=offi_symbol(library,name); if(!symbol) return onull(); if(count>4) return ofail("ffi float ABI arity exceeds four");
  double a[4]={0,0,0,0}; for(size_t i=0;i<count;i++) { if(!onum(values[i],&a[i])) return ofail("ffi float call expects numeric arguments"); }
  double result=0; switch(count) { case 0: result=((double (*)(void))symbol)(); break; case 1: result=((double (*)(double))symbol)(a[0]); break; case 2: result=((double (*)(double,double))symbol)(a[0],a[1]); break; case 3: result=((double (*)(double,double,double))symbol)(a[0],a[1],a[2]); break; case 4: result=((double (*)(double,double,double,double))symbol)(a[0],a[1],a[2],a[3]); break; }
  return ofloatbits(result,(uint16_t)bits);
}
static OValue offi_string(OValue library, const char *name, const OValue *values, size_t count) {
  void *symbol=offi_symbol(library,name); if(!symbol) return onull(); if(count>4) return ofail("ffi string ABI arity exceeds four");
  char *a[4]={NULL,NULL,NULL,NULL}; for(size_t i=0;i<count;i++) { if(values[i].kind!=O_STRING || memchr(values[i].as.s.data,0,values[i].as.s.len)) return ofail("ffi string call expects NUL-free strings"); a[i]=(char *)malloc(values[i].as.s.len+1); if(!a[i]) { for(size_t j=0;j<i;j++) free(a[j]); return ofail("out of memory"); } memcpy(a[i],values[i].as.s.data,values[i].as.s.len); a[i][values[i].as.s.len]=0; }
  const char *result=NULL; switch(count) { case 0: result=((const char *(*)(void))symbol)(); break; case 1: result=((const char *(*)(const char *))symbol)(a[0]); break; case 2: result=((const char *(*)(const char *,const char *))symbol)(a[0],a[1]); break; case 3: result=((const char *(*)(const char *,const char *,const char *))symbol)(a[0],a[1],a[2]); break; case 4: result=((const char *(*)(const char *,const char *,const char *,const char *))symbol)(a[0],a[1],a[2],a[3]); break; }
  for(size_t i=0;i<count;i++) free(a[i]); if(!result) return ofail("foreign string call returned null"); size_t len=strlen(result); char *copy=(char *)malloc(len+1); if(!copy) return ofail("out of memory"); memcpy(copy,result,len+1); if(!otrack(copy)) { free(copy); return ofail("out of memory"); } return ostringview(copy,len);
}
static int offi_bytes(OValue value, unsigned char **out, size_t *length) { if(value.kind!=O_LIST) return 0; *length=value.as.list.len; *out=*length?(unsigned char *)malloc(*length):NULL; if(*length&&!*out) return 0; for(size_t i=0;i<*length;i++) { OValue item=value.as.list.items[i]; if(item.kind!=O_UINT || item.as.u>255) { free(*out); *out=NULL; return 0; } (*out)[i]=(unsigned char)item.as.u; } return 1; }
static OValue offi_const_buffer(OValue library, const char *name, int result_kind, int result_bits, const OValue *values, size_t count) { void *symbol=offi_symbol(library,name); if(!symbol) return onull(); if(count<1||count>2) return ofail("ffi const_buffer ABI supports one or two buffers"); unsigned char *data[2]={NULL,NULL}; size_t length[2]={0,0}; for(size_t i=0;i<count;i++) if(!offi_bytes(values[i],&data[i],&length[i])) { free(data[0]); free(data[1]); return ofail("ffi const_buffer expects byte lists"); } int64_t result=0; if(count==1) result=((int64_t (*)(const unsigned char *,uintptr_t))symbol)(data[0],length[0]); else result=((int64_t (*)(const unsigned char *,uintptr_t,const unsigned char *,uintptr_t))symbol)(data[0],length[0],data[1],length[1]); free(data[0]); free(data[1]); if(result_kind==0) return onull(); if(result_kind==1) return obool(result!=0); if(result_kind==2) return ouintbits((uint64_t)result,(uint16_t)result_bits); return ointbits(result,(uint16_t)result_bits); }
static OValue offi_mutable_buffer(OValue library, const char *name, int result_kind, int result_bits, OValue value) { void *symbol=offi_symbol(library,name); if(!symbol) return onull(); unsigned char *data=NULL; size_t length=0; if(!offi_bytes(value,&data,&length)) return ofail("ffi buffer expects a byte list"); int64_t result=((int64_t (*)(unsigned char *,uintptr_t))symbol)(data,length); for(size_t i=0;i<length;i++) value.as.list.items[i]=ouintbits(data[i],8); free(data); if(result_kind==0) return onull(); if(result_kind==1) return obool(result!=0); if(result_kind==2) return ouintbits((uint64_t)result,(uint16_t)result_bits); return ointbits(result,(uint16_t)result_bits); }
`
