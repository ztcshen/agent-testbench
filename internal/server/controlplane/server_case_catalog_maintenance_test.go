package controlplane_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

const caseCatalogSecretSentinel = "catalog-secret-must-not-leak"

func TestServerCaseCatalogPatchUsesETagCASAndPreservesHiddenConfig(t *testing.T) {
	ctx, runtime, handler, first := newCaseCatalogMaintenanceServer(t)

	getResponse, getRaw, getPayload := doCaseCatalogRequest(t, handler, http.MethodGet, "/api/case-catalog", "", "", http.StatusOK)
	firstETag := getResponse.Header.Get("ETag")
	if firstETag == "" || getPayload["revision"] != float64(first.Revision) {
		t.Fatalf("initial catalog response = etag %q payload %#v", firstETag, getPayload)
	}
	assertCaseCatalogResponseIsSafe(t, getRaw)

	doCaseCatalogRequest(t, handler, http.MethodPatch, "/api/case-catalog", `{"case":{"id":"case.alpha","description":"missing precondition"}}`, "", http.StatusPreconditionRequired)
	missingSnapshot, err := runtime.GetProfileCatalogSnapshot(ctx, "sample")
	if err != nil || missingSnapshot.Revision != first.Revision {
		t.Fatalf("missing If-Match changed catalog: snapshot=%#v err=%v", missingSnapshot, err)
	}

	updatedResponse, _, updatedPayload := doCaseCatalogRequest(t, handler, http.MethodPatch, "/api/case-catalog", `{"case":{"id":"case.alpha","description":"updated safely","tags":["regression","smoke"]}}`, firstETag, http.StatusOK)
	secondETag := updatedResponse.Header.Get("ETag")
	if secondETag == "" || secondETag == firstETag || updatedPayload["revision"] != float64(first.Revision+1) {
		t.Fatalf("updated catalog response = etag %q payload %#v", secondETag, updatedPayload)
	}

	_, _, conflict := doCaseCatalogRequest(t, handler, http.MethodPatch, "/api/case-catalog", `{"case":{"id":"case.alpha","description":"stale overwrite"}}`, firstETag, http.StatusPreconditionFailed)
	if conflict["code"] != "catalog_revision_conflict" || conflict["currentRevision"] != float64(first.Revision+1) {
		t.Fatalf("stale patch response = %#v", conflict)
	}

	stored, err := runtime.GetProfileCatalogSnapshot(ctx, "sample")
	if err != nil {
		t.Fatalf("get stored catalog: %v", err)
	}
	if stored.Catalog.APICases[0].Description != "updated safely" {
		t.Fatalf("stale update changed case: %#v", stored.Catalog.APICases[0])
	}
	if stored.Catalog.APICases[0].PayloadTemplateJSON != `{"token":"`+caseCatalogSecretSentinel+`"}` ||
		stored.Catalog.TemplateConfigs[0].ConfigJSON != `{"secret":"`+caseCatalogSecretSentinel+`"}` {
		t.Fatalf("safe patch did not preserve hidden catalog fields: %#v", stored.Catalog)
	}
}

func TestServerCaseCatalogPatchAppliesDraftDefaultAndSharedValidation(t *testing.T) {
	ctx, runtime, handler, _ := newCaseCatalogMaintenanceServer(t)
	etag := caseCatalogCurrentETag(t, handler)

	createdResponse, _, created := doCaseCatalogRequest(t, handler, http.MethodPatch, "/api/case-catalog", `{"case":{"id":"case.new","displayName":"New Draft","nodeId":"node.alpha"}}`, etag, http.StatusOK)
	createdCase := created["case"].(map[string]any)
	if created["created"] != true || createdCase["status"] != "draft" {
		t.Fatalf("new case response = %#v", created)
	}
	etag = createdResponse.Header.Get("ETag")

	_, _, invalidStatus := doCaseCatalogRequest(t, handler, http.MethodPatch, "/api/case-catalog", `{"case":{"id":"case.new","status":"unknown"}}`, etag, http.StatusBadRequest)
	assertCaseCatalogIssueCode(t, invalidStatus, "invalid-status")
	_, _, unrunnable := doCaseCatalogRequest(t, handler, http.MethodPatch, "/api/case-catalog", `{"case":{"id":"case.new","status":"active"}}`, etag, http.StatusBadRequest)
	assertCaseCatalogIssueCode(t, unrunnable, "active-case-not-runnable")

	snapshot, err := runtime.GetProfileCatalogSnapshot(ctx, "sample")
	if err != nil {
		t.Fatalf("get catalog after invalid patches: %v", err)
	}
	if snapshot.Revision != 2 || snapshot.Catalog.APICases[1].Status != "draft" {
		t.Fatalf("invalid patch mutated catalog: %#v", snapshot)
	}
}

