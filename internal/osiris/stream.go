package osiris

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
)

type streamValue struct {
	mu     sync.Mutex
	id     string
	path   string
	mode   string
	file   *os.File
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
	closed bool
}

// openReadStream opens a readable file stream for chunked and line-oriented
// reads. The returned handle stays valid until close is called, and repeated
// reads advance the underlying file cursor without buffering the entire file
// into memory.
func openReadStream(path string) (*streamValue, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &streamValue{
		id:     path,
		path:   path,
		mode:   "read",
		file:   f,
		reader: bufio.NewReader(f),
	}, nil
}

// openReadStreamValue opens a readable file stream and returns it as an Osiris
// value. It is a thin wrapper around openReadStream that keeps the builtin
// surface small and predictable.
func openReadStreamValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("openread expects 1 argument")
	}
	path, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	stream, err := openReadStream(path)
	if err != nil {
		return nullValue(), err
	}
	return streamValueOf(stream), nil
}

// openWriteStream opens a writable file stream. The underlying file is
// truncated by default so callers get a clean output target that can be filled
// incrementally with writechunk calls and flushed explicitly when needed.
func openWriteStream(path string, appendMode bool) (*streamValue, error) {
	flag := os.O_CREATE | os.O_WRONLY
	if appendMode {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(path, flag, 0o644)
	if err != nil {
		return nil, err
	}
	return &streamValue{
		id:     path,
		path:   path,
		mode:   "write",
		file:   f,
		writer: bufio.NewWriter(f),
	}, nil
}

// openWriteStreamValue opens a writable file stream and returns it as an
// Osiris value.
func openWriteStreamValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("openwrite expects 1 argument")
	}
	path, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	stream, err := openWriteStream(path, false)
	if err != nil {
		return nullValue(), err
	}
	return streamValueOf(stream), nil
}

// readLine reads a single line from the stream without the trailing newline.
// It returns io.EOF as a null value so call sites can distinguish end-of-file
// from a legitimate empty line while still using the ordinary Osiris value
// model.
func (s *streamValue) readLine() (Value, error) {
	if s == nil {
		return nullValue(), fmt.Errorf("expected stream, got null")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureOpen(); err != nil {
		return nullValue(), err
	}
	if s.reader == nil {
		return nullValue(), fmt.Errorf("stream %s is not readable", s.path)
	}
	line, err := s.reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nullValue(), err
	}
	if line == "" && err == io.EOF {
		return nullValue(), io.EOF
	}
	line = trimLineEnding(line)
	if err == io.EOF && line == "" {
		return nullValue(), io.EOF
	}
	return stringValue(line), nil
}

// readLineValue reads a single line from an Osiris stream handle and returns
// null when the stream reaches end-of-file.
func readLineValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("readline expects 1 argument")
	}
	stream, err := args[0].AsStream()
	if err != nil {
		return nullValue(), err
	}
	v, err := stream.readLine()
	if err == io.EOF {
		return nullValue(), nil
	}
	return v, err
}

// readChunk reads up to n bytes from the stream and returns them as a list of
// byte values. It stops at the current file position, so repeated calls walk
// forward through the file without rewinding.
func (s *streamValue) readChunk(n int) (Value, error) {
	if s == nil {
		return nullValue(), fmt.Errorf("expected stream, got null")
	}
	if n < 0 {
		return nullValue(), fmt.Errorf("readchunk expects a non-negative size")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureOpen(); err != nil {
		return nullValue(), err
	}
	if s.reader == nil {
		return nullValue(), fmt.Errorf("stream %s is not readable", s.path)
	}
	if n == 0 {
		return listValue(nil), nil
	}
	buf := make([]byte, n)
	read, err := s.reader.Read(buf)
	if err != nil && err != io.EOF {
		return nullValue(), err
	}
	if read == 0 && err == io.EOF {
		return nullValue(), io.EOF
	}
	out := make([]Value, read)
	for i := 0; i < read; i++ {
		out[i] = uintValue(uint64(buf[i]), 8)
	}
	return listValue(out), nil
}

// readChunkValue reads up to n bytes from an Osiris stream handle and returns
// them as a list of byte values. It returns null at end-of-file.
func readChunkValue(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("readchunk expects 2 arguments")
	}
	stream, err := args[0].AsStream()
	if err != nil {
		return nullValue(), err
	}
	n, err := args[1].Int()
	if err != nil {
		return nullValue(), err
	}
	v, err := stream.readChunk(n)
	if err == io.EOF {
		return nullValue(), nil
	}
	return v, err
}

