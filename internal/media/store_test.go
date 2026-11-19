package media

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"path/filepath"
	"testing"

	"github.com/rim-buei/kinframe/internal/db"
	"github.com/rim-buei/kinframe/internal/storage"
)

func setupMedia(t *testing.T) (*Store, string, string) {
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
INSERT INTO accounts (id, provider, provider_subject, email, display_name)
VALUES ('a1', 'google', 's1', 'a@example.com', 'A');
INSERT INTO spaces (id, name, slug, created_by_account_id) VALUES ('sp1', 'S', 's', 'a1');
INSERT INTO memberships (id, space_id, account_id, role) VALUES ('m1', 'sp1', 'a1', 'owner');
`)
	if err != nil {
		t.Fatal(err)
	}
	fs, err := storage.NewFSStore(filepath.Join(dir, "obj"))
	if err != nil {
		t.Fatal(err)
	}
	return NewStore(database, fs, 1024*1024), "sp1", "a1"
}

func tinyJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 3), G: uint8(y * 4), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAlbumUploadDelete(t *testing.T) {
	ctx := context.Background()
	st, spaceID, accountID := setupMedia(t)

	al, err := st.CreateAlbum(ctx, spaceID, "Trip", "desc", accountID)
	if err != nil {
		t.Fatal(err)
	}
	raw := tinyJPEG(t)
	item, err := st.UploadMedia(ctx, UploadOptions{
		AlbumID:     al.ID,
		UploadedBy:  accountID,
		Filename:    "shot.jpg",
		ContentType: "image/jpeg",
		Size:        int64(len(raw)),
		Body:        bytes.NewReader(raw),
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Kind != KindPhoto || item.ThumbKey == "" || item.PreviewKey == "" {
		t.Fatalf("item: %+v", item)
	}
	if item.Width == nil || *item.Width != 64 {
		t.Fatalf("width: %v", item.Width)
	}

	list, err := st.ListMedia(ctx, al.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %d", err, len(list))
	}

	rc, ct, err := st.OpenObject(ctx, item, "thumb")
	if err != nil {
		t.Fatal(err)
	}
	_ = rc.Close()
	if ct != "image/jpeg" {
		t.Fatalf("ct %s", ct)
	}

	if err := st.SoftDeleteMedia(ctx, item.ID); err != nil {
		t.Fatal(err)
	}
	list, err = st.ListMedia(ctx, al.ID)
	if err != nil || len(list) != 0 {
		t.Fatalf("after delete: %v %d", err, len(list))
	}
}

func TestUploadRejectsOversize(t *testing.T) {
	ctx := context.Background()
	st, spaceID, accountID := setupMedia(t)
	st.maxBytes = 100
	al, err := st.CreateAlbum(ctx, spaceID, "Tiny", "", accountID)
	if err != nil {
		t.Fatal(err)
	}
	raw := tinyJPEG(t)
	_, err = st.UploadMedia(ctx, UploadOptions{
		AlbumID: al.ID, UploadedBy: accountID, Filename: "x.jpg",
		ContentType: "image/jpeg", Size: int64(len(raw)), Body: bytes.NewReader(raw),
	})
	if err != ErrTooLarge {
		t.Fatalf("want ErrTooLarge, got %v", err)
	}
}

func TestBuildImageDerivatives(t *testing.T) {
	raw := tinyJPEG(t)
	w, h, th, pr, err := BuildImageDerivatives(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if w != 64 || h != 48 || len(th.JPEG) == 0 || len(pr.JPEG) == 0 {
		t.Fatalf("%d %d thumb=%d preview=%d", w, h, len(th.JPEG), len(pr.JPEG))
	}
}
