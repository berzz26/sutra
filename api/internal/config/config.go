// Package config loads application configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
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
