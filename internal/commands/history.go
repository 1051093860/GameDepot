package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/1051093860/gamedepot/internal/app"
	gdgit "github.com/1051093860/gamedepot/internal/git"
	"github.com/1051093860/gamedepot/internal/historyindex"
	"github.com/1051093860/gamedepot/internal/workspace"
)

func History(ctx context.Context, start string, targetPath string) error {
	if targetPath == "" {
		return fmt.Errorf("path is required")
	}
	targetPath, err := workspace.CleanRelPath(targetPath)
	if err != nil {
		return err
	}
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}
	idx, err := historyindex.Build(gdgit.New(a.Root), a.Config.ManifestPath)
	if err != nil {
		return err
	}
	items := idx.ForPath(targetPath)
	if len(items) == 0 {
		fmt.Println("No history found for", targetPath)
		return nil
	}
	fmt.Println(targetPath)
	fmt.Println("commit       time                       storage  sha256        size        deleted  message")
	for _, it := range items {
		fmt.Printf("%s  %s  %-7s  %-12s  %-10d  %-7t  %s\n", shortCommit(it.Commit), trimTime(it.Date), it.Storage, shortSHA(it.SHA256), it.Size, it.Deleted, it.Message)
	}
	return nil
}

func shortCommit(s string) string {
	if len(s) <= 10 {
		return s
	}
	return s[:10]
}

func trimTime(s string) string {
	if len(s) <= 25 {
		return s
	}
	return strings.TrimSpace(s[:25])
}
