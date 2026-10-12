package controlplane_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

func TestTrustedTestKitExecutesStoreCaseReferenceFromCatalog(t *testing.T) {
	for _, bundleCase := range []bool{false, true} {
		name := "catalog"
		if bundleCase {
			name = "bundle-enriched-from-catalog"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			calls := 0
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodGet || r.URL.Path != "/record" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				_, _ = w.Write([]byte(`{"status":"ready"}`))
			}))
			defer target.Close()
			runtime := openTestKitSQLiteStore(t, ctx, name+".sqlite")
			casePath := "store://cases/case.record"
			catalog := store.ProfileCatalog{
				ProfileID: "sample", IndexedAt: time.Now().UTC(),
				APICases: []store.CatalogAPICase{{
					ID: "case.record", Status: "active", CasePath: casePath,
					BaseURL: target.URL, EvidenceDir: filepath.Join(t.TempDir(), "evidence"),
				}},
				TemplateConfigs: []store.CatalogTemplateConfig{{
					ID: "config.record", ScopeType: "case", ScopeID: "case.record", Status: "active",
					ConfigJSON: `{"caseExecution":{"method":"GET","path":"/record","expectedHttpCodes":[200],"expectedResponseContains":["ready"]}}`,
				}},
			}
			if err := runtime.ReplaceProfileCatalog(ctx, catalog); err != nil {
				t.Fatal(err)
			}
			bundle := profile.Bundle{ID: "sample"}
			if bundleCase {
				bundle.APICases = []profile.APICase{{ID: "case.record", CasePath: casePath}}
			}
			result, err := controlplane.RunTrustedTestKitCase(ctx, bundle, runtime, controlplane.TrustedTestKitRunRequest{
				CaseID: "case.record", InlineTraceCollect: true,
			})
			if err != nil || result["ok"] != true || calls != 1 {
				t.Fatalf("Store reference execution = %#v, requests=%d, err=%v", result, calls, err)
			}
			runID, _ := result["runId"].(string)
			runs, err := runtime.ListAPICaseRuns(ctx, runID)
			if err != nil || len(runs) != 1 || runs[0].Status != store.StatusPassed {
				t.Fatalf("stored case runs = %#v, err=%v", runs, err)
			}
			root, _ := result["evidenceRoot"].(string)
			response := readTestKitEvidenceObject(t, filepath.Join(root, "response.json"))
			if response["statusCode"] != float64(http.StatusOK) {
				t.Fatalf("persisted HTTP response evidence = %#v", response)
			}
		})
	}
}

func TestTrustedTestKitKeepsPhysicalFileCasePrecedence(t *testing.T) {
	for _, casePath := range []string{"missing-case.json", "store://assets/case.record"} {
		t.Run(strings.ReplaceAll(casePath, "/", "-"), func(t *testing.T) {
			ctx := context.Background()
			calls := 0
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.WriteHeader(http.StatusOK)
			}))
			defer target.Close()
			runtime := openTestKitSQLiteStore(t, ctx, "physical.sqlite")
			if err := runtime.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
				ProfileID: "sample", IndexedAt: time.Now().UTC(),
				APICases: []store.CatalogAPICase{{ID: "case.record", CasePath: casePath, BaseURL: target.URL}},
				TemplateConfigs: []store.CatalogTemplateConfig{{
					ID: "config.record", ScopeType: "case", ScopeID: "case.record", Status: "active",
					ConfigJSON: `{"caseExecution":{"method":"GET","path":"/record"}}`,
				}},
			}); err != nil {
				t.Fatal(err)
			}
			result, err := controlplane.RunTrustedTestKitCase(ctx, profile.Bundle{ID: "sample"}, runtime, controlplane.TrustedTestKitRunRequest{CaseID: "case.record", InlineTraceCollect: true})
			if err != nil || result["ok"] != false || calls != 0 {
				t.Fatalf("missing physical case = %#v, requests=%d, err=%v", result, calls, err)
			}
			failure, _ := result["error"].(string)
			if !strings.Contains(failure, "read api case") {
				t.Fatalf("physical source should remain authoritative: %s", failure)
			}
		})
	}
}