func TestServerCaseCatalogHistoryIsSafeAndRollbackCreatesRevision(t *testing.T) {
	_, _, handler, first := newCaseCatalogMaintenanceServer(t)
	firstETag := caseCatalogCurrentETag(t, handler)
	updatedResponse, _, _ := doCaseCatalogRequest(t, handler, http.MethodPatch, "/api/case-catalog", `{"case":{"id":"case.alpha","description":"revision two"}}`, firstETag, http.StatusOK)
	secondETag := updatedResponse.Header.Get("ETag")

	_, historyRaw, history := doCaseCatalogRequest(t, handler, http.MethodGet, "/api/case-catalog/history", "", "", http.StatusOK)
	assertCaseCatalogResponseIsSafe(t, historyRaw)
	items := history["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["revision"] != float64(2) || items[1].(map[string]any)["revision"] != float64(1) {
		t.Fatalf("catalog history = %#v", history)
	}
	seedSummary := items[1].(map[string]any)["summary"].(map[string]any)
	if len(seedSummary) != 1 || seedSummary["caseId"] != "case.alpha" {
		t.Fatalf("history returned unsafe or incomplete seed summary: %#v", seedSummary)
	}

	rollbackResponse, _, rollback := doCaseCatalogRequest(t, handler, http.MethodPost, "/api/case-catalog/rollback", `{"revision":1}`, secondETag, http.StatusOK)
	thirdETag := rollbackResponse.Header.Get("ETag")
	if rollback["revision"] != float64(first.Revision+2) || thirdETag == "" || thirdETag == secondETag {
		t.Fatalf("rollback response = etag %q payload %#v", thirdETag, rollback)
	}

	_, getRaw, current := doCaseCatalogRequest(t, handler, http.MethodGet, "/api/case-catalog", "", "", http.StatusOK)
	assertCaseCatalogResponseIsSafe(t, getRaw)
	caseValue := current["cases"].([]any)[0].(map[string]any)
	if caseValue["description"] != "before" || current["revision"] != float64(3) {
		t.Fatalf("rolled back catalog = %#v", current)
	}

	_, _, sameRevision := doCaseCatalogRequest(t, handler, http.MethodPost, "/api/case-catalog/rollback", `{"revision":1}`, thirdETag, http.StatusConflict)
	if sameRevision["error"] != "requested catalog revision is already current" {
		t.Fatalf("same-content rollback response = %#v", sameRevision)
	}

	_, finalHistoryRaw, finalHistory := doCaseCatalogRequest(t, handler, http.MethodGet, "/api/case-catalog/history", "", "", http.StatusOK)
	assertCaseCatalogResponseIsSafe(t, finalHistoryRaw)
	finalItems := finalHistory["items"].([]any)
	if len(finalItems) != 3 || finalItems[0].(map[string]any)["operation"] != "rollback" {
		t.Fatalf("history after rollback = %#v", finalHistory)
	}
	rollbackSummary := finalItems[0].(map[string]any)["summary"].(map[string]any)
	if rollbackSummary["targetRevision"] != float64(1) || rollbackSummary["previousRevision"] != float64(2) {
		t.Fatalf("rollback history summary = %#v", rollbackSummary)
	}
}

func TestServerCaseCatalogRequiresVersionedRuntime(t *testing.T) {
	handler := controlplane.NewWithStore(profile.Bundle{ID: "sample"}, nil)
	_, _, payload := doCaseCatalogRequest(t, handler, http.MethodGet, "/api/case-catalog", "", "", http.StatusNotImplemented)
	if payload["error"] != "runtime store is not configured" {
		t.Fatalf("unconfigured runtime response = %#v", payload)
	}
}

