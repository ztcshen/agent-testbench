package controlplane_test

import (
	"fmt"
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
)

func TestServerStartsAsyncAPICaseBatchRunForAllNodeCases(t *testing.T) {
	_, s := openAPICaseBatchSQLiteStore(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/v1/node-cases/") {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer target.Close()

	dir := t.TempDir()
	bundle := profile.Bundle{
		ID: "sample",
		InterfaceNodes: []profile.InterfaceNode{
			{ID: "node.alpha", DisplayName: "Node Alpha", Operation: "Create Item", Method: "GET", Path: "/v1/node-cases"},
		},
	}
	for i := 1; i <= 3; i++ {
		caseID := fmt.Sprintf("case.alpha.%02d", i)
		casePath := filepath.Join(dir, caseID+".json")
		if err := os.WriteFile(casePath, []byte(fmt.Sprintf(`{
  "id": %q,
  "title": "Node Case",
  "request": {"method": "GET", "path": "/v1/node-cases/%02d"},
  "assertions": {"expectedStatusCodes": [200], "responseContains": ["ok"]}
}`, caseID, i)), 0o644); err != nil {
			t.Fatalf("write api case: %v", err)
		}
		bundle.APICases = append(bundle.APICases, profile.APICase{
			ID:          caseID,
			DisplayName: fmt.Sprintf("Node Case %02d", i),
			NodeID:      "node.alpha",
			Scenario:    fmt.Sprintf("scenario-%02d", i),
			CasePath:    casePath,
			BaseURL:     target.URL,
			EvidenceDir: filepath.Join(dir, "evidence"),
		})
	}
	server := httptest.NewServer(controlplane.NewWithStore(bundle, s))
	defer server.Close()

	body := `{"requestId":"node-all-001","nodeIds":["node.alpha"]}`
	var created struct {
		ReportURL      string `json:"reportUrl"`
		HTMLReportURL  string `json:"htmlReportUrl"`
		JUnitReportURL string `json:"junitReportUrl"`
		Total          int    `json:"total"`
	}
	postJSONInto(t, server.URL+"/api/cases/batch-runs", body, http.StatusAccepted, &created)
	if created.Total != 3 || created.JUnitReportURL == "" {
		t.Fatalf("created batch = %#v", created)
	}

	report := waitAPICaseBatchReport(t, server.URL+created.ReportURL)
	if !report.OK || report.Status != store.StatusPassed || report.Completed != 3 || report.Passed != 3 || len(report.Cases) != 3 {
		t.Fatalf("node batch report = %#v", report)
	}
	if len(report.Nodes) != 1 || report.Nodes[0].DisplayName != "Node Alpha" || report.Nodes[0].Operation != "Create Item" || report.Nodes[0].Method != "GET" || report.Nodes[0].Path != "/v1/node-cases" {
		t.Fatalf("node batch report nodes = %#v", report.Nodes)
	}
	if report.Cases[0].DisplayName != "Node Case 01" || report.Cases[0].Scenario != "scenario-01" || report.Cases[0].NodeDisplayName != "Node Alpha" || report.Cases[0].Operation != "Create Item" {
		t.Fatalf("node batch report case metadata = %#v", report.Cases[0])
	}
	htmlResp, err := http.Get(server.URL + created.HTMLReportURL)
	if err != nil {
		t.Fatalf("get node batch html report: %v", err)
	}
	defer htmlResp.Body.Close()
	htmlRaw, err := io.ReadAll(htmlResp.Body)
	if err != nil {
		t.Fatalf("read node batch html report: %v", err)
	}
	html := string(htmlRaw)
	for _, want := range []string{"Node Alpha", "Create Item", "GET", "/v1/node-cases", "Node Case 01", "scenario-01"} {
		if !strings.Contains(html, want) {
			t.Fatalf("node batch html missing %q: %s", want, html)
		}
	}
	junitResp, err := http.Get(server.URL + created.JUnitReportURL)
	if err != nil {
		t.Fatalf("get node batch junit report: %v", err)
	}
	defer junitResp.Body.Close()
	junitRaw, err := io.ReadAll(junitResp.Body)
	if err != nil {
		t.Fatalf("read node batch junit report: %v", err)
	}
	for _, want := range []string{`<testsuite name="API Case Batch node-all-001" tests="3" failures="0"`, `name="case.alpha.01"`, `classname="node.alpha"`} {
		if !strings.Contains(string(junitRaw), want) {
			t.Fatalf("node batch junit missing %q: %s", want, junitRaw)
		}
	}
}

func TestServerStartsAsyncAPICaseBatchRunForExactCaseIDs(t *testing.T) {
	_, s := openAPICaseBatchSQLiteStore(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/exact/first", "/v1/exact/third":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/exact/second":
			t.Fatalf("unselected case should not be run")
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer target.Close()

	dir := t.TempDir()
	bundle := profile.Bundle{
		ID:      "sample",
		BaseDir: dir,
		InterfaceNodes: []profile.InterfaceNode{
			{ID: "node.alpha", DisplayName: "Node Alpha", Operation: "Exact"},
		},
		APICases: []profile.APICase{
			{ID: "case.first", DisplayName: "First Case", NodeID: "node.alpha", CasePath: writeAPICaseBatchGETCase(t, dir, "case.first", "/v1/exact/first"), BaseURL: target.URL, EvidenceDir: filepath.Join(dir, "evidence"), SortOrder: 1},
			{ID: "case.second", DisplayName: "Second Case", NodeID: "node.alpha", CasePath: writeAPICaseBatchGETCase(t, dir, "case.second", "/v1/exact/second"), BaseURL: target.URL, EvidenceDir: filepath.Join(dir, "evidence"), SortOrder: 2},
			{ID: "case.third", DisplayName: "Third Case", NodeID: "node.alpha", CasePath: writeAPICaseBatchGETCase(t, dir, "case.third", "/v1/exact/third"), BaseURL: target.URL, EvidenceDir: filepath.Join(dir, "evidence"), SortOrder: 3},
		},
	}
	server := httptest.NewServer(controlplane.NewWithStore(bundle, s))
	defer server.Close()

	body := `{"requestId":"exact-001","caseIds":["case.third","case.first"]}`
	var created struct {
		ReportURL string   `json:"reportUrl"`
		CaseIDs   []string `json:"caseIds"`
		Total     int      `json:"total"`
	}
	postJSONInto(t, server.URL+"/api/cases/batch-runs", body, http.StatusAccepted, &created)
	if created.Total != 2 || strings.Join(created.CaseIDs, ",") != "case.third,case.first" {
		t.Fatalf("created exact batch = %#v", created)
	}
	report := waitAPICaseBatchReport(t, server.URL+created.ReportURL)
	if !report.OK || report.Status != store.StatusPassed || report.Completed != 2 || len(report.Cases) != 2 {
		t.Fatalf("exact batch report = %#v", report)
	}
	if report.Cases[0].CaseID != "case.third" || report.Cases[1].CaseID != "case.first" {
		t.Fatalf("exact case order = %#v", report.Cases)
	}
}
