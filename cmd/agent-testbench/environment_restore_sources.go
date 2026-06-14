package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-testbench/internal/domain/environmentsource"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

type environmentRestorePackageReport struct {
	Configured bool     `json:"configured"`
	URL        string   `json:"url,omitempty"`
	Branch     string   `json:"branch,omitempty"`
	Ref        string   `json:"ref,omitempty"`
	Checkout   string   `json:"checkout,omitempty"`
	Exists     bool     `json:"exists"`
	Action     string   `json:"action"`
	Command    []string `json:"command,omitempty"`
	OK         bool     `json:"ok"`
	Output     string   `json:"output,omitempty"`
	Error      string   `json:"error,omitempty"`
}

type environmentRestoreRepoReport struct {
	ServiceID string   `json:"serviceId"`
	URL       string   `json:"url,omitempty"`
	Branch    string   `json:"branch,omitempty"`
	Ref       string   `json:"ref,omitempty"`
	Checkout  string   `json:"checkout"`
	Exists    bool     `json:"exists"`
	Action    string   `json:"action"`
	Command   []string `json:"command,omitempty"`
	OK        bool     `json:"ok"`
	Output    string   `json:"output,omitempty"`
	Error     string   `json:"error,omitempty"`
}

type environmentRestoreSourcePolicy = environmentsource.SourcePolicy
type environmentRestorePackageSpec = environmentsource.PackageSpec
type environmentRestoreRepoSpec = environmentsource.RepoSpec

func environmentRestorePackage(ctx context.Context, spec environmentRestorePackageSpec, execute bool, pull bool, storeGeneratedRestore bool) environmentRestorePackageReport {
	report := environmentRestorePackageReport{
		Configured: strings.TrimSpace(spec.URL) != "" || strings.TrimSpace(spec.Ref) != "",
		URL:        spec.URL,
		Branch:     spec.Branch,
		Ref:        spec.Ref,
		Checkout:   spec.Checkout,
		OK:         true,
	}
	if !report.Configured {
		report.Action = "not-configured"
		return report
	}
	if storeGeneratedRestore {
		report.Action = "ignored-for-sql-store-restore"
		return report
	}
	repoReport := environmentRestoreRepo(ctx, environmentRestoreRepoSpec{
		ServiceID: "environment-package",
		URL:       spec.URL,
		Branch:    spec.Branch,
		Ref:       spec.Ref,
		Checkout:  spec.Checkout,
	}, execute, pull)
	report.Exists = repoReport.Exists
	report.Action = repoReport.Action
	report.Command = repoReport.Command
	report.OK = repoReport.OK
	report.Output = repoReport.Output
	report.Error = repoReport.Error
	return report
}

func environmentRestoreRequiresRemoteSources(storeURL string) bool {
	backend, err := storeBackendFromURL(strings.TrimSpace(storeURL))
	if err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "postgres", "mysql":
		return true
	default:
		return false
	}
}

func environmentRestoreComponentGraphReport(envID string, graph store.EnvironmentComponentGraph) environmentRestoreComponentGraph {
	return controlplane.EnvironmentComponentGraphReadinessReport(envID, graph)
}

func environmentRestoreRepo(ctx context.Context, spec environmentRestoreRepoSpec, execute bool, pull bool) environmentRestoreRepoReport {
	report := environmentRestoreRepoReport{
		ServiceID: spec.ServiceID,
		URL:       spec.URL,
		Branch:    spec.Branch,
		Ref:       spec.Ref,
		Checkout:  spec.Checkout,
		OK:        true,
	}
	if stat, err := os.Stat(spec.Checkout); err == nil && stat.IsDir() {
		return environmentRestoreExistingRepo(ctx, spec, report, execute, pull)
	}
	return environmentRestoreMissingRepo(ctx, spec, report, execute)
}

