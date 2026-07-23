package osiris

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// loadModule operates on Runtime values and keeps the runtime-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func (rt *Runtime) loadModule(ctx context.Context, ref, base string, stack []string) (Value, error) {
	ref, selector := splitModuleRef(ref)
	if strings.HasPrefix(ref, "std/") {
		key := ref
		rt.moduleMu.Lock()
		if mod, ok := rt.modules[key]; ok {
			rt.moduleMu.Unlock()
			if selector != "" {
				mv, err := mod.AsModule()
				if err != nil {
					return nullValue(), err
				}
				if !versionMatchesSelector(mv.version, selector) {
					return nullValue(), fmt.Errorf("stdlib module %s version %s does not satisfy %q", key, mv.version, selector)
				}
			}
			return mod, nil
		}
		rt.moduleMu.Unlock()

		for _, seen := range stack {
			if seen == key {
				return nullValue(), importCycleError(stack, key)
			}
		}

		mod, err := stdlibModule(key, selector)
		if err != nil {
			return nullValue(), err
		}
		rt.moduleMu.Lock()
		rt.modules[key] = mod
		rt.moduleMu.Unlock()
		return mod, nil
	}

	reg := rt.registryFor(base)
	key, src, err := reg.materialize(ref, base)
	if err != nil {
		return nullValue(), err
	}

	rt.moduleMu.Lock()
	if mod, ok := rt.modules[key]; ok {
		rt.moduleMu.Unlock()
		if selector != "" {
			mv, err := mod.AsModule()
			if err != nil {
				return nullValue(), err
			}
			if !versionMatchesSelector(mv.version, selector) {
				return nullValue(), fmt.Errorf("module %s version %s does not satisfy %q", key, mv.version, selector)
			}
		}
		return mod, nil
	}
	rt.moduleMu.Unlock()

	for _, seen := range stack {
		if seen == key {
			return nullValue(), importCycleError(stack, key)
		}
	}

	prog, err := Parse(key, src)
	if err != nil {
		return nullValue(), err
	}

	sess := NewSession(rt, ctx, key)
	if _, err := sess.ExecProgram(prog, true, append(stack, key)); err != nil {
		return nullValue(), err
	}
	if selector != "" && !versionMatchesSelector(sess.packageVersion, selector) {
		if sess.packageVersion == "" {
			return nullValue(), fmt.Errorf("module %s has no version but %q was requested", key, selector)
		}
		return nullValue(), fmt.Errorf("module %s version %s does not satisfy %q", key, sess.packageVersion, selector)
	}
	mod, err := sess.moduleValue()
	if err != nil {
		return nullValue(), err
	}
	if err := reg.recordManifest(key, sess.packageName, sess.packageVersion); err != nil {
		return nullValue(), err
	}

	rt.moduleMu.Lock()
	rt.modules[key] = mod
	rt.moduleMu.Unlock()
	return mod, nil
}

// registryFor operates on Runtime values and keeps the runtime-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns *moduleRegistry so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func (rt *Runtime) registryFor(base string) *moduleRegistry {
	root := registryRootFor(base)
	rt.registryMu.Lock()
	defer rt.registryMu.Unlock()
	if reg, ok := rt.registries[root]; ok {
		return reg
	}
	reg := newModuleRegistry(root)
	rt.registries[root] = reg
	return reg
}

type moduleKind uint8

const (
	moduleKindFile moduleKind = iota
	moduleKindStdlib
)

// resolveModulePath handles the resolve module path step in the surrounding
// package and returns string, moduleKind, error.
//
// It returns string, moduleKind, error so callers can continue with the next
// stage of the pipeline without re-deriving the same information.
func resolveModulePath(ref, base string) (string, moduleKind, error) {
	if strings.HasPrefix(ref, "std/") {
		return ref, moduleKindStdlib, nil
	}

	candidate := ref
	if !filepath.IsAbs(candidate) {
		dir := filepath.Dir(base)
		if base == "" || strings.HasPrefix(base, "<") {
			dir, _ = os.Getwd()
		}
		candidate = filepath.Join(dir, candidate)
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", 0, err
	}
	if _, err := os.Stat(abs); err == nil {
		return abs, moduleKindFile, nil
	}
	if filepath.Ext(abs) == "" {
		withExt := abs + ".osr"
		if _, err := os.Stat(withExt); err == nil {
			return withExt, moduleKindFile, nil
		}
		return withExt, moduleKindFile, fmt.Errorf("module file not found at %s", withExt)
	}
	return abs, moduleKindFile, fmt.Errorf("module file not found at %s", abs)
}

// resolveModuleRef handles the resolve module ref step in the surrounding
// package and returns string, moduleKind, string, error.
//
// It returns string, moduleKind, string, error so callers can continue with
// the next stage of the pipeline without re-deriving the same information.
func resolveModuleRef(ref, base string) (string, moduleKind, string, error) {
	abs, kind, err := resolveModulePath(ref, base)
	if err != nil {
		return "", 0, "", err
	}
	if kind == moduleKindStdlib {
		return ref, kind, "", nil
	}
	data, err := os.ReadFile(abs)
	if err == nil {
		return abs, kind, string(data), nil
	}
	if filepath.Ext(abs) == "" {
		withExt := abs + ".osr"
		data, err = os.ReadFile(withExt)
		if err == nil {
			return withExt, kind, string(data), nil
		}
	}
	return "", 0, "", err
}

