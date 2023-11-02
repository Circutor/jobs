package jobs

import (
	"fmt"

	"github.com/hibiken/asynq"
)

// Server is a wrapper around asynq.Server.
type Server struct {
	config      Config
	asynqServer *asynq.Server
}

// ServerOption is a function for optional params that allow custom
// configurations for the server.
type ServerOption func(s *Server)

// NewServer returns a new server.
func NewServer(redisURL string, options ...ServerOption) *Server {
	s := &Server{
		config: DefaultConfig(),
	}

	for _, option := range options {
		option(s)
	}

	s.asynqServer = asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisURL},
		s.config.toAsynqConfig(),
	)

	return s
}

// WithConfig is a ServerOption that allows to set a custom configuration.
func WithConfig(config Config) ServerOption {
	return func(s *Server) {
		s.config = config
	}
}

// Run starts the server.
func (s *Server) Run(mux *ServerMux) error {
	if err := s.asynqServer.Run(mux.asynqServerMux()); err != nil {
		return fmt.Errorf(":s.asynqServer.Run %w", err)
	}

	return nil
}
