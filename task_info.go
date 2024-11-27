package jobs

import (
	"time"

	"gorm.io/gorm"
)

// TaskInfoStatus is a custom type to define the status of a task.
type TaskInfoStatus string

const (
	// TaskInfoStatusPending is the status of a task when it is pending.
	TaskInfoStatusPending TaskInfoStatus = "pending"
	// TaskInfoStatusRunning is the status of a task when it is running.
	TaskInfoStatusRunning TaskInfoStatus = "running"
	// TaskInfoStatusFinished is the status of a task when it is finished.
	TaskInfoStatusFinished TaskInfoStatus = "finished"
	// TaskInfoStatusFailed is the status of a task when it is failed.
	TaskInfoStatusFailed TaskInfoStatus = "failed"
)

// TaskInfo is the definition for the status of a task.
type TaskInfo struct {
	ID       string
	TaskType string
	Payload  string
	Status   TaskInfoStatus
	Result   []byte
}

func (t *TaskInfo) toDBTaskInfo() *dbTaskInfo {
	return &dbTaskInfo{
		ID:       t.ID,
		TaskType: t.TaskType,
		Payload:  t.Payload,
		Status:   string(t.Status),
		Result:   string(t.Result),
	}
}

type dbTaskInfo struct {
	ID       string `gorm:"primaryKey"`
	TaskType string `gorm:"size:255"`
	Payload  string `gorm:"size:1000"`
	Status   string `gorm:"size:100;default:'pending'"`
	Result   string `gorm:"size:1000"`

	CreatedAt time.Time
	UpdatedAt time.Time

	gorm.Model
}
