package controlplane

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindStaticDirPrefersExecutableBundle(t *testing.T) {
	executablePath, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	executableStaticDir := filepath.Join(filepath.Dir(executablePath), "control-plane", "static")
	if _, err := os.Stat(executableStaticDir); err == nil {
		t.Skipf("test executable already has static assets at %s", executableStaticDir)
	} else if !os.IsNotExist(err) {
		t.Fatalf("inspect executable static dir: %v", err)
	}
	if err := os.MkdirAll(executableStaticDir, 0o755); err != nil {
		t.Fatalf("create executable static dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(filepath.Join(filepath.Dir(executablePath), "control-plane"))
	})

	workingDir := t.TempDir()
	t.Chdir(workingDir)
	workingStaticDir := filepath.Join(workingDir, "control-plane", "static")
	if err := os.MkdirAll(workingStaticDir, 0o755); err != nil {
		t.Fatalf("create working static dir: %v", err)
	}

	if got := findStaticDir(); got != executableStaticDir {
		t.Fatalf("findStaticDir() = %q; want executable-bundled static dir %q", got, executableStaticDir)
	}
}
