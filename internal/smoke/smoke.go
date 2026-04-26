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
	"io"
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

type Options struct {
	Workspace      string
	Report         string
	Clean          bool
	Keep           bool
	ProjectID      string
	StoreProfile   string
	IsolatedConfig bool
	RunGC          bool
}

type Result struct {
	Name          string `json:"name"`
	Command       string `json:"command,omitempty"`
	Dir           string `json:"dir,omitempty"`
	ExpectFailure bool   `json:"expect_failure,omitempty"`
	ExitOK        bool   `json:"exit_ok"`
	Passed        bool   `json:"passed"`
	DurationMS    int64  `json:"duration_ms"`
	Output        string `json:"output,omitempty"`
	Error         string `json:"error,omitempty"`
}

type Report struct {
	Version         string            `json:"version"`
	StartedAt       string            `json:"started_at"`
	FinishedAt      string            `json:"finished_at"`
	Passed          bool              `json:"passed"`
	Workspace       string            `json:"workspace"`
	ProjectRoot     string            `json:"project_root"`
	ReportPath      string            `json:"report_path"`
	Executable      string            `json:"executable"`
	GlobalConfigDir string            `json:"global_config_dir"`
	Hashes          map[string]string `json:"hashes"`
	Steps           []Result          `json:"steps"`
}

type runner struct {
	opts            Options
	exe             string
	workspace       string
	projectRoot     string
	reportPath      string
	globalConfigDir string
	env             []string
	started         time.Time
	report          Report
	failed          bool
}

func Run(ctx context.Context, opts Options) error {
	r, err := newRunner(opts)
	if err != nil {
		return err
	}
	return r.run(ctx)
}

func RegisterFlags(fs *flag.FlagSet, opts *Options) {
	fs.StringVar(&opts.Workspace, "workspace", "GameDepot_SmokeWorkspace", "workspace directory to create and test")
	fs.StringVar(&opts.Report, "report", "gamedepot_smoke_report.txt", "report file path")
	fs.BoolVar(&opts.Clean, "clean", true, "remove the smoke workspace before running")
	fs.BoolVar(&opts.Keep, "keep", true, "keep the generated workspace for inspection")
	fs.StringVar(&opts.ProjectID, "project", "SimUEProject", "simulated project id/name")
	fs.StringVar(&opts.StoreProfile, "profile", "", "store profile to use for this project; empty keeps the template default local profile")
	fs.BoolVar(&opts.IsolatedConfig, "isolated-config", true, "use an isolated GAMEDEPOT_CONFIG_DIR under the smoke workspace")
	fs.BoolVar(&opts.RunGC, "gc", true, "run delete-version and gc smoke steps")
}

