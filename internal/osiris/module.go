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

func makeModule(name, version string, exports map[string]Value) Value {
	return moduleValueOf(name, version, func(ctx context.Context, member string) (Value, error) {
		if v, ok := exports[member]; ok {
			return v, nil
		}
		return nullValue(), fmt.Errorf("module %s has no member %q", name, member)
	}, exports)
}

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

func builtinHash(args []Value) (Value, error)    { return stringValue(hashValue(args)), nil }
func builtinMix(args []Value) (Value, error)     { return mixValue(args), nil }
func builtinCat(args []Value) (Value, error)     { return catValue(args) }
func builtinUpper(args []Value) (Value, error)   { return oneString(args, strings.ToUpper) }
func builtinLower(args []Value) (Value, error)   { return oneString(args, strings.ToLower) }
func builtinTrim(args []Value) (Value, error)    { return oneString(args, strings.TrimSpace) }
func builtinReplace(args []Value) (Value, error) { return replaceValue(args) }
func builtinRepeat(args []Value) (Value, error)  { return repeatValue(args) }
func builtinSplit(args []Value) (Value, error)   { return splitValue(args) }
func repeatBuiltin(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("repeat expects 2 arguments")
	}
	if args[0].kind == kindString {
		return repeatValue(args)
	}
	return tensorRepeatValue(args)
}
func splitBuiltin(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("split expects 2 arguments")
	}
	if args[0].kind == kindString {
		return splitValue(args)
	}
	return tensorSplitValue(args)
}
func builtinChunks(args []Value) (Value, error)    { return chunksValue(args) }
func builtinJoin(args []Value) (Value, error)      { return joinValue(args) }
func builtinSlug(args []Value) (Value, error)      { return slugValue(args) }
func builtinLen(args []Value) (Value, error)       { return lenValue(args) }
func builtinRange(args []Value) (Value, error)     { return rangeValue(args) }
func builtinTake(args []Value) (Value, error)      { return takeValue(args) }
func builtinSum(args []Value) (Value, error)       { return sumValue(args) }
func builtinProduct(args []Value) (Value, error)   { return productValue(args) }
func builtinAbs(args []Value) (Value, error)       { return absValue(args) }
func builtinComplex(args []Value) (Value, error)   { return complexValueFromArgs(args) }
func builtinReal(args []Value) (Value, error)      { return realValue(args) }
func builtinImag(args []Value) (Value, error)      { return imagValue(args) }
func builtinMagnitude(args []Value) (Value, error) { return magnitudeValue(args) }
func builtinMin(args []Value) (Value, error)       { return minValue(args) }
func builtinMax(args []Value) (Value, error)       { return maxValue(args) }
func builtinClamp(args []Value) (Value, error)     { return clampValue(args) }
func builtinFloor(args []Value) (Value, error)     { return roundUnary(args, math.Floor) }
func builtinCeil(args []Value) (Value, error)      { return roundUnary(args, math.Ceil) }
func builtinRound(args []Value) (Value, error)     { return roundUnary(args, math.Round) }
func builtinSqrt(args []Value) (Value, error)      { return sqrtValue(args) }
func builtinPow(args []Value) (Value, error)       { return powValue(args) }
func builtinExpt(args []Value) (Value, error)      { return powValue(args) }
func builtinExp(args []Value) (Value, error)       { return expValue(args) }
func builtinLog(args []Value) (Value, error)       { return logValue(args) }
func builtinLog10(args []Value) (Value, error)     { return log10Value(args) }
func builtinLog2(args []Value) (Value, error)      { return log2Value(args) }
func builtinI8(args []Value) (Value, error)        { return castSignedWidthValue(args, 8) }
func builtinI16(args []Value) (Value, error)       { return castSignedWidthValue(args, 16) }
func builtinI32(args []Value) (Value, error)       { return castSignedWidthValue(args, 32) }
func builtinI64(args []Value) (Value, error)       { return castSignedWidthValue(args, 64) }
func builtinI128(args []Value) (Value, error)      { return castSignedWidthValue(args, 128) }
func builtinI256(args []Value) (Value, error)      { return castSignedWidthValue(args, 256) }
func builtinI512(args []Value) (Value, error)      { return castSignedWidthValue(args, 512) }
func builtinI1024(args []Value) (Value, error)     { return castSignedWidthValue(args, 1024) }
func builtinU8(args []Value) (Value, error)        { return castUnsignedWidthValue(args, 8) }
func builtinU16(args []Value) (Value, error)       { return castUnsignedWidthValue(args, 16) }
func builtinU32(args []Value) (Value, error)       { return castUnsignedWidthValue(args, 32) }
func builtinU64(args []Value) (Value, error)       { return castUnsignedWidthValue(args, 64) }
func builtinU128(args []Value) (Value, error)      { return castUnsignedWidthValue(args, 128) }
func builtinU256(args []Value) (Value, error)      { return castUnsignedWidthValue(args, 256) }
func builtinU512(args []Value) (Value, error)      { return castUnsignedWidthValue(args, 512) }
func builtinU1024(args []Value) (Value, error)     { return castUnsignedWidthValue(args, 1024) }
func builtinUint(args []Value) (Value, error)      { return castUnsignedWidthValue(args, 64) }
func builtinByte(args []Value) (Value, error)      { return castUnsignedWidthValue(args, 8) }
func builtinUF8(args []Value) (Value, error)       { return castPackedUnsignedWidthValue(args, 8, false) }
func builtinUF16(args []Value) (Value, error)      { return castPackedUnsignedWidthValue(args, 16, false) }
func builtinUF32(args []Value) (Value, error)      { return castPackedUnsignedWidthValue(args, 32, false) }
func builtinUFNum(args []Value) (Value, error)     { return castPackedUnsignedWidthValue(args, 64, true) }
func builtinUF64(args []Value) (Value, error)      { return castPackedUnsignedWidthValue(args, 64, false) }
func builtinUF128(args []Value) (Value, error)     { return castPackedUnsignedWidthValue(args, 128, false) }
func builtinUF256(args []Value) (Value, error)     { return castPackedUnsignedWidthValue(args, 256, false) }
func builtinUF512(args []Value) (Value, error)     { return castPackedUnsignedWidthValue(args, 512, false) }
func builtinUF1024(args []Value) (Value, error) {
	return castPackedUnsignedWidthValue(args, 1024, false)
}
func builtinF32(args []Value) (Value, error)      { return castFloatWidthValue(args, 32) }
func builtinF64(args []Value) (Value, error)      { return castFloatWidthValue(args, 64) }
func builtinBF8(args []Value) (Value, error)      { return castBFloatWidthValue(args, 8) }
func builtinBF16(args []Value) (Value, error)     { return castBFloatWidthValue(args, 16) }
func builtinBF32(args []Value) (Value, error)     { return castBFloatWidthValue(args, 32) }
func builtinBF64(args []Value) (Value, error)     { return castBFloatWidthValue(args, 64) }
func builtinBF128(args []Value) (Value, error)    { return castBFloatWidthValue(args, 128) }
func builtinBF256(args []Value) (Value, error)    { return castBFloatWidthValue(args, 256) }
func builtinBF512(args []Value) (Value, error)    { return castBFloatWidthValue(args, 512) }
func builtinBF1024(args []Value) (Value, error)   { return castBFloatWidthValue(args, 1024) }
func builtinBigInt(args []Value) (Value, error)   { return bigIntValueFromArgs(args) }
func builtinBigFloat(args []Value) (Value, error) { return bigFloatValueFromArgs(args) }
func builtinSin(args []Value) (Value, error)      { return mathUnaryValue(args, "sin", math.Sin, false) }
func builtinCos(args []Value) (Value, error)      { return mathUnaryValue(args, "cos", math.Cos, false) }
func builtinTan(args []Value) (Value, error)      { return mathUnaryValue(args, "tan", math.Tan, false) }
func builtinAsin(args []Value) (Value, error)     { return mathUnaryValue(args, "asin", math.Asin, true) }
func builtinAcos(args []Value) (Value, error)     { return mathUnaryValue(args, "acos", math.Acos, true) }
func builtinAtan(args []Value) (Value, error)     { return mathUnaryValue(args, "atan", math.Atan, false) }
func builtinHypot(args []Value) (Value, error)    { return hypotValue(args) }
func builtinNorm(args []Value) (Value, error)     { return normValue(args) }
func builtinVector(args []Value) (Value, error)   { return vectorValueFromArgs(args) }
func builtinNoise(args []Value) (Value, error)    { return noiseValue(args), nil }

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

