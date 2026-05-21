package osiris

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"math/bits"
	"strconv"
	"strings"
)

func maskForWidth(bitsWidth uint8) uint64 {
	if bitsWidth == 0 || bitsWidth >= 64 {
		return ^uint64(0)
	}
	return (uint64(1) << bitsWidth) - 1
}

func formatUint(v uint64, bitsWidth uint8) string {
	if bitsWidth == 0 || bitsWidth == 64 {
		return fmt.Sprintf("%du64", v)
	}
	return fmt.Sprintf("%du%d", v&maskForWidth(bitsWidth), bitsWidth)
}

func parseNumericLiteral(text string) (Value, error) {
	cleaned := strings.ReplaceAll(text, "_", "")
	if cleaned == "" {
		return nullValue(), fmt.Errorf("invalid numeric literal %q", text)
	}

	suffixBits, stripped := parseWidthSuffix(cleaned)
	if isFloatLiteral(stripped) {
		n, err := strconv.ParseFloat(stripped, 64)
		if err != nil {
			return nullValue(), fmt.Errorf("invalid numeric literal %q", text)
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
		return nullValue(), fmt.Errorf("invalid numeric literal %q", text)
	}
	u, err := strconv.ParseUint(digits, base, 64)
	if err != nil {
		return nullValue(), fmt.Errorf("invalid numeric literal %q", text)
	}
	return uintValue(u, suffixBits), nil
}

func parseWidthSuffix(text string) (uint8, string) {
	// Recognize trailing u8/u16/u32/u64 annotations.
	switch {
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

func isFloatLiteral(text string) bool {
	if strings.ContainsAny(text, ".eE") {
		return true
	}
	if strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X") || strings.HasPrefix(text, "0b") || strings.HasPrefix(text, "0B") || strings.HasPrefix(text, "0o") || strings.HasPrefix(text, "0O") {
		return false
	}
	return false
}

func uintFromValue(v Value) (uint64, uint8, error) {
	switch v.kind {
	case kindUint:
		return v.u, v.bits, nil
	case kindNumber:
		if math.Trunc(v.num) != v.num || v.num < 0 {
			return 0, 0, fmt.Errorf("expected unsigned integer, got %s", formatNumber(v.num))
		}
		return uint64(v.num), 64, nil
	case kindBool:
		if v.boo {
			return 1, 64, nil
		}
		return 0, 64, nil
	default:
		return 0, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
	}
}

func uintUnary(v Value, fn func(uint64, uint8) uint64) (Value, error) {
	u, bitsWidth, err := uintFromValue(v)
	if err != nil {
		return nullValue(), err
	}
	return uintValue(fn(u, bitsWidth), bitsWidth), nil
}

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
	return uintValue(fn(ua, ub), width), nil
}

func evalBitwiseUnary(v Value) (Value, error) {
	u, bitsWidth, err := uintFromValue(v)
	if err != nil {
		return nullValue(), err
	}
	return uintValue(^u, bitsWidth), nil
}

func evalBitwiseBinary(op tokenKind, left, right Value) (Value, error) {
	lu, lw, err := uintFromValue(left)
	if err != nil {
		return nullValue(), err
	}
	ru, rw, err := uintFromValue(right)
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
	mask := maskForWidth(width)
	switch op {
	case tokenAmp:
		return uintValue(lu&ru, width), nil
	case tokenPipe:
		return uintValue((lu|ru)&mask, width), nil
	case tokenCaret:
		return uintValue((lu^ru)&mask, width), nil
	case tokenShiftLeft:
		shift := uint(ru % 64)
		return uintValue((lu<<shift)&mask, width), nil
	case tokenShiftRight:
		shift := uint(ru % 64)
		return uintValue((lu>>shift)&mask, width), nil
	default:
		return nullValue(), fmt.Errorf("unsupported bitwise operator %s", op)
	}
}

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

func packBytes(values []Value) ([]byte, error) {
	out := make([]byte, len(values))
	for i, v := range values {
		u, _, err := uintFromValue(v)
		if err != nil {
			return nil, err
		}
		out[i] = byte(u & 0xff)
	}
	return out, nil
}

func unpackBytes(data []byte) []Value {
	out := make([]Value, len(data))
	for i, b := range data {
		out[i] = uintValue(uint64(b), 8)
	}
	return out
}

func hexBytes(data []byte) string {
	return hex.EncodeToString(data)
}

func binBytes(data []byte) string {
	var b strings.Builder
	for _, by := range data {
		b.WriteString(fmt.Sprintf("%08b", by))
	}
	return b.String()
}

func castWidthValue(args []Value, width uint8) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("expected 1 argument")
	}
	u, _, err := uintFromValue(args[0])
	if err != nil {
		return nullValue(), err
	}
	return uintValue(u, width), nil
}

func bytesFromValue(v Value) ([]byte, error) {
	switch v.kind {
	case kindString:
		return []byte(v.str), nil
	case kindList:
		return packBytes(v.list)
	case kindUint:
		return u64Bytes(v.u), nil
	case kindNumber:
		u, _, err := uintFromValue(v)
		if err != nil {
			return nil, err
		}
		return u64Bytes(u), nil
	default:
		return nil, fmt.Errorf("cannot encode %s as bytes", v.kind)
	}
}

func u64Bytes(v uint64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	return buf[:]
}

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
		return nullValue(), fmt.Errorf("bytes expects string, list, or integer value")
	}
}

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
