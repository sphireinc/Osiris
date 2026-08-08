package osiris

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"math/bits"
	"math/cmplx"
	"strconv"
	"strings"
)

// maskForWidth handles the mask for width step in the surrounding package and
// returns uint64.
//
// It returns uint64 so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func maskForWidth(bitsWidth uint8) uint64 {
	if bitsWidth == 0 || bitsWidth >= 64 {
		return ^uint64(0)
	}
	return (uint64(1) << bitsWidth) - 1
}

// formatUint handles the format uint step in the surrounding package and
// returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func formatUint(v uint64, bitsWidth uint8) string {
	if bitsWidth == 0 || bitsWidth == 64 {
		return fmt.Sprintf("%du64", v)
	}
	return fmt.Sprintf("%du%d", v&maskForWidth(bitsWidth), bitsWidth)
}

// formatWideUint handles the format wide uint step in the surrounding package
// and returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func formatWideUint(v *big.Int, bitsWidth uint16) string {
	if bitsWidth == 0 {
		bitsWidth = 64
	}
	if v == nil {
		return fmt.Sprintf("0u%d", bitsWidth)
	}
	return fmt.Sprintf("%su%d", v.String(), bitsWidth)
}

// formatUintValue handles the format uint value step in the surrounding
// package and returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func formatUintValue(v Value) string {
	if v.packed {
		if v.native {
			if v.ub != nil {
				return fmt.Sprintf("%sufnum", v.ub.String())
			}
			return fmt.Sprintf("%dufnum", v.u)
		}
		if v.ub != nil {
			return fmt.Sprintf("%suf%d", v.ub.String(), v.ubitsToUint16())
		}
		return fmt.Sprintf("%duf%d", v.u, v.bits)
	}
	if v.kind == kindUint && v.ub != nil {
		return formatWideUint(v.ub, v.ubitsToUint16())
	}
	return formatUint(v.u, v.bits)
}

// isWideUintValue handles the is wide uint value step in the surrounding
// package and returns bool.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func isWideUintValue(v Value) bool {
	return v.kind == kindUint && v.ub != nil
}

// unsignedMask handles the unsigned mask step in the surrounding package and
// returns *big.Int.
//
// It returns *big.Int so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func unsignedMask(bitsWidth uint16) *big.Int {
	if bitsWidth == 0 {
		bitsWidth = 64
	}
	if bitsWidth >= 1024 {
		// Keep the function safe for the requested width family while avoiding
		// pathological allocations on nonsense widths.
		return new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), uint(bitsWidth)), big.NewInt(1))
	}
	return new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), uint(bitsWidth)), big.NewInt(1))
}

// uintValueFromBig handles the uint value from big step in the surrounding
// package and returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func uintValueFromBig(v *big.Int, bitsWidth uint16) Value {
	return uintValueFromBigStyle(v, bitsWidth, unsignedStyleFromValues())
}

// uintValueFromBigStyle handles the uint value from big style step in the
// surrounding package and returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func uintValueFromBigStyle(v *big.Int, bitsWidth uint16, style unsignedStyle) Value {
	if bitsWidth == 0 {
		bitsWidth = 64
	}
	wrapped := wrapUnsignedWidth(v, bitsWidth)
	if bitsWidth <= 64 {
		if style.packed {
			return packedUintValue(wrapped.Uint64(), uint8(bitsWidth), style.native)
		}
		return uintValue(wrapped.Uint64(), uint8(bitsWidth))
	}
	if style.packed {
		return packedWideUintValue(wrapped, bitsWidth, style.native)
	}
	return wideUintValue(wrapped, bitsWidth)
}

type unsignedStyle struct {
	packed bool
	native bool
}

// unsignedStyleFromValues handles the unsigned style from values step in the
// surrounding package and returns unsignedStyle.
//
// It returns unsignedStyle so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func unsignedStyleFromValues(values ...Value) unsignedStyle {
	var style unsignedStyle
	for _, v := range values {
		if v.kind != kindUint || !v.packed {
			continue
		}
		style.packed = true
		if style.native || v.native {
			style.native = style.native || v.native
		}
		if !v.native {
			style.native = false
		}
	}
	return style
}

// equalUnsignedValues handles the equal unsigned values step in the
// surrounding package and returns bool, error.
//
// It returns bool and an error. Callers should only trust the value when the
// error is nil.
func equalUnsignedValues(left, right Value) (bool, error) {
	if left.kind != kindUint || right.kind != kindUint {
		return false, fmt.Errorf("expected unsigned integers")
	}
	if left.ub != nil || right.ub != nil {
		lu, lw, err := unsignedIntFromValue(left)
		if err != nil {
			return false, err
		}
		ru, rw, err := unsignedIntFromValue(right)
		if err != nil {
			return false, err
		}
		return lw == rw && lu.Cmp(ru) == 0, nil
	}
	return left.u == right.u && left.bits == right.bits, nil
}

// compareUnsignedValues handles the compare unsigned values step in the
// surrounding package and returns int, error.
//
// It returns int and an error. Callers should only trust the value when the
// error is nil.
func compareUnsignedValues(left, right Value) (int, error) {
	lu, _, err := unsignedIntFromValue(left)
	if err != nil {
		return 0, err
	}
	ru, _, err := unsignedIntFromValue(right)
	if err != nil {
		return 0, err
	}
	return lu.Cmp(ru), nil
}

// parseNumericLiteral handles the parse numeric literal step in the
// surrounding package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func parseNumericLiteral(text string) (Value, error) {
	cleaned := strings.ReplaceAll(text, "_", "")
	if cleaned == "" {
		return nullValue(), fmt.Errorf("invalid numeric literal %q", text)
	}

	if base, ok := parseComplexSuffix(cleaned); ok {
		scalar, err := parseScalarNumericLiteral(base)
		if err != nil {
			return nullValue(), err
		}
		n, err := scalar.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		return complexValueStyled(complex(0, n), complexComponentBits(scalar)), nil
	}

	return parseScalarNumericLiteral(cleaned)
}

