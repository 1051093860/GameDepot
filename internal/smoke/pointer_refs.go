package smoke

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/1051093860/gamedepot/internal/commands"
	"github.com/1051093860/gamedepot/internal/config"
	gdgit "github.com/1051093860/gamedepot/internal/git"
	"github.com/1051093860/gamedepot/internal/historyindex"
)

type PointerRefsOptions struct {
	Workspace string
	Clean     bool
	Keep      bool
}

func RegisterPointerRefsFlags(fs interface {
	StringVar(*string, string, string, string)
	BoolVar(*bool, string, bool, string)
}, opts *PointerRefsOptions) {
	fs.StringVar(&opts.Workspace, "workspace", filepath.Join(os.TempDir(), "gamedepot-pointer-refs-smoke"), "smoke workspace")
	fs.BoolVar(&opts.Clean, "clean", true, "remove the smoke workspace before running")
	fs.BoolVar(&opts.Keep, "keep", true, "keep the generated workspace for inspection")
}

// RunPointerRefs validates the v0.11 pointer-refs workflow end to end:
// init -> publish -> clone/update -> concurrent edits on different assets -> publish
// plus a same-asset conflict check. It uses only local Git and local object storage.
func RunPointerRefs(ctx context.Context, opts PointerRefsOptions) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git is required for smoke test: %w", err)
	}
	if strings.TrimSpace(opts.Workspace) == "" {
		opts.Workspace = filepath.Join(os.TempDir(), "gamedepot-pointer-refs-smoke")
	}
	workspace, err := filepath.Abs(opts.Workspace)
	if err != nil {
		return err
	}
	if opts.Clean {
		_ = os.RemoveAll(workspace)
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return err
	}
	if !opts.Keep {
		defer os.RemoveAll(workspace)
	}

	oldConfigDir, hadConfigDir := os.LookupEnv(config.EnvConfigDir)
	globalConfigDir := filepath.Join(workspace, "global-config")
	if err := os.Setenv(config.EnvConfigDir, globalConfigDir); err != nil {
		return err
	}
	defer func() {
		if hadConfigDir {
			_ = os.Setenv(config.EnvConfigDir, oldConfigDir)
		} else {
			_ = os.Unsetenv(config.EnvConfigDir)
		}
	}()

	sharedStore := filepath.Join(workspace, "shared-blob-store")
	if err := config.SaveGlobalConfig(config.GlobalConfig{
		DefaultProfile: "local",
		User:           config.GlobalUser{Name: "Smoke", Email: "smoke@example.com"},
		Profiles: map[string]config.StoreProfile{
			"local": {Type: "local", Path: sharedStore},
		},
	}); err != nil {
		return err
	}

	remote := filepath.Join(workspace, "remote.git")
	if err := runGitSmoke(workspace, "init", "--bare", remote); err != nil {
		return err
	}

	devA := filepath.Join(workspace, "DevA")
	if err := createUEProject(devA, "SmokeProject"); err != nil {
		return err
	}
	if err := runGitSmoke(devA, "init"); err != nil {
		return err
	}
	if err := configureGitUser(devA, "Dev A"); err != nil {
		return err
	}
	if err := runGitSmoke(devA, "remote", "add", "origin", remote); err != nil {
		return err
	}
	if err := commands.InitUEExisting(devA, "SmokeProject"); err != nil {
		return err
	}

	writeSmokeFile(devA, "Content/Asset1.uasset", "asset1-v1")
	writeSmokeFile(devA, "Content/Asset2.uasset", "asset2-v1")
	writeSmokeFile(devA, "Config/DefaultGame.ini", "[/Script/Smoke]\nName=Initial\n")
	if err := commands.Publish(ctx, devA, "initial pointer refs", commands.PublishOptions{}); err != nil {
		return fmt.Errorf("DevA initial publish: %w", err)
	}
	if err := assertGitTracked(devA, "depot/refs/Content/Asset1.uasset.gdref", true); err != nil {
		return err
	}
	if err := assertGitTracked(devA, "Content/Asset1.uasset", false); err != nil {
		return err
	}
	idx, err := historyindex.Build(gdgit.New(devA), "depot/refs")
	if err != nil {
		return fmt.Errorf("build pointer-ref history index: %w", err)
	}
	historyItems := idx.ForPath("Content/Asset1.uasset")
	if len(historyItems) == 0 {
		return fmt.Errorf("pointer-ref history for Content/Asset1.uasset is empty")
	}
	fastIdx, err := historyindex.BuildForPath(gdgit.New(devA), "depot/refs", "Content/Asset1.uasset")
	if err != nil {
		return fmt.Errorf("build fast pointer-ref history index: %w", err)
	}
	fastHistoryItems := fastIdx.ForPath("Content/Asset1.uasset")
	if len(fastHistoryItems) != len(historyItems) || fastHistoryItems[0].SHA256 != historyItems[0].SHA256 {
		return fmt.Errorf("fast pointer-ref history mismatch: slow=%d fast=%d", len(historyItems), len(fastHistoryItems))
	}
	writeSmokeFile(devA, "Content/Asset1.uasset", "corrupted local before restore")
	if err := commands.RestoreVersion(ctx, devA, "Content/Asset1.uasset", historyItems[0].Commit, true); err != nil {
		return fmt.Errorf("restore pointer-ref history version: %w", err)
	}
	if err := assertSmokeFile(devA, "Content/Asset1.uasset", "asset1-v1"); err != nil {
		return fmt.Errorf("pointer-ref history restore produced wrong content: %w", err)
	}

	devB := filepath.Join(workspace, "DevB")
	// Simulate a common Windows setup where Git global core.autocrlf=true during clone.
	// .gitattributes must keep .gdref files LF-clean, otherwise update/pull sees
	// pointer refs as locally modified and refuses to fast-forward.
	if err := runGitSmoke(workspace, "clone", "-c", "core.autocrlf=true", remote, devB); err != nil {
		return err
	}
	if err := configureGitUser(devB, "Dev B"); err != nil {
		return err
	}
	if err := commands.Update(ctx, devB, commands.UpdateOptions{}); err != nil {
		return fmt.Errorf("DevB initial update: %w", err)
	}
	if err := assertSmokeFile(devB, "Content/Asset1.uasset", "asset1-v1"); err != nil {
		return err
	}
	if err := assertSmokeFile(devB, "Content/Asset2.uasset", "asset2-v1"); err != nil {
		return err
	}

	writeSmokeFile(devA, "Content/Asset1.uasset", "asset1-v2-from-A")
	if err := commands.Publish(ctx, devA, "A updates asset1", commands.PublishOptions{}); err != nil {
		return fmt.Errorf("DevA publish asset1: %w", err)
	}

	writeSmokeFile(devB, "Content/Asset2.uasset", "asset2-v2-from-B-local")
	if err := commands.Update(ctx, devB, commands.UpdateOptions{}); err != nil {
		return fmt.Errorf("DevB update with local asset2 edit should succeed: %w", err)
	}
	if err := assertSmokeFile(devB, "Content/Asset1.uasset", "asset1-v2-from-A"); err != nil {
		return err
	}
	if err := assertSmokeFile(devB, "Content/Asset2.uasset", "asset2-v2-from-B-local"); err != nil {
		return fmt.Errorf("local-only Asset2 edit was not preserved: %w", err)
	}
	if err := commands.Publish(ctx, devB, "B updates asset2", commands.PublishOptions{}); err != nil {
		return fmt.Errorf("DevB publish asset2 without rolling back asset1: %w", err)
	}

	devC := filepath.Join(workspace, "DevC")
	if err := runGitSmoke(workspace, "clone", "-c", "core.autocrlf=true", remote, devC); err != nil {
		return err
	}
	if err := configureGitUser(devC, "Dev C"); err != nil {
		return err
	}
	if err := commands.Update(ctx, devC, commands.UpdateOptions{}); err != nil {
		return fmt.Errorf("DevC update final state: %w", err)
	}
	if err := assertSmokeFile(devC, "Content/Asset1.uasset", "asset1-v2-from-A"); err != nil {
		return err
	}
	if err := assertSmokeFile(devC, "Content/Asset2.uasset", "asset2-v2-from-B-local"); err != nil {
		return err
	}

	// Publish must also be safe when the remote branch has advanced and the
	// user did not run update first. B edits Asset1 while A publishes an
	// unrelated Asset2 change; B publish should pre-update, preserve B's local
	// Asset1 edit, include A's Asset2 ref, and push a linear commit.
	if err := commands.Update(ctx, devA, commands.UpdateOptions{}); err != nil {
		return fmt.Errorf("DevA update before direct publish smoke: %w", err)
	}
	writeSmokeFile(devA, "Content/Asset2.uasset", "asset2-v3-from-A-before-B-direct-publish")
	if err := commands.Publish(ctx, devA, "A updates asset2 before B direct publish", commands.PublishOptions{}); err != nil {
		return fmt.Errorf("DevA publish asset2 before B direct publish: %w", err)
	}
	writeSmokeFile(devB, "Content/Asset1.uasset", "asset1-v3-from-B-direct-publish")
	if err := commands.Publish(ctx, devB, "B directly publishes asset1 while remote advanced", commands.PublishOptions{}); err != nil {
		return fmt.Errorf("DevB direct publish should absorb remote Asset2: %w", err)
	}
	if err := commands.Update(ctx, devC, commands.UpdateOptions{}); err != nil {
		return fmt.Errorf("DevC update after B direct publish: %w", err)
	}
	if err := assertSmokeFile(devC, "Content/Asset1.uasset", "asset1-v3-from-B-direct-publish"); err != nil {
		return fmt.Errorf("direct publish did not preserve B Asset1: %w", err)
	}
	if err := assertSmokeFile(devC, "Content/Asset2.uasset", "asset2-v3-from-A-before-B-direct-publish"); err != nil {
		return fmt.Errorf("direct publish rolled back A Asset2: %w", err)
	}

	// Same-asset conflict: B has a local edit based on asset1-v2; A first
	// updates unrelated Asset2 from B, then publishes asset1-v3.
	writeSmokeFile(devB, "Content/Asset1.uasset", "asset1-local-conflict-from-B")
	if err := commands.Update(ctx, devA, commands.UpdateOptions{}); err != nil {
		return fmt.Errorf("DevA update before asset1 v3: %w", err)
	}
	writeSmokeFile(devA, "Content/Asset1.uasset", "asset1-v3-from-A")
	if err := commands.Publish(ctx, devA, "A updates asset1 again", commands.PublishOptions{}); err != nil {
		return fmt.Errorf("DevA publish asset1 v3: %w", err)
	}
	if err := commands.Update(ctx, devB, commands.UpdateOptions{Strict: true}); err == nil {
		return fmt.Errorf("DevB strict update should fail while local Asset1 edit exists")
	}
	if err := commands.Update(ctx, devB, commands.UpdateOptions{}); err == nil {
		return fmt.Errorf("DevB update should be blocked by same-asset conflict")
	}
	if err := assertSmokeFile(devB, "Content/Asset1.uasset", "asset1-local-conflict-from-B"); err != nil {
		return fmt.Errorf("conflicting local Asset1 edit was overwritten: %w", err)
	}
	if st, err := commands.GetConflicts(ctx, devB); err != nil || !st.Active || len(st.Conflicts) != 1 {
		return fmt.Errorf("expected one active update conflict, got state=%+v err=%v", st, err)
	}
	if err := commands.ResolveConflict(ctx, devB, "Content/Asset1.uasset", "remote"); err != nil {
		return fmt.Errorf("DevB resolve conflict using remote: %w", err)
	}
	if err := assertSmokeFile(devB, "Content/Asset1.uasset", "asset1-v3-from-A"); err != nil {
		return fmt.Errorf("remote conflict resolution did not materialize remote Asset1: %w", err)
	}
	if st, err := commands.GetConflicts(ctx, devB); err != nil || st.Active || len(st.Conflicts) != 0 {
		return fmt.Errorf("expected conflicts to be cleared after remote resolution, got state=%+v err=%v", st, err)
	}

	// Same-asset conflict, but this time B explicitly keeps the local version.
	// Resolving local must upload the blob, create a new pointer-ref commit, and push it.
	writeSmokeFile(devB, "Content/Asset1.uasset", "asset1-v4-keep-local-from-B")
	writeSmokeFile(devA, "Content/Asset1.uasset", "asset1-v5-from-A")
	if err := commands.Publish(ctx, devA, "A updates asset1 to v5", commands.PublishOptions{}); err != nil {
		return fmt.Errorf("DevA publish asset1 v5: %w", err)
	}
	if err := commands.Update(ctx, devB, commands.UpdateOptions{}); err == nil {
		return fmt.Errorf("DevB update should be blocked by second same-asset conflict")
	}
	if err := commands.ResolveConflict(ctx, devB, "Content/Asset1.uasset", "local"); err != nil {
		return fmt.Errorf("DevB resolve conflict by keeping local and publishing: %w", err)
	}
	if err := assertSmokeFile(devB, "Content/Asset1.uasset", "asset1-v4-keep-local-from-B"); err != nil {
		return fmt.Errorf("local conflict resolution changed B Asset1 unexpectedly: %w", err)
	}
	if err := commands.Update(ctx, devC, commands.UpdateOptions{}); err != nil {
		return fmt.Errorf("DevC update after B keep-local resolution: %w", err)
	}
	if err := assertSmokeFile(devC, "Content/Asset1.uasset", "asset1-v4-keep-local-from-B"); err != nil {
		return fmt.Errorf("keep-local conflict resolution was not pushed to remote: %w", err)
	}

	// Deletion handling: clean remote deletion should remove local files and
	// both-sides deletion must not leave phantom entries in local-index.
	if err := commands.Update(ctx, devA, commands.UpdateOptions{}); err != nil {
		return fmt.Errorf("DevA update before deletion smoke: %w", err)
	}
	writeSmokeFile(devA, "Content/RemoteDelete.uasset", "remote-delete-v1")
	writeSmokeFile(devA, "Content/BothDelete.uasset", "both-delete-v1")
	writeSmokeFile(devA, "Content/DeleteConflict.uasset", "delete-conflict-v1")
	writeSmokeFile(devA, "Content/DeleteVsModify.uasset", "delete-vs-modify-v1")
	if err := commands.Publish(ctx, devA, "A adds deletion smoke assets", commands.PublishOptions{}); err != nil {
		return fmt.Errorf("DevA publish deletion smoke assets: %w", err)
	}
	if err := commands.Update(ctx, devB, commands.UpdateOptions{}); err != nil {
		return fmt.Errorf("DevB update deletion smoke assets: %w", err)
	}
	if err := commands.Update(ctx, devC, commands.UpdateOptions{}); err != nil {
		return fmt.Errorf("DevC update deletion smoke assets: %w", err)
	}

	if err := os.Remove(filepath.Join(devA, "Content", "RemoteDelete.uasset")); err != nil {
		return err
	}
	if err := commands.Publish(ctx, devA, "A deletes RemoteDelete", commands.PublishOptions{}); err != nil {
		return fmt.Errorf("DevA publish RemoteDelete deletion: %w", err)
	}
	if err := commands.Update(ctx, devB, commands.UpdateOptions{}); err != nil {
		return fmt.Errorf("DevB update clean remote deletion: %w", err)
	}
	if err := assertMissingSmokeFile(devB, "Content/RemoteDelete.uasset"); err != nil {
		return fmt.Errorf("clean remote deletion was not materialized: %w", err)
	}

	if err := os.Remove(filepath.Join(devB, "Content", "BothDelete.uasset")); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(devA, "Content", "BothDelete.uasset")); err != nil {
		return err
	}
	if err := commands.Publish(ctx, devA, "A deletes BothDelete", commands.PublishOptions{}); err != nil {
		return fmt.Errorf("DevA publish BothDelete deletion: %w", err)
	}
	if err := commands.Update(ctx, devB, commands.UpdateOptions{}); err != nil {
		return fmt.Errorf("DevB update both-sides deletion: %w", err)
	}
	if err := assertMissingSmokeFile(devB, "Content/BothDelete.uasset"); err != nil {
		return fmt.Errorf("both-sides deletion left a file behind: %w", err)
	}
	if err := commands.Update(ctx, devB, commands.UpdateOptions{Strict: true}); err != nil {
		return fmt.Errorf("DevB strict update after both-sides deletion should be clean: %w", err)
	}

	writeSmokeFile(devB, "Content/LocalScratch.uasset", "scratch should be removed by force update")
	if err := commands.Update(ctx, devB, commands.UpdateOptions{Force: true}); err != nil {
		return fmt.Errorf("DevB force update with local untracked Content: %w", err)
	}
	if err := assertMissingSmokeFile(devB, "Content/LocalScratch.uasset"); err != nil {
		return fmt.Errorf("force update did not remove local untracked Content: %w", err)
	}

	writeSmokeFile(devB, "Content/DeleteConflict.uasset", "delete-conflict-local-edit-from-B")
	if err := os.Remove(filepath.Join(devA, "Content", "DeleteConflict.uasset")); err != nil {
		return err
	}
	if err := commands.Publish(ctx, devA, "A deletes DeleteConflict", commands.PublishOptions{}); err != nil {
		return fmt.Errorf("DevA publish DeleteConflict deletion: %w", err)
	}
	if err := commands.Update(ctx, devB, commands.UpdateOptions{}); err == nil {
		return fmt.Errorf("DevB update should conflict on remote deletion vs local modification")
	}
	if err := commands.ResolveConflict(ctx, devB, "Content/DeleteConflict.uasset", "remote"); err != nil {
		return fmt.Errorf("DevB resolve remote deletion: %w", err)
	}
	if err := assertMissingSmokeFile(devB, "Content/DeleteConflict.uasset"); err != nil {
		return fmt.Errorf("remote deletion resolution did not remove local file: %w", err)
	}

	if err := os.Remove(filepath.Join(devB, "Content", "DeleteVsModify.uasset")); err != nil {
		return err
	}
	writeSmokeFile(devA, "Content/DeleteVsModify.uasset", "delete-vs-modify-v2-from-A")
	if err := commands.Publish(ctx, devA, "A modifies DeleteVsModify", commands.PublishOptions{}); err != nil {
		return fmt.Errorf("DevA publish DeleteVsModify modification: %w", err)
	}
	if err := commands.Update(ctx, devB, commands.UpdateOptions{}); err == nil {
		return fmt.Errorf("DevB update should conflict on local deletion vs remote modification")
	}
	if err := commands.ResolveConflict(ctx, devB, "Content/DeleteVsModify.uasset", "remote"); err != nil {
		return fmt.Errorf("DevB resolve remote modification over local deletion: %w", err)
	}
	if err := assertSmokeFile(devB, "Content/DeleteVsModify.uasset", "delete-vs-modify-v2-from-A"); err != nil {
		return fmt.Errorf("remote modification resolution did not restore file: %w", err)
	}

	fmt.Println("Pointer refs smoke PASS")
	fmt.Println("Workspace:", workspace)
	fmt.Println("Shared blob store:", sharedStore)
	return nil
}

