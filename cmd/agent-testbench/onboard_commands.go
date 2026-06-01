package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type onboardReport struct {
	OK      bool                 `json:"ok"`
	Repo    string               `json:"repo"`
	Store   setupStoreReport     `json:"store"`
	Runtime setupRuntimeReport   `json:"runtime"`
	Shell   onboardShellReport   `json:"shell"`
	Checks  []onboardCheckReport `json:"checks"`
	Smoke   onboardSmokeReport   `json:"smoke"`
	Next    []string             `json:"next"`
}

type onboardShellReport struct {
	Installed    bool   `json:"installed"`
	EntryPath    string `json:"entryPath,omitempty"`
	TargetPath   string `json:"targetPath,omitempty"`
	OnPath       bool   `json:"onPath"`
	TargetExists bool   `json:"targetExists"`
	Error        string `json:"error,omitempty"`
}

type onboardCheckReport struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

type onboardSmokeReport struct {
	Mode   string `json:"mode"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

const (
	onboardSmokeNone     = "none"
	onboardSmokeCommands = "commands"
	onboardSmokeStore    = "store"
)

func runOnboard(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("onboard", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	repoFlag := flags.String("repo", "", "AgentTestBench git checkout to configure")
	storeName := flags.String("store", "local", "Local Store config name to create or update")
	storeURL := flags.String("url", "", "PostgreSQL, MySQL, or SQLite Store DSN")
	sqlitePath := flags.String("sqlite", "", "SQLite Store path")
	buildRuntime := flags.Bool("build-runtime", true, "Build the local runtime binary into REPO/.runtime/bin")
	installShell := flags.Bool("install-shell", false, "Install an agent-testbench shell entrypoint")
	binDir := flags.String("bin-dir", filepath.Join(userHomeDir(), ".local", "bin"), "Directory for the shell entrypoint")
	smoke := flags.String("smoke", onboardSmokeCommands, "Smoke mode: none, commands, or store")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable onboard report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected onboard arguments: %s", strings.Join(flags.Args(), " "))
	}
	report, err := buildOnboardReport(ctx, onboardOptions{
		Repo:         *repoFlag,
		StoreName:    *storeName,
		StoreURL:     *storeURL,
		SQLitePath:   *sqlitePath,
		BuildRuntime: *buildRuntime,
		InstallShell: *installShell,
		BinDir:       *binDir,
		Smoke:        *smoke,
	})
	if *jsonOutput {
		if writeErr := writeIndentedJSON(report); writeErr != nil {
			return writeErr
		}
	}
	if err != nil {
		return err
	}
	if !*jsonOutput {
		printOnboardReport(report)
	}
	return nil
}

type onboardOptions struct {
	Repo         string
	StoreName    string
	StoreURL     string
	SQLitePath   string
	BuildRuntime bool
	InstallShell bool
	BinDir       string
	Smoke        string
}

func buildOnboardReport(ctx context.Context, opts onboardOptions) (onboardReport, error) {
	smokeMode := strings.ToLower(strings.TrimSpace(stringDefault(opts.Smoke, onboardSmokeCommands)))
	if smokeMode != onboardSmokeNone && smokeMode != onboardSmokeCommands && smokeMode != onboardSmokeStore {
		return onboardReport{OK: false, Smoke: onboardSmokeReport{Mode: smokeMode}}, fmt.Errorf("unsupported onboard smoke mode %q; use none, commands, or store", smokeMode)
	}
	setup, err := setupLocalRuntime(ctx, setupOptions{
		Repo:         opts.Repo,
		StoreName:    opts.StoreName,
		StoreURL:     opts.StoreURL,
		SQLitePath:   opts.SQLitePath,
		BuildRuntime: opts.BuildRuntime,
	})
	report := onboardReport{
		OK:      setup.OK,
		Repo:    setup.Repo,
		Store:   setup.Store,
		Runtime: setup.Runtime,
		Next: append([]string{
			"agent-testbench task list",
			"agent-testbench task run catalog-smoke --command \"commands --json\"",
		}, setup.Next...),
	}
	if err != nil {
		report.OK = false
		return report, err
	}
	if opts.InstallShell {
		report.Shell = installOnboardShell(opts.BinDir, setup.Runtime.Path)
		if report.Shell.Error != "" {
			report.OK = false
			return report, fmt.Errorf("install shell entrypoint: %s", report.Shell.Error)
		}
	} else {
		report.Shell = inspectOnboardShell(setup.Runtime.Path)
	}
	report.Checks = append(report.Checks,
		onboardCheckReport{Name: "store-config", OK: setup.Store.Active, Detail: setup.Store.Name + " (" + setup.Store.Backend + ")"},
		onboardCheckReport{Name: "runtime-path", OK: setup.Runtime.Path != "", Detail: setup.Runtime.Path},
	)
	report.Smoke = runOnboardSmoke(ctx, smokeMode, setup.Store.Name)
	report.OK = report.OK && report.Smoke.OK
	if !report.Smoke.OK {
		return report, fmt.Errorf("onboard smoke %s failed: %s", report.Smoke.Mode, report.Smoke.Detail)
	}
	return report, nil
}

func installOnboardShell(binDir string, runtimePath string) onboardShellReport {
	binDir = strings.TrimSpace(binDir)
	if binDir == "" {
		binDir = filepath.Join(userHomeDir(), ".local", "bin")
	}
	entry := filepath.Join(binDir, "agent-testbench")
	report := onboardShellReport{Installed: false, EntryPath: entry, TargetPath: runtimePath, OnPath: directoryOnPath(binDir)}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		report.Error = err.Error()
		return report
	}
	if info, err := os.Lstat(entry); err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			report.Error = fmt.Sprintf("existing shell entrypoint %s is not a symlink; remove it or choose another --bin-dir", entry)
			return report
		}
		if err := os.Remove(entry); err != nil {
			report.Error = err.Error()
			return report
		}
	} else if !os.IsNotExist(err) {
		report.Error = err.Error()
		return report
	}
	if err := os.Symlink(runtimePath, entry); err != nil {
		report.Error = err.Error()
		return report
	}
	report.Installed = true
	if info, err := os.Stat(runtimePath); err == nil && !info.IsDir() {
		report.TargetExists = true
	}
	return report
}

func inspectOnboardShell(runtimePath string) onboardShellReport {
	report := onboardShellReport{TargetPath: runtimePath}
	if found, err := os.Executable(); err == nil {
		report.EntryPath = found
	}
	if info, err := os.Stat(runtimePath); err == nil && !info.IsDir() {
		report.TargetExists = true
	}
	if path, err := execLookPath("agent-testbench"); err == nil {
		report.OnPath = true
		report.EntryPath = path
	}
	return report
}

var execLookPath = func(file string) (string, error) {
	return exec.LookPath(file)
}

func directoryOnPath(dir string) bool {
	dir = filepath.Clean(strings.TrimSpace(dir))
	for _, item := range filepath.SplitList(os.Getenv("PATH")) {
		if filepath.Clean(item) == dir {
			return true
		}
	}
	return false
}

func runOnboardSmoke(ctx context.Context, mode string, storeName string) onboardSmokeReport {
	switch mode {
	case onboardSmokeNone:
		return onboardSmokeReport{Mode: mode, OK: true}
	case onboardSmokeCommands:
		report := commandCatalog(cliCommandTask)
		return onboardSmokeReport{Mode: mode, OK: report.Count > 0, Detail: fmt.Sprintf("%d task commands indexed", report.Count)}
	case onboardSmokeStore:
		storeURL, err := resolveRequiredDailyStoreReference(storeName, "")
		if err != nil {
			return onboardSmokeReport{Mode: mode, OK: false, Detail: err.Error()}
		}
		runtime, err := openStore(ctx, storeURL)
		if err != nil {
			return onboardSmokeReport{Mode: mode, OK: false, Detail: err.Error()}
		}
		closeCLIStore(runtime)
		return onboardSmokeReport{Mode: mode, OK: true, Detail: "store opened and upgraded"}
	default:
		return onboardSmokeReport{Mode: mode, OK: false, Detail: "unsupported smoke mode"}
	}
}

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "."
	}
	return home
}

func printOnboardReport(report onboardReport) {
	fmt.Println("AgentTestBench Onboard")
	fmt.Printf("Repo: %s\n", report.Repo)
	fmt.Printf("Store: %s (%s)\n", report.Store.Name, report.Store.Backend)
	fmt.Printf("Runtime: %s\n", report.Runtime.Path)
	if report.Shell.EntryPath != "" {
		fmt.Printf("Shell: %s\n", report.Shell.EntryPath)
	}
	fmt.Printf("Smoke: %s ok=%t\n", report.Smoke.Mode, report.Smoke.OK)
	printNextActions(report.Next)
}
