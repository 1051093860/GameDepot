package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/1051093860/gamedepot/internal/app"
	"github.com/1051093860/gamedepot/internal/blob"
	gdgit "github.com/1051093860/gamedepot/internal/git"
	"github.com/1051093860/gamedepot/internal/localindex"
	gdrefs "github.com/1051093860/gamedepot/internal/refs"
	"github.com/1051093860/gamedepot/internal/submitplan"
	"github.com/1051093860/gamedepot/internal/workspace"
)

const ConflictsRelPath = ".gamedepot/state/conflicts.json"

type ConflictState struct {
	Active       bool             `json:"active"`
	Type         string           `json:"type"`
	Branch       string           `json:"branch,omitempty"`
	Remote       string           `json:"remote,omitempty"`
	RemoteCommit string           `json:"remote_commit,omitempty"`
	CreatedAt    string           `json:"created_at"`
	Conflicts    []ConflictRecord `json:"conflicts"`
}

type ConflictRecord struct {
	Path      string `json:"path"`
	RefPath   string `json:"ref_path"`
	Kind      string `json:"kind"`
	BaseOID   string `json:"base_oid,omitempty"`
	LocalOID  string `json:"local_oid,omitempty"`
	RemoteOID string `json:"remote_oid,omitempty"`
	Decision  string `json:"decision,omitempty"`
}

func conflictStatePath(root string) string {
	return filepath.Join(root, filepath.FromSlash(ConflictsRelPath))
}

func saveUpdateConflicts(root string, actions []updateAction) error {
	g := gdgit.New(root)
	branch, remote := currentBranchAndRemote(g)
	remoteCommit, _ := g.CurrentCommit()
	st := ConflictState{
		Active:       true,
		Type:         "update",
		Branch:       branch,
		Remote:       remote,
		RemoteCommit: remoteCommit,
		CreatedAt:    time.Now().Format(time.RFC3339),
		Conflicts:    make([]ConflictRecord, 0, len(actions)),
	}
	for _, a := range actions {
		refPath, _ := gdrefs.RefPathFor(a.Path)
		st.Conflicts = append(st.Conflicts, ConflictRecord{
			Path:      a.Path,
			RefPath:   refPath,
			Kind:      a.Kind,
			BaseOID:   a.BaseOID,
			LocalOID:  a.LocalOID,
			RemoteOID: a.RemoteOID,
		})
	}
	return saveConflictState(root, st)
}

func saveConflictState(root string, st ConflictState) error {
	p := conflictStatePath(root)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(p, data, 0o644)
}

func loadConflictState(root string) (ConflictState, error) {
	p := conflictStatePath(root)
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return ConflictState{}, nil
	}
	if err != nil {
		return ConflictState{}, err
	}
	var st ConflictState
	if err := json.Unmarshal(data, &st); err != nil {
		return ConflictState{}, fmt.Errorf("read %s: %w", ConflictsRelPath, err)
	}
	return st, nil
}

