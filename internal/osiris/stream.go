package osiris

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

type streamValue struct {
	mu      sync.Mutex
	readMu  sync.Mutex
	writeMu sync.Mutex
	id      string
	path    string
	mode    string
	file    *os.File
	conn    net.Conn
	reader  *bufio.Reader
	writer  *bufio.Writer
	closer  io.Closer
	closed  bool
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
	path, err := filesystemPathArgument(args[0], "openread")
	if err != nil {
		return nullValue(), err
	}
	stream, err := openReadStream(path)
	if err != nil {
		return nullValue(), filesystemHostFailure("openread", path, err)
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
	mode := "write"
	if appendMode {
		mode = "append"
		if _, err := f.Seek(0, io.SeekEnd); err != nil {
			_ = f.Close()
			return nil, err
		}
	}
	return &streamValue{
		id:     path,
		path:   path,
		mode:   mode,
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
	path, err := filesystemPathArgument(args[0], "openwrite")
	if err != nil {
		return nullValue(), err
	}
	stream, err := openWriteStream(path, false)
	if err != nil {
		return nullValue(), filesystemHostFailure("openwrite", path, err)
	}
	return streamValueOf(stream), nil
}

// openAppendStreamValue opens an owned writable stream whose writes always
// target the current end of the file. The initial logical cursor is the file
// size so tell is deterministic before the first write.
func openAppendStreamValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, "openappend", "", "openappend expects 1 argument", nil)
	}
	path, err := filesystemPathArgument(args[0], "openappend")
	if err != nil {
		return nullValue(), err
	}
	stream, err := openWriteStream(path, true)
	if err != nil {
		return nullValue(), filesystemHostFailure("openappend", path, err)
	}
	return streamValueOf(stream), nil
}

func openAppendStreamContextValue(ctx context.Context, args []Value) (Value, error) {
	if len(args) == 1 && args[0].kind == kindString {
		if err := filesystemContext(ctx, "openappend", args[0].str); err != nil {
			return nullValue(), err
		}
	}
	return openAppendStreamValue(args)
}

// seek repositions an owned file stream. Buffered readers are first aligned
// with their logical cursor and buffered writers are flushed, so "current"
// always means the position visible to Osiris code.
func (s *streamValue) seek(offset int64, origin string) (int64, error) {
	if s == nil {
		return 0, filesystemFailure(ErrorKindType, "seek", "", "seek expects a stream", nil)
	}
	whence := 0
	switch origin {
	case "start":
		whence = io.SeekStart
	case "current":
		whence = io.SeekCurrent
	case "end":
		whence = io.SeekEnd
	default:
		return 0, filesystemFailure(ErrorKindInvalidArgument, "seek", s.path, `seek origin must be "start", "current", or "end"`, nil)
	}
	s.readMu.Lock()
	defer s.readMu.Unlock()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.mu.Lock()
	if err := s.ensureOpen(); err != nil {
		s.mu.Unlock()
		return 0, err
	}
	file, reader, writer, path := s.file, s.reader, s.writer, s.path
	s.mu.Unlock()
	if file == nil || s.conn != nil {
		return 0, filesystemFailure(ErrorKindUnsupported, "seek", path, "seek requires a file stream", nil)
	}
	if writer != nil {
		if err := writer.Flush(); err != nil {
			return 0, filesystemHostFailure("seek", path, err)
		}
	}
	if reader != nil && reader.Buffered() > 0 {
		if _, err := file.Seek(-int64(reader.Buffered()), io.SeekCurrent); err != nil {
			return 0, filesystemHostFailure("seek", path, err)
		}
		reader.Reset(file)
	}
	position, err := file.Seek(offset, whence)
	if err != nil {
		return 0, filesystemHostFailure("seek", path, err)
	}
	if position < 0 {
		return 0, filesystemFailure(ErrorKindInvalidArgument, "seek", path, "seek cannot produce a negative position", nil)
	}
	if reader != nil {
		reader.Reset(file)
	}
	return position, nil
}

