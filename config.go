package jobs

import "github.com/hibiken/asynq"

type Config struct {
	// Concurrency specifies the maximum number of concurrent
	// workers that can process a task queue.
	Concurrency int

	// Queues specifies the priority of task queues.
	// The order of the queue names matter.
	Queues map[string]int
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Concurrency: 10,
		Queues: map[string]int{
			"critical": 6,
			"default":  10,
			"low":      1,
		},
	}
}

func (c *Config) toAsynqConfig() asynq.Config {
	return asynq.Config{
		Concurrency: c.Concurrency,
		Queues:      c.Queues,
	}
}
