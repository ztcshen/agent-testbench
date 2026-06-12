package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-testbench/internal/store/sqlite"
)

func TestProfileInitCommandWritesExternalBundle(t *testing.T) {
	profileDir := filepath.Join(t.TempDir(), "external-profile")

	out := runCLI(t, "profile", "init", "--output", profileDir, "--id", "sample", "--display-name", "Sample Profile")
	if !strings.Contains(out, "Initialized external profile bundle: sample") || !strings.Contains(out, profileDir) {
		t.Fatalf("profile init output = %q", out)
	}
	for _, path := range []string{
		"profile.json",
		"README.md",
		".gitignore",
		"services",
		"workflows",
		"interface-nodes",
		"cases",
		"request-templates",
		"case-dependencies",
		"workflow-bindings",
		"fixtures",
	} {
		if _, err := os.Stat(filepath.Join(profileDir, path)); err != nil {
			t.Fatalf("expected generated path %s: %v", path, err)
		}
	}
	ignore, err := os.ReadFile(filepath.Join(profileDir, ".gitignore"))
	if err != nil {
		t.Fatalf("read generated ignore file: %v", err)
	}
	for _, want := range []string{".runtime/", "*.sqlite", "*.log"} {
		if !strings.Contains(string(ignore), want) {
			t.Fatalf("generated ignore file missing %q:\n%s", want, ignore)
		}
	}
	readme, err := os.ReadFile(filepath.Join(profileDir, "README.md"))
	if err != nil {
		t.Fatalf("read generated readme: %v", err)
	}
	if !strings.Contains(string(readme), "--store local-personal") || strings.Contains(string(readme), "--store-url .runtime/store.sqlite") {
		t.Fatalf("generated readme should use Store-first commands:\n%s", readme)
	}

	inspect := runCLI(t, "profile", "inspect", "--profile", profileDir)
	if !strings.Contains(inspect, "Profile: sample") || !strings.Contains(inspect, "Display Name: Sample Profile") {
		t.Fatalf("inspect generated profile = %q", inspect)
	}
}

func TestTemplatePackageCommandAliasesProfileLifecycle(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), "source-template-package")
	writeWorkflowProfile(t, sourceDir)
	profileHome := filepath.Join(t.TempDir(), "template-package-home")

	install := runCLI(t, "template-package", "install", "--from", sourceDir, "--profile-home", profileHome)
	if !strings.Contains(install, "sample") || !strings.Contains(install, filepath.Join(profileHome, "sample")) {
		t.Fatalf("template-package install output = %q", install)
	}

	inspect := runCLI(t, "template-package", "inspect", "--template-package", "sample", "--profile-home", profileHome)
	if !strings.Contains(inspect, "sample") || !strings.Contains(inspect, "Workflows: 1") {
		t.Fatalf("template-package inspect output = %q", inspect)
	}

	dbPath := filepath.Join(t.TempDir(), "store.sqlite")
	verify := runCLI(t, "template-packages", "verify", "--template-package", "sample", "--profile-home", profileHome, "--store", "sqlite://"+dbPath)
	if !strings.Contains(verify, "OK: true") {
		t.Fatalf("template-packages verify output = %q", verify)
	}
}

