package osiris

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// StdlibRegexVersion is the stable syntax and Unicode contract for the
// stdlib regular-expression backend. It is RE2 syntax as implemented by
// the Go standard library, with Unicode UTF-8 input and linear-time matching.
const StdlibRegexVersion = "re2-go1-unicode-1"

const (
	defaultStdlibRegexPatternBytes = 16 << 10
	defaultStdlibRegexInputBytes   = 8 << 20
	defaultStdlibRegexMatches      = 100_000
	defaultStdlibRegexOutputBytes  = 16 << 20
	defaultStdlibRegexSplitParts   = 100_001
)

// StdlibRegexLimits bounds pattern, input, match, split, and replacement
// output work. Zero fields select the documented defaults.
type StdlibRegexLimits struct {
	MaxPatternBytes int64
	MaxInputBytes   int64
	MaxMatches      int
	MaxOutputBytes  int64
	MaxSplitParts   int
}

func (l StdlibRegexLimits) normalized() StdlibRegexLimits {
	if l.MaxPatternBytes <= 0 {
		l.MaxPatternBytes = defaultStdlibRegexPatternBytes
	}
	if l.MaxInputBytes <= 0 {
		l.MaxInputBytes = defaultStdlibRegexInputBytes
	}
	if l.MaxMatches <= 0 {
		l.MaxMatches = defaultStdlibRegexMatches
	}
	if l.MaxOutputBytes <= 0 {
		l.MaxOutputBytes = defaultStdlibRegexOutputBytes
	}
	if l.MaxSplitParts <= 0 {
		l.MaxSplitParts = defaultStdlibRegexSplitParts
	}
	return l
}

// StdlibRegexCapture is one numbered capture. StartByte and EndByte are
// UTF-8 byte offsets into the original input; -1/-1 denotes an unmatched
// optional capture.
type StdlibRegexCapture struct {
	Index     int
	Text      string
	StartByte int
	EndByte   int
}

// StdlibRegexMatch contains the whole match and captures in numbered order.
type StdlibRegexMatch struct {
	Text      string
	StartByte int
	EndByte   int
	Captures  []StdlibRegexCapture
}

// StdlibCompiledRegex is an immutable compiled RE2 expression. It does not
// own external resources and is safe for concurrent use.
type StdlibCompiledRegex struct {
	pattern string
	limits  StdlibRegexLimits
	re      *regexp.Regexp
}

// StdlibCompileRegex validates UTF-8 RE2 syntax and compiles pattern under
// limits. RE2 rejects constructs with potentially super-linear behavior;
// input, match, split, and output bounds still apply to every operation.
func StdlibCompileRegex(ctx context.Context, pattern string, limits StdlibRegexLimits) (*StdlibCompiledRegex, error) {
	if err := stdlibCheckContext(ctx, "regex compile"); err != nil {
		return nil, err
	}
	limits = limits.normalized()
	if !utf8.ValidString(pattern) {
		return nil, fmt.Errorf("regex compile: %w: pattern is not valid UTF-8", ErrStdlibInvalid)
	}
	if int64(len(pattern)) > limits.MaxPatternBytes {
		return nil, stdlibLimitError("regex compile", "pattern bytes", int64(len(pattern)), limits.MaxPatternBytes)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("regex compile: %w: %v", ErrStdlibInvalid, err)
	}
	return &StdlibCompiledRegex{pattern: pattern, limits: limits, re: re}, nil
}

// Version returns the stable syntax and Unicode version string.
func (r *StdlibCompiledRegex) Version() string { return StdlibRegexVersion }

// Pattern returns the original pattern.
func (r *StdlibCompiledRegex) Pattern() string {
	if r == nil {
		return ""
	}
	return r.pattern
}

func (r *StdlibCompiledRegex) checkInput(operation, input string) error {
	if r == nil || r.re == nil {
		return fmt.Errorf("regex %s: %w: nil compiled expression", operation, ErrStdlibInvalid)
	}
	if !utf8.ValidString(input) {
		return fmt.Errorf("regex %s: %w: input is not valid UTF-8", operation, ErrStdlibInvalid)
	}
	if int64(len(input)) > r.limits.MaxInputBytes {
		return stdlibLimitError("regex "+operation, "input bytes", int64(len(input)), r.limits.MaxInputBytes)
	}
	return nil
}

