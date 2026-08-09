package osiris

import (
	"context"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ExtendedRegexVersion identifies the additive regex inspection and bounded
// operation API staged by extended standard library. It retains the stdlib RE2 engine.
const ExtendedRegexVersion = "1.1.0"

const (
	ExtendedRegexMaxPatternBytes = 16 << 10
	ExtendedRegexMaxInputBytes   = 8 << 20
	ExtendedRegexMaxMatches      = 100_000
	ExtendedRegexMaxOutputBytes  = 16 << 20
	ExtendedRegexMaxSplitParts   = 100_001
)

// ExtendedRegexIndex is a UTF-8 byte range in the original input.
type ExtendedRegexIndex struct {
	StartByte int
	EndByte   int
}

// ExtendedRegexNamedCapture describes one named or numbered capture.
type ExtendedRegexNamedCapture struct {
	Index     int
	Name      string
	Text      string
	StartByte int
	EndByte   int
}

// ExtendedRegexNamedMatch contains a complete match and named capture metadata.
type ExtendedRegexNamedMatch struct {
	Text      string
	StartByte int
	EndByte   int
	Captures  []ExtendedRegexNamedCapture
}

// ExtendedRegexInfo is deterministic, immutable pattern metadata.
type ExtendedRegexInfo struct {
	Pattern         string
	EngineVersion   string
	CaptureCount    int
	CaptureNames    []string
	LiteralPrefix   string
	LiteralComplete bool
}

// ExtendedCompiledRegex is a stdlib-extended inspection wrapper around the existing
// immutable, bounded RE2 compiler.
type ExtendedCompiledRegex struct {
	base *StdlibCompiledRegex
}

// ExtendedCompileRegex validates and compiles a pattern under stdlib limits.
func ExtendedCompileRegex(ctx context.Context, pattern string, limits StdlibRegexLimits) (*ExtendedCompiledRegex, error) {
	compiled, err := StdlibCompileRegex(ctx, pattern, limits)
	if err != nil {
		return nil, err
	}
	return &ExtendedCompiledRegex{base: compiled}, nil
}

// Info reports capture and literal-prefix metadata without inspecting input.
func (r *ExtendedCompiledRegex) Info() (ExtendedRegexInfo, error) {
	if r == nil || r.base == nil || r.base.re == nil {
		return ExtendedRegexInfo{}, fmt.Errorf("std/regex.pattern_info: nil compiled expression")
	}
	prefix, complete := r.base.re.LiteralPrefix()
	names := append([]string(nil), r.base.re.SubexpNames()[1:]...)
	return ExtendedRegexInfo{
		Pattern:         r.base.Pattern(),
		EngineVersion:   ExtendedRegexVersion,
		CaptureCount:    r.base.re.NumSubexp(),
		CaptureNames:    names,
		LiteralPrefix:   prefix,
		LiteralComplete: complete,
	}, nil
}

// FindIndexes returns bounded whole-match byte ranges.
func (r *ExtendedCompiledRegex) FindIndexes(ctx context.Context, input string) ([]ExtendedRegexIndex, error) {
	if r == nil || r.base == nil || r.base.re == nil {
		return nil, fmt.Errorf("std/regex.find_indices: nil compiled expression")
	}
	if err := r.base.checkInput("find_indices", input); err != nil {
		return nil, err
	}
	if err := extendedRegexCheckContext(ctx, "find_indices"); err != nil {
		return nil, err
	}
	indices := r.base.re.FindAllStringIndex(input, r.base.limits.MaxMatches+1)
	if len(indices) > r.base.limits.MaxMatches {
		return nil, stdlibLimitError("regex find_indices", "matches", int64(len(indices)), int64(r.base.limits.MaxMatches))
	}
	out := make([]ExtendedRegexIndex, len(indices))
	for i, index := range indices {
		if err := extendedRegexCheckContext(ctx, "find_indices"); err != nil {
			return nil, err
		}
		out[i] = ExtendedRegexIndex{StartByte: index[0], EndByte: index[1]}
	}
	return out, nil
}

// FindNamed returns bounded matches with deterministic capture names and
// UTF-8 byte offsets. Unnamed captures retain an empty Name.
func (r *ExtendedCompiledRegex) FindNamed(ctx context.Context, input string) ([]ExtendedRegexNamedMatch, error) {
	if r == nil || r.base == nil || r.base.re == nil {
		return nil, fmt.Errorf("std/regex.find_named: nil compiled expression")
	}
	if err := r.base.checkInput("find_named", input); err != nil {
		return nil, err
	}
	indices := r.base.re.FindAllStringSubmatchIndex(input, r.base.limits.MaxMatches+1)
	if len(indices) > r.base.limits.MaxMatches {
		return nil, stdlibLimitError("regex find_named", "matches", int64(len(indices)), int64(r.base.limits.MaxMatches))
	}
	names := r.base.re.SubexpNames()
	out := make([]ExtendedRegexNamedMatch, 0, len(indices))
	for _, index := range indices {
		if err := extendedRegexCheckContext(ctx, "find_named"); err != nil {
			return nil, err
		}
		match := ExtendedRegexNamedMatch{StartByte: index[0], EndByte: index[1], Text: input[index[0]:index[1]]}
		for capture := 1; capture < len(index)/2; capture++ {
			start, end := index[2*capture], index[2*capture+1]
			item := ExtendedRegexNamedCapture{Index: capture, Name: names[capture], StartByte: start, EndByte: end}
			if start >= 0 && end >= start {
				item.Text = input[start:end]
			}
			match.Captures = append(match.Captures, item)
		}
		out = append(out, match)
	}
	return out, nil
}

func extendedRegexCheckContext(ctx context.Context, operation string) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("std/regex.%s: %w", operation, ctx.Err())
	default:
		return nil
	}
}

