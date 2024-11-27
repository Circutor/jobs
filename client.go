package jobs

import (
	"fmt"

	"github.com/hibiken/asynq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Client is a wrapper around asynq.Client.
type Client struct {
	asynqClient *asynq.Client
	inspector   *asynq.Inspector
	gormDB      *gorm.DB
}

// ClientOption is a function for optional params that allow custom
// configurations for the server.
type ClientOption func(s *Server) error

// New returns a new client.
func NewClient(redisURL string, db int, options ...ClientOption) *Client {
	return &Client{
		asynqClient: asynq.NewClient(asynq.RedisClientOpt{Addr: redisURL, DB: db}), inspector: asynq.NewInspector(asynq.RedisClientOpt{Addr: redisURL, DB: db}),
	}
}

// WithClientDBMiddleware is a ClientOption that allows to set a custom database
// middleware, to store the tasks (an its statuses) in a database.
func WithClientDBMiddleware(postgresURL string) ClientOption {
	return func(s *Server) error {
		db, err := gorm.Open(postgres.Open(postgresURL), &gorm.Config{})
		if err != nil {
			return fmt.Errorf("failed to connect database: %v", err)
		}

		if err = db.AutoMigrate(&dbTaskInfo{}); err != nil {
			return fmt.Errorf("failed to migrate database: %v", err)
		}

		s.gormDB = db

		return nil
	}
}

// Close closes the client.
func (c *Client) Close() {
	c.asynqClient.Close()
}

// Get task result.
func (c *Client) RetrieveTaskResultByID(taskID string) ([]byte, error) {
	taskInfo, err := c.inspector.GetTaskInfo("default", taskID)
	if err != nil {
		return nil, fmt.Errorf("c.inspector.GetTaskInfo %w", err)
	}

	return taskInfo.Result, nil
}

// Enqueue enqueues a task.
func (c *Client) Enqueue(t *Task) (*TaskInfo, error) {
	_, err := c.asynqClient.Enqueue(t.toAsynqTask())
	if err != nil {
		return &TaskInfo{}, fmt.Errorf(":c.asynqClient.Enqueue %w", err)
	}

	taskInfo := t.toTaskInfo(TaskInfoStatusPending)

	if c.gormDB != nil {
		if err := c.gormDB.Create(taskInfo.toDBTaskInfo()).Error; err != nil {
			return &TaskInfo{}, fmt.Errorf("c.gormDB.Create %w", err)
		}
	}

	return taskInfo, nil
}
