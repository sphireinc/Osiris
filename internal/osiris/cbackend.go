package osiris

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type CArtifact struct {
	Source string
}

// CCompileOptions selects optional native link-time facilities.
type CCompileOptions struct {
	LinalgProviderSource string
	// PublicEmitSpacing matches the CLI's blank-line separation between
	// top-level emitted values. Library callers retain the compact historical
	// stream unless they request the public presentation contract.
	PublicEmitSpacing bool
	// LinalgProviderFlags are explicit linker arguments for an external system
	// numerical library, such as "-framework", "Accelerate" on macOS or
	// "-llapack", "-lblas" on systems that provide LAPACK separately.
	LinalgProviderFlags []string
	// NativeCapabilities is the explicit host capability set available to the
	// generated executable. It is copied into the native build contract and is
	// also used when validating capability-gated FFI manifests. An omitted
	// capability is never inferred from the build host.
	NativeCapabilities []string
}

// CCompileManifest describes an addressable native executable artifact. Cache
// keys include every input that can affect machine code generation.
type CCompileManifest struct {
	FormatVersion        int      `json:"format_version"`
	Key                  string   `json:"key"`
	SourceSHA256         string   `json:"source_sha256"`
	Compiler             string   `json:"compiler"`
	CompilerIdentity     string   `json:"compiler_identity"`
	Flags                []string `json:"flags"`
	LinalgProviderSHA256 string   `json:"linalg_provider_sha256,omitempty"`
	Target               string   `json:"target"`
	CacheHit             bool     `json:"-"`
}

const cCompileManifestVersion = 1

func normalizedNativeCapabilities(capabilities []string) []string {
	seen := make(map[string]struct{}, len(capabilities))
	result := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		capability = strings.TrimSpace(capability)
		if capability == "" {
			continue
		}
		if _, ok := seen[capability]; ok {
			continue
		}
		seen[capability] = struct{}{}
		result = append(result, capability)
	}
	sort.Strings(result)
	return result
}

