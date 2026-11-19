// Package config loads instance settings from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds runtime settings for app and worker.
type Config struct {
	HTTPAddr       string
	DatabasePath   string
	PublicBaseURL  string
	MaxUploadBytes int64
	ThemesDir      string

	// Session cookie sealing (AES-GCM). Required for login sessions.
	SessionSecret string
	SessionTTL    time.Duration

	// S3 / Glacier (stubbed; values accepted for Compose wiring).
	S3Endpoint          string
	S3Region            string
	S3Bucket            string
	S3AccessKeyID       string
	S3SecretAccessKey   string
	GlacierStorageClass string

	// OIDC — each self-host operator supplies their own client credentials.
	GoogleClientID     string
	GoogleClientSecret string
	AppleClientID      string
	AppleClientSecret  string // pre-built JWT client secret (optional if key fields set)
	AppleTeamID        string
	AppleKeyID         string
	ApplePrivateKey    string // PEM; "\n" sequences are expanded

	// DevFakeOIDC enables a local-only fake IdP for smoke tests (never in production).
	DevFakeOIDC bool
}

// Load reads configuration from environment variables with sensible defaults.
func Load() (Config, error) {
	ttlHours := getenvInt64("SESSION_TTL_HOURS", 720) // 30 days
	if ttlHours < 1 {
		return Config{}, fmt.Errorf("SESSION_TTL_HOURS must be >= 1")
	}

	cfg := Config{
		HTTPAddr:            getenv("HTTP_ADDR", ":8080"),
		DatabasePath:        getenv("DATABASE_PATH", "/data/app.db"),
		PublicBaseURL:       strings.TrimRight(getenv("PUBLIC_BASE_URL", "http://localhost:8080"), "/"),
		MaxUploadBytes:      getenvInt64("MAX_UPLOAD_BYTES", 100*1024*1024),
		ThemesDir:           getenv("THEMES_DIR", "/themes"),
		SessionSecret:       os.Getenv("SESSION_SECRET"),
		SessionTTL:          time.Duration(ttlHours) * time.Hour,
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
		AppleTeamID:         os.Getenv("APPLE_TEAM_ID"),
		AppleKeyID:          os.Getenv("APPLE_KEY_ID"),
		ApplePrivateKey:     expandPEM(os.Getenv("APPLE_PRIVATE_KEY")),
		DevFakeOIDC:         getenvBool("DEV_FAKE_OIDC", false),
	}
	return cfg, nil
}

// CookieSecure reports whether session cookies should set the Secure flag
// (true when PUBLIC_BASE_URL is https).
func (c Config) CookieSecure() bool {
	return strings.HasPrefix(strings.ToLower(c.PublicBaseURL), "https://")
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

func getenvBool(key string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func expandPEM(s string) string {
	return strings.ReplaceAll(s, `\n`, "\n")
}
