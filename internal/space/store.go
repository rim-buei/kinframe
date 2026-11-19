package space

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Common store errors.
var (
	ErrNotFound      = errors.New("not found")
	ErrForbidden     = errors.New("forbidden")
	ErrInvalidInvite = errors.New("invalid invite")
	ErrInviteGone    = errors.New("invite unavailable")
)

// Space is a tenant container for albums.
type Space struct {
	ID                 string
	Name               string
	Slug               string
	CreatedByAccountID string
	CreatedAt          time.Time
	DeletedAt          *time.Time
}

// Membership links an account to a space with a role.
type Membership struct {
	ID        string
	SpaceID   string
	AccountID string
	Role      Role
	CreatedAt time.Time
}

// InviteLink is a hashed invite token for joining a space.
type InviteLink struct {
	ID          string
	SpaceID     string
	DefaultRole Role
	CreatedBy   string
	ExpiresAt   *time.Time
	RevokedAt   *time.Time
	MaxUses     *int
	UseCount    int
}

// CreatedInvite is returned once with the plaintext token.
type CreatedInvite struct {
	Invite InviteLink
	Token  string // plaintext; never stored
}

// SpaceWithRole is a space plus the caller's membership role.
type SpaceWithRole struct {
	Space Space
	Role  Role
}

// Store persists spaces, memberships, and invites.
type Store struct {
	db *sql.DB
}

