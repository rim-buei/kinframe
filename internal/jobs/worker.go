// Package jobs sketches the SQLite-backed album archive/restore worker.
package jobs

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/rim-buei/kinframe/internal/config"
	"github.com/rim-buei/kinframe/internal/storage"
)

// Worker polls the jobs table and will eventually run Glacier transitions.
type Worker struct {
	cfg    config.Config
	db     *sql.DB
	logger *slog.Logger
	store  storage.ObjectStore
}

// NewWorker constructs a worker with a stub object store.
func NewWorker(cfg config.Config, db *sql.DB, logger *slog.Logger) *Worker {
	return &Worker{
		cfg:    cfg,
		db:     db,
		logger: logger,
		store:  storage.StubStore{},
	}
}

// Run loops until ctx is cancelled. Archive/restore execution is TODO.
func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	_ = w.writeHeartbeat()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("worker stopping")
			return nil
		case <-ticker.C:
			_ = w.writeHeartbeat()
			if err := w.tick(ctx); err != nil {
				w.logger.Warn("worker tick", "err", err)
			}
		}
	}
}

func (w *Worker) tick(ctx context.Context) error {
	// TODO: lease queued jobs (album_archive / album_restore) with
	// UPDATE ... WHERE status='queued' AND run_after<=now, then call
	// storage.TransitionStorageClass / RestoreObject and update albums.archive_state.
	_ = ctx
	_ = w.store
	var n int
	err := w.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM jobs WHERE status = 'queued' AND run_after <= datetime('now')`,
	).Scan(&n)
	if err != nil {
		return err
	}
	if n > 0 {
		w.logger.Info("queued jobs waiting (execution not implemented)", "count", n)
	}
	return nil
}

func (w *Worker) writeHeartbeat() error {
	dir := filepath.Dir(w.cfg.DatabasePath)
	path := filepath.Join(dir, "worker.heartbeat")
	return os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}
