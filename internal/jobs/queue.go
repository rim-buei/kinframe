package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rim-buei/kinframe/internal/media"
)

// JobType is album_archive or album_restore.
type JobType string

const (
	TypeAlbumArchive JobType = "album_archive"
	TypeAlbumRestore JobType = "album_restore"
)

// Status values for jobs.status.
type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

// Job is a row from the jobs table.
type Job struct {
	ID          string
	Type        JobType
	AlbumID     string
	SpaceID     string
	Status      Status
	ProgressPct int
	ETASeconds  *int
	Error       string
	Attempts    int
	RunAfter    time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

var (
	ErrJobConflict = errors.New("active job already exists for album")
	ErrNoJob       = errors.New("no job available")
)

// Queue creates and leases SQLite-backed jobs.
type Queue struct {
	db *sql.DB
}

// NewQueue wraps the database.
func NewQueue(db *sql.DB) *Queue { return &Queue{db: db} }

// EnqueueArchive creates an archive job and sets album → archiving (caller enforces authz).
func (q *Queue) EnqueueArchive(ctx context.Context, album media.Album) (Job, error) {
	if album.ArchiveState != media.ArchiveHot {
		return Job{}, fmt.Errorf("album must be hot to archive (state=%s)", album.ArchiveState)
	}
	return q.enqueue(ctx, album, TypeAlbumArchive, media.ArchiveArchiving)
}

// EnqueueRestore creates a restore job and sets album → restoring.
func (q *Queue) EnqueueRestore(ctx context.Context, album media.Album) (Job, error) {
	if album.ArchiveState != media.ArchiveGlacier {
		return Job{}, fmt.Errorf("album must be glacier to restore (state=%s)", album.ArchiveState)
	}
	return q.enqueue(ctx, album, TypeAlbumRestore, media.ArchiveRestoring)
}

func (q *Queue) enqueue(ctx context.Context, album media.Album, typ JobType, next media.ArchiveState) (Job, error) {
	var active int
	err := q.db.QueryRowContext(ctx, `
SELECT COUNT(1) FROM jobs
WHERE album_id = ? AND status IN ('queued', 'running')
`, album.ID).Scan(&active)
	if err != nil {
		return Job{}, err
	}
	if active > 0 {
		return Job{}, ErrJobConflict
	}

	id := uuid.NewString()
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
INSERT INTO jobs (id, type, album_id, space_id, status)
VALUES (?, ?, ?, ?, 'queued')
`, id, string(typ), album.ID, album.SpaceID)
	if err != nil {
		return Job{}, err
	}
	_, err = tx.ExecContext(ctx, `
UPDATE albums SET archive_state = ?, archive_job_id = ?, updated_at = datetime('now')
WHERE id = ?
`, string(next), id, album.ID)
	if err != nil {
		return Job{}, err
	}
	if err := tx.Commit(); err != nil {
		return Job{}, err
	}
	return q.Get(ctx, id)
}

// Get loads a job by id.
func (q *Queue) Get(ctx context.Context, id string) (Job, error) {
	row := q.db.QueryRowContext(ctx, `
SELECT id, type, album_id, space_id, status, progress_pct, eta_seconds, error, attempts, run_after, created_at, updated_at
FROM jobs WHERE id = ?
`, id)
	return scanJob(row)
}

// LeaseOne claims the next queued job (single-writer friendly).
func (q *Queue) LeaseOne(ctx context.Context) (Job, error) {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var id string
	err = tx.QueryRowContext(ctx, `
SELECT id FROM jobs
WHERE status = 'queued' AND run_after <= datetime('now')
ORDER BY created_at ASC
LIMIT 1
`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNoJob
	}
	if err != nil {
		return Job{}, err
	}
	_, err = tx.ExecContext(ctx, `
UPDATE jobs SET status = 'running', attempts = attempts + 1, updated_at = datetime('now')
WHERE id = ? AND status = 'queued'
`, id)
	if err != nil {
		return Job{}, err
	}
	if err := tx.Commit(); err != nil {
		return Job{}, err
	}
	return q.Get(ctx, id)
}

// SetProgress updates progress_pct.
func (q *Queue) SetProgress(ctx context.Context, id string, pct int) error {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	_, err := q.db.ExecContext(ctx, `
UPDATE jobs SET progress_pct = ?, updated_at = datetime('now') WHERE id = ?
`, pct, id)
	return err
}

// Succeed marks the job done and sets the album to the terminal archive state.
func (q *Queue) Succeed(ctx context.Context, job Job, albumState media.ArchiveState) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
UPDATE jobs SET status = 'succeeded', progress_pct = 100, error = NULL, updated_at = datetime('now')
WHERE id = ?
`, job.ID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
UPDATE albums SET archive_state = ?, updated_at = datetime('now') WHERE id = ?
`, string(albumState), job.AlbumID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Fail marks the job failed and rolls the album back to a stable state.
func (q *Queue) Fail(ctx context.Context, job Job, albumRollback media.ArchiveState, msg string) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
UPDATE jobs SET status = 'failed', error = ?, updated_at = datetime('now') WHERE id = ?
`, msg, job.ID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
UPDATE albums SET archive_state = ?, updated_at = datetime('now') WHERE id = ?
`, string(albumRollback), job.AlbumID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanJob(row scannable) (Job, error) {
	var j Job
	var typ, status, runAfter, created, updated string
	var eta sql.NullInt64
	var errMsg sql.NullString
	err := row.Scan(&j.ID, &typ, &j.AlbumID, &j.SpaceID, &status, &j.ProgressPct, &eta, &errMsg, &j.Attempts, &runAfter, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNoJob
	}
	if err != nil {
		return Job{}, err
	}
	j.Type = JobType(typ)
	j.Status = Status(status)
	if eta.Valid {
		v := int(eta.Int64)
		j.ETASeconds = &v
	}
	j.Error = errMsg.String
	j.RunAfter, _ = parseTime(runAfter)
	j.CreatedAt, _ = parseTime(created)
	j.UpdatedAt, _ = parseTime(updated)
	return j, nil
}

func parseTime(s string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("bad time %q", s)
}
