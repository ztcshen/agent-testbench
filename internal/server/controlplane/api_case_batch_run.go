package controlplane

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/store"
)

func handleAPICaseBatchRunStart(w http.ResponseWriter, r *http.Request, bundle profile.Bundle, runtime store.Store, runner *apiCaseBatchRunner, collector traceCollector) {
	payload, ok := readPublicAPICaseBatchRunPayload(w, r)
	if !ok {
		return
	}
	request := apiCaseBatchRunRequest{
		RequestID:  strings.TrimSpace(valueString(payload["requestId"])),
		CaseIDs:    stringListValue(payload["caseIds"]),
		NodeIDs:    stringListValue(payload["nodeIds"]),
		WorkflowID: strings.TrimSpace(valueString(payload["workflowId"])),
		Suite:      apiCaseBatchSuiteSelectorValue(payload["suite"]),
	}
	applyAPICaseBatchRunOptionsFromPayload(&request, payload)
	report, status, err := startAPICaseBatchRun(r.Context(), bundle, runtime, runner, request, collector)
	if writeAPICaseBatchStartError(w, status, err, nil) {
		return
	}
	writeJSONStatus(w, http.StatusAccepted, report)
}

func applyAPICaseBatchRunOptionsFromPayload(request *apiCaseBatchRunRequest, payload map[string]any) {
	request.TimeoutSeconds = intValue(payload[apiFieldTimeoutSeconds])
	request.Overrides = mapValue(payload["overrides"])
}

func startAPICaseBatchRun(ctx context.Context, bundle profile.Bundle, runtime store.Store, runner *apiCaseBatchRunner, request apiCaseBatchRunRequest, collector traceCollector) (apiCaseBatchRunReport, int, error) {
	request, plans, status, err := prepareAPICaseBatchRun(ctx, bundle, runtime, request)
	if err != nil {
		return apiCaseBatchRunReport{}, status, err
	}
	now := time.Now().UTC()
	report := newAPICaseBatchRunReport(bundle, runner, request, plans, now)
	if err := persistInitialAPICaseBatchRun(ctx, runtime, report); err != nil {
		return apiCaseBatchRunReport{}, http.StatusInternalServerError, err
	}
	runner.save(report)
	launchAPICaseBatchRun(runner, runtime, bundle, request, plans, report.BatchRunID, collector)
	return report, http.StatusAccepted, nil
}

func prepareAPICaseBatchRun(ctx context.Context, bundle profile.Bundle, runtime store.Store, request apiCaseBatchRunRequest) (apiCaseBatchRunRequest, []apiCaseBatchCasePlan, int, error) {
	if request.RequestID == "" {
		return request, nil, http.StatusBadRequest, errors.New("requestId is required")
	}
	request.CaseIDs = compactUniqueStringListPreserveOrder(request.CaseIDs)
	request.NodeIDs = compactUniqueStringList(request.NodeIDs)
	request.Suite = normalizeAPICaseBatchSuiteSelector(request.Suite)
	if apiCaseBatchSelectorFamilyCount(request) != 1 {
		return request, nil, http.StatusBadRequest, publicAPICaseBatchPayloadError{
			code:    "invalid_batch_selector",
			message: "exactly one of caseIds, nodeIds, workflowId, or suite is required",
		}
	}
	if status, err := validateAPICaseBatchEnvironmentWorkflowGate(ctx, runtime, request); err != nil {
		return request, nil, status, err
	}
	plans, err := apiCaseBatchPlans(ctx, bundle, runtime, request)
	if err != nil {
		var planErr apiCaseBatchPlanError
		if errors.As(err, &planErr) {
			return request, nil, planErr.Status, planErr
		}
		return request, nil, http.StatusInternalServerError, err
	}
	if len(plans) == 0 {
		return request, nil, http.StatusBadRequest, errors.New("no api cases matched selector")
	}
	if err := normalizeAPICaseBatchPlanTimeouts(plans); err != nil {
		return request, nil, http.StatusBadRequest, err
	}
	return request, plans, 0, nil
}

func newAPICaseBatchRunReport(bundle profile.Bundle, runner *apiCaseBatchRunner, request apiCaseBatchRunRequest, plans []apiCaseBatchCasePlan, now time.Time) apiCaseBatchRunReport {
	batchRunID := newAPICaseBatchRunID(request.RequestID)
	reportDir := filepath.Join(apiCaseBatchReportDir(request, plans), batchRunID)
	reportURL := "/api/cases/batch-runs/" + url.PathEscape(batchRunID)
	report := apiCaseBatchRunReport{
		OK:                   true,
		BatchRunID:           batchRunID,
		RequestID:            request.RequestID,
		EnvironmentID:        request.EnvironmentID,
		ProfileID:            bundle.ID,
		CaseIDs:              request.CaseIDs,
		NodeIDs:              request.NodeIDs,
		WorkflowID:           request.WorkflowID,
		Status:               store.StatusRunning,
		Total:                len(plans),
		ReportURL:            reportURL,
		StartedAt:            now.Format(time.RFC3339Nano),
		Nodes:                apiCaseBatchNodesFromPlans(plans),
		Cases:                apiCaseBatchCaseReportsFromPlans(plans),
		HTMLReportPath:       filepath.Join(reportDir, "report.html"),
		HTMLReportURL:        reportURL + "/report.html",
		JUnitReportPath:      filepath.Join(reportDir, "report.junit.xml"),
		JUnitReportURL:       reportURL + "/report.junit.xml",
		ArtifactManifestPath: filepath.Join(reportDir, "artifacts.json"),
		ArtifactManifestURL:  reportURL + "/artifacts.json",
		FailureSummaryPath:   filepath.Join(reportDir, "failures.json"),
		FailureSummaryURL:    reportURL + "/failures.json",
	}
	report.lease = runner.newLease(now)
	if request.Suite.configured() {
		suite := request.Suite
		report.Suite = &suite
	}
	return report
}

