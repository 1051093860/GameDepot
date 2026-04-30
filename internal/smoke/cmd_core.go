package smoke

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/1051093860/gamedepot/internal/config"
)

type CmdCoreOptions struct {
	Workspace string
	Report    string
	Clean     bool
	Keep      bool
	ProjectID string
}

type CmdCoreReport struct {
	Version         string            `json:"version"`
	StartedAt       string            `json:"started_at"`
	FinishedAt      string            `json:"finished_at"`
	Passed          bool              `json:"passed"`
	Workspace       string            `json:"workspace"`
	ProjectRoot     string            `json:"project_root"`
	PeerRoot        string            `json:"peer_root"`
	OriginRemote    string            `json:"origin_remote"`
	UpstreamRemote  string            `json:"upstream_remote"`
	SharedBlobStore string            `json:"shared_blob_store"`
	ReportPath      string            `json:"report_path"`
	Executable      string            `json:"executable"`
	GlobalConfigDir string            `json:"global_config_dir"`
	Hashes          map[string]string `json:"hashes"`
	Steps           []Result          `json:"steps"`
}

type cmdCoreRunner struct {
	opts            CmdCoreOptions
	exe             string
	workspace       string
	projectRoot     string
	peerRoot        string
	originRemote    string
	upstreamRemote  string
	sharedBlobStore string
	reportPath      string
	globalConfigDir string
	env             []string
	started         time.Time
	report          CmdCoreReport
	failed          bool
}

func RegisterCmdCoreFlags(fs *flag.FlagSet, opts *CmdCoreOptions) {
	fs.StringVar(&opts.Workspace, "workspace", "GameDepot_CmdCoreSmokeWorkspace", "workspace directory to create and test")
	fs.StringVar(&opts.Report, "report", "gamedepot_cmd_core_smoke_report.md", "report file path")
	fs.BoolVar(&opts.Clean, "clean", true, "remove the smoke workspace before running")
	fs.BoolVar(&opts.Keep, "keep", true, "keep the generated workspace for inspection")
	fs.StringVar(&opts.ProjectID, "project", "SimCmdCoreProject", "simulated project id/name")
}

func RunCmdCore(ctx context.Context, opts CmdCoreOptions) error {
	r, err := newCmdCoreRunner(opts)
	if err != nil {
		return err
	}
	return r.run(ctx)
}

func newCmdCoreRunner(opts CmdCoreOptions) (*cmdCoreRunner, error) {
	if strings.TrimSpace(opts.ProjectID) == "" {
		opts.ProjectID = "SimCmdCoreProject"
	}
	if strings.TrimSpace(opts.Workspace) == "" {
		opts.Workspace = "GameDepot_CmdCoreSmokeWorkspace"
	}
	if strings.TrimSpace(opts.Report) == "" {
		opts.Report = "gamedepot_cmd_core_smoke_report.md"
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return nil, err
	}
	workspace, err := filepath.Abs(opts.Workspace)
	if err != nil {
		return nil, err
	}
	reportPath, err := filepath.Abs(opts.Report)
	if err != nil {
		return nil, err
	}
	globalConfigDir := filepath.Join(workspace, "_global_config")
	env := append(os.Environ(), config.EnvConfigDir+"="+globalConfigDir)
	return &cmdCoreRunner{
		opts:            opts,
		exe:             exe,
		workspace:       workspace,
		projectRoot:     filepath.Join(workspace, opts.ProjectID),
		peerRoot:        filepath.Join(workspace, "PeerWork"),
		originRemote:    filepath.Join(workspace, "_git_remote", opts.ProjectID+".git"),
		upstreamRemote:  filepath.Join(workspace, "_git_remote", opts.ProjectID+"-upstream.git"),
		sharedBlobStore: filepath.Join(workspace, "_shared_blobs"),
		reportPath:      reportPath,
		globalConfigDir: globalConfigDir,
		env:             env,
		started:         time.Now(),
	}, nil
}

