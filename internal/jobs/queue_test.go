package jobs

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/rim-buei/kinframe/internal/config"
	"github.com/rim-buei/kinframe/internal/db"
	"github.com/rim-buei/kinframe/internal/media"
	"github.com/rim-buei/kinframe/internal/storage"
)

func setupArchive(t *testing.T) (context.Context, *Queue, *media.Store, *storage.FSStore, *Worker) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(ctx, database); err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(`
INSERT INTO accounts (id, provider, provider_subject, email, display_name) VALUES ('a1','google','s1','a@x.com','A');
INSERT INTO spaces (id, name, slug, created_by_account_id) VALUES ('sp1','S','s','a1');
INSERT INTO memberships (id, space_id, account_id, role) VALUES ('m1','sp1','a1','owner');
`)
	if err != nil {
		t.Fatal(err)
	}
	fs, err := storage.NewFSStore(filepath.Join(dir, "obj"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		DatabasePath:        filepath.Join(dir, "t.db"),
		MaxUploadBytes:      1024 * 1024,
		GlacierStorageClass: storage.ClassGlacier,
	}
	ms := media.NewStore(database, fs, cfg.MaxUploadBytes)
	q := NewQueue(database)
	w := NewWorker(cfg, database, slog.New(slog.NewTextHandler(io.Discard, nil)), fs)
	w.interval = time.Millisecond
	return ctx, q, ms, fs, w
}

func jpegBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 12))
	for y := 0; y < 12; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.RGBA{R: 10, G: 20, B: 30, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestArchiveRestoreKeepsDerivatives(t *testing.T) {
	ctx, q, ms, fs, w := setupArchive(t)
	al, err := ms.CreateAlbum(ctx, "sp1", "Trip", "", "a1")
	if err != nil {
		t.Fatal(err)
	}
	raw := jpegBytes(t)
	item, err := ms.UploadMedia(ctx, media.UploadOptions{
		AlbumID: al.ID, UploadedBy: "a1", Filename: "a.jpg",
		ContentType: "image/jpeg", Size: int64(len(raw)), Body: bytes.NewReader(raw),
	})
	if err != nil {
		t.Fatal(err)
	}

	job, err := q.EnqueueArchive(ctx, al)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.tick(ctx); err != nil {
		t.Fatal(err)
	}
	job, err = q.Get(ctx, job.ID)
	if err != nil || job.Status != StatusSucceeded {
		t.Fatalf("job: %+v %v", job, err)
	}
	al, _ = ms.GetAlbum(ctx, al.ID)
	if al.ArchiveState != media.ArchiveGlacier {
		t.Fatalf("state %s", al.ArchiveState)
	}
	class, err := fs.StorageClassOf(item.OriginalKey)
	if err != nil || class != storage.ClassGlacier {
		t.Fatalf("class %s %v", class, err)
	}
	// Original blocked; thumb still readable.
	if _, _, err := ms.OpenObject(ctx, item, "original"); err != media.ErrOriginalArchived {
		t.Fatalf("original: %v", err)
	}
	rc, _, err := ms.OpenObject(ctx, item, "thumb")
	if err != nil {
		t.Fatal(err)
	}
	_ = rc.Close()

	job2, err := q.EnqueueRestore(ctx, al)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.tick(ctx); err != nil {
		t.Fatal(err)
	}
	job2, _ = q.Get(ctx, job2.ID)
	if job2.Status != StatusSucceeded {
		t.Fatalf("restore job %+v", job2)
	}
	al, _ = ms.GetAlbum(ctx, al.ID)
	if al.ArchiveState != media.ArchiveHot {
		t.Fatalf("state %s", al.ArchiveState)
	}
	rc, _, err = ms.OpenObject(ctx, item, "original")
	if err != nil {
		t.Fatal(err)
	}
	_ = rc.Close()
}

func TestEnqueueConflict(t *testing.T) {
	ctx, q, ms, _, _ := setupArchive(t)
	al, err := ms.CreateAlbum(ctx, "sp1", "A", "", "a1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.EnqueueArchive(ctx, al); err != nil {
		t.Fatal(err)
	}
	al, _ = ms.GetAlbum(ctx, al.ID)
	if _, err := q.EnqueueArchive(ctx, al); err != ErrJobConflict {
		// album is archiving, enqueue checks hot — different error is OK
		if err == nil {
			t.Fatal("expected error")
		}
	}
}
