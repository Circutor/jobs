package jobs

import (
	"fmt"

	"github.com/hibiken/asynq"
)

// Client is a wrapper around asynq.Client.
type Client struct {
	asynqClient *asynq.Client
}

// New returns a new client.
func New(redisURL string) *Client {
	return &Client{
		asynqClient: asynq.NewClient(asynq.RedisClientOpt{Addr: "localhost:6379"}),
	}
}

// Close closes the client.
func (c *Client) Close() {
	c.asynqClient.Close()
}

// Enqueue enqueues a task.
func (c *Client) Enqueue(t Task) error {
	if _, err := c.asynqClient.Enqueue(t.toAsynqTask()); err != nil {
		return fmt.Errorf(":c.asynqClient.Enqueue %w", err)
	}

	return nil
}
