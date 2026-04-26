package store

import (
	"context"
	"io"
)

type ObjectStore interface {
	HasObject(ctx context.Context, key string) (bool, error)
	PutObject(ctx context.Context, key string, r io.Reader) error
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)
	DeleteObject(ctx context.Context, key string) error
	ListObjects(ctx context.Context, prefix string) ([]string, error)
}

type BlobStore interface {
	ObjectStore
	Has(ctx context.Context, sha256 string) (bool, error)
	Put(ctx context.Context, sha256 string, r io.Reader) error
	Get(ctx context.Context, sha256 string) (io.ReadCloser, error)
	Delete(ctx context.Context, sha256 string) error
}
