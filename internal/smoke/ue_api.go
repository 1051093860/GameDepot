package smoke

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/1051093860/gamedepot/internal/config"
)

type UEAPIOptions struct {
	Workspace string
	Report    string
	Clean     bool
	Keep      bool
	ProjectID string
}

type UEAPIReport struct {
	Version         string   `json:"version"`
	StartedAt       string   `json:"started_at"`
	FinishedAt      string   `json:"finished_at"`
	Passed          bool     `json:"passed"`
	Workspace       string   `json:"workspace"`
	ProjectRoot     string   `json:"project_root"`
	ReportPath      string   `json:"report_path"`
	Executable      string   `json:"executable"`
	GlobalConfigDir string   `json:"global_config_dir"`
	APIBase         string   `json:"api_base"`
	Steps           []Result `json:"steps"`
}

type ueAPIRunner struct {
	opts            UEAPIOptions
	exe             string
	workspace       string
	projectRoot     string
	originRemote    string
	sharedStore     string
	reportPath      string
	globalConfigDir string
	env             []string
	started         time.Time
	report          UEAPIReport
	failed          bool
	apiBase         string
	daemon          *exec.Cmd
}

func RegisterUEAPIFlags(fs *flag.FlagSet, opts *UEAPIOptions) {
	fs.StringVar(&opts.Workspace, "workspace", "GameDepot_UEAPISmokeWorkspace", "workspace directory to create and test")
	fs.StringVar(&opts.Report, "report", "gamedepot_ue_api_smoke_report.md", "report file path")
	fs.BoolVar(&opts.Clean, "clean", true, "remove the smoke workspace before running")
	fs.BoolVar(&opts.Keep, "keep", true, "keep generated workspace for inspection")
	fs.StringVar(&opts.ProjectID, "project", "SimUEAPIProject", "simulated project id/name")
}

func RunUEAPI(ctx context.Context, opts UEAPIOptions) error {
	r, err := newUEAPIRunner(opts)
	if err != nil {
		return err
	}
	return r.run(ctx)
}

