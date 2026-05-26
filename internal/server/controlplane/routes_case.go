package controlplane

import "net/http"

func registerCaseRoutes(mux *http.ServeMux, deps routeDeps) {
	runtime := deps.runtime
	profiles := deps.profiles
	collector := deps.collector
	caseBatchRunner := deps.caseBatchRunner
	handleMethod(mux, "/api/case/runs", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleCaseRuns(w, r, profiles.Current(), runtime)
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
	handleMethod(mux, "/api/case/incomplete-batches", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleCaseIncompleteBatches(w, r, profiles.Current(), runtime)
	})
	handleMethod(mux, "/api/case/suite-coverage", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleCaseSuiteCoverage(w, r, profiles.Current(), runtime)
	})
	handleMethod(mux, "/api/case/suite-inspection", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleCaseSuiteInspection(w, r, profiles.Current(), runtime)
	})
	handleMethod(mux, "/api/case/suite-plan", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleCaseSuitePlan(w, r, profiles.Current(), runtime)
	})
	handleMethod(mux, "/api/case/suite-stability", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleCaseSuiteStability(w, r, profiles.Current(), runtime)
	})
	handleMethod(mux, "/api/case/suite-priority", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleCaseSuitePriority(w, r, profiles.Current(), runtime)
	})
	handleMethod(mux, "/api/case/suite-brief", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleCaseSuiteBrief(w, r, profiles.Current(), runtime)
	})
	handleMethod(mux, "/api/case/suite-quality", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleCaseSuiteQuality(w, r, profiles.Current(), runtime)
	})
	handleMethod(mux, "/api/case/suite-quality-plan", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleCaseSuiteQualityPlan(w, r, profiles.Current(), runtime)
	})
	handleMethod(mux, "/api/case/suite-impact", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleCaseSuiteImpact(w, r, profiles.Current(), runtime)
	})
	handleMethod(mux, "/api/case/suite-impact-runs", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		handleCaseSuiteImpactRun(w, r, profiles.Current(), runtime, caseBatchRunner, collector)
	})
	handleMethod(mux, "/api/replay/evidence", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleReplayEvidence(w, r)
	})
	handleMethod(mux, "/api/cases/capabilities", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		payload, err := apiCaseCapabilitiesFromBundleWithStore(r.Context(), profiles.Current(), runtime)
		if err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, payload)
	})
	handleMethod(mux, "/api/cases/run", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		handleAPICaseRun(w, r, profiles.Current(), runtime)
	})
	handleMethod(mux, "/api/cases/batch-runs", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		handleAPICaseBatchRunStart(w, r, profiles.Current(), runtime, caseBatchRunner, collector)
	})
	handleMethod(mux, "/api/cases/batch-runs/", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleAPICaseBatchRunReport(w, r, caseBatchRunner)
	})
	handleMethod(mux, "/api/test-kit/run", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		handleTestKitRun(w, r, profiles.Current(), runtime, collector)
	})
	handleMethod(mux, "/api/test-kit/run-batch", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		handleTestKitRunBatch(w, r, profiles.Current(), runtime)
	})
}
