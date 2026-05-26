package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

func TestTraceTopologyCollectPersistsProviderSpanRefs(t *testing.T) {
	ctx := context.Background()
	s := openTraceCollectSQLiteStore(t, ctx)
	defer s.Close()

	startedAt := time.Date(2026, 5, 15, 8, 30, 0, 0, time.UTC)
	createTraceCollectRun(t, ctx, s, store.Run{
		ID:         "run.alpha",
		ProfileID:  "sample",
		WorkflowID: "workflow.alpha",
		Status:     store.StatusPassed,
		StartedAt:  startedAt,
		FinishedAt: startedAt.Add(3 * time.Second),
	})

	provider := newProviderSpanRefsTraceProvider(t)
	defer provider.Close()
	server := newTraceCollectRouteServer(s, provider.URL)
	defer server.Close()

	response := traceCollectRouteResponseForTest{}
	postTraceTopologyCollect(t, server.URL, map[string]any{
		"runId":     "run.alpha",
		"stepId":    "step.alpha",
		"caseId":    "case.alpha",
		"requestId": "request.alpha",
		"endpoint":  "/alpha",
		"startedAt": startedAt.Format(time.RFC3339Nano),
	}, http.StatusOK, &response)

	requireProviderSpanRefsResponse(t, response)
	requireStoredProviderSpanRefsTopology(t, ctx, s)
	requirePassedTraceCollectPostProcessTask(t, ctx, s)
}

