package osiris

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

var cIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var sha256Digest = regexp.MustCompile(`^[0-9a-f]{64}$`)

// FFIFunction is the manifest-level ABI description for one C symbol. The
// runtime accepts only this closed vocabulary; there is no integer-address or
// variadic escape hatch.
type FFIFunction struct {
	Name              string   `json:"name"`
	Symbol            string   `json:"symbol"`
	Args              []string `json:"args"`
	Result            string   `json:"result"`
	Ownership         string   `json:"ownership"`
	CallingConvention string   `json:"calling_convention"`
	ThreadAffinity    string   `json:"thread_affinity"`
	Nullable          []bool   `json:"nullable,omitempty"`
}

type FFIConstant struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Value  string `json:"value"`
	Symbol string `json:"symbol,omitempty"`
}

// FFIField describes one fixed-layout field. Structs cross the safe v1
// boundary as bounded byte records; callers never receive a C address.
type FFIField struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Offset int    `json:"offset"`
}

type FFIStruct struct {
	Name      string     `json:"name"`
	Size      int        `json:"size"`
	Alignment int        `json:"alignment"`
	ByteOrder string     `json:"byte_order"`
	Fields    []FFIField `json:"fields"`
}

type FFIHandleType struct {
	Name          string `json:"name"`
	ReleaseSymbol string `json:"release_symbol"`
}

type FFICallback struct {
	Name              string   `json:"name"`
	Args              []string `json:"args"`
	Result            string   `json:"result"`
	CallingConvention string   `json:"calling_convention"`
}

// FFIManifest is the reproducible, capability-gated description consumed by
// future direct-link and dynamic-load adapters. Loading validates this object
// before any native library is opened.
type FFIManifest struct {
	ABIVersion   int             `json:"abi_version"`
	Library      string          `json:"library"`
	Version      string          `json:"version"`
	Platform     string          `json:"platform"`
	Architecture string          `json:"architecture"`
	SHA256       string          `json:"sha256"`
	Capabilities []string        `json:"capabilities"`
	SearchPolicy string          `json:"search_policy"`
	Functions    []FFIFunction   `json:"functions"`
	Constants    []FFIConstant   `json:"constants,omitempty"`
	Structs      []FFIStruct     `json:"structs,omitempty"`
	Handles      []FFIHandleType `json:"handles,omitempty"`
	Callbacks    []FFICallback   `json:"callbacks,omitempty"`
}

var allowedFFITypes = map[string]bool{
	"void": true, "bool": true, "i8": true, "u8": true, "i16": true,
	"u16": true, "i32": true, "u32": true, "i64": true, "u64": true,
	"f32": true, "f64": true, "string": true, "const_buffer": true,
	"buffer": true,
}

func validateFFIType(name string) bool {
	if allowedFFITypes[name] {
		return true
	}
	return strings.HasPrefix(name, "handle:") && cIdentifier.MatchString(strings.TrimPrefix(name, "handle:"))
}

func ffiNamedType(name, prefix string) (string, bool) {
	if !strings.HasPrefix(name, prefix) {
		return "", false
	}
	value := strings.TrimPrefix(name, prefix)
	return value, value != "" && cIdentifier.MatchString(value)
}

