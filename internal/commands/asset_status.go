package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/1051093860/gamedepot/internal/app"
	"github.com/1051093860/gamedepot/internal/blob"
	gdgit "github.com/1051093860/gamedepot/internal/git"
	"github.com/1051093860/gamedepot/internal/historyindex"
	"github.com/1051093860/gamedepot/internal/manifest"
	"github.com/1051093860/gamedepot/internal/workspace"
)

type AssetStatusOptions struct {
	JSON      bool
	Recursive bool
	// IncludeHistory controls Git manifest-history traversal. The traversal is
	// cached by HEAD so repeated UE refreshes do not rebuild it every time.
	IncludeHistory bool
	// IncludeRemote controls real object-store existence checks. UE status should
	// normally keep this false and use the local blob cache signal instead.
	IncludeRemote bool
	// HashLocal controls SHA-256 hashing of local files. Full-project and
	// multi-file refreshes should avoid hashing every large UE asset; selected
	// single-file detail requests can enable it.
	HashLocal bool
}

type AssetStatus struct {
	Path        string `json:"path"`
	HistoryOnly bool   `json:"history_only"`
	// Mode is kept for compatibility with older API consumers. In v0.8 it is the
	// effective storage for the current Git version when the path exists in the
	// manifest, otherwise it is the next-submit rule mode.
	Mode                 string `json:"mode"`
	DesiredMode          string `json:"desired_mode"`
	ManifestStorage      string `json:"manifest_storage"`
	Kind                 string `json:"kind"`
	Status               string `json:"status"`
	GitTracked           bool   `json:"git_tracked"`
	GitPorcelain         string `json:"git_porcelain,omitempty"`
	LocalExists          bool   `json:"local_exists"`
	LocalSHA256          string `json:"local_sha256,omitempty"`
	ManifestSHA256       string `json:"manifest_sha256,omitempty"`
	CurrentRemoteExists  bool   `json:"current_remote_exists"`
	CurrentRemoteChecked bool   `json:"current_remote_checked"`
	// CurrentBlobCached means the blob is present in the local GameDepot blob
	// cache. For a local-store profile this is also the real store; for an OSS/S3
	// profile it is only the local availability signal used by fast UE status.
	CurrentBlobCached    bool   `json:"current_blob_cached"`
	CurrentBlobAvailable bool   `json:"current_blob_available"`
	HistoryTotal         int    `json:"history_total"`
	HistoryRestorable    int    `json:"history_restorable"`
	HistoryMissing       int    `json:"history_missing"`
	Recoverability       string `json:"recoverability"`
	Severity             string `json:"severity"`
	Message              string `json:"message"`
}

func AssetStatusCommand(ctx context.Context, start, target string, opts AssetStatusOptions) error {
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}
	statuses, err := ComputeAssetStatusesWithOptions(ctx, a, target, opts.Recursive, AssetStatusOptions{IncludeHistory: true, IncludeRemote: true, HashLocal: true})
	if err != nil {
		return err
	}
	if opts.JSON {
		data, _ := json.MarshalIndent(statuses, "", "  ")
		fmt.Println(string(data))
		return nil
	}
	for _, s := range statuses {
		fmt.Println(s.Path)
		fmt.Printf("  mode: %s\n", s.Mode)
		fmt.Printf("  kind: %s\n", s.Kind)
		fmt.Printf("  status: %s\n", s.Status)
		if s.CurrentRemoteChecked {
			fmt.Printf("  current: %s\n", yesNo(s.CurrentRemoteExists, "remote ok", "remote missing"))
		} else if s.CurrentBlobCached {
			fmt.Printf("  current: local cache ok\n")
		} else {
			fmt.Printf("  current: not checked\n")
		}
		if s.HistoryTotal > 0 {
			fmt.Printf("  history: %d/%d restorable, %d missing\n", s.HistoryRestorable, s.HistoryTotal, s.HistoryMissing)
		}
		fmt.Printf("  recoverability: %s (%s)\n", s.Recoverability, s.Severity)
		if s.Message != "" {
			fmt.Printf("  message: %s\n", s.Message)
		}
	}
	return nil
}

func ComputeAssetStatuses(ctx context.Context, a *app.App, target string, recursive bool) ([]AssetStatus, error) {
	return ComputeAssetStatusesWithOptions(ctx, a, target, recursive, AssetStatusOptions{IncludeHistory: true, IncludeRemote: true, HashLocal: true})
}

