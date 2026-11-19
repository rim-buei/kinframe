package httpapi

import (
	"crypto/subtle"
	"errors"
	"html"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rim-buei/kinframe/internal/auth"
)

var loginPageTmpl = template.Must(template.New("login").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Sign in · kinframe</title>
  <link rel="stylesheet" href="/themes/default/static/styles.css">
  <style>
    main { max-width: 28rem; margin: 0 auto; padding: 4rem 1.25rem; }
    h1 { font-size: 2rem; font-weight: 600; letter-spacing: -0.02em; margin: 0 0 0.35rem; }
    .brand { font-size: 0.85rem; text-transform: uppercase; letter-spacing: 0.12em; color: var(--muted); margin-bottom: 2rem; }
    p.lead { color: var(--muted); margin: 0 0 2rem; line-height: 1.45; }
    .actions { display: flex; flex-direction: column; gap: 0.75rem; }
    .actions a {
      display: block; text-align: center; text-decoration: none;
      padding: 0.85rem 1rem; border: 1px solid #c5d0cb; background: rgba(255,255,255,0.65);
      color: var(--fg); border-radius: 2px;
    }
    .actions a:hover { border-color: var(--accent); color: var(--accent); }
    .muted { color: var(--muted); font-size: 0.9rem; margin-top: 1.5rem; }
    .err { color: #8b2e2e; margin-bottom: 1rem; }
  </style>
</head>
<body>
  <main>
    <div class="brand">kinframe</div>
    <h1>Sign in</h1>
    <p class="lead">Family albums via Google or Apple. No passwords.</p>
    {{if .Error}}<p class="err">{{.Error}}</p>{{end}}
    <div class="actions">
      {{if .Google}}<a href="/auth/login/google">Continue with Google</a>{{end}}
      {{if .Apple}}<a href="/auth/login/apple">Continue with Apple</a>{{end}}
      {{if .Dev}}<a href="/auth/login/dev">Dev login (local only)</a>{{end}}
    </div>
    {{if not .Any}}<p class="muted">No SSO providers configured. Set Google/Apple client credentials, or DEV_FAKE_OIDC=1 for local smoke tests.</p>{{end}}
  </main>
</body>
</html>`))

type loginPageData struct {
	Google bool
	Apple  bool
	Dev    bool
	Any    bool
	Error  string
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.AccountFromContext(r.Context()); ok {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	d := loginPageData{
		Google: s.oidc != nil && s.oidc.Configured(auth.ProviderGoogle),
		Apple:  s.oidc != nil && s.oidc.Configured(auth.ProviderApple),
		Dev:    s.oidc != nil && s.oidc.Configured(auth.ProviderDev),
		Error:  r.URL.Query().Get("error"),
	}
	d.Any = d.Google || d.Apple || d.Dev
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = loginPageTmpl.Execute(w, d)
}

func (s *Server) handleAuthStart(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil || s.sessions == nil {
		http.Error(w, "auth not configured", http.StatusServiceUnavailable)
		return
	}
	provider, err := auth.ParseProvider(chi.URLParam(r, "provider"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !s.oidc.Configured(provider) {
		http.Error(w, "provider not configured", http.StatusServiceUnavailable)
		return
	}
	returnTo := sanitizeReturnTo(r.URL.Query().Get("return_to"))
	st, err := auth.NewOIDCState(provider, returnTo)
	if err != nil {
		s.logger.Error("oidc state", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	sealed, err := auth.SealState(s.sealer, st)
	if err != nil {
		s.logger.Error("seal state", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.sessions.SetStateCookie(w, sealed)
	urlStr, err := s.oidc.AuthCodeURL(provider, auth.StateParam(sealed))
	if err != nil {
		if errors.Is(err, auth.ErrNotConfigured) {
			http.Error(w, "provider not configured", http.StatusServiceUnavailable)
			return
		}
		s.logger.Error("auth url", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, urlStr, http.StatusFound)
}

func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil || s.sessions == nil || s.sealer == nil {
		http.Error(w, "auth not configured", http.StatusServiceUnavailable)
		return
	}
	provider, err := auth.ParseProvider(chi.URLParam(r, "provider"))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/login?error="+url.QueryEscape("invalid callback"), http.StatusFound)
		return
	}
	if e := r.Form.Get("error"); e != "" {
		desc := r.Form.Get("error_description")
		msg := e
		if desc != "" {
			msg = desc
		}
		http.Redirect(w, r, "/login?error="+url.QueryEscape(msg), http.StatusFound)
		return
	}

	stateParam := r.Form.Get("state")
	cookieState, err := auth.ReadStateCookie(r)
	if err != nil || stateParam == "" || subtleConstantTimeEq(stateParam, cookieState) == false {
		http.Redirect(w, r, "/login?error="+url.QueryEscape("invalid state"), http.StatusFound)
		return
	}
	st, err := auth.OpenState(s.sealer, cookieState, provider)
	if err != nil {
		http.Redirect(w, r, "/login?error="+url.QueryEscape("expired or invalid state"), http.StatusFound)
		return
	}
	s.sessions.ClearStateCookie(w)

	code := r.Form.Get("code")
	ident, err := s.oidc.Exchange(r.Context(), provider, code, r.Form)
	if err != nil {
		s.logger.Error("oidc exchange", "provider", provider, "err", err)
		http.Redirect(w, r, "/login?error="+url.QueryEscape("sign-in failed"), http.StatusFound)
		return
	}

	account, err := auth.UpsertAccount(r.Context(), s.db, ident)
	if err != nil {
		s.logger.Error("upsert account", "err", err)
		http.Redirect(w, r, "/login?error="+url.QueryEscape("account error"), http.StatusFound)
		return
	}
	if _, err := s.sessions.Create(r.Context(), w, account.ID); err != nil {
		s.logger.Error("create session", "err", err)
		http.Redirect(w, r, "/login?error="+url.QueryEscape("session error"), http.StatusFound)
		return
	}

	dest := st.ReturnTo
	if dest == "" {
		dest = "/"
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if s.sessions != nil {
		_ = s.sessions.Revoke(r.Context(), w, r)
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	a, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":           a.ID,
		"provider":     a.Provider,
		"email":        a.Email,
		"display_name": a.DisplayName,
		"avatar_url":   a.AvatarURL,
	})
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	a, ok := auth.AccountFromContext(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	_, _ = w.Write([]byte(`<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><title>kinframe</title>
<link rel="stylesheet" href="/themes/default/static/styles.css">
<style>main{max-width:40rem;margin:0 auto;padding:3rem 1.25rem}nav{margin-bottom:2rem}</style>
</head><body><main>
<nav><strong>kinframe</strong> · signed in as ` + html.EscapeString(a.DisplayName) + ` · <a href="/auth/logout">Log out</a></nav>
<p>Spaces and albums come next. Session auth is live.</p>
<p><a href="/api/me">/api/me</a></p>
</main></body></html>`))
}

func sanitizeReturnTo(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || !strings.HasPrefix(v, "/") || strings.HasPrefix(v, "//") {
		return ""
	}
	return v
}

func subtleConstantTimeEq(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
