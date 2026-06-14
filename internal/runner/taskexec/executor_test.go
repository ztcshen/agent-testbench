package taskexec

import (
	"context"
	"strings"
	"testing"
)

func TestExecuteShellCapturesOutputAndExitCode(t *testing.T) {
	output, exitCode, err := Execute(context.Background(), "shell", "printf task-ok")

	if err != nil {
		t.Fatalf("execute shell: %v", err)
	}
	if exitCode != 0 || output != "task-ok" {
		t.Fatalf("shell result output=%q exit=%d", output, exitCode)
	}
}

func TestExecuteShellReportsExitCode(t *testing.T) {
	output, exitCode, err := Execute(context.Background(), "shell", "printf failure && exit 7")

	if err == nil {
		t.Fatal("expected shell execution error")
	}
	if exitCode != 7 || output != "failure" {
		t.Fatalf("shell failure output=%q exit=%d", output, exitCode)
	}
}

func TestSplitCommandLineHonorsQuotesAndEscapes(t *testing.T) {
	args, err := splitCommandLine(`case run "case one" --label='nightly run' escaped\ value`)

	if err != nil {
		t.Fatalf("split command line: %v", err)
	}
	if strings.Join(args, "|") != "case|run|case one|--label=nightly run|escaped value" {
		t.Fatalf("args = %#v", args)
	}
}

func TestSplitCommandLineRejectsUnterminatedQuote(t *testing.T) {
	_, err := splitCommandLine(`case run "missing`)

	if err == nil || !strings.Contains(err.Error(), "unterminated quote") {
		t.Fatalf("expected unterminated quote error, got %v", err)
	}
}
