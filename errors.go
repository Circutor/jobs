package jobs

import (
	"errors"
	"time"
)

type RateLimitError struct {
	RetryIn    time.Duration
	MaxRetries int
}

func (e *RateLimitError) Error() string {
	return "rate limit exceeded, retry after " + e.RetryIn.String()
}

func IsRateLimitError(err error) bool {
	_, ok := err.(*RateLimitError)
	return ok
}

func RetryDelayFunc(n int, err error, task *Task) time.Duration {
	var rateLimitErr *RateLimitError
	if errors.As(err, &rateLimitErr) {
		return rateLimitErr.RetryIn
	}

	return time.Duration(n) * time.Second
}
