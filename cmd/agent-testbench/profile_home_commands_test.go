package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileInstallCommandCopiesBundleIntoProfileHome(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), "source-profile")
	writeWorkflowProfile(t, sourceDir)
	writeGeneratedProfileState(t, sourceDir,
		filepath.Join(".runtime", "store.sqlite"),
		filepath.Join(".runtime", "evidence", "run.json"),
		filepath.Join(".git", "config"),
		"debug.log",
		"local.sqlite",
	)
	profileHome := filepath.Join(t.TempDir(), "profile-home")

	out := runCLI(t, "profile", "install", "--from", sourceDir, "--profile-home", profileHome)
	if !strings.Contains(out, "Installed profile: sample") || !strings.Contains(out, filepath.Join(profileHome, "sample")) {
		t.Fatalf("profile install output = %q", out)
	}
	for _, path := range []string{"profile.json", filepath.Join("workflows", "workflow.json"), filepath.Join("cases", "case.json")} {
		if _, err := os.Stat(filepath.Join(profileHome, "sample", path)); err != nil {
			t.Fatalf("expected installed path %s: %v", path, err)
		}
	}
	for _, path := range []string{
		filepath.Join(".runtime", "store.sqlite"),
		filepath.Join(".runtime", "evidence", "run.json"),
		filepath.Join(".git", "config"),
		"debug.log",
		"local.sqlite",
	} {
		if _, err := os.Stat(filepath.Join(profileHome, "sample", path)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("generated state should not be installed at %s: %v", path, err)
		}
	}

	inspect := runCLI(t, "profile", "inspect", "--profile", "sample", "--profile-home", profileHome)
	if !strings.Contains(inspect, "Profile: sample") || !strings.Contains(inspect, "Workflows: 1") {
		t.Fatalf("inspect installed profile = %q", inspect)
	}

	dbPath := filepath.Join(t.TempDir(), "store.sqlite")
	verify := runCLI(t, "profile", "verify", "--profile", "sample", "--profile-home", profileHome, "--store", "sqlite://"+dbPath)
	if !strings.Contains(verify, "Profile Verification: sample") || !strings.Contains(verify, "OK: true") {
		t.Fatalf("verify installed profile = %q", verify)
	}
}

func TestProfilePackCommandWritesCleanArchive(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), "source-profile")
	writeWorkflowProfile(t, sourceDir)
	writeGeneratedProfileState(t, sourceDir,
		filepath.Join(".runtime", "store.sqlite"),
		"debug.log",
		"local.sqlite",
	)
	outputPath := filepath.Join(t.TempDir(), "sample-profile.tar.gz")

	out := runCLI(t, "profile", "pack", "--profile", sourceDir, "--output", outputPath, "--json")

	var report struct {
		ID           string `json:"id"`
		SourcePath   string `json:"sourcePath"`
		OutputPath   string `json:"outputPath"`
		BundleDigest string `json:"bundleDigest"`
		FileCount    int    `json:"fileCount"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode profile pack report: %v\n%s", err, out)
	}
	if report.ID != "sample" || report.SourcePath != sourceDir || report.OutputPath != outputPath || report.FileCount == 0 || !strings.HasPrefix(report.BundleDigest, "sha256:") {
		t.Fatalf("profile pack report = %#v", report)
	}
	entries := readTarGZEntries(t, outputPath)
	for _, want := range []string{"sample/profile.json", "sample/workflows/workflow.json", "sample/cases/case.json"} {
		if !containsString(entries, want) {
			t.Fatalf("profile archive missing %s: %#v", want, entries)
		}
	}
	for _, unwanted := range []string{"sample/.runtime/store.sqlite", "sample/debug.log", "sample/local.sqlite"} {
		if containsString(entries, unwanted) {
			t.Fatalf("profile archive included generated state %s: %#v", unwanted, entries)
		}
	}
}

func writeGeneratedProfileState(t *testing.T, profileDir string, paths ...string) {
	t.Helper()
	for _, path := range paths {
		fullPath := filepath.Join(profileDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("create generated state parent %s: %v", path, err)
		}
		if err := os.WriteFile(fullPath, []byte("generated"), 0o644); err != nil {
			t.Fatalf("write generated state %s: %v", path, err)
		}
	}
}

func TestProfilePackCommandResolvesInstalledProfileID(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), "source-profile")
	writeWorkflowProfile(t, sourceDir)
	profileHome := filepath.Join(t.TempDir(), "profile-home")
	runCLI(t, "profile", "install", "--from", sourceDir, "--profile-home", profileHome)
	outputPath := filepath.Join(t.TempDir(), "installed-profile.tar.gz")

	out := runCLI(t, "profile", "pack", "--profile", "sample", "--profile-home", profileHome, "--output", outputPath)

	if !strings.Contains(out, "Packed profile: sample") || !strings.Contains(out, outputPath) {
		t.Fatalf("profile pack installed output = %q", out)
	}
	if !containsString(readTarGZEntries(t, outputPath), "sample/profile.json") {
		t.Fatalf("installed profile archive missing manifest")
	}
}

func TestProfileInstallCommandAcceptsPackedArchive(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), "source-profile")
	writeWorkflowProfile(t, sourceDir)
	archivePath := filepath.Join(t.TempDir(), "sample-profile.tar.gz")
	runCLI(t, "profile", "pack", "--profile", sourceDir, "--output", archivePath)
	profileHome := filepath.Join(t.TempDir(), "profile-home")

	out := runCLI(t, "profile", "install", "--from", archivePath, "--profile-home", profileHome, "--json")

	var report struct {
		ID         string `json:"id"`
		SourcePath string `json:"sourcePath"`
		TargetPath string `json:"targetPath"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode archive install report: %v\n%s", err, out)
	}

	if report.ID != "sample" || report.SourcePath != archivePath || report.TargetPath != filepath.Join(profileHome, "sample") {
		t.Fatalf("profile install archive report = %#v", report)
	}
	if _, err := os.Stat(filepath.Join(profileHome, "sample", "profile.json")); err != nil {
		t.Fatalf("installed archive manifest missing: %v", err)
	}
	dbPath := filepath.Join(t.TempDir(), "store.sqlite")
	verify := runCLI(t, "profile", "verify", "--profile", "sample", "--profile-home", profileHome, "--store", "sqlite://"+dbPath)
	if !strings.Contains(verify, "Profile Verification: sample") || !strings.Contains(verify, "OK: true") {
		t.Fatalf("verify installed archive profile = %q", verify)
	}
}

