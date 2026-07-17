package main

import (
	"os"
	"path/filepath"
	"testing"

	"agent-testbench/internal/store"
)

func TestEnvironmentRestoreWorkspaceWriterRejectsFileAndParentSymlinks(t *testing.T) {
	for _, target := range []string{"file", "parent"} {
		t.Run(target, func(t *testing.T) {
			workspace := t.TempDir()
			outside := t.TempDir()
			outsideFile := filepath.Join(outside, "outside.txt")
			writeFile(t, outsideFile, "outside-must-survive")
			relative := filepath.Join("config", "projected.txt")
			if target == "file" {
				if err := os.Mkdir(filepath.Join(workspace, "config"), 0o755); err != nil {
					t.Fatalf("create config directory: %v", err)
				}
				if err := os.Symlink(outsideFile, filepath.Join(workspace, relative)); err != nil {
					t.Fatalf("create file symlink: %v", err)
				}
			} else if err := os.Symlink(outside, filepath.Join(workspace, "config")); err != nil {
				t.Fatalf("create parent symlink: %v", err)
			}
			if err := writeEnvironmentRestoreWorkspaceFile(workspace, relative, []byte("projected"), 0o600); err == nil {
				t.Fatalf("workspace writer followed %s symlink", target)
			}
			assertFileContent(t, outsideFile, "outside-must-survive")
		})
	}
}

func TestEnvironmentRestoreWorkspaceWriterReplacesHardlinkWithoutChangingOutsideFile(t *testing.T) {
	workspace := t.TempDir()
	outsideFile := filepath.Join(t.TempDir(), "outside.txt")
	writeFile(t, outsideFile, "outside-must-survive")
	target := filepath.Join(workspace, "projected.txt")
	if err := os.Link(outsideFile, target); err != nil {
		t.Fatalf("create hardlink fixture: %v", err)
	}

	if err := writeEnvironmentRestoreWorkspaceFile(workspace, "projected.txt", []byte("projected"), 0o600); err != nil {
		t.Fatalf("write projected file: %v", err)
	}
	assertFileContent(t, outsideFile, "outside-must-survive")
	assertFileContent(t, target, "projected")
}

func TestPrepareEnvironmentRestoreGeneratedFilesRejectsParentSymlink(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(workspace, "config")); err != nil {
		t.Fatalf("create generated file parent symlink: %v", err)
	}
	reports := prepareEnvironmentRestoreGeneratedFiles(map[string]any{
		"generatedFiles": map[string]any{"config/app.env": "APP_MODE=test\n"},
	}, workspace, true)
	if len(reports) != 1 || reports[0].OK || reports[0].Error == "" {
		t.Fatalf("generated file symlink report = %#v", reports)
	}
	if _, err := os.Lstat(filepath.Join(outside, "app.env")); !os.IsNotExist(err) {
		t.Fatalf("generated file escaped workspace: %v", err)
	}
}

func TestEnvironmentRestoreEdgeAssetReaderRejectsFileAndParentSymlinks(t *testing.T) {
	for _, target := range []string{"file", "parent"} {
		t.Run(target, func(t *testing.T) {
			workspace := t.TempDir()
			outside := t.TempDir()
			outsideFile := filepath.Join(outside, "outside.sql")
			writeFile(t, outsideFile, "select 'outside-secret';")
			relative := filepath.Join("assets", "seed.sql")
			if target == "file" {
				if err := os.Mkdir(filepath.Join(workspace, "assets"), 0o755); err != nil {
					t.Fatalf("create assets directory: %v", err)
				}
				if err := os.Symlink(outsideFile, filepath.Join(workspace, relative)); err != nil {
					t.Fatalf("create asset file symlink: %v", err)
				}
			} else if err := os.Symlink(outside, filepath.Join(workspace, "assets")); err != nil {
				t.Fatalf("create asset parent symlink: %v", err)
			}
			content, err := environmentRestoreEdgeAssetContent(store.ComponentConfigAsset{TargetPath: relative}, workspace)
			if err == nil || content != "" {
				t.Fatalf("edge asset reader followed %s symlink: content=%q err=%v", target, content, err)
			}
		})
	}
}

func TestEnvironmentRestoreMySQLProjectionRejectsParentSymlink(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(workspace, "initdb")); err != nil {
		t.Fatalf("create mysql projection parent symlink: %v", err)
	}
	item := environmentRestoreAppliedAsset{TargetPath: "initdb/001.sql", OK: true}
	written := environmentRestoreProjectMySQLInitDBAsset(store.ComponentConfigAsset{
		TargetPath:    item.TargetPath,
		ContentInline: "create table t(id int);",
	}, workspace, &item)
	if written || item.OK || item.Error == "" {
		t.Fatalf("mysql projection symlink result: written=%t item=%#v", written, item)
	}
	if _, err := os.Lstat(filepath.Join(outside, "001.sql")); !os.IsNotExist(err) {
		t.Fatalf("mysql projection escaped workspace: %v", err)
	}
}
