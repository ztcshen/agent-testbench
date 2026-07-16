package controlplane

import (
	"net/http"

	"agent-testbench/internal/domain/profile"
)

func registerCaseRoutes(mux *http.ServeMux, deps routeDeps) {
	runtime := deps.runtime
	collector := deps.collector
	caseBatchRunner := deps.caseBatchRunner
	registerCaseCatalogMaintenanceRoutes(mux, runtime)
	handleCurrentProfileMethod(mux, "/api/case/runs", http.MethodGet, deps, func(w http.ResponseWriter, r *http.Request, bundle profile.Bundle) {
		handleCaseRuns(w, r, bundle, runtime)
	})
	handleMethod(mux, "/api/case/evidence", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleCaseEvidence(w, r, runtime)
	})
	handleMethod(mux, "/api/case-run/evidence", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleCaseRunEvidence(w, r, runtime)
	})
	handleMethod(mux, "/api/case/timing", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleCaseTiming(w, r, runtime)
	})
	handleMethod(mux, "/api/post-process-tasks", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handlePostProcessTasks(w, r, runtime)
	})
	handleCurrentProfileMethod(mux, "/api/case/incomplete-batches", http.MethodGet, deps, func(w http.ResponseWriter, r *http.Request, bundle profile.Bundle) {
		handleCaseIncompleteBatches(w, r, bundle, runtime)
	})
	handleCurrentProfileMethod(mux, "/api/case/suite-coverage", http.MethodGet, deps, func(w http.ResponseWriter, r *http.Request, bundle profile.Bundle) {
		handleCaseSuiteCoverage(w, r, bundle, runtime)
	})
	handleCurrentProfileMethod(mux, "/api/case/suite-inspection", http.MethodGet, deps, func(w http.ResponseWriter, r *http.Request, bundle profile.Bundle) {
		handleCaseSuiteInspection(w, r, bundle, runtime)
	})
	handleCurrentProfileMethod(mux, "/api/case/suite-plan", http.MethodGet, deps, func(w http.ResponseWriter, r *http.Request, bundle profile.Bundle) {
		handleCaseSuitePlan(w, r, bundle, runtime)
	})
	handleCurrentProfileMethod(mux, "/api/case/suite-stability", http.MethodGet, deps, func(w http.ResponseWriter, r *http.Request, bundle profile.Bundle) {
		handleCaseSuiteStability(w, r, bundle, runtime)
	})
	handleCurrentProfileMethod(mux, "/api/case/suite-priority", http.MethodGet, deps, func(w http.ResponseWriter, r *http.Request, bundle profile.Bundle) {
		handleCaseSuitePriority(w, r, bundle, runtime)
	})
	handleCurrentProfileMethod(mux, "/api/case/suite-brief", http.MethodGet, deps, func(w http.ResponseWriter, r *http.Request, bundle profile.Bundle) {
		handleCaseSuiteBrief(w, r, bundle, runtime)
	})
	handleCurrentProfileMethod(mux, "/api/case/suite-quality", http.MethodGet, deps, func(w http.ResponseWriter, r *http.Request, bundle profile.Bundle) {
		handleCaseSuiteQuality(w, r, bundle, runtime)
	})
	handleCurrentProfileMethod(mux, "/api/case/suite-quality-plan", http.MethodGet, deps, func(w http.ResponseWriter, r *http.Request, bundle profile.Bundle) {
		handleCaseSuiteQualityPlan(w, r, bundle, runtime)
	})
	handleCurrentProfileMethod(mux, "/api/case/suite-impact", http.MethodGet, deps, func(w http.ResponseWriter, r *http.Request, bundle profile.Bundle) {
		handleCaseSuiteImpact(w, r, bundle, runtime)
	})
	handleCurrentProfileMethod(mux, "/api/case/suite-impact-runs", http.MethodPost, deps, func(w http.ResponseWriter, r *http.Request, bundle profile.Bundle) {
		handleCaseSuiteImpactRun(w, r, bundle, runtime, caseBatchRunner, collector)
	})
	handleMethod(mux, "/api/replay/evidence", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleReplayEvidence(w, r)
	})
	handleMethod(mux, "/api/cases/capabilities", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		bundle := deps.profiles.Current()
		catalogRevision := int64(0)
		var err error
		if deps.caseCatalogSnapshot != nil {
			bundle = *deps.caseCatalogSnapshot
		} else {
			bundle, catalogRevision, err = currentProfileBundleSnapshot(r.Context(), runtime, bundle)
			if err != nil {
				writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
		}
		payload, err := apiCaseCapabilitiesFromBundleWithRuns(r.Context(), bundle, catalogRevision, runtime)
		if err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, payload)
	})
	handleCurrentProfileMethod(mux, "/api/cases/run", http.MethodPost, deps, func(w http.ResponseWriter, r *http.Request, bundle profile.Bundle) {
		handleAPICaseRun(w, r, bundle, runtime)
	})
	handleCurrentProfileMethod(mux, "/api/cases/batch-runs", http.MethodPost, deps, func(w http.ResponseWriter, r *http.Request, bundle profile.Bundle) {
		handleAPICaseBatchRunStart(w, r, bundle, runtime, caseBatchRunner, collector)
	})
	handleMethod(mux, "/api/cases/batch-runs/", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleAPICaseBatchRunReport(w, r, runtime, caseBatchRunner)
	})
	handleCurrentProfileMethod(mux, "/api/test-kit/run", http.MethodPost, deps, func(w http.ResponseWriter, r *http.Request, bundle profile.Bundle) {
		handleTestKitRun(w, r, bundle, runtime, collector)
	})
	handleCurrentProfileMethod(mux, "/api/test-kit/run-batch", http.MethodPost, deps, func(w http.ResponseWriter, r *http.Request, bundle profile.Bundle) {
		handleTestKitRunBatch(w, r, bundle, runtime)
	})
}

func handleCurrentProfileMethod(mux *http.ServeMux, path string, method string, deps routeDeps, handler func(http.ResponseWriter, *http.Request, profile.Bundle)) {
	handleMethod(mux, path, method, func(w http.ResponseWriter, r *http.Request) {
		if deps.caseCatalogSnapshot != nil {
			handler(w, r, *deps.caseCatalogSnapshot)
			return
		}
		bundle, err := currentProfileBundle(r.Context(), deps.runtime, deps.profiles.Current())
		if err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		handler(w, r, bundle)
	})
}
