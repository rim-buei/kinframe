package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FSStore stores objects on the local filesystem (local/dev smoke without AWS).
type FSStore struct {
	root string
}

// NewFSStore creates a filesystem-backed ObjectStore under root.
func NewFSStore(root string) (*FSStore, error) {
	root = filepath.Clean(root)
	if root == "" || root == "." {
		return nil, fmt.Errorf("storage path required")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &FSStore{root: root}, nil
}

func (s *FSStore) resolve(key string) (string, error) {
	key = strings.TrimPrefix(key, "/")
	if key == "" || strings.Contains(key, "..") {
		return "", fmt.Errorf("invalid object key")
	}
	full := filepath.Join(s.root, filepath.FromSlash(key))
	rel, err := filepath.Rel(s.root, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("invalid object key")
	}
	return full, nil
}

// Put writes an object to disk.
func (s *FSStore) Put(_ context.Context, key string, body io.Reader, _ string) error {
	full, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	tmp := full + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, body)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	return os.Rename(tmp, full)
}

// Get opens an object for reading.
func (s *FSStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	full, err := s.resolve(key)
	if err != nil {
		return nil, err
	}
	return os.Open(full)
}

// Delete removes an object if present.
func (s *FSStore) Delete(_ context.Context, key string) error {
	full, err := s.resolve(key)
	if err != nil {
		return err
	}
	err = os.Remove(full)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// PresignPut is unsupported for the filesystem backend (upload via the app).
func (s *FSStore) PresignPut(context.Context, string, string, time.Duration) (string, error) {
	return "", fmt.Errorf("filesystem store does not support presigned PUT; upload via the app API")
}

// PresignGet is unsupported for the filesystem backend (stream via the app).
func (s *FSStore) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "", fmt.Errorf("filesystem store does not support presigned GET; fetch via the app API")
}

// TransitionStorageClass is a Glacier hook; no-op on filesystem.
func (s *FSStore) TransitionStorageClass(context.Context, string, string) error {
	return nil
}

// RestoreObject is a Glacier hook; no-op on filesystem.
func (s *FSStore) RestoreObject(context.Context, string, int, string) error {
	return nil
}

// Root returns the filesystem root (tests / diagnostics).
func (s *FSStore) Root() string { return s.root }
