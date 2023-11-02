package jobs

import (
	"time"

	"github.com/hibiken/asynq"
)

// Task is the definition of a task and its options.
type Task struct {
	kind    string
	payload []byte

	maxRetry int
	timeout  time.Duration
}

// TaskOption is a function for optional params that allow custom
// configurations for the task.
type TaskOption func(t *Task)

// NewTask creates a new task.
func NewTask(kind string, payload []byte, options ...TaskOption) Task {
	t := Task{
		kind:    kind,
		payload: payload,
	}

	for _, option := range options {
		option(&t)
	}

	return t
}

// MaxRetry is a TaskOption that allows to set the maximum number of retries.
func MaxRetry(n int) TaskOption {
	return func(t *Task) {
		t.maxRetry = n
	}
}

// Timeout is a TaskOption that allows to set the timeout.
func Timeout(d time.Duration) TaskOption {
	return func(t *Task) {
		t.timeout = d
	}
}

func (t Task) toAsynqTask() *asynq.Task {
	return asynq.NewTask(
		t.kind, t.payload, asynq.MaxRetry(t.maxRetry), asynq.Timeout(t.timeout))
}
