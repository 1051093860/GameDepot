package commands

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/1051093860/gamedepot/internal/app"
	gdgc "github.com/1051093860/gamedepot/internal/gc"
	"github.com/1051093860/gamedepot/internal/workspace"
)

type GCOptions struct {
	DryRun         bool
	ProtectTags    []string
	ProtectAllTags bool
	JSON           bool
}

func GC(ctx context.Context, start string, opts GCOptions) error {
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}
	res, err := gdgc.Run(ctx, a, gdgc.Options{
		DryRun:         opts.DryRun,
		ProtectTags:    opts.ProtectTags,
		ProtectAllTags: opts.ProtectAllTags,
		JSON:           opts.JSON,
	})
	if err != nil {
		return err
	}
	if opts.JSON {
		data, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	mode := "EXECUTE"
	if res.DryRun {
		mode = "DRY-RUN"
	}
	fmt.Printf("GC %s: remote blobs=%d referenced=%d candidates=%d deleted=%d\n", mode, res.RemoteBlob, res.Referenced, len(res.Candidates), res.Deleted)
	if len(res.ProtectedRefs) > 0 {
		fmt.Println("protected refs:")
		for _, p := range res.ProtectedRefs {
			if p.Error != "" {
				fmt.Printf("  - %s error=%s\n", p.Name, p.Error)
			} else {
				fmt.Printf("  - %s blobs=%d\n", p.Name, p.BlobCount)
			}
		}
	}
	if len(res.Candidates) > 0 {
		fmt.Println("candidates:")
		for _, c := range res.Candidates {
			fmt.Printf("  %s  %s\n", shortSHA(c.SHA256), c.Key)
		}
	}
	if res.LogPath != "" {
		fmt.Println("deletion log:", res.LogPath)
	}
	if res.DryRun {
		fmt.Println("No blobs deleted. Re-run with --execute to delete candidates.")
	}
	return nil
}

type DeleteVersionOptions struct {
	SHA256       string
	DryRun       bool
	ForceCurrent bool
	JSON         bool
}

func DeleteVersion(ctx context.Context, start string, targetPath string, opts DeleteVersionOptions) error {
	if targetPath == "" {
		return fmt.Errorf("usage: gamedepot delete-version <path> --sha256 <sha256> [--execute]")
	}
	if opts.SHA256 == "" {
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
	res, err := gdgc.DeleteVersion(ctx, a, targetPath, opts.SHA256, gdgc.DeleteVersionOptions{
		DryRun:       opts.DryRun,
		ForceCurrent: opts.ForceCurrent,
	})
	if err != nil {
		return err
	}
	if opts.JSON {
		data, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(data))
		return nil
	}
	if res.DryRun {
		fmt.Printf("delete-version DRY-RUN: %s %s\n", shortSHA(res.SHA256), res.Path)
		fmt.Println("No blob deleted. Re-run with --execute to delete this version.")
		return nil
	}
	fmt.Printf("deleted version: %s %s\n", shortSHA(res.SHA256), res.Path)
	if res.LogPath != "" {
		fmt.Println("deletion log:", res.LogPath)
	}
	return nil
}