func TestTemplatePackageCatalogIndexCommandReadsStoreCatalog(t *testing.T) {
	profileDir := filepath.Join(t.TempDir(), "template-package")
	writeWorkflowProfile(t, profileDir)
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	runCLI(t, "template-package", "import", "--from", profileDir, "--store", "sqlite://"+storePath)

	out := runCLI(t, "template-package", "catalog-index", "--store", "sqlite://"+storePath, "--json")

	var report struct {
		ProfileID string `json:"profileId"`
		Counts    struct {
			Services       int `json:"services"`
			Workflows      int `json:"workflows"`
			InterfaceNodes int `json:"interfaceNodes"`
			APICases       int `json:"apiCases"`
		} `json:"counts"`
		ConfigVersion *struct {
			ProfileID string `json:"profileId"`
			Active    bool   `json:"active"`
		} `json:"configVersion"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode catalog-index json: %v\n%s", err, out)
	}
	if report.ProfileID != "sample" || report.Counts.Services != 0 || report.Counts.Workflows != 1 || report.Counts.InterfaceNodes != 1 || report.Counts.APICases != 1 {
		t.Fatalf("catalog-index report = %#v", report)
	}
	if report.ConfigVersion == nil || report.ConfigVersion.ProfileID != "sample" || !report.ConfigVersion.Active {
		t.Fatalf("catalog-index config version = %#v", report.ConfigVersion)
	}
}

func TestProfileInitCommandRejectsCoreProfilesPath(t *testing.T) {
	out := runCLIFails(t, "profile", "init", "--output", "profiles/sample")
	if !strings.Contains(out, "outside this core repository") {
		t.Fatalf("profile init rejection output = %q", out)
	}
}

func TestProfileInspectCommand(t *testing.T) {
	profileDir := writeEmptyProfileBundle(t)
	out := runCLI(t, "profile", "inspect", "--profile", profileDir)
	for _, want := range []string{"Profile: empty", "Display Name: Empty Profile", "Workflows: 0", "API Cases: 0", "Request Templates: 0", "Case Dependencies: 0", "Workflow Bindings: 0"} {
		if !strings.Contains(out, want) {
			t.Fatalf("profile inspect output missing %q: %q", want, out)
		}
	}
}

func TestProfileAuditCommandAcceptsPackedArchive(t *testing.T) {
	profileDir := writeEmptyProfileBundle(t)
	archivePath := filepath.Join(t.TempDir(), "empty-profile.tgz")
	runCLI(t, "profile", "pack", "--profile", profileDir, "--output", archivePath)
	profileHome := filepath.Join(t.TempDir(), "profile-home")

	out := runCLI(t, "profile", "audit", "--profile", archivePath, "--offline-template-package", "--profile-home", profileHome, "--json")

	var report struct {
		ProfileID string `json:"profileId"`
		OK        bool   `json:"ok"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode profile audit archive report: %v\n%s", err, out)
	}
	targetPath := filepath.Join(profileHome, "empty")
	if report.ProfileID != "empty" || !report.OK {
		t.Fatalf("profile audit archive report = %#v", report)
	}
	if _, err := os.Stat(filepath.Join(targetPath, "profile.json")); err != nil {
		t.Fatalf("installed audit archive manifest missing: %v", err)
	}
}

func TestProfileImportCommandIndexesBundleInStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "store.sqlite")
	profileDir := writeEmptyProfileBundle(t)

	out := runCLI(t, "profile", "import", "--from", profileDir, "--store", "sqlite://"+dbPath)
	if !strings.Contains(out, "Imported profile: empty") {
		t.Fatalf("profile import output = %q", out)
	}

	s, err := sqlite.Open(context.Background(), sqlite.Config{Path: dbPath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	index, err := s.GetProfileIndex(context.Background(), "empty")
	if err != nil {
		t.Fatalf("get profile index: %v", err)
	}
	if index.BundlePath == "" || !strings.HasPrefix(index.BundleDigest, "sha256:") {
		t.Fatalf("profile index = %#v", index)
	}
	if got := sqliteScalar(t, dbPath, "select value from kv where key = 'active_profile_id';"); got != "empty" {
		t.Fatalf("active profile catalog index = %q", got)
	}
}

func TestProfileImportReportsNodeCaseDiff(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "store.sqlite")
	profileDir := writeProfileWithCatalogCases(t, []string{"case.alpha"})
	runCLI(t, "profile", "import", "--from", profileDir, "--store", "sqlite://"+dbPath)

	profileDir = writeProfileWithCatalogCases(t, []string{"case.alpha", "case.beta"})
	out := runCLI(t, "profile", "import", "--from", profileDir, "--store", "sqlite://"+dbPath, "--json")
	var report struct {
		Diff struct {
			HasPreviousCatalog bool `json:"hasPreviousCatalog"`
			APICases           struct {
				Before int      `json:"before"`
				After  int      `json:"after"`
				Added  []string `json:"added"`
			} `json:"apiCases"`
			NodeCaseDeltas []struct {
				NodeID string `json:"nodeId"`
				Before int    `json:"before"`
				After  int    `json:"after"`
				Delta  int    `json:"delta"`
			} `json:"nodeCaseDeltas"`
		} `json:"diff"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode import diff: %v\n%s", err, out)
	}
	if !report.Diff.HasPreviousCatalog || report.Diff.APICases.Before != 1 || report.Diff.APICases.After != 2 || strings.Join(report.Diff.APICases.Added, ",") != "case.beta" {
		t.Fatalf("import case diff = %#v", report.Diff.APICases)
	}
	if len(report.Diff.NodeCaseDeltas) != 1 || report.Diff.NodeCaseDeltas[0].NodeID != "node.alpha" || report.Diff.NodeCaseDeltas[0].Before != 1 || report.Diff.NodeCaseDeltas[0].After != 2 || report.Diff.NodeCaseDeltas[0].Delta != 1 {
		t.Fatalf("import node deltas = %#v", report.Diff.NodeCaseDeltas)
	}
}

func TestProfileDoctorAndRepairManifest(t *testing.T) {
	profileDir := writeProfileWithCatalogCases(t, []string{"case.alpha", "case.beta"})
	manifestPath := writeProfileRepairManifest(t, profileDir, []string{"case.beta"})
	removeProfileCatalogCase(t, profileDir, "case.beta")
	if err := os.Remove(filepath.Join(profileDir, "cases", "case.beta.json")); err != nil {
		t.Fatalf("remove case file: %v", err)
	}

	out := runCLIFails(t, "profile", "doctor", "--profile", profileDir, "--case-id", "case.beta", "--json")
	var doctorBefore profileDoctorReport
	if err := json.Unmarshal([]byte(jsonPrefix(out)), &doctorBefore); err != nil {
		t.Fatalf("decode doctor before repair: %v\n%s", err, out)
	}
	if doctorBefore.OK {
		t.Fatalf("doctor should fail before repair: %#v", doctorBefore)
	}

	dryRunOut := runCLI(t, "profile", "repair", "--from-manifest", manifestPath, "--profile", profileDir, "--json")
	var dryRun profileRepairReport
	if err := json.Unmarshal([]byte(dryRunOut), &dryRun); err != nil {
		t.Fatalf("decode dry-run repair: %v\n%s", err, dryRunOut)
	}
	if _, err := os.Stat(filepath.Join(profileDir, "cases", "case.beta.json")); dryRun.Applied || dryRun.Summary.CatalogCasesRestored != 1 || dryRun.Summary.CaseFilesRestored != 1 || err == nil {
		t.Fatalf("dry-run repair = %#v", dryRun)
	}

	applyOut := runCLI(t, "profile", "repair", "--from-manifest", manifestPath, "--profile", profileDir, "--apply", "--json")
	var applied profileRepairReport
	if err := json.Unmarshal([]byte(applyOut), &applied); err != nil {
		t.Fatalf("decode applied repair: %v\n%s", err, applyOut)
	}
	if !applied.Applied || applied.Summary.CatalogCasesRestored != 1 || applied.Summary.CaseFilesRestored != 1 {
		t.Fatalf("applied repair = %#v", applied)
	}

	doctorOut := runCLI(t, "profile", "doctor", "--profile", profileDir, "--case-id", "case.beta", "--json")
	var doctorAfter profileDoctorReport
	if err := json.Unmarshal([]byte(doctorOut), &doctorAfter); err != nil {
		t.Fatalf("decode doctor after repair: %v\n%s", err, doctorOut)
	}
	if !doctorAfter.OK {
		t.Fatalf("doctor should pass after repair: %#v", doctorAfter)
	}
}

func TestProfileImportCommandAcceptsPackedArchive(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "store.sqlite")
	profileDir := writeEmptyProfileBundle(t)
	archivePath := filepath.Join(t.TempDir(), "empty-profile.tar.gz")
	runCLI(t, "profile", "pack", "--profile", profileDir, "--output", archivePath)
	profileHome := filepath.Join(t.TempDir(), "profile-home")

	out := runCLI(t, "profile", "import", "--from", archivePath, "--profile-home", profileHome, "--store", "sqlite://"+dbPath, "--json")

	var report struct {
		ProfileID  string `json:"profileId"`
		BundlePath string `json:"bundlePath"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode profile import archive report: %v\n%s", err, out)
	}
	targetPath := filepath.Join(profileHome, "empty")
	if report.ProfileID != "empty" || report.BundlePath != targetPath {
		t.Fatalf("profile import archive report = %#v", report)
	}
	if _, err := os.Stat(filepath.Join(targetPath, "profile.json")); err != nil {
		t.Fatalf("installed archive manifest missing: %v", err)
	}
	if got := sqliteScalar(t, dbPath, "select source_path from config_versions where active = 1;"); got != targetPath {
		t.Fatalf("archive import config source path = %q, want %q", got, targetPath)
	}
}

