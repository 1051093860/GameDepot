package gc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/1051093860/gamedepot/internal/app"
	gdgit "github.com/1051093860/gamedepot/internal/git"
	"github.com/1051093860/gamedepot/internal/manifest"
)

type Options struct {
	DryRun         bool
	ProtectTags    []string
	ProtectAllTags bool
	JSON           bool
}

type Candidate struct {
	SHA256 string `json:"sha256"`
	Key    string `json:"key"`
	Reason string `json:"reason"`
}

type ProtectedRef struct {
	Name       string `json:"name"`
	BlobCount  int    `json:"blob_count"`
	MissingRef bool   `json:"missing_ref,omitempty"`
	Error      string `json:"error,omitempty"`
}

type Result struct {
	DryRun        bool           `json:"dry_run"`
	Deleted       int            `json:"deleted"`
	Candidates    []Candidate    `json:"candidates"`
	ProtectedRefs []ProtectedRef `json:"protected_refs"`
	Referenced    int            `json:"referenced_blob_count"`
	RemoteBlob    int            `json:"remote_blob_count"`
	LogPath       string         `json:"log_path,omitempty"`
}

type DeleteVersionOptions struct {
	DryRun       bool
	ForceCurrent bool
}

type DeleteVersionResult struct {
	Path           string `json:"path"`
	SHA256         string `json:"sha256"`
	DryRun         bool   `json:"dry_run"`
	Deleted        bool   `json:"deleted"`
	FoundInHistory bool   `json:"found_in_history"`
	WasCurrent     bool   `json:"was_current"`
	LogPath        string `json:"log_path,omitempty"`
}

func Run(ctx context.Context, a *app.App, opts Options) (Result, error) {
	if !opts.DryRun && len(opts.ProtectTags) == 0 && !opts.ProtectAllTags {
		// This is allowed, but keep the behavior explicit in the result.
	}

	refs, protected, err := referencedSet(ctx, a, opts)
	if err != nil {
		return Result{}, err
	}

	keys, err := a.Store.ListObjects(ctx, "sha256/")
	if err != nil {
		return Result{}, fmt.Errorf("list store blobs: %w", err)
	}
	sort.Strings(keys)

	var candidates []Candidate
	for _, key := range keys {
		sha, ok := SHAFromBlobKey(key)
		if !ok {
			continue
		}
		if _, ok := refs[sha]; ok {
			continue
		}
		candidates = append(candidates, Candidate{SHA256: sha, Key: key, Reason: "not referenced by current manifest or protected refs"})
	}

	res := Result{
		DryRun:        opts.DryRun,
		Candidates:    candidates,
		ProtectedRefs: protected,
		Referenced:    len(refs),
		RemoteBlob:    countBlobKeys(keys),
	}

	if opts.DryRun || len(candidates) == 0 {
		return res, nil
	}

	for _, c := range candidates {
		if err := a.Store.Delete(ctx, c.SHA256); err != nil {
			return res, fmt.Errorf("delete blob %s: %w", c.SHA256, err)
		}
		res.Deleted++
	}

	logPath, err := WriteDeletionLog(a.Root, map[string]any{
		"type":             "gc",
		"time":             time.Now().UTC().Format(time.RFC3339),
		"project_id":       a.Config.ProjectID,
		"store_profile":    a.StoreInfo.Profile,
		"store_type":       a.StoreInfo.Type,
		"dry_run":          false,
		"deleted":          res.Deleted,
		"candidates":       candidates,
		"protected_tags":   opts.ProtectTags,
		"protect_all_tags": opts.ProtectAllTags,
	})
	if err != nil {
		return res, err
	}
	res.LogPath = logPath
	return res, nil
}

