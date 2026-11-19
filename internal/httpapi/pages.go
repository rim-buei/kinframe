package httpapi

import (
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rim-buei/kinframe/internal/auth"
	"github.com/rim-buei/kinframe/internal/space"
)

var pageFuncs = template.FuncMap{
	"archiveLabel": archiveStateLabel,
}

var spacesHomeTmpl = template.Must(template.New("spaces").Funcs(pageFuncs).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Spaces · kinframe</title>
  <link rel="stylesheet" href="/themes/default/static/styles.css">
</head>
<body>
<main class="shell">
  <header class="topbar">
    <a class="brand-lockup" href="/">kinframe</a>
    <div class="topbar-meta">
      <span>{{.DisplayName}}</span>
      <a href="/auth/logout">Log out</a>
    </div>
  </header>
  <h1 class="page-title">Your spaces</h1>
  <p class="page-lead">Shared places for family albums.</p>
  {{if .Error}}<p class="flash flash-err" role="alert">{{.Error}}</p>{{end}}
  <form class="stack" method="post" action="/spaces">
    <input type="text" name="name" placeholder="New space name" required aria-label="New space name">
    <button type="submit">Create space</button>
  </form>
  {{if .Spaces}}
  <ul class="list">
    {{range .Spaces}}
    <li class="list-item">
      <a href="/spaces/{{.Space.Slug}}">{{.Space.Name}}</a>
      <span class="role-tag">{{.Role}}</span>
    </li>
    {{end}}
  </ul>
  {{else}}
  <p class="empty">No spaces yet. Create one to invite family.</p>
  {{end}}
</main>
</body>
</html>`))

var spaceDetailTmpl = template.Must(template.New("space").Funcs(pageFuncs).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Space.Name}} · kinframe</title>
  <link rel="stylesheet" href="/themes/default/static/styles.css">
</head>
<body>
<main class="shell">
  <header class="topbar">
    <a class="brand-lockup" href="/">kinframe</a>
    <div class="topbar-meta">
      <a class="crumb" href="/">Spaces</a>
      <span>{{.DisplayName}}</span>
      <a href="/auth/logout">Log out</a>
    </div>
  </header>
  <h1 class="page-title">{{.Space.Name}}</h1>
  <p class="meta">slug <code>{{.Space.Slug}}</code> · your role <strong>{{.Role}}</strong></p>
  {{if .Flash}}<p class="flash">{{.Flash}}</p>{{end}}
  {{if .Error}}<p class="flash flash-err" role="alert">{{.Error}}</p>{{end}}

  <h2 class="section-title">Albums</h2>
  {{if .CanEdit}}
  <form class="stack" method="post" action="/spaces/{{.Space.Slug}}/albums">
    <input type="text" name="title" placeholder="New album title" required aria-label="New album title">
    <button type="submit">Create album</button>
  </form>
  {{end}}
  {{if .Albums}}
  <ul class="list">
    {{range .Albums}}
    <li class="list-item">
      <a href="/albums/{{.ID}}">{{.Title}}</a>
      <span class="state state-{{.ArchiveState}}">{{archiveLabel .ArchiveState}}</span>
    </li>
    {{end}}
  </ul>
  {{else}}
  <p class="empty">No albums yet.</p>
  {{end}}

  <h2 class="section-title">Members</h2>
  <table class="table">
    <thead><tr><th>Name</th><th>Email</th><th>Role</th></tr></thead>
    <tbody>
    {{range .Members}}
    <tr><td>{{.DisplayName}}</td><td>{{.Email}}</td><td>{{.Role}}</td></tr>
    {{end}}
    </tbody>
  </table>

  {{if .CanInvite}}
  <h2 class="section-title">Invite link</h2>
  <form class="stack" method="post" action="/spaces/{{.Space.Slug}}/invites">
    <label class="field">Role
      <select name="role">
        <option value="viewer">viewer</option>
        <option value="editor">editor</option>
        <option value="admin">admin</option>
      </select>
    </label>
    <button type="submit">Create invite</button>
  </form>
  {{if .NewInviteURL}}
  <p class="meta">Share this link (shown once):</p>
  <p class="token"><a href="{{.NewInviteURL}}">{{.NewInviteURL}}</a></p>
  {{end}}

  {{if .Invites}}
  <h2 class="section-title">Invites</h2>
  <table class="table">
    <thead><tr><th>Role</th><th>Uses</th><th>Status</th><th></th></tr></thead>
    <tbody>
    {{range .Invites}}
    <tr>
      <td>{{.DefaultRole}}</td>
      <td>{{.UseCount}}{{if .MaxUses}} / {{.MaxUses}}{{end}}</td>
      <td>{{if .RevokedAt}}revoked{{else}}active{{end}}</td>
      <td>
        {{if not .RevokedAt}}
        <form method="post" action="/spaces/{{$.Space.Slug}}/invites/{{.ID}}/revoke">
          <button class="ghost" type="submit">Revoke</button>
        </form>
        {{end}}
      </td>
    </tr>
    {{end}}
    </tbody>
  </table>
  {{end}}
  {{end}}
</main>
</body>
</html>`))

