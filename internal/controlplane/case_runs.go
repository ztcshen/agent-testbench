package controlplane

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"open-test-sandbox/internal/store"
)

func handleCaseRuns(w http.ResponseWriter, r *http.Request, runtime store.Store) {
	if runtime == nil {
		writeJSON(w, emptyCaseRunsPayload())
		return
	}
	runs, err := runtime.ListRuns(r.Context())
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	items := make([]map[string]any, 0)
	for i := len(runs) - 1; i >= 0; i-- {
		run := runs[i]
		caseRuns, err := runtime.ListAPICaseRuns(r.Context(), run.ID)
		if err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		evidence, err := runtime.ListEvidence(r.Context(), run.ID)
		if err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		for j := len(caseRuns) - 1; j >= 0; j-- {
			items = append(items, caseRunItem(run, caseRuns[j], evidence))
		}
	}
	writeJSON(w, map[string]any{
		"ok":       true,
		"caseRuns": items,
		"warnings": []string{},
	})
}

func emptyCaseRunsPayload() map[string]any {
	return map[string]any{
		"ok":       true,
		"caseRuns": []map[string]any{},
		"warnings": []string{},
	}
}

func caseRunItem(run store.Run, item store.APICaseRun, evidence []store.EvidenceRecord) map[string]any {
	request := jsonObject(item.RequestSummaryJSON)
	assertion := jsonObject(item.AssertionSummaryJSON)
	operation := caseRunOperation(request, item.CaseID)
	evidenceCount := 0
	for _, record := range evidence {
		if record.CaseRunID == item.ID {
			evidenceCount++
		}
	}
	return map[string]any{
		"id":            item.ID,
		"runId":         item.RunID,
		"caseId":        item.CaseID,
		"status":        item.Status,
		"operation":     operation,
		"evidencePath":  run.EvidenceRoot,
		"evidenceCount": evidenceCount,
		"updatedAt":     latestTime(item.CreatedAt, run.UpdatedAt, run.CreatedAt),
		"failureReason": caseRunFailureReason(assertion),
	}
}

func jsonObject(raw string) map[string]any {
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{}
	}
	return out
}

func caseRunOperation(summary map[string]any, fallback string) string {
	method := strings.ToUpper(valueString(summary["method"]))
	path := valueString(summary["path"])
	if method != "" && path != "" {
		return method + " " + path
	}
	if method != "" {
		return method
	}
	if path != "" {
		return path
	}
	return fallback
}

func caseRunFailureReason(assertion map[string]any) string {
	status := strings.ToLower(valueString(assertion["status"]))
	if status == "" || status == store.StatusPassed {
		return ""
	}
	if count := valueString(assertion["errorCount"]); count != "" && count != "0" {
		return "assertion errors: " + count
	}
	return "assertion status: " + status
}

func latestTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}
