package jobs

import (
	"fmt"

	"github.com/hibiken/asynq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Server is a wrapper around asynq.Server.
type Server struct {
	config      Config
	asynqServer *asynq.Server
	gormDB      *gorm.DB
}

// ServerOption is a function for optional params that allow custom
// configurations for the server.
type ServerOption func(s *Server) error

// NewServer returns a new server.
func NewServer(redisURL string, options ...ServerOption) (*Server, error) {
	s := &Server{
		config: DefaultConfig(),
	}

	for _, option := range options {
		if err := option(s); err != nil {
			return nil, fmt.Errorf("option %w", err)
		}
	}

	s.asynqServer = asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisURL},
		s.config.toAsynqConfig(),
	)

	return s, nil
}

// WithServerDBMiddleware is a ServerOption that allows to set a custom database
// middleware, to store the tasks (an its statuses) in a database.
func WithServerDBMiddleware(postgresURL string) ServerOption {
	return func(s *Server) error {
		db, err := gorm.Open(postgres.Open(postgresURL), &gorm.Config{})
		if err != nil {
			return fmt.Errorf("failed to connect database: %v", err)
		}

		if err = db.AutoMigrate(&dbTaskInfo{}); err != nil {
			return fmt.Errorf("failed to migrate database: %v", err)
		}

		s.gormDB = db

		return nil
	}
}

// WithConfig is a ServerOption that allows to set a custom configuration.
func WithConfig(config Config) ServerOption {
	return func(s *Server) error {
		s.config = config
		return nil
	}
}

// Run starts the server.
func (s *Server) Run(mux *ServerMux) error {
	if err := s.asynqServer.Run(mux.asynqServerMux(s.gormDB)); err != nil {
		return fmt.Errorf(":s.asynqServer.Run %w", err)
	}

	return nil
}
