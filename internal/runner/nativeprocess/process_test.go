//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package nativeprocess_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/runner/nativeprocess"
)

func TestProcessCapturesExitAndBoundsOutput(t *testing.T) {
	for _, test := range []struct {
		name, category string
		exit           int
	}{
		{"ok", "", 0}, {"failed", "process-exit", 3}, {"large", "output-limit", -1}, {"wait", "timeout", -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := nativeprocess.Run(context.Background(), nativeprocess.Command{
				Executable: os.Args[0], Args: []string{"-test.run=^TestProcessHelper$", "--", test.name},
				Directory: t.TempDir(), Env: []string{"NATIVE_PROCESS_HELPER=1", "GORACE=atexit_sleep_ms=0"}, Timeout: 150 * time.Millisecond,
			})
			if result.Category != test.category || result.ExitCode != test.exit {
				t.Fatalf("outcome = %#v", result)
			}
			if len(result.Stdout) > nativeprocess.MaxOutputBytes {
				t.Fatal("output escaped limit")
			}
			if test.name == "ok" && string(result.Stdout) != "hello" {
				t.Fatalf("stdout=%q", result.Stdout)
			}
		})
	}
}

func TestProcessHelper(t *testing.T) {
	if os.Getenv("NATIVE_PROCESS_HELPER") != "1" {
		return
	}
	switch os.Args[len(os.Args)-1] {
	case "ok":
		fmt.Print("hello")
	case "failed":
		os.Exit(3)
	case "large":
		fmt.Print(strings.Repeat("x", nativeprocess.MaxOutputBytes+1000))
		<-time.After(time.Second)
	case "wait":
		<-time.After(time.Hour)
	}
	os.Exit(0)
}

func TestProcessCancelledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := nativeprocess.Run(ctx, nativeprocess.Command{Executable: os.Args[0], Timeout: time.Second})
	if result.Category != "cancelled" || result.ExitCode != -1 {
		t.Fatalf("result=%#v", result)
	}
}