func TestTraceTopologyCollectRecordsFailedPostProcessTaskOnProviderError(t *testing.T) {
	ctx := context.Background()
	s := openTraceCollectSQLiteStore(t, ctx)
	defer s.Close()

	createTraceCollectRun(t, ctx, s, store.Run{
		ID:         "run.provider-error",
		ProfileID:  "sample",
		WorkflowID: "workflow.alpha",
		Status:     store.StatusFailed,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	})

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"queryTrace":{"spans":[]}}}`))
	}))
	defer provider.Close()
	server := newTraceCollectRouteServer(s, provider.URL)
	defer server.Close()

	postTraceTopologyCollect(t, server.URL, map[string]any{
		"runId":     "run.provider-error",
		"stepId":    "step.alpha",
		"caseId":    "case.alpha",
		"requestId": "request.alpha",
		"traceId":   "trace.missing",
	}, http.StatusBadRequest, nil)

	tasks, err := s.ListPostProcessTasks(ctx, "run.provider-error")
	if err != nil {
		t.Fatalf("list post-process tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Kind != postProcessKindTraceTopology || tasks[0].Status != store.StatusFailed {
		t.Fatalf("post-process tasks = %#v", tasks)
	}
	if tasks[0].WorkflowID != "workflow.alpha" || tasks[0].StepID != "step.alpha" || tasks[0].CaseID != "case.alpha" || tasks[0].Error == "" {
		t.Fatalf("failed post-process task = %#v", tasks[0])
	}
}

func TestTraceTopologyCollectUsesExplicitTraceID(t *testing.T) {
	ctx := context.Background()
	s := openTraceCollectSQLiteStore(t, ctx)
	defer s.Close()

	createTraceCollectRun(t, ctx, s, store.Run{
		ID:         "run.direct",
		ProfileID:  "sample",
		WorkflowID: "workflow.alpha",
		Status:     store.StatusPassed,
	})

	provider := newExplicitTraceIDProvider(t, "trace.direct")
	defer provider.Close()
	server := newTraceCollectRouteServer(s, provider.URL)
	defer server.Close()

	postTraceTopologyCollect(t, server.URL, map[string]any{
		"runId":     "run.direct",
		"stepId":    "step.direct",
		"caseId":    "case.direct",
		"requestId": "request.direct",
		"traceId":   "trace.direct",
	}, http.StatusOK, nil)

	rows, err := s.ListTraceTopologies(ctx, "run.direct")
	if err != nil {
		t.Fatalf("list trace topologies: %v", err)
	}
	if len(rows) != 1 || rows[0].TraceID != "trace.direct" || rows[0].RequestID != "request.direct" || rows[0].Status != "complete" {
		t.Fatalf("stored direct topology = %#v", rows)
	}
}

type traceCollectRouteResponseForTest struct {
	TraceTopology traceCollectRowResponseForTest      `json:"traceTopology"`
	Topology      traceCollectTopologyResponseForTest `json:"topology"`
}

type traceCollectRowResponseForTest struct {
	WorkflowRunID string `json:"workflowRunId"`
	TraceID       string `json:"traceId"`
	Status        string `json:"status"`
}

type traceCollectTopologyResponseForTest struct {
	SpanCount      int                               `json:"spanCount"`
	ConfirmedEdges []traceCollectEdgeResponseForTest `json:"confirmedEdges"`
}

type traceCollectEdgeResponseForTest struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

func openTraceCollectSQLiteStore(t *testing.T, ctx context.Context) *sqlite.Store {
	t.Helper()
	s, err := sqlite.Open(ctx, sqlite.Config{Path: filepath.Join(t.TempDir(), "sandbox.sqlite")})
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	return s
}

func createTraceCollectRun(t *testing.T, ctx context.Context, s store.Store, run store.Run) {
	t.Helper()
	if _, err := s.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
}

func newTraceCollectRouteServer(runtime store.Store, providerURL string) *httptest.Server {
	return httptest.NewServer(NewWithOptions(profile.Bundle{ID: "sample"}, Options{
		Runtime:         runtime,
		TraceGraphQLURL: providerURL,
	}))
}

func newProviderSpanRefsTraceProvider(t *testing.T) *httptest.Server {
	return newTraceCollectProviderStub(t, traceCollectProviderStub{
		candidateResponse: `{"data":{"queryBasicTraces":{"traces":[{"endpointNames":["POST:/alpha"],"duration":120,"start":"2026-05-15 0830","isError":false,"traceIds":["trace.alpha"]}]}}}`,
		traceResponse:     `{"data":{"queryTrace":{"spans":[{"traceId":"trace.alpha","segmentId":"segment.entry","spanId":0,"parentSpanId":-1,"refs":[],"serviceCode":"service.entry","endpointName":"/alpha","type":"Entry","component":"Tomcat"},{"traceId":"trace.alpha","segmentId":"segment.worker","spanId":0,"parentSpanId":-1,"refs":[{"traceId":"trace.alpha","parentSegmentId":"segment.entry","parentSpanId":0,"type":"CrossProcess"}],"serviceCode":"service.worker","endpointName":"POST:/alpha","type":"Entry","component":"Server"}]}}}`,
	})
}

func newExplicitTraceIDProvider(t *testing.T, expectedTraceID string) *httptest.Server {
	return newTraceCollectProviderStub(t, traceCollectProviderStub{
		expectedTraceID: expectedTraceID,
		traceResponse:   `{"data":{"queryTrace":{"spans":[{"traceId":"trace.direct","segmentId":"segment.entry","spanId":0,"parentSpanId":-1,"refs":[],"serviceCode":"service.entry","endpointName":"/direct","type":"Entry","component":"Tomcat"},{"traceId":"trace.direct","segmentId":"segment.worker","spanId":0,"parentSpanId":-1,"refs":[{"traceId":"trace.direct","parentSegmentId":"segment.entry","parentSpanId":0,"type":"CrossProcess"}],"serviceCode":"service.worker","endpointName":"POST:/direct","type":"Entry","component":"Server"}]}}}`,
	})
}

type traceCollectProviderStub struct {
	candidateResponse string
	traceResponse     string
	expectedTraceID   string
}

func newTraceCollectProviderStub(t *testing.T, stub traceCollectProviderStub) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload := map[string]any{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		stub.write(t, w, valueString(payload["query"]), mapFromAny(payload["variables"]))
	}))
}

func (stub traceCollectProviderStub) write(t *testing.T, w http.ResponseWriter, query string, variables map[string]any) {
	t.Helper()

	if strings.Contains(query, "queryBasicTraces") {
		if stub.candidateResponse == "" {
			t.Fatalf("explicit trace id should not query candidates")
		}
		_, _ = w.Write([]byte(stub.candidateResponse))
		return
	}
	if strings.Contains(query, "queryTrace") {
		if stub.expectedTraceID != "" && variables["traceId"] != stub.expectedTraceID {
			t.Fatalf("trace id variable = %#v", variables)
		}
		_, _ = w.Write([]byte(stub.traceResponse))
		return
	}
	t.Fatalf("unexpected provider query: %s", query)
}

func postTraceTopologyCollect(t *testing.T, serverURL string, body map[string]any, wantStatus int, out any) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal collect request: %v", err)
	}
	resp, err := http.Post(serverURL+"/api/trace-topology/collect", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("collect topology: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		t.Fatalf("collect status = %d", resp.StatusCode)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode collect response: %v", err)
		}
	}
}

func requireProviderSpanRefsResponse(t *testing.T, payload traceCollectRouteResponseForTest) {
	t.Helper()
	if payload.TraceTopology.WorkflowRunID != "run.alpha" || payload.TraceTopology.TraceID != "trace.alpha" || payload.TraceTopology.Status != "complete" {
		t.Fatalf("trace topology row = %#v", payload.TraceTopology)
	}
	if payload.Topology.SpanCount != 2 || len(payload.Topology.ConfirmedEdges) != 1 {
		t.Fatalf("topology summary = %#v", payload.Topology)
	}
	edge := payload.Topology.ConfirmedEdges[0]
	if edge.Source != "service.entry" || edge.Target != "service.worker" {
		t.Fatalf("confirmed edge = %#v", edge)
	}
}

func requireStoredProviderSpanRefsTopology(t *testing.T, ctx context.Context, s store.Store) {
	t.Helper()
	rows, err := s.ListTraceTopologies(ctx, "run.alpha")
	if err != nil {
		t.Fatalf("list trace topologies: %v", err)
	}
	if len(rows) != 1 || rows[0].WorkflowID != "workflow.alpha" || rows[0].CaseID != "case.alpha" {
		t.Fatalf("stored topologies = %#v", rows)
	}
	if strings.TrimSpace(rows[0].ID) == "" {
		t.Fatalf("stored topology id should be generated when payload omits id: %#v", rows[0])
	}
}

func requirePassedTraceCollectPostProcessTask(t *testing.T, ctx context.Context, s store.Store) {
	t.Helper()
	tasks, err := s.ListPostProcessTasks(ctx, "run.alpha")
	if err != nil {
		t.Fatalf("list post-process tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Kind != postProcessKindTraceTopology || tasks[0].Status != store.StatusPassed {
		t.Fatalf("post-process tasks = %#v", tasks)
	}
	if tasks[0].WorkflowID != "workflow.alpha" || tasks[0].StepID != "step.alpha" || tasks[0].CaseID != "case.alpha" {
		t.Fatalf("post-process task context = %#v", tasks[0])
	}
	var summary map[string]any
	if err := json.Unmarshal([]byte(tasks[0].SummaryJSON), &summary); err != nil {
		t.Fatalf("decode post-process task summary: %v", err)
	}
	if summary["traceId"] != "trace.alpha" || summary["requestId"] != "request.alpha" || summary["topologyStatus"] != "complete" || summary["spanCount"] != float64(2) {
		t.Fatalf("post-process task summary = %#v", summary)
	}
}

func TestTraceCandidatesPreferRunTimeWindow(t *testing.T) {
	startedAt := time.Date(2026, 5, 18, 11, 1, 34, 793739000, time.UTC)
	finishedAt := time.Date(2026, 5, 18, 11, 1, 35, 223739000, time.UTC)
	candidates := []traceCandidate{
		{TraceID: "trace.too-early", Start: "1779102093735"},
		{TraceID: "trace.inside", Start: "1779102095116"},
	}

	sortTraceCandidatesByRunWindow(candidates, startedAt, finishedAt)

	if candidates[0].TraceID != "trace.inside" {
		t.Fatalf("first candidate = %#v", candidates[0])
	}
}

func TestTestKitTraceTopologyCollectPayloadUsesSandboxCallbackPath(t *testing.T) {
	payload, ok := testKitTraceTopologyCollectPayload("run.callback", map[string]any{
		"stepId": "callback",
	}, map[string]any{
		"ok":     true,
		"caseId": "case.callback",
		"result": map[string]any{
			"request": map[string]any{
				"path": "/__sandbox/llt/callback",
				"headers": map[string]any{
					"X-Sandbox-Callback-Path": "/sample-app/v1/llt/notice",
				},
			},
			"response": map[string]any{
				"headers": map[string]any{},
			},
		},
	})
	if !ok {
		t.Fatalf("collect payload was not built")
	}
	if payload["endpoint"] != "/sample-app/v1/llt/notice" {
		t.Fatalf("endpoint = %#v", payload["endpoint"])
	}
}

func TestTestKitTraceTopologyCollectPayloadUsesConfiguredTraceEndpoint(t *testing.T) {
	payload, ok := testKitTraceTopologyCollectPayload("run.gateway", map[string]any{
		"stepId":        "submit",
		"traceEndpoint": "POST:/api/v1/sample/orders/submit",
	}, map[string]any{
		"ok":     true,
		"caseId": "case.submit",
		"result": map[string]any{
			"request": map[string]any{
				"path": "/v1/orders/submit",
			},
			"response": map[string]any{},
		},
	})
	if !ok {
		t.Fatalf("collect payload was not built")
	}
	if payload["endpoint"] != "POST:/api/v1/sample/orders/submit" {
		t.Fatalf("endpoint = %#v", payload["endpoint"])
	}
}

func TestReadJSONPayloadPreservesLargeNumericOverrides(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/test-kit/run", strings.NewReader(`{
		"overrides": {
			"payout_id": 9161030727085880
		}
	}`))

	payload, err := readJSONPayload(request)
	if err != nil {
		t.Fatalf("read payload: %v", err)
	}
	overrides := mapFromAny(payload["overrides"])
	rendered := renderCaseString("{{override:payout_id}}", overrides)

	if rendered != "9161030727085880" {
		t.Fatalf("rendered payout_id = %q", rendered)
	}
}
