package osiris

import (
	"context"
	"errors"
	"fmt"
	"io"
)

const stdlibContractVersion = "1.0.0"

var errStdlibCancelled = context.Canceled

func stdlibCheckContext(ctx context.Context, operation string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("stdlib %s: %w", operation, err)
	}
	return nil
}

type stdlibContextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *stdlibContextReader) Read(p []byte) (int, error) {
	if err := stdlibCheckContext(r.ctx, "stream read"); err != nil {
		return 0, err
	}
	n, err := r.r.Read(p)
	if checkErr := stdlibCheckContext(r.ctx, "stream read"); checkErr != nil {
		return n, checkErr
	}
	return n, err
}

func stdlibLimitError(operation, limit string, got, allowed int64) error {
	return fmt.Errorf("stdlib %s: %w: %s (%d exceeds %d)", operation, ErrStdlibLimit, limit, got, allowed)
}

// ErrStdlibLimit identifies a deterministic resource-limit rejection.
var ErrStdlibLimit = errors.New("stdlib resource limit exceeded")

// ErrStdlibInvalid identifies malformed or unsupported stdlib input.
var ErrStdlibInvalid = errors.New("stdlib invalid input")

// ErrStdlibPath identifies an unsafe archive path.
var ErrStdlibPath = errors.New("stdlib unsafe archive path")
