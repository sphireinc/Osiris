package osiris

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"sync"
)

type evalTask struct {
	key string
	run func() (Value, error)
}

// TaskTraceEvent captures one scheduling or task lifecycle observation from a
// runtime that has task tracing enabled. The trace is intentionally compact so
// it can be consumed by tests, debug output, or future CLI diagnostics without
// needing to reconstruct state from multiple subsystems.
type TaskTraceEvent struct {
	Seq    uint64
	Kind   string
	TaskID string
	Status string
	Detail string
}

type deterministicScheduler struct {
	debug bool
	trace func(TaskTraceEvent)
}

// run operates on deterministicScheduler values and keeps the
// deterministicscheduler-specific behavior attached to the receiver so callers
// can use the method as the natural entry point for that data structure.
//
// It returns []Value and an error. Callers should only trust the value when
// the error is nil.
func (d deterministicScheduler) run(ctx context.Context, tasks []evalTask) ([]Value, error) {
	if len(tasks) == 0 {
		return nil, nil
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		return tasks[i].key < tasks[j].key
	})
	if len(tasks) == 1 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		v, err := tasks[0].run()
		if err != nil {
			return nil, fmt.Errorf("task %s failed: %w", tasks[0].key, err)
		}
		return []Value{v}, nil
	}
	if d.debug {
		out := make([]Value, len(tasks))
		for i := range tasks {
			if d.trace != nil {
				d.trace(TaskTraceEvent{Kind: "scheduler.start", TaskID: tasks[i].key, Status: "running", Detail: "deterministic"})
			}
			if err := ctx.Err(); err != nil {
				if d.trace != nil {
					d.trace(TaskTraceEvent{Kind: "scheduler.cancel", TaskID: tasks[i].key, Status: "canceled", Detail: err.Error()})
				}
				return nil, err
			}
			v, err := tasks[i].run()
			if d.trace != nil {
				if err != nil {
					d.trace(TaskTraceEvent{Kind: "scheduler.finish", TaskID: tasks[i].key, Status: "error", Detail: err.Error()})
				} else {
					d.trace(TaskTraceEvent{Kind: "scheduler.finish", TaskID: tasks[i].key, Status: "done", Detail: v.Kind()})
				}
			}
			if err != nil {
				return nil, fmt.Errorf("task %s failed: %w", tasks[i].key, err)
			}
			out[i] = v
		}
		return out, nil
	}

	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	if workers > len(tasks) {
		workers = len(tasks)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		idx int
		val Value
		err error
	}

	jobs := make(chan int)
	results := make(chan result, len(tasks))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				if d.trace != nil {
					d.trace(TaskTraceEvent{Kind: "scheduler.start", TaskID: tasks[idx].key, Status: "running", Detail: "worker"})
				}
				if err := runCtx.Err(); err != nil {
					if d.trace != nil {
						d.trace(TaskTraceEvent{Kind: "scheduler.cancel", TaskID: tasks[idx].key, Status: "canceled", Detail: err.Error()})
					}
					results <- result{idx: idx, err: err}
					continue
				}
				val, err := tasks[idx].run()
				if d.trace != nil {
					if err != nil {
						d.trace(TaskTraceEvent{Kind: "scheduler.finish", TaskID: tasks[idx].key, Status: "error", Detail: err.Error()})
					} else {
						d.trace(TaskTraceEvent{Kind: "scheduler.finish", TaskID: tasks[idx].key, Status: "done", Detail: val.Kind()})
					}
				}
				results <- result{idx: idx, val: val, err: err}
				if err != nil {
					cancel()
				}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for idx := range tasks {
			select {
			case jobs <- idx:
			case <-runCtx.Done():
				return
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	out := make([]Value, len(tasks))
	var firstErr error
	for res := range results {
		if res.err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("task %s failed: %w", tasks[res.idx].key, res.err)
			}
			continue
		}
		out[res.idx] = res.val
	}
	if firstErr != nil {
		return nil, firstErr
	}
	if err := runCtx.Err(); err != nil && err != context.Canceled {
		return nil, err
	}
	return out, nil
}
