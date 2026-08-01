package osiris

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"
)

const codecHeader = "OSR1"

// maxCodecValueBytes bounds one untrusted encoded value before recursive
// decoding or allocation. It is a resource-control limit, not a sandbox.
const maxCodecValueBytes = 64 << 20

type byteCursor struct {
	data []byte
	off  int
}

// remaining operates on byteCursor values and keeps the bytecursor-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns int so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func (c *byteCursor) remaining() int { return len(c.data) - c.off }

// byte operates on byteCursor values and keeps the bytecursor-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns byte and an error. Callers should only trust the value when the
// error is nil.
func (c *byteCursor) byte() (byte, error) {
	if c.off >= len(c.data) {
		return 0, fmt.Errorf("unexpected end of encoded value")
	}
	b := c.data[c.off]
	c.off++
	return b, nil
}

// take operates on byteCursor values and keeps the bytecursor-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns []byte and an error. Callers should only trust the value when the
// error is nil.
func (c *byteCursor) take(n int) ([]byte, error) {
	if n < 0 || c.off+n > len(c.data) {
		return nil, fmt.Errorf("unexpected end of encoded value")
	}
	out := c.data[c.off : c.off+n]
	c.off += n
	return out, nil
}

// uvarint operates on byteCursor values and keeps the bytecursor-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns uint64 and an error. Callers should only trust the value when the
// error is nil.
func (c *byteCursor) uvarint() (uint64, error) {
	v, n := binary.Uvarint(c.data[c.off:])
	if n <= 0 {
		return 0, fmt.Errorf("invalid compact integer encoding")
	}
	c.off += n
	return v, nil
}

func (c *byteCursor) varint() (int64, error) {
	v, n := binary.Varint(c.data[c.off:])
	if n <= 0 {
		return 0, fmt.Errorf("invalid compact signed integer encoding")
	}
	c.off += n
	return v, nil
}

// encodeValue handles the encode value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func encodeValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("encode expects 1 argument")
	}
	raw, err := encodeValueBytes(args[0])
	if err != nil {
		return nullValue(), err
	}
	return stringValue(string(raw)), nil
}

// decodeValue handles the decode value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func decodeValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("decode expects 1 argument")
	}
	data, err := bytesFromValue(args[0])
	if err != nil {
		return nullValue(), err
	}
	return decodeValueBytes(data)
}

// encodeValueBytes handles the encode value bytes step in the surrounding
// package and returns []byte, error.
//
// It returns []byte and an error. Callers should only trust the value when the
// error is nil.
func encodeValueBytes(v Value) ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, 16))
	buf.WriteString(codecHeader)
	if err := encodeInto(buf, v); err != nil {
		return nil, err
	}
	return append([]byte(nil), buf.Bytes()...), nil
}