// Validate checks the closed ABI vocabulary and the caller's explicit grants.
// A nil or missing grant never authorizes a capability, even when the manifest
// itself is otherwise valid.
func (manifest FFIManifest) Validate(granted map[string]bool) error {
	if manifest.ABIVersion != 1 {
		return fmt.Errorf("ffi manifest: unsupported abi_version %d", manifest.ABIVersion)
	}
	if manifest.Library == "" || !filepath.IsAbs(manifest.Library) {
		return fmt.Errorf("ffi manifest: library must be an absolute path")
	}
	if manifest.SearchPolicy != "absolute" {
		return fmt.Errorf("ffi manifest: search_policy must be absolute")
	}
	if !sha256Digest.MatchString(manifest.SHA256) {
		return fmt.Errorf("ffi manifest: sha256 must be a lowercase SHA-256 digest")
	}
	if manifest.Platform != "" && manifest.Platform != runtime.GOOS {
		return fmt.Errorf("ffi manifest: platform %q does not match %q", manifest.Platform, runtime.GOOS)
	}
	if manifest.Architecture != "" && manifest.Architecture != runtime.GOARCH {
		return fmt.Errorf("ffi manifest: architecture %q does not match %q", manifest.Architecture, runtime.GOARCH)
	}
	seenCapabilities := make(map[string]struct{}, len(manifest.Capabilities))
	for _, capability := range manifest.Capabilities {
		if capability == "" || strings.ContainsAny(capability, "\r\n") {
			return fmt.Errorf("ffi manifest: invalid capability")
		}
		if _, duplicate := seenCapabilities[capability]; duplicate {
			return fmt.Errorf("ffi manifest: duplicate capability %q", capability)
		}
		seenCapabilities[capability] = struct{}{}
		if !granted[capability] {
			return fmt.Errorf("ffi manifest: capability %q was not granted", capability)
		}
	}
	seenNames := make(map[string]struct{}, len(manifest.Functions)+len(manifest.Constants))
	seenStructs := make(map[string]struct{}, len(manifest.Structs))
	for _, structure := range manifest.Structs {
		if !cIdentifier.MatchString(structure.Name) || structure.Size <= 0 || structure.Alignment <= 0 || (structure.ByteOrder != "" && structure.ByteOrder != "little" && structure.ByteOrder != "big") {
			return fmt.Errorf("ffi manifest: invalid struct %q layout", structure.Name)
		}
		if _, duplicate := seenStructs[structure.Name]; duplicate {
			return fmt.Errorf("ffi manifest: duplicate struct %q", structure.Name)
		}
		seenStructs[structure.Name] = struct{}{}
		seenFields := make(map[string]struct{}, len(structure.Fields))
		for _, field := range structure.Fields {
			if !cIdentifier.MatchString(field.Name) || field.Offset < 0 || field.Offset >= structure.Size {
				return fmt.Errorf("ffi manifest: invalid field %s.%s", structure.Name, field.Name)
			}
			if _, duplicate := seenFields[field.Name]; duplicate {
				return fmt.Errorf("ffi manifest: duplicate field %s.%s", structure.Name, field.Name)
			}
			seenFields[field.Name] = struct{}{}
			if !validateFFIType(field.Type) || strings.HasPrefix(field.Type, "struct:") || strings.HasPrefix(field.Type, "handle:") || strings.HasPrefix(field.Type, "callback:") {
				return fmt.Errorf("ffi manifest: struct field %s.%s must use a fixed scalar type", structure.Name, field.Name)
			}
		}
	}
	seenHandles := make(map[string]struct{}, len(manifest.Handles))
	for _, handle := range manifest.Handles {
		if !cIdentifier.MatchString(handle.Name) || !cIdentifier.MatchString(handle.ReleaseSymbol) {
			return fmt.Errorf("ffi manifest: invalid handle declaration %q", handle.Name)
		}
		if _, duplicate := seenHandles[handle.Name]; duplicate {
			return fmt.Errorf("ffi manifest: duplicate handle %q", handle.Name)
		}
		seenHandles[handle.Name] = struct{}{}
	}
	seenCallbacks := make(map[string]struct{}, len(manifest.Callbacks))
	for _, callback := range manifest.Callbacks {
		if !cIdentifier.MatchString(callback.Name) || callback.Result == "" || !validateFFIType(callback.Result) {
			return fmt.Errorf("ffi manifest: invalid callback %q", callback.Name)
		}
		if _, duplicate := seenCallbacks[callback.Name]; duplicate {
			return fmt.Errorf("ffi manifest: duplicate callback %q", callback.Name)
		}
		seenCallbacks[callback.Name] = struct{}{}
		if callback.CallingConvention != "c" && callback.CallingConvention != "system" {
			return fmt.Errorf("ffi manifest: invalid callback calling convention %q", callback.CallingConvention)
		}
		for _, argument := range callback.Args {
			if !validateFFIType(argument) {
				return fmt.Errorf("ffi manifest: unsupported callback argument type %q", argument)
			}
		}
	}
	for _, function := range manifest.Functions {
		if !cIdentifier.MatchString(function.Name) || !cIdentifier.MatchString(function.Symbol) {
			return fmt.Errorf("ffi manifest: function names and symbols must be C identifiers")
		}
		if _, duplicate := seenNames[function.Name]; duplicate {
			return fmt.Errorf("ffi manifest: duplicate member %q", function.Name)
		}
		seenNames[function.Name] = struct{}{}
		if function.Result == "" || (!validateFFIType(function.Result) && !strings.HasPrefix(function.Result, "struct:") && !strings.HasPrefix(function.Result, "callback:") && !strings.HasPrefix(function.Result, "handle:")) {
			return fmt.Errorf("ffi manifest: unsupported result type %q", function.Result)
		}
		for _, argument := range function.Args {
			if !validateFFIType(argument) && !strings.HasPrefix(argument, "struct:") && !strings.HasPrefix(argument, "callback:") {
				return fmt.Errorf("ffi manifest: unsupported argument type %q", argument)
			}
			if named, ok := ffiNamedType(argument, "struct:"); ok {
				if _, exists := seenStructs[named]; !exists {
					return fmt.Errorf("ffi manifest: function references undeclared struct %q", named)
				}
			}
			if named, ok := ffiNamedType(argument, "handle:"); ok {
				if _, exists := seenHandles[named]; !exists {
					return fmt.Errorf("ffi manifest: function references undeclared handle %q", named)
				}
			}
			if named, ok := ffiNamedType(argument, "callback:"); ok {
				if _, exists := seenCallbacks[named]; !exists {
					return fmt.Errorf("ffi manifest: function references undeclared callback %q", named)
				}
			}
		}
		if !validateFFIType(function.Result) && !strings.HasPrefix(function.Result, "struct:") && !strings.HasPrefix(function.Result, "callback:") {
			return fmt.Errorf("ffi manifest: unsupported result type %q", function.Result)
		}
		if named, ok := ffiNamedType(function.Result, "struct:"); ok {
			if _, exists := seenStructs[named]; !exists {
				return fmt.Errorf("ffi manifest: function references undeclared struct %q", named)
			}
		}
		if named, ok := ffiNamedType(function.Result, "handle:"); ok {
			if _, exists := seenHandles[named]; !exists {
				return fmt.Errorf("ffi manifest: function references undeclared handle %q", named)
			}
		}
		if len(function.Nullable) != 0 && len(function.Nullable) != len(function.Args) {
			return fmt.Errorf("ffi manifest: nullable must match argument count")
		}
		if function.Ownership != "borrowed" && function.Ownership != "owned" && function.Ownership != "transferred" && function.Ownership != "retained" {
			return fmt.Errorf("ffi manifest: invalid ownership %q", function.Ownership)
		}
		if function.CallingConvention != "c" && function.CallingConvention != "system" {
			return fmt.Errorf("ffi manifest: invalid calling convention %q", function.CallingConvention)
		}
		if function.ThreadAffinity != "any" && function.ThreadAffinity != "task" {
			return fmt.Errorf("ffi manifest: invalid thread affinity %q", function.ThreadAffinity)
		}
	}
	for _, constant := range manifest.Constants {
		if !cIdentifier.MatchString(constant.Name) || (constant.Symbol != "" && !cIdentifier.MatchString(constant.Symbol)) {
			return fmt.Errorf("ffi manifest: invalid constant name or symbol")
		}
		if _, duplicate := seenNames[constant.Name]; duplicate {
			return fmt.Errorf("ffi manifest: duplicate member %q", constant.Name)
		}
		seenNames[constant.Name] = struct{}{}
		if !validateFFIType(constant.Type) || constant.Type == "void" {
			return fmt.Errorf("ffi manifest: unsupported constant type %q", constant.Type)
		}
		if _, err := parseFFIConstant(constant.Type, constant.Value); err != nil {
			return fmt.Errorf("ffi manifest: constant %q: %w", constant.Name, err)
		}
	}
	return nil
}

