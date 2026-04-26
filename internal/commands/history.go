package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/1051093860/gamedepot/internal/app"
	gdgit "github.com/1051093860/gamedepot/internal/git"
	"github.com/1051093860/gamedepot/internal/manifest"
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

	g := gdgit.New(a.Root)

	commits, err := g.LogFile(a.Config.ManifestPath)
	if err != nil {
		return err
	}

	if len(commits) == 0 {
		fmt.Println("No manifest history found")
		return nil
	}

	fmt.Println(targetPath)
	fmt.Println("commit       time                       sha256        size        deleted")

	lastKey := ""

	for _, c := range commits {
		raw, err := g.ShowFileAtCommit(c.Hash, a.Config.ManifestPath)
		if err != nil {
			continue
		}

		var m manifest.Manifest

		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			continue
		}

		e, ok := m.Entries[targetPath]
		if !ok {
			continue
		}

		key := fmt.Sprintf("%s|%d|%t", e.SHA256, e.Size, e.Deleted)
		if key == lastKey {
			continue
		}

		lastKey = key

		fmt.Printf(
			"%s  %s  %-12s  %-10d  %t\n",
			shortCommit(c.Hash),
			trimTime(c.Time),
			shortSHA(e.SHA256),
			e.Size,
			e.Deleted,
		)
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
