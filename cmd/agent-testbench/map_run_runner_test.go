package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"agent-testbench/internal/store"
)

func TestMapRunReportsUnsupportedNonHTTPRunner(t *testing.T) {
	ctx := context.Background()
	storeRef := seedExecutableMapCommandStoreWithMQCase(t, ctx)

	runCLI(t, "map", "import-workflows", "--store", storeRef, "--json")
	out := runCLIFails(t, "map", "run", "--store", storeRef, "--map", "map.profile.flow", "--case", "case.submit.success", "--json")
	report := decodeMapRunCommandReport(t, out)

	if report.OK || report.Status != store.StatusFailed || report.Summary.FailedTasks != 1 {
		t.Fatalf("unsupported MQ case should fail one task = %#v", report)
	}
	if len(report.Tasks) != 1 || !strings.Contains(report.Tasks[0].Reason, "unsupported map case runner") || !strings.Contains(report.Tasks[0].Reason, "executor.mq") {
		t.Fatalf("unsupported runner reason = %#v", report.Tasks)
	}
}

func seedExecutableMapCommandStoreWithMQCase(t *testing.T, ctx context.Context) string {
	t.Helper()
	submitSeen := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prepare":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"ok":true}`)
		case "/submit":
			submitSeen = true
			t.Fatalf("MQ case should not be executed through HTTP: %s", r.URL.Path)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(func() {
		server.Close()
		if submitSeen {
			t.Fatalf("MQ case reached HTTP server")
		}
	})
	storePath := filepath.Join(t.TempDir(), "map-run-mq.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	catalog := mapCommandExecutableProfileCatalogFixture(server.URL)
	for i := range catalog.APICases {
		if catalog.APICases[i].ID == "case.submit.success" {
			catalog.APICases[i].ExecutorID = "executor.mq"
			catalog.APICases[i].SourceKind = "mq"
		}
	}
	if err := runtime.ReplaceProfileCatalog(ctx, catalog); err != nil {
		t.Fatalf("seed mq profile catalog: %v", err)
	}
	closeCLIStore(runtime)
	return storeRef
}