// ComputeAssetStatusesForPathsWithOptions computes exact-path statuses in one
// batch. It is the fast path for editor selected-assets status: one manifest
// load, one Git status snapshot, optional per-file hashing, and no project-wide
// file walk.
func ComputeAssetStatusesForPathsWithOptions(ctx context.Context, a *app.App, paths []string, opts AssetStatusOptions) ([]AssetStatus, error) {
	m, err := loadStatusManifest(a)
	if err != nil {
		return nil, err
	}

	byPath := map[string]workspace.FileInfo{}
	localSet := map[string]struct{}{}
	cleaned := make([]string, 0, len(paths))
	seen := map[string]struct{}{}
	for _, raw := range paths {
		p, err := workspace.CleanRelPath(raw)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		cleaned = append(cleaned, p)
		if fi, ok, err := localFileInfoForStatus(a, p, opts.HashLocal); err != nil {
			return nil, err
		} else if ok {
			byPath[p] = fi
			localSet[p] = struct{}{}
		} else if localPathExists(a, p) {
			localSet[p] = struct{}{}
		}
	}
	sort.Strings(cleaned)

	idx, err := maybeHistoryIndex(a, opts.IncludeHistory)
	if err != nil {
		// Keep status usable in workspaces that are not Git repositories yet.
		idx = nil
	}
	snap := buildGitStatusSnapshot(gdgit.New(a.Root))

	out := make([]AssetStatus, 0, len(cleaned))
	for _, p := range cleaned {
		st, err := computeOneStatus(ctx, a, m, byPath, p, opts, idx, snap)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}

func ComputeAssetStatusesWithOptions(ctx context.Context, a *app.App, target string, recursive bool, opts AssetStatusOptions) ([]AssetStatus, error) {
	m, err := loadStatusManifest(a)
	if err != nil {
		return nil, err
	}

	cleanTarget := ""
	if strings.TrimSpace(target) != "" {
		cleanTarget, err = workspace.CleanRelPath(target)
		if err != nil {
			return nil, err
		}
	}

	byPath := map[string]workspace.FileInfo{}
	localSet := map[string]struct{}{}
	if cleanTarget != "" && !recursive {
		if fi, ok, err := localFileInfoForStatus(a, cleanTarget, opts.HashLocal); err != nil {
			return nil, err
		} else if ok {
			byPath[cleanTarget] = fi
			localSet[cleanTarget] = struct{}{}
		} else if localPathExists(a, cleanTarget) {
			localSet[cleanTarget] = struct{}{}
		}
	} else {
		var files []workspace.FileInfo
		if cleanTarget == "" || workspace.IsGameDepotManagedPath(cleanTarget) {
			files, err = workspace.ScanManagedWithOptions(a.Root, a.Config, workspace.ScanOptions{HashFiles: opts.HashLocal})
		} else {
			files, err = workspace.ScanAllWithOptions(a.Root, a.Config, workspace.ScanOptions{HashFiles: opts.HashLocal})
		}
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			byPath[f.Path] = f
			localSet[f.Path] = struct{}{}
		}
	}

	paths := map[string]struct{}{}
	for p := range m.Entries {
		if includeStatusPath(p, cleanTarget, recursive) {
			paths[p] = struct{}{}
		}
	}
	for p := range byPath {
		if includeStatusPath(p, cleanTarget, recursive) {
			paths[p] = struct{}{}
		}
	}

	idx, err := maybeHistoryIndex(a, opts.IncludeHistory)
	if err != nil {
		// Keep status usable in workspaces that are not Git repositories yet.
		idx = nil
	}
	if idx != nil {
		for _, it := range historyindex.HistoryOnly(*idx, m, localSet) {
			if includeStatusPath(it.Path, cleanTarget, recursive) {
				paths[it.Path] = struct{}{}
			}
		}
	}

	if cleanTarget != "" && !recursive {
		paths[cleanTarget] = struct{}{}
	}

	var list []string
	for p := range paths {
		list = append(list, p)
	}
	sort.Strings(list)

	snap := buildGitStatusSnapshot(gdgit.New(a.Root))
	out := make([]AssetStatus, 0, len(list))
	for _, p := range list {
		st, err := computeOneStatus(ctx, a, m, byPath, p, opts, idx, snap)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}

func loadStatusManifest(a *app.App) (manifest.Manifest, error) {
	m, err := manifest.Load(a.ManifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return manifest.New(a.Config.ProjectID), nil
		}
		return manifest.Manifest{}, err
	}
	m.Normalize()
	pruneManifestToManagedPaths(&m)
	return m, nil
}