// ExtendedRegexReplaceN replaces at most n matches. n < 0 means all bounded
// matches; n == 0 returns the original input and zero replacements.
func (r *ExtendedCompiledRegex) ReplaceN(ctx context.Context, input, replacement string, n int) (string, int, error) {
	if r == nil || r.base == nil || r.base.re == nil {
		return "", 0, fmt.Errorf("std/regex.replace_n: nil compiled expression")
	}
	if n == 0 {
		if err := r.base.checkInput("replace_n", input); err != nil {
			return "", 0, err
		}
		return input, 0, nil
	}
	if n > r.base.limits.MaxMatches {
		return "", 0, stdlibLimitError("regex replace_n", "requested matches", int64(n), int64(r.base.limits.MaxMatches))
	}
	if err := r.base.checkInput("replace_n", input); err != nil {
		return "", 0, err
	}
	indices := r.base.re.FindAllStringSubmatchIndex(input, r.base.limits.MaxMatches+1)
	if len(indices) > r.base.limits.MaxMatches {
		return "", 0, stdlibLimitError("regex replace_n", "matches", int64(len(indices)), int64(r.base.limits.MaxMatches))
	}
	if n > 0 && len(indices) > n {
		indices = indices[:n]
	}
	var builder strings.Builder
	position := 0
	for _, index := range indices {
		if err := extendedRegexCheckContext(ctx, "replace_n"); err != nil {
			return "", 0, err
		}
		builder.WriteString(input[position:index[0]])
		builder.Write(r.base.re.ExpandString(nil, replacement, input, index))
		if int64(builder.Len()) > r.base.limits.MaxOutputBytes {
			return "", 0, stdlibLimitError("regex replace_n", "output bytes", int64(builder.Len()), r.base.limits.MaxOutputBytes)
		}
		position = index[1]
	}
	builder.WriteString(input[position:])
	if int64(builder.Len()) > r.base.limits.MaxOutputBytes {
		return "", 0, stdlibLimitError("regex replace_n", "output bytes", int64(builder.Len()), r.base.limits.MaxOutputBytes)
	}
	return builder.String(), len(indices), nil
}

// ExtendedRegexSplitN splits into at most n parts. n must be positive.
func (r *ExtendedCompiledRegex) SplitN(ctx context.Context, input string, n int) ([]string, error) {
	if r == nil || r.base == nil || r.base.re == nil {
		return nil, fmt.Errorf("std/regex.split_n: nil compiled expression")
	}
	if n <= 0 {
		return nil, fmt.Errorf("std/regex.split_n: parts must be positive")
	}
	if n > r.base.limits.MaxSplitParts {
		return nil, stdlibLimitError("regex split_n", "requested parts", int64(n), int64(r.base.limits.MaxSplitParts))
	}
	if err := r.base.checkInput("split_n", input); err != nil {
		return nil, err
	}
	if n == 1 {
		return []string{input}, nil
	}
	indices := r.base.re.FindAllStringIndex(input, n-1)
	parts := make([]string, 0, len(indices)+1)
	position := 0
	for _, index := range indices {
		if err := extendedRegexCheckContext(ctx, "split_n"); err != nil {
			return nil, err
		}
		parts = append(parts, input[position:index[0]])
		position = index[1]
	}
	parts = append(parts, input[position:])
	return parts, nil
}

// ExtendedRegexQuoteMeta quotes all regexp metacharacters under the input and
// output bounds used by the additive API.
func ExtendedRegexQuoteMeta(ctx context.Context, input string, limits StdlibRegexLimits) (string, error) {
	limits = limits.normalized()
	if !utf8.ValidString(input) {
		return "", fmt.Errorf("std/regex.quote_meta: input is not valid UTF-8")
	}
	if int64(len(input)) > limits.MaxInputBytes {
		return "", stdlibLimitError("regex quote_meta", "input bytes", int64(len(input)), limits.MaxInputBytes)
	}
	if err := extendedRegexCheckContext(ctx, "quote_meta"); err != nil {
		return "", err
	}
	output := regexp.QuoteMeta(input)
	if int64(len(output)) > limits.MaxOutputBytes {
		return "", stdlibLimitError("regex quote_meta", "output bytes", int64(len(output)), limits.MaxOutputBytes)
	}
	return output, nil
}

