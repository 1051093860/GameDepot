package ueplugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type SmokeOptions struct {
	Workspace string
	Report    string
	Project   string
	Clean     bool
}

func RunSmoke(opts SmokeOptions) error {
	if opts.Workspace == "" {
		opts.Workspace = "GameDepot_UEPluginSmokeWorkspace"
	}
	if opts.Report == "" {
		opts.Report = "gamedepot_ue_plugin_smoke_report.md"
	}
	if opts.Project == "" {
		opts.Project = "FakeUEProject"
	}
	ws, _ := filepath.Abs(opts.Workspace)
	if opts.Clean {
		_ = os.RemoveAll(ws)
	}
	projectRoot := filepath.Join(ws, opts.Project)
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		return err
	}
	uproject := filepath.Join(projectRoot, opts.Project+".uproject")
	if err := os.WriteFile(uproject, []byte(`{"FileVersion":3,"EngineAssociation":"5.0","Category":"","Description":""}`), 0o644); err != nil {
		return err
	}
	steps := []string{}
	add := func(name string, err error) error {
		status := "PASS"
		msg := ""
		if err != nil {
			status = "FAIL"
			msg = err.Error()
		}
		steps = append(steps, fmt.Sprintf("| %s | %s | %s |", name, status, strings.ReplaceAll(msg, "|", "\\|")))
		return err
	}
	if err := add("install", Install(InstallOptions{Project: projectRoot, Overwrite: true, EnablePlugin: true, WriteProjectConfig: true})); err != nil {
		return writeSmokeReport(opts.Report, ws, projectRoot, steps, err)
	}
	if err := add("verify", Verify(projectRoot)); err != nil {
		return writeSmokeReport(opts.Report, ws, projectRoot, steps, err)
	}
	if err := add("write low-memory UBT config", WriteUBTConfig(projectRoot, true)); err != nil {
		return writeSmokeReport(opts.Report, ws, projectRoot, steps, err)
	}
	if err := add("list", List(projectRoot)); err != nil {
		return writeSmokeReport(opts.Report, ws, projectRoot, steps, err)
	}
	return writeSmokeReport(opts.Report, ws, projectRoot, steps, nil)
}

func writeSmokeReport(report, ws, projectRoot string, steps []string, finalErr error) error {
	if report == "" {
		report = filepath.Join(ws, "gamedepot_ue_plugin_smoke_report.md")
	}
	reportAbs, _ := filepath.Abs(report)
	result := "PASS"
	if finalErr != nil {
		result = "FAIL"
	}
	content := strings.Builder{}
	content.WriteString("# GameDepot UE Plugin Smoke Test\n\n")
	content.WriteString("- Time: " + time.Now().Format(time.RFC3339) + "\n")
	content.WriteString("- Result: " + result + "\n")
	content.WriteString("- Workspace: `" + ws + "`\n")
	content.WriteString("- Project: `" + projectRoot + "`\n\n")
	content.WriteString("| Step | Result | Message |\n|---|---:|---|\n")
	for _, s := range steps {
		content.WriteString(s + "\n")
	}
	if err := os.WriteFile(reportAbs, []byte(content.String()), 0o644); err != nil {
		return err
	}
	fmt.Println("UE plugin smoke test result:", result)
	fmt.Println("report:", reportAbs)
	return finalErr
}