// stdlibModule handles the stdlib module step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func stdlibModule(name, selector string) (Value, error) {
	switch name {
	case "std/text":
		version := "1.0.0"
		if !versionMatchesSelector(version, selector) {
			return nullValue(), fmt.Errorf("stdlib module %s version %s does not satisfy %q", name, version, selector)
		}
		return makeModule(name, version, map[string]Value{
			"trim":    functionValueOf("std/text.trim", func(args []Value) (Value, error) { return builtinTrim(args) }),
			"upper":   functionValueOf("std/text.upper", func(args []Value) (Value, error) { return builtinUpper(args) }),
			"lower":   functionValueOf("std/text.lower", func(args []Value) (Value, error) { return builtinLower(args) }),
			"replace": functionValueOf("std/text.replace", func(args []Value) (Value, error) { return builtinReplace(args) }),
			"repeat":  functionValueOf("std/text.repeat", func(args []Value) (Value, error) { return builtinRepeat(args) }),
			"split":   functionValueOf("std/text.split", func(args []Value) (Value, error) { return builtinSplit(args) }),
			"join":    functionValueOf("std/text.join", func(args []Value) (Value, error) { return builtinJoin(args) }),
			"chunks":  functionValueOf("std/text.chunks", func(args []Value) (Value, error) { return builtinChunks(args) }),
			"slug":    functionValueOf("std/text.slug", func(args []Value) (Value, error) { return builtinSlug(args) }),
			"cat":     functionValueOf("std/text.cat", func(args []Value) (Value, error) { return builtinCat(args) }),
		}), nil
	case "std/math":
		version := "1.0.0"
		if !versionMatchesSelector(version, selector) {
			return nullValue(), fmt.Errorf("stdlib module %s version %s does not satisfy %q", name, version, selector)
		}
		return makeModule(name, version, map[string]Value{
			"pi":           numberValue(math.Pi),
			"tau":          numberValue(math.Pi * 2),
			"abs":          functionValueOf("std/math.abs", func(args []Value) (Value, error) { return builtinAbs(args) }),
			"i8":           functionValueOf("std/math.i8", func(args []Value) (Value, error) { return builtinI8(args) }),
			"i16":          functionValueOf("std/math.i16", func(args []Value) (Value, error) { return builtinI16(args) }),
			"i32":          functionValueOf("std/math.i32", func(args []Value) (Value, error) { return builtinI32(args) }),
			"i64":          functionValueOf("std/math.i64", func(args []Value) (Value, error) { return builtinI64(args) }),
			"i128":         functionValueOf("std/math.i128", func(args []Value) (Value, error) { return builtinI128(args) }),
			"i256":         functionValueOf("std/math.i256", func(args []Value) (Value, error) { return builtinI256(args) }),
			"i512":         functionValueOf("std/math.i512", func(args []Value) (Value, error) { return builtinI512(args) }),
			"i1024":        functionValueOf("std/math.i1024", func(args []Value) (Value, error) { return builtinI1024(args) }),
			"u8":           functionValueOf("std/math.u8", func(args []Value) (Value, error) { return builtinU8(args) }),
			"u16":          functionValueOf("std/math.u16", func(args []Value) (Value, error) { return builtinU16(args) }),
			"u32":          functionValueOf("std/math.u32", func(args []Value) (Value, error) { return builtinU32(args) }),
			"u64":          functionValueOf("std/math.u64", func(args []Value) (Value, error) { return builtinU64(args) }),
			"u128":         functionValueOf("std/math.u128", func(args []Value) (Value, error) { return builtinU128(args) }),
			"u256":         functionValueOf("std/math.u256", func(args []Value) (Value, error) { return builtinU256(args) }),
			"u512":         functionValueOf("std/math.u512", func(args []Value) (Value, error) { return builtinU512(args) }),
			"u1024":        functionValueOf("std/math.u1024", func(args []Value) (Value, error) { return builtinU1024(args) }),
			"uint":         functionValueOf("std/math.uint", func(args []Value) (Value, error) { return builtinUint(args) }),
			"byte":         functionValueOf("std/math.byte", func(args []Value) (Value, error) { return builtinByte(args) }),
			"uf8":          functionValueOf("std/math.uf8", func(args []Value) (Value, error) { return builtinUF8(args) }),
			"uf16":         functionValueOf("std/math.uf16", func(args []Value) (Value, error) { return builtinUF16(args) }),
			"uf32":         functionValueOf("std/math.uf32", func(args []Value) (Value, error) { return builtinUF32(args) }),
			"ufnum":        functionValueOf("std/math.ufnum", func(args []Value) (Value, error) { return builtinUFNum(args) }),
			"uf64":         functionValueOf("std/math.uf64", func(args []Value) (Value, error) { return builtinUF64(args) }),
			"uf128":        functionValueOf("std/math.uf128", func(args []Value) (Value, error) { return builtinUF128(args) }),
			"uf256":        functionValueOf("std/math.uf256", func(args []Value) (Value, error) { return builtinUF256(args) }),
			"uf512":        functionValueOf("std/math.uf512", func(args []Value) (Value, error) { return builtinUF512(args) }),
			"uf1024":       functionValueOf("std/math.uf1024", func(args []Value) (Value, error) { return builtinUF1024(args) }),
			"f32":          functionValueOf("std/math.f32", func(args []Value) (Value, error) { return builtinF32(args) }),
			"f64":          functionValueOf("std/math.f64", func(args []Value) (Value, error) { return builtinF64(args) }),
			"bf8":          functionValueOf("std/math.bf8", func(args []Value) (Value, error) { return builtinBF8(args) }),
			"bf16":         functionValueOf("std/math.bf16", func(args []Value) (Value, error) { return builtinBF16(args) }),
			"bf32":         functionValueOf("std/math.bf32", func(args []Value) (Value, error) { return builtinBF32(args) }),
			"bf64":         functionValueOf("std/math.bf64", func(args []Value) (Value, error) { return builtinBF64(args) }),
			"bf128":        functionValueOf("std/math.bf128", func(args []Value) (Value, error) { return builtinBF128(args) }),
			"bf256":        functionValueOf("std/math.bf256", func(args []Value) (Value, error) { return builtinBF256(args) }),
			"bf512":        functionValueOf("std/math.bf512", func(args []Value) (Value, error) { return builtinBF512(args) }),
			"bf1024":       functionValueOf("std/math.bf1024", func(args []Value) (Value, error) { return builtinBF1024(args) }),
			"sin":          functionValueOf("std/math.sin", func(args []Value) (Value, error) { return builtinSin(args) }),
			"cos":          functionValueOf("std/math.cos", func(args []Value) (Value, error) { return builtinCos(args) }),
			"tan":          functionValueOf("std/math.tan", func(args []Value) (Value, error) { return builtinTan(args) }),
			"asin":         functionValueOf("std/math.asin", func(args []Value) (Value, error) { return builtinAsin(args) }),
			"acos":         functionValueOf("std/math.acos", func(args []Value) (Value, error) { return builtinAcos(args) }),
			"atan":         functionValueOf("std/math.atan", func(args []Value) (Value, error) { return builtinAtan(args) }),
			"sinh":         functionValueOf("std/math.sinh", sinhValue),
			"cosh":         functionValueOf("std/math.cosh", coshValue),
			"tanh":         functionValueOf("std/math.tanh", tanhValue),
			"asinh":        functionValueOf("std/math.asinh", asinhValue),
			"acosh":        functionValueOf("std/math.acosh", acoshValue),
			"atanh":        functionValueOf("std/math.atanh", atanhValue),
			"expm1":        functionValueOf("std/math.expm1", expm1Value),
			"log1p":        functionValueOf("std/math.log1p", log1pValue),
			"cbrt":         functionValueOf("std/math.cbrt", cbrtValue),
			"erf":          functionValueOf("std/math.erf", erfValue),
			"erfc":         functionValueOf("std/math.erfc", erfcValue),
			"gamma":        functionValueOf("std/math.gamma", gammaValue),
			"lgamma":       functionValueOf("std/math.lgamma", lgammaValue),
			"complex":      functionValueOf("std/math.complex", func(args []Value) (Value, error) { return builtinComplex(args) }),
			"real":         functionValueOf("std/math.real", func(args []Value) (Value, error) { return builtinReal(args) }),
			"imag":         functionValueOf("std/math.imag", func(args []Value) (Value, error) { return builtinImag(args) }),
			"magnitude":    functionValueOf("std/math.magnitude", func(args []Value) (Value, error) { return builtinMagnitude(args) }),
			"exp":          functionValueOf("std/math.exp", func(args []Value) (Value, error) { return builtinExp(args) }),
			"pow":          functionValueOf("std/math.pow", func(args []Value) (Value, error) { return builtinPow(args) }),
			"expt":         functionValueOf("std/math.expt", func(args []Value) (Value, error) { return builtinExpt(args) }),
			"log":          functionValueOf("std/math.log", func(args []Value) (Value, error) { return builtinLog(args) }),
			"log10":        functionValueOf("std/math.log10", func(args []Value) (Value, error) { return builtinLog10(args) }),
			"log2":         functionValueOf("std/math.log2", func(args []Value) (Value, error) { return builtinLog2(args) }),
			"rat":          functionValueOf("std/math.rat", func(args []Value) (Value, error) { return decimalValueFromArgs(args) }),
			"rational":     functionValueOf("std/math.rational", func(args []Value) (Value, error) { return decimalValueFromArgs(args) }),
			"interval":     functionValueOf("std/math.interval", func(args []Value) (Value, error) { return intervalValueFromArgs(args) }),
			"modint":       functionValueOf("std/math.modint", func(args []Value) (Value, error) { return modIntValueFromArgs(args) }),
			"fixed":        functionValueOf("std/math.fixed", func(args []Value) (Value, error) { return fixedValueFromArgs(args) }),
			"zeros":        functionValueOf("std/math.zeros", zerosValue),
			"ones":         functionValueOf("std/math.ones", onesValue),
			"full":         functionValueOf("std/math.full", fullValue),
			"eye":          functionValueOf("std/math.eye", eyeValue),
			"identity":     functionValueOf("std/math.identity", identityValue),
			"arange":       functionValueOf("std/math.arange", arangeValue),
			"linspace":     functionValueOf("std/math.linspace", linspaceValue),
			"logspace":     functionValueOf("std/math.logspace", logspaceValue),
			"bigint":       functionValueOf("std/math.bigint", func(args []Value) (Value, error) { return builtinBigInt(args) }),
			"bigfloat":     functionValueOf("std/math.bigfloat", func(args []Value) (Value, error) { return builtinBigFloat(args) }),
			"hypot":        functionValueOf("std/math.hypot", func(args []Value) (Value, error) { return builtinHypot(args) }),
			"norm":         functionValueOf("std/math.norm", func(args []Value) (Value, error) { return builtinNorm(args) }),
			"vector":       functionValueOf("std/math.vector", func(args []Value) (Value, error) { return builtinVector(args) }),
			"vec":          functionValueOf("std/math.vec", func(args []Value) (Value, error) { return builtinVector(args) }),
			"shape":        functionValueOf("std/math.shape", func(args []Value) (Value, error) { return tensorShapeValue(args) }),
			"flatten":      functionValueOf("std/math.flatten", func(args []Value) (Value, error) { return tensorFlattenValue(args) }),
			"squeeze":      functionValueOf("std/math.squeeze", func(args []Value) (Value, error) { return tensorSqueezeValue(args) }),
			"expand_dims":  functionValueOf("std/math.expand_dims", func(args []Value) (Value, error) { return tensorExpandDimsValue(args) }),
			"concat":       functionValueOf("std/math.concat", func(args []Value) (Value, error) { return tensorConcatValue(args) }),
			"stack":        functionValueOf("std/math.stack", func(args []Value) (Value, error) { return tensorStackValue(args) }),
			"broadcast_to": functionValueOf("std/math.broadcast_to", func(args []Value) (Value, error) { return tensorBroadcastToValue(args) }),
			"where":        functionValueOf("std/math.where", func(args []Value) (Value, error) { return tensorWhereValue(args) }),
			"clip":         functionValueOf("std/math.clip", func(args []Value) (Value, error) { return tensorClipValue(args) }),
			"gather":       functionValueOf("std/math.gather", func(args []Value) (Value, error) { return tensorGatherValue(args) }),
			"scatter":      functionValueOf("std/math.scatter", func(args []Value) (Value, error) { return tensorScatterValue(args) }),
			"pad":          functionValueOf("std/math.pad", func(args []Value) (Value, error) { return tensorPadValue(args) }),
			"tile":         functionValueOf("std/math.tile", func(args []Value) (Value, error) { return tensorTileValue(args) }),
			"transpose":    functionValueOf("std/math.transpose", func(args []Value) (Value, error) { return tensorTransposeValue(args) }),
			"dot":          functionValueOf("std/math.dot", func(args []Value) (Value, error) { return tensorDotValue(args) }),
			"matmul":       functionValueOf("std/math.matmul", func(args []Value) (Value, error) { return tensorMatMulValue(args) }),
			"outer":        functionValueOf("std/math.outer", func(args []Value) (Value, error) { return outerValue(args) }),
			"trace":        functionValueOf("std/math.trace", func(args []Value) (Value, error) { return traceValue(args) }),
			"det":          functionValueOf("std/math.det", func(args []Value) (Value, error) { return detValue(args) }),
			"solve":        functionValueOf("std/math.solve", func(args []Value) (Value, error) { return solveValue(args) }),
			"inv":          functionValueOf("std/math.inv", func(args []Value) (Value, error) { return invValue(args) }),
			"qr":           functionValueOf("std/math.qr", func(args []Value) (Value, error) { return qrValue(args) }),
			"cholesky":     functionValueOf("std/math.cholesky", func(args []Value) (Value, error) { return choleskyValue(args) }),
			"eig":          functionValueOf("std/math.eig", func(args []Value) (Value, error) { return eigValue(args) }),
			"eigh":         functionValueOf("std/math.eigh", func(args []Value) (Value, error) { return eighValue(args) }),
			"svd":          functionValueOf("std/math.svd", func(args []Value) (Value, error) { return svdValue(args) }),
			"reshape":      functionValueOf("std/math.reshape", func(args []Value) (Value, error) { return tensorReshapeValue(args) }),
			"tensor":       functionValueOf("std/math.tensor", func(args []Value) (Value, error) { return tensorValueFromArgs(args) }),
			"matrix":       functionValueOf("std/math.matrix", func(args []Value) (Value, error) { return matrixValueFromArgs(args) }),
			"min":          functionValueOf("std/math.min", func(args []Value) (Value, error) { return builtinMin(args) }),
			"max":          functionValueOf("std/math.max", func(args []Value) (Value, error) { return builtinMax(args) }),
			"clamp":        functionValueOf("std/math.clamp", func(args []Value) (Value, error) { return builtinClamp(args) }),
			"floor":        functionValueOf("std/math.floor", func(args []Value) (Value, error) { return builtinFloor(args) }),
			"ceil":         functionValueOf("std/math.ceil", func(args []Value) (Value, error) { return builtinCeil(args) }),
			"round":        functionValueOf("std/math.round", func(args []Value) (Value, error) { return builtinRound(args) }),
			"sqrt":         functionValueOf("std/math.sqrt", func(args []Value) (Value, error) { return builtinSqrt(args) }),
			"sign":         functionValueOf("std/math.sign", signValue),
			"step":         functionValueOf("std/math.step", stepValue),
			"heaviside":    functionValueOf("std/math.heaviside", heavisideValue),
			"relu":         functionValueOf("std/math.relu", reluValue),
			"sigmoid":      functionValueOf("std/math.sigmoid", sigmoidValue),
			"softplus":     functionValueOf("std/math.softplus", softplusValue),
			"mean":         functionValueOf("std/math.mean", meanValue),
			"var":          functionValueOf("std/math.var", varValue),
			"std":          functionValueOf("std/math.std", stdValue),
			"median":       functionValueOf("std/math.median", medianValue),
			"quantile":     functionValueOf("std/math.quantile", quantileValue),
			"percentile":   functionValueOf("std/math.percentile", percentileValue),
			"argmin":       functionValueOf("std/math.argmin", argminValue),
			"argmax":       functionValueOf("std/math.argmax", argmaxValue),
			"any":          functionValueOf("std/math.any", anyValue),
			"all":          functionValueOf("std/math.all", allValue),
			"cumsum":       functionValueOf("std/math.cumsum", cumsumValue),
			"cumprod":      functionValueOf("std/math.cumprod", cumprodValue),
		}), nil
	case "std/crypto":
		version := "1.0.0"
		if !versionMatchesSelector(version, selector) {
			return nullValue(), fmt.Errorf("stdlib module %s version %s does not satisfy %q", name, version, selector)
		}
		return makeModule(name, version, map[string]Value{
			"hash":  functionValueOf("std/crypto.hash", func(args []Value) (Value, error) { return builtinHash(args) }),
			"mix":   functionValueOf("std/crypto.mix", func(args []Value) (Value, error) { return builtinMix(args) }),
			"noise": functionValueOf("std/crypto.noise", func(args []Value) (Value, error) { return builtinNoise(args) }),
		}), nil
	case "std/list":
		version := "1.0.0"
		if !versionMatchesSelector(version, selector) {
			return nullValue(), fmt.Errorf("stdlib module %s version %s does not satisfy %q", name, version, selector)
		}
		return makeModule(name, version, map[string]Value{
			"len":     functionValueOf("std/list.len", func(args []Value) (Value, error) { return builtinLen(args) }),
			"range":   functionValueOf("std/list.range", func(args []Value) (Value, error) { return builtinRange(args) }),
			"take":    functionValueOf("std/list.take", func(args []Value) (Value, error) { return builtinTake(args) }),
			"sum":     functionValueOf("std/list.sum", func(args []Value) (Value, error) { return builtinSum(args) }),
			"product": functionValueOf("std/list.product", func(args []Value) (Value, error) { return builtinProduct(args) }),
		}), nil
	default:
		return nullValue(), fmt.Errorf("unknown stdlib module %s", name)
	}
}