func DeleteVersion(ctx context.Context, a *app.App, targetPath string, sha string, opts DeleteVersionOptions) (DeleteVersionResult, error) {
	sha = strings.ToLower(strings.TrimSpace(sha))
	if !IsSHA256(sha) {
		return DeleteVersionResult{}, fmt.Errorf("--sha256 must be a full 64-character sha256")
	}

	m, err := manifest.Load(a.ManifestPath)
	if err != nil {
		return DeleteVersionResult{}, err
	}
	current, isCurrent := m.Entries[targetPath]
	wasCurrent := isCurrent && !current.Deleted && strings.EqualFold(current.SHA256, sha)
	if wasCurrent && !opts.ForceCurrent {
		return DeleteVersionResult{}, fmt.Errorf("refusing to delete the current manifest version for %s; pass --force-current if you really want to break current restore", targetPath)
	}

	found, err := ShaAppearsInManifestHistory(a.Root, a.Config.ManifestPath, targetPath, sha)
	if err != nil {
		return DeleteVersionResult{}, err
	}
	if !found && !wasCurrent {
		return DeleteVersionResult{}, fmt.Errorf("sha256 %s was not found in manifest history for %s", sha, targetPath)
	}

	res := DeleteVersionResult{Path: targetPath, SHA256: sha, DryRun: opts.DryRun, FoundInHistory: found, WasCurrent: wasCurrent}
	if opts.DryRun {
		return res, nil
	}

	if err := a.Store.Delete(ctx, sha); err != nil {
		return res, err
	}
	res.Deleted = true

	logPath, err := WriteDeletionLog(a.Root, map[string]any{
		"type":          "delete-version",
		"time":          time.Now().UTC().Format(time.RFC3339),
		"project_id":    a.Config.ProjectID,
		"store_profile": a.StoreInfo.Profile,
		"store_type":    a.StoreInfo.Type,
		"path":          targetPath,
		"sha256":        sha,
		"was_current":   wasCurrent,
	})
	if err != nil {
		return res, err
	}
	res.LogPath = logPath
	return res, nil
}

func referencedSet(ctx context.Context, a *app.App, opts Options) (map[string]struct{}, []ProtectedRef, error) {
	_ = ctx
	refs := map[string]struct{}{}
	protected := []ProtectedRef{}

	m, err := manifest.Load(a.ManifestPath)
	if err != nil {
		return nil, nil, err
	}
	addManifestRefs(refs, m)
	protected = append(protected, ProtectedRef{Name: "HEAD/current", BlobCount: countManifestRefs(m)})

	g := gdgit.New(a.Root)
	tags := append([]string{}, opts.ProtectTags...)
	if opts.ProtectAllTags {
		all, err := g.Tags()
		if err != nil {
			return nil, nil, err
		}
		tags = append(tags, all...)
	}
	tags = unique(tags)

	for _, tag := range tags {
		raw, err := g.ShowFileAtCommit(tag, a.Config.ManifestPath)
		if err != nil {
			protected = append(protected, ProtectedRef{Name: tag, MissingRef: true, Error: err.Error()})
			continue
		}
		var tm manifest.Manifest
		if err := json.Unmarshal([]byte(raw), &tm); err != nil {
			protected = append(protected, ProtectedRef{Name: tag, MissingRef: true, Error: err.Error()})
			continue
		}
		addManifestRefs(refs, tm)
		protected = append(protected, ProtectedRef{Name: tag, BlobCount: countManifestRefs(tm)})
	}

	return refs, protected, nil
}

func addManifestRefs(out map[string]struct{}, m manifest.Manifest) {
	for _, e := range m.Entries {
		if e.Deleted || e.SHA256 == "" {
			continue
		}
		out[strings.ToLower(e.SHA256)] = struct{}{}
	}
}

func countManifestRefs(m manifest.Manifest) int {
	n := 0
	for _, e := range m.Entries {
		if !e.Deleted && e.SHA256 != "" {
			n++
		}
	}
	return n
}

func countBlobKeys(keys []string) int {
	n := 0
	for _, key := range keys {
		if _, ok := SHAFromBlobKey(key); ok {
			n++
		}
	}
	return n
}

func SHAFromBlobKey(key string) (string, bool) {
	key = strings.ReplaceAll(key, "\\", "/")
	base := key
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if !strings.HasSuffix(base, ".blob") {
		return "", false
	}
	sha := strings.TrimSuffix(base, ".blob")
	if !IsSHA256(sha) {
		return "", false
	}
	return strings.ToLower(sha), true
}

func IsSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func ShaAppearsInManifestHistory(root string, manifestPath string, targetPath string, sha string) (bool, error) {
	g := gdgit.New(root)
	commits, err := g.LogFile(manifestPath)
	if err != nil {
		return false, err
	}
	for _, c := range commits {
		raw, err := g.ShowFileAtCommit(c.Hash, manifestPath)
		if err != nil {
			continue
		}
		var m manifest.Manifest
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			continue
		}
		if e, ok := m.Entries[targetPath]; ok && strings.EqualFold(e.SHA256, sha) {
			return true, nil
		}
	}
	return false, nil
}

func WriteDeletionLog(root string, record any) (string, error) {
	path := filepath.Join(root, ".gamedepot", "logs", "deletions.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return "", err
	}
	return path, nil
}

func unique(in []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