// parseScalarNumericLiteral handles the parse scalar numeric literal step in
// the surrounding package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func parseScalarNumericLiteral(cleaned string) (Value, error) {
	suffixBits, stripped := parseWidthSuffix(cleaned)
	if baseText, bits, ok := parseSignedSuffix(stripped); ok {
		digits := baseText
		base := 10
		switch {
		case strings.HasPrefix(baseText, "0x") || strings.HasPrefix(baseText, "0X"):
			base = 16
			digits = baseText[2:]
		case strings.HasPrefix(baseText, "0b") || strings.HasPrefix(baseText, "0B"):
			base = 2
			digits = baseText[2:]
		case strings.HasPrefix(baseText, "0o") || strings.HasPrefix(baseText, "0O"):
			base = 8
			digits = baseText[2:]
		}
		if digits == "" {
			return nullValue(), fmt.Errorf("invalid numeric literal %q", cleaned)
		}
		i, ok := new(big.Int).SetString(digits, base)
		if !ok {
			return nullValue(), fmt.Errorf("invalid numeric literal %q", cleaned)
		}
		return intValue(i, bits), nil
	}
	if baseText, ok := parseByteSuffix(stripped); ok {
		digits := baseText
		base := 10
		switch {
		case strings.HasPrefix(baseText, "0x") || strings.HasPrefix(baseText, "0X"):
			base = 16
			digits = baseText[2:]
		case strings.HasPrefix(baseText, "0b") || strings.HasPrefix(baseText, "0B"):
			base = 2
			digits = baseText[2:]
		case strings.HasPrefix(baseText, "0o") || strings.HasPrefix(baseText, "0O"):
			base = 8
			digits = baseText[2:]
		}
		if digits == "" {
			return nullValue(), fmt.Errorf("invalid numeric literal %q", cleaned)
		}
		u, err := strconv.ParseUint(digits, base, 64)
		if err != nil {
			return nullValue(), fmt.Errorf("invalid numeric literal %q", cleaned)
		}
		return uintValue(u, 8), nil
	}
	if baseText, bits, packed, native := parsePackedUnsignedWidthSuffix(stripped); packed {
		digits := baseText
		base := 10
		switch {
		case strings.HasPrefix(baseText, "0x") || strings.HasPrefix(baseText, "0X"):
			base = 16
			digits = baseText[2:]
		case strings.HasPrefix(baseText, "0b") || strings.HasPrefix(baseText, "0B"):
			base = 2
			digits = baseText[2:]
		case strings.HasPrefix(baseText, "0o") || strings.HasPrefix(baseText, "0O"):
			base = 8
			digits = baseText[2:]
		}
		if digits == "" {
			return nullValue(), fmt.Errorf("invalid numeric literal %q", cleaned)
		}
		u, err := strconv.ParseUint(digits, base, 64)
		if err != nil {
			if bi, ok := new(big.Int).SetString(digits, base); ok {
				if bits <= 64 {
					return packedUintValue(bi.Uint64(), uint8(bits), native), nil
				}
				return packedWideUintValue(bi, bits, native), nil
			}
			return nullValue(), fmt.Errorf("invalid numeric literal %q", cleaned)
		}
		if bits <= 64 {
			return packedUintValue(u, uint8(bits), native), nil
		}
		return packedWideUintValue(new(big.Int).SetUint64(u), bits, native), nil
	}
	if baseText, bits, ok := parseBFloatWidthSuffix(stripped); ok {
		n, err := strconv.ParseFloat(baseText, 64)
		if err != nil {
			return nullValue(), fmt.Errorf("invalid numeric literal %q", cleaned)
		}
		return bfloatValue(n, bits), nil
	}
	if baseText, bits, ok := parseUnsignedWidthSuffix(stripped); ok {
		digits := baseText
		base := 10
		switch {
		case strings.HasPrefix(baseText, "0x") || strings.HasPrefix(baseText, "0X"):
			base = 16
			digits = baseText[2:]
		case strings.HasPrefix(baseText, "0b") || strings.HasPrefix(baseText, "0B"):
			base = 2
			digits = baseText[2:]
		case strings.HasPrefix(baseText, "0o") || strings.HasPrefix(baseText, "0O"):
			base = 8
			digits = baseText[2:]
		}
		if digits == "" {
			return nullValue(), fmt.Errorf("invalid numeric literal %q", cleaned)
		}
		u, err := strconv.ParseUint(digits, base, 64)
		if err != nil {
			if bi, ok := new(big.Int).SetString(digits, base); ok {
				if bits <= 64 {
					return uintValue(bi.Uint64(), uint8(bits)), nil
				}
				return wideUintValue(bi, bits), nil
			}
			return nullValue(), fmt.Errorf("invalid numeric literal %q", cleaned)
		}
		if bits <= 64 {
			return uintValue(u, uint8(bits)), nil
		}
		return wideUintValue(new(big.Int).SetUint64(u), bits), nil
	}
	if baseText, bits, ok := parseFloatWidthSuffix(stripped); ok {
		n, err := strconv.ParseFloat(baseText, 64)
		if err != nil {
			return nullValue(), fmt.Errorf("invalid numeric literal %q", cleaned)
		}
		return floatValue(n, bits), nil
	}
	if decText, ok := parseDecimalSuffix(stripped); ok {
		rat, err := parseDecimalText(decText)
		if err != nil {
			return nullValue(), fmt.Errorf("invalid decimal literal %q", cleaned)
		}
		return decimalValue(rat), nil
	}
	if isFloatLiteral(stripped) {
		n, err := strconv.ParseFloat(stripped, 64)
		if err != nil {
			return nullValue(), fmt.Errorf("invalid numeric literal %q", cleaned)
		}
		return numberValue(n), nil
	}

	base := 10
	digits := stripped
	switch {
	case strings.HasPrefix(stripped, "0x") || strings.HasPrefix(stripped, "0X"):
		base = 16
		digits = stripped[2:]
	case strings.HasPrefix(stripped, "0b") || strings.HasPrefix(stripped, "0B"):
		base = 2
		digits = stripped[2:]
	case strings.HasPrefix(stripped, "0o") || strings.HasPrefix(stripped, "0O"):
		base = 8
		digits = stripped[2:]
	}
	if digits == "" {
		return nullValue(), fmt.Errorf("invalid numeric literal %q", cleaned)
	}
	u, err := strconv.ParseUint(digits, base, 64)
	if err != nil {
		return nullValue(), fmt.Errorf("invalid numeric literal %q", cleaned)
	}
	return uintValue(u, suffixBits), nil
}

// parseFloatWidthSuffix handles the parse float width suffix step in the
// surrounding package and returns string, uint16, bool.
//
// It returns string, uint16, bool so callers can continue with the next stage
// of the pipeline without re-deriving the same information.
func parseFloatWidthSuffix(text string) (string, uint16, bool) {
	if len(text) < 3 {
		return "", 0, false
	}
	for _, width := range []string{"1024", "512", "256", "128", "64", "32", "16", "8"} {
		suffix := "f" + width
		if strings.HasSuffix(text, suffix) {
			n, err := strconv.Atoi(width)
			if err != nil {
				return "", 0, false
			}
			return text[:len(text)-len(suffix)], uint16(n), true
		}
	}
	return "", 0, false
}

// parseBFloatWidthSuffix handles the parse bfloat width suffix step in the
// surrounding package and returns string, uint16, bool.
//
// It returns string, uint16, bool so callers can continue with the next stage
// of the pipeline without re-deriving the same information.
func parseBFloatWidthSuffix(text string) (string, uint16, bool) {
	if len(text) < 4 {
		return "", 0, false
	}
	for _, width := range []string{"1024", "512", "256", "128", "64", "32", "16", "8"} {
		suffix := "bf" + width
		if strings.HasSuffix(text, suffix) {
			n, err := strconv.Atoi(width)
			if err != nil {
				return "", 0, false
			}
			return text[:len(text)-len(suffix)], uint16(n), true
		}
	}
	return "", 0, false
}

// parseUnsignedSuffix handles the parse unsigned suffix step in the
// surrounding package and returns string, bool.
//
// It returns string, bool so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func parseUnsignedSuffix(text string) (string, bool) {
	if len(text) == 0 {
		return "", false
	}
	last := text[len(text)-1]
	if (last == 'u' || last == 'U') && !strings.ContainsAny(text[:len(text)-1], ".eE") {
		return text[:len(text)-1], true
	}
	return "", false
}

// parseUnsignedWidthSuffix handles the parse unsigned width suffix step in the
// surrounding package and returns string, uint16, bool.
//
// It returns string, uint16, bool so callers can continue with the next stage
// of the pipeline without re-deriving the same information.
func parseUnsignedWidthSuffix(text string) (string, uint16, bool) {
	for _, width := range []string{"1024", "512", "256", "128", "64", "32", "16", "8"} {
		suffix := "u" + width
		if strings.HasSuffix(text, suffix) && !strings.ContainsAny(text[:len(text)-len(suffix)], ".eE") {
			n, err := strconv.Atoi(width)
			if err != nil {
				return "", 0, false
			}
			return text[:len(text)-len(suffix)], uint16(n), true
		}
	}
	if baseText, ok := parseUnsignedSuffix(text); ok {
		return baseText, 64, true
	}
	return "", 0, false
}

// parsePackedUnsignedWidthSuffix handles the parse packed unsigned width
// suffix step in the surrounding package and returns string, uint16, bool,
// bool.
//
// It returns string, uint16, bool, bool so callers can continue with the next
// stage of the pipeline without re-deriving the same information.
func parsePackedUnsignedWidthSuffix(text string) (string, uint16, bool, bool) {
	if strings.HasSuffix(text, "ufnum") {
		return text[:len(text)-len("ufnum")], 64, true, true
	}
	for _, width := range []string{"1024", "512", "256", "128", "64", "32", "16", "8"} {
		suffix := "uf" + width
		if strings.HasSuffix(text, suffix) && !strings.ContainsAny(text[:len(text)-len(suffix)], ".eE") {
			n, err := strconv.Atoi(width)
			if err != nil {
				return "", 0, false, false
			}
			return text[:len(text)-len(suffix)], uint16(n), true, false
		}
	}
	return "", 0, false, false
}

// parseSignedSuffix handles the parse signed suffix step in the surrounding
// package and returns string, uint16, bool.
//
// It returns string, uint16, bool so callers can continue with the next stage
// of the pipeline without re-deriving the same information.
func parseSignedSuffix(text string) (string, uint16, bool) {
	if len(text) < 2 {
		return "", 0, false
	}
	for _, width := range []string{"1024", "512", "256", "128", "64", "32", "16", "8"} {
		suffix := "i" + width
		if strings.HasSuffix(text, suffix) && !strings.ContainsAny(text[:len(text)-len(suffix)], ".eE") {
			n, err := strconv.Atoi(width)
			if err != nil {
				return "", 0, false
			}
			return text[:len(text)-len(suffix)], uint16(n), true
		}
	}
	return "", 0, false
}

// parseByteSuffix handles the parse byte suffix step in the surrounding
// package and returns string, bool.
//
// It returns string, bool so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func parseByteSuffix(text string) (string, bool) {
	if len(text) == 0 {
		return "", false
	}
	last := text[len(text)-1]
	if (last == 'b' || last == 'B') && !strings.ContainsAny(text[:len(text)-1], ".eE") {
		return text[:len(text)-1], true
	}
	return "", false
}

// parseComplexSuffix handles the parse complex suffix step in the surrounding
// package and returns string, bool.
//
// It returns string, bool so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func parseComplexSuffix(text string) (string, bool) {
	if len(text) == 0 {
		return "", false
	}
	last := text[len(text)-1]
	if last == 'i' || last == 'I' {
		return text[:len(text)-1], true
	}
	return "", false
}

// parseDecimalSuffix handles the parse decimal suffix step in the surrounding
// package and returns string, bool.
//
// It returns string, bool so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func parseDecimalSuffix(text string) (string, bool) {
	if len(text) == 0 {
		return "", false
	}
	last := text[len(text)-1]
	if last == 'd' || last == 'D' {
		return text[:len(text)-1], true
	}
	return "", false
}

