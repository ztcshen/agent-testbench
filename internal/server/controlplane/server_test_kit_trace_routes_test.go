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

const (
	singleSpanTraceResponse = `{"data":{"queryTrace":{"spans":[{"traceId":"trace.alpha","segmentId":"segment.entry","spanId":0,"parentSpanId":-1,"refs":[],"serviceCode":"service.entry","endpointName":"/callback","type":"Entry","component":"Tomcat"}]}}}`
	linkedSpanTraceResponse = `{"data":{"queryTrace":{"spans":[{"traceId":"trace.alpha","segmentId":"segment.entry","spanId":0,"parentSpanId":-1,"refs":[],"serviceCode":"service.entry","endpointName":"/callback","type":"Entry","component":"Tomcat"},{"traceId":"trace.alpha","segmentId":"segment.worker","spanId":0,"parentSpanId":-1,"refs":[{"traceId":"trace.alpha","parentSegmentId":"segment.entry","parentSpanId":0,"type":"CrossProcess"}],"serviceCode":"service.worker","endpointName":"GET:/callback","type":"Entry","component":"Server"}]}}}`
)

type traceTopologyRouteFixture struct {
	store           *sqlite.Store
	traceGraphQLURL string
}

type traceTopologyGraphQLPayload struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

func TestServerCollectsTraceTopologyForSingleTestKitRun(t *testing.T) {
	ctx := context.Background()
	fixture := newTraceTopologyRouteFixture(t, ctx, singleSpanTraceResponse, true)
	result := runTrustedTraceTopologyTestKitRun(t, ctx, fixture.store, fixture.traceGraphQLURL)
	if result["ok"] != true {
		t.Fatalf("test kit run result = %#v", result)
	}
	run := requireSingleTestKitRun(t, ctx, fixture.store)
	requireSingleTraceTopologyCollected(t, ctx, fixture.store, run.ID)
}

func TestServerReturnsTraceTopologyForWorkflowStepTestKitRun(t *testing.T) {
	ctx := context.Background()
	fixture := newTraceTopologyRouteFixture(t, ctx, linkedSpanTraceResponse, false)
	result := runTrustedTraceTopologyTestKitRun(t, ctx, fixture.store, fixture.traceGraphQLURL)
	topology := result["traceTopology"].(map[string]any)
	if topology["provider"] != "skywalking" || topology["status"] != "complete" || topology["traceId"] != "trace.alpha" {
		t.Fatalf("trace topology should be returned inline: %#v", topology)
	}
	if edges := topology["confirmedEdges"].([]any); len(edges) != 1 {
		t.Fatalf("trace topology edges = %#v", edges)
	}
}

func TestServerRecordsSkippedTraceTopologyTaskWhenTraceProviderMissing(t *testing.T) {
	ctx := context.Background()
	target := newTraceTopologyTarget()
	defer target.Close()

	s := openTestKitSQLiteStore(t, ctx, "store.sqlite")
	seedTraceTopologyCaseCatalog(t, ctx, s, target.URL)

	runTrustedTraceTopologyTestKitRun(t, ctx, s, "")
	run := requireSingleTestKitRun(t, ctx, s)
	requireSkippedTraceTopologyTask(t, ctx, s, run.ID)
}

func newTraceTopologyRouteFixture(t *testing.T, ctx context.Context, queryTraceResponse string, assertTraceID bool) traceTopologyRouteFixture {
	t.Helper()

	target := newTraceTopologyTarget()
	t.Cleanup(target.Close)
	provider := newTraceTopologyProvider(t, queryTraceResponse, assertTraceID)
	t.Cleanup(provider.Close)
	s := openTestKitSQLiteStore(t, ctx, "sandbox.sqlite")
	seedTraceTopologyCaseCatalog(t, ctx, s, target.URL)
	return traceTopologyRouteFixture{store: s, traceGraphQLURL: provider.URL}
}

func newTraceTopologyTarget() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Request-Id", "request.alpha")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
}

func newTraceTopologyProvider(t *testing.T, queryTraceResponse string, assertTraceID bool) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload := decodeTraceTopologyGraphQLPayload(t, r)
		w.Header().Set("Content-Type", "application/json")
		writeTraceTopologyGraphQLResponse(t, w, payload, queryTraceResponse, assertTraceID)
	}))
}

func decodeTraceTopologyGraphQLPayload(t *testing.T, r *http.Request) traceTopologyGraphQLPayload {
	t.Helper()

	var payload traceTopologyGraphQLPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		t.Fatalf("decode provider request: %v", err)
	}
	return payload
}

