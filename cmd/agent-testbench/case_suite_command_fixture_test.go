package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

type caseSuiteCoverageFixture struct {
	profileDir    string
	profileID     string
	nodeID        string
	defaultCaseID string
	variantCaseID string
	unrunCaseID   string
	configID      string
}

type caseSuiteCoverageRun struct {
	runID  string
	caseID string
	status string
	offset time.Duration
}

func publishUniqueCaseSuiteCoverageProfile(t *testing.T) caseSuiteCoverageFixture {
	t.Helper()
	fixture := writeUniqueCaseSuiteCoverageProfile(t)
	runCLI(t, "config", "publish", "--from", fixture.profileDir)
	return fixture
}

func publishCaseSuiteReadinessHistory(t *testing.T, storeRef string, label string) caseSuiteCoverageFixture {
	t.Helper()
	fixture := publishUniqueCaseSuiteCoverageProfile(t)
	recordCaseSuiteCoverageRuns(t, storeRef, label,
		caseSuiteCoverageRun{runID: uniqueTestID(t, "run.default.latest"), caseID: fixture.defaultCaseID, status: "passed", offset: -time.Minute},
		caseSuiteCoverageRun{runID: uniqueTestID(t, "run.variant.latest"), caseID: fixture.variantCaseID, status: "failed", offset: 0},
	)
	return fixture
}

func publishCaseSuitePriorityHistory(t *testing.T, storeRef string, label string) caseSuiteCoverageFixture {
	t.Helper()
	fixture := publishUniqueCaseSuiteCoverageProfile(t)
	recordCaseSuiteCoverageRuns(t, storeRef, label,
		caseSuiteCoverageRun{runID: uniqueTestID(t, "run.default.1"), caseID: fixture.defaultCaseID, status: "passed", offset: -2 * time.Minute},
		caseSuiteCoverageRun{runID: uniqueTestID(t, "run.variant.1"), caseID: fixture.variantCaseID, status: "passed", offset: -time.Minute},
		caseSuiteCoverageRun{runID: uniqueTestID(t, "run.variant.2"), caseID: fixture.variantCaseID, status: "failed", offset: 0},
	)
	return fixture
}

func recordCaseSuiteCoverageRuns(t *testing.T, storeRef string, label string, runs ...caseSuiteCoverageRun) {
	t.Helper()
	ctx := context.Background()
	s, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open %s store: %v", label, err)
	}
	base := time.Now().UTC()
	for _, run := range runs {
		recordCaseRunForCoverage(t, ctx, s, run.runID, run.caseID, run.status, base.Add(run.offset))
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close %s store: %v", label, err)
	}
}

func publishUniqueCaseSuiteQualityProfile(t *testing.T) caseSuiteQualityFixture {
	t.Helper()
	fixture := writeUniqueCaseSuiteQualityProfile(t)
	runCLI(t, "config", "publish", "--from", fixture.profileDir)
	return fixture
}

func newCaseSuiteStatusServer(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("mode") {
		case "bad":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"status":"rejected"}`)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"status":"accepted"}`)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func writeUniqueCaseSuiteCoverageProfile(t *testing.T) caseSuiteCoverageFixture {
	t.Helper()
	fixture := caseSuiteCoverageFixture{
		profileDir:    t.TempDir(),
		profileID:     uniqueTestID(t, "profile.case-suite-coverage"),
		nodeID:        uniqueTestID(t, "node.case-suite-coverage"),
		defaultCaseID: uniqueTestID(t, "case.default"),
		variantCaseID: uniqueTestID(t, "case.variant"),
		unrunCaseID:   uniqueTestID(t, "case.unrun"),
		configID:      uniqueTestID(t, "config.case.variant"),
	}
	writeFile(t, filepath.Join(fixture.profileDir, "profile.json"), fmt.Sprintf(`{
  "id": %q,
  "displayName": "Sample Profile",
  "services": [{"id":"service.alpha","displayName":"Service Alpha","startupCommand":"true"}],
  "workflows": [],
  "interfaceNodes": [{"id":%q,"displayName":"Node Alpha","serviceId":"service.alpha","operation":"Alpha","method":"GET","path":"/alpha"}],
  "apiCases": [
    {"id":%q,"displayName":"Default Case","nodeId":%q,"sortOrder":1,"tags":["regression","smoke"],"priority":"p0","owner":"team-a","description":"Default maintained case.","casePath":"cases/default.json"},
    {"id":%q,"displayName":"Variant Case","nodeId":%q,"sortOrder":2,"tags":["regression"],"priority":"p1","owner":"team-a","description":"Variant maintained case."},
    {"id":%q,"displayName":"Unrun Case","nodeId":%q,"sortOrder":3,"tags":["regression"],"priority":"p2","owner":"team-b","description":"Unrun maintained case."}
  ],
  "requestTemplates": [],
  "templateConfigs": [
    {
      "id": %q,
      "scopeType": "case",
      "scopeId": %q,
      "status": "active",
      "configJson": %q
    }
  ],
  "caseDependencies": [],
  "workflowBindings": [],
  "fixtures": []
}`, fixture.profileID, fixture.nodeID, fixture.defaultCaseID, fixture.nodeID, fixture.variantCaseID, fixture.nodeID, fixture.unrunCaseID, fixture.nodeID, fixture.configID, fixture.variantCaseID, fmt.Sprintf(`{"caseId":%q,"caseExecution":{"method":"GET","nodeId":%q,"path":"/alpha","expectedHttpCodes":[200]}}`, fixture.variantCaseID, fixture.nodeID)))
	return fixture
}
