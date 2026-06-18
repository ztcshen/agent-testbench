package controlplane_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

func TestServerExposesTestKitRunContracts(t *testing.T) {
	ctx := context.Background()
	s := openTestKitSQLiteStore(t, ctx, "sandbox.sqlite")

	bundle := profile.Bundle{
		ID:          "sample",
		DisplayName: "Sample Profile",
		APICases: []profile.APICase{
			{ID: "case.alpha", DisplayName: "Case Alpha", NodeID: "node.alpha"},
		},
	}
	server := httptest.NewServer(controlplane.NewWithStore(bundle, s))
	defer server.Close()

	var result map[string]any
	postJSONInto(t, server.URL+"/api/test-kit/run", `{
		"caseId":"case.alpha",
		"workflowId":"workflow.alpha",
			"stepId":"step.alpha"
		}`, http.StatusOK, &result)
	if result["ok"] != false || result["caseId"] != "case.alpha" || result["stepId"] != "step.alpha" {
		t.Fatalf("test kit run result = %#v", result)
	}

	runs := decodeJSONResponse(t, server.URL+"/api/runs", http.StatusOK)
	workflowRuns := runs["workflowRuns"].([]any)
	if len(workflowRuns) != 1 || workflowRuns[0].(map[string]any)["workflowId"] != "workflow.alpha" {
		t.Fatalf("test kit run should be indexed in store: %#v", runs)
	}
}

func TestServerExposesTestKitBatchContract(t *testing.T) {
	bundle := profile.Bundle{
		ID:          "sample",
		DisplayName: "Sample Profile",
		APICases: []profile.APICase{
			{ID: "case.alpha", DisplayName: "Case Alpha", NodeID: "node.alpha"},
			{ID: "case.beta", DisplayName: "Case Beta", NodeID: "node.alpha"},
		},
	}
	server := httptest.NewServer(controlplane.New(bundle))
	defer server.Close()

	var payload struct {
		OK      bool             `json:"ok"`
		Results []map[string]any `json:"results"`
		Summary struct {
			CaseCount int `json:"caseCount"`
			Passed    int `json:"passed"`
		} `json:"summary"`
	}
	postJSONInto(t, server.URL+"/api/test-kit/run-batch", `{
			"caseIds":["case.alpha","case.beta"]
		}`, http.StatusOK, &payload)
	if payload.OK || len(payload.Results) != 2 || payload.Summary.CaseCount != 2 || payload.Summary.Passed != 0 {
		t.Fatalf("test kit batch payload = %#v", payload)
	}
}

func TestServerTestKitBatchForwardsOverrides(t *testing.T) {
	ctx := context.Background()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("item_id") != "item-123" {
			t.Fatalf("target query = %s", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer target.Close()

	s := openTestKitSQLiteStore(t, ctx, "sandbox.sqlite")
	if err := s.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID: "sample",
		IndexedAt: time.Now().UTC(),
		APICases: []store.CatalogAPICase{
			{ID: "case.alpha", DisplayName: "Case Alpha", NodeID: "node.alpha", BaseURL: target.URL, Status: "active"},
		},
		TemplateConfigs: []store.CatalogTemplateConfig{{
			ID:         "cfg.case.alpha",
			TemplateID: "template.case.alpha",
			ScopeType:  "api-case",
			ScopeID:    "case.alpha",
			Status:     "active",
			ConfigJSON: `{"caseId":"case.alpha","caseExecution":{"method":"GET","nodeId":"node.alpha","path":"/callback","query":{"item_id":"{{override:item_id}}"},"expectedHttpCodes":[200]},"inputs":[{"name":"item_id","source":"override"}]}`,
		}},
	}); err != nil {
		t.Fatalf("replace profile catalog: %v", err)
	}
	server := httptest.NewServer(controlplane.NewWithStore(profile.Bundle{ID: "sample"}, s))
	defer server.Close()

	var payload struct {
		OK      bool             `json:"ok"`
		Results []map[string]any `json:"results"`
	}
	postJSONInto(t, server.URL+"/api/test-kit/run-batch", `{
		"caseIds":["case.alpha"],
		"overrides":{"item_id":"item-123"}
	}`, http.StatusOK, &payload)
	if !payload.OK || len(payload.Results) != 1 || payload.Results[0]["status"] != store.StatusPassed {
		t.Fatalf("batch should pass with forwarded overrides = %#v", payload)
	}
}