func seekContextValue(ctx context.Context, args []Value) (Value, error) {
	const operation = "seek"
	if len(args) != 3 {
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, operation, "", "seek expects a stream, offset, and origin", nil)
	}
	stream, err := args[0].AsStream()
	if err != nil {
		return nullValue(), filesystemFailure(ErrorKindType, operation, "", "seek expects a stream", nil)
	}
	offset, err := filesystemIntegerArgument(args[1], operation, "offset")
	if err != nil {
		return nullValue(), err
	}
	if args[2].kind != kindString {
		return nullValue(), filesystemFailure(ErrorKindType, operation, stream.path, "seek origin must be a string", nil)
	}
	if err := filesystemContext(ctx, operation, stream.path); err != nil {
		return nullValue(), err
	}
	position, err := stream.seek(offset, args[2].str)
	if err != nil {
		return nullValue(), err
	}
	return uintValue(uint64(position), 64), nil
}

func seekValue(args []Value) (Value, error) {
	return seekContextValue(context.Background(), args)
}

// tell reports the logical byte position, compensating for read-ahead and
// flushing buffered writes before consulting the file descriptor.
func (s *streamValue) tell() (int64, error) {
	if s == nil {
		return 0, filesystemFailure(ErrorKindType, "tell", "", "tell expects a stream", nil)
	}
	s.readMu.Lock()
	defer s.readMu.Unlock()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.mu.Lock()
	if err := s.ensureOpen(); err != nil {
		s.mu.Unlock()
		return 0, err
	}
	file, reader, writer, path := s.file, s.reader, s.writer, s.path
	s.mu.Unlock()
	if file == nil || s.conn != nil {
		return 0, filesystemFailure(ErrorKindUnsupported, "tell", path, "tell requires a file stream", nil)
	}
	if writer != nil {
		if err := writer.Flush(); err != nil {
			return 0, filesystemHostFailure("tell", path, err)
		}
	}
	position, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, filesystemHostFailure("tell", path, err)
	}
	if reader != nil {
		position -= int64(reader.Buffered())
	}
	if position < 0 {
		return 0, filesystemFailure(ErrorKindIO, "tell", path, "tell produced an invalid negative position", nil)
	}
	return position, nil
}

func tellContextValue(ctx context.Context, args []Value) (Value, error) {
	const operation = "tell"
	if len(args) != 1 {
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, operation, "", "tell expects 1 argument", nil)
	}
	stream, err := args[0].AsStream()
	if err != nil {
		return nullValue(), filesystemFailure(ErrorKindType, operation, "", "tell expects a stream", nil)
	}
	if err := filesystemContext(ctx, operation, stream.path); err != nil {
		return nullValue(), err
	}
	position, err := stream.tell()
	if err != nil {
		return nullValue(), err
	}
	return uintValue(uint64(position), 64), nil
}

func tellValue(args []Value) (Value, error) {
	return tellContextValue(context.Background(), args)
}

// truncate resizes an owned writable file stream without changing its logical
// cursor. It flushes buffered bytes before resizing the underlying file.
func (s *streamValue) truncate(size int64) error {
	if s == nil {
		return filesystemFailure(ErrorKindType, "truncate", "", "truncate expects a stream", nil)
	}
	s.readMu.Lock()
	defer s.readMu.Unlock()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.mu.Lock()
	if err := s.ensureOpen(); err != nil {
		s.mu.Unlock()
		return err
	}
	file, writer, mode, path := s.file, s.writer, s.mode, s.path
	s.mu.Unlock()
	if file == nil || s.conn != nil || (mode != "write" && mode != "append") {
		return filesystemFailure(ErrorKindUnsupported, "truncate", path, "truncate requires a writable file stream", nil)
	}
	if writer != nil {
		if err := writer.Flush(); err != nil {
			return filesystemHostFailure("truncate", path, err)
		}
	}
	if err := file.Truncate(size); err != nil {
		return filesystemHostFailure("truncate", path, err)
	}
	return nil
}