func newRunner(opts Options) (*runner, error) {
	if opts.ProjectID == "" {
		opts.ProjectID = "SimUEProject"
	}
	if opts.Workspace == "" {
		opts.Workspace = "GameDepot_SmokeWorkspace"
	}
	if opts.Report == "" {
		opts.Report = "gamedepot_smoke_report.txt"
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

	projectRoot := filepath.Join(workspace, opts.ProjectID)
	globalConfigDir := filepath.Join(workspace, "_global_config")
	env := os.Environ()
	if opts.IsolatedConfig {
		env = append(env, config.EnvConfigDir+"="+globalConfigDir)
	}

	return &runner{
		opts:            opts,
		exe:             exe,
		workspace:       workspace,
		projectRoot:     projectRoot,
		reportPath:      reportPath,
		globalConfigDir: globalConfigDir,
		env:             env,
		started:         time.Now(),
	}, nil
}

func (r *runner) run(ctx context.Context) error {
	if r.opts.Clean {
		if err := os.RemoveAll(r.workspace); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(r.projectRoot, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(r.reportPath), 0o755); err != nil {
		return err
	}

	r.report = Report{
		Version:         "v0.5-smoke",
		StartedAt:       r.started.UTC().Format(time.RFC3339),
		Workspace:       r.workspace,
		ProjectRoot:     r.projectRoot,
		ReportPath:      r.reportPath,
		Executable:      r.exe,
		GlobalConfigDir: r.globalConfigDir,
		Hashes:          map[string]string{},
		Steps:           []Result{},
	}

	r.section("prepare")
	r.stepCommand(ctx, "git version", r.projectRoot, false, "git", "--version")
	r.stepCommand(ctx, "git init", r.projectRoot, false, "git", "init")
	r.stepCommand(ctx, "git config user.email", r.projectRoot, false, "git", "config", "user.email", "smoke@example.com")
	r.stepCommand(ctx, "git config user.name", r.projectRoot, false, "git", "config", "user.name", "GameDepot Smoke Test")

	r.section("init GameDepot project")
	r.stepGD(ctx, "gamedepot init", false, "init", "--project", r.opts.ProjectID, "--template", "ue5")
	r.stepGD(ctx, "isolated config path", false, "config", "path")
	r.stepGD(ctx, "set smoke user", false, "config", "user", "--name", "Smoke Tester", "--email", "smoke@example.com")
	r.stepGD(ctx, "doctor", false, "doctor")
	if r.opts.StoreProfile != "" {
		r.stepGD(ctx, "project use store profile", false, "config", "project-use", r.opts.StoreProfile)
	}
	r.stepGD(ctx, "store info", false, "store", "info")
	r.stepGD(ctx, "store check", false, "store", "check")

	r.section("create simulated UE5 files")
	if err := r.createFakeProjectFiles(); err != nil {
		r.addManual("create fake project files", false, "", err)
	} else {
		r.addManual("create fake project files", true, "created .umap/.uasset/.xlsx/.cpp/.ini/.uproject and ignored runtime files", nil)
	}

	r.section("classify and lock")
	r.stepGD(ctx, "classify all", false, "classify", "--all")
	r.stepGD(ctx, "classify json", false, "classify", "--json", "--all")
	r.stepGD(ctx, "status json before submit", false, "status", "--json")
	r.stepGD(ctx, "lock main map", false, "lock", "Content/Maps/Main.umap", "--note", "smoke test map edit")
	r.stepGD(ctx, "locks json after lock", false, "locks", "--json")

	r.section("submit and verify")
	r.stepGD(ctx, "submit initial import", false, "submit", "-m", "smoke initial simulated UE project")
	r.stepGD(ctx, "verify after submit", false, "verify")
	r.stepGD(ctx, "list manifest entries", false, "ls", "--all")
	r.stepCommand(ctx, "git status after submit", r.projectRoot, false, "git", "status", "--short")
	r.stepCommand(ctx, "tag initial smoke milestone", r.projectRoot, false, "git", "tag", "smoke-initial")

	r.section("restore current version")
	mainMap := filepath.Join(r.projectRoot, filepath.FromSlash("Content/Maps/Main.umap"))
	if err := os.Remove(mainMap); err != nil {
		r.addManual("remove main map before restore", false, "", err)
	} else {
		r.addManual("remove main map before restore", true, "removed Content/Maps/Main.umap", nil)
	}
	r.stepGD(ctx, "restore current main map", false, "restore", "Content/Maps/Main.umap")
	r.verifyHash("verify restored main map hash", "Content/Maps/Main.umap")

	r.section("dirty-file protection")
	if err := os.WriteFile(mainMap, []byte("dirty local edit that is not in blob store\n"), 0o644); err != nil {
		r.addManual("write dirty main map", false, "", err)
	} else {
		r.addManual("write dirty main map", true, "wrote unsubmitted local content", nil)
	}
	r.stepGD(ctx, "restore refuses dirty main map", true, "restore", "Content/Maps/Main.umap")
	r.stepGD(ctx, "restore dirty main map with force", false, "restore", "Content/Maps/Main.umap", "--force")
	r.verifyHash("verify forced restored main map hash", "Content/Maps/Main.umap")

	r.section("modify assets and resubmit")
	if err := r.modifyFakeFiles(); err != nil {
		r.addManual("modify simulated files", false, "", err)
	} else {
		r.addManual("modify simulated files", true, "modified Hero.uasset and Config/DefaultGame.ini", nil)
	}
	r.stepGD(ctx, "status json after modifications", false, "status", "--json")
	r.stepGD(ctx, "submit modifications", false, "submit", "-m", "smoke modify simulated assets")
	r.stepGD(ctx, "verify after second submit", false, "verify")
	r.stepGD(ctx, "history main map", false, "history", "Content/Maps/Main.umap")
	r.stepGD(ctx, "history hero asset", false, "history", "Content/Characters/Hero.uasset")

	if r.opts.RunGC {
		r.section("delete-version and gc")
		oldHero := r.report.Hashes["Content/Characters/Hero.uasset"]
		if oldHero != "" {
			r.stepGD(ctx, "delete-version old hero dry-run", false, "delete-version", "Content/Characters/Hero.uasset", "--sha256", oldHero)
			r.stepGD(ctx, "gc dry-run current manifest", false, "gc", "--dry-run")
			r.stepGD(ctx, "gc dry-run with protected tag", false, "gc", "--dry-run", "--protect-tag", "smoke-initial")
			r.stepGD(ctx, "delete-version old hero execute", false, "delete-version", "Content/Characters/Hero.uasset", "--sha256", oldHero, "--execute")
			r.stepGD(ctx, "gc dry-run after delete-version", false, "gc", "--dry-run")
		}
	}

	r.section("sync and unlock")
	r.stepGD(ctx, "sync force", false, "sync", "--force")
	r.stepGD(ctx, "unlock main map", false, "unlock", "Content/Maps/Main.umap")
	r.stepGD(ctx, "locks after unlock", false, "locks")
	r.stepGD(ctx, "final verify", false, "verify")
	r.stepCommand(ctx, "final git status", r.projectRoot, false, "git", "status", "--short")

	r.report.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	r.report.Passed = !r.failed
	if err := r.writeReport(); err != nil {
		return err
	}

	fmt.Println("Smoke test report written:", r.reportPath)
	fmt.Println("Smoke workspace:", r.workspace)
	if r.report.Passed {
		fmt.Println("Smoke test result: PASS")
	} else {
		fmt.Println("Smoke test result: FAIL")
	}

	if r.opts.Keep {
		fmt.Println("Workspace kept for inspection. Remove it manually when done.")
	} else if !pathInside(r.reportPath, r.workspace) {
		if err := os.RemoveAll(r.workspace); err != nil {
			return fmt.Errorf("remove smoke workspace: %w", err)
		}
		fmt.Println("Smoke workspace removed.")
	} else {
		fmt.Println("Workspace kept because the report is inside it.")
	}

	if !r.report.Passed {
		return fmt.Errorf("smoke test failed; see %s", r.reportPath)
	}
	return nil
}

func (r *runner) section(name string) {
	r.addManual("== "+name+" ==", true, "", nil)
}

func (r *runner) stepGD(ctx context.Context, name string, expectFailure bool, args ...string) {
	r.stepCommand(ctx, name, r.projectRoot, expectFailure, r.exe, args...)
}

func (r *runner) stepCommand(ctx context.Context, name string, dir string, expectFailure bool, command string, args ...string) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
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
	res := Result{
		Name:          name,
		Command:       quoteCommand(command, args...),
		Dir:           dir,
		ExpectFailure: expectFailure,
		ExitOK:        exitOK,
		Passed:        passed,
		DurationMS:    time.Since(start).Milliseconds(),
		Output:        trimLong(out.String(), 16_000),
	}
	if err != nil {
		res.Error = err.Error()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			res.Error = "timeout after 60s: " + res.Error
		}
	}
	if !passed {
		r.failed = true
	}
	r.report.Steps = append(r.report.Steps, res)
}

