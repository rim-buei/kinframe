package auth

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Account is a persisted SSO user row.
type Account struct {
	ID              string
	Provider        Provider
	ProviderSubject string
	Email           string
	DisplayName     string
	AvatarURL       string
	CreatedAt       time.Time
}

// UpsertAccount inserts or updates an account keyed by (provider, provider_subject).
func UpsertAccount(ctx context.Context, db *sql.DB, id Identity) (Account, error) {
	if id.Provider == "" || id.ProviderSubject == "" {
		return Account{}, fmt.Errorf("identity missing provider or subject")
	}
	email := id.Email
	if email == "" {
		email = fmt.Sprintf("%s@%s.invalid", id.ProviderSubject, id.Provider)
	}
	display := id.DisplayName
	if display == "" {
		display = email
	}

	const q = `
INSERT INTO accounts (id, provider, provider_subject, email, display_name, avatar_url)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(provider, provider_subject) DO UPDATE SET
  email = excluded.email,
  display_name = excluded.display_name,
  avatar_url = excluded.avatar_url
RETURNING id, provider, provider_subject, email, display_name, avatar_url, created_at
`
	newID := uuid.NewString()
	var a Account
	var created string
	err := db.QueryRowContext(ctx, q,
		newID, string(id.Provider), id.ProviderSubject, email, display, id.AvatarURL,
	).Scan(&a.ID, &a.Provider, &a.ProviderSubject, &a.Email, &a.DisplayName, &a.AvatarURL, &created)
	if err != nil {
		return Account{}, fmt.Errorf("upsert account: %w", err)
	}
	a.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	return a, nil
}

// GetAccount loads an account by id.
func GetAccount(ctx context.Context, db *sql.DB, id string) (Account, error) {
	const q = `
SELECT id, provider, provider_subject, email, display_name, avatar_url, created_at
FROM accounts WHERE id = ?
`
	var a Account
	var created string
	err := db.QueryRowContext(ctx, q, id).Scan(
		&a.ID, &a.Provider, &a.ProviderSubject, &a.Email, &a.DisplayName, &a.AvatarURL, &created,
	)
	if err != nil {
		return Account{}, err
	}
	a.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	return a, nil
}