func includeStatusPath(path, target string, recursive bool) bool {
	if target == "" {
		return true
	}
	path = strings.TrimPrefix(strings.ReplaceAll(path, "\\", "/"), "./")
	target = strings.TrimPrefix(strings.ReplaceAll(target, "\\", "/"), "./")
	if recursive {
		return path == target || strings.HasPrefix(path, strings.TrimSuffix(target, "/")+"/")
	}
	return path == target
}

func localFileInfoForStatus(a *app.App, rel string, hash bool) (workspace.FileInfo, bool, error) {
	abs, err := workspace.SafeJoin(a.Root, rel)
	if err != nil {
		return workspace.FileInfo{}, false, err
	}
	info, err := os.Stat(abs)
	if os.IsNotExist(err) {
		return workspace.FileInfo{}, false, nil
	}
	if err != nil {
		return workspace.FileInfo{}, false, err
	}
	if info.IsDir() {
		return workspace.FileInfo{}, false, nil
	}
	class, err := workspace.ClassifyRel(rel, a.Config)
	if err != nil {
		return workspace.FileInfo{}, false, err
	}
	if string(class.Mode) == "ignore" {
		return workspace.FileInfo{}, false, nil
	}
	sha := ""
	if hash {
		sha, err = blob.SHA256File(abs)
		if err != nil {
			return workspace.FileInfo{}, false, err
		}
	}
	return workspace.FileInfo{
		Path:        rel,
		AbsPath:     abs,
		Size:        info.Size(),
		MTimeUnix:   info.ModTime().Unix(),
		SHA256:      sha,
		Kind:        class.Kind,
		Mode:        class.Mode,
		RulePattern: class.RulePattern,
		Matched:     class.Matched,
	}, true, nil
}

func localPathExists(a *app.App, rel string) bool {
	abs, err := workspace.SafeJoin(a.Root, rel)
	if err != nil {
		return false
	}
	info, err := os.Stat(abs)
	return err == nil && !info.IsDir()
}

type gitStatusSnapshot struct {
	tracked   map[string]struct{}
	porcelain map[string]string
}

func buildGitStatusSnapshot(g gdgit.Git) gitStatusSnapshot {
	s := gitStatusSnapshot{tracked: map[string]struct{}{}, porcelain: map[string]string{}}
	if files, err := g.LsFiles(); err == nil {
		for _, p := range files {
			s.tracked[normalizeStatusPath(p)] = struct{}{}
		}
	}
	if out, err := g.Run("status", "--porcelain=v1"); err == nil {
		for _, raw := range strings.Split(strings.TrimRight(out, "\r\n"), "\n") {
			line := strings.TrimRight(raw, "\r")
			if strings.TrimSpace(line) == "" {
				continue
			}
			p := porcelainPath(line)
			if p == "" {
				continue
			}
			s.porcelain[p] = strings.ReplaceAll(line, "\\", "/")
		}
	}
	return s
}

func (s gitStatusSnapshot) isTracked(path string) bool {
	_, ok := s.tracked[normalizeStatusPath(path)]
	return ok
}

func (s gitStatusSnapshot) porcelainFor(path string) string {
	return s.porcelain[normalizeStatusPath(path)]
}

func porcelainPath(line string) string {
	if len(line) < 4 {
		return ""
	}
	p := strings.TrimSpace(line[3:])
	if i := strings.LastIndex(p, " -> "); i >= 0 {
		p = strings.TrimSpace(p[i+4:])
	}
	p = strings.Trim(p, "\"")
	return normalizeStatusPath(p)
}

func normalizeStatusPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(p, "./")
	return p
}

var historyStatusCache = struct {
	sync.Mutex
	key  string
	head string
	idx  historyindex.Index
}{}

