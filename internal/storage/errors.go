package storage

import "errors"

// ErrNotConfigured is returned by stub storage until S3 credentials are wired.
var ErrNotConfigured = errors.New("object store not configured (scaffold stub)")

// ErrObjectArchived means the original is in Glacier (or FS-simulated archive)
// and is not readable until restore completes.
var ErrObjectArchived = errors.New("object is archived")

// Class names used with TransitionStorageClass.
const (
	ClassStandard = "STANDARD"
	ClassGlacier  = "GLACIER" // S3 Glacier Flexible Retrieval
)
