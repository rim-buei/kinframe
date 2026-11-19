package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rim-buei/kinframe/internal/storage"
)

// Kind is photo or video.
type Kind string

const (
	KindPhoto Kind = "photo"
	KindVideo Kind = "video"
)

// ArchiveState mirrors albums.archive_state (Glacier hooks later).
type ArchiveState string

const (
	ArchiveHot       ArchiveState = "hot"
	ArchiveArchiving ArchiveState = "archiving"
	ArchiveGlacier   ArchiveState = "glacier"
	ArchiveRestoring ArchiveState = "restoring"
)

// Common errors.
var (
	ErrNotFound      = errors.New("not found")
	ErrForbiddenEdit = errors.New("album not editable in current archive state")
	ErrTooLarge      = errors.New("file exceeds upload limit")
	ErrAlbumLimit    = errors.New("album capacity exceeded")
)

// Album is a media collection inside a space.
type Album struct {
	ID           string
	SpaceID      string
	Title        string
	Description  string
	ArchiveState ArchiveState
	ArchiveJobID string
	MaxBytes     *int64
	MaxItems     *int
	CreatedBy    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// MediaItem is an uploaded photo or video.
type MediaItem struct {
	ID          string
	AlbumID     string
	SpaceID     string
	Kind        Kind
	OriginalKey string
	ByteSize    int64
	ContentType string
	Width       *int
	Height      *int
	DurationMS  *int
	Checksum    string
	UploadedBy  string
	CreatedAt   time.Time
	DeletedAt   *time.Time
	ThumbKey    string
	PreviewKey  string
}

// Store persists albums/media and writes objects via ObjectStore.
type Store struct {
	db       *sql.DB
	objects  storage.ObjectStore
	maxBytes int64
}

// NewStore wires DB + object store. maxBytes is the instance default (100MiB).
func NewStore(db *sql.DB, objects storage.ObjectStore, maxBytes int64) *Store {
	if maxBytes <= 0 {
		maxBytes = 100 * 1024 * 1024
	}
	return &Store{db: db, objects: objects, maxBytes: maxBytes}
}

// CreateAlbum inserts an album in hot state.
func (s *Store) CreateAlbum(ctx context.Context, spaceID, title, description, createdBy string) (Album, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Album{}, fmt.Errorf("title required")
	}
	id := uuid.NewString()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO albums (id, space_id, title, description, created_by)
VALUES (?, ?, ?, ?, ?)
`, id, spaceID, title, strings.TrimSpace(description), createdBy)
	if err != nil {
		return Album{}, err
	}
	return s.GetAlbum(ctx, id)
}

// ListAlbums returns albums for a space.
func (s *Store) ListAlbums(ctx context.Context, spaceID string) ([]Album, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, space_id, title, description, archive_state, archive_job_id, max_bytes, max_items,
       created_by, created_at, updated_at
FROM albums WHERE space_id = ? ORDER BY created_at DESC
`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Album
	for rows.Next() {
		a, err := scanAlbum(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetAlbum loads by id.
func (s *Store) GetAlbum(ctx context.Context, albumID string) (Album, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, space_id, title, description, archive_state, archive_job_id, max_bytes, max_items,
       created_by, created_at, updated_at
FROM albums WHERE id = ?
`, albumID)
	a, err := scanAlbum(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Album{}, ErrNotFound
	}
	return a, err
}

// UpdateAlbum updates title/description.
func (s *Store) UpdateAlbum(ctx context.Context, albumID, title, description string) (Album, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Album{}, fmt.Errorf("title required")
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE albums SET title = ?, description = ?, updated_at = datetime('now') WHERE id = ?
`, title, strings.TrimSpace(description), albumID)
	if err != nil {
		return Album{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Album{}, ErrNotFound
	}
	return s.GetAlbum(ctx, albumID)
}

// DeleteAlbum removes album rows after soft-deleting media (objects left for later GC).
func (s *Store) DeleteAlbum(ctx context.Context, albumID string) error {
	a, err := s.GetAlbum(ctx, albumID)
	if err != nil {
		return err
	}
	if a.ArchiveState != ArchiveHot {
		return ErrForbiddenEdit
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `UPDATE media SET deleted_at = datetime('now') WHERE album_id = ? AND deleted_at IS NULL`, albumID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM albums WHERE id = ?`, albumID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// ListMedia returns non-deleted media for an album (newest first).
func (s *Store) ListMedia(ctx context.Context, albumID string) ([]MediaItem, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT m.id, m.album_id, m.space_id, m.kind, m.original_key, m.byte_size, m.content_type,
       m.width, m.height, m.duration_ms, m.checksum, m.uploaded_by, m.created_at, m.deleted_at,
       COALESCE(t.object_key, ''), COALESCE(p.object_key, '')
FROM media m
LEFT JOIN media_derivatives t ON t.media_id = m.id AND t.kind = 'thumb'
LEFT JOIN media_derivatives p ON p.media_id = m.id AND p.kind = 'preview'
WHERE m.album_id = ? AND m.deleted_at IS NULL
ORDER BY m.created_at DESC
`, albumID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MediaItem
	for rows.Next() {
		item, err := scanMedia(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// GetMedia loads a media row (including soft-deleted for auth path checks).
func (s *Store) GetMedia(ctx context.Context, mediaID string) (MediaItem, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT m.id, m.album_id, m.space_id, m.kind, m.original_key, m.byte_size, m.content_type,
       m.width, m.height, m.duration_ms, m.checksum, m.uploaded_by, m.created_at, m.deleted_at,
       COALESCE(t.object_key, ''), COALESCE(p.object_key, '')
FROM media m
LEFT JOIN media_derivatives t ON t.media_id = m.id AND t.kind = 'thumb'
LEFT JOIN media_derivatives p ON p.media_id = m.id AND p.kind = 'preview'
WHERE m.id = ?
`, mediaID)
	item, err := scanMedia(row)
	if errors.Is(err, sql.ErrNoRows) {
		return MediaItem{}, ErrNotFound
	}
	return item, err
}

// UploadOptions configures a single upload.
type UploadOptions struct {
	AlbumID     string
	UploadedBy  string
	Filename    string
	ContentType string
	Size        int64
	Body        io.Reader
}

// UploadMedia stores original + derivatives and inserts DB rows.
func (s *Store) UploadMedia(ctx context.Context, opt UploadOptions) (MediaItem, error) {
	album, err := s.GetAlbum(ctx, opt.AlbumID)
	if err != nil {
		return MediaItem{}, err
	}
	if album.ArchiveState != ArchiveHot {
		return MediaItem{}, ErrForbiddenEdit
	}
	limit := s.maxBytes
	if album.MaxBytes != nil && *album.MaxBytes > 0 && *album.MaxBytes < limit {
		limit = *album.MaxBytes
	}
	if opt.Size > 0 && opt.Size > limit {
		return MediaItem{}, ErrTooLarge
	}
	if album.MaxItems != nil {
		var n int
		_ = s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM media WHERE album_id = ? AND deleted_at IS NULL`, album.ID).Scan(&n)
		if n >= *album.MaxItems {
			return MediaItem{}, ErrAlbumLimit
		}
	}

	kind, err := classifyContentType(opt.ContentType, opt.Filename)
	if err != nil {
		return MediaItem{}, err
	}

	// Buffer original (needed for image decode + checksum + size check).
	raw, err := io.ReadAll(io.LimitReader(opt.Body, limit+1))
	if err != nil {
		return MediaItem{}, err
	}
	if int64(len(raw)) > limit {
		return MediaItem{}, ErrTooLarge
	}
	sum := sha256.Sum256(raw)
	checksum := hex.EncodeToString(sum[:])

	mediaID := uuid.NewString()
	origKey := storage.OriginalKey(album.SpaceID, album.ID, mediaID)
	thumbKey := storage.ThumbKey(album.SpaceID, album.ID, mediaID)
	previewKey := storage.PreviewKey(album.SpaceID, album.ID, mediaID)

	var width, height *int
	var thumb, preview Derivative

	switch kind {
	case KindPhoto:
		ow, oh, th, pr, err := BuildImageDerivatives(bytes.NewReader(raw))
		if err != nil {
			return MediaItem{}, err
		}
		width, height = &ow, &oh
		thumb, preview = th, pr
	case KindVideo:
		th, pr, err := BuildVideoStubDerivatives()
		if err != nil {
			return MediaItem{}, err
		}
		thumb, preview = th, pr
	}

	if err := s.objects.Put(ctx, origKey, bytes.NewReader(raw), opt.ContentType); err != nil {
		return MediaItem{}, fmt.Errorf("put original: %w", err)
	}
	if err := s.objects.Put(ctx, thumbKey, bytes.NewReader(thumb.JPEG), "image/jpeg"); err != nil {
		_ = s.objects.Delete(ctx, origKey)
		return MediaItem{}, fmt.Errorf("put thumb: %w", err)
	}
	if err := s.objects.Put(ctx, previewKey, bytes.NewReader(preview.JPEG), "image/jpeg"); err != nil {
		_ = s.objects.Delete(ctx, origKey)
		_ = s.objects.Delete(ctx, thumbKey)
		return MediaItem{}, fmt.Errorf("put preview: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MediaItem{}, err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
INSERT INTO media (id, album_id, space_id, kind, original_key, byte_size, content_type, width, height, checksum, uploaded_by)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, mediaID, album.ID, album.SpaceID, string(kind), origKey, len(raw), opt.ContentType, width, height, checksum, opt.UploadedBy)
	if err != nil {
		return MediaItem{}, err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO media_derivatives (id, media_id, kind, object_key, byte_size, width, height)
VALUES (?, ?, 'thumb', ?, ?, ?, ?)
`, uuid.NewString(), mediaID, thumbKey, len(thumb.JPEG), thumb.Width, thumb.Height)
	if err != nil {
		return MediaItem{}, err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO media_derivatives (id, media_id, kind, object_key, byte_size, width, height)
VALUES (?, ?, 'preview', ?, ?, ?, ?)
`, uuid.NewString(), mediaID, previewKey, len(preview.JPEG), preview.Width, preview.Height)
	if err != nil {
		return MediaItem{}, err
	}
	_, _ = tx.ExecContext(ctx, `UPDATE albums SET updated_at = datetime('now') WHERE id = ?`, album.ID)
	if err := tx.Commit(); err != nil {
		return MediaItem{}, err
	}
	return s.GetMedia(ctx, mediaID)
}

// SoftDeleteMedia marks media deleted. Objects retained for Glacier/GC later.
func (s *Store) SoftDeleteMedia(ctx context.Context, mediaID string) error {
	item, err := s.GetMedia(ctx, mediaID)
	if err != nil {
		return err
	}
	if item.DeletedAt != nil {
		return nil
	}
	album, err := s.GetAlbum(ctx, item.AlbumID)
	if err != nil {
		return err
	}
	if album.ArchiveState != ArchiveHot {
		return ErrForbiddenEdit
	}
	_, err = s.db.ExecContext(ctx, `
UPDATE media SET deleted_at = datetime('now') WHERE id = ?
`, mediaID)
	return err
}

// OpenObject returns a reader for original/thumb/preview.
func (s *Store) OpenObject(ctx context.Context, item MediaItem, kind string) (io.ReadCloser, string, error) {
	switch kind {
	case "original", "":
		return open(ctx, s.objects, item.OriginalKey, item.ContentType)
	case "thumb":
		if item.ThumbKey == "" {
			return nil, "", ErrNotFound
		}
		return open(ctx, s.objects, item.ThumbKey, "image/jpeg")
	case "preview":
		if item.PreviewKey == "" {
			return nil, "", ErrNotFound
		}
		return open(ctx, s.objects, item.PreviewKey, "image/jpeg")
	default:
		return nil, "", fmt.Errorf("unknown kind %q", kind)
	}
}

func open(ctx context.Context, store storage.ObjectStore, key, ct string) (io.ReadCloser, string, error) {
	rc, err := store.Get(ctx, key)
	return rc, ct, err
}

type scannable interface {
	Scan(dest ...any) error
}

func scanAlbum(row scannable) (Album, error) {
	var a Album
	var desc, job sql.NullString
	var state string
	var maxBytes sql.NullInt64
	var maxItems sql.NullInt64
	var created, updated string
	err := row.Scan(&a.ID, &a.SpaceID, &a.Title, &desc, &state, &job, &maxBytes, &maxItems, &a.CreatedBy, &created, &updated)
	if err != nil {
		return Album{}, err
	}
	a.Description = desc.String
	a.ArchiveState = ArchiveState(state)
	a.ArchiveJobID = job.String
	if maxBytes.Valid {
		v := maxBytes.Int64
		a.MaxBytes = &v
	}
	if maxItems.Valid {
		v := int(maxItems.Int64)
		a.MaxItems = &v
	}
	a.CreatedAt, _ = parseTime(created)
	a.UpdatedAt, _ = parseTime(updated)
	return a, nil
}

func scanMedia(row scannable) (MediaItem, error) {
	var m MediaItem
	var kind string
	var w, h, dur sql.NullInt64
	var checksum sql.NullString
	var created string
	var deleted sql.NullString
	err := row.Scan(
		&m.ID, &m.AlbumID, &m.SpaceID, &kind, &m.OriginalKey, &m.ByteSize, &m.ContentType,
		&w, &h, &dur, &checksum, &m.UploadedBy, &created, &deleted, &m.ThumbKey, &m.PreviewKey,
	)
	if err != nil {
		return MediaItem{}, err
	}
	m.Kind = Kind(kind)
	if w.Valid {
		v := int(w.Int64)
		m.Width = &v
	}
	if h.Valid {
		v := int(h.Int64)
		m.Height = &v
	}
	if dur.Valid {
		v := int(dur.Int64)
		m.DurationMS = &v
	}
	m.Checksum = checksum.String
	m.CreatedAt, _ = parseTime(created)
	if deleted.Valid && deleted.String != "" {
		t, _ := parseTime(deleted.String)
		m.DeletedAt = &t
	}
	return m, nil
}

func parseTime(s string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("bad time %q", s)
}
