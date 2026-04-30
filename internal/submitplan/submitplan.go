package submitplan

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/1051093860/gamedepot/internal/app"
	"github.com/1051093860/gamedepot/internal/manifest"
	"github.com/1051093860/gamedepot/internal/rules"
	"github.com/1051093860/gamedepot/internal/workspace"
)

type Action string

const (
	ActionGitAdd     Action = "git_add"
	ActionUploadBlob Action = "upload_blob"
	ActionGitToBlob  Action = "git_to_blob"
	ActionBlobToGit  Action = "blob_to_git"
	ActionUpdateBlob Action = "update_blob"
	ActionRemove     Action = "remove"
	ActionIgnore     Action = "ignore"
	ActionReview     Action = "block_review"
	ActionNoop       Action = "noop"
)

type Item struct {
	Path            string             `json:"path"`
	PreviousStorage manifest.Storage   `json:"previous_storage"`
	DesiredStorage  manifest.Storage   `json:"desired_storage"`
	DesiredMode     rules.Mode         `json:"desired_mode"`
	Action          Action             `json:"action"`
	LocalExists     bool               `json:"local_exists"`
	LocalSHA256     string             `json:"local_sha256,omitempty"`
	Size            int64              `json:"size"`
	Kind            string             `json:"kind,omitempty"`
	File            workspace.FileInfo `json:"-"`
}

type Blocker struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type Plan struct {
	Items    []Item    `json:"items"`
	Blockers []Blocker `json:"blockers"`
}

func Build(ctx context.Context, a *app.App, m manifest.Manifest, allowReview bool) (Plan, error) {
	_ = ctx
	m.Normalize()
	files, err := workspace.ScanManaged(a.Root, a.Config)
	if err != nil {
		return Plan{}, err
	}
	byPath := map[string]workspace.FileInfo{}
	paths := map[string]struct{}{}
	for _, f := range files {
		if !workspace.IsGameDepotManagedPath(f.Path) {
			continue
		}
		byPath[f.Path] = f
		paths[f.Path] = struct{}{}
	}
	for p, e := range m.Entries {
		if !workspace.IsGameDepotManagedPath(p) {
			continue
		}
		if !e.Deleted {
			paths[p] = struct{}{}
		}
	}
	list := make([]string, 0, len(paths))
	for p := range paths {
		list = append(list, p)
	}
	sort.Strings(list)

	plan := Plan{}
	for _, p := range list {
		fi, hasLocal := byPath[p]
		old, hadOld := m.Entries[p]
		if old.Storage == "" {
			if old.SHA256 != "" {
				old.Storage = manifest.StorageBlob
			} else {
				old.Storage = manifest.StorageGit
			}
		}
		class, _ := workspace.ClassifyRel(p, a.Config)
		desiredMode := class.Mode
		kind := class.Kind
		if hasLocal {
			desiredMode = fi.Mode
			kind = fi.Kind
		}
		item := Item{Path: p, DesiredMode: desiredMode, Kind: kind, LocalExists: hasLocal}
		if hasLocal {
			item.File = fi
			item.LocalSHA256 = fi.SHA256
			item.Size = fi.Size
		}
		if hadOld && !old.Deleted {
			item.PreviousStorage = old.Storage
		}
		switch desiredMode {
		case rules.ModeReview:
			item.Action = ActionReview
			if !allowReview {
				plan.Blockers = append(plan.Blockers, Blocker{Path: p, Reason: "file needs a Git/OSS/Ignore rule"})
			}
		case rules.ModeIgnore:
			item.Action = ActionIgnore
		case rules.ModeGit:
			item.DesiredStorage = manifest.StorageGit
			if item.PreviousStorage == manifest.StorageBlob {
				item.Action = ActionBlobToGit
			} else {
				item.Action = ActionGitAdd
			}
		case rules.ModeBlob:
			item.DesiredStorage = manifest.StorageBlob
			if item.PreviousStorage == manifest.StorageGit {
				item.Action = ActionGitToBlob
			} else if item.PreviousStorage == manifest.StorageBlob {
				if hasLocal && old.SHA256 != fi.SHA256 {
					item.Action = ActionUpdateBlob
				} else {
					item.Action = ActionNoop
				}
			} else {
				item.Action = ActionUploadBlob
			}
		default:
			item.Action = ActionReview
			if !allowReview {
				plan.Blockers = append(plan.Blockers, Blocker{Path: p, Reason: "unsupported desired mode"})
			}
		}
		if (item.DesiredStorage == manifest.StorageGit || item.DesiredStorage == manifest.StorageBlob) && !hasLocal {
			if hadOld && !old.Deleted {
				item.Action = ActionRemove
			} else if !allowReview {
				plan.Blockers = append(plan.Blockers, Blocker{Path: p, Reason: "classified but local file is missing"})
			}
		}
		plan.Items = append(plan.Items, item)
	}
	return plan, nil
}

func UploadBlob(ctx context.Context, a *app.App, f workspace.FileInfo) error {
	exists, err := a.Store.Has(ctx, f.SHA256)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	in, err := os.Open(f.AbsPath)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := a.Store.Put(ctx, f.SHA256, in); err != nil {
		return err
	}
	fmt.Printf("uploaded %s %s\n", f.SHA256[:min(12, len(f.SHA256))], f.Path)
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
