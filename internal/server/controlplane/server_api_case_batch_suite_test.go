package controlplane_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/domain/profilecatalog"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

func TestServerExposesAPICaseBatchFailureSummary(t *testing.T) {
	ctx, s := openAPICaseBatchSQLiteStore(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/failures/pass":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/failures/fail":
			http.Error(w, "not ok", http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer target.Close()

	dir := t.TempDir()
	bundle := profile.Bundle{
		ID: "sample",
		FailureCategories: []profile.FailureCategoryRule{
			{
				Name: "Product errors",
				Matchers: profile.FailureCategoryMatchers{
					Statuses:          []string{store.StatusFailed},
					FailureCategories: []string{"assertion-mismatch"},
					MessageContains:   []string{"not expected"},
				},
			},
			{
				Name: "Later matching rule",
				Matchers: profile.FailureCategoryMatchers{
					Statuses:          []string{store.StatusFailed},
					FailureCategories: []string{"assertion-mismatch"},
				},
			},
		},
		InterfaceNodes: []profile.InterfaceNode{
			{ID: "node.alpha", DisplayName: "Node Alpha", Operation: "Failure Summary"},
		},
		APICases: []profile.APICase{
			{ID: "case.pass", DisplayName: "Passing Case", NodeID: "node.alpha", CasePath: writeAPICaseBatchGETCase(t, dir, "case.pass", "/v1/failures/pass"), BaseURL: target.URL, EvidenceDir: filepath.Join(dir, "evidence")},
			{ID: "case.fail", DisplayName: "Failing Case", NodeID: "node.alpha", CasePath: writeAPICaseBatchGETCase(t, dir, "case.fail", "/v1/failures/fail"), BaseURL: target.URL, EvidenceDir: filepath.Join(dir, "evidence")},
		},
	}
	if err := s.ReplaceProfileCatalog(ctx, profilecatalog.FromBundle(bundle, time.Now().UTC())); err != nil {
		t.Fatalf("replace profile catalog: %v", err)
	}
	server := httptest.NewServer(controlplane.NewWithStore(bundle, s))
	defer server.Close()

	var created struct {
		ReportURL         string `json:"reportUrl"`
		FailureSummaryURL string `json:"failureSummaryUrl"`
	}
	postJSONInto(t, server.URL+"/api/cases/batch-runs", `{"requestId":"failure-001","caseIds":["case.pass","case.fail"]}`, http.StatusAccepted, &created)
	if created.FailureSummaryURL == "" {
		t.Fatalf("failure summary url missing: %#v", created)
	}
	report := waitAPICaseBatchReport(t, server.URL+created.ReportURL)
	if report.OK || report.Status != store.StatusFailed || report.Failed != 1 {
		t.Fatalf("failed batch report = %#v", report)
	}

	summaryResp, err := http.Get(server.URL + created.FailureSummaryURL)
	if err != nil {
		t.Fatalf("get failure summary: %v", err)
	}
	defer summaryResp.Body.Close()
	var summary struct {
		OK         bool   `json:"ok"`
		BatchRunID string `json:"batchRunId"`
		RequestID  string `json:"requestId"`
		Failed     int    `json:"failed"`
		Failures   []struct {
			CaseID          string `json:"caseId"`
			CaseRunID       string `json:"caseRunId"`
			Status          string `json:"status"`
			FailureCategory string `json:"failureCategory"`
			DetailURL       string `json:"detailUrl"`
			EvidencePath    string `json:"evidencePath"`
			Error           string `json:"error"`
		} `json:"failures"`
	}
	if err := json.NewDecoder(summaryResp.Body).Decode(&summary); err != nil {
		t.Fatalf("decode failure summary: %v", err)
	}
	if summary.OK || summary.RequestID != "failure-001" || summary.Failed != 1 || len(summary.Failures) != 1 {
		t.Fatalf("failure summary = %#v", summary)
	}
	failure := summary.Failures[0]
	if failure.CaseID != "case.fail" || failure.Status != store.StatusFailed || failure.FailureCategory != "Product errors" || failure.CaseRunID == "" || failure.DetailURL == "" || failure.EvidencePath == "" || failure.Error == "" {
		t.Fatalf("failure item = %#v", failure)
	}
}

func TestServerDoesNotReuseBootstrapFailureRulesAcrossProfileCatalogs(t *testing.T) {
	ctx, s := openAPICaseBatchSQLiteStore(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not ok", http.StatusInternalServerError)
	}))
	defer target.Close()

	dir := t.TempDir()
	bootstrap := profile.Bundle{
		ID: "bootstrap-profile",
		FailureCategories: []profile.FailureCategoryRule{{
			Name: "Bootstrap product errors",
			Matchers: profile.FailureCategoryMatchers{
				Statuses:          []string{store.StatusFailed},
				FailureCategories: []string{"assertion-mismatch"},
			},
		}},
	}
	storeBundle := profile.Bundle{
		ID: "store-profile",
		InterfaceNodes: []profile.InterfaceNode{
			{ID: "node.store", DisplayName: "Store Node", Operation: "Store Failure"},
		},
		APICases: []profile.APICase{
			{ID: "case.store.fail", DisplayName: "Store Failing Case", NodeID: "node.store", CasePath: writeAPICaseBatchGETCase(t, dir, "case.store.fail", "/v1/store/fail"), BaseURL: target.URL, EvidenceDir: filepath.Join(dir, "evidence")},
		},
	}
	if err := s.ReplaceProfileCatalog(ctx, profilecatalog.FromBundle(storeBundle, time.Now().UTC())); err != nil {
		t.Fatalf("replace store profile catalog: %v", err)
	}
	server := httptest.NewServer(controlplane.NewWithStore(bootstrap, s))
	defer server.Close()

	var created struct {
		ReportURL         string `json:"reportUrl"`
		FailureSummaryURL string `json:"failureSummaryUrl"`
	}
	postJSONInto(t, server.URL+"/api/cases/batch-runs", `{"requestId":"profile-mismatch-001","caseIds":["case.store.fail"]}`, http.StatusAccepted, &created)
	if created.FailureSummaryURL == "" {
		t.Fatalf("failure summary url missing: %#v", created)
	}
	report := waitAPICaseBatchReport(t, server.URL+created.ReportURL)
	if report.ProfileID != "store-profile" || report.OK || report.Failed != 1 {
		t.Fatalf("store profile batch report = %#v", report)
	}

	summaryResp, err := http.Get(server.URL + created.FailureSummaryURL)
	if err != nil {
		t.Fatalf("get failure summary: %v", err)
	}
	defer summaryResp.Body.Close()
	var summary struct {
		Failures []struct {
			CaseID          string `json:"caseId"`
			FailureCategory string `json:"failureCategory"`
		} `json:"failures"`
	}
	if err := json.NewDecoder(summaryResp.Body).Decode(&summary); err != nil {
		t.Fatalf("decode failure summary: %v", err)
	}
	if len(summary.Failures) != 1 || summary.Failures[0].CaseID != "case.store.fail" {
		t.Fatalf("profile mismatch failure summary = %#v", summary)
	}
	if summary.Failures[0].FailureCategory != "assertion-mismatch" {
		t.Fatalf("store profile must not use bootstrap failure rule, got %#v", summary.Failures[0])
	}
}

