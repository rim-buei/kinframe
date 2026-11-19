package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rim-buei/kinframe/internal/auth"
	"github.com/rim-buei/kinframe/internal/jobs"
	"github.com/rim-buei/kinframe/internal/space"
)

func jobJSON(j jobs.Job) map[string]any {
	return map[string]any{
		"id":           j.ID,
		"type":         j.Type,
		"album_id":     j.AlbumID,
		"status":       j.Status,
		"progress_pct": j.ProgressPct,
		"eta_seconds":  j.ETASeconds,
		"error":        nullIfEmpty(j.Error),
		"attempts":     j.Attempts,
		"created_at":   j.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":   j.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *Server) handleArchiveAlbum(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	al, err := s.media.GetAlbum(r.Context(), chi.URLParam(r, "albumID"))
	if err != nil {
		writeMediaErr(w, err)
		return
	}
	if _, m, err := s.spaces.RequireMembership(r.Context(), al.SpaceID, a.ID, space.RoleAdmin); err != nil {
		writeMediaErr(w, err)
		return
	} else if !m.Role.CanArchive() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	job, err := s.jobs.EnqueueArchive(r.Context(), al)
	if err != nil {
		writeArchiveErr(w, err)
		return
	}
	al2, _ := s.media.GetAlbum(r.Context(), al.ID)
	writeJSON(w, http.StatusAccepted, map[string]any{"album": albumJSON(al2), "job": jobJSON(job)})
}

func (s *Server) handleRestoreAlbum(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	al, err := s.media.GetAlbum(r.Context(), chi.URLParam(r, "albumID"))
	if err != nil {
		writeMediaErr(w, err)
		return
	}
	if _, m, err := s.spaces.RequireMembership(r.Context(), al.SpaceID, a.ID, space.RoleAdmin); err != nil {
		writeMediaErr(w, err)
		return
	} else if !m.Role.CanArchive() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	job, err := s.jobs.EnqueueRestore(r.Context(), al)
	if err != nil {
		writeArchiveErr(w, err)
		return
	}
	al2, _ := s.media.GetAlbum(r.Context(), al.ID)
	writeJSON(w, http.StatusAccepted, map[string]any{"album": albumJSON(al2), "job": jobJSON(job)})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	job, err := s.jobs.Get(r.Context(), chi.URLParam(r, "jobID"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if _, _, err := s.spaces.RequireMembership(r.Context(), job.SpaceID, a.ID, space.RoleViewer); err != nil {
		writeMediaErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, jobJSON(job))
}

func writeArchiveErr(w http.ResponseWriter, err error) {
	if errors.Is(err, jobs.ErrJobConflict) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
}

func (s *Server) handleAlbumArchiveForm(w http.ResponseWriter, r *http.Request) {
	a, ok := auth.AccountFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	al, err := s.media.GetAlbum(r.Context(), chi.URLParam(r, "albumID"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if _, m, err := s.spaces.RequireMembership(r.Context(), al.SpaceID, a.ID, space.RoleAdmin); err != nil || !m.Role.CanArchive() {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if _, err := s.jobs.EnqueueArchive(r.Context(), al); err != nil {
		http.Redirect(w, r, "/albums/"+al.ID+"?error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/albums/"+al.ID+"?flash="+url.QueryEscape("Archive queued"), http.StatusFound)
}

func (s *Server) handleAlbumRestoreForm(w http.ResponseWriter, r *http.Request) {
	a, ok := auth.AccountFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	al, err := s.media.GetAlbum(r.Context(), chi.URLParam(r, "albumID"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if _, m, err := s.spaces.RequireMembership(r.Context(), al.SpaceID, a.ID, space.RoleAdmin); err != nil || !m.Role.CanArchive() {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if _, err := s.jobs.EnqueueRestore(r.Context(), al); err != nil {
		http.Redirect(w, r, "/albums/"+al.ID+"?error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/albums/"+al.ID+"?flash="+url.QueryEscape("Restore queued"), http.StatusFound)
}
