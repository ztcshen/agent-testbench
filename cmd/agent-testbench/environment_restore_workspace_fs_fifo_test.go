//go:build darwin || linux || freebsd || openbsd || netbsd

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestEnvironmentRestoreWorkspaceWriterAtomicallyReplacesFIFO(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "projected.txt")
	if err := syscall.Mkfifo(target, 0o600); err != nil {
		t.Fatalf("create fifo fixture: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- writeEnvironmentRestoreWorkspaceFile(workspace, "projected.txt", []byte("projected"), 0o600)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("replace fifo: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("workspace writer blocked while replacing fifo")
	}

	info, err := os.Lstat(target)
	if err != nil {
		t.Fatalf("inspect replaced fifo: %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("replaced fifo mode = %s, want regular file", info.Mode())
	}
	assertFileContent(t, target, "projected")
}