func nativeCapabilityMacro(capability string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(capability) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// CompileC compiles the c into the requested backend artifact so the caller
// can execute or inspect the program without keeping the source interpreter in
// the hot path.
//
// It returns *CArtifact and an error. Callers should only trust the value when
// the error is nil.
func CompileC(ctx context.Context, rt *Runtime, prog *Program) (*CArtifact, error) {
	return CompileCWithOptions(ctx, rt, prog, CCompileOptions{})
}

// CompileCWithOptions lowers a program with explicit native backend options.
func CompileCWithOptions(ctx context.Context, rt *Runtime, prog *Program, options CCompileOptions) (*CArtifact, error) {
	if err := rejectReservedTypeFutureBackend(prog, "native C"); err != nil {
		return nil, err
	}
	if rt == nil {
		rt = NewRuntime()
	}
	ir, err := rt.BuildIR(ctx, prog)
	if err != nil {
		return nil, err
	}
	if name, span, ok := nativeUnsupportedFilesystemBuiltin(ir); ok {
		return nil, fmt.Errorf(
			"%s:%d:%d: native C backend: filesystem builtin %q is not supported",
			prog.SourceName,
			span.start.line,
			span.start.col,
			name,
		)
	}
	if name, span, ok := nativeUnsupportedNetworkingBuiltin(ir); ok {
		return nil, fmt.Errorf(
			"%s:%d:%d: native C backend: networking builtin %q is not supported by the hosted-only native ABI",
			prog.SourceName,
			span.start.line,
			span.start.col,
			name,
		)
	}
	ffiPlan, err := buildNativeFFIPlanWithCapabilities(rt, ir, options.NativeCapabilities)
	if err != nil {
		return nil, err
	}
	if !nativeProgramHasExceptionalAsync(ir) {
		optimizeNativeLoops(ir)
	}
	source, err := emitCIRWithOptionsAndFFI(ir, options, ffiPlan)
	if err != nil {
		return nil, err
	}
	return &CArtifact{Source: source}, nil
}

func nativeProgramContainsDefer(prog *Program) (sourceSpan, bool) {
	if prog == nil {
		return sourceSpan{}, false
	}
	for _, stmt := range prog.Statements {
		if span, ok := nativeStmtContainsDefer(stmt); ok {
			return span, true
		}
	}
	return sourceSpan{}, false
}

func nativeStmtContainsDefer(stmt Stmt) (sourceSpan, bool) {
	switch st := stmt.(type) {
	case deferStmt:
		return st.Span, true
	case letStmt:
		return nativeExprContainsDefer(st.Expr)
	case assignStmt:
		return nativeExprContainsDefer(st.Expr)
	case emitStmt:
		return nativeExprContainsDefer(st.Expr)
	case returnStmt:
		if st.Expr == nil {
			return sourceSpan{}, false
		}
		return nativeExprContainsDefer(st.Expr)
	case whileStmt:
		if span, ok := nativeExprContainsDefer(st.Cond); ok {
			return span, true
		}
		return nativeExprContainsDefer(st.Body)
	case forStmt:
		if span, ok := nativeExprContainsDefer(st.Iter); ok {
			return span, true
		}
		return nativeExprContainsDefer(st.Body)
	case funcStmt:
		return nativeExprContainsDefer(st.Body)
	case implStmt:
		for _, method := range st.Methods {
			if span, ok := nativeExprContainsDefer(method.Body); ok {
				return span, true
			}
		}
	}
	return sourceSpan{}, false
}

func nativeExprContainsDefer(expr Expr) (sourceSpan, bool) {
	switch n := expr.(type) {
	case *blockExpr:
		for _, stmt := range n.stmts {
			if span, ok := nativeStmtContainsDefer(stmt); ok {
				return span, true
			}
		}
		return nativeExprContainsDefer(n.tail)
	case *callExpr:
		if span, ok := nativeExprContainsDefer(n.callee); ok {
			return span, true
		}
		for _, arg := range n.args {
			if span, ok := nativeExprContainsDefer(arg); ok {
				return span, true
			}
		}
	case *memberExpr:
		return nativeExprContainsDefer(n.object)
	case *indexExpr:
		if span, ok := nativeExprContainsDefer(n.Object); ok {
			return span, true
		}
		return nativeExprContainsDefer(n.Index)
	case *unaryExpr:
		return nativeExprContainsDefer(n.expr)
	case *binaryExpr:
		if span, ok := nativeExprContainsDefer(n.left); ok {
			return span, true
		}
		return nativeExprContainsDefer(n.right)
	case *listExpr:
		for _, item := range n.items {
			if span, ok := nativeExprContainsDefer(item); ok {
				return span, true
			}
		}
	case *tupleExpr:
		for _, item := range n.items {
			if span, ok := nativeExprContainsDefer(item); ok {
				return span, true
			}
		}
	case *structExpr:
		for _, field := range n.Fields {
			if span, ok := nativeExprContainsDefer(field.Expr); ok {
				return span, true
			}
		}
	case *structUpdateExpr:
		if span, ok := nativeExprContainsDefer(n.Base); ok {
			return span, true
		}
		for _, field := range n.Fields {
			if span, ok := nativeExprContainsDefer(field.Expr); ok {
				return span, true
			}
		}
	case *matchExpr:
		if span, ok := nativeExprContainsDefer(n.subject); ok {
			return span, true
		}
		for _, arm := range n.arms {
			if span, ok := nativeExprContainsDefer(arm.guard); ok {
				return span, true
			}
			if span, ok := nativeExprContainsDefer(arm.body); ok {
				return span, true
			}
		}
	case *tryExpr:
		if span, ok := nativeExprContainsDefer(n.body); ok {
			return span, true
		}
		return nativeExprContainsDefer(n.handler)
	case *propagateExpr:
		return nativeExprContainsDefer(n.expr)
	}
	return sourceSpan{}, false
}

func nativeProgramHasExceptionalAsync(program *IRProgram) bool {
	for _, function := range program.Functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				switch instruction.Op {
				case "async", "try", "resultbind":
					return true
				}
			}
		}
	}
	return false
}

func nativeUnsupportedFilesystemBuiltin(program *IRProgram) (string, sourceSpan, bool) {
	if program == nil {
		return "", sourceSpan{}, false
	}
	unsupported := make(map[string]struct{})
	for _, name := range filesystemBuiltinNames() {
		unsupported[name] = struct{}{}
	}
	for _, function := range program.Functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				if instruction.Op != "builtin" {
					continue
				}
				if _, ok := unsupported[instruction.Text]; ok {
					return instruction.Text, instruction.Span, true
				}
			}
		}
	}
	return "", sourceSpan{}, false
}