// importCycleError handles the import cycle error step in the surrounding
// package and returns error.
//
// It returns an error when the operation cannot complete cleanly.
func importCycleError(stack []string, key string) error {
	idx := -1
	for i, seen := range stack {
		if seen == key {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("import cycle detected at %s", key)
	}
	chain := append([]string(nil), stack[idx:]...)
	chain = append(chain, key)
	return fmt.Errorf("import cycle: %s", strings.Join(chain, " -> "))
}

// makeModule handles the make module step in the surrounding package and
// returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func makeModule(name, version string, exports map[string]Value) Value {
	return moduleValueOf(name, version, func(ctx context.Context, member string) (Value, error) {
		if v, ok := exports[member]; ok {
			return v, nil
		}
		return nullValue(), fmt.Errorf("module %s has no member %q", name, member)
	}, exports)
}

// splitModuleRef handles the split module ref step in the surrounding package
// and returns string, string.
//
// It returns string, string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func splitModuleRef(ref string) (string, string) {
	i := strings.LastIndex(ref, "@")
	if i <= 0 || i == len(ref)-1 {
		return ref, ""
	}
	suffix := ref[i+1:]
	if !looksLikeSemverSelector(suffix) {
		return ref, ""
	}
	return ref[:i], suffix
}

// looksLikeSemverSelector handles the looks like semver selector step in the
// surrounding package and returns bool.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func looksLikeSemverSelector(s string) bool {
	if s == "" {
		return false
	}
	if s[0] == '^' || s[0] == '~' {
		s = s[1:]
	}
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= '0' && c <= '9') || c == '.' {
			continue
		}
		return false
	}
	return true
}