func apiCaseBatchCaseReportsFromPlans(plans []apiCaseBatchCasePlan) []apiCaseBatchCaseReport {
	cases := make([]apiCaseBatchCaseReport, 0, len(plans))
	for _, plan := range plans {
		cases = append(cases, apiCaseBatchCaseReport{
			CaseID:          plan.ID,
			DisplayName:     plan.DisplayName,
			Scenario:        plan.Scenario,
			NodeID:          plan.NodeID,
			NodeDisplayName: plan.NodeDisplayName,
			Operation:       plan.Operation,
			Method:          plan.Method,
			Path:            plan.Path,
			StepID:          plan.StepID,
			Status:          store.StatusRunning,
		})
	}
	return cases
}

func persistInitialAPICaseBatchRun(ctx context.Context, runtime store.Store, report apiCaseBatchRunReport) error {
	if err := writeAPICaseBatchHTMLReport(report); err != nil {
		return err
	}
	if err := writeAPICaseBatchJUnitReport(report); err != nil {
		return err
	}
	if err := writeAPICaseBatchArtifactManifest(report); err != nil {
		return err
	}
	if err := writeAPICaseBatchFailureSummary(report); err != nil {
		return err
	}
	return createAPICaseBatchRunParent(ctx, runtime, report)
}

func launchAPICaseBatchRun(runner *apiCaseBatchRunner, runtime store.Store, bundle profile.Bundle, request apiCaseBatchRunRequest, plans []apiCaseBatchCasePlan, batchRunID string, collector traceCollector) {
	runCtx, cancel := context.WithCancel(context.Background())
	runner.startHeartbeat(runCtx, cancel, runtime, batchRunID)
	go func() {
		defer cancel()
		runner.run(runCtx, batchRunID, bundle, request.EnvironmentID, request.WorkflowID, plans, runtime, bundle.FailureCategories, collector)
	}()
}

func apiCaseBatchSelectorFamilyCount(request apiCaseBatchRunRequest) int {
	count := 0
	if len(request.CaseIDs) > 0 {
		count++
	}
	if len(request.NodeIDs) > 0 {
		count++
	}
	if strings.TrimSpace(request.WorkflowID) != "" {
		count++
	}
	if request.Suite.configured() {
		count++
	}
	return count
}

func validateAPICaseBatchEnvironmentWorkflowGate(ctx context.Context, runtime store.Store, request apiCaseBatchRunRequest) (int, error) {
	workflowID := strings.TrimSpace(request.WorkflowID)
	if workflowID == "" || runtime == nil {
		return 0, nil
	}
	environments, err := runtime.ListEnvironments(ctx)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	ids := []string{}
	matchedEnvironment := false
	for _, env := range environments {
		if strings.TrimSpace(env.VerificationWorkflowID) != workflowID {
			continue
		}
		envID := strings.TrimSpace(env.ID)
		ids = append(ids, envID)
		if request.EnvironmentAcceptance && envID == strings.TrimSpace(request.EnvironmentID) {
			matchedEnvironment = true
		}
	}
	if len(ids) == 0 || matchedEnvironment {
		return 0, nil
	}
	return http.StatusConflict, fmt.Errorf("workflow %s is bound to environment %s; run it through environment acceptance after restore instead of the generic batch API: POST /api/environments/%s/acceptance-runs or agent-testbench environment restore %s --store STORE_NAME_OR_DSN --workspace WORKSPACE --execute --run-workflow --server-url URL", workflowID, strings.Join(ids, ", "), url.PathEscape(ids[0]), ids[0])
}

func handleAPICaseBatchRunReport(w http.ResponseWriter, r *http.Request, runtime store.Store, runner *apiCaseBatchRunner) {
	idValue := strings.TrimPrefix(r.URL.Path, "/api/cases/batch-runs/")
	wantsHTML := strings.HasSuffix(idValue, "/report.html")
	wantsJUnit := strings.HasSuffix(idValue, "/report.junit.xml")
	wantsArtifacts := strings.HasSuffix(idValue, "/artifacts.json")
	wantsFailures := strings.HasSuffix(idValue, "/failures.json")
	if wantsHTML {
		idValue = strings.TrimSuffix(idValue, "/report.html")
	}
	if wantsJUnit {
		idValue = strings.TrimSuffix(idValue, "/report.junit.xml")
	}
	if wantsArtifacts {
		idValue = strings.TrimSuffix(idValue, "/artifacts.json")
	}
	if wantsFailures {
		idValue = strings.TrimSuffix(idValue, "/failures.json")
	}
	id, err := url.PathUnescape(idValue)
	if err != nil || strings.TrimSpace(id) == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "batchRunId is required"})
		return
	}
	report, ok := runner.get(id)
	if !ok {
		var loadErr error
		report, ok, loadErr = storedAPICaseBatchRunReport(r.Context(), runtime, id)
		if loadErr != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": loadErr.Error()})
			return
		}
		if !ok {
			writeJSONStatus(w, http.StatusNotFound, map[string]any{"ok": false, "error": "batch run not found"})
			return
		}
	}
	if wantsHTML {
		http.ServeFile(w, r, report.HTMLReportPath)
		return
	}
	if wantsJUnit {
		http.ServeFile(w, r, report.JUnitReportPath)
		return
	}
	if wantsArtifacts {
		http.ServeFile(w, r, report.ArtifactManifestPath)
		return
	}
	if wantsFailures {
		http.ServeFile(w, r, report.FailureSummaryPath)
		return
	}
	writeJSON(w, report)
}
