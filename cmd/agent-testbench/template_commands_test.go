package main

import (
	"context"
	"encoding/json"
	"testing"

	"agent-testbench/internal/store"
)

func TestWorkflowAuditAllowsExplicitOfflineTemplatePackage(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowProfile(t, dir)

	out := runCLI(t, "workflow", "audit", "--profile", dir, "--offline-template-package", "--workflow", "workflow.alpha", "--json")
	var report struct {
		OK         bool   `json:"ok"`
		WorkflowID string `json:"workflowId"`
		IssueCount int    `json:"issueCount"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode offline workflow audit json: %v\n%s", err, out)
	}
	if !report.OK || report.WorkflowID != "workflow.alpha" || report.IssueCount != 0 {
		t.Fatalf("offline workflow audit report = %#v", report)
	}
}

func TestTemplateRenderCommandPrintsRequestPreview(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-template-render-pg")
	runTemplateRenderCommandPrintsRequestPreview(t, storeRef, "PostgreSQL")
}

func TestTemplateRenderCommandUsesNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-template-render-mysql")
	runTemplateRenderCommandPrintsRequestPreview(t, storeRef, "MySQL")
}

func runTemplateRenderCommandPrintsRequestPreview(t *testing.T, storeRef string, label string) {
	t.Helper()
	dir := t.TempDir()
	writeTemplateProfile(t, dir)
	runCLI(t, "config", "publish", "--from", dir)

	out := runCLI(t, "template", "render", "--template", "template.create", "--fixture", "fixture.item")

	var rendered struct {
		Method string         `json:"method"`
		Path   string         `json:"path"`
		Body   map[string]any `json:"body"`
	}
	if err := json.Unmarshal([]byte(out), &rendered); err != nil {
		t.Fatalf("decode %s template render output: %v\n%s", label, err, out)
	}
	if rendered.Method != "POST" || rendered.Path != "/v1/items/item-001" {
		t.Fatalf("%s rendered request identity = %#v", label, rendered)
	}
	if rendered.Body["id"] != "item-001" || rendered.Body["quantity"].(float64) != 3 {
		t.Fatalf("%s rendered request body = %#v", label, rendered.Body)
	}

	ctx := context.Background()
	s, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open %s template store: %v", label, err)
	}
	if err := s.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID: "current",
		InterfaceNodes: []store.CatalogInterfaceNode{
			{ID: "node.store", Method: "PATCH", Path: "/v1/items/{{.itemId}}", Status: "active"},
		},
		RequestTemplates: []store.CatalogRequestTemplate{
			{
				ID:           "template.store",
				NodeID:       "node.store",
				Method:       "PATCH",
				Path:         "/v1/items/{{.itemId}}",
				TemplateJSON: `{"id":"{{.itemId}}","enabled":{{.enabled}}}`,
			},
		},
		Fixtures: []store.CatalogFixture{
			{
				ID:       "fixture.store",
				Kind:     "json",
				DataJSON: `{"itemId":"item-002","enabled":true}`,
			},
		},
	}); err != nil {
		t.Fatalf("seed %s template store: %v", label, err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close %s template store: %v", label, err)
	}

	storeOut := runCLI(t, "template", "render", "--template", "template.store", "--fixture", "fixture.store")
	var storeRendered struct {
		Method string         `json:"method"`
		Path   string         `json:"path"`
		Body   map[string]any `json:"body"`
	}
	if err := json.Unmarshal([]byte(storeOut), &storeRendered); err != nil {
		t.Fatalf("decode %s store template render output: %v\n%s", label, err, storeOut)
	}
	if storeRendered.Method != "PATCH" || storeRendered.Path != "/v1/items/item-002" || storeRendered.Body["enabled"] != true {
		t.Fatalf("%s store rendered request = %#v", label, storeRendered)
	}
}
