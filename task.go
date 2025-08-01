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

	maxRetry     int
	timeout      time.Duration
	retention    time.Duration
	processIn    time.Duration
	queueName    string
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
		ID:       uuid.NewString(),
		Kind:     kind,
		Payload:  payload,
		maxRetry: 200,

		// Add default retention time of 1 day.
		retention: time.Hour * 24 * 1,
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

// ProcessIn is a TaskOption that allows to set the process in duration.
func ProcessIn(d time.Duration) TaskOption {
	return func(t *Task) {
		t.processIn = d
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

// QueueName is a TaskOption that allows to set the queue name.
func QueueName(name string) TaskOption {
	return func(t *Task) {
		t.queueName = name
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
		opts := []asynq.Option{
			asynq.MaxRetry(t.maxRetry),
			asynq.Timeout(t.timeout),
			asynq.Retention(t.retention),
			asynq.TaskID(t.ID),
			asynq.ProcessIn(t.processIn),
		}

		if t.queueName != "" {
			opts = append(opts, asynq.Queue(t.queueName))
		}

		t.originalTask = asynq.NewTask(t.Kind, t.Payload, opts...)
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
