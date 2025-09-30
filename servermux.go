package jobs

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/hibiken/asynq"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
)

// Handler is the definition of a Handler func for job execution
type Handler func(context.Context, *Task) error

// Middleware is a function which receives a Handler and returns another Handler.
// Typically, the returned handler is a closure which does something with the context and task passed
// to it, and then calls the handler passed as parameter to the MiddlewareFunc.
type Middleware func(Handler) Handler

// ServerMux is a wrapper around asynq.ServeMux, that keeps track of all the
// kinds of jobs with their handlers.
type ServerMux struct {
	handlers          map[string]Handler
	gormDB            *gorm.DB
	middlewares       []Middleware
	globalRateLimiter *rate.Limiter
}

// NewServerMux returns a new server mux.
func NewServerMux() *ServerMux {
	return &ServerMux{
		handlers: make(map[string]Handler),
	}
}

// HandleFunc registers a handler for a given kind of job.
func (m *ServerMux) HandleFunc(kind string, handler Handler) {
	m.handlers[kind] = handler
}

func (m *ServerMux) HandleFuncWithRateLimit(kind string, handler Handler, config RateLimitConfig) {
	rateLimitedHandler := m.RateLimitMiddleware(config)(handler)
	m.HandleFunc(kind, rateLimitedHandler)
}

// Use appends a Middleware to the chain.
// Middlewares are executed in the order that they are applied to the ServeMux.
func (m *ServerMux) Use(mws ...Middleware) {
	m.middlewares = append(m.middlewares, mws...)
}

func (m *ServerMux) SetGlobalRateLimit(config RateLimitConfig) {
	m.globalRateLimiter = rate.NewLimiter(rate.Limit(config.Rate), config.Burst)

	globalRateLimitMW := func(next Handler) Handler {
		return func(ctx context.Context, t *Task) error {
			if !m.globalRateLimiter.Allow() {
				retryRange := config.MaxRetryDelay - config.MinRetryDelay
				randomDelay := time.Duration(rand.Float64() * float64(retryRange))
				retryIn := config.MinRetryDelay + randomDelay

				return &RateLimitError{
					RetryIn: retryIn,
				}
			}

			return next(ctx, t)
		}
	}

	m.middlewares = append(m.middlewares, globalRateLimitMW)
}

func (m *ServerMux) createPerHandlerRateLimit(rateLimitConfig RateLimitConfig) Middleware {
	limiter := rate.NewLimiter(rate.Limit(rateLimitConfig.Rate), rateLimitConfig.Burst)

	return func(next Handler) Handler {
		return func(ctx context.Context, t *Task) error {
			if !limiter.Allow() {
				retryRange := rateLimitConfig.MaxRetryDelay - rateLimitConfig.MinRetryDelay
				randomDelay := time.Duration(rand.Float64() * float64(retryRange))
				retryIn := rateLimitConfig.MinRetryDelay + randomDelay

				return &RateLimitError{
					RetryIn: retryIn,
				}
			}

			return next(ctx, t)
		}
	}
}

func wrapHandler(originalHandler Handler) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		_, origianlPayload := unwrapPayload(t.Payload())

		return originalHandler(ctx, &Task{
			ID:           t.ResultWriter().TaskID(),
			Kind:         t.Type(),
			Payload:      origianlPayload,
			originalTask: t,
		})
	}
}

func wrapMiddleware(originalMiddleware Middleware) asynq.MiddlewareFunc {
	return func(h asynq.Handler) asynq.Handler {
		return asynq.HandlerFunc(func(ctx context.Context, t *asynq.Task) error {
			adaptedHandler := func(ctx context.Context, task *Task) error {
				return h.ProcessTask(ctx, t)
			}
			wrappedHandler := originalMiddleware(adaptedHandler)

			return wrappedHandler(ctx, &Task{
				ID:           t.ResultWriter().TaskID(),
				Kind:         t.Type(),
				Payload:      t.Payload(),
				originalTask: t,
			})
		})
	}
}

func (m *ServerMux) asynqServerMux(gormDB *gorm.DB) *asynq.ServeMux {
	m.gormDB = gormDB

	asynqMux := asynq.NewServeMux()
	asynqMux.Use(m.sequentialTaskMiddleware)
	asynqMux.Use(m.dbMiddleware)

	for _, mw := range m.middlewares {
		asynqMux.Use(wrapMiddleware(mw))
	}

	for kind, handler := range m.handlers {
		asynqMux.HandleFunc(kind, wrapHandler(handler))
	}

	return asynqMux
}

func (m *ServerMux) dbMiddleware(h asynq.Handler) asynq.Handler {
	return asynq.HandlerFunc(func(ctx context.Context, t *asynq.Task) error {
		jobsTask := fromAsynqTask(t)

		if m.gormDB != nil {
			taskInfo := jobsTask.toTaskInfo(TaskInfoStatusRunning)
			//nolint staticcheck
			if err := m.gormDB.Updates(taskInfo.toDBTaskInfo()).Error; err != nil {
				// TODO: log error
			}
		}

		var err error
		defer func() {
			if m.gormDB != nil {
				status := TaskInfoStatusFinished
				if err != nil {
					status = TaskInfoStatusFailed
				}

				taskInfo := jobsTask.toTaskInfo(status)
				//nolint staticcheck
				if err := m.gormDB.Updates(taskInfo.toDBTaskInfo()).Error; err != nil {
					// TODO: log error
				}
			}
		}()

		if err = h.ProcessTask(ctx, t); err != nil {
			return fmt.Errorf("h.ProcessTask %w", err)
		}

		return nil
	})
}

func (m *ServerMux) RateLimitMiddleware(rateLimitConfig RateLimitConfig) Middleware {
	return m.createPerHandlerRateLimit(rateLimitConfig)
}

func (m *ServerMux) sequentialTaskMiddleware(h asynq.Handler) asynq.Handler {
	return asynq.HandlerFunc(func(ctx context.Context, t *asynq.Task) error {
		sequential, _ := unwrapPayload(t.Payload())

		if sequential {
			if m.gormDB != nil {
				running, err := m.isAnotherTaskOfSameKindRunning(t.Type())
				if err != nil {
					return fmt.Errorf("m.isAnotherTaskOfSameKindRunning %w", err)
				}

				if running {
					return &RateLimitError{
						RetryIn: time.Second * 10,
					}
				}
			}
		}

		return h.ProcessTask(ctx, t)
	})
}

func (m *ServerMux) isAnotherTaskOfSameKindRunning(kind string) (bool, error) {
	if m.gormDB == nil {
		return false, nil
	}

	var count int64
	err := m.gormDB.Model(&dbTaskInfo{}).
		Where("task_type = ? AND status = ?", kind, TaskInfoStatusRunning).
		Count(&count).Error

	if err != nil {
		return false, fmt.Errorf("m.gormDB.Model.Count %w", err)
	}

	return count > 0, nil
}
