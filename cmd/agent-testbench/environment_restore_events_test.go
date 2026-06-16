package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAgentObservedCommandReturnsWhenContextTimesOutWithChildProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	started := time.Now()
	result := runAgentObservedCommand(ctx, agentObservedCommandOptions{
		Command: []string{"/bin/sh", "-c", "sleep 5"},
	})
	elapsed := time.Since(started)

	if elapsed > time.Second {
		t.Fatalf("observed command did not return promptly after timeout: elapsed=%s result=%#v", elapsed, result)
	}
	if !errors.Is(result.Err, context.DeadlineExceeded) || !strings.Contains(result.Error, "timed out") || result.ExitCode != 124 {
		t.Fatalf("timeout result = %#v", result)
	}
}

func TestEnvironmentRestoreStreamJSONEmitsPlanProgressAndTimeout(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	workspace := filepath.Join(t.TempDir(), "workspace")
	fakeDockerEnv, _ := fakeDockerCommand(t)
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "services:\n  web:\n    image: alpine:3.20\n")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.restore.plan.timeout",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--compose-service", "web",
		"--verification-workflow", "workflow.core-10",
	)

	env := append([]string{}, fakeDockerEnv...)
	env = append(env,
		"AGENT_TESTBENCH_FAKE_DOCKER_COMPOSE_VERSION_SLEEP=1",
		"AGENT_TESTBENCH_RESTORE_PLAN_PROGRESS_INTERVAL_MS=1",
		"AGENT_TESTBENCH_RESTORE_PLAN_TIMEOUT_MS=30",
	)
	out := runCLIFailsWithEnv(t, env, "environment", "restore",
		"--store", "sqlite://"+storePath,
		"--workspace", workspace,
		"--execute",
		"--output-format", "stream-json",
		"env.restore.plan.timeout",
	)
	events := decodeAgentStreamEvents(t, jsonLinesOnly(out))
	if !agentStreamHasEvent(events, "tool_observation", "environment.restore.plan", "waiting", "docker.compose.version") {
		t.Fatalf("stream missing restore plan waiting observation: %#v", events)
	}
	if !agentStreamHasEvent(events, "step_completed", "environment.restore.plan", "failed", "env.restore.plan.timeout") {
		t.Fatalf("stream missing restore plan failed completion: %#v", events)
	}
	if !agentStreamHasEvent(events, "run_completed", "environment.restore", "failed", "env.restore.plan.timeout") {
		t.Fatalf("stream missing failed run completion: %#v", events)
	}
	last := events[len(events)-1]
	rawReport, err := json.Marshal(last["report"])
	if err != nil {
		t.Fatalf("marshal timeout report: %v", err)
	}
	var report environmentRestoreReport
	if err := json.Unmarshal(rawReport, &report); err != nil {
		t.Fatalf("decode timeout report: %v\n%s", err, rawReport)
	}
	if report.OK || !strings.Contains(report.Error, "environment restore plan timed out") || report.EnvironmentID != "env.restore.plan.timeout" {
		t.Fatalf("timeout report = %#v", report)
	}
}

func jsonLinesOnly(output string) string {
	lines := []string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "{") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}
