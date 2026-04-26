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
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/1051093860/gamedepot/internal/config"
	"github.com/1051093860/gamedepot/internal/server"
)

type APIOptions struct {
	Workspace    string
	Report       string
	Clean        bool
	Keep         bool
	ProjectID    string
	StoreProfile string
}

type APIReport struct {
	Version         string            `json:"version"`
	StartedAt       string            `json:"started_at"`
	FinishedAt      string            `json:"finished_at"`
	Passed          bool              `json:"passed"`
	Workspace       string            `json:"workspace"`
	ProjectRoot     string            `json:"project_root"`
	ReportPath      string            `json:"report_path"`
	Executable      string            `json:"executable"`
	GlobalConfigDir string            `json:"global_config_dir"`
	BaseURL         string            `json:"base_url"`
	Hashes          map[string]string `json:"hashes"`
	Steps           []Result          `json:"steps"`
}

type apiRunner struct {
	opts            APIOptions
	exe             string
	workspace       string
	projectRoot     string
	reportPath      string
	globalConfigDir string
	env             []string
	started         time.Time
	report          APIReport
	failed          bool
	baseURL         string
	token           string
}

func RegisterAPIFlags(fs *flag.FlagSet, opts *APIOptions) {
	fs.StringVar(&opts.Workspace, "workspace", "GameDepot_APISmokeWorkspace", "workspace directory to create and test")
	fs.StringVar(&opts.Report, "report", "gamedepot_api_smoke_report.md", "report file path")
	fs.BoolVar(&opts.Clean, "clean", true, "remove the API smoke workspace before running")
	fs.BoolVar(&opts.Keep, "keep", true, "keep the generated workspace for inspection")
	fs.StringVar(&opts.ProjectID, "project", "SimAPIProject", "simulated project id/name")
	fs.StringVar(&opts.StoreProfile, "profile", "", "optional store profile to use for this project")
}

func RunAPI(ctx context.Context, opts APIOptions) error {
	r, err := newAPIRunner(opts)
	if err != nil {
		return err
	}
	return r.run(ctx)
}

