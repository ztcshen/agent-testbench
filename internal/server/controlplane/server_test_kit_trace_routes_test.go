package controlplane_test

import (
	"context"
	"encoding/json"
	"fmt"
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
	store  *sqlite.Store
	target *httptest.Server
	server *httptest.Server
}

type traceTopologyGraphQLPayload struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

func TestServerCollectsTraceTopologyForSingleTestKitRun(t *testing.T) {
	ctx := context.Background()
	fixture := newTraceTopologyRouteFixture(t, ctx, singleSpanTraceResponse, true)
	result := postTraceTopologyTestKitRun(t, fixture.server.URL, fixture.target.URL)
	if result["ok"] != true {
		t.Fatalf("test kit run result = %#v", result)
	}
	run := requireSingleTestKitRun(t, ctx, fixture.store)
	requireSingleTraceTopologyCollected(t, ctx, fixture.store, run.ID)
}

func TestServerReturnsTraceTopologyForWorkflowStepTestKitRun(t *testing.T) {
	ctx := context.Background()
	fixture := newTraceTopologyRouteFixture(t, ctx, linkedSpanTraceResponse, false)
	result := postTraceTopologyTestKitRun(t, fixture.server.URL, fixture.target.URL)
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
	seedTraceTopologyCaseCatalog(t, ctx, s)
	server := httptest.NewServer(controlplane.NewWithOptions(profile.Bundle{ID: "sample"}, controlplane.Options{Runtime: s}))
	defer server.Close()

	postTraceTopologyTestKitRun(t, server.URL, target.URL)
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
	seedTraceTopologyCaseCatalog(t, ctx, s)
	server := httptest.NewServer(controlplane.NewWithOptions(profile.Bundle{ID: "sample"}, controlplane.Options{
		Runtime:         s,
		TraceGraphQLURL: provider.URL,
	}))
	t.Cleanup(server.Close)
	return traceTopologyRouteFixture{store: s, target: target, server: server}
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

func seedTraceTopologyCaseCatalog(t *testing.T, ctx context.Context, s *sqlite.Store) {
	t.Helper()

	if err := s.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID: "sample",
		IndexedAt: time.Now().UTC(),
		APICases: []store.CatalogAPICase{
			{ID: "case.alpha", DisplayName: "Case Alpha", NodeID: "node.alpha", Status: "active"},
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

func postTraceTopologyTestKitRun(t *testing.T, serverURL, targetURL string) map[string]any {
	t.Helper()

	var result map[string]any
	postJSONInto(t, serverURL+"/api/test-kit/run", fmt.Sprintf(`{
		"caseId":"case.alpha",
		"workflowId":"workflow.alpha",
		"stepId":"step.alpha",
		"baseUrl":%q,
		"timeoutSeconds":5
	}`, targetURL), http.StatusOK, &result)
	return result
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
