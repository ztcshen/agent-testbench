package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-testbench/internal/store/mysql"
	"agent-testbench/internal/store/postgres"
	"agent-testbench/internal/store/sqlite"
)

type statusCommandReport struct {
	OK      bool                `json:"ok"`
	Version string              `json:"version"`
	Repo    statusRepoReport    `json:"repo"`
	Runtime statusRuntimeReport `json:"runtime"`
	Store   statusStoreReport   `json:"store"`
	Next    []string            `json:"next"`
}

type statusRepoReport struct {
	Path     string `json:"path"`
	Branch   string `json:"branch,omitempty"`
	Revision string `json:"revision,omitempty"`
	Upstream string `json:"upstream,omitempty"`
	Dirty    bool   `json:"dirty"`
	Error    string `json:"error,omitempty"`
}

type statusRuntimeReport struct {
	Path                 string `json:"path"`
	Exists               bool   `json:"exists"`
	Executable           bool   `json:"executable"`
	ActivePath           string `json:"activePath,omitempty"`
	ActiveMatchesRuntime bool   `json:"activeMatchesRuntime"`
	Fresh                bool   `json:"fresh"`
	StaleReason          string `json:"staleReason,omitempty"`
	BuildRevision        string `json:"buildRevision,omitempty"`
	BinaryModifiedAt     string `json:"binaryModifiedAt,omitempty"`
	SourceRevision       string `json:"sourceRevision,omitempty"`
	SourceCommitAt       string `json:"sourceCommitAt,omitempty"`
	RepairCommand        string `json:"repairCommand,omitempty"`
}

type statusStoreReport struct {
	Configured bool   `json:"configured"`
	Name       string `json:"name,omitempty"`
	Backend    string `json:"backend,omitempty"`
	URL        string `json:"url,omitempty"`
	RawURL     string `json:"-"`
	ConfigPath string `json:"configPath,omitempty"`
	Detail     string `json:"detail,omitempty"`
	Schema     any    `json:"schema,omitempty"`
}

func runStatus(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	deep := flags.Bool("deep", false, "Include slower Store schema checks")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable status report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected status arguments: %s", strings.Join(flags.Args(), " "))
	}
	report := buildStatusReport(ctx, *deep)
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printStatusReport(report)
	return nil
}

func buildStatusReport(ctx context.Context, deep bool) statusCommandReport {
	repo := statusRepo(ctx)
	runtime := statusRuntime(ctx, repo)
	store := statusStore()
	if deep && store.Configured {
		store.Schema = statusStoreSchema(ctx, store)
	}
	next := statusNextActions(runtime, store)
	return statusCommandReport{
		OK:      repo.Error == "",
		Version: version,
		Repo:    repo,
		Runtime: runtime,
		Store:   store,
		Next:    next,
	}
}

func statusRepo(ctx context.Context) statusRepoReport {
	repo, err := resolveUpdateRepo("")
	if err != nil {
		return statusRepoReport{Error: err.Error()}
	}
	if root, rootErr := updateGitOutput(ctx, repo, "rev-parse", "--show-toplevel"); rootErr == nil {
		repo = root
	}
	report := statusRepoReport{Path: repo}
	if branch, branchErr := updateGitOutput(ctx, repo, "branch", "--show-current"); branchErr == nil {
		report.Branch = branch
	}
	if revision, revErr := updateGitOutput(ctx, repo, "rev-parse", "HEAD"); revErr == nil {
		report.Revision = revision
	}
	if upstream, upstreamErr := updateGitOutput(ctx, repo, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); upstreamErr == nil {
		report.Upstream = upstream
	}
	if dirty, dirtyErr := updateTrackedDirty(ctx, repo); dirtyErr == nil {
		report.Dirty = dirty
	}
	if report.Revision == "" {
		report.Error = "not a git checkout"
	}
	return report
}