// NewStore wraps a SQLite database.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// CreateSpace inserts a space, owner membership, and default space_settings.
func (s *Store) CreateSpace(ctx context.Context, name, createdBy string) (SpaceWithRole, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return SpaceWithRole{}, fmt.Errorf("name required")
	}
	slug, err := UniqueSlug(name, func(candidate string) (bool, error) {
		var n int
		err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM spaces WHERE slug = ?`, candidate).Scan(&n)
		return n == 0, err
	})
	if err != nil {
		return SpaceWithRole{}, err
	}

	spaceID := uuid.NewString()
	memberID := uuid.NewString()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SpaceWithRole{}, err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
INSERT INTO spaces (id, name, slug, created_by_account_id) VALUES (?, ?, ?, ?)
`, spaceID, name, slug, createdBy)
	if err != nil {
		return SpaceWithRole{}, fmt.Errorf("insert space: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO memberships (id, space_id, account_id, role) VALUES (?, ?, ?, ?)
`, memberID, spaceID, createdBy, string(RoleOwner))
	if err != nil {
		return SpaceWithRole{}, fmt.Errorf("insert owner membership: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO space_settings (space_id, theme_id) VALUES (?, 'default')
`, spaceID)
	if err != nil {
		return SpaceWithRole{}, fmt.Errorf("insert space_settings: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return SpaceWithRole{}, err
	}
	sp, err := s.GetSpace(ctx, spaceID)
	if err != nil {
		return SpaceWithRole{}, err
	}
	return SpaceWithRole{Space: sp, Role: RoleOwner}, nil
}

// ListSpacesForAccount returns non-deleted spaces the account belongs to.
func (s *Store) ListSpacesForAccount(ctx context.Context, accountID string) ([]SpaceWithRole, error) {
	const q = `
SELECT sp.id, sp.name, sp.slug, sp.created_by_account_id, sp.created_at, sp.deleted_at, m.role
FROM spaces sp
JOIN memberships m ON m.space_id = sp.id
WHERE m.account_id = ? AND sp.deleted_at IS NULL
ORDER BY sp.created_at DESC
`
	rows, err := s.db.QueryContext(ctx, q, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SpaceWithRole
	for rows.Next() {
		var sw SpaceWithRole
		var created string
		var deleted sql.NullString
		var role string
		if err := rows.Scan(&sw.Space.ID, &sw.Space.Name, &sw.Space.Slug, &sw.Space.CreatedByAccountID, &created, &deleted, &role); err != nil {
			return nil, err
		}
		sw.Space.CreatedAt, _ = parseTime(created)
		sw.Space.DeletedAt = nullTime(deleted)
		sw.Role = Role(role)
		out = append(out, sw)
	}
	return out, rows.Err()
}

// GetSpace loads a space by id or slug (includes soft-deleted for auth checks).
func (s *Store) GetSpace(ctx context.Context, idOrSlug string) (Space, error) {
	const q = `
SELECT id, name, slug, created_by_account_id, created_at, deleted_at
FROM spaces WHERE id = ? OR slug = ?
LIMIT 1
`
	var sp Space
	var created string
	var deleted sql.NullString
	err := s.db.QueryRowContext(ctx, q, idOrSlug, idOrSlug).Scan(
		&sp.ID, &sp.Name, &sp.Slug, &sp.CreatedByAccountID, &created, &deleted,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Space{}, ErrNotFound
	}
	if err != nil {
		return Space{}, err
	}
	sp.CreatedAt, _ = parseTime(created)
	sp.DeletedAt = nullTime(deleted)
	return sp, nil
}

// UpdateSpaceName renames a space (caller enforces authz).
func (s *Store) UpdateSpaceName(ctx context.Context, spaceID, name string) (Space, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Space{}, fmt.Errorf("name required")
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE spaces SET name = ? WHERE id = ? AND deleted_at IS NULL
`, name, spaceID)
	if err != nil {
		return Space{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Space{}, ErrNotFound
	}
	return s.GetSpace(ctx, spaceID)
}

// SoftDeleteSpace sets deleted_at (owner-only at handler layer).
func (s *Store) SoftDeleteSpace(ctx context.Context, spaceID string) error {
	res, err := s.db.ExecContext(ctx, `
UPDATE spaces SET deleted_at = datetime('now') WHERE id = ? AND deleted_at IS NULL
`, spaceID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetMembership returns membership for account in space.
func (s *Store) GetMembership(ctx context.Context, spaceID, accountID string) (Membership, error) {
	const q = `
SELECT id, space_id, account_id, role, created_at FROM memberships
WHERE space_id = ? AND account_id = ?
`
	var m Membership
	var role, created string
	err := s.db.QueryRowContext(ctx, q, spaceID, accountID).Scan(
		&m.ID, &m.SpaceID, &m.AccountID, &role, &created,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Membership{}, ErrNotFound
	}
	if err != nil {
		return Membership{}, err
	}
	m.Role = Role(role)
	m.CreatedAt, _ = parseTime(created)
	return m, nil
}

// RequireMembership loads space + membership and checks min role. Rejects deleted spaces.
func (s *Store) RequireMembership(ctx context.Context, idOrSlug, accountID string, min Role) (Space, Membership, error) {
	sp, err := s.GetSpace(ctx, idOrSlug)
	if err != nil {
		return Space{}, Membership{}, err
	}
	if sp.DeletedAt != nil {
		return Space{}, Membership{}, ErrNotFound
	}
	m, err := s.GetMembership(ctx, sp.ID, accountID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Space{}, Membership{}, ErrForbidden
		}
		return Space{}, Membership{}, err
	}
	if !m.Role.AtLeast(min) {
		return Space{}, Membership{}, ErrForbidden
	}
	return sp, m, nil
}

// MemberView is a membership with account display fields.
type MemberView struct {
	Membership
	Email       string
	DisplayName string
}

// ListMembers returns memberships for a space with account emails/names.
func (s *Store) ListMembers(ctx context.Context, spaceID string) ([]MemberView, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT m.id, m.space_id, m.account_id, m.role, m.created_at, a.email, a.display_name
FROM memberships m
JOIN accounts a ON a.id = m.account_id
WHERE m.space_id = ?
ORDER BY m.created_at ASC
`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MemberView
	for rows.Next() {
		var mv MemberView
		var role, created string
		if err := rows.Scan(&mv.ID, &mv.SpaceID, &mv.AccountID, &role, &created, &mv.Email, &mv.DisplayName); err != nil {
			return nil, err
		}
		mv.Role = Role(role)
		mv.CreatedAt, _ = parseTime(created)
		out = append(out, mv)
	}
	return out, rows.Err()
}

// UpdateMemberRole changes a member's role. Cannot change the sole owner via this path.
func (s *Store) UpdateMemberRole(ctx context.Context, spaceID, targetAccountID string, newRole Role, actor Role) error {
	if newRole == RoleOwner {
		return fmt.Errorf("use ownership transfer to assign owner")
	}
	target, err := s.GetMembership(ctx, spaceID, targetAccountID)
	if err != nil {
		return err
	}
	if target.Role == RoleOwner {
		return ErrForbidden
	}
	// Admins cannot modify other admins (only owner can).
	if actor == RoleAdmin && target.Role.AtLeast(RoleAdmin) {
		return ErrForbidden
	}
	if actor == RoleAdmin && newRole.AtLeast(RoleAdmin) {
		return ErrForbidden
	}
	_, err = s.db.ExecContext(ctx, `
UPDATE memberships SET role = ? WHERE space_id = ? AND account_id = ?
`, string(newRole), spaceID, targetAccountID)
	return err
}

// RemoveMember deletes a membership. Cannot remove the owner.
func (s *Store) RemoveMember(ctx context.Context, spaceID, targetAccountID string, actor Role) error {
	target, err := s.GetMembership(ctx, spaceID, targetAccountID)
	if err != nil {
		return err
	}
	if target.Role == RoleOwner {
		return ErrForbidden
	}
	if actor == RoleAdmin && target.Role.AtLeast(RoleAdmin) {
		return ErrForbidden
	}
	_, err = s.db.ExecContext(ctx, `
DELETE FROM memberships WHERE space_id = ? AND account_id = ?
`, spaceID, targetAccountID)
	return err
}

// CreateInvite issues a new invite link; returns plaintext token once.
func (s *Store) CreateInvite(ctx context.Context, spaceID, createdBy string, role Role, expiresAt *time.Time, maxUses *int) (CreatedInvite, error) {
	role, err := ParseInviteRole(string(role))
	if err != nil {
		return CreatedInvite{}, err
	}
	raw, err := randomToken(32)
	if err != nil {
		return CreatedInvite{}, err
	}
	id := uuid.NewString()
	var exp any
	if expiresAt != nil {
		exp = expiresAt.UTC().Format("2006-01-02 15:04:05")
	}
	var max any
	if maxUses != nil {
		max = *maxUses
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO invite_links (id, space_id, token_hash, default_role, created_by, expires_at, max_uses)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, id, spaceID, hashToken(raw), string(role), createdBy, exp, max)
	if err != nil {
		return CreatedInvite{}, err
	}
	inv, err := s.getInviteByID(ctx, id)
	if err != nil {
		return CreatedInvite{}, err
	}
	return CreatedInvite{Invite: inv, Token: raw}, nil
}

// ListInvites returns invites for a space (including revoked).
func (s *Store) ListInvites(ctx context.Context, spaceID string) ([]InviteLink, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, space_id, default_role, created_by, expires_at, revoked_at, max_uses, use_count
FROM invite_links WHERE space_id = ? ORDER BY id DESC
`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInvites(rows)
}

// RevokeInvite sets revoked_at.
func (s *Store) RevokeInvite(ctx context.Context, spaceID, inviteID string) error {
	res, err := s.db.ExecContext(ctx, `
UPDATE invite_links SET revoked_at = datetime('now')
WHERE id = ? AND space_id = ? AND revoked_at IS NULL
`, inviteID, spaceID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// AcceptInvite joins or upgrades membership using a plaintext token.
func (s *Store) AcceptInvite(ctx context.Context, token, accountID string) (Space, Membership, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Space{}, Membership{}, ErrInvalidInvite
	}
	inv, err := s.getInviteByHash(ctx, hashToken(token))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Space{}, Membership{}, ErrInvalidInvite
		}
		return Space{}, Membership{}, err
	}
	if inv.RevokedAt != nil {
		return Space{}, Membership{}, ErrInviteGone
	}
	if inv.ExpiresAt != nil && time.Now().UTC().After(*inv.ExpiresAt) {
		return Space{}, Membership{}, ErrInviteGone
	}
	if inv.MaxUses != nil && inv.UseCount >= *inv.MaxUses {
		return Space{}, Membership{}, ErrInviteGone
	}

	sp, err := s.GetSpace(ctx, inv.SpaceID)
	if err != nil {
		return Space{}, Membership{}, err
	}
	if sp.DeletedAt != nil {
		return Space{}, Membership{}, ErrInviteGone
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Space{}, Membership{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var existingRole sql.NullString
	var existingID sql.NullString
	err = tx.QueryRowContext(ctx, `
SELECT id, role FROM memberships WHERE space_id = ? AND account_id = ?
`, inv.SpaceID, accountID).Scan(&existingID, &existingRole)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Space{}, Membership{}, err
	}

	var mem Membership
	if existingID.Valid {
		cur := Role(existingRole.String)
		mem.ID = existingID.String
		mem.SpaceID = inv.SpaceID
		mem.AccountID = accountID
		mem.Role = cur
		if inv.DefaultRole.HigherThan(cur) {
			_, err = tx.ExecContext(ctx, `
UPDATE memberships SET role = ? WHERE id = ?
`, string(inv.DefaultRole), mem.ID)
			if err != nil {
				return Space{}, Membership{}, err
			}
			mem.Role = inv.DefaultRole
		}
	} else {
		mem = Membership{
			ID:        uuid.NewString(),
			SpaceID:   inv.SpaceID,
			AccountID: accountID,
			Role:      inv.DefaultRole,
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO memberships (id, space_id, account_id, role) VALUES (?, ?, ?, ?)
`, mem.ID, mem.SpaceID, mem.AccountID, string(mem.Role))
		if err != nil {
			return Space{}, Membership{}, err
		}
	}

	_, err = tx.ExecContext(ctx, `
UPDATE invite_links SET use_count = use_count + 1 WHERE id = ?
`, inv.ID)
	if err != nil {
		return Space{}, Membership{}, err
	}
	if err := tx.Commit(); err != nil {
		return Space{}, Membership{}, err
	}
	return sp, mem, nil
}

// LookupInviteByToken returns invite + space for UI (no consume).
func (s *Store) LookupInviteByToken(ctx context.Context, token string) (InviteLink, Space, error) {
	inv, err := s.getInviteByHash(ctx, hashToken(strings.TrimSpace(token)))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return InviteLink{}, Space{}, ErrInvalidInvite
		}
		return InviteLink{}, Space{}, err
	}
	sp, err := s.GetSpace(ctx, inv.SpaceID)
	if err != nil {
		return InviteLink{}, Space{}, err
	}
	return inv, sp, nil
}

