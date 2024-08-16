package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/berachain/offchain-sdk/log"
)

// DefaultReadHeaderTimeout is the default timeout for reading the header.
const DefaultReadHeaderTimeout = 10 * time.Second

// Handler represents an HTTP handler with its path.
type Handler struct {
	Path    string
	Handler http.Handler
}

// Middleware defines a function that wraps an HTTP handler.
type Middleware func(http.Handler) http.Handler

// Server represents an HTTP server with configurable middleware and handlers.
type Server struct {
	cfg          *Config
	logger       log.Logger
	mux          *http.ServeMux
	srv          *http.Server
	closer       sync.Once
	middlewares  []Middleware
}

// New creates and returns a new Server instance.
func New(cfg *Config, logger log.Logger, middlewares ...Middleware) *Server {
	return &Server{
		cfg:         cfg,
		logger:      logger,
		mux:         http.NewServeMux(),
		middlewares: middlewares,
	}
}

// RegisterHandler adds a handler to the server for the specified path.
func (s *Server) RegisterHandler(h *Handler) {
	s.mux.Handle(h.Path, h.Handler)
}

// RegisterMiddleware adds a middleware to the server.
func (s *Server) RegisterMiddleware(m Middleware) {
	s.middlewares = append(s.middlewares, m)
}

// applyMiddlewares applies the middlewares to the server's handler in reverse order.
// The last middleware in the slice will be the outermost.
func (s *Server) applyMiddlewares() http.Handler {
	var h http.Handler = s.mux
	for i := len(s.middlewares) - 1; i >= 0; i-- {
		h = s.middlewares[i](h)
	}
	return h
}

// Start initializes and starts the server.
// This method is blocking and should be run in a separate goroutine.
func (s *Server) Start(ctx context.Context) {
	s.srv = &http.Server{
		Addr:              fmt.Sprintf("%s:%d", s.cfg.HTTP.Host, s.cfg.HTTP.Port),
		Handler:           s.applyMiddlewares(),
		ReadHeaderTimeout: DefaultReadHeaderTimeout,
	}

	go func() {
		s.logger.Info("Starting HTTP server", "address", s.srv.Addr)
		if err := s.srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("HTTP server error", "error", err)
		}
	}()

	// Wait for the context to be done
	<-ctx.Done()
	s.Stop()
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() {
	s.closer.Do(func() {
		// Give a timeout for graceful shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.srv.Shutdown(ctx); err != nil {
			s.logger.Error("HTTP server shutdown error", "error", err)
		} else {
			s.logger.Info("HTTP server gracefully stopped")
		}
	})
}
