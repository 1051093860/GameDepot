package store

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/1051093860/gamedepot/internal/blob"
)

type LocalBlobStore struct {
	Root string
}

func NewLocalBlobStore(root string) *LocalBlobStore {
	return &LocalBlobStore{Root: root}
}

func (s *LocalBlobStore) blobKey(sha256 string) string {
	if len(sha256) < 4 {
		return "sha256/" + sha256 + ".blob"
	}
	return filepath.ToSlash(filepath.Join("sha256", sha256[0:2], sha256[2:4], sha256+".blob"))
}

func (s *LocalBlobStore) blobPath(sha256 string) string {
	return blob.PathForSHA256(s.Root, sha256)
}

func (s *LocalBlobStore) objectPath(key string) (string, error) {
	key = cleanObjectKey(key)
	if key == "" {
		return "", fmt.Errorf("object key is required")
	}
	if strings.Contains(key, "../") || key == ".." || strings.HasPrefix(key, "/") || filepath.IsAbs(key) {
		return "", fmt.Errorf("unsafe object key %q", key)
	}
	path := filepath.Join(s.Root, filepath.FromSlash(key))
	rootAbs, err := filepath.Abs(s.Root)
	if err != nil {
		return "", err
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if pathAbs != rootAbs && !strings.HasPrefix(pathAbs, rootAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("object key escapes store root: %q", key)
	}
	return path, nil
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
	return s.PutObject(ctx, s.blobKey(sha256), r)
}

func (s *LocalBlobStore) Get(ctx context.Context, sha256 string) (io.ReadCloser, error) {
	return s.GetObject(ctx, s.blobKey(sha256))
}

func (s *LocalBlobStore) Delete(ctx context.Context, sha256 string) error {
	return s.DeleteObject(ctx, s.blobKey(sha256))
}

func (s *LocalBlobStore) HasObject(ctx context.Context, key string) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	path, err := s.objectPath(key)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (s *LocalBlobStore) PutObject(ctx context.Context, key string, r io.Reader) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	dstPath, err := s.objectPath(key)
	if err != nil {
		return err
	}
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

func (s *LocalBlobStore) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	path, err := s.objectPath(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open object %s: %w", key, err)
	}
	return f, nil
}

func (s *LocalBlobStore) DeleteObject(ctx context.Context, key string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	path, err := s.objectPath(key)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (s *LocalBlobStore) ListObjects(ctx context.Context, prefix string) ([]string, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	prefix = cleanObjectKey(prefix)
	var out []string
	if _, err := os.Stat(s.Root); os.IsNotExist(err) {
		return out, nil
	} else if err != nil {
		return nil, err
	}

	err := filepath.WalkDir(s.Root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.Root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if prefix == "" || strings.HasPrefix(rel, prefix) {
			out = append(out, rel)
		}
		return nil
	})
	return out, err
}

func cleanObjectKey(key string) string {
	key = strings.ReplaceAll(key, "\\", "/")
	key = strings.TrimSpace(key)
	key = strings.Trim(key, "/")
	return key
}