func (s *Store) getInviteByID(ctx context.Context, id string) (InviteLink, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, space_id, default_role, created_by, expires_at, revoked_at, max_uses, use_count
FROM invite_links WHERE id = ?
`, id)
	return scanInvite(row)
}

func (s *Store) getInviteByHash(ctx context.Context, hash string) (InviteLink, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, space_id, default_role, created_by, expires_at, revoked_at, max_uses, use_count
FROM invite_links WHERE token_hash = ?
`, hash)
	return scanInvite(row)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanInvite(row scanner) (InviteLink, error) {
	var inv InviteLink
	var role string
	var exp, rev sql.NullString
	var max sql.NullInt64
	err := row.Scan(&inv.ID, &inv.SpaceID, &role, &inv.CreatedBy, &exp, &rev, &max, &inv.UseCount)
	if errors.Is(err, sql.ErrNoRows) {
		return InviteLink{}, ErrNotFound
	}
	if err != nil {
		return InviteLink{}, err
	}
	inv.DefaultRole = Role(role)
	inv.ExpiresAt = nullTime(exp)
	inv.RevokedAt = nullTime(rev)
	if max.Valid {
		v := int(max.Int64)
		inv.MaxUses = &v
	}
	return inv, nil
}

func scanInvites(rows *sql.Rows) ([]InviteLink, error) {
	var out []InviteLink
	for rows.Next() {
		inv, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func parseTime(s string) (time.Time, error) {
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
	} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("parse time %q", s)
}

func nullTime(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t, err := parseTime(ns.String)
	if err != nil {
		return nil
	}
	return &t
}
