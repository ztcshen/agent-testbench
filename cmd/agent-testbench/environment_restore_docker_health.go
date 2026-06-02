package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const environmentRestoreDockerStateExited = "exited"

func waitEnvironmentRestoreHealthChecks(ctx context.Context, checks []any, timeout time.Duration, workspace string, composeBaseArgs []string) []environmentRestoreHealthCheckReport {
	out := make([]environmentRestoreHealthCheckReport, 0, len(checks))
	deadline := time.Now().Add(timeout)
	for _, raw := range checks {
		check, ok := environmentRestoreHealthCheckFromAny(raw)
		if !ok {
			continue
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			check.Error = "health check deadline reached before probe started"
			out = append(out, check)
			continue
		}
		switch check.Kind {
		case "url", "":
			if check.URL == "" {
				continue
			}
			out = append(out, waitEnvironmentRestoreURLHealthCheck(ctx, check, remaining))
		case "tcp":
			if check.Address == "" {
				continue
			}
			out = append(out, waitEnvironmentRestoreTCPHealthCheck(ctx, check, remaining))
		case "command":
			if check.Command == "" {
				continue
			}
			out = append(out, waitEnvironmentRestoreCommandHealthCheck(ctx, check, remaining, workspace))
		case "compose-service":
			if check.Service == "" {
				continue
			}
			out = append(out, waitEnvironmentRestoreComposeServiceHealthCheck(ctx, check, remaining, workspace, composeBaseArgs))
		case "container":
			if check.Container == "" {
				continue
			}
			out = append(out, waitEnvironmentRestoreContainerHealthCheck(ctx, check, remaining))
		default:
			check.Error = "unsupported health check kind: " + check.Kind
			out = append(out, check)
		}
	}
	return out
}

func environmentRestoreHealthCheckFromAny(raw any) (environmentRestoreHealthCheckReport, bool) {
	item, ok := raw.(map[string]any)
	if !ok {
		return environmentRestoreHealthCheckReport{}, false
	}
	kind := strings.TrimSpace(valueString(item["kind"]))
	if kind == "" && strings.TrimSpace(valueString(item["url"])) != "" {
		kind = "url"
	}
	return environmentRestoreHealthCheckReport{
		ID:        strings.TrimSpace(valueString(item["id"])),
		Kind:      kind,
		URL:       strings.TrimSpace(valueString(item["url"])),
		Address:   strings.TrimSpace(valueString(item["address"])),
		Command:   strings.TrimSpace(valueString(item["command"])),
		Service:   strings.TrimSpace(valueString(item["service"])),
		Container: strings.TrimSpace(valueString(item["container"])),
		Expect:    strings.ToLower(strings.TrimSpace(valueString(item["expect"]))),
		OneShot:   boolFromReportAny(item["oneShot"]),
	}, true
}

func waitEnvironmentRestoreURLHealthCheck(ctx context.Context, check environmentRestoreHealthCheckReport, timeout time.Duration) environmentRestoreHealthCheckReport {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	progress := newEnvironmentRestoreHealthProgress(ctx, check, timeout)
	progress.start()
	var lastErr string
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, check.URL, nil)
		if err != nil {
			check.Error = err.Error()
			progress.done(false, check.Error)
			return check
		}
		resp, err := client.Do(req)
		if err == nil {
			check.StatusCode = resp.StatusCode
			if closeErr := resp.Body.Close(); closeErr != nil {
				lastErr = closeErr.Error()
				check.Error = lastErr
				progress.done(false, check.Error)
				return check
			}
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				check.OK = true
				check.Error = ""
				progress.done(true, fmt.Sprintf("HTTP %d", resp.StatusCode))
				return check
			}
			lastErr = fmt.Sprintf("health check returned HTTP %d", resp.StatusCode)
		} else {
			lastErr = err.Error()
		}
		var keepWaiting bool
		check, keepWaiting = waitEnvironmentRestoreHealthPoll(ctx, check, &progress, deadline, lastErr)
		if !keepWaiting {
			return check
		}
	}
}

func waitEnvironmentRestoreTCPHealthCheck(ctx context.Context, check environmentRestoreHealthCheckReport, timeout time.Duration) environmentRestoreHealthCheckReport {
	deadline := time.Now().Add(timeout)
	progress := newEnvironmentRestoreHealthProgress(ctx, check, timeout)
	progress.start()
	var lastErr string
	for {
		dialer := net.Dialer{Timeout: 2 * time.Second}
		conn, err := dialer.DialContext(ctx, "tcp", check.Address)
		if err == nil {
			if closeErr := conn.Close(); closeErr != nil {
				check.Error = closeErr.Error()
				progress.done(false, check.Error)
				return check
			}
			check.OK = true
			check.Error = ""
			progress.done(true, "tcp connected")
			return check
		}
		lastErr = err.Error()
		var keepWaiting bool
		check, keepWaiting = waitEnvironmentRestoreHealthPoll(ctx, check, &progress, deadline, lastErr)
		if !keepWaiting {
			return check
		}
	}
}

