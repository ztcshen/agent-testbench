package casesuite

import (
	"context"
	"sort"
	"strings"
	"time"

	"agent-testbench/internal/domain/execution"
	"agent-testbench/internal/domain/profile"
)

type PriorityOptions struct {
	Signals        []string `json:"signals,omitempty"`
	Limit          int      `json:"limit,omitempty"`
	RequestID      string   `json:"requestId,omitempty"`
	BaseURL        string   `json:"baseUrl,omitempty"`
	EvidenceDir    string   `json:"evidenceDir,omitempty"`
	TimeoutSeconds int      `json:"timeoutSeconds,omitempty"`
}

type PriorityCounts struct {
	Total    int `json:"total"`
	Ready    int `json:"ready"`
	Blocked  int `json:"blocked"`
	Selected int `json:"selected"`
	Skipped  int `json:"skipped"`
}

type PriorityItem struct {
	InspectionItem
	Score   int      `json:"score"`
	Reasons []string `json:"reasons,omitempty"`
}

type PriorityReport struct {
	OK           bool            `json:"ok"`
	ProfileID    string          `json:"profileId"`
	GeneratedAt  string          `json:"generatedAt"`
	Filters      Filter          `json:"filters"`
	Options      PriorityOptions `json:"options"`
	Counts       PriorityCounts  `json:"counts"`
	CaseIDs      []string        `json:"caseIds"`
	Selected     []PriorityItem  `json:"selected"`
	Skipped      []PriorityItem  `json:"skipped"`
	Blocked      []PriorityItem  `json:"blocked"`
	BatchRequest BatchRequest    `json:"batchRequest"`
	Warnings     []string        `json:"warnings,omitempty"`
}

func Priority(ctx context.Context, bundle profile.Bundle, runtime RecordStore, filter Filter, cases []profile.APICase, options PriorityOptions) (PriorityReport, error) {
	filter = NormalizeFilter(filter)
	options.Signals = NormalizeStringList(options.Signals)
	inspection, err := Inspect(ctx, bundle, runtime, filter, cases)
	if err != nil {
		return PriorityReport{}, err
	}
	stability, err := Stability(ctx, bundle, runtime, filter, cases, StabilityOptions{Limit: 10})
	if err != nil {
		return PriorityReport{}, err
	}
	return priorityFromParts(bundle, filter, inspection, stability, options), nil
}

func priorityFromParts(bundle profile.Bundle, filter Filter, inspection InspectionReport, stability StabilityReport, options PriorityOptions) PriorityReport {
	filter = NormalizeFilter(filter)
	options.Signals = NormalizeStringList(options.Signals)
	impact := collectImpact(bundle, options.Signals)
	scored, blocked := scoredPriorityItems(inspection.Items, stabilityItemsByCase(stability.Items), impact.caseReasons)
	report := PriorityReport{
		OK:          true,
		ProfileID:   bundle.ID,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Filters:     filter,
		Options:     options,
		Warnings:    appendUniqueStrings(nil, inspection.Warnings...),
	}
	report.Warnings = appendUniqueStrings(report.Warnings, stability.Warnings...)
	report.Blocked = blocked
	sortPriorityItems(scored)
	limit := priorityLimit(options.Limit, len(scored))
	report.Selected = append(report.Selected, scored[:limit]...)
	report.Skipped = append(report.Skipped, scored[limit:]...)
	for _, item := range report.Selected {
		report.CaseIDs = append(report.CaseIDs, item.CaseID)
	}
	report.Counts = PriorityCounts{
		Total:    len(inspection.Items),
		Ready:    len(scored),
		Blocked:  len(report.Blocked),
		Selected: len(report.Selected),
		Skipped:  len(report.Skipped),
	}
	report.BatchRequest = newBatchRequest(report.CaseIDs, options.RequestID, options.BaseURL, options.EvidenceDir, options.TimeoutSeconds)
	if len(report.CaseIDs) == 0 {
		report.OK = false
		report.Warnings = append(report.Warnings, "no ready cases selected for prioritized execution")
	}
	return report
}

func scoredPriorityItems(items []InspectionItem, stabilityByCase map[string]StabilityItem, impactReasons map[string][]string) ([]PriorityItem, []PriorityItem) {
	scored := make([]PriorityItem, 0, len(items))
	blocked := []PriorityItem{}
	for _, item := range items {
		row := PriorityItem{InspectionItem: item}
		row.Score, row.Reasons = priorityScore(item, stabilityByCase[item.CaseID], impactReasons[item.CaseID])
		if item.Ready {
			scored = append(scored, row)
		} else {
			blocked = append(blocked, row)
		}
	}
	return scored, blocked
}

func stabilityItemsByCase(items []StabilityItem) map[string]StabilityItem {
	out := map[string]StabilityItem{}
	for _, item := range items {
		out[item.CaseID] = item
	}
	return out
}

func sortPriorityItems(items []PriorityItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score != items[j].Score {
			return items[i].Score > items[j].Score
		}
		if items[i].Priority != items[j].Priority {
			return priorityWeight(items[i].Priority) > priorityWeight(items[j].Priority)
		}
		if items[i].NodeID != items[j].NodeID {
			return items[i].NodeID < items[j].NodeID
		}
		return items[i].CaseID < items[j].CaseID
	})
}

func priorityLimit(requested int, available int) int {
	if requested <= 0 || requested > available {
		return available
	}
	return requested
}

func priorityScore(item InspectionItem, stability StabilityItem, impactReasons []string) (int, []string) {
	score := 0
	reasons := []string{}
	if len(impactReasons) > 0 {
		score += 100
		reasons = append(reasons, "impacted")
	}
	switch NormalizeRunState(item.LatestStatus) {
	case execution.StatusFailed:
		score += 60
		reasons = append(reasons, "latest failed")
	case "not-run":
		score += 30
		reasons = append(reasons, "not run")
	case execution.StatusPassed:
		score += 5
		reasons = append(reasons, "latest passed")
	}
	if stability.Unstable {
		score += 40
		reasons = append(reasons, "unstable")
	}
	if weight := priorityWeight(item.Priority); weight > 0 {
		score += weight
		reasons = append(reasons, "priority "+strings.ToLower(strings.TrimSpace(item.Priority)))
	}
	if !item.Ready {
		score -= 1000
		reasons = append(reasons, "blocked")
	}
	return score, reasons
}

func priorityWeight(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "p0", "0", "critical":
		return 30
	case "p1", "1", "high":
		return 20
	case "p2", "2", "medium":
		return 10
	default:
		return 0
	}
}