func maybeHistoryIndex(a *app.App, include bool) (*historyindex.Index, error) {
	if !include {
		return nil, nil
	}
	g := gdgit.New(a.Root)
	head, err := g.CurrentCommit()
	if err != nil {
		return nil, err
	}
	key := a.Root + "|" + a.Config.ManifestPath
	historyStatusCache.Lock()
	if historyStatusCache.key == key && historyStatusCache.head == head {
		idx := historyStatusCache.idx
		historyStatusCache.Unlock()
		return &idx, nil
	}
	historyStatusCache.Unlock()

	built, err := historyindex.Build(g, a.Config.ManifestPath)
	if err != nil {
		return nil, err
	}

	historyStatusCache.Lock()
	historyStatusCache.key = key
	historyStatusCache.head = head
	historyStatusCache.idx = built
	historyStatusCache.Unlock()

	return &built, nil
}

func computeOneStatus(ctx context.Context, a *app.App, m manifest.Manifest, byPath map[string]workspace.FileInfo, p string, opts AssetStatusOptions, idx *historyindex.Index, gitSnap gitStatusSnapshot) (AssetStatus, error) {
	fi, hasLocalInfo := byPath[p]
	entry, hasManifest := m.Entries[p]

	// DesiredMode is always the next-submit classification, even when the file is
	// not tracked by Git, not staged, or not present in the current manifest yet.
	// This lets UE show a useful value for newly-created assets: New -> OSS,
	// New -> Git, Review, or Ignored instead of an empty/unknown state.
	desiredMode := "unknown"
	kind := ""
	if class, err := workspace.ClassifyRel(p, a.Config); err == nil {
		desiredMode = string(class.Mode)
		kind = class.Kind
	}
	if hasLocalInfo {
		desiredMode = string(fi.Mode)
		if fi.Kind != "" {
			kind = fi.Kind
		}
	}
	if hasManifest {
		if entry.Storage == "" {
			if entry.SHA256 != "" {
				entry.Storage = manifest.StorageBlob
			} else {
				entry.Storage = manifest.StorageGit
			}
		}
		if kind == "" {
			kind = entry.Kind
		}
	}

	manifestStorage := "none"
	effectiveMode := desiredMode
	if hasManifest {
		manifestStorage = string(entry.Storage)
		effectiveMode = manifestStorage
	}
	st := AssetStatus{Path: p, Mode: effectiveMode, DesiredMode: desiredMode, ManifestStorage: manifestStorage, Kind: kind}
	st.GitTracked = gitSnap.isTracked(p)
	st.GitPorcelain = gitSnap.porcelainFor(p)

	abs, err := workspace.SafeJoin(a.Root, p)
	if err == nil {
		if _, statErr := os.Stat(abs); statErr == nil {
			st.LocalExists = true
			if hasLocalInfo && fi.SHA256 != "" {
				st.LocalSHA256 = fi.SHA256
			} else if opts.HashLocal {
				sha, err := blob.SHA256File(abs)
				if err == nil {
					st.LocalSHA256 = sha
				}
			}
		}
	}
	if hasManifest && !entry.Deleted {
		st.ManifestSHA256 = entry.SHA256
	}
	if !hasManifest {
		if !st.LocalExists && idx != nil {
			items := idx.ForPath(p)
			if len(items) > 0 && !items[0].Deleted {
				st.HistoryOnly = true
				st.Status = "history_only"
				st.Recoverability = "history_restore"
				st.Severity = "info"
				st.Message = "This file is not in the current Content tree or manifest, but older Git/manifest history contains restorable versions."
				return st, nil
			}
		}
		if st.LocalExists {
			switch desiredMode {
			case "blob":
				st.Status = "new"
				st.Recoverability = "local_only"
				st.Severity = "warning"
				st.Message = "Local file is not in the current manifest yet. Next submit will store it in OSS/blob storage."
			case "git":
				st.Status = "new"
				st.Recoverability = "local_only"
				st.Severity = "warning"
				st.Message = "Local file is not in the current manifest yet. Next submit will store it directly in Git."
			case "ignore":
				st.Status = "ignored"
				st.Recoverability = "ignored"
				st.Severity = "info"
				st.Message = "Local file is ignored by the current GameDepot rules and will not be submitted."
			case "review":
				st.Status = "review"
				st.Recoverability = "needs_rule"
				st.Severity = "warning"
				st.Message = "Local file is not classified by the current rules. Set a Git/OSS/Ignore rule before submitting."
			default:
				st.Status = "new"
				st.Recoverability = "local_only"
				st.Severity = "warning"
				st.Message = "Local file is not in the current manifest yet. Set or confirm a rule before submitting."
			}
		} else {
			if desiredMode != "unknown" {
				st.Status = "missing_unsubmitted"
				st.Recoverability = "missing"
				st.Severity = "info"
				st.Message = "This path is not in the current manifest and no local file exists, but the current rules can classify it."
			} else {
				st.Status = "unknown"
				st.Recoverability = "unknown"
				st.Severity = "info"
			}
		}
		return st, nil
	}
	if entry.Deleted {
		st.Status = "deleted"
		st.Recoverability = "deleted"
		st.Severity = "info"
		return st, nil
	}
	if entry.Storage == manifest.StorageGit {
		if !st.LocalExists {
			st.Status = "missing_git_file"
			st.Recoverability = "git_checkout"
			st.Severity = "warning"
			st.Message = "This version says the file belongs in Git, but it is missing locally. Run git checkout/sync for this version."
			return st, nil
		}
		if !st.GitTracked {
			st.Status = "git_untracked_conflict"
			st.Recoverability = "submit_required"
			st.Severity = "warning"
			st.Message = "This version says the file belongs in Git, but the file is not tracked in the current Git index. Submit or re-checkout this version."
			return st, nil
		}
		if st.GitPorcelain == "" {
			st.Status = "synced"
			st.Recoverability = "git_history"
			st.Severity = "ok"
			st.Message = "This version stores the file in Git and the working tree is clean."
		} else {
			st.Status = "modified"
			st.Recoverability = "local_only"
			st.Severity = "warning"
			st.Message = "This Git-managed file has local changes that are not committed yet."
		}
		return st, nil
	}
	if entry.Storage == manifest.StorageBlob && st.GitTracked {
		st.Status = "routing_conflict_git_tracked"
		st.Recoverability = "submit_required"
		st.Severity = "error"
		st.Message = "Manifest says this file belongs in OSS/blob storage, but Git still tracks the file body. Submit through GameDepot to run git rm --cached and rewrite the manifest route."
		return st, nil
	}

	if entry.Storage == manifest.StorageBlob && entry.SHA256 != "" {
		st.CurrentBlobCached = localBlobCached(a, entry.SHA256)
		st.CurrentBlobAvailable = st.CurrentBlobCached
	}
	remoteOK := false
	remoteChecked := false
	if opts.IncludeRemote && entry.SHA256 != "" {
		remoteChecked = true
		remoteOK, _ = a.Store.Has(ctx, entry.SHA256)
		if remoteOK {
			st.CurrentBlobAvailable = true
		}
	}
	st.CurrentRemoteExists = remoteOK
	st.CurrentRemoteChecked = remoteChecked

	if st.LocalExists {
		if st.LocalSHA256 == "" {
			st.Status = "present_unverified"
		} else if st.LocalSHA256 == entry.SHA256 {
			st.Status = "synced"
		} else {
			st.Status = "modified"
		}
	} else {
		st.Status = "missing_local"
	}
	if opts.IncludeHistory && idx != nil {
		hist, missing := historyRestorabilityFromIndex(ctx, a, idx, p, opts.IncludeRemote)
		st.HistoryTotal = hist
		st.HistoryMissing = missing
		st.HistoryRestorable = hist - missing
	}

	if !remoteChecked {
		if !st.LocalExists {
			if st.CurrentBlobCached {
				st.Recoverability = "restorable_cached"
				st.Severity = "ok"
				st.Message = "Local file is missing, but the current blob exists in the local GameDepot cache and can be restored without querying OSS."
			} else {
				st.Recoverability = "cache_miss"
				st.Severity = "warning"
				st.Message = "Local file is missing and the current blob is not in the local cache. Fast status did not query OSS; restore/sync can still try the remote store."
			}
		} else if st.LocalSHA256 == "" {
			st.Recoverability = "unverified"
			st.Severity = "info"
			if st.CurrentBlobCached {
				st.Message = "Local file exists and the current blob is cached. Hash and remote checks were skipped for this fast status refresh."
			} else {
				st.Message = "Local file exists. Hash and remote checks were skipped for this fast status refresh."
			}
		} else if st.LocalSHA256 != entry.SHA256 {
			st.Recoverability = "local_only"
			st.Severity = "warning"
			st.Message = "Local file differs from manifest. Submit to upload it for teammates."
		} else if st.CurrentBlobCached {
			st.Recoverability = "restorable_cached"
			st.Severity = "ok"
			st.Message = "Local file matches the manifest hash and the current blob is cached locally."
		} else {
			st.Recoverability = "unverified"
			st.Severity = "info"
			st.Message = "Local file matches the manifest hash. Remote blob was not checked in this fast status refresh."
		}
	} else if !st.CurrentBlobAvailable {
		if st.LocalExists {
			st.Recoverability = "current_blob_missing"
			st.Severity = "error"
			st.Message = "Current manifest blob is missing from store/cache. If the local file still matches manifest SHA, run repair-current-blob."
		} else {
			st.Recoverability = "lost"
			st.Severity = "error"
			st.Message = "Local file and current remote/cache blob are both missing. Ask another teammate to re-upload."
		}
	} else if !st.LocalExists {
		if st.CurrentBlobCached {
			st.Recoverability = "restorable_cached"
			st.Severity = "ok"
			st.Message = "Local file is missing but the current blob is cached locally and can be restored."
		} else {
			st.Recoverability = "restorable"
			st.Severity = "ok"
			st.Message = "Local file is missing but current blob exists and can be restored."
		}
	} else if st.LocalSHA256 != "" && st.LocalSHA256 != entry.SHA256 {
		st.Recoverability = "local_only"
		st.Severity = "warning"
		st.Message = "Local file differs from manifest. Submit to upload it for teammates."
	} else if st.HistoryMissing > 0 {
		st.Recoverability = "history_broken"
		st.Severity = "warning"
		st.Message = fmt.Sprintf("Current version is restorable, but %d historical blob(s) are missing.", st.HistoryMissing)
	} else if st.CurrentBlobCached {
		st.Recoverability = "restorable_cached"
		st.Severity = "ok"
		st.Message = "Current version is available from local cache."
	} else {
		st.Recoverability = "restorable"
		st.Severity = "ok"
		st.Message = "Current version is restorable."
	}
	return st, nil
}

