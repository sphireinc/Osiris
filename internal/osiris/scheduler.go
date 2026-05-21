package osiris

import (
	"context"
	"fmt"
	"sort"
)

type evalTask struct {
	key string
	run func() (Value, error)
}

type deterministicScheduler struct{}

func (deterministicScheduler) run(ctx context.Context, tasks []evalTask) ([]Value, error) {
	if len(tasks) == 0 {
		return nil, nil
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		return tasks[i].key < tasks[j].key
	})
	out := make([]Value, len(tasks))
	for i, task := range tasks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		v, err := task.run()
		if err != nil {
			return nil, fmt.Errorf("task %s failed: %w", task.key, err)
		}
		out[i] = v
	}
	return out, nil
}
