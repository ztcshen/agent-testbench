package controlplane

import (
	"net/http"
	"strings"

	"open-test-sandbox/internal/profile"
	"open-test-sandbox/internal/store"
)

func handleAgentTestWorkbench(w http.ResponseWriter, r *http.Request, bundle profile.Bundle, runtime store.Store) {
	payload := agentTestEmptyPayload()
	if runtime == nil {
		writeJSON(w, payload)
		return
	}
	runs, err := runtime.ListRuns(r.Context())
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	agentRuns := agentTestRuns(runs)
	profiles := agentTestProfiles(bundle)
	capabilities := agentTestCapabilities()
	payload["capabilities"] = capabilities
	payload["profiles"] = profiles
	payload["agentRuns"] = agentRuns
	payload["summary"] = agentTestSummary(agentRuns, capabilities, profiles)
	writeJSON(w, payload)
}

func agentTestEmptyPayload() map[string]any {
	return map[string]any{
		"ok": true,
		"summary": map[string]any{
			"capabilityCount":         0,
			"profileCount":            0,
			"runCount":                0,
			"configEventCount":        0,
			"escalationEventCount":    0,
			"acceptanceReportCount":   0,
			"statusCounts":            map[string]int{},
			"failureKinds":            map[string]int{},
			"latestFailureKind":       "no active failure",
			"latestAcceptanceVerdict": "",
			"latestAcceptanceStatus":  "",
		},
		"capabilities":      []map[string]any{},
		"profiles":          []map[string]any{},
		"agentRuns":         []map[string]any{},
		"configEvents":      []map[string]any{},
		"escalationEvents":  []map[string]any{},
		"acceptanceReports": []map[string]any{},
		"warnings":          []string{},
	}
}

func agentTestCapabilities() []map[string]any {
	return []map[string]any{
		{
			"id":          "evidence-index",
			"title":       "Evidence Diagnosis Index",
			"status":      "available",
			"description": "Run summaries expose diagnosis, evidence roots, status counts, and next-step hints.",
			"evidence":    []string{"runs.summary_json", "evidence_records"},
		},
		{
			"id":          "profile-workbench",
			"title":       "Profile Workbench",
			"status":      "available",
			"description": "Active profile metadata is available for local-first run review.",
			"evidence":    []string{"profile.json", "profile_index"},
		},
		{
			"id":          "case-evidence",
			"title":       "API Case Evidence",
			"status":      "available",
			"description": "API case runs and evidence records are linked from Store data.",
			"evidence":    []string{"api_case_runs", "evidence_records"},
		},
	}
}

func agentTestProfiles(bundle profile.Bundle) []map[string]any {
	if strings.TrimSpace(bundle.ID) == "" || bundle.Counts() == (profile.Counts{}) {
		return []map[string]any{}
	}
	return []map[string]any{{
		"id":             bundle.ID,
		"title":          firstNonEmpty(bundle.DisplayName, bundle.ID),
		"stepCount":      len(bundle.Workflows),
		"workflowCount":  len(bundle.Workflows),
		"caseCount":      len(bundle.APICases),
		"requiredConfig": []map[string]any{},
		"evidenceKinds":  []string{"runs", "evidence"},
		"allowedChanges": []map[string]any{},
	}}
}

func agentTestRuns(runs []store.Run) []map[string]any {
	items := make([]map[string]any, 0, len(runs))
	for i := len(runs) - 1; i >= 0; i-- {
		run := runs[i]
		diagnosis := agentTestDiagnosis(run.SummaryJSON)
		failureKind := agentTestFailureKind(run, diagnosis)
		items = append(items, map[string]any{
			"id":                run.ID,
			"runId":             run.ID,
			"repoPath":          "",
			"resolvedServiceId": run.WorkflowID,
			"workflowId":        run.WorkflowID,
			"ref":               "",
			"commitId":          "",
			"profileId":         run.ProfileID,
			"status":            run.Status,
			"failureKind":       failureKind,
			"evidenceRoot":      run.EvidenceRoot,
			"diagnosis":         diagnosis,
			"blockedReport":     nil,
			"startedAt":         run.StartedAt,
			"endedAt":           run.FinishedAt,
			"createdAt":         run.CreatedAt,
		})
	}
	return items
}

func agentTestDiagnosis(summaryJSON string) map[string]any {
	summary := jsonObject(summaryJSON)
	if diagnosis, ok := summary["diagnosisIndex"].(map[string]any); ok {
		return diagnosis
	}
	if nested, ok := summary["summary"].(map[string]any); ok {
		if diagnosis, ok := nested["diagnosisIndex"].(map[string]any); ok {
			return diagnosis
		}
	}
	return map[string]any{}
}

func agentTestFailureKind(run store.Run, diagnosis map[string]any) string {
	if kind := valueString(diagnosis["failureKind"]); kind != "" {
		return kind
	}
	summary := jsonObject(run.SummaryJSON)
	if kind := firstNonEmpty(valueString(summary["failureKind"]), valueString(summary["failure_kind"])); kind != "" {
		return kind
	}
	if run.Status == store.StatusFailed {
		return store.StatusFailed
	}
	return ""
}

func agentTestSummary(runs []map[string]any, capabilities []map[string]any, profiles []map[string]any) map[string]any {
	statusCounts := map[string]int{}
	failureKinds := map[string]int{}
	for _, run := range runs {
		statusCounts[firstNonEmpty(valueString(run["status"]), "unknown")]++
		if kind := valueString(run["failureKind"]); kind != "" {
			failureKinds[kind]++
		}
	}
	latestFailureKind := "no active failure"
	if len(runs) > 0 {
		if kind := valueString(runs[0]["failureKind"]); kind != "" {
			latestFailureKind = kind
		}
	}
	return map[string]any{
		"capabilityCount":         len(capabilities),
		"profileCount":            len(profiles),
		"runCount":                len(runs),
		"configEventCount":        0,
		"escalationEventCount":    0,
		"acceptanceReportCount":   0,
		"statusCounts":            statusCounts,
		"failureKinds":            failureKinds,
		"latestFailureKind":       latestFailureKind,
		"latestAcceptanceVerdict": "",
		"latestAcceptanceStatus":  "",
	}
}
