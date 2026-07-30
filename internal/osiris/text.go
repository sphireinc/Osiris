package osiris

import (
	"fmt"
	"math"
	"math/big"
	"strings"
	"unicode/utf8"
)

func textFailure(kind ErrorKind, format string, args ...any) error {
	return newOsirisError(kind, fmt.Sprintf(format, args...))
}

func textArity(name string, args []Value, want int) error {
	if len(args) == want {
		return nil
	}
	return textFailure(ErrorKindInvalidArgument, "%s expects %d arguments, got %d", name, want, len(args))
}

func textStringArgument(name string, args []Value, index int) (string, error) {
	if index < 0 || index >= len(args) {
		return "", textFailure(ErrorKindInvalidArgument, "%s is missing argument %d", name, index+1)
	}
	if args[index].kind != kindString {
		return "", textFailure(ErrorKindType, "%s argument %d expects string, got %s", name, index+1, args[index].Kind())
	}
	return args[index].str, nil
}

func textTwoStrings(name string, args []Value) (string, string, error) {
	if err := textArity(name, args, 2); err != nil {
		return "", "", err
	}
	first, err := textStringArgument(name, args, 0)
	if err != nil {
		return "", "", err
	}
	second, err := textStringArgument(name, args, 1)
	if err != nil {
		return "", "", err
	}
	return first, second, nil
}

func textThreeStrings(name string, args []Value) (string, string, string, error) {
	if err := textArity(name, args, 3); err != nil {
		return "", "", "", err
	}
	first, err := textStringArgument(name, args, 0)
	if err != nil {
		return "", "", "", err
	}
	second, err := textStringArgument(name, args, 1)
	if err != nil {
		return "", "", "", err
	}
	third, err := textStringArgument(name, args, 2)
	if err != nil {
		return "", "", "", err
	}
	return first, second, third, nil
}

func textLimitArgument(name string, args []Value, index int) (int, error) {
	if index < 0 || index >= len(args) {
		return 0, textFailure(ErrorKindInvalidArgument, "%s is missing argument %d", name, index+1)
	}
	value := args[index]
	var integer *big.Int
	switch value.kind {
	case kindInt:
		if value.i == nil {
			integer = new(big.Int)
		} else {
			integer = new(big.Int).Set(value.i)
		}
	case kindUint:
		if value.ub != nil {
			integer = new(big.Int).Set(value.ub)
		} else {
			integer = new(big.Int).SetUint64(value.u)
		}
	case kindBigInt:
		if value.bi == nil {
			integer = new(big.Int)
		} else {
			integer = new(big.Int).Set(value.bi)
		}
	case kindNumber:
		if math.IsNaN(value.num) || math.IsInf(value.num, 0) || math.Trunc(value.num) != value.num {
			return 0, textFailure(ErrorKindType, "%s argument %d expects an integer, got %s", name, index+1, value.Kind())
		}
		integer, _ = new(big.Float).SetFloat64(value.num).Int(nil)
	default:
		return 0, textFailure(ErrorKindType, "%s argument %d expects an integer, got %s", name, index+1, value.Kind())
	}

	maxHost := new(big.Int).SetUint64(uint64(^uint(0) >> 1))
	minHost := new(big.Int).Neg(new(big.Int).Add(new(big.Int).Set(maxHost), big.NewInt(1)))
	if integer.Cmp(minHost) < 0 || integer.Cmp(maxHost) > 0 {
		return 0, textFailure(ErrorKindOutOfRange, "%s argument %d exceeds the host allocation limit", name, index+1)
	}
	return int(integer.Int64()), nil
}

func textStringList(parts []string) Value {
	values := make([]Value, len(parts))
	for index, part := range parts {
		values[index] = stringValue(part)
	}
	return listValue(values)
}

func textIndexOption(text string, byteIndex int) Value {
	if byteIndex < 0 {
		return optionNoneValue()
	}
	return optionSomeValue(uintValue(uint64(utf8.RuneCountInString(text[:byteIndex])), 64))
}

