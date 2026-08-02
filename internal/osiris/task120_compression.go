package osiris

import (
	"compress/gzip"
	"compress/zlib"
	"context"
	"fmt"
	"io"
	"time"
)

// Task120CompressionKind selects the versioned standard-library stream
// format. No auto-detection is performed.
type Task120CompressionKind string

const (
	Task120CompressionGZIP Task120CompressionKind = "gzip-1"
	Task120CompressionZlib Task120CompressionKind = "zlib-1"
)

// Task120CompressionLimits bounds bytes read, bytes written, and decompressed
// expansion. Zero fields select 64 MiB input, 256 MiB output, a 1000:1 ratio,
// and an 32 KiB copy buffer.
type Task120CompressionLimits struct {
	MaxInputBytes     int64
	MaxOutputBytes    int64
	MaxExpansionRatio int64
	BufferBytes       int
}

func (l Task120CompressionLimits) normalized() Task120CompressionLimits {
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

// Task120CompressionStats reports bytes consumed from src and produced to
// dst. The caller retains ownership of src and dst; only the internal codec
// wrapper is closed by the operation.
type Task120CompressionStats struct {
	InputBytes  int64
	OutputBytes int64
}

type task120CountingReader struct {
	ctx context.Context
	r   io.Reader
	max int64
	n   int64
}

func (r *task120CountingReader) Read(p []byte) (int, error) {
	if err := task120CheckContext(r.ctx, "compression read"); err != nil {
		return 0, err
	}
	if r.n == r.max {
		var one [1]byte
		n, err := r.r.Read(one[:])
		if n > 0 {
			return 0, task120LimitError("compression read", "input bytes", r.n+int64(n), r.max)
		}
		return 0, err
	}
	if r.n > r.max {
		return 0, task120LimitError("compression read", "input bytes", r.n, r.max)
	}
	if int64(len(p)) > r.max-r.n {
		p = p[:r.max-r.n]
	}
	n, err := r.r.Read(p)
	r.n += int64(n)
	if r.n > r.max {
		return n, task120LimitError("compression read", "input bytes", r.n, r.max)
	}
	return n, err
}

type task120LimitedWriter struct {
	ctx       context.Context
	w         io.Writer
	max       int64
	maxRatio  int64
	inputRead func() int64
	n         int64
}

func (w *task120LimitedWriter) Write(p []byte) (int, error) {
	if err := task120CheckContext(w.ctx, "compression write"); err != nil {
		return 0, err
	}
	if int64(len(p)) > w.max-w.n {
		return 0, task120LimitError("compression write", "output bytes", w.n+int64(len(p)), w.max)
	}
	if w.maxRatio > 0 {
		input := w.inputRead()
		if input < 1 {
			input = 1
		}
		if w.n+int64(len(p)) > input*w.maxRatio {
			return 0, task120LimitError("compression write", "expansion ratio", w.n+int64(len(p)), input*w.maxRatio)
		}
	}
	n, err := w.w.Write(p)
	w.n += int64(n)
	return n, err
}

func task120Codec(kind Task120CompressionKind) error {
	if kind != Task120CompressionGZIP && kind != Task120CompressionZlib {
		return fmt.Errorf("compression: %w: unsupported format %q", ErrTask120Invalid, kind)
	}
	return nil
}

// Task120Compress streams gzip or zlib from src to dst. It never closes the
// caller-owned streams, emits deterministic headers, and honors cancellation
// between bounded reads and writes.
func Task120Compress(ctx context.Context, kind Task120CompressionKind, src io.Reader, dst io.Writer, limits Task120CompressionLimits) (Task120CompressionStats, error) {
	if src == nil || dst == nil {
		return Task120CompressionStats{}, fmt.Errorf("compression: %w: nil source or destination", ErrTask120Invalid)
	}
	if err := task120Codec(kind); err != nil {
		return Task120CompressionStats{}, err
	}
	limits = limits.normalized()
	input := &task120CountingReader{ctx: ctx, r: src, max: limits.MaxInputBytes}
	boundedDestination := &task120LimitedWriter{ctx: ctx, w: dst, max: limits.MaxOutputBytes, inputRead: func() int64 { return input.n }}
	var writer io.WriteCloser
	switch kind {
	case Task120CompressionGZIP:
		gzipWriter := gzip.NewWriter(boundedDestination)
		gzipWriter.Header.ModTime = time.Unix(0, 0).UTC()
		gzipWriter.Header.Name = ""
		gzipWriter.Header.Comment = ""
		writer = gzipWriter
	case Task120CompressionZlib:
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
		return Task120CompressionStats{InputBytes: input.n, OutputBytes: boundedDestination.n}, fmt.Errorf("compression: %w", copyErr)
	}
	if closeErr != nil {
		return Task120CompressionStats{InputBytes: input.n, OutputBytes: boundedDestination.n}, fmt.Errorf("compression close: %w", closeErr)
	}
	if err := task120CheckContext(ctx, "compression"); err != nil {
		return Task120CompressionStats{InputBytes: input.n, OutputBytes: boundedDestination.n}, err
	}
	return Task120CompressionStats{InputBytes: input.n, OutputBytes: boundedDestination.n}, nil
}

// Task120Decompress streams gzip or zlib from src to dst. It validates the
// format selected by kind, rejects malformed streams, applies byte and
// expansion-ratio limits, and leaves caller-owned streams open.
func Task120Decompress(ctx context.Context, kind Task120CompressionKind, src io.Reader, dst io.Writer, limits Task120CompressionLimits) (Task120CompressionStats, error) {
	if src == nil || dst == nil {
		return Task120CompressionStats{}, fmt.Errorf("decompression: %w: nil source or destination", ErrTask120Invalid)
	}
	if err := task120Codec(kind); err != nil {
		return Task120CompressionStats{}, err
	}
	limits = limits.normalized()
	input := &task120CountingReader{ctx: ctx, r: src, max: limits.MaxInputBytes}
	var reader io.ReadCloser
	var err error
	switch kind {
	case Task120CompressionGZIP:
		reader, err = gzip.NewReader(input)
	case Task120CompressionZlib:
		reader, err = zlib.NewReader(input)
	}
	if err != nil {
		return Task120CompressionStats{InputBytes: input.n}, fmt.Errorf("decompression header: %w", err)
	}
	output := &task120LimitedWriter{ctx: ctx, w: dst, max: limits.MaxOutputBytes, maxRatio: limits.MaxExpansionRatio, inputRead: func() int64 { return input.n }}
	buf := make([]byte, limits.BufferBytes)
	_, copyErr := io.CopyBuffer(output, reader, buf)
	closeErr := reader.Close()
	if copyErr != nil {
		return Task120CompressionStats{InputBytes: input.n, OutputBytes: output.n}, fmt.Errorf("decompression: %w", copyErr)
	}
	if closeErr != nil {
		return Task120CompressionStats{InputBytes: input.n, OutputBytes: output.n}, fmt.Errorf("decompression close: %w", closeErr)
	}
	if err := task120CheckContext(ctx, "decompression"); err != nil {
		return Task120CompressionStats{InputBytes: input.n, OutputBytes: output.n}, err
	}
	return Task120CompressionStats{InputBytes: input.n, OutputBytes: output.n}, nil
}
