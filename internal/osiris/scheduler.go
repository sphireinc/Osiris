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

type deterministicScheduler struct{}

// run operates on deterministicScheduler values and keeps the
// deterministicscheduler-specific behavior attached to the receiver so callers
// can use the method as the natural entry point for that data structure.
//
// It returns []Value and an error. Callers should only trust the value when
// the error is nil.
func (deterministicScheduler) run(ctx context.Context, tasks []evalTask) ([]Value, error) {
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
				if err := runCtx.Err(); err != nil {
					results <- result{idx: idx, err: err}
					continue
				}
				val, err := tasks[idx].run()
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
