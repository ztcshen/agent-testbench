// Package nativeprocess runs bounded local processes and owns their lifecycle.
package nativeprocess

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

const MaxOutputBytes = 2 << 20

type Command struct {
	Executable string
	Args       []string
	Directory  string
	Env        []string
	Input      []byte
	Timeout    time.Duration
}

type Result struct {
	Input      []byte
	Stdout     []byte
	Stderr     []byte
	ExitCode   int
	StartedAt  time.Time
	FinishedAt time.Time
	Category   string
	Error      string
}

func Run(ctx context.Context, command Command) Result {
	result := Result{Input: command.Input, ExitCode: -1, StartedAt: time.Now().UTC()}
	if command.Timeout <= 0 {
		command.Timeout = time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, command.Timeout)
	defer cancel()
	stdout := &boundedOutput{cancel: cancel}
	stderr := &boundedOutput{cancel: cancel}
	cmd := exec.CommandContext(runCtx, command.Executable, command.Args...)
	cmd.Dir, cmd.Env, cmd.Stdin = command.Directory, command.Env, bytes.NewReader(command.Input)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmd.WaitDelay = time.Second
	err := configureProcess(cmd)
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		result.Category, result.Error = "process-start", err.Error()
		if runCtx.Err() != nil {
			classifyProcessOutcome(&result, runCtx, err, false)
		}
	} else {
		err = cmd.Wait()
		// An adapter must not leave owned background processes behind.
		if stopErr := stopProcessGroup(cmd); stopErr != nil && !errors.Is(stopErr, os.ErrProcessDone) && err == nil {
			err = stopErr
		}
		result.ExitCode = cmd.ProcessState.ExitCode()
		classifyProcessOutcome(&result, runCtx, err, stdout.overflowed() || stderr.overflowed())
	}
	result.Stdout, result.Stderr = stdout.content(), stderr.content()
	result.FinishedAt = time.Now().UTC()
	return result
}

func classifyProcessOutcome(result *Result, ctx context.Context, err error, overflow bool) {
	switch {
	case overflow:
		result.Category, result.Error = "output-limit", "native executor output exceeds the byte limit"
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		result.Category, result.Error = "timeout", "native executor exceeded its execution deadline"
	case errors.Is(ctx.Err(), context.Canceled):
		result.Category, result.Error = "cancelled", "native executor was cancelled"
	case err != nil:
		result.Category, result.Error = "process-io", err.Error()
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			result.Category = "process-exit"
		}
	}
}

type boundedOutput struct {
	mu       sync.Mutex
	buffer   bytes.Buffer
	exceeded bool
	cancel   context.CancelFunc
}

func (output *boundedOutput) Write(data []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()
	remaining := MaxOutputBytes - output.buffer.Len()
	if len(data) <= remaining {
		return output.buffer.Write(data)
	}
	_, _ = output.buffer.Write(data[:remaining])
	output.exceeded = true
	output.cancel()
	return len(data), nil
}

func (output *boundedOutput) content() []byte {
	output.mu.Lock()
	defer output.mu.Unlock()
	return append([]byte(nil), output.buffer.Bytes()...)
}

func (output *boundedOutput) overflowed() bool {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.exceeded
}

var _ io.Writer = (*boundedOutput)(nil)