func TestProfileImportCommandCanEmitJSONReport(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "store.sqlite")
	profileDir := writeEmptyProfileBundle(t)

	out := runCLI(t, "profile", "import", "--from", profileDir, "--store", "sqlite://"+dbPath, "--json")

	var report struct {
		ProfileID    string `json:"profileId"`
		BundlePath   string `json:"bundlePath"`
		BundleDigest string `json:"bundleDigest"`
		Counts       struct {
			Services         int `json:"services"`
			Workflows        int `json:"workflows"`
			InterfaceNodes   int `json:"interfaceNodes"`
			APICases         int `json:"apiCases"`
			RequestTemplates int `json:"requestTemplates"`
			CaseDependencies int `json:"caseDependencies"`
			WorkflowBindings int `json:"workflowBindings"`
			Fixtures         int `json:"fixtures"`
		} `json:"counts"`
		CatalogIndex struct {
			ProfileID   string `json:"profileId"`
			IndexedAt   string `json:"indexedAt"`
			StoreCounts struct {
				Services        int `json:"services"`
				Workflows       int `json:"workflows"`
				Templates       int `json:"templates"`
				TemplateConfigs int `json:"templateConfigs"`
			} `json:"counts"`
		} `json:"catalogIndex"`
		StorePath  string   `json:"storePath"`
		ImportedAt string   `json:"importedAt"`
		ReadModels []string `json:"readModels"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode profile import json: %v\n%s", err, out)
	}
	if report.ProfileID != "empty" || report.BundlePath != profileDir {
		t.Fatalf("report profile/path = %#v", report)
	}
	if !strings.HasPrefix(report.BundleDigest, "sha256:") || report.StorePath != "sqlite://"+dbPath || report.ImportedAt == "" {
		t.Fatalf("report digest/store/import time = %#v", report)
	}
	if report.Counts.Services != 0 || report.Counts.APICases != 0 || report.Counts.WorkflowBindings != 0 {
		t.Fatalf("report counts = %#v", report.Counts)
	}
	if report.CatalogIndex.ProfileID != "empty" || report.CatalogIndex.IndexedAt == "" {
		t.Fatalf("report catalog index identity = %#v", report.CatalogIndex)
	}
	if report.CatalogIndex.StoreCounts.Services != 0 || report.CatalogIndex.StoreCounts.Templates != 0 || report.CatalogIndex.StoreCounts.TemplateConfigs != 0 {
		t.Fatalf("report catalog index counts = %#v", report.CatalogIndex.StoreCounts)
	}
	if strings.Join(report.ReadModels, ",") != "interface-nodes,catalog,dashboard" {
		t.Fatalf("profile import read models = %#v", report.ReadModels)
	}
}
