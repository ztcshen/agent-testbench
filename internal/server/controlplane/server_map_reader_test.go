package controlplane_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

func TestMapReaderUsesGraphProfileAndPreservesSource(t *testing.T) {
	ctx, runtime := openAPICaseBatchSQLiteStore(t)
	catalog := store.ProfileCatalog{ProfileID: "graph-profile", IndexedAt: time.Now().UTC(), APICases: []store.CatalogAPICase{{ID: "case.create", DisplayName: "Create item", NodeID: "api.create"}}, InterfaceNodes: []store.CatalogInterfaceNode{{ID: "api.create", Method: "POST", Path: "/items"}}}
	if err := runtime.ReplaceProfileCatalog(ctx, catalog); err != nil {
		t.Fatal(err)
	}
	graph := store.TestPlanGraph{Map: store.TestPlanMap{ID: "map.generic", ProfileID: catalog.ProfileID, DisplayName: "Item lifecycle", Status: "active"}, Nodes: []store.TestPlanNode{{ID: "create", CaseID: "case.create", InterfaceNodeID: "api.create", Role: "primary", SummaryJSON: `{"displayName":"Create","password":"map-secret"}`}}, Paths: []store.TestPlanPath{{ID: "path.create", WorkflowID: "flow.create", DisplayName: "Create item"}}, PathSteps: []store.TestPlanPathStep{{PathID: "path.create", NodeID: "create", CaseID: "case.create", StepID: "create", StepIndex: 1}}}
	if err := runtime.ReplaceTestPlanGraph(ctx, graph); err != nil {
		t.Fatal(err)
	}
	plan := store.TestMapPlanRecord{Instance: store.TestMapPlanInstance{ID: "plan.example", MapID: graph.Map.ID, ProfileID: catalog.ProfileID, Mode: "run", Status: "passed", CreatedAt: time.Now().UTC()}, Tasks: []store.TestMapPlanTask{{ID: "task.create", PlanID: "plan.example", NodeID: "create", CaseID: "case.create", Status: "passed"}}}
	if err := runtime.SaveTestMapPlan(ctx, plan); err != nil {
		t.Fatal(err)
	}
	handler := controlplane.NewWithStore(profile.Bundle{ID: "different-profile"}, runtime)
	_, _, list := doCaseCatalogRequest(t, handler, http.MethodGet, "/api/maps", "", "", http.StatusOK)
	if len(list["maps"].([]any)) != 1 {
		t.Fatal("Store map was not discoverable")
	}
	_, raw, detail := doCaseCatalogRequest(t, handler, http.MethodGet, "/api/map?id=map.generic", "", "", http.StatusOK)
	if strings.Contains(string(raw), "map-secret") {
		t.Fatal("map metadata exposed credentials")
	}
	context := detail["context"].(map[string]any)
	if context["profileId"] != "graph-profile" || context["cases"].([]any)[0].(map[string]any)["displayName"] != "Create item" {
		t.Fatal("map used the wrong profile catalog")
	}
	_, _, loaded := doCaseCatalogRequest(t, handler, http.MethodGet, "/api/map/plan?id=plan.example&mapId=map.generic", "", "", http.StatusOK)
	if loaded["plan"].(map[string]any)["status"] != "passed" {
		t.Fatal("persisted plan status was lost")
	}
	doCaseCatalogRequest(t, handler, http.MethodGet, "/api/map/plan?id=plan.example&mapId=other", "", "", http.StatusNotFound)
	rejected := httptest.NewRecorder()
	handler.ServeHTTP(rejected, httptest.NewRequest(http.MethodPost, "/api/map?id=map.generic", strings.NewReader(`{}`)))
	if rejected.Code != http.StatusMethodNotAllowed {
		t.Fatal("map browsing allowed a mutation")
	}
	stored, err := runtime.GetTestPlanGraph(ctx, graph.Map.ID)
	if err != nil || !strings.Contains(stored.Nodes[0].SummaryJSON, "map-secret") {
		t.Fatal("read surface changed the source graph")
	}
	if _, err := json.Marshal(detail); err != nil {
		t.Fatal(err)
	}
}