// encodeInto handles the encode into step in the surrounding package and
// returns error.
//
// It returns an error when the operation cannot complete cleanly.
func encodeInto(buf *bytes.Buffer, v Value) error {
	switch v.kind {
	case kindNull:
		return buf.WriteByte(0x00)
	case kindBool:
		if v.boo {
			return buf.WriteByte(0x02)
		}
		return buf.WriteByte(0x01)
	case kindNumber:
		if v.bfloat {
			if err := buf.WriteByte(0x12); err != nil {
				return err
			}
			if err := buf.WriteByte(byte(v.fbits)); err != nil {
				return err
			}
			var tmp [8]byte
			binary.BigEndian.PutUint64(tmp[:], math.Float64bits(v.num))
			_, err := buf.Write(tmp[:])
			return err
		}
		if v.fbits != 0 {
			if err := buf.WriteByte(0x10); err != nil {
				return err
			}
			if err := buf.WriteByte(byte(v.fbits)); err != nil {
				return err
			}
		} else if err := buf.WriteByte(0x03); err != nil {
			return err
		}
		var tmp [8]byte
		binary.BigEndian.PutUint64(tmp[:], math.Float64bits(v.num))
		_, err := buf.Write(tmp[:])
		return err
	case kindDecimal:
		if err := buf.WriteByte(0x07); err != nil {
			return err
		}
		text := v.String()
		buf.Write(appendUvarint(nil, uint64(len(text))))
		_, err := buf.WriteString(text)
		return err
	case kindInterval:
		if err := buf.WriteByte(0x13); err != nil {
			return err
		}
		text := "interval[0, 0]"
		if v.iv != nil {
			text = formatInterval(v.iv)
		}
		buf.Write(appendUvarint(nil, uint64(len(text))))
		_, err := buf.WriteString(text)
		return err
	case kindModInt:
		if err := buf.WriteByte(0x14); err != nil {
			return err
		}
		text := "modint[1, 0]"
		if v.mi != nil {
			text = "modint[" + v.mi.mod.String() + ", " + v.mi.val.String() + "]"
		}
		buf.Write(appendUvarint(nil, uint64(len(text))))
		_, err := buf.WriteString(text)
		return err
	case kindFixed:
		if err := buf.WriteByte(0x15); err != nil {
			return err
		}
		text := "fixed[64, 0, 0]"
		if v.fx != nil {
			text = "fixed[" + strconv.FormatUint(uint64(v.fx.width), 10) + ", " + strconv.FormatUint(uint64(v.fx.frac), 10) + ", " + v.fx.raw.String() + "]"
		}
		buf.Write(appendUvarint(nil, uint64(len(text))))
		_, err := buf.WriteString(text)
		return err
	case kindComplex:
		if v.fbits != 0 {
			if err := buf.WriteByte(0x18); err != nil {
				return err
			}
			if err := buf.WriteByte(byte(v.fbits)); err != nil {
				return err
			}
		} else if err := buf.WriteByte(0x08); err != nil {
			return err
		}
		var tmp [16]byte
		binary.BigEndian.PutUint64(tmp[:8], math.Float64bits(real(v.cx)))
		binary.BigEndian.PutUint64(tmp[8:], math.Float64bits(imag(v.cx)))
		_, err := buf.Write(tmp[:])
		return err
	case kindDual:
		if err := buf.WriteByte(0x19); err != nil {
			return err
		}
		var tmp [16]byte
		if v.dual != nil {
			binary.BigEndian.PutUint64(tmp[:8], math.Float64bits(v.dual.primal))
			binary.BigEndian.PutUint64(tmp[8:], math.Float64bits(v.dual.tangent))
		}
		_, err := buf.Write(tmp[:])
		return err
	case kindBigInt:
		if err := buf.WriteByte(0x0b); err != nil {
			return err
		}
		text := "0"
		if v.bi != nil {
			text = v.bi.String()
		}
		buf.Write(appendUvarint(nil, uint64(len(text))))
		_, err := buf.WriteString(text)
		return err
	case kindBigFloat:
		if err := buf.WriteByte(0x0c); err != nil {
			return err
		}
		text := "0"
		if v.bf != nil {
			r, _ := v.bf.Rat(nil)
			text = r.RatString()
		}
		buf.Write(appendUvarint(nil, uint64(len(text))))
		_, err := buf.WriteString(text)
		return err
	case kindInt:
		if err := buf.WriteByte(0x0e); err != nil {
			return err
		}
		width := v.ibits
		if width == 0 {
			width = 64
		}
		if err := buf.WriteByte(byte(width)); err != nil {
			return err
		}
		text := "0"
		if v.i != nil {
			text = v.i.String()
		}
		buf.Write(appendUvarint(nil, uint64(len(text))))
		_, err := buf.WriteString(text)
		return err
	case kindQuantity:
		if err := buf.WriteByte(0x0d); err != nil {
			return err
		}
		mag := "0"
		var dims map[string]int
		if v.qty != nil {
			if v.qty.mag != nil {
				mag = v.qty.mag.RatString()
			}
			dims = v.qty.dims
		}
		buf.Write(appendUvarint(nil, uint64(len(mag))))
		if _, err := buf.WriteString(mag); err != nil {
			return err
		}
		buf.Write(appendUvarint(nil, uint64(len(dims))))
		keys := make([]string, 0, len(dims))
		for k := range dims {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			buf.Write(appendUvarint(nil, uint64(len(k))))
			if _, err := buf.WriteString(k); err != nil {
				return err
			}
			var tmp [binary.MaxVarintLen64]byte
			n := binary.PutVarint(tmp[:], int64(dims[k]))
			if _, err := buf.Write(tmp[:n]); err != nil {
				return err
			}
		}
		return nil
	case kindTensor:
		if err := buf.WriteByte(0x09); err != nil {
			return err
		}
		buf.Write(appendUvarint(nil, uint64(len(v.shape))))
		for _, dim := range v.shape {
			buf.Write(appendUvarint(nil, uint64(dim)))
		}
		buf.Write(appendUvarint(nil, uint64(len(v.data))))
		for i := range v.data {
			if err := encodeInto(buf, v.data[i]); err != nil {
				return err
			}
		}
		return nil
	case kindError:
		return encodeErrorRecord(buf, v.errorRecord())
	case kindResult:
		if err := buf.WriteByte(0x16); err != nil {
			return err
		}
		rec := v.resultRecord()
		if rec == nil || !rec.ok {
			if err := buf.WriteByte(0); err != nil {
				return err
			}
			if rec != nil && rec.err != nil {
				return encodeErrorRecord(buf, rec.err)
			}
			return encodeErrorRecord(buf, newOsirisError(ErrorKindUnknown, ""))
		}
		if err := buf.WriteByte(1); err != nil {
			return err
		}
		return encodeInto(buf, rec.val)
	case kindOption:
		if err := buf.WriteByte(0x17); err != nil {
			return err
		}
		rec := v.optionRecord()
		if rec == nil || !rec.present {
			return buf.WriteByte(0)
		}
		if err := buf.WriteByte(1); err != nil {
			return err
		}
		return encodeInto(buf, rec.val)
	case kindUint:
		if v.packed {
			if err := buf.WriteByte(0x11); err != nil {
				return err
			}
			width := v.ubitsToUint16()
			if width == 0 {
				width = uint16(v.bits)
			}
			if width == 0 {
				width = 64
			}
			buf.Write(appendUvarint(nil, uint64(width)))
			if v.native {
				if err := buf.WriteByte(1); err != nil {
					return err
				}
			} else if err := buf.WriteByte(0); err != nil {
				return err
			}
			if v.ub != nil && width > 64 {
				if err := buf.WriteByte(1); err != nil {
					return err
				}
				text := v.ub.String()
				buf.Write(appendUvarint(nil, uint64(len(text))))
				_, err := buf.WriteString(text)
				return err
			}
			if err := buf.WriteByte(0); err != nil {
				return err
			}
			var tmp [8]byte
			if v.ub != nil {
				binary.BigEndian.PutUint64(tmp[:], v.ub.Uint64())
			} else {
				binary.BigEndian.PutUint64(tmp[:], v.u)
			}
			_, err := buf.Write(tmp[:])
			return err
		}
		if v.ub != nil && v.ubitsToUint16() > 64 {
			if err := buf.WriteByte(0x0f); err != nil {
				return err
			}
			buf.Write(appendUvarint(nil, uint64(v.ubitsToUint16())))
			text := v.ub.String()
			buf.Write(appendUvarint(nil, uint64(len(text))))
			_, err := buf.WriteString(text)
			return err
		}
		if err := buf.WriteByte(0x04); err != nil {
			return err
		}
		width := v.bits
		if width == 0 {
			width = 64
		}
		if err := buf.WriteByte(width); err != nil {
			return err
		}
		var tmp [8]byte
		if v.ub != nil {
			binary.BigEndian.PutUint64(tmp[:], v.ub.Uint64())
		} else {
			binary.BigEndian.PutUint64(tmp[:], v.u)
		}
		_, err := buf.Write(tmp[:])
		return err
	case kindString:
		if err := buf.WriteByte(0x05); err != nil {
			return err
		}
		buf.Write(appendUvarint(nil, uint64(len(v.str))))
		_, err := buf.WriteString(v.str)
		return err
	case kindList:
		if err := buf.WriteByte(0x06); err != nil {
			return err
		}
		buf.Write(appendUvarint(nil, uint64(len(v.list))))
		for i := range v.list {
			if err := encodeInto(buf, v.list[i]); err != nil {
				return err
			}
		}
		return nil
	case kindDuration:
		if err := buf.WriteByte(0x1a); err != nil {
			return err
		}
		var raw [binary.MaxVarintLen64]byte
		n := binary.PutVarint(raw[:], v.duration)
		_, err := buf.Write(raw[:n])
		return err
	case kindTimeLocation:
		if err := buf.WriteByte(0x1b); err != nil {
			return err
		}
		return encodeTimeLocation(buf, v.location)
	case kindDateTime:
		if v.datetime == nil {
			return fmt.Errorf("cannot encode nil datetime")
		}
		if err := buf.WriteByte(0x1c); err != nil {
			return err
		}
		var raw [binary.MaxVarintLen64]byte
		n := binary.PutVarint(raw[:], v.datetime.unixSeconds)
		if _, err := buf.Write(raw[:n]); err != nil {
			return err
		}
		buf.Write(appendUvarint(nil, uint64(v.datetime.nanosecond)))
		return encodeTimeLocation(buf, v.datetime.location)
	case kindTuple:
		if err := buf.WriteByte(0x1d); err != nil {
			return err
		}
		buf.Write(appendUvarint(nil, uint64(len(v.tuple))))
		for _, item := range v.tuple {
			if err := encodeInto(buf, item); err != nil {
				return err
			}
		}
		return nil
	case kindStruct:
		if v.structType == nil || v.structValue == nil {
			return fmt.Errorf("cannot encode incomplete struct value")
		}
		if err := buf.WriteByte(0x1e); err != nil {
			return err
		}
		if err := encodeCodecString(buf, v.structType.name); err != nil {
			return err
		}
		if err := encodeCodecString(buf, v.structType.target); err != nil {
			return err
		}
		if v.structType.newtype {
			if err := buf.WriteByte(1); err != nil {
				return err
			}
		} else if err := buf.WriteByte(0); err != nil {
			return err
		}
		buf.Write(appendUvarint(nil, uint64(len(v.structType.fields))))
		if len(v.structType.fields) != len(v.structValue.fields) {
			return fmt.Errorf("cannot encode struct with mismatched field count")
		}
		for i, field := range v.structType.fields {
			if err := encodeCodecString(buf, field.name); err != nil {
				return err
			}
			if err := encodeCodecString(buf, field.typeName); err != nil {
				return err
			}
			if err := encodeCodecString(buf, field.tag); err != nil {
				return err
			}
			if err := encodeInto(buf, v.structValue.fields[i]); err != nil {
				return err
			}
		}
		return nil
	case kindStream:
		return fmt.Errorf("stream values cannot be encoded")
	default:
		return fmt.Errorf("cannot encode %s", v.kind)
	}
}

