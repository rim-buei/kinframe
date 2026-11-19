package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rim-buei/kinframe/internal/auth"
	"github.com/rim-buei/kinframe/internal/space"
)

func (s *Server) requireAccount(w http.ResponseWriter, r *http.Request) (auth.Account, bool) {
	a, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return auth.Account{}, false
	}
	return a, true
}

func writeSpaceErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, space.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case errors.Is(err, space.ErrForbidden):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
	case errors.Is(err, space.ErrInvalidInvite), errors.Is(err, space.ErrInviteGone):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
}

func spaceJSON(sw space.SpaceWithRole) map[string]any {
	return map[string]any{
		"id":         sw.Space.ID,
		"name":       sw.Space.Name,
		"slug":       sw.Space.Slug,
		"role":       sw.Role,
		"created_at": sw.Space.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func (s *Server) handleListSpaces(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	list, err := s.spaces.ListSpacesForAccount(r.Context(), a.ID)
	if err != nil {
		s.logger.Error("list spaces", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, sw := range list {
		out = append(out, spaceJSON(sw))
	}
	writeJSON(w, http.StatusOK, map[string]any{"spaces": out})
}

func (s *Server) handleCreateSpace(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sw, err := s.spaces.CreateSpace(r.Context(), body.Name, a.ID)
	if err != nil {
		writeSpaceErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, spaceJSON(sw))
}

func (s *Server) handleGetSpace(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	sp, m, err := s.spaces.RequireMembership(r.Context(), chi.URLParam(r, "spaceID"), a.ID, space.RoleViewer)
	if err != nil {
		writeSpaceErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, spaceJSON(space.SpaceWithRole{Space: sp, Role: m.Role}))
}

func (s *Server) handleUpdateSpace(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	sp, _, err := s.spaces.RequireMembership(r.Context(), chi.URLParam(r, "spaceID"), a.ID, space.RoleAdmin)
	if err != nil {
		writeSpaceErr(w, err)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	updated, err := s.spaces.UpdateSpaceName(r.Context(), sp.ID, body.Name)
	if err != nil {
		writeSpaceErr(w, err)
		return
	}
	m, _ := s.spaces.GetMembership(r.Context(), updated.ID, a.ID)
	writeJSON(w, http.StatusOK, spaceJSON(space.SpaceWithRole{Space: updated, Role: m.Role}))
}

func (s *Server) handleDeleteSpace(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	sp, _, err := s.spaces.RequireMembership(r.Context(), chi.URLParam(r, "spaceID"), a.ID, space.RoleOwner)
	if err != nil {
		writeSpaceErr(w, err)
		return
	}
	if err := s.spaces.SoftDeleteSpace(r.Context(), sp.ID); err != nil {
		writeSpaceErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	sp, _, err := s.spaces.RequireMembership(r.Context(), chi.URLParam(r, "spaceID"), a.ID, space.RoleViewer)
	if err != nil {
		writeSpaceErr(w, err)
		return
	}
	members, err := s.spaces.ListMembers(r.Context(), sp.ID)
	if err != nil {
		s.logger.Error("list members", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	out := make([]map[string]any, 0, len(members))
	for _, m := range members {
		out = append(out, map[string]any{
			"account_id":   m.AccountID,
			"role":         m.Role,
			"email":        m.Email,
			"display_name": m.DisplayName,
			"created_at":   m.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": out})
}

func (s *Server) handleUpdateMember(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	sp, actor, err := s.spaces.RequireMembership(r.Context(), chi.URLParam(r, "spaceID"), a.ID, space.RoleAdmin)
	if err != nil {
		writeSpaceErr(w, err)
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	role, err := space.ParseInviteRole(body.Role)
	if err != nil {
		writeSpaceErr(w, err)
		return
	}
	if err := s.spaces.UpdateMemberRole(r.Context(), sp.ID, chi.URLParam(r, "accountID"), role, actor.Role); err != nil {
		writeSpaceErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	sp, actor, err := s.spaces.RequireMembership(r.Context(), chi.URLParam(r, "spaceID"), a.ID, space.RoleAdmin)
	if err != nil {
		writeSpaceErr(w, err)
		return
	}
	if err := s.spaces.RemoveMember(r.Context(), sp.ID, chi.URLParam(r, "accountID"), actor.Role); err != nil {
		writeSpaceErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	sp, _, err := s.spaces.RequireMembership(r.Context(), chi.URLParam(r, "spaceID"), a.ID, space.RoleAdmin)
	if err != nil {
		writeSpaceErr(w, err)
		return
	}
	var body struct {
		Role       string `json:"role"`
		ExpiresInH *int   `json:"expires_in_hours"`
		MaxUses    *int   `json:"max_uses"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	role := space.RoleViewer
	if body.Role != "" {
		role, err = space.ParseInviteRole(body.Role)
		if err != nil {
			writeSpaceErr(w, err)
			return
		}
	}
	var exp *time.Time
	if body.ExpiresInH != nil && *body.ExpiresInH > 0 {
		t := time.Now().UTC().Add(time.Duration(*body.ExpiresInH) * time.Hour)
		exp = &t
	}
	created, err := s.spaces.CreateInvite(r.Context(), sp.ID, a.ID, role, exp, body.MaxUses)
	if err != nil {
		writeSpaceErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":          created.Invite.ID,
		"role":        created.Invite.DefaultRole,
		"token":       created.Token,
		"invite_path": "/invite/" + created.Token,
		"invite_url":  strings.TrimRight(s.cfg.PublicBaseURL, "/") + "/invite/" + created.Token,
		"use_count":   created.Invite.UseCount,
		"max_uses":    created.Invite.MaxUses,
		"expires_at":  formatOptTime(created.Invite.ExpiresAt),
	})
}

func (s *Server) handleListInvites(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	sp, _, err := s.spaces.RequireMembership(r.Context(), chi.URLParam(r, "spaceID"), a.ID, space.RoleAdmin)
	if err != nil {
		writeSpaceErr(w, err)
		return
	}
	list, err := s.spaces.ListInvites(r.Context(), sp.ID)
	if err != nil {
		s.logger.Error("list invites", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, inv := range list {
		out = append(out, map[string]any{
			"id":         inv.ID,
			"role":       inv.DefaultRole,
			"use_count":  inv.UseCount,
			"max_uses":   inv.MaxUses,
			"expires_at": formatOptTime(inv.ExpiresAt),
			"revoked_at": formatOptTime(inv.RevokedAt),
			"created_by": inv.CreatedBy,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"invites": out})
}

func (s *Server) handleRevokeInvite(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	sp, _, err := s.spaces.RequireMembership(r.Context(), chi.URLParam(r, "spaceID"), a.ID, space.RoleAdmin)
	if err != nil {
		writeSpaceErr(w, err)
		return
	}
	if err := s.spaces.RevokeInvite(r.Context(), sp.ID, chi.URLParam(r, "inviteID")); err != nil {
		writeSpaceErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAcceptInviteAPI(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireAccount(w, r)
	if !ok {
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sp, m, err := s.spaces.AcceptInvite(r.Context(), body.Token, a.ID)
	if err != nil {
		writeSpaceErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, spaceJSON(space.SpaceWithRole{Space: sp, Role: m.Role}))
}

func formatOptTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}
