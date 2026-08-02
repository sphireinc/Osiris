package osiris

import (
	"context"
	"errors"
	"fmt"
	"io"
)

const task120Version = "1.0.0"

var errTask120Cancelled = context.Canceled

func task120CheckContext(ctx context.Context, operation string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("task120 %s: %w", operation, err)
	}
	return nil
}

type task120ContextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *task120ContextReader) Read(p []byte) (int, error) {
	if err := task120CheckContext(r.ctx, "stream read"); err != nil {
		return 0, err
	}
	n, err := r.r.Read(p)
	if checkErr := task120CheckContext(r.ctx, "stream read"); checkErr != nil {
		return n, checkErr
	}
	return n, err
}

func task120LimitError(operation, limit string, got, allowed int64) error {
	return fmt.Errorf("task120 %s: %w: %s (%d exceeds %d)", operation, ErrTask120Limit, limit, got, allowed)
}

// ErrTask120Limit identifies a deterministic resource-limit rejection.
var ErrTask120Limit = errors.New("task-120 resource limit exceeded")

// ErrTask120Invalid identifies malformed or unsupported task-120 input.
var ErrTask120Invalid = errors.New("task-120 invalid input")

// ErrTask120Path identifies an unsafe archive path.
var ErrTask120Path = errors.New("task-120 unsafe archive path")
