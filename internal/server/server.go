package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/1051093860/gamedepot/internal/app"
	"github.com/1051093860/gamedepot/internal/commands"
	gdgit "github.com/1051093860/gamedepot/internal/git"
	"github.com/1051093860/gamedepot/internal/manifest"
	"github.com/1051093860/gamedepot/internal/store"
)

type Options struct {
	Root    string
	Addr    string
	Token   string
	Version string
}

type Server struct {
	root    string
	addr    string
	token   string
	version string
	mu      sync.Mutex
}

type Response struct {
	OK     bool           `json:"ok"`
	Data   any            `json:"data,omitempty"`
	Output string         `json:"output,omitempty"`
	Error  string         `json:"error,omitempty"`
	Meta   map[string]any `json:"meta,omitempty"`
}

func New(opts Options) *Server {
	root := opts.Root
	if root == "" {
		root = "."
	}
	addr := opts.Addr
	if addr == "" {
		addr = "127.0.0.1:17320"
	}
	return &Server{root: root, addr: addr, token: opts.Token, version: opts.Version}
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	h := s.Handler()
	srv := &http.Server{
		Addr:              s.addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       120 * time.Second,
		WriteTimeout:      120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return ctx.Err()
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", s.handleHealth)
	mux.HandleFunc("/api/v1/version", s.handleVersion)
	mux.HandleFunc("/api/v1/store", s.handleStoreInfo)
	mux.HandleFunc("/api/v1/store/check", s.handleStoreCheck)
	mux.HandleFunc("/api/v1/status", s.handleStatus)
	mux.HandleFunc("/api/v1/classify", s.handleClassify)
	mux.HandleFunc("/api/v1/manifest", s.handleManifest)
	mux.HandleFunc("/api/v1/locks", s.handleLocks)
	mux.HandleFunc("/api/v1/history", s.handleHistory)
	mux.HandleFunc("/api/v1/git/status", s.handleGitStatus)
	mux.HandleFunc("/api/v1/submit", s.handleSubmit)
	mux.HandleFunc("/api/v1/sync", s.handleSync)
	mux.HandleFunc("/api/v1/restore", s.handleRestore)
	mux.HandleFunc("/api/v1/lock", s.handleLock)
	mux.HandleFunc("/api/v1/unlock", s.handleUnlock)
	mux.HandleFunc("/api/v1/verify", s.handleVerify)
	mux.HandleFunc("/api/v1/gc", s.handleGC)
	mux.HandleFunc("/api/v1/delete-version", s.handleDeleteVersion)
	return s.withMiddleware(mux)
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://127.0.0.1")
		w.Header().Set("Access-Control-Allow-Headers", "authorization, content-type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if s.token != "" {
			want := "Bearer " + s.token
			if r.Header.Get("Authorization") != want {
				writeJSON(w, http.StatusUnauthorized, Response{OK: false, Error: "unauthorized"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	a, err := app.Load(r.Context(), s.root)
	data := map[string]any{"status": "ok", "version": s.version, "root": s.root}
	if err == nil {
		data["project_id"] = a.Config.ProjectID
		data["store"] = a.StoreInfo
	} else {
		data["project_error"] = err.Error()
	}
	writeOK(w, data, "")
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeOK(w, map[string]string{"version": s.version}, "")
}

func (s *Server) handleStoreInfo(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	a, err := app.Load(r.Context(), s.root)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, a.StoreInfo, "")
}

func (s *Server) handleStoreCheck(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	a, err := app.Load(r.Context(), s.root)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := store.Check(r.Context(), a.Store); err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, map[string]string{"status": "ok"}, "")
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	s.runJSONCommand(w, r, func() error { return commands.Status(r.Context(), s.root, true) })
}

func (s *Server) handleClassify(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	target := r.URL.Query().Get("target")
	all := parseBool(r.URL.Query().Get("all"))
	s.runJSONCommand(w, r, func() error { return commands.Classify(r.Context(), s.root, target, true, all) })
}

func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	a, err := app.Load(r.Context(), s.root)
	if err != nil {
		writeErr(w, err)
		return
	}
	m, err := manifest.Load(a.ManifestPath)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, m, "")
}

func (s *Server) handleLocks(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	s.runJSONCommand(w, r, func() error { return commands.Locks(r.Context(), s.root, true) })
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	target := r.URL.Query().Get("path")
	if target == "" {
		writeErr(w, fmt.Errorf("path query parameter is required"))
		return
	}
	s.runTextCommand(w, r, func() error { return commands.History(r.Context(), s.root, target) })
}

func (s *Server) handleGitStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	a, err := app.Load(r.Context(), s.root)
	if err != nil {
		writeErr(w, err)
		return
	}
	out, err := gdgit.New(a.Root).StatusPorcelain()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, map[string]string{"porcelain": out}, "")
}

