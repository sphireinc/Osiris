package osiris

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type WASMArtifact struct {
	WAT    string
	Binary []byte
	JSON   string
}

func CompileWASM(ctx context.Context, rt *Runtime, prog *Program) (*WASMArtifact, error) {
	sess := NewSession(rt, ctx, prog.SourceName)
	outputs, err := sess.ExecProgram(prog, false, nil)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(outputs)
	if err != nil {
		return nil, err
	}
	artifact := &WASMArtifact{
		JSON:   string(payload),
		WAT:    buildWATSnapshot(prog.SourceName, payload),
		Binary: buildWASMSnapshot(payload),
	}
	return artifact, nil
}

func buildWATSnapshot(sourceName string, payload []byte) string {
	pages := wasmPages(len(payload))
	var b strings.Builder
	b.WriteString("(module\n")
	b.WriteString("  ;; osiris snapshot from ")
	b.WriteString(sourceName)
	b.WriteString("\n")
	b.WriteString("  (memory (export \"memory\") ")
	b.WriteString(strconv.Itoa(pages))
	b.WriteString(")\n")
	b.WriteString("  (global (export \"result_ptr\") i32 (i32.const 0))\n")
	b.WriteString("  (global (export \"result_len\") i32 (i32.const ")
	b.WriteString(strconv.Itoa(len(payload)))
	b.WriteString("))\n")
	b.WriteString("  (func (export \"run\"))\n")
	b.WriteString("  (data (i32.const 0) \"")
	b.WriteString(watEscapeBytes(payload))
	b.WriteString("\")\n")
	b.WriteString(")\n")
	return b.String()
}

func buildWASMSnapshot(payload []byte) []byte {
	pages := wasmPages(len(payload))
	var out bytes.Buffer
	out.Write([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00})

	writeWASMSection(&out, 1, func(buf *bytes.Buffer) {
		writeULEB(buf, 1)
		buf.WriteByte(0x60)
		writeULEB(buf, 0)
		writeULEB(buf, 0)
	})

	writeWASMSection(&out, 3, func(buf *bytes.Buffer) {
		writeULEB(buf, 1)
		writeULEB(buf, 0)
	})

	writeWASMSection(&out, 5, func(buf *bytes.Buffer) {
		writeULEB(buf, 1)
		buf.WriteByte(0x00)
		writeULEB(buf, uint64(pages))
	})

	writeWASMSection(&out, 6, func(buf *bytes.Buffer) {
		writeULEB(buf, 2)
		writeWASMGlobal(buf, 0)
		writeWASMGlobal(buf, int32(len(payload)))
	})

	writeWASMSection(&out, 7, func(buf *bytes.Buffer) {
		writeULEB(buf, 4)
		writeWASMExport(buf, "memory", 0x02, 0)
		writeWASMExport(buf, "result_ptr", 0x03, 0)
		writeWASMExport(buf, "result_len", 0x03, 1)
		writeWASMExport(buf, "run", 0x00, 0)
	})

	writeWASMSection(&out, 10, func(buf *bytes.Buffer) {
		writeULEB(buf, 1)
		var body bytes.Buffer
		writeULEB(&body, 0)
		body.WriteByte(0x0b)
		writeULEB(buf, uint64(body.Len()))
		buf.Write(body.Bytes())
	})

	writeWASMSection(&out, 11, func(buf *bytes.Buffer) {
		writeULEB(buf, 1)
		writeULEB(buf, 0)
		writeWASMConstExpr(buf, 0)
		writeULEB(buf, uint64(len(payload)))
		buf.Write(payload)
	})

	return out.Bytes()
}

func writeWASMGlobal(buf *bytes.Buffer, value int32) {
	buf.WriteByte(0x7f)
	buf.WriteByte(0x00)
	writeWASMConstExpr(buf, value)
}

func writeWASMConstExpr(buf *bytes.Buffer, value int32) {
	buf.WriteByte(0x41)
	writeSLEB(buf, int64(value))
	buf.WriteByte(0x0b)
}

func writeWASMExport(buf *bytes.Buffer, name string, kind byte, index uint64) {
	writeULEB(buf, uint64(len(name)))
	buf.WriteString(name)
	buf.WriteByte(kind)
	writeULEB(buf, index)
}

func writeWASMSection(buf *bytes.Buffer, id byte, build func(*bytes.Buffer)) {
	var payload bytes.Buffer
	build(&payload)
	buf.WriteByte(id)
	writeULEB(buf, uint64(payload.Len()))
	buf.Write(payload.Bytes())
}

func writeULEB(buf *bytes.Buffer, v uint64) {
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		buf.WriteByte(b)
		if v == 0 {
			return
		}
	}
}

func writeSLEB(buf *bytes.Buffer, v int64) {
	more := true
	for more {
		b := byte(v & 0x7f)
		v >>= 7
		signBit := b & 0x40
		more = !((v == 0 && signBit == 0) || (v == -1 && signBit != 0))
		if more {
			b |= 0x80
		}
		buf.WriteByte(b)
	}
}

func watEscapeBytes(payload []byte) string {
	var b strings.Builder
	for _, c := range payload {
		switch c {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteByte(c)
		case '\n':
			b.WriteString(`\0a`)
		case '\r':
			b.WriteString(`\0d`)
		case '\t':
			b.WriteString(`\09`)
		default:
			if c >= 0x20 && c <= 0x7e {
				b.WriteByte(c)
			} else {
				b.WriteString(`\`)
				b.WriteString(fmt.Sprintf("%02x", c))
			}
		}
	}
	return b.String()
}

func wasmPages(n int) int {
	if n <= 0 {
		return 1
	}
	const page = 65536
	return (n + page - 1) / page
}