// LoadFFIManifest validates the manifest and the pinned library bytes before
// a loader is allowed to use it. The returned object contains no live pointer
// and is safe to retain across interpreter/VM execution.
func LoadFFIManifest(path string, granted map[string]bool) (FFIManifest, error) {
	return LoadFFIManifestWithGrants(path, granted)
}

// FFIHandle is an opaque, idempotently closable ownership token. It does not
// expose an address or permit conversion to an integer.
type FFIHandle struct {
	mu       sync.Mutex
	typeID   string
	value    any
	release  func(any) error
	closed   bool
	closing  chan struct{}
	closeErr error
	borrows  int
}

// FFILibrary is a manifest-pinned, capability-authorized native library. The
// native address is kept behind the unexported adapter and is never converted
// into an Osiris integer or exposed to user code.
type FFILibrary struct {
	mu              sync.Mutex
	manifest        FFIManifest
	native          ffiNativeLibrary
	closed          bool
	closing         chan struct{}
	closeErr        error
	activeCalls     int
	activeHandles   int
	capabilityCheck func(string) bool
}

// GrantCapability and RevokeCapability are host-only controls. Language code
// can use an already granted capability but cannot create a new grant.
func (r *Runtime) GrantCapability(name string) {
	if r == nil || name == "" {
		return
	}
	r.capabilityMu.Lock()
	if r.capabilities == nil {
		r.capabilities = make(map[string]bool)
	}
	r.capabilities[name] = true
	r.capabilityMu.Unlock()
}

func (r *Runtime) RevokeCapability(name string) {
	if r == nil {
		return
	}
	r.capabilityMu.Lock()
	delete(r.capabilities, name)
	r.capabilityMu.Unlock()
}

func (r *Runtime) Capabilities() map[string]bool {
	out := make(map[string]bool)
	if r == nil {
		return out
	}
	r.capabilityMu.RLock()
	for name, granted := range r.capabilities {
		out[name] = granted
	}
	r.capabilityMu.RUnlock()
	return out
}

func (r *Runtime) hasCapability(name string) bool {
	if r == nil || name == "" {
		return false
	}
	r.capabilityMu.RLock()
	granted := r.capabilities[name]
	r.capabilityMu.RUnlock()
	return granted
}

func (r *Runtime) ffiLoadContextValue(ctx context.Context, args []Value) (Value, error) {
	if err := contextError(ctx); err != nil {
		return nullValue(), err
	}
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("ffi_load expects a manifest path")
	}
	path, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	library, err := loadFFILibraryWithChecker(path, r.hasCapability)
	if err != nil {
		return nullValue(), err
	}
	return ffiLibraryValueOf(library), nil
}

func ffiCallContextValue(ctx context.Context, args []Value) (Value, error) {
	if err := contextError(ctx); err != nil {
		return nullValue(), err
	}
	if len(args) < 2 {
		return nullValue(), fmt.Errorf("ffi_call expects library, function name, and arguments")
	}
	if args[0].kind != kindFFILibrary || args[0].ffi == nil {
		return nullValue(), fmt.Errorf("ffi_call expects an ffi library")
	}
	name, err := args[1].AsString()
	if err != nil {
		return nullValue(), err
	}
	return args[0].ffi.CallContext(ctx, name, args[2:])
}