// versionMatchesSelector handles the version matches selector step in the
// surrounding package and returns bool.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func versionMatchesSelector(version, selector string) bool {
	if selector == "" {
		return true
	}
	if selector[0] == '^' || selector[0] == '~' {
		selector = selector[1:]
	}
	if version == "" {
		return false
	}
	v, err := parseSemVersion(version)
	if err != nil {
		return false
	}
	return v.MatchesSelector(selector)
}

// builtinHash handles the builtin hash step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinHash(args []Value) (Value, error) { return stringValue(hashValue(args)), nil }

// builtinMix handles the builtin mix step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinMix(args []Value) (Value, error) { return mixValue(args), nil }

// builtinCat handles the builtin cat step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinCat(args []Value) (Value, error) { return catValue(args) }

// builtinUpper handles the builtin upper step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinUpper(args []Value) (Value, error) { return oneString(args, strings.ToUpper) }

// builtinLower handles the builtin lower step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinLower(args []Value) (Value, error) { return oneString(args, strings.ToLower) }

// builtinTrim handles the builtin trim step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinTrim(args []Value) (Value, error) { return oneString(args, strings.TrimSpace) }

// builtinReplace handles the builtin replace step in the surrounding package
// and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinReplace(args []Value) (Value, error) { return replaceValue(args) }

