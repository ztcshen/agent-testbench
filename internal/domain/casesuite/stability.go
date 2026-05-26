package casesuite

import (
	"context"
	"sort"
	"time"

	"agent-testbench/internal/domain/execution"
	"agent-testbench/internal/domain/profile"
)

type StabilityOptions struct {
	Limit int `json:"limit,omitempty"`
}

type StabilityCounts struct {
	Total    int `json:"total"`
	Stable   int `json:"stable"`
	Unstable int `json:"unstable"`
	NotRun   int `json:"notRun"`
	Passed   int `json:"passed"`
	Failed   int `json:"failed"`
}

type StabilityRecentRun struct {
	RunID     string `json:"runId"`
	CaseRunID string `json:"caseRunId"`
	Status    string `json:"status"`
	DetailURL string `json:"detailUrl,omitempty"`
	ElapsedMs int64  `json:"elapsedMs,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
}

type StabilityItem struct {
	CaseID       string               `json:"caseId"`
	Title        string               `json:"title"`
	Description  string               `json:"description,omitempty"`
	NodeID       string               `json:"nodeId,omitempty"`
	NodeName     string               `json:"nodeName,omitempty"`
	Tags         []string             `json:"tags,omitempty"`
	Priority     string               `json:"priority,omitempty"`
	Owner        string               `json:"owner,omitempty"`
	LatestStatus string               `json:"latestStatus"`
	Passed       int                  `json:"passed"`
	Failed       int                  `json:"failed"`
	Transitions  int                  `json:"transitions"`
	Unstable     bool                 `json:"unstable"`
	Reason       string               `json:"reason,omitempty"`
	Recent       []StabilityRecentRun `json:"recent,omitempty"`
}

type StabilityReport struct {
	OK          bool             `json:"ok"`
	ProfileID   string           `json:"profileId"`
	GeneratedAt string           `json:"generatedAt"`
	Filters     Filter           `json:"filters"`
	Options     StabilityOptions `json:"options"`
	Counts      StabilityCounts  `json:"counts"`
	Items       []StabilityItem  `json:"items"`
	Warnings    []string         `json:"warnings,omitempty"`
}

func Stability(ctx context.Context, bundle profile.Bundle, runtime RecordStore, filter Filter, cases []profile.APICase, options StabilityOptions) (StabilityReport, error) {
	filter = NormalizeFilter(filter)
	if options.Limit <= 0 {
		options.Limit = 10
	}
	report := StabilityReport{
		OK:          true,
		ProfileID:   bundle.ID,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Filters:     filter,
		Options:     options,
		Counts:      StabilityCounts{Total: len(cases)},
		Items:       []StabilityItem{},
	}
	if runtime == nil {
		report.OK = len(cases) == 0
		report.Counts.NotRun = len(cases)
		report.Warnings = append(report.Warnings, "runtime store is not configured")
	}
	records, err := RecordsForCaseIDs(ctx, runtime, CaseIDs(cases))
	if err != nil {
		return StabilityReport{}, err
	}
	recordsByCase := recordsGroupedByCase(records)
	nodesByID := interfaceNodesByID(bundle.InterfaceNodes)
	for _, item := range cases {
		row := stabilityItemForCase(item, nodesByID[item.NodeID], recordsByCase[item.ID], options.Limit)
		applyStabilityCounts(&report, row)
		report.Items = append(report.Items, row)
	}
	return report, nil
}

func stabilityItemForCase(item profile.APICase, node profile.InterfaceNode, records []execution.APICaseRunRecord, limit int) StabilityItem {
	row := baseStabilityItem(item, node)
	if len(records) == 0 {
		row.Reason = ReasonNoRunRecorded
		return row
	}
	if len(records) > limit {
		records = records[:limit]
	}
	row.Recent = stabilityRecentRuns(records)
	row.LatestStatus = NormalizeRunState(records[0].CaseRun.Status)
	addStabilityRunStats(&row, records)
	row.Unstable = row.Passed > 0 && row.Failed > 0 && row.Transitions > 0
	if row.Unstable {
		row.Reason = "recent runs include both passed and failed results"
	}
	return row
}

func baseStabilityItem(item profile.APICase, node profile.InterfaceNode) StabilityItem {
	return StabilityItem{
		CaseID:       item.ID,
		Title:        firstNonEmpty(item.DisplayName, item.ID),
		Description:  item.Description,
		NodeID:       item.NodeID,
		NodeName:     firstNonEmpty(node.DisplayName, item.NodeID),
		Tags:         append([]string(nil), item.Tags...),
		Priority:     item.Priority,
		Owner:        item.Owner,
		LatestStatus: "not-run",
	}
}

func addStabilityRunStats(row *StabilityItem, records []execution.APICaseRunRecord) {
	for index, record := range records {
		status := NormalizeRunState(record.CaseRun.Status)
		switch status {
		case execution.StatusPassed:
			row.Passed++
		case execution.StatusFailed:
			row.Failed++
		}
		if index > 0 && status != NormalizeRunState(records[index-1].CaseRun.Status) {
			row.Transitions++
		}
	}
}

func applyStabilityCounts(report *StabilityReport, row StabilityItem) {
	switch {
	case row.Reason == ReasonNoRunRecorded:
		report.Counts.NotRun++
		report.OK = false
	case row.Unstable:
		report.Counts.Unstable++
		report.OK = false
	default:
		report.Counts.Stable++
	}
	if row.LatestStatus == execution.StatusPassed {
		report.Counts.Passed++
	}
	if row.LatestStatus == execution.StatusFailed {
		report.Counts.Failed++
	}
}

func recordsGroupedByCase(records []execution.APICaseRunRecord) map[string][]execution.APICaseRunRecord {
	out := map[string][]execution.APICaseRunRecord{}
	for _, record := range records {
		caseID := record.CaseRun.CaseID
		out[caseID] = append(out[caseID], record)
	}
	for caseID := range out {
		sort.SliceStable(out[caseID], func(i, j int) bool {
			return RecordNewer(out[caseID][i], out[caseID][j])
		})
	}
	return out
}

func stabilityRecentRuns(records []execution.APICaseRunRecord) []StabilityRecentRun {
	out := make([]StabilityRecentRun, 0, len(records))
	for _, record := range records {
		out = append(out, StabilityRecentRun{
			RunID:     record.Run.ID,
			CaseRunID: record.CaseRun.ID,
			Status:    NormalizeRunState(record.CaseRun.Status),
			DetailURL: DetailURL(record.CaseRun.ID),
			ElapsedMs: ElapsedMs(record.CaseRun.StartedAt, record.CaseRun.FinishedAt),
			CreatedAt: record.CaseRun.CreatedAt.Format(time.RFC3339Nano),
		})
	}
	return out
}
