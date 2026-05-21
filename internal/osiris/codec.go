package osiris

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
)

const codecHeader = "OSR1"

type byteCursor struct {
	data []byte
	off  int
}

func (c *byteCursor) remaining() int { return len(c.data) - c.off }

func (c *byteCursor) byte() (byte, error) {
	if c.off >= len(c.data) {
		return 0, fmt.Errorf("unexpected end of encoded value")
	}
	b := c.data[c.off]
	c.off++
	return b, nil
}

func (c *byteCursor) take(n int) ([]byte, error) {
	if n < 0 || c.off+n > len(c.data) {
		return nil, fmt.Errorf("unexpected end of encoded value")
	}
	out := c.data[c.off : c.off+n]
	c.off += n
	return out, nil
}

func (c *byteCursor) uvarint() (uint64, error) {
	v, n := binary.Uvarint(c.data[c.off:])
	if n <= 0 {
		return 0, fmt.Errorf("invalid compact integer encoding")
	}
	c.off += n
	return v, nil
}

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

func encodeValueBytes(v Value) ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, 16))
	buf.WriteString(codecHeader)
	if err := encodeInto(buf, v); err != nil {
		return nil, err
	}
	return append([]byte(nil), buf.Bytes()...), nil
}

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
		if err := buf.WriteByte(0x03); err != nil {
			return err
		}
		var tmp [8]byte
		binary.BigEndian.PutUint64(tmp[:], math.Float64bits(v.num))
		_, err := buf.Write(tmp[:])
		return err
	case kindUint:
		if err := buf.WriteByte(0x04); err != nil {
			return err
		}
		if err := buf.WriteByte(v.bits); err != nil {
			return err
		}
		var tmp [8]byte
		binary.BigEndian.PutUint64(tmp[:], v.u)
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
	default:
		return fmt.Errorf("cannot encode %s", v.kind)
	}
}

func decodeValueBytes(data []byte) (Value, error) {
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
	default:
		return nullValue(), fmt.Errorf("unknown encoded tag 0x%02x", tag)
	}
}

func appendUvarint(dst []byte, n uint64) []byte {
	var tmp [10]byte
	sz := binary.PutUvarint(tmp[:], n)
	return append(dst, tmp[:sz]...)
}
