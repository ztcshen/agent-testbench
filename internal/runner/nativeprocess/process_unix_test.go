//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package nativeprocess_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"agent-testbench/internal/runner/nativeprocess"
)

func TestCancellationStopsOwnedDescendants(t *testing.T) {
	dir := t.TempDir()
	result := nativeprocess.Run(context.Background(), nativeprocess.Command{
		Executable: "/bin/sh", Args: []string{"-c", "(sleep 0.4; touch escaped) & wait"},
		Directory: dir, Env: os.Environ(), Timeout: 75 * time.Millisecond,
	})
	if result.Category != "timeout" {
		t.Fatalf("result=%#v", result)
	}
	time.Sleep(450 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(dir, "escaped")); !os.IsNotExist(err) {
		t.Fatalf("child survived cancellation: %v", err)
	}
}

func TestOutputCaptureFailureIsNotASuccessfulProcessExit(t *testing.T) {
	result := nativeprocess.Run(context.Background(), nativeprocess.Command{
		Executable: "/bin/sh", Args: []string{"-c", "printf '{}'; sleep 5 &"},
		Directory: t.TempDir(), Env: os.Environ(), Timeout: 3 * time.Second,
	})
	if result.Category != "process-io" || result.ExitCode != 0 {
		t.Fatalf("result=%#v", result)
	}
}
