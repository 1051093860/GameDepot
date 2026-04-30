package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/1051093860/gamedepot/internal/app"
	gdgc "github.com/1051093860/gamedepot/internal/gc"
	"github.com/1051093860/gamedepot/internal/manifest"
)

type GCImpactOptions struct {
	ProtectTags    []string
	ProtectAllTags bool
	JSON           bool
}

type GCImpactAsset struct {
	Path                    string `json:"path"`
	CurrentAffected         bool   `json:"current_affected"`
	AffectedHistoryVersions int    `json:"affected_history_versions"`
}

type GCImpactResult struct {
	DeleteCount             int             `json:"delete_count"`
	CurrentVersionAffected  int             `json:"current_version_affected"`
	HistoryVersionsAffected int             `json:"history_versions_affected"`
	Assets                  []GCImpactAsset `json:"assets"`
	SafeToExecute           bool            `json:"safe_to_execute"`
}

func GCImpact(ctx context.Context, start string, opts GCImpactOptions) error {
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}
	res, err := gdgc.Run(ctx, a, gdgc.Options{DryRun: true, ProtectTags: opts.ProtectTags, ProtectAllTags: opts.ProtectAllTags})
	if err != nil {
		return err
	}
	cand := map[string]struct{}{}
	for _, c := range res.Candidates {
		cand[c.SHA256] = struct{}{}
	}
	m, err := manifest.Load(a.ManifestPath)
	if err != nil {
		return err
	}
	assets := map[string]*GCImpactAsset{}
	for p, e := range m.Entries {
		if e.Deleted || e.Storage != manifest.StorageBlob || e.SHA256 == "" {
			continue
		}
		if _, ok := cand[e.SHA256]; ok {
			a := assets[p]
			if a == nil {
				a = &GCImpactAsset{Path: p}
				assets[p] = a
			}
			a.CurrentAffected = true
		}
	}
	for p := range m.Entries {
		shas, err := manifestHistorySHAs(a.Root, a.Config.ManifestPath, p)
		if err != nil {
			continue
		}
		seen := map[string]struct{}{}
		for _, sha := range shas {
			if _, done := seen[sha]; done {
				continue
			}
			seen[sha] = struct{}{}
			if _, ok := cand[sha]; ok {
				ia := assets[p]
				if ia == nil {
					ia = &GCImpactAsset{Path: p}
					assets[p] = ia
				}
				ia.AffectedHistoryVersions++
			}
		}
	}
	out := GCImpactResult{DeleteCount: len(res.Candidates), SafeToExecute: true}
	for _, a := range assets {
		if a.CurrentAffected {
			out.CurrentVersionAffected++
		}
		out.HistoryVersionsAffected += a.AffectedHistoryVersions
		out.Assets = append(out.Assets, *a)
	}
	sort.Slice(out.Assets, func(i, j int) bool { return out.Assets[i].Path < out.Assets[j].Path })
	if out.CurrentVersionAffected > 0 {
		out.SafeToExecute = false
	}
	if opts.JSON {
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
		return nil
	}
	fmt.Println("GC Impact Preview")
	fmt.Printf("Will delete blobs: %d\n", out.DeleteCount)
	fmt.Printf("Current versions affected: %d\n", out.CurrentVersionAffected)
	fmt.Printf("Historical versions affected: %d\n", out.HistoryVersionsAffected)
	fmt.Printf("Safe to execute: %t\n", out.SafeToExecute)
	if len(out.Assets) > 0 {
		fmt.Println("Affected assets:")
	}
	for _, a := range out.Assets {
		fmt.Printf("  %s current=%t history=%d\n", a.Path, a.CurrentAffected, a.AffectedHistoryVersions)
	}
	return nil
}
