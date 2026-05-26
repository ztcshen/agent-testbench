package controlplane

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/store"
)

type runnableAPICase struct {
	Case        profile.APICase
	Execution   *caseExecutionConfig
	CaseBaseURL string
}

type caseExecutionConfig struct {
	Method                string         `json:"method"`
	NodeID                string         `json:"nodeId"`
	Path                  string         `json:"path"`
	Query                 map[string]any `json:"query"`
	Headers               map[string]any `json:"headers"`
	Auth                  map[string]any `json:"auth"`
	Body                  any            `json:"body"`
	ExpectedHTTPCodes     []int          `json:"expectedHttpCodes"`
	ExpectedResponse      []string       `json:"expectedResponseContains"`
	RequireRequestID      bool           `json:"requireRequestId"`
	Signed                bool           `json:"signed"`
	TraceEndpoint         string         `json:"traceEndpoint"`
	TraceCorrelatorFields []string       `json:"traceCorrelatorFields"`
}

type caseExecutionTemplateConfig struct {
	CaseID        string              `json:"caseId"`
	CaseExecution caseExecutionConfig `json:"caseExecution"`
	Exports       []map[string]any    `json:"exports"`
}

var caseSerialCounter uint64

func handleTestKitRun(w http.ResponseWriter, r *http.Request, bundle profile.Bundle, runtime store.Store, collector traceCollector) {
	payload, err := readJSONPayload(r)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid json"})
		return
	}
	result, status := testKitCaseResult(r.Context(), bundle, runtime, payload)
	if status == http.StatusOK {
		runID, err := recordTestKitRun(r, bundle, runtime, payload, result)
		if err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		attachCaseRunEvidenceHandles(result, runID)
		if runID != "" {
			if shouldInlineTestKitTraceTopology(payload) {
				collectAndRecordTestKitTraceTopology(r.Context(), runtime, collector, runID, payload, result)
			} else {
				scheduleTestKitTraceTopology(runtime, collector, runID, payload, result)
			}
		}
	}
	writeJSONStatus(w, status, result)
}

func handleTestKitRunBatch(w http.ResponseWriter, r *http.Request, bundle profile.Bundle, runtime store.Store) {
	payload, err := readJSONPayload(r)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid json"})
		return
	}
	caseIDs := testKitCaseIDs(payload["caseIds"])
	if len(caseIDs) == 0 {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "caseIds are required"})
		return
	}

	results := make([]map[string]any, 0, len(caseIDs))
	passed := 0
	started := time.Now()
	for _, caseID := range caseIDs {
		itemPayload := map[string]any{
			"caseId":         caseID,
			"baseUrl":        payload["baseUrl"],
			"timeoutSeconds": payload["timeoutSeconds"],
		}
		result, _ := testKitCaseResult(r.Context(), bundle, runtime, itemPayload)
		runID, err := recordTestKitRunWithContext(r.Context(), bundle, runtime, itemPayload, result)
		if err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		attachCaseRunEvidenceHandles(result, runID)
		if result["ok"] == true {
			passed++
		}
		results = append(results, result)
	}
	writeJSON(w, map[string]any{
		"ok":        passed == len(results),
		"results":   results,
		"elapsedMs": time.Since(started).Milliseconds(),
		"summary": map[string]any{
			"caseCount": len(results),
			"passed":    passed,
			"failed":    len(results) - passed,
		},
	})
}

func attachCaseRunEvidenceHandles(result map[string]any, runID string) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return
	}
	caseRunID := runID + ".case"
	result["runId"] = runID
	result["caseRunId"] = caseRunID
	result["detailUrl"] = "/api/case-run/evidence?caseRunId=" + url.QueryEscape(caseRunID)
	result["viewerUrl"] = "/evidence-viewer.html?caseRun=" + url.QueryEscape(runID)
}

func testKitCaseResult(ctx context.Context, bundle profile.Bundle, runtime store.Store, payload map[string]any) (map[string]any, int) {
	started := time.Now()
	caseID := valueString(payload["caseId"])
	if caseID == "" {
		return map[string]any{"ok": false, "error": "caseId is required", "code": http.StatusBadRequest}, http.StatusBadRequest
	}
	item, ok := findRunnableAPICase(ctx, bundle, runtime, caseID, payload)
	if !ok {
		return map[string]any{
			"ok":     false,
			"caseId": caseID,
			"status": store.StatusFailed,
			"error":  "api case not found",
			"code":   http.StatusNotFound,
		}, http.StatusNotFound
	}

	executionResult := executeTestKitCase(ctx, bundle, runtime, item, payload)
	runOK := executionResult.ok
	status := store.StatusPassed
	if !runOK {
		status = store.StatusFailed
	}
	stepID := valueString(payload["stepId"])
	result := map[string]any{
		"ok":        runOK,
		"caseId":    item.Case.ID,
		"title":     firstNonEmpty(item.Case.DisplayName, item.Case.ID),
		"stepId":    stepID,
		"status":    status,
		"elapsedMs": time.Since(started).Milliseconds(),
		"summary": map[string]any{
			"caseId":        item.Case.ID,
			"stepId":        stepID,
			"failureReason": executionResult.failureReason,
			"httpCode":      executionResult.httpCode,
			"targetBaseUrl": executionResult.baseURL,
		},
		"result": executionResult.result,
	}
	if executionResult.failureReason != "" {
		result["error"] = executionResult.failureReason
	}
	return result, http.StatusOK
}
