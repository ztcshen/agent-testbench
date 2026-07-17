package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvironmentRestoreProgressRedactsEnvironmentSecrets(t *testing.T) {
	secret := "sentinel-progress-secret"
	var progress strings.Builder
	ctx := contextWithEnvironmentRestoreProgress(context.Background(), &progress)
	ctx = contextWithEnvironmentOutputRedactor(ctx, map[string]any{
		"env": map[string]any{"API_TOKEN": secret},
	})
	environmentRestoreProgressf(ctx, "restore health failed: %s\n", secret)
	assertEnvironmentOutputHasNoSecrets(t, progress.String(), secret)
}

func TestEnvironmentRestoreOutputAndSummaryDoNotLeakSecrets(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	workspace := filepath.Join(t.TempDir(), "workspace")
	envSecret := "sentinel-restore-env-secret"
	commandSecret := "sentinel-unkeyed-command-secret"
	fileSecret := "sentinel-generated-file-secret"
	generatedSource := filepath.Join(t.TempDir(), "runtime.env")
	writeFile(t, generatedSource, "FILE_SECRET="+fileSecret+"\n")
	startCommand := "printf '%s\\n' '" + commandSecret + "'; exit 17"

	registerOutput := runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.output.redaction",
		"--start-command", startCommand,
		"--compose-generated-file", "runtime.env="+generatedSource,
		"--compose-env", "APP_RUNTIME_VALUE="+envSecret,
		"--health-command", "true",
		"--verification-workflow", "workflow.output-redaction",
		"--json",
	)
	assertEnvironmentOutputHasNoSecrets(t, registerOutput, envSecret, commandSecret, fileSecret, startCommand)

	jsonOutput := runCLIFails(t, "environment", "restore",
		"--store", "sqlite://"+storePath,
		"--workspace", workspace,
		"--execute",
		"--json",
		"env.output.redaction",
	)
	assertEnvironmentOutputHasNoSecrets(t, jsonOutput, envSecret, commandSecret, fileSecret, startCommand)
	textOutput := runCLIFails(t, "environment", "restore",
		"--store", "sqlite://"+storePath,
		"--workspace", workspace,
		"--execute",
		"env.output.redaction",
	)
	assertEnvironmentOutputHasNoSecrets(t, textOutput, envSecret, commandSecret, fileSecret, startCommand)

	streamOutput := runCLIFails(t, "environment", "restore",
		"--store", "sqlite://"+storePath,
		"--workspace", workspace,
		"--execute",
		"--output-format", "stream-json",
		"env.output.redaction",
	)
	assertEnvironmentOutputHasNoSecrets(t, streamOutput, envSecret, commandSecret, fileSecret, startCommand)

	runtime, err := openStore(context.Background(), "sqlite://"+storePath)
	if err != nil {
		t.Fatalf("open Store: %v", err)
	}
	defer closeCLIStore(runtime)
	env, err := runtime.GetEnvironment(context.Background(), "env.output.redaction")
	if err != nil {
		t.Fatalf("get environment: %v", err)
	}
	assertEnvironmentOutputHasNoSecrets(t, env.SummaryJSON, envSecret, commandSecret, fileSecret, startCommand)

	rawGenerated, err := os.ReadFile(filepath.Join(workspace, "runtime.env"))
	if err != nil {
		t.Fatalf("read projected generated file: %v", err)
	}
	if !strings.Contains(string(rawGenerated), fileSecret) {
		t.Fatalf("execution projection should retain the real generated content: %q", rawGenerated)
	}
}

func assertEnvironmentOutputHasNoSecrets(t *testing.T, output string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if strings.Contains(output, secret) {
			t.Fatalf("environment output leaked sentinel secret %q:\n%s", secret, output)
		}
	}
}