func ffiConstantContextValue(ctx context.Context, args []Value) (Value, error) {
	if err := contextError(ctx); err != nil {
		return nullValue(), err
	}
	if len(args) != 2 || args[0].kind != kindFFILibrary || args[0].ffi == nil {
		return nullValue(), fmt.Errorf("ffi_const expects an ffi library and constant name")
	}
	name, err := args[1].AsString()
	if err != nil {
		return nullValue(), err
	}
	return args[0].ffi.Constant(name)
}

func ffiCloseContextValue(_ context.Context, args []Value) (Value, error) {
	if len(args) != 1 || args[0].kind != kindFFILibrary || args[0].ffi == nil {
		return nullValue(), fmt.Errorf("ffi_close expects an ffi library")
	}
	if err := args[0].ffi.Close(); err != nil {
		return nullValue(), err
	}
	return boolValue(true), nil
}

// LoadFFILibrary validates identity and capabilities before opening the
// library. The host must provide grants explicitly; source code cannot grant
// itself an FFI capability.
func LoadFFILibrary(manifestPath string, granted map[string]bool) (*FFILibrary, error) {
	return loadFFILibraryWithChecker(manifestPath, func(name string) bool { return granted[name] })
}

func loadFFILibraryWithChecker(manifestPath string, capabilityCheck func(string) bool) (*FFILibrary, error) {
	if capabilityCheck == nil {
		capabilityCheck = func(string) bool { return false }
	}
	granted := make(map[string]bool)
	// The manifest must still be validated against a concrete grant snapshot
	// before loading. The live checker below additionally enforces revocation on
	// every call after the library has been opened.
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var candidate FFIManifest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&candidate); err != nil {
		return nil, fmt.Errorf("ffi manifest: %w", err)
	}
	for _, capability := range candidate.Capabilities {
		granted[capability] = capabilityCheck(capability)
	}
	manifest, err := LoadFFIManifestWithGrants(manifestPath, granted)
	if err != nil {
		return nil, err
	}
	native, err := openFFINativeLibrary(manifest.Library)
	if err != nil {
		return nil, fmt.Errorf("ffi: open %s: %w", manifest.Library, err)
	}
	for _, function := range manifest.Functions {
		if err := native.lookup(function.Symbol); err != nil {
			native.close()
			return nil, fmt.Errorf("ffi: symbol %s: %w", function.Symbol, err)
		}
	}
	for _, handle := range manifest.Handles {
		if err := native.lookup(handle.ReleaseSymbol); err != nil {
			native.close()
			return nil, fmt.Errorf("ffi: handle release symbol %s: %w", handle.ReleaseSymbol, err)
		}
	}
	return &FFILibrary{manifest: manifest, native: native, capabilityCheck: capabilityCheck}, nil
}

func LoadFFIManifestWithGrants(path string, granted map[string]bool) (FFIManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return FFIManifest{}, err
	}
	var manifest FFIManifest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return FFIManifest{}, fmt.Errorf("ffi manifest: %w", err)
	}
	if err := manifest.Validate(granted); err != nil {
		return FFIManifest{}, err
	}
	library, err := os.ReadFile(manifest.Library)
	if err != nil {
		return FFIManifest{}, fmt.Errorf("ffi manifest: library: %w", err)
	}
	digest := sha256.Sum256(library)
	if got := hex.EncodeToString(digest[:]); got != manifest.SHA256 {
		return FFIManifest{}, fmt.Errorf("ffi manifest: library sha256 mismatch")
	}
	return manifest, nil
}

func (library *FFILibrary) Close() error {
	if library == nil {
		return nil
	}
	library.mu.Lock()
	if library.closed {
		err := library.closeErr
		library.mu.Unlock()
		return err
	}
	if closing := library.closing; closing != nil {
		library.mu.Unlock()
		<-closing
		library.mu.Lock()
		err := library.closeErr
		library.mu.Unlock()
		return err
	}
	if library.activeCalls != 0 || library.activeHandles != 0 {
		library.mu.Unlock()
		return fmt.Errorf("ffi: library has active calls or unreleased handles")
	}
	library.closing = make(chan struct{})
	native := library.native
	library.mu.Unlock()
	err := native.close()
	library.mu.Lock()
	library.closeErr = err
	if err == nil {
		library.closed = true
		library.native = ffiNativeLibrary{}
	}
	closing := library.closing
	library.closing = nil
	close(closing)
	library.mu.Unlock()
	return err
}

func (library *FFILibrary) Call(name string, args []Value) (Value, error) {
	return library.CallContext(context.Background(), name, args)
}

