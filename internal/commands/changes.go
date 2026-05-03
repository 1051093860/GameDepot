package commands

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/1051093860/gamedepot/internal/app"
	"github.com/1051093860/gamedepot/internal/localindex"
	gdrefs "github.com/1051093860/gamedepot/internal/refs"
	"github.com/1051093860/gamedepot/internal/workspace"
)

type AssetChangesOptions struct {
	// ExactHash enables SHA-256 hashing for local Content files. The UE panel keeps
	// this false by default so listing remains fast; publish/update still perform
	// exact hashing before changing anything.
	ExactHash bool
	Limit     int
}

type AssetChangesSummary struct {
	LocalAdded     int `json:"local_added"`
	LocalModified  int `json:"local_modified"`
	LocalDeleted   int `json:"local_deleted"`
	RemoteChanged  int `json:"remote_changed"`
	Conflicts      int `json:"conflicts"`
	DisplayedItems int `json:"displayed_items"`
	TotalItems     int `json:"total_items"`
}

type AssetChangeItem struct {
	Path      string `json:"path"`
	State     string `json:"state"`
	Kind      string `json:"kind,omitempty"`
	Message   string `json:"message"`
	BaseOID   string `json:"base_oid,omitempty"`
	LocalOID  string `json:"local_oid,omitempty"`
	RemoteOID string `json:"remote_oid,omitempty"`
	Size      int64  `json:"size,omitempty"`
	MTimeUnix int64  `json:"mtime_unix,omitempty"`
}

type AssetChangesResult struct {
	Summary         AssetChangesSummary `json:"summary"`
	Items           []AssetChangeItem   `json:"items"`
	FastMode        bool                `json:"fast_mode"`
	ExactHash       bool                `json:"exact_hash"`
	GeneratedAt     string              `json:"generated_at"`
	RemoteCheckedAt string              `json:"remote_checked_at,omitempty"`
	Message         string              `json:"message,omitempty"`
}

func ComputeAssetChanges(ctx context.Context, start string, opts AssetChangesOptions) (AssetChangesResult, error) {
	_ = ctx
	a, err := app.Load(ctx, start)
	if err != nil {
		return AssetChangesResult{}, err
	}
	return ComputeAssetChangesForApp(a, opts)
}

