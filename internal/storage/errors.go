package storage

import "errors"

// ErrNotConfigured is returned by stub storage until S3 credentials are wired.
var ErrNotConfigured = errors.New("object store not configured (scaffold stub)")