func (library *FFILibrary) CallContext(ctx context.Context, name string, args []Value) (Value, error) {
	if library == nil {
		return nullValue(), fmt.Errorf("ffi: nil library")
	}
	library.mu.Lock()
	if library.closed || library.closing != nil {
		library.mu.Unlock()
		return nullValue(), fmt.Errorf("ffi: library is closed or closing")
	}
	native := library.native
	var declaration *FFIFunction
	for i := range library.manifest.Functions {
		if library.manifest.Functions[i].Name == name {
			declaration = &library.manifest.Functions[i]
			break
		}
	}
	library.mu.Unlock()
	if declaration == nil {
		return nullValue(), fmt.Errorf("ffi: function %q is not declared", name)
	}
	library.mu.Lock()
	if library.closed || library.closing != nil {
		library.mu.Unlock()
		return nullValue(), fmt.Errorf("ffi: library is closed or closing")
	}
	library.activeCalls++
	library.mu.Unlock()
	defer func() {
		library.mu.Lock()
		library.activeCalls--
		library.mu.Unlock()
	}()
	if err := library.requireCapabilities(); err != nil {
		return nullValue(), err
	}
	if len(args) != len(declaration.Args) {
		return nullValue(), fmt.Errorf("ffi: %s expects %d argument(s), got %d", name, len(declaration.Args), len(args))
	}
	if err := contextError(ctx); err != nil {
		return nullValue(), err
	}
	if declaration.ThreadAffinity == "task" && ctx == nil {
		return nullValue(), fmt.Errorf("ffi: %s requires a task context", name)
	}
	if ffiHandleSignature(declaration) {
		var input any
		var borrowed *FFIHandle
		if len(args) == 1 {
			if args[0].kind != kindFFIHandle || args[0].ffiHandle == nil {
				return nullValue(), fmt.Errorf("ffi: %s expects an open opaque handle", name)
			}
			borrowed = args[0].ffiHandle
			var err error
			input, err = borrowed.acquire()
			if err != nil {
				return nullValue(), fmt.Errorf("ffi: %s expects an open opaque handle: %w", name, err)
			}
			defer borrowed.releaseBorrow()
		}
		output, err := native.callHandle(declaration.Symbol, input)
		if err != nil {
			return nullValue(), networkFailure("ffi_call", name, err)
		}
		if declaration.Result == "void" {
			return nullValue(), nil
		}
		handleName, _ := ffiNamedType(declaration.Result, "handle:")
		var releaseSymbol string
		for _, handle := range library.manifest.Handles {
			if handle.Name == handleName {
				releaseSymbol = handle.ReleaseSymbol
				break
			}
		}
		handle, err := NewFFIHandle("handle:"+handleName, output)
		if err != nil {
			return nullValue(), err
		}
		library.mu.Lock()
		library.activeHandles++
		library.mu.Unlock()
		handle.release = func(value any) error {
			err := native.releaseHandle(releaseSymbol, value)
			library.mu.Lock()
			if library.activeHandles > 0 {
				library.activeHandles--
			}
			library.mu.Unlock()
			return err
		}
		return ffiHandleValueOf(handle), nil
	}
	if ffiCallbackSignature(declaration, library.manifest) {
		if len(args) != 1 || args[0].kind != kindFunction || args[0].fn == nil {
			return nullValue(), fmt.Errorf("ffi: %s expects one callback function", name)
		}
		callbackID, unregister, err := registerFFICallback(ctx, args[0])
		if err != nil {
			return nullValue(), err
		}
		result, callErr := native.callCallbackInteger(declaration.Symbol, callbackID)
		unregister()
		if callErr != nil {
			return nullValue(), networkFailure("ffi_call", name, callErr)
		}
		if err := contextError(ctx); err != nil {
			return nullValue(), err
		}
		return ffiIntegerResult(declaration.Result, result), nil
	}
	if ffiStringSignature(declaration) {
		values := make([]string, len(args))
		for i, arg := range args {
			if arg.kind != kindString || strings.IndexByte(arg.str, 0) >= 0 {
				return nullValue(), fmt.Errorf("ffi: %s argument %d expects a NUL-free string", name, i+1)
			}
			values[i] = arg.str
		}
		result, err := native.callString(declaration.Symbol, values)
		if err != nil {
			return nullValue(), networkFailure("ffi_call", name, err)
		}
		return stringValue(result), nil
	}
	if ffiConstBufferSignature(declaration) {
		buffers := make([][]byte, len(args))
		for i, arg := range args {
			data, err := bytesFromValue(arg)
			if err != nil {
				return nullValue(), fmt.Errorf("ffi: %s argument %d expects byte-compatible const_buffer", name, i+1)
			}
			buffers[i] = data
		}
		result, err := native.callConstBufferInteger(declaration.Symbol, buffers)
		if err != nil {
			return nullValue(), networkFailure("ffi_call", name, err)
		}
		switch declaration.Result {
		case "void":
			return nullValue(), nil
		case "bool":
			return boolValue(result != 0), nil
		case "u8", "u16", "u32", "u64":
			return uintValue(uint64(result), uint8(uintWidth(declaration.Result))), nil
		default:
			return intValue(big.NewInt(result), intWidth(declaration.Result)), nil
		}
	}
	if ffiMutableBufferSignature(declaration) {
		data, err := mutableBufferFromValue(args[0])
		if err != nil {
			return nullValue(), fmt.Errorf("ffi: %s argument 1: %w", name, err)
		}
		result, err := native.callMutableBufferInteger(declaration.Symbol, data)
		if err != nil {
			return nullValue(), networkFailure("ffi_call", name, err)
		}
		if err := writeMutableBuffer(args[0], data); err != nil {
			return nullValue(), fmt.Errorf("ffi: %s: %w", name, err)
		}
		return ffiIntegerResult(declaration.Result, result), nil
	}
	if ffiStructSignature(declaration, library.manifest) {
		data, err := encodeFFIStruct(args[0], declaration.Args[0], library.manifest)
		if err != nil {
			return nullValue(), fmt.Errorf("ffi: %s argument 1: %w", name, err)
		}
		result, err := native.callConstBufferInteger(declaration.Symbol, [][]byte{data})
		if err != nil {
			return nullValue(), networkFailure("ffi_call", name, err)
		}
		return ffiIntegerResult(declaration.Result, result), nil
	}
	if !ffiScalarSignature(declaration) {
		return nullValue(), fmt.Errorf("ffi: %s uses an ABI shape not supported by the direct v1 adapter", name)
	}
	if declaration.ThreadAffinity == "task" {
		// Calls are synchronous and remain on the current runtime task. A future
		// scheduler adapter may provide an explicit task handoff; it must not be
		// inferred from a foreign callback.
	}
	if declaration.Result == "f32" || declaration.Result == "f64" {
		values := make([]float64, len(args))
		for i, arg := range args {
			value, err := ffiFloatArgument(arg, declaration.Args[i])
			if err != nil {
				return nullValue(), fmt.Errorf("ffi: %s argument %d: %w", name, i+1, err)
			}
			values[i] = value
		}
		result, err := native.callFloat(declaration.Symbol, values)
		if err != nil {
			return nullValue(), networkFailure("ffi_call", name, err)
		}
		return numberValue(result), nil
	}
	values := make([]int64, len(args))
	for i, arg := range args {
		value, err := ffiIntegerArgument(arg, declaration.Args[i])
		if err != nil {
			return nullValue(), fmt.Errorf("ffi: %s argument %d: %w", name, i+1, err)
		}
		values[i] = value
	}
	result, err := native.callInteger(declaration.Symbol, values)
	if err != nil {
		return nullValue(), networkFailure("ffi_call", name, err)
	}
	return ffiIntegerResult(declaration.Result, result), nil
}