func nativeUnsupportedNetworkingBuiltin(program *IRProgram) (string, sourceSpan, bool) {
	if program == nil {
		return "", sourceSpan{}, false
	}
	unsupported := map[string]struct{}{}
	for _, function := range program.Functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				if instruction.Op != "builtin" {
					continue
				}
				if _, ok := unsupported[instruction.Text]; ok {
					return instruction.Text, instruction.Span, true
				}
			}
		}
	}
	return "", sourceSpan{}, false
}

func nativeUnsupportedFFIBuiltin(program *IRProgram) (string, sourceSpan, bool) {
	if program == nil {
		return "", sourceSpan{}, false
	}
	for _, function := range program.Functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				if instruction.Op == "builtin" && (instruction.Text == "ffi_load" || instruction.Text == "ffi_call" || instruction.Text == "ffi_close") {
					return instruction.Text, instruction.Span, true
				}
			}
		}
	}
	return "", sourceSpan{}, false
}

type nativeFFIPlan struct {
	calls        map[string]FFIFunction
	constants    map[string]FFIConstant
	loads        map[string]string
	capabilities map[string]bool
}

func (p *nativeFFIPlan) callKey(functionName, libraryValue, memberValue string) string {
	return functionName + "\x00" + libraryValue + "\x00" + memberValue
}

func (p *nativeFFIPlan) loadKey(functionName, libraryValue string) string {
	return functionName + "\x00" + libraryValue
}

func buildNativeFFIPlan(rt *Runtime, program *IRProgram) (*nativeFFIPlan, error) {
	return buildNativeFFIPlanWithCapabilities(rt, program, nil)
}

func buildNativeFFIPlanWithCapabilities(rt *Runtime, program *IRProgram, nativeCapabilities []string) (*nativeFFIPlan, error) {
	plan := &nativeFFIPlan{calls: make(map[string]FFIFunction), constants: make(map[string]FFIConstant), loads: make(map[string]string), capabilities: make(map[string]bool)}
	if program == nil {
		return plan, nil
	}
	granted := map[string]bool{}
	if rt != nil {
		granted = rt.Capabilities()
	}
	for _, capability := range nativeCapabilities {
		if capability != "" {
			granted[capability] = true
			plan.capabilities[capability] = true
		}
	}
	for capability, granted := range granted {
		if granted {
			plan.capabilities[capability] = true
		}
	}
	for _, fn := range program.Functions {
		if fn == nil {
			continue
		}
		stringsByValue := make(map[string]string)
		librariesByValue := make(map[string]FFIManifest)
		builtinsByValue := make(map[string]string)
		for _, block := range fn.Blocks {
			for _, instruction := range block.Instrs {
				switch instruction.Op {
				case "builtin":
					builtinsByValue[instruction.Dest] = instruction.Text
				case "const":
					if instruction.Value.kind == kindString {
						stringsByValue[instruction.Dest] = instruction.Value.str
					}
				case "copy", "assign":
					if len(instruction.Args) == 1 {
						if value, ok := stringsByValue[instruction.Args[0]]; ok {
							stringsByValue[instruction.Dest] = value
						}
						if library, ok := librariesByValue[instruction.Args[0]]; ok {
							librariesByValue[instruction.Dest] = library
						}
					}
				case "call":
					builtin := builtinsByValue[instruction.Text]
					switch builtin {
					case "ffi_load":
						if len(instruction.Args) != 1 {
							return nil, nativeCInstrError(program.SourceName, instruction, "ffi_load expects one manifest path")
						}
						path, ok := stringsByValue[instruction.Args[0]]
						if !ok || path == "" {
							return nil, nativeCInstrError(program.SourceName, instruction, "native FFI requires a literal manifest path")
						}
						manifest, err := LoadFFIManifestWithGrants(path, granted)
						if err != nil {
							return nil, nativeCInstrError(program.SourceName, instruction, "native FFI manifest %q: %v", path, err)
						}
						native, err := openFFINativeLibrary(manifest.Library)
						if err != nil {
							return nil, nativeCInstrError(program.SourceName, instruction, "native FFI library %q: %v", manifest.Library, err)
						}
						for _, declaration := range manifest.Functions {
							if err := native.lookup(declaration.Symbol); err != nil {
								native.close()
								return nil, nativeCInstrError(program.SourceName, instruction, "native FFI symbol %q: %v", declaration.Symbol, err)
							}
						}
						for _, handle := range manifest.Handles {
							if err := native.lookup(handle.ReleaseSymbol); err != nil {
								native.close()
								return nil, nativeCInstrError(program.SourceName, instruction, "native FFI handle release symbol %q: %v", handle.ReleaseSymbol, err)
							}
						}
						native.close()
						librariesByValue[instruction.Dest] = manifest
						plan.loads[plan.loadKey(fn.Name, instruction.Dest)] = manifest.Library
					case "ffi_call", "ffi_const":
						if len(instruction.Args) < 2 {
							return nil, nativeCInstrError(program.SourceName, instruction, "%s expects a library and declared member name", builtin)
						}
						manifest, ok := librariesByValue[instruction.Args[0]]
						if !ok {
							return nil, nativeCInstrError(program.SourceName, instruction, "%s requires a manifest-loaded library", builtin)
						}
						member, ok := stringsByValue[instruction.Args[1]]
						if !ok || member == "" {
							return nil, nativeCInstrError(program.SourceName, instruction, "native %s requires a literal declared member name", builtin)
						}
						if builtin == "ffi_const" {
							for _, constant := range manifest.Constants {
								if constant.Name == member {
									plan.constants[plan.callKey(fn.Name, instruction.Args[0], instruction.Args[1])] = constant
									goto constantFound
								}
							}
							return nil, nativeCInstrError(program.SourceName, instruction, "native FFI constant %q is not declared", member)
						}
						for _, declaration := range manifest.Functions {
							if declaration.Name == member {
								plan.calls[plan.callKey(fn.Name, instruction.Args[0], instruction.Args[1])] = declaration
								goto functionFound
							}
						}
						return nil, nativeCInstrError(program.SourceName, instruction, "native FFI function %q is not declared", member)
					}
				constantFound:
				functionFound:
				}
			}
		}
	}
	return plan, nil
}

