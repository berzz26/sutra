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
	Workers       int           // PROCESSING_WORKERS
	Shutdown      time.Duration // SHUTDOWN_TIMEOUT
	Embedding     EmbeddingConfig
	Search        SearchConfig
}

type SearchConfig struct {
	CandidatePool int
	RRFK          int
	Timeout       time.Duration
}

type EmbeddingConfig struct {
	Model         string
	Version       string
	ONNXPath      string
	TokenizerPath string
	RuntimePath   string
	Dimensions    int
	BatchSize     int
	Threads       int
	Timeout       time.Duration
	TargetWords   int
	MinWords      int
	MaxWords      int
	OverlapWords  int
	MaxTokens     int
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
	cfg.Workers, err = getInt("PROCESSING_WORKERS", 2)
	if err != nil {
		return nil, err
	}
	if cfg.Workers <= 0 {
		return nil, fmt.Errorf("config: PROCESSING_WORKERS must be positive")
	}
	cfg.Shutdown, err = getSeconds("SHUTDOWN_TIMEOUT_SECONDS", 15)
	if err != nil {
		return nil, err
	}
	cfg.Embedding = EmbeddingConfig{
		Model: getenv("EMBEDDING_MODEL", "BAAI/bge-small-en-v1.5"), Version: getenv("EMBEDDING_MODEL_VERSION", "v1.5"),
		ONNXPath:      getenv("EMBEDDING_ONNX_PATH", "embedding/models/bge-small-en-v1.5/model.onnx"),
		TokenizerPath: getenv("EMBEDDING_TOKENIZER_PATH", "embedding/models/bge-small-en-v1.5/tokenizer.json"), RuntimePath: getenv("ONNXRUNTIME_SHARED_LIBRARY_PATH", "third_party/onnxruntime/onnxruntime-linux-x64-1.31.1/lib/libonnxruntime.so"), Dimensions: 384,
		BatchSize: 32, Threads: 4, Timeout: 5 * time.Minute, MaxTokens: 512,
		TargetWords: 180, MinWords: 80, MaxWords: 250, OverlapWords: 30,
	}
	if cfg.Embedding.Dimensions, err = getInt("EMBEDDING_DIMENSIONS", 384); err != nil {
		return nil, err
	}
	if cfg.Embedding.BatchSize, err = getInt("EMBEDDING_BATCH_SIZE", 32); err != nil {
		return nil, err
	}
	if cfg.Embedding.Threads, err = getInt("EMBEDDING_ONNX_THREADS", 4); err != nil {
		return nil, err
	}
	if cfg.Embedding.MaxTokens, err = getInt("EMBEDDING_MAX_TOKENS", 512); err != nil {
		return nil, err
	}
	if cfg.Embedding.Timeout, err = time.ParseDuration(getenv("EMBEDDING_TIMEOUT", "5m")); err != nil {
		return nil, fmt.Errorf("config: EMBEDDING_TIMEOUT: %w", err)
	}
	for name, value := range map[string]int{"EMBEDDING_DIMENSIONS": cfg.Embedding.Dimensions, "EMBEDDING_BATCH_SIZE": cfg.Embedding.BatchSize, "EMBEDDING_ONNX_THREADS": cfg.Embedding.Threads, "EMBEDDING_MAX_TOKENS": cfg.Embedding.MaxTokens} {
		if value <= 0 {
			return nil, fmt.Errorf("config: %s must be positive", name)
		}
	}
	if cfg.Embedding.MaxTokens > 512 {
		return nil, fmt.Errorf("config: EMBEDDING_MAX_TOKENS cannot exceed 512")
	}
	cfg.Search.CandidatePool, err = getInt("SEARCH_CANDIDATE_POOL", 100)
	if err != nil {
		return nil, err
	}
	cfg.Search.RRFK, err = getInt("SEARCH_RRF_K", 60)
	if err != nil {
		return nil, err
	}
	if cfg.Search.CandidatePool <= 0 || cfg.Search.RRFK <= 0 {
		return nil, fmt.Errorf("config: SEARCH_CANDIDATE_POOL and SEARCH_RRF_K must be positive")
	}
	if cfg.Search.Timeout, err = time.ParseDuration(getenv("SEARCH_TIMEOUT", "10s")); err != nil {
		return nil, fmt.Errorf("config: SEARCH_TIMEOUT: %w", err)
	}
	if cfg.Search.Timeout <= 0 {
		return nil, fmt.Errorf("config: SEARCH_TIMEOUT must be positive")
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