func TestServerStartsAsyncAPICaseBatchRunForMaintainedSuiteRunStates(t *testing.T) {
	ctx, s := openAPICaseBatchSQLiteStore(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/suite/variant", "/v1/suite/unrun":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/suite/passed":
			t.Fatalf("already passed case should not be rerun")
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer target.Close()

	dir := t.TempDir()
	bundle := profile.Bundle{
		ID: "sample",
		InterfaceNodes: []profile.InterfaceNode{
			{ID: "node.alpha", DisplayName: "Node Alpha", Operation: "Suite", Method: "GET", Path: "/v1/suite"},
		},
		APICases: []profile.APICase{
			{ID: "case.passed", DisplayName: "Passed Case", NodeID: "node.alpha", Tags: []string{"regression"}, Owner: "team-a", Priority: "p0", CasePath: writeAPICaseBatchGETCase(t, dir, "case.passed", "/v1/suite/passed"), BaseURL: target.URL, EvidenceDir: filepath.Join(dir, "evidence"), SortOrder: 1},
			{ID: "case.variant", DisplayName: "Variant Case", NodeID: "node.alpha", Tags: []string{"regression"}, Owner: "team-a", Priority: "p1", CasePath: writeAPICaseBatchGETCase(t, dir, "case.variant", "/v1/suite/variant"), BaseURL: target.URL, EvidenceDir: filepath.Join(dir, "evidence"), SortOrder: 2},
			{ID: "case.unrun", DisplayName: "Unrun Case", NodeID: "node.alpha", Tags: []string{"regression"}, Owner: "team-b", Priority: "p2", CasePath: writeAPICaseBatchGETCase(t, dir, "case.unrun", "/v1/suite/unrun"), BaseURL: target.URL, EvidenceDir: filepath.Join(dir, "evidence"), SortOrder: 3},
			{ID: "case.other", DisplayName: "Other Case", NodeID: "node.alpha", Tags: []string{"smoke"}, Owner: "team-a", Priority: "p2", CasePath: writeAPICaseBatchGETCase(t, dir, "case.other", "/v1/suite/other"), BaseURL: target.URL, EvidenceDir: filepath.Join(dir, "evidence"), SortOrder: 4},
		},
	}
	base := mustParseTime(t, "2026-05-16T01:00:00Z")
	for _, item := range []struct {
		runID  string
		caseID string
		status string
		at     time.Time
	}{
		{runID: "run.passed.latest", caseID: "case.passed", status: store.StatusPassed, at: base},
		{runID: "run.variant.latest", caseID: "case.variant", status: store.StatusFailed, at: base.Add(time.Minute)},
	} {
		if _, err := s.CreateRun(ctx, store.Run{ID: item.runID, ProfileID: "sample", WorkflowID: item.caseID, Status: item.status, StartedAt: item.at, FinishedAt: item.at.Add(time.Second), CreatedAt: item.at, UpdatedAt: item.at.Add(time.Second)}); err != nil {
			t.Fatalf("create run %s: %v", item.runID, err)
		}
		if _, err := s.RecordAPICaseRun(ctx, store.APICaseRun{ID: item.runID + ".case", RunID: item.runID, CaseID: item.caseID, Status: item.status, StartedAt: item.at, FinishedAt: item.at.Add(time.Second), CreatedAt: item.at}); err != nil {
			t.Fatalf("record case run %s: %v", item.runID, err)
		}
	}
	server := httptest.NewServer(controlplane.NewWithStore(bundle, s))
	defer server.Close()

	body := `{"requestId":"suite-rerun-001","suite":{"tags":["regression"],"status":"active","runStates":["failed","not-run"]}}`
	var created struct {
		ReportURL string `json:"reportUrl"`
		Total     int    `json:"total"`
		Suite     struct {
			Tags      []string `json:"tags"`
			RunStates []string `json:"runStates"`
		} `json:"suite"`
	}
	postJSONInto(t, server.URL+"/api/cases/batch-runs", body, http.StatusAccepted, &created)
	if created.Total != 2 || strings.Join(created.Suite.Tags, ",") != "regression" || strings.Join(created.Suite.RunStates, ",") != "failed,not-run" {
		t.Fatalf("suite batch response = %#v", created)
	}
	report := waitAPICaseBatchReport(t, server.URL+created.ReportURL)
	if !report.OK || report.Completed != 2 || report.Passed != 2 || len(report.Cases) != 2 {
		t.Fatalf("suite batch report = %#v", report)
	}
	gotCases := []string{report.Cases[0].CaseID, report.Cases[1].CaseID}
	if strings.Join(gotCases, ",") != "case.variant,case.unrun" {
		t.Fatalf("suite rerun cases = %#v", gotCases)
	}
}