func truncateStreamContextValue(ctx context.Context, args []Value) (Value, error) {
	const operation = "truncate"
	if len(args) != 2 {
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, operation, "", "truncate expects a stream and size", nil)
	}
	stream, err := args[0].AsStream()
	if err != nil {
		return nullValue(), filesystemFailure(ErrorKindType, operation, "", "truncate expects a stream", nil)
	}
	size, err := filesystemNonnegativeArgument(args[1], operation, "size", uint64(^uint64(0)>>1))
	if err != nil {
		return nullValue(), attachFilesystemResource(err, stream.path)
	}
	if err := filesystemContext(ctx, operation, stream.path); err != nil {
		return nullValue(), err
	}
	if err := stream.truncate(int64(size)); err != nil {
		return nullValue(), err
	}
	return boolValue(true), nil
}

func truncateStreamValue(args []Value) (Value, error) {
	return truncateStreamContextValue(context.Background(), args)
}

// readLine reads a single line from the stream without the trailing newline.
// It returns io.EOF as a null value so call sites can distinguish end-of-file
// from a legitimate empty line while still using the ordinary Osiris value
// model.
func (s *streamValue) readLine() (Value, error) {
	if s == nil {
		return nullValue(), filesystemFailure(ErrorKindType, "readline", "", "readline expects a stream", nil)
	}
	s.readMu.Lock()
	defer s.readMu.Unlock()
	s.mu.Lock()
	if err := s.ensureOpen(); err != nil {
		s.mu.Unlock()
		return nullValue(), err
	}
	reader := s.reader
	path := s.path
	s.mu.Unlock()
	if reader == nil {
		return nullValue(), filesystemFailure(ErrorKindUnsupported, "readline", path, "readline requires a readable stream", nil)
	}
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nullValue(), filesystemHostFailure("readline", path, err)
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

func readLineContextValue(ctx context.Context, args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("readline expects 1 argument")
	}
	stream, err := args[0].AsStream()
	if err != nil {
		return nullValue(), err
	}
	v, err := stream.readLineContext(ctx)
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
		return nullValue(), filesystemFailure(ErrorKindType, "readchunk", "", "readchunk expects a stream", nil)
	}
	if n < 0 {
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, "readchunk", s.path, "readchunk expects a non-negative size", nil)
	}
	s.readMu.Lock()
	defer s.readMu.Unlock()
	s.mu.Lock()
	if err := s.ensureOpen(); err != nil {
		s.mu.Unlock()
		return nullValue(), err
	}
	reader := s.reader
	path := s.path
	s.mu.Unlock()
	if reader == nil {
		return nullValue(), filesystemFailure(ErrorKindUnsupported, "readchunk", path, "readchunk requires a readable stream", nil)
	}
	if n == 0 {
		return listValue(nil), nil
	}
	buf := make([]byte, n)
	read, err := reader.Read(buf)
	if err != nil && err != io.EOF {
		return nullValue(), filesystemHostFailure("readchunk", path, err)
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

func readChunkContextValue(ctx context.Context, args []Value) (Value, error) {
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
	v, err := stream.readChunkContext(ctx, n)
	if err == io.EOF {
		return nullValue(), nil
	}
	return v, err
}

func (s *streamValue) readLineContext(ctx context.Context) (Value, error) {
	if s == nil || s.conn == nil {
		if ctx != nil && ctx.Err() != nil {
			return nullValue(), ctx.Err()
		}
		return s.readLine()
	}
	s.readMu.Lock()
	defer s.readMu.Unlock()
	reader, conn, err := s.tcpReader()
	if err != nil {
		return nullValue(), err
	}
	line, err := readTCPContext(ctx, conn, func() (string, error) { return reader.ReadString('\n') })
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

func (s *streamValue) readChunkContext(ctx context.Context, n int) (Value, error) {
	if n < 0 {
		return nullValue(), fmt.Errorf("readchunk expects a non-negative size")
	}
	if s == nil || s.conn == nil {
		if ctx != nil && ctx.Err() != nil {
			return nullValue(), ctx.Err()
		}
		return s.readChunk(n)
	}
	if n == 0 {
		return listValue(nil), nil
	}
	s.readMu.Lock()
	defer s.readMu.Unlock()
	reader, conn, err := s.tcpReader()
	if err != nil {
		return nullValue(), err
	}
	buf := make([]byte, n)
	read, err := readTCPContext(ctx, conn, func() (int, error) { return reader.Read(buf) })
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

func (s *streamValue) tcpReader() (*bufio.Reader, net.Conn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureOpen(); err != nil {
		return nil, nil, err
	}
	if s.reader == nil || s.conn == nil {
		return nil, nil, fmt.Errorf("stream %s is not a readable TCP stream", s.path)
	}
	return s.reader, s.conn, nil
}

func readTCPContext[T any](ctx context.Context, conn net.Conn, read func() (T, error)) (T, error) {
	var zero T
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetReadDeadline(deadline); err != nil {
			return zero, err
		}
		defer conn.SetReadDeadline(time.Time{})
	}
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.SetReadDeadline(time.Now())
		case <-done:
		}
		close(stopped)
	}()
	value, err := read()
	close(done)
	<-stopped
	if ctx.Err() != nil {
		return zero, ctx.Err()
	}
	return value, err
}

