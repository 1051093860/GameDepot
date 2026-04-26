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

	files, err := workspace.Scan(a.Root, a.Config)
	if err != nil {
		return err
	}

	blobFiles := workspace.FilterByMode(files, rules.ModeBlob)
	gitFiles := workspace.FilterByMode(files, rules.ModeGit)
	d := manifest.Compare(m, blobFiles)

	report := statusReport{
		Added:     d.Added,
		Modified:  d.Modified,
		Deleted:   d.Deleted,
		Unchanged: d.Unchanged,
		GitFiles:  gitFiles,
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

	printFileSection("Blob added", d.Added)
	printFileSection("Blob modified", d.Modified)
	printEntrySection("Blob deleted", d.Deleted)

	if len(gitFiles) > 0 {
		fmt.Println("Git-managed files matched by GameDepot rules:")
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
		"\nSummary: %d blob added, %d blob modified, %d blob deleted, %d blob unchanged, %d git-managed matched\n",
		len(d.Added),
		len(d.Modified),
		len(d.Deleted),
		len(d.Unchanged),
		len(gitFiles),
	)

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
