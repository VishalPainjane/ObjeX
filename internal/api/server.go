package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// Server wraps HTTP serving with logging middleware.
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

// NewServer builds an HTTP server for addr with the given handler.
func NewServer(addr string, handler http.Handler, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		httpServer: &http.Server{
			Addr:              addr,
			Handler:           loggingMiddleware(logger, handler),
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       5 * time.Minute,
			WriteTimeout:      5 * time.Minute,
		},
		logger: logger,
	}
}

// Start begins listening.
func (s *Server) Start() error {
	s.logger.Info("http server listening", "addr", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func loggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