// transferToContext copies at most limit bytes (or until EOF when limit is
// negative) through one bounded buffer. It borrows both streams; callers keep
// ownership and must close them separately.
func (s *streamValue) transferToContext(ctx context.Context, destination *streamValue, limit int64) (int64, error) {
	if s == nil || destination == nil {
		return 0, fmt.Errorf("transfer expects readable and writable streams")
	}
	if s == destination {
		return 0, fmt.Errorf("transfer source and destination must differ")
	}
	if limit < -1 {
		return 0, fmt.Errorf("transfer limit must be non-negative")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.readMu.Lock()
	defer s.readMu.Unlock()
	destination.writeMu.Lock()
	defer destination.writeMu.Unlock()
	s.mu.Lock()
	if err := s.ensureOpen(); err != nil {
		s.mu.Unlock()
		return 0, err
	}
	reader, sourceConn, sourcePath := s.reader, s.conn, s.path
	s.mu.Unlock()
	destination.mu.Lock()
	if err := destination.ensureOpen(); err != nil {
		destination.mu.Unlock()
		return 0, err
	}
	writer, destinationConn, destinationPath := destination.writer, destination.conn, destination.path
	destination.mu.Unlock()
	if reader == nil {
		return 0, fmt.Errorf("stream %s is not readable", sourcePath)
	}
	if writer == nil {
		return 0, fmt.Errorf("stream %s is not writable", destinationPath)
	}
	buffer := make([]byte, 32<<10)
	var copied int64
	for limit < 0 || copied < limit {
		if err := ctx.Err(); err != nil {
			return copied, err
		}
		want := len(buffer)
		if limit >= 0 && int64(want) > limit-copied {
			want = int(limit - copied)
		}
		if want == 0 {
			break
		}
		var read int
		var readErr error
		if sourceConn != nil {
			read, readErr = readTCPContext(ctx, sourceConn, func() (int, error) { return reader.Read(buffer[:want]) })
		} else {
			read, readErr = reader.Read(buffer[:want])
		}
		if read > 0 {
			written := 0
			for written < read {
				if err := ctx.Err(); err != nil {
					return copied, err
				}
				var n int
				var writeErr error
				if destinationConn != nil {
					n, writeErr = writeTCPContext(ctx, destinationConn, func() (int, error) { return writer.Write(buffer[written:read]) })
				} else {
					n, writeErr = writer.Write(buffer[written:read])
				}
				written += n
				copied += int64(n)
				if writeErr != nil {
					return copied, writeErr
				}
				if n == 0 {
					return copied, io.ErrShortWrite
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return copied, readErr
		}
	}
	if destinationConn != nil {
		if _, err := writeTCPContext(ctx, destinationConn, func() (struct{}, error) { return struct{}{}, writer.Flush() }); err != nil {
			return copied, err
		}
	} else if err := writer.Flush(); err != nil {
		return copied, err
	}
	return copied, nil
}

func transferContextValue(ctx context.Context, args []Value) (Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return nullValue(), fmt.Errorf("transfer expects source, destination, and optional limit")
	}
	source, err := args[0].AsStream()
	if err != nil {
		return nullValue(), err
	}
	destination, err := args[1].AsStream()
	if err != nil {
		return nullValue(), err
	}
	limit := int64(-1)
	if len(args) == 3 {
		n, err := args[2].Int()
		if err != nil {
			return nullValue(), err
		}
		limit = int64(n)
	}
	copied, err := source.transferToContext(ctx, destination, limit)
	if err != nil {
		return nullValue(), err
	}
	return uintValue(uint64(copied), 64), nil
}

func writeTCPContext[T any](ctx context.Context, conn net.Conn, write func() (T, error)) (T, error) {
	var zero T
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetWriteDeadline(deadline); err != nil {
			return zero, err
		}
		defer conn.SetWriteDeadline(time.Time{})
	}
	done, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.SetWriteDeadline(time.Now())
		case <-done:
		}
		close(stopped)
	}()
	value, err := write()
	close(done)
	<-stopped
	if ctx.Err() != nil {
		return zero, ctx.Err()
	}
	return value, err
}

