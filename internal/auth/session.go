package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

const (
	// SessionCookieName is the encrypted HTTP-only session cookie.
	SessionCookieName = "kf_session"
	// StateCookieName holds sealed OIDC CSRF state during the login redirect.
	StateCookieName = "kf_oidc_state"
)

// ErrNoSession means the request has no valid session.
var ErrNoSession = errors.New("no session")

// Session is a server-side session row.
type Session struct {
	ID        string
	AccountID string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// SessionManager creates, loads, and revokes cookie-backed DB sessions.
type SessionManager struct {
	db     *sql.DB
	sealer *Sealer
	ttl    time.Duration
	secure bool
}

// NewSessionManager wires cookie sealing to the sessions table.
func NewSessionManager(db *sql.DB, sealer *Sealer, ttl time.Duration, secureCookie bool) *SessionManager {
	return &SessionManager{db: db, sealer: sealer, ttl: ttl, secure: secureCookie}
}

// Create issues a new session row and sets the encrypted session cookie.
func (m *SessionManager) Create(ctx context.Context, w http.ResponseWriter, accountID string) (Session, error) {
	raw, err := randomToken(32)
	if err != nil {
		return Session{}, err
	}
	hash := hashToken(raw)
	id := uuid.NewString()
	expires := time.Now().UTC().Add(m.ttl)
	const q = `
INSERT INTO sessions (id, account_id, token_hash, expires_at)
VALUES (?, ?, ?, ?)
`
	_, err = m.db.ExecContext(ctx, q, id, accountID, hash, expires.Format("2006-01-02 15:04:05"))
	if err != nil {
		return Session{}, fmt.Errorf("insert session: %w", err)
	}

	sealed, err := m.sealer.Seal([]byte(raw))
	if err != nil {
		return Session{}, err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sealed,
		Path:     "/",
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
		MaxAge:   int(m.ttl.Seconds()),
	})
	return Session{ID: id, AccountID: accountID, ExpiresAt: expires, CreatedAt: time.Now().UTC()}, nil
}

// LoadAccount reads the session cookie, verifies the DB row, and returns the account.
func (m *SessionManager) LoadAccount(ctx context.Context, r *http.Request) (Account, Session, error) {
	c, err := r.Cookie(SessionCookieName)
	if err != nil || c.Value == "" {
		return Account{}, Session{}, ErrNoSession
	}
	rawBytes, err := m.sealer.Open(c.Value)
	if err != nil {
		return Account{}, Session{}, ErrNoSession
	}
	hash := hashToken(string(rawBytes))

	const q = `
SELECT s.id, s.account_id, s.expires_at, s.created_at,
       a.id, a.provider, a.provider_subject, a.email, a.display_name, a.avatar_url, a.created_at
FROM sessions s
JOIN accounts a ON a.id = s.account_id
WHERE s.token_hash = ?
`
	var sess Session
	var a Account
	var exp, sCreated, aCreated string
	err = m.db.QueryRowContext(ctx, q, hash).Scan(
		&sess.ID, &sess.AccountID, &exp, &sCreated,
		&a.ID, &a.Provider, &a.ProviderSubject, &a.Email, &a.DisplayName, &a.AvatarURL, &aCreated,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, Session{}, ErrNoSession
	}
	if err != nil {
		return Account{}, Session{}, err
	}
	sess.ExpiresAt, _ = time.Parse("2006-01-02 15:04:05", exp)
	sess.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", sCreated)
	a.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", aCreated)
	if time.Now().UTC().After(sess.ExpiresAt) {
		_ = m.deleteByID(ctx, sess.ID)
		return Account{}, Session{}, ErrNoSession
	}
	return a, sess, nil
}

// Revoke deletes the session row (if any) and clears the cookie.
func (m *SessionManager) Revoke(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	c, err := r.Cookie(SessionCookieName)
	if err == nil && c.Value != "" {
		if raw, openErr := m.sealer.Open(c.Value); openErr == nil {
			_, _ = m.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, hashToken(string(raw)))
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
	})
	return nil
}

// SetStateCookie stores sealed OIDC CSRF state.
func (m *SessionManager) SetStateCookie(w http.ResponseWriter, sealed string) {
	http.SetCookie(w, &http.Cookie{
		Name:     StateCookieName,
		Value:    sealed,
		Path:     "/",
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(stateTTL.Seconds()),
	})
}

// ClearStateCookie removes the OIDC state cookie.
func (m *SessionManager) ClearStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     StateCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
	})
}

// ReadStateCookie returns the sealed state cookie value.
func ReadStateCookie(r *http.Request) (string, error) {
	c, err := r.Cookie(StateCookieName)
	if err != nil || c.Value == "" {
		return "", ErrInvalidState
	}
	return c.Value, nil
}

func (m *SessionManager) deleteByID(ctx context.Context, id string) error {
	_, err := m.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
