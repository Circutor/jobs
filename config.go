package jobs

import (
	"time"

	"github.com/hibiken/asynq"
)

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

type RateLimitConfig struct {
	// Rate per second (events/sec)
	Rate float64
	// Burst capacity
	Burst int
	// Min retry delay when rate limited
	MinRetryDelay time.Duration
	// Max retry delay when rate limited
	MaxRetryDelay time.Duration
}

func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Rate:          10,
		Burst:         30,
		MinRetryDelay: 1 * time.Second,
		MaxRetryDelay: 10 * time.Second,
	}
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

	// Predicate function to determine whether the error returned from Handler is a failure.
	// If the function returns false, Server will not increment the retried counter for the task,
	// and Server won't record the queue stats (processed and failed stats) to avoid skewing the error
	// rate of the queue.
	//
	// By default, if the given error is non-nil the function returns true.
	IsFailure func(error) bool

	RateLimitConfig RateLimitConfig
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
		RateLimitConfig: DefaultRateLimitConfig(),
	}
}

func (c *Config) toAsynqConfig() asynq.Config {
	config := asynq.Config{
		Concurrency:     c.Concurrency,
		Queues:          c.Queues,
		Logger:          c.Logger,
		ShutdownTimeout: 10 * time.Second,
	}

	if c.IsFailure != nil {
		config.IsFailure = c.IsFailure
	}

	return config
}