// parseWidthSuffix handles the parse width suffix step in the surrounding
// package and returns uint8, string.
//
// It returns uint8, string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func parseWidthSuffix(text string) (uint8, string) {
	// Recognize trailing u8/u16/u32/u64 annotations, but leave packed forms
	// like uf8 alone so the packed parser can handle them.
	switch {
	case strings.HasSuffix(text, "uf8"), strings.HasSuffix(text, "uf16"), strings.HasSuffix(text, "uf32"), strings.HasSuffix(text, "uf64"):
		return 64, text
	case strings.HasSuffix(text, "u8"):
		return 8, strings.TrimSuffix(text, "u8")
	case strings.HasSuffix(text, "u16"):
		return 16, strings.TrimSuffix(text, "u16")
	case strings.HasSuffix(text, "u32"):
		return 32, strings.TrimSuffix(text, "u32")
	case strings.HasSuffix(text, "u64"):
		return 64, strings.TrimSuffix(text, "u64")
	default:
		return 64, text
	}
}

// formatInt handles the format int step in the surrounding package and returns
// string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func formatInt(v *big.Int, bitsWidth uint16) string {
	if bitsWidth == 0 {
		bitsWidth = 64
	}
	if v == nil {
		return "0i" + strconv.FormatUint(uint64(bitsWidth), 10)
	}
	return v.String() + "i" + strconv.FormatUint(uint64(bitsWidth), 10)
}

// wrapSignedWidth handles the wrap signed width step in the surrounding
// package and returns *big.Int.
//
// It returns *big.Int so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func wrapSignedWidth(v *big.Int, bitsWidth uint16) *big.Int {
	if bitsWidth == 0 {
		bitsWidth = 64
	}
	if v == nil {
		return big.NewInt(0)
	}
	mod := new(big.Int).Lsh(big.NewInt(1), uint(bitsWidth))
	wrapped := new(big.Int).Mod(new(big.Int).Set(v), mod)
	half := new(big.Int).Rsh(mod, 1)
	if wrapped.Cmp(half) >= 0 {
		wrapped.Sub(wrapped, mod)
	}
	return wrapped
}

// wrapUnsignedWidth handles the wrap unsigned width step in the surrounding
// package and returns *big.Int.
//
// It returns *big.Int so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func wrapUnsignedWidth(v *big.Int, bitsWidth uint16) *big.Int {
	if bitsWidth == 0 {
		bitsWidth = 64
	}
	if v == nil {
		return big.NewInt(0)
	}
	mod := new(big.Int).Lsh(big.NewInt(1), uint(bitsWidth))
	return new(big.Int).Mod(new(big.Int).Set(v), mod)
}

// signedIntFromValue handles the signed int from value step in the surrounding
// package and returns *big.Int, uint16, error.
//
// It returns *big.Int, uint16, error so callers can continue with the next
// stage of the pipeline without re-deriving the same information.
func signedIntFromValue(v Value) (*big.Int, uint16, error) {
	switch v.kind {
	case kindInt:
		if v.i == nil {
			return big.NewInt(0), v.ibits, nil
		}
		return new(big.Int).Set(v.i), v.ibits, nil
	case kindUint:
		if v.ub != nil {
			return new(big.Int).Set(v.ub), v.ubitsToUint16(), nil
		}
		return new(big.Int).SetUint64(v.u), uint16(v.bits), nil
	case kindBigInt:
		if v.bi == nil {
			return big.NewInt(0), 0, nil
		}
		return new(big.Int).Set(v.bi), 0, nil
	case kindBigFloat:
		if v.bf == nil {
			return big.NewInt(0), 0, nil
		}
		i, acc := new(big.Float).Copy(v.bf).Int(nil)
		if acc != big.Exact {
			return nil, 0, fmt.Errorf("expected integer, got %s", formatBigFloat(v.bf))
		}
		return i, 0, nil
	case kindDecimal:
		if v.dec == nil || !v.dec.IsInt() {
			return nil, 0, fmt.Errorf("expected integer, got %s", formatDecimal(v.dec))
		}
		return new(big.Int).Set(v.dec.Num()), 0, nil
	case kindFixed:
		i, width, _, err := fixedIntFromValue(v)
		if err != nil {
			return nil, 0, err
		}
		return i, width, nil
	case kindModInt:
		if v.mi == nil {
			return big.NewInt(0), 0, nil
		}
		return new(big.Int).Set(v.mi.val), 0, nil
	case kindNumber:
		if math.Trunc(v.num) != v.num {
			return nil, 0, fmt.Errorf("expected integer, got %s", formatNumber(v.num))
		}
		return big.NewInt(int64(v.num)), 0, nil
	case kindBool:
		if v.boo {
			return big.NewInt(1), 0, nil
		}
		return big.NewInt(0), 0, nil
	case kindQuantity:
		if v.qty == nil || len(v.qty.dims) != 0 || !v.qty.mag.IsInt() {
			return nil, 0, fmt.Errorf("expected integer, got %s", v.kind)
		}
		return new(big.Int).Set(v.qty.mag.Num()), 0, nil
	default:
		return nil, 0, fmt.Errorf("expected integer, got %s", v.kind)
	}
}

// exactUnsignedDivisionValue handles the exact unsigned division value step in
// the surrounding package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func exactUnsignedDivisionValue(left, right Value) (Value, error) {
	lu, lw, err := unsignedIntFromValue(left)
	if err != nil {
		return nullValue(), err
	}
	ru, rw, err := unsignedIntFromValue(right)
	if err != nil {
		return nullValue(), err
	}
	if ru.Sign() == 0 {
		return nullValue(), fmt.Errorf("division by zero")
	}
	width := lw
	if rw > width {
		width = rw
	}
	q, r := new(big.Int).QuoRem(lu, ru, new(big.Int))
	if r.Sign() == 0 {
		return uintValueFromBigStyle(q, width, unsignedStyleFromValues(left, right)), nil
	}
	return decimalValue(new(big.Rat).SetFrac(lu, ru)), nil
}

// exactSignedDivisionValue handles the exact signed division value step in the
// surrounding package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func exactSignedDivisionValue(left, right Value) (Value, error) {
	li, lw, err := signedIntFromValue(left)
	if err != nil {
		return nullValue(), err
	}
	ri, rw, err := signedIntFromValue(right)
	if err != nil {
		return nullValue(), err
	}
	if ri.Sign() == 0 {
		return nullValue(), fmt.Errorf("division by zero")
	}
	width := lw
	if rw > width {
		width = rw
	}
	if width == 0 {
		width = 64
	}
	q, r := new(big.Int).QuoRem(li, ri, new(big.Int))
	if r.Sign() == 0 {
		if left.kind == kindInt || right.kind == kindInt {
			return intValue(q, width), nil
		}
		return bigIntValue(q), nil
	}
	return decimalValue(new(big.Rat).SetFrac(li, ri)), nil
}

// wideUnsignedBinary handles the wide unsigned binary step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func wideUnsignedBinary(op tokenKind, left, right Value) (Value, error) {
	lu, lw, err := unsignedIntFromValue(left)
	if err != nil {
		return nullValue(), err
	}
	ru, rw, err := unsignedIntFromValue(right)
	if err != nil {
		return nullValue(), err
	}
	width := lw
	if rw > width {
		width = rw
	}
	if width == 0 {
		width = 64
	}
	mod := new(big.Int).Lsh(big.NewInt(1), uint(width))
	switch op {
	case tokenPlus:
		return uintValueFromBigStyle(new(big.Int).Mod(new(big.Int).Add(lu, ru), mod), width, unsignedStyleFromValues(left, right)), nil
	case tokenMinus:
		return uintValueFromBigStyle(new(big.Int).Mod(new(big.Int).Sub(lu, ru), mod), width, unsignedStyleFromValues(left, right)), nil
	case tokenStar:
		return uintValueFromBigStyle(new(big.Int).Mod(new(big.Int).Mul(lu, ru), mod), width, unsignedStyleFromValues(left, right)), nil
	case tokenPercent:
		if ru.Sign() == 0 {
			return nullValue(), fmt.Errorf("modulo by zero")
		}
		return uintValueFromBigStyle(new(big.Int).Mod(lu, ru), width, unsignedStyleFromValues(left, right)), nil
	default:
		return nullValue(), fmt.Errorf("unsupported unsigned operator %s", op)
	}
}

// exactPowValue handles the exact pow value step in the surrounding package
// and returns Value, bool, error.
//
// It returns Value, bool, error so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func exactPowValue(base, exponent Value) (Value, bool, error) {
	if base.kind == kindFixed {
		ei, err := bigIntFromValue(exponent)
		if err != nil {
			return nullValue(), false, nil
		}
		if ei.Sign() < 0 {
			return nullValue(), false, nil
		}
		if base.fx == nil {
			out, err := fixedValueOf(64, 0, big.NewInt(0))
			return out, true, err
		}
		r := fixedToRat(base.fx)
		pow := new(big.Rat).SetInt64(1)
		remaining := new(big.Int).Set(ei)
		cur := new(big.Rat).Set(r)
		zero := big.NewInt(0)
		one := big.NewInt(1)
		for remaining.Sign() > 0 {
			if new(big.Int).And(remaining, one).Cmp(zero) != 0 {
				pow.Mul(pow, cur)
			}
			cur.Mul(cur, cur)
			remaining.Rsh(remaining, 1)
		}
		out, err := fixedValueOf(base.fx.width, base.fx.frac, fixedRawFromRat(pow, base.fx.frac))
		if err != nil {
			return nullValue(), false, err
		}
		return out, true, nil
	}
	bi, err := bigIntFromValue(base)
	if err != nil {
		return nullValue(), false, nil
	}
	ei, err := bigIntFromValue(exponent)
	if err != nil {
		return nullValue(), false, nil
	}
	if ei.Sign() < 0 {
		return nullValue(), false, nil
	}
	out := new(big.Int).Exp(bi, ei, nil)
	switch base.kind {
	case kindInt:
		return intValue(out, base.ibits), true, nil
	case kindUint:
		if out.IsUint64() {
			return uintValue(out.Uint64(), base.bits), true, nil
		}
		return bigIntValue(out), true, nil
	}
	return bigIntValue(out), true, nil
}

