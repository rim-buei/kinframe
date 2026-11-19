// Package httpapi provides the chi HTTP surface for the app process.
package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rim-buei/kinframe/internal/config"
)

// Server is the HTTP app.
type Server struct {
	cfg    config.Config
	db     *sql.DB
	logger *slog.Logger
	http   *http.Server
}

// NewServer builds a chi router with health and placeholder API routes.
func NewServer(cfg config.Config, db *sql.DB, logger *slog.Logger) *Server {
	s := &Server{cfg: cfg, db: db, logger: logger}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/healthz", s.handleHealthz)

	r.Route("/api", func(r chi.Router) {
		r.Get("/", s.handleAPIRoot)
		// Placeholder routes — wired in later phases.
		r.Get("/spaces", s.notImplemented("list spaces"))
		r.Post("/spaces", s.notImplemented("create space"))
		r.Get("/albums", s.notImplemented("list albums"))
		r.Get("/auth/login/{provider}", s.notImplemented("OIDC login start"))
		r.Get("/auth/callback/{provider}", s.notImplemented("OIDC callback"))
	})

	s.http = &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s
}

// ListenAndServe starts the HTTP server until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.http.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.http.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "unhealthy",
			"error":  "database unreachable",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAPIRoot(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":    "kinframe",
		"version": "scaffold",
		"note":    "API placeholders; see architecture Phase A",
	})
}

func (s *Server) notImplemented(op string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "not implemented",
			"op":    op,
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
