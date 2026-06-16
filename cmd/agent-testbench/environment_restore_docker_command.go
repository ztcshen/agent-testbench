package main

import (
	"context"
	"os"
	"os/exec"
	"syscall"
)

func runRestoreGitCommand(ctx context.Context, args ...string) (string, string) {
	return runRestoreExecCommand(ctx, "", append([]string{"git"}, args...), "", false)
}

func runRestoreCommand(ctx context.Context, workdir string, command []string) (string, string) {
	return runRestoreExecCommand(ctx, workdir, command, "", false)
}

func runRestoreCommandWithInput(ctx context.Context, workdir string, command []string, input string) (string, string) {
	return runRestoreExecCommand(ctx, workdir, command, input, true)
}

func runRestoreExecCommand(ctx context.Context, workdir string, command []string, input string, hasInput bool) (string, string) {
	result := runAgentObservedCommand(ctx, agentObservedCommandOptions{
		Workdir:   workdir,
		Command:   command,
		Input:     input,
		HasInput:  hasInput,
		Configure: configureRestoreCommandCancellation,
	})
	return result.Output, result.Error
}

func configureRestoreCommandCancellation(cmd *exec.Cmd) {
	configureObservedCommandCancellation(cmd)
}

func configureObservedCommandCancellation(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return cancelObservedCommand(cmd)
	}
}

func cancelObservedCommand(cmd *exec.Cmd) error {
	if cmd == nil {
		return os.ErrProcessDone
	}
	if cmd.Process == nil {
		return os.ErrProcessDone
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		if killErr := cmd.Process.Kill(); killErr != nil {
			return err
		}
		return nil
	}
	return nil
}
