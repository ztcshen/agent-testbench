package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDemoCommandRunsLocalAPIAndIndexesEvidence(t *testing.T) {
	outputDir := t.TempDir()
	env := []string{"AGENT_TESTBENCH_CONFIG_HOME=" + t.TempDir()}

	out := runCLIWithEnv(t, env, "demo", "--output-dir", outputDir)
	for _, want := range []string{
		"AgentTestBench Demo",
		"Case Run: demo-create-item",
		"Case: case.create-item",
		"Status: passed",
		"Evidence bundle:",
		"Store: sqlite://",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("demo output missing %q:\n%s", want, out)
		}
	}

	storeRef := "sqlite://" + filepath.Join(outputDir, "store.sqlite")
	runs := runCLIWithEnv(t, env, "case", "inspect", "--view", "runs", "--store", storeRef, "--json")
	if !strings.Contains(runs, "demo-create-item") || !strings.Contains(runs, "case.create-item") {
		t.Fatalf("demo run was not indexed in %s:\n%s", storeRef, runs)
	}
}
