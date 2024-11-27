package jobs

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// Task is the definition of a task and its options.
type Task struct {
	ID      string
	Kind    string
	Payload []byte
	Result  []byte

	maxRetry  int
	timeout   time.Duration
	retention time.Duration

	originalTask *asynq.Task
}

func (t *Task) toTaskInfo(status TaskInfoStatus) *TaskInfo {
	return &TaskInfo{
		ID:       t.ID,
		TaskType: t.Kind,
		Payload:  string(t.Payload),
		Status:   status,
		Result:   t.Result,
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

		// Add default retention time of 15 days.
		retention: time.Hour * 24 * 15,
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

func (t *Task) WriteResult(result any) (n int, err error) {
	byteResult, err := json.Marshal(result)
	if err != nil {
		return 0, err
	}

	if t.originalTask == nil {
		return 0, asynq.ErrTaskNotFound
	}

	t.Result = byteResult

	return t.originalTask.ResultWriter().Write(byteResult)
}

func (t *Task) toAsynqTask() *asynq.Task {
	if t.originalTask == nil {
		t.originalTask = asynq.NewTask(
			t.Kind, t.Payload,
			asynq.MaxRetry(t.maxRetry),
			asynq.Timeout(t.timeout),
			asynq.Retention(t.retention),
			asynq.TaskID(t.ID),
		)
	}

	return t.originalTask
}

func fromAsynqTask(task *asynq.Task) Task {
	return Task{
		ID:      task.ResultWriter().TaskID(),
		Kind:    task.Type(),
		Payload: task.Payload(),
	}
}
