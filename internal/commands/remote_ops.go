package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/1051093860/gamedepot/internal/app"
	gdgit "github.com/1051093860/gamedepot/internal/git"
)

// Push uses Git's own remote/branch configuration. If no remote is configured,
// GameDepot remains a local version-management tool and push is a no-op.
func Push(ctx context.Context, start string) error {
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}
	g := gdgit.New(a.Root)
	if !g.HasAnyRemote() {
		fmt.Println("No git remote configured; using local version management only.")
		return nil
	}
	branch, _ := g.CurrentBranch()
	if branch == "" || branch == "HEAD" {
		_, err = g.Run("push")
	} else {
		_, err = g.Run("push", "-u", "origin", branch)
	}
	if err != nil && strings.Contains(err.Error(), "origin") {
		_, err = g.Run("push")
	}
	return err
}

// Pull uses Git's own remote/branch configuration. If no remote is configured,
// GameDepot stays local and pull is a no-op.
func Pull(ctx context.Context, start string) error {
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}
	g := gdgit.New(a.Root)
	if !g.HasAnyRemote() {
		fmt.Println("No git remote configured; using local version management only.")
		return nil
	}
	_, err = g.Run("pull", "--ff-only")
	return err
}
