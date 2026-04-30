package smoke

import (
	"bytes"
	"context"
	"encoding/json"
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

type V08APIOptions struct {
	Workspace string
	Clean     bool
	Keep      bool
	ProjectID string
}

func RegisterV08APIFlags(fs interface {
	StringVar(*string, string, string, string)
	BoolVar(*bool, string, bool, string)
}, opts *V08APIOptions) {
	fs.StringVar(&opts.Workspace, "workspace", filepath.Join(os.TempDir(), "gamedepot-v08-api-smoke"), "smoke workspace")
	fs.BoolVar(&opts.Clean, "clean", true, "remove the smoke workspace before running")
	fs.BoolVar(&opts.Keep, "keep", true, "keep generated workspace for inspection")
	fs.StringVar(&opts.ProjectID, "project", "V08APISmokeProject", "simulated project id/name")
}

type v08APIRunner struct {
	opts            V08APIOptions
	exe             string
	workspace       string
	root            string
	globalConfigDir string
	env             []string
	daemon          *exec.Cmd
	apiBase         string
	steps           []Result
	failed          bool
}

func RunV08API(ctx context.Context, opts V08APIOptions) error {
	if opts.Workspace == "" {
		opts.Workspace = filepath.Join(os.TempDir(), "gamedepot-v08-api-smoke")
	}
	if opts.ProjectID == "" {
		opts.ProjectID = "V08APISmokeProject"
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return err
	}
	workspace, err := filepath.Abs(opts.Workspace)
	if err != nil {
		return err
	}
	r := &v08APIRunner{opts: opts, exe: exe, workspace: workspace, root: filepath.Join(workspace, "UEProject"), globalConfigDir: filepath.Join(workspace, "_global_config")}
	r.env = append(os.Environ(), config.EnvConfigDir+"="+r.globalConfigDir)
	return r.run(ctx)
}

func (r *v08APIRunner) run(ctx context.Context) error {
	if r.opts.Clean {
		_ = os.RemoveAll(r.workspace)
	}
	if err := os.MkdirAll(r.root, 0o755); err != nil {
		return err
	}
	defer func() {
		if r.daemon != nil && r.daemon.Process != nil {
			_ = r.daemon.Process.Kill()
		}
		if !r.opts.Keep {
			_ = os.RemoveAll(r.workspace)
		}
	}()

	r.cmd(ctx, "git init", r.root, false, "git", "init")
	r.cmd(ctx, "git branch main", r.root, false, "git", "branch", "-M", "main")
	r.cmd(ctx, "git config user.email", r.root, false, "git", "config", "user.email", "v08-api@example.com")
	r.cmd(ctx, "git config user.name", r.root, false, "git", "config", "user.name", "V08 API Smoke")
	r.cmd(ctx, "git config core.autocrlf false", r.root, false, "git", "config", "core.autocrlf", "false")
	r.cmd(ctx, "git config core.eol lf", r.root, false, "git", "config", "core.eol", "lf")
	r.gd(ctx, "init", r.root, false, "init", "--project", r.opts.ProjectID, "--template", "ue5")

	r.write("Content/Hero.uasset", "hero-v1")
	r.write("Config/DefaultGame.ini", "[/Script/Game]\nName=V08API\n")
	r.write("Content/Data/Table.json", "{\"version\":1}\n")
	r.write("Content/Unknown.custom", "custom-v1\n")

	if err := r.startDaemon(ctx); err != nil {
		r.add("start daemon", false, "", err)
		return r.finish()
	}
	r.add("start daemon", true, r.apiBase, nil)

	r.mustHTTP(ctx, "GET health", "GET", "/api/ue/v1/health", nil)
	r.mustHTTP(ctx, "GET config", "GET", "/api/ue/v1/config", nil)
	r.mustHTTP(ctx, "POST config identity", "POST", "/api/ue/v1/config", map[string]any{"user": map[string]any{"identity": "v08-api-user"}})
	r.mustHTTP(ctx, "GET overview", "GET", "/api/ue/v1/overview", nil)
	r.mustHTTP(ctx, "GET rules", "GET", "/api/ue/v1/rules", nil)

	status := r.mustHTTP(ctx, "GET assets/status new files", "GET", "/api/ue/v1/assets/status?include_history=true", nil)
	r.assertStatusContains(status, "Content/Hero.uasset", "new")
	r.assertStatusContains(status, "Content/Unknown.custom", "review")

	failTask := r.mustHTTP(ctx, "POST submit blocked by review", "POST", "/api/ue/v1/submit", map[string]any{"message": "should fail because review exists"})
	r.waitTask(ctx, "submit blocked task failed", failTask, "failed")

	r.mustHTTP(ctx, "POST rule upsert raw custom blob", "POST", "/api/ue/v1/rules/upsert", map[string]any{"id": "manual_custom_blob", "pattern": "Content/Unknown.custom", "mode": "blob", "scope": "exact"})
	r.mustHTTP(ctx, "POST rule reorder", "POST", "/api/ue/v1/rules/reorder", map[string]any{"id": "manual_custom_blob", "direction": "down"})
	r.mustHTTP(ctx, "POST rule delete missing expected failure", "POST", "/api/ue/v1/rules/delete", map[string]any{"id": "does_not_exist"}, expectFailure())

	okTask := r.mustHTTP(ctx, "POST submit initial", "POST", "/api/ue/v1/submit", map[string]any{"message": "v08 api initial"})
	r.waitTask(ctx, "initial submit task", okTask, "succeeded")

	r.write("Content/Data/Table.json", "{\"version\":2}\n")
	r.mustHTTP(ctx, "POST rule upsert table blob", "POST", "/api/ue/v1/rules/upsert", map[string]any{"paths": []string{"Content/Data/Table.json"}, "mode": "blob", "scope": "exact"})
	gitToBlob := r.mustHTTP(ctx, "POST submit git to blob", "POST", "/api/ue/v1/submit", map[string]any{"message": "v08 api table git to blob"})
	r.waitTask(ctx, "git to blob task", gitToBlob, "succeeded")

	r.write("Content/Data/Table.json", "{\"version\":3}\n")
	r.mustHTTP(ctx, "POST rule upsert table git", "POST", "/api/ue/v1/rules/upsert", map[string]any{"paths": []string{"Content/Data/Table.json"}, "mode": "git", "scope": "exact"})
	blobToGit := r.mustHTTP(ctx, "POST submit blob to git", "POST", "/api/ue/v1/submit", map[string]any{"message": "v08 api table blob to git"})
	r.waitTask(ctx, "blob to git task", blobToGit, "succeeded")

	history := r.mustHTTP(ctx, "POST asset history", "POST", "/api/ue/v1/assets/history", map[string]any{"path": "Content/Data/Table.json"})
	gitCommit, blobCommit := r.pickHistoryCommits(history)
	if gitCommit == "" || blobCommit == "" {
		r.add("history contains git and blob commits", false, string(history), fmt.Errorf("missing git/blob history commits"))
	} else {
		r.add("history contains git and blob commits", true, gitCommit+" / "+blobCommit, nil)
	}

	if gitCommit != "" {
		restoreGit := r.mustHTTP(ctx, "POST restore git history", "POST", "/api/ue/v1/assets/restore", map[string]any{"path": "Content/Data/Table.json", "commit": gitCommit, "force": true})
		r.waitTask(ctx, "restore git history task", restoreGit, "succeeded")
		r.assertFile("restore git history content", "Content/Data/Table.json", "{\"version\":1}\n")
	}
	if blobCommit != "" {
		restoreBlob := r.mustHTTP(ctx, "POST restore blob history", "POST", "/api/ue/v1/assets/restore", map[string]any{"path": "Content/Data/Table.json", "commit": blobCommit, "force": true})
		r.waitTask(ctx, "restore blob history task", restoreBlob, "succeeded")
		r.assertFile("restore blob history content", "Content/Data/Table.json", "{\"version\":2}\n")
	}

	r.mustHTTP(ctx, "POST revert asset", "POST", "/api/ue/v1/assets/revert", map[string]any{"paths": []string{"Content/Data/Table.json"}, "force": true})
	r.assertFile("revert current content", "Content/Data/Table.json", "{\"version\":3}\n")

	r.mustHTTP(ctx, "POST lock acquire", "POST", "/api/ue/v1/locks/acquire", map[string]any{"paths": []string{"Content/Hero.uasset"}, "note": "api smoke"})
	locksOut := r.mustHTTP(ctx, "GET locks status", "GET", "/api/ue/v1/locks/status?path=Content/Hero.uasset", nil)
	r.assertJSONListNonEmpty("locks status non-empty", locksOut, "locks")
	r.mustHTTP(ctx, "POST lock release", "POST", "/api/ue/v1/locks/release", map[string]any{"paths": []string{"Content/Hero.uasset"}, "force": true})

	syncTask := r.mustHTTP(ctx, "POST sync no pull", "POST", "/api/ue/v1/sync", map[string]any{"force": true, "pull_git": false})
	r.waitTask(ctx, "sync no pull task", syncTask, "succeeded")
	r.mustHTTP(ctx, "POST admin shutdown", "POST", "/api/ue/v1/admin/shutdown", map[string]any{})
	return r.finish()
}

type expectFailureMarker struct{}

func expectFailure() expectFailureMarker { return expectFailureMarker{} }

func (r *v08APIRunner) mustHTTP(ctx context.Context, name, method, path string, body any, markers ...expectFailureMarker) []byte {
	expectFailure := len(markers) > 0
	out, ok := r.http(ctx, name, method, path, body, expectFailure)
	if !ok && !expectFailure {
		return out
	}
	return out
}

func (r *v08APIRunner) http(ctx context.Context, name, method, path string, body any, expectFailure bool) ([]byte, bool) {
	start := time.Now()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, r.apiBase+path, reader)
	if err != nil {
		r.add(name, false, "", err)
		return nil, false
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	out := []byte{}
	exitOK := false
	if err == nil {
		out, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		exitOK = resp.StatusCode >= 200 && resp.StatusCode < 300
	} else {
		out = []byte(err.Error())
	}
	passed := exitOK != expectFailure
	res := Result{Name: name, Command: method + " " + path, Dir: r.root, ExpectFailure: expectFailure, ExitOK: exitOK, Passed: passed, DurationMS: time.Since(start).Milliseconds(), Output: string(out)}
	if err != nil {
		res.Error = err.Error()
	}
	r.steps = append(r.steps, res)
	if !passed {
		r.failed = true
	}
	return out, passed
}

func (r *v08APIRunner) waitTask(ctx context.Context, name string, createResp []byte, want string) {
	var v struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(createResp, &v); err != nil || v.TaskID == "" {
		r.add(name, false, string(createResp), fmt.Errorf("missing task_id"))
		return
	}
	deadline := time.Now().Add(20 * time.Second)
	last := []byte{}
	for time.Now().Before(deadline) {
		out, _ := r.http(ctx, name+" poll", "GET", "/api/ue/v1/tasks/"+v.TaskID, nil, false)
		last = out
		var tr struct {
			Task struct {
				Status string `json:"status"`
				Error  string `json:"error"`
			} `json:"task"`
		}
		_ = json.Unmarshal(out, &tr)
		if tr.Task.Status == want {
			r.add(name, true, string(out), nil)
			return
		}
		if tr.Task.Status == "failed" || tr.Task.Status == "cancelled" {
			if want == tr.Task.Status {
				r.add(name, true, string(out), nil)
			} else {
				r.add(name, false, string(out), fmt.Errorf("task status %s: %s", tr.Task.Status, tr.Task.Error))
			}
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	r.add(name, false, string(last), fmt.Errorf("task did not reach %s", want))
}

func (r *v08APIRunner) pickHistoryCommits(data []byte) (gitCommit, blobCommit string) {
	var v struct {
		Versions []struct {
			Commit  string `json:"commit"`
			Storage string `json:"storage"`
			Deleted bool   `json:"deleted"`
		} `json:"versions"`
	}
	_ = json.Unmarshal(data, &v)
	for i := len(v.Versions) - 1; i >= 0; i-- {
		it := v.Versions[i]
		if it.Deleted {
			continue
		}
		switch it.Storage {
		case "git":
			if gitCommit == "" {
				gitCommit = it.Commit
			}
		case "blob":
			if blobCommit == "" {
				blobCommit = it.Commit
			}
		}
	}
	return gitCommit, blobCommit
}

func (r *v08APIRunner) assertStatusContains(data []byte, path string, status string) {
	var v struct {
		Assets []struct {
			Path   string `json:"path"`
			Status string `json:"status"`
		} `json:"assets"`
	}
	_ = json.Unmarshal(data, &v)
	for _, it := range v.Assets {
		if it.Path == path && it.Status == status {
			r.add("status contains "+path+" "+status, true, "", nil)
			return
		}
	}
	r.add("status contains "+path+" "+status, false, string(data), fmt.Errorf("status not found"))
}

func (r *v08APIRunner) assertJSONListNonEmpty(name string, data []byte, key string) {
	var v map[string]any
	_ = json.Unmarshal(data, &v)
	list, _ := v[key].([]any)
	if len(list) > 0 {
		r.add(name, true, "", nil)
		return
	}
	r.add(name, false, string(data), fmt.Errorf("%s empty", key))
}

func (r *v08APIRunner) assertFile(name, rel, want string) {
	data, err := os.ReadFile(filepath.Join(r.root, filepath.FromSlash(rel)))
	if err != nil {
		r.add(name, false, "", err)
		return
	}
	got := strings.ReplaceAll(string(data), "\r\n", "\n")
	if got != want {
		r.add(name, false, got, fmt.Errorf("want %q", want))
		return
	}
	r.add(name, true, got, nil)
}

func (r *v08APIRunner) startDaemon(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, r.exe, "daemon", "--root", r.root, "--addr", "127.0.0.1:0", "--started-by", "v08-api-smoke")
	cmd.Env = r.env
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Start(); err != nil {
		return err
	}
	r.daemon = cmd
	runtimePath := filepath.Join(r.root, ".gamedepot", "runtime", "daemon.json")
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(runtimePath)
		if err == nil {
			var v struct {
				Addr string `json:"addr"`
			}
			if json.Unmarshal(data, &v) == nil && v.Addr != "" {
				r.apiBase = "http://" + v.Addr
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("daemon runtime file not written: %s", runtimePath)
}

func (r *v08APIRunner) cmd(ctx context.Context, name, dir string, expectFailure bool, exe string, args ...string) {
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
	r.steps = append(r.steps, res)
	if !passed {
		r.failed = true
	}
}

func (r *v08APIRunner) gd(ctx context.Context, name, dir string, expectFailure bool, args ...string) {
	r.cmd(ctx, name, dir, expectFailure, r.exe, args...)
}

func (r *v08APIRunner) write(rel, body string) {
	p := filepath.Join(r.root, filepath.FromSlash(rel))
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(body), 0o644)
}

func (r *v08APIRunner) add(name string, passed bool, output string, err error) {
	res := Result{Name: name, ExitOK: err == nil, Passed: passed, Output: output}
	if err != nil {
		res.Error = err.Error()
	}
	r.steps = append(r.steps, res)
	if !passed {
		r.failed = true
	}
}

func (r *v08APIRunner) finish() error {
	data, _ := json.MarshalIndent(map[string]any{"passed": !r.failed, "workspace": r.workspace, "project_root": r.root, "api_base": r.apiBase, "steps": r.steps}, "", "  ")
	_ = os.WriteFile(filepath.Join(r.workspace, "v08_api_smoke_report.json"), data, 0o644)
	if r.failed {
		fmt.Println(string(data))
		return fmt.Errorf("v08 api smoke failed; see %s", filepath.Join(r.workspace, "v08_api_smoke_report.json"))
	}
	fmt.Println("V08 API smoke PASS")
	fmt.Println("Workspace:", r.workspace)
	return nil
}
