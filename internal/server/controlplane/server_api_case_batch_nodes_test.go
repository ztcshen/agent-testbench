package controlplane_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

type apiCaseBatchNodeFixture struct {
	bundle profile.Bundle
	dir    string
}

type apiCaseBatchRunCreatedForTest struct {
	OK                  bool   `json:"ok"`
	BatchRunID          string `json:"batchRunId"`
	RequestID           string `json:"requestId"`
	Status              string `json:"status"`
	WorkflowID          string `json:"workflowId"`
	ReportURL           string `json:"reportUrl"`
	HTMLReportURL       string `json:"htmlReportUrl"`
	JUnitReportURL      string `json:"junitReportUrl"`
	ArtifactManifestURL string `json:"artifactManifestUrl"`
	Total               int    `json:"total"`
}

type apiCaseBatchManifestForTest struct {
	BatchRunID string `json:"batchRunId"`
	Status     string `json:"status"`
	Artifacts  []struct {
		Kind      string `json:"kind"`
		CaseID    string `json:"caseId,omitempty"`
		URL       string `json:"url,omitempty"`
		Path      string `json:"path,omitempty"`
		MediaType string `json:"mediaType,omitempty"`
	} `json:"artifacts"`
}

func TestServerStartsAsyncAPICaseBatchRunForNodes(t *testing.T) {
	ctx, s := openAPICaseBatchSQLiteStore(t)
	fixture := newAPICaseBatchNodeFixture(t)
	server := httptest.NewServer(controlplane.NewWithStore(fixture.bundle, s))
	defer server.Close()

	created := postAPICaseBatchNodeRun(t, server.URL)
	report := waitAPICaseBatchReport(t, server.URL+created.ReportURL)
	requireAPICaseBatchNodeReport(t, fixture.dir, created, report)
	requireAPICaseBatchNodeHTMLReport(t, server.URL, report)
	requireAPICaseBatchNodeJUnitReport(t, server.URL, report)
	requireAPICaseBatchNodeManifest(t, server.URL, created, report)
	requireAPICaseBatchNodeCaseRows(t, report)
	requireAPICaseBatchNodeStoreRecords(t, ctx, s, created, report)
}

func newAPICaseBatchNodeFixture(t *testing.T) apiCaseBatchNodeFixture {
	t.Helper()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/items":
			requireAPICaseBatchCreateItemRequest(t, w, r)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/items/item-override":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"found"}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(target.Close)

	dir := t.TempDir()
	return apiCaseBatchNodeFixture{
		dir: dir,
		bundle: profile.Bundle{
			ID:          "sample",
			DisplayName: "Sample Profile",
			InterfaceNodes: []profile.InterfaceNode{
				{ID: "node.alpha", DisplayName: "Node Alpha"},
				{ID: "node.beta", DisplayName: "Node Beta"},
			},
			APICases: writeAPICaseBatchNodeCases(t, dir, target.URL),
		},
	}
}

