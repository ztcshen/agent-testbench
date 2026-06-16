package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func withProfileCatalogWriteLock(storeIdentity string, fn func() error) (err error) {
	lockPath, err := profileCatalogWriteLockPath(storeIdentity)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() {
		if unlockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); unlockErr != nil && err == nil {
			err = unlockErr
		}
	}()
	return fn()
}

func profileCatalogWriteLockPath(storeIdentity string) (string, error) {
	storeIdentity = strings.TrimSpace(storeIdentity)
	if storeIdentity == "" {
		storeIdentity = "default"
	}
	sum := sha256.Sum256([]byte(storeIdentity))
	dir := filepath.Join(os.TempDir(), "agent-testbench-catalog-locks")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".lock"), nil
}