func waitEnvironmentRestoreHealthPoll(ctx context.Context, check environmentRestoreHealthCheckReport, progress *environmentRestoreHealthProgress, deadline time.Time, lastErr string) (environmentRestoreHealthCheckReport, bool) {
	if time.Now().After(deadline) {
		check.Error = lastErr
		progress.done(false, check.Error)
		return check, false
	}
	progress.waiting(lastErr, deadline)
	select {
	case <-ctx.Done():
		check.Error = ctx.Err().Error()
		progress.done(false, check.Error)
		return check, false
	case <-time.After(250 * time.Millisecond):
		return check, true
	}
}

func waitEnvironmentRestoreCommandHealthCheck(ctx context.Context, check environmentRestoreHealthCheckReport, timeout time.Duration, workspace string) environmentRestoreHealthCheckReport {
	return waitEnvironmentRestoreCommand(ctx, check, timeout, workspace, []string{"/bin/sh", "-c", check.Command}, func(check *environmentRestoreHealthCheckReport, output string) bool {
		check.Output = truncateReportText(output, 200)
		return true
	})
}

func waitEnvironmentRestoreComposeServiceHealthCheck(ctx context.Context, check environmentRestoreHealthCheckReport, timeout time.Duration, workspace string, composeBaseArgs []string) environmentRestoreHealthCheckReport {
	if len(composeBaseArgs) == 0 {
		check.Error = "compose service health check requires composeFile"
		return check
	}
	command := append(append([]string{"docker", "compose"}, composeBaseArgs...), "ps", "-a", "--format", "json", check.Service)
	return waitEnvironmentRestoreCommand(ctx, check, timeout, workspace, command, func(check *environmentRestoreHealthCheckReport, output string) bool {
		check.Output = truncateReportText(output, 200)
		state, health, exitCode, hasExitCode := parseComposeServiceHealth(output)
		check.State = state
		check.Health = health
		if hasExitCode {
			check.ExitCode = exitCode
		}
		return state == "running" && (health == "" || health == "healthy") || environmentRestoreExitedCompleted(check, state, exitCode, hasExitCode)
	})
}

func environmentRestoreExitedCompleted(check *environmentRestoreHealthCheckReport, state string, exitCode int, hasExitCode bool) bool {
	if state != environmentRestoreDockerStateExited || !hasExitCode || exitCode != 0 {
		return false
	}
	return check.OneShot || check.Expect == "completed" || check.Expect == "service_completed_successfully"
}

func waitEnvironmentRestoreContainerHealthCheck(ctx context.Context, check environmentRestoreHealthCheckReport, timeout time.Duration) environmentRestoreHealthCheckReport {
	command := []string{"docker", "inspect", "--format", "{{.State.Status}}\t{{if .State.Health}}{{.State.Health.Status}}{{end}}\t{{.State.ExitCode}}", check.Container}
	return waitEnvironmentRestoreCommand(ctx, check, timeout, "", command, func(check *environmentRestoreHealthCheckReport, output string) bool {
		check.Output = truncateReportText(output, 200)
		state, health, exitCode, hasExitCode := parseContainerHealth(output)
		check.State = state
		check.Health = health
		if hasExitCode {
			check.ExitCode = exitCode
		}
		return check.State == "running" && (check.Health == "" || check.Health == "healthy") || environmentRestoreExitedCompleted(check, check.State, exitCode, hasExitCode)
	})
}

func parseContainerHealth(output string) (string, string, int, bool) {
	parts := strings.Split(strings.TrimSpace(output), "\t")
	if len(parts) >= 3 {
		exitCode, ok := parseHealthExitCode(parts[2])
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), exitCode, ok
	}
	fields := strings.Fields(output)
	if len(fields) == 0 {
		return "", "", 0, false
	}
	state := strings.TrimSpace(fields[0])
	if len(fields) == 1 {
		return state, "", 0, false
	}
	if exitCode, ok := parseHealthExitCode(fields[len(fields)-1]); ok {
		health := ""
		if len(fields) > 2 {
			health = strings.TrimSpace(fields[1])
		}
		return state, health, exitCode, true
	}
	return state, strings.TrimSpace(fields[1]), 0, false
}

func parseHealthExitCode(value string) (int, bool) {
	exitCode, err := strconv.Atoi(strings.TrimSpace(value))
	return exitCode, err == nil
}

