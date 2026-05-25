package osiris

import (
	"context"
	"fmt"
	"sync"
)

type taskValue struct {
	rt     *Runtime
	id     string
	done   chan struct{}
	cancel context.CancelFunc

	once sync.Once
	mu   sync.Mutex
	val  Value
	err  error
	outs []Value
}

// newTaskValue handles the new task value step in the surrounding package and
// returns *taskValue.
//
// It returns *taskValue so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func newTaskValue(rt *Runtime, id string, cancel context.CancelFunc) *taskValue {
	return &taskValue{
		rt:     rt,
		id:     id,
		done:   make(chan struct{}),
		cancel: cancel,
	}
}

// spawnValueTask operates on Session values and keeps the session-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func (s *Session) spawnValueTask(run func(context.Context) (Value, error, []Value)) Value {
	parentCtx := context.Background()
	if s != nil && s.ctx != nil {
		parentCtx = s.ctx
	}
	childCtx, cancel := context.WithCancel(parentCtx)
	rt := (*Runtime)(nil)
	if s != nil {
		rt = s.rt
	}
	if rt == nil {
		rt = NewRuntime()
	}
	task := newTaskValue(rt, rt.nextTaskID(), cancel)
	rt.recordTaskTrace("task.spawn", task.id, "pending", "spawned")
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				task.complete(nullValue(), fmt.Errorf("task panicked: %v", rec), nil)
			}
		}()
		v, err, outs := run(childCtx)
		task.complete(v, err, outs)
	}()
	return taskValueOf(task)
}

// complete operates on taskValue values and keeps the taskvalue-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (t *taskValue) complete(v Value, err error, outs []Value) {
	if t == nil {
		return
	}
	t.once.Do(func() {
		t.mu.Lock()
		t.val = v
		t.err = err
		if len(outs) > 0 {
			t.outs = append([]Value(nil), outs...)
		}
		t.mu.Unlock()
		if t.rt != nil {
			status := "done"
			detail := v.Kind()
			if err != nil {
				status = "error"
				detail = err.Error()
			}
			t.rt.recordTaskTrace("task.complete", t.id, status, detail)
		}
		close(t.done)
	})
}

// await operates on taskValue values and keeps the taskvalue-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func (t *taskValue) await(ctx context.Context, outputs *[]Value) (Value, error) {
	if t == nil {
		return nullValue(), fmt.Errorf("expected task, got null")
	}
	if t.rt != nil {
		t.rt.recordTaskTrace("task.await", t.id, "pending", "await requested")
	}
	select {
	case <-t.done:
		t.mu.Lock()
		defer t.mu.Unlock()
		if outputs != nil && len(t.outs) > 0 {
			*outputs = append(*outputs, t.outs...)
		}
		if t.rt != nil {
			status := "done"
			detail := t.val.Kind()
			if t.err != nil {
				status = "error"
				detail = t.err.Error()
			}
			t.rt.recordTaskTrace("task.await", t.id, status, detail)
		}
		return t.val, t.err
	case <-ctx.Done():
		if t.rt != nil {
			t.rt.recordTaskTrace("task.await", t.id, "canceled", ctx.Err().Error())
		}
		return nullValue(), ctx.Err()
	}
}

// cancelTask operates on taskValue values and keeps the taskvalue-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func (t *taskValue) cancelTask() bool {
	if t == nil {
		return false
	}
	select {
	case <-t.done:
		if t.rt != nil {
			t.rt.recordTaskTrace("task.cancel", t.id, "done", "already complete")
		}
		return false
	default:
		if t.cancel != nil {
			t.cancel()
		}
		if t.rt != nil {
			t.rt.recordTaskTrace("task.cancel", t.id, "pending", "cancellation requested")
		}
		return true
	}
}

// status operates on taskValue values and keeps the taskvalue-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (t *taskValue) status() string {
	if t == nil {
		return "nil"
	}
	select {
	case <-t.done:
		return "done"
	default:
		return "pending"
	}
}

// snapshot operates on taskValue values and keeps the taskvalue-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns string, string, Value, error so callers can continue with the
// next stage of the pipeline without re-deriving the same information.
func (t *taskValue) snapshot() (string, string, Value, error) {
	if t == nil {
		return "", "nil", nullValue(), nil
	}
	select {
	case <-t.done:
		t.mu.Lock()
		defer t.mu.Unlock()
		return t.id, "done", t.val, t.err
	default:
		return t.id, "pending", nullValue(), nil
	}
}

// String operates on taskValue values and keeps the taskvalue-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (t *taskValue) String() string {
	id, status, v, err := t.snapshot()
	switch {
	case t == nil:
		return "<task nil>"
	case status == "pending":
		return "<task " + id + " pending>"
	case err != nil:
		return "<task " + id + " failed: " + err.Error() + ">"
	default:
		return "<task " + id + " done " + v.String() + ">"
	}
}

// nextTaskID operates on Runtime values and keeps the runtime-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (r *Runtime) nextTaskID() string {
	if r == nil {
		return "task-0"
	}
	r.taskMu.Lock()
	defer r.taskMu.Unlock()
	r.taskSeq++
	return fmt.Sprintf("task-%d", r.taskSeq)
}
