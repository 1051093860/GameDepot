package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/1051093860/gamedepot/internal/app"
	gdgit "github.com/1051093860/gamedepot/internal/git"
	"github.com/1051093860/gamedepot/internal/manifest"
	"github.com/1051093860/gamedepot/internal/rules"
	"github.com/1051093860/gamedepot/internal/workspace"
)

type statusReport struct {
	ReviewFiles  []workspace.FileInfo `json:"review_files"`
	Added        []workspace.FileInfo `json:"added"`
	Modified     []workspace.FileInfo `json:"modified"`
	Deleted      []manifest.Entry     `json:"deleted"`
	Unchanged    []workspace.FileInfo `json:"unchanged"`
	GitFiles     []workspace.FileInfo `json:"git_files"`
	GitPorcelain string               `json:"git_porcelain"`
}

func Status(ctx context.Context, start string, jsonOut bool) error {
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}

	m, err := manifest.Load(a.ManifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			m = manifest.New(a.Config.ProjectID)
		} else {
			return err
		}
	}
	pruneManifestToManagedPaths(&m)

	allFiles, err := workspace.ScanManaged(a.Root, a.Config)
	if err != nil {
		return err
	}
	managedFiles := make([]workspace.FileInfo, 0, len(allFiles))
	for _, f := range allFiles {
		if workspace.IsGameDepotManagedPath(f.Path) {
			managedFiles = append(managedFiles, f)
		}
	}
	files := make([]workspace.FileInfo, 0, len(managedFiles))
	for _, f := range managedFiles {
		if f.Mode == rules.ModeBlob || f.Mode == rules.ModeGit {
			files = append(files, f)
		}
	}

	blobFiles := workspace.FilterByMode(files, rules.ModeBlob)
	gitFiles := workspace.FilterByMode(files, rules.ModeGit)
	d := manifest.Compare(m, blobFiles)

	report := statusReport{
		ReviewFiles: workspace.ReviewFiles(managedFiles),
		Added:       d.Added,
		Modified:    d.Modified,
		Deleted:     d.Deleted,
		Unchanged:   d.Unchanged,
		GitFiles:    gitFiles,
	}

	g := gdgit.New(a.Root)
	if out, err := g.StatusPorcelain(); err == nil {
		report.GitPorcelain = out
	}

	if jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	printFileSection("Needs rule review", report.ReviewFiles)
	printFileSection("Blob added", d.Added)
	printFileSection("Blob modified", d.Modified)
	printEntrySection("Blob deleted", d.Deleted)

	if len(gitFiles) > 0 {
		fmt.Println("Content Git files matched by GameDepot rules:")
		for _, f := range gitFiles {
			fmt.Printf("  %s  %s  %d bytes  %s\n", f.Path, shortSHA(f.SHA256), f.Size, f.Kind)
		}
		fmt.Println()
	}

	if report.GitPorcelain != "" {
		fmt.Println("Git working tree changes:")
		fmt.Print(report.GitPorcelain)
		fmt.Println()
	}

	fmt.Printf(
		"\nSummary: %d blob added, %d blob modified, %d blob deleted, %d blob unchanged, %d Content git file(s)\n",
		len(d.Added),
		len(d.Modified),
		len(d.Deleted),
		len(d.Unchanged),
		len(gitFiles),
	)
	if len(report.ReviewFiles) > 0 {
		fmt.Printf("Review required: %d unmanaged file(s). Submit will fail until rules are added or --allow-unmanaged is used.\n", len(report.ReviewFiles))
	}

	return nil
}

func printFileSection(title string, files []workspace.FileInfo) {
	if len(files) == 0 {
		return
	}

	fmt.Println(title + ":")

	for _, f := range files {
		fmt.Printf("  %s  %s  %d bytes  %s\n", f.Path, shortSHA(f.SHA256), f.Size, f.Kind)
	}

	fmt.Println()
}

func printEntrySection(title string, entries []manifest.Entry) {
	if len(entries) == 0 {
		return
	}

	fmt.Println(title + ":")

	for _, e := range entries {
		fmt.Printf("  %s  %s  %d bytes  %s\n", e.Path, shortSHA(e.SHA256), e.Size, e.Kind)
	}

	fmt.Println()
}

func shortSHA(s string) string {
	if len(s) <= 12 {
		return s
	}

	return s[:12]
}
