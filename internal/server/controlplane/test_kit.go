package controlplane

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/store"
)

type runnableAPICase struct {
	Case        profile.APICase
	Execution   *caseExecutionConfig
	Inputs      []map[string]any
	CaseBaseURL string
}

type caseExecutionConfig struct {
	Method                 string         `json:"method"`
	NodeID                 string         `json:"nodeId"`
	Path                   string         `json:"path"`
	Query                  map[string]any `json:"query"`
	Headers                map[string]any `json:"headers"`
	Auth                   map[string]any `json:"auth"`
	Body                   any            `json:"body"`
	ExpectedHTTPCodes      []int          `json:"expectedHttpCodes"`
	ExpectedResponse       []string       `json:"expectedResponseContains"`
	ExpectedResponseAbsent []string       `json:"expectedResponseNotContains"`
	RequireRequestID       bool           `json:"requireRequestId"`
	Signed                 bool           `json:"signed"`
	TraceEndpoint          string         `json:"traceEndpoint"`
	TraceCorrelatorFields  []string       `json:"traceCorrelatorFields"`
}

type caseExecutionTemplateConfig struct {
	CaseID        string              `json:"caseId"`
	CaseExecution caseExecutionConfig `json:"caseExecution"`
	Exports       []map[string]any    `json:"exports"`
	Inputs        []map[string]any    `json:"inputs"`
}

var caseSerialCounter uint64

// TrustedTestKitRunRequest is the in-process execution contract used by the
// CLI and map runner. Fields that select a target, local Evidence directory,
// run identity, or planner metadata must never be populated from an HTTP
// request. Public handlers accept only the smaller Store-native contract.
type TrustedTestKitRunRequest struct {
	CaseID             string
	WorkflowID         string
	StepID             string
	Overrides          map[string]any
	TimeoutSeconds     int
	BaseURL            string
	EvidenceDir        string
	RunID              string
	EnvironmentID      string
	TestPlanMapID      string
	TestPlanPathID     string
	TestPlanNodeID     string
	TestPlanOperation  string
	PlannerSummary     map[string]any
	InlineTraceCollect bool
	TraceGraphQLURL    string
}

func (request TrustedTestKitRunRequest) payload() map[string]any {
	payload := map[string]any{
		"caseId":             strings.TrimSpace(request.CaseID),
		"workflowId":         strings.TrimSpace(request.WorkflowID),
		"stepId":             strings.TrimSpace(request.StepID),
		"baseUrl":            strings.TrimSpace(request.BaseURL),
		apiFieldEvidenceDir:  strings.TrimSpace(request.EvidenceDir),
		"runId":              strings.TrimSpace(request.RunID),
		"environmentId":      strings.TrimSpace(request.EnvironmentID),
		"testPlanMapId":      strings.TrimSpace(request.TestPlanMapID),
		"testPlanPathId":     strings.TrimSpace(request.TestPlanPathID),
		"testPlanNodeId":     strings.TrimSpace(request.TestPlanNodeID),
		"testPlanOperation":  strings.TrimSpace(request.TestPlanOperation),
		"inlineTraceCollect": request.InlineTraceCollect,
	}
	if len(request.Overrides) > 0 {
		payload["overrides"] = request.Overrides
	}
	if request.TimeoutSeconds != 0 {
		payload[apiFieldTimeoutSeconds] = request.TimeoutSeconds
	}
	if len(request.PlannerSummary) > 0 {
		payload["plannerSummary"] = request.PlannerSummary
	}
	return payload
}

// RunTrustedTestKitCase executes one Store catalog case for an in-process CLI
// caller. Callers must construct TrustedTestKitRunRequest from already trusted
// command state, never by decoding an external HTTP request into it.
func RunTrustedTestKitCase(ctx context.Context, bundle profile.Bundle, runtime store.Store, request TrustedTestKitRunRequest) (map[string]any, error) {
	if err := validateTrustedTestKitRunID(request.RunID); err != nil {
		return nil, err
	}
	collector := traceCollector{GraphQLURL: strings.TrimSpace(request.TraceGraphQLURL)}
	result, status, err := runTestKitCase(ctx, bundle, runtime, collector, request.payload())
	if result == nil {
		result = map[string]any{}
	}
	result["httpStatus"] = status
	return result, err
}

func validateTrustedTestKitRunID(raw string) error {
	runID := strings.TrimSpace(raw)
	if runID == "" {
		return nil
	}
	if runID == "." || runID == ".." {
		return fmt.Errorf("run ID must be a safe single path segment")
	}
	for _, char := range runID {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			continue
		}
		switch char {
		case '.', '-', '_':
			continue
		default:
			return fmt.Errorf("run ID must use only ASCII letters, digits, dot, dash, or underscore")
		}
	}
	return nil
}