func ComputeAssetChangesForApp(a *app.App, opts AssetChangesOptions) (AssetChangesResult, error) {
	if opts.Limit <= 0 {
		opts.Limit = 500
	}

	idx, err := localindex.Load(a.Root)
	if err != nil {
		return AssetChangesResult{}, err
	}
	refStore := gdrefs.NewStore(a.Root)
	currentRefs, err := refStore.LoadAll()
	if err != nil {
		return AssetChangesResult{}, err
	}
	files, err := workspace.ScanManagedWithOptions(a.Root, a.Config, workspace.ScanOptions{HashFiles: opts.ExactHash})
	if err != nil {
		return AssetChangesResult{}, err
	}
	localByPath := map[string]workspace.FileInfo{}
	for _, f := range files {
		localByPath[f.Path] = f
	}

	conflictState, _ := loadConflictState(a.Root)
	conflictByPath := map[string]ConflictRecord{}
	if conflictState.Active {
		for _, c := range conflictState.Conflicts {
			conflictByPath[c.Path] = c
		}
	}

	paths := map[string]struct{}{}
	for _, p := range idx.Paths() {
		paths[p] = struct{}{}
	}
	for p := range currentRefs {
		paths[p] = struct{}{}
	}
	for p := range localByPath {
		paths[p] = struct{}{}
	}
	for p := range conflictByPath {
		paths[p] = struct{}{}
	}

	var sorted []string
	for p := range paths {
		if workspace.IsGameDepotManagedPath(p) {
			sorted = append(sorted, p)
		}
	}
	sort.Strings(sorted)

	result := AssetChangesResult{
		FastMode:    !opts.ExactHash,
		ExactHash:   opts.ExactHash,
		GeneratedAt: time.Now().Format(time.RFC3339),
	}

	items := make([]AssetChangeItem, 0)
	for _, p := range sorted {
		if rec, ok := conflictByPath[p]; ok {
			item := AssetChangeItem{Path: p, State: "conflict", Kind: rec.Kind, Message: conflictMessage(rec.Kind), BaseOID: rec.BaseOID, LocalOID: rec.LocalOID, RemoteOID: rec.RemoteOID}
			items = append(items, item)
			result.Summary.Conflicts++
			continue
		}

		baseOID := idx.BaseOID(p)
		ref, hasRef := currentRefs[p]
		local, hasLocal := localByPath[p]

		if baseOID == "" && hasLocal {
			localOID := ""
			if opts.ExactHash && local.SHA256 != "" {
				localOID = gdrefs.EnsureOID(local.SHA256)
			}
			items = append(items, AssetChangeItem{Path: p, State: "local_added", Kind: "added", Message: "Local Content file is not published yet.", LocalOID: localOID, Size: local.Size, MTimeUnix: local.MTimeUnix})
			result.Summary.LocalAdded++
			continue
		}

		if baseOID == "" && !hasLocal && hasRef {
			items = append(items, AssetChangeItem{Path: p, State: "remote_changed", Kind: "remote_added", Message: "Remote ref exists but local Content is not materialized. Run Update.", RemoteOID: ref.OID, Size: ref.Size})
			result.Summary.RemoteChanged++
			continue
		}

		if baseOID != "" && !hasLocal {
			items = append(items, AssetChangeItem{Path: p, State: "local_deleted", Kind: "deleted", Message: "Tracked Content file was deleted locally and is not published yet.", BaseOID: baseOID})
			result.Summary.LocalDeleted++
			continue
		}

		if baseOID != "" && hasLocal {
			localOID := ""
			if opts.ExactHash && local.SHA256 != "" {
				localOID = gdrefs.EnsureOID(local.SHA256)
				if localOID != baseOID {
					items = append(items, AssetChangeItem{Path: p, State: "local_modified", Kind: "modified", Message: "Local Content differs from the last GameDepot base.", BaseOID: baseOID, LocalOID: localOID, Size: local.Size, MTimeUnix: local.MTimeUnix})
					result.Summary.LocalModified++
				}
				continue
			}

			if hasRef && ref.OID != "" && ref.OID != baseOID {
				items = append(items, AssetChangeItem{Path: p, State: "local_modified", Kind: "ref_changed", Message: "Pointer ref differs from local base. Publish/update should reconcile it.", BaseOID: baseOID, RemoteOID: ref.OID, Size: local.Size, MTimeUnix: local.MTimeUnix})
				result.Summary.LocalModified++
				continue
			}

			if hasRef && ref.Size > 0 && local.Size != ref.Size {
				items = append(items, AssetChangeItem{Path: p, State: "local_modified", Kind: "size_changed", Message: "Local file size differs from the current ref. Publish will verify exact content.", BaseOID: baseOID, Size: local.Size, MTimeUnix: local.MTimeUnix})
				result.Summary.LocalModified++
				continue
			}
		}

		if baseOID != "" && !hasRef {
			// Rare but important: ref disappeared while the local base still knows the file.
			items = append(items, AssetChangeItem{Path: p, State: "remote_changed", Kind: "ref_missing", Message: "Pointer ref is missing locally. Run Update or Publish to reconcile.", BaseOID: baseOID})
			result.Summary.RemoteChanged++
			continue
		}
	}

	result.Summary.TotalItems = len(items)
	if len(items) > opts.Limit {
		result.Items = append(result.Items, items[:opts.Limit]...)
	} else {
		result.Items = items
	}
	result.Summary.DisplayedItems = len(result.Items)
	if result.Summary.TotalItems == 0 {
		if opts.ExactHash {
			result.Message = "No exact local Content changes or conflicts."
		} else {
			result.Message = "No obvious local Content changes or conflicts. Publish still performs exact verification."
		}
	}
	return result, nil
}

func conflictMessage(kind string) string {
	switch strings.TrimSpace(kind) {
	case "conflict-modified", "both_modified":
		return "Local and remote both modified this asset."
	case "conflict-deleted-remote", "local_modified_remote_deleted":
		return "Local changed this asset, but remote deleted it."
	case "conflict-local-deleted", "local_deleted_remote_modified":
		return "Local deleted this asset, but remote modified it."
	default:
		if kind == "" {
			return "Asset conflict requires a version decision."
		}
		return "Asset conflict requires a version decision: " + kind
	}
}
