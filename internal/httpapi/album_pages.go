package httpapi

import (
	"html/template"
	"io"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/rim-buei/kinframe/internal/auth"
	"github.com/rim-buei/kinframe/internal/media"
	"github.com/rim-buei/kinframe/internal/space"
)

func uploadOpts(albumID, accountID, filename, contentType string, size int64, body io.Reader) media.UploadOptions {
	return media.UploadOptions{
		AlbumID:     albumID,
		UploadedBy:  accountID,
		Filename:    filename,
		ContentType: contentType,
		Size:        size,
		Body:        body,
	}
}

var albumPageTmpl = template.Must(template.New("album").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Album.Title}} · kinframe</title>
  <link rel="stylesheet" href="/themes/default/static/styles.css">
</head>
<body>
<main class="shell shell-wide">
  <header class="topbar">
    <a class="brand-lockup" href="/">kinframe</a>
    <div class="topbar-meta">
      <a class="crumb" href="/spaces/{{.SpaceSlug}}">{{.SpaceName}}</a>
      <span>{{.DisplayName}}</span>
      <a href="/auth/logout">Log out</a>
    </div>
  </header>
  <h1 class="page-title">{{.Album.Title}}</h1>
  <p class="meta">
    <span class="state state-{{.Album.ArchiveState}}">{{.StateLabel}}</span>
    {{if .Album.Description}} · {{.Album.Description}}{{end}}
  </p>
  {{if .Flash}}<p class="flash">{{.Flash}}</p>{{end}}
  {{if .Error}}<p class="flash flash-err" role="alert">{{.Error}}</p>{{end}}

  {{if .CanArchive}}
  <div class="archive-bar">
    {{if eq .Album.ArchiveState "hot"}}
    <form method="post" action="/albums/{{.Album.ID}}/archive"><button type="submit">Archive to Glacier</button></form>
    {{end}}
    {{if eq .Album.ArchiveState "glacier"}}
    <form method="post" action="/albums/{{.Album.ID}}/restore"><button type="submit">Restore from Glacier</button></form>
    {{end}}
    {{if or (eq .Album.ArchiveState "archiving") (eq .Album.ArchiveState "restoring")}}
    <p class="archive-hint">Job in progress… refresh for updates. Thumbnails stay available.</p>
    {{end}}
  </div>
  {{end}}

  {{if and .CanEdit (eq .Album.ArchiveState "hot")}}
  <form class="stack upload-panel" method="post" action="/albums/{{.Album.ID}}/upload" enctype="multipart/form-data">
    <input type="file" name="file" accept="image/*,video/*" required aria-label="Choose photo or video">
    <button type="submit">Upload</button>
  </form>
  {{end}}

  {{if .Media}}
  <div class="media-grid">
    {{range .Media}}
    <figure class="media-item">
      <a href="/api/media/{{.ID}}/file?kind=preview"><img src="/api/media/{{.ID}}/file?kind=thumb" alt=""></a>
      <figcaption>
        <span>{{.Kind}} · {{.ByteSize}} B</span>
        {{if $.CanEdit}}
        <form class="inline" method="post" action="/albums/{{$.Album.ID}}/media/{{.ID}}/delete">
          <button class="danger" type="submit">Delete</button>
        </form>
        {{end}}
      </figcaption>
    </figure>
    {{end}}
  </div>
  {{else}}
  <p class="empty">No media yet.</p>
  {{end}}

  {{if .CanEdit}}
  <div class="danger-zone">
    <form method="post" action="/albums/{{.Album.ID}}/delete" onsubmit="return confirm('Delete this album?')">
      <button class="danger" type="submit">Delete album</button>
    </form>
  </div>
  {{end}}
</main>
</body>
</html>`))

func (s *Server) handleAlbumPage(w http.ResponseWriter, r *http.Request) {
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
	sp, m, err := s.spaces.RequireMembership(r.Context(), al.SpaceID, a.ID, space.RoleViewer)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	items, _ := s.media.ListMedia(r.Context(), al.ID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = albumPageTmpl.Execute(w, map[string]any{
		"DisplayName": a.DisplayName,
		"Album":       al,
		"Media":       items,
		"SpaceSlug":   sp.Slug,
		"SpaceName":   sp.Name,
		"CanEdit":     m.Role.CanEditContent(),
		"CanArchive":  m.Role.CanArchive(),
		"StateLabel":  archiveStateLabel(al.ArchiveState),
		"Flash":       r.URL.Query().Get("flash"),
		"Error":       r.URL.Query().Get("error"),
	})
}

func archiveStateLabel(st media.ArchiveState) string {
	switch st {
	case media.ArchiveHot:
		return "warm"
	case media.ArchiveArchiving:
		return "archiving"
	case media.ArchiveGlacier:
		return "archived"
	case media.ArchiveRestoring:
		return "restoring"
	default:
		return string(st)
	}
}

func (s *Server) handleAlbumUploadForm(w http.ResponseWriter, r *http.Request) {
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
	if _, _, err := s.spaces.RequireMembership(r.Context(), al.SpaceID, a.ID, space.RoleEditor); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	maxMem := s.cfg.MaxUploadBytes + (1 << 20)
	if err := r.ParseMultipartForm(maxMem); err != nil {
		http.Redirect(w, r, "/albums/"+al.ID+"?error="+url.QueryEscape("invalid upload"), http.StatusFound)
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		http.Redirect(w, r, "/albums/"+al.ID+"?error="+url.QueryEscape("file required"), http.StatusFound)
		return
	}
	defer file.Close()
	ct := hdr.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	_, err = s.media.UploadMedia(r.Context(), uploadOpts(al.ID, a.ID, hdr.Filename, ct, hdr.Size, file))
	if err != nil {
		http.Redirect(w, r, "/albums/"+al.ID+"?error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/albums/"+al.ID+"?flash="+url.QueryEscape("Uploaded"), http.StatusFound)
}

func (s *Server) handleAlbumDeleteMediaForm(w http.ResponseWriter, r *http.Request) {
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
	if _, _, err := s.spaces.RequireMembership(r.Context(), al.SpaceID, a.ID, space.RoleEditor); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_ = s.media.SoftDeleteMedia(r.Context(), chi.URLParam(r, "mediaID"))
	http.Redirect(w, r, "/albums/"+al.ID+"?flash="+url.QueryEscape("Deleted"), http.StatusFound)
}

func (s *Server) handleAlbumDeleteForm(w http.ResponseWriter, r *http.Request) {
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
	sp, _, err := s.spaces.RequireMembership(r.Context(), al.SpaceID, a.ID, space.RoleEditor)
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := s.media.DeleteAlbum(r.Context(), al.ID); err != nil {
		http.Redirect(w, r, "/albums/"+al.ID+"?error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/spaces/"+sp.Slug+"?flash="+url.QueryEscape("Album deleted"), http.StatusFound)
}
