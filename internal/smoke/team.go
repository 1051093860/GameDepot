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
)

type TeamOptions struct {
	Workspace    string
	Report       string
	Clean        bool
	Keep         bool
	ProjectID    string
	StoreProfile string
	RunGC        bool
}

type TeamReport struct {
	Version      string            `json:"version"`
	StartedAt    string            `json:"started_at"`
	FinishedAt   string            `json:"finished_at"`
	Passed       bool              `json:"passed"`
	Workspace    string            `json:"workspace"`
	GitRemote    string            `json:"git_remote"`
	AliceRoot    string            `json:"alice_root"`
	BobRoot      string            `json:"bob_root"`
	ReportPath   string            `json:"report_path"`
	Executable   string            `json:"executable"`
	StoreProfile string            `json:"store_profile"`
	Hashes       map[string]string `json:"hashes"`
	Steps        []Result          `json:"steps"`
}

type teamRunner struct {
	opts       TeamOptions
	exe        string
	workspace  string
	reportPath string
	gitRemote  string
	aliceRoot  string
	bobRoot    string
	env        []string
	started    time.Time
	report     TeamReport
	failed     bool
}

func RegisterTeamFlags(fs *flag.FlagSet, opts *TeamOptions) {
	fs.StringVar(&opts.Workspace, "workspace", "GameDepot_TeamSmokeWorkspace", "workspace directory to create and test")
	fs.StringVar(&opts.Report, "report", "gamedepot_team_smoke_report.md", "report file path")
	fs.BoolVar(&opts.Clean, "clean", true, "remove the team smoke workspace before running")
	fs.BoolVar(&opts.Keep, "keep", true, "keep the generated workspace for inspection")
	fs.StringVar(&opts.ProjectID, "project", "", "simulated project id/name; empty creates a unique SimTeamProject timestamp")
	fs.StringVar(&opts.StoreProfile, "profile", "", "store profile shared by Alice and Bob; required")
	fs.BoolVar(&opts.RunGC, "gc", true, "run gc dry-run team smoke steps")
}

func RunTeam(ctx context.Context, opts TeamOptions) error {
	r, err := newTeamRunner(opts)
	if err != nil {
		return err
	}
	return r.run(ctx)
}

