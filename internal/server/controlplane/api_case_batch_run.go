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
	"agent-testbench/internal/domain/profilecatalog"
	"agent-testbench/internal/store"
)

func handleAPICaseBatchRunStart(w http.ResponseWriter, r *http.Request, bundle profile.Bundle, runtime store.Store, runner *apiCaseBatchRunner, collector traceCollector) {
	payload, err := readJSONPayload(r)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid json"})
		return
	}
	request := apiCaseBatchRunRequest{
		RequestID:     strings.TrimSpace(valueString(payload["requestId"])),
		EnvironmentID: strings.TrimSpace(valueString(payload["environmentId"])),
		CaseIDs:       stringListValue(payload["caseIds"]),
		NodeIDs:       stringListValue(payload["nodeIds"]),
		WorkflowID:    strings.TrimSpace(valueString(payload["workflowId"])),
		Suite:         apiCaseBatchSuiteSelectorValue(payload["suite"]),
	}
	applyAPICaseBatchRunOptionsFromPayload(&request, payload)
	report, status, err := startAPICaseBatchRun(r.Context(), bundle, runtime, runner, request, collector)
	if err != nil {
		writeJSONStatus(w, status, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSONStatus(w, http.StatusAccepted, report)
}

func applyAPICaseBatchRunOptionsFromPayload(request *apiCaseBatchRunRequest, payload map[string]any) {
	request.BaseURL = strings.TrimSpace(valueString(payload["baseUrl"]))
	request.EvidenceDir = strings.TrimSpace(valueString(payload["evidenceDir"]))
	request.TimeoutSeconds = intValue(payload["timeoutSeconds"])
	request.Overrides = mapValue(payload["overrides"])
}

func startAPICaseBatchRun(ctx context.Context, bundle profile.Bundle, runtime store.Store, runner *apiCaseBatchRunner, request apiCaseBatchRunRequest, collector traceCollector) (apiCaseBatchRunReport, int, error) {
	if request.RequestID == "" {
		return apiCaseBatchRunReport{}, http.StatusBadRequest, errors.New("requestId is required")
	}
	request.CaseIDs = compactUniqueStringListPreserveOrder(request.CaseIDs)
	request.NodeIDs = compactUniqueStringList(request.NodeIDs)
	request.Suite = normalizeAPICaseBatchSuiteSelector(request.Suite)
	if len(request.CaseIDs) == 0 && len(request.NodeIDs) == 0 && request.WorkflowID == "" && !request.Suite.configured() {
		return apiCaseBatchRunReport{}, http.StatusBadRequest, errors.New("caseIds, nodeIds, workflowId, or suite is required")
	}
	if status, err := validateAPICaseBatchEnvironmentWorkflowGate(ctx, runtime, request); err != nil {
		return apiCaseBatchRunReport{}, status, err
	}
	bundle = apiCaseBatchPlanningBundle(ctx, runtime, bundle)
	plans, err := apiCaseBatchPlans(ctx, bundle, runtime, request)
	if err != nil {
		var planErr apiCaseBatchPlanError
		if errors.As(err, &planErr) {
			return apiCaseBatchRunReport{}, planErr.Status, planErr
		}
		return apiCaseBatchRunReport{}, http.StatusInternalServerError, err
	}
	if len(plans) == 0 {
		return apiCaseBatchRunReport{}, http.StatusBadRequest, errors.New("no api cases matched selector")
	}

	batchRunID := newAPICaseBatchRunID(request.RequestID)
	now := time.Now().UTC()
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
		ReportURL:            "/api/cases/batch-runs/" + url.PathEscape(batchRunID),
		StartedAt:            now.Format(time.RFC3339Nano),
		Nodes:                apiCaseBatchNodesFromPlans(plans),
		Cases:                make([]apiCaseBatchCaseReport, 0, len(plans)),
		HTMLReportPath:       filepath.Join(apiCaseBatchReportDir(request, plans), batchRunID, "report.html"),
		HTMLReportURL:        "/api/cases/batch-runs/" + url.PathEscape(batchRunID) + "/report.html",
		JUnitReportPath:      filepath.Join(apiCaseBatchReportDir(request, plans), batchRunID, "report.junit.xml"),
		JUnitReportURL:       "/api/cases/batch-runs/" + url.PathEscape(batchRunID) + "/report.junit.xml",
		ArtifactManifestPath: filepath.Join(apiCaseBatchReportDir(request, plans), batchRunID, "artifacts.json"),
		ArtifactManifestURL:  "/api/cases/batch-runs/" + url.PathEscape(batchRunID) + "/artifacts.json",
		FailureSummaryPath:   filepath.Join(apiCaseBatchReportDir(request, plans), batchRunID, "failures.json"),
		FailureSummaryURL:    "/api/cases/batch-runs/" + url.PathEscape(batchRunID) + "/failures.json",
	}
	if request.Suite.configured() {
		suite := request.Suite
		report.Suite = &suite
	}
	for _, plan := range plans {
		report.Cases = append(report.Cases, apiCaseBatchCaseReport{
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
	if err := writeAPICaseBatchHTMLReport(report); err != nil {
		return apiCaseBatchRunReport{}, http.StatusInternalServerError, err
	}
	if err := writeAPICaseBatchJUnitReport(report); err != nil {
		return apiCaseBatchRunReport{}, http.StatusInternalServerError, err
	}
	if err := writeAPICaseBatchArtifactManifest(report); err != nil {
		return apiCaseBatchRunReport{}, http.StatusInternalServerError, err
	}
	if err := writeAPICaseBatchFailureSummary(report); err != nil {
		return apiCaseBatchRunReport{}, http.StatusInternalServerError, err
	}
	runner.save(report)

	go runner.run(context.Background(), batchRunID, bundle, request.EnvironmentID, request.WorkflowID, plans, runtime, bundle.FailureCategories, collector)
	return report, http.StatusAccepted, nil
}

func apiCaseBatchPlanningBundle(ctx context.Context, runtime store.Store, fallback profile.Bundle) profile.Bundle {
	if runtime == nil {
		return fallback
	}
	catalog, err := runtime.GetProfileCatalog(ctx)
	if err != nil || strings.TrimSpace(catalog.ProfileID) == "" {
		return fallback
	}
	refreshed := profilecatalog.ToBundle(catalog)
	if len(refreshed.FailureCategories) == 0 && len(fallback.FailureCategories) > 0 && strings.TrimSpace(refreshed.ID) == strings.TrimSpace(fallback.ID) {
		refreshed.FailureCategories = append([]profile.FailureCategoryRule(nil), fallback.FailureCategories...)
	}
	return refreshed
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

func handleAPICaseBatchRunReport(w http.ResponseWriter, r *http.Request, runner *apiCaseBatchRunner) {
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
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"ok": false, "error": "batch run not found"})
		return
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