func (r *runner) addManual(name string, passed bool, output string, err error) {
	res := Result{
		Name:       name,
		ExitOK:     err == nil,
		Passed:     passed && err == nil,
		DurationMS: 0,
		Output:     output,
	}
	if err != nil {
		res.Error = err.Error()
	}
	if !res.Passed {
		r.failed = true
	}
	r.report.Steps = append(r.report.Steps, res)
}

func (r *runner) createFakeProjectFiles() error {
	projectID := r.opts.ProjectID
	files := []struct {
		rel  string
		text string
	}{
		{projectID + ".uproject", `{"FileVersion":3,"EngineAssociation":"5.4","Category":"Games","Description":"GameDepot simulated UE project"}` + "\n"},
		{"Config/DefaultGame.ini", "[/Script/EngineSettings.GameMapsSettings]\nGameDefaultMap=/Game/Maps/Main\n"},
		{"Config/DefaultEngine.ini", "[/Script/Engine.Engine]\nGameViewportClientClassName=/Script/Engine.GameViewportClient\n"},
		{"Source/" + projectID + "/" + projectID + ".Build.cs", "using UnrealBuildTool;\npublic class " + projectID + " : ModuleRules { public " + projectID + "(ReadOnlyTargetRules Target) : base(Target) {} }\n"},
		{"Source/" + projectID + "/GameMode.cpp", "// simulated UE C++ file managed by Git\n"},
		{"Docs/SmokeTest.md", "# Smoke Test\n\nGenerated by `gamedepot smoke-test`.\n"},
		{"Saved/Logs/Smoke.log", "ignored runtime log\n"},
	}
	for _, f := range files {
		if err := writeText(filepath.Join(r.projectRoot, filepath.FromSlash(f.rel)), f.text); err != nil {
			return err
		}
	}

	binaries := []struct {
		rel  string
		size int
		seed int64
	}{
		{"Content/Maps/Main.umap", 256 * 1024, 1001},
		{"Content/Characters/Hero.uasset", 160 * 1024, 1002},
		{"Content/Props/Crate.uasset", 96 * 1024, 1003},
		{"External/Planning/balance.xlsx", 40 * 1024, 1004},
		{"External/Art/source/Hero.blend", 128 * 1024, 1005},
		{"External/SharedTools/BlenderPortable.zip", 192 * 1024, 1006},
	}
	for _, f := range binaries {
		rel, sha, err := r.writeBinary(f.rel, f.size, f.seed)
		if err != nil {
			return err
		}
		r.report.Hashes[rel] = sha
	}
	return nil
}

