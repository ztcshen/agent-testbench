package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

var environmentRestoreWorkspaceTempCounter atomic.Uint64

func writeEnvironmentRestoreWorkspaceFile(workspace string, relativePath string, content []byte, mode os.FileMode) (resultErr error) {
	clean, err := environmentRestoreWorkspaceRelativePath(relativePath)
	if err != nil {
		return err
	}
	root, err := openEnvironmentRestoreWorkspaceRoot(workspace, true)
	if err != nil {
		return err
	}
	defer func() {
		if err := root.Close(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close restore workspace root: %w", err))
		}
	}()
	parent := filepath.Dir(clean)
	if err := rejectEnvironmentRestoreWorkspaceSymlinks(root, parent); err != nil {
		return err
	}
	if parent != "." {
		if err := root.MkdirAll(parent, 0o755); err != nil {
			return fmt.Errorf("create restore workspace directory %s: %w", parent, err)
		}
	}
	if err := rejectEnvironmentRestoreWorkspaceSymlinks(root, clean); err != nil {
		return err
	}
	tempPath, file, err := createEnvironmentRestoreWorkspaceTempFile(root, parent, filepath.Base(clean))
	if err != nil {
		return fmt.Errorf("create restore workspace temporary file for %s: %w", clean, err)
	}
	keepTemp := true
	defer func() {
		if keepTemp {
			if err := root.Remove(tempPath); err != nil && !os.IsNotExist(err) {
				resultErr = errors.Join(resultErr, fmt.Errorf("remove restore workspace temporary file %s: %w", tempPath, err))
			}
		}
	}()
	if err := file.Chmod(mode.Perm()); err != nil {
		return closeEnvironmentRestoreWorkspaceTempFile(file, clean, fmt.Errorf("set restore workspace file permissions for %s: %w", clean, err))
	}
	if _, err := file.Write(content); err != nil {
		return closeEnvironmentRestoreWorkspaceTempFile(file, clean, fmt.Errorf("write restore workspace file %s: %w", clean, err))
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close restore workspace file %s: %w", clean, err)
	}
	if err := root.Rename(tempPath, clean); err != nil {
		return fmt.Errorf("replace restore workspace file %s: %w", clean, err)
	}
	keepTemp = false
	return nil
}

func closeEnvironmentRestoreWorkspaceTempFile(file *os.File, path string, operationErr error) error {
	if err := file.Close(); err != nil {
		return errors.Join(operationErr, fmt.Errorf("close restore workspace temporary file for %s: %w", path, err))
	}
	return operationErr
}

func createEnvironmentRestoreWorkspaceTempFile(root *os.Root, parent string, base string) (string, *os.File, error) {
	for attempt := 0; attempt < 100; attempt++ {
		sequence := environmentRestoreWorkspaceTempCounter.Add(1)
		name := fmt.Sprintf(".%s.agent-testbench-%d.tmp", base, sequence)
		if parent != "." {
			name = filepath.Join(parent, name)
		}
		file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return name, file, nil
		}
		if !os.IsExist(err) {
			return "", nil, err
		}
	}
	return "", nil, fmt.Errorf("exhausted temporary file names")
}

func readEnvironmentRestoreWorkspaceFile(workspace string, relativePath string) ([]byte, error) {
	raw, _, err := readEnvironmentRestoreWorkspaceFileWithInfo(workspace, relativePath)
	return raw, err
}

func readEnvironmentRestoreWorkspaceFileWithInfo(workspace string, relativePath string) (raw []byte, info os.FileInfo, resultErr error) {
	clean, err := environmentRestoreWorkspaceRelativePath(relativePath)
	if err != nil {
		return nil, nil, err
	}
	root, err := openEnvironmentRestoreWorkspaceRoot(workspace, false)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if err := root.Close(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close restore workspace root: %w", err))
		}
	}()
	if err := rejectEnvironmentRestoreWorkspaceSymlinks(root, clean); err != nil {
		return nil, nil, err
	}
	info, err = root.Lstat(clean)
	if err != nil {
		return nil, nil, fmt.Errorf("inspect restore workspace file %s: %w", clean, err)
	}
	raw, err = root.ReadFile(clean)
	if err != nil {
		return nil, nil, fmt.Errorf("read restore workspace file %s: %w", clean, err)
	}
	return raw, info, nil
}

func openEnvironmentRestoreWorkspaceRoot(workspace string, create bool) (*os.Root, error) {
	workspace = filepath.Clean(strings.TrimSpace(workspace))
	if workspace == "" || workspace == "." {
		return nil, fmt.Errorf("restore workspace is required")
	}
	if create {
		if err := os.MkdirAll(workspace, 0o755); err != nil {
			return nil, fmt.Errorf("create restore workspace: %w", err)
		}
	}
	info, err := os.Lstat(workspace)
	if err != nil {
		return nil, fmt.Errorf("inspect restore workspace: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("restore workspace must be a real directory, not a symlink")
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return nil, fmt.Errorf("open restore workspace root: %w", err)
	}
	return root, nil
}

func environmentRestoreWorkspaceRelativePath(path string) (string, error) {
	raw := strings.TrimSpace(path)
	if raw == "" || filepath.IsAbs(raw) {
		return "", fmt.Errorf("restore workspace file path must be relative: %s", raw)
	}
	clean := filepath.Clean(raw)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("restore workspace file path must stay inside the workspace: %s", raw)
	}
	return clean, nil
}

func rejectEnvironmentRestoreWorkspaceSymlinks(root *os.Root, path string) error {
	if path == "." || path == "" {
		return nil
	}
	current := ""
	for _, part := range strings.Split(filepath.Clean(path), string(os.PathSeparator)) {
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect restore workspace path %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("restore workspace path must not contain symlinks: %s", current)
		}
	}
	return nil
}