func statusRuntime(ctx context.Context, repo statusRepoReport) statusRuntimeReport {
	repoPath := repo.Path
	if strings.TrimSpace(repoPath) == "" {
		repoPath = "."
	}
	path, err := resolveUpdateOutputPath(repoPath, filepath.Join(".runtime", "bin", "agent-testbench"))
	if err != nil {
		path = filepath.Join(repoPath, ".runtime", "bin", "agent-testbench")
	}
	report := statusRuntimeReport{Path: path, SourceRevision: repo.Revision, RepairCommand: runtimeFreshnessRepairCommand(repoPath)}
	if active, err := os.Executable(); err == nil {
		report.ActivePath = filepath.Clean(active)
		report.ActiveMatchesRuntime = sameRuntimePath(report.ActivePath, path)
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		return report
	}
	report.Exists = true
	report.Executable = info.Mode()&0o111 != 0
	report.BinaryModifiedAt = info.ModTime().UTC().Format(time.RFC3339)
	report.Fresh = true
	var repoCommitTime time.Time
	if commitTime, err := statusRepoCommitTime(ctx, repoPath); err == nil {
		repoCommitTime = commitTime
		report.SourceCommitAt = repoCommitTime.UTC().Format(time.RFC3339)
	}
	if strings.TrimSpace(repo.Revision) != "" {
		buildRevision, err := statusRuntimeBuildRevision(ctx, repoPath, path)
		report.BuildRevision = strings.TrimSpace(buildRevision)
		if err != nil || report.BuildRevision == "" {
			report.Fresh = false
			report.StaleReason = "runtime binary does not report a build revision"
			return report
		}
		if report.BuildRevision != strings.TrimSpace(repo.Revision) {
			report.Fresh = false
			report.StaleReason = "runtime binary was built from a different git revision"
			return report
		}
	}
	if !repoCommitTime.IsZero() {
		if info.ModTime().Before(repoCommitTime) {
			report.Fresh = false
			report.StaleReason = "runtime binary is older than the current git HEAD"
		}
	}
	return report
}

func statusStore() statusStoreReport {
	path, pathErr := storeConfigPath()
	report := statusStoreReport{}
	if pathErr == nil {
		report.ConfigPath = path
	}
	cfg, err := loadStoreConfig()
	if err != nil {
		report.Detail = err.Error()
		return report
	}
	if strings.TrimSpace(cfg.Active) == "" {
		report.Detail = "no active Store configured"
		return report
	}
	entry, ok := cfg.Stores[cfg.Active]
	if !ok {
		report.Detail = fmt.Sprintf("active Store %q is missing from config", cfg.Active)
		return report
	}
	report.Configured = true
	report.Name = entry.Name
	report.Backend = entry.Backend
	report.RawURL = entry.URL
	report.URL = maskStoreURL(entry.URL)
	return report
}

func statusStoreSchema(ctx context.Context, store statusStoreReport) storeStatusReport {
	backend, err := storeBackendFromURL(store.RawURL)
	if err != nil {
		return storeStatusReport{OK: false, Backend: store.Backend, URL: store.URL, Error: err.Error()}
	}
	switch backend {
	case "postgres":
		cfg, cfgErr := postgres.ParseConfigFromURL(store.RawURL)
		if cfgErr != nil {
			return postgresStoreStatusErrorReport(store.RawURL, cfgErr)
		}
		status, statusErr := postgresSchemaStatus(ctx, cfg)
		if statusErr != nil {
			return postgresStoreStatusErrorReport(store.RawURL, statusErr)
		}
		return postgresStoreStatusReport(status)
	case "mysql":
		cfg, cfgErr := mysql.ParseConfigFromURL(store.RawURL)
		if cfgErr != nil {
			return mysqlStoreStatusErrorReport(store.RawURL, cfgErr)
		}
		status, statusErr := mysqlSchemaStatus(ctx, cfg)
		if statusErr != nil {
			return mysqlStoreStatusErrorReport(store.RawURL, statusErr)
		}
		return mysqlStoreStatusReport(status)
	default:
		cfg, cfgErr := sqlite.ParseConfigFromURL(store.RawURL)
		if cfgErr != nil {
			return sqliteStoreStatusErrorReport(cfg, cfgErr)
		}
		if readyErr := sqliteStoreStatusFileReady(cfg); readyErr != nil {
			return sqliteStoreStatusErrorReport(cfg, readyErr)
		}
		status, statusErr := sqlite.SchemaStatus(ctx, cfg)
		if statusErr != nil {
			return sqliteStoreStatusErrorReport(cfg, statusErr)
		}
		return sqliteStoreStatusReport(status)
	}
}

