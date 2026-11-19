package auth

import (
	"net/http"
)

// Middleware loads the current session account into the request context when present.
// Missing or invalid sessions are ignored (handlers decide whether login is required).
func Middleware(sessions *SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if sessions != nil {
				if a, _, err := sessions.LoadAccount(r.Context(), r); err == nil {
					r = r.WithContext(ContextWithAccount(r.Context(), a))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAccount redirects browsers to /login or returns 401 for API paths.
func RequireAccount(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := AccountFromContext(r.Context()); !ok {
			if len(r.URL.Path) >= 4 && r.URL.Path[:4] == "/api" {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}
