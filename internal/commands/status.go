package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/1051093860/gamedepot/internal/app"
	gdgit "github.com/1051093860/gamedepot/internal/git"
	"github.com/1051093860/gamedepot/internal/localindex"
	gdrefs "github.com/1051093860/gamedepot/internal/refs"
	"github.com/1051093860/gamedepot/internal/workspace"
)

type statusReport struct {
	Added        []string `json:"added"`
	Modified     []string `json:"modified"`
	Deleted      []string `json:"deleted"`
	Stale        []string `json:"stale"`
	Conflict     []string `json:"conflict"`
	Unchanged    []string `json:"unchanged"`
	GitPorcelain string   `json:"git_porcelain"`
}

func Status(ctx context.Context, start string, jsonOut bool) error {
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}
	refs, err := gdrefs.NewStore(a.Root).LoadAll()
	if err != nil {
		return err
	}
	idx, err := localindex.Load(a.Root)
	if err != nil {
		return err
	}
	localFiles, err := workspace.ScanManaged(a.Root, a.Config)
	if err != nil {
		return err
	}
	localByPath := map[string]workspace.FileInfo{}
	for _, f := range localFiles {
		localByPath[f.Path] = f
	}

	report := statusReport{}
	seen := map[string]bool{}
	for _, p := range sortedFileInfoPaths(localByPath) {
		seen[p] = true
		f := localByPath[p]
		localOID := gdrefs.EnsureOID(f.SHA256)
		baseOID := idx.BaseOID(p)
		ref, hasRef := refs[p]
		if !hasRef {
			report.Added = append(report.Added, p)
			continue
		}
		remoteOID := gdrefs.EnsureOID(ref.OID)
		switch {
		case localOID == remoteOID:
			report.Unchanged = append(report.Unchanged, p)
		case baseOID == remoteOID:
			report.Modified = append(report.Modified, p)
		case baseOID != "" && localOID == baseOID && remoteOID != baseOID:
			report.Stale = append(report.Stale, p)
		default:
			report.Conflict = append(report.Conflict, p)
		}
	}
	for _, p := range gdrefs.SortedPaths(refs) {
		if !seen[p] {
			report.Deleted = append(report.Deleted, p)
		}
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
	printPathSection("Content added", report.Added)
	printPathSection("Content modified", report.Modified)
	printPathSection("Content deleted", report.Deleted)
	printPathSection("Content stale; run update before publish", report.Stale)
	printPathSection("Content conflicts", report.Conflict)
	if report.GitPorcelain != "" {
		fmt.Println("Git working tree changes:")
		fmt.Print(report.GitPorcelain)
		fmt.Println()
	}
	fmt.Printf("\nSummary: %d added, %d modified, %d deleted, %d stale, %d conflicts, %d unchanged\n", len(report.Added), len(report.Modified), len(report.Deleted), len(report.Stale), len(report.Conflict), len(report.Unchanged))
	return nil
}

func printPathSection(title string, paths []string) {
	if len(paths) == 0 {
		return
	}
	sort.Strings(paths)
	fmt.Println(title + ":")
	for _, p := range paths {
		fmt.Printf("  %s\n", p)
	}
	fmt.Println()
}

func shortSHA(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12]
}