func nativeCapabilityGranted(plan *nativeFFIPlan, capability string) bool {
	if capability == "" {
		return true
	}
	return plan != nil && plan.capabilities[capability]
}

// cEscapeString handles the c escape string step in the surrounding package
// and returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func cEscapeString(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteByte(c)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c >= 0x20 && c <= 0x7e {
				b.WriteByte(c)
				continue
			}
			// A fixed three-digit octal escape cannot consume following
			// hexadecimal source characters the way C's \x escape can.
			b.WriteString(fmt.Sprintf(`\%03o`, c))
		}
	}
	return b.String()
}

// CompileCObject compiles the cobject into the requested backend artifact so
// the caller can execute or inspect the program without keeping the source
// interpreter in the hot path.
//
// It returns an error when the operation cannot complete cleanly.
func CompileCObject(ctx context.Context, source string, outputPath string) error {
	return compileCArtifact(ctx, source, outputPath, true, CCompileOptions{})
}

// CompileCExecutable compiles the cexecutable into the requested backend
// artifact so the caller can execute or inspect the program without keeping
// the source interpreter in the hot path.
//
// It returns an error when the operation cannot complete cleanly.
func CompileCExecutable(ctx context.Context, source string, outputPath string) error {
	return CompileCExecutableWithOptions(ctx, source, outputPath, CCompileOptions{})
}

// CompileCExecutableWithOptions compiles generated C and optional provider C.
func CompileCExecutableWithOptions(ctx context.Context, source, outputPath string, options CCompileOptions) error {
	return compileCArtifact(ctx, source, outputPath, false, options)
}

// CompileCExecutableCached compiles source into outputPath, reusing a
// content-addressed executable in cacheDir when the generated C, compiler, and
// compilation flags are identical. cacheDir is explicit so builds stay
// reproducible and callers control cache lifetime.
func CompileCExecutableCached(ctx context.Context, source, outputPath, cacheDir string) (*CCompileManifest, error) {
	return CompileCExecutableCachedWithOptions(ctx, source, outputPath, cacheDir, CCompileOptions{})
}