func TestServerTestKitRequiredInputsAcceptFalseyOverrides(t *testing.T) {
	ctx := context.Background()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("enabled") != "false" || r.URL.Query().Get("quantity") != "0" {
			t.Fatalf("target query = %s", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer target.Close()

	s := openTestKitSQLiteStore(t, ctx, "sandbox.sqlite")
	if err := s.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID: "sample",
		IndexedAt: time.Now().UTC(),
		APICases: []store.CatalogAPICase{
			{ID: "case.falsey", DisplayName: "Case Falsey", NodeID: "node.alpha", BaseURL: target.URL, Status: "active"},
		},
		TemplateConfigs: []store.CatalogTemplateConfig{{
			ID:         "cfg.case.falsey",
			TemplateID: "template.case.falsey",
			ScopeType:  "api-case",
			ScopeID:    "case.falsey",
			Status:     "active",
			ConfigJSON: `{"caseId":"case.falsey","caseExecution":{"method":"GET","nodeId":"node.alpha","path":"/callback","query":{"enabled":"{{override:enabled}}","quantity":"{{override:quantity}}"},"expectedHttpCodes":[200]},"inputs":[{"name":"enabled","source":"override"},{"name":"quantity","source":"override"}]}`,
		}},
	}); err != nil {
		t.Fatalf("replace profile catalog: %v", err)
	}
	server := httptest.NewServer(controlplane.NewWithStore(profile.Bundle{ID: "sample"}, s))
	defer server.Close()

	var result map[string]any
	postJSONInto(t, server.URL+"/api/test-kit/run", `{
		"caseId":"case.falsey",
		"overrides":{"enabled":false,"quantity":0}
	}`, http.StatusOK, &result)
	if result["ok"] != true || result["status"] != store.StatusPassed {
		t.Fatalf("required inputs should accept falsey overrides = %#v", result)
	}
}

func TestServerTestKitRequiredInputsUsePresenceNotStringTruthiness(t *testing.T) {
	ctx := context.Background()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer target.Close()

	s := openTestKitSQLiteStore(t, ctx, "sandbox.sqlite")
	if err := s.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID: "sample",
		IndexedAt: time.Now().UTC(),
		APICases: []store.CatalogAPICase{
			{ID: "case.empty", DisplayName: "Case Empty", NodeID: "node.alpha", BaseURL: target.URL, Status: "active"},
		},
		TemplateConfigs: []store.CatalogTemplateConfig{{
			ID:         "cfg.case.empty",
			TemplateID: "template.case.empty",
			ScopeType:  "api-case",
			ScopeID:    "case.empty",
			Status:     "active",
			ConfigJSON: `{"caseId":"case.empty","caseExecution":{"method":"GET","nodeId":"node.alpha","path":"/callback","expectedHttpCodes":[200]},"inputs":[{"name":"note","source":"override"}]}`,
		}},
	}); err != nil {
		t.Fatalf("replace profile catalog: %v", err)
	}
	server := httptest.NewServer(controlplane.NewWithStore(profile.Bundle{ID: "sample"}, s))
	defer server.Close()

	var result map[string]any
	postJSONInto(t, server.URL+"/api/test-kit/run", `{
		"caseId":"case.empty",
		"overrides":{"note":""}
	}`, http.StatusOK, &result)
	if result["ok"] != true || result["status"] != store.StatusPassed {
		t.Fatalf("required inputs should use key presence, result = %#v", result)
	}
}