// floatResultBits handles the float result bits step in the surrounding
// package and returns uint16.
//
// It returns uint16 so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func floatResultBits(left, right Value) uint16 {
	l := left.fbits
	r := right.fbits
	if l == 0 && r == 0 {
		return 0
	}
	if l == 0 {
		l = 64
	}
	if r == 0 {
		r = 64
	}
	if l > r {
		return l
	}
	return r
}

// complexComponentBits returns the precision carried by a complex value or
// by an explicitly typed real component. A zero result is the default complex
// family rather than an implicit f64 annotation.
func complexComponentBits(v Value) uint16 {
	if v.kind == kindComplex || v.kind == kindNumber {
		return v.fbits
	}
	return 0
}

// complexResultBits applies the scalar float promotion rule to complex
// components. A default numeric participant promotes an explicit f32 result
// to f64; fully f32 inputs remain complex<f32>.
func complexResultBits(values ...Value) uint16 {
	return floatResultBitsMany(values...)
}

// floatUnaryBits handles the float unary bits step in the surrounding package
// and returns uint16.
//
// It returns uint16 so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func floatUnaryBits(v Value) uint16 {
	if v.fbits == 0 {
		return 0
	}
	return v.fbits
}

// floatUnaryBFloat handles the float unary bfloat step in the surrounding
// package and returns bool.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func floatUnaryBFloat(v Value) bool {
	return v.bfloat
}

// floatValueStyled handles the float value styled step in the surrounding
// package and returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func floatValueStyled(n float64, bits uint16, bfloat bool) Value {
	if bfloat {
		return bfloatValue(n, bits)
	}
	return floatValue(n, bits)
}

// floatResultBitsMany handles the float result bits many step in the
// surrounding package and returns uint16.
//
// It returns uint16 so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func floatResultBitsMany(values ...Value) uint16 {
	var bits uint16
	var sawExplicit bool
	var sawDefault bool
	for _, v := range values {
		if v.fbits == 0 {
			sawDefault = true
			continue
		}
		sawExplicit = true
		if v.fbits > bits {
			bits = v.fbits
		}
	}
	if !sawExplicit {
		return 0
	}
	if sawDefault && bits < 64 {
		return 64
	}
	return bits
}

// floatResultBFloat handles the float result bfloat step in the surrounding
// package and returns bool.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func floatResultBFloat(values ...Value) bool {
	for _, v := range values {
		if v.bfloat {
			return true
		}
	}
	return false
}

// floatResultValue handles the float result value step in the surrounding
// package and returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func floatResultValue(n float64, values ...Value) Value {
	if floatResultBFloat(values...) {
		return bfloatValue(n, floatResultBitsMany(values...))
	}
	return floatValue(n, floatResultBitsMany(values...))
}

// parseDecimalText handles the parse decimal text step in the surrounding
// package and returns *big.Rat, error.
//
// It returns *big.Rat and an error. Callers should only trust the value when
// the error is nil.
func parseDecimalText(text string) (*big.Rat, error) {
	if text == "" {
		return nil, fmt.Errorf("invalid decimal literal")
	}
	if strings.Contains(text, "/") {
		parts := strings.SplitN(text, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid decimal literal")
		}
		num, ok := new(big.Int).SetString(parts[0], 10)
		if !ok {
			return nil, fmt.Errorf("invalid decimal literal")
		}
		den, ok := new(big.Int).SetString(parts[1], 10)
		if !ok || den.Sign() == 0 {
			return nil, fmt.Errorf("invalid decimal literal")
		}
		return new(big.Rat).SetFrac(num, den), nil
	}

	mantissa := text
	exp := 0
	if idx := strings.IndexAny(text, "eE"); idx >= 0 {
		mantissa = text[:idx]
		if mantissa == "" {
			return nil, fmt.Errorf("invalid decimal literal")
		}
		v, err := strconv.Atoi(text[idx+1:])
		if err != nil {
			return nil, fmt.Errorf("invalid decimal literal")
		}
		exp = v
	}
	if mantissa == "" {
		return nil, fmt.Errorf("invalid decimal literal")
	}
	if strings.Count(mantissa, ".") > 1 {
		return nil, fmt.Errorf("invalid decimal literal")
	}
	parts := strings.SplitN(mantissa, ".", 2)
	intPart := parts[0]
	fracPart := ""
	if len(parts) == 2 {
		fracPart = parts[1]
	}
	if intPart == "" && fracPart == "" {
		return nil, fmt.Errorf("invalid decimal literal")
	}
	digits := intPart + fracPart
	if digits == "" {
		return nil, fmt.Errorf("invalid decimal literal")
	}
	coeff, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return nil, fmt.Errorf("invalid decimal literal")
	}
	scale := len(fracPart) - exp
	if scale < 0 {
		coeff.Mul(coeff, pow10Big(-scale))
		return new(big.Rat).SetInt(coeff), nil
	}
	den := pow10Big(scale)
	return new(big.Rat).SetFrac(coeff, den), nil
}

// pow10Big handles the pow10 big step in the surrounding package and returns
// *big.Int.
//
// It returns *big.Int so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func pow10Big(n int) *big.Int {
	if n <= 0 {
		return big.NewInt(1)
	}
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}

// complexFromValue handles the complex from value step in the surrounding
// package and returns complex128, error.
//
// It returns complex128 and an error. Callers should only trust the value when
// the error is nil.
func complexFromValue(v Value) (complex128, error) {
	switch v.kind {
	case kindComplex:
		return v.cx, nil
	case kindNumber:
		return complex(v.num, 0), nil
	case kindDecimal:
		f, err := v.AsNumber()
		if err != nil {
			return 0, err
		}
		return complex(f, 0), nil
	case kindUint, kindBool:
		f, err := v.AsNumber()
		if err != nil {
			return 0, err
		}
		return complex(f, 0), nil
	case kindInt:
		f, err := v.AsNumber()
		if err != nil {
			return 0, err
		}
		return complex(f, 0), nil
	default:
		return 0, fmt.Errorf("expected complex-compatible value, got %s", v.kind)
	}
}

// complexValueFromArgs handles the complex value from args step in the
// surrounding package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func complexValueFromArgs(args []Value) (Value, error) {
	switch len(args) {
	case 1:
		c, err := complexFromValue(args[0])
		if err != nil {
			return nullValue(), err
		}
		return complexValueStyled(c, complexComponentBits(args[0])), nil
	case 2:
		re, err := args[0].AsNumber()
		if err != nil {
			return nullValue(), err
		}
		im, err := args[1].AsNumber()
		if err != nil {
			return nullValue(), err
		}
		return complexValueStyled(complex(re, im), complexResultBits(args...)), nil
	default:
		return nullValue(), fmt.Errorf("complex expects 1 or 2 arguments")
	}
}

// realValue handles the real value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func realValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("real expects 1 argument")
	}
	c, err := complexFromValue(args[0])
	if err != nil {
		return nullValue(), err
	}
	return floatValue(real(c), complexComponentBits(args[0])), nil
}

// imagValue handles the imag value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func imagValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("imag expects 1 argument")
	}
	c, err := complexFromValue(args[0])
	if err != nil {
		return nullValue(), err
	}
	return floatValue(imag(c), complexComponentBits(args[0])), nil
}

// conjugateValue returns the complex conjugate of a scalar or every element
// of a tensor. Real values retain their original numeric representation.
func conjugateValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("conjugate expects 1 argument")
	}
	x := args[0]
	if x.kind == kindTensor {
		data := make([]Value, len(x.data))
		for i, item := range x.data {
			value, err := conjugateValue([]Value{item})
			if err != nil {
				return nullValue(), err
			}
			data[i] = value
		}
		return tensorValue(x.shape, data), nil
	}
	if x.kind != kindComplex {
		if _, err := complexFromValue(x); err != nil {
			return nullValue(), err
		}
		return x, nil
	}
	return complexValueStyled(complex(real(x.cx), -imag(x.cx)), complexComponentBits(x)), nil
}

