package jobs

import "github.com/hibiken/asynq"

type Logger interface {
	// Debug logs a message at Debug level.
	Debug(args ...interface{})

	// Info logs a message at Info level.
	Info(args ...interface{})

	// Warn logs a message at Warning level.
	Warn(args ...interface{})

	// Error logs a message at Error level.
	Error(args ...interface{})

	// Fatal logs a message at Fatal level
	// and process will exit with status set to 1.
	Fatal(args ...interface{})
}

type Config struct {
	// Concurrency specifies the maximum number of concurrent
	// workers that can process a task queue.
	Concurrency int

	// Queues specifies the priority of task queues.
	// The order of the queue names matter.
	Queues map[string]int

	// Logger specifies the logger used by the server instance.
	//
	// If unset, default logger is used.
	Logger Logger
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
		Logger:      c.Logger,
	}
}
