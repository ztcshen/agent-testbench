package controlplane_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

func TestWorkflowStudioSavesAndExecutesScopedBindings(t *testing.T) {
	ctx, runtime := openAPICaseBatchSQLiteStore(t)
	var omitID atomic.Bool
	var consumerCalls atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/items" && r.URL.Path != "/items/item-42" {
			http.Error(w, "unbound item id", http.StatusBadRequest)
			return
		}
		if r.URL.Path != "/items" {
			consumerCalls.Add(1)
		}
		if omitID.Load() {
			_, _ = w.Write([]byte(`{"state":"no exported id"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"item-42"}`))
	}))
	t.Cleanup(target.Close)
	catalog := store.ProfileCatalog{ProfileID: "studio", IndexedAt: time.Now().UTC()}
	for _, name := range []string{"create", "read"} {
		path := "/items"
		if name == "read" {
			path += "/{{override:itemId}}"
		}
		catalog.InterfaceNodes = append(catalog.InterfaceNodes, store.CatalogInterfaceNode{ID: "node." + name, Method: "GET", Path: path, Status: "active"})
		catalog.APICases = append(catalog.APICases, store.CatalogAPICase{ID: "case." + name, NodeID: "node." + name, DisplayName: name, Status: "active", BaseURL: target.URL, EvidenceDir: t.TempDir()})
		doc, _ := json.Marshal(map[string]any{"caseId": "case." + name, "marker": "preserve", "caseExecution": map[string]any{"method": "GET", "path": path, "headers": map[string]any{"Authorization": "Bearer studio-secret-sentinel"}, "expectedHttpCodes": []int{200}}})
		catalog.TemplateConfigs = append(catalog.TemplateConfigs, store.CatalogTemplateConfig{ID: "config." + name, ScopeType: "api-case", ScopeID: "case." + name, Status: "active", ConfigJSON: string(doc)})
	}
	if _, err := runtime.CompareAndSwapProfileCatalog(ctx, 0, catalog, store.ProfileCatalogMutation{Operation: "seed", SummaryJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	handler := controlplane.NewWithStore(profile.Bundle{ID: "studio"}, runtime)
	response, raw, _ := doCaseCatalogRequest(t, handler, http.MethodGet, "/api/workflow-studio", "", "", http.StatusOK)
	if strings.Contains(string(raw), "studio-secret-sentinel") {
		t.Fatal("studio exposed a request credential")
	}
	etag := response.Header.Get("ETag")
	body := `{"workflow":{"id":"workflow.visual","displayName":"Visual flow"},"steps":[{"id":"create","title":"Create item","caseId":"case.create","required":true,"position":{"x":50,"y":50}},{"id":"read","title":"Read item","caseId":"case.read","required":true,"position":{"x":50,"y":250},"bindings":[{"parameter":"itemId","sourceStepId":"create","from":"responsebody","path":"$.id"}]}]}`
	updated, _, _ := doCaseCatalogRequest(t, handler, http.MethodPut, "/api/workflow-studio", body, etag, http.StatusOK)
	doCaseCatalogRequest(t, handler, http.MethodPut, "/api/workflow-studio", body, etag, http.StatusPreconditionFailed)
	invalid := strings.Replace(body, `"sourceStepId":"create"`, `"sourceStepId":"read"`, 1)
	doCaseCatalogRequest(t, handler, http.MethodPut, "/api/workflow-studio", invalid, updated.Header.Get("ETag"), http.StatusBadRequest)
	_, _, loaded := doCaseCatalogRequest(t, handler, http.MethodGet, "/api/workflow-studio", "", "", http.StatusOK)
	if updated.Header.Get("ETag") == etag || len(loaded["workflows"].([]any)) != 1 {
		t.Fatalf("workflow was not persisted: %#v", loaded)
	}
	// Re-saving the loaded scene must not accumulate private aliases or expose secrets.
	workflows := loaded["workflows"].([]any)
	roundTrip, _ := json.Marshal(workflows[0])
	_, safeRaw, _ := doCaseCatalogRequest(t, handler, http.MethodPut, "/api/workflow-studio", string(roundTrip), updated.Header.Get("ETag"), http.StatusOK)
	if strings.Contains(string(safeRaw), "studio-secret-sentinel") {
		t.Fatal("saved scene exposed credentials")
	}
	api := httptest.NewServer(handler)
	t.Cleanup(api.Close)
	started, _, result := doCaseCatalogRequest(t, handler, http.MethodPost, "/api/cases/batch-runs", `{"workflowId":"workflow.visual","requestId":"studio-run"}`, "", http.StatusAccepted)
	_ = started
	runID, _ := result["batchRunId"].(string)
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		_, _, result = doCaseCatalogRequest(t, handler, http.MethodGet, "/api/cases/batch-runs/"+runID, "", "", http.StatusOK)
		if result["status"] == "running" {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		if result["status"] != "passed" {
			t.Fatalf("scoped binding did not execute: %#v", result)
		}
		snapshot, err := runtime.GetProfileCatalogSnapshot(ctx, "studio")
		if err != nil || snapshot.Catalog.TemplateConfigs[0].ConfigJSON != catalog.TemplateConfigs[0].ConfigJSON {
			t.Fatalf("shared case config changed: %v", err)
		}
		t.Run("missing selected export prevents consumer request", func(t *testing.T) {
			before := consumerCalls.Load()
			omitID.Store(true)
			_, _, next := doCaseCatalogRequest(t, handler, http.MethodPost, "/api/cases/batch-runs", `{"workflowId":"workflow.visual","requestId":"studio-missing-export"}`, "", http.StatusAccepted)
			id := next["batchRunId"].(string)
			until := time.Now().Add(8 * time.Second)
			for time.Now().Before(until) {
				_, _, next = doCaseCatalogRequest(t, handler, http.MethodGet, "/api/cases/batch-runs/"+id, "", "", http.StatusOK)
				if next["status"] == "running" {
					time.Sleep(20 * time.Millisecond)
					continue
				}
				if next["status"] != "failed" || consumerCalls.Load() != before {
					t.Fatalf("consumer ran without its selected input: %#v", next)
				}
				return
			}
			t.Fatal("missing-input run did not finish")
		})
		return
	}
	t.Fatal("studio workflow did not finish")
}
