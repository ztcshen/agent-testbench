package controlplane_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

func TestStepLibraryVersionsParametersAndRepeatedReferences(t *testing.T) {
	ctx, runtime := openAPICaseBatchSQLiteStore(t)
	target, recorded := stepLibraryRecordingTarget(t)
	catalog := stepLibraryHTTPFixtureCatalog(t, target.URL)
	if _, err := runtime.CompareAndSwapProfileCatalog(ctx, 0, catalog, store.ProfileCatalogMutation{Operation: "seed"}); err != nil {
		t.Fatal(err)
	}
	handler := controlplane.NewWithStore(profile.Bundle{ID: catalog.ProfileID}, runtime)
	get, _, _ := doCaseCatalogRequest(t, handler, http.MethodGet, "/api/step-library", "", "", http.StatusOK)
	definition := map[string]any{"id": "block.consumer", "displayName": "Create item", "steps": []any{map[string]any{"id": "consume", "caseId": "case.consumer", "required": true}}, "parameters": []any{
		map[string]any{"name": "itemId", "dataType": "string", "required": true, "targets": []any{map[string]any{"stepId": "consume", "parameter": "itemId"}}},
		map[string]any{"name": "quantity", "dataType": "integer", "required": true, "targets": []any{map[string]any{"stepId": "consume", "parameter": "quantity"}}},
		map[string]any{"name": "enabled", "dataType": "boolean", "required": true, "hasDefault": true, "defaultValue": false, "targets": []any{map[string]any{"stepId": "consume", "parameter": "enabled"}}},
		map[string]any{"name": "note", "dataType": "string", "required": true, "hasDefault": true, "defaultValue": "original", "targets": []any{map[string]any{"stepId": "consume", "parameter": "note"}}},
	}}
	encode := func(value any) string { raw, _ := json.Marshal(value); return string(raw) }
	saved, _, library := doCaseCatalogRequest(t, handler, http.MethodPut, "/api/step-library", encode(map[string]any{"definition": definition}), get.Header.Get("ETag"), http.StatusOK)
	doCaseCatalogRequest(t, handler, http.MethodPut, "/api/step-library", encode(map[string]any{"definition": definition}), get.Header.Get("ETag"), http.StatusPreconditionFailed)
	block := library["blocks"].([]any)[0].(map[string]any)["definition"].(map[string]any)
	revision := block["revision"].(string)
	literal := `quoted "{{override:mustRemainLiteral}}"`
	steps := []any{map[string]any{"id": "source", "caseId": "case.source", "required": true}}
	for _, instance := range []string{"first", "second"} {
		group := map[string]any{"id": instance, "blockId": "block.consumer", "revision": revision, "values": map[string]any{"note": literal}, "bindings": []any{
			map[string]any{"parameter": "itemId", "sourceStepId": "source", "from": "responsebody", "path": "$.id"},
			map[string]any{"parameter": "quantity", "sourceStepId": "source", "from": "responsebody", "path": "$.quantity"},
		}}
		steps = append(steps, map[string]any{"id": instance + ".consume", "caseId": "case.consumer", "required": true, "group": group})
	}
	draft := map[string]any{"workflow": map[string]any{"id": "workflow.groups", "displayName": "Repeated groups"}, "steps": steps}
	workflowSaved, _, scene := doCaseCatalogRequest(t, handler, http.MethodPut, "/api/workflow-studio", encode(draft), saved.Header.Get("ETag"), http.StatusOK)
	loaded := scene["workflows"].([]any)[0].(map[string]any)
	loadedSteps := loaded["steps"].([]any)
	if len(loadedSteps) != 3 || loadedSteps[1].(map[string]any)["group"].(map[string]any)["revision"] != revision {
		t.Fatalf("group round trip lost metadata: %#v", loaded)
	}
	latest, _, library := doCaseCatalogRequest(t, handler, http.MethodGet, "/api/step-library", "", "", http.StatusOK)
	usages := library["blocks"].([]any)[0].(map[string]any)["usages"].([]any)
	if len(usages) != 2 {
		t.Fatalf("usage index should distinguish repeated instances: %#v", usages)
	}
	doCaseCatalogRequest(t, handler, http.MethodDelete, "/api/step-library", `{"id":"block.consumer"}`, latest.Header.Get("ETag"), http.StatusConflict)
	changed := strings.Replace(encode(definition), `"original"`, `"new default"`, 1)
	updated, _, library := doCaseCatalogRequest(t, handler, http.MethodPut, "/api/step-library", `{"definition":`+changed+`}`, workflowSaved.Header.Get("ETag"), http.StatusOK)
	newRevision := library["blocks"].([]any)[0].(map[string]any)["definition"].(map[string]any)["revision"].(string)
	if newRevision == revision {
		t.Fatal("edited definition did not get a new revision")
	}
	for _, raw := range library["blocks"].([]any)[0].(map[string]any)["usages"].([]any) {
		if !raw.(map[string]any)["outdated"].(bool) {
			t.Fatal("old references should be visibly outdated")
		}
	}
	// Old references remain valid after editing the library definition.
	_, _, _ = doCaseCatalogRequest(t, handler, http.MethodPut, "/api/workflow-studio", encode(loaded), updated.Header.Get("ETag"), http.StatusOK)
	_, _, started := doCaseCatalogRequest(t, handler, http.MethodPost, "/api/cases/batch-runs", `{"workflowId":"workflow.groups","requestId":"group-run"}`, "", http.StatusAccepted)
	id := started["batchRunId"].(string)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		_, _, report := doCaseCatalogRequest(t, handler, http.MethodGet, "/api/cases/batch-runs/"+id, "", "", http.StatusOK)
		if report["status"] == "running" {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		if report["status"] != "passed" {
			t.Fatalf("groups failed actual HTTP execution: %#v", report)
		}
		actual := recorded()
		if len(actual) != 2 {
			t.Fatalf("expected two independent invocations, got %d", len(actual))
		}
		for _, body := range actual {
			if body["quantity"] != float64(7) || body["enabled"] != false || body["note"] != literal {
				t.Fatalf("typed or literal input was changed: %#v", body)
			}
		}
		return
	}
	t.Fatal("group execution did not finish")
}