func CompileCExecutableCachedWithOptions(ctx context.Context, source, outputPath, cacheDir string, options CCompileOptions) (*CCompileManifest, error) {
	compiler := os.Getenv("CC")
	if compiler == "" {
		compiler = "cc"
	}
	compilerPath, err := exec.LookPath(compiler)
	if err != nil {
		return nil, fmt.Errorf("%s not found: %w", compiler, err)
	}
	identityOut, err := exec.CommandContext(ctx, compilerPath, "--version").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("identify C compiler %s: %w", compilerPath, err)
	}
	sanitizerFlags := nativeSanitizerFlags()
	flags := []string{"-std=c11", "-O3", "-DNDEBUG", "-ffp-contract=fast"}
	if len(sanitizerFlags) > 0 {
		flags = append(flags, "-O1", "-g", "-fno-omit-frame-pointer")
		flags = append(flags, sanitizerFlags...)
	} else {
		flags = append(flags, "-flto")
	}
	flags = append(flags, "-lm")
	if runtime.GOOS == "linux" && strings.Contains(source, "O_FFI") {
		flags = append(flags, "-ldl")
	}
	flags = append(flags, options.LinalgProviderFlags...)
	if nativeTLSProviderRequested(options.NativeCapabilities) {
		opensslFlags, err := nativeOpenSSLFlags(ctx)
		if err != nil {
			return nil, err
		}
		flags = append(flags, opensslFlags...)
	}
	for _, capability := range normalizedNativeCapabilities(options.NativeCapabilities) {
		flags = append(flags, "-DOSIRIS_CAP_"+nativeCapabilityMacro(capability)+"=1")
	}
	sourceHash := sha256.Sum256([]byte(source))
	providerHash := ""
	if options.LinalgProviderSource != "" {
		data, err := os.ReadFile(options.LinalgProviderSource)
		if err != nil {
			return nil, fmt.Errorf("native numerical provider %q: %w", options.LinalgProviderSource, err)
		}
		hash := sha256.Sum256(data)
		providerHash = hex.EncodeToString(hash[:])
	}
	target := "native-executable/" + runtime.GOOS + "/" + runtime.GOARCH
	keyInput := strings.Join([]string{fmt.Sprintf("c-cache-v%d", cCompileManifestVersion), hex.EncodeToString(sourceHash[:]), providerHash, compilerPath, string(identityOut), strings.Join(flags, "\x00"), target}, "\x00")
	keyHash := sha256.Sum256([]byte(keyInput))
	manifest := &CCompileManifest{
		FormatVersion:        cCompileManifestVersion,
		Key:                  hex.EncodeToString(keyHash[:]),
		SourceSHA256:         hex.EncodeToString(sourceHash[:]),
		LinalgProviderSHA256: providerHash,
		Compiler:             compilerPath,
		CompilerIdentity:     strings.TrimSpace(string(identityOut)),
		Flags:                flags,
		Target:               target,
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, err
	}
	cachePath := filepath.Join(cacheDir, manifest.Key)
	if info, err := os.Stat(cachePath); err == nil && !info.IsDir() {
		if err := validateCCompileManifest(cachePath+".json", manifest); err != nil {
			return nil, err
		}
		if err := copyExecutable(cachePath, outputPath); err != nil {
			return nil, err
		}
		manifest.CacheHit = true
		return manifest, nil
	}
	tempPath := cachePath + ".tmp"
	if err := compileCArtifact(ctx, source, tempPath, false, options); err != nil {
		return nil, err
	}
	if err := os.Rename(tempPath, cachePath); err != nil {
		_ = os.Remove(tempPath)
		return nil, err
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(cachePath+".json", append(encoded, '\n'), 0o644); err != nil {
		return nil, err
	}
	if err := copyExecutable(cachePath, outputPath); err != nil {
		return nil, err
	}
	return manifest, nil
}

func validateCCompileManifest(path string, expected *CCompileManifest) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("native executable cache manifest %q: %w", path, err)
	}
	var got CCompileManifest
	if err := json.Unmarshal(data, &got); err != nil {
		return fmt.Errorf("native executable cache manifest %q is corrupt: %w", path, err)
	}
	if got.FormatVersion != cCompileManifestVersion {
		return fmt.Errorf("native executable cache manifest %q has unsupported format version %d (expected %d)", path, got.FormatVersion, cCompileManifestVersion)
	}
	if got.Key != expected.Key || got.SourceSHA256 != expected.SourceSHA256 || got.LinalgProviderSHA256 != expected.LinalgProviderSHA256 || got.Compiler != expected.Compiler || got.CompilerIdentity != expected.CompilerIdentity || got.Target != expected.Target || strings.Join(got.Flags, "\x00") != strings.Join(expected.Flags, "\x00") {
		return fmt.Errorf("native executable cache manifest %q does not match current semantic build inputs", path)
	}
	return nil
}

