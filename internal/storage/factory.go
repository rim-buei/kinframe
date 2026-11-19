package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/rim-buei/kinframe/internal/config"
)

// NewFromConfig selects the object store backend.
//
//	STORAGE_BACKEND=fs (default when S3_BUCKET empty) → filesystem at STORAGE_PATH
//	STORAGE_BACKEND=s3 or S3_BUCKET set → AWS S3 (Glacier Flexible Retrieval supported)
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
		if cfg.S3Bucket == "" {
			return nil, "", fmt.Errorf("STORAGE_BACKEND=s3 requires S3_BUCKET")
		}
		store, err := NewS3Store(context.Background(), cfg)
		if err != nil {
			return nil, "", err
		}
		return store, "s3:" + cfg.S3Bucket, nil
	default:
		return nil, "", fmt.Errorf("unknown STORAGE_BACKEND %q (want fs|stub|s3)", backend)
	}
}