func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req struct {
		Message string `json:"message"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	s.runTextCommand(w, r, func() error { return commands.Submit(r.Context(), s.root, req.Message) })
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req struct {
		Force bool `json:"force"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	s.runTextCommand(w, r, func() error { return commands.Sync(r.Context(), s.root, req.Force) })
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
		Force  bool   `json:"force"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	s.runTextCommand(w, r, func() error { return commands.Restore(r.Context(), s.root, req.Path, req.SHA256, req.Force) })
}

func (s *Server) handleLock(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req struct {
		Path         string `json:"path"`
		Owner        string `json:"owner"`
		Host         string `json:"host"`
		Note         string `json:"note"`
		Force        bool   `json:"force"`
		AllowNonBlob bool   `json:"allow_non_blob"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	opts := commands.LockOptions{Owner: req.Owner, Host: req.Host, Note: req.Note, Force: req.Force, AllowNonBlob: req.AllowNonBlob}
	s.runTextCommand(w, r, func() error { return commands.Lock(r.Context(), s.root, req.Path, opts) })
}

func (s *Server) handleUnlock(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req struct {
		Path  string `json:"path"`
		Owner string `json:"owner"`
		Host  string `json:"host"`
		Force bool   `json:"force"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	s.runTextCommand(w, r, func() error { return commands.Unlock(r.Context(), s.root, req.Path, req.Owner, req.Host, req.Force) })
}

func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req struct {
		LocalOnly  bool `json:"local_only"`
		RemoteOnly bool `json:"remote_only"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	s.runTextCommand(w, r, func() error {
		return commands.VerifyWithOptions(r.Context(), s.root, commands.VerifyOptions{LocalOnly: req.LocalOnly, RemoteOnly: req.RemoteOnly})
	})
}

func (s *Server) handleGC(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req struct {
		DryRun         *bool    `json:"dry_run"`
		JSON           bool     `json:"json"`
		ProtectTags    []string `json:"protect_tags"`
		ProtectAllTags bool     `json:"protect_all_tags"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	dry := true
	if req.DryRun != nil {
		dry = *req.DryRun
	}
	opts := commands.GCOptions{DryRun: dry, JSON: req.JSON, ProtectTags: req.ProtectTags, ProtectAllTags: req.ProtectAllTags}
	if req.JSON {
		s.runJSONCommand(w, r, func() error { return commands.GC(r.Context(), s.root, opts) })
		return
	}
	s.runTextCommand(w, r, func() error { return commands.GC(r.Context(), s.root, opts) })
}

func (s *Server) handleDeleteVersion(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req struct {
		Path         string `json:"path"`
		SHA256       string `json:"sha256"`
		DryRun       *bool  `json:"dry_run"`
		JSON         bool   `json:"json"`
		ForceCurrent bool   `json:"force_current"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	dry := true
	if req.DryRun != nil {
		dry = *req.DryRun
	}
	opts := commands.DeleteVersionOptions{SHA256: req.SHA256, DryRun: dry, JSON: req.JSON, ForceCurrent: req.ForceCurrent}
	if req.JSON {
		s.runJSONCommand(w, r, func() error { return commands.DeleteVersion(r.Context(), s.root, req.Path, opts) })
		return
	}
	s.runTextCommand(w, r, func() error { return commands.DeleteVersion(r.Context(), s.root, req.Path, opts) })
}

func (s *Server) runTextCommand(w http.ResponseWriter, r *http.Request, fn func() error) {
	out, err := s.capture(fn)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, Response{OK: false, Output: out, Error: err.Error()})
		return
	}
	writeOK(w, nil, out)
}

func (s *Server) runJSONCommand(w http.ResponseWriter, r *http.Request, fn func() error) {
	out, err := s.capture(fn)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, Response{OK: false, Output: out, Error: err.Error()})
		return
	}
	var data any
	if strings.TrimSpace(out) != "" {
		if err := json.Unmarshal([]byte(out), &data); err != nil {
			writeJSON(w, http.StatusInternalServerError, Response{OK: false, Output: out, Error: "command did not return valid JSON: " + err.Error()})
			return
		}
	}
	writeOK(w, data, out)
}

func (s *Server) capture(fn func() error) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	runErr := fn()
	_ = w.Close()
	os.Stdout = old
	out := <-done
	_ = r.Close()
	return out, runErr
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, Response{OK: false, Error: "invalid JSON: " + err.Error()})
		return false
	}
	return true
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		writeJSON(w, http.StatusMethodNotAllowed, Response{OK: false, Error: "method not allowed"})
		return false
	}
	return true
}

func writeOK(w http.ResponseWriter, data any, output string) {
	writeJSON(w, http.StatusOK, Response{OK: true, Data: data, Output: output})
}

func writeErr(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, Response{OK: false, Error: err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, v Response) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func parseBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "1" || s == "true" || s == "yes" || s == "y" || s == "on"
}