func newAPIRunner(opts APIOptions) (*apiRunner, error) {
	if opts.ProjectID == "" {
		opts.ProjectID = "SimAPIProject"
	}
	if opts.Workspace == "" {
		opts.Workspace = "GameDepot_APISmokeWorkspace"
	}
	if opts.Report == "" {
		opts.Report = "gamedepot_api_smoke_report.md"
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
	env := append(os.Environ(), config.EnvConfigDir+"="+globalConfigDir)
	return &apiRunner{opts: opts, exe: exe, workspace: workspace, projectRoot: projectRoot, reportPath: reportPath, globalConfigDir: globalConfigDir, env: env, started: time.Now(), token: "api-smoke-token"}, nil
}

func (r *apiRunner) run(ctx context.Context) error {
	oldConfigDir := os.Getenv(config.EnvConfigDir)
	_ = os.Setenv(config.EnvConfigDir, r.globalConfigDir)
	defer func() { _ = os.Setenv(config.EnvConfigDir, oldConfigDir) }()

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

	r.report = APIReport{Version: "v0.6-api-smoke", StartedAt: r.started.UTC().Format(time.RFC3339), Workspace: r.workspace, ProjectRoot: r.projectRoot, ReportPath: r.reportPath, Executable: r.exe, GlobalConfigDir: r.globalConfigDir, Hashes: map[string]string{}, Steps: []Result{}}

	r.section("prepare project")
	r.stepCommand(ctx, "git version", r.projectRoot, false, "git", "--version")
	r.stepCommand(ctx, "git init", r.projectRoot, false, "git", "init")
	r.stepCommand(ctx, "git config user.email", r.projectRoot, false, "git", "config", "user.email", "api-smoke@example.com")
	r.stepCommand(ctx, "git config user.name", r.projectRoot, false, "git", "config", "user.name", "GameDepot API Smoke")
	r.stepGD(ctx, "gamedepot init", false, "init", "--project", r.opts.ProjectID, "--template", "ue5")
	r.stepGD(ctx, "config user", false, "config", "user", "--name", "API Smoke", "--email", "api-smoke@example.com")
	if r.opts.StoreProfile != "" {
		r.stepGD(ctx, "project use store profile", false, "config", "project-use", r.opts.StoreProfile)
	}
	if err := r.createFakeProjectFiles(); err != nil {
		r.addManual("create fake project files", false, "", err)
	} else {
		r.addManual("create fake project files", true, "created simulated API project files", nil)
	}

	h := server.New(server.Options{Root: r.projectRoot, Token: r.token, Version: "v0.6"}).Handler()
	ts := httptest.NewServer(h)
	defer ts.Close()
	r.baseURL = ts.URL
	r.report.BaseURL = ts.URL

	r.section("read APIs")
	r.stepHTTP(ctx, "health", false, http.MethodGet, "/api/v1/health", nil)
	r.stepHTTP(ctx, "version", false, http.MethodGet, "/api/v1/version", nil)
	r.stepHTTP(ctx, "store info", false, http.MethodGet, "/api/v1/store", nil)
	r.stepHTTP(ctx, "store check", false, http.MethodPost, "/api/v1/store/check", map[string]any{})
	r.stepHTTP(ctx, "classify all", false, http.MethodGet, "/api/v1/classify?all=true", nil)
	r.stepHTTP(ctx, "status before submit", false, http.MethodGet, "/api/v1/status", nil)

	r.section("write APIs")
	r.stepHTTP(ctx, "lock main map", false, http.MethodPost, "/api/v1/lock", map[string]any{"path": "Content/Maps/Main.umap", "note": "api smoke edit"})
	r.stepHTTP(ctx, "locks after lock", false, http.MethodGet, "/api/v1/locks", nil)
	r.stepHTTP(ctx, "submit initial", false, http.MethodPost, "/api/v1/submit", map[string]any{"message": "api smoke initial import"})
	r.stepHTTP(ctx, "verify after submit", false, http.MethodPost, "/api/v1/verify", map[string]any{})
	r.stepHTTP(ctx, "manifest after submit", false, http.MethodGet, "/api/v1/manifest", nil)
	r.stepCommand(ctx, "git tag api-smoke-initial", r.projectRoot, false, "git", "tag", "api-smoke-initial")

	r.section("restore APIs")
	mainMap := filepath.Join(r.projectRoot, filepath.FromSlash("Content/Maps/Main.umap"))
	if err := os.Remove(mainMap); err != nil {
		r.addManual("remove main map", false, "", err)
	} else {
		r.addManual("remove main map", true, "removed Content/Maps/Main.umap", nil)
	}
	r.stepHTTP(ctx, "restore current main map", false, http.MethodPost, "/api/v1/restore", map[string]any{"path": "Content/Maps/Main.umap"})
	r.verifyHash("verify restored main map hash", "Content/Maps/Main.umap")
	if err := os.WriteFile(mainMap, []byte("dirty api local edit\n"), 0o644); err != nil {
		r.addManual("write dirty main map", false, "", err)
	} else {
		r.addManual("write dirty main map", true, "wrote unsubmitted local content", nil)
	}
	r.stepHTTP(ctx, "restore refuses dirty map", true, http.MethodPost, "/api/v1/restore", map[string]any{"path": "Content/Maps/Main.umap"})
	r.stepHTTP(ctx, "restore dirty map force", false, http.MethodPost, "/api/v1/restore", map[string]any{"path": "Content/Maps/Main.umap", "force": true})
	r.verifyHash("verify forced restored main map hash", "Content/Maps/Main.umap")

	r.section("second submit and maintenance APIs")
	if err := r.modifyFakeFiles(); err != nil {
		r.addManual("modify fake files", false, "", err)
	} else {
		r.addManual("modify fake files", true, "modified Hero.uasset and Config/DefaultGame.ini", nil)
	}
	r.stepHTTP(ctx, "submit modifications", false, http.MethodPost, "/api/v1/submit", map[string]any{"message": "api smoke modifications"})
	r.stepHTTP(ctx, "history hero", false, http.MethodGet, "/api/v1/history?path=Content/Characters/Hero.uasset", nil)
	r.stepHTTP(ctx, "gc dry-run json", false, http.MethodPost, "/api/v1/gc", map[string]any{"json": true, "dry_run": true})
	oldHero := r.report.Hashes["Content/Characters/Hero.uasset"]
	if oldHero != "" {
		r.stepHTTP(ctx, "delete-version old hero dry-run json", false, http.MethodPost, "/api/v1/delete-version", map[string]any{"path": "Content/Characters/Hero.uasset", "sha256": oldHero, "json": true, "dry_run": true})
	}
	r.stepHTTP(ctx, "unlock main map", false, http.MethodPost, "/api/v1/unlock", map[string]any{"path": "Content/Maps/Main.umap"})
	r.stepHTTP(ctx, "locks after unlock", false, http.MethodGet, "/api/v1/locks", nil)
	r.stepHTTP(ctx, "sync force", false, http.MethodPost, "/api/v1/sync", map[string]any{"force": true})
	r.stepHTTP(ctx, "git status", false, http.MethodGet, "/api/v1/git/status", nil)

	r.report.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	r.report.Passed = !r.failed
	if err := r.writeReport(); err != nil {
		return err
	}
	fmt.Println("API smoke test report written:", r.reportPath)
	fmt.Println("API smoke workspace:", r.workspace)
	if r.report.Passed {
		fmt.Println("API smoke test result: PASS")
	} else {
		fmt.Println("API smoke test result: FAIL")
	}
	if !r.opts.Keep && !pathInside(r.reportPath, r.workspace) {
		_ = os.RemoveAll(r.workspace)
	}
	if !r.report.Passed {
		return fmt.Errorf("api smoke test failed; see %s", r.reportPath)
	}
	return nil
}

func (r *apiRunner) section(name string) { r.addManual("== "+name+" ==", true, "", nil) }
func (r *apiRunner) stepGD(ctx context.Context, name string, expectFailure bool, args ...string) {
	r.stepCommand(ctx, name, r.projectRoot, expectFailure, r.exe, args...)
}
func (r *apiRunner) stepCommand(ctx context.Context, name string, dir string, expectFailure bool, command string, args ...string) {
	start := time.Now()
	cctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, command, args...)
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
		if errors.Is(cctx.Err(), context.DeadlineExceeded) {
			res.Error = "timeout after 120s: " + res.Error
		}
	}
	if !passed {
		r.failed = true
	}
	r.report.Steps = append(r.report.Steps, res)
}
func (r *apiRunner) stepHTTP(ctx context.Context, name string, expectFailure bool, method string, path string, body any) {
	start := time.Now()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, r.baseURL+path, reader)
	if err != nil {
		r.addManual(name, false, "", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	var out string
	if resp != nil {
		data, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		out = string(data)
	}
	exitOK := err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300
	passed := exitOK != expectFailure
	res := Result{Name: name, Command: method + " " + path, Dir: r.projectRoot, ExpectFailure: expectFailure, ExitOK: exitOK, Passed: passed, DurationMS: time.Since(start).Milliseconds(), Output: trimLong(out, 24000)}
	if resp != nil {
		res.Command = fmt.Sprintf("%s %s -> HTTP %d", method, path, resp.StatusCode)
	}
	if err != nil {
		res.Error = err.Error()
	}
	if !passed {
		r.failed = true
	}
	r.report.Steps = append(r.report.Steps, res)
}
func (r *apiRunner) addManual(name string, passed bool, output string, err error) {
	res := Result{Name: name, ExitOK: err == nil, Passed: passed && err == nil, Output: output}
	if err != nil {
		res.Error = err.Error()
	}
	if !res.Passed {
		r.failed = true
	}
	r.report.Steps = append(r.report.Steps, res)
}
func (r *apiRunner) createFakeProjectFiles() error {
	pid := r.opts.ProjectID
	texts := []struct{ rel, text string }{{pid + ".uproject", `{"FileVersion":3,"EngineAssociation":"5.4","Category":"Games","Description":"GameDepot API simulated UE project"}` + "\n"}, {"Config/DefaultGame.ini", "[/Script/EngineSettings.GameMapsSettings]\nGameDefaultMap=/Game/Maps/Main\n"}, {"Config/DefaultEngine.ini", "[/Script/Engine.Engine]\nGameViewportClientClassName=/Script/Engine.GameViewportClient\n"}, {"Source/" + pid + "/GameMode.cpp", "// API smoke C++ file\n"}, {"Docs/APISmokeTest.md", "# API Smoke Test\n"}, {"Saved/Logs/APISmoke.log", "ignored runtime log\n"}}
	for _, f := range texts {
		if err := writeText(filepath.Join(r.projectRoot, filepath.FromSlash(f.rel)), f.text); err != nil {
			return err
		}
	}
	binaries := []struct {
		rel  string
		size int
		seed int64
	}{{"Content/Maps/Main.umap", 128 * 1024, 3001}, {"Content/Characters/Hero.uasset", 96 * 1024, 3002}, {"Content/Props/Crate.uasset", 64 * 1024, 3003}, {"External/Planning/balance.xlsx", 24 * 1024, 3004}, {"External/Art/source/Hero.blend", 80 * 1024, 3005}, {"External/SharedTools/BlenderPortable.zip", 96 * 1024, 3006}}
	for _, f := range binaries {
		rel, sha, err := r.writeBinary(f.rel, f.size, f.seed)
		if err != nil {
			return err
		}
		r.report.Hashes[rel] = sha
	}
	return nil
}
func (r *apiRunner) modifyFakeFiles() error {
	_, sha, err := r.writeBinary("Content/Characters/Hero.uasset", 110*1024, 4002)
	if err != nil {
		return err
	}
	r.report.Hashes["Content/Characters/Hero.uasset#modified"] = sha
	return appendText(filepath.Join(r.projectRoot, filepath.FromSlash("Config/DefaultGame.ini")), "\n[/Script/GameDepot.APISmoke]\nModified=true\n")
}
func (r *apiRunner) writeBinary(rel string, size int, seed int64) (string, string, error) {
	path := filepath.Join(r.projectRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return rel, "", err
	}
	buf := make([]byte, size)
	rng := rand.New(rand.NewSource(seed))
	if _, err := rng.Read(buf); err != nil {
		return rel, "", err
	}
	copy(buf, []byte("GAMEDEPOT_API_SIMULATED_BINARY:"+rel+"\n"))
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		return rel, "", err
	}
	sum := sha256.Sum256(buf)
	return filepath.ToSlash(rel), hex.EncodeToString(sum[:]), nil
}
func (r *apiRunner) verifyHash(name string, rel string) {
	data, err := os.ReadFile(filepath.Join(r.projectRoot, filepath.FromSlash(rel)))
	if err != nil {
		r.addManual(name, false, "", err)
		return
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	want := r.report.Hashes[filepath.ToSlash(rel)]
	r.addManual(name, want == got && want != "", fmt.Sprintf("%s\nwant: %s\ngot:  %s", rel, want, got), nil)
}
func (r *apiRunner) writeReport() error {
	var b strings.Builder
	b.WriteString("# GameDepot API Smoke Test Report\n\n```json\n")
	summary := map[string]any{"passed": r.report.Passed, "started_at": r.report.StartedAt, "finished_at": r.report.FinishedAt, "workspace": r.report.Workspace, "project_root": r.report.ProjectRoot, "base_url": r.report.BaseURL, "global_config_dir": r.report.GlobalConfigDir, "goos": runtime.GOOS, "goarch": runtime.GOARCH}
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