// builtinRepeat handles the builtin repeat step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinRepeat(args []Value) (Value, error) { return repeatValue(args) }

// builtinSplit handles the builtin split step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinSplit(args []Value) (Value, error) { return splitValue(args) }

// repeatBuiltin handles the repeat builtin step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func repeatBuiltin(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("repeat expects 2 arguments")
	}
	if args[0].kind == kindString {
		return repeatValue(args)
	}
	return tensorRepeatValue(args)
}

// splitBuiltin handles the split builtin step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func splitBuiltin(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("split expects 2 arguments")
	}
	if args[0].kind == kindString {
		return splitValue(args)
	}
	return tensorSplitValue(args)
}

// builtinChunks handles the builtin chunks step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinChunks(args []Value) (Value, error) { return chunksValue(args) }

// builtinJoin handles the builtin join step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinJoin(args []Value) (Value, error) { return joinValue(args) }

// builtinSlug handles the builtin slug step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinSlug(args []Value) (Value, error) { return slugValue(args) }

// builtinLen handles the builtin len step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinLen(args []Value) (Value, error) { return lenValue(args) }

// builtinRange handles the builtin range step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinRange(args []Value) (Value, error) { return rangeValue(args) }

// builtinTake handles the builtin take step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinTake(args []Value) (Value, error) { return takeValue(args) }

// builtinSum handles the builtin sum step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinSum(args []Value) (Value, error) { return sumValue(args) }

// builtinProduct handles the builtin product step in the surrounding package
// and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinProduct(args []Value) (Value, error) { return productValue(args) }

// builtinAbs handles the builtin abs step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinAbs(args []Value) (Value, error) { return absValue(args) }

// builtinComplex handles the builtin complex step in the surrounding package
// and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinComplex(args []Value) (Value, error) { return complexValueFromArgs(args) }

// builtinReal handles the builtin real step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinReal(args []Value) (Value, error) { return realValue(args) }

// builtinImag handles the builtin imag step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinImag(args []Value) (Value, error) { return imagValue(args) }

// builtinMagnitude handles the builtin magnitude step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinMagnitude(args []Value) (Value, error) { return magnitudeValue(args) }

// builtinMin handles the builtin min step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinMin(args []Value) (Value, error) { return minValue(args) }

// builtinMax handles the builtin max step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinMax(args []Value) (Value, error) { return maxValue(args) }

// builtinClamp handles the builtin clamp step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinClamp(args []Value) (Value, error) { return clampValue(args) }

// builtinFloor handles the builtin floor step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinFloor(args []Value) (Value, error) { return roundUnary(args, math.Floor) }

// builtinCeil handles the builtin ceil step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinCeil(args []Value) (Value, error) { return roundUnary(args, math.Ceil) }

// builtinRound handles the builtin round step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinRound(args []Value) (Value, error) { return roundUnary(args, math.Round) }

// builtinSqrt handles the builtin sqrt step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinSqrt(args []Value) (Value, error) { return sqrtValue(args) }

// builtinPow handles the builtin pow step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinPow(args []Value) (Value, error) { return powValue(args) }

// builtinExpt handles the builtin expt step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinExpt(args []Value) (Value, error) { return powValue(args) }

// builtinExp handles the builtin exp step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinExp(args []Value) (Value, error) { return expValue(args) }

// builtinLog handles the builtin log step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinLog(args []Value) (Value, error) { return logValue(args) }

// builtinLog10 handles the builtin log10 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinLog10(args []Value) (Value, error) { return log10Value(args) }

// builtinLog2 handles the builtin log2 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinLog2(args []Value) (Value, error) { return log2Value(args) }

// builtinI8 handles the builtin i8 step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinI8(args []Value) (Value, error) { return castSignedWidthValue(args, 8) }

// builtinI16 handles the builtin i16 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinI16(args []Value) (Value, error) { return castSignedWidthValue(args, 16) }

// builtinI32 handles the builtin i32 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinI32(args []Value) (Value, error) { return castSignedWidthValue(args, 32) }

// builtinI64 handles the builtin i64 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinI64(args []Value) (Value, error) { return castSignedWidthValue(args, 64) }

// builtinI128 handles the builtin i128 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinI128(args []Value) (Value, error) { return castSignedWidthValue(args, 128) }

// builtinI256 handles the builtin i256 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinI256(args []Value) (Value, error) { return castSignedWidthValue(args, 256) }

// builtinI512 handles the builtin i512 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinI512(args []Value) (Value, error) { return castSignedWidthValue(args, 512) }

// builtinI1024 handles the builtin i1024 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinI1024(args []Value) (Value, error) { return castSignedWidthValue(args, 1024) }

// builtinU8 handles the builtin u8 step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinU8(args []Value) (Value, error) { return castUnsignedWidthValue(args, 8) }

// builtinU16 handles the builtin u16 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinU16(args []Value) (Value, error) { return castUnsignedWidthValue(args, 16) }

// builtinU32 handles the builtin u32 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinU32(args []Value) (Value, error) { return castUnsignedWidthValue(args, 32) }

// builtinU64 handles the builtin u64 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinU64(args []Value) (Value, error) { return castUnsignedWidthValue(args, 64) }

// builtinU128 handles the builtin u128 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinU128(args []Value) (Value, error) { return castUnsignedWidthValue(args, 128) }

// builtinU256 handles the builtin u256 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinU256(args []Value) (Value, error) { return castUnsignedWidthValue(args, 256) }

// builtinU512 handles the builtin u512 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinU512(args []Value) (Value, error) { return castUnsignedWidthValue(args, 512) }

// builtinU1024 handles the builtin u1024 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinU1024(args []Value) (Value, error) { return castUnsignedWidthValue(args, 1024) }

// builtinUint handles the builtin uint step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinUint(args []Value) (Value, error) { return castUnsignedWidthValue(args, 64) }

// builtinByte handles the builtin byte step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinByte(args []Value) (Value, error) { return castUnsignedWidthValue(args, 8) }

// builtinUF8 handles the builtin uf8 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinUF8(args []Value) (Value, error) { return castPackedUnsignedWidthValue(args, 8, false) }

// builtinUF16 handles the builtin uf16 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinUF16(args []Value) (Value, error) { return castPackedUnsignedWidthValue(args, 16, false) }

// builtinUF32 handles the builtin uf32 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinUF32(args []Value) (Value, error) { return castPackedUnsignedWidthValue(args, 32, false) }

// builtinUFNum handles the builtin ufnum step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinUFNum(args []Value) (Value, error) { return castPackedUnsignedWidthValue(args, 64, true) }

