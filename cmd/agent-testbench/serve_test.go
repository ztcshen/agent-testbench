package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/domain/profilecatalog"
	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

func TestServeHandlerUsesConfiguredStore(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	_, err = s.CreateRun(ctx, store.Run{
		ID:           "run.alpha",
		ProfileID:    "empty",
		WorkflowID:   "workflow.alpha",
		Status:       store.StatusPassed,
		EvidenceRoot: ".runtime/evidence/run.alpha",
		SummaryJSON:  `{"steps":[{"stepId":"step.alpha","ok":true}]}`,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	handler, cleanup, err := serveHandlerFromArgs([]string{
		"--store", "sqlite://" + storePath,
	})
	if err != nil {
		t.Fatalf("build serve handler: %v", err)
	}
	defer cleanup()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/runs", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("runs status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "run.alpha") {
		t.Fatalf("serve handler did not use configured store: %s", rec.Body.String())
	}
}

func TestServeHandlerRequiresActiveStore(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("AGENT_TESTBENCH_CONFIG_HOME", configHome)
	cwd := t.TempDir()
	originalCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir temp cwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalCwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	_, _, err = serveHandlerFromArgs(nil)
	if err == nil {
		t.Fatal("serve handler should require an active Store")
	}
	if !errors.Is(err, errNoActiveStoreConfigured) {
		t.Fatalf("serve handler error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(cwd, "runtime", "store.sqlite")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("serve should not create an implicit sqlite store, stat err=%v", statErr)
	}
}

func TestServeHandlerAcceptsLocationAgnosticStoreFlag(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	_, err = s.CreateRun(ctx, store.Run{
		ID:           "run.store.flag",
		ProfileID:    "empty",
		WorkflowID:   "workflow.alpha",
		Status:       store.StatusPassed,
		EvidenceRoot: ".runtime/evidence/run.store.flag",
		SummaryJSON:  `{"steps":[{"stepId":"step.alpha","ok":true}]}`,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	handler, cleanup, err := serveHandlerFromArgs([]string{
		"--store", "sqlite://" + storePath,
	})
	if err != nil {
		t.Fatalf("build serve handler: %v", err)
	}
	defer cleanup()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/runs", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("runs status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "run.store.flag") {
		t.Fatalf("serve handler did not use --store: %s", rec.Body.String())
	}

	current := httptest.NewRecorder()
	handler.ServeHTTP(current, httptest.NewRequest(http.MethodGet, "/api/store/current", nil))
	if current.Code != http.StatusOK {
		t.Fatalf("store current status = %d body=%s", current.Code, current.Body.String())
	}
	var payload struct {
		OK         bool   `json:"ok"`
		Configured bool   `json:"configured"`
		Backend    string `json:"backend"`
		URL        string `json:"url"`
		Source     string `json:"source"`
	}
	if err := json.Unmarshal(current.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode store current payload: %v\n%s", err, current.Body.String())
	}
	if !payload.OK || !payload.Configured || payload.Backend != "sqlite" || payload.Source != "store-flag" || payload.URL != "sqlite://"+storePath {
		t.Fatalf("store current payload = %#v", payload)
	}
}

func TestServeHandlerCanBootFromPublishedStoreCatalogWithoutProfilePath(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	sourcePath := filepath.Join(t.TempDir(), "sources", "service-alpha", "main-4e8d26674209")
	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := s.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID: "team-alpha",
		Services: []store.CatalogService{
			{ID: "service.alpha", DisplayName: "Service Alpha", Kind: "http", SourcePath: sourcePath},
		},
		InterfaceNodes: []store.CatalogInterfaceNode{
			{ID: "node.alpha", DisplayName: "Node Alpha", ServiceID: "service.alpha", Operation: "create", Status: "active"},
		},
		APICases: []store.CatalogAPICase{
			{ID: "case.alpha", DisplayName: "Case Alpha", NodeID: "node.alpha", Status: "active"},
		},
	}); err != nil {
		t.Fatalf("replace profile catalog: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	handler, cleanup, err := serveHandlerFromArgs([]string{"--store", "sqlite://" + storePath})
	if err != nil {
		t.Fatalf("build serve handler from store catalog: %v", err)
	}
	defer cleanup()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/interface-nodes", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("interface nodes status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Source struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"source"`
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode interface nodes payload: %v\n%s", err, rec.Body.String())
	}
	if payload.Source.ID != "team-alpha" || payload.Source.Kind != "store" || len(payload.Items) != 1 || payload.Items[0].ID != "node.alpha" {
		t.Fatalf("serve handler did not use published catalog: %#v", payload)
	}

	dashboard := httptest.NewRecorder()
	handler.ServeHTTP(dashboard, httptest.NewRequest(http.MethodGet, "/api/dashboard", nil))
	if dashboard.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d body=%s", dashboard.Code, dashboard.Body.String())
	}
	if !strings.Contains(dashboard.Body.String(), sourcePath) || !strings.Contains(dashboard.Body.String(), "4e8d26674209") {
		t.Fatalf("dashboard did not use published runtime source: %s", dashboard.Body.String())
	}
}

func TestServeBundleUsesPublishedCatalogBeforeProfilePath(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	storePath := filepath.Join(dir, "store.sqlite")
	profileDir := filepath.Join(dir, "external-profile")
	writeFile(t, filepath.Join(profileDir, "profile.json"), `{
  "id": "sample",
  "displayName": "Sample Profile",
  "services": [],
  "workflows": [],
  "interfaceNodes": [{"id":"node.alpha","displayName":"Node Alpha"}],
  "apiCases": [{"id":"case.alpha","displayName":"Case Alpha","nodeId":"node.alpha","casePath":"runnable/case-alpha.json"}],
  "requestTemplates": [],
  "caseDependencies": [],
  "workflowBindings": [],
  "fixtures": []
}`)
	writeFile(t, filepath.Join(profileDir, "runnable", "case-alpha.json"), `{"id":"case.alpha","request":{"method":"GET","path":"/v1/items"},"assertions":{"expectedStatusCodes":[200]}}`)

	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	if _, err := publishProfileBundleToStore(ctx, s, profileDir, storePath, false, false); err != nil {
		t.Fatalf("publish profile: %v", err)
	}
	sourceBundle, err := profile.Load(profileDir)
	if err != nil {
		t.Fatalf("load profile: %v", err)
	}
	catalog := profilecatalog.FromBundle(sourceBundle, time.Now().UTC())
	catalog.APICases[0].CasePath = "store/case-alpha.json"
	if err := s.ReplaceProfileCatalog(ctx, catalog); err != nil {
		t.Fatalf("replace catalog: %v", err)
	}

	bundle, err := serveBundle(ctx, s)
	if err != nil {
		t.Fatalf("serve bundle: %v", err)
	}
	if len(bundle.APICases) != 1 || bundle.APICases[0].CasePath != "store/case-alpha.json" {
		t.Fatalf("serve bundle api cases = %#v", bundle.APICases)
	}
}

func TestServeHandlerPublishesProfilePathIntoStoreBeforeServing(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	profileDir := filepath.Join(t.TempDir(), "external-profile")
	writeWorkflowProfile(t, profileDir)

	handler, cleanup, err := serveHandlerFromArgs([]string{
		"--profile", profileDir,
		"--store", "sqlite://" + storePath,
	})
	if err != nil {
		t.Fatalf("build serve handler with profile path: %v", err)
	}
	defer cleanup()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/interface-nodes", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("interface nodes status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Source struct {
			ID string `json:"id"`
		} `json:"source"`
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode interface nodes payload: %v\n%s", err, rec.Body.String())
	}
	if payload.Source.ID != "sample" || len(payload.Items) != 1 || payload.Items[0].ID != "node.alpha" {
		t.Fatalf("interface nodes payload = %#v", payload)
	}
	if got := sqliteScalar(t, storePath, "select value from kv where key = 'active_profile_id';"); got != "sample" {
		t.Fatalf("active profile id = %q", got)
	}
	if got := sqliteScalar(t, storePath, "select count(*) from config_read_model where profile_id = 'sample';"); got == "0" {
		t.Fatalf("expected serve --profile to publish read models")
	}
}

func TestServeHandlerPublishesInstalledProfileIDBeforeServing(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	profileHome := filepath.Join(t.TempDir(), "profile-home")
	sourceDir := filepath.Join(t.TempDir(), "external-profile")
	writeWorkflowProfile(t, sourceDir)
	runCLI(t, "profile", "install", "--from", sourceDir, "--profile-home", profileHome)

	handler, cleanup, err := serveHandlerFromArgs([]string{
		"--profile", "sample",
		"--profile-home", profileHome,
		"--store", "sqlite://" + storePath,
	})
	if err != nil {
		t.Fatalf("build serve handler with installed profile id: %v", err)
	}
	defer cleanup()

	profiles := httptest.NewRecorder()
	handler.ServeHTTP(profiles, httptest.NewRequest(http.MethodGet, "/api/profile/installed", nil))
	if profiles.Code != http.StatusOK || !strings.Contains(profiles.Body.String(), profileHome) {
		t.Fatalf("installed profiles response = %d %s", profiles.Code, profiles.Body.String())
	}
	if got := sqliteScalar(t, storePath, "select value from kv where key = 'active_profile_id';"); got != "sample" {
		t.Fatalf("active profile id = %q", got)
	}
}

func TestServeAndEvidenceTasksUseNamedPostgreSQLActiveStore(t *testing.T) {
	storeName := "daily-serve-pg"
	storeRef := configureNamedPostgreSQLActiveStore(t, storeName)
	runServeAndEvidenceTasksUseNamedActiveStore(t, storeRef, storeName, "postgres", "pg", "PostgreSQL")
}

func TestServeAndEvidenceTasksUseNamedMySQLActiveStore(t *testing.T) {
	storeName := "daily-serve-mysql"
	storeRef := configureNamedMySQLActiveStore(t, storeName)
	runServeAndEvidenceTasksUseNamedActiveStore(t, storeRef, storeName, "mysql", "mysql", "MySQL")
}

func runServeAndEvidenceTasksUseNamedActiveStore(t *testing.T, storeRef string, storeName string, backend string, runLabel string, label string) {
	t.Helper()
	runID := "run.tasks." + runLabel + "." + time.Now().UTC().Format("20060102150405.000000000")
	ctx := context.Background()
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open %s task store: %v", label, err)
	}
	seedPostProcessTaskFixture(t, ctx, runtime, runID, runID+".")
	if err := runtime.Close(); err != nil {
		t.Fatalf("close %s task store: %v", label, err)
	}

	profileDir := writeInterfaceNodeBatchReportProfile(t)
	runCLI(t, "config", "publish", "--from", profileDir)

	listOut := runCLI(t, "evidence", "list", "--run", runID, "--json")
	var evidenceReport struct {
		Runs []struct {
			ID            string `json:"id"`
			EvidenceCount int    `json:"evidenceCount"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(listOut), &evidenceReport); err != nil {
		t.Fatalf("decode %s evidence list json: %v\n%s", label, err, listOut)
	}
	if len(evidenceReport.Runs) != 1 || evidenceReport.Runs[0].ID != runID || evidenceReport.Runs[0].EvidenceCount != 1 {
		t.Fatalf("%s evidence list report = %#v", label, evidenceReport.Runs)
	}

	tasksOut := runCLI(t,
		"evidence", "tasks",
		"--run", runID,
		"--step", "step-a",
		"--kind", "trace_topology_collect",
		"--json",
	)
	var tasksReport struct {
		RunID  string `json:"runId"`
		Counts struct {
			Total  int `json:"total"`
			Passed int `json:"passed"`
		} `json:"counts"`
		Tasks []struct {
			ID            string `json:"id"`
			StepID        string `json:"stepId"`
			DisplayStatus string `json:"displayStatus"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(tasksOut), &tasksReport); err != nil {
		t.Fatalf("decode %s evidence tasks json: %v\n%s", label, err, tasksOut)
	}
	if tasksReport.RunID != runID || tasksReport.Counts.Total != 1 || tasksReport.Counts.Passed != 1 || len(tasksReport.Tasks) != 1 {
		t.Fatalf("%s evidence tasks report = %#v", label, tasksReport)
	}
	if !strings.Contains(tasksReport.Tasks[0].ID, "task.trace") || tasksReport.Tasks[0].StepID != "step-a" || tasksReport.Tasks[0].DisplayStatus != "passed: completed" {
		t.Fatalf("%s evidence task = %#v", label, tasksReport.Tasks[0])
	}

	handler, cleanup, err := serveHandlerFromArgs(nil)
	if err != nil {
		t.Fatalf("build serve handler from active SQL Store: %v", err)
	}
	defer cleanup()

	current := httptest.NewRecorder()
	handler.ServeHTTP(current, httptest.NewRequest(http.MethodGet, "/api/store/current", nil))
	if current.Code != http.StatusOK {
		t.Fatalf("store current status = %d body=%s", current.Code, current.Body.String())
	}
	var storeInfo struct {
		OK         bool   `json:"ok"`
		Configured bool   `json:"configured"`
		Name       string `json:"name"`
		Backend    string `json:"backend"`
		Source     string `json:"source"`
	}
	if err := json.Unmarshal(current.Body.Bytes(), &storeInfo); err != nil {
		t.Fatalf("decode %s store current payload: %v\n%s", label, err, current.Body.String())
	}
	if !storeInfo.OK || !storeInfo.Configured || storeInfo.Name != storeName || storeInfo.Backend != backend || storeInfo.Source != "active-config" {
		t.Fatalf("%s store current payload = %#v", label, storeInfo)
	}

	runs := httptest.NewRecorder()
	handler.ServeHTTP(runs, httptest.NewRequest(http.MethodGet, "/api/runs", nil))
	if runs.Code != http.StatusOK || !strings.Contains(runs.Body.String(), runID) {
		t.Fatalf("%s serve runs via active SQL Store = %d %s", label, runs.Code, runs.Body.String())
	}

	nodes := httptest.NewRecorder()
	handler.ServeHTTP(nodes, httptest.NewRequest(http.MethodGet, "/api/interface-nodes", nil))
	if nodes.Code != http.StatusOK {
		t.Fatalf("interface nodes status = %d body=%s", nodes.Code, nodes.Body.String())
	}
	var nodesPayload struct {
		Source struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"source"`
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(nodes.Body.Bytes(), &nodesPayload); err != nil {
		t.Fatalf("decode %s interface nodes payload: %v\n%s", label, err, nodes.Body.String())
	}
	if nodesPayload.Source.ID != "sample" || nodesPayload.Source.Kind != "read-model" || len(nodesPayload.Items) != 1 || nodesPayload.Items[0].ID != "node.alpha" {
		t.Fatalf("%s serve catalog payload = %#v", label, nodesPayload)
	}

	apiImportDir := writeEmptyProfileBundle(t)
	importRec := httptest.NewRecorder()
	handler.ServeHTTP(importRec, httptest.NewRequest(http.MethodPost, "/api/profile/import", strings.NewReader(`{"path":`+mustJSON(t, apiImportDir)+`}`)))
	if importRec.Code != http.StatusOK {
		t.Fatalf("profile import status = %d body=%s", importRec.Code, importRec.Body.String())
	}
	var importPayload struct {
		ProfileID  string   `json:"profileId"`
		BundlePath string   `json:"bundlePath"`
		ReadModels []string `json:"readModels"`
	}
	if err := json.Unmarshal(importRec.Body.Bytes(), &importPayload); err != nil {
		t.Fatalf("decode %s serve profile import payload: %v\n%s", label, err, importRec.Body.String())
	}
	if importPayload.ProfileID != "empty" || importPayload.BundlePath != apiImportDir || strings.Join(importPayload.ReadModels, ",") != "interface-nodes,catalog,dashboard" {
		t.Fatalf("%s serve profile import payload = %#v", label, importPayload)
	}

	apiVerifyDir := writeInterfaceNodeCaseProfile(t)
	verifyRec := httptest.NewRecorder()
	handler.ServeHTTP(verifyRec, httptest.NewRequest(http.MethodPost, "/api/profile/verify", strings.NewReader(`{"path":`+mustJSON(t, apiVerifyDir)+`}`)))
	if verifyRec.Code != http.StatusOK {
		t.Fatalf("profile verify status = %d body=%s", verifyRec.Code, verifyRec.Body.String())
	}
	var verifyPayload struct {
		OK        bool   `json:"ok"`
		ProfileID string `json:"profileId"`
		Publish   struct {
			ProfileID  string   `json:"profileId"`
			BundlePath string   `json:"bundlePath"`
			ReadModels []string `json:"readModels"`
		} `json:"publish"`
		Summary struct {
			FailedChecks int `json:"failedChecks"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(verifyRec.Body.Bytes(), &verifyPayload); err != nil {
		t.Fatalf("decode %s serve profile verify payload: %v\n%s", label, err, verifyRec.Body.String())
	}
	if !verifyPayload.OK || verifyPayload.ProfileID != "sample" || verifyPayload.Publish.ProfileID != "sample" || verifyPayload.Publish.BundlePath != apiVerifyDir || !hasReadModels(verifyPayload.Publish.ReadModels, "interface-nodes", "catalog", "dashboard") || verifyPayload.Summary.FailedChecks != 0 {
		t.Fatalf("%s serve profile verify payload = %#v", label, verifyPayload)
	}

	runtime, err = openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("reopen %s serve profile Store: %v", label, err)
	}
	defer runtime.Close()
	verifiedIndex, err := runtime.GetProfileIndex(ctx, "sample")
	if err != nil {
		t.Fatalf("get %s serve profile index: %v", label, err)
	}
	if verifiedIndex.BundlePath != apiVerifyDir || !strings.HasPrefix(verifiedIndex.BundleDigest, "sha256:") {
		t.Fatalf("%s serve profile index = %#v", label, verifiedIndex)
	}
	verifiedCatalog, err := runtime.GetProfileCatalog(ctx)
	if err != nil {
		t.Fatalf("get %s serve profile catalog: %v", label, err)
	}
	if verifiedCatalog.ProfileID != "sample" || len(verifiedCatalog.APICases) != 2 {
		t.Fatalf("%s serve profile catalog = %#v", label, verifiedCatalog)
	}

	apiLegacyPath := filepath.Join(t.TempDir(), "legacy-api.sqlite")
	apiLegacySuffix := time.Now().UTC().UnixNano()
	apiLegacyWorkflowID := apiLegacySuffix
	apiLegacyCaseID := apiLegacySuffix + 1
	apiLegacyParentRunID := fmt.Sprintf("case-run-parent-api-%s-%d", runLabel, apiLegacySuffix)
	createLegacyRuntimeDBWithIDs(t, apiLegacyPath, apiLegacyWorkflowID, apiLegacyCaseID, apiLegacyParentRunID)
	importEvidenceRec := httptest.NewRecorder()
	handler.ServeHTTP(importEvidenceRec, httptest.NewRequest(http.MethodPost, "/api/evidence/import", strings.NewReader(`{"sourcePath":`+mustJSON(t, apiLegacyPath)+`,"profileId":"sample"}`)))
	if importEvidenceRec.Code != http.StatusOK {
		t.Fatalf("evidence import status = %d body=%s", importEvidenceRec.Code, importEvidenceRec.Body.String())
	}
	var importEvidencePayload struct {
		OK              bool   `json:"ok"`
		SourcePath      string `json:"sourcePath"`
		ProfileID       string `json:"profileId"`
		RunCount        int    `json:"runCount"`
		APICaseRunCount int    `json:"apiCaseRunCount"`
		EvidenceCount   int    `json:"evidenceCount"`
	}
	if err := json.Unmarshal(importEvidenceRec.Body.Bytes(), &importEvidencePayload); err != nil {
		t.Fatalf("decode %s serve evidence import payload: %v\n%s", label, err, importEvidenceRec.Body.String())
	}
	if !importEvidencePayload.OK || importEvidencePayload.SourcePath != apiLegacyPath || importEvidencePayload.ProfileID != "sample" || importEvidencePayload.RunCount != 2 || importEvidencePayload.APICaseRunCount != 1 || importEvidencePayload.EvidenceCount != 1 {
		t.Fatalf("%s serve evidence import payload = %#v", label, importEvidencePayload)
	}
	evidenceListRec := httptest.NewRecorder()
	handler.ServeHTTP(evidenceListRec, httptest.NewRequest(http.MethodGet, "/api/evidence/list?run="+apiLegacyParentRunID, nil))
	if evidenceListRec.Code != http.StatusOK {
		t.Fatalf("evidence list status = %d body=%s", evidenceListRec.Code, evidenceListRec.Body.String())
	}
	var importedEvidencePayload struct {
		Runs []struct {
			ID              string `json:"id"`
			APICaseRunCount int    `json:"apiCaseRunCount"`
			EvidenceCount   int    `json:"evidenceCount"`
			EvidenceRecords []struct {
				ID        string `json:"id"`
				CaseRunID string `json:"caseRunId"`
				Kind      string `json:"kind"`
				URI       string `json:"uri"`
			} `json:"evidenceRecords"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(evidenceListRec.Body.Bytes(), &importedEvidencePayload); err != nil {
		t.Fatalf("decode %s serve evidence list payload: %v\n%s", label, err, evidenceListRec.Body.String())
	}
	if len(importedEvidencePayload.Runs) != 1 || importedEvidencePayload.Runs[0].ID != apiLegacyParentRunID || importedEvidencePayload.Runs[0].APICaseRunCount != 1 || importedEvidencePayload.Runs[0].EvidenceCount != 1 || len(importedEvidencePayload.Runs[0].EvidenceRecords) != 1 {
		t.Fatalf("%s serve evidence list payload = %#v", label, importedEvidencePayload.Runs)
	}
	importedRecord := importedEvidencePayload.Runs[0].EvidenceRecords[0]
	if importedRecord.ID != fmt.Sprintf("legacy-evidence-%d", apiLegacyCaseID) || importedRecord.CaseRunID != fmt.Sprintf("legacy-case-run-%d", apiLegacyCaseID) || importedRecord.Kind != "case-run" || importedRecord.URI != ".runtime/cases/"+apiLegacyParentRunID {
		t.Fatalf("%s serve evidence list record = %#v", label, importedRecord)
	}
}