func newTeamRunner(opts TeamOptions) (*teamRunner, error) {
	if strings.TrimSpace(opts.ProjectID) == "" {
		opts.ProjectID = "SimTeamProject_" + time.Now().UTC().Format("20060102_150405")
	}
	if opts.Workspace == "" {
		opts.Workspace = "GameDepot_TeamSmokeWorkspace"
	}
	if opts.Report == "" {
		opts.Report = "gamedepot_team_smoke_report.md"
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
	return &teamRunner{
		opts:       opts,
		exe:        exe,
		workspace:  workspace,
		reportPath: reportPath,
		gitRemote:  filepath.Join(workspace, "_git_remote", opts.ProjectID+".git"),
		aliceRoot:  filepath.Join(workspace, "AliceWork"),
		bobRoot:    filepath.Join(workspace, "BobWork"),
		env:        os.Environ(),
		started:    time.Now(),
	}, nil
}

func (r *teamRunner) run(ctx context.Context) error {
	if r.opts.Clean {
		if err := os.RemoveAll(r.workspace); err != nil {
			return err
		}
	}
	for _, dir := range []string{filepath.Dir(r.gitRemote), r.aliceRoot, filepath.Dir(r.reportPath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	r.report = TeamReport{Version: "v0.6-team-smoke", StartedAt: r.started.UTC().Format(time.RFC3339), Workspace: r.workspace, GitRemote: r.gitRemote, AliceRoot: r.aliceRoot, BobRoot: r.bobRoot, ReportPath: r.reportPath, Executable: r.exe, StoreProfile: r.opts.StoreProfile, Hashes: map[string]string{}, Steps: []Result{}}

	r.section("prepare shared git remote and Alice workspace")
	r.stepCommand(ctx, "git version", r.workspace, false, "git", "--version")
	r.stepCommand(ctx, "create bare git remote", filepath.Dir(r.gitRemote), false, "git", "init", "--bare", r.gitRemote)
	r.stepCommand(ctx, "alice git init", r.aliceRoot, false, "git", "init")
	r.stepCommand(ctx, "alice git branch main", r.aliceRoot, false, "git", "branch", "-M", "main")
	r.stepCommand(ctx, "alice git config email", r.aliceRoot, false, "git", "config", "user.email", "alice@example.com")
	r.stepCommand(ctx, "alice git config name", r.aliceRoot, false, "git", "config", "user.name", "Alice GameDepot")

	r.section("Alice initializes simulated UE project")
	r.stepGD(ctx, "alice gamedepot init", r.aliceRoot, false, "init", "--project", r.opts.ProjectID, "--template", "ue5")
	r.stepGD(ctx, "alice project use shared store profile", r.aliceRoot, false, "config", "project-use", r.opts.StoreProfile)
	r.stepGD(ctx, "alice doctor", r.aliceRoot, false, "doctor")
	r.stepGD(ctx, "alice store info", r.aliceRoot, false, "store", "info")
	r.stepGD(ctx, "alice store check", r.aliceRoot, false, "store", "check")
	if err := r.createFakeProjectFiles(r.aliceRoot, "initial"); err != nil {
		r.addManual("alice create fake UE files", false, "", err)
	} else {
		r.addManual("alice create fake UE files", true, "created simulated .umap/.uasset/.xlsx/.blend/.zip plus Git-managed files", nil)
	}
	r.stepGD(ctx, "alice classify all", r.aliceRoot, false, "classify", "--all")
	r.stepGD(ctx, "alice status json", r.aliceRoot, false, "status", "--json")
	r.stepGD(ctx, "alice lock main map", r.aliceRoot, false, "lock", "Content/Maps/Main.umap", "--owner", "Alice", "--host", "AlicePC", "--note", "Alice edits main map")
	r.stepGD(ctx, "alice locks json", r.aliceRoot, false, "locks", "--json")
	r.stepGD(ctx, "alice submit initial", r.aliceRoot, false, "submit", "-m", "team smoke initial import")
	r.stepGD(ctx, "alice verify", r.aliceRoot, false, "verify")
	r.stepCommand(ctx, "alice add origin", r.aliceRoot, false, "git", "remote", "add", "origin", r.gitRemote)
	r.stepCommand(ctx, "alice push initial main", r.aliceRoot, false, "git", "push", "-u", "origin", "main")
	r.stepCommand(ctx, "set bare remote HEAD to main", r.workspace, false, "git", "--git-dir", r.gitRemote, "symbolic-ref", "HEAD", "refs/heads/main")
	r.stepCommand(ctx, "alice tag initial milestone", r.aliceRoot, false, "git", "tag", "team-initial")
	r.stepCommand(ctx, "alice push tags", r.aliceRoot, false, "git", "push", "--tags")

	r.section("Bob clones and syncs blob-managed files")
	r.stepCommand(ctx, "bob clone", r.workspace, false, "git", "clone", r.gitRemote, r.bobRoot)
	r.stepCommand(ctx, "bob git config email", r.bobRoot, false, "git", "config", "user.email", "bob@example.com")
	r.stepCommand(ctx, "bob git config name", r.bobRoot, false, "git", "config", "user.name", "Bob GameDepot")
	r.stepGD(ctx, "bob doctor", r.bobRoot, false, "doctor")
	r.stepGD(ctx, "bob store info", r.bobRoot, false, "store", "info")
	r.stepGD(ctx, "bob sync initial blobs", r.bobRoot, false, "sync", "--force")
	r.verifyHash("bob verifies restored initial main map", r.bobRoot, "Content/Maps/Main.umap", "Content/Maps/Main.umap#initial")
	r.verifyHash("bob verifies restored initial hero asset", r.bobRoot, "Content/Characters/Hero.uasset", "Content/Characters/Hero.uasset#initial")
	r.stepGD(ctx, "bob verify after initial sync", r.bobRoot, false, "verify")
	r.stepGD(ctx, "bob lock main map fails while Alice owns it", r.bobRoot, true, "lock", "Content/Maps/Main.umap", "--owner", "Bob", "--host", "BobPC", "--note", "Bob tries same map")

	r.section("Alice edits map, pushes manifest, unlocks")
	if err := r.writeBinary(r.aliceRoot, "Content/Maps/Main.umap", 300*1024, 3001, "alice-map"); err != nil {
		r.addManual("alice modifies main map", false, "", err)
	} else {
		r.addManual("alice modifies main map", true, "Content/Maps/Main.umap changed", nil)
	}
	r.stepGD(ctx, "alice submit modified map", r.aliceRoot, false, "submit", "-m", "Alice modifies main map")
	r.stepCommand(ctx, "alice push modified map", r.aliceRoot, false, "git", "push", "origin", "main")
	r.stepGD(ctx, "alice unlock main map", r.aliceRoot, false, "unlock", "Content/Maps/Main.umap", "--owner", "Alice", "--host", "AlicePC")
	r.stepGD(ctx, "alice locks after unlock", r.aliceRoot, false, "locks")

	r.section("Bob pulls Alice changes and syncs from store")
	r.stepCommand(ctx, "bob pull Alice map commit", r.bobRoot, false, "git", "pull", "--ff-only")
	r.stepGD(ctx, "bob sync Alice map", r.bobRoot, false, "sync", "--force")
	r.verifyHash("bob verifies Alice modified main map", r.bobRoot, "Content/Maps/Main.umap", "Content/Maps/Main.umap#alice-map")
	r.stepGD(ctx, "bob lock main map after Alice unlock", r.bobRoot, false, "lock", "Content/Maps/Main.umap", "--owner", "Bob", "--host", "BobPC", "--note", "Bob verifies map lock after Alice")
	r.stepGD(ctx, "bob unlock main map", r.bobRoot, false, "unlock", "Content/Maps/Main.umap", "--owner", "Bob", "--host", "BobPC")

	r.section("Bob edits hero asset, pushes manifest")
	r.stepGD(ctx, "bob lock hero asset", r.bobRoot, false, "lock", "Content/Characters/Hero.uasset", "--owner", "Bob", "--host", "BobPC", "--note", "Bob edits hero")
	if err := r.writeBinary(r.bobRoot, "Content/Characters/Hero.uasset", 220*1024, 4002, "bob-hero"); err != nil {
		r.addManual("bob modifies hero asset", false, "", err)
	} else {
		r.addManual("bob modifies hero asset", true, "Content/Characters/Hero.uasset changed", nil)
	}
	r.stepGD(ctx, "bob submit modified hero", r.bobRoot, false, "submit", "-m", "Bob modifies hero asset")
	r.stepCommand(ctx, "bob push modified hero", r.bobRoot, false, "git", "push", "origin", "main")
	r.stepGD(ctx, "bob unlock hero asset", r.bobRoot, false, "unlock", "Content/Characters/Hero.uasset", "--owner", "Bob", "--host", "BobPC")

	r.section("Alice pulls Bob changes and syncs from store")
	r.stepCommand(ctx, "alice pull Bob hero commit", r.aliceRoot, false, "git", "pull", "--ff-only")
	r.stepGD(ctx, "alice sync Bob hero", r.aliceRoot, false, "sync", "--force")
	r.verifyHash("alice verifies Bob modified hero asset", r.aliceRoot, "Content/Characters/Hero.uasset", "Content/Characters/Hero.uasset#bob-hero")
	r.stepGD(ctx, "alice verify remote-only", r.aliceRoot, false, "verify", "--remote-only")
	r.stepGD(ctx, "bob verify remote-only", r.bobRoot, false, "verify", "--remote-only")

	if r.opts.RunGC {
		r.section("GC safety dry-runs")
		r.stepGD(ctx, "alice gc dry-run current head", r.aliceRoot, false, "gc", "--dry-run")
		r.stepGD(ctx, "alice gc dry-run protect initial tag", r.aliceRoot, false, "gc", "--dry-run", "--protect-tag", "team-initial")
		r.stepGD(ctx, "alice gc dry-run protect all tags", r.aliceRoot, false, "gc", "--dry-run", "--protect-all-tags")
	}

	r.section("final state")
	r.stepGD(ctx, "alice locks final", r.aliceRoot, false, "locks", "--json")
	r.stepGD(ctx, "bob locks final", r.bobRoot, false, "locks", "--json")
	r.stepCommand(ctx, "alice final git status", r.aliceRoot, false, "git", "status", "--short")
	r.stepCommand(ctx, "bob final git status", r.bobRoot, false, "git", "status", "--short")

	r.report.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	r.report.Passed = !r.failed
	if err := r.writeReport(); err != nil {
		return err
	}
	fmt.Println("Team smoke test report written:", r.reportPath)
	fmt.Println("Team smoke workspace:", r.workspace)
	fmt.Println("Project ID:", r.opts.ProjectID)
	if r.report.Passed {
		fmt.Println("Team smoke test result: PASS")
	} else {
		fmt.Println("Team smoke test result: FAIL")
	}
	if r.opts.Keep {
		fmt.Println("Workspace kept for inspection. Remove it manually when done.")
	} else if !pathInside(r.reportPath, r.workspace) {
		if err := os.RemoveAll(r.workspace); err != nil {
			return fmt.Errorf("remove team smoke workspace: %w", err)
		}
		fmt.Println("Team smoke workspace removed.")
	} else {
		fmt.Println("Workspace kept because the report is inside it.")
	}
	if !r.report.Passed {
		return fmt.Errorf("team smoke test failed; see %s", r.reportPath)
	}
	return nil
}

func (r *teamRunner) section(name string) {
	r.addManual("== "+name+" ==", true, "", nil)
}

func (r *teamRunner) stepGD(ctx context.Context, name string, dir string, expectFailure bool, args ...string) {
	r.stepCommand(ctx, name, dir, expectFailure, r.exe, args...)
}

func (r *teamRunner) stepCommand(ctx context.Context, name string, dir string, expectFailure bool, command string, args ...string) {
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
	res := Result{Name: name, Command: quoteCommand(command, args...), Dir: dir, ExpectFailure: expectFailure, ExitOK: exitOK, Passed: passed, DurationMS: time.Since(start).Milliseconds(), Output: trimLong(out.String(), 24000)}
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

func (r *teamRunner) addManual(name string, passed bool, output string, err error) {
	res := Result{Name: name, ExitOK: err == nil, Passed: passed && err == nil, Output: output}
	if err != nil {
		res.Error = err.Error()
	}
	if !res.Passed {
		r.failed = true
	}
	r.report.Steps = append(r.report.Steps, res)
}

func (r *teamRunner) createFakeProjectFiles(root string, label string) error {
	projectID := r.opts.ProjectID
	texts := []struct{ rel, text string }{
		{projectID + ".uproject", `{"FileVersion":3,"EngineAssociation":"5.4","Category":"Games","Description":"GameDepot team simulated UE project"}` + "\n"},
		{"Config/DefaultGame.ini", "[/Script/EngineSettings.GameMapsSettings]\nGameDefaultMap=/Game/Maps/Main\n"},
		{"Config/DefaultEngine.ini", "[/Script/Engine.Engine]\nGameViewportClientClassName=/Script/Engine.GameViewportClient\n"},
		{"Source/" + projectID + "/" + projectID + ".Build.cs", "using UnrealBuildTool;\npublic class " + projectID + " : ModuleRules { public " + projectID + "(ReadOnlyTargetRules Target) : base(Target) {} }\n"},
		{"Source/" + projectID + "/GameMode.cpp", "// simulated UE C++ file managed by Git\n"},
		{"Docs/TeamSmokeTest.md", "# Team Smoke Test\n\nGenerated by `gamedepot team-smoke-test`.\n"},
		{"Saved/Logs/TeamSmoke.log", "ignored runtime log\n"},
	}
	for _, f := range texts {
		if err := writeText(filepath.Join(root, filepath.FromSlash(f.rel)), f.text); err != nil {
			return err
		}
	}
	binaries := []struct {
		rel  string
		size int
		seed int64
	}{
		{"Content/Maps/Main.umap", 256 * 1024, 1101},
		{"Content/Characters/Hero.uasset", 160 * 1024, 1102},
		{"Content/Props/Crate.uasset", 96 * 1024, 1103},
		{"External/Planning/balance.xlsx", 40 * 1024, 1104},
		{"External/Art/source/Hero.blend", 128 * 1024, 1105},
		{"External/SharedTools/BlenderPortable.zip", 192 * 1024, 1106},
	}
	for _, f := range binaries {
		if err := r.writeBinary(root, f.rel, f.size, f.seed, label); err != nil {
			return err
		}
	}
	return nil
}

func (r *teamRunner) writeBinary(root string, rel string, size int, seed int64, label string) error {
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	buf := make([]byte, size)
	rng := rand.New(rand.NewSource(seed))
	if _, err := rng.Read(buf); err != nil {
		return err
	}
	copy(buf, []byte("GAMEDEPOT_TEAM_SIMULATED_BINARY:"+label+":"+rel+"\n"))
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		return err
	}
	sum := sha256.Sum256(buf)
	r.report.Hashes[filepath.ToSlash(rel)+"#"+label] = hex.EncodeToString(sum[:])
	return nil
}

func (r *teamRunner) verifyHash(name string, root string, rel string, key string) {
	path := filepath.Join(root, filepath.FromSlash(rel))
	data, err := os.ReadFile(path)
	if err != nil {
		r.addManual(name, false, "", err)
		return
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	want := r.report.Hashes[key]
	out := fmt.Sprintf("%s\nkey:  %s\nwant: %s\ngot:  %s", rel, key, want, got)
	r.addManual(name, want != "" && want == got, out, nil)
}

func (r *teamRunner) writeReport() error {
	var b strings.Builder
	b.WriteString("# GameDepot Team Smoke Test Report\n\n")
	b.WriteString("```json\n")
	summary := map[string]any{
		"passed":        r.report.Passed,
		"started_at":    r.report.StartedAt,
		"finished_at":   r.report.FinishedAt,
		"workspace":     r.report.Workspace,
		"git_remote":    r.report.GitRemote,
		"alice_root":    r.report.AliceRoot,
		"bob_root":      r.report.BobRoot,
		"executable":    r.report.Executable,
		"store_profile": r.report.StoreProfile,
		"goos":          runtime.GOOS,
		"goarch":        runtime.GOARCH,
	}
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