func extendedRegexValue(ctx context.Context, args []Value, operation string, fn func(*ExtendedCompiledRegex, string) (Value, error)) (Value, error) {
	if len(args) < 2 {
		return nullValue(), fmt.Errorf("std/regex.%s expects pattern and input", operation)
	}
	pattern, err := taskStringArg(args[0], "regex pattern")
	if err != nil {
		return nullValue(), err
	}
	input, err := taskStringArg(args[1], "regex input")
	if err != nil {
		return nullValue(), err
	}
	compiled, err := ExtendedCompileRegex(ctx, pattern, StdlibRegexLimits{})
	if err != nil {
		return nullValue(), err
	}
	return fn(compiled, input)
}

func extendedRegexInt(value Value, name string) (int, error) {
	result, err := value.Int()
	if err != nil || result < 0 {
		return 0, fmt.Errorf("std/regex.%s must be a non-negative integer", name)
	}
	return result, nil
}

func extendedRegexIndexValue(index ExtendedRegexIndex) Value {
	return mapValueOf([]mapEntry{
		{key: stringValue("start_byte"), value: intValue(big.NewInt(int64(index.StartByte)), 64)},
		{key: stringValue("end_byte"), value: intValue(big.NewInt(int64(index.EndByte)), 64)},
	})
}