func copyExecutable(sourcePath, outputPath string) error {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, data, 0o755); err != nil {
		return err
	}
	return os.Chmod(outputPath, 0o755)
}

// compileCArtifact handles the compile cartifact step in the surrounding
// package and returns error.
//
// It returns an error when the operation cannot complete cleanly.
func compileCArtifact(ctx context.Context, source, outputPath string, object bool, options CCompileOptions) error {
	compiler := os.Getenv("CC")
	if compiler == "" {
		compiler = "cc"
	}
	if _, err := exec.LookPath(compiler); err != nil {
		return fmt.Errorf("%s not found: %w", compiler, err)
	}
	dir, err := newNativeTempDir()
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	srcPath := filepath.Join(dir, "snapshot.c")
	if err := os.WriteFile(srcPath, []byte(source), 0o644); err != nil {
		return err
	}

	args := []string{"-std=c11", "-O3", "-DNDEBUG", "-ffp-contract=fast"}
	sanitizerFlags := nativeSanitizerFlags()
	if len(sanitizerFlags) > 0 {
		args = append(args, "-O1", "-g", "-fno-omit-frame-pointer")
		args = append(args, sanitizerFlags...)
	}
	if object {
		args = append(args, "-c", srcPath, "-o", outputPath)
	} else {
		if len(sanitizerFlags) == 0 {
			args = append(args, "-flto")
		}
		args = append(args, srcPath, "-o", outputPath)
		if options.LinalgProviderSource != "" {
			if _, err := os.Stat(options.LinalgProviderSource); err != nil {
				return fmt.Errorf("native numerical provider %q: %w", options.LinalgProviderSource, err)
			}
			args = append(args, options.LinalgProviderSource)
		}
		args = append(args, options.LinalgProviderFlags...)
	}
	if nativeTLSProviderRequested(options.NativeCapabilities) {
		opensslFlags, err := nativeOpenSSLFlags(ctx)
		if err != nil {
			return err
		}
		args = append(args, opensslFlags...)
	}
	cmd := exec.CommandContext(ctx, compiler, args...)
	if !object {
		cmd.Args = append(cmd.Args, "-lm")
		if runtime.GOOS == "windows" {
			// The Windows native host adapter owns HANDLEs and Winsock sockets;
			// these libraries provide CreateProcess/CreateFile security support
			// and the ws2tcpip networking ABI used by generated C.
			cmd.Args = append(cmd.Args, "-lws2_32", "-ladvapi32")
		}
		if strings.Contains(source, "O_FFI") && runtime.GOOS == "linux" {
			cmd.Args = append(cmd.Args, "-ldl")
		}
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s failed: %w\n%s", compiler, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func nativeOpenSSLFlags(ctx context.Context) ([]string, error) {
	pkgConfig, err := exec.LookPath("pkg-config")
	if err != nil {
		return nil, fmt.Errorf("native TLS provider requires pkg-config and OpenSSL: %w", err)
	}
	output, err := exec.CommandContext(ctx, pkgConfig, "--cflags", "--libs", "openssl").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("native TLS provider requires an OpenSSL pkg-config package: %w\n%s", err, strings.TrimSpace(string(output)))
	}
	return strings.Fields(string(output)), nil
}

func nativeSanitizerFlags() []string {
	switch os.Getenv("OSIRIS_C_SANITIZERS") {
	case "", "0":
		return nil
	case "address":
		return []string{"-fsanitize=address"}
	case "undefined":
		return []string{"-fsanitize=undefined"}
	default:
		return []string{"-fsanitize=address,undefined"}
	}
}

// newNativeTempDir creates compiler scratch space with the operating system's
// private temporary-directory mode (0700 on Unix). It is factored for the
// security contract and its permission regression test.
func newNativeTempDir() (string, error) {
	return os.MkdirTemp("", "osiris-c-*")
}