// builtinUF64 handles the builtin uf64 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinUF64(args []Value) (Value, error) { return castPackedUnsignedWidthValue(args, 64, false) }

// builtinUF128 handles the builtin uf128 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinUF128(args []Value) (Value, error) { return castPackedUnsignedWidthValue(args, 128, false) }

// builtinUF256 handles the builtin uf256 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinUF256(args []Value) (Value, error) { return castPackedUnsignedWidthValue(args, 256, false) }

// builtinUF512 handles the builtin uf512 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinUF512(args []Value) (Value, error) { return castPackedUnsignedWidthValue(args, 512, false) }

// builtinUF1024 handles the builtin uf1024 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinUF1024(args []Value) (Value, error) {
	return castPackedUnsignedWidthValue(args, 1024, false)
}

// builtinF32 handles the builtin f32 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinF32(args []Value) (Value, error) { return castFloatWidthValue(args, 32) }

// builtinF64 handles the builtin f64 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinF64(args []Value) (Value, error) { return castFloatWidthValue(args, 64) }

// builtinBF8 handles the builtin bf8 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinBF8(args []Value) (Value, error) { return castBFloatWidthValue(args, 8) }

// builtinBF16 handles the builtin bf16 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinBF16(args []Value) (Value, error) { return castBFloatWidthValue(args, 16) }

// builtinBF32 handles the builtin bf32 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinBF32(args []Value) (Value, error) { return castBFloatWidthValue(args, 32) }

// builtinBF64 handles the builtin bf64 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinBF64(args []Value) (Value, error) { return castBFloatWidthValue(args, 64) }

// builtinBF128 handles the builtin bf128 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinBF128(args []Value) (Value, error) { return castBFloatWidthValue(args, 128) }

// builtinBF256 handles the builtin bf256 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinBF256(args []Value) (Value, error) { return castBFloatWidthValue(args, 256) }

// builtinBF512 handles the builtin bf512 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinBF512(args []Value) (Value, error) { return castBFloatWidthValue(args, 512) }

// builtinBF1024 handles the builtin bf1024 step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinBF1024(args []Value) (Value, error) { return castBFloatWidthValue(args, 1024) }

// builtinBigInt handles the builtin big int step in the surrounding package
// and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinBigInt(args []Value) (Value, error) { return bigIntValueFromArgs(args) }

// builtinBigFloat handles the builtin big float step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinBigFloat(args []Value) (Value, error) { return bigFloatValueFromArgs(args) }

// builtinSin handles the builtin sin step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinSin(args []Value) (Value, error) { return mathUnaryValue(args, "sin", math.Sin, false) }

// builtinCos handles the builtin cos step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinCos(args []Value) (Value, error) { return mathUnaryValue(args, "cos", math.Cos, false) }

// builtinTan handles the builtin tan step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinTan(args []Value) (Value, error) { return mathUnaryValue(args, "tan", math.Tan, false) }

// builtinAsin handles the builtin asin step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinAsin(args []Value) (Value, error) { return mathUnaryValue(args, "asin", math.Asin, true) }

// builtinAcos handles the builtin acos step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinAcos(args []Value) (Value, error) { return mathUnaryValue(args, "acos", math.Acos, true) }

// builtinAtan handles the builtin atan step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinAtan(args []Value) (Value, error) { return mathUnaryValue(args, "atan", math.Atan, false) }

// builtinHypot handles the builtin hypot step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinHypot(args []Value) (Value, error) { return hypotValue(args) }

// builtinNorm handles the builtin norm step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinNorm(args []Value) (Value, error) { return normValue(args) }

// builtinVector handles the builtin vector step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinVector(args []Value) (Value, error) { return vectorValueFromArgs(args) }

// builtinNoise handles the builtin noise step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func builtinNoise(args []Value) (Value, error) { return noiseValue(args), nil }

// oneString handles the one string step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func oneString(args []Value, fn func(string) string) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("expected 1 argument")
	}
	s, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	return stringValue(fn(s)), nil
}

// catValue handles the cat value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func catValue(args []Value) (Value, error) {
	var b strings.Builder
	for _, arg := range args {
		b.WriteString(textify(arg))
	}
	return stringValue(b.String()), nil
}

// replaceValue handles the replace value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func replaceValue(args []Value) (Value, error) {
	if len(args) != 3 {
		return nullValue(), fmt.Errorf("replace expects 3 arguments")
	}
	s, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	old, err := args[1].AsString()
	if err != nil {
		return nullValue(), err
	}
	newv, err := args[2].AsString()
	if err != nil {
		return nullValue(), err
	}
	return stringValue(strings.ReplaceAll(s, old, newv)), nil
}

// repeatValue handles the repeat value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func repeatValue(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("repeat expects 2 arguments")
	}
	s, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	n, err := args[1].Int()
	if err != nil {
		return nullValue(), err
	}
	if n < 0 {
		return nullValue(), fmt.Errorf("repeat count must be non-negative")
	}
	return stringValue(strings.Repeat(s, n)), nil
}

// splitValue handles the split value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func splitValue(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("split expects 2 arguments")
	}
	s, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	sep, err := args[1].AsString()
	if err != nil {
		return nullValue(), err
	}
	parts := strings.Split(s, sep)
	out := make([]Value, len(parts))
	for i := range parts {
		out[i] = stringValue(parts[i])
	}
	return listValue(out), nil
}

// chunksValue handles the chunks value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func chunksValue(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("chunks expects 2 arguments")
	}
	s, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	n, err := args[1].Int()
	if err != nil {
		return nullValue(), err
	}
	if n <= 0 {
		return nullValue(), fmt.Errorf("chunk size must be positive")
	}
	runes := []rune(s)
	out := make([]Value, 0, (len(runes)+n-1)/n)
	for i := 0; i < len(runes); i += n {
		j := i + n
		if j > len(runes) {
			j = len(runes)
		}
		out = append(out, stringValue(string(runes[i:j])))
	}
	return listValue(out), nil
}

// joinValue handles the join value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func joinValue(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("join expects 2 arguments")
	}
	sep, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	items, err := args[1].AsList()
	if err != nil {
		return nullValue(), err
	}
	parts := make([]string, len(items))
	for i := range items {
		parts[i] = textify(items[i])
	}
	return stringValue(strings.Join(parts, sep)), nil
}

// slugValue handles the slug value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func slugValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("slug expects 1 argument")
	}
	s, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	s = strings.ToLower(s)
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}
	return stringValue(strings.Trim(b.String(), "-")), nil
}