// magnitudeValue handles the magnitude value step in the surrounding package
// and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func magnitudeValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("magnitude expects 1 argument")
	}
	switch args[0].kind {
	case kindInt:
		i, width, err := signedIntFromValue(args[0])
		if err != nil {
			return nullValue(), err
		}
		if i.Sign() < 0 {
			i.Neg(i)
		}
		return intValue(i, width), nil
	case kindBigInt:
		bi, err := bigIntFromValue(args[0])
		if err != nil {
			return nullValue(), err
		}
		if bi.Sign() < 0 {
			bi.Neg(bi)
		}
		return bigIntValue(bi), nil
	case kindQuantity:
		q := args[0].qty
		if q == nil {
			return quantityValueOf(new(big.Rat), nil), nil
		}
		mag := new(big.Rat).Set(q.mag)
		if mag.Sign() < 0 {
			mag.Neg(mag)
		}
		return quantityValueOf(mag, q.dims), nil
	case kindNumber:
		return floatValueStyled(math.Abs(args[0].num), args[0].fbits, args[0].bfloat), nil
	case kindDecimal:
		r, err := decimalFromValue(args[0])
		if err != nil {
			return nullValue(), err
		}
		if r.Sign() < 0 {
			r.Neg(r)
		}
		return decimalValue(r), nil
	case kindUint:
		return args[0], nil
	case kindBool:
		if args[0].boo {
			return floatValueStyled(1, args[0].fbits, args[0].bfloat), nil
		}
		return floatValueStyled(0, args[0].fbits, args[0].bfloat), nil
	}
	c, err := complexFromValue(args[0])
	if err != nil {
		return nullValue(), err
	}
	return floatResultValue(cmplx.Abs(c), args[0]), nil
}

// complexBinary handles the complex binary step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func complexBinary(op tokenKind, left, right Value) (Value, error) {
	lc, err := complexFromValue(left)
	if err != nil {
		return nullValue(), err
	}
	rc, err := complexFromValue(right)
	if err != nil {
		return nullValue(), err
	}
	switch op {
	case tokenPlus:
		return complexValueStyled(lc+rc, complexResultBits(left, right)), nil
	case tokenMinus:
		return complexValueStyled(lc-rc, complexResultBits(left, right)), nil
	case tokenStar:
		return complexValueStyled(lc*rc, complexResultBits(left, right)), nil
	case tokenSlash:
		if rc == 0 {
			return nullValue(), fmt.Errorf("division by zero")
		}
		return complexValueStyled(lc/rc, complexResultBits(left, right)), nil
	default:
		return nullValue(), fmt.Errorf("unsupported complex operator %s", op)
	}
}

func dualValueFromArgs(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("dual expects 2 arguments")
	}
	primal, err := args[0].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	tangent, err := args[1].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	return dualValueOf(primal, tangent), nil
}

func dualParts(v Value) (float64, float64, error) {
	if v.kind == kindDual {
		if v.dual == nil {
			return 0, 0, nil
		}
		return v.dual.primal, v.dual.tangent, nil
	}
	primal, err := v.AsNumber()
	if err != nil {
		return 0, 0, err
	}
	return primal, 0, nil
}

func dualBinary(op tokenKind, left, right Value) (Value, error) {
	a, da, err := dualParts(left)
	if err != nil {
		return nullValue(), err
	}
	b, db, err := dualParts(right)
	if err != nil {
		return nullValue(), err
	}
	switch op {
	case tokenPlus:
		return dualValueOf(a+b, da+db), nil
	case tokenMinus:
		return dualValueOf(a-b, da-db), nil
	case tokenStar:
		return dualValueOf(a*b, da*b+a*db), nil
	case tokenSlash:
		if b == 0 {
			return nullValue(), fmt.Errorf("division by zero")
		}
		return dualValueOf(a/b, (da*b-a*db)/(b*b)), nil
	default:
		return nullValue(), fmt.Errorf("unsupported dual operator %s", op)
	}
}

func primalValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("primal expects 1 argument")
	}
	if args[0].kind == kindDual && args[0].dual != nil {
		return numberValue(args[0].dual.primal), nil
	}
	return args[0], nil
}

func tangentValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("tangent expects 1 argument")
	}
	if args[0].kind == kindDual && args[0].dual != nil {
		return numberValue(args[0].dual.tangent), nil
	}
	if _, err := args[0].AsNumber(); err != nil {
		return nullValue(), err
	}
	return numberValue(0), nil
}

// isFloatLiteral handles the is float literal step in the surrounding package
// and returns bool.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func isFloatLiteral(text string) bool {
	if strings.ContainsAny(text, ".eE") {
		return true
	}
	if strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X") || strings.HasPrefix(text, "0b") || strings.HasPrefix(text, "0B") || strings.HasPrefix(text, "0o") || strings.HasPrefix(text, "0O") {
		return false
	}
	return false
}

// uintFromValue handles the uint from value step in the surrounding package
// and returns uint64, uint8, error.
//
// It returns uint64, uint8, error so callers can continue with the next stage
// of the pipeline without re-deriving the same information.
func uintFromValue(v Value) (uint64, uint8, error) {
	switch v.kind {
	case kindUint:
		if v.ub != nil {
			if v.ubitsToUint16() > 64 || !v.ub.IsUint64() {
				return 0, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
			}
			return v.ub.Uint64(), uint8(v.ubitsToUint16()), nil
		}
		return v.u, v.bits, nil
	case kindNumber:
		if math.Trunc(v.num) != v.num || v.num < 0 {
			return 0, 0, fmt.Errorf("expected unsigned integer, got %s", formatNumber(v.num))
		}
		return uint64(v.num), 64, nil
	case kindDecimal:
		if v.dec == nil || !v.dec.IsInt() {
			return 0, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
		}
		if !v.dec.Num().IsUint64() {
			return 0, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
		}
		return v.dec.Num().Uint64(), 64, nil
	case kindFixed:
		i, _, _, err := fixedIntFromValue(v)
		if err != nil {
			return 0, 0, err
		}
		if !i.IsUint64() {
			return 0, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
		}
		return i.Uint64(), 64, nil
	case kindModInt:
		if v.mi == nil || !v.mi.val.IsUint64() {
			return 0, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
		}
		return v.mi.val.Uint64(), 64, nil
	case kindInt:
		if v.i == nil || v.i.Sign() < 0 || !v.i.IsUint64() {
			return 0, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
		}
		return v.i.Uint64(), v.ibitsToUint8(), nil
	case kindBool:
		if v.boo {
			return 1, 64, nil
		}
		return 0, 64, nil
	default:
		return 0, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
	}
}

// ubitsToUint16 operates on Value values and keeps the value-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns uint16 so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (v Value) ubitsToUint16() uint16 {
	if v.kind != kindUint {
		return 64
	}
	if v.ubits != 0 {
		return v.ubits
	}
	if v.packed && v.bits != 0 {
		return uint16(v.bits)
	}
	return 64
}

// unsignedIntFromValue handles the unsigned int from value step in the
// surrounding package and returns *big.Int, uint16, error.
//
// It returns *big.Int, uint16, error so callers can continue with the next
// stage of the pipeline without re-deriving the same information.
func unsignedIntFromValue(v Value) (*big.Int, uint16, error) {
	switch v.kind {
	case kindUint:
		if v.ub != nil {
			return new(big.Int).Set(v.ub), v.ubitsToUint16(), nil
		}
		return new(big.Int).SetUint64(v.u), uint16(v.bits), nil
	case kindBigInt:
		if v.bi == nil {
			return big.NewInt(0), 0, nil
		}
		if v.bi.Sign() < 0 {
			return nil, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
		}
		return new(big.Int).Set(v.bi), 0, nil
	case kindBigFloat:
		if v.bf == nil {
			return big.NewInt(0), 0, nil
		}
		i, acc := new(big.Float).Copy(v.bf).Int(nil)
		if acc != big.Exact || i.Sign() < 0 {
			return nil, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
		}
		return i, 0, nil
	case kindInt:
		if v.i == nil || v.i.Sign() < 0 {
			return nil, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
		}
		return new(big.Int).Set(v.i), v.ibits, nil
	case kindDecimal:
		if v.dec == nil || !v.dec.IsInt() || v.dec.Sign() < 0 {
			return nil, 0, fmt.Errorf("expected unsigned integer, got %s", formatDecimal(v.dec))
		}
		return new(big.Int).Set(v.dec.Num()), 0, nil
	case kindFixed:
		i, _, _, err := fixedIntFromValue(v)
		if err != nil {
			return nil, 0, err
		}
		if i.Sign() < 0 {
			return nil, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
		}
		return i, 0, nil
	case kindModInt:
		if v.mi == nil {
			return big.NewInt(0), 0, nil
		}
		return new(big.Int).Set(v.mi.val), 0, nil
	case kindNumber:
		if math.Trunc(v.num) != v.num || v.num < 0 {
			return nil, 0, fmt.Errorf("expected unsigned integer, got %s", formatNumber(v.num))
		}
		return big.NewInt(int64(v.num)), 0, nil
	case kindBool:
		if v.boo {
			return big.NewInt(1), 0, nil
		}
		return big.NewInt(0), 0, nil
	case kindQuantity:
		if v.qty == nil || len(v.qty.dims) != 0 || !v.qty.mag.IsInt() || v.qty.mag.Sign() < 0 {
			return nil, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
		}
		return new(big.Int).Set(v.qty.mag.Num()), 0, nil
	default:
		return nil, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
	}
}

// ibitsToUint8 operates on Value values and keeps the value-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns uint8 so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func (v Value) ibitsToUint8() uint8 {
	if v.kind != kindInt || v.ibits == 0 || v.ibits > 64 {
		return 64
	}
	return uint8(v.ibits)
}

