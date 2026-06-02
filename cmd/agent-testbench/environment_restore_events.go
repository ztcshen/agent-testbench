package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type agentEventStreamContextKey struct{}

const (
	cliOutputFormatText       = "text"
	cliOutputFormatJSON       = "json"
	cliOutputFormatStreamJSON = "stream-json"

	agentCommandStatusStarted   = "started"
	agentCommandStatusRunning   = "running"
	agentCommandStatusCompleted = "completed"
	agentCommandStatusFailed    = "failed"

	dockerComposeCommandVersion = "version"
)

type agentEventStream struct {
	writer  io.Writer
	mu      sync.Mutex
	seq     int
	runID   string
	started time.Time
}

type agentStreamEvent struct {
	Type          string   `json:"type"`
	Seq           int      `json:"seq"`
	Timestamp     string   `json:"timestamp"`
	RunID         string   `json:"runId,omitempty"`
	EnvironmentID string   `json:"environmentId,omitempty"`
	Phase         string   `json:"phase,omitempty"`
	Status        string   `json:"status,omitempty"`
	Target        string   `json:"target,omitempty"`
	Message       string   `json:"message,omitempty"`
	Command       []string `json:"command,omitempty"`
	Workdir       string   `json:"workdir,omitempty"`
	ElapsedMs     int64    `json:"elapsedMs,omitempty"`
	RemainingMs   int64    `json:"remainingMs,omitempty"`
	Error         string   `json:"error,omitempty"`
	Report        any      `json:"report,omitempty"`
}

func contextWithEnvironmentRestoreEventStream(ctx context.Context, writer io.Writer) context.Context {
	return contextWithAgentEventStream(ctx, writer)
}

func contextWithAgentEventStream(ctx context.Context, writer io.Writer) context.Context {
	if writer == nil {
		return ctx
	}
	stream := &agentEventStream{
		writer:  writer,
		started: time.Now(),
	}
	return context.WithValue(ctx, agentEventStreamContextKey{}, stream)
}

func agentEventStreamFromContext(ctx context.Context) *agentEventStream {
	stream, ok := ctx.Value(agentEventStreamContextKey{}).(*agentEventStream)
	if !ok {
		return nil
	}
	return stream
}

func agentHasEventStream(ctx context.Context) bool {
	return agentEventStreamFromContext(ctx) != nil
}

func environmentRestoreEmitEvent(ctx context.Context, event agentStreamEvent) {
	agentEmitEvent(ctx, event)
}

func agentEmitEvent(ctx context.Context, event agentStreamEvent) {
	stream := agentEventStreamFromContext(ctx)
	if stream == nil {
		return
	}
	stream.mu.Lock()
	defer stream.mu.Unlock()
	stream.seq++
	event.Seq = stream.seq
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if event.RunID == "" {
		event.RunID = stream.runID
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return
	}
	if _, err := stream.writer.Write(append(raw, '\n')); err != nil {
		return
	}
}

func environmentRestoreEmitRunStarted(ctx context.Context, report environmentRestoreReport) {
	agentEmitRunStarted(ctx, report.RestoreID, "environment.restore", report.EnvironmentID, "environment restore started")
}

func agentEmitRunStarted(ctx context.Context, runID string, phase string, target string, message string) {
	stream := agentEventStreamFromContext(ctx)
	if stream != nil {
		stream.mu.Lock()
		stream.runID = strings.TrimSpace(runID)
		stream.started = time.Now()
		stream.mu.Unlock()
	}
	agentEmitEvent(ctx, agentStreamEvent{
		Type:    "run_started",
		Phase:   phase,
		Status:  "running",
		Target:  strings.TrimSpace(target),
		Message: strings.TrimSpace(message),
	})
}

func environmentRestoreEmitRunCompleted(ctx context.Context, report environmentRestoreReport) {
	status := "passed"
	if !report.OK {
		status = "failed"
	}
	agentEmitRunCompleted(ctx, "environment.restore", status, report.EnvironmentID, "environment restore completed", report.Error, report)
}

func agentEmitRunCompleted(ctx context.Context, phase string, status string, target string, message string, errText string, report any) {
	agentEmitEvent(ctx, agentStreamEvent{
		Type:    "run_completed",
		Phase:   strings.TrimSpace(phase),
		Status:  strings.TrimSpace(status),
		Target:  strings.TrimSpace(target),
		Message: strings.TrimSpace(message),
		Error:   strings.TrimSpace(errText),
		Report:  report,
	})
}

func newEnvironmentMigrationRunID(baseline bool) string {
	return environmentMigrationRunPhase(baseline) + "." + time.Now().UTC().Format("20060102T150405.000000000Z")
}

func environmentMigrationRunPhase(baseline bool) string {
	if baseline {
		return "environment.migration.baseline"
	}
	return "environment.migration.apply"
}

