package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-testbench/internal/domain/updaterelease"
)

type updateCommandOptions struct {
	Repo      string
	Remote    string
	Branch    string
	Release   string
	Channel   string
	Output    string
	CheckOnly bool
	Force     bool
	JSON      bool
}

type updateCommandReport struct {
	OK              bool                `json:"ok"`
	CheckOnly       bool                `json:"checkOnly"`
	Repo            string              `json:"repo"`
	Remote          string              `json:"remote"`
	Branch          string              `json:"branch"`
	Release         string              `json:"release,omitempty"`
	Channel         string              `json:"channel,omitempty"`
	RuntimePath     string              `json:"runtimePath,omitempty"`
	LocalRevision   string              `json:"localRevision,omitempty"`
	RemoteRevision  string              `json:"remoteRevision,omitempty"`
	UpdateAvailable bool                `json:"updateAvailable"`
	Updated         bool                `json:"updated"`
	Dirty           bool                `json:"dirty"`
	Steps           []updateCommandStep `json:"steps"`
}

type updateCommandStep struct {
	Name    string   `json:"name"`
	Command []string `json:"command"`
	OK      bool     `json:"ok"`
	Output  string   `json:"output,omitempty"`
	Error   string   `json:"error,omitempty"`
}

const (
	updateChannelMain    = "main"
	updateDefaultRemote  = "origin"
	updateReleaseLatest  = "latest"
	updateChannelRelease = "release"
)

