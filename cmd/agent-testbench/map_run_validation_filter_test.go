package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"agent-testbench/internal/domain/mapplanner"
	"agent-testbench/internal/store"
)

func TestMapRunFiltersValidationCasesByInterfaceAndFamily(t *testing.T) {
	ctx := context.Background()
	storeRef := seedExecutableMapCommandStoreWithValidationFamilies(t, ctx)
	runCLI(t, "map", "import-workflows", "--store", storeRef, "--json")

	report := decodeMapRunCommandReport(t, runCLI(t, "map", "run", "--store", storeRef, "--map", "map.profile.flow", "--scope", "cases", "--interface", "node.submit", "--validation-family", "length", "--json"))
	if !report.OK || report.Summary.TotalTasks != 2 || len(report.Tasks) != 2 {
		t.Fatalf("filtered validation run report = %#v", report)
	}
	caseTasks := 0
	for _, task := range report.Tasks {
		if task.Kind != mapplanner.TaskRunCase {
			continue
		}
		caseTasks++
		if task.CaseID != "case.submit.length.invalid" {
			t.Fatalf("filtered run should execute only length validation case: %#v", report.Tasks)
		}
	}
	if caseTasks != 1 {
		t.Fatalf("filtered run should contain one validation case task: %#v", report.Tasks)
	}
}

func seedExecutableMapCommandStoreWithValidationFamilies(t *testing.T, ctx context.Context) string {
	t.Helper()
	lengthSeen := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/validate-length":
			lengthSeen = true
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":"too long"}`)
		case "/validate":
			t.Fatalf("required validation case should be filtered out")
		case "/prepare", "/submit":
			t.Fatalf("case-scope validation run should reuse materialization instead of running workflow path: %s", r.URL.Path)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(func() {
		server.Close()
		if !lengthSeen {
			t.Fatalf("length validation endpoint was not called")
		}
	})

	storePath := filepath.Join(t.TempDir(), "map-run-validation-family.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	catalog := mapCommandExecutableProfileCatalogFixture(server.URL)
	catalog.APICases = append(catalog.APICases, store.CatalogAPICase{
		ID:                "case.submit.length.invalid",
		DisplayName:       "Submit field too long",
		NodeID:            "node.submit",
		CaseType:          "negative",
		RequestTemplateID: "template.submit",
		RenderMode:        "template_patch",
		PatchJSON:         `[{"op":"replace","path":"$.field","value":"too-long"}]`,
		ExpectedJSON:      `{"status":400}`,
		BaseURL:           server.URL,
		Status:            "active",
		SortOrder:         4,
	})
	catalog.CaseDependencies = append(catalog.CaseDependencies, store.CatalogCaseDependency{
		ID: "dependency.length.invalid", CaseID: "case.submit.length.invalid", FixtureID: "fixture.before.submit", Required: true, MappingsJSON: `[]`,
	})
	catalog.TemplateConfigs = append(catalog.TemplateConfigs, store.CatalogTemplateConfig{
		ID:         "cfg.case.submit.length.invalid",
		ScopeType:  "api-case",
		ScopeID:    "case.submit.length.invalid",
		ConfigJSON: `{"caseId":"case.submit.length.invalid","caseExecution":{"method":"POST","nodeId":"node.submit","path":"/validate-length","body":{"field":"too-long"},"expectedHttpCodes":[400],"expectedResponseContains":["too long"]}}`,
		Status:     "active",
	})
	if err := runtime.ReplaceProfileCatalog(ctx, catalog); err != nil {
		t.Fatalf("seed executable profile catalog: %v", err)
	}
	closeCLIStore(runtime)
	return storeRef
}
