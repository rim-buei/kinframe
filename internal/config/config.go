// Package config loads instance settings from the environment.
package config

import (
	"os"
	"strconv"
)

// Config holds runtime settings for app and worker.
type Config struct {
	HTTPAddr       string
	DatabasePath   string
	PublicBaseURL  string
	MaxUploadBytes int64
	ThemesDir      string

	// S3 / Glacier (stubbed; values accepted for Compose wiring).
	S3Endpoint         string
	S3Region           string
	S3Bucket           string
	S3AccessKeyID      string
	S3SecretAccessKey  string
	GlacierStorageClass string

	// OIDC (stubbed).
	GoogleClientID     string
	GoogleClientSecret string
	AppleClientID      string
	AppleClientSecret  string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:            getenv("HTTP_ADDR", ":8080"),
		DatabasePath:        getenv("DATABASE_PATH", "/data/app.db"),
		PublicBaseURL:       getenv("PUBLIC_BASE_URL", "http://localhost"),
		MaxUploadBytes:      getenvInt64("MAX_UPLOAD_BYTES", 100*1024*1024),
		ThemesDir:           getenv("THEMES_DIR", "/themes"),
		S3Endpoint:          os.Getenv("S3_ENDPOINT"),
		S3Region:            getenv("S3_REGION", "us-east-1"),
		S3Bucket:            os.Getenv("S3_BUCKET"),
		S3AccessKeyID:       os.Getenv("S3_ACCESS_KEY_ID"),
		S3SecretAccessKey:   os.Getenv("S3_SECRET_ACCESS_KEY"),
		GlacierStorageClass: getenv("GLACIER_STORAGE_CLASS", "GLACIER"), // Glacier Flexible Retrieval
		GoogleClientID:      os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret:  os.Getenv("GOOGLE_CLIENT_SECRET"),
		AppleClientID:       os.Getenv("APPLE_CLIENT_ID"),
		AppleClientSecret:   os.Getenv("APPLE_CLIENT_SECRET"),
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt64(key string, fallback int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}
