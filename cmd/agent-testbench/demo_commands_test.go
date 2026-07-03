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

func TestDemoCommandRefusesCleanForExistingOutputDir(t *testing.T) {
	outputDir := t.TempDir()
	env := []string{"AGENT_TESTBENCH_CONFIG_HOME=" + t.TempDir()}

	out := runCLIFailsWithEnv(t, env, "demo", "--output-dir", outputDir, "--clean")
	if !strings.Contains(out, "refuses --clean with pre-existing --output-dir") {
		t.Fatalf("demo should refuse cleaning an existing output dir:\n%s", out)
	}
}

func TestDemoCommandCleanAllowsImplicitTemporaryOutput(t *testing.T) {
	env := []string{"AGENT_TESTBENCH_CONFIG_HOME=" + t.TempDir()}

	out := runCLIWithEnv(t, env, "demo", "--clean")
	if !strings.Contains(out, "Demo output cleanup: enabled") {
		t.Fatalf("demo --clean should report cleanup:\n%s", out)
	}
	if strings.Contains(out, "Next:") {
		t.Fatalf("demo --clean should not print an inspect command for removed output:\n%s", out)
	}
}

func TestDemoInspectCommandUsesRunnableStoreReference(t *testing.T) {
	tests := []struct {
		name        string
		storeRef    string
		resolvedURL string
		want        string
		wantOK      bool
	}{
		{
			name:        "default sqlite store",
			resolvedURL: "sqlite:///tmp/agent-testbench-demo/store.sqlite",
			want:        "sqlite:///tmp/agent-testbench-demo/store.sqlite",
			wantOK:      true,
		},
		{
			name:        "named credentialed store",
			storeRef:    "team-smoke",
			resolvedURL: "mysql://user:secret@example.com:3306/agent_testbench_smoke?tls=false",
			want:        "team-smoke",
			wantOK:      true,
		},
		{
			name:        "explicit credentialed dsn",
			storeRef:    "postgres://user:secret@example.com:5432/agent_testbench_smoke?sslmode=disable",
			resolvedURL: "postgres://user:secret@example.com:5432/agent_testbench_smoke?sslmode=disable",
			wantOK:      false,
		},
		{
			name:        "explicit safe sqlite dsn",
			storeRef:    "sqlite:///tmp/agent-testbench-demo/store.sqlite",
			resolvedURL: "sqlite:///tmp/agent-testbench-demo/store.sqlite",
			want:        "sqlite:///tmp/agent-testbench-demo/store.sqlite",
			wantOK:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := demoInspectStoreReference(tt.storeRef, tt.resolvedURL)
			if ok != tt.wantOK || got != tt.want {
				t.Fatalf("demoInspectStoreReference() = %q, %t; want %q, %t", got, ok, tt.want, tt.wantOK)
			}
			if strings.Contains(got, "xxxxx") {
				t.Fatalf("inspect reference should not use a masked credential placeholder: %q", got)
			}
		})
	}
}

func TestDemoMySQLStoreGuard(t *testing.T) {
	if err := requireSafeDemoMySQLStore("mysql://user:secret@example.com:3306/agent_testbench_smoke?tls=false"); err != nil {
		t.Fatalf("safe MySQL demo store rejected: %v", err)
	}

	err := requireSafeDemoMySQLStore("mysql://user:secret@example.com:3306/business_prod?tls=false")
	if err == nil {
		t.Fatal("unsafe MySQL demo store unexpectedly accepted")
	}
	message := err.Error()
	if !strings.Contains(message, "business_prod") || strings.Contains(message, "secret") {
		t.Fatalf("unsafe MySQL error should name only the database, got: %s", message)
	}
}