func environmentRestoreExistingRepo(ctx context.Context, spec environmentRestoreRepoSpec, report environmentRestoreRepoReport, execute bool, pull bool) environmentRestoreRepoReport {
	report.Exists = true
	if strings.TrimSpace(spec.URL) != "" && environmentRestoreDirIsEmpty(spec.Checkout) {
		return environmentRestoreEmptyCheckout(ctx, spec, report, execute)
	}
	if strings.TrimSpace(spec.URL) != "" || strings.TrimSpace(spec.Ref) != "" {
		if ok, errText := environmentRestoreValidateCheckout(ctx, spec); !ok {
			report.OK = false
			report.Action = "invalid-existing-checkout"
			report.Error = errText
			return report
		}
	}
	if strings.TrimSpace(spec.Ref) != "" {
		return environmentRestoreExistingRef(ctx, spec, report, execute)
	}
	if strings.TrimSpace(spec.URL) == "" || !execute || !pull {
		report.Action = "use-existing-checkout"
		return report
	}
	return environmentRestorePullExisting(ctx, spec, report)
}

func environmentRestoreEmptyCheckout(ctx context.Context, spec environmentRestoreRepoSpec, report environmentRestoreRepoReport, execute bool) environmentRestoreRepoReport {
	if !execute {
		report.Exists = false
		report.Action = "clone"
		args := restoreGitCloneArgs(spec)
		report.Command = append([]string{"git"}, args...)
		return report
	}
	return environmentRestoreCloneIntoCheckout(ctx, spec, report)
}

func environmentRestoreExistingRef(ctx context.Context, spec environmentRestoreRepoSpec, report environmentRestoreRepoReport, execute bool) environmentRestoreRepoReport {
	if atRef, _ := environmentRestoreCheckoutDetachedAtRef(ctx, spec); atRef {
		report.Action = "use-existing-checkout"
		return report
	}
	checkoutCommands := environmentRestoreExistingRefCommands(spec)
	report.Action = "checkout-existing-ref"
	report.Command = flattenRestoreCommands(checkoutCommands)
	if !execute {
		return report
	}
	outputs := make([]string, 0, len(checkoutCommands))
	for _, command := range checkoutCommands {
		if len(command) == 0 {
			continue
		}
		output, errText := runRestoreGitCommand(ctx, command[1:]...)
		if strings.TrimSpace(output) != "" {
			outputs = append(outputs, output)
		}
		if errText != "" {
			report.OK = false
			report.Output = strings.Join(outputs, "\n")
			report.Error = errText
			return report
		}
	}
	report.Output = strings.Join(outputs, "\n")
	report.OK = true
	return report
}

func environmentRestorePullExisting(ctx context.Context, spec environmentRestoreRepoSpec, report environmentRestoreRepoReport) environmentRestoreRepoReport {
	args := []string{"-C", spec.Checkout, "pull", "--ff-only"}
	report.Action = "pull-existing-checkout"
	report.Command = append([]string{"git"}, args...)
	report.Output, report.Error = runRestoreGitCommand(ctx, args...)
	report.OK = report.Error == ""
	return report
}

func environmentRestoreMissingRepo(ctx context.Context, spec environmentRestoreRepoSpec, report environmentRestoreRepoReport, execute bool) environmentRestoreRepoReport {
	if strings.TrimSpace(spec.URL) == "" {
		report.OK = false
		report.Action = "missing-repo-url"
		report.Error = "repository url is required when checkout is missing"
		return report
	}
	if !execute {
		report.Action = "clone"
		args := restoreGitCloneArgs(spec)
		report.Command = append([]string{"git"}, args...)
		return report
	}
	if err := os.MkdirAll(filepath.Dir(spec.Checkout), 0o755); err != nil {
		report.OK = false
		report.Action = "prepare-checkout-parent"
		report.Error = err.Error()
		return report
	}
	return environmentRestoreCloneIntoCheckout(ctx, spec, report)
}