func textUnaryTransformValue(name string, args []Value, transform func(string) string) (Value, error) {
	if err := textArity(name, args, 1); err != nil {
		return nullValue(), err
	}
	text, err := textStringArgument(name, args, 0)
	if err != nil {
		return nullValue(), err
	}
	return stringValue(transform(text)), nil
}

func textCompareValue(args []Value) (Value, error) {
	left, right, err := textTwoStrings("compare", args)
	if err != nil {
		return nullValue(), err
	}
	return intValue(big.NewInt(int64(strings.Compare(left, right))), 64), nil
}

func textEqualFoldValue(args []Value) (Value, error) {
	left, right, err := textTwoStrings("equal_fold", args)
	if err != nil {
		return nullValue(), err
	}
	return boolValue(strings.EqualFold(left, right)), nil
}

func textContainsValue(args []Value) (Value, error) {
	text, needle, err := textTwoStrings("contains", args)
	if err != nil {
		return nullValue(), err
	}
	return boolValue(strings.Contains(text, needle)), nil
}

func textContainsAnyValue(args []Value) (Value, error) {
	text, codePoints, err := textTwoStrings("contains_any", args)
	if err != nil {
		return nullValue(), err
	}
	return boolValue(strings.ContainsAny(text, codePoints)), nil
}

func textHasPrefixValue(args []Value) (Value, error) {
	text, prefix, err := textTwoStrings("has_prefix", args)
	if err != nil {
		return nullValue(), err
	}
	return boolValue(strings.HasPrefix(text, prefix)), nil
}

func textHasSuffixValue(args []Value) (Value, error) {
	text, suffix, err := textTwoStrings("has_suffix", args)
	if err != nil {
		return nullValue(), err
	}
	return boolValue(strings.HasSuffix(text, suffix)), nil
}

func textCountValue(args []Value) (Value, error) {
	text, needle, err := textTwoStrings("count", args)
	if err != nil {
		return nullValue(), err
	}
	return uintValue(uint64(strings.Count(text, needle)), 64), nil
}

func textIndexValue(args []Value) (Value, error) {
	text, needle, err := textTwoStrings("index", args)
	if err != nil {
		return nullValue(), err
	}
	return textIndexOption(text, strings.Index(text, needle)), nil
}

func textIndexAnyValue(args []Value) (Value, error) {
	text, codePoints, err := textTwoStrings("index_any", args)
	if err != nil {
		return nullValue(), err
	}
	return textIndexOption(text, strings.IndexAny(text, codePoints)), nil
}

func textLastIndexValue(args []Value) (Value, error) {
	text, needle, err := textTwoStrings("last_index", args)
	if err != nil {
		return nullValue(), err
	}
	return textIndexOption(text, strings.LastIndex(text, needle)), nil
}

func textLastIndexAnyValue(args []Value) (Value, error) {
	text, codePoints, err := textTwoStrings("last_index_any", args)
	if err != nil {
		return nullValue(), err
	}
	return textIndexOption(text, strings.LastIndexAny(text, codePoints)), nil
}

func textCutValue(args []Value) (Value, error) {
	text, separator, err := textTwoStrings("cut", args)
	if err != nil {
		return nullValue(), err
	}
	before, after, found := strings.Cut(text, separator)
	if !found {
		return optionNoneValue(), nil
	}
	return optionSomeValue(textStringList([]string{before, after})), nil
}

func textCutPrefixValue(args []Value) (Value, error) {
	text, prefix, err := textTwoStrings("cut_prefix", args)
	if err != nil {
		return nullValue(), err
	}
	after, found := strings.CutPrefix(text, prefix)
	if !found {
		return optionNoneValue(), nil
	}
	return optionSomeValue(stringValue(after)), nil
}

func textCutSuffixValue(args []Value) (Value, error) {
	text, suffix, err := textTwoStrings("cut_suffix", args)
	if err != nil {
		return nullValue(), err
	}
	before, found := strings.CutSuffix(text, suffix)
	if !found {
		return optionNoneValue(), nil
	}
	return optionSomeValue(stringValue(before)), nil
}

