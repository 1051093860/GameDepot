package commands

import (
	"context"
	"fmt"

	"github.com/1051093860/gamedepot/internal/app"
	"github.com/1051093860/gamedepot/internal/restoreops"
)

func RestoreVersion(ctx context.Context, start, path, commit string, force bool) error {
	if path == "" || commit == "" {
		return fmt.Errorf("usage: gamedepot restore-version <path> --commit <commit>")
	}
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}
	if err := restoreops.RestoreVersion(ctx, a, path, commit, force); err != nil {
		return err
	}
	fmt.Printf("Restored %s from commit %s. Submit to make this the current version.\n", path, shortCommit(commit))
	return nil
}

func RevertAssets(ctx context.Context, start string, paths []string, force bool) error {
	if len(paths) == 0 {
		return fmt.Errorf("at least one path is required")
	}
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}
	if err := restoreops.RevertAssets(ctx, a, paths, force); err != nil {
		return err
	}
	fmt.Printf("Reverted %d asset(s).\n", len(paths))
	return nil
}