func writeTraceTopologyGraphQLResponse(t *testing.T, w http.ResponseWriter, payload traceTopologyGraphQLPayload, queryTraceResponse string, assertTraceID bool) {
	t.Helper()

	switch {
	case strings.Contains(payload.Query, "queryBasicTraces"):
		_, _ = w.Write([]byte(`{"data":{"queryBasicTraces":{"traces":[{"endpointNames":["GET:/callback"],"duration":80,"start":"2026-05-15 0830","isError":false,"traceIds":["trace.alpha"]}]}}}`))
	case strings.Contains(payload.Query, "queryTrace"):
		if assertTraceID && payload.Variables["traceId"] != "trace.alpha" {
			t.Fatalf("trace id variable = %#v", payload.Variables)
		}
		_, _ = w.Write([]byte(queryTraceResponse))
	default:
		t.Fatalf("unexpected provider query: %s", payload.Query)
	}
}

func seedTraceTopologyCaseCatalog(t *testing.T, ctx context.Context, s *sqlite.Store, baseURL string) {
	t.Helper()

	if err := s.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID: "sample",
		IndexedAt: time.Now().UTC(),
		APICases: []store.CatalogAPICase{
			{ID: "case.alpha", DisplayName: "Case Alpha", NodeID: "node.alpha", Status: "active", BaseURL: baseURL},
		},
		TemplateConfigs: []store.CatalogTemplateConfig{
			{
				ID:         "cfg.case.alpha",
				TemplateID: "template.case.alpha",
				NodeID:     "node.alpha",
				WorkflowID: "workflow.alpha",
				ScopeType:  "step",
				ScopeID:    "step.alpha",
				Title:      "Case Alpha Runtime",
				Status:     "active",
				ConfigJSON: `{
					"caseId":"case.alpha",
					"caseExecution":{
						"method":"GET",
						"nodeId":"service.alpha",
						"path":"/callback",
						"expectedHttpCodes":[200]
					}
				}`,
			},
		},
	}); err != nil {
		t.Fatalf("replace profile catalog: %v", err)
	}
}

func runTrustedTraceTopologyTestKitRun(t *testing.T, ctx context.Context, runtime store.Store, traceGraphQLURL string) map[string]any {
	t.Helper()

	result, err := controlplane.RunTrustedTestKitCase(ctx, profile.Bundle{ID: "sample"}, runtime, controlplane.TrustedTestKitRunRequest{
		CaseID: "case.alpha", WorkflowID: "workflow.alpha", StepID: "step.alpha", TimeoutSeconds: 5,
		TraceGraphQLURL: traceGraphQLURL,
	})
	if err != nil {
		t.Fatalf("run trusted trace topology case: %v", err)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("encode trusted trace topology result: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode trusted trace topology result: %v", err)
	}
	return decoded
}

func requireSingleTestKitRun(t *testing.T, ctx context.Context, s *sqlite.Store) store.Run {
	t.Helper()

	runs, err := s.ListRuns(ctx)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs = %#v", runs)
	}
	return runs[0]
}

func requireSingleTraceTopologyCollected(t *testing.T, ctx context.Context, s *sqlite.Store, runID string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		topologies, err := s.ListTraceTopologies(ctx, runID)
		if err != nil {
			t.Fatalf("list trace topologies: %v", err)
		}
		if len(topologies) == 1 && topologies[0].CaseID == "case.alpha" && topologies[0].StepID == "step.alpha" && topologies[0].RequestID == "request.alpha" {
			requirePassedTraceTopologyTask(t, ctx, s, runID)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("stored trace topology was not collected asynchronously")
}

func requirePassedTraceTopologyTask(t *testing.T, ctx context.Context, s *sqlite.Store, runID string) {
	t.Helper()

	tasks, err := s.ListPostProcessTasks(ctx, runID)
	if err != nil {
		t.Fatalf("list post process tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Kind != "trace_topology_collect" || tasks[0].Status != store.StatusPassed || tasks[0].DurationMs < 0 {
		t.Fatalf("trace post process tasks = %#v", tasks)
	}
}

func requireSkippedTraceTopologyTask(t *testing.T, ctx context.Context, s *sqlite.Store, runID string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		tasks, err := s.ListPostProcessTasks(ctx, runID)
		if err != nil {
			t.Fatalf("list post process tasks: %v", err)
		}
		if len(tasks) == 1 {
			if tasks[0].Kind != "trace_topology_collect" || tasks[0].Status != store.StatusSkipped || tasks[0].StepID != "step.alpha" {
				t.Fatalf("trace task should record skipped collection: %#v", tasks)
			}
			if !strings.Contains(tasks[0].Error, "TraceGraphQLURL") {
				t.Fatalf("trace skipped task should explain missing provider config: %#v", tasks[0])
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("missing trace topology skipped task")
}