func createUEProject(root, name string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	writeSmokeFile(root, name+".uproject", "{}")
	return nil
}

func configureGitUser(dir, name string) error {
	if err := runGitSmoke(dir, "config", "user.email", strings.ToLower(strings.ReplaceAll(name, " ", ""))+"@example.com"); err != nil {
		return err
	}
	if err := runGitSmoke(dir, "config", "user.name", name); err != nil {
		return err
	}
	if err := runGitSmoke(dir, "config", "core.autocrlf", "false"); err != nil {
		return err
	}
	return runGitSmoke(dir, "config", "core.eol", "lf")
}

func runGitSmoke(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %v in %s failed: %w\n%s", args, dir, err, string(out))
	}
	return nil
}

func writeSmokeFile(root, rel, body string) {
	p := filepath.Join(root, filepath.FromSlash(rel))
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(body), 0o644)
}

func assertSmokeFile(root, rel, want string) error {
	p := filepath.Join(root, filepath.FromSlash(rel))
	data, err := os.ReadFile(p)
	if err != nil {
		return fmt.Errorf("read %s: %w", rel, err)
	}
	got := string(data)
	if got != want {
		return fmt.Errorf("%s got %q want %q", rel, got, want)
	}
	return nil
}

func assertMissingSmokeFile(root, rel string) error {
	p := filepath.Join(root, filepath.FromSlash(rel))
	if _, err := os.Stat(p); err == nil {
		return fmt.Errorf("%s still exists", rel)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", rel, err)
	}
	return nil
}

func assertGitTracked(root, rel string, want bool) error {
	cmd := exec.Command("git", "ls-files", "--", rel)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git ls-files %s failed: %w\n%s", rel, err, string(out))
	}
	got := strings.TrimSpace(string(out)) == rel
	if got != want {
		return fmt.Errorf("git tracked state for %s got %v want %v", rel, got, want)
	}
	return nil
}
