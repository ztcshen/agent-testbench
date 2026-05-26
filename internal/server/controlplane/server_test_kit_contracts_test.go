package controlplane_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
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