func (r *cmdCoreRunner) run(ctx context.Context) error {
	if r.opts.Clean {
		if err := os.RemoveAll(r.workspace); err != nil {
			return err
		}
	}
	for _, dir := range []string{r.projectRoot, filepath.Dir(r.originRemote), r.sharedBlobStore, filepath.Dir(r.reportPath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	r.report = CmdCoreReport{
		Version:         "v0.6.1-cmd-core-smoke",
		StartedAt:       r.started.UTC().Format(time.RFC3339),
		Workspace:       r.workspace,
		ProjectRoot:     r.projectRoot,
		PeerRoot:        r.peerRoot,
		OriginRemote:    r.originRemote,
		UpstreamRemote:  r.upstreamRemote,
		SharedBlobStore: r.sharedBlobStore,
		ReportPath:      r.reportPath,
		Executable:      r.exe,
		GlobalConfigDir: r.globalConfigDir,
		Hashes:          map[string]string{},
		Steps:           []Result{},
	}

	r.section("prepare git, shared local blob store, and remotes")
	r.stepCommand(ctx, "git version", r.workspace, false, "git", "--version")
	r.stepCommand(ctx, "create bare origin remote", filepath.Dir(r.originRemote), false, "git", "init", "--bare", r.originRemote)
	r.stepCommand(ctx, "create bare upstream remote", filepath.Dir(r.upstreamRemote), false, "git", "init", "--bare", r.upstreamRemote)
	r.stepCommand(ctx, "project git init", r.projectRoot, false, "git", "init")
	r.stepCommand(ctx, "project branch main", r.projectRoot, false, "git", "branch", "-M", "main")
	r.stepCommand(ctx, "project git config email", r.projectRoot, false, "git", "config", "user.email", "cmd-smoke@example.com")
	r.stepCommand(ctx, "project git config name", r.projectRoot, false, "git", "config", "user.name", "GameDepot Cmd Smoke")

	r.section("init GameDepot and configure Git remote/upstream through CLI")
	r.stepGD(ctx, r.projectRoot, "gamedepot init", false, "init", "--project", r.opts.ProjectID, "--template", "ue5")
	r.stepGD(ctx, r.projectRoot, "config path isolated", false, "config", "path")
	r.stepGD(ctx, r.projectRoot, "set global user", false, "config", "user", "--name", "Cmd Smoke", "--email", "cmd-smoke@example.com")
	r.stepGD(ctx, r.projectRoot, "add shared local store profile", false, "config", "add-local", "cmd-shared", "--path", r.sharedBlobStore)
	r.stepGD(ctx, r.projectRoot, "project use shared local store", false, "config", "project-use", "cmd-shared")
	r.stepGD(ctx, r.projectRoot, "set git remote origin", false, "git-config", "set-remote", "--name", "origin", "--url", r.originRemote)
	r.stepGD(ctx, r.projectRoot, "set git upstream", false, "git-config", "set-upstream", "--name", "upstream", "--url", r.upstreamRemote)
	r.stepGD(ctx, r.projectRoot, "git-config show", false, "git-config", "show")
	r.stepGD(ctx, r.projectRoot, "git-config test", false, "git-config", "test")
	r.stepGD(ctx, r.projectRoot, "doctor", false, "doctor")
	r.stepGD(ctx, r.projectRoot, "store info", false, "store", "info")
	r.stepGD(ctx, r.projectRoot, "store check", false, "store", "check")

	r.section("create simulated UE project files")
	if err := r.createFakeProjectFiles(r.projectRoot, "initial"); err != nil {
		r.addManual("create simulated UE files", false, "", err)
	} else {
		r.addManual("create simulated UE files", true, "created .umap/.uasset/.xlsx/.blend/.zip and Git-managed files", nil)
	}

	r.section("classify, lock, submit --push, verify, asset-status")
	r.stepGD(ctx, r.projectRoot, "classify all", false, "classify", "--all")
	r.stepGD(ctx, r.projectRoot, "classify json", false, "classify", "--json", "--all")
	r.stepGD(ctx, r.projectRoot, "status json before submit", false, "status", "--json")
	r.stepGD(ctx, r.projectRoot, "lock main map", false, "lock", "Content/Maps/Main.umap", "--owner", "CmdSmoke", "--host", "CmdHost", "--note", "cmd-core smoke")
	r.stepGD(ctx, r.projectRoot, "locks json", false, "locks", "--json")
	r.stepGD(ctx, r.projectRoot, "submit initial --push", false, "submit", "-m", "cmd smoke initial import", "--push")
	r.stepCommand(ctx, "set bare origin HEAD to main", r.workspace, false, "git", "--git-dir", r.originRemote, "symbolic-ref", "HEAD", "refs/heads/main")
	r.stepCommand(ctx, "tag initial milestone", r.projectRoot, false, "git", "tag", "cmd-smoke-initial")
	r.stepCommand(ctx, "push initial tag", r.projectRoot, false, "git", "-C", r.projectRoot, "push", "origin", "cmd-smoke-initial")
	r.stepGD(ctx, r.projectRoot, "verify remote only", false, "verify", "--remote-only")
	r.stepGD(ctx, r.projectRoot, "asset-status main map", false, "asset-status", "Content/Maps/Main.umap")
	r.stepGD(ctx, r.projectRoot, "asset-status content recursive json", false, "asset-status", "Content", "--recursive", "--json")

	r.section("restore and dirty overwrite protection")
	mainMap := filepath.Join(r.projectRoot, filepath.FromSlash("Content/Maps/Main.umap"))
	if err := os.Remove(mainMap); err != nil {
		r.addManual("remove main map before restore", false, "", err)
	} else {
		r.addManual("remove main map before restore", true, "removed local map", nil)
	}
	r.stepGD(ctx, r.projectRoot, "restore current main map", false, "restore", "Content/Maps/Main.umap")
	r.verifyHashAt("verify restored main map", r.projectRoot, "Content/Maps/Main.umap", "Content/Maps/Main.umap#initial")
	if err := os.WriteFile(mainMap, []byte("dirty local edit\n"), 0o644); err != nil {
		r.addManual("write dirty map", false, "", err)
	} else {
		r.addManual("write dirty map", true, "dirty edit created", nil)
	}
	r.stepGD(ctx, r.projectRoot, "restore refuses dirty map", true, "restore", "Content/Maps/Main.umap")
	r.stepGD(ctx, r.projectRoot, "restore dirty map --force", false, "restore", "Content/Maps/Main.umap", "--force")
	r.verifyHashAt("verify force-restored main map", r.projectRoot, "Content/Maps/Main.umap", "Content/Maps/Main.umap#initial")

	r.section("peer clone and sync from Git + shared blob store")
	r.stepCommand(ctx, "peer clone origin", r.workspace, false, "git", "clone", r.originRemote, r.peerRoot)
	r.stepCommand(ctx, "peer git config email", r.peerRoot, false, "git", "config", "user.email", "peer@example.com")
	r.stepCommand(ctx, "peer git config name", r.peerRoot, false, "git", "config", "user.name", "GameDepot Peer")
	for _, rel := range []string{"Content/Maps/Main.umap", "Content/Characters/Hero.uasset", "Content/Props/Crate.uasset"} {
		_ = os.Remove(filepath.Join(r.peerRoot, filepath.FromSlash(rel)))
	}
	r.addManual("peer removes local blob files", true, "peer clone now requires sync to restore blobs", nil)
	r.stepGD(ctx, r.peerRoot, "peer sync --force pulls and restores", false, "sync", "--force")
	r.verifyHashAt("peer verifies main map", r.peerRoot, "Content/Maps/Main.umap", "Content/Maps/Main.umap#initial")
	r.verifyHashAt("peer verifies hero asset", r.peerRoot, "Content/Characters/Hero.uasset", "Content/Characters/Hero.uasset#initial")

	r.section("peer edits blob, submit --push; project sync pulls remote manifest and blob")
	if err := r.writeBinary(r.peerRoot, "Content/Characters/Hero.uasset", 200*1024, 6002, "peer-hero"); err != nil {
		r.addManual("peer modifies hero", false, "", err)
	} else {
		r.addManual("peer modifies hero", true, "peer hero changed", nil)
	}
	r.stepGD(ctx, r.peerRoot, "peer submit hero --push", false, "submit", "-m", "peer updates hero", "--push")
	r.stepGD(ctx, r.projectRoot, "project sync --force pulls peer change", false, "sync", "--force")
	r.verifyHashAt("project verifies peer hero", r.projectRoot, "Content/Characters/Hero.uasset", "Content/Characters/Hero.uasset#peer-hero")
	r.stepGD(ctx, r.projectRoot, "asset-status hero after peer sync", false, "asset-status", "Content/Characters/Hero.uasset")
	r.stepGD(ctx, r.projectRoot, "sync --force --no-pull", false, "sync", "--force", "--no-pull")

	r.section("repair-current-blob for deleted current blob")
	peerHero := r.report.Hashes["Content/Characters/Hero.uasset#peer-hero"]
	if peerHero == "" {
		r.addManual("repair setup", false, "", fmt.Errorf("missing peer hero hash"))
	} else {
		r.stepGD(ctx, r.projectRoot, "delete current hero blob", false, "delete-version", "Content/Characters/Hero.uasset", "--sha256", peerHero, "--execute", "--force-current")
		r.stepGD(ctx, r.projectRoot, "asset-status current blob missing", false, "asset-status", "Content/Characters/Hero.uasset")
		r.stepGD(ctx, r.projectRoot, "repair current hero blob", false, "repair-current-blob", "Content/Characters/Hero.uasset")
		r.stepGD(ctx, r.projectRoot, "asset-status after repair", false, "asset-status", "Content/Characters/Hero.uasset")
	}

	r.section("history, gc-impact, gc dry-run")
	r.stepGD(ctx, r.projectRoot, "history hero", false, "history", "Content/Characters/Hero.uasset")
	r.stepGD(ctx, r.projectRoot, "gc dry-run", false, "gc", "--dry-run")
	r.stepGD(ctx, r.projectRoot, "gc-impact dry-run", false, "gc-impact", "--dry-run")
	r.stepGD(ctx, r.projectRoot, "gc-impact json", false, "gc-impact", "--dry-run", "--json")
	r.stepGD(ctx, r.projectRoot, "gc-impact protect-all-tags", false, "gc-impact", "--dry-run", "--protect-all-tags")
	r.stepGD(ctx, r.projectRoot, "unlock main map", false, "unlock", "Content/Maps/Main.umap", "--owner", "CmdSmoke", "--host", "CmdHost")
	r.stepGD(ctx, r.projectRoot, "locks after unlock", false, "locks")
	r.stepGD(ctx, r.projectRoot, "final verify", false, "verify")
	r.stepCommand(ctx, "final git status", r.projectRoot, false, "git", "status", "--short")

	r.report.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	r.report.Passed = !r.failed
	if err := r.writeReport(); err != nil {
		return err
	}

	fmt.Println("Cmd core smoke report written:", r.reportPath)
	fmt.Println("Cmd core smoke workspace:", r.workspace)
	if r.report.Passed {
		fmt.Println("Cmd core smoke test result: PASS")
	} else {
		fmt.Println("Cmd core smoke test result: FAIL")
	}
	if r.opts.Keep {
		fmt.Println("Workspace kept for inspection.")
	} else if !pathInside(r.reportPath, r.workspace) {
		if err := os.RemoveAll(r.workspace); err != nil {
			return err
		}
		fmt.Println("Workspace removed.")
	}
	if !r.report.Passed {
		return fmt.Errorf("cmd core smoke test failed; see %s", r.reportPath)
	}
	return nil
}

func (r *cmdCoreRunner) section(name string) {
	r.addManual("== "+name+" ==", true, "", nil)
}

func (r *cmdCoreRunner) stepGD(ctx context.Context, dir, name string, expectFailure bool, args ...string) {
	r.stepCommand(ctx, name, dir, expectFailure, r.exe, args...)
}

func (r *cmdCoreRunner) stepCommand(ctx context.Context, name, dir string, expectFailure bool, command string, args ...string) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = dir
	cmd.Env = r.env
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	exitOK := err == nil
	passed := exitOK != expectFailure
	res := Result{Name: name, Command: quoteCommand(command, args...), Dir: dir, ExpectFailure: expectFailure, ExitOK: exitOK, Passed: passed, DurationMS: time.Since(start).Milliseconds(), Output: trimLong(out.String(), 16_000)}
	if err != nil {
		res.Error = err.Error()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			res.Error = "timeout after 120s: " + res.Error
		}
	}
	if !passed {
		r.failed = true
	}
	r.report.Steps = append(r.report.Steps, res)
}

