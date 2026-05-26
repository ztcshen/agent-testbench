package controlplane

import (
	"context"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

func shouldInlineTestKitTraceTopology(payload map[string]any) bool {
	return strings.TrimSpace(valueString(payload["workflowId"])) != "" && strings.TrimSpace(valueString(payload["stepId"])) != ""
}

func collectAndRecordTestKitTraceTopology(ctx context.Context, runtime store.Store, collector traceCollector, runID string, payload map[string]any, result map[string]any) {
	collectPayload, ok := testKitTraceTopologyCollectPayload(runID, payload, result)
	if !ok || runtime == nil {
		return
	}
	if strings.TrimSpace(collector.GraphQLURL) == "" {
		recordSkippedTestKitTraceTopologyTask(runtime, runID, payload, collectPayload, "TraceGraphQLURL is not configured; trace topology collection skipped")
		return
	}
	started := time.Now().UTC()
	status := store.StatusPassed
	errText := ""
	summary := map[string]any{}
	defer func() {
		recordTestKitTraceTopologyTask(runtime, runID, payload, collectPayload, started, time.Now().UTC(), status, errText, summary)
	}()
	collectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	row, topology, err := collectTraceTopologyWithRetry(collectCtx, runtime, collector, collectPayload)
	if err != nil {
		status = store.StatusFailed
		errText = err.Error()
		result["traceTopologyError"] = err.Error()
		return
	}
	result["traceTopology"] = topology
	result["traceTopologyRow"] = traceTopologyPayload(row)
	summary["traceId"] = row.TraceID
	summary["requestId"] = row.RequestID
	summary["topologyStatus"] = topology.Status
	summary["spanCount"] = topology.SpanCount
}

func scheduleTestKitTraceTopology(runtime store.Store, collector traceCollector, runID string, payload map[string]any, result map[string]any) {
	collectPayload, ok := testKitTraceTopologyCollectPayload(runID, payload, result)
	if !ok || runtime == nil {
		return
	}
	if strings.TrimSpace(collector.GraphQLURL) == "" {
		recordSkippedTestKitTraceTopologyTask(runtime, runID, payload, collectPayload, "TraceGraphQLURL is not configured; trace topology collection skipped")
		return
	}
	go func() {
		started := time.Now().UTC()
		status := store.StatusPassed
		errText := ""
		summary := map[string]any{}
		defer func() {
			recordTestKitTraceTopologyTask(runtime, runID, payload, collectPayload, started, time.Now().UTC(), status, errText, summary)
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		row, topology, err := collectTraceTopologyWithRetry(ctx, runtime, collector, collectPayload)
		if err != nil {
			status = store.StatusFailed
			errText = err.Error()
			return
		}
		summary["traceId"] = row.TraceID
		summary["requestId"] = row.RequestID
		summary["topologyStatus"] = topology.Status
		summary["spanCount"] = topology.SpanCount
	}()
}

func recordSkippedTestKitTraceTopologyTask(runtime store.Store, runID string, payload map[string]any, collectPayload map[string]any, errText string) {
	now := time.Now().UTC()
	recordTestKitTraceTopologyTask(runtime, runID, payload, collectPayload, now, now, store.StatusSkipped, errText, map[string]any{"reason": "trace_provider_missing"})
}

func recordTestKitTraceTopologyTask(runtime store.Store, runID string, payload map[string]any, collectPayload map[string]any, started time.Time, finished time.Time, status string, errText string, summary map[string]any) {
	recordPostProcessTask(context.Background(), runtime, store.PostProcessTask{
		ID:          runID + "." + safeRuntimeLogPathSegment(valueString(collectPayload["stepId"])) + "." + postProcessKindTraceTopology,
		RunID:       runID,
		WorkflowID:  valueString(payload["workflowId"]),
		StepID:      valueString(collectPayload["stepId"]),
		CaseID:      valueString(collectPayload["caseId"]),
		Kind:        postProcessKindTraceTopology,
		Status:      status,
		StartedAt:   started,
		FinishedAt:  finished,
		DurationMs:  finished.Sub(started).Milliseconds(),
		Error:       errText,
		SummaryJSON: compactJSON(summary),
		CreatedAt:   finished,
	})
}

func testKitTraceTopologyCollectPayload(runID string, payload map[string]any, result map[string]any) (map[string]any, bool) {
	if runID == "" || result["ok"] != true {
		return nil, false
	}
	request := mapFromAny(mapFromAny(result["result"])["request"])
	response := mapFromAny(mapFromAny(result["result"])["response"])
	headers := request["headers"]
	endpoint := firstNonEmpty(
		valueString(payload["traceEndpoint"]),
		valueString(headerValue(headers, "X-Sandbox-Trace-Endpoint")),
		valueString(headerValue(headers, "X-Sandbox-Callback-Path")),
		valueString(request["path"]),
		valueString(request["fullUrl"]),
	)
	if endpoint == "" {
		return nil, false
	}
	return map[string]any{
		"runId":     runID,
		"caseId":    result["caseId"],
		"stepId":    firstNonEmpty(valueString(payload["stepId"]), valueString(result["stepId"])),
		"requestId": responseRequestID(response),
		"endpoint":  endpoint,
	}, true
}

func collectTraceTopologyWithRetry(ctx context.Context, runtime store.Store, collector traceCollector, payload map[string]any) (store.TraceTopology, traceTopology, error) {
	var lastErr error
	attempt := 0
	for {
		row, topology, err := collectTraceTopology(ctx, runtime, collector, payload)
		if err == nil {
			return row, topology, nil
		}
		lastErr = err
		if !retryableTraceCollectError(err) {
			break
		}
		attempt++
		if attempt >= 15 {
			break
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return store.TraceTopology{}, traceTopology{}, ctx.Err()
		case <-timer.C:
		}
	}
	return store.TraceTopology{}, traceTopology{}, lastErr
}

func retryableTraceCollectError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not indexed yet") || strings.Contains(message, "no queryable traces")
}

func responseRequestID(response map[string]any) string {
	for _, key := range []string{"Request-Id", "Request-ID", "request-id", "X-Request-Id", "X-Request-ID"} {
		if value := strings.TrimSpace(valueString(headerValue(response["headers"], key))); value != "" {
			return value
		}
	}
	return ""
}

func headerValue(headers any, key string) any {
	switch typed := headers.(type) {
	case map[string]any:
		return typed[key]
	case map[string]string:
		if value, ok := typed[key]; ok {
			return value
		}
		for itemKey, value := range typed {
			if strings.EqualFold(itemKey, key) {
				return value
			}
		}
		return nil
	default:
		return nil
	}
}

func testKitRequestSummary(result map[string]any, stepID string, caseID string) map[string]any {
	request := mapFromAny(mapFromAny(result["result"])["request"])
	if len(request) == 0 {
		request = map[string]any{}
	}
	request["caseId"] = caseID
	request["stepId"] = stepID
	return request
}
