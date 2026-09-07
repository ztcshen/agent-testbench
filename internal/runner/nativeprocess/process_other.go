//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly

package nativeprocess

import (
	"errors"
	"os/exec"
)

func configureProcess(_ *exec.Cmd) error {
	return errors.New("lightweight direct execution requires a platform with supported process groups")
}

func stopProcessGroup(_ *exec.Cmd) error { return nil }