func newCaseCatalogMaintenanceServer(t *testing.T) (context.Context, *sqlite.Store, http.Handler, store.ProfileCatalogSnapshot) {
	t.Helper()
	ctx, runtime := openAPICaseBatchSQLiteStore(t)
	catalogValue := store.ProfileCatalog{
		ProfileID: "sample",
		IndexedAt: time.Date(2026, time.July, 16, 4, 0, 0, 0, time.UTC),
		InterfaceNodes: []store.CatalogInterfaceNode{{
			ID: "node.alpha", DisplayName: "Alpha", Method: http.MethodPost, Path: "/alpha", Status: "active",
		}},
		APICases: []store.CatalogAPICase{{
			ID: "case.alpha", DisplayName: "Alpha Case", Description: "before", NodeID: "node.alpha",
			Status: "draft", PayloadTemplateJSON: `{"token":"` + caseCatalogSecretSentinel + `"}`,
			PatchJSON: `{"password":"` + caseCatalogSecretSentinel + `"}`,
		}},
		RequestTemplates: []store.CatalogRequestTemplate{{
			ID: "template.alpha", DisplayName: "Alpha Template", NodeID: "node.alpha", Method: http.MethodPost,
			Path: "/alpha", Status: "active", TemplateJSON: `{"credential":"` + caseCatalogSecretSentinel + `"}`,
		}},
		TemplateConfigs: []store.CatalogTemplateConfig{{
			ID: "config.alpha", ScopeType: "case", ScopeID: "case.alpha", Status: "active",
			ConfigJSON: `{"secret":"` + caseCatalogSecretSentinel + `"}`,
		}},
	}
	first, err := runtime.CompareAndSwapProfileCatalog(ctx, 0, catalogValue, store.ProfileCatalogMutation{
		Operation:   "seed",
		SummaryJSON: `{"caseId":"case.alpha","secret":"` + caseCatalogSecretSentinel + `"}`,
	})
	if err != nil {
		t.Fatalf("seed versioned catalog: %v", err)
	}
	return ctx, runtime, controlplane.NewWithStore(profile.Bundle{ID: "bootstrap"}, runtime), first
}

func doCaseCatalogRequest(t *testing.T, handler http.Handler, method string, path string, body string, ifMatch string, wantStatus int) (*http.Response, []byte, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if ifMatch != "" {
		request.Header.Set("If-Match", ifMatch)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	response := recorder.Result()
	t.Cleanup(func() { response.Body.Close() })
	raw := recorder.Body.Bytes()
	if response.StatusCode != wantStatus {
		t.Fatalf("%s %s status = %d, want %d body=%s", method, path, response.StatusCode, wantStatus, raw)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode %s %s: %v body=%s", method, path, err, raw)
	}
	return response, raw, payload
}

func caseCatalogCurrentETag(t *testing.T, handler http.Handler) string {
	t.Helper()
	response, _, _ := doCaseCatalogRequest(t, handler, http.MethodGet, "/api/case-catalog", "", "", http.StatusOK)
	return response.Header.Get("ETag")
}

func assertCaseCatalogResponseIsSafe(t *testing.T, raw []byte) {
	t.Helper()
	text := string(raw)
	for _, forbidden := range []string{
		caseCatalogSecretSentinel,
		`"catalog"`,
		`"configJson"`,
		`"templateJson"`,
		`"payloadTemplateJson"`,
		`"patchJson"`,
		`"expectedJson"`,
		`"defaultOverridesJson"`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("case catalog response exposed %q: %s", forbidden, raw)
		}
	}
}

func assertCaseCatalogIssueCode(t *testing.T, payload map[string]any, want string) {
	t.Helper()
	issues, ok := payload["issues"].([]any)
	if !ok {
		t.Fatalf("validation issues missing: %#v", payload)
	}
	for _, item := range issues {
		if issue, ok := item.(map[string]any); ok && issue["code"] == want {
			return
		}
	}
	t.Fatalf("validation issue %q missing: %#v", want, payload)
}
