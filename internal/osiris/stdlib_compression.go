package osiris

import (
	"compress/gzip"
	"compress/zlib"
	"context"
	"fmt"
	"io"
	"time"
)

// StdlibCompressionKind selects the versioned standard-library stream
// format. No auto-detection is performed.
type StdlibCompressionKind string

const (
	StdlibCompressionGZIP StdlibCompressionKind = "gzip-1"
	StdlibCompressionZlib StdlibCompressionKind = "zlib-1"
)

// StdlibCompressionLimits bounds bytes read, bytes written, and decompressed
// expansion. Zero fields select 64 MiB input, 256 MiB output, a 1000:1 ratio,
// and an 32 KiB copy buffer.
type StdlibCompressionLimits struct {
	MaxInputBytes     int64
	MaxOutputBytes    int64
	MaxExpansionRatio int64
	BufferBytes       int
}

func (l StdlibCompressionLimits) normalized() StdlibCompressionLimits {
	if l.MaxInputBytes <= 0 {
		l.MaxInputBytes = 64 << 20
	}
	if l.MaxOutputBytes <= 0 {
		l.MaxOutputBytes = 256 << 20
	}
	if l.MaxExpansionRatio <= 0 {
		l.MaxExpansionRatio = 1000
	}
	if l.BufferBytes <= 0 {
		l.BufferBytes = 32 << 10
	}
	if l.BufferBytes > 1<<20 {
		l.BufferBytes = 1 << 20
	}
	return l
}

// StdlibCompressionStats reports bytes consumed from src and produced to
// dst. The caller retains ownership of src and dst; only the internal codec
// wrapper is closed by the operation.
type StdlibCompressionStats struct {
	InputBytes  int64
	OutputBytes int64
}

type stdlibCountingReader struct {
	ctx context.Context
	r   io.Reader
	max int64
	n   int64
}

func (r *stdlibCountingReader) Read(p []byte) (int, error) {
	if err := stdlibCheckContext(r.ctx, "compression read"); err != nil {
		return 0, err
	}
	if r.n == r.max {
		var one [1]byte
		n, err := r.r.Read(one[:])
		if n > 0 {
			return 0, stdlibLimitError("compression read", "input bytes", r.n+int64(n), r.max)
		}
		return 0, err
	}
	if r.n > r.max {
		return 0, stdlibLimitError("compression read", "input bytes", r.n, r.max)
	}
	if int64(len(p)) > r.max-r.n {
		p = p[:r.max-r.n]
	}
	n, err := r.r.Read(p)
	r.n += int64(n)
	if r.n > r.max {
		return n, stdlibLimitError("compression read", "input bytes", r.n, r.max)
	}
	return n, err
}

type stdlibLimitedWriter struct {
	ctx       context.Context
	w         io.Writer
	max       int64
	maxRatio  int64
	inputRead func() int64
	n         int64
}

func (w *stdlibLimitedWriter) Write(p []byte) (int, error) {
	if err := stdlibCheckContext(w.ctx, "compression write"); err != nil {
		return 0, err
	}
	if int64(len(p)) > w.max-w.n {
		return 0, stdlibLimitError("compression write", "output bytes", w.n+int64(len(p)), w.max)
	}
	if w.maxRatio > 0 {
		input := w.inputRead()
		if input < 1 {
			input = 1
		}
		if w.n+int64(len(p)) > input*w.maxRatio {
			return 0, stdlibLimitError("compression write", "expansion ratio", w.n+int64(len(p)), input*w.maxRatio)
		}
	}
	n, err := w.w.Write(p)
	w.n += int64(n)
	return n, err
}

func stdlibCodec(kind StdlibCompressionKind) error {
	if kind != StdlibCompressionGZIP && kind != StdlibCompressionZlib {
		return fmt.Errorf("compression: %w: unsupported format %q", ErrStdlibInvalid, kind)
	}
	return nil
}

