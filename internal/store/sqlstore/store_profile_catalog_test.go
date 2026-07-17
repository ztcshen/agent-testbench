package sqlstore_test

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlstore"
)

func TestStoreReplacesAndReadsProfileCatalogSnapshotThroughDatabaseSQL(t *testing.T) {
	exerciseStoreReplacesAndReadsProfileCatalogSnapshot(t, profileCatalogDialectExpectation{
		dialect:         sqlstore.PostgresDialect{},
		upsertFragments: []string{"on conflict(profile_id) do update"},
	})
}

func TestStoreReplacesProfileCatalogSnapshotUsesMySQLDialect(t *testing.T) {
	exerciseStoreReplacesAndReadsProfileCatalogSnapshot(t, profileCatalogDialectExpectation{
		dialect: sqlstore.MySQLDialect{},
		reject:  "$1",
		upsertFragments: []string{
			"on duplicate key update",
			"catalog_json = values(catalog_json)",
			"template_configs = values(template_configs)",
		},
		requireGeneratedCounts: true,
	})
}

func TestReplaceProfileCatalogFailsClosedWhenRevisionChangesAfterRead(t *testing.T) {
	ctx := context.Background()
	db, state := openFakeSQLDB(t)
	defer db.Close()
	runtime := sqlstore.New(db, sqlstore.PostgresDialect{})
	stale := sampleProfileCatalog(time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC))
	payload, err := json.Marshal(stale)
	if err != nil {
		t.Fatalf("encode stale profile catalog: %v", err)
	}

	// ReplaceProfileCatalog observes revision 1, then another maintenance writer
	// advances the head to revision 2 before the full-replace claim executes.
	state.queueRows(fakeRows{
		columns: []string{"catalog_json", "revision", "catalog_sha256", "updated_at"},
		values:  [][]driver.Value{{string(payload), int64(1), "revision-one", stale.IndexedAt.Format(time.RFC3339Nano)}},
	})
	state.queueRows(fakeRows{
		columns: []string{"revision"},
		values:  [][]driver.Value{{int64(2)}},
	})
	state.queueExecRowsAffected(0)

	err = runtime.ReplaceProfileCatalog(ctx, stale)
	if !errors.Is(err, store.ErrProfileCatalogRevisionConflict) {
		t.Fatalf("stale full replacement error = %v, want revision conflict", err)
	}
	var conflict *store.ProfileCatalogRevisionConflictError
	if !errors.As(err, &conflict) || conflict.ExpectedRevision != 1 || conflict.ActualRevision != 2 {
		t.Fatalf("stale full replacement conflict = %#v", conflict)
	}

	execs := state.execsSnapshot()
	if len(execs) != 1 || !strings.Contains(execs[0].query, "update profile_catalog_heads") {
		t.Fatalf("stale full replacement writes = %#v", execs)
	}
	for _, call := range execs {
		if strings.Contains(call.query, "insert into profile_catalogs") || strings.Contains(call.query, "insert into profile_catalog_versions") {
			t.Fatalf("stale full replacement overwrote revision 2 or created revision 3: %#v", call)
		}
	}
}

type profileCatalogDialectExpectation struct {
	dialect                sqlstore.Dialect
	reject                 string
	upsertFragments        []string
	requireGeneratedCounts bool
}

