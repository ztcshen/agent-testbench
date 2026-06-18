package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
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

func TestProfileCatalogListShowsHistoricalCatalogs(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "store.sqlite")
	storeRef := "sqlite://" + dbPath
	seedProfileCatalogRestoreFixture(t, dbPath)

	out := runCLI(t, "profile", "catalog", "list", "--store", storeRef, "--json")

	var report struct {
		OK    bool `json:"ok"`
		Count int  `json:"count"`
		Items []struct {
			ProfileID string `json:"profileId"`
			Counts    struct {
				Workflows int `json:"workflows"`
				APICases  int `json:"apiCases"`
			} `json:"counts"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode catalog list json: %v\n%s", err, out)
	}
	if !report.OK || report.Count != 2 || len(report.Items) != 2 {
		t.Fatalf("catalog list report = %#v", report)
	}
	if report.Items[0].ProfileID != "profile.workflow-batch-report.testfixture" || report.Items[1].ProfileID != "profile.integration-catalog" {
		t.Fatalf("catalog list order = %#v", report.Items)
	}
	if report.Items[1].Counts.Workflows != 1 || report.Items[1].Counts.APICases != 1 {
		t.Fatalf("catalog list integration catalog counts = %#v", report.Items[1].Counts)
	}
}

func TestProfileCatalogRestorePromotesHistoricalCatalogAndActiveConfig(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "store.sqlite")
	storeRef := "sqlite://" + dbPath
	seedProfileCatalogRestoreFixture(t, dbPath)

	beforeWorkflow := runCLI(t, "workflow", "discover", "--store", storeRef, "--filter", "primary", "--json")
	var beforeWorkflowReport workflowListReport
	if err := json.Unmarshal([]byte(beforeWorkflow), &beforeWorkflowReport); err != nil {
		t.Fatalf("decode workflow discover before restore: %v\n%s", err, beforeWorkflow)
	}
	if beforeWorkflowReport.Count != 0 {
		t.Fatalf("workflow discover should not see primary flow before restore: %#v", beforeWorkflowReport)
	}

	restoreOut := runCLI(t, "profile", "catalog", "restore", "--store", storeRef, "--profile", "profile.integration-catalog", "--json")
	var restoreReport struct {
		OK     bool `json:"ok"`
		Before struct {
			ProfileID string `json:"profileId"`
		} `json:"before"`
		After struct {
			ProfileID string `json:"profileId"`
		} `json:"after"`
		ConfigVersion *struct {
			ProfileID string `json:"profileId"`
			Active    bool   `json:"active"`
		} `json:"configVersion"`
	}
	if err := json.Unmarshal([]byte(restoreOut), &restoreReport); err != nil {
		t.Fatalf("decode catalog restore json: %v\n%s", err, restoreOut)
	}
	if !restoreReport.OK || restoreReport.Before.ProfileID != "profile.workflow-batch-report.testfixture" || restoreReport.After.ProfileID != "profile.integration-catalog" {
		t.Fatalf("catalog restore report = %#v", restoreReport)
	}
	if restoreReport.ConfigVersion == nil || restoreReport.ConfigVersion.ProfileID != "profile.integration-catalog" || !restoreReport.ConfigVersion.Active {
		t.Fatalf("catalog restore config version = %#v", restoreReport.ConfigVersion)
	}

	afterWorkflow := runCLI(t, "workflow", "discover", "--store", storeRef, "--filter", "primary", "--json")
	var afterWorkflowReport workflowListReport
	if err := json.Unmarshal([]byte(afterWorkflow), &afterWorkflowReport); err != nil {
		t.Fatalf("decode workflow discover after restore: %v\n%s", err, afterWorkflow)
	}
	if afterWorkflowReport.ProfileID != "profile.integration-catalog" || afterWorkflowReport.Count != 1 || afterWorkflowReport.Items[0].ID != "workflow.integration_primary_flow" {
		t.Fatalf("workflow discover after restore = %#v", afterWorkflowReport)
	}

	afterCase := runCLI(t, "case", "discover", "--store", storeRef, "--filter", "primary", "--json")
	var afterCaseReport caseListReport
	if err := json.Unmarshal([]byte(afterCase), &afterCaseReport); err != nil {
		t.Fatalf("decode case discover after restore: %v\n%s", err, afterCase)
	}
	if afterCaseReport.ProfileID != "profile.integration-catalog" || afterCaseReport.Count != 1 || afterCaseReport.Items[0].ID != "integration.primary.default" {
		t.Fatalf("case discover after restore = %#v", afterCaseReport)
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
	if got := sqliteScalar(t, dbPath, "select profile_id from config_versions where active = 1;"); got != "empty" {
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

func seedProfileCatalogRestoreFixture(t *testing.T, dbPath string) {
	t.Helper()

	ctx := context.Background()
	s, err := sqlite.Open(ctx, sqlite.Config{Path: dbPath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	integrationIndexedAt := time.Date(2026, 6, 17, 7, 29, 15, 0, time.UTC)
	fixtureIndexedAt := integrationIndexedAt.Add(3 * time.Hour)
	integrationCatalog := store.ProfileCatalog{
		ProfileID: "profile.integration-catalog",
		IndexedAt: integrationIndexedAt,
		Services:  []store.CatalogService{{ID: "service.alpha", DisplayName: "Alpha Service", Kind: "http"}},
		Workflows: []store.CatalogWorkflow{{ID: "workflow.integration_primary_flow", DisplayName: "Integration Primary Flow"}},
		APICases:  []store.CatalogAPICase{{ID: "integration.primary.default", DisplayName: "Primary Default", NodeID: "node.primary_operation", CasePath: "cases/primary.json"}},
		Fixtures:  []store.CatalogFixture{{ID: "fixture.primary", DisplayName: "Primary Fixture", Kind: "store_snapshot", DataJSON: `{}`}},
		InterfaceNodes: []store.CatalogInterfaceNode{
			{ID: "node.primary_operation", DisplayName: "Primary Operation", ServiceID: "service.alpha"},
		},
		WorkflowBindings: []store.CatalogWorkflowBinding{
			{WorkflowID: "workflow.integration_primary_flow", StepID: "primary_operation", NodeID: "node.primary_operation", CaseID: "integration.primary.default", Required: true, SortOrder: 1},
		},
	}
	testCatalog := store.ProfileCatalog{
		ProfileID:      "profile.workflow-batch-report.testfixture",
		IndexedAt:      fixtureIndexedAt,
		Services:       []store.CatalogService{{ID: "service.test", DisplayName: "Test Service", Kind: "http"}},
		Workflows:      []store.CatalogWorkflow{{ID: "workflow.unit.fixture", DisplayName: "Unit Fixture"}},
		InterfaceNodes: []store.CatalogInterfaceNode{{ID: "node.test", DisplayName: "Test API", ServiceID: "service.test"}},
		APICases:       []store.CatalogAPICase{{ID: "case.unit.fixture", DisplayName: "Unit Fixture Case", NodeID: "node.test", CasePath: "cases/unit.json"}},
		WorkflowBindings: []store.CatalogWorkflowBinding{
			{WorkflowID: "workflow.unit.fixture", StepID: "unit", NodeID: "node.test", CaseID: "case.unit.fixture", Required: true, SortOrder: 1},
		},
	}
	if err := s.ReplaceProfileCatalog(ctx, integrationCatalog); err != nil {
		t.Fatalf("seed integration catalog: %v", err)
	}
	if _, err := s.UpsertConfigVersion(ctx, store.ConfigVersion{
		ID:           "config.profile.integration-catalog.20260617T072915Z",
		ProfileID:    "profile.integration-catalog",
		SourcePath:   "/profiles/integration-catalog",
		BundleDigest: "sha256:integration",
		Active:       true,
		PublishedAt:  integrationIndexedAt,
		CreatedAt:    integrationIndexedAt,
	}); err != nil {
		t.Fatalf("seed integration config version: %v", err)
	}
	if err := s.ReplaceProfileCatalog(ctx, testCatalog); err != nil {
		t.Fatalf("seed fixture catalog: %v", err)
	}
	if _, err := s.UpsertConfigVersion(ctx, store.ConfigVersion{
		ID:           "config.profile.workflow-batch-report.testfixture.20260617T105011Z",
		ProfileID:    "profile.workflow-batch-report.testfixture",
		SourcePath:   "/tmp/TestWorkflowReportFailsAdmissionWhenStepInputIsMissing/001",
		BundleDigest: "sha256:test",
		Active:       true,
		PublishedAt:  fixtureIndexedAt,
		CreatedAt:    fixtureIndexedAt,
	}); err != nil {
		t.Fatalf("seed fixture config version: %v", err)
	}
}