func catValue(args []Value) (Value, error) {
	var b strings.Builder
	for _, arg := range args {
		b.WriteString(textify(arg))
	}
	return stringValue(b.String()), nil
}

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

func absValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("abs expects 1 argument")
	}
	return magnitudeValue(args)
}

func minValue(args []Value) (Value, error) {
	return foldNumeric(args, func(best, next float64) float64 {
		if next < best {
			return next
		}
		return best
	})
}

func maxValue(args []Value) (Value, error) {
	return foldNumeric(args, func(best, next float64) float64 {
		if next > best {
			return next
		}
		return best
	})
}

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

func noiseValue(args []Value) Value {
	var acc uint64 = 0x6a09e667f3bcc909
	for i, arg := range args {
		h := hashBytes([]byte(arg.String()))
		acc ^= h + uint64(i)*0x9e3779b97f4a7c15
		acc = bitsRotate(acc, 27) ^ (acc << 7)
	}
	return stringValue(fmt.Sprintf("%016x", acc))
}

func hashBytes(data []byte) uint64 {
	var h uint64 = 0xcbf29ce484222325
	for _, b := range data {
		h ^= uint64(b)
		h *= 0x100000001b3
		h = bitsRotate(h, 11) ^ (h >> 5)
	}
	return h
}

func bitsRotate(v uint64, n uint) uint64 {
	return (v << n) | (v >> (64 - n))
}