var invitePageTmpl = template.Must(template.New("invite").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Join {{.SpaceName}} · kinframe</title>
  <link rel="stylesheet" href="/themes/default/static/styles.css">
</head>
<body>
<main class="shell shell-narrow">
  <p class="invite-kicker">kinframe</p>
  <h1 class="page-title">Join {{.SpaceName}}</h1>
  <p class="page-lead">You will join as <strong>{{.Role}}</strong>.</p>
  {{if .Error}}<p class="flash flash-err" role="alert">{{.Error}}</p>{{end}}
  {{if .LoggedIn}}
  <form method="post" action="/invite/{{.Token}}/accept">
    <button class="btn-block" type="submit">Accept invite</button>
  </form>
  {{else}}
  <p class="muted">Sign in first, then return to this link.</p>
  <a class="btn btn-block" href="/login?return_to={{.ReturnTo}}">Sign in</a>
  {{end}}
</main>
</body>
</html>`))

func (s *Server) handleSpacesHome(w http.ResponseWriter, r *http.Request) {
	a, ok := auth.AccountFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method == http.MethodPost {
		name := strings.TrimSpace(r.FormValue("name"))
		sw, err := s.spaces.CreateSpace(r.Context(), name, a.ID)
		if err != nil {
			s.renderSpacesHome(w, a, nil, err.Error())
			return
		}
		http.Redirect(w, r, "/spaces/"+sw.Space.Slug, http.StatusFound)
		return
	}
	list, err := s.spaces.ListSpacesForAccount(r.Context(), a.ID)
	if err != nil {
		s.logger.Error("list spaces ui", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.renderSpacesHome(w, a, list, "")
}

func (s *Server) renderSpacesHome(w http.ResponseWriter, a auth.Account, list []space.SpaceWithRole, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = spacesHomeTmpl.Execute(w, map[string]any{
		"DisplayName": a.DisplayName,
		"Spaces":      list,
		"Error":       errMsg,
	})
}

func (s *Server) handleSpacePage(w http.ResponseWriter, r *http.Request) {
	a, ok := auth.AccountFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	slug := chi.URLParam(r, "spaceID")
	sp, m, err := s.spaces.RequireMembership(r.Context(), slug, a.ID, space.RoleViewer)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	members, _ := s.spaces.ListMembers(r.Context(), sp.ID)
	albums, _ := s.media.ListAlbums(r.Context(), sp.ID)
	var invites []space.InviteLink
	canInvite := m.Role.CanCreateInvite()
	if canInvite {
		invites, _ = s.spaces.ListInvites(r.Context(), sp.ID)
	}
	flash := r.URL.Query().Get("flash")
	newURL := r.URL.Query().Get("invite_url")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = spaceDetailTmpl.Execute(w, map[string]any{
		"DisplayName":  a.DisplayName,
		"Space":        sp,
		"Role":         m.Role,
		"Members":      members,
		"Albums":       albums,
		"CanEdit":      m.Role.CanEditContent(),
		"CanInvite":    canInvite,
		"Invites":      invites,
		"NewInviteURL": newURL,
		"Flash":        flash,
		"Error":        r.URL.Query().Get("error"),
	})
}

func (s *Server) handleSpaceCreateAlbumForm(w http.ResponseWriter, r *http.Request) {
	a, ok := auth.AccountFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	slug := chi.URLParam(r, "spaceID")
	sp, _, err := s.spaces.RequireMembership(r.Context(), slug, a.ID, space.RoleEditor)
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	al, err := s.media.CreateAlbum(r.Context(), sp.ID, r.FormValue("title"), r.FormValue("description"), a.ID)
	if err != nil {
		http.Redirect(w, r, "/spaces/"+sp.Slug+"?error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/albums/"+al.ID, http.StatusFound)
}

func (s *Server) handleSpaceCreateInviteForm(w http.ResponseWriter, r *http.Request) {
	a, ok := auth.AccountFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	slug := chi.URLParam(r, "spaceID")
	sp, _, err := s.spaces.RequireMembership(r.Context(), slug, a.ID, space.RoleAdmin)
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	role, err := space.ParseInviteRole(r.FormValue("role"))
	if err != nil {
		role = space.RoleViewer
	}
	created, err := s.spaces.CreateInvite(r.Context(), sp.ID, a.ID, role, nil, nil)
	if err != nil {
		http.Redirect(w, r, "/spaces/"+sp.Slug+"?error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	inviteURL := strings.TrimRight(s.cfg.PublicBaseURL, "/") + "/invite/" + created.Token
	http.Redirect(w, r, "/spaces/"+sp.Slug+"?flash="+url.QueryEscape("Invite created")+"&invite_url="+url.QueryEscape(inviteURL), http.StatusFound)
}

func (s *Server) handleSpaceRevokeInviteForm(w http.ResponseWriter, r *http.Request) {
	a, ok := auth.AccountFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	slug := chi.URLParam(r, "spaceID")
	sp, _, err := s.spaces.RequireMembership(r.Context(), slug, a.ID, space.RoleAdmin)
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_ = s.spaces.RevokeInvite(r.Context(), sp.ID, chi.URLParam(r, "inviteID"))
	http.Redirect(w, r, "/spaces/"+sp.Slug+"?flash="+url.QueryEscape("Invite revoked"), http.StatusFound)
}

func (s *Server) handleInvitePage(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	inv, sp, err := s.spaces.LookupInviteByToken(r.Context(), token)
	if err != nil {
		http.Error(w, "invalid invite", http.StatusBadRequest)
		return
	}
	_, loggedIn := auth.AccountFromContext(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = invitePageTmpl.Execute(w, map[string]any{
		"SpaceName": sp.Name,
		"Role":      inv.DefaultRole,
		"Token":     token,
		"LoggedIn":  loggedIn,
		"ReturnTo":  url.QueryEscape("/invite/" + token),
		"Error":     r.URL.Query().Get("error"),
	})
}

func (s *Server) handleInviteAcceptForm(w http.ResponseWriter, r *http.Request) {
	a, ok := auth.AccountFromContext(r.Context())
	if !ok {
		token := chi.URLParam(r, "token")
		http.Redirect(w, r, "/login?return_to="+url.QueryEscape("/invite/"+token), http.StatusFound)
		return
	}
	token := chi.URLParam(r, "token")
	sp, _, err := s.spaces.AcceptInvite(r.Context(), token, a.ID)
	if err != nil {
		http.Redirect(w, r, "/invite/"+token+"?error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/spaces/"+sp.Slug, http.StatusFound)
}
