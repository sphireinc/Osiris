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
	for _, module := range []string{"std/time", "std/text", "std/math", "std/crypto", "std/signal", "std/net", "std/os", "std/list", "std/unsafe"} {
		entries = append(entries, BackendCapability{
			ID:       "module/" + module,
			Surface:  "stdlib module",
			VM:       BackendImplemented,
			Native:   BackendUnsupported,
			Evidence: "internal/osiris/module.go; internal/osiris/bytecode.go",
			Note:     "module availability is implemented; native support is recorded per exported member below",
		})
		inventoryRuntime := NewRuntime()
		if capability := stdlibModuleCapability(module); capability != "" {
			inventoryRuntime.GrantCapability(capability)
		}
		mod, err := stdlibModule(inventoryRuntime, module, "")
		if err != nil {
			continue
		}
		moduleValue, err := mod.AsModule()
		if err != nil {
			continue
		}
		members := make([]string, 0, len(moduleValue.members))
		for member := range moduleValue.members {
			members = append(members, member)
		}
		sort.Strings(members)
		for _, member := range members {
			entries = append(entries, BackendCapability{
				ID:       "module/" + module + "/" + member,
				Surface:  "stdlib module member",
				VM:       BackendImplemented,
				Native:   nativeModuleMemberStatus(module, member),
				Evidence: "internal/osiris/module.go; internal/osiris/cbackend_native.go",
				Note:     nativeModuleMemberNote(module, member),
			})
		}
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
	case "cat", "upper", "lower", "trim", "replace", "repeat", "split", "chunks", "join", "len", "take", "slice", "at", "setat", "range", "sum", "product", "divmod", "gcd", "lcm", "count_ones", "count_zeros", "leading_zeros", "trailing_zeros", "bit_width", "bit_floor", "bit_ceil", "has_single_bit", "byte_swap", "encode", "decode", "json", "format", "slug", "noise", "uint", "byte", "bytes", "u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64", "f32", "f64", "number", "complex", "dual", "primal", "tangent", "real", "imag", "conjugate", "magnitude", "abs", "sqrt", "exp", "exp2", "atan2", "trunc", "fmod", "remainder", "fma", "log", "log10", "log2", "pow", "expt", "sin", "cos", "tan", "asin", "acos", "atan", "sinh", "cosh", "tanh", "asinh", "acosh", "atanh", "expm1", "log1p", "cbrt", "erf", "erfc", "gamma", "lgamma", "floor", "ceil", "round", "hypot", "norm", "min", "max", "clamp", "sign", "step", "heaviside", "relu", "sigmoid", "softplus", "mean", "std", "var", "median", "quantile", "percentile", "argmin", "argmax", "any", "all", "cumsum", "cumprod", "phase", "vector", "vec", "zeros", "ones", "full", "identity", "eye", "arange", "linspace", "logspace", "tensor", "matrix", "reshape", "shape", "flatten", "squeeze", "expand_dims", "concat", "stack", "tile", "broadcast_to", "where", "clip", "gather", "scatter", "pad", "transpose", "dot", "matmul", "outer", "trace", "det", "solve", "inv", "cholesky", "qr", "clone", "view":
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

func nativeModuleMemberStatus(module, member string) BackendStatus {
	if module == "std/math" {
		if member == "pi" || member == "tau" || nativeBuiltinStatus(member) == BackendImplemented {
			return BackendImplemented
		}
		switch member {
		case "gcd", "lcm", "count_ones", "count_zeros", "leading_zeros", "trailing_zeros", "bit_width", "bit_floor", "bit_ceil", "has_single_bit", "byte_swap", "zeros", "ones", "full", "arange", "linspace", "logspace", "outer", "trace", "det", "solve", "inv", "cholesky", "qr", "reshape":
			return BackendImplemented
		}
		return BackendUnsupported
	}
	switch module {
	case "std/net":
		switch member {
		case "tcp_listen", "tcp_accept", "tcp_dial", "udp_bind", "udp_connect", "udp_send", "udp_receive", "dns_lookup", "tls_dial", "tls_dial_config", "tls_listen", "http_get", "http_request", "http_request_stream", "http_serve":
			return BackendImplemented
		}
	case "std/os":
		switch member {
		case "is_path_separator", "read_file", "write_file", "getenv", "env_names", "environ", "setenv", "unsetenv", "hostname", "getpid", "getppid", "getuid", "geteuid", "getgid", "getegid", "getpagesize", "process_spawn", "start_process", "find_process", "process_wait", "process_state", "process_kill", "process_signal", "process_release", "process_stdin", "process_stdout", "process_stderr", "pipe", "stdin", "stdout", "stderr", "fd", "name", "new_file", "with_handle", "syscall_conn", "signal_subscribe", "close", "read", "write", "write_string", "read_at", "write_at", "seek_file", "sync", "set_deadline", "set_read_deadline", "set_write_deadline", "open", "create", "open_file", "open_root", "root_create", "root_open", "root_open_file", "root_read_file", "root_write_file", "root_stat", "root_lstat", "root_chown", "root_lchown", "root_chtimes", "root_fs", "root_readlink", "root_mkdir", "root_mkdir_all", "root_remove", "root_remove_all", "root_rename", "root_link", "root_symlink", "root_chmod", "root_close", "stat", "lstat", "read_dir", "mkdir", "mkdir_all", "remove", "remove_all", "truncate", "rename", "link", "symlink", "chmod", "readlink", "watch":
			return BackendImplemented
		}
	case "std/crypto":
		if member == "sha256" || member == "sha512" || member == "sha512_256" || member == "random" || member == "hash" || member == "digest" || member == "digest_bytes" || member == "hmac" || member == "hex_encode" || member == "hex_decode" || member == "compare_digest" || member == "base64_encode" || member == "base64_decode" || member == "base64url_encode" || member == "base64url_decode" || member == "algorithm_supported" {
			return BackendImplemented
		}
	case "std/list":
		if member == "range" || member == "len" || member == "take" || member == "sum" || member == "product" || member == "reverse" || member == "at" || member == "index_of" || member == "last_index_of" || member == "contains" || member == "count" || member == "sort" || member == "unique" || member == "partition" || member == "slice" || member == "chunks" || member == "windows" || member == "zip" || member == "concat" || member == "repeat" || member == "empty" || member == "all" || member == "any" || member == "none" || member == "enumerate" || member == "flatten" {
			return BackendImplemented
		}
	case "std/signal":
		switch member {
		case "rectangular", "hann", "hamming", "blackman", "bartlett",
			"convolve",
			"correlate", "filter_fir", "resample", "mean", "variance", "energy", "rms",
			"normalize", "fft", "ifft":
			return BackendImplemented
		}
	case "std/text":
		switch member {
		case "trim", "trim_space", "trim_left", "trim_right", "lower", "contains", "contains_any", "has_prefix", "has_suffix", "fields", "join", "slug", "upper", "compare", "equal_fold", "count", "index", "last_index", "index_any", "last_index_any", "cut", "cut_prefix", "cut_suffix", "trim_prefix", "trim_suffix", "replace", "replace_n", "split", "split_after", "split_n", "split_after_n", "chunks", "repeat":
			return BackendImplemented
		}
	case "std/time":
		switch member {
		case "duration", "add", "add_date", "date", "duration_string", "unix", "unix_milli", "unix_micro", "format", "parse_duration", "rfc3339", "rfc3339_nano", "sub", "round", "truncate", "utc_location", "utc", "compare", "equal", "before", "after", "year", "month", "day", "hour", "minute", "second", "nanosecond", "microsecond", "millisecond", "weekday", "year_day":
			return BackendImplemented
		}
	}
	return BackendUnsupported
}

func nativeModuleMemberNote(module, member string) string {
	if nativeModuleMemberStatus(module, member) == BackendImplemented {
		return "direct C lowering is covered for the documented argument and value subset; unsupported shapes fail at compile time"
	}
	if module == "std/net" {
		return "host capability; native C reports a source-spanned unsupported diagnostic"
	}
	if module == "std/os" {
		return "host capability; native C reports a source-spanned unsupported diagnostic"
	}
	if module == "std/unsafe" {
		return "explicit unsafe.memory capability; native C reports a source-spanned unsupported diagnostic"
	}
	return "native C rejects this module member with a source-spanned diagnostic"
}

func nativeConstructStatus(name string) BackendStatus {
	return BackendImplemented
}

func nativeConstructNote(name string) string {
	if nativeConstructStatus(name) == BackendImplemented {
		return "direct IR/C lowering or explicit backend validation"
	}
	return "direct IR/C lowering or explicit backend validation"
}