func textFieldsValue(args []Value) (Value, error) {
	if err := textArity("fields", args, 1); err != nil {
		return nullValue(), err
	}
	text, err := textStringArgument("fields", args, 0)
	if err != nil {
		return nullValue(), err
	}
	return textStringList(strings.Fields(text)), nil
}

func textSplitAfterValue(args []Value) (Value, error) {
	text, separator, err := textTwoStrings("split_after", args)
	if err != nil {
		return nullValue(), err
	}
	return textStringList(strings.SplitAfter(text, separator)), nil
}

func textSplitNValue(args []Value) (Value, error) {
	return textSplitLimitedValue("split_n", args, false)
}

func textSplitAfterNValue(args []Value) (Value, error) {
	return textSplitLimitedValue("split_after_n", args, true)
}

func textSplitLimitedValue(name string, args []Value, after bool) (Value, error) {
	if err := textArity(name, args, 3); err != nil {
		return nullValue(), err
	}
	text, err := textStringArgument(name, args, 0)
	if err != nil {
		return nullValue(), err
	}
	separator, err := textStringArgument(name, args, 1)
	if err != nil {
		return nullValue(), err
	}
	limit, err := textLimitArgument(name, args, 2)
	if err != nil {
		return nullValue(), err
	}
	if after {
		return textStringList(strings.SplitAfterN(text, separator, limit)), nil
	}
	return textStringList(strings.SplitN(text, separator, limit)), nil
}

func textTrimSpaceValue(args []Value) (Value, error) {
	if err := textArity("trim_space", args, 1); err != nil {
		return nullValue(), err
	}
	text, err := textStringArgument("trim_space", args, 0)
	if err != nil {
		return nullValue(), err
	}
	return stringValue(strings.TrimSpace(text)), nil
}

func textTrimLeftValue(args []Value) (Value, error) {
	text, codePoints, err := textTwoStrings("trim_left", args)
	if err != nil {
		return nullValue(), err
	}
	return stringValue(strings.TrimLeft(text, codePoints)), nil
}

func textTrimRightValue(args []Value) (Value, error) {
	text, codePoints, err := textTwoStrings("trim_right", args)
	if err != nil {
		return nullValue(), err
	}
	return stringValue(strings.TrimRight(text, codePoints)), nil
}

func textTrimPrefixValue(args []Value) (Value, error) {
	text, prefix, err := textTwoStrings("trim_prefix", args)
	if err != nil {
		return nullValue(), err
	}
	return stringValue(strings.TrimPrefix(text, prefix)), nil
}

func textTrimSuffixValue(args []Value) (Value, error) {
	text, suffix, err := textTwoStrings("trim_suffix", args)
	if err != nil {
		return nullValue(), err
	}
	return stringValue(strings.TrimSuffix(text, suffix)), nil
}

func textReplaceNValue(args []Value) (Value, error) {
	if err := textArity("replace_n", args, 4); err != nil {
		return nullValue(), err
	}
	text, err := textStringArgument("replace_n", args, 0)
	if err != nil {
		return nullValue(), err
	}
	old, err := textStringArgument("replace_n", args, 1)
	if err != nil {
		return nullValue(), err
	}
	replacement, err := textStringArgument("replace_n", args, 2)
	if err != nil {
		return nullValue(), err
	}
	limit, err := textLimitArgument("replace_n", args, 3)
	if err != nil {
		return nullValue(), err
	}

	if err := textReplacementSize("replace_n", text, old, replacement, limit); err != nil {
		return nullValue(), err
	}
	return stringValue(strings.Replace(text, old, replacement, limit)), nil
}

func textReplacementSize(name, text, old, replacement string, limit int) error {
	replacements := strings.Count(text, old)
	if limit >= 0 && replacements > limit {
		replacements = limit
	}
	if growth := len(replacement) - len(old); growth > 0 && replacements > (int(^uint(0)>>1)-len(text))/growth {
		return textFailure(ErrorKindOutOfRange, "%s result exceeds the host allocation limit", name)
	}
	return nil
}
