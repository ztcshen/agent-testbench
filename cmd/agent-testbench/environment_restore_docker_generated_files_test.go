package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareEnvironmentRestoreGeneratedFilesPreservesWhitespace(t *testing.T) {
	workspace := t.TempDir()
	want := "\n#!/bin/sh\nprintf 'ready'\n"
	reports := prepareEnvironmentRestoreGeneratedFiles(map[string]any{
		"generatedFiles": map[string]any{
			"scripts/run.sh": want,
		},
	}, workspace, true)
	if len(reports) != 1 || !reports[0].OK {
		t.Fatalf("generated file reports = %#v", reports)
	}
	got, err := os.ReadFile(filepath.Join(workspace, "scripts", "run.sh"))
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	if string(got) != want {
		t.Fatalf("generated file content = %q, want %q", got, want)
	}
}
