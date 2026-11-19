package storage

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestFSArchiveSimulation(t *testing.T) {
	s, err := NewFSStore(filepath.Join(t.TempDir(), "o"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := "spaces/a/albums/b/originals/c"
	if err := s.Put(ctx, key, bytes.NewReader([]byte("orig")), "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	if err := s.TransitionStorageClass(ctx, key, ClassGlacier); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, key); !errors.Is(err, ErrObjectArchived) {
		t.Fatalf("got %v", err)
	}
	if err := s.RestoreObject(ctx, key, 7, "Standard"); err != nil {
		t.Fatal(err)
	}
	rc, err := s.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	_ = rc.Close()
}