func clearConflicts(root string) error {
	err := os.Remove(conflictStatePath(root))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func Conflicts(ctx context.Context, start string, jsonOut bool) error {
	_ = ctx
	root, err := workspaceRoot(start)
	if err != nil {
		return err
	}
	st, err := loadConflictState(root)
	if err != nil {
		return err
	}
	if jsonOut {
		data, _ := json.MarshalIndent(st, "", "  ")
		fmt.Println(string(data))
		return nil
	}
	if !st.Active || len(st.Conflicts) == 0 {
		fmt.Println("No active GameDepot conflicts")
		return nil
	}
	fmt.Printf("Active GameDepot %s conflicts (%d):\n", st.Type, len(st.Conflicts))
	for _, c := range st.Conflicts {
		fmt.Printf("  %s  %s  base=%s local=%s remote=%s\n", c.Kind, c.Path, shortOID(c.BaseOID), shortOID(c.LocalOID), shortOID(c.RemoteOID))
	}
	fmt.Println("Resolve with `gamedepot resolve <path> --remote` or `gamedepot resolve <path> --local`.")
	return nil
}

func GetConflicts(ctx context.Context, start string) (ConflictState, error) {
	_ = ctx
	root, err := workspaceRoot(start)
	if err != nil {
		return ConflictState{}, err
	}
	return loadConflictState(root)
}

func ResolveConflict(ctx context.Context, start, assetPath, decision string) error {
	decision = normalizeDecision(decision)
	if decision == "abort" {
		a, err := app.Load(ctx, start)
		if err != nil {
			return err
		}
		return clearConflicts(a.Root)
	}
	if decision != "remote" && decision != "local" {
		return fmt.Errorf("decision must be remote or local")
	}
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}
	assetPath, err = workspace.CleanRelPath(assetPath)
	if err != nil {
		return err
	}
	st, err := loadConflictState(a.Root)
	if err != nil {
		return err
	}
	if !st.Active || len(st.Conflicts) == 0 {
		return fmt.Errorf("no active GameDepot conflicts")
	}
	idx := -1
	var rec ConflictRecord
	for i, c := range st.Conflicts {
		if c.Path == assetPath {
			idx = i
			rec = c
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("conflict not found for %s", assetPath)
	}
	if decision == "remote" {
		if err := resolveUseRemote(ctx, a, rec); err != nil {
			return err
		}
		fmt.Printf("resolved %s using remote version\n", rec.Path)
	} else {
		if err := resolveKeepLocalAndPublish(ctx, a, rec); err != nil {
			return err
		}
		fmt.Printf("resolved %s by keeping local version and publishing it\n", rec.Path)
	}
	st.Conflicts = append(st.Conflicts[:idx], st.Conflicts[idx+1:]...)
	if len(st.Conflicts) == 0 {
		return clearConflicts(a.Root)
	}
	return saveConflictState(a.Root, st)
}

func normalizeDecision(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, " ", "_")
	switch s {
	case "use_remote", "theirs":
		return "remote"
	case "keep_local", "keep_local_and_publish", "ours":
		return "local"
	case "abort", "cancel":
		return "abort"
	default:
		return s
	}
}

func resolveUseRemote(ctx context.Context, a *app.App, rec ConflictRecord) error {
	idx, err := localindex.Load(a.Root)
	if err != nil {
		return err
	}
	if rec.RemoteOID == "" {
		dst, err := workspace.SafeJoin(a.Root, rec.Path)
		if err != nil {
			return err
		}
		if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
			return err
		}
		idx.Delete(rec.Path)
		return localindex.Save(a.Root, idx)
	}
	sha := gdrefs.SHAFromOID(rec.RemoteOID)
	dst, err := workspace.SafeJoin(a.Root, rec.Path)
	if err != nil {
		return err
	}
	if err := downloadBlobToFile(ctx, a.Store, sha, dst); err != nil {
		return err
	}
	actual, err := blob.SHA256File(dst)
	if err != nil {
		return err
	}
	if actual != sha {
		return fmt.Errorf("sha256 mismatch after download: %s", rec.Path)
	}
	idx.SetBase(rec.Path, gdrefs.EnsureOID(rec.RemoteOID))
	return localindex.Save(a.Root, idx)
}

func resolveKeepLocalAndPublish(ctx context.Context, a *app.App, rec ConflictRecord) error {
	idx, err := localindex.Load(a.Root)
	if err != nil {
		return err
	}
	refStore := gdrefs.NewStore(a.Root)
	g := gdgit.New(a.Root)
	localFiles, err := workspace.ScanManaged(a.Root, a.Config)
	if err != nil {
		return err
	}
	var file workspace.FileInfo
	hasLocal := false
	for _, f := range localFiles {
		if f.Path == rec.Path {
			file = f
			hasLocal = true
			break
		}
	}
	if hasLocal {
		if err := submitplan.UploadBlob(ctx, a, file); err != nil {
			return err
		}
		ref := gdrefs.NewAssetRef(rec.Path, file.SHA256, file.Size, file.Kind)
		if err := refStore.Save(ref); err != nil {
			return err
		}
		idx.SetBase(rec.Path, ref.OID)
	} else {
		if err := refStore.Delete(rec.Path); err != nil {
			return err
		}
		idx.Delete(rec.Path)
	}
	if err := localindex.Save(a.Root, idx); err != nil {
		return err
	}
	if err := g.AddA("depot/refs"); err != nil {
		return err
	}
	staged, err := g.StagedNames()
	if err != nil {
		return err
	}
	if strings.TrimSpace(staged) == "" {
		return nil
	}
	msg := fmt.Sprintf("Resolve update conflict: keep local %s", rec.Path)
	if err := g.Commit(msg); err != nil {
		return err
	}
	return Push(ctx, a.Root)
}

func workspaceRoot(start string) (string, error) {
	a, err := app.Load(context.Background(), start)
	if err != nil {
		return "", err
	}
	return a.Root, nil
}
