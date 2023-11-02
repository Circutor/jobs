package jobs

import (
	"context"

	"github.com/hibiken/asynq"
)

// Handler is the definition of a Handler func for job execution
type Handler func(context.Context, *Task) error

// ServerMux is a wrapper around asynq.ServeMux, that keeps track of all the
// kinds of jobs with their handlers.
type ServerMux struct {
	handlers map[string]Handler
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

func wrapHandler(originalHandler Handler) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		return originalHandler(ctx, &Task{
			kind:    t.Type(),
			payload: t.Payload(),
		})
	}
}

func (m *ServerMux) asynqServerMux() *asynq.ServeMux {
	asynqMux := asynq.NewServeMux()

	for kind, handler := range m.handlers {
		asynqMux.HandleFunc(kind, wrapHandler(handler))
	}

	return asynqMux
}