// writeChunk writes a byte-compatible value to the stream and returns the
// number of bytes accepted by the underlying writer. The function preserves
// incremental output behavior so large results can be streamed without holding
// the entire payload in memory.
func (s *streamValue) writeChunk(v Value) (Value, error) {
	if s == nil {
		return nullValue(), filesystemFailure(ErrorKindType, "writechunk", "", "writechunk expects a stream", nil)
	}
	data, err := bytesFromValue(v)
	if err != nil {
		return nullValue(), filesystemFailure(ErrorKindType, "writechunk", s.path, "writechunk expects byte-compatible data", nil)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.mu.Lock()
	if err := s.ensureOpen(); err != nil {
		s.mu.Unlock()
		return nullValue(), err
	}
	writer := s.writer
	path := s.path
	s.mu.Unlock()
	if writer == nil {
		return nullValue(), filesystemFailure(ErrorKindUnsupported, "writechunk", path, "writechunk requires a writable stream", nil)
	}
	n, err := writer.Write(data)
	if err != nil {
		return nullValue(), filesystemHostFailure("writechunk", path, err)
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
		return nullValue(), filesystemFailure(ErrorKindType, "flush", "", "flush expects a stream", nil)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.mu.Lock()
	if err := s.ensureOpen(); err != nil {
		s.mu.Unlock()
		return nullValue(), err
	}
	writer := s.writer
	s.mu.Unlock()
	if writer == nil {
		return boolValue(true), nil
	}
	if err := writer.Flush(); err != nil {
		return nullValue(), filesystemHostFailure("flush", s.path, err)
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
		return boolValue(false), filesystemFailure(ErrorKindType, "close", "", "close expects a stream", nil)
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return boolValue(false), nil
	}
	s.closed = true
	writer, file, conn, closer, path := s.writer, s.file, s.conn, s.closer, s.path
	s.closer = nil
	s.mu.Unlock()
	var firstErr error
	if writer != nil {
		s.writeMu.Lock()
		if err := writer.Flush(); err != nil {
			firstErr = filesystemHostFailure("close", path, err)
		}
		s.writeMu.Unlock()
	}
	if file != nil {
		if err := file.Close(); err != nil {
			if firstErr == nil {
				firstErr = filesystemHostFailure("close", path, err)
			}
		}
	}
	if conn != nil {
		if err := conn.Close(); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if closer != nil && closer != file && closer != conn {
		if err := closer.Close(); err != nil && firstErr == nil {
			firstErr = filesystemHostFailure("close", path, err)
		}
	}
	if firstErr != nil {
		return nullValue(), firstErr
	}
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
		record := newOsirisError(ErrorKindIO, "stream is closed").
			withMetadata("operation", stringValue("stream")).
			withMetadata("component", stringValue("stream"))
		if s.path != "" {
			record = record.withMetadata("resource", stringValue(s.path))
		}
		return record
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
