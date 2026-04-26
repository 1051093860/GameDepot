package app

import (
	"context"
	"path/filepath"

	"github.com/1051093860/gamedepot/internal/config"
	"github.com/1051093860/gamedepot/internal/store"
)

type App struct {
	Root         string
	Config       config.Config
	Store        store.BlobStore
	StoreInfo    store.Info
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

	bs, info, err := store.NewFromProject(root, cfg)
	if err != nil {
		return nil, err
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
		StoreInfo:    info,
		ManifestPath: filepath.Join(root, cfg.ManifestPath),
	}, nil
}