func (r *runner) modifyFakeFiles() error {
	_, sha, err := r.writeBinary("Content/Characters/Hero.uasset", 180*1024, 2002)
	if err != nil {
		return err
	}
	r.report.Hashes["Content/Characters/Hero.uasset#modified"] = sha
	return appendText(filepath.Join(r.projectRoot, filepath.FromSlash("Config/DefaultGame.ini")), "\n[/Script/GameDepot.Smoke]\nModified=true\n")
}

func (r *runner) writeBinary(rel string, size int, seed int64) (string, string, error) {
	path := filepath.Join(r.projectRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return rel, "", err
	}
	buf := make([]byte, size)
	rng := rand.New(rand.NewSource(seed))
	if _, err := rng.Read(buf); err != nil {
		return rel, "", err
	}
	copy(buf, []byte("GAMEDEPOT_SIMULATED_BINARY:"+rel+"\n"))
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		return rel, "", err
	}
	sum := sha256.Sum256(buf)
	return filepath.ToSlash(rel), hex.EncodeToString(sum[:]), nil
}

func (r *runner) verifyHash(name string, rel string) {
	path := filepath.Join(r.projectRoot, filepath.FromSlash(rel))
	data, err := os.ReadFile(path)
	if err != nil {
		r.addManual(name, false, "", err)
		return
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	want := r.report.Hashes[filepath.ToSlash(rel)]
	if want == "" {
		r.addManual(name, false, "", fmt.Errorf("no expected hash recorded for %s", rel))
		return
	}
	out := fmt.Sprintf("%s\nwant: %s\ngot:  %s", rel, want, got)
	r.addManual(name, want == got, out, nil)
}

func (r *runner) writeReport() error {
	var b strings.Builder
	b.WriteString("# GameDepot Smoke Test Report\n\n")
	b.WriteString("```json\n")
	summary := map[string]any{
		"passed":            r.report.Passed,
		"started_at":        r.report.StartedAt,
		"finished_at":       r.report.FinishedAt,
		"workspace":         r.report.Workspace,
		"project_root":      r.report.ProjectRoot,
		"executable":        r.report.Executable,
		"global_config_dir": r.report.GlobalConfigDir,
		"goos":              runtime.GOOS,
		"goarch":            runtime.GOARCH,
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

func pathInside(path string, root string) bool {
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	if pathAbs == rootAbs {
		return true
	}
	return strings.HasPrefix(pathAbs, rootAbs+string(os.PathSeparator))
}

func writeText(path string, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

func appendText(path string, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.WriteString(f, body)
	return err
}

func quoteCommand(command string, args ...string) string {
	parts := append([]string{command}, args...)
	for i, p := range parts {
		if strings.ContainsAny(p, " \t\n\"'") {
			parts[i] = fmt.Sprintf("%q", p)
		}
	}
	return strings.Join(parts, " ")
}

func trimLong(s string, max int) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if len(s) <= max {
		return strings.TrimRight(s, "\n")
	}
	return strings.TrimRight(s[:max], "\n") + "\n...<truncated>..."
}