func ffiIntegerResult(typ string, result int64) Value {
	switch typ {
	case "void":
		return nullValue()
	case "bool":
		return boolValue(result != 0)
	case "u8", "u16", "u32", "u64":
		return uintValue(uint64(result), uint8(uintWidth(typ)))
	default:
		return intValue(big.NewInt(result), intWidth(typ))
	}
}

func mutableBufferFromValue(value Value) ([]byte, error) {
	if value.kind != kindList {
		return nil, fmt.Errorf("mutable buffer requires a byte list")
	}
	data, err := packBytes(value.list)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), data...), nil
}

func writeMutableBuffer(value Value, data []byte) error {
	if value.kind != kindList || len(value.list) != len(data) {
		return fmt.Errorf("mutable buffer length changed across foreign call")
	}
	for i, byteValue := range data {
		if value.list[i].kind != kindUint {
			return fmt.Errorf("mutable buffer contains a non-byte element")
		}
		value.list[i] = uintValue(uint64(byteValue), 8)
	}
	return nil
}

func ffiScalarSignature(function *FFIFunction) bool {
	if function == nil || len(function.Args) > 4 {
		return false
	}
	floatResult := function.Result == "f32" || function.Result == "f64"
	for _, argument := range function.Args {
		floatArgument := argument == "f32" || argument == "f64"
		if floatArgument != floatResult || (!floatArgument && !ffiIntegerType(argument)) {
			return false
		}
	}
	return floatResult || function.Result == "void" || function.Result == "bool" || ffiIntegerType(function.Result)
}

func ffiStringSignature(function *FFIFunction) bool {
	if function == nil || function.Result != "string" || len(function.Args) > 4 {
		return false
	}
	if function.Ownership != "borrowed" {
		return false
	}
	for _, argument := range function.Args {
		if argument != "string" {
			return false
		}
	}
	return true
}

func ffiConstBufferSignature(function *FFIFunction) bool {
	if function == nil || len(function.Args) > 2 || function.Ownership != "borrowed" {
		return false
	}
	if function.Result != "void" && function.Result != "bool" && !ffiIntegerType(function.Result) {
		return false
	}
	for _, argument := range function.Args {
		if argument != "const_buffer" {
			return false
		}
	}
	return len(function.Args) > 0
}

func ffiMutableBufferSignature(function *FFIFunction) bool {
	if function == nil || len(function.Args) != 1 || function.Args[0] != "buffer" {
		return false
	}
	return function.Result == "void" || function.Result == "bool" || ffiIntegerType(function.Result)
}

func ffiHandleSignature(function *FFIFunction) bool {
	if function == nil || len(function.Args) > 1 {
		return false
	}
	if !strings.HasPrefix(function.Result, "handle:") {
		return false
	}
	return len(function.Args) == 0 || strings.HasPrefix(function.Args[0], "handle:")
}

