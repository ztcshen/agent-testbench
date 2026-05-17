package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"open-test-sandbox/internal/profile"
	"open-test-sandbox/internal/store"
)

type caseSuiteCoverageFilter struct {
	Filter   string   `json:"filter,omitempty"`
	NodeID   string   `json:"nodeId,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Status   string   `json:"status,omitempty"`
	Owner    string   `json:"owner,omitempty"`
	Priority string   `json:"priority,omitempty"`
}

type caseSuiteCoverageCounts struct {
	Total  int `json:"total"`
	Passed int `json:"passed"`
	Failed int `json:"failed"`
	NotRun int `json:"notRun"`
}

type caseSuiteCoverageItem struct {
	CaseID       string   `json:"caseId"`
	Title        string   `json:"title"`
	Description  string   `json:"description,omitempty"`
	NodeID       string   `json:"nodeId,omitempty"`
	NodeName     string   `json:"nodeName,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	Priority     string   `json:"priority,omitempty"`
	Owner        string   `json:"owner,omitempty"`
	LatestStatus string   `json:"latestStatus"`
	LatestRunID  string   `json:"latestRunId,omitempty"`
	CaseRunID    string   `json:"caseRunId,omitempty"`
	DetailURL    string   `json:"detailUrl,omitempty"`
	ElapsedMs    int64    `json:"elapsedMs,omitempty"`
	HasPassed    bool     `json:"hasPassed"`
	Reason       string   `json:"reason,omitempty"`
}

type caseSuiteCoverageReport struct {
	OK          bool                    `json:"ok"`
	ProfileID   string                  `json:"profileId"`
	GeneratedAt string                  `json:"generatedAt"`
	Filters     caseSuiteCoverageFilter `json:"filters"`
	Counts      caseSuiteCoverageCounts `json:"counts"`
	Items       []caseSuiteCoverageItem `json:"items"`
	Warnings    []string                `json:"warnings,omitempty"`
}

func handleCaseSuiteCoverage(w http.ResponseWriter, r *http.Request, bundle profile.Bundle, runtime store.Store) {
	filter := caseSuiteCoverageFilterFromRequest(r)
	items := selectedSuiteCoverageCases(bundle, filter)
	report, err := suiteCoverageReport(r.Context(), bundle, runtime, filter, items)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, report)
}

func caseSuiteCoverageFilterFromRequest(r *http.Request) caseSuiteCoverageFilter {
	query := r.URL.Query()
	return normalizeSuiteCoverageFilter(caseSuiteCoverageFilter{
		Filter:   query.Get("filter"),
		NodeID:   firstNonEmpty(query.Get("node"), query.Get("nodeId")),
		Tags:     queryStringList(query["tag"], query["tags"]),
		Status:   firstNonEmpty(query.Get("status"), "active"),
		Owner:    query.Get("owner"),
		Priority: query.Get("priority"),
	})
}

