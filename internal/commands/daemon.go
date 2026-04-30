package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type DaemonStopOptions struct {
	Kill bool
}

type daemonRuntimeInfo struct {
	PID         int    `json:"pid"`
	Addr        string `json:"addr"`
	Token       string `json:"token"`
	ProjectRoot string `json:"project_root"`
	Version     string `json:"version"`
}

func DaemonStop(ctx context.Context, root string, opts DaemonStopOptions) error {
	runtimePath := filepath.Join(root, ".gamedepot", "runtime", "daemon.json")
	data, err := os.ReadFile(runtimePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var info daemonRuntimeInfo
	if err := json.Unmarshal(data, &info); err != nil {
		if opts.Kill {
			_ = os.Remove(runtimePath)
			return nil
		}
		return fmt.Errorf("invalid daemon runtime file %s: %w", runtimePath, err)
	}

	stopped := false
	if strings.TrimSpace(info.Addr) != "" {
		reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, "http://"+info.Addr+"/api/ue/v1/admin/shutdown", bytes.NewBufferString("{}"))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			if info.Token != "" {
				req.Header.Set("Authorization", "Bearer "+info.Token)
			}
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				body, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					stopped = true
				} else if !opts.Kill {
					return fmt.Errorf("daemon shutdown returned HTTP %d: %s", resp.StatusCode, string(body))
				}
			} else if !opts.Kill {
				return err
			}
		} else if !opts.Kill {
			return err
		}
	}

	if !stopped && opts.Kill && info.PID > 0 {
		if p, err := os.FindProcess(info.PID); err == nil {
			_ = p.Kill()
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(runtimePath); os.IsNotExist(err) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = os.Remove(runtimePath)
	return nil
}
