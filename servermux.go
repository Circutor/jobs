package jobs

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
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
	handlers    map[string]Handler
	gormDB      *gorm.DB
	middlewares []Middleware
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

// Use appends a Middleware to the chain.
// Middlewares are executed in the order that they are applied to the ServeMux.
func (m *ServerMux) Use(mws ...Middleware) {
	m.middlewares = append(m.middlewares, mws...)
}

func wrapHandler(originalHandler Handler) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		return originalHandler(ctx, &Task{
			ID:      t.ResultWriter().TaskID(),
			Kind:    t.Type(),
			Payload: t.Payload(),
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
				ID:      t.ResultWriter().TaskID(),
				Kind:    t.Type(),
				Payload: t.Payload(),
			})
		})
	}
}

func (m *ServerMux) asynqServerMux(gormDB *gorm.DB) *asynq.ServeMux {
	// TODO: Likely the gormDB should be a field of ServerMux set during
	// initialization.
	m.gormDB = gormDB

	asynqMux := asynq.NewServeMux()
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

func (m *ServerMux) saveResultsMiddleware(h Handler) Handler {
	return func(ctx context.Context, t *Task) error {
		err := h(ctx, t)
		if err != nil {
			return fmt.Errorf("ServerMux.handler(%v) - err: %w", t, err)
		}

		result := t.Result
		fmt.Println(result)
		_, err = t.toAsynqTask().ResultWriter().Write(result)
		if err != nil {
			return fmt.Errorf("ServerMux.saveResultsMiddleware.Write(%v) - err: %w", result, err)
		}

		return nil
	}
}
