// Package jobs implements the SQLite-backed album archive/restore worker.
package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/rim-buei/kinframe/internal/config"
	"github.com/rim-buei/kinframe/internal/media"
	"github.com/rim-buei/kinframe/internal/storage"
)

// Worker leases jobs and transitions originals between hot and Glacier.
type Worker struct {
	cfg      config.Config
	db       *sql.DB
	logger   *slog.Logger
	store    storage.ObjectStore
	queue    *Queue
	media    *media.Store
	interval time.Duration
}

// NewWorker constructs a worker with the configured object store.
func NewWorker(cfg config.Config, db *sql.DB, logger *slog.Logger, store storage.ObjectStore) *Worker {
	return &Worker{
		cfg:      cfg,
		db:       db,
		logger:   logger,
		store:    store,
		queue:    NewQueue(db),
		media:    media.NewStore(db, store, cfg.MaxUploadBytes),
		interval: 2 * time.Second,
	}
}

// Run loops until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	_ = w.writeHeartbeat()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("worker stopping")
			return nil
		case <-ticker.C:
			_ = w.writeHeartbeat()
			if err := w.tick(ctx); err != nil && !errors.Is(err, ErrNoJob) {
				w.logger.Warn("worker tick", "err", err)
			}
		}
	}
}

func (w *Worker) tick(ctx context.Context) error {
	job, err := w.queue.LeaseOne(ctx)
	if err != nil {
		return err
	}
	w.logger.Info("leased job", "id", job.ID, "type", job.Type, "album_id", job.AlbumID)
	if err := w.runJob(ctx, job); err != nil {
		w.logger.Error("job failed", "id", job.ID, "err", err)
		rollback := media.ArchiveHot
		if job.Type == TypeAlbumRestore {
			rollback = media.ArchiveGlacier
		}
		_ = w.queue.Fail(ctx, job, rollback, err.Error())
		return err
	}
	return nil
}

func (w *Worker) runJob(ctx context.Context, job Job) error {
	keys, err := w.media.ListOriginalKeys(ctx, job.AlbumID)
	if err != nil {
		return err
	}
	total := len(keys)
	if total == 0 {
		// Empty album still transitions.
		return w.finish(ctx, job)
	}

	glacierClass := w.cfg.GlacierStorageClass
	if glacierClass == "" {
		glacierClass = storage.ClassGlacier
	}

	for i, key := range keys {
		switch job.Type {
		case TypeAlbumArchive:
			if err := w.store.TransitionStorageClass(ctx, key, glacierClass); err != nil {
				return fmt.Errorf("archive %s: %w", key, err)
			}
		case TypeAlbumRestore:
			// Start restore (FS completes immediately; S3 may be async — then copy to STANDARD).
			if err := w.store.RestoreObject(ctx, key, 7, "Standard"); err != nil {
				// Some S3 responses are OK when already restored / in progress.
				w.logger.Warn("restore object", "key", key, "err", err)
			}
			// Ensure hot class after restore request (FS sim; S3 CopyObject to STANDARD when available).
			if err := w.store.TransitionStorageClass(ctx, key, storage.ClassStandard); err != nil {
				return fmt.Errorf("reheat %s: %w", key, err)
			}
		default:
			return fmt.Errorf("unknown job type %s", job.Type)
		}
		pct := ((i + 1) * 100) / total
		_ = w.queue.SetProgress(ctx, job.ID, pct)
	}
	return w.finish(ctx, job)
}

func (w *Worker) finish(ctx context.Context, job Job) error {
	switch job.Type {
	case TypeAlbumArchive:
		return w.queue.Succeed(ctx, job, media.ArchiveGlacier)
	case TypeAlbumRestore:
		return w.queue.Succeed(ctx, job, media.ArchiveHot)
	default:
		return fmt.Errorf("unknown job type %s", job.Type)
	}
}

func (w *Worker) writeHeartbeat() error {
	dir := filepath.Dir(w.cfg.DatabasePath)
	path := filepath.Join(dir, "worker.heartbeat")
	return os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}