func runUpdate(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("update", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	repo := flags.String("repo", "", "AgentTestBench git checkout to update")
	remote := flags.String("remote", "", "Git remote to fetch and pull, defaults to the current upstream remote")
	branch := flags.String("branch", "", "Git branch to fetch and pull, defaults to the current upstream branch")
	release := flags.String("release", "", "Git release tag to fetch and pull; use 'latest' for the highest remote version tag")
	channel := flags.String("channel", "", "Update channel: main or release")
	output := flags.String("output", filepath.Join(".runtime", "bin", "agent-testbench"), "Runtime binary output path")
	checkOnly := flags.Bool("check", false, "Fetch and compare remote revision without pulling or rebuilding")
	force := flags.Bool("force", false, "Allow update when tracked files are locally modified")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable update report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected update arguments: %s", strings.Join(flags.Args(), " "))
	}
	report, err := updateRuntime(ctx, updateCommandOptions{
		Repo:      *repo,
		Remote:    *remote,
		Branch:    *branch,
		Release:   *release,
		Channel:   *channel,
		Output:    *output,
		CheckOnly: *checkOnly,
		Force:     *force,
		JSON:      *jsonOutput,
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
		printUpdateReport(report)
	}
	return nil
}

func updateRuntime(ctx context.Context, opts updateCommandOptions) (updateCommandReport, error) {
	repo, err := resolveUpdateRepo(opts.Repo)
	if err != nil {
		return updateCommandReport{OK: false}, err
	}
	remote, branch, release, channel, err := resolveUpdateTargetWithChannel(ctx, repo, opts.Remote, opts.Branch, opts.Release, opts.Channel)
	report := updateCommandReport{
		OK:        false,
		CheckOnly: opts.CheckOnly,
		Repo:      repo,
		Remote:    remote,
		Branch:    branch,
		Release:   release,
		Channel:   channel,
	}
	if err != nil {
		return report, err
	}
	report.LocalRevision, err = updateGitOutput(ctx, repo, "rev-parse", "HEAD")
	if err != nil {
		return report, err
	}
	fetchStep := runUpdateGitStep(ctx, repo, "fetch", "--prune", remote, branch)
	report.Steps = append(report.Steps, fetchStep)
	if !fetchStep.OK {
		return report, updateStepError(fetchStep)
	}
	report.RemoteRevision, err = updateGitOutput(ctx, repo, "rev-parse", "FETCH_HEAD^{commit}")
	if err != nil {
		return report, err
	}
	report.UpdateAvailable = report.LocalRevision != report.RemoteRevision && updateRevisionIsAncestor(ctx, repo, report.LocalRevision, report.RemoteRevision)
	if opts.CheckOnly {
		report.OK = true
		return report, nil
	}
	dirty, err := updateTrackedDirty(ctx, repo)
	if err != nil {
		return report, err
	}
	report.Dirty = dirty
	if dirty && !opts.Force {
		return report, fmt.Errorf("tracked files have local changes; Next: commit or stash tracked edits, then rerun update, or rerun update with --force")
	}
	pullStep := runUpdateGitStep(ctx, repo, "pull", "--ff-only", remote, branch)
	report.Steps = append(report.Steps, pullStep)
	if !pullStep.OK {
		return report, updateStepError(pullStep)
	}
	currentRevision, err := updateGitOutput(ctx, repo, "rev-parse", "HEAD")
	if err != nil {
		return report, err
	}
	report.Updated = currentRevision != report.LocalRevision
	report.LocalRevision = currentRevision
	report.RuntimePath, err = resolveUpdateOutputPath(repo, opts.Output)
	if err != nil {
		return report, err
	}
	if err := os.MkdirAll(filepath.Dir(report.RuntimePath), 0o755); err != nil {
		return report, err
	}
	buildStep := runUpdateCommandStep(ctx, repo, "build-runtime", runtimeBuildCommand(ctx, repo, report.RuntimePath)...)
	report.Steps = append(report.Steps, buildStep)
	if !buildStep.OK {
		return report, updateStepError(buildStep)
	}
	report.OK = true
	return report, nil
}

func resolveUpdateTargetWithChannel(ctx context.Context, repo string, remote string, branch string, release string, channel string) (string, string, string, string, error) {
	channel = strings.ToLower(strings.TrimSpace(channel))
	switch channel {
	case "":
	case updateChannelMain:
		if strings.TrimSpace(release) != "" {
			return strings.TrimSpace(remote), strings.TrimSpace(branch), strings.TrimSpace(release), channel, fmt.Errorf("--channel main cannot be combined with --release")
		}
		if strings.TrimSpace(branch) == "" {
			branch = updateChannelMain
		}
	case updateChannelRelease:
		if strings.TrimSpace(branch) != "" {
			return strings.TrimSpace(remote), strings.TrimSpace(branch), strings.TrimSpace(release), channel, fmt.Errorf("--channel release cannot be combined with --branch")
		}
		if strings.TrimSpace(release) == "" {
			release = updateReleaseLatest
		}
	default:
		return strings.TrimSpace(remote), strings.TrimSpace(branch), strings.TrimSpace(release), channel, fmt.Errorf("unsupported update channel %q; use main or release", channel)
	}
	release = strings.TrimSpace(release)
	if release == "" {
		remote, branch, err := resolveUpdateTarget(ctx, repo, remote, branch)
		return remote, branch, "", channel, err
	}
	if strings.TrimSpace(branch) != "" {
		return strings.TrimSpace(remote), strings.TrimSpace(branch), release, channel, fmt.Errorf("--release cannot be combined with --branch")
	}
	resolvedRemote := resolveUpdateRemote(ctx, repo, remote)
	if release == updateReleaseLatest {
		resolvedRelease, err := resolveLatestUpdateRelease(ctx, repo, resolvedRemote)
		if err != nil {
			return resolvedRemote, "", updateReleaseLatest, channel, err
		}
		release = resolvedRelease
	}
	return resolvedRemote, release, release, channel, nil
}

func resolveUpdateRemote(ctx context.Context, repo string, remote string) string {
	remote = strings.TrimSpace(remote)
	if remote != "" {
		return remote
	}
	upstream, err := updateGitOutput(ctx, repo, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if err == nil {
		upstreamRemote, _ := splitUpdateUpstream(upstream)
		if upstreamRemote != "" {
			return upstreamRemote
		}
	}
	if updateRemoteExists(ctx, repo, "github") {
		return "github"
	}
	return updateDefaultRemote
}

func updateRemoteExists(ctx context.Context, repo string, remote string) bool {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return false
	}
	_, err := updateGitOutput(ctx, repo, "remote", "get-url", remote)
	return err == nil
}

func resolveLatestUpdateRelease(ctx context.Context, repo string, remote string) (string, error) {
	out, err := updateGitOutput(ctx, repo, "ls-remote", "--tags", remote)
	if err != nil {
		return "", err
	}
	tag := updaterelease.LatestTagFromRemoteOutput(out)
	if tag == "" {
		return "", fmt.Errorf("remote %q has no tags to use as latest release", remote)
	}
	return tag, nil
}

func resolveUpdateRepo(repo string) (string, error) {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		repo = strings.TrimSpace(os.Getenv("AGENT_TESTBENCH_REPO"))
	}
	if repo == "" {
		repo = "."
	}
	return filepath.Abs(repo)
}

