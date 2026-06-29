package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

func TestCaseCatalogUpsertCreatesActiveStoreBackedAPICase(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "case-catalog.sqlite")
	storeRef := "sqlite://" + storePath
	ctx := context.Background()
	seedCaseCatalogUpsertStore(t, ctx, storePath)

	out := runCLI(t, "case", "catalog", "upsert",
		"--store", storeRef,
		"--case", "case.submit.smoke",
		"--node", "node.submit",
		"--display-name", "Submit smoke",
		"--request-template", "template.submit",
		"--render-mode", "template_patch",
		"--patch-json", `[{"op":"add","path":"$.trace","value":"smoke"}]`,
		"--expected-json", `{"status":200}`,
		"--default-override", "executorParam=ent8001",
		"--json",
	)
	var report struct {
		OK      bool `json:"ok"`
		Created bool `json:"created"`
		Case    struct {
			ID                string         `json:"id"`
			DisplayName       string         `json:"displayName"`
			NodeID            string         `json:"nodeId"`
			RequestTemplateID string         `json:"requestTemplateId"`
			RenderMode        string         `json:"renderMode"`
			Status            string         `json:"status"`
			DefaultOverrides  map[string]any `json:"defaultOverrides"`
		} `json:"case"`
		Counts struct {
			Before struct {
				APICases int `json:"apiCases"`
			} `json:"before"`
			After struct {
				APICases int `json:"apiCases"`
			} `json:"after"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode case catalog upsert json: %v\n%s", err, out)
	}
	if !report.OK || !report.Created || report.Case.ID != "case.submit.smoke" || report.Case.Status != "active" || report.Case.RenderMode != "template_patch" {
		t.Fatalf("case catalog upsert report = %#v", report)
	}
	if report.Case.DefaultOverrides["executorParam"] != "ent8001" || report.Counts.Before.APICases != 0 || report.Counts.After.APICases != 1 {
		t.Fatalf("case catalog counts/defaults = %#v", report)
	}

	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("reopen sqlite store: %v", err)
	}
	defer s.Close()
	catalog, err := s.GetProfileCatalog(ctx)
	if err != nil {
		t.Fatalf("get profile catalog: %v", err)
	}
	item, ok := findCatalogAPICase(catalog.APICases, "case.submit.smoke")
	if !ok || item.NodeID != "node.submit" || item.RequestTemplateID != "template.submit" || item.DefaultOverridesJSON != `{"executorParam":"ent8001"}` {
		t.Fatalf("persisted api case = %#v", item)
	}
}

func TestCaseCatalogUpsertRejectsInvalidJSONFieldShapes(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "case-catalog-json-shape.sqlite")
	storeRef := "sqlite://" + storePath
	ctx := context.Background()
	seedCaseCatalogUpsertStore(t, ctx, storePath)

	patchOut := runCLIFails(t, "case", "catalog", "upsert",
		"--store", storeRef,
		"--case", "case.submit.bad-patch",
		"--node", "node.submit",
		"--patch-json", `{}`,
		"--json",
	)
	if !strings.Contains(patchOut, "--patch-json must be a JSON array") {
		t.Fatalf("patch shape error = %s", patchOut)
	}

	expectedOut := runCLIFails(t, "case", "catalog", "upsert",
		"--store", storeRef,
		"--case", "case.submit.bad-expected",
		"--node", "node.submit",
		"--expected-json", `[]`,
		"--json",
	)
	if !strings.Contains(expectedOut, "--expected-json must be a JSON object") {
		t.Fatalf("expected shape error = %s", expectedOut)
	}
}

func seedCaseCatalogUpsertStore(t *testing.T, ctx context.Context, storePath string) {
	t.Helper()
	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer s.Close()
	if err := s.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID: "default",
		IndexedAt: time.Now().UTC(),
		InterfaceNodes: []store.CatalogInterfaceNode{{
			ID:     "node.submit",
			Method: "POST",
			Path:   "/submit",
			Status: "active",
		}},
		RequestTemplates: []store.CatalogRequestTemplate{{
			ID:     "template.submit",
			NodeID: "node.submit",
			Method: "POST",
			Path:   "/submit",
			Status: "active",
		}},
	}); err != nil {
		t.Fatalf("replace profile catalog: %v", err)
	}
}
