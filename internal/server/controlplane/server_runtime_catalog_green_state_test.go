package controlplane_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

type interfaceGreenStateFixture struct {
	Server *httptest.Server
	Target *httptest.Server
}

type interfaceGreenStateBatchPayload struct {
	Results []struct {
		RunID     string `json:"runId"`
		CaseRunID string `json:"caseRunId"`
		DetailURL string `json:"detailUrl"`
	} `json:"results"`
}

func newInterfaceGreenStateFixture(t *testing.T) interfaceGreenStateFixture {
	t.Helper()
	ctx := context.Background()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(target.Close)
	s, err := sqlite.Open(ctx, sqlite.Config{Path: filepath.Join(t.TempDir(), "sandbox.sqlite")})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	if err := s.ReplaceProfileCatalog(ctx, interfaceGreenStateCatalog()); err != nil {
		t.Fatalf("replace profile catalog: %v", err)
	}
	server := httptest.NewServer(controlplane.NewWithStore(profile.Bundle{ID: "sample", DisplayName: "Sample Profile"}, s))
	t.Cleanup(server.Close)
	return interfaceGreenStateFixture{Server: server, Target: target}
}

func interfaceGreenStateCatalog() store.ProfileCatalog {
	return store.ProfileCatalog{
		ProfileID: "sample",
		IndexedAt: time.Now().UTC(),
		InterfaceNodes: []store.CatalogInterfaceNode{
			{ID: "interface.alpha", DisplayName: "Alpha", Status: "active"},
		},
		APICases: []store.CatalogAPICase{
			{ID: "case.alpha.one", DisplayName: "Alpha one", NodeID: "interface.alpha", CaseType: "success", RequiredForAdmission: true, Status: "active"},
			{ID: "case.alpha.two", DisplayName: "Alpha two", NodeID: "interface.alpha", CaseType: "success", RequiredForAdmission: true, Status: "active"},
			{ID: "case.alpha.optional", DisplayName: "Alpha optional", NodeID: "interface.alpha", CaseType: "success", RequiredForAdmission: false, Status: "active"},
		},
		TemplateConfigs: []store.CatalogTemplateConfig{
			{ID: "cfg.one", ScopeType: "step", ScopeID: "one", Status: "active", ConfigJSON: `{"caseId":"case.alpha.one","caseExecution":{"method":"GET","nodeId":"service.alpha","path":"/ok","expectedHttpCodes":[200]}}`},
			{ID: "cfg.two", ScopeType: "step", ScopeID: "two", Status: "active", ConfigJSON: `{"caseId":"case.alpha.two","caseExecution":{"method":"GET","nodeId":"service.alpha","path":"/ok","expectedHttpCodes":[200]}}`},
			{ID: "cfg.optional", ScopeType: "step", ScopeID: "optional", Status: "active", ConfigJSON: `{"caseId":"case.alpha.optional","caseExecution":{"method":"GET","nodeId":"service.alpha","path":"/ok","expectedHttpCodes":[200]}}`},
		},
	}
}

func postInterfaceGreenStateBatch(t *testing.T, fixture interfaceGreenStateFixture) interfaceGreenStateBatchPayload {
	t.Helper()
	resp, err := http.Post(
		fixture.Server.URL+"/api/test-kit/run-batch",
		"application/json",
		strings.NewReader(fmt.Sprintf(`{"caseIds":["case.alpha.one","case.alpha.two","case.alpha.optional"],"baseUrl":%q}`, fixture.Target.URL)),
	)
	if err != nil {
		t.Fatalf("post batch: %v", err)
	}
	defer resp.Body.Close()
	var payload interfaceGreenStateBatchPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode batch response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("batch status = %d", resp.StatusCode)
	}
	return payload
}

func assertInterfaceGreenStateBatchEvidence(t *testing.T, serverURL string, payload interfaceGreenStateBatchPayload) {
	t.Helper()
	if len(payload.Results) != 3 {
		t.Fatalf("batch results = %#v", payload.Results)
	}
	for _, item := range payload.Results {
		wantDetailURL := "/api/case-run/evidence?caseRunId=" + url.QueryEscape(item.CaseRunID)
		if item.RunID == "" || item.CaseRunID != item.RunID+".case" || item.DetailURL != wantDetailURL {
			t.Fatalf("batch case evidence handles = %#v", item)
		}
	}
	detail := decodeJSONResponse(t, serverURL+payload.Results[0].DetailURL, http.StatusOK)
	if detail["ok"] != true {
		t.Fatalf("batch case detail lookup = %#v", detail)
	}
}

func assertInterfaceNodeGreenState(t *testing.T, serverURL string) {
	t.Helper()
	resp, err := http.Get(serverURL + "/api/interface-node?id=interface.alpha")
	if err != nil {
		t.Fatalf("get interface node: %v", err)
	}
	defer resp.Body.Close()
	var payload struct {
		Admission struct {
			Status          string `json:"status"`
			PassedCaseCount int    `json:"passedCaseCount"`
		} `json:"admission"`
		Cases []struct {
			ID        string         `json:"id"`
			LatestRun map[string]any `json:"latestRun"`
		} `json:"cases"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode interface node: %v", err)
	}
	if payload.Admission.Status != store.StatusPassed || payload.Admission.PassedCaseCount != 2 {
		t.Fatalf("admission = %#v", payload.Admission)
	}
	for _, item := range payload.Cases {
		if item.LatestRun["status"] != store.StatusPassed {
			t.Fatalf("case %s latest run = %#v", item.ID, item.LatestRun)
		}
	}
}

func assertInterfaceNodeListGreenState(t *testing.T, serverURL string) {
	t.Helper()
	list := decodeJSONResponse(t, serverURL+"/api/interface-nodes", http.StatusOK)
	if list["ok"] != true || list["templateId"] != "TPL-INTERFACE-NODE-CASE-LIST-V1" {
		t.Fatalf("interface node list envelope = %#v", list)
	}
	filters := list["filters"].(map[string]any)
	if filters["serviceId"] != "" || filters["operation"] != "" {
		t.Fatalf("interface node list filters = %#v", filters)
	}
	items := list["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("interface node list = %#v", list)
	}
	row := items[0].(map[string]any)
	if row["status"] != "active" || row["admissionStatus"] != store.StatusPassed || row["passedCaseCount"] != float64(2) {
		t.Fatalf("interface node list row = %#v", row)
	}
	if row["operation"] != "Alpha" || row["latestRunId"] == "" {
		t.Fatalf("interface node list row = %#v", row)
	}
}