func waitEnvironmentRestoreCommand(ctx context.Context, check environmentRestoreHealthCheckReport, timeout time.Duration, workspace string, command []string, ok func(*environmentRestoreHealthCheckReport, string) bool) environmentRestoreHealthCheckReport {
	deadline := time.Now().Add(timeout)
	progress := newEnvironmentRestoreHealthProgress(ctx, check, timeout)
	progress.start()
	var lastErr string
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			check.Error = firstNonEmpty(lastErr, "health check deadline reached before probe started")
			progress.done(false, check.Error)
			return check
		}
		commandCtx, cancel := context.WithTimeout(ctx, minDuration(2*time.Second, remaining))
		output, errText := runRestoreCommand(commandCtx, workspace, command)
		if commandCtx.Err() == context.DeadlineExceeded {
			errText = "health command timed out: " + commandCtx.Err().Error()
		}
		cancel()
		if errText == "" && ok(&check, output) {
			check.OK = true
			check.Error = ""
			if check.Output == "" {
				check.Output = truncateReportText(output, 200)
			}
			progress.done(true, "ready")
			return check
		}
		if errText != "" {
			lastErr = errText
		} else {
			lastErr = "health command did not report ready"
		}
		if time.Now().After(deadline) {
			check.Error = lastErr
			if check.Output == "" {
				check.Output = truncateReportText(output, 200)
			}
			progress.done(false, check.Error)
			return check
		}
		progress.waiting(lastErr, deadline)
		select {
		case <-ctx.Done():
			check.Error = ctx.Err().Error()
			progress.done(false, check.Error)
			return check
		case <-time.After(250 * time.Millisecond):
		}
	}
}

type environmentRestoreHealthProgress struct {
	ctx       context.Context
	target    string
	timeout   time.Duration
	lastPrint time.Time
}

func newEnvironmentRestoreHealthProgress(ctx context.Context, check environmentRestoreHealthCheckReport, timeout time.Duration) environmentRestoreHealthProgress {
	return environmentRestoreHealthProgress{
		ctx:     ctx,
		target:  environmentRestoreHealthProgressTarget(check),
		timeout: timeout,
	}
}

func (p *environmentRestoreHealthProgress) start() {
	environmentRestoreProgressf(p.ctx, "restore health checking: %s timeout=%s\n", p.target, p.timeout)
	environmentRestoreEmitStep(p.ctx, "step_started", "health.wait", "running", p.target, "health check started", "")
	p.lastPrint = time.Now()
}

func (p *environmentRestoreHealthProgress) waiting(lastErr string, deadline time.Time) {
	if time.Since(p.lastPrint) < 2*time.Second {
		return
	}
	remaining := time.Until(deadline).Round(time.Second)
	if remaining < 0 {
		remaining = 0
	}
	environmentRestoreProgressf(p.ctx, "restore health waiting: %s last=%s remaining=%s\n", p.target, lastErr, remaining)
	environmentRestoreEmitEvent(p.ctx, agentStreamEvent{
		Type:        "tool_observation",
		Phase:       "health.wait",
		Status:      "waiting",
		Target:      p.target,
		Message:     truncateReportText(lastErr, 200),
		RemainingMs: remaining.Milliseconds(),
	})
	p.lastPrint = time.Now()
}

func (p *environmentRestoreHealthProgress) done(ok bool, detail string) {
	state := "failed"
	if ok {
		state = "ok"
	}
	environmentRestoreProgressf(p.ctx, "restore health %s: %s last=%s\n", state, p.target, detail)
	status := "failed"
	errText := detail
	if ok {
		status = "passed"
		errText = ""
	}
	environmentRestoreEmitStep(p.ctx, "step_completed", "health.wait", status, p.target, detail, errText)
}

func environmentRestoreHealthProgressTarget(check environmentRestoreHealthCheckReport) string {
	switch strings.TrimSpace(check.Kind) {
	case "url", "":
		return "url " + check.URL
	case "tcp":
		return "tcp " + check.Address
	case "compose-service":
		return "compose-service " + check.Service
	case "container":
		return "container " + check.Container
	case "command":
		if check.ID != "" {
			return "command " + check.ID
		}
		return "command health check"
	default:
		if check.ID != "" {
			return check.Kind + " " + check.ID
		}
		return check.Kind
	}
}

func parseComposeServiceHealth(output string) (string, string, int, bool) {
	output = strings.TrimSpace(output)
	if output == "" {
		return "", "", 0, false
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(output), &object); err == nil && object != nil {
		state, health, exitCode, hasExitCode := composeServiceHealthFromObject(object)
		return state, health, exitCode, hasExitCode
	}
	var array []map[string]any
	if err := json.Unmarshal([]byte(output), &array); err == nil && len(array) > 0 {
		state, health, exitCode, hasExitCode := composeServiceHealthFromObject(array[0])
		return state, health, exitCode, hasExitCode
	}
	lower := strings.ToLower(output)
	state := ""
	health := ""
	if strings.Contains(lower, "running") {
		state = "running"
	}
	if strings.Contains(lower, "unhealthy") {
		health = "unhealthy"
	} else if strings.Contains(lower, "healthy") {
		health = "healthy"
	}
	return state, health, 0, false
}

func composeServiceHealthFromObject(object map[string]any) (string, string, int, bool) {
	state := strings.ToLower(valueString(firstNonNil(object["State"], object["state"])))
	health := strings.ToLower(valueString(firstNonNil(object["Health"], object["health"])))
	exitCode, ok := intFromAny(firstNonNil(object["ExitCode"], object["exitCode"], object["Exit"], object["exit"]))
	return state, health, exitCode, ok
}

func intFromAny(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case string:
		typed = strings.TrimSpace(typed)
		if typed == "" {
			return 0, false
		}
		var parsed int
		if _, err := fmt.Sscanf(typed, "%d", &parsed); err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func minDuration(left time.Duration, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
}
