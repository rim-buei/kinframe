package storage

import (
	"fmt"
	"strings"

	"github.com/rim-buei/kinframe/internal/config"
)

// NewFromConfig selects the object store backend.
//
//	STORAGE_BACKEND=fs (default when S3_BUCKET empty) → filesystem at STORAGE_PATH
//	STORAGE_BACKEND=s3 or S3_BUCKET set → stub until AWS SDK is wired (returns error if forced without impl)
//	STORAGE_BACKEND=stub → explicit no-op stub
func NewFromConfig(cfg config.Config) (ObjectStore, string, error) {
	backend := strings.ToLower(strings.TrimSpace(cfg.StorageBackend))
	if backend == "" {
		if cfg.S3Bucket != "" {
			backend = "s3"
		} else {
			backend = "fs"
		}
	}
	switch backend {
	case "fs", "filesystem", "local":
		path := cfg.StoragePath
		if path == "" {
			path = "./data/objects"
		}
		store, err := NewFSStore(path)
		if err != nil {
			return nil, "", err
		}
		return store, "fs:" + path, nil
	case "stub":
		return StubStore{}, "stub", nil
	case "s3":
		// Real AWS SDK wiring is a later slice; refuse silently broken "configured" S3.
		if cfg.S3Bucket == "" {
			return nil, "", fmt.Errorf("STORAGE_BACKEND=s3 requires S3_BUCKET")
		}
		return nil, "", fmt.Errorf("S3 backend not implemented yet; use STORAGE_BACKEND=fs for local/dev")
	default:
		return nil, "", fmt.Errorf("unknown STORAGE_BACKEND %q (want fs|stub|s3)", backend)
	}
}
