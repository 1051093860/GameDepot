package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/1051093860/gamedepot/internal/config"
	"github.com/1051093860/gamedepot/internal/store"
)

type App struct {
	Root         string
	Config       config.Config
	Store        store.BlobStore
	ManifestPath string
}

func Load(ctx context.Context, start string) (*App, error) {
	root, err := config.FindRoot(start)
	if err != nil {
		return nil, err
	}

	cfg, err := config.Load(root)
	if err != nil {
		return nil, err
	}

	var bs store.BlobStore

	switch cfg.Store.Type {
	case "local":
		bs = store.NewLocalBlobStore(filepath.Join(root, cfg.Store.Root))
	default:
		return nil, fmt.Errorf("unsupported store.type %q", cfg.Store.Type)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return &App{
		Root:         root,
		Config:       cfg,
		Store:        bs,
		ManifestPath: filepath.Join(root, cfg.ManifestPath),
	}, nil
}
