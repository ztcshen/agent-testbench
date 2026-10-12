package controlplane_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
)

func TestServerExposesEmptyProfileAssetLists(t *testing.T) {
	server := httptest.NewServer(controlplane.New(profile.Bundle{
		ID:          "empty",
		DisplayName: "Empty Profile",
	}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/profile/assets")
	if err != nil {
		t.Fatalf("get profile assets api: %v", err)
	}
	defer resp.Body.Close()

	var payload struct {
		Services       []profile.Service       `json:"services"`
		Workflows      []profile.Workflow      `json:"workflows"`
		InterfaceNodes []profile.InterfaceNode `json:"interfaceNodes"`
		APICases       []profile.APICase       `json:"apiCases"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode profile assets api: %v", err)
	}
	if payload.Services == nil || payload.Workflows == nil || payload.InterfaceNodes == nil || payload.APICases == nil {
		t.Fatalf("empty asset lists should encode as arrays: %#v", payload)
	}
}

func TestServerRequiresDesktopForRetiredBrowserPages(t *testing.T) {
	server := httptest.NewServer(controlplane.New(loadEmptyProfile(t)))
	defer server.Close()
	for _, path := range []string{"/index.html", "/dashboard.html", "/workflows.html", "/workflow-studio.html", "/api-cases.html", "/workflow-detail.html", "/environment-nodes.html", "/evidence-viewer.html", "/styles.css", "/assets/react/workflowStudio.js"} {
		resp, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		err = json.NewDecoder(resp.Body).Decode(&payload)
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusGone || payload["frontend"] != "electron" {
			t.Fatalf("%s should require the desktop app: status=%d payload=%#v err=%v", path, resp.StatusCode, payload, err)
		}
	}
}

func TestServerRootDescribesElectronAPIWorkspace(t *testing.T) {
	server := httptest.NewServer(controlplane.New(loadEmptyProfile(t)))
	defer server.Close()
	resp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || payload["frontend"] != "electron" || payload["product"] != "AgentTestBench" {
		t.Fatalf("unexpected root response: %#v", payload)
	}
}

func TestServerExposesStateForWorkbenchIndex(t *testing.T) {
	bundle := profile.Bundle{
		ID:          "sample",
		DisplayName: "Sample Profile",
		Services: []profile.Service{
			{ID: "service.alpha", DisplayName: "Service Alpha", Kind: "http"},
		},
	}
	server := httptest.NewServer(controlplane.New(bundle))
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/state")
	if err != nil {
		t.Fatalf("get state api: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("state status = %d", resp.StatusCode)
	}

	var payload struct {
		Services []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Kind   string `json:"kind"`
			Status string `json:"status"`
		} `json:"services"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode state api: %v", err)
	}
	if len(payload.Services) != 1 || payload.Services[0].ID != "service.alpha" || payload.Services[0].Name != "Service Alpha" {
		t.Fatalf("state services = %#v", payload.Services)
	}
	if payload.Services[0].Status != "missing" {
		t.Fatalf("state service status = %#v", payload.Services[0])
	}
}