func TestStepLibraryRejectsIncompleteGroupsAndInvalidInputs(t *testing.T) {
	ctx, runtime := openAPICaseBatchSQLiteStore(t)
	config := `{"caseId":"case.read","caseExecution":{"method":"GET","path":"/items/{{override:count}}"}}`
	catalog := store.ProfileCatalog{ProfileID: "group-validation", IndexedAt: time.Now().UTC(), InterfaceNodes: []store.CatalogInterfaceNode{{ID: "node.read", Method: "GET", Path: "/items"}}, APICases: []store.CatalogAPICase{{ID: "case.read", NodeID: "node.read", Status: "active"}}, TemplateConfigs: []store.CatalogTemplateConfig{{ID: "read", ScopeType: "api-case", ScopeID: "case.read", ConfigJSON: config, Status: "active"}}}
	if _, err := runtime.CompareAndSwapProfileCatalog(ctx, 0, catalog, store.ProfileCatalogMutation{}); err != nil {
		t.Fatal(err)
	}
	handler := controlplane.NewWithStore(profile.Bundle{ID: catalog.ProfileID}, runtime)
	get, _, _ := doCaseCatalogRequest(t, handler, http.MethodGet, "/api/step-library", "", "", http.StatusOK)
	definition := `{"id":"block.read","displayName":"Read","steps":[{"id":"one","caseId":"case.read"},{"id":"two","caseId":"case.read"}],"parameters":[{"name":"count","dataType":"integer","required":true,"targets":[{"stepId":"one","parameter":"count"},{"stepId":"two","parameter":"count"}]}]}`
	saved, _, body := doCaseCatalogRequest(t, handler, http.MethodPut, "/api/step-library", `{"definition":`+definition+`}`, get.Header.Get("ETag"), http.StatusOK)
	revision := body["blocks"].([]any)[0].(map[string]any)["definition"].(map[string]any)["revision"].(string)
	group := fmt.Sprintf(`{"id":"instance","blockId":"block.read","revision":%q,"values":{"count":"wrong"}}`, revision)
	for _, steps := range []string{
		`[{"id":"instance.one","caseId":"case.read","group":` + group + `}]`,
		`[{"id":"instance.one","caseId":"case.read","group":` + group + `},{"id":"instance.two","caseId":"case.read","group":` + group + `}]`,
	} {
		doCaseCatalogRequest(t, handler, http.MethodPut, "/api/workflow-studio", `{"workflow":{"id":"workflow.invalid","displayName":"Invalid"},"steps":`+steps+`}`, saved.Header.Get("ETag"), http.StatusBadRequest)
	}
	_, _, latest := doCaseCatalogRequest(t, handler, http.MethodGet, "/api/workflow-studio", "", "", http.StatusOK)
	if len(latest["workflows"].([]any)) != 0 {
		t.Fatal("rejected edit changed the active catalog")
	}
}

func stepLibraryHTTPFixtureCatalog(t *testing.T, targetURL string) store.ProfileCatalog {
	t.Helper()
	catalog := store.ProfileCatalog{ProfileID: "step-library", IndexedAt: time.Now().UTC()}
	for _, name := range []string{"source", "consumer"} {
		method, path := "GET", "/source"
		if name == "consumer" {
			method, path = "POST", "/items/{{override:itemId}}"
		}
		catalog.InterfaceNodes = append(catalog.InterfaceNodes, store.CatalogInterfaceNode{ID: "node." + name, Method: method, Path: path, Status: "active"})
		catalog.APICases = append(catalog.APICases, store.CatalogAPICase{ID: "case." + name, NodeID: "node." + name, DisplayName: name, Status: "active", BaseURL: targetURL, EvidenceDir: t.TempDir()})
		execution := map[string]any{"method": method, "path": path, "expectedHttpCodes": []int{200}}
		if name == "consumer" {
			execution["body"] = map[string]any{"quantity": "{{override:quantity}}", "enabled": "{{override:enabled}}", "note": "{{override:note}}"}
		}
		raw, _ := json.Marshal(map[string]any{"caseId": "case." + name, "caseExecution": execution})
		catalog.TemplateConfigs = append(catalog.TemplateConfigs, store.CatalogTemplateConfig{ID: "config." + name, ScopeType: "api-case", ScopeID: "case." + name, Status: "active", ConfigJSON: string(raw)})
	}
	return catalog
}

func stepLibraryRecordingTarget(t *testing.T) (*httptest.Server, func() []map[string]any) {
	t.Helper()
	var mu sync.Mutex
	requests := []map[string]any{}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/source" {
			_, _ = w.Write([]byte(`{"id":"sample-42","quantity":7}`))
			return
		}
		if r.URL.Path != "/items/sample-42" {
			http.Error(w, "incorrect group binding", 400)
			return
		}
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "invalid body", 400)
			return
		}
		mu.Lock()
		requests = append(requests, body)
		mu.Unlock()
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(target.Close)
	return target, func() []map[string]any {
		mu.Lock()
		defer mu.Unlock()
		return append([]map[string]any(nil), requests...)
	}
}