// StdlibCompress streams gzip or zlib from src to dst. It never closes the
// caller-owned streams, emits deterministic headers, and honors cancellation
// between bounded reads and writes.
func StdlibCompress(ctx context.Context, kind StdlibCompressionKind, src io.Reader, dst io.Writer, limits StdlibCompressionLimits) (StdlibCompressionStats, error) {
	if src == nil || dst == nil {
		return StdlibCompressionStats{}, fmt.Errorf("compression: %w: nil source or destination", ErrStdlibInvalid)
	}
	if err := stdlibCodec(kind); err != nil {
		return StdlibCompressionStats{}, err
	}
	limits = limits.normalized()
	input := &stdlibCountingReader{ctx: ctx, r: src, max: limits.MaxInputBytes}
	boundedDestination := &stdlibLimitedWriter{ctx: ctx, w: dst, max: limits.MaxOutputBytes, inputRead: func() int64 { return input.n }}
	var writer io.WriteCloser
	switch kind {
	case StdlibCompressionGZIP:
		gzipWriter := gzip.NewWriter(boundedDestination)
		gzipWriter.Header.ModTime = time.Unix(0, 0).UTC()
		gzipWriter.Header.Name = ""
		gzipWriter.Header.Comment = ""
		writer = gzipWriter
	case StdlibCompressionZlib:
		zlibWriter := zlib.NewWriter(boundedDestination)
		writer = zlibWriter
	}
	copyErr := func() error {
		buf := make([]byte, limits.BufferBytes)
		_, err := io.CopyBuffer(writer, input, buf)
		return err
	}()
	closeErr := writer.Close()
	if copyErr != nil {
		return StdlibCompressionStats{InputBytes: input.n, OutputBytes: boundedDestination.n}, fmt.Errorf("compression: %w", copyErr)
	}
	if closeErr != nil {
		return StdlibCompressionStats{InputBytes: input.n, OutputBytes: boundedDestination.n}, fmt.Errorf("compression close: %w", closeErr)
	}
	if err := stdlibCheckContext(ctx, "compression"); err != nil {
		return StdlibCompressionStats{InputBytes: input.n, OutputBytes: boundedDestination.n}, err
	}
	return StdlibCompressionStats{InputBytes: input.n, OutputBytes: boundedDestination.n}, nil
}

// StdlibDecompress streams gzip or zlib from src to dst. It validates the
// format selected by kind, rejects malformed streams, applies byte and
// expansion-ratio limits, and leaves caller-owned streams open.
func StdlibDecompress(ctx context.Context, kind StdlibCompressionKind, src io.Reader, dst io.Writer, limits StdlibCompressionLimits) (StdlibCompressionStats, error) {
	if src == nil || dst == nil {
		return StdlibCompressionStats{}, fmt.Errorf("decompression: %w: nil source or destination", ErrStdlibInvalid)
	}
	if err := stdlibCodec(kind); err != nil {
		return StdlibCompressionStats{}, err
	}
	limits = limits.normalized()
	input := &stdlibCountingReader{ctx: ctx, r: src, max: limits.MaxInputBytes}
	var reader io.ReadCloser
	var err error
	switch kind {
	case StdlibCompressionGZIP:
		reader, err = gzip.NewReader(input)
	case StdlibCompressionZlib:
		reader, err = zlib.NewReader(input)
	}
	if err != nil {
		return StdlibCompressionStats{InputBytes: input.n}, fmt.Errorf("decompression header: %w", err)
	}
	output := &stdlibLimitedWriter{ctx: ctx, w: dst, max: limits.MaxOutputBytes, maxRatio: limits.MaxExpansionRatio, inputRead: func() int64 { return input.n }}
	buf := make([]byte, limits.BufferBytes)
	_, copyErr := io.CopyBuffer(output, reader, buf)
	closeErr := reader.Close()
	if copyErr != nil {
		return StdlibCompressionStats{InputBytes: input.n, OutputBytes: output.n}, fmt.Errorf("decompression: %w", copyErr)
	}
	if closeErr != nil {
		return StdlibCompressionStats{InputBytes: input.n, OutputBytes: output.n}, fmt.Errorf("decompression close: %w", closeErr)
	}
	if err := stdlibCheckContext(ctx, "decompression"); err != nil {
		return StdlibCompressionStats{InputBytes: input.n, OutputBytes: output.n}, err
	}
	return StdlibCompressionStats{InputBytes: input.n, OutputBytes: output.n}, nil
}
