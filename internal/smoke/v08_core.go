package smoke

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/1051093860/gamedepot/internal/app"
	"github.com/1051093860/gamedepot/internal/commands"
	gdgit "github.com/1051093860/gamedepot/internal/git"
	"github.com/1051093860/gamedepot/internal/historyindex"
	"github.com/1051093860/gamedepot/internal/manifest"
	"github.com/1051093860/gamedepot/internal/rules"
)

type V08CoreOptions struct {
	Workspace string
	Clean     bool
}

func RegisterV08CoreFlags(fs interface {
	StringVar(*string, string, string, string)
	BoolVar(*bool, string, bool, string)
}, opts *V08CoreOptions) {
	fs.StringVar(&opts.Workspace, "workspace", filepath.Join(os.TempDir(), "gamedepot-v08-core-smoke"), "smoke workspace")
	fs.BoolVar(&opts.Clean, "clean", true, "remove the smoke workspace before running")
}

func RunV08Core(ctx context.Context, opts V08CoreOptions) error {
	if opts.Workspace == "" {
		opts.Workspace = filepath.Join(os.TempDir(), "gamedepot-v08-core-smoke")
	}
	if opts.Clean {
		_ = os.RemoveAll(opts.Workspace)
	}
	root := filepath.Join(opts.Workspace, "UEProject")
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	if err := runIn(root, "git", "init"); err != nil {
		return err
	}
	_ = runIn(root, "git", "config", "user.email", "v08-smoke@example.com")
	_ = runIn(root, "git", "config", "user.name", "V08 Smoke")
	_ = runIn(root, "git", "config", "core.autocrlf", "false")
	_ = runIn(root, "git", "config", "core.eol", "lf")
	if err := commands.InitWithTemplate(root, "v08-smoke", "ue5"); err != nil {
		return err
	}
	writeFile(root, "Content/Hero.uasset", "hero-v1")
	writeFile(root, "Config/DefaultGame.ini", "[/Script/Game]\nName=Smoke\n")
	writeFile(root, "Content/Data/Table.json", "{\"version\":1}\n")
	if err := commands.SubmitWithOptions(ctx, root, "v08 initial git/blob routes", commands.SubmitOptions{}); err != nil {
		return fmt.Errorf("initial submit: %w", err)
	}
	g := gdgit.New(root)
	commitA, _ := g.CurrentCommit()
	m := mustManifest(root)
	assertStorage(m, "Content/Hero.uasset", manifest.StorageBlob)
	assertStorage(m, "Content/Data/Table.json", manifest.StorageGit)
	if tracked, _ := g.IsTracked("Content/Hero.uasset"); tracked {
		return fmt.Errorf("Hero.uasset should be blob-routed and not Git tracked")
	}
	if tracked, _ := g.IsTracked("Content/Data/Table.json"); !tracked {
		return fmt.Errorf("Table.json should be Git tracked in commit A")
	}

	writeFile(root, "Content/Data/Table.json", "{\"version\":2}\n")
	if _, err := commands.RulesSet(ctx, root, commands.RuleSetOptions{Paths: []string{"Content/Data/Table.json"}, Mode: rules.ModeBlob, Scope: commands.RuleScopeExact}, false); err != nil {
		return err
	}
	if err := commands.SubmitWithOptions(ctx, root, "v08 table git to blob", commands.SubmitOptions{}); err != nil {
		return fmt.Errorf("git->blob submit: %w", err)
	}
	commitB, _ := g.CurrentCommit()
	m = mustManifest(root)
	assertStorage(m, "Content/Data/Table.json", manifest.StorageBlob)
	if tracked, _ := g.IsTracked("Content/Data/Table.json"); tracked {
		return fmt.Errorf("Table.json should not be Git tracked in blob commit")
	}

	writeFile(root, "Content/Data/Table.json", "{\"version\":3}\n")
	if _, err := commands.RulesSet(ctx, root, commands.RuleSetOptions{Paths: []string{"Content/Data/Table.json"}, Mode: rules.ModeGit, Scope: commands.RuleScopeExact}, false); err != nil {
		return err
	}
	if err := commands.SubmitWithOptions(ctx, root, "v08 table blob to git", commands.SubmitOptions{}); err != nil {
		return fmt.Errorf("blob->git submit: %w", err)
	}
	m = mustManifest(root)
	assertStorage(m, "Content/Data/Table.json", manifest.StorageGit)
	if tracked, _ := g.IsTracked("Content/Data/Table.json"); !tracked {
		return fmt.Errorf("Table.json should be Git tracked again")
	}

	idx, err := historyindex.Build(g, "depot/manifests/main.gdmanifest.json")
	if err != nil {
		return err
	}
	items := idx.ForPath("Content/Data/Table.json")
	seenGit, seenBlob := false, false
	for _, it := range items {
		if it.Storage == manifest.StorageGit {
			seenGit = true
		}
		if it.Storage == manifest.StorageBlob {
			seenBlob = true
		}
	}
	if !seenGit || !seenBlob {
		return fmt.Errorf("history should include both git and blob versions for table: git=%v blob=%v", seenGit, seenBlob)
	}

	if err := commands.RestoreVersion(ctx, root, "Content/Data/Table.json", commitA, true); err != nil {
		return fmt.Errorf("restore git version: %w", err)
	}
	if got := normalizeSmokeText(readFile(root, "Content/Data/Table.json")); got != "{\"version\":1}\n" {
		return fmt.Errorf("restore commit A got %q", got)
	}
	if err := commands.RestoreVersion(ctx, root, "Content/Data/Table.json", commitB, true); err != nil {
		return fmt.Errorf("restore blob version: %w", err)
	}
	if got := normalizeSmokeText(readFile(root, "Content/Data/Table.json")); got != "{\"version\":2}\n" {
		return fmt.Errorf("restore commit B got %q", got)
	}
	if err := commands.RevertAssets(ctx, root, []string{"Content/Data/Table.json"}, true); err != nil {
		return fmt.Errorf("revert current git file: %w", err)
	}
	if got := normalizeSmokeText(readFile(root, "Content/Data/Table.json")); got != "{\"version\":3}\n" {
		return fmt.Errorf("revert got %q", got)
	}

	// Status smoke: unsubmitted asset must still get a next-submit route.
	writeFile(root, "Content/NewLocal.uasset", "new")
	a, err := app.Load(ctx, root)
	if err != nil {
		return err
	}
	statuses, err := commands.ComputeAssetStatuses(ctx, a, "Content/NewLocal.uasset", false)
	if err != nil {
		return err
	}
	if len(statuses) == 0 || statuses[0].Status != "new" || statuses[0].DesiredMode != "blob" {
		return fmt.Errorf("new uasset status not routed to blob: %+v", statuses)
	}

	fmt.Println("V08 core smoke PASS")
	fmt.Println("Workspace:", root)
	return nil
}

func runIn(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %v failed: %w\n%s", name, args, err, string(out))
	}
	return nil
}
func writeFile(root, rel, body string) {
	p := filepath.Join(root, filepath.FromSlash(rel))
	_ = os.MkdirAll(filepath.Dir(p), 0755)
	_ = os.WriteFile(p, []byte(body), 0644)
}
func readFile(root, rel string) string {
	b, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	return string(b)
}
func normalizeSmokeText(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}
func mustManifest(root string) manifest.Manifest {
	m, err := manifest.Load(filepath.Join(root, "depot/manifests/main.gdmanifest.json"))
	if err != nil {
		panic(err)
	}
	return m
}
func assertStorage(m manifest.Manifest, path string, want manifest.Storage) {
	e, ok := m.Get(path)
	if !ok || e.Storage != want {
		panic(fmt.Sprintf("%s storage got %+v ok=%v want %s", path, e, ok, want))
	}
}
