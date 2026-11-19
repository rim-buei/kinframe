package storage

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"
)

func TestFSStoreRoundTrip(t *testing.T) {
	s, err := NewFSStore(filepath.Join(t.TempDir(), "obj"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := "spaces/a/albums/b/originals/c"
	if err := s.Put(ctx, key, bytes.NewReader([]byte("hello")), "text/plain"); err != nil {
		t.Fatal(err)
	}
	rc, err := s.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil || string(b) != "hello" {
		t.Fatalf("got %q %v", b, err)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
}

func TestFSStoreRejectsTraversal(t *testing.T) {
	s, err := NewFSStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(context.Background(), "../etc/passwd", bytes.NewReader([]byte("x")), ""); err == nil {
		t.Fatal("expected error")
	}
}