// lenValue handles the len value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func lenValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("len expects 1 argument")
	}
	switch args[0].kind {
	case kindList:
		return numberValue(float64(len(args[0].list))), nil
	case kindTensor:
		return numberValue(float64(len(args[0].data))), nil
	case kindString:
		return numberValue(float64(len([]rune(args[0].str)))), nil
	default:
		return nullValue(), fmt.Errorf("len expects a string, list, or tensor")
	}
}

// rangeValue handles the range value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func rangeValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("range expects 1 argument")
	}
	n, err := args[0].Int()
	if err != nil {
		return nullValue(), err
	}
	if n < 0 {
		return nullValue(), fmt.Errorf("range count must be non-negative")
	}
	out := make([]Value, n)
	for i := range out {
		out[i] = numberValue(float64(i))
	}
	return listValue(out), nil
}

// takeValue handles the take value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func takeValue(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("take expects 2 arguments")
	}
	n, err := args[1].Int()
	if err != nil {
		return nullValue(), err
	}
	if n < 0 {
		return nullValue(), fmt.Errorf("take count must be non-negative")
	}
	switch args[0].kind {
	case kindString:
		runes := []rune(args[0].str)
		if n > len(runes) {
			n = len(runes)
		}
		return stringValue(string(runes[:n])), nil
	case kindList:
		if n > len(args[0].list) {
			n = len(args[0].list)
		}
		return listValue(append([]Value(nil), args[0].list[:n]...)), nil
	case kindTensor:
		if n > len(args[0].data) {
			n = len(args[0].data)
		}
		return newTensorValue([]int{n}, append([]Value(nil), args[0].data[:n]...)), nil
	default:
		return nullValue(), fmt.Errorf("take expects a string, list, or tensor")
	}
}

// sliceValue returns an owned half-open slice of a tensor axis.
func sliceValue(args []Value) (Value, error) {
	if len(args) != 3 && len(args) != 4 {
		return nullValue(), fmt.Errorf("slice expects 3 or 4 arguments")
	}
	if args[0].kind != kindTensor || len(args[0].shape) == 0 {
		return nullValue(), fmt.Errorf("slice expects a tensor with a leading dimension")
	}
	start, err := args[1].Int()
	if err != nil || start < 0 {
		return nullValue(), fmt.Errorf("slice start must be a non-negative integer")
	}
	end, err := args[2].Int()
	if err != nil || end < 0 {
		return nullValue(), fmt.Errorf("slice end must be a non-negative integer")
	}
	if start > end {
		return nullValue(), fmt.Errorf("slice end must not precede start")
	}
	axis := 0
	if len(args) == 4 {
		axis, err = args[3].Int()
		if err != nil || axis < 0 || axis >= len(args[0].shape) {
			return nullValue(), fmt.Errorf("slice axis out of range")
		}
	}
	limit := args[0].shape[axis]
	if start > limit {
		start = limit
	}
	if end > limit {
		end = limit
	}
	outerSize := tensorShapeProduct(args[0].shape[:axis])
	innerSize := tensorShapeProduct(args[0].shape[axis+1:])
	spanSize := end - start
	shape := append([]int(nil), args[0].shape...)
	shape[axis] = spanSize
	data := make([]Value, outerSize*spanSize*innerSize)
	for outer := 0; outer < outerSize; outer++ {
		sourceStart := (outer*limit + start) * innerSize
		destinationStart := outer * spanSize * innerSize
		copy(data[destinationStart:], args[0].data[sourceStart:sourceStart+spanSize*innerSize])
	}
	return newTensorValue(shape, data), nil
}

// sumValue handles the sum value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func sumValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("sum expects 1 argument")
	}
	items, err := args[0].AsList()
	if err != nil {
		return nullValue(), err
	}
	var total float64
	for _, item := range items {
		n, err := item.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		total += n
	}
	return floatResultValue(total, items...), nil
}

// productValue handles the product value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func productValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("product expects 1 argument")
	}
	items, err := args[0].AsList()
	if err != nil {
		return nullValue(), err
	}
	total := 1.0
	for _, item := range items {
		n, err := item.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		total *= n
	}
	return floatResultValue(total, items...), nil
}

// absValue handles the abs value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func absValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("abs expects 1 argument")
	}
	return magnitudeValue(args)
}

// minValue handles the min value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func minValue(args []Value) (Value, error) {
	return foldNumeric(args, func(best, next float64) float64 {
		if next < best {
			return next
		}
		return best
	})
}

// maxValue handles the max value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func maxValue(args []Value) (Value, error) {
	return foldNumeric(args, func(best, next float64) float64 {
		if next > best {
			return next
		}
		return best
	})
}

// clampValue handles the clamp value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func clampValue(args []Value) (Value, error) {
	if len(args) != 3 {
		return nullValue(), fmt.Errorf("clamp expects 3 arguments")
	}
	v, err := args[0].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	lo, err := args[1].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	hi, err := args[2].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	if v < lo {
		v = lo
	} else if v > hi {
		v = hi
	}
	return floatResultValue(v, args...), nil
}

// roundUnary handles the round unary step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func roundUnary(args []Value, fn func(float64) float64) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("expected 1 argument")
	}
	n, err := args[0].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	return floatValueStyled(fn(n), floatUnaryBits(args[0]), floatUnaryBFloat(args[0])), nil
}

// sqrtValue handles the sqrt value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func sqrtValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("sqrt expects 1 argument")
	}
	n, err := args[0].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	if n < 0 {
		return nullValue(), fmt.Errorf("sqrt domain error")
	}
	return floatValueStyled(math.Sqrt(n), floatUnaryBits(args[0]), floatUnaryBFloat(args[0])), nil
}

// powValue handles the pow value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func powValue(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("pow expects 2 arguments")
	}
	if out, ok, err := exactPowValue(args[0], args[1]); ok || err != nil {
		return out, err
	}
	a, err := args[0].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	b, err := args[1].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	return floatResultValue(math.Pow(a, b), args...), nil
}

// expValue handles the exp value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func expValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("exp expects 1 argument")
	}
	n, err := args[0].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	out := math.Exp(n)
	if math.IsNaN(out) || math.IsInf(out, 0) {
		return nullValue(), fmt.Errorf("exp overflow")
	}
	return floatValueStyled(out, floatUnaryBits(args[0]), floatUnaryBFloat(args[0])), nil
}

// logValue handles the log value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func logValue(args []Value) (Value, error) {
	switch len(args) {
	case 1:
		n, err := args[0].AsNumber()
		if err != nil {
			return nullValue(), err
		}
		if n <= 0 {
			return nullValue(), fmt.Errorf("log domain error")
		}
		return floatValueStyled(math.Log(n), floatUnaryBits(args[0]), floatUnaryBFloat(args[0])), nil
	case 2:
		n, err := args[0].AsNumber()
		if err != nil {
			return nullValue(), err
		}
		base, err := args[1].AsNumber()
		if err != nil {
			return nullValue(), err
		}
		if n <= 0 || base <= 0 || base == 1 {
			return nullValue(), fmt.Errorf("log domain error")
		}
		return floatResultValue(math.Log(n)/math.Log(base), args...), nil
	default:
		return nullValue(), fmt.Errorf("log expects 1 or 2 arguments")
	}
}