// uintUnary handles the uint unary step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func uintUnary(v Value, fn func(uint64, uint8) uint64) (Value, error) {
	u, bitsWidth, err := uintFromValue(v)
	if err != nil {
		return nullValue(), err
	}
	if v.packed {
		return packedUintValue(fn(u, bitsWidth), bitsWidth, v.native), nil
	}
	return uintValue(fn(u, bitsWidth), bitsWidth), nil
}

// uintBinary handles the uint binary step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func uintBinary(a, b Value, fn func(uint64, uint64) uint64) (Value, error) {
	ua, wa, err := uintFromValue(a)
	if err != nil {
		return nullValue(), err
	}
	ub, wb, err := uintFromValue(b)
	if err != nil {
		return nullValue(), err
	}
	width := wa
	if wb < width {
		width = wb
	}
	if width == 0 {
		width = 64
	}
	style := unsignedStyleFromValues(a, b)
	if style.packed {
		return packedUintValue(fn(ua, ub), width, style.native), nil
	}
	return uintValue(fn(ua, ub), width), nil
}

// evalBitwiseUnary handles the eval bitwise unary step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func evalBitwiseUnary(v Value) (Value, error) {
	if v.kind == kindInt {
		i, width, err := signedIntFromValue(v)
		if err != nil {
			return nullValue(), err
		}
		if width == 0 {
			width = 64
		}
		return intValue(new(big.Int).Not(i), width), nil
	}
	if isWideUintValue(v) {
		u, width, err := unsignedIntFromValue(v)
		if err != nil {
			return nullValue(), err
		}
		mask := unsignedMask(width)
		return uintValueFromBigStyle(new(big.Int).Xor(u, mask), width, unsignedStyleFromValues(v)), nil
	}
	u, bitsWidth, err := uintFromValue(v)
	if err != nil {
		return nullValue(), err
	}
	if v.packed {
		return packedUintValue(^u, bitsWidth, v.native), nil
	}
	return uintValue(^u, bitsWidth), nil
}

// evalBitwiseBinary handles the eval bitwise binary step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func evalBitwiseBinary(op tokenKind, left, right Value) (Value, error) {
	if left.kind == kindInt || right.kind == kindInt {
		li, lw, err := signedIntFromValue(left)
		if err != nil {
			return nullValue(), err
		}
		ri, rw, err := signedIntFromValue(right)
		if err != nil {
			return nullValue(), err
		}
		width := lw
		if op != tokenShiftLeft && op != tokenShiftRight && rw > width {
			width = rw
		}
		if width == 0 {
			width = 64
		}
		switch op {
		case tokenAmp:
			return intValue(new(big.Int).And(li, ri), width), nil
		case tokenPipe:
			return intValue(new(big.Int).Or(li, ri), width), nil
		case tokenCaret:
			return intValue(new(big.Int).Xor(li, ri), width), nil
		case tokenShiftLeft:
			shift := uint(0)
			if s, _, err := uintFromValue(right); err == nil {
				shift = uint(s % 64)
			} else {
				return nullValue(), err
			}
			return intValue(new(big.Int).Lsh(li, shift), width), nil
		case tokenShiftRight:
			shift := uint(0)
			if s, _, err := uintFromValue(right); err == nil {
				shift = uint(s % 64)
			} else {
				return nullValue(), err
			}
			return intValue(new(big.Int).Rsh(li, shift), width), nil
		default:
			return nullValue(), fmt.Errorf("unsupported bitwise operator %s", op)
		}
	}
	if isWideUintValue(left) || isWideUintValue(right) {
		lu, lw, err := unsignedIntFromValue(left)
		if err != nil {
			return nullValue(), err
		}
		ru, rw, err := unsignedIntFromValue(right)
		if err != nil {
			return nullValue(), err
		}
		width := lw
		if op != tokenShiftLeft && op != tokenShiftRight && rw > width {
			width = rw
		}
		if width == 0 {
			width = 64
		}
		mask := unsignedMask(width)
		switch op {
		case tokenAmp:
			return uintValueFromBigStyle(new(big.Int).And(lu, ru), width, unsignedStyleFromValues(left, right)), nil
		case tokenPipe:
			return uintValueFromBigStyle(new(big.Int).And(new(big.Int).Or(lu, ru), mask), width, unsignedStyleFromValues(left, right)), nil
		case tokenCaret:
			return uintValueFromBigStyle(new(big.Int).And(new(big.Int).Xor(lu, ru), mask), width, unsignedStyleFromValues(left, right)), nil
		case tokenShiftLeft:
			shift, err := unsignedShiftAmount(right)
			if err != nil {
				return nullValue(), err
			}
			return uintValueFromBigStyle(new(big.Int).And(new(big.Int).Lsh(lu, shift), mask), width, unsignedStyleFromValues(left, right)), nil
		case tokenShiftRight:
			shift, err := unsignedShiftAmount(right)
			if err != nil {
				return nullValue(), err
			}
			return uintValueFromBigStyle(new(big.Int).Rsh(lu, shift), width, unsignedStyleFromValues(left, right)), nil
		default:
			return nullValue(), fmt.Errorf("unsupported bitwise operator %s", op)
		}
	}
	lu, lw, err := uintFromValue(left)
	if err != nil {
		return nullValue(), err
	}
	ru, rw, err := uintFromValue(right)
	if err != nil {
		return nullValue(), err
	}
	width := lw
	if op != tokenShiftLeft && op != tokenShiftRight && rw > width {
		width = rw
	}
	if width == 0 {
		width = 64
	}
	mask := maskForWidth(width)
	switch op {
	case tokenAmp:
		if left.packed || right.packed {
			return packedUintValue(lu&ru, width, left.native && right.native), nil
		}
		return uintValue(lu&ru, width), nil
	case tokenPipe:
		if left.packed || right.packed {
			return packedUintValue((lu|ru)&mask, width, left.native && right.native), nil
		}
		return uintValue((lu|ru)&mask, width), nil
	case tokenCaret:
		if left.packed || right.packed {
			return packedUintValue((lu^ru)&mask, width, left.native && right.native), nil
		}
		return uintValue((lu^ru)&mask, width), nil
	case tokenShiftLeft:
		shift := uint(ru % 64)
		if left.packed || right.packed {
			return packedUintValue((lu<<shift)&mask, width, left.native && right.native), nil
		}
		return uintValue((lu<<shift)&mask, width), nil
	case tokenShiftRight:
		shift := uint(ru % 64)
		if left.packed || right.packed {
			return packedUintValue((lu>>shift)&mask, width, left.native && right.native), nil
		}
		return uintValue((lu>>shift)&mask, width), nil
	default:
		return nullValue(), fmt.Errorf("unsupported bitwise operator %s", op)
	}
}

// unsignedShiftAmount handles the unsigned shift amount step in the
// surrounding package and returns uint, error.
//
// It returns uint and an error. Callers should only trust the value when the
// error is nil.
func unsignedShiftAmount(v Value) (uint, error) {
	u, _, err := unsignedIntFromValue(v)
	if err != nil {
		return 0, err
	}
	if !u.IsUint64() {
		return 0, fmt.Errorf("shift count too large")
	}
	return uint(u.Uint64()), nil
}

// rotlValue handles the rotl value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func rotlValue(left, right Value) (Value, error) {
	u, width, err := unsignedIntFromValue(left)
	if err != nil {
		return nullValue(), err
	}
	shift, err := unsignedShiftAmount(right)
	if err != nil {
		return nullValue(), err
	}
	if width == 0 {
		width = 64
	}
	if width <= 64 && u.IsUint64() {
		if left.packed {
			return packedUintValue(rotl(u.Uint64(), uint8(width), uint64(shift)), uint8(width), left.native), nil
		}
		return uintValue(rotl(u.Uint64(), uint8(width), uint64(shift)), uint8(width)), nil
	}
	mask := unsignedMask(width)
	bw := int(width)
	if bw == 0 {
		return uintValueFromBigStyle(u, width, unsignedStyleFromValues(left, right)), nil
	}
	s := int(shift % uint(bw))
	lo := new(big.Int).And(u, mask)
	if s == 0 {
		return uintValueFromBigStyle(lo, width, unsignedStyleFromValues(left, right)), nil
	}
	leftPart := new(big.Int).Lsh(lo, uint(s))
	rightPart := new(big.Int).Rsh(lo, uint(bw-s))
	return uintValueFromBigStyle(new(big.Int).And(new(big.Int).Or(leftPart, rightPart), mask), width, unsignedStyleFromValues(left, right)), nil
}

