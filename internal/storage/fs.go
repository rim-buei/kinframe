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
// Glacier is simulated with sidecar marker files so originals become unreadable
// while thumbs/previews stay available.
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

func (s *FSStore) classPath(full string) string { return full + ".storage_class" }

func (s *FSStore) readClass(full string) string {
	b, err := os.ReadFile(s.classPath(full))
	if err != nil {
		return ClassStandard
	}
	return strings.TrimSpace(string(b))
}

func (s *FSStore) writeClass(full, class string) error {
	if class == "" || class == ClassStandard {
		_ = os.Remove(s.classPath(full))
		return nil
	}
	return os.WriteFile(s.classPath(full), []byte(class+"\n"), 0o644)
}

// Put writes an object to disk (and clears any archive marker).
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
	if err := os.Rename(tmp, full); err != nil {
		return err
	}
	_ = s.writeClass(full, ClassStandard)
	return nil
}

// Get opens an object for reading. Archived originals return ErrObjectArchived.
func (s *FSStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	full, err := s.resolve(key)
	if err != nil {
		return nil, err
	}
	if s.readClass(full) == ClassGlacier {
		return nil, ErrObjectArchived
	}
	return os.Open(full)
}

// Delete removes an object if present.
func (s *FSStore) Delete(_ context.Context, key string) error {
	full, err := s.resolve(key)
	if err != nil {
		return err
	}
	_ = os.Remove(s.classPath(full))
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

// TransitionStorageClass simulates Glacier by marking the object archived (or warm).
func (s *FSStore) TransitionStorageClass(_ context.Context, key string, storageClass string) error {
	full, err := s.resolve(key)
	if err != nil {
		return err
	}
	if _, err := os.Stat(full); err != nil {
		return err
	}
	switch strings.ToUpper(storageClass) {
	case ClassGlacier, "GLACIER_IR", "DEEP_ARCHIVE":
		return s.writeClass(full, ClassGlacier)
	case ClassStandard, "STANDARD_IA", "":
		return s.writeClass(full, ClassStandard)
	default:
		return s.writeClass(full, storageClass)
	}
}

// RestoreObject simulates an immediate Glacier restore (dev/smoke).
func (s *FSStore) RestoreObject(_ context.Context, key string, _ int, _ string) error {
	full, err := s.resolve(key)
	if err != nil {
		return err
	}
	if _, err := os.Stat(full); err != nil {
		return err
	}
	// Instant restore for local smoke: clear archive marker.
	return s.writeClass(full, ClassStandard)
}

// Root returns the filesystem root (tests / diagnostics).
func (s *FSStore) Root() string { return s.root }

// StorageClassOf returns the simulated class for a key (tests).
func (s *FSStore) StorageClassOf(key string) (string, error) {
	full, err := s.resolve(key)
	if err != nil {
		return "", err
	}
	return s.readClass(full), nil
}
