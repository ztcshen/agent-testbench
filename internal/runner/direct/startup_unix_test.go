//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package direct_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"agent-testbench/internal/runner/direct"
	"agent-testbench/internal/store"
)

func TestDirectStartupFailureStillRecordsOutcome(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(f.binding.ProjectDir, "adapter-command")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexec \"$DIRECT_TEST_BINARY\" '-test.run=^TestDirectAdapter$'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DIRECT_TEST_BINARY", os.Args[0])
	f.binding.Executable, f.binding.Args = path, nil
	f.binding.EnvironmentNames = append(f.binding.EnvironmentNames, "DIRECT_TEST_BINARY")
	if _, err := direct.Register(context.Background(), f.runtime, direct.RegisterRequest{Binding: f.binding}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	report := f.run(t, "pass", "not-executable")
	if report.FailureCategory != "process-start" || report.Status != store.StatusFailed || f.hits.Load() != 0 {
		t.Fatalf("report=%#v", report)
	}
}

func TestDirectRegistrationRejectsNamedPipeSource(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(f.binding.ProjectDir, "case.json")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := direct.Register(context.Background(), f.runtime, direct.RegisterRequest{Binding: f.binding}); err == nil {
		t.Fatal("accepted a pipe as source")
	}
}

func TestDirectRefusesEscapingSourceAndExistingEvidenceDirectory(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(f.binding.ProjectDir, "case.json")
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if _, err := direct.Register(context.Background(), f.runtime, direct.RegisterRequest{Binding: f.binding}); !errors.Is(err, direct.ErrInvalid) {
		t.Fatalf("accepted escaping source: %v", err)
	}
	dir := filepath.Join(f.binding.EvidenceDir, "occupied")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "result.json")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	report := f.run(t, "pass", "occupied")
	if report.FailureCategory != "evidence" || f.hits.Load() != 0 {
		t.Fatalf("report=%#v", report)
	}
	raw, err := os.ReadFile(marker)
	if err != nil || string(raw) != "keep" {
		t.Fatalf("existing Evidence changed: %q %v", raw, err)
	}
}