func environmentMigrationRunMessage(baseline bool, state string) string {
	action := "migration apply"
	if baseline {
		action = "migration baseline"
	}
	return action + " " + state
}

func environmentMigrationItemStatus(item environmentMigrationItem) string {
	return statusText(item.OK)
}

func environmentMigrationItemMessage(baseline bool, state string, item environmentMigrationItem) string {
	action := "apply migration"
	if baseline {
		action = "baseline migration"
	}
	if item.Version != "" {
		return action + " " + item.Version + " " + state
	}
	return action + " " + state
}

func environmentMigrationReportError(report environmentMigrationReport) string {
	if report.OK {
		return ""
	}
	return "one or more environment migrations failed"
}

func environmentRestoreEmitStep(ctx context.Context, eventType string, phase string, status string, target string, message string, errText string) {
	agentEmitStep(ctx, eventType, phase, status, target, message, errText)
}

func agentEmitStep(ctx context.Context, eventType string, phase string, status string, target string, message string, errText string) {
	agentEmitEvent(ctx, agentStreamEvent{
		Type:    eventType,
		Phase:   phase,
		Status:  status,
		Target:  strings.TrimSpace(target),
		Message: strings.TrimSpace(message),
		Error:   strings.TrimSpace(errText),
	})
}

func agentEmitCommand(ctx context.Context, status string, workdir string, command []string, started time.Time, message string, errText string) {
	eventType := "tool_observation"
	switch status {
	case agentCommandStatusStarted:
		eventType = "tool_call_started"
	case agentCommandStatusCompleted, agentCommandStatusFailed:
		eventType = "tool_call_completed"
	}
	elapsedMs := int64(0)
	if !started.IsZero() {
		elapsedMs = time.Since(started).Milliseconds()
	}
	agentEmitEvent(ctx, agentStreamEvent{
		Type:      eventType,
		Phase:     "command",
		Status:    status,
		Target:    restoreCommandTarget(command),
		Message:   truncateReportText(message, 200),
		Command:   append([]string(nil), command...),
		Workdir:   strings.TrimSpace(workdir),
		ElapsedMs: elapsedMs,
		Error:     strings.TrimSpace(errText),
	})
}

type agentObservedCommandOptions struct {
	Workdir   string
	Command   []string
	Input     string
	HasInput  bool
	Configure func(*exec.Cmd)
}

type agentObservedCommandResult struct {
	Output   string
	Error    string
	Err      error
	ExitCode int
}

func runAgentObservedCommand(ctx context.Context, options agentObservedCommandOptions) agentObservedCommandResult {
	if len(options.Command) == 0 {
		return agentObservedCommandResult{Error: "empty command", Err: errAgentObservedCommandEmpty, ExitCode: 1}
	}
	cmd := exec.CommandContext(ctx, options.Command[0], options.Command[1:]...)
	if options.Configure != nil {
		options.Configure(cmd)
	}
	cmd.Dir = options.Workdir
	if options.HasInput {
		cmd.Stdin = bytes.NewBufferString(options.Input)
	}
	started := time.Now()
	agentEmitCommand(ctx, agentCommandStatusStarted, options.Workdir, options.Command, started, "", "")
	resultCh := make(chan agentObservedCommandResult, 1)
	go func() {
		out, err := cmd.CombinedOutput()
		output := strings.TrimSpace(string(out))
		result := agentObservedCommandResult{Output: output, Err: err}
		if err != nil {
			result.ExitCode = 1
			if exitErr, ok := err.(*exec.ExitError); ok {
				result.ExitCode = exitErr.ExitCode()
			}
			result.Error = err.Error()
			if output != "" {
				result.Error += ": " + output
			}
		}
		resultCh <- result
	}()
	if !agentHasEventStream(ctx) {
		return <-resultCh
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case result := <-resultCh:
			status := agentCommandStatusCompleted
			if result.Error != "" {
				status = agentCommandStatusFailed
			}
			agentEmitCommand(ctx, status, options.Workdir, options.Command, started, result.Output, result.Error)
			return result
		case <-ticker.C:
			agentEmitCommand(ctx, agentCommandStatusRunning, options.Workdir, options.Command, started, "command still running", "")
		}
	}
}

var errAgentObservedCommandEmpty = errString("empty command")

type errString string

func (err errString) Error() string {
	return string(err)
}

func restoreCommandTarget(command []string) string {
	if len(command) == 0 {
		return ""
	}
	if len(command) >= 2 && command[0] == "docker" && command[1] == "compose" {
		for _, part := range command[2:] {
			switch part {
			case "build", "config", "down", "exec", "images", "ps", "pull", "up", dockerComposeCommandVersion:
				return "docker compose " + part
			}
		}
		return "docker compose"
	}
	if len(command) >= 2 && command[0] == "git" {
		return strings.Join(command[:2], " ")
	}
	return command[0]
}
