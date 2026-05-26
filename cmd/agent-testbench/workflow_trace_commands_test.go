package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

func TestTraceTopologyCollectCommandPersistsTopology(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	startedAt := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	if _, err := s.CreateRun(ctx, store.Run{
		ID:         "run.trace",
		ProfileID:  "sample",
		WorkflowID: "workflow.alpha",
		Status:     store.StatusPassed,
		StartedAt:  startedAt,
		FinishedAt: startedAt.Add(3 * time.Second),
		CreatedAt:  startedAt,
		UpdatedAt:  startedAt.Add(3 * time.Second),
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(payload.Query, "queryBasicTraces"):
			_, _ = w.Write([]byte(`{"data":{"queryBasicTraces":{"traces":[{"endpointNames":["POST:/alpha"],"duration":120,"start":"2026-05-18 1000","isError":false,"traceIds":["trace.alpha"]}]}}}`))
		case strings.Contains(payload.Query, "queryTrace"):
			_, _ = w.Write([]byte(`{"data":{"queryTrace":{"spans":[{"traceId":"trace.alpha","segmentId":"segment.entry","spanId":0,"parentSpanId":-1,"refs":[],"serviceCode":"service.entry","endpointName":"/alpha","type":"Entry","component":"Tomcat"},{"traceId":"trace.alpha","segmentId":"segment.worker","spanId":0,"parentSpanId":-1,"refs":[{"traceId":"trace.alpha","parentSegmentId":"segment.entry","parentSpanId":0,"type":"CrossProcess"}],"serviceCode":"service.worker","endpointName":"POST:/alpha","type":"Entry","component":"Server"}]}}}`))
		default:
			t.Fatalf("unexpected provider query: %s", payload.Query)
		}
	}))
	defer provider.Close()

	out := runCLI(t, "trace", "topology", "collect",
		"--store", "sqlite://"+storePath,
		"--trace-graphql-url", provider.URL,
		"--run", "run.trace",
		"--step", "step.alpha",
		"--case", "case.alpha",
		"--request", "request.alpha",
		"--endpoint", "/alpha",
		"--started-at", startedAt.Format(time.RFC3339Nano),
		"--json",
	)

	var payload struct {
		OK            bool `json:"ok"`
		TraceTopology struct {
			WorkflowRunID string `json:"workflowRunId"`
			TraceID       string `json:"traceId"`
			Status        string `json:"status"`
		} `json:"traceTopology"`
		Topology struct {
			SpanCount      int `json:"spanCount"`
			ConfirmedEdges []struct {
				Source string `json:"source"`
				Target string `json:"target"`
			} `json:"confirmedEdges"`
		} `json:"topology"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode trace topology collect json: %v\n%s", err, out)
	}
	if !payload.OK || payload.TraceTopology.WorkflowRunID != "run.trace" || payload.TraceTopology.TraceID != "trace.alpha" || payload.TraceTopology.Status != "complete" {
		t.Fatalf("trace topology collect payload = %#v", payload)
	}
	if payload.Topology.SpanCount != 2 || len(payload.Topology.ConfirmedEdges) != 1 || payload.Topology.ConfirmedEdges[0].Source != "service.entry" || payload.Topology.ConfirmedEdges[0].Target != "service.worker" {
		t.Fatalf("trace topology = %#v", payload.Topology)
	}
	s, err = sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer s.Close()
	rows, err := s.ListTraceTopologies(ctx, "run.trace")
	if err != nil {
		t.Fatalf("list trace topologies: %v", err)
	}
	if len(rows) != 1 || rows[0].StepID != "step.alpha" || rows[0].CaseID != "case.alpha" || rows[0].RequestID != "request.alpha" {
		t.Fatalf("stored topologies = %#v", rows)
	}
}

func TestReplayEvidenceCommandEmitsShellPayload(t *testing.T) {
	out := runCLI(t, "replay", "evidence", "--trace-id", "TRACE-1", "--json")

	var payload struct {
		OK  bool `json:"ok"`
		Run struct {
			TraceID string `json:"traceId"`
		} `json:"run"`
		Evidence struct {
			TraceID string `json:"traceId"`
			Systems []any  `json:"systems"`
		} `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode replay evidence json: %v\n%s", err, out)
	}
	if !payload.OK || payload.Run.TraceID != "TRACE-1" || payload.Evidence.TraceID != "TRACE-1" || len(payload.Evidence.Systems) != 0 {
		t.Fatalf("replay evidence payload = %#v", payload)
	}
}