func environmentRestoreCloneIntoCheckout(ctx context.Context, spec environmentRestoreRepoSpec, report environmentRestoreRepoReport) environmentRestoreRepoReport {
	args := restoreGitCloneArgs(spec)
	report.Action = "clone"
	report.Command = append([]string{"git"}, args...)
	report.Output, report.Error = runRestoreGitCommand(ctx, args...)
	report.OK = report.Error == ""
	if report.OK && strings.TrimSpace(spec.Ref) != "" {
		checkoutArgs := []string{"-C", spec.Checkout, "checkout", "--detach", strings.TrimSpace(spec.Ref)}
		report.Command = append(report.Command, append([]string{"&&", "git"}, checkoutArgs...)...)
		output, errText := runRestoreGitCommand(ctx, checkoutArgs...)
		if strings.TrimSpace(output) != "" {
			report.Output = strings.TrimSpace(report.Output + "\n" + output)
		}
		report.Error = errText
		report.OK = report.Error == ""
	}
	return report
}

func environmentRestoreDirIsEmpty(path string) bool {
	entries, err := os.ReadDir(path)
	return err == nil && len(entries) == 0
}

func environmentRestoreExistingRefCommands(spec environmentRestoreRepoSpec) [][]string {
	out := [][]string{}
	if strings.TrimSpace(spec.URL) != "" {
		out = append(out, []string{"git", "-C", spec.Checkout, "fetch", "--tags", "origin"})
	}
	out = append(out, []string{"git", "-C", spec.Checkout, "checkout", "--detach", strings.TrimSpace(spec.Ref)})
	return out
}

func flattenRestoreCommands(commands [][]string) []string {
	out := []string{}
	for _, command := range commands {
		if len(command) == 0 {
			continue
		}
		if len(out) > 0 {
			out = append(out, "&&")
		}
		out = append(out, command...)
	}
	return out
}

func environmentRestoreValidateCheckout(ctx context.Context, spec environmentRestoreRepoSpec) (bool, string) {
	if _, errText := runRestoreGitCommand(ctx, "-C", spec.Checkout, "rev-parse", "--is-inside-work-tree"); errText != "" {
		return false, "existing checkout is not a Git repository: " + spec.Checkout
	}
	if strings.TrimSpace(spec.URL) != "" {
		remote, errText := runRestoreGitCommand(ctx, "-C", spec.Checkout, "remote", "get-url", "origin")
		if errText != "" {
			return false, errText
		}
		if strings.TrimSpace(remote) != strings.TrimSpace(spec.URL) {
			return false, fmt.Sprintf("existing checkout origin mismatch: got %s want %s", strings.TrimSpace(remote), strings.TrimSpace(spec.URL))
		}
	}
	if dirty, errText := runRestoreGitCommand(ctx, "-C", spec.Checkout, "status", "--porcelain"); errText != "" {
		return false, errText
	} else if strings.TrimSpace(dirty) != "" {
		return false, "existing checkout has uncommitted changes"
	}
	return true, ""
}

func environmentRestoreCheckoutDetachedAtRef(ctx context.Context, spec environmentRestoreRepoSpec) (bool, string) {
	head, errText := runRestoreGitCommand(ctx, "-C", spec.Checkout, "rev-parse", "HEAD")
	if errText != "" {
		return false, errText
	}
	target, errText := runRestoreGitCommand(ctx, "-C", spec.Checkout, "rev-parse", strings.TrimSpace(spec.Ref)+"^{commit}")
	if errText != "" {
		return false, errText
	}
	branch, errText := runRestoreGitCommand(ctx, "-C", spec.Checkout, "rev-parse", "--abbrev-ref", "HEAD")
	if errText != "" {
		return false, errText
	}
	return strings.TrimSpace(head) == strings.TrimSpace(target) && strings.TrimSpace(branch) == "HEAD", ""
}

func restoreGitCloneArgs(spec environmentRestoreRepoSpec) []string {
	args := []string{"clone"}
	if strings.TrimSpace(spec.Branch) != "" {
		args = append(args, "--branch", strings.TrimSpace(spec.Branch))
	}
	args = append(args, strings.TrimSpace(spec.URL), strings.TrimSpace(spec.Checkout))
	return args
}
