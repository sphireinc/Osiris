package osiris

import "sort"

// BackendStatus is the checked status of one public language surface on a
// backend. Unsupported means that the compiler must reject the surface with a
// source-spanned diagnostic; it never means that the backend silently falls
// back to another evaluator.
type BackendStatus string

const (
	BackendImplemented BackendStatus = "implemented"
	BackendUnsupported BackendStatus = "unsupported"
)

// BackendCapability is the machine-readable parity ledger used by tests and
// documentation. VM is intentionally derived from the public runtime tables;
// native is a conservative allow-list of the operations for which the C
// emitter has direct lowering today.
type BackendCapability struct {
	ID       string
	Surface  string
	VM       BackendStatus
	Native   BackendStatus
	Evidence string
	Note     string
}

// BackendCapabilityInventory returns one entry for every registered builtin,
// context builtin, stdlib module, and stable core construct. Keeping the
// inventory derived from the registration tables prevents a new public name
// from becoming undocumented merely because it was added to a map.
func BackendCapabilityInventory() []BackendCapability {
	builtins := make(map[string]Builtin)
	contextBuiltins := make(map[string]contextCallable)
	registerBuiltins(builtins)
	registerContextBuiltins(contextBuiltins)

	entries := make([]BackendCapability, 0, len(builtins)+len(contextBuiltins)+24)
	for name := range builtins {
		entries = append(entries, BackendCapability{
			ID:       "builtin/" + name,
			Surface:  "builtin",
			VM:       BackendImplemented,
			Native:   nativeBuiltinStatus(name),
			Evidence: "internal/osiris/runtime.go; internal/osiris/bytecode.go",
			Note:     nativeBuiltinNote(name),
		})
	}
	for name := range contextBuiltins {
		entries = append(entries, BackendCapability{
			ID:       "context-builtin/" + name,
			Surface:  "context builtin",
			VM:       BackendImplemented,
			Native:   BackendUnsupported,
			Evidence: "internal/osiris/runtime.go; internal/osiris/bytecode.go",
			Note:     "host capability; native C reports a source-spanned unsupported diagnostic",
		})
	}
	for _, module := range []string{"std/time", "std/text", "std/math", "std/crypto", "std/signal", "std/net", "std/list"} {
		entries = append(entries, BackendCapability{
			ID:       "module/" + module,
			Surface:  "stdlib module",
			VM:       BackendImplemented,
			Native:   nativeModuleStatus(module),
			Evidence: "internal/osiris/module.go; internal/osiris/bytecode.go",
			Note:     nativeModuleNote(module),
		})
	}
	for _, construct := range []string{
		"functions", "closures", "typed-returns", "recursion", "traits", "impl-methods",
		"while", "for", "break", "continue", "defer", "try-catch", "propagate",
		"spawn", "await", "cancel", "timeout", "parallel", "race", "select",
		"result-combinators", "pattern-match", "imports",
	} {
		entries = append(entries, BackendCapability{
			ID:       "syntax/" + construct,
			Surface:  "language construct",
			VM:       BackendImplemented,
			Native:   nativeConstructStatus(construct),
			Evidence: "internal/osiris/bytecode.go; internal/osiris/cbackend_native.go",
			Note:     nativeConstructNote(construct),
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries
}

func nativeBuiltinStatus(name string) BackendStatus {
	// These are host-independent primitive lowerings in cbackend_native.go.
	switch name {
	case "cat", "upper", "lower", "trim", "replace", "repeat", "split", "chunks", "join", "len", "take", "slice", "range", "sum", "product", "json", "format", "slug", "noise", "u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64", "f32", "f64", "number", "real", "imag", "abs", "exp", "log", "log10", "log2", "pow", "sin", "cos", "tan", "asin", "acos", "atan", "atan2", "hypot", "norm", "vector", "vec", "clone", "view", "tensor", "matrix", "shape", "flatten", "squeeze", "expand_dims", "concat", "stack", "tile", "broadcast_to", "where", "clip", "gather", "scatter", "pad", "transpose", "dot", "matmul":
		return BackendImplemented
	default:
		return BackendUnsupported
	}
}

func nativeBuiltinNote(name string) string {
	if nativeBuiltinStatus(name) == BackendImplemented {
		return "direct C lowering is covered by the native emitter"
	}
	return "native C rejects this builtin unless its direct lowering is added"
}

func nativeModuleStatus(name string) BackendStatus {
	if name == "std/net" {
		return BackendUnsupported
	}
	return BackendImplemented
}

func nativeModuleNote(name string) string {
	if nativeModuleStatus(name) == BackendImplemented {
		return "module members are lowered only where the native emitter has a direct case"
	}
	return "networking is hosted-only until a native provider is added"
}

func nativeConstructStatus(name string) BackendStatus {
	if name == "defer" {
		return BackendUnsupported
	}
	return BackendImplemented
}

func nativeConstructNote(name string) string {
	if nativeConstructStatus(name) == BackendImplemented {
		return "direct IR/C lowering or explicit backend validation"
	}
	return "native C emits a source-spanned diagnostic; cleanup-edge lowering remains scoped to task 116"
}
