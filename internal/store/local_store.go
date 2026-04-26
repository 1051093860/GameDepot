package store

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/1051093860/gamedepot/internal/blob"
)

type LocalBlobStore struct {
	Root string
}

func NewLocalBlobStore(root string) *LocalBlobStore {
	return &LocalBlobStore{Root: root}
}

func (s *LocalBlobStore) blobPath(sha256 string) string {
	return blob.PathForSHA256(s.Root, sha256)
}

func (s *LocalBlobStore) Has(ctx context.Context, sha256 string) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	_, err := os.Stat(s.blobPath(sha256))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (s *LocalBlobStore) Put(ctx context.Context, sha256 string, r io.Reader) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	dstPath := s.blobPath(sha256)
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}

	tmpPath := dstPath + ".tmp"
	tmp, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := io.Copy(tmp, r); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, dstPath)
}

func (s *LocalBlobStore) Get(ctx context.Context, sha256 string) (io.ReadCloser, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	path := s.blobPath(sha256)
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open blob %s: %w", sha256, err)
	}
	return f, nil
}

func (s *LocalBlobStore) Delete(ctx context.Context, sha256 string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	err := os.Remove(s.blobPath(sha256))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
