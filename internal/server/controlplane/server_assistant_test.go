package controlplane_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

func TestAssistantSessionPersistsAndRejectsStaleRevision(t *testing.T) {
	ctx, runtime := openAPICaseBatchSQLiteStore(t)
	graph := store.TestPlanGraph{Map: store.TestPlanMap{ID: "map.assistant", ProfileID: "profile.assistant", Status: "active"}, Nodes: []store.TestPlanNode{{ID: "node.create", CaseID: "case.create", Role: "primary", SummaryJSON: `{"password":"source-secret"}`}}}
	if err := runtime.ReplaceTestPlanGraph(ctx, graph); err != nil {
		t.Fatal(err)
	}
	handler := controlplane.NewWithStore(profile.Bundle{}, runtime)
	_, _, created := doCaseCatalogRequest(t, handler, http.MethodPost, "/api/assistant/sessions", `{"mapId":"map.assistant","ownerId":"owner.test","title":"Map analysis"}`, "", http.StatusOK)
	id := created["session"].(map[string]any)["id"].(string)
	_, _, updated := doCaseCatalogRequest(t, handler, http.MethodPut, "/api/assistant/session?id="+id, `{"revision":1,"status":"running","state":{"messages":[{"role":"user","content":"Analyze dependencies"}],"password":"session-secret"}}`, "", http.StatusOK)
	if updated["session"].(map[string]any)["revision"].(float64) != 2 {
		t.Fatal("session revision did not advance")
	}
	doCaseCatalogRequest(t, handler, http.MethodPut, "/api/assistant/session?id="+id, `{"revision":1,"status":"completed","state":{}}`, "", http.StatusConflict)
	_, raw, loaded := doCaseCatalogRequest(t, handler, http.MethodGet, "/api/assistant/session?id="+id, "", "", http.StatusOK)
	if strings.Contains(string(raw), "session-secret") || loaded["session"].(map[string]any)["status"] != "running" {
		t.Fatal("session state leaked or stale write won")
	}
	_, raw, tool := doCaseCatalogRequest(t, handler, http.MethodPost, "/api/assistant/tool", `{"sessionId":"`+id+`","action":"map.read","args":{}}`, "", http.StatusOK)
	if strings.Contains(string(raw), "source-secret") || tool["data"] == nil {
		t.Fatal("tool context was not safely projected")
	}
	doCaseCatalogRequest(t, handler, http.MethodPost, "/api/assistant/tool", `{"sessionId":"`+id+`","action":"node.explain","args":{"nodeId":"foreign-node"}}`, "", http.StatusBadRequest)
	doCaseCatalogRequest(t, handler, http.MethodPost, "/api/assistant/tool", `{"sessionId":"`+id+`","action":"case.run","args":{}}`, "", http.StatusBadRequest)
	for _, profileID := range []string{graph.Map.ProfileID, "profile.foreign"} {
		run, err := runtime.CreateRun(ctx, store.Run{ID: "run." + profileID, ProfileID: profileID, WorkflowID: "workflow.fixture", Status: store.StatusFailed, CreatedAt: time.Now().UTC()})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.RecordAPICaseRun(ctx, store.APICaseRun{ID: "case-run." + profileID, RunID: run.ID, CaseID: "case.create", Status: store.StatusFailed, AssertionSummaryJSON: `{"status":"failed","message":"expected 200, got 400"}`}); err != nil {
			t.Fatal(err)
		}
	}
	_, _, history := doCaseCatalogRequest(t, handler, http.MethodPost, "/api/assistant/tool", `{"sessionId":"`+id+`","action":"runs.list","args":{}}`, "", http.StatusOK)
	if len(history["data"].(map[string]any)["runs"].([]any)) != 1 {
		t.Fatal("assistant mixed runs from another profile")
	}
	doCaseCatalogRequest(t, handler, http.MethodPost, "/api/assistant/tool", `{"sessionId":"`+id+`","action":"evidence.read","args":{"caseRunId":"case-run.profile.foreign"}}`, "", http.StatusBadRequest)
	graph.Nodes[0].RequiredPropertyJSON = `{"itemId":"required"}`
	if err := runtime.ReplaceTestPlanGraph(ctx, graph); err != nil {
		t.Fatal(err)
	}
	doCaseCatalogRequest(t, handler, http.MethodPost, "/api/assistant/tool", `{"sessionId":"`+id+`","action":"map.read","args":{}}`, "", http.StatusConflict)
}