func ffiCallbackSignature(function *FFIFunction, manifest FFIManifest) bool {
	if function == nil || len(function.Args) != 1 || !strings.HasPrefix(function.Args[0], "callback:") {
		return false
	}
	if function.Result != "i8" && function.Result != "i16" && function.Result != "i32" && function.Result != "i64" && function.Result != "u8" && function.Result != "u16" && function.Result != "u32" && function.Result != "u64" {
		return false
	}
	name, ok := ffiNamedType(function.Args[0], "callback:")
	if !ok {
		return false
	}
	for _, callback := range manifest.Callbacks {
		if callback.Name == name && len(callback.Args) == 1 && callback.Args[0] == "i64" && callback.Result == "i64" {
			return true
		}
	}
	return false
}

func ffiStructSignature(function *FFIFunction, manifest FFIManifest) bool {
	if function == nil || len(function.Args) != 1 || function.Result == "" {
		return false
	}
	name, ok := ffiNamedType(function.Args[0], "struct:")
	if !ok || (function.Result != "void" && !ffiIntegerType(function.Result)) {
		return false
	}
	for _, structure := range manifest.Structs {
		if structure.Name == name {
			return true
		}
	}
	return false
}

func encodeFFIStruct(value Value, typ string, manifest FFIManifest) ([]byte, error) {
	name, ok := ffiNamedType(typ, "struct:")
	if !ok || value.kind != kindStruct || value.structValue == nil || value.structValue.typ == nil || value.structValue.typ.name != name {
		return nil, fmt.Errorf("expected struct:%s value", name)
	}
	var declaration *FFIStruct
	for i := range manifest.Structs {
		if manifest.Structs[i].Name == name {
			declaration = &manifest.Structs[i]
			break
		}
	}
	if declaration == nil {
		return nil, fmt.Errorf("struct %q is not declared", name)
	}
	out := make([]byte, declaration.Size)
	for _, field := range declaration.Fields {
		index, exists := value.structValue.typ.byName[field.Name]
		if !exists || index >= len(value.structValue.fields) {
			return nil, fmt.Errorf("struct field %s is missing", field.Name)
		}
		encoded, err := encodeFFIScalar(value.structValue.fields[index], field.Type, declaration.ByteOrder)
		if err != nil || field.Offset+len(encoded) > len(out) {
			if err == nil {
				err = fmt.Errorf("field %s exceeds struct size", field.Name)
			}
			return nil, err
		}
		copy(out[field.Offset:], encoded)
	}
	return out, nil
}

func encodeFFIScalar(value Value, typ, byteOrder string) ([]byte, error) {
	if byteOrder == "" {
		byteOrder = "little"
	}
	switch {
	case typ == "bool":
		if value.kind != kindBool {
			return nil, fmt.Errorf("expected bool")
		}
		if value.boo {
			return []byte{1}, nil
		}
		return []byte{0}, nil
	case strings.HasPrefix(typ, "u") || strings.HasPrefix(typ, "i"):
		integer, err := value.Int()
		if err != nil {
			return nil, fmt.Errorf("expected integer %s", typ)
		}
		width := intWidth(typ) / 8
		if width <= 0 || width > 8 {
			return nil, fmt.Errorf("unsupported integer width %s", typ)
		}
		out := make([]byte, width)
		for i := range out {
			index := i
			if byteOrder == "big" {
				index = len(out) - 1 - i
			}
			out[index] = byte(uint64(integer) >> (8 * i))
		}
		return out, nil
	case typ == "f32" || typ == "f64":
		value, err := ffiFloatArgument(value, typ)
		if err != nil {
			return nil, err
		}
		if typ == "f32" {
			bits := math.Float32bits(float32(value))
			if byteOrder == "little" {
				return []byte{byte(bits), byte(bits >> 8), byte(bits >> 16), byte(bits >> 24)}, nil
			}
			return []byte{byte(bits >> 24), byte(bits >> 16), byte(bits >> 8), byte(bits)}, nil
		}
		bits := math.Float64bits(value)
		out := make([]byte, 8)
		for i := range out {
			index := i
			if byteOrder == "big" {
				index = len(out) - 1 - i
			}
			out[index] = byte(bits >> (8 * i))
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported struct field type %q", typ)
	}
}

func (library *FFILibrary) Constant(name string) (Value, error) {
	if library == nil {
		return nullValue(), fmt.Errorf("ffi: nil library")
	}
	library.mu.Lock()
	defer library.mu.Unlock()
	if library.closed || library.closing != nil {
		return nullValue(), fmt.Errorf("ffi: library is closed")
	}
	if err := library.requireCapabilitiesLocked(); err != nil {
		return nullValue(), err
	}
	for _, constant := range library.manifest.Constants {
		if constant.Name == name {
			return parseFFIConstant(constant.Type, constant.Value)
		}
	}
	return nullValue(), fmt.Errorf("ffi: constant %q is not declared", name)
}

func (library *FFILibrary) requireCapabilities() error {
	library.mu.Lock()
	defer library.mu.Unlock()
	return library.requireCapabilitiesLocked()
}

func (library *FFILibrary) requireCapabilitiesLocked() error {
	for _, capability := range library.manifest.Capabilities {
		if library.capabilityCheck == nil || !library.capabilityCheck(capability) {
			return fmt.Errorf("ffi: capability %q was revoked", capability)
		}
	}
	return nil
}

func ffiIntegerType(name string) bool {
	return strings.HasPrefix(name, "i") || strings.HasPrefix(name, "u") || name == "bool" || name == "void"
}

func parseFFIConstant(typ, raw string) (Value, error) {
	switch typ {
	case "bool":
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return nullValue(), fmt.Errorf("expected bool")
		}
		return boolValue(value), nil
	case "string":
		return stringValue(raw), nil
	case "f32", "f64":
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nullValue(), fmt.Errorf("expected %s", typ)
		}
		if typ == "f32" {
			return floatValue(value, 32), nil
		}
		return floatValue(value, 64), nil
	default:
		if strings.HasPrefix(typ, "u") {
			unsigned, err := strconv.ParseUint(raw, 0, 64)
			if err != nil {
				return nullValue(), fmt.Errorf("expected %s", typ)
			}
			return uintValue(unsigned, uint8(uintWidth(typ))), nil
		}
		if strings.HasPrefix(typ, "i") {
			value, err := strconv.ParseInt(raw, 0, 64)
			if err != nil {
				return nullValue(), fmt.Errorf("expected %s", typ)
			}
			return intValue(big.NewInt(value), intWidth(typ)), nil
		}
		return nullValue(), fmt.Errorf("constant type %q is not statically representable", typ)
	}
}