// writeChunk writes a byte-compatible value to the stream and returns the
// number of bytes accepted by the underlying writer. The function preserves
// incremental output behavior so large results can be streamed without holding
// the entire payload in memory.
func (s *streamValue) writeChunk(v Value) (Value, error) {
	if s == nil {
		return nullValue(), fmt.Errorf("expected stream, got null")
	}
	data, err := bytesFromValue(v)
	if err != nil {
		return nullValue(), err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureOpen(); err != nil {
		return nullValue(), err
	}
	if s.writer == nil {
		return nullValue(), fmt.Errorf("stream %s is not writable", s.path)
	}
	n, err := s.writer.Write(data)
	if err != nil {
		return nullValue(), err
	}
	return uintValue(uint64(n), 64), nil
}

// writeChunkValue writes a byte-compatible value to an Osiris stream handle
// and returns the number of bytes written.
func writeChunkValue(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("writechunk expects 2 arguments")
	}
	stream, err := args[0].AsStream()
	if err != nil {
		return nullValue(), err
	}
	return stream.writeChunk(args[1])
}

// flush writes any buffered output to the underlying file descriptor.
func (s *streamValue) flush() (Value, error) {
	if s == nil {
		return nullValue(), fmt.Errorf("expected stream, got null")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureOpen(); err != nil {
		return nullValue(), err
	}
	if s.writer == nil {
		return boolValue(true), nil
	}
	if err := s.writer.Flush(); err != nil {
		return nullValue(), err
	}
	return boolValue(true), nil
}

// flushStreamValue flushes the buffered writer attached to a stream handle.
func flushStreamValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("flush expects 1 argument")
	}
	stream, err := args[0].AsStream()
	if err != nil {
		return nullValue(), err
	}
	return stream.flush()
}

// close releases the underlying file descriptor. The handle remains printable
// after close, but further read or write operations will fail.
func (s *streamValue) close() (Value, error) {
	if s == nil {
		return boolValue(false), fmt.Errorf("expected stream, got null")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return boolValue(false), nil
	}
	if s.writer != nil {
		if err := s.writer.Flush(); err != nil {
			return nullValue(), err
		}
	}
	if s.file != nil {
		if err := s.file.Close(); err != nil {
			return nullValue(), err
		}
	}
	if s.conn != nil {
		if err := s.conn.Close(); err != nil {
			return nullValue(), err
		}
	}
	s.closed = true
	return boolValue(true), nil
}

// closeStreamValue closes the underlying file descriptor for a stream handle.
func closeStreamValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("close expects 1 argument")
	}
	stream, err := args[0].AsStream()
	if err != nil {
		return nullValue(), err
	}
	return stream.close()
}

// ensureOpen validates that the stream has not already been closed.
func (s *streamValue) ensureOpen() error {
	if s.closed {
		return fmt.Errorf("stream %s is closed", s.path)
	}
	return nil
}

// String renders the stream handle in a short, human-readable form that is
// stable enough for tests and diagnostics.
func (s *streamValue) String() string {
	if s == nil {
		return "<stream>"
	}
	return "<stream " + s.mode + " " + s.path + ">"
}

// toInterface converts the stream handle into a diagnostic-friendly JSON
// object without exposing the live file descriptor itself.
func (s *streamValue) toInterface() any {
	if s == nil {
		return nil
	}
	return map[string]any{"id": s.id, "path": s.path, "mode": s.mode, "closed": s.closed}
}

func trimLineEnding(s string) string {
	if len(s) == 0 {
		return s
	}
	if s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	if len(s) > 0 && s[len(s)-1] == '\r' {
		s = s[:len(s)-1]
	}
	return s
}