func sqliteStoreStatusFileReady(cfg sqlite.Config) error {
	cfg = cfg.Resolve()
	info, err := os.Stat(cfg.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("sqlite store file does not exist; run agent-testbench store upgrade to create it")
		}
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("sqlite store path is a directory")
	}
	return nil
}

func statusNextActions(runtime statusRuntimeReport, store statusStoreReport) []string {
	next := []string{}
	if !store.Configured {
		next = append(next,
			"agent-testbench store config set NAME --url sqlite://PATH",
			"agent-testbench store use NAME",
		)
	}
	if !runtime.Exists {
		next = append(next, "agent-testbench setup --build-runtime")
		next = append(next, "agent-testbench update --channel main")
	}
	if runtime.Exists && runtime.Executable && !runtime.ActiveMatchesRuntime {
		next = append(next, "put "+filepath.Dir(runtime.Path)+" before stale wrappers on PATH, or set ATB_BIN="+runtime.Path)
	}
	if runtime.Exists && runtime.Executable && !runtime.Fresh {
		next = append(next, runtime.RepairCommand)
	}
	next = append(next, "agent-testbench commands --filter \"case gate\"")
	return next
}

func sameRuntimePath(left string, right string) bool {
	left = filepath.Clean(strings.TrimSpace(left))
	right = filepath.Clean(strings.TrimSpace(right))
	if left == "" || right == "" {
		return false
	}
	leftEval, leftErr := filepath.EvalSymlinks(left)
	rightEval, rightErr := filepath.EvalSymlinks(right)
	if leftErr == nil {
		left = filepath.Clean(leftEval)
	}
	if rightErr == nil {
		right = filepath.Clean(rightEval)
	}
	return left == right
}

func printStatusReport(report statusCommandReport) {
	fmt.Println("AgentTestBench Status")
	fmt.Printf("Version: %s\n", report.Version)
	fmt.Println()
	fmt.Println("Repo")
	fmt.Printf("  Path: %s\n", report.Repo.Path)
	fmt.Printf("  Branch: %s\n", stringDefault(report.Repo.Branch, "(unknown)"))
	fmt.Printf("  Revision: %s\n", stringDefault(shortRevision(report.Repo.Revision), "(unknown)"))
	fmt.Printf("  Upstream: %s\n", stringDefault(report.Repo.Upstream, "(none)"))
	fmt.Printf("  Dirty: %t\n", report.Repo.Dirty)
	fmt.Println()
	fmt.Println("Runtime")
	fmt.Printf("  Binary: %s\n", report.Runtime.Path)
	if report.Runtime.ActivePath != "" {
		fmt.Printf("  Active: %s\n", report.Runtime.ActivePath)
	}
	fmt.Printf("  Ready: %t\n", report.Runtime.Exists && report.Runtime.Executable)
	if report.Runtime.Exists {
		fmt.Printf("  Fresh: %t\n", report.Runtime.Fresh)
		if report.Runtime.StaleReason != "" {
			fmt.Printf("  Stale Reason: %s\n", report.Runtime.StaleReason)
		}
	}
	fmt.Println()
	fmt.Println("Store")
	if report.Store.Configured {
		fmt.Printf("  Active: %s (%s)\n", report.Store.Name, report.Store.Backend)
		fmt.Printf("  URL: %s\n", report.Store.URL)
	} else {
		fmt.Printf("  Active: none (%s)\n", stringDefault(report.Store.Detail, "not configured"))
	}
	printNextActions(report.Next)
}

func printNextActions(next []string) {
	if len(next) == 0 {
		return
	}
	fmt.Println()
	fmt.Println("Next")
	for _, item := range next {
		fmt.Printf("  - %s\n", item)
	}
}

func shortRevision(revision string) string {
	revision = strings.TrimSpace(revision)
	if len(revision) <= 12 {
		return revision
	}
	return revision[:12]
}