func intWidth(name string) uint16 {
	if len(name) > 1 {
		var width uint16
		_, _ = fmt.Sscanf(name[1:], "%d", &width)
		if width > 0 {
			return width
		}
	}
	return 64
}

func uintWidth(name string) uint16 { return intWidth(name) }

func ffiIntegerArgument(value Value, typ string) (int64, error) {
	if typ == "bool" {
		if value.kind != kindBool {
			return 0, fmt.Errorf("expected bool, got %s", value.Kind())
		}
		if value.boo {
			return 1, nil
		}
		return 0, nil
	}
	integer, err := value.Int()
	if err != nil {
		return 0, fmt.Errorf("expected representable %s", typ)
	}
	return int64(integer), nil
}

func ffiFloatArgument(value Value, typ string) (float64, error) {
	if value.kind != kindNumber && value.kind != kindFixed && value.kind != kindInt && value.kind != kindUint {
		return 0, fmt.Errorf("expected %s, got %s", typ, value.Kind())
	}
	return value.AsNumber()
}

func NewFFIHandle(typeID string, value any) (*FFIHandle, error) {
	if !strings.HasPrefix(typeID, "handle:") || !cIdentifier.MatchString(strings.TrimPrefix(typeID, "handle:")) {
		return nil, fmt.Errorf("invalid ffi handle type %q", typeID)
	}
	return &FFIHandle{typeID: typeID, value: value}, nil
}

func (handle *FFIHandle) TypeID() string {
	if handle == nil {
		return ""
	}
	return handle.typeID
}

func (handle *FFIHandle) IsClosed() bool {
	if handle == nil {
		return true
	}
	handle.mu.Lock()
	closed := handle.closed
	handle.mu.Unlock()
	return closed
}

// acquire borrows the native value for one synchronous foreign call. Close
// cannot mark the handle closed while a borrow exists, and a close that wins
// the lock prevents later borrows from observing a stale native address.
func (handle *FFIHandle) acquire() (any, error) {
	if handle == nil {
		return nil, fmt.Errorf("ffi: nil handle")
	}
	handle.mu.Lock()
	defer handle.mu.Unlock()
	if handle.closed {
		return nil, fmt.Errorf("handle is closed")
	}
	handle.borrows++
	return handle.value, nil
}

func (handle *FFIHandle) releaseBorrow() {
	if handle == nil {
		return
	}
	handle.mu.Lock()
	if handle.borrows > 0 {
		handle.borrows--
	}
	handle.mu.Unlock()
}

func (handle *FFIHandle) Close(release func(any) error) error {
	if handle == nil {
		return nil
	}
	handle.mu.Lock()
	if handle.closed {
		closing := handle.closing
		err := handle.closeErr
		handle.mu.Unlock()
		if closing != nil {
			<-closing
			handle.mu.Lock()
			err = handle.closeErr
			handle.mu.Unlock()
		}
		return err
	}
	if handle.borrows != 0 {
		handle.mu.Unlock()
		return fmt.Errorf("ffi: handle is in use")
	}
	handle.closed = true
	value := handle.value
	handle.value = nil
	handle.closing = make(chan struct{})
	ownedRelease := handle.release
	handle.mu.Unlock()
	// A library-owned release callback cannot be overridden by a caller. The
	// argument remains useful for standalone handles created with NewFFIHandle.
	if ownedRelease != nil {
		release = ownedRelease
	}
	var err error
	if release == nil {
		err = nil
	} else {
		err = release(value)
	}
	handle.mu.Lock()
	handle.closeErr = err
	close(handle.closing)
	handle.mu.Unlock()
	return err
}