func suiteCoverageReport(ctx context.Context, bundle profile.Bundle, runtime store.Store, filter caseSuiteCoverageFilter, cases []profile.APICase) (caseSuiteCoverageReport, error) {
	report := caseSuiteCoverageReport{
		OK:          true,
		ProfileID:   bundle.ID,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Filters:     normalizeSuiteCoverageFilter(filter),
		Counts:      caseSuiteCoverageCounts{Total: len(cases)},
		Items:       []caseSuiteCoverageItem{},
	}
	if runtime == nil {
		report.OK = len(cases) == 0
		report.Counts.NotRun = len(cases)
		report.Warnings = append(report.Warnings, "runtime store is not configured")
	}
	records, err := suiteCoverageRecords(ctx, runtime, suiteCoverageCaseIDs(cases))
	if err != nil {
		return caseSuiteCoverageReport{}, err
	}
	stateByCase := suiteCoverageStateByCase(records)
	nodesByID := map[string]profile.InterfaceNode{}
	for _, node := range bundle.InterfaceNodes {
		nodesByID[node.ID] = node
	}
	for _, item := range cases {
		state := stateByCase[item.ID]
		node := nodesByID[item.NodeID]
		row := caseSuiteCoverageItem{
			CaseID:      item.ID,
			Title:       firstNonEmpty(item.DisplayName, item.ID),
			Description: item.Description,
			NodeID:      item.NodeID,
			NodeName:    firstNonEmpty(node.DisplayName, item.NodeID),
			Tags:        append([]string(nil), item.Tags...),
			Priority:    item.Priority,
			Owner:       item.Owner,
			HasPassed:   state.HasPassed,
		}
		if state.Latest.CaseRun.ID == "" {
			row.LatestStatus = "not-run"
			row.Reason = "no run recorded in Store"
			report.Counts.NotRun++
			report.OK = false
		} else {
			row.LatestStatus = state.Latest.CaseRun.Status
			row.LatestRunID = state.Latest.Run.ID
			row.CaseRunID = state.Latest.CaseRun.ID
			row.DetailURL = apiCaseEvidenceDetailURL(row.CaseRunID)
			row.ElapsedMs = elapsedMillis(state.Latest.CaseRun.StartedAt, state.Latest.CaseRun.FinishedAt)
			if isPassedStatus(state.Latest.CaseRun.Status) {
				report.Counts.Passed++
			} else {
				report.Counts.Failed++
				report.OK = false
				row.Reason = firstNonEmpty(assertionSummaryReason(state.Latest.CaseRun.AssertionSummaryJSON), "latest run is "+state.Latest.CaseRun.Status)
			}
		}
		report.Items = append(report.Items, row)
	}
	return report, nil
}

type suiteCoverageState struct {
	Latest    store.APICaseRunRecord
	HasPassed bool
}

func suiteCoverageStateByCase(records []store.APICaseRunRecord) map[string]suiteCoverageState {
	out := map[string]suiteCoverageState{}
	for _, record := range records {
		caseID := record.CaseRun.CaseID
		state := out[caseID]
		if isPassedStatus(record.CaseRun.Status) {
			state.HasPassed = true
		}
		if state.Latest.CaseRun.ID == "" || suiteCoverageRecordNewer(record, state.Latest) {
			state.Latest = record
		}
		out[caseID] = state
	}
	return out
}

func suiteCoverageRecordNewer(left store.APICaseRunRecord, right store.APICaseRunRecord) bool {
	if left.CaseRun.CreatedAt.After(right.CaseRun.CreatedAt) {
		return true
	}
	return left.CaseRun.CreatedAt.Equal(right.CaseRun.CreatedAt) && left.CaseRun.ID > right.CaseRun.ID
}

func suiteCoverageRecords(ctx context.Context, runtime store.Store, caseIDs []string) ([]store.APICaseRunRecord, error) {
	if runtime == nil || len(caseIDs) == 0 {
		return []store.APICaseRunRecord{}, nil
	}
	if fast, ok := runtime.(interface {
		ListAPICaseRunRecordsForCaseIDs(context.Context, []string) ([]store.APICaseRunRecord, error)
	}); ok {
		return fast.ListAPICaseRunRecordsForCaseIDs(ctx, caseIDs)
	}
	caseSet := map[string]bool{}
	for _, id := range caseIDs {
		caseSet[id] = true
	}
	runs, err := runtime.ListRuns(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]store.APICaseRunRecord, 0)
	for _, run := range runs {
		caseRuns, err := runtime.ListAPICaseRuns(ctx, run.ID)
		if err != nil {
			return nil, err
		}
		for _, caseRun := range caseRuns {
			if caseSet[caseRun.CaseID] {
				out = append(out, store.APICaseRunRecord{Run: run, CaseRun: caseRun})
			}
		}
	}
	return out, nil
}