func requireAPICaseBatchCreateItemRequest(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	var request map[string]any
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		t.Fatalf("decode target request: %v", err)
	}
	if request["id"] != "item-override" {
		http.Error(w, "missing override", http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"created"}`))
}

func writeAPICaseBatchNodeCases(t *testing.T, dir string, targetURL string) []profile.APICase {
	t.Helper()

	firstCasePath := filepath.Join(dir, "case-alpha.json")
	writeAPICaseBatchNodeCase(t, firstCasePath, `{
  "id": "case.alpha",
  "title": "Create Item",
  "request": {
    "method": "POST",
    "path": "/v1/items",
    "headers": {"Content-Type": "application/json"},
    "body": {"id": "item-001"}
  },
  "assertions": {
    "expectedStatusCodes": [200],
    "responseContains": ["created"]
  }
}`)
	secondCasePath := filepath.Join(dir, "case-beta.json")
	writeAPICaseBatchNodeCase(t, secondCasePath, `{
  "id": "case.beta",
  "title": "Find Item",
  "request": {
    "method": "GET",
    "path": "/v1/items/item-override"
  },
  "assertions": {
    "expectedStatusCodes": [200],
    "responseContains": ["found"]
  }
}`)
	return []profile.APICase{
		{ID: "case.alpha", DisplayName: "Case Alpha", NodeID: "node.alpha", CasePath: firstCasePath, BaseURL: targetURL, EvidenceDir: filepath.Join(dir, "evidence")},
		{ID: "case.beta", DisplayName: "Case Beta", NodeID: "node.beta", CasePath: secondCasePath, BaseURL: targetURL, EvidenceDir: filepath.Join(dir, "evidence")},
	}
}

func writeAPICaseBatchNodeCase(t *testing.T, path string, body string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write api case %s: %v", path, err)
	}
}

func postAPICaseBatchNodeRun(t *testing.T, serverURL string) apiCaseBatchRunCreatedForTest {
	t.Helper()

	body := `{"requestId":"change-001","nodeIds":["node.alpha","node.beta"],"overrides":{"id":"item-override"}}`
	var created apiCaseBatchRunCreatedForTest
	postJSONInto(t, serverURL+"/api/cases/batch-runs", body, http.StatusAccepted, &created)
	if !created.OK || created.BatchRunID == "" || created.RequestID != "change-001" || created.Total != 2 {
		t.Fatalf("api case batch run response = %#v", created)
	}
	if created.ReportURL == "" || created.HTMLReportURL == "" || created.JUnitReportURL == "" || created.ArtifactManifestURL == "" {
		t.Fatalf("api case batch report links = %#v", created)
	}
	return created
}

func requireAPICaseBatchNodeReport(t *testing.T, dir string, created apiCaseBatchRunCreatedForTest, report apiCaseBatchReportForTest) {
	t.Helper()

	if !report.OK || report.Status != store.StatusPassed || report.Completed != 2 || report.Passed != 2 || report.Failed != 0 || len(report.Cases) != 2 {
		t.Fatalf("api case batch report = %#v", report)
	}
	if report.HTMLReportPath == "" || !strings.HasPrefix(report.HTMLReportPath, filepath.Join(dir, "evidence")) || report.HTMLReportURL != created.HTMLReportURL {
		t.Fatalf("api case batch html report fields = %#v", report)
	}
	if report.JUnitReportPath == "" || !strings.HasPrefix(report.JUnitReportPath, filepath.Join(dir, "evidence")) || report.JUnitReportURL != created.JUnitReportURL {
		t.Fatalf("api case batch junit report fields = %#v", report)
	}
	if report.ArtifactManifestPath == "" || !strings.HasPrefix(report.ArtifactManifestPath, filepath.Join(dir, "evidence")) || report.ArtifactManifestURL != created.ArtifactManifestURL {
		t.Fatalf("api case batch artifact manifest fields = %#v", report)
	}
}

func requireAPICaseBatchNodeHTMLReport(t *testing.T, serverURL string, report apiCaseBatchReportForTest) {
	t.Helper()

	htmlRaw := readAPICaseBatchArtifact(t, serverURL+report.HTMLReportURL, "html")
	html := string(htmlRaw)
	if !strings.Contains(html, "API Case Batch Report") || !strings.Contains(html, "change-001") || !strings.Contains(html, "case.alpha") || !strings.Contains(html, "case.beta") {
		t.Fatalf("api case batch html report = %s", html)
	}
	if _, err := os.Stat(report.HTMLReportPath); err != nil {
		t.Fatalf("stat api case batch html report: %v", err)
	}
}

func requireAPICaseBatchNodeJUnitReport(t *testing.T, serverURL string, report apiCaseBatchReportForTest) {
	t.Helper()

	junitRaw := readAPICaseBatchArtifact(t, serverURL+report.JUnitReportURL, "junit")
	for _, want := range []string{`<testsuite name="API Case Batch change-001" tests="2" failures="0"`, `name="case.alpha"`, `classname="node.alpha"`} {
		if !strings.Contains(string(junitRaw), want) {
			t.Fatalf("api case batch junit missing %q: %s", want, junitRaw)
		}
	}
	if _, err := os.Stat(report.JUnitReportPath); err != nil {
		t.Fatalf("stat api case batch junit report: %v", err)
	}
}

func requireAPICaseBatchNodeManifest(t *testing.T, serverURL string, created apiCaseBatchRunCreatedForTest, report apiCaseBatchReportForTest) {
	t.Helper()

	raw := readAPICaseBatchArtifact(t, serverURL+report.ArtifactManifestURL, "artifact manifest")
	var manifest apiCaseBatchManifestForTest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode api case batch artifact manifest: %v", err)
	}
	if manifest.BatchRunID != created.BatchRunID || manifest.Status != store.StatusPassed {
		t.Fatalf("artifact manifest header = %#v", manifest)
	}
	requireAPICaseBatchManifestArtifacts(t, manifest)
	if _, err := os.Stat(report.ArtifactManifestPath); err != nil {
		t.Fatalf("stat api case batch artifact manifest: %v", err)
	}
}

func requireAPICaseBatchManifestArtifacts(t *testing.T, manifest apiCaseBatchManifestForTest) {
	t.Helper()

	artifactKeys := map[string]bool{}
	for _, artifact := range manifest.Artifacts {
		artifactKeys[artifact.Kind+"|"+artifact.CaseID] = true
		if artifact.Kind == "junit" && artifact.MediaType != "application/xml" {
			t.Fatalf("junit artifact = %#v", artifact)
		}
	}
	for _, want := range []string{"json|", "html|", "junit|", "case-detail|case.alpha", "case-evidence|case.alpha", "case-detail|case.beta", "case-evidence|case.beta"} {
		if !artifactKeys[want] {
			t.Fatalf("artifact manifest missing %q: %#v", want, manifest.Artifacts)
		}
	}
}

func requireAPICaseBatchNodeCaseRows(t *testing.T, report apiCaseBatchReportForTest) {
	t.Helper()

	for _, item := range report.Cases {
		if item.RunID == "" || item.CaseRunID != item.RunID+".case" || item.ViewerURL == "" || item.DetailURL == "" || item.Status != store.StatusPassed || item.ElapsedMs < 0 {
			t.Fatalf("api case batch case report = %#v", item)
		}
	}
}

func requireAPICaseBatchNodeStoreRecords(t *testing.T, ctx context.Context, s *sqlite.Store, created apiCaseBatchRunCreatedForTest, report apiCaseBatchReportForTest) {
	t.Helper()

	runs, err := s.ListRuns(ctx)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("stored runs = %#v", runs)
	}
	batchRun, err := s.GetRun(ctx, created.BatchRunID)
	if err != nil {
		t.Fatalf("get stored batch run: %v", err)
	}
	if batchRun.Status != store.StatusPassed || batchRun.ProfileID != "sample" || batchRun.EvidenceRoot != filepath.Dir(report.HTMLReportPath) {
		t.Fatalf("stored batch run = %#v", batchRun)
	}
	requireAPICaseBatchNodeEvidenceRecords(t, ctx, s, created, report)
}

func requireAPICaseBatchNodeEvidenceRecords(t *testing.T, ctx context.Context, s *sqlite.Store, created apiCaseBatchRunCreatedForTest, report apiCaseBatchReportForTest) {
	t.Helper()

	batchEvidence, err := s.ListEvidence(ctx, created.BatchRunID)
	if err != nil {
		t.Fatalf("list batch evidence: %v", err)
	}
	evidenceByKind := map[string]store.EvidenceRecord{}
	for _, row := range batchEvidence {
		evidenceByKind[row.Kind] = row
	}
	for kind, want := range expectedAPICaseBatchNodeEvidence(report) {
		row, ok := evidenceByKind[kind]
		if !ok {
			t.Fatalf("batch evidence missing %s: %#v", kind, batchEvidence)
		}
		if row.URI != want || row.RunID != created.BatchRunID || row.Category != "report" || row.Visibility != "public" {
			t.Fatalf("batch evidence %s = %#v", kind, row)
		}
	}
}

func expectedAPICaseBatchNodeEvidence(report apiCaseBatchReportForTest) map[string]string {
	reportDir := filepath.Dir(report.HTMLReportPath)
	return map[string]string{
		"html":              report.HTMLReportPath,
		"junit":             report.JUnitReportPath,
		"artifact-manifest": report.ArtifactManifestPath,
		"failure-summary":   filepath.Join(reportDir, "failures.json"),
	}
}

func readAPICaseBatchArtifact(t *testing.T, url string, label string) []byte {
	t.Helper()

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("get api case batch %s: %v", label, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("api case batch %s status = %d body=%s", label, resp.StatusCode, raw)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read api case batch %s: %v", label, err)
	}
	return raw
}
