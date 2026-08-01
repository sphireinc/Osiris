package osiris

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
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

// FFIManifest is the reproducible, capability-gated description consumed by
// future direct-link and dynamic-load adapters. Loading validates this object
// before any native library is opened.
type FFIManifest struct {
	ABIVersion   int           `json:"abi_version"`
	Library      string        `json:"library"`
	Version      string        `json:"version"`
	Platform     string        `json:"platform"`
	Architecture string        `json:"architecture"`
	SHA256       string        `json:"sha256"`
	Capabilities []string      `json:"capabilities"`
	SearchPolicy string        `json:"search_policy"`
	Functions    []FFIFunction `json:"functions"`
	Constants    []FFIConstant `json:"constants,omitempty"`
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
	for _, function := range manifest.Functions {
		if !cIdentifier.MatchString(function.Name) || !cIdentifier.MatchString(function.Symbol) {
			return fmt.Errorf("ffi manifest: function names and symbols must be C identifiers")
		}
		if _, duplicate := seenNames[function.Name]; duplicate {
			return fmt.Errorf("ffi manifest: duplicate member %q", function.Name)
		}
		seenNames[function.Name] = struct{}{}
		if function.Result == "" || !validateFFIType(function.Result) {
			return fmt.Errorf("ffi manifest: unsupported result type %q", function.Result)
		}
		for _, argument := range function.Args {
			if !validateFFIType(argument) {
				return fmt.Errorf("ffi manifest: unsupported argument type %q", argument)
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
	}
	return nil
}

// LoadFFIManifest validates the manifest and the pinned library bytes before
// a loader is allowed to use it. The returned object contains no live pointer
// and is safe to retain across interpreter/VM execution.
func LoadFFIManifest(path string, granted map[string]bool) (FFIManifest, error) {
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

// FFIHandle is an opaque, idempotently closable ownership token. It does not
// expose an address or permit conversion to an integer.
type FFIHandle struct {
	mu     sync.Mutex
	typeID string
	value  any
	closed bool
}

// FFILibrary is a manifest-pinned, capability-authorized native library. The
// native address is kept behind the unexported adapter and is never converted
// into an Osiris integer or exposed to user code.
type FFILibrary struct {
	mu       sync.Mutex
	manifest FFIManifest
	native   ffiNativeLibrary
	closed   bool
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
	library, err := LoadFFILibrary(path, r.Capabilities())
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
	return args[0].ffi.Call(name, args[2:])
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
	manifest, err := LoadFFIManifest(manifestPath, granted)
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
	return &FFILibrary{manifest: manifest, native: native}, nil
}

func (library *FFILibrary) Close() error {
	if library == nil {
		return nil
	}
	library.mu.Lock()
	if library.closed {
		library.mu.Unlock()
		return nil
	}
	library.closed = true
	native := library.native
	library.native = ffiNativeLibrary{}
	library.mu.Unlock()
	return native.close()
}

func (library *FFILibrary) Call(name string, args []Value) (Value, error) {
	if library == nil {
		return nullValue(), fmt.Errorf("ffi: nil library")
	}
	library.mu.Lock()
	if library.closed {
		library.mu.Unlock()
		return nullValue(), fmt.Errorf("ffi: library is closed")
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
	if len(args) != len(declaration.Args) {
		return nullValue(), fmt.Errorf("ffi: %s expects %d argument(s), got %d", name, len(declaration.Args), len(args))
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

func ffiIntegerType(name string) bool {
	return strings.HasPrefix(name, "i") || strings.HasPrefix(name, "u") || name == "bool" || name == "void"
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

func (handle *FFIHandle) Close(release func(any) error) error {
	if handle == nil {
		return nil
	}
	handle.mu.Lock()
	if handle.closed {
		handle.mu.Unlock()
		return nil
	}
	handle.closed = true
	value := handle.value
	handle.value = nil
	handle.mu.Unlock()
	if release == nil {
		return nil
	}
	return release(value)
}
