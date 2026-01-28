package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/usepackyard/packyard/internal/config"
)

// Storage abstracts file storage across backends.
type Storage interface {
	Put(ctx context.Context, key string, r io.Reader, size int64) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}

func New(cfg *config.Config) (Storage, error) {
	switch cfg.Storage.Type {
	case "local":
		return NewLocal(cfg.Storage.LocalPath)
	case "s3":
		return NewS3(cfg.S3)
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", cfg.Storage.Type)
	}
}