func localBlobCached(a *app.App, sha string) bool {
	if strings.TrimSpace(sha) == "" {
		return false
	}
	roots := []string{}
	if a.StoreInfo.Type == "local" && strings.TrimSpace(a.StoreInfo.Path) != "" {
		roots = append(roots, a.StoreInfo.Path)
	}
	roots = append(roots, filepath.Join(a.Root, ".gamedepot", "remote_blobs"))
	seen := map[string]struct{}{}
	for _, root := range roots {
		if root == "" {
			continue
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			abs = root
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		if _, err := os.Stat(blob.PathForSHA256(abs, sha)); err == nil {
			return true
		}
	}
	return false
}

func historyRestorabilityFromIndex(ctx context.Context, a *app.App, idx *historyindex.Index, path string, checkRemote bool) (total int, missing int) {
	seen := map[string]struct{}{}
	for _, it := range idx.ForPath(path) {
		if it.Deleted || it.SHA256 == "" {
			continue
		}
		if _, ok := seen[it.SHA256]; ok {
			continue
		}
		seen[it.SHA256] = struct{}{}
		total++
		if checkRemote {
			ok, err := a.Store.Has(ctx, it.SHA256)
			if err != nil || !ok {
				missing++
			}
		}
	}
	return total, missing
}

func historyRestorability(ctx context.Context, a *app.App, path string) (total int, missing int, err error) {
	versions, err := manifestHistorySHAs(a.Root, a.Config.ManifestPath, path)
	if err != nil {
		return 0, 0, err
	}
	seen := map[string]struct{}{}
	for _, sha := range versions {
		if sha == "" {
			continue
		}
		if _, ok := seen[sha]; ok {
			continue
		}
		seen[sha] = struct{}{}
		total++
		ok, err := a.Store.Has(ctx, sha)
		if err != nil {
			return total, missing, err
		}
		if !ok {
			missing++
		}
	}
	return total, missing, nil
}

func manifestHistorySHAs(root, manifestPath, path string) ([]string, error) {
	g := gdgit.New(root)
	commits, err := g.LogFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, c := range commits {
		raw, err := g.ShowFileAtCommit(c.Hash, manifestPath)
		if err != nil {
			continue
		}
		var m manifest.Manifest
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			continue
		}
		if e, ok := m.Entries[path]; ok && !e.Deleted && e.Storage == manifest.StorageBlob {
			out = append(out, e.SHA256)
		}
	}
	return out, nil
}

func yesNo(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}
