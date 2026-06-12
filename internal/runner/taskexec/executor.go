// Package taskexec executes post-process task commands from workflow evidence runs.
package taskexec

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
)

func Execute(ctx context.Context, kind string, command string) (string, int, error) {
	if strings.TrimSpace(kind) == "shell" {
		return executeShell(ctx, command)
	}
	return executeAgentTestBench(ctx, command)
}

func executeAgentTestBench(ctx context.Context, command string) (string, int, error) {
	args, err := splitCommandLine(command)
	if err != nil {
		return "", -1, err
	}
	if len(args) == 0 {
		return "", -1, errors.New("task command is empty")
	}
	exe, err := os.Executable()
	if err != nil {
		return "", -1, err
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return string(out), exitErr.ExitCode(), err
	}
	return string(out), -1, err
}

func executeShell(ctx context.Context, command string) (string, int, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", -1, errors.New("task command is empty")
	}
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return string(out), exitErr.ExitCode(), err
	}
	return string(out), -1, err
}

func splitCommandLine(command string) ([]string, error) {
	args := []string{}
	var b strings.Builder
	var quote rune
	escaped := false
	for _, r := range command {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			b.WriteRune(r)
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == ' ' || r == '\t' || r == '\n' {
			if b.Len() > 0 {
				args = append(args, b.String())
				b.Reset()
			}
			continue
		}
		b.WriteRune(r)
	}
	if escaped {
		b.WriteRune('\\')
	}
	if quote != 0 {
		return nil, errors.New("unterminated quote in task command")
	}
	if b.Len() > 0 {
		args = append(args, b.String())
	}
	return args, nil
}
