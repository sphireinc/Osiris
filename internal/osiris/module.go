package osiris

import (
	"context"
	"fmt"
	"math"
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
			"pi":    numberValue(math.Pi),
			"tau":   numberValue(math.Pi * 2),
			"abs":   functionValueOf("std/math.abs", func(args []Value) (Value, error) { return builtinAbs(args) }),
			"min":   functionValueOf("std/math.min", func(args []Value) (Value, error) { return builtinMin(args) }),
			"max":   functionValueOf("std/math.max", func(args []Value) (Value, error) { return builtinMax(args) }),
			"clamp": functionValueOf("std/math.clamp", func(args []Value) (Value, error) { return builtinClamp(args) }),
			"floor": functionValueOf("std/math.floor", func(args []Value) (Value, error) { return builtinFloor(args) }),
			"ceil":  functionValueOf("std/math.ceil", func(args []Value) (Value, error) { return builtinCeil(args) }),
			"round": functionValueOf("std/math.round", func(args []Value) (Value, error) { return builtinRound(args) }),
			"sqrt":  functionValueOf("std/math.sqrt", func(args []Value) (Value, error) { return builtinSqrt(args) }),
			"pow":   functionValueOf("std/math.pow", func(args []Value) (Value, error) { return builtinPow(args) }),
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
func builtinChunks(args []Value) (Value, error)  { return chunksValue(args) }
func builtinJoin(args []Value) (Value, error)    { return joinValue(args) }
func builtinSlug(args []Value) (Value, error)    { return slugValue(args) }
func builtinLen(args []Value) (Value, error)     { return lenValue(args) }
func builtinRange(args []Value) (Value, error)   { return rangeValue(args) }
func builtinTake(args []Value) (Value, error)    { return takeValue(args) }
func builtinSum(args []Value) (Value, error)     { return sumValue(args) }
func builtinProduct(args []Value) (Value, error) { return productValue(args) }
func builtinAbs(args []Value) (Value, error)     { return absValue(args) }
func builtinMin(args []Value) (Value, error)     { return minValue(args) }
func builtinMax(args []Value) (Value, error)     { return maxValue(args) }
func builtinClamp(args []Value) (Value, error)   { return clampValue(args) }
func builtinFloor(args []Value) (Value, error)   { return roundUnary(args, math.Floor) }
func builtinCeil(args []Value) (Value, error)    { return roundUnary(args, math.Ceil) }
func builtinRound(args []Value) (Value, error)   { return roundUnary(args, math.Round) }
func builtinSqrt(args []Value) (Value, error)    { return sqrtValue(args) }
func builtinPow(args []Value) (Value, error)     { return powValue(args) }
func builtinNoise(args []Value) (Value, error)   { return noiseValue(args), nil }

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
	case kindString:
		return numberValue(float64(len([]rune(args[0].str)))), nil
	default:
		return nullValue(), fmt.Errorf("len expects a string or list")
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
	default:
		return nullValue(), fmt.Errorf("take expects a string or list")
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
	return numberValue(total), nil
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
	return numberValue(total), nil
}

func absValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("abs expects 1 argument")
	}
	n, err := args[0].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	if n < 0 {
		n = -n
	}
	return numberValue(n), nil
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
	return numberValue(v), nil
}

func roundUnary(args []Value, fn func(float64) float64) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("expected 1 argument")
	}
	n, err := args[0].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	return numberValue(fn(n)), nil
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
	return numberValue(math.Sqrt(n)), nil
}

func powValue(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("pow expects 2 arguments")
	}
	a, err := args[0].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	b, err := args[1].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	return numberValue(math.Pow(a, b)), nil
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
	return numberValue(best), nil
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