// log10Value handles the log10 value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func log10Value(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("log10 expects 1 argument")
	}
	n, err := args[0].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	if n <= 0 {
		return nullValue(), fmt.Errorf("log10 domain error")
	}
	return floatValueStyled(math.Log10(n), floatUnaryBits(args[0]), floatUnaryBFloat(args[0])), nil
}

// log2Value handles the log2 value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func log2Value(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("log2 expects 1 argument")
	}
	n, err := args[0].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	if n <= 0 {
		return nullValue(), fmt.Errorf("log2 domain error")
	}
	return floatValueStyled(math.Log2(n), floatUnaryBits(args[0]), floatUnaryBFloat(args[0])), nil
}

// mathUnaryValue handles the math unary value step in the surrounding package
// and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func mathUnaryValue(args []Value, name string, fn func(float64) float64, rejectInvalid bool) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("%s expects 1 argument", name)
	}
	n, err := args[0].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	out := fn(n)
	if rejectInvalid && (math.IsNaN(out) || math.IsInf(out, 0)) {
		return nullValue(), fmt.Errorf("%s domain error", name)
	}
	return floatValueStyled(out, floatUnaryBits(args[0]), floatUnaryBFloat(args[0])), nil
}

// hypotValue handles the hypot value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func hypotValue(args []Value) (Value, error) {
	if len(args) < 2 {
		return nullValue(), fmt.Errorf("hypot expects at least 2 arguments")
	}
	var total float64
	for _, arg := range args {
		n, err := arg.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		total += n * n
	}
	return floatResultValue(math.Sqrt(total), args...), nil
}

// normValue handles the norm value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func normValue(args []Value) (Value, error) {
	if len(args) == 0 {
		return nullValue(), fmt.Errorf("norm expects at least 1 argument")
	}
	values := args
	if len(args) == 1 {
		switch args[0].kind {
		case kindList, kindTensor:
			var err error
			values, err = args[0].AsList()
			if err != nil {
				return nullValue(), err
			}
		}
	}
	var total float64
	for _, arg := range values {
		n, err := arg.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		total += n * n
	}
	return floatResultValue(math.Sqrt(total), args...), nil
}

// vectorValueFromArgs handles the vector value from args step in the
// surrounding package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func vectorValueFromArgs(args []Value) (Value, error) {
	switch len(args) {
	case 0:
		return newTensorValue([]int{0}, nil), nil
	case 1:
		if args[0].kind == kindList || args[0].kind == kindTensor {
			return tensorValueFromArgs(args)
		}
		return tensorFromShapeAndData([]int{1}, []Value{args[0]})
	default:
		return tensorFromShapeAndData([]int{len(args)}, append([]Value(nil), args...))
	}
}

// bigIntValueFromArgs handles the big int value from args step in the
// surrounding package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func bigIntValueFromArgs(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("bigint expects 1 argument")
	}
	switch args[0].kind {
	case kindString:
		if bi, ok := new(big.Int).SetString(strings.TrimSpace(args[0].str), 0); ok {
			return bigIntValue(bi), nil
		}
		return nullValue(), fmt.Errorf("invalid bigint literal")
	default:
		bi, err := bigIntFromValue(args[0])
		if err != nil {
			return nullValue(), err
		}
		return bigIntValue(bi), nil
	}
}

// bigFloatValueFromArgs handles the big float value from args step in the
// surrounding package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func bigFloatValueFromArgs(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("bigfloat expects 1 argument")
	}
	switch args[0].kind {
	case kindString:
		text := strings.TrimSpace(args[0].str)
		if text == "" {
			return nullValue(), fmt.Errorf("invalid bigfloat literal")
		}
		if bf, ok := new(big.Float).SetString(text); ok {
			return bigFloatValue(bf), nil
		}
		if rat, ok := new(big.Rat).SetString(text); ok {
			return bigFloatValue(new(big.Float).SetRat(rat)), nil
		}
		return nullValue(), fmt.Errorf("invalid bigfloat literal")
	default:
		bf, err := bigFloatFromValue(args[0])
		if err != nil {
			return nullValue(), err
		}
		return bigFloatValue(bf), nil
	}
}

// foldNumeric handles the fold numeric step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func foldNumeric(args []Value, fn func(best, next float64) float64) (Value, error) {
	if len(args) == 0 {
		return nullValue(), fmt.Errorf("expected at least 1 argument")
	}
	best, err := args[0].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	for _, arg := range args[1:] {
		next, err := arg.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		best = fn(best, next)
	}
	return floatResultValue(best, args...), nil
}

// mixValue handles the mix value step in the surrounding package and returns
// Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func mixValue(args []Value) Value {
	// 64-bit byte mixer with a bit-scrambled avalanche step.
	var state uint64 = 0xcbf29ce484222325
	for _, arg := range args {
		b := []byte(arg.String())
		for i := 0; i < len(b); i++ {
			state ^= uint64(b[i])
			state *= 0x100000001b3
			state = bitsRotate(state, 13) ^ (state >> 7)
		}
		state ^= 0xff
		state = bitsRotate(state, 17) + 0x9e3779b97f4a7c15
	}
	return stringValue(fmt.Sprintf("%016x", state))
}

// noiseValue handles the noise value step in the surrounding package and
// returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func noiseValue(args []Value) Value {
	var acc uint64 = 0x6a09e667f3bcc909
	for i, arg := range args {
		h := hashBytes([]byte(arg.String()))
		acc ^= h + uint64(i)*0x9e3779b97f4a7c15
		acc = bitsRotate(acc, 27) ^ (acc << 7)
	}
	return stringValue(fmt.Sprintf("%016x", acc))
}

// hashBytes handles the hash bytes step in the surrounding package and returns
// uint64.
//
// It returns uint64 so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func hashBytes(data []byte) uint64 {
	var h uint64 = 0xcbf29ce484222325
	for _, b := range data {
		h ^= uint64(b)
		h *= 0x100000001b3
		h = bitsRotate(h, 11) ^ (h >> 5)
	}
	return h
}

// bitsRotate handles the bits rotate step in the surrounding package and
// returns uint64.
//
// It returns uint64 so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func bitsRotate(v uint64, n uint) uint64 {
	return (v << n) | (v >> (64 - n))
}
