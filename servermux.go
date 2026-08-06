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
	logger            Logger
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

func (m *ServerMux) asynqServerMux(gormDB *gorm.DB, logger Logger) *asynq.ServeMux {
	m.gormDB = gormDB
	m.logger = logger

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

		var stampedDeadline *time.Time

		if m.gormDB != nil {
			values := map[string]any{
				"status": TaskInfoStatusRunning,
				"result": "",
			}

			if deadline, ok := ctx.Deadline(); ok {
				deadline = deadline.Truncate(time.Microsecond)
				stampedDeadline = &deadline
				values["expires_at"] = deadline
			}

			if err := m.gormDB.Model(&dbTaskInfo{}).
				Where("id = ?", jobsTask.ID).
				Updates(values).Error; err != nil {
				m.logError("jobs: could not mark task id=%s kind=%s as running: %v", jobsTask.ID, t.Type(), err)
			}
		}

		var err error

		// returned tells the deferred update whether the handler actually
		// came back. It stays false when the handler panics.
		var returned bool

		defer func() {
			if m.gormDB != nil {
				status := TaskInfoStatusFinished
				if err != nil || !returned {
					status = TaskInfoStatusFailed
				}

				query := m.gormDB.Model(&dbTaskInfo{}).Where("id = ?", jobsTask.ID)
				if stampedDeadline != nil {
					query = query.Where("expires_at = ?", *stampedDeadline)
				}

				result := query.Updates(map[string]any{
					"status": status,
					"result": "",
				})
				if result.Error != nil {
					m.logError("jobs: could not record task id=%s kind=%s completion as %s: %v", jobsTask.ID, t.Type(), status, result.Error)
				} else if result.RowsAffected == 0 {
					m.logWarn("jobs: task id=%s kind=%s finished as %s after another attempt took over its row", jobsTask.ID, t.Type(), status)
				}
			}
		}()

		err = h.ProcessTask(ctx, t)
		returned = true

		if err != nil {
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
				m.sweepExpiredTasksByKind(t.Type())

				running, err := m.isAnotherTaskOfSameKindRunning(t.Type(), t.ResultWriter().TaskID())
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

func (m *ServerMux) isAnotherTaskOfSameKindRunning(kind, taskID string) (bool, error) {
	if m.gormDB == nil {
		return false, nil
	}

	var count int64
	err := m.gormDB.Model(&dbTaskInfo{}).
		Where("task_type = ? AND status = ? AND id <> ?", kind, TaskInfoStatusRunning, taskID).
		Count(&count).Error

	if err != nil {
		return false, fmt.Errorf("m.gormDB.Model.Count %w", err)
	}

	return count > 0, nil
}

// sweepExpiredTasksByKind marks as failed every running task of the given kind
// whose deadline has already passed, meaning no live process can still be
// executing it.
func (m *ServerMux) sweepExpiredTasksByKind(kind string) {
	if m.gormDB == nil {
		return
	}

	result := m.gormDB.Model(&dbTaskInfo{}).
		Where("task_type = ? AND status = ? AND (expires_at IS NULL OR expires_at < NOW())", kind, TaskInfoStatusRunning).
		Updates(map[string]any{
			"status": TaskInfoStatusFailed,
			"result": expiredTaskResult,
		})

	if result.Error != nil {
		m.logError("jobs: could not sweep expired tasks of kind=%s: %v", kind, result.Error)

		return
	}

	if result.RowsAffected > 0 {
		m.logWarn("jobs: reclaimed %d expired task(s) of kind=%s left running by a dead process", result.RowsAffected, kind)
	}
}

func (m *ServerMux) sweepAllExpiredTasks() {
	if m.gormDB == nil {
		return
	}

	result := m.gormDB.Model(&dbTaskInfo{}).
		Where("status = ? AND (expires_at IS NULL OR expires_at < NOW())", TaskInfoStatusRunning).
		Updates(map[string]any{
			"status": TaskInfoStatusFailed,
			"result": expiredTaskResult,
		})

	if result.Error != nil {
		m.logError("jobs: could not sweep expired tasks at startup: %v", result.Error)

		return
	}

	if result.RowsAffected > 0 {
		m.logWarn("jobs: reclaimed %d expired task(s) left running by a dead process", result.RowsAffected)
	}
}

func (m *ServerMux) logError(format string, args ...any) {
	if m.logger == nil {
		return
	}

	m.logger.Error(fmt.Sprintf(format, args...))
}

func (m *ServerMux) logWarn(format string, args ...any) {
	if m.logger == nil {
		return
	}

	m.logger.Warn(fmt.Sprintf(format, args...))
}
