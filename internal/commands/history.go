package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/1051093860/gamedepot/internal/app"
	gdgit "github.com/1051093860/gamedepot/internal/git"
	"github.com/1051093860/gamedepot/internal/historyindex"
	"github.com/1051093860/gamedepot/internal/workspace"
)

type HistoryOptions struct {
	JSON bool
}

type HistoryResult struct {
	Path     string              `json:"path"`
	Versions []historyindex.Item `json:"versions"`
}

func History(ctx context.Context, start string, targetPath string) error {
	return HistoryWithOptions(ctx, start, targetPath, HistoryOptions{})
}

func HistoryWithOptions(ctx context.Context, start string, targetPath string, opts HistoryOptions) error {
	res, err := HistoryVersions(ctx, start, targetPath)
	if err != nil {
		return err
	}
	if opts.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	if len(res.Versions) == 0 {
		fmt.Println("No history found for", res.Path)
		return nil
	}
	fmt.Println(res.Path)
	fmt.Println("commit       time                       storage  sha256        size        deleted  message")
	for _, it := range res.Versions {
		fmt.Printf("%s  %s  %-7s  %-12s  %-10d  %-7t  %s\n", shortCommit(it.Commit), trimTime(it.Date), it.Storage, shortSHA(it.SHA256), it.Size, it.Deleted, it.Message)
	}
	return nil
}

func HistoryVersions(ctx context.Context, start string, targetPath string) (HistoryResult, error) {
	if targetPath == "" {
		return HistoryResult{}, fmt.Errorf("path is required")
	}
	cleaned, err := workspace.CleanRelPath(targetPath)
	if err != nil {
		return HistoryResult{}, err
	}
	a, err := app.Load(ctx, start)
	if err != nil {
		return HistoryResult{}, err
	}
	idx, err := historyindex.BuildForPath(gdgit.New(a.Root), a.Config.ManifestPath, cleaned)
	if err != nil {
		return HistoryResult{}, err
	}
	return HistoryResult{Path: cleaned, Versions: idx.ForPath(cleaned)}, nil
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
