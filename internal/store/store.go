package store

import (
	"context"
	"io"
)

type BlobStore interface {
	Has(ctx context.Context, sha256 string) (bool, error)
	Put(ctx context.Context, sha256 string, r io.Reader) error
	Get(ctx context.Context, sha256 string) (io.ReadCloser, error)
	Delete(ctx context.Context, sha256 string) error
}