func (r *cmdCoreRunner) addManual(name string, passed bool, output string, err error) {
	res := Result{Name: name, ExitOK: err == nil, Passed: passed && err == nil, Output: output}
	if err != nil {
		res.Error = err.Error()
	}
	if !res.Passed {
		r.failed = true
	}
	r.report.Steps = append(r.report.Steps, res)
}

func (r *cmdCoreRunner) createFakeProjectFiles(root, label string) error {
	projectID := r.opts.ProjectID
	files := []struct{ rel, text string }{
		{projectID + ".uproject", `{"FileVersion":3,"EngineAssociation":"5.4","Category":"Games","Description":"GameDepot cmd smoke simulated UE project"}` + "\n"},
		{"Config/DefaultGame.ini", "[/Script/EngineSettings.GameMapsSettings]\nGameDefaultMap=/Game/Maps/Main\n"},
		{"Config/DefaultEngine.ini", "[/Script/Engine.Engine]\nGameViewportClientClassName=/Script/Engine.GameViewportClient\n"},
		{"Source/" + projectID + "/" + projectID + ".Build.cs", "using UnrealBuildTool;\npublic class " + projectID + " : ModuleRules { public " + projectID + "(ReadOnlyTargetRules Target) : base(Target) {} }\n"},
		{"Source/" + projectID + "/GameMode.cpp", "// simulated UE C++ file managed by Git\n"},
		{"Docs/CmdCoreSmoke.md", "# Cmd Core Smoke Test\n"},
		{"Saved/Logs/Smoke.log", "ignored runtime log\n"},
	}
	for _, f := range files {
		if err := writeText(filepath.Join(root, filepath.FromSlash(f.rel)), f.text); err != nil {
			return err
		}
	}
	bins := []struct {
		rel  string
		size int
		seed int64
	}{
		{"Content/Maps/Main.umap", 256 * 1024, 5001},
		{"Content/Characters/Hero.uasset", 160 * 1024, 5002},
		{"Content/Props/Crate.uasset", 96 * 1024, 5003},
		{"External/Planning/balance.xlsx", 40 * 1024, 5004},
		{"External/Art/source/Hero.blend", 128 * 1024, 5005},
		{"External/SharedTools/BlenderPortable.zip", 192 * 1024, 5006},
	}
	for _, f := range bins {
		if err := r.writeBinary(root, f.rel, f.size, f.seed, label); err != nil {
			return err
		}
	}
	return nil
}

