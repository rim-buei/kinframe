// Package storage defines object-store interfaces. S3 is the v1 backend; GCS is not implemented.
package storage

import (
	"context"
	"io"
	"time"
)

// ObjectStore is the S3-facing interface used by app and worker.
// TODO: implement with AWS SDK v2 (presigned PUT, GetObject, CopyObject, DeleteObject).
type ObjectStore interface {
	PresignPut(ctx context.Context, key string, contentType string, expiry time.Duration) (url string, err error)
	PresignGet(ctx context.Context, key string, expiry time.Duration) (url string, err error)
	Put(ctx context.Context, key string, body io.Reader, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	// TransitionStorageClass moves an object between hot and archive classes.
	// TODO: implement via CopyObject with StorageClass (Glacier Flexible Retrieval by default).
	TransitionStorageClass(ctx context.Context, key string, storageClass string) error
	// RestoreObject starts a Glacier restore. TODO: wire RestoreObject API + poll completion.
	RestoreObject(ctx context.Context, key string, days int, tier string) error
}

// StubStore is a no-op ObjectStore for local scaffold / Compose without S3 credentials.
type StubStore struct{}

// PresignPut implements ObjectStore.
func (StubStore) PresignPut(context.Context, string, string, time.Duration) (string, error) {
	return "", ErrNotConfigured
}

// PresignGet implements ObjectStore.
func (StubStore) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "", ErrNotConfigured
}

// Put implements ObjectStore.
func (StubStore) Put(context.Context, string, io.Reader, string) error { return ErrNotConfigured }

// Get implements ObjectStore.
func (StubStore) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, ErrNotConfigured
}

// Delete implements ObjectStore.
func (StubStore) Delete(context.Context, string) error { return ErrNotConfigured }

// TransitionStorageClass implements ObjectStore.
func (StubStore) TransitionStorageClass(context.Context, string, string) error {
	return ErrNotConfigured
}

// RestoreObject implements ObjectStore.
func (StubStore) RestoreObject(context.Context, string, int, string) error {
	return ErrNotConfigured
}