func extendedRegexModule() map[string]Value {
	limits := StdlibRegexLimits{}
	mod := map[string]Value{
		"version":           stringValue(ExtendedRegexVersion),
		"engine_version":    functionValueOf("std/regex.engine_version", func([]Value) (Value, error) { return stringValue(StdlibRegexVersion), nil }),
		"max_pattern_bytes": uintValue(ExtendedRegexMaxPatternBytes, 64),
		"max_input_bytes":   uintValue(ExtendedRegexMaxInputBytes, 64),
		"max_matches":       uintValue(ExtendedRegexMaxMatches, 64),
		"max_output_bytes":  uintValue(ExtendedRegexMaxOutputBytes, 64),
		"max_split_parts":   uintValue(ExtendedRegexMaxSplitParts, 64),
		"capture_count": contextFunctionValueOf("std/regex.capture_count", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("std/regex.capture_count expects pattern")
			}
			pattern, err := taskStringArg(args[0], "regex pattern")
			if err != nil {
				return nullValue(), err
			}
			compiled, err := ExtendedCompileRegex(ctx, pattern, limits)
			if err != nil {
				return nullValue(), err
			}
			info, err := compiled.Info()
			return intValue(big.NewInt(int64(info.CaptureCount)), 64), err
		}),
		"capture_names": contextFunctionValueOf("std/regex.capture_names", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("std/regex.capture_names expects pattern")
			}
			pattern, err := taskStringArg(args[0], "regex pattern")
			if err != nil {
				return nullValue(), err
			}
			compiled, err := ExtendedCompileRegex(ctx, pattern, limits)
			if err != nil {
				return nullValue(), err
			}
			info, err := compiled.Info()
			if err != nil {
				return nullValue(), err
			}
			names := make([]Value, len(info.CaptureNames))
			for i, name := range info.CaptureNames {
				names[i] = stringValue(name)
			}
			return listValue(names), nil
		}),
		"pattern_info": contextFunctionValueOf("std/regex.pattern_info", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("std/regex.pattern_info expects pattern")
			}
			pattern, err := taskStringArg(args[0], "regex pattern")
			if err != nil {
				return nullValue(), err
			}
			compiled, err := ExtendedCompileRegex(ctx, pattern, limits)
			if err != nil {
				return nullValue(), err
			}
			info, err := compiled.Info()
			if err != nil {
				return nullValue(), err
			}
			names := make([]Value, len(info.CaptureNames))
			for i, name := range info.CaptureNames {
				names[i] = stringValue(name)
			}
			return mapValueOf([]mapEntry{
				{key: stringValue("pattern"), value: stringValue(info.Pattern)},
				{key: stringValue("engine_version"), value: stringValue(info.EngineVersion)},
				{key: stringValue("capture_count"), value: intValue(big.NewInt(int64(info.CaptureCount)), 64)},
				{key: stringValue("capture_names"), value: listValue(names)},
				{key: stringValue("literal_prefix"), value: stringValue(info.LiteralPrefix)},
				{key: stringValue("literal_complete"), value: boolValue(info.LiteralComplete)},
			}), nil
		}),
		"find_indices": contextFunctionValueOf("std/regex.find_indices", func(ctx context.Context, args []Value) (Value, error) {
			return extendedRegexValue(ctx, args, "find_indices", func(compiled *ExtendedCompiledRegex, input string) (Value, error) {
				indices, err := compiled.FindIndexes(ctx, input)
				if err != nil {
					return nullValue(), err
				}
				out := make([]Value, len(indices))
				for i, index := range indices {
					out[i] = extendedRegexIndexValue(index)
				}
				return listValue(out), nil
			})
		}),
		"find_named": contextFunctionValueOf("std/regex.find_named", func(ctx context.Context, args []Value) (Value, error) {
			return extendedRegexValue(ctx, args, "find_named", func(compiled *ExtendedCompiledRegex, input string) (Value, error) {
				matches, err := compiled.FindNamed(ctx, input)
				if err != nil {
					return nullValue(), err
				}
				out := make([]Value, len(matches))
				for i, match := range matches {
					captures := make([]Value, len(match.Captures))
					for j, capture := range match.Captures {
						captures[j] = mapValueOf([]mapEntry{
							{key: stringValue("index"), value: intValue(big.NewInt(int64(capture.Index)), 64)},
							{key: stringValue("name"), value: stringValue(capture.Name)},
							{key: stringValue("text"), value: stringValue(capture.Text)},
							{key: stringValue("start_byte"), value: intValue(big.NewInt(int64(capture.StartByte)), 64)},
							{key: stringValue("end_byte"), value: intValue(big.NewInt(int64(capture.EndByte)), 64)},
						})
					}
					out[i] = mapValueOf([]mapEntry{
						{key: stringValue("text"), value: stringValue(match.Text)},
						{key: stringValue("start_byte"), value: intValue(big.NewInt(int64(match.StartByte)), 64)},
						{key: stringValue("end_byte"), value: intValue(big.NewInt(int64(match.EndByte)), 64)},
						{key: stringValue("captures"), value: listValue(captures)},
					})
				}
				return listValue(out), nil
			})
		}),
		"match_count": contextFunctionValueOf("std/regex.match_count", func(ctx context.Context, args []Value) (Value, error) {
			return extendedRegexValue(ctx, args, "match_count", func(compiled *ExtendedCompiledRegex, input string) (Value, error) {
				indices, err := compiled.FindIndexes(ctx, input)
				return uintValue(uint64(len(indices)), 64), err
			})
		}),
		"replace_first": contextFunctionValueOf("std/regex.replace_first", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 3 {
				return nullValue(), fmt.Errorf("std/regex.replace_first expects pattern, input, replacement")
			}
			return extendedRegexValue(ctx, args[:2], "replace_first", func(compiled *ExtendedCompiledRegex, input string) (Value, error) {
				replacement, err := taskStringArg(args[2], "regex replacement")
				if err != nil {
					return nullValue(), err
				}
				result, _, err := compiled.ReplaceN(ctx, input, replacement, 1)
				return stringValue(result), err
			})
		}),
		"replace_n": contextFunctionValueOf("std/regex.replace_n", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 4 {
				return nullValue(), fmt.Errorf("std/regex.replace_n expects pattern, input, replacement, count")
			}
			return extendedRegexValue(ctx, args[:2], "replace_n", func(compiled *ExtendedCompiledRegex, input string) (Value, error) {
				replacement, err := taskStringArg(args[2], "regex replacement")
				if err != nil {
					return nullValue(), err
				}
				count, err := extendedRegexInt(args[3], "replacement count")
				if err != nil {
					return nullValue(), err
				}
				result, _, err := compiled.ReplaceN(ctx, input, replacement, count)
				return stringValue(result), err
			})
		}),
		"split_n": contextFunctionValueOf("std/regex.split_n", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 3 {
				return nullValue(), fmt.Errorf("std/regex.split_n expects pattern, input, parts")
			}
			return extendedRegexValue(ctx, args[:2], "split_n", func(compiled *ExtendedCompiledRegex, input string) (Value, error) {
				parts, err := extendedRegexInt(args[2], "split parts")
				if err != nil {
					return nullValue(), err
				}
				values, err := compiled.SplitN(ctx, input, parts)
				if err != nil {
					return nullValue(), err
				}
				out := make([]Value, len(values))
				for i, value := range values {
					out[i] = stringValue(value)
				}
				return listValue(out), nil
			})
		}),
		"quote_meta": contextFunctionValueOf("std/regex.quote_meta", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("std/regex.quote_meta expects text")
			}
			input, err := taskStringArg(args[0], "regex text")
			if err != nil {
				return nullValue(), err
			}
			output, err := ExtendedRegexQuoteMeta(ctx, input, limits)
			return stringValue(output), err
		}),
	}
	mod["escape"] = mod["quote_meta"]
	return mod
}
