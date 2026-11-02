// Package artifact stores transcript files in an optional S3-compatible
// object store (MinIO). It is a no-op when artifact storage is disabled.
package artifact

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"regexp"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/berzz/searcher/backend/internal/config"
)

// Store is implemented by object stores used for transcript artifacts. The
// interface exists so ingestion can work with the real MinIO client or no
// store at all.
type Store interface {
	// Put writes an object and verifies the write landed (StatObject).
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	// Delete removes an object. Deleting a missing key is not an error.
	Delete(ctx context.Context, key string) error
}

// MinioStore implements Store against any S3-compatible endpoint.
type MinioStore struct {
	client *minio.Client
	bucket string
}

// New builds a MinioStore from the artifact-store configuration block.
func New(cfg config.ArtifactConfig) (*MinioStore, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("artifact: create minio client: %w", err)
	}
	return &MinioStore{client: client, bucket: cfg.Bucket}, nil
}

// Put writes then verifies: a successful call means the object actually
// exists with the expected size.
func (s *MinioStore) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	info, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("artifact: put %s: %w", key, err)
	}
	stat, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return fmt.Errorf("artifact: verify %s after upload: %w", key, err)
	}
	if stat.Size != info.Size {
		return fmt.Errorf("artifact: verify %s: stored %d bytes, expected %d", key, stat.Size, info.Size)
	}
	return nil
}

func (s *MinioStore) Delete(ctx context.Context, key string) error {
	if _, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{}); err != nil {
		switch {
		case isMissing(err):
			return nil
		default:
			return fmt.Errorf("artifact: stat %s: %w", key, err)
		}
	}
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("artifact: remove %s: %w", key, err)
	}
	return nil
}

// StoreOrNoop returns a Store from config, or nil when disabled — callers
// must treat nil as "artifact storage disabled" and continue without it.
func StoreOrNoop(cfg config.ArtifactConfig) (Store, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	store, err := New(cfg)
	if err != nil {
		return nil, err
	}
	if err := store.ensureBucket(context.Background()); err != nil {
		return nil, err
	}
	slog.Info("artifact store enabled", "bucket", cfg.Bucket)
	return store, nil
}

func (s *MinioStore) ensureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("artifact: check bucket %s: %w", s.bucket, err)
	}
	if exists {
		return nil
	}
	if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("artifact: create bucket %s: %w", s.bucket, err)
	}
	return nil
}

var missingObjectCodes = regexp.MustCompile(`NoSuchKey|404|Not Found`)

func isMissing(err error) bool {
	resp := minio.ToErrorResponse(err)
	return resp.Code == "NoSuchKey" || missingObjectCodes.MatchString(err.Error())
}