// Match reports whether the input contains a match. It checks
// cancellation before and after the bounded RE2 operation.
func (r *StdlibCompiledRegex) Match(ctx context.Context, input string) (bool, error) {
	if err := stdlibCheckContext(ctx, "regex match"); err != nil {
		return false, err
	}
	if err := r.checkInput("match", input); err != nil {
		return false, err
	}
	matched := r.re.MatchString(input)
	if err := stdlibCheckContext(ctx, "regex match"); err != nil {
		return false, err
	}
	return matched, nil
}

func (r *StdlibCompiledRegex) matches(ctx context.Context, input, operation string) ([]StdlibRegexMatch, error) {
	if err := stdlibCheckContext(ctx, "regex "+operation); err != nil {
		return nil, err
	}
	if err := r.checkInput(operation, input); err != nil {
		return nil, err
	}
	indices := r.re.FindAllStringSubmatchIndex(input, r.limits.MaxMatches+1)
	if len(indices) > r.limits.MaxMatches {
		return nil, stdlibLimitError("regex "+operation, "matches", int64(len(indices)), int64(r.limits.MaxMatches))
	}
	out := make([]StdlibRegexMatch, 0, len(indices))
	for _, index := range indices {
		if err := stdlibCheckContext(ctx, "regex "+operation); err != nil {
			return nil, err
		}
		m := StdlibRegexMatch{StartByte: index[0], EndByte: index[1], Text: input[index[0]:index[1]]}
		for capture := 0; capture < len(index)/2; capture++ {
			start, end := index[2*capture], index[2*capture+1]
			item := StdlibRegexCapture{Index: capture, StartByte: start, EndByte: end}
			if start >= 0 && end >= start {
				item.Text = input[start:end]
			}
			m.Captures = append(m.Captures, item)
		}
		out = append(out, m)
	}
	return out, nil
}

// Find returns bounded matches with numbered UTF-8 capture offsets.
func (r *StdlibCompiledRegex) Find(ctx context.Context, input string) ([]StdlibRegexMatch, error) {
	return r.matches(ctx, input, "find")
}

// Replace replaces matches using Go/RE2 $name and $1 expansion. The result
// is built incrementally and cannot exceed MaxOutputBytes. It returns the
// replacement count and does not mutate the compiled expression.
func (r *StdlibCompiledRegex) Replace(ctx context.Context, input, replacement string) (string, int, error) {
	if err := stdlibCheckContext(ctx, "regex replace"); err != nil {
		return "", 0, err
	}
	if err := r.checkInput("replace", input); err != nil {
		return "", 0, err
	}
	indices := r.re.FindAllStringSubmatchIndex(input, r.limits.MaxMatches+1)
	if len(indices) > r.limits.MaxMatches {
		return "", 0, stdlibLimitError("regex replace", "matches", int64(len(indices)), int64(r.limits.MaxMatches))
	}
	var b strings.Builder
	count := 0
	position := 0
	for _, index := range indices {
		if err := stdlibCheckContext(ctx, "regex replace"); err != nil {
			return "", 0, err
		}
		b.WriteString(input[position:index[0]])
		expanded := r.re.ExpandString(nil, replacement, input, index)
		b.Write(expanded)
		if int64(b.Len()) > r.limits.MaxOutputBytes {
			return "", 0, stdlibLimitError("regex replace", "output bytes", int64(b.Len()), r.limits.MaxOutputBytes)
		}
		position = index[1]
		count++
	}
	b.WriteString(input[position:])
	if int64(b.Len()) > r.limits.MaxOutputBytes {
		return "", 0, stdlibLimitError("regex replace", "output bytes", int64(b.Len()), r.limits.MaxOutputBytes)
	}
	return b.String(), count, nil
}

// Split splits at bounded matches. The result includes empty fields, follows
// regexp.FindAllStringIndex semantics, and is capped by MaxSplitParts.
func (r *StdlibCompiledRegex) Split(ctx context.Context, input string) ([]string, error) {
	if err := stdlibCheckContext(ctx, "regex split"); err != nil {
		return nil, err
	}
	if err := r.checkInput("split", input); err != nil {
		return nil, err
	}
	indices := r.re.FindAllStringIndex(input, r.limits.MaxSplitParts)
	if len(indices)+1 > r.limits.MaxSplitParts {
		return nil, stdlibLimitError("regex split", "split parts", int64(len(indices)+1), int64(r.limits.MaxSplitParts))
	}
	parts := make([]string, 0, len(indices)+1)
	position := 0
	for _, index := range indices {
		if err := stdlibCheckContext(ctx, "regex split"); err != nil {
			return nil, err
		}
		parts = append(parts, input[position:index[0]])
		position = index[1]
	}
	parts = append(parts, input[position:])
	return parts, nil
}
