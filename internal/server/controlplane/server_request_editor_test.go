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

func TestRequestEditorPersistsNativeRequestAndKeepsSecrets(t *testing.T) {
	ctx, runtime := openAPICaseBatchSQLiteStore(t)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer editor-secret" || r.URL.Query().Get("limit") != "3" {
			http.Error(w, "request configuration was lost", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"item-42"}`))
	}))
	t.Cleanup(target.Close)
	catalog := store.ProfileCatalog{
		ProfileID: "editor", IndexedAt: time.Now().UTC(),
		Services:        []store.CatalogService{{ID: "service.http", DisplayName: "HTTP API", Kind: "http"}, {ID: "service.secondary", DisplayName: "Secondary HTTP API", Kind: "http"}},
		InterfaceNodes:  []store.CatalogInterfaceNode{{ID: "node.items", ServiceID: "service.http", Method: "GET", Path: "/items", Status: "active"}},
		APICases:        []store.CatalogAPICase{{ID: "case.items", NodeID: "node.items", Status: "active", BaseURL: target.URL, EvidenceDir: t.TempDir()}},
		TemplateConfigs: []store.CatalogTemplateConfig{{ID: "config.items", ScopeType: "case", ScopeID: "case.items", Status: "active", ConfigJSON: `{"caseId":"case.items","marker":"keep","caseExecution":{"method":"GET","path":"/items","headers":{"Authorization":"Bearer editor-secret"},"expectedHttpCodes":[200]}}`}},
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(catalog.TemplateConfigs[0].ConfigJSON), &config); err != nil {
		t.Fatal(err)
	}
	config["caseExecution"].(map[string]any)["body"] = `{"password":"editor-body-secret"}`
	encoded, _ := json.Marshal(config)
	catalog.TemplateConfigs[0].ConfigJSON = string(encoded)
	if _, err := runtime.CompareAndSwapProfileCatalog(ctx, 0, catalog, store.ProfileCatalogMutation{Operation: "seed", SummaryJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	handler := controlplane.NewWithStore(profile.Bundle{ID: "editor"}, runtime)
	get, raw, payload := doCaseCatalogRequest(t, handler, http.MethodGet, "/api/request-editor", "", "", http.StatusOK)
	if strings.Contains(string(raw), "editor-secret") || strings.Contains(string(raw), "editor-body-secret") {
		t.Fatal("request editor exposed saved credentials")
	}
	draft := payload["requests"].([]any)[0].(map[string]any)
	draft["displayName"] = "List items"
	draft["serviceId"] = "service.secondary"
	execution := draft["execution"].(map[string]any)
	execution["query"] = map[string]any{"limit": "3"}
	body, _ := json.Marshal(map[string]any{"request": draft})
	doCaseCatalogRequest(t, handler, http.MethodPut, "/api/request-editor", string(body), "", http.StatusPreconditionRequired)
	updated, _, _ := doCaseCatalogRequest(t, handler, http.MethodPut, "/api/request-editor", string(body), get.Header.Get("ETag"), http.StatusOK)
	doCaseCatalogRequest(t, handler, http.MethodPut, "/api/request-editor", string(body), get.Header.Get("ETag"), http.StatusPreconditionFailed)
	snapshot, err := runtime.GetProfileCatalogSnapshot(ctx, "editor")
	if err != nil || !strings.Contains(snapshot.Catalog.TemplateConfigs[0].ConfigJSON, "editor-secret") || !strings.Contains(snapshot.Catalog.TemplateConfigs[0].ConfigJSON, "editor-body-secret") || !strings.Contains(snapshot.Catalog.TemplateConfigs[0].ConfigJSON, `"marker":"keep"`) {
		t.Fatalf("round trip lost native configuration: %v", err)
	}
	if snapshot.Catalog.InterfaceNodes[0].ServiceID != "service.secondary" {
		t.Fatal("request editor did not persist the selected service")
	}
	_, _, run := doCaseCatalogRequest(t, handler, http.MethodPost, "/api/test-kit/run", `{"caseId":"case.items"}`, "", http.StatusOK)
	if run["status"] != "passed" {
		t.Fatalf("saved request did not execute: %#v", run)
	}
	create := `{"request":{"id":"case.new","nodeId":"node.new","serviceId":"service.http","displayName":"New request","baseUrl":"` + target.URL + `","execution":{"method":"GET","path":"/items","expectedHttpCodes":[200]}}}`
	_, _, saved := doCaseCatalogRequest(t, handler, http.MethodPut, "/api/request-editor", create, updated.Header.Get("ETag"), http.StatusOK)
	if len(saved["requests"].([]any)) != 2 {
		t.Fatal("new request was not registered in Store")
	}
}