func TestProfileInstallCommandRejectsUnsafeArchivePath(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "unsafe.tar.gz")
	writeTarGZEntries(t, archivePath, map[string]string{
		"sample/profile.json": `{"id":"sample","displayName":"Sample Profile"}`,
		"../escaped.txt":      "nope",
	})
	profileHome := filepath.Join(t.TempDir(), "profile-home")

	out := runCLIFails(t, "profile", "install", "--from", archivePath, "--profile-home", profileHome)

	if !strings.Contains(out, "escapes profile root") {
		t.Fatalf("unsafe archive output = %q", out)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(profileHome), "escaped.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsafe archive wrote escaped path: %v", err)
	}
}

func TestProfileListCommandReportsInstalledBundles(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), "source-profile")
	writeWorkflowProfile(t, sourceDir)
	profileHome := filepath.Join(t.TempDir(), "profile-home")
	runCLI(t, "profile", "install", "--from", sourceDir, "--profile-home", profileHome)

	out := runCLI(t, "profile", "list", "--profile-home", profileHome, "--json")
	var report struct {
		ProfileHome string `json:"profileHome"`
		Profiles    []struct {
			ID           string `json:"id"`
			DisplayName  string `json:"displayName"`
			Path         string `json:"path"`
			BundleDigest string `json:"bundleDigest"`
			Counts       struct {
				Workflows int `json:"workflows"`
				APICases  int `json:"apiCases"`
			} `json:"counts"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode profile list: %v\n%s", err, out)
	}
	if report.ProfileHome != profileHome || len(report.Profiles) != 1 {
		t.Fatalf("profile list identity = %#v", report)
	}
	item := report.Profiles[0]
	if item.ID != "sample" || item.DisplayName != "Sample Profile" || item.Path != filepath.Join(profileHome, "sample") || !strings.HasPrefix(item.BundleDigest, "sha256:") {
		t.Fatalf("profile list item = %#v", item)
	}
	if item.Counts.Workflows != 1 || item.Counts.APICases != 1 {
		t.Fatalf("profile list counts = %#v", item.Counts)
	}
}

func TestProfileListCommandReportsInvalidInstalledBundle(t *testing.T) {
	profileHome := filepath.Join(t.TempDir(), "profile-home")
	brokenDir := filepath.Join(profileHome, "broken")
	if err := os.MkdirAll(brokenDir, 0o755); err != nil {
		t.Fatalf("create broken profile dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brokenDir, "profile.json"), []byte(`{"id":`), 0o644); err != nil {
		t.Fatalf("write broken profile: %v", err)
	}

	out := runCLI(t, "profile", "list", "--profile-home", profileHome, "--json")
	var report struct {
		Profiles []struct {
			ID    string `json:"id"`
			Path  string `json:"path"`
			Valid bool   `json:"valid"`
			Error string `json:"error"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode invalid profile list report: %v\n%s", err, out)
	}
	if len(report.Profiles) != 1 || report.Profiles[0].ID != "broken" || report.Profiles[0].Path != brokenDir || report.Profiles[0].Valid || report.Profiles[0].Error == "" {
		t.Fatalf("invalid profile list report = %#v", report)
	}
}
