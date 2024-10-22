package jobs

import (
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// Task is the definition of a task and its options.
type Task struct {
	ID      string
	Kind    string
	Payload []byte

	maxRetry  int
	timeout   time.Duration
	retention time.Duration
}

func (t Task) toTaskInfo(status TaskInfoStatus) *TaskInfo {
	return &TaskInfo{
		ID:       t.ID,
		TaskType: t.Kind,
		Payload:  string(t.Payload),
		Status:   status,
	}
}

// TaskOption is a function for optional params that allow custom
// configurations for the task.
type TaskOption func(t *Task)

// NewTask creates a new task.
func NewTask(kind string, payload []byte, options ...TaskOption) Task {
	t := Task{
		ID:      uuid.NewString(),
		Kind:    kind,
		Payload: payload,
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

// Retention is a TaskOption that allows to set the retention.
func Retention(d time.Duration) TaskOption {
	return func(t *Task) {
		t.retention = d
	}
}

func (t Task) toAsynqTask() *asynq.Task {
	return asynq.NewTask(
		t.Kind, t.Payload,
		asynq.MaxRetry(t.maxRetry),
		asynq.Timeout(t.timeout),
		asynq.Retention(t.retention),
		asynq.TaskID(t.ID),
	)
}

func fromAsynqTask(task *asynq.Task) Task {
	return Task{
		ID:      task.ResultWriter().TaskID(),
		Kind:    task.Type(),
		Payload: task.Payload(),
	}
}
