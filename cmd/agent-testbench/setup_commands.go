package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type setupCommandReport struct {
	OK      bool                `json:"ok"`
	Repo    string              `json:"repo"`
	Store   setupStoreReport    `json:"store"`
	Runtime setupRuntimeReport  `json:"runtime"`
	Steps   []updateCommandStep `json:"steps,omitempty"`
	Next    []string            `json:"next"`
}

type setupStoreReport struct {
	Name    string `json:"name"`
	Backend string `json:"backend"`
	URL     string `json:"url"`
	Active  bool   `json:"active"`
}

type setupRuntimeReport struct {
	Path  string `json:"path"`
	Built bool   `json:"built"`
}

func statusRepoCommitTime(ctx context.Context, repo string) (time.Time, error) {
	out, err := updateGitOutput(ctx, repo, "show", "-s", "--format=%ct", "HEAD")
	if err != nil {
		return time.Time{}, err
	}
	seconds, parseErr := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if parseErr != nil {
		return time.Time{}, parseErr
	}
	return time.Unix(seconds, 0), nil
}

func statusRuntimeBuildRevision(ctx context.Context, repo string, path string) (string, error) {
	infoCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	output, errText := runRestoreCommand(infoCtx, repo, []string{path, "version", "--json"})
	if errText != "" {
		return "", errors.New(errText)
	}
	var report versionCommandReport
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		return "", err
	}
	return strings.TrimSpace(report.BuildRevision), nil
}

func runtimeFreshnessRepairCommand(repo string) string {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		repo = "."
	}
	return "agent-testbench setup --repo " + quoteCommandValue(repo) + " --build-runtime --runtime-only"
}

func doctorRuntimeFreshnessCheck(runtime statusRuntimeReport) doctorCheckReport {
	fix := runtime.RepairCommand
	if strings.TrimSpace(fix) == "" {
		fix = "agent-testbench setup --build-runtime --runtime-only"
	}
	if !runtime.Exists || !runtime.Executable {
		return doctorCheckReport{Name: "runtime-freshness", Code: doctorCodeRuntimeFreshness, OK: false, Optional: true, Detail: "runtime binary is not ready", Fix: fix}
	}
	if runtime.Fresh {
		return doctorCheckReport{Name: "runtime-freshness", Code: doctorCodeRuntimeFreshness, OK: true, Optional: true, Detail: "runtime binary is at least as new as git HEAD"}
	}
	detail := runtime.StaleReason
	if detail == "" {
		detail = "runtime binary may be stale"
	}
	return doctorCheckReport{
		Name:     "runtime-freshness",
		Code:     doctorCodeRuntimeFreshness,
		OK:       false,
		Optional: true,
		Detail:   detail,
		Fix:      fix,
	}
}

func doctorToolCheck(name string, optional bool) doctorCheckReport {
	path, err := exec.LookPath(name)
	if err != nil {
		fix := fmt.Sprintf("install %s and ensure it is on PATH", name)
		if optional {
			fix = fmt.Sprintf("install %s before Docker-backed restore flows", name)
		}
		return doctorCheckReport{Name: "tool-" + name, Code: "tool." + name, OK: false, Optional: optional, Detail: "not found on PATH", Fix: fix}
	}
	return doctorCheckReport{Name: "tool-" + name, Code: "tool." + name, OK: true, Optional: optional, Detail: path}
}

func doctorRepoCheck(repo statusRepoReport) doctorCheckReport {
	if repo.Error != "" {
		return doctorCheckReport{Name: "git-checkout", Code: "git.checkout", OK: false, Detail: repo.Error, Fix: "run from an AgentTestBench git checkout or pass --repo to update"}
	}
	detail := repo.Path
	if repo.Branch != "" {
		detail = fmt.Sprintf("%s on %s", repo.Path, repo.Branch)
	}
	return doctorCheckReport{Name: "git-checkout", Code: "git.checkout", OK: true, Detail: detail}
}

func doctorStoreCheck(store statusStoreReport) doctorCheckReport {
	if store.Configured {
		return doctorCheckReport{Name: doctorCheckActiveStore, Code: "store.active", OK: true, Fixed: doctorActiveStoreIsFixed(), Detail: fmt.Sprintf("%s (%s)", store.Name, store.Backend)}
	}
	fix := "run agent-testbench store config set NAME --url sqlite://PATH, then agent-testbench store use NAME"
	return doctorCheckReport{
		Name:   doctorCheckActiveStore,
		Code:   "store.active",
		OK:     false,
		Fixed:  doctorActiveStoreIsFixed(),
		Detail: fmt.Sprintf("%s; %s", stringDefault(store.Detail, "no active Store configured"), fix),
		Fix:    fix,
	}
}

func doctorRuntimeDirectoryCheck(runtime statusRuntimeReport) doctorCheckReport {
	dir := filepath.Dir(runtime.Path)
	info, err := os.Stat(dir)
	if err == nil && info.IsDir() {
		return doctorCheckReport{Name: "runtime-directory", Code: "runtime.directory", OK: true, Fixed: doctorRuntimeDirectoryWasFixed(), Detail: dir}
	}
	return doctorCheckReport{Name: "runtime-directory", Code: "runtime.directory", OK: false, Optional: true, Detail: dir + " is missing", Fix: "run agent-testbench doctor --fix"}
}

func doctorRuntimeCheck(runtime statusRuntimeReport) doctorCheckReport {
	if runtime.Exists && runtime.Executable {
		return doctorCheckReport{Name: "runtime-binary", Code: "runtime.binary", OK: true, Optional: true, Detail: runtime.Path}
	}
	detail := "missing"
	if runtime.Exists {
		detail = "exists but is not executable"
	}
	return doctorCheckReport{Name: "runtime-binary", Code: "runtime.binary", OK: false, Optional: true, Detail: detail, Fix: "run agent-testbench update"}
}

