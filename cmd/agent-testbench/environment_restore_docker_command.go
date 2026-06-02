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
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			if err == syscall.ESRCH {
				return os.ErrProcessDone
			}
			return err
		}
		return nil
	}
}