func newUEAPIRunner(opts UEAPIOptions) (*ueAPIRunner, error) {
	if opts.ProjectID == "" {
		opts.ProjectID = "SimUEAPIProject"
	}
	if opts.Workspace == "" {
		opts.Workspace = "GameDepot_UEAPISmokeWorkspace"
	}
	if opts.Report == "" {
		opts.Report = "gamedepot_ue_api_smoke_report.md"
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
	report, err := filepath.Abs(opts.Report)
	if err != nil {
		return nil, err
	}
	globalConfigDir := filepath.Join(workspace, "_global_config")
	env := append(os.Environ(), config.EnvConfigDir+"="+globalConfigDir)
	return &ueAPIRunner{opts: opts, exe: exe, workspace: workspace, projectRoot: filepath.Join(workspace, opts.ProjectID), originRemote: filepath.Join(workspace, "_git_remote", opts.ProjectID+".git"), sharedStore: filepath.Join(workspace, "_shared_blobs"), reportPath: report, globalConfigDir: globalConfigDir, env: env, started: time.Now()}, nil
}

func (r *ueAPIRunner) run(ctx context.Context) error {
	if r.opts.Clean {
		_ = os.RemoveAll(r.workspace)
	}
	for _, d := range []string{r.projectRoot, filepath.Dir(r.originRemote), r.sharedStore, filepath.Dir(r.reportPath)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	r.report = UEAPIReport{Version: "v0.7-ue-api-smoke", StartedAt: r.started.UTC().Format(time.RFC3339), Workspace: r.workspace, ProjectRoot: r.projectRoot, ReportPath: r.reportPath, Executable: r.exe, GlobalConfigDir: r.globalConfigDir, Steps: []Result{}}
	defer func() {
		if r.daemon != nil && r.daemon.Process != nil {
			_ = r.daemon.Process.Kill()
		}
	}()

	r.section("prepare project and shared local store")
	r.cmd(ctx, "git init bare origin", filepath.Dir(r.originRemote), false, "git", "init", "--bare", r.originRemote)
	r.cmd(ctx, "git init project", r.projectRoot, false, "git", "init")
	r.cmd(ctx, "git branch main", r.projectRoot, false, "git", "branch", "-M", "main")
	r.cmd(ctx, "git config email", r.projectRoot, false, "git", "config", "user.email", "ue-api@example.com")
	r.cmd(ctx, "git config name", r.projectRoot, false, "git", "config", "user.name", "GameDepot UE API Smoke")
	r.gd(ctx, "init", r.projectRoot, false, "init", "--project", r.opts.ProjectID, "--template", "ue5")
	r.gd(ctx, "set global user", r.projectRoot, false, "config", "user", "--name", "UE API Smoke", "--email", "ue-api@example.com")
	r.gd(ctx, "add shared local store", r.projectRoot, false, "config", "add-local", "ue-api-shared", "--path", r.sharedStore)
	r.gd(ctx, "project use local store", r.projectRoot, false, "config", "project-use", "ue-api-shared")
	r.gd(ctx, "set remote", r.projectRoot, false, "git-config", "set-remote", "--name", "origin", "--url", r.originRemote)

	r.section("create simulated UE project files and initial submit")
	if err := r.createFakeFiles(); err != nil {
		r.add("create fake files", false, "", err)
	} else {
		r.add("create fake files", true, "created fake UE assets", nil)
	}
	r.gd(ctx, "submit initial --push", r.projectRoot, false, "submit", "-m", "ue api initial", "--push")
	r.cmd(ctx, "set bare origin HEAD main", r.workspace, false, "git", "--git-dir", r.originRemote, "symbolic-ref", "HEAD", "refs/heads/main")

	r.section("start daemon with auto port")
	if err := r.startDaemon(ctx); err != nil {
		r.add("start daemon", false, "", err)
		r.finish()
		return fmt.Errorf("daemon failed to start")
	}
	r.add("start daemon", true, r.apiBase, nil)

	r.section("UE dedicated API endpoints")
	r.http(ctx, "GET health", "GET", "/api/ue/v1/health", nil, false)
	r.http(ctx, "GET overview", "GET", "/api/ue/v1/overview", nil, false)
	r.http(ctx, "GET settings", "GET", "/api/ue/v1/settings", nil, false)
	r.http(ctx, "POST git test", "POST", "/api/ue/v1/git/test", map[string]any{"remote": "origin"}, false)
	r.http(ctx, "POST store test", "POST", "/api/ue/v1/store/test", map[string]any{}, false)
	r.http(ctx, "POST asset status", "POST", "/api/ue/v1/assets/status", map[string]any{"paths": []string{"Content/Maps/Main.umap", "Content/Characters/Hero.uasset"}, "include_history": true, "include_remote": true}, false)
	r.http(ctx, "POST map status", "POST", "/api/ue/v1/map/status", map[string]any{"path": "Content/Maps/Main.umap"}, false)
	r.http(ctx, "POST asset lock", "POST", "/api/ue/v1/assets/lock", map[string]any{"paths": []string{"Content/Maps/Main.umap"}, "note": "ue api smoke"}, false)
	r.http(ctx, "POST asset unlock", "POST", "/api/ue/v1/assets/unlock", map[string]any{"paths": []string{"Content/Maps/Main.umap"}, "force": true}, false)
	r.http(ctx, "POST project gc preview", "POST", "/api/ue/v1/project/gc-preview", map[string]any{"protect_all_tags": true}, false)
	r.http(ctx, "POST asset history", "POST", "/api/ue/v1/assets/history", map[string]any{"path": "Content/Characters/Hero.uasset"}, false)
	r.http(ctx, "GET diagnostics", "GET", "/api/ue/v1/diagnostics", nil, false)

	r.section("task API")
	r.httpTask(ctx, "project verify task", "/api/ue/v1/project/verify", map[string]any{"local": true, "remote": true})
	r.httpTask(ctx, "project sync task", "/api/ue/v1/project/sync", map[string]any{"force": true, "pull_git": true})
	r.httpTask(ctx, "project submit task", "/api/ue/v1/project/submit", map[string]any{"message": "ue api no-op submit", "push": false, "verify_after_submit": false})
	r.httpTask(ctx, "restore task", "/api/ue/v1/assets/restore", map[string]any{"paths": []string{"Content/Maps/Main.umap"}, "force": true})

	r.section("repair and shutdown")
	r.http(ctx, "POST repair current blob", "POST", "/api/ue/v1/assets/repair-current-blob", map[string]any{"paths": []string{"Content/Maps/Main.umap"}}, false)
	r.http(ctx, "POST admin shutdown", "POST", "/api/ue/v1/admin/shutdown", map[string]any{}, false)
	time.Sleep(300 * time.Millisecond)
	r.finish()
	if !r.report.Passed {
		return fmt.Errorf("ue-api smoke failed; see %s", r.reportPath)
	}
	if !r.opts.Keep {
		_ = os.RemoveAll(r.workspace)
	}
	fmt.Println("UE API smoke test result: PASS")
	fmt.Println("Report:", r.reportPath)
	return nil
}

func (r *ueAPIRunner) startDaemon(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, r.exe, "daemon", "--root", r.projectRoot, "--addr", "127.0.0.1:0")
	cmd.Env = r.env
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Start(); err != nil {
		return err
	}
	r.daemon = cmd
	runtimePath := filepath.Join(r.projectRoot, ".gamedepot", "runtime", "daemon.json")
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(runtimePath)
		if err == nil {
			var v struct {
				Addr string `json:"addr"`
			}
			if json.Unmarshal(data, &v) == nil && v.Addr != "" {
				r.apiBase = "http://" + v.Addr
				r.report.APIBase = r.apiBase
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("daemon runtime file not written: %s", runtimePath)
}

func (r *ueAPIRunner) createFakeFiles() error {
	files := map[string]string{
		"Content/Maps/Main.umap":         "fake map v1\n",
		"Content/Characters/Hero.uasset": "fake hero v1\n",
		"Config/DefaultGame.ini":         "[/Script/EngineSettings.GameMapsSettings]\n",
		r.opts.ProjectID + ".uproject":   "{\"FileVersion\":3}\n",
	}
	for rel, body := range files {
		p := filepath.Join(r.projectRoot, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (r *ueAPIRunner) cmd(ctx context.Context, name, dir string, expectFailure bool, exe string, args ...string) {
	r.step(ctx, name, dir, expectFailure, exe, args...)
}
func (r *ueAPIRunner) gd(ctx context.Context, name, dir string, expectFailure bool, args ...string) {
	r.step(ctx, name, dir, expectFailure, r.exe, args...)
}
func (r *ueAPIRunner) step(ctx context.Context, name, dir string, expectFailure bool, exe string, args ...string) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = dir
	cmd.Env = r.env
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	exitOK := err == nil
	passed := exitOK != expectFailure
	res := Result{Name: name, Command: exe + " " + strings.Join(args, " "), Dir: dir, ExpectFailure: expectFailure, ExitOK: exitOK, Passed: passed, DurationMS: time.Since(start).Milliseconds(), Output: out.String()}
	if err != nil {
		res.Error = err.Error()
	}
	r.report.Steps = append(r.report.Steps, res)
	if !passed {
		r.failed = true
	}
}

func (r *ueAPIRunner) http(ctx context.Context, name, method, path string, body any, expectFailure bool) []byte {
	start := time.Now()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, r.apiBase+path, reader)
	if err != nil {
		r.add(name, false, "", err)
		return nil
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	output := ""
	exitOK := false
	if err == nil {
		data, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		output = string(data)
		exitOK = resp.StatusCode >= 200 && resp.StatusCode < 300
	} else {
		output = err.Error()
	}
	passed := exitOK != expectFailure
	res := Result{Name: name, Command: method + " " + path, Dir: r.projectRoot, ExpectFailure: expectFailure, ExitOK: exitOK, Passed: passed, DurationMS: time.Since(start).Milliseconds(), Output: output}
	if err != nil {
		res.Error = err.Error()
	}
	r.report.Steps = append(r.report.Steps, res)
	if !passed {
		r.failed = true
	}
	return []byte(output)
}

func (r *ueAPIRunner) httpTask(ctx context.Context, name, path string, body any) {
	data := r.http(ctx, name+" create", "POST", path, body, false)
	var res struct {
		OK     bool   `json:"ok"`
		TaskID string `json:"task_id"`
	}
	if json.Unmarshal(data, &res) != nil || res.TaskID == "" {
		r.add(name+" parse task", false, string(data), fmt.Errorf("missing task_id"))
		return
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		out := r.http(ctx, name+" poll", "GET", "/api/ue/v1/tasks/"+res.TaskID, nil, false)
		var tr struct {
			OK   bool `json:"ok"`
			Task struct {
				Status string `json:"status"`
				Error  string `json:"error"`
			} `json:"task"`
		}
		_ = json.Unmarshal(out, &tr)
		if tr.Task.Status == "succeeded" {
			return
		}
		if tr.Task.Status == "failed" || tr.Task.Status == "cancelled" {
			r.add(name+" task failed", false, string(out), fmt.Errorf(tr.Task.Error))
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	r.add(name+" task timeout", false, "", fmt.Errorf("task did not finish"))
}

func (r *ueAPIRunner) add(name string, passed bool, output string, err error) {
	res := Result{Name: name, ExitOK: err == nil, Passed: passed, Output: output}
	if err != nil {
		res.Error = err.Error()
	}
	r.report.Steps = append(r.report.Steps, res)
	if !passed {
		r.failed = true
	}
}
func (r *ueAPIRunner) section(name string) {
	r.report.Steps = append(r.report.Steps, Result{Name: "== " + name + " ==", ExitOK: true, Passed: true})
}
func (r *ueAPIRunner) finish() {
	r.report.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	r.report.Passed = !r.failed
	data, _ := json.MarshalIndent(r.report, "", "  ")
	_ = os.WriteFile(r.reportPath, []byte("# GameDepot UE API Smoke Report\n\n```json\n"+string(data)+"\n```\n"), 0o644)
}
