// Package httpapi provides the chi HTTP surface for the app process.
package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rim-buei/kinframe/internal/auth"
	"github.com/rim-buei/kinframe/internal/config"
	"github.com/rim-buei/kinframe/internal/media"
	"github.com/rim-buei/kinframe/internal/space"
	"github.com/rim-buei/kinframe/internal/storage"
)

// Server is the HTTP app.
type Server struct {
	cfg      config.Config
	db       *sql.DB
	logger   *slog.Logger
	http     *http.Server
	sealer   *auth.Sealer
	sessions *auth.SessionManager
	oidc     auth.OIDCClient
	spaces   *space.Store
	media    *media.Store
	objects  storage.ObjectStore
}

// NewServer builds a chi router with health, auth, spaces, albums, and media routes.
func NewServer(cfg config.Config, db *sql.DB, logger *slog.Logger, oidc auth.OIDCClient, sealer *auth.Sealer, objects storage.ObjectStore) (*Server, error) {
	s := &Server{
		cfg:     cfg,
		db:      db,
		logger:  logger,
		oidc:    oidc,
		sealer:  sealer,
		spaces:  space.NewStore(db),
		media:   media.NewStore(db, objects, cfg.MaxUploadBytes),
		objects: objects,
	}
	if sealer != nil {
		s.sessions = auth.NewSessionManager(db, sealer, cfg.SessionTTL, cfg.CookieSecure())
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(120 * time.Second))
	if s.sessions != nil {
		r.Use(auth.Middleware(s.sessions))
	}

	r.Get("/healthz", s.handleHealthz)

	r.Get("/", s.handleSpacesHome)
	r.Post("/spaces", s.handleSpacesHome)
	r.Get("/spaces/{spaceID}", s.handleSpacePage)
	r.Post("/spaces/{spaceID}/invites", s.handleSpaceCreateInviteForm)
	r.Post("/spaces/{spaceID}/invites/{inviteID}/revoke", s.handleSpaceRevokeInviteForm)
	r.Post("/spaces/{spaceID}/albums", s.handleSpaceCreateAlbumForm)

	r.Get("/albums/{albumID}", s.handleAlbumPage)
	r.Post("/albums/{albumID}/upload", s.handleAlbumUploadForm)
	r.Post("/albums/{albumID}/media/{mediaID}/delete", s.handleAlbumDeleteMediaForm)
	r.Post("/albums/{albumID}/delete", s.handleAlbumDeleteForm)

	r.Get("/invite/{token}", s.handleInvitePage)
	r.Post("/invite/{token}/accept", s.handleInviteAcceptForm)

	r.Get("/login", s.handleLoginPage)
	r.Get("/auth/login/{provider}", s.handleAuthStart)
	r.Get("/auth/callback/{provider}", s.handleAuthCallback)
	r.Post("/auth/callback/{provider}", s.handleAuthCallback)
	r.Get("/auth/logout", s.handleLogout)
	r.Post("/auth/logout", s.handleLogout)

	r.Get("/themes/{themeID}/static/*", s.handleThemeStatic)

	r.Route("/api", func(r chi.Router) {
		r.Get("/", s.handleAPIRoot)
		r.Get("/me", s.handleMe)

		r.Get("/spaces", s.handleListSpaces)
		r.Post("/spaces", s.handleCreateSpace)
		r.Get("/spaces/{spaceID}", s.handleGetSpace)
		r.Patch("/spaces/{spaceID}", s.handleUpdateSpace)
		r.Delete("/spaces/{spaceID}", s.handleDeleteSpace)

		r.Get("/spaces/{spaceID}/members", s.handleListMembers)
		r.Patch("/spaces/{spaceID}/members/{accountID}", s.handleUpdateMember)
		r.Delete("/spaces/{spaceID}/members/{accountID}", s.handleRemoveMember)

		r.Get("/spaces/{spaceID}/invites", s.handleListInvites)
		r.Post("/spaces/{spaceID}/invites", s.handleCreateInvite)
		r.Delete("/spaces/{spaceID}/invites/{inviteID}", s.handleRevokeInvite)

		r.Post("/invites/accept", s.handleAcceptInviteAPI)

		r.Get("/spaces/{spaceID}/albums", s.handleListAlbums)
		r.Post("/spaces/{spaceID}/albums", s.handleCreateAlbum)
		r.Get("/albums/{albumID}", s.handleGetAlbum)
		r.Patch("/albums/{albumID}", s.handleUpdateAlbum)
		r.Delete("/albums/{albumID}", s.handleDeleteAlbum)
		r.Get("/albums/{albumID}/media", s.handleListMedia)
		r.Post("/albums/{albumID}/media", s.handleUploadMedia)
		r.Delete("/media/{mediaID}", s.handleDeleteMedia)
		r.Get("/media/{mediaID}/file", s.handleMediaFile)
	})

	s.http = &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s, nil
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
		"version": "0.3.0-media",
		"auth": map[string]bool{
			"google": s.oidc != nil && s.oidc.Configured(auth.ProviderGoogle),
			"apple":  s.oidc != nil && s.oidc.Configured(auth.ProviderApple),
			"dev":    s.oidc != nil && s.oidc.Configured(auth.ProviderDev),
		},
	})
}

func (s *Server) handleThemeStatic(w http.ResponseWriter, r *http.Request) {
	themeID := chi.URLParam(r, "themeID")
	if themeID == "" || strings.Contains(themeID, "..") {
		http.NotFound(w, r)
		return
	}
	star := chi.URLParam(r, "*")
	base := filepath.Join(s.cfg.ThemesDir, themeID, "static")
	full := filepath.Join(base, filepath.Clean("/"+star))
	rel, err := filepath.Rel(base, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		http.NotFound(w, r)
		return
	}
	if _, err := os.Stat(full); err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, full)
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

// BuildOIDC constructs the OIDC client (real and/or fake) for the app process.
func BuildOIDC(ctx context.Context, cfg config.Config, logger *slog.Logger) (auth.OIDCClient, error) {
	var real auth.OIDCClient
	needDiscovery := cfg.GoogleClientID != "" || cfg.AppleClientID != ""
	if needDiscovery {
		c, err := auth.NewOIDC(ctx, cfg)
		if err != nil {
			if !cfg.DevFakeOIDC {
				return nil, err
			}
			logger.Warn("OIDC discovery failed; continuing with DEV_FAKE_OIDC only", "err", err)
		} else {
			real = c
		}
	}
	if cfg.DevFakeOIDC {
		logger.Warn("DEV_FAKE_OIDC enabled — local smoke IdP only; disable in production")
		return auth.NewFakeOIDC(cfg, real), nil
	}
	if real == nil {
		return auth.NewOIDCEmpty(), nil
	}
	return real, nil
}

// RequireSessionSecret ensures SESSION_SECRET is present when serving auth cookies.
func RequireSessionSecret(cfg config.Config) (*auth.Sealer, error) {
	if cfg.SessionSecret == "" {
		return nil, fmt.Errorf("SESSION_SECRET is required (set a long random string in the environment)")
	}
	return auth.NewSealer(cfg.SessionSecret)
}