func selectedSuiteCoverageCases(bundle profile.Bundle, filter caseSuiteCoverageFilter) []profile.APICase {
	out := make([]profile.APICase, 0)
	for _, item := range bundle.APICases {
		if suiteCoverageCaseMatches(item, filter) {
			out = append(out, item)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].NodeID != out[j].NodeID {
			return out[i].NodeID < out[j].NodeID
		}
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func suiteCoverageCaseMatches(item profile.APICase, filter caseSuiteCoverageFilter) bool {
	filter = normalizeSuiteCoverageFilter(filter)
	if filter.NodeID != "" && item.NodeID != filter.NodeID {
		return false
	}
	if filter.Status != "" && !strings.EqualFold(suiteCoverageCaseStatus(item), filter.Status) {
		return false
	}
	if filter.Owner != "" && !strings.EqualFold(strings.TrimSpace(item.Owner), filter.Owner) {
		return false
	}
	if filter.Priority != "" && !strings.EqualFold(strings.TrimSpace(item.Priority), filter.Priority) {
		return false
	}
	if len(filter.Tags) > 0 && !suiteCoverageHasAllTags(item.Tags, filter.Tags) {
		return false
	}
	return suiteCoverageMatchesText(filter.Filter, item.ID, item.DisplayName, item.Description, item.Scenario, item.Owner, item.Priority, strings.Join(item.Tags, " "), item.NodeID)
}

func normalizeSuiteCoverageFilter(filter caseSuiteCoverageFilter) caseSuiteCoverageFilter {
	filter.Filter = strings.TrimSpace(filter.Filter)
	filter.NodeID = strings.TrimSpace(filter.NodeID)
	filter.Status = strings.TrimSpace(filter.Status)
	filter.Owner = strings.TrimSpace(filter.Owner)
	filter.Priority = strings.TrimSpace(filter.Priority)
	filter.Tags = normalizeQueryStringList(filter.Tags)
	return filter
}

func suiteCoverageCaseStatus(item profile.APICase) string {
	if strings.TrimSpace(item.Status) == "" {
		return "active"
	}
	return item.Status
}

func suiteCoverageHasAllTags(actual []string, required []string) bool {
	actualSet := map[string]bool{}
	for _, tag := range actual {
		if normalized := suiteCoverageSearchText(tag); normalized != "" {
			actualSet[normalized] = true
		}
	}
	for _, tag := range required {
		if normalized := suiteCoverageSearchText(tag); normalized != "" && !actualSet[normalized] {
			return false
		}
	}
	return true
}

func suiteCoverageMatchesText(filter string, values ...string) bool {
	needle := suiteCoverageSearchText(filter)
	if needle == "" {
		return true
	}
	for _, value := range values {
		haystack := suiteCoverageSearchText(value)
		if haystack != "" && (strings.Contains(haystack, needle) || strings.Contains(needle, haystack)) {
			return true
		}
	}
	return false
}

func suiteCoverageSearchText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "-", "", "_", "", ".", "", "/", "")
	return replacer.Replace(value)
}

func queryStringList(groups ...[]string) []string {
	out := []string{}
	for _, group := range groups {
		out = append(out, group...)
	}
	return normalizeQueryStringList(out)
}

func normalizeQueryStringList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			key := strings.ToLower(part)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, part)
		}
	}
	return out
}

func suiteCoverageCaseIDs(cases []profile.APICase) []string {
	out := make([]string, 0, len(cases))
	for _, item := range cases {
		if strings.TrimSpace(item.ID) != "" {
			out = append(out, item.ID)
		}
	}
	return out
}

func assertionSummaryReason(summaryJSON string) string {
	var payload struct {
		FailureReason string `json:"failureReason"`
		ErrorCount    int    `json:"errorCount"`
	}
	if json.Unmarshal([]byte(summaryJSON), &payload) != nil {
		return ""
	}
	if strings.TrimSpace(payload.FailureReason) != "" {
		return payload.FailureReason
	}
	if payload.ErrorCount > 0 {
		return fmt.Sprintf("assertion errors: %d", payload.ErrorCount)
	}
	return ""
}

func elapsedMillis(started time.Time, finished time.Time) int64 {
	if started.IsZero() || finished.IsZero() || finished.Before(started) {
		return 0
	}
	return finished.Sub(started).Milliseconds()
}
