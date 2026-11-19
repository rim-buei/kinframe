package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rim-buei/kinframe/internal/media"
	"github.com/rim-buei/kinframe/internal/space"
)

func albumJSON(a media.Album) map[string]any {
	return map[string]any{
		"id":            a.ID,
		"space_id":      a.SpaceID,
		"title":         a.Title,
		"description":   a.Description,
		"archive_state": a.ArchiveState,
		"max_bytes":     a.MaxBytes,
		"max_items":     a.MaxItems,
		"created_at":    a.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":    a.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func mediaJSON(m media.MediaItem) map[string]any {
	return map[string]any{
		"id":           m.ID,
		"album_id":     m.AlbumID,
		"space_id":     m.SpaceID,
		"kind":         m.Kind,
		"byte_size":    m.ByteSize,
		"content_type": m.ContentType,
		"width":        m.Width,
		"height":       m.Height,
		"checksum":     m.Checksum,
		"uploaded_by":  m.UploadedBy,
		"created_at":   m.CreatedAt.UTC().Format(time.RFC3339),
		"thumb_url":    "/api/media/" + m.ID + "/file?kind=thumb",
		"preview_url":  "/api/media/" + m.ID + "/file?kind=preview",
		"original_url": "/api/media/" + m.ID + "/file?kind=original",
	}
}

func writeMediaErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, media.ErrNotFound), errors.Is(err, space.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case errors.Is(err, space.ErrForbidden):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
	case errors.Is(err, media.ErrForbiddenEdit):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, media.ErrTooLarge):
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": err.Error()})
	case errors.Is(err, media.ErrAlbumLimit):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
}

func (s *Server) handleListAlbums(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	sp, _, err := s.spaces.RequireMembership(r.Context(), chi.URLParam(r, "spaceID"), a.ID, space.RoleViewer)
	if err != nil {
		writeMediaErr(w, err)
		return
	}
	list, err := s.media.ListAlbums(r.Context(), sp.ID)
	if err != nil {
		s.logger.Error("list albums", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, al := range list {
		out = append(out, albumJSON(al))
	}
	writeJSON(w, http.StatusOK, map[string]any{"albums": out})
}

func (s *Server) handleCreateAlbum(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	sp, _, err := s.spaces.RequireMembership(r.Context(), chi.URLParam(r, "spaceID"), a.ID, space.RoleEditor)
	if err != nil {
		writeMediaErr(w, err)
		return
	}
	var body struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	al, err := s.media.CreateAlbum(r.Context(), sp.ID, body.Title, body.Description, a.ID)
	if err != nil {
		writeMediaErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, albumJSON(al))
}

func (s *Server) handleGetAlbum(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	al, err := s.media.GetAlbum(r.Context(), chi.URLParam(r, "albumID"))
	if err != nil {
		writeMediaErr(w, err)
		return
	}
	if _, _, err := s.spaces.RequireMembership(r.Context(), al.SpaceID, a.ID, space.RoleViewer); err != nil {
		writeMediaErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, albumJSON(al))
}

func (s *Server) handleUpdateAlbum(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	al, err := s.media.GetAlbum(r.Context(), chi.URLParam(r, "albumID"))
	if err != nil {
		writeMediaErr(w, err)
		return
	}
	if _, _, err := s.spaces.RequireMembership(r.Context(), al.SpaceID, a.ID, space.RoleEditor); err != nil {
		writeMediaErr(w, err)
		return
	}
	var body struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	updated, err := s.media.UpdateAlbum(r.Context(), al.ID, body.Title, body.Description)
	if err != nil {
		writeMediaErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, albumJSON(updated))
}

func (s *Server) handleDeleteAlbum(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	al, err := s.media.GetAlbum(r.Context(), chi.URLParam(r, "albumID"))
	if err != nil {
		writeMediaErr(w, err)
		return
	}
	if _, _, err := s.spaces.RequireMembership(r.Context(), al.SpaceID, a.ID, space.RoleEditor); err != nil {
		writeMediaErr(w, err)
		return
	}
	if err := s.media.DeleteAlbum(r.Context(), al.ID); err != nil {
		writeMediaErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListMedia(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	al, err := s.media.GetAlbum(r.Context(), chi.URLParam(r, "albumID"))
	if err != nil {
		writeMediaErr(w, err)
		return
	}
	if _, _, err := s.spaces.RequireMembership(r.Context(), al.SpaceID, a.ID, space.RoleViewer); err != nil {
		writeMediaErr(w, err)
		return
	}
	list, err := s.media.ListMedia(r.Context(), al.ID)
	if err != nil {
		s.logger.Error("list media", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, m := range list {
		out = append(out, mediaJSON(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"media": out})
}

func (s *Server) handleUploadMedia(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	al, err := s.media.GetAlbum(r.Context(), chi.URLParam(r, "albumID"))
	if err != nil {
		writeMediaErr(w, err)
		return
	}
	if _, _, err := s.spaces.RequireMembership(r.Context(), al.SpaceID, a.ID, space.RoleEditor); err != nil {
		writeMediaErr(w, err)
		return
	}

	maxMem := s.cfg.MaxUploadBytes + (1 << 20)
	if err := r.ParseMultipartForm(maxMem); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart form"})
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file required"})
		return
	}
	defer file.Close()

	ct := hdr.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	item, err := s.media.UploadMedia(r.Context(), media.UploadOptions{
		AlbumID:     al.ID,
		UploadedBy:  a.ID,
		Filename:    hdr.Filename,
		ContentType: ct,
		Size:        hdr.Size,
		Body:        file,
	})
	if err != nil {
		writeMediaErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, mediaJSON(item))
}

func (s *Server) handleDeleteMedia(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	item, err := s.media.GetMedia(r.Context(), chi.URLParam(r, "mediaID"))
	if err != nil {
		writeMediaErr(w, err)
		return
	}
	if item.DeletedAt != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if _, _, err := s.spaces.RequireMembership(r.Context(), item.SpaceID, a.ID, space.RoleEditor); err != nil {
		writeMediaErr(w, err)
		return
	}
	if err := s.media.SoftDeleteMedia(r.Context(), item.ID); err != nil {
		writeMediaErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMediaFile(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	item, err := s.media.GetMedia(r.Context(), chi.URLParam(r, "mediaID"))
	if err != nil || item.DeletedAt != nil {
		writeMediaErr(w, media.ErrNotFound)
		return
	}
	if _, _, err := s.spaces.RequireMembership(r.Context(), item.SpaceID, a.ID, space.RoleViewer); err != nil {
		writeMediaErr(w, err)
		return
	}
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "preview"
	}
	rc, ct, err := s.media.OpenObject(r.Context(), item, kind)
	if err != nil {
		writeMediaErr(w, err)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	if kind == "original" {
		w.Header().Set("Content-Length", strconv.FormatInt(item.ByteSize, 10))
	}
	_, _ = io.Copy(w, rc)
}
