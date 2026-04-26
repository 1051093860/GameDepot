package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/1051093860/gamedepot/internal/app"
	"github.com/1051093860/gamedepot/internal/blob"
	"github.com/1051093860/gamedepot/internal/workspace"
)

func Restore(ctx context.Context, start string, targetPath string, sha256 string, force bool) error {
	if targetPath == "" {
		return fmt.Errorf("path is required")
	}

	if sha256 == "" {
		return fmt.Errorf("--sha256 is required")
	}

	targetPath, err := workspace.CleanRelPath(targetPath)
	if err != nil {
		return err
	}

	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}

	dst, err := workspace.SafeJoin(a.Root, targetPath)
	if err != nil {
		return err
	}

	if _, err := os.Stat(dst); err == nil {
		localSHA, err := blob.SHA256File(dst)
		if err != nil {
			return err
		}
		if localSHA != sha256 {
			if err := protectKnownOrForce(ctx, a.Store, dst, targetPath, localSHA, force); err != nil {
				return err
			}
		}
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	if err := downloadBlobToFile(ctx, a.Store, sha256, dst); err != nil {
		return err
	}

	actual, err := blob.SHA256File(dst)
	if err != nil {
		return err
	}

	if actual != sha256 {
		return fmt.Errorf("sha256 mismatch after restore: want %s got %s", sha256, actual)
	}

	fmt.Printf("Restored %s from %s\n", targetPath, shortSHA(sha256))
	fmt.Println("Run `gamedepot submit -m \"restore file\"` if you want to commit this restored version.")

	return nil
}