func runSetup(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	repoFlag := flags.String("repo", "", "AgentTestBench git checkout to configure")
	storeName := flags.String("store", "local", "Local Store config name to create or update")
	storeURL := flags.String("url", "", "PostgreSQL, MySQL, or SQLite Store DSN")
	sqlitePath := flags.String("sqlite", "", "SQLite Store path; defaults to REPO/.runtime/agent-testbench-local.sqlite")
	buildRuntime := flags.Bool("build-runtime", false, "Build the local runtime binary into REPO/.runtime/bin")
	runtimeOnly := flags.Bool("runtime-only", false, "Only build the runtime binary; do not create, update, or switch Store config")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable setup report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected setup arguments: %s", strings.Join(flags.Args(), " "))
	}
	report, err := setupLocalRuntime(ctx, setupOptions{
		Repo:         *repoFlag,
		StoreName:    *storeName,
		StoreURL:     *storeURL,
		SQLitePath:   *sqlitePath,
		BuildRuntime: *buildRuntime,
		RuntimeOnly:  *runtimeOnly,
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
		printSetupReport(report)
	}
	return nil
}

type setupOptions struct {
	Repo         string
	StoreName    string
	StoreURL     string
	SQLitePath   string
	BuildRuntime bool
	RuntimeOnly  bool
}

func setupLocalRuntime(ctx context.Context, opts setupOptions) (setupCommandReport, error) {
	repo, err := resolveSetupRepoCheckout(ctx, opts.Repo)
	if err != nil {
		return setupCommandReport{OK: false}, err
	}
	runtimePath, err := resolveUpdateOutputPath(repo, filepath.Join(".runtime", "bin", "agent-testbench"))
	if err != nil {
		return setupCommandReport{OK: false, Repo: repo}, err
	}
	report := setupCommandReport{
		OK:      true,
		Repo:    repo,
		Runtime: setupRuntimeReport{Path: runtimePath},
		Next: []string{
			"agent-testbench status",
			"agent-testbench doctor",
		},
	}
	if !opts.RuntimeOnly {
		storeURL, storeErr := setupStoreURL(repo, opts.StoreURL, opts.SQLitePath)
		if storeErr != nil {
			return setupCommandReport{OK: false, Repo: repo}, storeErr
		}
		entry, entryErr := newStoreConfigEntry(strings.TrimSpace(opts.StoreName), storeURL)
		if entryErr != nil {
			return setupCommandReport{OK: false, Repo: repo}, entryErr
		}
		cfg, cfgErr := loadStoreConfig()
		if cfgErr != nil {
			return setupCommandReport{OK: false, Repo: repo}, cfgErr
		}
		if cfg.Stores == nil {
			cfg.Stores = map[string]storeConfigEntry{}
		}
		cfg.Stores[entry.Name] = entry
		cfg.Active = entry.Name
		if saveErr := saveStoreConfig(cfg); saveErr != nil {
			return setupCommandReport{OK: false, Repo: repo}, saveErr
		}
		report.Store = setupStoreReport{
			Name:    entry.Name,
			Backend: entry.Backend,
			URL:     maskStoreURL(entry.URL),
			Active:  true,
		}
		report.Next = append(report.Next, "agent-testbench store status --store "+entry.Name)
	}
	if err := os.MkdirAll(filepath.Dir(runtimePath), 0o755); err != nil {
		report.OK = false
		return report, err
	}
	if opts.BuildRuntime {
		step := runUpdateCommandStep(ctx, repo, "build-runtime", runtimeBuildCommand(ctx, repo, runtimePath)...)
		report.Steps = append(report.Steps, step)
		report.Runtime.Built = step.OK
		if !step.OK {
			report.OK = false
			return report, updateStepError(step)
		}
	}
	return report, nil
}

func resolveSetupRepoCheckout(ctx context.Context, repoFlag string) (string, error) {
	repo, err := resolveUpdateRepo(repoFlag)
	if err != nil {
		return "", err
	}
	root, err := updateGitOutput(ctx, repo, "rev-parse", "--show-toplevel")
	if err != nil || strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("setup --repo must point to an AgentTestBench git checkout: %s", repo)
	}
	return root, nil
}

func setupStoreURL(repo string, explicitURL string, sqlitePath string) (string, error) {
	explicitURL = strings.TrimSpace(explicitURL)
	sqlitePath = strings.TrimSpace(sqlitePath)
	if explicitURL != "" && sqlitePath != "" {
		return "", fmt.Errorf("--url and --sqlite cannot be combined")
	}
	if explicitURL != "" {
		return explicitURL, nil
	}
	if sqlitePath == "" {
		sqlitePath = filepath.Join(repo, ".runtime", "agent-testbench-local.sqlite")
	} else if !filepath.IsAbs(sqlitePath) {
		sqlitePath = filepath.Join(repo, sqlitePath)
	}
	return "sqlite://" + filepath.Clean(sqlitePath), nil
}

func printSetupReport(report setupCommandReport) {
	fmt.Println("AgentTestBench Setup")
	fmt.Printf("Repo: %s\n", report.Repo)
	if report.Store.Name != "" {
		fmt.Printf("Store: %s (%s)\n", report.Store.Name, report.Store.Backend)
	}
	fmt.Printf("Runtime: %s\n", report.Runtime.Path)
	if report.Runtime.Built {
		fmt.Println("Runtime Built: true")
	}
	printNextActions(report.Next)
}