func resolveUpdateTarget(ctx context.Context, repo string, remote string, branch string) (string, string, error) {
	remote = strings.TrimSpace(remote)
	branch = strings.TrimSpace(branch)
	if remote != "" && branch != "" {
		return remote, branch, nil
	}
	upstream, err := updateGitOutput(ctx, repo, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if err == nil {
		upstreamRemote, upstreamBranch := splitUpdateUpstream(upstream)
		if remote == "" {
			remote = upstreamRemote
		}
		if branch == "" {
			branch = upstreamBranch
		}
	}
	if remote == "" {
		remote = resolveUpdateRemote(ctx, repo, "")
	}
	if branch == "" {
		branch, err = updateGitOutput(ctx, repo, "branch", "--show-current")
		if err != nil || strings.TrimSpace(branch) == "" {
			return remote, branch, fmt.Errorf("cannot infer update branch; pass --branch explicitly")
		}
	}
	return remote, branch, nil
}

func splitUpdateUpstream(upstream string) (string, string) {
	parts := strings.SplitN(strings.TrimSpace(upstream), "/", 2)
	if len(parts) != 2 {
		return "", strings.TrimSpace(upstream)
	}
	return parts[0], parts[1]
}

func resolveUpdateOutputPath(repo string, output string) (string, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return "", fmt.Errorf("update output path is required")
	}
	if filepath.IsAbs(output) {
		return filepath.Clean(output), nil
	}
	return filepath.Join(repo, output), nil
}

func updateTrackedDirty(ctx context.Context, repo string) (bool, error) {
	out, err := updateGitOutput(ctx, repo, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func updateRevisionIsAncestor(ctx context.Context, repo string, ancestor string, descendant string) bool {
	_, errText := runRestoreCommand(ctx, repo, []string{"git", "-C", repo, "merge-base", "--is-ancestor", ancestor, descendant})
	return errText == ""
}

func runUpdateGitStep(ctx context.Context, repo string, args ...string) updateCommandStep {
	return runUpdateCommandStep(ctx, repo, "git-"+args[0], append([]string{"git", "-C", repo}, args...)...)
}

func runUpdateCommandStep(ctx context.Context, workdir string, name string, command ...string) updateCommandStep {
	output, errText := runRestoreCommand(ctx, workdir, command)
	step := updateCommandStep{Name: name, Command: command, OK: errText == "", Output: output}
	if errText != "" {
		step.Error = errText
	}
	return step
}

func runtimeBuildCommand(ctx context.Context, repo string, output string) []string {
	args := []string{"go", "build"}
	if revision, err := updateGitOutput(ctx, repo, "rev-parse", "HEAD"); err == nil && strings.TrimSpace(revision) != "" {
		args = append(args, "-ldflags", "-X main.buildRevision="+strings.TrimSpace(revision))
	}
	return append(args, "-o", output, "./cmd/agent-testbench")
}

func updateGitOutput(ctx context.Context, repo string, args ...string) (string, error) {
	output, errText := runRestoreCommand(ctx, repo, append([]string{"git", "-C", repo}, args...))
	if errText != "" {
		return output, fmt.Errorf("git %s failed: %s", strings.Join(args, " "), errText)
	}
	return strings.TrimSpace(output), nil
}

func updateStepError(step updateCommandStep) error {
	if step.Error != "" {
		return fmt.Errorf("%s failed: %s", step.Name, step.Error)
	}
	return fmt.Errorf("%s failed", step.Name)
}

func printUpdateReport(report updateCommandReport) {
	fmt.Println("AgentTestBench Update")
	fmt.Printf("Repo: %s\n", report.Repo)
	fmt.Printf("Remote: %s/%s\n", report.Remote, report.Branch)
	if report.Channel != "" {
		fmt.Printf("Channel: %s\n", report.Channel)
	}
	if report.Release != "" {
		fmt.Printf("Release: %s\n", report.Release)
	}
	if report.CheckOnly {
		fmt.Printf("Update Available: %t\n", report.UpdateAvailable)
		fmt.Printf("Local: %s\n", report.LocalRevision)
		fmt.Printf("Remote: %s\n", report.RemoteRevision)
		if report.UpdateAvailable {
			fmt.Printf("Next: agent-testbench update --remote %s --branch %s\n", report.Remote, report.Branch)
		} else {
			fmt.Println("Next: no update is available for the selected target")
		}
		return
	}
	fmt.Printf("Updated: %t\n", report.Updated)
	fmt.Printf("Runtime: %s\n", report.RuntimePath)
}
