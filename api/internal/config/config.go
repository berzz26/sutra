// Package config loads application configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration. Every field is explicit and
// documented so operators know exactly what the application reads.
type Config struct {
	// Server
	Host string // SERVER_HOST
	Port string // SERVER_PORT

	// Database
	DatabaseURL string // DATABASE_URL

	// Logging
	// Values: "debug", "info", "warn", "error"
	LogLevel string // LOG_LEVEL

	// Artifact store (optional, used later for uploaded transcripts/media).
	Artifact ArtifactConfig

	// Ingestion
	YTDLPBinary   string        // YTDLP_BINARY
	YTDLPTimeout  time.Duration // YTDLP_TIMEOUT_SECONDS (seconds)
	MaxUpload     int64         // TRANSCRIPT_MAX_UPLOAD_MB (bytes beyond config)
	MaxConcurrent int           // TRANSCRIPT_MAX_CONCURRENT_INGESTIONS
}

// ArtifactConfig describes the optional S3-compatible object store.
type ArtifactConfig struct {
	Enabled   bool   // ARTIFACT_STORE_ENABLED
	Endpoint  string // MINIO_ENDPOINT
	AccessKey string // MINIO_ACCESS_KEY
	SecretKey string // MINIO_SECRET_KEY
	Bucket    string // MINIO_BUCKET
	Region    string // MINIO_REGION
	UseSSL    bool   // MINIO_USE_SSL
}

func Load() (*Config, error) {
	cfg := &Config{
		Host:        getenv("SERVER_HOST", "0.0.0.0"),
		Port:        getenv("SERVER_PORT", "8080"),
		DatabaseURL: getenv("DATABASE_URL", ""),
		LogLevel:    getenv("LOG_LEVEL", "info"),
	}

	artifactEnabled, err := getBool("ARTIFACT_STORE_ENABLED", false)
	if err != nil {
		return nil, err
	}
	useSSL, err := getBool("MINIO_USE_SSL", false)
	if err != nil {
		return nil, err
	}

	cfg.Artifact = ArtifactConfig{
		Enabled:   artifactEnabled,
		Endpoint:  getenv("MINIO_ENDPOINT", ""),
		AccessKey: getenv("MINIO_ACCESS_KEY", ""),
		SecretKey: getenv("MINIO_SECRET_KEY", ""),
		Bucket:    getenv("MINIO_BUCKET", ""),
		Region:    getenv("MINIO_REGION", "us-east-1"),
		UseSSL:    useSSL,
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("config: DATABASE_URL is required")
	}
	if artifactEnabled {
		if cfg.Artifact.Endpoint == "" || cfg.Artifact.Bucket == "" {
			return nil, fmt.Errorf("config: MINIO_ENDPOINT and MINIO_BUCKET are required when ARTIFACT_STORE_ENABLED=true")
		}
	}

	cfg.YTDLPBinary = getenv("YTDLP_BINARY", "yt-dlp")
	cfg.YTDLPTimeout, err = getSeconds("YTDLP_TIMEOUT_SECONDS", 120)
	if err != nil {
		return nil, err
	}
	uploadMB, err := getInt("TRANSCRIPT_MAX_UPLOAD_MB", 20)
	if err != nil {
		return nil, err
	}
	if uploadMB <= 0 {
		return nil, fmt.Errorf("config: TRANSCRIPT_MAX_UPLOAD_MB must be positive")
	}
	cfg.MaxUpload = int64(uploadMB) * 1024 * 1024
	cfg.MaxConcurrent, err = getInt("TRANSCRIPT_MAX_CONCURRENT_INGESTIONS", 2)
	if err != nil {
		return nil, err
	}
	if cfg.MaxConcurrent <= 0 {
		return nil, fmt.Errorf("config: TRANSCRIPT_MAX_CONCURRENT_INGESTIONS must be positive")
	}

	return cfg, nil
}

func getenv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func getBool(key string, fallback bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("config: invalid boolean for %s: %q", key, v)
	}
	return b, nil
}

func getInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("config: invalid integer for %s: %q", key, v)
	}
	return n, nil
}

func getSeconds(key string, fallback int) (time.Duration, error) {
	n, err := getInt(key, fallback)
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		return 0, fmt.Errorf("config: %s must be positive", key)
	}
	return time.Duration(n) * time.Second, nil
}