// decodeValueBytes handles the decode value bytes step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func decodeValueBytes(data []byte) (Value, error) {
	if len(data) > maxCodecValueBytes {
		return nullValue(), fmt.Errorf("encoded value exceeds %d-byte limit", maxCodecValueBytes)
	}
	cur := byteCursor{data: data}
	head, err := cur.take(len(codecHeader))
	if err != nil {
		return nullValue(), err
	}
	if string(head) != codecHeader {
		return nullValue(), fmt.Errorf("invalid codec header")
	}
	v, err := decodeFrom(&cur)
	if err != nil {
		return nullValue(), err
	}
	if cur.remaining() != 0 {
		return nullValue(), fmt.Errorf("trailing bytes after encoded value")
	}
	return v, nil
}

// decodeFrom handles the decode from step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func decodeFrom(cur *byteCursor) (Value, error) {
	tag, err := cur.byte()
	if err != nil {
		return nullValue(), err
	}
	switch tag {
	case 0x00:
		return nullValue(), nil
	case 0x01:
		return boolValue(false), nil
	case 0x02:
		return boolValue(true), nil
	case 0x03:
		raw, err := cur.take(8)
		if err != nil {
			return nullValue(), err
		}
		return numberValue(math.Float64frombits(binary.BigEndian.Uint64(raw))), nil
	case 0x10:
		width, err := cur.byte()
		if err != nil {
			return nullValue(), err
		}
		raw, err := cur.take(8)
		if err != nil {
			return nullValue(), err
		}
		return floatValue(math.Float64frombits(binary.BigEndian.Uint64(raw)), uint16(width)), nil
	case 0x12:
		width, err := cur.byte()
		if err != nil {
			return nullValue(), err
		}
		raw, err := cur.take(8)
		if err != nil {
			return nullValue(), err
		}
		return bfloatValue(math.Float64frombits(binary.BigEndian.Uint64(raw)), uint16(width)), nil
	case 0x07:
		n, err := cur.uvarint()
		if err != nil {
			return nullValue(), err
		}
		raw, err := cur.take(int(n))
		if err != nil {
			return nullValue(), err
		}
		return decimalValueFromString(string(raw))
	case 0x13:
		n, err := cur.uvarint()
		if err != nil {
			return nullValue(), err
		}
		raw, err := cur.take(int(n))
		if err != nil {
			return nullValue(), err
		}
		text := strings.TrimSpace(string(raw))
		if !strings.HasPrefix(text, "interval[") || !strings.HasSuffix(text, "]") {
			return nullValue(), fmt.Errorf("invalid interval encoding")
		}
		body := strings.TrimSuffix(strings.TrimPrefix(text, "interval["), "]")
		parts := strings.SplitN(body, ",", 2)
		if len(parts) != 2 {
			return nullValue(), fmt.Errorf("invalid interval encoding")
		}
		lo, ok := new(big.Rat).SetString(strings.TrimSpace(parts[0]))
		if !ok {
			return nullValue(), fmt.Errorf("invalid interval encoding")
		}
		hi, ok := new(big.Rat).SetString(strings.TrimSpace(parts[1]))
		if !ok {
			return nullValue(), fmt.Errorf("invalid interval encoding")
		}
		return intervalValueOf(newIntervalValue(lo, hi)), nil
	case 0x14:
		n, err := cur.uvarint()
		if err != nil {
			return nullValue(), err
		}
		raw, err := cur.take(int(n))
		if err != nil {
			return nullValue(), err
		}
		text := strings.TrimSpace(string(raw))
		if !strings.HasPrefix(text, "modint[") || !strings.HasSuffix(text, "]") {
			return nullValue(), fmt.Errorf("invalid modint encoding")
		}
		body := strings.TrimSuffix(strings.TrimPrefix(text, "modint["), "]")
		parts := strings.SplitN(body, ",", 2)
		if len(parts) != 2 {
			return nullValue(), fmt.Errorf("invalid modint encoding")
		}
		mod, ok := new(big.Int).SetString(strings.TrimSpace(parts[0]), 0)
		if !ok {
			return nullValue(), fmt.Errorf("invalid modint encoding")
		}
		val, ok := new(big.Int).SetString(strings.TrimSpace(parts[1]), 0)
		if !ok {
			return nullValue(), fmt.Errorf("invalid modint encoding")
		}
		return modIntValueOf(mod, val)
	case 0x15:
		n, err := cur.uvarint()
		if err != nil {
			return nullValue(), err
		}
		raw, err := cur.take(int(n))
		if err != nil {
			return nullValue(), err
		}
		text := strings.TrimSpace(string(raw))
		if !strings.HasPrefix(text, "fixed[") || !strings.HasSuffix(text, "]") {
			return nullValue(), fmt.Errorf("invalid fixed encoding")
		}
		body := strings.TrimSuffix(strings.TrimPrefix(text, "fixed["), "]")
		parts := strings.SplitN(body, ",", 3)
		if len(parts) != 3 {
			return nullValue(), fmt.Errorf("invalid fixed encoding")
		}
		widthI, ok := new(big.Int).SetString(strings.TrimSpace(parts[0]), 10)
		if !ok || !widthI.IsUint64() {
			return nullValue(), fmt.Errorf("invalid fixed encoding")
		}
		fracI, ok := new(big.Int).SetString(strings.TrimSpace(parts[1]), 10)
		if !ok || !fracI.IsUint64() {
			return nullValue(), fmt.Errorf("invalid fixed encoding")
		}
		rawI, ok := new(big.Int).SetString(strings.TrimSpace(parts[2]), 10)
		if !ok {
			return nullValue(), fmt.Errorf("invalid fixed encoding")
		}
		return fixedValueOf(uint16(widthI.Uint64()), uint16(fracI.Uint64()), rawI)
	case 0x08:
		raw, err := cur.take(16)
		if err != nil {
			return nullValue(), err
		}
		re := math.Float64frombits(binary.BigEndian.Uint64(raw[:8]))
		im := math.Float64frombits(binary.BigEndian.Uint64(raw[8:]))
		return complexValue(complex(re, im)), nil
	case 0x18:
		bits, err := cur.byte()
		if err != nil {
			return nullValue(), err
		}
		if bits != 32 && bits != 64 {
			return nullValue(), fmt.Errorf("invalid complex precision encoding")
		}
		raw, err := cur.take(16)
		if err != nil {
			return nullValue(), err
		}
		re := math.Float64frombits(binary.BigEndian.Uint64(raw[:8]))
		im := math.Float64frombits(binary.BigEndian.Uint64(raw[8:]))
		return complexValueStyled(complex(re, im), uint16(bits)), nil
	case 0x19:
		raw, err := cur.take(16)
		if err != nil {
			return nullValue(), err
		}
		return dualValueOf(math.Float64frombits(binary.BigEndian.Uint64(raw[:8])), math.Float64frombits(binary.BigEndian.Uint64(raw[8:]))), nil
	case 0x0b:
		n, err := cur.uvarint()
		if err != nil {
			return nullValue(), err
		}
		raw, err := cur.take(int(n))
		if err != nil {
			return nullValue(), err
		}
		bi, ok := new(big.Int).SetString(string(raw), 10)
		if !ok {
			return nullValue(), fmt.Errorf("invalid big integer encoding")
		}
		return bigIntValue(bi), nil
	case 0x0c:
		n, err := cur.uvarint()
		if err != nil {
			return nullValue(), err
		}
		raw, err := cur.take(int(n))
		if err != nil {
			return nullValue(), err
		}
		rat, ok := new(big.Rat).SetString(string(raw))
		if !ok {
			return nullValue(), fmt.Errorf("invalid big float encoding")
		}
		return bigFloatValue(new(big.Float).SetRat(rat)), nil
	case 0x0d:
		magLen, err := cur.uvarint()
		if err != nil {
			return nullValue(), err
		}
		raw, err := cur.take(int(magLen))
		if err != nil {
			return nullValue(), err
		}
		mag, ok := new(big.Rat).SetString(string(raw))
		if !ok {
			return nullValue(), fmt.Errorf("invalid quantity encoding")
		}
		dimsLen, err := cur.uvarint()
		if err != nil {
			return nullValue(), err
		}
		dims := make(map[string]int, dimsLen)
		for i := uint64(0); i < dimsLen; i++ {
			nameLen, err := cur.uvarint()
			if err != nil {
				return nullValue(), err
			}
			nameRaw, err := cur.take(int(nameLen))
			if err != nil {
				return nullValue(), err
			}
			exp, n := binary.Varint(cur.data[cur.off:])
			if n <= 0 {
				return nullValue(), fmt.Errorf("invalid quantity encoding")
			}
			cur.off += n
			dims[string(nameRaw)] = int(exp)
		}
		return quantityValueOf(mag, dims), nil
	case 0x0e:
		bitsWidth, err := cur.byte()
		if err != nil {
			return nullValue(), err
		}
		n, err := cur.uvarint()
		if err != nil {
			return nullValue(), err
		}
		raw, err := cur.take(int(n))
		if err != nil {
			return nullValue(), err
		}
		i, ok := new(big.Int).SetString(string(raw), 10)
		if !ok {
			return nullValue(), fmt.Errorf("invalid signed integer encoding")
		}
		return intValue(i, uint16(bitsWidth)), nil
	case 0x09:
		shapeLen, err := cur.uvarint()
		if err != nil {
			return nullValue(), err
		}
		shape := make([]int, shapeLen)
		for i := range shape {
			n, err := cur.uvarint()
			if err != nil {
				return nullValue(), err
			}
			shape[i] = int(n)
		}
		dataLen, err := cur.uvarint()
		if err != nil {
			return nullValue(), err
		}
		if dataLen != uint64(tensorShapeProduct(shape)) {
			return nullValue(), fmt.Errorf("encoded tensor shape does not match element count")
		}
		data := make([]Value, 0, dataLen)
		for i := uint64(0); i < dataLen; i++ {
			item, err := decodeFrom(cur)
			if err != nil {
				return nullValue(), err
			}
			data = append(data, item)
		}
		return newTensorValue(shape, data), nil
	case 0x0a:
		rec, err := decodeErrorRecord(cur)
		if err != nil {
			return nullValue(), err
		}
		return errorValueFromRecord(rec), nil
	case 0x16:
		present, err := cur.byte()
		if err != nil {
			return nullValue(), err
		}
		if present == 0 {
			rec, err := decodeTaggedErrorRecord(cur)
			if err != nil {
				return nullValue(), err
			}
			return resultErrValue(rec), nil
		}
		val, err := decodeFrom(cur)
		if err != nil {
			return nullValue(), err
		}
		return resultOkValue(val), nil
	case 0x17:
		present, err := cur.byte()
		if err != nil {
			return nullValue(), err
		}
		if present == 0 {
			return optionNoneValue(), nil
		}
		val, err := decodeFrom(cur)
		if err != nil {
			return nullValue(), err
		}
		return optionSomeValue(val), nil
	case 0x04:
		bitsWidth, err := cur.byte()
		if err != nil {
			return nullValue(), err
		}
		raw, err := cur.take(8)
		if err != nil {
			return nullValue(), err
		}
		return uintValue(binary.BigEndian.Uint64(raw), bitsWidth), nil
	case 0x0f:
		bitsWidth, err := cur.uvarint()
		if err != nil {
			return nullValue(), err
		}
		n, err := cur.uvarint()
		if err != nil {
			return nullValue(), err
		}
		raw, err := cur.take(int(n))
		if err != nil {
			return nullValue(), err
		}
		bi, ok := new(big.Int).SetString(string(raw), 10)
		if !ok {
			return nullValue(), fmt.Errorf("invalid wide unsigned integer encoding")
		}
		return wideUintValue(bi, uint16(bitsWidth)), nil
	case 0x11:
		bitsWidth, err := cur.uvarint()
		if err != nil {
			return nullValue(), err
		}
		nativeFlag, err := cur.byte()
		if err != nil {
			return nullValue(), err
		}
		mode, err := cur.byte()
		if err != nil {
			return nullValue(), err
		}
		native := nativeFlag != 0
		if mode == 1 {
			n, err := cur.uvarint()
			if err != nil {
				return nullValue(), err
			}
			raw, err := cur.take(int(n))
			if err != nil {
				return nullValue(), err
			}
			bi, ok := new(big.Int).SetString(string(raw), 10)
			if !ok {
				return nullValue(), fmt.Errorf("invalid packed wide unsigned integer encoding")
			}
			return packedWideUintValue(bi, uint16(bitsWidth), native), nil
		}
		raw, err := cur.take(8)
		if err != nil {
			return nullValue(), err
		}
		return packedUintValue(binary.BigEndian.Uint64(raw), uint8(bitsWidth), native), nil
	case 0x05:
		n, err := cur.uvarint()
		if err != nil {
			return nullValue(), err
		}
		raw, err := cur.take(int(n))
		if err != nil {
			return nullValue(), err
		}
		return stringValue(string(raw)), nil
	case 0x06:
		n, err := cur.uvarint()
		if err != nil {
			return nullValue(), err
		}
		items := make([]Value, 0, n)
		for i := uint64(0); i < n; i++ {
			item, err := decodeFrom(cur)
			if err != nil {
				return nullValue(), err
			}
			items = append(items, item)
		}
		return listValue(items), nil
	case 0x1a:
		value, err := cur.varint()
		if err != nil {
			return nullValue(), err
		}
		return durationValue(value), nil
	case 0x1b:
		location, err := decodeTimeLocation(cur)
		if err != nil {
			return nullValue(), err
		}
		return timeLocationValueOf(location), nil
	case 0x1c:
		seconds, err := cur.varint()
		if err != nil {
			return nullValue(), err
		}
		nanosecond, err := cur.uvarint()
		if err != nil {
			return nullValue(), err
		}
		if nanosecond > 999999999 {
			return nullValue(), fmt.Errorf("invalid datetime nanosecond encoding")
		}
		location, err := decodeTimeLocation(cur)
		if err != nil {
			return nullValue(), err
		}
		return dateTimeValueFromTime(time.Unix(seconds, int64(nanosecond)).In(location.goLocation()), location)
	case 0x1d:
		n, err := cur.uvarint()
		if err != nil {
			return nullValue(), err
		}
		if n > uint64(cur.remaining()) {
			return nullValue(), fmt.Errorf("invalid tuple element count")
		}
		items := make([]Value, 0, n)
		for i := uint64(0); i < n; i++ {
			item, err := decodeFrom(cur)
			if err != nil {
				return nullValue(), err
			}
			items = append(items, item)
		}
		return tupleValue(items), nil
	case 0x1e:
		name, err := decodeCodecString(cur)
		if err != nil {
			return nullValue(), err
		}
		target, err := decodeCodecString(cur)
		if err != nil {
			return nullValue(), err
		}
		newtypeFlag, err := cur.byte()
		if err != nil {
			return nullValue(), err
		}
		count, err := cur.uvarint()
		if err != nil {
			return nullValue(), err
		}
		if count > uint64(cur.remaining()) {
			return nullValue(), fmt.Errorf("invalid struct field count")
		}
		typ := &structTypeValue{name: name, target: target, newtype: newtypeFlag != 0, fields: make([]structFieldValue, 0, count), byName: make(map[string]int)}
		values := make([]Value, 0, count)
		for i := uint64(0); i < count; i++ {
			fieldName, err := decodeCodecString(cur)
			if err != nil {
				return nullValue(), err
			}
			fieldType, err := decodeCodecString(cur)
			if err != nil {
				return nullValue(), err
			}
			tagText, err := decodeCodecString(cur)
			if err != nil {
				return nullValue(), err
			}
			tagValues, tagErr := parseStructTag(tagText)
			if tagErr != nil {
				return nullValue(), fmt.Errorf("invalid encoded struct tag: %w", tagErr)
			}
			if _, duplicate := typ.byName[fieldName]; duplicate {
				return nullValue(), fmt.Errorf("duplicate encoded struct field %q", fieldName)
			}
			typ.byName[fieldName] = len(typ.fields)
			typ.fields = append(typ.fields, structFieldValue{name: fieldName, typeName: fieldType, tag: tagText, tagValues: tagValues})
			value, err := decodeFrom(cur)
			if err != nil {
				return nullValue(), err
			}
			values = append(values, value)
		}
		return structInstanceValueOf(typ, values), nil
	default:
		return nullValue(), fmt.Errorf("unknown encoded tag 0x%02x", tag)
	}
}