func handleTestKitRun(w http.ResponseWriter, r *http.Request, bundle profile.Bundle, runtime store.Store, collector traceCollector) {
	payload, ok := readPublicTestKitRunPayload(w, r)
	if !ok {
		return
	}
	result, status, err := runTestKitCase(r.Context(), bundle, runtime, collector, payload)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSONStatus(w, status, result)
}

func runTestKitCase(ctx context.Context, bundle profile.Bundle, runtime store.Store, collector traceCollector, payload map[string]any) (map[string]any, int, error) {
	result, status := testKitCaseResult(ctx, bundle, runtime, payload)
	if status != http.StatusOK {
		return result, status, nil
	}
	runID, err := recordTestKitRunWithContext(ctx, bundle, runtime, payload, result)
	if err != nil {
		return result, http.StatusInternalServerError, err
	}
	attachCaseRunEvidenceHandles(result, runID)
	if runID == "" {
		return result, status, nil
	}
	if shouldInlineTestKitTraceTopology(payload) {
		collectAndRecordTestKitTraceTopology(ctx, runtime, collector, runID, payload, result)
	} else {
		scheduleTestKitTraceTopology(runtime, collector, runID, payload, result)
	}
	return result, status, nil
}

func validatePublicTestKitRunPayload(payload map[string]any) error {
	rejected := make([]string, 0)
	for _, field := range []string{
		"baseUrl",
		apiFieldEvidenceDir,
		"environmentId",
		"inlineTraceCollect",
		"plannerSummary",
		"plannerSummaryJson",
		"runId",
		"stepId",
		"testPlanMapId",
		"testPlanNodeId",
		"testPlanOperation",
		"testPlanPathId",
		"traceEndpoint",
		"workflowId",
	} {
		if _, ok := payload[field]; ok {
			rejected = append(rejected, field)
		}
	}
	if len(rejected) > 0 {
		sort.Strings(rejected)
		return fmt.Errorf("public test-kit execution cannot set trusted fields: %s; configure targets and Evidence paths in the Store catalog", strings.Join(rejected, ", "))
	}
	_, err := testKitTimeout(payload, 0)
	return err
}

func readPublicTestKitRunPayload(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	payload, err := readJSONPayload(r)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid json"})
		return nil, false
	}
	if err := validatePublicTestKitRunPayload(payload); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{
			"ok":         false,
			"error":      err.Error(),
			apiFieldCode: "trusted_execution_context_rejected",
		})
		return nil, false
	}
	return payload, true
}

func handleTestKitRunBatch(w http.ResponseWriter, r *http.Request, bundle profile.Bundle, runtime store.Store) {
	payload, ok := readPublicTestKitRunPayload(w, r)
	if !ok {
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
			"caseId":               caseID,
			"baseUrl":              payload["baseUrl"],
			apiFieldTimeoutSeconds: payload[apiFieldTimeoutSeconds],
			"overrides":            payload["overrides"],
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
		return map[string]any{"ok": false, "error": "caseId is required", apiFieldCode: http.StatusBadRequest}, http.StatusBadRequest
	}
	item, ok := findRunnableAPICase(ctx, bundle, runtime, caseID, payload)
	if !ok {
		return map[string]any{
			"ok":         false,
			"caseId":     caseID,
			"status":     store.StatusFailed,
			"error":      "api case not found",
			apiFieldCode: http.StatusNotFound,
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
		"ok":          runOK,
		"caseId":      item.Case.ID,
		apiFieldTitle: firstNonEmpty(item.Case.DisplayName, item.Case.ID),
		"stepId":      stepID,
		"status":      status,
		"elapsedMs":   time.Since(started).Milliseconds(),
		"summary": map[string]any{
			"caseId":        item.Case.ID,
			"stepId":        stepID,
			"failureReason": executionResult.failureReason,
			"httpCode":      executionResult.httpCode,
			"targetBaseUrl": executionResult.baseURL,
		},
		"result": executionResult.result,
	}
	if executionResult.runID != "" {
		result["runId"] = executionResult.runID
	}
	if executionResult.failureReason != "" {
		result["error"] = executionResult.failureReason
	}
	responseStatus := executionResult.httpStatus
	if responseStatus == 0 {
		responseStatus = http.StatusOK
	}
	if responseStatus != http.StatusOK {
		result[apiFieldCode] = responseStatus
	}
	return result, responseStatus
}