// rotrValue handles the rotr value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func rotrValue(left, right Value) (Value, error) {
	u, width, err := unsignedIntFromValue(left)
	if err != nil {
		return nullValue(), err
	}
	shift, err := unsignedShiftAmount(right)
	if err != nil {
		return nullValue(), err
	}
	if width == 0 {
		width = 64
	}
	if width <= 64 && u.IsUint64() {
		if left.packed {
			return packedUintValue(rotr(u.Uint64(), uint8(width), uint64(shift)), uint8(width), left.native), nil
		}
		return uintValue(rotr(u.Uint64(), uint8(width), uint64(shift)), uint8(width)), nil
	}
	mask := unsignedMask(width)
	bw := int(width)
	if bw == 0 {
		return uintValueFromBigStyle(u, width, unsignedStyleFromValues(left, right)), nil
	}
	s := int(shift % uint(bw))
	lo := new(big.Int).And(u, mask)
	if s == 0 {
		return uintValueFromBigStyle(lo, width, unsignedStyleFromValues(left, right)), nil
	}
	leftPart := new(big.Int).Lsh(lo, uint(bw-s))
	rightPart := new(big.Int).Rsh(lo, uint(s))
	return uintValueFromBigStyle(new(big.Int).And(new(big.Int).Or(leftPart, rightPart), mask), width, unsignedStyleFromValues(left, right)), nil
}

// rotl handles the rotl step in the surrounding package and returns uint64.
//
// It returns uint64 so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func rotl(u uint64, bitsWidth uint8, n uint64) uint64 {
	if bitsWidth == 0 || bitsWidth >= 64 {
		return bits.RotateLeft64(u, int(n))
	}
	mask := maskForWidth(bitsWidth)
	bw := uint64(bitsWidth)
	shift := n % bw
	u &= mask
	return ((u << shift) | (u >> (bw - shift))) & mask
}

// rotr handles the rotr step in the surrounding package and returns uint64.
//
// It returns uint64 so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func rotr(u uint64, bitsWidth uint8, n uint64) uint64 {
	if bitsWidth == 0 || bitsWidth >= 64 {
		return bits.RotateLeft64(u, -int(n))
	}
	mask := maskForWidth(bitsWidth)
	bw := uint64(bitsWidth)
	shift := n % bw
	u &= mask
	return ((u >> shift) | (u << (bw - shift))) & mask
}

// packBytes handles the pack bytes step in the surrounding package and returns
// []byte, error.
//
// It returns []byte and an error. Callers should only trust the value when the
// error is nil.
func packBytes(values []Value) ([]byte, error) {
	out := make([]byte, len(values))
	for i, v := range values {
		u, _, err := unsignedIntFromValue(v)
		if err != nil {
			return nil, err
		}
		out[i] = byte(u.Uint64() & 0xff)
	}
	return out, nil
}

// unpackBytes handles the unpack bytes step in the surrounding package and
// returns []Value.
//
// It returns []Value so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func unpackBytes(data []byte) []Value {
	out := make([]Value, len(data))
	for i, b := range data {
		out[i] = uintValue(uint64(b), 8)
	}
	return out
}

// hexBytes handles the hex bytes step in the surrounding package and returns
// string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func hexBytes(data []byte) string {
	return hex.EncodeToString(data)
}

// binBytes handles the bin bytes step in the surrounding package and returns
// string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func binBytes(data []byte) string {
	var b strings.Builder
	for _, by := range data {
		b.WriteString(fmt.Sprintf("%08b", by))
	}
	return b.String()
}

// castWidthValue handles the cast width value step in the surrounding package
// and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func castWidthValue(args []Value, width uint8) (Value, error) {
	return castUnsignedWidthValue(args, uint16(width))
}

// castUnsignedWidthValue handles the cast unsigned width value step in the
// surrounding package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func castUnsignedWidthValue(args []Value, width uint16) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("expected 1 argument")
	}
	u, _, err := unsignedIntFromValue(args[0])
	if err != nil {
		return nullValue(), err
	}
	return uintValueFromBig(u, width), nil
}

// castPackedUnsignedWidthValue handles the cast packed unsigned width value
// step in the surrounding package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func castPackedUnsignedWidthValue(args []Value, width uint16, native bool) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("expected 1 argument")
	}
	u, _, err := unsignedIntFromValue(args[0])
	if err != nil {
		return nullValue(), err
	}
	return uintValueFromBigStyle(u, width, unsignedStyle{packed: true, native: native}), nil
}

// castFloatWidthValue handles the cast float width value step in the
// surrounding package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func castFloatWidthValue(args []Value, width uint16) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("expected 1 argument")
	}
	n, err := args[0].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	return floatValueStyled(n, width, false), nil
}

// castBFloatWidthValue handles the cast bfloat width value step in the
// surrounding package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func castBFloatWidthValue(args []Value, width uint16) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("expected 1 argument")
	}
	n, err := args[0].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	return bfloatValue(n, width), nil
}

// castSignedWidthValue handles the cast signed width value step in the
// surrounding package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func castSignedWidthValue(args []Value, width uint16) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("expected 1 argument")
	}
	i, _, err := signedIntFromValue(args[0])
	if err != nil {
		return nullValue(), err
	}
	return intValue(i, width), nil
}

// bytesFromValue handles the bytes from value step in the surrounding package
// and returns []byte, error.
//
// It returns []byte and an error. Callers should only trust the value when the
// error is nil.
func bytesFromValue(v Value) ([]byte, error) {
	switch v.kind {
	case kindString:
		return []byte(v.str), nil
	case kindList:
		return packBytes(v.list)
	case kindTensor:
		return packBytes(v.data)
	case kindUint:
		if v.ub != nil {
			if v.ubitsToUint16() <= 64 {
				return u64Bytes(v.ub.Uint64()), nil
			}
			size := int((v.ubitsToUint16() + 7) / 8)
			raw := v.ub.Bytes()
			if len(raw) >= size {
				return append([]byte(nil), raw...), nil
			}
			out := make([]byte, size)
			copy(out[size-len(raw):], raw)
			return out, nil
		}
		return u64Bytes(v.u), nil
	case kindNumber:
		u, _, err := uintFromValue(v)
		if err != nil {
			return nil, err
		}
		return u64Bytes(u), nil
	case kindDecimal:
		u, _, err := uintFromValue(v)
		if err != nil {
			return nil, err
		}
		return u64Bytes(u), nil
	default:
		return nil, fmt.Errorf("cannot encode %s as bytes", v.kind)
	}
}

// u64Bytes handles the u64 bytes step in the surrounding package and returns
// []byte.
//
// It returns []byte so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func u64Bytes(v uint64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	return buf[:]
}

// hexValue handles the hex value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func hexValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("hex expects 1 argument")
	}
	b, err := bytesFromValue(args[0])
	if err != nil {
		return nullValue(), err
	}
	return stringValue(hexBytes(b)), nil
}

// binValue handles the bin value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func binValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("bin expects 1 argument")
	}
	b, err := bytesFromValue(args[0])
	if err != nil {
		return nullValue(), err
	}
	return stringValue(binBytes(b)), nil
}

// bytesValue handles the bytes value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func bytesValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("bytes expects 1 argument")
	}
	switch args[0].kind {
	case kindString:
		out := make([]Value, len(args[0].str))
		for i := range args[0].str {
			out[i] = uintValue(uint64(args[0].str[i]), 8)
		}
		return listValue(out), nil
	case kindList:
		return listValue(append([]Value(nil), args[0].list...)), nil
	case kindTensor:
		return listValue(append([]Value(nil), args[0].data...)), nil
	case kindUint, kindNumber:
		b, err := bytesFromValue(args[0])
		if err != nil {
			return nullValue(), err
		}
		out := make([]Value, len(b))
		for i, by := range b {
			out[i] = uintValue(uint64(by), 8)
		}
		return listValue(out), nil
	default:
		return nullValue(), fmt.Errorf("bytes expects string, list, tensor, or integer value")
	}
}

// packValue handles the pack value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func packValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("pack expects 1 argument")
	}
	items, err := args[0].AsList()
	if err != nil {
		return nullValue(), err
	}
	b, err := packBytes(items)
	if err != nil {
		return nullValue(), err
	}
	return stringValue(string(b)), nil
}

// unpackValue handles the unpack value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func unpackValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("unpack expects 1 argument")
	}
	s, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	return listValue(unpackBytes([]byte(s))), nil
}

// atValue handles the at value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func atValue(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("at expects 2 arguments")
	}
	if args[0].kind == kindFuture {
		return futureIndex(args[0], args[1])
	}
	if args[0].kind == kindTensor && args[1].kind == kindList {
		coordinateRank := len(args[1].list)
		if coordinateRank == 0 || coordinateRank > len(args[0].shape) {
			return nullValue(), fmt.Errorf("tensor coordinate rank does not match tensor rank")
		}
		offset := 0
		stride := 1
		for axis := len(args[0].shape) - 1; axis >= coordinateRank; axis-- {
			stride *= args[0].shape[axis]
		}
		blockLen := stride
		for axis := coordinateRank - 1; axis >= 0; axis-- {
			coordinate, err := args[1].list[axis].Int()
			if err != nil || coordinate < 0 || coordinate >= args[0].shape[axis] {
				return nullValue(), fmt.Errorf("tensor coordinate out of range")
			}
			offset += coordinate * stride
			stride *= args[0].shape[axis]
		}
		if coordinateRank == len(args[0].shape) {
			return args[0].data[offset], nil
		}
		nextShape := append([]int(nil), args[0].shape[coordinateRank:]...)
		nextData := append([]Value(nil), args[0].data[offset:offset+blockLen]...)
		return newTensorValue(nextShape, nextData), nil
	}
	idx, err := args[1].Int()
	if err != nil {
		return nullValue(), err
	}
	if idx < 0 {
		return nullValue(), fmt.Errorf("at index must be non-negative")
	}
	switch args[0].kind {
	case kindString:
		runes := []rune(args[0].str)
		if idx >= len(runes) {
			return nullValue(), fmt.Errorf("at index out of range")
		}
		return stringValue(string(runes[idx])), nil
	case kindList:
		if idx >= len(args[0].list) {
			return nullValue(), fmt.Errorf("at index out of range")
		}
		return args[0].list[idx], nil
	case kindTuple:
		if idx >= len(args[0].tuple) {
			return nullValue(), fmt.Errorf("at index out of range")
		}
		return args[0].tuple[idx], nil
	case kindTensor:
		if idx >= len(args[0].data) {
			return nullValue(), fmt.Errorf("at index out of range")
		}
		return args[0].data[idx], nil
	default:
		return nullValue(), fmt.Errorf("at expects a string, list, or tensor")
	}
}