func encodeCodecString(buf *bytes.Buffer, text string) error {
	buf.Write(appendUvarint(nil, uint64(len(text))))
	_, err := buf.WriteString(text)
	return err
}

func decodeCodecString(cur *byteCursor) (string, error) {
	n, err := cur.uvarint()
	if err != nil {
		return "", err
	}
	raw, err := cur.take(int(n))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func encodeTimeLocation(buf *bytes.Buffer, location *timeLocationValue) error {
	if location == nil {
		location = utcTimeLocation()
	}
	if err := buf.WriteByte(byte(location.class)); err != nil {
		return err
	}
	buf.Write(appendUvarint(nil, uint64(len(location.name))))
	if _, err := buf.WriteString(location.name); err != nil {
		return err
	}
	if location.class == timeLocationFixed {
		var raw [binary.MaxVarintLen64]byte
		n := binary.PutVarint(raw[:], int64(location.offset))
		if _, err := buf.Write(raw[:n]); err != nil {
			return err
		}
	}
	return nil
}

func decodeTimeLocation(cur *byteCursor) (*timeLocationValue, error) {
	class, err := cur.byte()
	if err != nil {
		return nil, err
	}
	nameLength, err := cur.uvarint()
	if err != nil {
		return nil, err
	}
	name, err := cur.take(int(nameLength))
	if err != nil {
		return nil, err
	}
	switch timeLocationClass(class) {
	case timeLocationUTC:
		if string(name) != "UTC" {
			return nil, fmt.Errorf("invalid UTC location encoding")
		}
		return utcTimeLocation(), nil
	case timeLocationFixed:
		offset, err := cur.varint()
		if err != nil {
			return nil, err
		}
		if len(name) == 0 || offset <= -24*60*60 || offset >= 24*60*60 {
			return nil, fmt.Errorf("invalid fixed timezone encoding")
		}
		return timeLocationValueOfFixed(string(name), int(offset)), nil
	case timeLocationIANA:
		if len(name) == 0 {
			return nil, fmt.Errorf("invalid IANA timezone encoding")
		}
		location, err := time.LoadLocation(string(name))
		if err != nil {
			return nil, fmt.Errorf("unavailable encoded IANA timezone %q: %w", name, err)
		}
		return timeLocationValueOfIANA(string(name), location), nil
	default:
		return nil, fmt.Errorf("invalid timezone class encoding")
	}
}

// appendUvarint handles the append uvarint step in the surrounding package and
// returns []byte.
//
// It returns []byte so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func appendUvarint(dst []byte, n uint64) []byte {
	var tmp [10]byte
	sz := binary.PutUvarint(tmp[:], n)
	return append(dst, tmp[:sz]...)
}