func (r *cmdCoreRunner) writeBinary(root, rel string, size int, seed int64, label string) error {
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	buf := make([]byte, size)
	rng := rand.New(rand.NewSource(seed))
	if _, err := rng.Read(buf); err != nil {
		return err
	}
	copy(buf, []byte("GAMEDEPOT_CMD_CORE_SMOKE:"+label+":"+rel+"\n"))
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		return err
	}
	sum := sha256.Sum256(buf)
	r.report.Hashes[filepath.ToSlash(rel)+"#"+label] = hex.EncodeToString(sum[:])
	return nil
}

func (r *cmdCoreRunner) verifyHashAt(name, root, rel, key string) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		r.addManual(name, false, "", err)
		return
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	want := r.report.Hashes[key]
	out := fmt.Sprintf("%s\nwant: %s\ngot:  %s", rel, want, got)
	r.addManual(name, want != "" && want == got, out, nil)
}

func (r *cmdCoreRunner) writeReport() error {
	var b strings.Builder
	b.WriteString("# GameDepot Cmd Core Smoke Test Report\n\n")
	b.WriteString("```json\n")
	summary := map[string]any{"passed": r.report.Passed, "started_at": r.report.StartedAt, "finished_at": r.report.FinishedAt, "workspace": r.report.Workspace, "project_root": r.report.ProjectRoot, "peer_root": r.report.PeerRoot, "origin_remote": r.report.OriginRemote, "shared_blob_store": r.report.SharedBlobStore, "executable": r.report.Executable, "goos": runtime.GOOS, "goarch": runtime.GOARCH}
	data, _ := json.MarshalIndent(summary, "", "  ")
	b.Write(data)
	b.WriteString("\n```\n\n")
	keys := make([]string, 0, len(r.report.Hashes))
	for k := range r.report.Hashes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		b.WriteString("## Simulated binary hashes\n\n")
		for _, k := range keys {
			b.WriteString("- `" + k + "`: `" + r.report.Hashes[k] + "`\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## Steps\n\n")
	for i, step := range r.report.Steps {
		status := "PASS"
		if !step.Passed {
			status = "FAIL"
		}
		b.WriteString(fmt.Sprintf("### %02d. %s — %s\n\n", i+1, step.Name, status))
		if step.Command != "" {
			b.WriteString("Command:\n\n```text\n" + step.Command + "\n```\n\n")
		}
		if step.Dir != "" {
			b.WriteString("Dir: `" + step.Dir + "`\n\n")
		}
		if step.ExpectFailure {
			b.WriteString("Expected failure: `true`\n\n")
		}
		if step.Error != "" {
			b.WriteString("Error:\n\n```text\n" + step.Error + "\n```\n\n")
		}
		if step.Output != "" {
			b.WriteString("Output:\n\n```text\n" + step.Output + "\n```\n\n")
		}
	}
	b.WriteString("## Full JSON\n\n```json\n")
	full, _ := json.MarshalIndent(r.report, "", "  ")
	b.Write(full)
	b.WriteString("\n```\n")
	return os.WriteFile(r.reportPath, []byte(b.String()), 0o644)
}