// setAtValue handles the set at value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func setAtValue(args []Value) (Value, error) {
	if len(args) != 3 {
		return nullValue(), fmt.Errorf("setat expects 3 arguments")
	}
	if args[0].kind == kindFuture {
		return futureSetIndex(args[0], args[1], args[2])
	}
	if args[0].kind == kindTensor && args[1].kind == kindList {
		coordinateRank := len(args[1].list)
		if coordinateRank == 0 || coordinateRank > len(args[0].shape) {
			return nullValue(), fmt.Errorf("tensor coordinate rank does not match tensor rank")
		}
		offset := 0
		stride := 1
		for axis := len(args[0].shape) - 1; axis >= coordinateRank; axis-- {
			stride *= args[0].shape[axis]
		}
		blockLen := stride
		for axis := coordinateRank - 1; axis >= 0; axis-- {
			coordinate, err := args[1].list[axis].Int()
			if err != nil || coordinate < 0 || coordinate >= args[0].shape[axis] {
				return nullValue(), fmt.Errorf("tensor coordinate out of range")
			}
			offset += coordinate * stride
			stride *= args[0].shape[axis]
		}
		next := append([]Value(nil), args[0].data...)
		if coordinateRank < len(args[0].shape) {
			if args[2].kind != kindTensor || !equalIntSlices(args[2].shape, args[0].shape[coordinateRank:]) {
				return nullValue(), fmt.Errorf("tensor replacement shape does not match coordinate slice")
			}
			copy(next[offset:offset+blockLen], args[2].data)
			return newTensorValue(args[0].shape, next), nil
		}
		number, err := args[2].AsNumber()
		if err != nil {
			return nullValue(), err
		}
		next[offset] = numberValue(number)
		return newTensorValue(args[0].shape, next), nil
	}
	idx, err := args[1].Int()
	if err != nil {
		return nullValue(), err
	}
	if idx < 0 {
		return nullValue(), fmt.Errorf("setat index must be non-negative")
	}
	switch args[0].kind {
	case kindString:
		runes := []rune(args[0].str)
		if idx >= len(runes) {
			return nullValue(), fmt.Errorf("setat index out of range")
		}
		replacement := []rune(textify(args[2]))
		if len(replacement) == 0 {
			return nullValue(), fmt.Errorf("setat replacement cannot be empty")
		}
		next := append([]rune(nil), runes...)
		next[idx] = replacement[0]
		return stringValue(string(next)), nil
	case kindList:
		if idx >= len(args[0].list) {
			return nullValue(), fmt.Errorf("setat index out of range")
		}
		next := append([]Value(nil), args[0].list...)
		next[idx] = args[2]
		return listValue(next), nil
	case kindTensor:
		if idx >= len(args[0].data) {
			return nullValue(), fmt.Errorf("setat index out of range")
		}
		next := append([]Value(nil), args[0].data...)
		next[idx] = args[2]
		return newTensorValue(args[0].shape, next), nil
	default:
		return nullValue(), fmt.Errorf("setat expects a string, list, or tensor")
	}
}

// decimalFromValue handles the decimal from value step in the surrounding
// package and returns *big.Rat, error.
//
// It returns *big.Rat and an error. Callers should only trust the value when
// the error is nil.
func decimalFromValue(v Value) (*big.Rat, error) {
	switch v.kind {
	case kindDecimal:
		if v.dec == nil {
			return new(big.Rat), nil
		}
		return new(big.Rat).Set(v.dec), nil
	case kindFixed:
		if v.fx == nil {
			return new(big.Rat), nil
		}
		return fixedToRat(v.fx), nil
	case kindModInt:
		if v.mi == nil {
			return new(big.Rat), nil
		}
		return new(big.Rat).SetInt(v.mi.val), nil
	case kindNumber:
		if math.IsNaN(v.num) || math.IsInf(v.num, 0) {
			return nil, fmt.Errorf("cannot convert %s to decimal", formatNumber(v.num))
		}
		r := new(big.Rat)
		r.SetFloat64(v.num)
		return r, nil
	case kindUint:
		if v.ub != nil {
			return new(big.Rat).SetInt(new(big.Int).Set(v.ub)), nil
		}
		return new(big.Rat).SetInt(new(big.Int).SetUint64(v.u)), nil
	case kindInt:
		if v.i == nil {
			return new(big.Rat), nil
		}
		return new(big.Rat).SetInt(v.i), nil
	case kindBool:
		if v.boo {
			return new(big.Rat).SetInt64(1), nil
		}
		return new(big.Rat), nil
	default:
		return nil, fmt.Errorf("cannot convert %s to decimal", v.kind)
	}
}

// decimalValueFromString handles the decimal value from string step in the
// surrounding package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func decimalValueFromString(text string) (Value, error) {
	rat, err := parseDecimalText(strings.TrimSpace(strings.TrimSuffix(text, "d")))
	if err != nil {
		return nullValue(), err
	}
	return decimalValue(rat), nil
}

// decimalBinary handles the decimal binary step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func decimalBinary(op tokenKind, left, right Value) (Value, error) {
	ln, err := decimalFromValue(left)
	if err != nil {
		return nullValue(), err
	}
	rn, err := decimalFromValue(right)
	if err != nil {
		return nullValue(), err
	}
	switch op {
	case tokenPlus:
		return decimalValue(new(big.Rat).Add(ln, rn)), nil
	case tokenMinus:
		return decimalValue(new(big.Rat).Sub(ln, rn)), nil
	case tokenStar:
		return decimalValue(new(big.Rat).Mul(ln, rn)), nil
	case tokenSlash:
		if rn.Sign() == 0 {
			return nullValue(), fmt.Errorf("division by zero")
		}
		return decimalValue(new(big.Rat).Quo(ln, rn)), nil
	default:
		return nullValue(), fmt.Errorf("unsupported decimal operator %s", op)
	}
}

// decimalCompare handles the decimal compare step in the surrounding package
// and returns int, error.
//
// It returns int and an error. Callers should only trust the value when the
// error is nil.
func decimalCompare(left, right Value) (int, error) {
	ln, err := decimalFromValue(left)
	if err != nil {
		return 0, err
	}
	rn, err := decimalFromValue(right)
	if err != nil {
		return 0, err
	}
	return ln.Cmp(rn), nil
}

// formatDecimal handles the format decimal step in the surrounding package and
// returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func formatDecimal(r *big.Rat) string {
	if r == nil {
		return "0d"
	}
	if r.Sign() == 0 {
		return "0d"
	}
	sign := ""
	num := new(big.Int).Set(r.Num())
	if num.Sign() < 0 {
		sign = "-"
		num.Abs(num)
	}
	den := new(big.Int).Set(r.Denom())
	tmp := new(big.Int).Set(den)
	twoCount := 0
	for tmp.Sign() != 0 {
		mod := new(big.Int)
		tmp.QuoRem(tmp, big.NewInt(2), mod)
		if mod.Sign() != 0 {
			break
		}
		twoCount++
	}
	tmp.Set(den)
	fiveCount := 0
	for tmp.Sign() != 0 {
		mod := new(big.Int)
		tmp.QuoRem(tmp, big.NewInt(5), mod)
		if mod.Sign() != 0 {
			break
		}
		fiveCount++
	}
	denCheck := new(big.Int).Set(den)
	for i := 0; i < twoCount; i++ {
		denCheck.Quo(denCheck, big.NewInt(2))
	}
	for i := 0; i < fiveCount; i++ {
		denCheck.Quo(denCheck, big.NewInt(5))
	}
	if denCheck.Cmp(big.NewInt(1)) == 0 {
		scale := twoCount
		if fiveCount > scale {
			scale = fiveCount
		}
		scaled := new(big.Int).Set(num)
		scaled.Mul(scaled, pow10Big(scale))
		scaled.Quo(scaled, den)
		s := scaled.String()
		if scale == 0 {
			return sign + s + "d"
		}
		if len(s) <= scale {
			s = strings.Repeat("0", scale-len(s)+1) + s
		}
		cut := len(s) - scale
		whole := s[:cut]
		frac := strings.TrimRight(s[cut:], "0")
		if frac == "" {
			return sign + whole + "d"
		}
		return sign + whole + "." + frac + "d"
	}
	return sign + r.Num().String() + "/" + r.Denom().String() + "d"
}