func exerciseStoreReplacesAndReadsProfileCatalogSnapshot(t *testing.T, tt profileCatalogDialectExpectation) {
	t.Helper()

	ctx := context.Background()
	db, state := openFakeSQLDB(t)
	defer db.Close()
	s := sqlstore.New(db, tt.dialect)
	indexedAt := time.Date(2026, 5, 19, 13, 0, 0, 0, time.UTC)
	catalog := sampleProfileCatalog(indexedAt)

	if err := s.ReplaceProfileCatalog(ctx, catalog); err != nil {
		t.Fatalf("replace profile catalog: %v", err)
	}
	exec := findProfileCatalogExec(t, state.execsSnapshot())
	assertSQLContains(t, exec.query, "profile catalog query", "insert into profile_catalogs", sqlValuesClause(tt.dialect, 13))
	assertSQLContains(t, exec.query, "profile catalog query", tt.upsertFragments...)
	assertSQLOmits(t, exec.query, "profile catalog query", tt.reject)
	if exec.args[0] != "profile.alpha" || exec.args[2] == "" {
		t.Fatalf("profile catalog args = %#v", exec.args)
	}
	if tt.requireGeneratedCounts && (exec.args[11] == 0 || exec.args[12] == 0) {
		t.Fatalf("profile catalog generated-count args = %#v", exec.args)
	}

	queueProfileCatalogIndexRow(state, indexedAt)
	index, err := s.GetProfileCatalogIndex(ctx)
	if err != nil {
		t.Fatalf("get profile catalog index: %v", err)
	}
	if index.ProfileID != "profile.alpha" || index.Counts.Services != 1 || index.Counts.Templates != 2 || index.Counts.TemplateConfigs != 1 {
		t.Fatalf("profile catalog index = %#v", index)
	}
	query := state.lastQuery(t)
	assertSQLContains(t, query.query, "profile catalog index query", "from profile_catalogs")

	state.queueRows(fakeRows{
		columns: []string{"catalog_json"},
		values:  [][]driver.Value{{exec.args[2]}},
	})
	loaded, err := s.GetProfileCatalog(ctx)
	if err != nil {
		t.Fatalf("get profile catalog: %v", err)
	}
	if loaded.ProfileID != "profile.alpha" || !loaded.IndexedAt.Equal(indexedAt) {
		t.Fatalf("loaded profile catalog identity = %#v", loaded)
	}
	if len(loaded.Services) != 1 || loaded.Services[0].SourcePath != "/tmp/source/service.alpha" {
		t.Fatalf("loaded profile catalog services = %#v", loaded.Services)
	}
	if len(loaded.APICases) != 1 || loaded.APICases[0].CasePath != "cases/case.alpha.json" {
		t.Fatalf("loaded profile catalog cases = %#v", loaded.APICases)
	}
	query = state.lastQuery(t)
	assertSQLContains(t, query.query, "profile catalog get query", "select catalog_json", "from profile_catalogs")
}

func findProfileCatalogExec(t *testing.T, calls []fakeSQLCall) fakeSQLCall {
	t.Helper()
	for _, call := range calls {
		if strings.Contains(call.query, "insert into profile_catalogs") {
			return call
		}
	}
	t.Fatalf("profile catalog exec not found in %#v", calls)
	return fakeSQLCall{}
}

func sampleProfileCatalog(indexedAt time.Time) store.ProfileCatalog {
	return store.ProfileCatalog{
		ProfileID: "profile.alpha",
		IndexedAt: indexedAt,
		Services: []store.CatalogService{
			{ID: "service.alpha", DisplayName: "Service Alpha", Kind: "http", SourcePath: "/tmp/source/service.alpha"},
		},
		Workflows: []store.CatalogWorkflow{
			{ID: "workflow.alpha", DisplayName: "Workflow Alpha"},
		},
		InterfaceNodes: []store.CatalogInterfaceNode{
			{ID: "node.alpha", DisplayName: "Node Alpha", ServiceID: "service.alpha"},
		},
		APICases: []store.CatalogAPICase{
			{ID: "case.alpha", DisplayName: "Case Alpha", NodeID: "node.alpha", CasePath: "cases/case.alpha.json"},
		},
		RequestTemplates: []store.CatalogRequestTemplate{
			{ID: "template.alpha", DisplayName: "Template Alpha", NodeID: "node.alpha", TemplateJSON: `{"method":"GET"}`},
		},
		WorkflowBindings: []store.CatalogWorkflowBinding{
			{WorkflowID: "workflow.alpha", StepID: "step.alpha", NodeID: "node.alpha", CaseID: "case.alpha", Required: true},
		},
		CaseDependencies: []store.CatalogCaseDependency{
			{ID: "dependency.alpha", CaseID: "case.alpha", FixtureID: "fixture.alpha", MappingsJSON: `[]`},
		},
		Fixtures: []store.CatalogFixture{
			{ID: "fixture.alpha", DisplayName: "Fixture Alpha", Kind: "json", DataJSON: `{}`},
		},
		TemplateConfigs: []store.CatalogTemplateConfig{
			{ID: "template-config.alpha", TemplateID: "template.alpha", ScopeType: "interface_node", ConfigJSON: `{}`},
		},
	}
}

func queueProfileCatalogIndexRow(state *fakeSQLState, indexedAt time.Time) {
	state.queueRows(fakeRows{
		columns: []string{"profile_id", "indexed_at", "services", "workflows", "interface_nodes", "api_cases", "request_templates", "workflow_bindings", "case_dependencies", "fixtures", "templates", "template_configs"},
		values: [][]driver.Value{{
			"profile.alpha", indexedAt.Format(time.RFC3339Nano), int64(1), int64(1), int64(1), int64(1), int64(1), int64(1), int64(1), int64(1), int64(2), int64(1),
		}},
	})
}
