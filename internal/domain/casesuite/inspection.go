package casesuite

import (
	"context"
	"strings"

	"agent-testbench/internal/domain/execution"
	"agent-testbench/internal/domain/profile"
)

func Inspect(ctx context.Context, bundle profile.Bundle, runtime RecordStore, filter Filter, cases []profile.APICase) (InspectionReport, error) {
	coverage, err := Coverage(ctx, bundle, runtime, filter, cases)
	if err != nil {
		return InspectionReport{}, err
	}
	configs := ExecutionConfigSet(ctx, bundle, runtime)
	coverageByCase := coverageItemsByCase(coverage.Items)
	report := newInspectionReport(bundle.ID, coverage, len(cases))
	for _, item := range cases {
		row := inspectionRow(item, coverageByCase[item.ID], configs[item.ID])
		addInspectionRow(&report, row)
	}
	return report, nil
}

func newInspectionReport(profileID string, coverage Report, total int) InspectionReport {
	report := InspectionReport{
		OK:          true,
		ProfileID:   profileID,
		GeneratedAt: coverage.GeneratedAt,
		Filters:     coverage.Filters,
		Counts:      InspectionCounts{Total: total},
		Items:       []InspectionItem{},
		Warnings:    append([]string(nil), coverage.Warnings...),
	}
	if total == 0 {
		report.OK = false
		report.Warnings = append(report.Warnings, "no cases matched selector")
	}
	return report
}

func coverageItemsByCase(items []Item) map[string]Item {
	out := make(map[string]Item, len(items))
	for _, item := range items {
		out[item.CaseID] = item
	}
	return out
}

func inspectionRow(item profile.APICase, coverageItem Item, hasExecutionConfig bool) InspectionItem {
	status := CaseStatus(item)
	row := InspectionItem{
		CaseID:             item.ID,
		Title:              firstNonEmpty(item.DisplayName, item.ID),
		Description:        item.Description,
		NodeID:             item.NodeID,
		NodeName:           coverageItem.NodeName,
		Tags:               append([]string(nil), item.Tags...),
		Priority:           item.Priority,
		Owner:              item.Owner,
		Status:             status,
		HasRunnableFile:    strings.TrimSpace(item.CasePath) != "",
		HasExecutionConfig: hasExecutionConfig,
		LatestStatus:       firstNonEmpty(coverageItem.LatestStatus, "not-run"),
		LatestRunID:        coverageItem.LatestRunID,
		CaseRunID:          coverageItem.CaseRunID,
		DetailURL:          coverageItem.DetailURL,
		ElapsedMs:          coverageItem.ElapsedMs,
		HasPassed:          coverageItem.HasPassed,
	}
	row.Issues = inspectionIssues(row)
	row.Ready = len(row.Issues) == 0
	row.SuggestedAction = SuggestedAction(row)
	return row
}

func inspectionIssues(row InspectionItem) []string {
	var issues []string
	if !IsExecutableCaseLifecycle(row.Status) {
		issues = append(issues, "case status is "+row.Status)
	}
	if !row.HasRunnableFile && !row.HasExecutionConfig {
		issues = append(issues, "missing runnable case file or execution config")
	}
	return issues
}

func addInspectionRow(report *InspectionReport, row InspectionItem) {
	if row.Ready {
		report.Counts.Ready++
	} else {
		report.OK = false
		report.Counts.Blocked++
	}
	if !IsExecutableCaseLifecycle(row.Status) {
		report.Counts.Inactive++
	}
	if !row.HasRunnableFile {
		report.Counts.MissingRunnable++
	}
	if !row.HasExecutionConfig {
		report.Counts.MissingExecution++
	}
	switch NormalizeRunState(row.LatestStatus) {
	case execution.StatusPassed:
		report.Counts.Passed++
	case execution.StatusFailed:
		report.Counts.Failed++
	default:
		report.Counts.NotRun++
	}
	report.Items = append(report.Items, row)
}
